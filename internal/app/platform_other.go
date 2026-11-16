//go:build !windows

// The desktop application is built for Windows. This file keeps the package
// buildable on the other platforms so that the rest of the code, and the
// tests, can run in CI on Linux.

package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
)

// systemProxyState is unused outside Windows.
type systemProxyState struct{}

// ErrTunnelUnsupported is returned when the tunnel mode is requested on a
// platform without the Wintun driver.
var ErrTunnelUnsupported = errors.New("app: the tunnel mode is only available on Windows, use the proxy mode")

func isElevated() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	return os.Geteuid() == 0
}

func wintunInfo() WintunView {
	return WintunView{Status: "unsupported", Detail: "the tunnel driver is only available on Windows"}
}

func installWintun(context.Context) error { return ErrTunnelUnsupported }

func (a *App) prepareTunnel() error { return ErrTunnelUnsupported }

func relaunchElevated([]string) error {
	return errors.New("app: restarting with administrator rights is only available on Windows")
}

func openURL(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func openPath(path string) error { return openURL(path) }

func setAutoStart(bool) error { return nil }

func systemProxySnapshot() (systemProxyState, error) { return systemProxyState{}, nil }

func systemProxySet(string) error {
	return errors.New("app: the system proxy is only available on Windows")
}

func systemProxyRestore(systemProxyState) error { return nil }

// cleanupStaleTunnel is a no-op outside Windows.
func cleanupStaleTunnel() error { return nil }
