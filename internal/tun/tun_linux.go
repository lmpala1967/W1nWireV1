//go:build linux

package tun

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// cloneDevicePath is the character device that creates new TUN devices.
const cloneDevicePath = "/dev/net/tun"

// ifreq mirrors the struct ifreq of the Linux kernel.
type ifreq struct {
	Name  [unix.IFNAMSIZ]byte
	Flags uint16
	_     [22]byte
}

// linuxDevice is a real TUN device.
type linuxDevice struct {
	file *os.File
	name string
	mtu  int
}

// Open creates a TUN device with the given name and MTU.
//
// The caller needs the CAP_NET_ADMIN capability, which in practice means
// running the server as root.
func Open(name string, mtu int) (Device, error) {
	fd, err := openDevice(name)
	if err != nil {
		return nil, err
	}

	deviceName := name
	file := os.NewFile(uintptr(fd), deviceName)
	return &linuxDevice{file: file, name: deviceName, mtu: mtu}, nil
}

// openDevice creates the TUN device and returns the file descriptor.
func openDevice(name string) (int, error) {
	fd, err := create(name)
	if err == nil {
		return fd, nil
	}
	if !errors.Is(err, unix.EBUSY) {
		return -1, err
	}
	// A device with the same name is still registered, most likely because
	// the previous instance of the server has not finished yet. Removing it
	// and trying once more is safe, the interface carries no state.
	if err := run("ip", "link", "del", name); err != nil {
		return -1, fmt.Errorf("tun: the device %s is busy and could not be removed: %w", name, err)
	}
	return create(name)
}

func create(name string) (int, error) {
	fd, err := unix.Open(cloneDevicePath, unix.O_RDWR, 0)
	if err != nil {
		return -1, fmt.Errorf("tun: open %s: %w", cloneDevicePath, err)
	}

	var req ifreq
	copy(req.Name[:], name)
	// IFF_TUN without IFF_NO_PI would prepend a 4 byte packet information
	// header to every packet, which the protocol does not expect.
	req.Flags = unix.IFF_TUN | unix.IFF_NO_PI

	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(unix.TUNSETIFF), uintptr(unsafe.Pointer(&req))); errno != 0 {
		unix.Close(fd)
		return -1, fmt.Errorf("tun: ioctl TUNSETIFF: %w", errno)
	}
	return fd, nil
}

// Read implements Device.
func (d *linuxDevice) Read(p []byte) (int, error) {
	return d.file.Read(p)
}

// Write implements Device.
func (d *linuxDevice) Write(p []byte) (int, error) {
	return d.file.Write(p)
}

// Close implements Device.
func (d *linuxDevice) Close() error { return d.file.Close() }

// Name implements Device.
func (d *linuxDevice) Name() string { return d.name }

// MTU implements Device.
func (d *linuxDevice) MTU() int { return d.mtu }
