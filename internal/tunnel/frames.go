// Package tunnel implements the payload framing that runs inside the
// encrypted data packets of the W1nWire protocol.
//
// Two payload families share the same transport:
//
//   - IP packets, used by the tunnel mode where a TUN device carries the
//     traffic of the operating system;
//   - streams, used by the proxy mode where the server opens the connection
//     and the traffic is relayed through the tunnel.
//
// Because the payloads are encrypted, the two families never interfere.
package tunnel

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Frame types.
const (
	FrameIP      byte = 0x01
	FrameOpen    byte = 0x02
	FrameOpenAck byte = 0x03
	FrameData    byte = 0x04
	FrameClose   byte = 0x05
)

// Protocols of a stream.
const (
	ProtoTCP byte = 0x06
	ProtoUDP byte = 0x07
)

// Status codes of a stream open acknowledgement.
const (
	StatusOK      byte = 0x00
	StatusError   byte = 0x01
	StatusRefused byte = 0x02
)

// ErrMalformed is returned when a frame does not follow the specification.
var ErrMalformed = errors.New("tunnel: malformed frame")

// Open describes a request to open a stream.
type Open struct {
	// StreamID identifies the stream inside the session.
	StreamID uint32
	// Protocol is ProtoTCP or ProtoUDP.
	Protocol byte
	// Address is the destination in host:port notation. The host is sent as
	// a name, the server resolves it, which keeps the DNS queries of the
	// client inside the tunnel.
	Address string
}

// Encode serialises an open frame.
func EncodeOpen(o Open) []byte {
	buf := make([]byte, 1+4+1+2+len(o.Address))
	buf[0] = FrameOpen
	binary.LittleEndian.PutUint32(buf[1:5], o.StreamID)
	buf[5] = o.Protocol
	binary.LittleEndian.PutUint16(buf[6:8], uint16(len(o.Address)))
	copy(buf[8:], o.Address)
	return buf
}

// DecodeOpen parses an open frame.
func DecodeOpen(b []byte) (Open, error) {
	if len(b) < 8 {
		return Open{}, fmt.Errorf("%w: open frame is %d bytes", ErrMalformed, len(b))
	}
	if b[0] != FrameOpen {
		return Open{}, fmt.Errorf("%w: unexpected frame 0x%02x", ErrMalformed, b[0])
	}
	length := int(binary.LittleEndian.Uint16(b[6:8]))
	if len(b) < 8+length {
		return Open{}, fmt.Errorf("%w: truncated address", ErrMalformed)
	}
	return Open{
		StreamID: binary.LittleEndian.Uint32(b[1:5]),
		Protocol: b[5],
		Address:  string(b[8 : 8+length]),
	}, nil
}

// EncodeOpenAck serialises the answer of the server.
func EncodeOpenAck(streamID uint32, status byte) []byte {
	buf := make([]byte, 6)
	buf[0] = FrameOpenAck
	binary.LittleEndian.PutUint32(buf[1:5], streamID)
	buf[5] = status
	return buf
}

// DecodeOpenAck parses an acknowledgement.
func DecodeOpenAck(b []byte) (uint32, byte, error) {
	if len(b) < 6 {
		return 0, 0, fmt.Errorf("%w: ack frame is %d bytes", ErrMalformed, len(b))
	}
	if b[0] != FrameOpenAck {
		return 0, 0, fmt.Errorf("%w: unexpected frame 0x%02x", ErrMalformed, b[0])
	}
	return binary.LittleEndian.Uint32(b[1:5]), b[5], nil
}

// EncodeData serialises a data frame.
func EncodeData(streamID uint32, payload []byte) []byte {
	buf := make([]byte, 5+len(payload))
	buf[0] = FrameData
	binary.LittleEndian.PutUint32(buf[1:5], streamID)
	copy(buf[5:], payload)
	return buf
}

// DecodeData parses a data frame.
func DecodeData(b []byte) (uint32, []byte, error) {
	if len(b) < 5 {
		return 0, nil, fmt.Errorf("%w: data frame is %d bytes", ErrMalformed, len(b))
	}
	if b[0] != FrameData {
		return 0, nil, fmt.Errorf("%w: unexpected frame 0x%02x", ErrMalformed, b[0])
	}
	return binary.LittleEndian.Uint32(b[1:5]), b[5:], nil
}

// EncodeClose serialises a close frame.
func EncodeClose(streamID uint32, reason byte) []byte {
	buf := make([]byte, 6)
	buf[0] = FrameClose
	binary.LittleEndian.PutUint32(buf[1:5], streamID)
	buf[5] = reason
	return buf
}

// DecodeClose parses a close frame.
func DecodeClose(b []byte) (uint32, byte, error) {
	if len(b) < 6 {
		return 0, 0, fmt.Errorf("%w: close frame is %d bytes", ErrMalformed, len(b))
	}
	if b[0] != FrameClose {
		return 0, 0, fmt.Errorf("%w: unexpected frame 0x%02x", ErrMalformed, b[0])
	}
	return binary.LittleEndian.Uint32(b[1:5]), b[5], nil
}

// Type returns the frame type of a payload.
func Type(b []byte) (byte, error) {
	if len(b) == 0 {
		return 0, fmt.Errorf("%w: empty payload", ErrMalformed)
	}
	return b[0], nil
}
