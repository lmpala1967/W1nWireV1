// Package protocol implements the W1nWire wire protocol: message framing, the
// noise style handshake and the encrypted data plane.
//
// The protocol runs over UDP. Every datagram starts with a single type byte
// followed by a version byte. All integers are little endian.
//
//	+--------+--------+-----------------+
//	| type   | ver    | payload         |
//	+--------+--------+-----------------+
//
// The full specification lives in docs/PROTOCOL.md.
package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/w1nchi/w1nwire/internal/crypto"
)

// Version is the current protocol version.
const Version uint8 = 1

// Message types.
const (
	TypeCookieRequest  byte = 0x01
	TypeCookieResponse byte = 0x02
	TypeHandshakeInit  byte = 0x03
	TypeHandshakeResp  byte = 0x04
	TypeData           byte = 0x05
	TypeKeepalive      byte = 0x06
)

// Sizes of the fixed size protocol fields.
const (
	KeySize     = crypto.KeySize
	MACSize     = 32
	MaxDatagram = 65535
	// MaxPayload is the largest plain text packet that fits into a datagram
	// after accounting for the framing overhead.
	MaxPayload = 1400
)

// ErrMalformed is returned when a datagram does not follow the protocol.
var ErrMalformed = errors.New("protocol: malformed message")

// CookieRequest is the first message of a handshake. The client sends it to
// obtain a cookie that proves it is able to receive replies at its address.
// The server answers with a CookieResponse.
//
//	+--------+--------+-----------+------------------+
//	| 0x01   | ver    | senderIdx | mac1 (32 bytes)  |
//	+--------+--------+-----------+------------------+
//
// mac1 = HMAC-SHA256(macKey, srcAddr || senderIdx)
type CookieRequest struct {
	Version     uint8
	SenderIndex uint32
	MAC1        [MACSize]byte
}

// Encode serialises the message.
func (m *CookieRequest) Encode() []byte {
	buf := make([]byte, 2+4+MACSize)
	buf[0] = TypeCookieRequest
	buf[1] = m.Version
	binary.LittleEndian.PutUint32(buf[2:6], m.SenderIndex)
	copy(buf[6:], m.MAC1[:])
	return buf
}

// DecodeCookieRequest parses a datagram.
func DecodeCookieRequest(b []byte) (*CookieRequest, error) {
	if len(b) < 2+4+MACSize {
		return nil, fmt.Errorf("%w: cookie request is %d bytes", ErrMalformed, len(b))
	}
	if b[0] != TypeCookieRequest {
		return nil, fmt.Errorf("%w: unexpected type 0x%02x", ErrMalformed, b[0])
	}
	m := &CookieRequest{Version: b[1]}
	m.SenderIndex = binary.LittleEndian.Uint32(b[2:6])
	copy(m.MAC1[:], b[6:2+4+MACSize])
	return m, nil
}

// CookieResponse carries the cookie issued by the server.
//
//	+--------+--------+-----------+------------------+------------------+
//	| 0x02   | ver    | senderIdx | mac1 (32 bytes)  | cookie           |
//	+--------+--------+-----------+------------------+------------------+
//
// The cookie is an AEAD sealed blob holding the expiry time and the address
// the cookie was issued for. It is stateless on the server side.
type CookieResponse struct {
	Version     uint8
	SenderIndex uint32
	MAC1        [MACSize]byte
	Cookie      []byte
}

// Encode serialises the message.
func (m *CookieResponse) Encode() []byte {
	buf := make([]byte, 2+4+MACSize+len(m.Cookie))
	buf[0] = TypeCookieResponse
	buf[1] = m.Version
	binary.LittleEndian.PutUint32(buf[2:6], m.SenderIndex)
	copy(buf[6:2+4+MACSize], m.MAC1[:])
	copy(buf[2+4+MACSize:], m.Cookie)
	return buf
}

// DecodeCookieResponse parses a datagram.
func DecodeCookieResponse(b []byte) (*CookieResponse, error) {
	if len(b) < 2+4+MACSize {
		return nil, fmt.Errorf("%w: cookie response is %d bytes", ErrMalformed, len(b))
	}
	if b[0] != TypeCookieResponse {
		return nil, fmt.Errorf("%w: unexpected type 0x%02x", ErrMalformed, b[0])
	}
	m := &CookieResponse{Version: b[1]}
	m.SenderIndex = binary.LittleEndian.Uint32(b[2:6])
	copy(m.MAC1[:], b[6:2+4+MACSize])
	m.Cookie = append([]byte(nil), b[2+4+MACSize:]...)
	return m, nil
}

// HandshakeInit is the first message of the key exchange.
//
//	+--------+--------+-----------+------------------+--------+---------+---------------+
//	| 0x03   | ver    | senderIdx | eph (32 bytes)   | ckLen  | cookie  | ciphertext    |
//	+--------+--------+-----------+------------------+--------+---------+---------------+
//
// ciphertext = AEAD(k1, counter=0, clientStaticPub || unixTimestamp)
//
// k1 = HKDF(DH(e_i, s_r) || DH(s_i, s_r), salt=transcript, "w1nwire/v1/msg1")
//
// The server is able to find the peer by trying every configured peer key:
// only the real client owns the private key that produces DH(s_i, s_r).
type HandshakeInit struct {
	Version     uint8
	SenderIndex uint32
	Ephemeral   crypto.PublicKey
	Cookie      []byte
	Ciphertext  []byte
}

// Encode serialises the message.
func (m *HandshakeInit) Encode() []byte {
	buf := make([]byte, 2+4+KeySize+2+len(m.Cookie)+len(m.Ciphertext))
	buf[0] = TypeHandshakeInit
	buf[1] = m.Version
	binary.LittleEndian.PutUint32(buf[2:6], m.SenderIndex)
	copy(buf[6:6+KeySize], m.Ephemeral[:])
	binary.LittleEndian.PutUint16(buf[6+KeySize:6+KeySize+2], uint16(len(m.Cookie)))
	copy(buf[6+KeySize+2:], m.Cookie)
	copy(buf[6+KeySize+2+len(m.Cookie):], m.Ciphertext)
	return buf
}

// DecodeHandshakeInit parses a datagram.
func DecodeHandshakeInit(b []byte) (*HandshakeInit, error) {
	const header = 2 + 4 + KeySize + 2
	if len(b) < header {
		return nil, fmt.Errorf("%w: handshake init is %d bytes", ErrMalformed, len(b))
	}
	if b[0] != TypeHandshakeInit {
		return nil, fmt.Errorf("%w: unexpected type 0x%02x", ErrMalformed, b[0])
	}
	m := &HandshakeInit{Version: b[1]}
	m.SenderIndex = binary.LittleEndian.Uint32(b[2:6])
	copy(m.Ephemeral[:], b[6:6+KeySize])
	cookieLen := int(binary.LittleEndian.Uint16(b[6+KeySize : 6+KeySize+2]))
	if len(b) < header+cookieLen {
		return nil, fmt.Errorf("%w: truncated cookie", ErrMalformed)
	}
	m.Cookie = append([]byte(nil), b[header:header+cookieLen]...)
	m.Ciphertext = append([]byte(nil), b[header+cookieLen:]...)
	return m, nil
}

// HandshakeResp is the answer of the server.
//
//	+--------+--------+-----------+------------------+---------------+
//	| 0x04   | ver    | senderIdx | eph (32 bytes)   | ciphertext    |
//	+--------+--------+-----------+------------------+---------------+
//
// ciphertext = AEAD(k2, counter=0, unixTimestamp || assignedIPv4 || flags)
//
// k2 = HKDF(DH(e_i, e_r) || DH(e_r, s_i), salt=transcriptAfterInit, "w1nwire/v1/msg2")
type HandshakeResp struct {
	Version     uint8
	SenderIndex uint32
	Ephemeral   crypto.PublicKey
	Ciphertext  []byte
}

// Encode serialises the message.
func (m *HandshakeResp) Encode() []byte {
	buf := make([]byte, 2+4+KeySize+len(m.Ciphertext))
	buf[0] = TypeHandshakeResp
	buf[1] = m.Version
	binary.LittleEndian.PutUint32(buf[2:6], m.SenderIndex)
	copy(buf[6:6+KeySize], m.Ephemeral[:])
	copy(buf[6+KeySize:], m.Ciphertext)
	return buf
}

// DecodeHandshakeResp parses a datagram.
func DecodeHandshakeResp(b []byte) (*HandshakeResp, error) {
	if len(b) < 2+4+KeySize {
		return nil, fmt.Errorf("%w: handshake response is %d bytes", ErrMalformed, len(b))
	}
	if b[0] != TypeHandshakeResp {
		return nil, fmt.Errorf("%w: unexpected type 0x%02x", ErrMalformed, b[0])
	}
	m := &HandshakeResp{Version: b[1]}
	m.SenderIndex = binary.LittleEndian.Uint32(b[2:6])
	copy(m.Ephemeral[:], b[6:6+KeySize])
	m.Ciphertext = append([]byte(nil), b[6+KeySize:]...)
	return m, nil
}

// DataPacket carries an encrypted IP packet.
//
//	+--------+-----------+-----------+-----------+---------------+-------------------+
//	| 0x05   | recvIdx   | sendIdx   | counter (8)   | ciphertext         |
//	+--------+-----------+-----------+-----------+---------------+-------------------+
//
// The counter is the AEAD nonce counter and is protected by the replay
// window of the receiving session. Both session indexes are carried in the
// header, so a receiver can demultiplex without keeping state per peer.
type DataPacket struct {
	ReceiverIndex uint32
	SenderIndex   uint32
	Counter       uint64
	Ciphertext    []byte
}

// Encode serialises the message.
func (m *DataPacket) Encode() []byte {
	buf := make([]byte, 1+4+4+8+len(m.Ciphertext))
	buf[0] = TypeData
	binary.LittleEndian.PutUint32(buf[1:5], m.ReceiverIndex)
	binary.LittleEndian.PutUint32(buf[5:9], m.SenderIndex)
	binary.LittleEndian.PutUint64(buf[9:17], m.Counter)
	copy(buf[17:], m.Ciphertext)
	return buf
}

// DecodeData parses a datagram.
func DecodeData(b []byte) (*DataPacket, error) {
	if len(b) < 1+4+4+8 {
		return nil, fmt.Errorf("%w: data packet is %d bytes", ErrMalformed, len(b))
	}
	if b[0] != TypeData {
		return nil, fmt.Errorf("%w: unexpected type 0x%02x", ErrMalformed, b[0])
	}
	m := &DataPacket{}
	m.ReceiverIndex = binary.LittleEndian.Uint32(b[1:5])
	m.SenderIndex = binary.LittleEndian.Uint32(b[5:9])
	m.Counter = binary.LittleEndian.Uint64(b[9:17])
	m.Ciphertext = append([]byte(nil), b[17:]...)
	return m, nil
}

// Keepalive is an encrypted empty payload. It keeps the NAT mapping alive and
// lets both sides measure the round trip time.
//
//	+--------+-----------+-----------+-----------+---------------+
//	| 0x06   | recvIdx   | sendIdx   | counter (8)   | tag (16 bytes) |
//	+--------+-----------+-----------+-----------+---------------+
type Keepalive struct {
	ReceiverIndex uint32
	SenderIndex   uint32
	Counter       uint64
	Tag           []byte
}

// Encode serialises the message.
func (m *Keepalive) Encode() []byte {
	buf := make([]byte, 1+4+4+8+len(m.Tag))
	buf[0] = TypeKeepalive
	binary.LittleEndian.PutUint32(buf[1:5], m.ReceiverIndex)
	binary.LittleEndian.PutUint32(buf[5:9], m.SenderIndex)
	binary.LittleEndian.PutUint64(buf[9:17], m.Counter)
	copy(buf[17:], m.Tag)
	return buf
}

// DecodeKeepalive parses a datagram.
func DecodeKeepalive(b []byte) (*Keepalive, error) {
	if len(b) < 1+4+4+8 {
		return nil, fmt.Errorf("%w: keepalive is %d bytes", ErrMalformed, len(b))
	}
	if b[0] != TypeKeepalive {
		return nil, fmt.Errorf("%w: unexpected type 0x%02x", ErrMalformed, b[0])
	}
	m := &Keepalive{}
	m.ReceiverIndex = binary.LittleEndian.Uint32(b[1:5])
	m.SenderIndex = binary.LittleEndian.Uint32(b[5:9])
	m.Counter = binary.LittleEndian.Uint64(b[9:17])
	m.Tag = append([]byte(nil), b[17:]...)
	return m, nil
}

// TypeOf returns the message type of a datagram.
func TypeOf(b []byte) (byte, error) {
	if len(b) == 0 {
		return 0, fmt.Errorf("%w: empty datagram", ErrMalformed)
	}
	return b[0], nil
}
