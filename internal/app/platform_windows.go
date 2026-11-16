//go:build windows

package app

import (
	"context"
	"fmt"
	"os"

	"github.com/w1nchi/w1nwire/internal/tun"
	"github.com/w1nchi/w1nwire/internal/winproxy"
	"github.com/w1nchi/w1nwire/internal/winutil"
)

// systemProxyState is the WinINET configuration captured before the
// application changed it.
type systemProxyState = winproxy.Snapshot

func isElevated() bool { return winutil.IsElevated() }

// wintunInfo inspects the tunnel driver of the current installation.
func wintunInfo() WintunView {
	path, version, err := tun.WintunStatus()
	switch {
	case path == "" && err == nil:
		return WintunView{Status: "missing"}
	case err != nil && path == "":
		return WintunView{Status: "missing", Detail: err.Error()}
	case err != nil:
		return WintunView{Status: "error", Version: version, Detail: err.Error()}
	default:
		return WintunView{Status: "ready", Version: version}
	}
}

func installWintun(ctx context.Context) error {
	_, err := tun.InstallWintun(ctx)
	return err
}

// prepareTunnel checks the two prerequisites of the tunnel mode: the rights
// to create a network adapter, and the Wintun driver itself.
func (a *App) prepareTunnel() error {
	if !isElevated() {
		a.logger.Warn("the tunnel mode was requested without administrator rights")
		return ErrElevation
	}
	a.mu.Lock()
	ready := a.wintun.Status == "ready"
	a.mu.Unlock()
	if ready {
		return nil
	}

	a.setBusy("wintun-download")
	defer a.setBusy("")
	a.logger.Info("downloading the wintun driver")
	err := installWintun(context.Background())
	a.mu.Lock()
	a.wintun = wintunInfo()
	info := a.wintun
	a.mu.Unlock()
	if err != nil {
		return fmt.Errorf("app: the wintun driver could not be installed: %w", err)
	}
	a.logger.Info("the wintun driver is ready", "version", info.Version)
	a.broadcast("state", a.Snapshot())
	return nil
}

func relaunchElevated(args []string) error {
	// The new instance is allowed to run next to the current one, otherwise
	// the single instance lock of the old process would reject it.
	return winutil.RelaunchElevated(args...)
}

func openURL(url string) error { return winutil.OpenURL(url) }

func openPath(path string) error {
	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
	}
	return winutil.OpenPath(path)
}

func setAutoStart(enabled bool) error { return winutil.SetAutoStart(enabled) }

func systemProxySnapshot() (systemProxyState, error) { return winproxy.Capture() }

func systemProxySet(addr string) error { return winproxy.Enable(addr) }

func systemProxyRestore(s systemProxyState) error { return winproxy.Restore(s) }

// cleanupStaleTunnel removes the routes a killed instance may have left
// behind. It is safe to call when no adapter exists.
func cleanupStaleTunnel() error { return tun.ResetInterface("W1nWire") }
