//go:build windows

package tun

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const (
	// wintunDLLName is the name of the userspace library that implements the
	// adapter and the packet rings.
	wintunDLLName = "wintun.dll"
	// wintunVersion is the release the download URL and the hash below pin.
	wintunVersion = "0.14.1"
	// wintunDownloadURL is the official build archive. The binary is GPLv2,
	// so it is downloaded on demand instead of being shipped with the EXE.
	wintunDownloadURL = "https://www.wintun.net/builds/wintun-0.14.1.zip"
	// wintunDLLSHA256 is the SHA-256 of bin/amd64/wintun.dll inside the
	// official wintun-0.14.1.zip. It is checked before the file is installed
	// so a corrupted or replaced download cannot be loaded.
	wintunDLLSHA256 = "e5da8447dc2c320edc0fc52fa01885c103de8c118481f683643cacc3220dafce"
	// wintunZipEntry is the path of the 64 bit DLL inside the archive.
	wintunZipEntry = "bin/amd64/wintun.dll"
	// wintunPool is the tunnel type that is shown in the device manager.
	wintunPool = "W1nWire"
	// wintunRingCapacity is the size of the receive and send rings. Four
	// megabytes absorb bursts on fast links without stalling the reader.
	wintunRingCapacity = 0x400000
	// readWaitTimeout is how long Read waits for the read event before it
	// checks the closed flag. A short timeout keeps Close responsive while
	// avoiding a busy loop.
	readWaitTimeout = 200
	// sendRetries is how often Write retries a full send ring.
	sendRetries = 10
	// maxZipSize and maxDLLSize bound the memory used while installing.
	maxZipSize = 8 << 20
	maxDLLSize = 4 << 20
)

// Errors that tell a caller whether wintun is usable at all.
var (
	// ErrWintunUnavailable means that no wintun.dll was found in any of the
	// searched locations.
	ErrWintunUnavailable = errors.New("tun: wintun.dll is not installed")
	// ErrWintunNotRunning means that the DLL is present but the kernel driver
	// is not active, which is normal until an administrator creates an
	// adapter for the first time.
	ErrWintunNotRunning = errors.New("tun: the wintun driver is not running")
)

// WintunAdapter is an opaque WINTUN_ADAPTER_HANDLE. Handles are pointer sized
// on every supported Windows architecture.
type WintunAdapter uintptr

// WintunSession is an opaque WINTUN_SESSION_HANDLE.
type WintunSession uintptr

// WintunPacket is an opaque WINTUN_PACKET_HANDLE. It points into a ring
// buffer that belongs to Wintun and is only valid until the packet is
// released.
type WintunPacket uintptr

// LUID is a NET_LUID, the stable identifier of a network interface.
type LUID uint64

// adapterGUID is fixed so that a stale adapter from a previous run can be
// identified and reopened instead of accumulating differently named devices.
var adapterGUID = windows.GUID{
	Data1: 0x8f6b0c71,
	Data2: 0x3d4a,
	Data3: 0x4c1e,
	Data4: [8]byte{0x9a, 0x2f, 0x6b, 0x7d, 0x41, 0xc8, 0x5e, 0x03},
}

// wintunBindings holds the procedures of one loaded wintun.dll. The DLL stays
// loaded for the lifetime of the process; unloading it while a session exists
// would invalidate the ring buffers.
type wintunBindings struct {
	createAdapter        *windows.Proc
	openAdapter          *windows.Proc
	closeAdapter         *windows.Proc
	getAdapterLUID       *windows.Proc
	runningDriverVersion *windows.Proc
	startSession         *windows.Proc
	endSession           *windows.Proc
	getReadWaitEvent     *windows.Proc
	receivePacket        *windows.Proc
	releaseReceivePacket *windows.Proc
	allocateSendPacket   *windows.Proc
	sendPacket           *windows.Proc
}

var (
	wintunMu       sync.Mutex
	wintunInstance *wintunBindings
)

// WintunPath returns the path of an installed wintun.dll.
//
// It searches next to the executable first, then the per user directory and
// finally System32. The first location wins, which lets a user override a
// system wide copy without touching the registry.
func WintunPath() (string, error) {
	for _, path := range wintunSearchPaths() {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path, nil
		}
	}
	dir, err := wintunInstallDir()
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrWintunUnavailable, err)
	}
	return "", fmt.Errorf("%w: place %s next to the executable or in %s", ErrWintunUnavailable, wintunDLLName, dir)
}

// InstallWintun returns the path of an installed wintun.dll, downloading the
// pinned official build when no copy exists yet.
//
// The archive is fetched over HTTPS, the SHA-256 of the extracted DLL is
// verified, and the file is moved into place with a rename so that a crash
// never leaves a truncated DLL behind.
func InstallWintun(ctx context.Context) (string, error) {
	if path, err := WintunPath(); err == nil {
		return path, nil
	}
	dir, err := wintunInstallDir()
	if err != nil {
		return "", fmt.Errorf("tun: determine the wintun install directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("tun: create %s: %w", dir, err)
	}
	data, err := downloadWintunDLL(ctx)
	if err != nil {
		return "", fmt.Errorf("tun: install wintun %s: %w (download %s manually and place %s in %s or next to the executable)", wintunVersion, err, wintunDownloadURL, wintunDLLName, dir)
	}
	target := filepath.Join(dir, wintunDLLName)
	if err := writeFileAtomic(target, data); err != nil {
		return "", err
	}
	return target, nil
}

// WintunVersion returns the version of the running Wintun driver, formatted as
// major.minor.patch or major.minor when the driver only exposes two
// components (for example 0x0000000e becomes "0.14").
//
// It returns ErrWintunNotRunning when the DLL is installed but no driver is
// active, which is the normal state before the first elevated adapter
// creation.
func WintunVersion() (string, error) {
	path, err := WintunPath()
	if err != nil {
		return "", err
	}
	return wintunVersionAt(path)
}

// WintunStatus reports the installed DLL and the running driver version for a
// GUI. A missing DLL is an error wrapping ErrWintunUnavailable, while an
// installed DLL whose driver is not running is not: the path is reported and
// the version is left empty.
func WintunStatus() (path string, version string, err error) {
	path, err = WintunPath()
	if err != nil {
		return "", "", err
	}
	version, err = wintunVersionAt(path)
	if err != nil {
		if errors.Is(err, ErrWintunNotRunning) {
			return path, "", nil
		}
		return path, "", err
	}
	return path, version, nil
}

// ensureWintun installs the DLL if necessary and returns its bindings.
func ensureWintun() (*wintunBindings, error) {
	path, err := InstallWintun(context.Background())
	if err != nil {
		return nil, err
	}
	return loadWintun(path)
}

// loadWintun loads the DLL at path once and resolves every procedure. Missing
// exports are reported instead of panicking, which keeps a downgraded DLL from
// crashing the process.
func loadWintun(path string) (*wintunBindings, error) {
	wintunMu.Lock()
	defer wintunMu.Unlock()
	if wintunInstance != nil {
		return wintunInstance, nil
	}

	dll, err := windows.LoadDLL(path)
	if err != nil {
		return nil, fmt.Errorf("tun: load %s: %w", path, err)
	}
	fail := func(err error) (*wintunBindings, error) {
		_ = dll.Release()
		return nil, err
	}

	bindings := &wintunBindings{}
	procedures := []struct {
		name string
		dst  **windows.Proc
	}{
		{"WintunCreateAdapter", &bindings.createAdapter},
		{"WintunOpenAdapter", &bindings.openAdapter},
		{"WintunCloseAdapter", &bindings.closeAdapter},
		{"WintunGetAdapterLUID", &bindings.getAdapterLUID},
		{"WintunGetRunningDriverVersion", &bindings.runningDriverVersion},
		{"WintunStartSession", &bindings.startSession},
		{"WintunEndSession", &bindings.endSession},
		{"WintunGetReadWaitEvent", &bindings.getReadWaitEvent},
		{"WintunReceivePacket", &bindings.receivePacket},
		{"WintunReleaseReceivePacket", &bindings.releaseReceivePacket},
		{"WintunAllocateSendPacket", &bindings.allocateSendPacket},
		{"WintunSendPacket", &bindings.sendPacket},
	}
	for _, procedure := range procedures {
		proc, err := dll.FindProc(procedure.name)
		if err != nil {
			return fail(fmt.Errorf("tun: %s does not provide %s: %w", path, procedure.name, err))
		}
		*procedure.dst = proc
	}
	wintunInstance = bindings
	return bindings, nil
}

// wintunVersionAt queries the driver version using an already installed DLL.
func wintunVersionAt(path string) (string, error) {
	api, err := loadWintun(path)
	if err != nil {
		return "", err
	}
	version, _, callErr := api.runningDriverVersion.Call()
	if version == 0 {
		return "", fmt.Errorf("%w: %v", ErrWintunNotRunning, callError(callErr))
	}
	return formatDriverVersion(uint32(version)), nil
}

// formatDriverVersion turns the packed version DWORD into text.
//
// WintunGetRunningDriverVersion returns the driver file's dwFileVersionMS.
// The Windows version resource stores that as major<<16 | minor, so 0.14.1
// arrives as 0x0000000e (the patch is not part of that field). Some builds
// and the Wintun documentation instead pack major<<24 | minor<<16 | patch,
// where 0.14.1 is 0x000e0001. Decode the packed form when the upper half is
// non zero, the classic three component form when the minor byte is present,
// and the two component form when only major/minor are encoded.
func formatDriverVersion(version uint32) string {
	switch {
	case version == 0:
		return "0.0.0"
	case version>>16 != 0:
		return fmt.Sprintf("%d.%d.%d", (version>>24)&0xff, (version>>16)&0xff, version&0xffff)
	case (version>>8)&0xff != 0:
		return fmt.Sprintf("%d.%d.%d", (version>>16)&0xff, (version>>8)&0xff, version&0xff)
	default:
		return fmt.Sprintf("%d.%d", (version>>16)&0xff, version&0xffff)
	}
}

// adapterError explains the common failure of a non elevated first start.
func adapterError(name string, callErr error) error {
	err := callError(callErr)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return fmt.Errorf("tun: create the wintun adapter %q: %w: installing the wintun driver requires administrator rights", name, err)
	}
	return fmt.Errorf("tun: create the wintun adapter %q: %w", name, err)
}

// callError converts the always non nil result of Proc.Call into an error.
// Errno zero means the API failed without setting a meaningful code.
func callError(err error) error {
	if errno, ok := err.(syscall.Errno); ok {
		if errno == 0 {
			return errors.New("unknown error")
		}
		return errno
	}
	if err == nil {
		return errors.New("unknown error")
	}
	return err
}

// wintunSearchPaths returns the places that may contain a usable DLL.
func wintunSearchPaths() []string {
	var paths []string
	if executable, err := os.Executable(); err == nil {
		if absolute, err := filepath.Abs(executable); err == nil {
			executable = absolute
		}
		paths = append(paths, filepath.Join(filepath.Dir(executable), wintunDLLName))
	}
	if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
		paths = append(paths, filepath.Join(dir, "W1nWire", wintunDLLName))
	}
	if root := os.Getenv("SystemRoot"); root != "" {
		paths = append(paths, filepath.Join(root, "System32", wintunDLLName))
	}
	return paths
}

// wintunInstallDir is the per user directory the automatic installer uses.
// The executable directory is not used because it is often read only.
func wintunInstallDir() (string, error) {
	dir := os.Getenv("LOCALAPPDATA")
	if dir == "" {
		return "", errors.New("LOCALAPPDATA is not set")
	}
	return filepath.Join(dir, "W1nWire"), nil
}

// downloadWintunDLL fetches the official archive, extracts the 64 bit DLL and
// verifies its hash. It returns the bytes without touching the filesystem so
// the caller controls where the DLL is installed.
func downloadWintunDLL(ctx context.Context) ([]byte, error) {
	if runtime.GOARCH != "amd64" {
		return nil, fmt.Errorf("automatic installation is only supported on amd64, install %s manually", wintunDLLName)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, wintunDownloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build the download request: %w", err)
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", wintunDownloadURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: unexpected HTTP status %s", wintunDownloadURL, response.Status)
	}
	archive, err := io.ReadAll(io.LimitReader(response.Body, maxZipSize+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", wintunDownloadURL, err)
	}
	if len(archive) > maxZipSize {
		return nil, fmt.Errorf("the archive is larger than %d bytes", maxZipSize)
	}

	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("open the wintun archive: %w", err)
	}
	for _, file := range reader.File {
		if !strings.HasSuffix(strings.ToLower(filepath.ToSlash(file.Name)), wintunZipEntry) {
			continue
		}
		source, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s in the archive: %w", wintunZipEntry, err)
		}
		data, err := io.ReadAll(io.LimitReader(source, maxDLLSize+1))
		source.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s from the archive: %w", wintunZipEntry, err)
		}
		if len(data) > maxDLLSize {
			return nil, fmt.Errorf("%s is larger than %d bytes", wintunZipEntry, maxDLLSize)
		}
		if err := verifyWintunDLL(data); err != nil {
			return nil, err
		}
		return data, nil
	}
	return nil, fmt.Errorf("the archive does not contain %s", wintunZipEntry)
}

// verifyWintunDLL compares the DLL against the pinned official build.
func verifyWintunDLL(data []byte) error {
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != wintunDLLSHA256 {
		return fmt.Errorf("the downloaded %s has SHA-256 %x, want %s", wintunDLLName, sum, wintunDLLSHA256)
	}
	return nil
}

// writeFileAtomic installs data at path with a temporary file and a rename.
// A crash leaves either the old file or the complete new one, never a partial
// DLL that would fail to load.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".wintun-*.tmp")
	if err != nil {
		return fmt.Errorf("tun: create a temporary file in %s: %w", dir, err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("tun: write %s: %w", tempName, err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("tun: sync %s: %w", tempName, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("tun: close %s: %w", tempName, err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("tun: install %s: %w", path, err)
	}
	return nil
}

// waitTimeout is WAIT_TIMEOUT from the Windows wait API. x/sys/windows does not
// export it, and it is not an error: it only means the read event was not
// signaled within the short poll interval.
const waitTimeout = 258
