//go:build !windows

package winutil

import "errors"

// ErrUnsupported is returned on the platforms without Windows integration.
var ErrUnsupported = errors.New("winutil: not supported on this platform")

func isElevated() bool { return false }

// RelaunchElevated is not supported on this platform.
func RelaunchElevated(...string) error { return ErrUnsupported }

// OpenURL is not supported on this platform.
func OpenURL(string) error { return ErrUnsupported }

// OpenPath is not supported on this platform.
func OpenPath(string) error { return ErrUnsupported }

// SetAutoStart is not supported on this platform.
func SetAutoStart(bool) error { return ErrUnsupported }

// SingleInstance is a no-op on this platform.
func SingleInstance(string) (func(), bool) { return func() {}, true }

// SetWindowIcon is a no-op on this platform.
func SetWindowIcon(uintptr) {}
