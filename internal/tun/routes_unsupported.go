//go:build !linux && !windows

package tun

// The routing helpers are only implemented on Linux. On other platforms the
// proxy mode is used, which does not touch the routing table.

// ConfigureInterface is a no-op outside Linux.
func ConfigureInterface(name, cidr string, mtu int) error { return nil }

// ConfigureDefaultRoute is a no-op outside Linux.
func ConfigureDefaultRoute(name, gateway string) error { return nil }

// ConfigureBypassRoute is a no-op outside Linux.
func ConfigureBypassRoute(serverIP string) error { return nil }

// ConfigureNAT is a no-op outside Linux.
func ConfigureNAT(subnet, outbound, iface string) error { return nil }
