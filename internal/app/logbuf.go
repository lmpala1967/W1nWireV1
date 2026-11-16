package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// LogEntry is one line of the application log, as shown in the interface.
type LogEntry struct {
	// T is the time of the entry, "15:04:05".
	T string `json:"t"`
	// Level is "debug", "info", "warn" or "error".
	Level string `json:"level"`
	// Msg is the formatted message.
	Msg string `json:"msg"`
}

// LogBuffer keeps the last entries in memory, forwards new ones to the
// subscribers of the interface and appends them to a file.
type LogBuffer struct {
	mu      sync.Mutex
	entries []LogEntry
	max     int
	subs    map[chan LogEntry]struct{}
	file    io.WriteCloser
	closed  bool
}

// NewLogBuffer creates a buffer of the given size. The file may be nil.
func NewLogBuffer(max int, file io.WriteCloser) *LogBuffer {
	if max <= 0 {
		max = 500
	}
	return &LogBuffer{
		entries: make([]LogEntry, 0, max),
		max:     max,
		subs:    make(map[chan LogEntry]struct{}),
		file:    file,
	}
}

// Add appends an entry.
func (b *LogBuffer) Add(level, msg string) {
	entry := LogEntry{T: time.Now().Format("15:04:05"), Level: level, Msg: msg}

	b.mu.Lock()
	if len(b.entries) == b.max {
		copy(b.entries, b.entries[1:])
		b.entries[len(b.entries)-1] = entry
	} else {
		b.entries = append(b.entries, entry)
	}
	for ch := range b.subs {
		select {
		case ch <- entry:
		default:
		}
	}
	file := b.file
	b.mu.Unlock()

	if file != nil {
		fmt.Fprintf(file, "%s %-5s %s\n", time.Now().Format("2006-01-02 15:04:05"), strings.ToUpper(level), msg)
	}
}

// Entries returns a copy of the buffered entries.
func (b *LogBuffer) Entries() []LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]LogEntry, len(b.entries))
	copy(out, b.entries)
	return out
}

// Clear removes the buffered entries.
func (b *LogBuffer) Clear() {
	b.mu.Lock()
	b.entries = b.entries[:0]
	b.mu.Unlock()
}

// Subscribe returns a channel of new entries and a function that cancels the
// subscription.
func (b *LogBuffer) Subscribe() (<-chan LogEntry, func()) {
	ch := make(chan LogEntry, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
	return ch, cancel
}

// Close closes the log file.
func (b *LogBuffer) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	for ch := range b.subs {
		close(ch)
	}
	b.subs = map[chan LogEntry]struct{}{}
	if b.file != nil {
		return b.file.Close()
	}
	return nil
}

// Handler is a slog.Handler that writes to a LogBuffer.
type Handler struct {
	buf   *LogBuffer
	level slog.Level
	attrs []slog.Attr
}

// NewHandler creates a handler that keeps entries in the buffer, at or above
// the given level.
func NewHandler(buf *LogBuffer, level slog.Level) *Handler {
	return &Handler{buf: buf, level: level}
}

// Enabled implements slog.Handler.
func (h *Handler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

// Handle implements slog.Handler.
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	var sb strings.Builder
	sb.WriteString(r.Message)
	for _, a := range h.attrs {
		sb.WriteString(" ")
		sb.WriteString(a.Key)
		sb.WriteString("=")
		sb.WriteString(formatValue(a.Value))
	}
	r.Attrs(func(a slog.Attr) bool {
		sb.WriteString(" ")
		sb.WriteString(a.Key)
		sb.WriteString("=")
		sb.WriteString(formatValue(a.Value))
		return true
	})
	h.buf.Add(levelName(r.Level), sb.String())
	return nil
}

// WithAttrs implements slog.Handler.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &next
}

// WithGroup implements slog.Handler.
func (h *Handler) WithGroup(string) slog.Handler { return h }

func formatValue(v slog.Value) string {
	v = v.Resolve()
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		if s == "" || strings.ContainsAny(s, " \t\n\"=") {
			return fmt.Sprintf("%q", s)
		}
		return s
	default:
		return fmt.Sprint(v.Any())
	}
}

func levelName(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "error"
	case l >= slog.LevelWarn:
		return "warn"
	case l >= slog.LevelInfo:
		return "info"
	default:
		return "debug"
	}
}

// OpenLogFile opens the log file of the application, creating the directory
// when needed.
func OpenLogFile(path string) (io.WriteCloser, error) {
	if err := os.MkdirAll(dirOf(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &rotatingFile{f: f, path: path, max: 4 << 20}, nil
}

// rotatingFile truncates the log when it grows beyond max bytes. A VPN log is
// only useful for the last few days, and an unbounded file in %APPDATA% is a
// bad neighbour.
type rotatingFile struct {
	mu   sync.Mutex
	f    *os.File
	path string
	size int64
	max  int64
}

func (r *rotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size == 0 {
		if info, err := r.f.Stat(); err == nil {
			r.size = info.Size()
		}
	}
	if r.size+int64(len(p)) > r.max {
		if err := r.f.Truncate(0); err == nil {
			if _, err := r.f.Seek(0, io.SeekStart); err == nil {
				r.size = 0
			}
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *rotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.f.Close()
}

func dirOf(path string) string {
	if i := strings.LastIndexAny(path, `\/`); i > 0 {
		return path[:i]
	}
	return "."
}
