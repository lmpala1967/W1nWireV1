// Package server implements the W1nWire server.
//
// The server terminates the handshake of every client, relays the encrypted
// traffic and routes the packets of the tunnel into the network of the host.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/w1nchi/w1nwire/internal/config"
	"github.com/w1nchi/w1nwire/internal/crypto"
	"github.com/w1nchi/w1nwire/internal/metrics"
	"github.com/w1nchi/w1nwire/internal/protocol"
	"github.com/w1nchi/w1nwire/internal/tun"
	"github.com/w1nchi/w1nwire/internal/tunnel"
)

// handshakeTimeout is how long the server waits for the answer of a client.
const handshakeTimeout = 10 * time.Second

// keepaliveInterval is how often the server sends a keepalive packet.
const keepaliveInterval = 15 * time.Second

// sessionTimeout is how long a session survives without any traffic.
const sessionTimeout = 3 * time.Minute

// peerState holds the runtime state of a client.
type peerState struct {
	cfg      config.PeerConfig
	key      protocol.Peer
	session  *protocol.Session
	streams  *tunnel.ServerStreams
	addr     *net.UDPAddr
	lastSeen time.Time
	// allowed are the source prefixes this peer may use inside the tunnel.
	// A peer that sends a packet with a source address outside of them is
	// dropped: without the check a peer could answer for another peer.
	allowed []*net.IPNet
}

// allowsSource reports whether the peer may send a packet with this source
// address.
func (p *peerState) allowsSource(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, network := range p.allowed {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// Server is the W1nWire server.
type Server struct {
	cfg       *config.ServerConfig
	priv      crypto.PrivateKey
	peers     []*peerState
	byID      map[string]*peerState
	byIP      map[string]*peerState
	sessions  map[uint32]*peerState
	responder *protocol.Responder
	metrics   *metrics.Metrics
	logger    *slog.Logger
	device    tun.Device

	mu   sync.RWMutex
	conn *net.UDPConn
}

// New creates a server from a configuration.
func New(cfg *config.ServerConfig, m *metrics.Metrics, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	priv, err := crypto.ParsePrivateKey(cfg.ServerPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("server: private key: %w", err)
	}

	s := &Server{
		cfg:      cfg,
		priv:     priv,
		byID:     make(map[string]*peerState),
		byIP:     make(map[string]*peerState),
		sessions: make(map[uint32]*peerState),
		metrics:  m,
		logger:   logger,
	}

	protocolPeers := make([]protocol.Peer, 0, len(cfg.Peers))
	for _, pc := range cfg.Peers {
		pub, err := crypto.ParsePublicKey(pc.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("server: peer %s: %w", pc.ID, err)
		}
		ip := net.ParseIP(pc.TunnelAddress).To4()
		if ip == nil {
			return nil, fmt.Errorf("server: peer %s: tunnel address must be an IPv4 address", pc.ID)
		}
		state := &peerState{
			cfg: pc,
			key: protocol.Peer{
				ID:         pc.ID,
				Public:     pub,
				AssignedIP: [4]byte{ip[0], ip[1], ip[2], ip[3]},
				AllowedIPs: pc.AllowedIPs,
			},
		}
		allowed, err := parseAllowedIPs(pc, ip)
		if err != nil {
			return nil, fmt.Errorf("server: peer %s: %w", pc.ID, err)
		}
		state.allowed = allowed
		state.streams = tunnel.NewServerStreams(func(payload []byte) error {
			return s.sendToPeer(state, payload)
		}, logger.With("peer", pc.ID))
		s.peers = append(s.peers, state)
		s.byID[pc.ID] = state
		s.byIP[pc.TunnelAddress] = state
		protocolPeers = append(protocolPeers, state.key)
	}

	s.responder, err = protocol.NewResponder(priv, protocolPeers)
	if err != nil {
		return nil, fmt.Errorf("server: responder: %w", err)
	}
	return s, nil
}

// PublicKey returns the static public key of the server.
func (s *Server) PublicKey() crypto.PublicKey { return s.priv.Public() }

// Addr returns the address the server listens on, or nil before Run has bound
// the socket. It is used by embedded deployments and by the self test.
func (s *Server) Addr() net.Addr {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.conn == nil {
		return nil
	}
	return s.conn.LocalAddr()
}

// Run starts the server and blocks until the context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	addr, err := net.ResolveUDPAddr("udp", s.cfg.Listen)
	if err != nil {
		return fmt.Errorf("server: resolve listen address: %w", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("server: listen on %s: %w", s.cfg.Listen, err)
	}
	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()
	defer conn.Close()

	s.logger.Info("server is listening", "address", conn.LocalAddr().String(), "peers", len(s.peers))
	s.updatePeerMetrics()

	if err := s.setupTunnel(); err != nil {
		return err
	}

	udpCh := make(chan datagram, 512)
	tunCh := make(chan []byte, 512)

	go s.readUDP(udpCh)
	if s.device != nil {
		go s.readTUN(tunCh)
	}

	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()
	sweeper := time.NewTicker(30 * time.Second)
	defer sweeper.Stop()

	for {
		select {
		case <-ctx.Done():
			s.shutdown()
			return nil
		case d := <-udpCh:
			s.handleDatagram(d)
		case pkt := <-tunCh:
			s.handleTunnelPacket(pkt)
		case <-ticker.C:
			s.sendKeepalives()
		case <-sweeper.C:
			s.expireSessions()
		}
	}
}

func (s *Server) setupTunnel() error {
	if !s.cfg.TUN.Enabled {
		s.logger.Info("tunnel device is disabled, only the proxy mode is served")
		return nil
	}
	device, err := tun.Open(s.cfg.TUN.Name, s.cfg.TUN.MTU)
	if err != nil {
		return fmt.Errorf("server: open tunnel device: %w", err)
	}
	s.device = device

	if err := tun.ConfigureInterface(s.cfg.TUN.Name, s.cfg.TUN.Address, s.cfg.TUN.MTU); err != nil {
		return fmt.Errorf("server: configure tunnel device: %w", err)
	}
	if s.cfg.NAT.Enabled {
		_, subnet, _ := net.ParseCIDR(s.cfg.TUN.Address)
		if err := tun.ConfigureNAT(subnet.String(), s.cfg.NAT.OutboundInterface, s.cfg.TUN.Name); err != nil {
			s.logger.Warn("the masquerading rule could not be installed", "error", err)
		}
	}
	s.logger.Info("tunnel device is ready", "name", device.Name(), "address", s.cfg.TUN.Address)
	return nil
}

type datagram struct {
	data []byte
	addr *net.UDPAddr
}

func (s *Server) readUDP(out chan<- datagram) {
	buf := make([]byte, 65535)
	for {
		n, addr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		payload := make([]byte, n)
		copy(payload, buf[:n])
		out <- datagram{data: payload, addr: addr}
	}
}

func (s *Server) readTUN(out chan<- []byte) {
	buf := make([]byte, s.cfg.TUN.MTU+64)
	for {
		n, err := s.device.Read(buf)
		if err != nil {
			return
		}
		pkt := make([]byte, n)
		copy(pkt, buf[:n])
		out <- pkt
	}
}

func (s *Server) handleDatagram(d datagram) {
	if len(d.data) == 0 {
		return
	}
	// A malformed packet must never take the whole server down: the packet
	// is dropped, the server keeps serving the other peers.
	defer func() {
		if recovered := recover(); recovered != nil {
			s.metrics.DecryptErrors.Inc()
			s.logger.Error("a packet could not be handled", "panic", recovered)
		}
	}()
	switch d.data[0] {
	case protocol.TypeCookieRequest:
		s.handleCookieRequest(d)
	case protocol.TypeHandshakeInit:
		s.handleHandshakeInit(d)
	case protocol.TypeData:
		s.handleData(d)
	case protocol.TypeKeepalive:
		s.handleKeepalive(d)
	default:
		s.logger.Debug("unknown message type", "type", d.data[0])
	}
}

func (s *Server) handleCookieRequest(d datagram) {
	started := time.Now()
	response, err := s.responder.HandleCookieRequest(d.addr, d.data, time.Now())
	if err != nil {
		s.metrics.ObserveHandshake("failed", time.Since(started))
		s.logger.Debug("cookie request rejected", "error", err)
		return
	}
	s.write(d.addr, response)
}

func (s *Server) handleHandshakeInit(d datagram) {
	started := time.Now()
	response, session, err := s.responder.HandleHandshakeInit(d.addr, d.data, time.Now())
	if err != nil {
		s.metrics.ObserveHandshake("failed", time.Since(started))
		s.logger.Debug("handshake rejected", "error", err)
		return
	}

	s.mu.Lock()
	peer := s.byID[session.PeerID]
	if peer != nil {
		// A client may start a new handshake at any time, for example to
		// refresh the ephemeral keys. The old session is dropped, otherwise
		// it would stay in the table forever.
		if peer.session != nil {
			delete(s.sessions, peer.session.RemoteIndex)
			peer.session.Close()
		}
		peer.session = session
		peer.addr = d.addr
		peer.lastSeen = time.Now()
		s.sessions[session.RemoteIndex] = peer
	}
	s.mu.Unlock()

	if peer != nil {
		s.metrics.LastHandshake.WithLabelValues(peer.cfg.ID).SetToCurrentTime()
		s.updatePeerMetrics()
		s.logger.Info("client connected", "peer", peer.cfg.ID, "address", d.addr.String())
	}
	s.metrics.ObserveHandshake("success", time.Since(started))
	s.write(d.addr, response)
}

func (s *Server) handleData(d datagram) {
	pkt, err := protocol.DecodeData(d.data)
	if err != nil {
		s.metrics.DecryptErrors.Inc()
		return
	}
	s.mu.RLock()
	peer := s.sessions[pkt.SenderIndex]
	s.mu.RUnlock()
	if peer == nil || peer.session == nil {
		s.metrics.DecryptErrors.Inc()
		return
	}
	// The address of a client may change behind a NAT, the session is
	// authenticated anyway, so the address is simply updated.
	s.mu.Lock()
	if peer.addr == nil || peer.addr.String() != d.addr.String() {
		peer.addr = d.addr
	}
	peer.lastSeen = time.Now()
	s.mu.Unlock()

	payload, err := peer.session.Decrypt(pkt.Counter, pkt.Ciphertext)
	if err != nil {
		if errors.Is(err, protocol.ErrReplay) {
			s.metrics.ReplayDropped.Inc()
		} else {
			s.metrics.DecryptErrors.Inc()
		}
		return
	}
	s.metrics.Packets.WithLabelValues("rx", "data").Inc()
	s.metrics.Bytes.WithLabelValues("rx").Add(float64(len(payload)))
	if len(payload) == 0 {
		// A peer may legitimately seal an empty plaintext, so the frame
		// byte has to be checked before it is read.
		return
	}
	s.logger.Debug("payload received", "peer", peer.cfg.ID, "bytes", len(payload), "frame", payload[0])

	if payload[0] == tunnel.FrameIP {
		// A peer may only send packets whose source address belongs to it.
		// Without this check a peer could send packets with the address of
		// another peer and answer for it.
		pkt := payload[1:]
		source := sourceIP(pkt)
		if !peer.allowsSource(source) {
			s.metrics.SpoofedPackets.WithLabelValues(peer.cfg.ID).Inc()
			s.logger.Warn("a packet with a forbidden source address was dropped",
				"peer", peer.cfg.ID, "source", ipString(source))
			return
		}
		s.deliverToTunnel(pkt)
		return
	}
	if err := peer.streams.Handle(payload); err != nil {
		s.logger.Debug("stream frame rejected", "peer", peer.cfg.ID, "error", err)
	}
}

func (s *Server) handleKeepalive(d datagram) {
	pkt, err := protocol.DecodeKeepalive(d.data)
	if err != nil {
		return
	}
	s.mu.RLock()
	peer := s.sessions[pkt.SenderIndex]
	s.mu.RUnlock()
	if peer == nil || peer.session == nil {
		return
	}
	if err := peer.session.DecryptKeepalive(pkt.Counter, pkt.Tag); err != nil {
		return
	}
	s.mu.Lock()
	peer.lastSeen = time.Now()
	s.mu.Unlock()
	s.metrics.Packets.WithLabelValues("rx", "keepalive").Inc()
}

func (s *Server) handleTunnelPacket(pkt []byte) {
	ip := sourceIP(pkt)
	if ip == nil {
		return
	}
	s.mu.RLock()
	peer := s.byIP[ip.String()]
	s.mu.RUnlock()
	if peer == nil || peer.session == nil {
		return
	}
	if err := s.sendToPeer(peer, append([]byte{tunnel.FrameIP}, pkt...)); err != nil {
		s.logger.Debug("packet could not be sent", "peer", peer.cfg.ID, "error", err)
	}
}

func (s *Server) sendToPeer(peer *peerState, payload []byte) error {
	s.mu.RLock()
	session := peer.session
	addr := peer.addr
	s.mu.RUnlock()
	if session == nil || addr == nil {
		return errors.New("server: the peer has no session")
	}
	datagram, err := session.Encrypt(payload)
	if err != nil {
		return err
	}
	s.metrics.Packets.WithLabelValues("tx", "data").Inc()
	s.metrics.Bytes.WithLabelValues("tx").Add(float64(len(payload)))
	return s.write(addr, datagram)
}

func (s *Server) deliverToTunnel(pkt []byte) {
	if s.device == nil {
		return
	}
	if _, err := s.device.Write(pkt); err != nil {
		s.logger.Debug("packet could not be written to the tunnel device", "error", err)
	}
}

func (s *Server) sendKeepalives() {
	s.mu.RLock()
	peers := make([]*peerState, len(s.peers))
	copy(peers, s.peers)
	s.mu.RUnlock()

	for _, peer := range peers {
		s.mu.RLock()
		session := peer.session
		addr := peer.addr
		s.mu.RUnlock()
		if session == nil || addr == nil {
			continue
		}
		datagram, err := session.EncryptKeepalive()
		if err != nil {
			continue
		}
		s.metrics.Packets.WithLabelValues("tx", "keepalive").Inc()
		s.write(addr, datagram)
	}
}

func (s *Server) expireSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, peer := range s.peers {
		if peer.session == nil {
			continue
		}
		if time.Since(peer.lastSeen) > sessionTimeout {
			s.logger.Info("session expired", "peer", peer.cfg.ID)
			delete(s.sessions, peer.session.RemoteIndex)
			peer.session.Close()
			peer.session = nil
			peer.addr = nil
		}
	}
	s.updatePeerMetricsLocked()
}

// updatePeerMetrics refreshes the metrics that describe the state of the
// peers.
func (s *Server) updatePeerMetrics() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updatePeerMetricsLocked()
}

func (s *Server) updatePeerMetricsLocked() {
	handshaked := 0
	offline := 0
	sessions := 0
	for _, peer := range s.peers {
		if peer.session != nil {
			handshaked++
			sessions++
		} else {
			offline++
		}
	}
	s.metrics.Peers.WithLabelValues("handshaked").Set(float64(handshaked))
	s.metrics.Peers.WithLabelValues("initiating").Set(0)
	s.metrics.Peers.WithLabelValues("offline").Set(float64(offline))
	s.metrics.SessionsActive.Set(float64(sessions))
}

func (s *Server) write(addr *net.UDPAddr, payload []byte) error {
	if addr == nil {
		return errors.New("server: no destination address")
	}
	_, err := s.conn.WriteToUDP(payload, addr)
	return err
}

func (s *Server) shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, peer := range s.peers {
		if peer.session != nil {
			peer.session.Close()
		}
		peer.streams.Close()
	}
	if s.device != nil {
		s.device.Close()
	}
}

// sourceIP returns the source address of an IP packet.
func sourceIP(pkt []byte) net.IP {
	if len(pkt) < 1 {
		return nil
	}
	switch pkt[0] >> 4 {
	case 4:
		if len(pkt) < 20 {
			return nil
		}
		return net.IP(pkt[12:16]).To4()
	case 6:
		if len(pkt) < 40 {
			return nil
		}
		return net.IP(pkt[8:24])
	default:
		return nil
	}
}

// ipString renders an address for the log, an absent address included.
func ipString(ip net.IP) string {
	if ip == nil {
		return "none"
	}
	return ip.String()
}

// parseAllowedIPs turns the allowed_ips of a peer into a list of prefixes.
// When the list is empty the peer may only use its own address, which is
// the same rule WireGuard applies.
func parseAllowedIPs(pc config.PeerConfig, assigned net.IP) ([]*net.IPNet, error) {
	if len(pc.AllowedIPs) == 0 {
		network := &net.IPNet{IP: assigned, Mask: net.CIDRMask(32, 32)}
		return []*net.IPNet{network}, nil
	}
	allowed := make([]*net.IPNet, 0, len(pc.AllowedIPs))
	for _, entry := range pc.AllowedIPs {
		_, network, err := net.ParseCIDR(entry)
		if err != nil {
			return nil, fmt.Errorf("allowed_ips %q is not a prefix: %w", entry, err)
		}
		allowed = append(allowed, network)
	}
	return allowed, nil
}
