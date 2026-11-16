// Package config loads and validates the JSON configuration files of the
// W1nWire server and client.
package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
)

// ServerConfig is the configuration of w1nwire-server.
type ServerConfig struct {
	// Listen is the UDP address the server binds to.
	Listen string `json:"listen"`
	// ServerPrivateKey is the base64 encoded static private key of the server.
	ServerPrivateKey string `json:"server_private_key"`
	// TUN describes the tunnel device that carries the client traffic.
	TUN TUNConfig `json:"tun"`
	// NAT controls the masquerading of the tunnel traffic.
	NAT NATConfig `json:"nat"`
	// HTTP configures the metrics endpoint.
	HTTP HTTPConfig `json:"http"`
	// Peers lists the clients that are allowed to connect.
	Peers []PeerConfig `json:"peers"`
}

// PeerConfig describes a single client.
type PeerConfig struct {
	// ID is a human readable name used in logs and metrics.
	ID string `json:"id"`
	// PublicKey is the base64 encoded static public key of the client.
	PublicKey string `json:"public_key"`
	// TunnelAddress is the address of the client inside the tunnel subnet.
	TunnelAddress string `json:"tunnel_address"`
	// AllowedIPs restricts the source prefixes the peer may send.
	AllowedIPs []string `json:"allowed_ips"`
}

// TUNConfig describes the tunnel device.
type TUNConfig struct {
	// Enabled turns the TUN device on. When it is off the server only serves
	// the proxy mode.
	Enabled bool `json:"enabled"`
	// Name is the name of the device, it must be three characters or less on
	// Linux.
	Name string `json:"name"`
	// Address is the address of the server side of the tunnel, in CIDR form.
	Address string `json:"address"`
	// MTU is the maximum transmission unit of the tunnel.
	MTU int `json:"mtu"`
}

// NATConfig controls the masquerading of the tunnel traffic.
type NATConfig struct {
	// Enabled turns on the iptables masquerade rule.
	Enabled bool `json:"enabled"`
	// OutboundInterface overrides the interface used for masquerading.
	OutboundInterface string `json:"outbound_interface"`
}

// HTTPConfig configures the HTTP endpoints.
type HTTPConfig struct {
	// Metrics exposes the Prometheus metrics.
	Metrics MetricsConfig `json:"metrics"`
}

// MetricsConfig configures the Prometheus endpoint.
type MetricsConfig struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
}

// ClientConfig is the configuration of w1nwire-client.
type ClientConfig struct {
	// Server describes the remote endpoint.
	Server ServerEndpoint `json:"server"`
	// PrivateKey is the base64 encoded static private key of the client.
	PrivateKey string `json:"private_key"`
	// TUN describes the local tunnel device.
	TUN ClientTUNConfig `json:"tun"`
	// SOCKS5 configures the local proxy.
	SOCKS5 SOCKS5Config `json:"socks5"`
	// DNS is the resolver that is used inside the tunnel.
	DNS string `json:"dns"`
	// HTTP configures the metrics endpoint.
	HTTP HTTPConfig `json:"http"`
}

// ServerEndpoint describes the server of the tunnel.
type ServerEndpoint struct {
	// Address is the host and port of the server, for example 1.2.3.4:51820.
	Address string `json:"address"`
	// PublicKey is the base64 encoded static public key of the server.
	PublicKey string `json:"public_key"`
}

// ClientTUNConfig describes the local tunnel device.
type ClientTUNConfig struct {
	// Enabled turns the TUN device on.
	Enabled bool `json:"enabled"`
	// Name is the name of the device.
	Name string `json:"name"`
	// Address is the address of the client inside the tunnel, in CIDR form.
	Address string `json:"address"`
	// Gateway is the address of the server inside the tunnel.
	Gateway string `json:"gateway"`
	// MTU is the maximum transmission unit of the tunnel.
	MTU int `json:"mtu"`
	// ConfigureRoutes installs the default route through the tunnel.
	ConfigureRoutes bool `json:"configure_routes"`
	// RouteAll sends all traffic through the tunnel, including the traffic
	// to the server itself.
	RouteAll bool `json:"route_all"`
	// AllowIPv6 lets IPv6 traffic use the physical interface while the
	// tunnel is up. It is off by default: a leak is worse than a missing
	// IPv6, and the tunnel itself is IPv4 only.
	AllowIPv6 bool `json:"allow_ipv6"`
}

// SOCKS5Config configures the local proxy.
type SOCKS5Config struct {
	// Enabled turns the local SOCKS5 proxy on.
	Enabled bool `json:"enabled"`
	// Listen is the address the proxy binds to.
	Listen string `json:"listen"`
}

// LoadServer reads and validates a server configuration.
func LoadServer(path string) (*ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var cfg ServerConfig
	dec := json.NewDecoder(strings.NewReader(trimBOM(string(data))))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadClient reads and validates a client configuration.
func LoadClient(path string) (*ClientConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var cfg ClientConfig
	dec := json.NewDecoder(strings.NewReader(trimBOM(string(data))))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// trimBOM removes the byte order mark that some Windows editors put in front
// of a UTF-8 file. Without this, a configuration written by Notepad would
// fail with "invalid character '\ufeff'" which says nothing to the user.
func trimBOM(s string) string {
	return strings.TrimPrefix(s, "\ufeff")
}

// Validate checks the server configuration for obvious mistakes.
func (c *ServerConfig) Validate() error {
	if c.Listen == "" {
		return fmt.Errorf("config: listen address is required")
	}
	if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		return fmt.Errorf("config: invalid listen address %q: %w", c.Listen, err)
	}
	if c.ServerPrivateKey == "" {
		return fmt.Errorf("config: server_private_key is required")
	}
	if c.TUN.Enabled {
		if c.TUN.Address == "" {
			return fmt.Errorf("config: tun.address is required when the tun device is enabled")
		}
		if _, _, err := net.ParseCIDR(c.TUN.Address); err != nil {
			return fmt.Errorf("config: invalid tun.address %q: %w", c.TUN.Address, err)
		}
	}
	if len(c.Peers) == 0 {
		return fmt.Errorf("config: at least one peer is required")
	}
	seen := map[string]bool{}
	for i, p := range c.Peers {
		if p.ID == "" {
			return fmt.Errorf("config: peers[%d].id is required", i)
		}
		if p.PublicKey == "" {
			return fmt.Errorf("config: peers[%d].public_key is required", i)
		}
		if p.TunnelAddress == "" {
			return fmt.Errorf("config: peers[%d].tunnel_address is required", i)
		}
		if net.ParseIP(p.TunnelAddress) == nil {
			return fmt.Errorf("config: peers[%d].tunnel_address %q is not an IP address", i, p.TunnelAddress)
		}
		if seen[p.ID] {
			return fmt.Errorf("config: duplicate peer id %q", p.ID)
		}
		seen[p.ID] = true
	}
	return nil
}

// Validate checks the client configuration for obvious mistakes.
func (c *ClientConfig) Validate() error {
	if c.Server.Address == "" {
		return fmt.Errorf("config: server.address is required")
	}
	if _, _, err := net.SplitHostPort(c.Server.Address); err != nil {
		return fmt.Errorf("config: invalid server.address %q: %w", c.Server.Address, err)
	}
	if c.Server.PublicKey == "" {
		return fmt.Errorf("config: server.public_key is required")
	}
	if c.PrivateKey == "" {
		return fmt.Errorf("config: private_key is required")
	}
	if !c.TUN.Enabled && !c.SOCKS5.Enabled {
		return fmt.Errorf("config: at least one of tun and socks5 must be enabled")
	}
	if c.TUN.Enabled {
		if c.TUN.Address == "" {
			return fmt.Errorf("config: tun.address is required when the tun device is enabled")
		}
		if _, _, err := net.ParseCIDR(c.TUN.Address); err != nil {
			return fmt.Errorf("config: invalid tun.address %q: %w", c.TUN.Address, err)
		}
	}
	if c.SOCKS5.Enabled {
		if c.SOCKS5.Listen == "" {
			return fmt.Errorf("config: socks5.listen is required when the proxy is enabled")
		}
		if _, _, err := net.SplitHostPort(c.SOCKS5.Listen); err != nil {
			return fmt.Errorf("config: invalid socks5.listen %q: %w", c.SOCKS5.Listen, err)
		}
	}
	return nil
}
