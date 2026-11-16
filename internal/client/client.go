// Package client implements the W1nWire client.
//
// The client offers two modes:
//
//   - the tunnel mode, where a TUN device carries the traffic of the whole
//     operating system. It needs Linux and root rights;
//   - the proxy mode, where a local SOCKS5 server forwards the traffic
//     through the tunnel. It works on every platform, including phones.
package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/w1nchi/w1nwire/internal/config"
	"github.com/w1nchi/w1nwire/internal/crypto"
	"github.com/w1nchi/w1nwire/internal/metrics"
	"github.com/w1nchi/w1nwire/internal/protocol"
	"github.com/w1nchi/w1nwire/internal/socks5"
	"github.com/w1nchi/w1nwire/internal/tun"
	"github.com/w1nchi/w1nwire/internal/tunnel"
)

const (
	// handshakeTimeout is how long the client waits for the answer of the
	// server during a handshake.
	handshakeTimeout = 10 * time.Second
	// keepaliveInterval is how often the client sends a keepalive packet.
	keepaliveInterval = 15 * time.Second
	// rekeyInterval is how often the client starts a new handshake. A new
	// handshake creates new ephemeral keys, which keeps the forward secrecy
	// of long running sessions.
	rekeyInterval = 2 * time.Minute
	// retryInterval is the pause between two failed handshake attempts.
	retryInterval = 5 * time.Second
)

// handshake states of the client.
const (
	stateIdle int = iota
	stateCookie
	stateInit
	stateReady
)

// Status is a snapshot of the runtime state of the client. It is designed to
// be polled by a user interface: every field is either immutable or updated
// behind the mutex of the client.
type Status struct {
	// State is one of "idle", "handshake", "connected".
	State string
	// Connected is true while a session is established.
	Connected bool
	// Since is the moment the current session was established.
	Since time.Time
	// AssignedIP is the address the server assigned to this client.
	AssignedIP string
	// LastHandshake is the moment of the last successful handshake.
	LastHandshake time.Time
	// RxBytes and TxBytes count the decrypted and encrypted payload bytes.
	RxBytes uint64
	TxBytes uint64
	// RxPackets and TxPackets count the data packets in each direction.
	RxPackets uint64
	TxPackets uint64
	// RttMs is the round trip time of the last handshake, in milliseconds.
	// It is zero until a handshake has completed.
	RttMs int
	// LastError describes the last problem that prevented a handshake.
	LastError string
}

// Client is the W1nWire client.
type Client struct {
	cfg       *config.ClientConfig
	priv      crypto.PrivateKey
	serverPub crypto.PublicKey
	metrics   *metrics.Metrics
	logger    *slog.Logger

	conn    *net.UDPConn
	device  tun.Device
	streams *tunnel.ClientStreams

	rxBytes   atomic.Uint64
	txBytes   atomic.Uint64
	rxPackets atomic.Uint64
	txPackets atomic.Uint64

	mu            sync.RWMutex
	session       *protocol.Session
	initiator     *protocol.Initiator
	state         int
	deadline      time.Time
	nextRetry     time.Time
	lastHandshake time.Time
	since         time.Time
	assignedIP    string
	lastError     string
	handshakeRTT  time.Duration
	handshakeAt   time.Time
}

// New creates a client from a configuration.
func New(cfg *config.ClientConfig, m *metrics.Metrics, logger *slog.Logger) (*Client, error) {
	if logger == nil {
		logger = slog.Default()
	}
	priv, err := crypto.ParsePrivateKey(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("client: private key: %w", err)
	}
	serverPub, err := crypto.ParsePublicKey(cfg.Server.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("client: server public key: %w", err)
	}
	return &Client{
		cfg:       cfg,
		priv:      priv,
		serverPub: serverPub,
		metrics:   m,
		logger:    logger,
		state:     stateIdle,
	}, nil
}

// Run starts the client and blocks until the context is cancelled.
func (c *Client) Run(ctx context.Context) error {
	remote, err := net.ResolveUDPAddr("udp", c.cfg.Server.Address)
	if err != nil {
		return fmt.Errorf("client: resolve server address: %w", err)
	}
	conn, err := net.DialUDP("udp", nil, remote)
	if err != nil {
		return fmt.Errorf("client: dial %s: %w", c.cfg.Server.Address, err)
	}
	c.conn = conn
	defer conn.Close()

	c.streams = tunnel.NewClientStreams(func(payload []byte) error {
		return c.sendPayload(payload)
	}, c.logger)

	if err := c.setupTunnel(); err != nil {
		// The device may already exist and the routes may already be
		// installed: close them before reporting the failure, otherwise the
		// machine keeps a broken default route.
		c.shutdown()
		return err
	}

	if c.cfg.SOCKS5.Enabled {
		proxy := socks5.NewServer(c.cfg.SOCKS5.Listen, c.openStream, c.logger)
		listener, err := proxy.Listen()
		if err != nil {
			c.shutdown()
			return err
		}
		go func() {
			if err := proxy.Serve(ctx, listener); err != nil {
				c.logger.Error("socks5 proxy stopped", "error", err)
			}
		}()
		c.logger.Info("socks5 proxy is enabled", "address", c.cfg.SOCKS5.Listen)
	}

	udpCh := make(chan []byte, 512)
	tunCh := make(chan []byte, 512)
	go c.readUDP(udpCh)
	if c.device != nil {
		go c.readTUN(tunCh)
	}

	keepalive := time.NewTicker(keepaliveInterval)
	defer keepalive.Stop()
	rekey := time.NewTicker(rekeyInterval)
	defer rekey.Stop()
	check := time.NewTicker(time.Second)
	defer check.Stop()

	c.logger.Info("client started", "server", c.cfg.Server.Address)

	for {
		select {
		case <-ctx.Done():
			c.shutdown()
			return nil
		case payload := <-udpCh:
			c.handleDatagram(payload)
		case pkt := <-tunCh:
			c.handleTunnelPacket(pkt)
		case <-keepalive.C:
			c.sendKeepalive()
		case <-rekey.C:
			c.startHandshake()
		case <-check.C:
			c.checkTimeouts()
		}
	}
}

func (c *Client) setupTunnel() error {
	if !c.cfg.TUN.Enabled {
		return nil
	}
	device, err := tun.Open(c.cfg.TUN.Name, c.cfg.TUN.MTU)
	if err != nil {
		return fmt.Errorf("client: open tunnel device: %w", err)
	}
	c.device = device

	if err := tun.ConfigureInterface(c.cfg.TUN.Name, c.cfg.TUN.Address, c.cfg.TUN.MTU); err != nil {
		return fmt.Errorf("client: configure tunnel device: %w", err)
	}

	// The resolver of the tunnel interface has to be the one from the
	// configuration. Without it the system keeps asking the resolver of the
	// physical network, whose replies would be routed into the tunnel and
	// lost.
	if configurable, ok := c.device.(tun.DNSConfigurer); ok && c.cfg.DNS != "" {
		if err := configurable.SetDNS([]string{c.cfg.DNS}); err != nil {
			c.logger.Warn("the resolver of the tunnel could not be configured", "error", err, "dns", c.cfg.DNS)
		} else {
			c.logger.Info("the resolver of the tunnel is set", "dns", c.cfg.DNS)
		}
	}

	tun.SetBlockIPv6(!c.cfg.TUN.AllowIPv6)

	if c.cfg.TUN.ConfigureRoutes {
		if err := tun.ConfigureDefaultRoute(c.cfg.TUN.Name, c.cfg.TUN.Gateway); err != nil {
			return fmt.Errorf("client: configure the default route: %w", err)
		}
		if !c.cfg.TUN.RouteAll {
			host, _, err := net.SplitHostPort(c.cfg.Server.Address)
			if err == nil {
				if err := tun.ConfigureBypassRoute(host); err != nil {
					c.logger.Warn("the bypass route could not be installed", "error", err)
				}
			}
		}
	}
	c.logger.Info("tunnel device is ready", "name", device.Name(), "address", c.cfg.TUN.Address)
	return nil
}

func (c *Client) readUDP(out chan<- []byte) {
	buf := make([]byte, 65535)
	for {
		n, err := c.conn.Read(buf)
		if err != nil {
			return
		}
		payload := make([]byte, n)
		copy(payload, buf[:n])
		out <- payload
	}
}

func (c *Client) readTUN(out chan<- []byte) {
	buf := make([]byte, c.cfg.TUN.MTU+64)
	for {
		n, err := c.device.Read(buf)
		if err != nil {
			return
		}
		pkt := make([]byte, n)
		copy(pkt, buf[:n])
		out <- pkt
	}
}

func (c *Client) handleDatagram(payload []byte) {
	if len(payload) == 0 {
		return
	}
	switch payload[0] {
	case protocol.TypeCookieResponse:
		c.handleCookieResponse(payload)
	case protocol.TypeHandshakeResp:
		c.handleHandshakeResponse(payload)
	case protocol.TypeData:
		c.handleData(payload)
	case protocol.TypeKeepalive:
		c.handleKeepalive(payload)
	default:
		c.logger.Debug("unknown message type", "type", payload[0])
	}
}

func (c *Client) handleCookieResponse(payload []byte) {
	c.mu.Lock()
	if c.state != stateCookie {
		c.mu.Unlock()
		return
	}
	initiator := c.initiator
	c.mu.Unlock()
	if initiator == nil {
		return
	}
	if err := initiator.ApplyCookieResponse(payload); err != nil {
		c.logger.Warn("the cookie was rejected", "error", err)
		c.resetHandshake()
		return
	}
	message, err := initiator.HandshakeInitBytes(time.Now())
	if err != nil {
		c.logger.Warn("the handshake could not be built", "error", err)
		c.resetHandshake()
		return
	}
	c.mu.Lock()
	c.state = stateInit
	c.handshakeAt = time.Now()
	c.mu.Unlock()
	if err := c.write(message); err != nil {
		c.logger.Warn("the handshake could not be sent", "error", err)
	}
}

func (c *Client) handleHandshakeResponse(payload []byte) {
	c.mu.Lock()
	if c.state != stateInit {
		c.mu.Unlock()
		return
	}
	initiator := c.initiator
	c.mu.Unlock()
	if initiator == nil {
		return
	}
	session, err := initiator.ApplyHandshakeResponse(payload, time.Now())
	if err != nil {
		c.logger.Warn("the server could not be authenticated", "error", err)
		c.resetHandshake()
		return
	}

	c.mu.Lock()
	if c.session != nil {
		c.session.Close()
	}
	c.session = session
	c.state = stateReady
	c.lastHandshake = time.Now()
	c.since = time.Now()
	ip := session.AssignedIP
	c.assignedIP = net.IP(ip[:]).String()
	if !c.handshakeAt.IsZero() {
		c.handshakeRTT = time.Since(c.handshakeAt)
		c.handshakeAt = time.Time{}
	}
	c.lastError = ""
	c.mu.Unlock()

	c.metrics.ClientConnected.WithLabelValues(c.cfg.Server.Address).Set(1)
	c.metrics.SessionsActive.Set(1)
	assigned := session.AssignedIP
	c.logger.Info("the tunnel is established", "server", c.cfg.Server.Address, "address", net.IP(assigned[:]).String())
}

func (c *Client) handleData(payload []byte) {
	pkt, err := protocol.DecodeData(payload)
	if err != nil {
		c.metrics.DecryptErrors.Inc()
		return
	}
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()
	if session == nil {
		return
	}
	plain, err := session.Decrypt(pkt.Counter, pkt.Ciphertext)
	if err != nil {
		if errors.Is(err, protocol.ErrReplay) {
			c.metrics.ReplayDropped.Inc()
		} else {
			c.metrics.DecryptErrors.Inc()
		}
		return
	}
	c.metrics.Packets.WithLabelValues("rx", "data").Inc()
	c.metrics.Bytes.WithLabelValues("rx").Add(float64(len(plain)))
	c.rxPackets.Add(1)
	c.rxBytes.Add(uint64(len(plain)))

	if len(plain) == 0 {
		return
	}
	if plain[0] == tunnel.FrameIP {
		if c.device != nil {
			if _, err := c.device.Write(plain[1:]); err != nil {
				c.logger.Debug("the packet could not be written to the tunnel device", "error", err)
			}
		}
		return
	}
	if err := c.streams.Handle(plain); err != nil {
		c.logger.Debug("stream frame rejected", "error", err)
	}
}

func (c *Client) handleKeepalive(payload []byte) {
	pkt, err := protocol.DecodeKeepalive(payload)
	if err != nil {
		return
	}
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()
	if session == nil {
		return
	}
	if err := session.DecryptKeepalive(pkt.Counter, pkt.Tag); err != nil {
		return
	}
	c.metrics.Packets.WithLabelValues("rx", "keepalive").Inc()
}

func (c *Client) handleTunnelPacket(pkt []byte) {
	payload := append([]byte{tunnel.FrameIP}, pkt...)
	if err := c.sendPayload(payload); err != nil {
		c.logger.Debug("the packet could not be sent", "error", err)
	}
}

func (c *Client) sendPayload(payload []byte) error {
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()
	if session == nil {
		return errors.New("client: the tunnel is not established")
	}
	datagram, err := session.Encrypt(payload)
	if err != nil {
		return err
	}
	c.metrics.Packets.WithLabelValues("tx", "data").Inc()
	c.metrics.Bytes.WithLabelValues("tx").Add(float64(len(payload)))
	c.txPackets.Add(1)
	c.txBytes.Add(uint64(len(payload)))
	return c.write(datagram)
}

func (c *Client) openStream(conn net.Conn, address string) error {
	_, err := c.streams.Open(conn, tunnel.ProtoTCP, address)
	return err
}

func (c *Client) sendKeepalive() {
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()
	if session == nil {
		return
	}
	datagram, err := session.EncryptKeepalive()
	if err != nil {
		return
	}
	c.metrics.Packets.WithLabelValues("tx", "keepalive").Inc()
	c.write(datagram)
}

func (c *Client) startHandshake() {
	c.mu.Lock()
	if c.state == stateCookie || c.state == stateInit {
		c.mu.Unlock()
		return
	}
	initiator, err := protocol.NewInitiator(c.priv, c.serverPub, 0)
	if err != nil {
		c.mu.Unlock()
		c.logger.Error("the handshake could not be started", "error", err)
		return
	}
	c.initiator = initiator
	c.state = stateCookie
	c.deadline = time.Now().Add(handshakeTimeout)
	message := initiator.CookieRequestBytes()
	c.mu.Unlock()

	if err := c.write(message); err != nil {
		c.logger.Warn("the cookie request could not be sent", "error", err)
	}
}

func (c *Client) checkTimeouts() {
	c.mu.Lock()
	state := c.state
	deadline := c.deadline
	c.mu.Unlock()

	switch state {
	case stateCookie, stateInit:
		if time.Now().After(deadline) {
			c.logger.Warn("the handshake timed out")
			c.setError("handshake timed out: the server did not answer")
			c.resetHandshake()
		}
	case stateIdle:
		c.mu.RLock()
		wait := c.nextRetry
		c.mu.RUnlock()
		if time.Now().After(wait) {
			c.startHandshake()
		}
	}
}

// setError records the reason of the last failed handshake.
func (c *Client) setError(message string) {
	c.mu.Lock()
	c.lastError = message
	c.mu.Unlock()
}

// Status returns a snapshot of the runtime state of the client.
func (c *Client) Status() Status {
	c.mu.RLock()
	state := c.state
	since := c.since
	assigned := c.assignedIP
	lastHandshake := c.lastHandshake
	lastError := c.lastError
	rtt := c.handshakeRTT
	connected := state == stateReady
	c.mu.RUnlock()

	name := "idle"
	switch state {
	case stateCookie, stateInit:
		name = "handshake"
	case stateReady:
		name = "connected"
	}
	return Status{
		State:         name,
		Connected:     connected,
		Since:         since,
		AssignedIP:    assigned,
		LastHandshake: lastHandshake,
		RxBytes:       c.rxBytes.Load(),
		TxBytes:       c.txBytes.Load(),
		RxPackets:     c.rxPackets.Load(),
		TxPackets:     c.txPackets.Load(),
		RttMs:         int(rtt.Milliseconds()),
		LastError:     lastError,
	}
}

func (c *Client) resetHandshake() {
	c.mu.Lock()
	c.state = stateIdle
	c.deadline = time.Time{}
	// The next attempt waits, so that a server that is down is not asked
	// once per second.
	c.nextRetry = time.Now().Add(retryInterval)
	c.mu.Unlock()
}

func (c *Client) write(payload []byte) error {
	_, err := c.conn.Write(payload)
	return err
}

func (c *Client) shutdown() {
	c.mu.Lock()
	session := c.session
	c.session = nil
	c.mu.Unlock()
	if session != nil {
		session.Close()
	}
	if c.streams != nil {
		c.streams.Close()
	}
	if c.device != nil {
		c.device.Close()
	}
	c.metrics.ClientConnected.WithLabelValues(c.cfg.Server.Address).Set(0)
	c.metrics.SessionsActive.Set(0)
	c.mu.Lock()
	c.state = stateIdle
	c.since = time.Time{}
	c.assignedIP = ""
	c.mu.Unlock()
}
