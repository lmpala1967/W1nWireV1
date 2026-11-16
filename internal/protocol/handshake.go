package protocol

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/w1nchi/w1nwire/internal/crypto"
)

// Key derivation labels. Every label is unique, the HKDF info strings
// separate the different keys from the same keying material.
const (
	infoMAC1    = "w1nwire/v1/mac1"
	infoCookie  = "w1nwire/v1/cookie"
	infoMsg1    = "w1nwire/v1/msg1"
	infoMsg2    = "w1nwire/v1/msg2"
	infoDataC2S = "w1nwire/v1/data/c2s"
	infoDataS2C = "w1nwire/v1/data/s2c"
)

// Handshake errors.
var (
	ErrHandshake    = errors.New("protocol: handshake failed")
	ErrCookie       = errors.New("protocol: invalid cookie")
	ErrUnknownPeer  = errors.New("protocol: unknown peer")
	ErrVersion      = errors.New("protocol: unsupported version")
	ErrNoCookie     = errors.New("protocol: cookie required")
	ErrBadHandshake = errors.New("protocol: unexpected handshake state")
)

// DefaultCookieTTL is how long an issued cookie stays valid.
const DefaultCookieTTL = 120 * time.Second

// MaxClockSkew is how much the clocks of the two ends may differ. The
// client rejects a server whose clock is further away than this, and the
// server rejects a client for the same reason, so the two checks are
// symmetric.
const MaxClockSkew = 5 * time.Minute

// Peer describes a client that is allowed to use the server.
type Peer struct {
	// ID is a human readable name, it is used for logging and metrics.
	ID string
	// Public is the static public key of the peer.
	Public crypto.PublicKey
	// AssignedIP is the address inside the tunnel subnet given to the peer.
	AssignedIP [4]byte
	// AllowedIPs restricts the source prefixes the peer may use. When it is
	// empty the assigned address is used.
	AllowedIPs []string
}

// Initiator drives the client side of the handshake.
type Initiator struct {
	privateKey   crypto.PrivateKey
	serverPublic crypto.PublicKey
	mac1Key      []byte

	senderIndex uint32
	cookie      []byte
	ephPriv     crypto.PrivateKey
	ephPub      crypto.PublicKey

	salt1 []byte
	salt2 []byte

	serverIndex uint32
	assignedIP  [4]byte
	flags       uint8
}

// NewInitiator creates the client side handshake state.
func NewInitiator(privateKey crypto.PrivateKey, serverPublic crypto.PublicKey, senderIndex uint32) (*Initiator, error) {
	mac1Key, err := crypto.Derive(serverPublic[:], nil, infoMAC1, crypto.KeySize)
	if err != nil {
		return nil, err
	}
	if senderIndex == 0 {
		raw, err := crypto.RandomBytes(4)
		if err != nil {
			return nil, err
		}
		senderIndex = binary.LittleEndian.Uint32(raw)
		if senderIndex == 0 {
			senderIndex = 1
		}
	}
	return &Initiator{
		privateKey:   privateKey,
		serverPublic: serverPublic,
		mac1Key:      mac1Key,
		senderIndex:  senderIndex,
	}, nil
}

// SenderIndex returns the session index chosen by the client.
func (i *Initiator) SenderIndex() uint32 { return i.senderIndex }

// CookieRequestBytes builds the first datagram of the handshake.
func (i *Initiator) CookieRequestBytes() []byte {
	mac1 := crypto.MAC(i.mac1Key, u32le(i.senderIndex))
	msg := &CookieRequest{Version: Version, SenderIndex: i.senderIndex}
	copy(msg.MAC1[:], mac1)
	return msg.Encode()
}

// ApplyCookieResponse stores the cookie issued by the server and prepares the
// ephemeral key pair for the key exchange.
func (i *Initiator) ApplyCookieResponse(b []byte) error {
	msg, err := DecodeCookieResponse(b)
	if err != nil {
		return err
	}
	if msg.Version != Version {
		return fmt.Errorf("%w: %d", ErrVersion, msg.Version)
	}
	if msg.SenderIndex != i.senderIndex {
		return fmt.Errorf("%w: index mismatch", ErrBadHandshake)
	}
	if len(msg.Cookie) == 0 {
		return fmt.Errorf("%w: empty cookie", ErrCookie)
	}
	i.cookie = msg.Cookie

	priv, pub, err := crypto.GenerateKey()
	if err != nil {
		return err
	}
	i.ephPriv, i.ephPub = priv, pub
	return nil
}

// HandshakeInitBytes builds the key exchange message.
func (i *Initiator) HandshakeInitBytes(now time.Time) ([]byte, error) {
	if len(i.cookie) == 0 {
		return nil, ErrNoCookie
	}

	salt1, err := i.computeSalt1()
	if err != nil {
		return nil, err
	}
	i.salt1 = salt1

	dhEphemeral, err := i.ephPriv.DH(i.serverPublic)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHandshake, err)
	}
	dhStatic, err := i.privateKey.DH(i.serverPublic)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHandshake, err)
	}

	key1, err := crypto.Derive(concat(dhEphemeral, dhStatic), salt1, infoMsg1, crypto.KeySize)
	if err != nil {
		return nil, err
	}
	aead, err := crypto.NewAEAD(key1)
	if err != nil {
		return nil, err
	}

	plaintext := make([]byte, 0, crypto.KeySize+8)
	staticPub := i.privateKey.Public()
	plaintext = append(plaintext, staticPub[:]...)
	plaintext = append(plaintext, u64le(uint64(now.Unix()))...)
	ciphertext := aead.Seal(0, plaintext)

	return (&HandshakeInit{
		Version:     Version,
		SenderIndex: i.senderIndex,
		Ephemeral:   i.ephPub,
		Cookie:      i.cookie,
		Ciphertext:  ciphertext,
	}).Encode(), nil
}

// ApplyHandshakeResponse verifies the server and returns the data session.
func (i *Initiator) ApplyHandshakeResponse(b []byte, now time.Time) (*Session, error) {
	msg, err := DecodeHandshakeResp(b)
	if err != nil {
		return nil, err
	}
	if msg.Version != Version {
		return nil, fmt.Errorf("%w: %d", ErrVersion, msg.Version)
	}
	if msg.SenderIndex != i.senderIndex {
		return nil, fmt.Errorf("%w: index mismatch", ErrBadHandshake)
	}

	dhEphemeral, err := i.ephPriv.DH(msg.Ephemeral)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHandshake, err)
	}
	dhStatic, err := i.privateKey.DH(msg.Ephemeral)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHandshake, err)
	}
	// dhStatic is DH(s_i, e_r): only the real server owns the matching
	// private key, so a valid decryption authenticates the server.

	salt2, err := i.computeSalt2(msg.Ephemeral)
	if err != nil {
		return nil, err
	}
	i.salt2 = salt2

	key2, err := crypto.Derive(concat(dhEphemeral, dhStatic), salt2, infoMsg2, crypto.KeySize)
	if err != nil {
		return nil, err
	}
	aead, err := crypto.NewAEAD(key2)
	if err != nil {
		return nil, err
	}
	plaintext, err := aead.Open(0, msg.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("%w: server is not authentic: %v", ErrHandshake, err)
	}
	if len(plaintext) < 17 {
		return nil, fmt.Errorf("%w: short response payload", ErrHandshake)
	}
	serverTS := int64(binary.LittleEndian.Uint64(plaintext[0:8]))
	if absDuration(now.Sub(time.Unix(serverTS, 0))) > MaxClockSkew {
		return nil, fmt.Errorf("%w: server timestamp is out of sync", ErrHandshake)
	}
	serverIndex := binary.LittleEndian.Uint32(plaintext[8:12])
	copy(i.assignedIP[:], plaintext[12:16])
	i.flags = plaintext[16]
	i.serverIndex = serverIndex

	sendKey, recvKey, err := i.deriveDataKeys(dhEphemeral)
	if err != nil {
		return nil, err
	}
	return NewSession("", i.senderIndex, serverIndex, sendKey, recvKey, i.assignedIP)
}

// AssignedIP returns the tunnel address assigned by the server.
func (i *Initiator) AssignedIP() [4]byte { return i.assignedIP }

// ServerIndex returns the session index chosen by the server.
func (i *Initiator) ServerIndex() uint32 { return i.serverIndex }

func (i *Initiator) computeSalt1() ([]byte, error) {
	h := sha256.New()
	h.Write([]byte(transcriptLabel))
	h.Write([]byte{Version})
	h.Write(u32le(i.senderIndex))
	h.Write(i.ephPub[:])
	h.Write(i.cookie)
	return h.Sum(nil), nil
}

func (i *Initiator) computeSalt2(serverEphemeral crypto.PublicKey) ([]byte, error) {
	if i.salt1 == nil {
		return nil, ErrNoCookie
	}
	h := sha256.New()
	h.Write(i.salt1)
	h.Write(serverEphemeral[:])
	return h.Sum(nil), nil
}

func (i *Initiator) deriveDataKeys(dhEphemeral []byte) (send, recv []byte, err error) {
	salt := i.salt2
	if salt == nil {
		return nil, nil, ErrBadHandshake
	}
	return crypto.DeriveTwo(dhEphemeral, salt, infoDataC2S, infoDataS2C)
}

// Responder drives the server side of the handshake.
type Responder struct {
	privateKey   crypto.PrivateKey
	serverPublic crypto.PublicKey
	mac1Key      []byte
	cookieKey    []byte
	cookieTTL    time.Duration
	peers        []Peer

	// mu guards lastTimestamp, the anti replay state of the peers.
	mu sync.Mutex
	// lastTimestamp remembers the newest client timestamp accepted for a
	// peer. A handshake that carries an older timestamp is a replay of a
	// message that was already used.
	lastTimestamp map[string]int64
}

// NewResponder creates the server side handshake state.
func NewResponder(privateKey crypto.PrivateKey, peers []Peer) (*Responder, error) {
	serverPublic := privateKey.Public()
	mac1Key, err := crypto.Derive(serverPublic[:], nil, infoMAC1, crypto.KeySize)
	if err != nil {
		return nil, err
	}
	cookieKey, err := crypto.RandomBytes(crypto.KeySize)
	if err != nil {
		return nil, err
	}
	return &Responder{
		privateKey:    privateKey,
		serverPublic:  serverPublic,
		mac1Key:       mac1Key,
		cookieKey:     cookieKey,
		cookieTTL:     DefaultCookieTTL,
		peers:         peers,
		lastTimestamp: make(map[string]int64, len(peers)),
	}, nil
}

// SetCookieTTL overrides how long issued cookies stay valid.
func (r *Responder) SetCookieTTL(ttl time.Duration) { r.cookieTTL = ttl }

// PublicKey returns the static public key of the server.
func (r *Responder) PublicKey() crypto.PublicKey { return r.serverPublic }

// HandleCookieRequest answers a cookie request.
//
// The cookie is a stateless AEAD blob that binds the observed source address
// and an expiry time. It allows the server to drop spoofed handshakes before
// it spends CPU on the peer lookup.
func (r *Responder) HandleCookieRequest(src *net.UDPAddr, b []byte, now time.Time) ([]byte, error) {
	msg, err := DecodeCookieRequest(b)
	if err != nil {
		return nil, err
	}
	if msg.Version != Version {
		return nil, fmt.Errorf("%w: %d", ErrVersion, msg.Version)
	}
	expected := crypto.MAC(r.mac1Key, u32le(msg.SenderIndex))
	if !crypto.ConstantTimeEqual(expected, msg.MAC1[:]) {
		return nil, fmt.Errorf("%w: bad mac1", ErrCookie)
	}

	cookie, err := r.issueCookie(src, now)
	if err != nil {
		return nil, err
	}
	return (&CookieResponse{
		Version:     Version,
		SenderIndex: msg.SenderIndex,
		MAC1:        msg.MAC1,
		Cookie:      cookie,
	}).Encode(), nil
}

func (r *Responder) issueCookie(src *net.UDPAddr, now time.Time) ([]byte, error) {
	nonceSeed, err := crypto.RandomBytes(12)
	if err != nil {
		return nil, err
	}
	aead, err := crypto.NewAEAD(r.cookieKey)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, 0, 8+18)
	plaintext = append(plaintext, u64le(uint64(now.Add(r.cookieTTL).Unix()))...)
	plaintext = append(plaintext, encodeAddr(src)...)
	sealed, err := aead.SealAt(nonceSeed, plaintext)
	if err != nil {
		return nil, err
	}
	return append(nonceSeed, sealed...), nil
}

// VerifyCookie checks the cookie and returns the expiry time.
func (r *Responder) VerifyCookie(src *net.UDPAddr, cookie []byte, now time.Time) (time.Time, error) {
	if len(cookie) < 12+16 {
		return time.Time{}, fmt.Errorf("%w: too short", ErrCookie)
	}
	aead, err := crypto.NewAEAD(r.cookieKey)
	if err != nil {
		return time.Time{}, err
	}
	plaintext, err := aead.OpenAt(cookie[:12], cookie[12:])
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %v", ErrCookie, err)
	}
	if len(plaintext) < 8+18 {
		return time.Time{}, fmt.Errorf("%w: malformed payload", ErrCookie)
	}
	expiry := time.Unix(int64(binary.LittleEndian.Uint64(plaintext[0:8])), 0)
	if now.After(expiry) {
		return time.Time{}, fmt.Errorf("%w: expired", ErrCookie)
	}
	if !crypto.ConstantTimeEqual(plaintext[8:8+18], encodeAddr(src)) {
		return time.Time{}, fmt.Errorf("%w: address mismatch", ErrCookie)
	}
	return expiry, nil
}

// HandleHandshakeInit processes the key exchange message and returns the
// response together with the freshly established session.
func (r *Responder) HandleHandshakeInit(src *net.UDPAddr, b []byte, now time.Time) ([]byte, *Session, error) {
	msg, err := DecodeHandshakeInit(b)
	if err != nil {
		return nil, nil, err
	}
	if msg.Version != Version {
		return nil, nil, fmt.Errorf("%w: %d", ErrVersion, msg.Version)
	}

	// The cookie binds the handshake to the address the server observed and
	// proves that the source is able to receive replies. It is validated
	// before the expensive peer lookup happens.
	if _, err := r.VerifyCookie(src, msg.Cookie, now); err != nil {
		return nil, nil, err
	}

	salt1, err := r.computeSalt1(msg.SenderIndex, msg.Ephemeral, msg.Cookie)
	if err != nil {
		return nil, nil, err
	}

	peer, err := r.findPeer(msg, salt1, now)
	if err != nil {
		return nil, nil, err
	}

	ephPriv, ephPub, err := crypto.GenerateKey()
	if err != nil {
		return nil, nil, err
	}

	salt2, err := r.computeSalt2(salt1, ephPub)
	if err != nil {
		return nil, nil, err
	}

	dhEphemeral, err := ephPriv.DH(msg.Ephemeral)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrHandshake, err)
	}
	// The second term is the ephemeral-static DH: it proves to the client
	// that the server owns the private key of its static key pair.
	dhEphemeralStatic, err := ephPriv.DH(peer.Public)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrHandshake, err)
	}

	key2, err := crypto.Derive(concat(dhEphemeral, dhEphemeralStatic), salt2, infoMsg2, crypto.KeySize)
	if err != nil {
		return nil, nil, err
	}
	aead, err := crypto.NewAEAD(key2)
	if err != nil {
		return nil, nil, err
	}

	indexRaw, err := crypto.RandomBytes(4)
	if err != nil {
		return nil, nil, err
	}
	serverIndex := binary.LittleEndian.Uint32(indexRaw)
	if serverIndex == 0 {
		serverIndex = 1
	}

	plaintext := make([]byte, 0, 17)
	plaintext = append(plaintext, u64le(uint64(now.Unix()))...)
	plaintext = append(plaintext, u32le(serverIndex)...)
	plaintext = append(plaintext, peer.AssignedIP[:]...)
	plaintext = append(plaintext, 0x01)

	response := (&HandshakeResp{
		Version:     Version,
		SenderIndex: msg.SenderIndex,
		Ephemeral:   ephPub,
		Ciphertext:  aead.Seal(0, plaintext),
	}).Encode()

	sendKey, recvKey, err := crypto.DeriveTwo(dhEphemeral, salt2, infoDataS2C, infoDataC2S)
	if err != nil {
		return nil, nil, err
	}
	session, err := NewSession(peer.ID, serverIndex, msg.SenderIndex, sendKey, recvKey, peer.AssignedIP)
	if err != nil {
		return nil, nil, err
	}
	return response, session, nil
}

// findPeer tries every configured peer until one of them can open the
// handshake message. Only the holder of the matching private key produces a
// valid key1, therefore a successful decryption authenticates the client.
func (r *Responder) findPeer(msg *HandshakeInit, salt1 []byte, now time.Time) (*Peer, error) {
	for idx := range r.peers {
		peer := &r.peers[idx]
		dhEphemeral, err := r.privateKey.DH(msg.Ephemeral)
		if err != nil {
			return nil, err
		}
		dhStatic, err := r.privateKey.DH(peer.Public)
		if err != nil {
			return nil, err
		}
		key1, err := crypto.Derive(concat(dhEphemeral, dhStatic), salt1, infoMsg1, crypto.KeySize)
		if err != nil {
			return nil, err
		}
		aead, err := crypto.NewAEAD(key1)
		if err != nil {
			return nil, err
		}
		plaintext, err := aead.Open(0, msg.Ciphertext)
		if err != nil {
			continue
		}
		if len(plaintext) < crypto.KeySize+8 {
			return nil, fmt.Errorf("%w: short handshake payload", ErrHandshake)
		}
		var static crypto.PublicKey
		copy(static[:], plaintext[:crypto.KeySize])
		if !crypto.ConstantTimeEqual(static[:], peer.Public[:]) {
			return nil, fmt.Errorf("%w: static key mismatch", ErrHandshake)
		}
		clientTS := int64(binary.LittleEndian.Uint64(plaintext[crypto.KeySize : crypto.KeySize+8]))
		if err := r.checkTimestamp(peer.ID, clientTS, now); err != nil {
			return nil, err
		}
		return peer, nil
	}
	return nil, fmt.Errorf("%w: no peer matched the handshake", ErrUnknownPeer)
}

// checkTimestamp rejects a handshake whose timestamp is out of sync with the
// clock of the server, and a handshake that replays a timestamp that was
// already used for the same peer.
//
// Without this a captured handshake could be sent again and would replace
// the session of the victim.
func (r *Responder) checkTimestamp(peerID string, clientTS int64, now time.Time) error {
	if absDuration(now.Sub(time.Unix(clientTS, 0))) > MaxClockSkew {
		return fmt.Errorf("%w: the client clock is out of sync", ErrHandshake)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if previous, seen := r.lastTimestamp[peerID]; seen && clientTS < previous {
		return fmt.Errorf("%w: the handshake has already been used", ErrHandshake)
	}
	r.lastTimestamp[peerID] = clientTS
	return nil
}

func (r *Responder) computeSalt1(senderIndex uint32, ephemeral crypto.PublicKey, cookie []byte) ([]byte, error) {
	h := sha256.New()
	h.Write([]byte(transcriptLabel))
	h.Write([]byte{Version})
	h.Write(u32le(senderIndex))
	h.Write(ephemeral[:])
	h.Write(cookie)
	return h.Sum(nil), nil
}

func (r *Responder) computeSalt2(salt1 []byte, serverEphemeral crypto.PublicKey) ([]byte, error) {
	h := sha256.New()
	h.Write(salt1)
	h.Write(serverEphemeral[:])
	return h.Sum(nil), nil
}

const transcriptLabel = "w1nwire/v1/transcript"

func concat(parts ...[]byte) []byte {
	size := 0
	for _, p := range parts {
		size += len(p)
	}
	out := make([]byte, 0, size)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func u32le(v uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, v)
	return buf
}

func u64le(v uint64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, v)
	return buf
}

// encodeAddr serialises an UDP address into 18 bytes: the IPv6 mapped address
// followed by the port in big endian.
//
// An IPv4 address becomes the IPv4 mapped form ::ffff:a.b.c.d, so the four
// octets live in the last four bytes of the address. Getting this wrong
// would silently drop two octets of the address and the cookie would then
// accept a different source address.
func encodeAddr(a *net.UDPAddr) []byte {
	out := make([]byte, 18)
	if a == nil {
		return out
	}
	ip := a.IP
	if v4 := ip.To4(); v4 != nil {
		out[10] = 0xff
		out[11] = 0xff
		copy(out[12:16], v4)
	} else if v6 := ip.To16(); v6 != nil {
		copy(out, v6)
	}
	binary.BigEndian.PutUint16(out[16:], uint16(a.Port))
	return out
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
