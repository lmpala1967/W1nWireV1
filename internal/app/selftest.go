package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/w1nchi/w1nwire/internal/client"
	"github.com/w1nchi/w1nwire/internal/config"
	"github.com/w1nchi/w1nwire/internal/crypto"
	"github.com/w1nchi/w1nwire/internal/metrics"
	"github.com/w1nchi/w1nwire/internal/server"
)

// selfTestMarker is the body returned by the origin server of the self test.
const selfTestMarker = "w1nwire-self-test-ok"

// SelfTest proves to the user that the application can actually carry
// traffic, without any external server and without a VPN driver.
//
// It starts a real W1nWire server on the loopback interface with the tunnel
// device disabled, a real W1nWire client in the proxy mode, and then performs
// a SOCKS5 CONNECT through that client to a small HTTP server. The whole path
// is exercised: handshake, key derivation, encryption, framed streams and the
// SOCKS5 listener.
//
// The tunnel mode cannot be tested this way because the server side needs a
// Linux TUN device; the proxy mode shares everything up to the TUN device.
func (a *App) SelfTest() error {
	a.mu.Lock()
	if a.conn != nil {
		a.mu.Unlock()
		return fmt.Errorf("app: disconnect before running the self test")
	}
	if a.busy != "" {
		a.mu.Unlock()
		return ErrBusy
	}
	a.busy = "self-test"
	a.mu.Unlock()
	defer a.setBusy("")

	a.logger.Info("self test: starting")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. The origin server that the client will reach through the tunnel.
	originLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("self test: listen for the origin: %w", err)
	}
	origin := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, selfTestMarker)
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go origin.Serve(originLn)
	defer origin.Close()
	originHost, originPort, _ := net.SplitHostPort(originLn.Addr().String())

	// 2. A real server, in the proxy mode (no TUN device).
	serverPriv, _, err := crypto.GenerateKey()
	if err != nil {
		return err
	}
	clientPriv, clientPub, err := crypto.GenerateKey()
	if err != nil {
		return err
	}
	serverCfg := &config.ServerConfig{
		Listen:           "127.0.0.1:0",
		ServerPrivateKey: serverPriv.String(),
		TUN:              config.TUNConfig{Enabled: false},
		Peers: []config.PeerConfig{{
			ID:            "self-test",
			PublicKey:     clientPub.String(),
			TunnelAddress: "10.66.0.2",
		}},
		HTTP: config.HTTPConfig{Metrics: config.MetricsConfig{Enabled: false}},
	}
	srv, err := server.New(serverCfg, metrics.New(prometheus.NewRegistry()), a.logger.With("component", "self-test-server"))
	if err != nil {
		return fmt.Errorf("self test: create the server: %w", err)
	}
	serverCtx, stopServer := context.WithCancel(ctx)
	defer stopServer()
	go func() {
		if err := srv.Run(serverCtx); err != nil {
			a.logger.Warn("self test: the server stopped", "error", err)
		}
	}()

	addr, err := waitForAddr(ctx, srv.Addr)
	if err != nil {
		return fmt.Errorf("self test: the server did not start: %w", err)
	}

	// 3. A real client in the proxy mode.
	proxyLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("self test: reserve a proxy port: %w", err)
	}
	proxyAddr := proxyLn.Addr().String()
	proxyLn.Close()

	clientCfg := &config.ClientConfig{
		Server:     config.ServerEndpoint{Address: addr, PublicKey: serverPriv.Public().String()},
		PrivateKey: clientPriv.String(),
		SOCKS5:     config.SOCKS5Config{Enabled: true, Listen: proxyAddr},
		DNS:        "1.1.1.1",
		HTTP:       config.HTTPConfig{Metrics: config.MetricsConfig{Enabled: false}},
	}
	cl, err := client.New(clientCfg, metrics.New(prometheus.NewRegistry()), a.logger.With("component", "self-test-client"))
	if err != nil {
		return fmt.Errorf("self test: create the client: %w", err)
	}
	clientCtx, stopClient := context.WithCancel(ctx)
	defer stopClient()
	go func() {
		if err := cl.Run(clientCtx); err != nil && !errors.Is(err, context.Canceled) {
			a.logger.Warn("self test: the client stopped", "error", err)
		}
	}()
	defer func() {
		stopClient()
		stopServer()
	}()

	if err := waitForConnected(ctx, cl, 15*time.Second); err != nil {
		return fmt.Errorf("self test: the tunnel did not come up: %w", err)
	}
	a.logger.Info("self test: the tunnel is up", "server", addr)

	// 4. A SOCKS5 CONNECT through the client to the origin server.
	port, _ := strconv.Atoi(originPort)
	body, err := socksHTTPGet(proxyAddr, originHost, port, "/", 10*time.Second)
	if err != nil {
		return fmt.Errorf("self test: the request through the tunnel failed: %w", err)
	}
	if !strings.Contains(body, selfTestMarker) {
		return fmt.Errorf("self test: unexpected body %q", truncate(body, 120))
	}

	a.logger.Info("self test: passed, the app can carry traffic", "through", addr, "proxy", proxyAddr)
	return nil
}

// waitForAddr waits until the server has bound its UDP socket.
func waitForAddr(ctx context.Context, addr func() net.Addr) (string, error) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got := addr(); got != nil && got.String() != "" {
			return got.String(), nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	return "", errors.New("timeout")
}

// waitForConnected waits until the client has an established session.
func waitForConnected(ctx context.Context, cl *client.Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cl.Status().Connected {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	return errors.New("timeout")
}

// socksHTTPGet performs a SOCKS5 CONNECT to host:port through the proxy and
// then a plain HTTP GET over the resulting connection. It is deliberately
// written by hand: importing a SOCKS5 client library for a self test would
// add a dependency to the binary for a hundred lines of code.
func socksHTTPGet(proxyAddr, host string, port int, path string, timeout time.Duration) (string, error) {
	conn, err := net.DialTimeout("tcp", proxyAddr, timeout)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	// Greeting: version 5, one method, "no authentication".
	if _, err := conn.Write([]byte{5, 1, 0}); err != nil {
		return "", err
	}
	var greeting [2]byte
	if _, err := io.ReadFull(conn, greeting[:]); err != nil {
		return "", err
	}
	if greeting[0] != 5 || greeting[1] != 0 {
		return "", fmt.Errorf("the proxy refused the authentication method (%v)", greeting[:])
	}

	// CONNECT with a domain name, which is what the client sends to the
	// server so that the name is resolved on the far side.
	req := []byte{5, 1, 0, 3, byte(len(host))}
	req = append(req, host...)
	req = append(req, byte(port>>8), byte(port))
	if _, err := conn.Write(req); err != nil {
		return "", err
	}

	var reply [4]byte
	if _, err := io.ReadFull(conn, reply[:]); err != nil {
		return "", err
	}
	if reply[0] != 5 || reply[1] != 0 {
		return "", fmt.Errorf("the proxy refused the connection (code %d)", reply[1])
	}
	switch reply[3] {
	case 1:
		var rest [6]byte
		if _, err := io.ReadFull(conn, rest[:]); err != nil {
			return "", err
		}
	case 3:
		var l [1]byte
		if _, err := io.ReadFull(conn, l[:]); err != nil {
			return "", err
		}
		buf := make([]byte, int(l[0])+2)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
	case 4:
		var rest [18]byte
		if _, err := io.ReadFull(conn, rest[:]); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("the proxy returned an unknown address type %d", reply[3])
	}

	request := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", path, host)
	if _, err := io.WriteString(conn, request); err != nil {
		return "", err
	}
	data, err := io.ReadAll(io.LimitReader(conn, 1<<20))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
