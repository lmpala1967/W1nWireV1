// Package socks5 implements the local proxy of the W1nWire client.
//
// The proxy accepts SOCKS5 connections and forwards them through the tunnel.
// It is the mode that works on every operating system, including phones, and
// it does not need a tunnel driver.
package socks5

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"time"
)

// Command codes of SOCKS5.
const (
	cmdConnect      byte = 0x01
	cmdBind         byte = 0x02
	cmdUDPAssociate byte = 0x03
)

// Address types of SOCKS5.
const (
	atypIPv4   byte = 0x01
	atypDomain byte = 0x03
	atypIPv6   byte = 0x04
)

// Reply codes of SOCKS5.
const (
	replySuccess             byte = 0x00
	replyGeneralFailure      byte = 0x01
	replyCommandNotSupported byte = 0x07
)

// handshakeTimeout limits the time a client may spend on the handshake.
const handshakeTimeout = 15 * time.Second

// StreamOpener opens a connection through the tunnel.
//
// The opener takes over the connection: it must keep relaying the data of
// conn until it is closed.
type StreamOpener func(conn net.Conn, address string) error

// Server is a minimal SOCKS5 server.
type Server struct {
	listen string
	open   StreamOpener
	logger *slog.Logger
}

// NewServer creates the proxy.
func NewServer(listen string, open StreamOpener, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{listen: listen, open: open, logger: logger}
}

// Listen binds the address of the proxy. It is separate from Serve so that a
// caller can report a busy port before it announces that the tunnel is up.
func (s *Server) Listen() (net.Listener, error) {
	listener, err := net.Listen("tcp", s.listen)
	if err != nil {
		return nil, fmt.Errorf("socks5: listen on %s: %w", s.listen, err)
	}
	return listener, nil
}

// Serve accepts connections on an already bound listener until the context is
// cancelled.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	if listener == nil {
		return errors.New("socks5: no listener")
	}
	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	s.logger.Info("socks5 proxy is listening", "address", listener.Addr().String())
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.logger.Debug("accept failed", "error", err)
			continue
		}
		go s.handle(conn)
	}
}

// ListenAndServe runs the proxy until the context is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	listener, err := s.Listen()
	if err != nil {
		return err
	}
	return s.Serve(ctx, listener)
}

func (s *Server) handle(conn net.Conn) {
	if err := s.negotiate(conn); err != nil {
		s.logger.Debug("socks5 negotiation failed", "error", err)
		conn.Close()
		return
	}
	address, err := readRequest(conn)
	if err != nil {
		s.logger.Debug("socks5 request failed", "error", err)
		conn.Close()
		return
	}
	conn.SetDeadline(time.Time{})

	if err := s.open(conn, address); err != nil {
		s.logger.Debug("socks5 stream failed", "address", address, "error", err)
		conn.Write([]byte{0x05, replyGeneralFailure, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0})
		conn.Close()
		return
	}
	// The connection is established, the client expects the answer of the
	// CONNECT request before it starts to send data.
	if _, err := conn.Write([]byte{0x05, replySuccess, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0}); err != nil {
		conn.Close()
		return
	}
	// The stream manager owns the connection now, it relays the data until
	// the connection is closed. The handler must not close it.
}

func (s *Server) negotiate(conn net.Conn) error {
	conn.SetDeadline(time.Now().Add(handshakeTimeout))
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}
	if header[0] != 0x05 {
		return fmt.Errorf("socks5: unsupported version %d", header[0])
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}
	supported := false
	for _, m := range methods {
		if m == 0x00 {
			supported = true
		}
	}
	if !supported {
		conn.Write([]byte{0x05, 0xff})
		return errors.New("socks5: no acceptable authentication method")
	}
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return err
	}
	return nil
}

func readRequest(conn net.Conn) (string, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", err
	}
	if header[0] != 0x05 {
		return "", fmt.Errorf("socks5: unsupported version %d", header[0])
	}
	if header[1] != cmdConnect {
		conn.Write([]byte{0x05, replyCommandNotSupported, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0})
		return "", fmt.Errorf("socks5: command %d is not supported", header[1])
	}

	var host string
	switch header[3] {
	case atypIPv4:
		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		host = net.IP(buf).String()
	case atypIPv6:
		buf := make([]byte, 16)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		host = net.IP(buf).String()
	case atypDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return "", err
		}
		buf := make([]byte, int(lenBuf[0]))
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		host = string(buf)
	default:
		return "", fmt.Errorf("socks5: unknown address type %d", header[3])
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(portBuf)
	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}
