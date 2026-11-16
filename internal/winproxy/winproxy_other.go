//go:build !windows

// Package winproxy is only implemented on Windows. On the other platforms
// the proxy mode never touches a system wide setting: the user configures
// the SOCKS5 listener in the application that needs it.
package winproxy

import "errors"

// ErrUnsupported is returned on platforms without a WinINET proxy.
var ErrUnsupported = errors.New("winproxy: the system proxy is only available on Windows")

// Capture returns an error on this platform.
func Capture() (Snapshot, error) { return Snapshot{}, ErrUnsupported }

// Enable returns an error on this platform.
func Enable(string) error { return ErrUnsupported }

// Restore returns an error on this platform.
func Restore(Snapshot) error { return ErrUnsupported }

// Current returns an error on this platform.
func Current() (bool, string, error) { return false, "", ErrUnsupported }
