// Package winproxy reads and writes the system wide proxy settings of
// Windows. The proxy mode of W1nWire uses it to point the applications that
// honour the WinINET settings at the local SOCKS5 listener, and to restore
// exactly the previous values when the tunnel goes down.
package winproxy

// Snapshot is the proxy configuration of the user before the application
// changed it.
type Snapshot struct {
	// Enabled is the previous value of ProxyEnable.
	Enabled bool `json:"enabled"`
	// Server is the previous value of ProxyServer.
	Server string `json:"server"`
	// Override is the previous value of ProxyOverride.
	Override string `json:"override"`
	// AutoConfigURL is the previous PAC script, if any. A PAC script takes
	// precedence over ProxyServer, so it has to be removed while the tunnel
	// is up and put back afterwards.
	AutoConfigURL string `json:"autoConfigUrl"`
}
