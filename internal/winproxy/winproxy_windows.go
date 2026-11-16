//go:build windows

package winproxy

import (
	"fmt"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// keyPath is the registry key that holds the WinINET proxy settings.
const keyPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// Options of InternetSetOptionW that make the running programs pick up a
// change without a logoff.
const (
	internetOptionSettingsChanged = 39
	internetOptionRefresh         = 37
)

var (
	modWininet             = windows.NewLazySystemDLL("wininet.dll")
	procInternetSetOptionW = modWininet.NewProc("InternetSetOptionW")
)

// Capture reads the current proxy configuration.
func Capture() (Snapshot, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if err != nil {
		return Snapshot{}, fmt.Errorf("winproxy: open the internet settings: %w", err)
	}
	defer key.Close()

	var s Snapshot
	if v, _, err := key.GetIntegerValue("ProxyEnable"); err == nil {
		s.Enabled = v != 0
	}
	if v, _, err := key.GetStringValue("ProxyServer"); err == nil {
		s.Server = v
	}
	if v, _, err := key.GetStringValue("ProxyOverride"); err == nil {
		s.Override = v
	}
	if v, _, err := key.GetStringValue("AutoConfigURL"); err == nil {
		s.AutoConfigURL = v
	}
	return s, nil
}

// Enable points the system proxy at the given SOCKS5 address.
func Enable(addr string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("winproxy: open the internet settings: %w", err)
	}
	defer key.Close()

	// The SOCKS entry applies to every protocol, which is what a user of a
	// VPN wants by default.
	if err := key.SetStringValue("ProxyServer", "socks="+addr); err != nil {
		return fmt.Errorf("winproxy: set ProxyServer: %w", err)
	}
	if err := key.SetStringValue("ProxyOverride", "<local>"); err != nil {
		return fmt.Errorf("winproxy: set ProxyOverride: %w", err)
	}
	if err := key.SetDWordValue("ProxyEnable", 1); err != nil {
		return fmt.Errorf("winproxy: set ProxyEnable: %w", err)
	}
	// A PAC script wins over the manual settings, remove it while the
	// tunnel is up. It is restored by Restore.
	if err := key.DeleteValue("AutoConfigURL"); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("winproxy: clear AutoConfigURL: %w", err)
	}
	refresh()
	return nil
}

// Restore puts back the configuration captured by Snapshot.
func Restore(s Snapshot) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("winproxy: open the internet settings: %w", err)
	}
	defer key.Close()

	if s.Server != "" {
		if err := key.SetStringValue("ProxyServer", s.Server); err != nil {
			return fmt.Errorf("winproxy: restore ProxyServer: %w", err)
		}
	} else if err := key.DeleteValue("ProxyServer"); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("winproxy: clear ProxyServer: %w", err)
	}

	if s.Override != "" {
		if err := key.SetStringValue("ProxyOverride", s.Override); err != nil {
			return fmt.Errorf("winproxy: restore ProxyOverride: %w", err)
		}
	} else if err := key.DeleteValue("ProxyOverride"); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("winproxy: clear ProxyOverride: %w", err)
	}

	enable := uint32(0)
	if s.Enabled {
		enable = 1
	}
	if err := key.SetDWordValue("ProxyEnable", enable); err != nil {
		return fmt.Errorf("winproxy: restore ProxyEnable: %w", err)
	}

	if s.AutoConfigURL != "" {
		if err := key.SetStringValue("AutoConfigURL", s.AutoConfigURL); err != nil {
			return fmt.Errorf("winproxy: restore AutoConfigURL: %w", err)
		}
	}
	refresh()
	return nil
}

// Current reports whether the system proxy is on and where it points.
func Current() (enabled bool, server string, err error) {
	s, err := Capture()
	if err != nil {
		return false, "", err
	}
	return s.Enabled, s.Server, nil
}

// refresh tells the running programs that the settings changed.
func refresh() {
	procInternetSetOptionW.Call(0, internetOptionSettingsChanged, 0, 0)
	procInternetSetOptionW.Call(0, internetOptionRefresh, 0, 0)
}
