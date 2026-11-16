//go:build !linux && !windows

package tun

import "errors"

// Open is not available on this platform.
//
// The client can still be used in the proxy mode, where the SOCKS5 server
// forwards the traffic through the tunnel without a kernel device.
func Open(name string, mtu int) (Device, error) {
	return nil, errors.New("tun: tunnel devices are only supported on linux, use the socks5 proxy mode instead")
}
