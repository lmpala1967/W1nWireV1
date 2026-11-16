package protocol

import (
	"errors"
	"fmt"
	"sync"

	"github.com/w1nchi/w1nwire/internal/crypto"
)

// ErrSessionClosed is returned when a session is used after it was closed.
var ErrSessionClosed = errors.New("protocol: session is closed")

// Session is an established data plane session between two peers.
//
// The session owns one key per direction. The send key is used to encrypt
// outgoing packets, the receive key to decrypt incoming ones. Each direction
// has its own counter space, which is used as the AEAD nonce.
type Session struct {
	mu sync.Mutex

	// PeerID identifies the remote peer, it is empty on the client side until
	// the server identifies itself.
	PeerID string

	// LocalIndex is the session index chosen by this side.
	LocalIndex uint32
	// RemoteIndex is the session index chosen by the remote side.
	RemoteIndex uint32

	// AssignedIP is the address the server assigned to the client. It is
	// meaningful on the client side only.
	AssignedIP [4]byte

	send    *crypto.AEAD
	recv    *crypto.AEAD
	counter uint64
	window  ReplayWindow
	closed  bool
}

// NewSession builds a session from the two directional keys.
func NewSession(peerID string, localIndex, remoteIndex uint32, sendKey, recvKey []byte, assignedIP [4]byte) (*Session, error) {
	send, err := crypto.NewAEAD(sendKey)
	if err != nil {
		return nil, err
	}
	recv, err := crypto.NewAEAD(recvKey)
	if err != nil {
		return nil, err
	}
	return &Session{
		PeerID:      peerID,
		LocalIndex:  localIndex,
		RemoteIndex: remoteIndex,
		AssignedIP:  assignedIP,
		send:        send,
		recv:        recv,
	}, nil
}

// Encrypt seals a payload and returns the datagram ready to be sent.
func (s *Session) Encrypt(payload []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrSessionClosed
	}
	counter := s.counter
	s.counter++
	ct := s.send.Seal(counter, payload)
	pkt := &DataPacket{
		ReceiverIndex: s.RemoteIndex,
		SenderIndex:   s.LocalIndex,
		Counter:       counter,
		Ciphertext:    ct,
	}
	return pkt.Encode(), nil
}

// EncryptKeepalive returns an encrypted empty payload.
func (s *Session) EncryptKeepalive() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrSessionClosed
	}
	counter := s.counter
	s.counter++
	tag := s.send.Seal(counter, nil)
	pkt := &Keepalive{
		ReceiverIndex: s.RemoteIndex,
		SenderIndex:   s.LocalIndex,
		Counter:       counter,
		Tag:           tag,
	}
	return pkt.Encode(), nil
}

// Decrypt opens a data packet. It rejects replayed packets.
func (s *Session) Decrypt(counter uint64, ciphertext []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrSessionClosed
	}
	// The packet is authenticated first and the replay window is moved
	// afterwards. A window that moves before the authentication would let a
	// forged packet with a huge counter shift the window and lock out every
	// legitimate packet of the session.
	out, err := s.recv.Open(counter, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("protocol: decrypt: %w", err)
	}
	if err := s.window.Accept(counter); err != nil {
		return nil, err
	}
	return out, nil
}

// DecryptKeepalive opens a keepalive packet.
func (s *Session) DecryptKeepalive(counter uint64, tag []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrSessionClosed
	}
	// The same order as in Decrypt: authenticate, then move the window.
	if _, err := s.recv.Open(counter, tag); err != nil {
		return fmt.Errorf("protocol: keepalive: %w", err)
	}
	return s.window.Accept(counter)
}

// SendCounter returns the next counter that will be used.
func (s *Session) SendCounter() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counter
}

// Close marks the session as unusable.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
}
