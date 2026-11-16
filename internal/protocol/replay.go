package protocol

import "errors"

// ErrReplay is returned when a packet is a duplicate or too old.
var ErrReplay = errors.New("protocol: replay detected")

// ReplayWindow implements the classic sliding window replay protection.
//
// The window remembers the highest counter it has seen and a bitmap of the
// 64 counters below it. Bit i of the bitmap belongs to the counter top-i.
// A packet is accepted when it is newer than the window or when its bit is
// not set yet. Everything that already fell out of the window is rejected.
type ReplayWindow struct {
	initialized bool
	top         uint64
	bitmap      uint64
}

// ReplayWindowSize is the number of packets the window remembers.
const ReplayWindowSize = 64

// Accept validates the counter and updates the window.
//
// The returned error is ErrReplay for both duplicates and packets that fell
// out of the window.
func (w *ReplayWindow) Accept(counter uint64) error {
	if !w.initialized {
		w.initialized = true
		w.top = counter
		w.bitmap = 1
		return nil
	}

	if counter > w.top {
		diff := counter - w.top
		if diff >= ReplayWindowSize {
			// The window is reset, only the new counter is remembered.
			w.bitmap = 1
		} else {
			w.bitmap = (w.bitmap << diff) | 1
		}
		w.top = counter
		return nil
	}

	index := w.top - counter
	if index >= ReplayWindowSize {
		return ErrReplay
	}

	mask := uint64(1) << index
	if w.bitmap&mask != 0 {
		return ErrReplay
	}
	w.bitmap |= mask
	return nil
}

// Top returns the highest counter accepted so far.
func (w *ReplayWindow) Top() uint64 { return w.top }
