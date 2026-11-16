//go:build windows

package winutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	swShowNormal = 1
	runKey       = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName = "W1nWire"
)

var (
	modShell32  = windows.NewLazySystemDLL("shell32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")
	modUser32   = windows.NewLazySystemDLL("user32.dll")

	procShellExecuteW    = modShell32.NewProc("ShellExecuteW")
	procCreateMutexW     = modKernel32.NewProc("CreateMutexW")
	procGetModuleHandleW = modKernel32.NewProc("GetModuleHandleW")
	procLoadIconW        = modUser32.NewProc("LoadIconW")
	procSendMessageW     = modUser32.NewProc("SendMessageW")
)

const (
	wmSetIcon = 0x0080
	iconSmall = 0
	iconBig   = 1
)

// SetWindowIcon applies the icon embedded in the executable to a window. The
// resource id 1 is the one that the resource generator writes.
func SetWindowIcon(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	hinstance, _, _ := procGetModuleHandleW.Call(0)
	icon, _, _ := procLoadIconW.Call(hinstance, 1)
	if icon == 0 {
		return
	}
	procSendMessageW.Call(hwnd, wmSetIcon, iconBig, icon)
	procSendMessageW.Call(hwnd, wmSetIcon, iconSmall, icon)
}

// isElevated reports whether the token of the process is elevated.
func isElevated() bool {
	token := windows.GetCurrentProcessToken()
	return token.IsElevated()
}

// RelaunchElevated starts a new instance of the application through the UAC
// prompt and leaves the current one running until the caller exits.
//
// The extra arguments are appended to the command line of the new process;
// the caller uses them to tell the new instance that it replaces the current
// one instead of being a forbidden second copy.
func RelaunchElevated(extraArgs ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("winutil: locate the executable: %w", err)
	}
	verb, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	path, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	cwd, err := syscall.UTF16PtrFromString(filepath.Dir(exe))
	if err != nil {
		return err
	}
	var params *uint16
	if len(extraArgs) > 0 {
		joined, err := syscall.UTF16PtrFromString(strings.Join(extraArgs, " "))
		if err != nil {
			return err
		}
		params = joined
	}
	ret, _, callErr := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(path)),
		uintptr(unsafe.Pointer(params)),
		uintptr(unsafe.Pointer(cwd)),
		swShowNormal,
	)
	// ShellExecute returns a value greater than 32 on success.
	if ret <= 32 {
		if callErr != syscall.Errno(0) {
			return fmt.Errorf("winutil: elevate: %w", callErr)
		}
		return fmt.Errorf("winutil: elevate: shell execute returned %d (the request was cancelled)", ret)
	}
	return nil
}

// OpenURL opens a URL in the default browser.
func OpenURL(url string) error {
	return shellOpen(url)
}

// OpenPath opens a file or a folder in the shell.
func OpenPath(path string) error {
	return shellOpen(path)
}

func shellOpen(target string) error {
	verb, err := syscall.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	arg, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	ret, _, callErr := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(arg)),
		0,
		0,
		swShowNormal,
	)
	if ret <= 32 {
		if callErr != syscall.Errno(0) {
			return fmt.Errorf("winutil: open %q: %w", target, callErr)
		}
		return fmt.Errorf("winutil: open %q: shell execute returned %d", target, ret)
	}
	return nil
}

// SetAutoStart adds or removes the application from the user autostart list.
func SetAutoStart(enabled bool) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("winutil: open the Run key: %w", err)
	}
	defer key.Close()

	if !enabled {
		if err := key.DeleteValue(runValueName); err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("winutil: remove the autostart entry: %w", err)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("winutil: locate the executable: %w", err)
	}
	if err := key.SetStringValue(runValueName, `"`+exe+`"`); err != nil {
		return fmt.Errorf("winutil: write the autostart entry: %w", err)
	}
	return nil
}

// SingleInstance creates a named mutex. It returns ok=false when another
// instance of the application already holds it, and a release function that
// must be called before the process exits.
func SingleInstance(name string) (release func(), ok bool) {
	ptr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return func() {}, true
	}
	handle, _, callErr := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(ptr)))
	if handle == 0 {
		// The mutex could not be created at all; running is better than
		// refusing to start.
		return func() {}, true
	}
	if callErr == windows.ERROR_ALREADY_EXISTS {
		windows.CloseHandle(windows.Handle(handle))
		return func() {}, false
	}
	release = func() {
		windows.CloseHandle(windows.Handle(handle))
	}
	return release, true
}

// ConsoleTitle is not used on Windows desktop builds but keeps the API
// symmetrical with the other platforms.
func ConsoleTitle(string) {}
