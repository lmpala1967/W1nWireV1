//go:build windows

package tun

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The constants below are from netioapi.h and nldef.h. They are kept as
// numbers so the file does not depend on undocumented x/sys aliases.
const (
	// routeMetric is the metric of every route this package installs. It is
	// lower than the automatic metric of physical interfaces, but the split
	// default routes win by prefix length anyway.
	routeMetric = 1
	// ifMaxStringSize is IF_MAX_STRING_SIZE; aliases are at most 255 WCHARs.
	ifMaxStringSize = 256
)

// sockaddrInet mirrors SOCKADDR_INET from ws2ipdef.h. It is a 28 byte union
// of SOCKADDR_IN and SOCKADDR_IN6. Data holds the union payload starting at
// byte four: for IPv4 the first four bytes are the address, for IPv6 the
// first four bytes are the flow label, the next sixteen the address and the
// last four the scope ID.
type sockaddrInet struct {
	Family uint16
	Port   uint16
	Data   [6]uint32
}

// ipAddressPrefix mirrors IP_ADDRESS_PREFIX from netioapi.h. The three byte
// tail keeps the structure at 32 bytes, which is what the Windows ABI uses
// before the next member.
type ipAddressPrefix struct {
	Prefix       sockaddrInet
	PrefixLength uint8
	_            [3]byte
}

// mibIPForwardRow2 mirrors MIB_IPFORWARD_ROW2 from netioapi.h. The layout is
// identical on 64 bit Windows: LUID at 0, interface index at 8, destination
// prefix at 12, next hop at 44, metric at 84 and the protocol at 88. The
// total size is 104.
type mibIPForwardRow2 struct {
	InterfaceLuid        uint64
	InterfaceIndex       uint32
	DestinationPrefix    ipAddressPrefix
	NextHop              sockaddrInet
	SitePrefixLength     uint8
	ValidLifetime        uint32
	PreferredLifetime    uint32
	Metric               uint32
	Protocol             uint32
	Loopback             uint8
	AutoconfigureAddress uint8
	Publish              uint8
	Immortal             uint8
	Age                  uint32
	Origin               uint32
}

// mibIPForwardTable2 mirrors MIB_IPFORWARD_TABLE2. The table starts with a
// count followed by an array of rows; the explicit padding puts the first row
// at offset eight, where the Windows SDK places it.
type mibIPForwardTable2 struct {
	NumEntries uint32
	_          [4]byte
	Table      [1]mibIPForwardRow2
}

// rows returns the variable length array that follows NumEntries.
func (t *mibIPForwardTable2) rows() []mibIPForwardRow2 {
	return unsafe.Slice(&t.Table[0], t.NumEntries)
}

// mibUnicastIPAddressRow mirrors MIB_UNICASTIPADDRESS_ROW from netioapi.h.
// Address is 28 bytes, the LUID is aligned to eight and the final timestamp
// leaves the structure at 80 bytes.
type mibUnicastIPAddressRow struct {
	Address            sockaddrInet
	_                  [4]byte
	InterfaceLuid      uint64
	InterfaceIndex     uint32
	PrefixOrigin       uint32
	SuffixOrigin       uint32
	ValidLifetime      uint32
	PreferredLifetime  uint32
	OnLinkPrefixLength uint8
	SkipAsSource       uint8
	_                  [2]byte
	DadState           uint32
	ScopeId            uint32
	CreationTimeStamp  int64
}

// mibIPInterfaceRow mirrors MIB_IPINTERFACE_ROW from netioapi.h. The fields
// through NlMtu are the ones this package changes; the trailing padding keeps
// the structure at least as large as the SDK definition, because the exact
// size of the offload bitfields differs between SDK revisions.
type mibIPInterfaceRow struct {
	Family                               uint16
	_                                    [6]byte
	InterfaceLuid                        uint64
	InterfaceIndex                       uint32
	MaxReassemblySize                    uint32
	InterfaceIdentifier                  uint64
	MinRouterAdvertisementInterval       uint32
	MaxRouterAdvertisementInterval       uint32
	AdvertisingEnabled                   uint8
	ForwardingEnabled                    uint8
	WeakHostSend                         uint8
	WeakHostReceive                      uint8
	UseAutomaticMetric                   uint8
	UseNeighborUnreachabilityDetection   uint8
	ManagedAddressConfigurationSupported uint8
	OtherStatefulConfigurationSupported  uint8
	AdvertiseDefaultRoute                uint8
	_                                    [3]byte
	RouterDiscoveryBehavior              uint32
	DadTransmits                         uint32
	BaseReachableTime                    uint32
	RetransmitTime                       uint32
	PathMtuDiscoveryTimeout              uint32
	LinkLocalAddressBehavior             uint32
	LinkLocalAddressTimeout              uint32
	ZoneIndices                          [16]uint32
	SitePrefixLength                     uint32
	Metric                               uint32
	NlMtu                                uint32
	Connected                            uint8
	SupportsWakeUpPatterns               uint8
	SupportsNeighborDiscovery            uint8
	SupportsRouterDiscovery              uint8
	ReachableTime                        uint32
	TransmitOffload                      uint8
	ReceiveOffload                       uint8
	DisableDefaultRoutes                 uint8
	_                                    [8]byte
}

// iphlpapi is present on every supported Windows version. The procedures are
// loaded lazily so the package still starts if one is missing (only the
// affected call fails, with a wrapped error).
var (
	iphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")

	procConvertInterfaceAliasToLuid = iphlpapi.NewProc("ConvertInterfaceAliasToLuid")
	procConvertInterfaceLuidToIndex = iphlpapi.NewProc("ConvertInterfaceLuidToIndex")
	procConvertInterfaceIndexToLuid = iphlpapi.NewProc("ConvertInterfaceIndexToLuid")
	procConvertInterfaceLuidToAlias = iphlpapi.NewProc("ConvertInterfaceLuidToAlias")

	procGetIpForwardTable2       = iphlpapi.NewProc("GetIpForwardTable2")
	procFreeMibTable             = iphlpapi.NewProc("FreeMibTable")
	procCreateIpForwardEntry2    = iphlpapi.NewProc("CreateIpForwardEntry2")
	procSetIpForwardEntry2       = iphlpapi.NewProc("SetIpForwardEntry2")
	procDeleteIpForwardEntry2    = iphlpapi.NewProc("DeleteIpForwardEntry2")
	procInitializeIpForwardEntry = iphlpapi.NewProc("InitializeIpForwardEntry")
	procGetBestRoute2            = iphlpapi.NewProc("GetBestRoute2")

	procGetIpInterfaceEntry  = iphlpapi.NewProc("GetIpInterfaceEntry")
	procSetIpInterfaceEntry  = iphlpapi.NewProc("SetIpInterfaceEntry")
	procCreateUnicastAddress = iphlpapi.NewProc("CreateUnicastIpAddressEntry")
	procInitializeUnicast    = iphlpapi.NewProc("InitializeUnicastIpAddressEntry")
)

// physicalRoute is the physical default route that existed before the tunnel
// was configured. The bypass route for the VPN server is pinned to it.
type physicalRoute struct {
	ifIndex uint32
	nextHop net.IP
	metric  uint32
}

// valid reports whether a route was actually recorded.
func (r physicalRoute) valid() bool { return r.ifIndex != 0 }

// createdRoute identifies one route this package installed. The destination
// prefix, interface and next hop together are enough for
// DeleteIpForwardEntry2; the metric is not part of the key.
type createdRoute struct {
	destination  net.IP
	prefixLength uint8
	ifIndex      uint32
	nextHop      net.IP
}

// String formats a route for error messages.
func (r createdRoute) String() string {
	nextHop := "on-link"
	if r.nextHop != nil && !r.nextHop.IsUnspecified() {
		nextHop = r.nextHop.String()
	}
	return fmt.Sprintf("%s/%d via %s on interface %d", r.destination, r.prefixLength, nextHop, r.ifIndex)
}

// persistedRoute is the on disk form of a created route. It lets a new
// process clean up the bypass route after the previous process was killed.
type persistedRoute struct {
	Destination  string `json:"destination"`
	PrefixLength uint8  `json:"prefix_length"`
	Interface    uint32 `json:"interface_index"`
	NextHop      string `json:"next_hop"`
}

// routeState is the JSON document in the per user state file.
type routeState struct {
	Routes []persistedRoute `json:"routes"`
}

var (
	routeMu              sync.Mutex
	createdRoutes        []createdRoute
	lastPhysicalRoute    physicalRoute
	tunnelInterfaceIndex uint32
	tunnelInterfaceAlias string
)

// ConfigureInterface assigns the address, the MTU and the interface metric.
//
// It snapshots the physical default route before it touches anything, because
// ConfigureBypassRoute must pin the server to the route that existed before
// the split default routes were installed. IP Helper is preferred; if any of
// its calls fails, netsh is used as a fallback.
func ConfigureInterface(name, cidr string, mtu int) error {
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("tun: parse the interface address %q: %w", cidr, err)
	}
	prefixLength, _ := network.Mask.Size()

	luid, ifIndex, identityErr := interfaceIdentity(name)
	if identityErr == nil {
		rememberTunnelInterfaceIndex(name, ifIndex)
		snapshotPhysicalRoute(name, ifIndex)
	} else {
		// The adapter may have been created a moment ago. Snapshotting by
		// alias still excludes it from the route selection.
		snapshotPhysicalRoute(name, 0)
	}

	var apiErr error
	if identityErr != nil {
		apiErr = identityErr
	} else {
		apiErr = configureInterfaceWithAPI(name, ip, prefixLength, mtu, luid, ifIndex)
		if apiErr == nil {
			return nil
		}
	}

	commandErr := configureInterfaceWithCommands(name, ip, prefixLength, mtu)
	if commandErr == nil {
		return nil
	}
	if apiErr != nil {
		return fmt.Errorf("tun: configure interface %q: %w (the netsh fallback also failed: %v)", name, apiErr, commandErr)
	}
	return commandErr
}

// ConfigureDefaultRoute installs 0.0.0.0/1 and 128.0.0.0/1 through the tunnel
// instead of replacing the physical default route. More specific routes win
// over 0.0.0.0/0, and removing the two routes restores the original routing
// table without any guesswork.
//
// The IPv6 pair ::/1 and 8000::/1 is best effort: a machine without IPv6 must
// not fail the connection, but if IPv6 works it must not leak around the
// tunnel.
func ConfigureDefaultRoute(name, gateway string) error {
	_, ifIndex, err := interfaceIdentity(name)
	if err != nil {
		return fmt.Errorf("tun: configure the default route: %w", err)
	}

	nextHop := net.IPv4zero
	if gateway != "" {
		parsed := net.ParseIP(gateway)
		if parsed == nil {
			return fmt.Errorf("tun: invalid gateway %q", gateway)
		}
		ip4 := parsed.To4()
		if ip4 == nil {
			return fmt.Errorf("tun: gateway %q is not an IPv4 address", gateway)
		}
		nextHop = ip4
	}

	ipv4Routes := []createdRoute{
		{destination: net.IPv4(0, 0, 0, 0).To4(), prefixLength: 1, ifIndex: ifIndex, nextHop: nextHop},
		{destination: net.IPv4(128, 0, 0, 0).To4(), prefixLength: 1, ifIndex: ifIndex, nextHop: nextHop},
	}
	added := make([]createdRoute, 0, len(ipv4Routes)+2)
	for _, route := range ipv4Routes {
		if err := addRoute(route); err != nil {
			for _, installed := range added {
				_ = deleteRoute(installed)
			}
			return err
		}
		added = append(added, route)
	}

	for _, prefix := range []string{"::/1", "8000::/1"} {
		if !BlockIPv6() {
			break
		}
		destination, _, err := net.ParseCIDR(prefix)
		if err != nil {
			continue
		}
		route := createdRoute{destination: destination, prefixLength: 1, ifIndex: ifIndex, nextHop: net.IPv6zero}
		if err := addRoute(route); err == nil {
			added = append(added, route)
		}
	}
	trackRoutes(added...)
	return nil
}

// ConfigureBypassRoute pins the VPN server to the physical default route. The
// route is a /32 so it beats both split default routes; without it the
// encrypted UDP packets would be sent into the tunnel and would loop.
func ConfigureBypassRoute(serverIP string) error {
	ip := net.ParseIP(serverIP)
	if ip == nil {
		return fmt.Errorf("tun: invalid server address %q", serverIP)
	}
	server := ip.To4()
	if server == nil {
		return fmt.Errorf("tun: the bypass route needs an IPv4 server address, got %q", serverIP)
	}

	routeMu.Lock()
	physical := lastPhysicalRoute
	routeMu.Unlock()
	if !physical.valid() {
		return errors.New("tun: the physical default route was not recorded; call ConfigureInterface before installing the bypass route")
	}
	nextHop := physical.nextHop
	if nextHop == nil {
		nextHop = net.IPv4zero
	}
	route := createdRoute{
		destination:  append(net.IP(nil), server...),
		prefixLength: 32,
		ifIndex:      physical.ifIndex,
		nextHop:      append(net.IP(nil), nextHop...),
	}
	if err := addRoute(route); err != nil {
		return err
	}
	trackRoutes(route)
	return nil
}

// DefaultRoute returns the alias and the gateway of the current physical
// default route. The Wintun adapter is always excluded, even when its split
// default routes currently win the lookup.
func DefaultRoute() (dev string, gateway string, err error) {
	routeMu.Lock()
	excludeIndex := tunnelInterfaceIndex
	excludeAlias := tunnelInterfaceAlias
	routeMu.Unlock()

	route, err := findDefaultRoute(excludeIndex, excludeAlias)
	if err != nil {
		return "", "", err
	}
	dev, err = interfaceAlias(route.ifIndex)
	if err != nil {
		return "", "", fmt.Errorf("tun: resolve the default route interface: %w", err)
	}
	if route.nextHop != nil && !route.nextHop.IsUnspecified() {
		gateway = route.nextHop.String()
	}
	return dev, gateway, nil
}

// ConfigureNAT is a no-op on Windows. The client only uses Wintun, and the
// server role is not implemented for Windows.
func ConfigureNAT(subnet, outbound, iface string) error { return nil }

// Cleanup removes every route this process installed. Deletion is
// idempotent: a route that is already gone is not an error.
func Cleanup() error {
	routeMu.Lock()
	routes := createdRoutes
	createdRoutes = nil
	routeMu.Unlock()

	var errs []error
	var failed []createdRoute
	for _, route := range routes {
		if err := deleteRoute(route); err != nil {
			errs = append(errs, err)
			failed = append(failed, route)
		}
	}

	routeMu.Lock()
	createdRoutes = append(createdRoutes, failed...)
	saveRoutesLocked()
	routeMu.Unlock()
	return errors.Join(errs...)
}

// ResetInterface removes stale tunnel routes after a crash. It is meant to be
// called at application start, before Open, and needs administrator rights
// because deleting routes does.
//
// It deletes the split default routes on the named adapter, the bypass route
// recorded by a previous process and anything still tracked in memory. A
// missing adapter or a missing route is not an error.
func ResetInterface(name string) error {
	_, ifIndex, identityErr := interfaceIdentity(name)
	if identityErr != nil && !isInterfaceNotFound(identityErr) {
		return fmt.Errorf("tun: reset interface %q: %w", name, identityErr)
	}

	var errs []error
	var failed []createdRoute
	if ifIndex != 0 {
		for _, family := range []uint16{windows.AF_INET, windows.AF_INET6} {
			if err := deleteStaleTunnelRoutes(ifIndex, family); err != nil {
				errs = append(errs, err)
			}
		}
	}

	// Routes written by the previous process, including the bypass route on
	// the physical interface.
	for _, route := range loadPersistedRoutes() {
		if err := deleteRoute(route); err != nil {
			errs = append(errs, err)
			failed = append(failed, route)
		}
	}

	// Routes that are still tracked in this process.
	routeMu.Lock()
	tracked := createdRoutes
	createdRoutes = nil
	routeMu.Unlock()
	for _, route := range tracked {
		if err := deleteRoute(route); err != nil {
			errs = append(errs, err)
			failed = append(failed, route)
		}
	}

	routeMu.Lock()
	createdRoutes = append(createdRoutes, failed...)
	saveRoutesLocked()
	routeMu.Unlock()
	return errors.Join(errs...)
}

// interfaceIdentity resolves an interface alias to its LUID and index.
//
// A freshly created Wintun adapter is not always registered in the interface
// list on the first query: the driver publishes it asynchronously. Retrying for
// a moment avoids a spurious failure of the whole connection on a fast machine.
func interfaceIdentity(alias string) (luid uint64, ifIndex uint32, err error) {
	const attempts = 20
	const pause = 100 * time.Millisecond

	aliasPtr, err := windows.UTF16PtrFromString(alias)
	if err != nil {
		return 0, 0, fmt.Errorf("tun: encode interface alias %q: %w", alias, err)
	}
	for attempt := 0; attempt < attempts; attempt++ {
		status, _, _ := procConvertInterfaceAliasToLuid.Call(uintptr(unsafe.Pointer(aliasPtr)), uintptr(unsafe.Pointer(&luid)))
		if status == 0 {
			status, _, _ = procConvertInterfaceLuidToIndex.Call(uintptr(unsafe.Pointer(&luid)), uintptr(unsafe.Pointer(&ifIndex)))
			if status == 0 {
				return luid, ifIndex, nil
			}
			err = fmt.Errorf("tun: get the index of interface %q: %w", alias, syscall.Errno(status))
		} else {
			err = fmt.Errorf("tun: resolve interface %q: %w", alias, syscall.Errno(status))
		}
		if !isInterfaceNotFound(err) {
			return 0, 0, err
		}
		time.Sleep(pause)
	}
	return 0, 0, err
}

// interfaceAlias resolves an interface index to its current alias.
func interfaceAlias(ifIndex uint32) (string, error) {
	var luid uint64
	status, _, _ := procConvertInterfaceIndexToLuid.Call(uintptr(ifIndex), uintptr(unsafe.Pointer(&luid)))
	if status != 0 {
		return "", fmt.Errorf("tun: resolve interface index %d: %w", ifIndex, syscall.Errno(status))
	}
	buffer := make([]uint16, ifMaxStringSize+1)
	status, _, _ = procConvertInterfaceLuidToAlias.Call(
		uintptr(unsafe.Pointer(&luid)),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
	)
	if status != 0 {
		return "", fmt.Errorf("tun: get the alias of interface %d: %w", ifIndex, syscall.Errno(status))
	}
	return windows.UTF16ToString(buffer), nil
}

// rememberTunnelInterface resolves and stores the tunnel identity, ignoring
// errors because the adapter may not be visible yet.
func rememberTunnelInterface(name string) {
	if _, ifIndex, err := interfaceIdentity(name); err == nil {
		rememberTunnelInterfaceIndex(name, ifIndex)
	}
}

// rememberTunnelInterfaceIndex records the interface that DefaultRoute must
// skip.
func rememberTunnelInterfaceIndex(name string, ifIndex uint32) {
	routeMu.Lock()
	tunnelInterfaceIndex = ifIndex
	tunnelInterfaceAlias = name
	routeMu.Unlock()
}

// forgetTunnelInterface clears the remembered tunnel after the adapter is
// closed, so a reused interface index cannot hide a real default route.
func forgetTunnelInterface() {
	routeMu.Lock()
	tunnelInterfaceIndex = 0
	tunnelInterfaceAlias = ""
	lastPhysicalRoute = physicalRoute{}
	routeMu.Unlock()
}

// snapshotPhysicalRoute records the physical default route before the tunnel
// routes exist. The snapshot is used only by ConfigureBypassRoute.
func snapshotPhysicalRoute(alias string, ifIndex uint32) {
	route, err := findDefaultRoute(ifIndex, alias)
	routeMu.Lock()
	defer routeMu.Unlock()
	if err != nil {
		lastPhysicalRoute = physicalRoute{}
		return
	}
	lastPhysicalRoute = route
}

// findDefaultRoute returns the best IPv4 default route that is not the tunnel.
func findDefaultRoute(excludeIndex uint32, excludeAlias string) (physicalRoute, error) {
	if route, err := defaultRouteFromTable(excludeIndex, excludeAlias); err == nil {
		return route, nil
	}
	return defaultRouteFromBestRoute(excludeIndex, excludeAlias)
}

// defaultRouteFromTable scans the forwarding table. It is used instead of
// GetBestRoute2 because the split default routes make GetBestRoute2 return the
// tunnel, while the physical 0.0.0.0/0 route must be found.
func defaultRouteFromTable(excludeIndex uint32, excludeAlias string) (physicalRoute, error) {
	table, err := getForwardTable(windows.AF_INET)
	if err != nil {
		return physicalRoute{}, err
	}
	defer freeMibTable(table)

	rows := table.rows()
	var best *mibIPForwardRow2
	for i := range rows {
		row := &rows[i]
		if row.DestinationPrefix.PrefixLength != 0 {
			continue
		}
		if !ipFromSockaddr(&row.DestinationPrefix.Prefix).IsUnspecified() {
			continue
		}
		if row.Loopback != 0 {
			continue
		}
		if isTunnelInterface(row.InterfaceIndex, excludeIndex, excludeAlias) {
			continue
		}
		if best == nil || row.Metric < best.Metric {
			best = row
		}
	}
	if best == nil {
		return physicalRoute{}, errors.New("tun: no physical default route found")
	}
	return physicalRoute{
		ifIndex: best.InterfaceIndex,
		nextHop: ipFromSockaddr(&best.NextHop),
		metric:  best.Metric,
	}, nil
}

// defaultRouteFromBestRoute is the fallback when the table cannot be read.
func defaultRouteFromBestRoute(excludeIndex uint32, excludeAlias string) (physicalRoute, error) {
	destination := sockaddrFromIP(net.IPv4zero)
	var row mibIPForwardRow2
	var source sockaddrInet
	status, _, _ := procGetBestRoute2.Call(
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&destination)),
		0,
		uintptr(unsafe.Pointer(&row)),
		uintptr(unsafe.Pointer(&source)),
	)
	if status != 0 {
		return physicalRoute{}, fmt.Errorf("tun: get the best default route: %w", syscall.Errno(status))
	}
	if isTunnelInterface(row.InterfaceIndex, excludeIndex, excludeAlias) {
		return physicalRoute{}, errors.New("tun: the best default route belongs to the tunnel")
	}
	return physicalRoute{
		ifIndex: row.InterfaceIndex,
		nextHop: ipFromSockaddr(&row.NextHop),
		metric:  row.Metric,
	}, nil
}

// isTunnelInterface reports whether an interface is the Wintun adapter.
func isTunnelInterface(ifIndex, excludeIndex uint32, excludeAlias string) bool {
	if excludeIndex != 0 && ifIndex == excludeIndex {
		return true
	}
	if excludeAlias == "" {
		return false
	}
	alias, err := interfaceAlias(ifIndex)
	return err == nil && strings.EqualFold(alias, excludeAlias)
}

// configureInterfaceWithAPI uses IP Helper to add the address and change the
// interface properties.
func configureInterfaceWithAPI(name string, ip net.IP, prefixLength, mtu int, luid uint64, ifIndex uint32) error {
	row := mibUnicastIPAddressRow{}
	procInitializeUnicast.Call(uintptr(unsafe.Pointer(&row)))
	row.Address = sockaddrFromIP(ip)
	row.InterfaceLuid = luid
	row.InterfaceIndex = ifIndex
	row.OnLinkPrefixLength = uint8(prefixLength)
	row.SkipAsSource = 0
	status, _, _ := procCreateUnicastAddress.Call(uintptr(unsafe.Pointer(&row)))
	if status != 0 && syscall.Errno(status) != windows.ERROR_OBJECT_ALREADY_EXISTS {
		return fmt.Errorf("tun: add the address %s to %q: %w", ip, name, syscall.Errno(status))
	}

	family := uint16(windows.AF_INET)
	if ip.To4() == nil {
		family = windows.AF_INET6
	}
	if err := setInterfaceMTU(luid, ifIndex, family, mtu); err != nil {
		return err
	}
	// A sane IPv6 MTU is needed for the blackhole routes added later. The
	// tunnel normally carries IPv4 only, so this failure is not fatal.
	other := uint16(windows.AF_INET6)
	if family == windows.AF_INET6 {
		other = windows.AF_INET
	}
	_ = setInterfaceMTU(luid, ifIndex, other, mtu)
	return nil
}

// setInterfaceMTU applies the MTU, the metric and the duplicate address
// detection setting to one address family of the interface.
func setInterfaceMTU(luid uint64, ifIndex uint32, family uint16, mtu int) error {
	row, err := getIPInterfaceRow(luid, ifIndex, family)
	if err != nil {
		return err
	}
	row.NlMtu = uint32(mtu)
	row.Metric = routeMetric
	row.UseAutomaticMetric = 0
	row.SitePrefixLength = 0
	row.DadTransmits = 0
	status, _, _ := procSetIpInterfaceEntry.Call(uintptr(unsafe.Pointer(row)))
	if status != 0 {
		return fmt.Errorf("tun: set the IP interface entry: %w", syscall.Errno(status))
	}
	return nil
}

// getIPInterfaceRow reads the mutable properties of an interface.
func getIPInterfaceRow(luid uint64, ifIndex uint32, family uint16) (*mibIPInterfaceRow, error) {
	row := &mibIPInterfaceRow{Family: family, InterfaceLuid: luid, InterfaceIndex: ifIndex}
	status, _, _ := procGetIpInterfaceEntry.Call(uintptr(unsafe.Pointer(row)))
	if status != 0 {
		return nil, fmt.Errorf("tun: get the IP interface entry: %w", syscall.Errno(status))
	}
	return row, nil
}

// configureInterfaceWithCommands is the netsh fallback. It is slower and less
// precise, but it works when an IP Helper call is rejected by a filter.
func configureInterfaceWithCommands(name string, ip net.IP, prefixLength, mtu int) error {
	family := "ipv4"
	if ip.To4() == nil {
		family = "ipv6"
	}

	if family == "ipv4" {
		mask := net.CIDRMask(prefixLength, 32)
		if err := run("netsh", "interface", "ipv4", "set", "address", "name="+name, "static", ip.String(), net.IP(mask).String()); err != nil {
			return err
		}
	} else {
		if err := run("netsh", "interface", "ipv6", "set", "address", "name="+name, fmt.Sprintf("%s/%d", ip, prefixLength)); err != nil {
			return err
		}
	}
	if err := run("netsh", "interface", family, "set", "interface", name, "metric=1"); err != nil {
		return err
	}
	if mtu > 0 {
		if err := run("netsh", "interface", family, "set", "subinterface", name, fmt.Sprintf("mtu=%d", mtu), "store=persistent"); err != nil {
			return err
		}
	}
	if family == "ipv4" && mtu > 0 {
		// Best effort; the IPv6 blackhole routes need a usable MTU.
		_ = run("netsh", "interface", "ipv6", "set", "subinterface", name, fmt.Sprintf("mtu=%d", mtu), "store=persistent")
	}
	return nil
}

// addRoute creates a route, or updates it when the same prefix already exists
// from an earlier run.
func addRoute(route createdRoute) error {
	row := mibIPForwardRow2{}
	procInitializeIpForwardEntry.Call(uintptr(unsafe.Pointer(&row)))
	row.InterfaceLuid = 0
	row.InterfaceIndex = route.ifIndex
	row.DestinationPrefix = ipAddressPrefix{
		Prefix:       sockaddrFromIP(route.destination),
		PrefixLength: route.prefixLength,
	}
	row.NextHop = sockaddrFromIP(route.nextHop)
	row.Metric = routeMetric
	row.Protocol = windows.MIB_IPPROTO_NETMGMT
	row.Origin = windows.NlroManual

	status, _, _ := procCreateIpForwardEntry2.Call(uintptr(unsafe.Pointer(&row)))
	if syscall.Errno(status) == windows.ERROR_OBJECT_ALREADY_EXISTS {
		status, _, _ = procSetIpForwardEntry2.Call(uintptr(unsafe.Pointer(&row)))
	}
	if status != 0 {
		return fmt.Errorf("tun: add route %s: %w", route, syscall.Errno(status))
	}
	return nil
}

// deleteRoute removes one route. A route that is already gone is success.
func deleteRoute(route createdRoute) error {
	row := mibIPForwardRow2{}
	procInitializeIpForwardEntry.Call(uintptr(unsafe.Pointer(&row)))
	row.InterfaceLuid = 0
	row.InterfaceIndex = route.ifIndex
	row.DestinationPrefix = ipAddressPrefix{
		Prefix:       sockaddrFromIP(route.destination),
		PrefixLength: route.prefixLength,
	}
	row.NextHop = sockaddrFromIP(route.nextHop)
	return deleteForwardRow(&row)
}

// deleteForwardRow removes a route that was read from the forwarding table or
// built by deleteRoute.
func deleteForwardRow(row *mibIPForwardRow2) error {
	status, _, _ := procDeleteIpForwardEntry2.Call(uintptr(unsafe.Pointer(row)))
	if status == 0 {
		return nil
	}
	errno := syscall.Errno(status)
	if errno == windows.ERROR_NOT_FOUND || errno == windows.ERROR_FILE_NOT_FOUND || errno == windows.ERROR_INVALID_PARAMETER {
		return nil
	}
	return fmt.Errorf("tun: delete route: %w", errno)
}

// deleteStaleTunnelRoutes removes the split default routes that a previous
// process left on the adapter.
func deleteStaleTunnelRoutes(ifIndex uint32, family uint16) error {
	table, err := getForwardTable(family)
	if err != nil {
		// An absent family (IPv6 disabled, for example) has no stale routes.
		if errors.Is(err, windows.ERROR_NOT_FOUND) {
			return nil
		}
		return err
	}
	defer freeMibTable(table)

	rows := table.rows()
	var errs []error
	for i := range rows {
		row := &rows[i]
		if row.InterfaceIndex != ifIndex {
			continue
		}
		if !isTunnelDefaultPrefix(ipFromSockaddr(&row.DestinationPrefix.Prefix), row.DestinationPrefix.PrefixLength) {
			continue
		}
		if err := deleteForwardRow(row); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// isTunnelDefaultPrefix reports whether the destination is one of the half
// default routes this package installs.
func isTunnelDefaultPrefix(ip net.IP, prefixLength uint8) bool {
	if prefixLength != 1 || ip == nil {
		return false
	}
	bytes := ip.To4()
	if bytes == nil {
		bytes = ip.To16()
	}
	if bytes == nil {
		return false
	}
	return bytes[0] == 0x00 || bytes[0] == 0x80
}

// trackRoutes adds routes to the registry and writes the state file used by
// ResetInterface after a crash.
func trackRoutes(routes ...createdRoute) {
	routeMu.Lock()
	createdRoutes = append(createdRoutes, routes...)
	saveRoutesLocked()
	routeMu.Unlock()
}

// getForwardTable reads a forwarding table of one address family.
func getForwardTable(family uint16) (*mibIPForwardTable2, error) {
	var table *mibIPForwardTable2
	status, _, _ := procGetIpForwardTable2.Call(uintptr(family), uintptr(unsafe.Pointer(&table)))
	if status != 0 {
		return nil, fmt.Errorf("tun: get the IP forward table: %w", syscall.Errno(status))
	}
	return table, nil
}

// freeMibTable releases a table returned by IP Helper.
func freeMibTable(table *mibIPForwardTable2) {
	if table != nil {
		procFreeMibTable.Call(uintptr(unsafe.Pointer(table)))
	}
}

// sockaddrFromIP fills a SOCKADDR_INET for an IPv4 or IPv6 address.
func sockaddrFromIP(ip net.IP) sockaddrInet {
	var address sockaddrInet
	if ip4 := ip.To4(); ip4 != nil {
		address.Family = uint16(windows.AF_INET)
		copy(address.bytes()[4:8], ip4)
		return address
	}
	address.Family = uint16(windows.AF_INET6)
	copy(address.bytes()[8:24], ip.To16())
	return address
}

// ipFromSockaddr extracts the address from a SOCKADDR_INET.
func ipFromSockaddr(address *sockaddrInet) net.IP {
	bytes := address.bytes()
	switch int(address.Family) {
	case windows.AF_INET:
		return net.IPv4(bytes[4], bytes[5], bytes[6], bytes[7]).To4()
	case windows.AF_INET6:
		return append(net.IP(nil), bytes[8:24]...)
	default:
		return nil
	}
}

// bytes exposes the 28 byte union as a byte slice without copying.
func (s *sockaddrInet) bytes() []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(s)), unsafe.Sizeof(*s))
}

// routeStatePath is the per user file that records installed routes.
func routeStatePath() (string, error) {
	dir, err := wintunInstallDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "routes.json"), nil
}

// saveRoutesLocked writes the current registry. The state file is advisory:
// a failure must never break the tunnel, so errors are ignored.
func saveRoutesLocked() {
	path, err := routeStatePath()
	if err != nil {
		return
	}
	if len(createdRoutes) == 0 {
		_ = os.Remove(path)
		return
	}
	state := routeState{Routes: make([]persistedRoute, 0, len(createdRoutes))}
	for _, route := range createdRoutes {
		nextHop := ""
		if route.nextHop != nil {
			nextHop = route.nextHop.String()
		}
		state.Routes = append(state.Routes, persistedRoute{
			Destination:  route.destination.String(),
			PrefixLength: route.prefixLength,
			Interface:    route.ifIndex,
			NextHop:      nextHop,
		})
	}
	data, err := json.Marshal(state)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = writeFileAtomic(path, data)
}

// loadPersistedRoutes reads the routes recorded by an earlier process.
func loadPersistedRoutes() []createdRoute {
	path, err := routeStatePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var state routeState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil
	}
	routes := make([]createdRoute, 0, len(state.Routes))
	for _, persisted := range state.Routes {
		destination := net.ParseIP(persisted.Destination)
		if destination == nil {
			continue
		}
		nextHop := net.ParseIP(persisted.NextHop)
		if nextHop == nil {
			continue
		}
		routes = append(routes, createdRoute{
			destination:  destination,
			prefixLength: persisted.PrefixLength,
			ifIndex:      persisted.Interface,
			nextHop:      nextHop,
		})
	}
	return routes
}

// isInterfaceNotFound reports whether alias resolution failed because the
// adapter does not exist. ConvertInterfaceAliasToLuid uses
// ERROR_INVALID_PARAMETER for an unknown alias on current Windows versions,
// while older versions and other netioapi calls use the two file not found
// codes.
func isInterfaceNotFound(err error) bool {
	return errors.Is(err, windows.ERROR_INVALID_PARAMETER) ||
		errors.Is(err, windows.ERROR_FILE_NOT_FOUND) ||
		errors.Is(err, windows.ERROR_NOT_FOUND)
}

// run executes a command and includes its output in the error. It is the
// Windows counterpart of the helper in routes_linux.go.
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tun: %s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
