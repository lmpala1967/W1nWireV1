//go:build linux

package tun

import (
	"fmt"
	"os/exec"
	"strings"
)

// ConfigureInterface assigns the address to the interface and brings it up.
func ConfigureInterface(name, cidr string, mtu int) error {
	if err := run("ip", "addr", "add", cidr, "dev", name); err != nil {
		return err
	}
	if err := run("ip", "link", "set", name, "up"); err != nil {
		return err
	}
	if mtu > 0 {
		if err := run("ip", "link", "set", name, "mtu", fmt.Sprint(mtu)); err != nil {
			return err
		}
	}
	return nil
}

// ConfigureDefaultRoute installs a default route through the tunnel.
func ConfigureDefaultRoute(name, gateway string) error {
	if gateway == "" {
		return run("ip", "route", "replace", "default", "dev", name)
	}
	return run("ip", "route", "replace", "default", "via", gateway, "dev", name)
}

// ConfigureBypassRoute pins the address of the server to the physical
// gateway. Without it the handshake would be routed into the tunnel and
// would never reach the server.
func ConfigureBypassRoute(serverIP string) error {
	gateway, dev, err := DefaultRoute()
	if err != nil {
		return err
	}
	if gateway == "" {
		return run("ip", "route", "replace", serverIP, "dev", dev)
	}
	return run("ip", "route", "replace", serverIP, "via", gateway, "dev", dev)
}

// DefaultRoute returns the interface and the gateway of the default route.
func DefaultRoute() (dev string, gateway string, err error) {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return "", "", fmt.Errorf("tun: detect the default route: %w", err)
	}
	fields := strings.Fields(string(out))
	for i, field := range fields {
		if i+1 >= len(fields) {
			break
		}
		switch field {
		case "dev":
			dev = fields[i+1]
		case "via":
			gateway = fields[i+1]
		}
	}
	if dev == "" {
		return "", "", fmt.Errorf("tun: no default route found")
	}
	return dev, gateway, nil
}

// ConfigureNAT enables masquerading for the tunnel subnet and allows the
// forwarded traffic.
func ConfigureNAT(subnet, outbound, iface string) error {
	if outbound == "" {
		_, dev, err := DefaultRoute()
		if err != nil {
			return err
		}
		outbound = dev
	}
	if err := run("sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		return err
	}
	// The -w flag waits for the lock of iptables, so that the parallel
	// start of the service does not fail with "another app is holding the
	// lock".
	if err := run("iptables", "-w", "5", "-t", "nat", "-C", "POSTROUTING", "-s", subnet, "-o", outbound, "-j", "MASQUERADE"); err != nil {
		if err := run("iptables", "-w", "5", "-t", "nat", "-A", "POSTROUTING", "-s", subnet, "-o", outbound, "-j", "MASQUERADE"); err != nil {
			return err
		}
	}
	if err := run("iptables", "-w", "5", "-C", "FORWARD", "-i", iface, "-o", outbound, "-j", "ACCEPT"); err != nil {
		if err := run("iptables", "-w", "5", "-A", "FORWARD", "-i", iface, "-o", outbound, "-j", "ACCEPT"); err != nil {
			return err
		}
	}
	return run("iptables", "-w", "5", "-C", "FORWARD", "-i", outbound, "-o", iface, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tun: %s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
