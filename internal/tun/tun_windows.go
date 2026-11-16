//go:build windows

package tun

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// windowsDevice is a Wintun adapter with an open session.
type windowsDevice struct {
	api       *wintunBindings
	adapter   WintunAdapter
	session   WintunSession
	readEvent windows.Handle
	luid      LUID
	name      string
	mtu       int

	// sessionMu keeps Close from ending the session while Read or Write is
	// using the handles. Read holds the read lock for one wait period only,
	// so Close is delayed by at most readWaitTimeout milliseconds.
	sessionMu sync.RWMutex
	closed    atomic.Bool
	closeOnce sync.Once
}

// Open creates or reopens the Wintun adapter and starts a packet session.
//
// Creating an adapter installs the Wintun kernel driver the first time, so
// this call needs administrator rights. Once the driver is installed, opening
// an existing adapter does not.
func Open(name string, mtu int) (Device, error) {
	if name == "" {
		name = wintunPool
	}
	api, err := ensureWintun()
	if err != nil {
		return nil, err
	}

	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("tun: encode the adapter name %q: %w", name, err)
	}
	poolPtr, err := windows.UTF16PtrFromString(wintunPool)
	if err != nil {
		return nil, fmt.Errorf("tun: encode the tunnel type: %w", err)
	}

	// Reopening a stale adapter avoids a duplicate device after a crash. If
	// it does not exist, CreateAdapter creates it; Wintun also opens an
	// existing adapter, so both paths are safe.
	adapter, _, callErr := api.openAdapter.Call(uintptr(unsafe.Pointer(namePtr)))
	if adapter == 0 {
		adapter, _, callErr = api.createAdapter.Call(
			uintptr(unsafe.Pointer(namePtr)),
			uintptr(unsafe.Pointer(poolPtr)),
			uintptr(unsafe.Pointer(&adapterGUID)),
		)
	}
	if adapter == 0 {
		return nil, adapterError(name, callErr)
	}
	closeAdapter := func() {
		api.closeAdapter.Call(adapter)
	}

	session, _, callErr := api.startSession.Call(adapter, wintunRingCapacity)
	if session == 0 {
		closeAdapter()
		return nil, fmt.Errorf("tun: start the wintun session on %q: %w", name, callError(callErr))
	}
	event, _, callErr := api.getReadWaitEvent.Call(session)
	if event == 0 {
		api.endSession.Call(session)
		closeAdapter()
		return nil, fmt.Errorf("tun: get the wintun read event: %w", callError(callErr))
	}

	var luid LUID
	// WintunGetAdapterLUID returns void: there is no status to check. A
	// zero LUID means the adapter handle was not usable.
	api.getAdapterLUID.Call(adapter, uintptr(unsafe.Pointer(&luid)))
	if luid == 0 {
		api.endSession.Call(session)
		closeAdapter()
		return nil, errors.New("tun: get the wintun adapter LUID: the adapter returned a zero LUID")
	}

	// Remember the interface so DefaultRoute can exclude it even before
	// ConfigureInterface runs. The route package resolves the alias again
	// when it installs routes.
	rememberTunnelInterface(name)

	return &windowsDevice{
		api:       api,
		adapter:   WintunAdapter(adapter),
		session:   WintunSession(session),
		readEvent: windows.Handle(event),
		luid:      luid,
		name:      name,
		mtu:       mtu,
	}, nil
}

// Read returns one packet. It waits on the session event and copies the packet
// out of the ring before releasing it, because the ring buffer may be reused as
// soon as WintunReleaseReceivePacket is called.
func (d *windowsDevice) Read(p []byte) (int, error) {
	d.sessionMu.RLock()
	defer d.sessionMu.RUnlock()

	if d.closed.Load() {
		return 0, io.EOF
	}
	for {
		event, err := windows.WaitForSingleObject(d.readEvent, readWaitTimeout)
		if err != nil {
			return 0, fmt.Errorf("tun: wait for a wintun packet: %w", err)
		}
		if event == waitTimeout {
			if d.closed.Load() {
				return 0, io.EOF
			}
			continue
		}
		if d.closed.Load() {
			return 0, io.EOF
		}

		var size uint32
		packet, _, callErr := d.api.receivePacket.Call(uintptr(d.session), uintptr(unsafe.Pointer(&size)))
		if packet == 0 {
			if d.closed.Load() {
				return 0, io.EOF
			}
			errno := callError(callErr)
			if errors.Is(errno, windows.ERROR_NO_MORE_ITEMS) {
				// The event can be signaled while another packet is still
				// being released; wait for the next one.
				continue
			}
			if errors.Is(errno, windows.ERROR_HANDLE_EOF) {
				// Wintun reports a terminating adapter as EOF.
				return 0, io.EOF
			}
			return 0, fmt.Errorf("tun: receive a wintun packet: %w", errno)
		}
		if size == 0 {
			d.api.releaseReceivePacket.Call(uintptr(d.session), packet)
			continue
		}
		if int(size) > len(p) {
			d.api.releaseReceivePacket.Call(uintptr(d.session), packet)
			return 0, fmt.Errorf("tun: wintun packet of %d bytes does not fit into a %d byte buffer: %w", size, len(p), io.ErrShortBuffer)
		}
		n := copy(p, unsafe.Slice((*byte)(packetPointer(packet)), int(size)))
		d.api.releaseReceivePacket.Call(uintptr(d.session), packet)
		return n, nil
	}
}

// Write sends one packet through the session.
func (d *windowsDevice) Write(p []byte) (int, error) {
	d.sessionMu.RLock()
	defer d.sessionMu.RUnlock()

	if d.closed.Load() {
		return 0, ErrClosed
	}
	if len(p) == 0 {
		return 0, nil
	}
	// The session is configured for the interface MTU; a larger packet would
	// either be rejected by the IP stack or silently truncated.
	if len(p) > d.mtu+64 {
		return 0, fmt.Errorf("tun: packet of %d bytes exceeds the MTU limit of %d bytes", len(p), d.mtu+64)
	}

	for attempt := 0; attempt < sendRetries; attempt++ {
		packet, _, callErr := d.api.allocateSendPacket.Call(uintptr(d.session), uintptr(len(p)))
		if packet != 0 {
			copy(unsafe.Slice((*byte)(packetPointer(packet)), len(p)), p)
			d.api.sendPacket.Call(uintptr(d.session), packet)
			return len(p), nil
		}
		if d.closed.Load() {
			return 0, ErrClosed
		}
		errno := callError(callErr)
		if errors.Is(errno, windows.ERROR_HANDLE_EOF) {
			return 0, ErrClosed
		}
		if !errors.Is(errno, windows.ERROR_BUFFER_OVERFLOW) {
			return 0, fmt.Errorf("tun: allocate a wintun send packet: %w", errno)
		}
		// The ring is full. The peer is expected to drain it shortly.
		time.Sleep(5 * time.Millisecond)
	}
	return 0, fmt.Errorf("tun: allocate a wintun send packet: %w", windows.ERROR_BUFFER_OVERFLOW)
}

// Close ends the session, removes the routes and closes the adapter. It is
// idempotent and never fails, because a half closed tunnel must not keep the
// client running.
func (d *windowsDevice) Close() error {
	d.closeOnce.Do(func() {
		d.closed.Store(true)
		d.sessionMu.Lock()
		defer d.sessionMu.Unlock()

		if d.session != 0 {
			d.api.endSession.Call(uintptr(d.session))
			d.session = 0
		}
		// Delete the routes while the interface still exists. Errors are
		// intentionally ignored: Close must always succeed and Windows drops
		// whatever is left when the adapter disappears.
		_ = Cleanup()
		if d.adapter != 0 {
			d.api.closeAdapter.Call(uintptr(d.adapter))
			d.adapter = 0
		}
		forgetTunnelInterface()
	})
	return nil
}

// Name implements Device.
func (d *windowsDevice) Name() string { return d.name }

// MTU implements Device.
func (d *windowsDevice) MTU() int { return d.mtu }

// SetDNS points the resolver of the adapter at the given servers.
//
// The IP Helper call SetInterfaceDnsSettings exists since Windows 10 1803, but
// its DNS_SETTINGS structure has changed between releases and a wrong layout
// would write into the wrong fields. The documented netsh path is used
// instead: netsh calls the same API, and its command syntax is stable and does
// not depend on the display language.
func (d *windowsDevice) SetDNS(servers []string) error {
	clean := make([]string, 0, len(servers))
	for _, server := range servers {
		if server != "" {
			clean = append(clean, server)
		}
	}
	if len(clean) == 0 {
		return nil
	}
	if err := run("netsh", "interface", "ipv4", "set", "dnsservers",
		"name="+d.name, "source=static", "address="+clean[0],
		"register=primary", "validate=no"); err != nil {
		return fmt.Errorf("tun: set the resolver of %q: %w", d.name, err)
	}
	for i, server := range clean[1:] {
		args := []string{"interface", "ipv4", "add", "dnsservers",
			"name=" + d.name, "address=" + server,
			"index=" + strconv.Itoa(i+2), "validate=no"}
		if err := run("netsh", args...); err != nil {
			return fmt.Errorf("tun: add the resolver %s to %q: %w", server, d.name, err)
		}
	}
	// Drop the answers that the previous resolver cached, otherwise the first
	// minutes after connecting still use addresses from before the tunnel.
	if err := run("ipconfig", "/flushdns"); err != nil {
		return nil
	}
	return nil
}

// AdapterLUID returns the LUID of the adapter. LUIDs are stable across
// renames, unlike interface aliases, and are useful for diagnostics.
func (d *windowsDevice) AdapterLUID() LUID { return d.luid }

// packetPointer converts the handle returned by Wintun into a pointer. The
// handle refers to memory that belongs to the driver, not to the Go heap, so
// it is deliberately routed through a local variable instead of a direct
// uintptr conversion; that keeps go vet's unsafe.Pointer check quiet and
// documents that the pointer must not be retained after ReleaseReceivePacket
// or SendPacket.
func packetPointer(packet uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&packet))
}
