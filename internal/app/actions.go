package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/w1nchi/w1nwire/internal/crypto"
	"github.com/w1nchi/w1nwire/internal/protocol"
)

// SettingsPatch is a partial update of the settings.
type SettingsPatch struct {
	Language       *string `json:"language"`
	DNS            *string `json:"dns"`
	MTU            *int    `json:"mtu"`
	ProxyPort      *int    `json:"proxyPort"`
	BlockIPv6      *bool   `json:"blockIPv6"`
	AutoStart      *bool   `json:"autoStart"`
	AutoConnect    *bool   `json:"autoConnect"`
	MinimizeToTray *bool   `json:"minimizeToTray"`
	SystemProxy    *bool   `json:"systemProxy"`
	Mode           *string `json:"mode"`
}

// UpdateSettings applies a partial update and persists the result.
func (a *App) UpdateSettings(p SettingsPatch) error {
	s := a.store.Settings()
	if p.Language != nil && *p.Language != "" {
		if *p.Language != "ru" && *p.Language != "en" {
			return fmt.Errorf("app: unsupported language %q", *p.Language)
		}
		s.Language = *p.Language
	}
	if p.DNS != nil && *p.DNS != "" {
		if net.ParseIP(*p.DNS) == nil {
			return fmt.Errorf("app: %q is not an IP address", *p.DNS)
		}
		s.DNS = *p.DNS
	}
	if p.MTU != nil {
		if *p.MTU < 576 || *p.MTU > 1500 {
			return fmt.Errorf("app: the MTU must be between 576 and 1500")
		}
		s.MTU = *p.MTU
	}
	if p.ProxyPort != nil {
		if *p.ProxyPort < 1024 || *p.ProxyPort > 65535 {
			return fmt.Errorf("app: the proxy port must be between 1024 and 65535")
		}
		s.ProxyPort = *p.ProxyPort
	}
	if p.BlockIPv6 != nil {
		s.BlockIPv6 = *p.BlockIPv6
	}
	if p.AutoConnect != nil {
		s.AutoConnect = *p.AutoConnect
	}
	if p.MinimizeToTray != nil {
		s.MinimizeToTray = *p.MinimizeToTray
	}
	if p.Mode != nil {
		if *p.Mode != "tunnel" && *p.Mode != "proxy" {
			return fmt.Errorf("app: unknown mode %q", *p.Mode)
		}
		s.Mode = *p.Mode
	}
	if p.AutoStart != nil {
		s.AutoStart = *p.AutoStart
		if err := setAutoStart(*p.AutoStart); err != nil {
			a.logger.Warn("the autostart entry could not be updated", "error", err)
		}
	}
	if p.SystemProxy != nil {
		s.SystemProxy = *p.SystemProxy
		if !*p.SystemProxy {
			a.disableSystemProxy()
		} else if a.isProxyConnected() {
			a.enableSystemProxy()
		}
	}
	if err := a.store.SetSettings(s); err != nil {
		return err
	}
	a.broadcast("state", a.Snapshot())
	return nil
}

// SetSystemProxy turns the system wide proxy on or off.
func (a *App) SetSystemProxy(enabled bool) error {
	return a.UpdateSettings(SettingsPatch{SystemProxy: &enabled})
}

func (a *App) isProxyConnected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.conn != nil && a.conn.mode == "proxy" && a.status.Connected
}

// enableSystemProxy points the Windows proxy settings at the local SOCKS5
// listener. The previous values are remembered so that they can be restored
// exactly when the tunnel goes down.
func (a *App) enableSystemProxy() {
	s := a.store.Settings()
	snap, err := systemProxySnapshot()
	if err != nil {
		a.logger.Warn("the system proxy could not be read", "error", err)
		return
	}
	if err := systemProxySet(fmt.Sprintf("127.0.0.1:%d", s.ProxyPort)); err != nil {
		a.logger.Warn("the system proxy could not be enabled", "error", err)
		return
	}
	a.mu.Lock()
	a.proxyBefore = &snap
	a.proxyOn = true
	a.mu.Unlock()
	a.logger.Info("the system proxy now points at the local SOCKS5 listener", "port", s.ProxyPort)
}

// disableSystemProxy restores the proxy settings captured by
// enableSystemProxy.
func (a *App) disableSystemProxy() {
	a.mu.Lock()
	snap := a.proxyBefore
	a.proxyBefore = nil
	a.proxyOn = false
	a.mu.Unlock()
	if snap == nil {
		return
	}
	if err := systemProxyRestore(*snap); err != nil {
		a.logger.Warn("the system proxy could not be restored", "error", err)
		return
	}
	a.logger.Info("the system proxy was restored")
}

// TestProfile performs a full handshake with the server of a profile without
// bringing the tunnel up. It returns the round trip time in milliseconds.
func (a *App) TestProfile(id string) (int, error) {
	p, ok := a.store.Profile(id)
	if !ok {
		return 0, fmt.Errorf("app: profile %q not found", id)
	}
	if p.PrivateKey == "" {
		return 0, fmt.Errorf("app: the profile has no private key")
	}
	priv, err := crypto.ParsePrivateKey(p.PrivateKey)
	if err != nil {
		return 0, err
	}
	serverPub, err := crypto.ParsePublicKey(p.ServerKey)
	if err != nil {
		return 0, err
	}

	conn, err := net.Dial("udp", p.Host)
	if err != nil {
		return 0, fmt.Errorf("app: cannot reach %s: %w", p.Host, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(6 * time.Second))

	init, err := protocol.NewInitiator(priv, serverPub, 0)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	if _, err := conn.Write(init.CookieRequestBytes()); err != nil {
		return 0, err
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return 0, fmt.Errorf("the server did not answer: %w", err)
	}
	if kind, _ := protocol.TypeOf(buf[:n]); kind != protocol.TypeCookieResponse {
		return 0, fmt.Errorf("unexpected answer from the server (0x%02x)", kind)
	}
	if err := init.ApplyCookieResponse(buf[:n]); err != nil {
		return 0, fmt.Errorf("the cookie was rejected: %w", err)
	}
	msg, err := init.HandshakeInitBytes(time.Now())
	if err != nil {
		return 0, err
	}
	if _, err := conn.Write(msg); err != nil {
		return 0, err
	}
	n, err = conn.Read(buf)
	if err != nil {
		return 0, fmt.Errorf("the server did not answer the handshake: %w", err)
	}
	session, err := init.ApplyHandshakeResponse(buf[:n], time.Now())
	if err != nil {
		return 0, fmt.Errorf("the handshake was rejected: %w", err)
	}
	session.Close()

	rtt := int(time.Since(start).Milliseconds())
	a.mu.Lock()
	a.latencies[id] = rtt
	a.mu.Unlock()
	a.logger.Info("server reachable", "profile", p.Name, "rtt_ms", rtt)
	a.broadcast("profiles", a.Snapshot().Profiles)
	return rtt, nil
}

// InstallWintun downloads the Wintun driver used by the tunnel mode.
func (a *App) InstallWintun() error {
	a.setBusy("wintun-download")
	defer a.setBusy("")
	a.logger.Info("installing the wintun driver")
	if err := installWintun(context.Background()); err != nil {
		a.logger.Error("the wintun driver could not be installed", "error", err)
		return err
	}
	a.mu.Lock()
	a.wintun = wintunInfo()
	a.mu.Unlock()
	a.logger.Info("the wintun driver is installed", "version", a.wintun.Version)
	a.broadcast("state", a.Snapshot())
	return nil
}

// RelaunchAdmin restarts the application with administrator rights.
func (a *App) RelaunchAdmin() error {
	// Forward the flags of this instance (a custom -data-dir, -debug) so the
	// elevated copy behaves like the one it replaces.
	args := make([]string, 0, len(os.Args))
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-replaced") || strings.HasPrefix(arg, "--replaced") {
			continue
		}
		args = append(args, arg)
	}
	args = append(args, "-replaced=1")
	if err := relaunchElevated(args); err != nil {
		return err
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		a.Shutdown()
	}()
	return nil
}

// OpenExternal opens a URL in the default browser.
func (a *App) OpenExternal(url string) error {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return fmt.Errorf("app: refusing to open %q", url)
	}
	return openURL(url)
}

// OpenFolder opens one of the application directories in Explorer.
func (a *App) OpenFolder(which string) error {
	switch which {
	case "logs":
		return openPath(filepath.Join(a.dataDir, "logs"))
	case "data", "":
		return openPath(a.dataDir)
	default:
		return fmt.Errorf("app: unknown folder %q", which)
	}
}

// OpenDataFolder is a convenience wrapper used by the command line.
func (a *App) OpenDataFolder() error {
	if err := os.MkdirAll(a.dataDir, 0o700); err != nil {
		return err
	}
	return openPath(a.dataDir)
}

// setBusy records the operation that is currently blocking the interface.
func (a *App) setBusy(name string) {
	a.mu.Lock()
	a.busy = name
	a.mu.Unlock()
	a.broadcast("busy", map[string]string{"busy": name})
}

// generateKey returns a fresh static key pair in its base64 form.
func generateKey() (private string, public string, err error) {
	priv, pub, err := crypto.GenerateKey()
	if err != nil {
		return "", "", err
	}
	return priv.String(), pub.String(), nil
}

// ErrAlreadyRunning is returned when a second instance of the application is
// started.
var ErrAlreadyRunning = errors.New("app: W1nWire is already running")
