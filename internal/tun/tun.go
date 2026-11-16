// Package tun provides access to a layer 3 tunnel device.
//
// On Linux the real TUN device is used, which requires the CAP_NET_ADMIN
// capability or root rights. On every other platform the package exposes a
// loopback device that is used by the tests, so the rest of the code stays
// portable.
package tun

import (
	"errors"
	"io"
)

// ErrClosed is returned when the device is used after it was closed.
var ErrClosed = errors.New("tun: device is closed")

// Device is a packet oriented tunnel device.
type Device interface {
	// Read reads one packet into p. It returns io.EOF when the device is
	// closed.
	Read(p []byte) (int, error)
	// Write writes one packet.
	Write(p []byte) (int, error)
	// Close closes the device.
	Close() error
	// Name returns the name of the device.
	Name() string
	// MTU returns the maximum transmission unit.
	MTU() int
}

// PacketDevice is implemented by devices that deliver whole packets.
type PacketDevice interface {
	Device
	// ReadPacket returns the next packet.
	ReadPacket() ([]byte, error)
}

// DNSConfigurer is implemented by devices whose resolver can be set. The
// tunnel mode needs it: with the split default routes in place, a query to the
// resolver of the physical network would be swallowed by the tunnel and the
// machine would stop resolving names.
type DNSConfigurer interface {
	SetDNS(servers []string) error
}

// blockIPv6 controls the best effort leak protection of the tunnel mode: while
// it is on, the two half routes that cover the IPv6 space point at the tunnel
// interface, so applications fall back to IPv4 instead of bypassing the VPN.
var blockIPv6 = true

// SetBlockIPv6 enables or disables the IPv6 leak protection. Platforms that do
// not implement it ignore the value.
func SetBlockIPv6(enabled bool) { blockIPv6 = enabled }

// BlockIPv6 reports whether the IPv6 leak protection is on.
func BlockIPv6() bool { return blockIPv6 }

// Loopback is an in memory device that is used on platforms without a
// tunnel driver and in the tests.
type Loopback struct {
	in     chan []byte
	out    chan []byte
	closed chan struct{}
	name   string
	mtu    int
	once   bool
}

// NewLoopback creates a loopback device. Packets written to it can be read
// from the returned reader and the other way around.
func NewLoopback(name string, mtu int) *Loopback {
	return &Loopback{
		in:     make(chan []byte, 64),
		out:    make(chan []byte, 64),
		closed: make(chan struct{}),
		name:   name,
		mtu:    mtu,
	}
}

// Read implements Device.
func (l *Loopback) Read(p []byte) (int, error) {
	select {
	case <-l.closed:
		return 0, io.EOF
	case pkt := <-l.in:
		n := copy(p, pkt)
		return n, nil
	}
}

// Write implements Device.
func (l *Loopback) Write(p []byte) (int, error) {
	select {
	case <-l.closed:
		return 0, ErrClosed
	default:
	}
	buf := make([]byte, len(p))
	copy(buf, p)
	select {
	case l.out <- buf:
		return len(p), nil
	case <-l.closed:
		return 0, ErrClosed
	}
}

// ReadPacket returns the next written packet.
func (l *Loopback) ReadPacket() ([]byte, error) {
	select {
	case <-l.closed:
		return nil, io.EOF
	case pkt := <-l.out:
		return pkt, nil
	}
}

// Close implements Device.
func (l *Loopback) Close() error {
	select {
	case <-l.closed:
		return nil
	default:
		close(l.closed)
	}
	return nil
}

// Name implements Device.
func (l *Loopback) Name() string { return l.name }

// MTU implements Device.
func (l *Loopback) MTU() int { return l.mtu }
