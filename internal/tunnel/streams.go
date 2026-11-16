package tunnel

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"
)

// copyBufferSize is the size of the buffer used when relaying data.
const copyBufferSize = 32 * 1024

// dialTimeout limits how long the server waits for a new connection.
const dialTimeout = 15 * time.Second

// ServerStreams implements the server side of the proxy mode.
//
// The server opens the real connection and relays the bytes through the
// tunnel. The traffic of the client therefore leaves the server with the
// public address of the server.
type ServerStreams struct {
	mu      sync.Mutex
	streams map[uint32]*ServerStream
	nextID  uint32
	send    func([]byte) error
	logger  *slog.Logger
}

// ServerStream is an open connection of the proxy mode.
type ServerStream struct {
	ID   uint32
	conn net.Conn
	once sync.Once
}

// NewServerStreams creates the manager.
func NewServerStreams(send func([]byte) error, logger *slog.Logger) *ServerStreams {
	if logger == nil {
		logger = slog.Default()
	}
	return &ServerStreams{
		streams: make(map[uint32]*ServerStream),
		send:    send,
		logger:  logger,
	}
}

// Handle processes an incoming payload.
func (s *ServerStreams) Handle(payload []byte) error {
	frameType, err := Type(payload)
	if err != nil {
		return err
	}
	switch frameType {
	case FrameOpen:
		return s.handleOpen(payload)
	case FrameData:
		return s.handleData(payload)
	case FrameClose:
		return s.handleClose(payload)
	default:
		return fmt.Errorf("%w: frame 0x%02x is not valid on the server", ErrMalformed, frameType)
	}
}

func (s *ServerStreams) handleOpen(payload []byte) error {
	open, err := DecodeOpen(payload)
	if err != nil {
		return err
	}
	if open.Protocol != ProtoTCP {
		s.send(EncodeOpenAck(open.StreamID, StatusRefused))
		return fmt.Errorf("tunnel: protocol 0x%02x is not supported", open.Protocol)
	}

	conn, err := net.DialTimeout("tcp", open.Address, dialTimeout)
	if err != nil {
		s.logger.Warn("stream open failed", "address", open.Address, "error", err)
		s.send(EncodeOpenAck(open.StreamID, StatusError))
		return nil
	}

	stream := &ServerStream{ID: open.StreamID, conn: conn}
	s.mu.Lock()
	s.streams[open.StreamID] = stream
	s.mu.Unlock()

	if err := s.send(EncodeOpenAck(open.StreamID, StatusOK)); err != nil {
		conn.Close()
		return err
	}
	go s.pump(stream)
	return nil
}

func (s *ServerStreams) handleData(payload []byte) error {
	id, data, err := DecodeData(payload)
	if err != nil {
		return err
	}
	s.logger.Debug("stream data received", "stream", id, "bytes", len(data))
	s.mu.Lock()
	stream := s.streams[id]
	s.mu.Unlock()
	if stream == nil {
		return nil
	}
	if _, err := stream.conn.Write(data); err != nil {
		s.closeStream(id)
		return nil
	}
	return nil
}

func (s *ServerStreams) handleClose(payload []byte) error {
	id, _, err := DecodeClose(payload)
	if err != nil {
		return err
	}
	s.closeStream(id)
	return nil
}

// pump copies the data of the connection into the tunnel.
func (s *ServerStreams) pump(stream *ServerStream) {
	buf := make([]byte, copyBufferSize)
	for {
		n, err := stream.conn.Read(buf)
		if n > 0 {
			if sendErr := s.send(EncodeData(stream.ID, buf[:n])); sendErr != nil {
				break
			}
		}
		if err != nil {
			if err != io.EOF {
				s.logger.Debug("stream read failed", "stream", stream.ID, "error", err)
			}
			break
		}
	}
	s.send(EncodeClose(stream.ID, StatusOK))
	s.closeStream(stream.ID)
}

func (s *ServerStreams) closeStream(id uint32) {
	s.mu.Lock()
	stream := s.streams[id]
	delete(s.streams, id)
	s.mu.Unlock()
	if stream != nil {
		stream.once.Do(func() { stream.conn.Close() })
	}
}

// Close tears down every open stream.
func (s *ServerStreams) Close() {
	s.mu.Lock()
	streams := s.streams
	s.streams = make(map[uint32]*ServerStream)
	s.mu.Unlock()
	for _, stream := range streams {
		stream.once.Do(func() { stream.conn.Close() })
	}
}

// Count returns the number of open streams.
func (s *ServerStreams) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.streams)
}

// ClientStreams implements the client side of the proxy mode.
type ClientStreams struct {
	mu      sync.Mutex
	streams map[uint32]*ClientStream
	nextID  uint32
	send    func([]byte) error
	logger  *slog.Logger
}

// ClientStream is a connection of the local application towards the server.
type ClientStream struct {
	ID   uint32
	conn net.Conn
	once sync.Once

	ready chan error
}

// NewClientStreams creates the manager.
func NewClientStreams(send func([]byte) error, logger *slog.Logger) *ClientStreams {
	if logger == nil {
		logger = slog.Default()
	}
	return &ClientStreams{
		streams: make(map[uint32]*ClientStream),
		send:    send,
		logger:  logger,
	}
}

// Open asks the server to open a connection to the given address.
//
// The address is sent as a name, the server resolves it. This keeps the DNS
// queries of the client inside the tunnel.
func (c *ClientStreams) Open(conn net.Conn, protocol byte, address string) (*ClientStream, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	stream := &ClientStream{ID: id, conn: conn, ready: make(chan error, 1)}
	c.streams[id] = stream
	c.mu.Unlock()

	if err := c.send(EncodeOpen(Open{StreamID: id, Protocol: protocol, Address: address})); err != nil {
		c.remove(id)
		return nil, err
	}

	select {
	case err := <-stream.ready:
		if err != nil {
			c.remove(id)
			return nil, err
		}
	case <-time.After(dialTimeout):
		c.remove(id)
		return nil, fmt.Errorf("tunnel: the server did not answer the stream request")
	}

	go c.pump(stream)
	return stream, nil
}

// Handle processes an incoming payload.
func (c *ClientStreams) Handle(payload []byte) error {
	frameType, err := Type(payload)
	if err != nil {
		return err
	}
	switch frameType {
	case FrameOpenAck:
		id, status, err := DecodeOpenAck(payload)
		if err != nil {
			return err
		}
		c.mu.Lock()
		stream := c.streams[id]
		c.mu.Unlock()
		if stream == nil {
			return nil
		}
		if status != StatusOK {
			stream.once.Do(func() {
				stream.ready <- fmt.Errorf("tunnel: the server refused the connection")
			})
			return nil
		}
		stream.once.Do(func() { stream.ready <- nil })
		return nil
	case FrameData:
		id, data, err := DecodeData(payload)
		if err != nil {
			return err
		}
		c.mu.Lock()
		stream := c.streams[id]
		c.mu.Unlock()
		if stream == nil {
			return nil
		}
		if _, err := stream.conn.Write(data); err != nil {
			c.remove(id)
		}
		return nil
	case FrameClose:
		id, _, err := DecodeClose(payload)
		if err != nil {
			return err
		}
		c.remove(id)
		return nil
	default:
		return fmt.Errorf("%w: frame 0x%02x is not valid on the client", ErrMalformed, frameType)
	}
}

// pump copies the data of the local connection into the tunnel.
func (c *ClientStreams) pump(stream *ClientStream) {
	buf := make([]byte, copyBufferSize)
	for {
		n, err := stream.conn.Read(buf)
		if n > 0 {
			c.logger.Debug("stream data to send", "stream", stream.ID, "bytes", n)
			if sendErr := c.send(EncodeData(stream.ID, buf[:n])); sendErr != nil {
				break
			}
		}
		if err != nil {
			break
		}
	}
	c.send(EncodeClose(stream.ID, StatusOK))
	c.remove(stream.ID)
}

func (c *ClientStreams) remove(id uint32) {
	c.mu.Lock()
	stream := c.streams[id]
	delete(c.streams, id)
	c.mu.Unlock()
	if stream != nil {
		stream.conn.Close()
	}
}

// Close tears down every open stream.
func (c *ClientStreams) Close() {
	c.mu.Lock()
	streams := c.streams
	c.streams = make(map[uint32]*ClientStream)
	c.mu.Unlock()
	for _, stream := range streams {
		stream.conn.Close()
	}
}
