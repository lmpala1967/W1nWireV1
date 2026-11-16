// Package server contains the tests of the W1nWire server.
package server

import (
	"context"
	"encoding/binary"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/w1nchi/w1nwire/internal/config"
	"github.com/w1nchi/w1nwire/internal/crypto"
	"github.com/w1nchi/w1nwire/internal/metrics"
	"github.com/w1nchi/w1nwire/internal/protocol"
	"github.com/w1nchi/w1nwire/internal/tunnel"
)

// TestAPeerMayNotSpoofTheAddressOfAnotherPeer starts a real server and a
// real client session and sends a tunnel packet whose source address
// belongs to somebody else. The server has to drop it.
//
// The client session is built by hand: the client package needs a tunnel
// device, and this test needs to send a packet the client would never send.
func TestAPeerMayNotSpoofTheAddressOfAnotherPeer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	serverPriv, serverPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	clientPriv, clientPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	registry := prometheus.NewRegistry()
	m := metrics.New(registry)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	srv, err := New(&config.ServerConfig{
		Listen:           net.JoinHostPort("127.0.0.1", itoa(port)),
		ServerPrivateKey: serverPriv.String(),
		TUN:              config.TUNConfig{Enabled: false},
		HTTP:             config.HTTPConfig{Metrics: config.MetricsConfig{Enabled: false}},
		Peers: []config.PeerConfig{{
			ID:            "test-peer",
			PublicKey:     clientPub.String(),
			TunnelAddress: "10.66.0.2",
			AllowedIPs:    []string{"10.66.0.2/32"},
		}},
	}, m, logger)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Run(ctx)
	waitForServer(t, srv)

	// Build the session over the real socket, so that the server registers
	// it the same way it does for a real client.
	conn, err := net.Dial("udp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	now := time.Now()
	initiator, err := protocol.NewInitiator(clientPriv, serverPub, 0x33445566)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(initiator.CookieRequestBytes()); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if err := initiator.ApplyCookieResponse(buf[:n]); err != nil {
		t.Fatal(err)
	}
	initBytes, err := initiator.HandshakeInitBytes(now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(initBytes); err != nil {
		t.Fatal(err)
	}
	n, err = conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	session, err := initiator.ApplyHandshakeResponse(buf[:n], now)
	if err != nil {
		t.Fatal(err)
	}

	// A packet with the address of the peer itself is accepted.
	if _, err := sendTunnelPacket(conn, session, "10.66.0.2"); err != nil {
		t.Fatal(err)
	}
	waitForCounter(t, registry, "w1nwire_spoofed_packets_total", 0)

	// A packet with the address of somebody else is dropped.
	if _, err := sendTunnelPacket(conn, session, "10.66.0.3"); err != nil {
		t.Fatal(err)
	}
	waitForCounter(t, registry, "w1nwire_spoofed_packets_total", 1)
}

// TestAllowedIPsFallBackToTheAddressOfThePeer checks the default rule: a
// peer without allowed_ips may only use its own address.
func TestAllowedIPsFallBackToTheAddressOfThePeer(t *testing.T) {
	allowed, err := parseAllowedIPs(config.PeerConfig{}, net.ParseIP("10.66.0.7").To4())
	if err != nil {
		t.Fatal(err)
	}
	peer := &peerState{allowed: allowed}
	if !peer.allowsSource(net.ParseIP("10.66.0.7")) {
		t.Error("the peer may not use its own address")
	}
	if peer.allowsSource(net.ParseIP("10.66.0.8")) {
		t.Error("the peer may use an address that is not its own")
	}
	if peer.allowsSource(nil) {
		t.Error("a malformed packet was accepted")
	}
}

// TestAllowedIPsMustBePrefixes rejects a broken configuration instead of
// silently ignoring it.
func TestAllowedIPsMustBePrefixes(t *testing.T) {
	_, err := parseAllowedIPs(config.PeerConfig{AllowedIPs: []string{"not-a-prefix"}}, net.ParseIP("10.66.0.7").To4())
	if err == nil {
		t.Fatal("a broken prefix was accepted")
	}
}

// sendTunnelPacket sends one IPv4 packet with the given source address
// through the session and returns the datagram.
func sendTunnelPacket(conn net.Conn, session *protocol.Session, source string) ([]byte, error) {
	payload := buildIPv4(source, "1.1.1.1")
	frame := append([]byte{tunnel.FrameIP}, payload...)
	datagram, err := session.Encrypt(frame)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(datagram); err != nil {
		return nil, err
	}
	return datagram, nil
}

// buildIPv4 builds a minimal IPv4 packet with the given source address.
func buildIPv4(source, destination string) []byte {
	pkt := make([]byte, 20)
	pkt[0] = 0x45 // IPv4, 20 bytes of header
	binary.BigEndian.PutUint16(pkt[2:4], 20)
	copy(pkt[12:16], net.ParseIP(source).To4())
	copy(pkt[16:20], net.ParseIP(destination).To4())
	return pkt
}

// waitForServer waits until the server has bound its socket.
func waitForServer(t *testing.T, srv *Server) {
	t.Helper()
	for attempt := 0; attempt < 100; attempt++ {
		if srv.Addr() != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the server did not start")
}

// waitForCounter waits until a counter of the registry reaches the wanted
// value.
func waitForCounter(t *testing.T, registry *prometheus.Registry, name string, want float64) {
	t.Helper()
	for attempt := 0; attempt < 100; attempt++ {
		if counterValue(t, registry, name) >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s = %v, want at least %v", name, counterValue(t, registry, name), want)
}

// counterValue reads the value of a counter from the registry.
func counterValue(t *testing.T, registry *prometheus.Registry, name string) float64 {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		total := 0.0
		for _, metric := range family.GetMetric() {
			total += metric.GetCounter().GetValue()
		}
		return total
	}
	return 0
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var digits []byte
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}
