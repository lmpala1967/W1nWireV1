// Package integration contains the end to end test of the project.
//
// The test starts a real server and a real client on the loopback interface,
// connects to a local HTTP server through the SOCKS5 proxy of the client and
// checks that the answer arrives unchanged.
package integration

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/w1nchi/w1nwire/internal/client"
	"github.com/w1nchi/w1nwire/internal/config"
	"github.com/w1nchi/w1nwire/internal/crypto"
	"github.com/w1nchi/w1nwire/internal/metrics"
	"github.com/w1nchi/w1nwire/internal/server"
)

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

// TestTunnelCarriesHTTPTraffic runs the whole stack and pushes a request
// through the tunnel.
func TestTunnelCarriesHTTPTraffic(t *testing.T) {
	level := slog.LevelInfo
	if os.Getenv("W1NWire_DEBUG") != "" {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	serverPriv, serverPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	clientPriv, clientPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	serverPort := freePort(t)
	socksPort := freePort(t)

	dir := t.TempDir()
	serverConfigPath := filepath.Join(dir, "server.json")
	clientConfigPath := filepath.Join(dir, "client.json")

	serverConfig := config.ServerConfig{
		Listen:           fmt.Sprintf("127.0.0.1:%d", serverPort),
		ServerPrivateKey: serverPriv.String(),
		TUN:              config.TUNConfig{Enabled: false},
		HTTP:             config.HTTPConfig{Metrics: config.MetricsConfig{Enabled: false}},
		Peers: []config.PeerConfig{{
			ID:            "test-peer",
			PublicKey:     clientPub.String(),
			TunnelAddress: "10.66.0.2",
			AllowedIPs:    []string{"10.66.0.2/32"},
		}},
	}
	clientConfig := config.ClientConfig{
		Server: config.ServerEndpoint{
			Address:   fmt.Sprintf("127.0.0.1:%d", serverPort),
			PublicKey: serverPub.String(),
		},
		PrivateKey: clientPriv.String(),
		TUN:        config.ClientTUNConfig{Enabled: false},
		SOCKS5:     config.SOCKS5Config{Enabled: true, Listen: fmt.Sprintf("127.0.0.1:%d", socksPort)},
		HTTP:       config.HTTPConfig{Metrics: config.MetricsConfig{Enabled: false}},
	}

	writeJSON(t, serverConfigPath, serverConfig)
	writeJSON(t, clientConfigPath, clientConfig)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry := prometheus.NewRegistry()
	srv, err := server.New(mustLoadServer(t, serverConfigPath), metrics.New(registry), logger)
	if err != nil {
		t.Fatal(err)
	}
	go srv.Run(ctx)

	cli, err := client.New(mustLoadClient(t, clientConfigPath), metrics.New(prometheus.NewRegistry()), logger)
	if err != nil {
		t.Fatal(err)
	}
	go cli.Run(ctx)

	// The origin server that has to be reached through the tunnel.
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-W1nwire", "yes")
		fmt.Fprintf(w, "hello from the other side %s", r.URL.Path)
	}))
	defer origin.Close()

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialSOCKS5(ctx, "127.0.0.1", socksPort, addr)
			},
		},
		Timeout: 20 * time.Second,
	}

	var lastErr error
	for attempt := 0; attempt < 30; attempt++ {
		resp, err := client.Get(origin.URL + "/ping")
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.Header.Get("X-W1nwire") != "yes" {
				t.Fatalf("the answer did not come from the origin server: %s", body)
			}
			if string(body) != "hello from the other side /ping" {
				t.Fatalf("unexpected body %q", body)
			}
			return
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("the request did not reach the origin server: %v", lastErr)
}

// dialSOCKS5 opens a CONNECT tunnel through the local proxy.
func dialSOCKS5(ctx context.Context, host string, port int, target string) (net.Conn, error) {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, err
	}
	if err := conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		conn.Close()
		return nil, err
	}

	// Greeting: version 5, one method, no authentication.
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		conn.Close()
		return nil, err
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		conn.Close()
		return nil, err
	}
	if reply[0] != 0x05 || reply[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks5: the proxy refused the authentication method")
	}

	// CONNECT request with the host name, so the resolution happens on the
	// server side.
	name, portStr, err := net.SplitHostPort(target)
	if err != nil {
		conn.Close()
		return nil, err
	}
	request := []byte{0x05, 0x01, 0x00, 0x03, byte(len(name))}
	request = append(request, name...)
	portNumber := make([]byte, 2)
	binary.BigEndian.PutUint16(portNumber, uint16(atoi(portStr)))
	request = append(request, portNumber...)
	if _, err := conn.Write(request); err != nil {
		conn.Close()
		return nil, err
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		conn.Close()
		return nil, err
	}
	if header[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks5: the proxy refused the connection, code %d", header[1])
	}
	switch header[3] {
	case 0x01:
		if _, err := io.ReadFull(conn, make([]byte, 4+2)); err != nil {
			conn.Close()
			return nil, err
		}
	case 0x03:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			conn.Close()
			return nil, err
		}
		if _, err := io.ReadFull(conn, make([]byte, int(lenBuf[0])+2)); err != nil {
			conn.Close()
			return nil, err
		}
	case 0x04:
		if _, err := io.ReadFull(conn, make([]byte, 16+2)); err != nil {
			conn.Close()
			return nil, err
		}
	}
	conn.SetDeadline(time.Time{})
	return conn, nil
}

func atoi(s string) int {
	value := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return value
		}
		value = value*10 + int(r-'0')
	}
	return value
}

func marshalIndent(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := marshalIndent(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustLoadServer(t *testing.T, path string) *config.ServerConfig {
	t.Helper()
	cfg, err := config.LoadServer(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func mustLoadClient(t *testing.T, path string) *config.ClientConfig {
	t.Helper()
	cfg, err := config.LoadClient(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
