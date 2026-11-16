// Package winutil wraps the few Windows specific operations that the
// desktop application needs: elevation, the autostart entry, opening a URL or
// a folder, and the single instance lock.
package winutil

// IsElevated reports whether the process runs with administrator rights.
func IsElevated() bool { return isElevated() }
