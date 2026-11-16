//go:build windows

package tun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestIPHelperLayouts pins the hand written structures to the 64 bit Windows
// layout. A wrong offset here corrupts the forwarding table or the interface
// properties, so the sizes are checked before any API call uses them.
func TestIPHelperLayouts(t *testing.T) {
	sizes := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"SOCKADDR_INET", unsafe.Sizeof(sockaddrInet{}), 28},
		{"IP_ADDRESS_PREFIX", unsafe.Sizeof(ipAddressPrefix{}), 32},
		{"MIB_IPFORWARD_ROW2", unsafe.Sizeof(mibIPForwardRow2{}), 104},
		{"MIB_IPFORWARD_TABLE2", unsafe.Sizeof(mibIPForwardTable2{}), 112},
		{"MIB_UNICASTIPADDRESS_ROW", unsafe.Sizeof(mibUnicastIPAddressRow{}), 80},
	}
	for _, size := range sizes {
		if size.got != size.want {
			t.Errorf("%s has size %d, want %d", size.name, size.got, size.want)
		}
	}

	offsets := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"MIB_IPFORWARD_ROW2.DestinationPrefix", unsafe.Offsetof(mibIPForwardRow2{}.DestinationPrefix), 12},
		{"MIB_IPFORWARD_ROW2.NextHop", unsafe.Offsetof(mibIPForwardRow2{}.NextHop), 44},
		{"MIB_IPFORWARD_ROW2.Metric", unsafe.Offsetof(mibIPForwardRow2{}.Metric), 84},
		{"MIB_IPFORWARD_ROW2.Origin", unsafe.Offsetof(mibIPForwardRow2{}.Origin), 100},
		{"MIB_UNICASTIPADDRESS_ROW.InterfaceLuid", unsafe.Offsetof(mibUnicastIPAddressRow{}.InterfaceLuid), 32},
		{"MIB_UNICASTIPADDRESS_ROW.OnLinkPrefixLength", unsafe.Offsetof(mibUnicastIPAddressRow{}.OnLinkPrefixLength), 60},
		{"MIB_IPINTERFACE_ROW.InterfaceLuid", unsafe.Offsetof(mibIPInterfaceRow{}.InterfaceLuid), 8},
		{"MIB_IPINTERFACE_ROW.InterfaceIndex", unsafe.Offsetof(mibIPInterfaceRow{}.InterfaceIndex), 16},
		{"MIB_IPINTERFACE_ROW.ZoneIndices", unsafe.Offsetof(mibIPInterfaceRow{}.ZoneIndices), 80},
		{"MIB_IPINTERFACE_ROW.SitePrefixLength", unsafe.Offsetof(mibIPInterfaceRow{}.SitePrefixLength), 144},
		{"MIB_IPINTERFACE_ROW.Metric", unsafe.Offsetof(mibIPInterfaceRow{}.Metric), 148},
		{"MIB_IPINTERFACE_ROW.NlMtu", unsafe.Offsetof(mibIPInterfaceRow{}.NlMtu), 152},
	}
	for _, offset := range offsets {
		if offset.got != offset.want {
			t.Errorf("%s is at offset %d, want %d", offset.name, offset.got, offset.want)
		}
	}
}

// TestSockaddrRoundTrip covers the union encoding that every route call uses.
func TestSockaddrRoundTrip(t *testing.T) {
	for _, text := range []string{"10.66.0.1", "192.168.1.1", "2001:db8::1", "::1"} {
		want := net.ParseIP(text)
		address := sockaddrFromIP(want)
		if got := ipFromSockaddr(&address); !got.Equal(want) {
			t.Errorf("%s round tripped to %s", text, got)
		}
	}
	if family := sockaddrFromIP(net.ParseIP("10.66.0.1")).Family; family != windows.AF_INET {
		t.Errorf("IPv4 family is %d", family)
	}
	if family := sockaddrFromIP(net.ParseIP("::1")).Family; family != windows.AF_INET6 {
		t.Errorf("IPv6 family is %d", family)
	}
}

// TestFormatDriverVersion checks the DWORD encodings used by Wintun.
func TestFormatDriverVersion(t *testing.T) {
	cases := map[uint32]string{
		0x000e0001: "0.14.1", // packed major<<24 | minor<<16 | patch
		0x00000e01: "0.14.1", // classic three component form
		0x0000000e: "0.14",   // dwFileVersionMS stores only major and minor
		0:          "0.0.0",
	}
	for version, want := range cases {
		if got := formatDriverVersion(version); got != want {
			t.Errorf("formatDriverVersion(%#08x) = %q, want %q", version, got, want)
		}
	}
}

// TestWintunDownload verifies the pinned archive and installs the DLL for the
// remaining tests. It is skipped when the machine is offline.
func TestWintunDownload(t *testing.T) {
	if testing.Short() {
		t.Skip("download test disabled in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	data, err := downloadWintunDLL(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "SHA-256") {
			t.Fatalf("the downloaded DLL failed verification: %v", err)
		}
		t.Skipf("the wintun archive could not be downloaded: %v", err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != wintunDLLSHA256 {
		t.Fatalf("downloaded DLL SHA-256 is %s, want %s", got, wintunDLLSHA256)
	}

	path, err := InstallWintun(ctx)
	if err != nil {
		t.Fatalf("InstallWintun after a successful download: %v", err)
	}
	if !strings.HasSuffix(strings.ToLower(path), wintunDLLName) {
		t.Fatalf("InstallWintun returned %q", path)
	}
	again, err := InstallWintun(ctx)
	if err != nil {
		t.Fatalf("InstallWintun is not idempotent: %v", err)
	}
	if again != path {
		t.Fatalf("InstallWintun returned %q and then %q", path, again)
	}
}

// TestWintunStatus checks the status call the desktop application uses.
func TestWintunStatus(t *testing.T) {
	path, version, err := WintunStatus()
	if err != nil {
		if !errors.Is(err, ErrWintunUnavailable) {
			t.Fatalf("WintunStatus: %v", err)
		}
		if path != "" {
			t.Fatalf("WintunStatus reported path %q together with the missing error", path)
		}
		t.Skipf("wintun.dll is not installed: %v", err)
	}
	if path == "" {
		t.Fatal("WintunStatus returned an empty path without an error")
	}
	if version == "" {
		t.Logf("wintun.dll is installed at %s but the driver is not running", path)
		return
	}
	running, err := WintunVersion()
	if err != nil {
		t.Fatalf("WintunVersion after WintunStatus reported %q: %v", version, err)
	}
	if running != version {
		t.Fatalf("WintunStatus reported %q but WintunVersion returned %q", version, running)
	}
}

// TestWintunVersion exercises WintunGetRunningDriverVersion. Without
// administrator rights the driver is usually not installed, which is a
// skippable condition rather than a failure.
func TestWintunVersion(t *testing.T) {
	version, err := WintunVersion()
	if err != nil {
		if errors.Is(err, ErrWintunNotRunning) {
			t.Skipf("the wintun driver is not running: %v", err)
		}
		t.Fatalf("WintunVersion: %v", err)
	}
	if len(strings.Split(version, ".")) != 3 {
		t.Fatalf("unexpected version %q", version)
	}
}

// TestDefaultRoute is a read-only check that the forwarding table and alias
// conversion decode correctly on this machine.
func TestDefaultRoute(t *testing.T) {
	dev, gateway, err := DefaultRoute()
	if err != nil {
		t.Skipf("no physical default route on this machine: %v", err)
	}
	if dev == "" {
		t.Fatal("DefaultRoute returned an empty interface")
	}
	t.Logf("physical default route: dev=%s gateway=%s", dev, gateway)
}

// TestIPInterfaceRow reads the interface properties back with the hand
// written MIB_IPINTERFACE_ROW. It needs no administrator rights and would
// fail with a shifted structure.
func TestIPInterfaceRow(t *testing.T) {
	dev, _, err := DefaultRoute()
	if err != nil {
		t.Skipf("no physical default route on this machine: %v", err)
	}
	luid, ifIndex, err := interfaceIdentity(dev)
	if err != nil {
		t.Fatalf("resolve %q: %v", dev, err)
	}
	row, err := getIPInterfaceRow(luid, ifIndex, windows.AF_INET)
	if err != nil {
		t.Fatalf("get the interface entry: %v", err)
	}
	if row.Family != windows.AF_INET {
		t.Fatalf("family is %d", row.Family)
	}
	if row.InterfaceLuid != luid || row.InterfaceIndex != ifIndex {
		t.Fatalf("interface identity does not round trip: luid=%d index=%d", row.InterfaceLuid, row.InterfaceIndex)
	}
	if row.NlMtu == 0 {
		t.Fatal("the interface MTU decoded as zero")
	}
}

// TestOpenRequiresAdministrator checks that a non elevated process gets a
// useful error instead of a crash when the driver cannot be installed.
func TestOpenRequiresAdministrator(t *testing.T) {
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("the process is elevated; adapter creation would succeed")
	}
	if _, err := WintunPath(); err != nil {
		if _, err := InstallWintun(context.Background()); err != nil {
			t.Skipf("wintun.dll is not available: %v", err)
		}
	}
	name := fmt.Sprintf("W1nWireTest-%d", os.Getpid())
	device, err := Open(name, 1420)
	if err == nil {
		_ = device.Close()
		t.Fatal("Open created an adapter without administrator rights")
	}
	message := err.Error()
	if !strings.Contains(message, "Access is denied") && !strings.Contains(message, "administrator") {
		t.Fatalf("the error does not explain the missing rights: %v", err)
	}
}

// TestResetInterfaceMissing verifies that resetting a missing adapter is a
// no-op, which is what the application calls on every start.
func TestResetInterfaceMissing(t *testing.T) {
	if path, err := routeStatePath(); err == nil {
		if _, err := os.Stat(path); err == nil {
			t.Skip("a previous process left route state, resetting it would need administrator rights")
		}
	}
	name := fmt.Sprintf("W1nWireTest-Missing-%d", os.Getpid())
	if err := ResetInterface(name); err != nil {
		t.Fatalf("ResetInterface on a missing adapter: %v", err)
	}
}
