package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/w1nchi/w1nwire/internal/client"
	"github.com/w1nchi/w1nwire/internal/config"
	"github.com/w1nchi/w1nwire/internal/invite"
	"github.com/w1nchi/w1nwire/internal/metrics"
)

// Event is a message pushed to the user interface.
type Event struct {
	Name string
	Data any
}

// WintunView describes the state of the Windows tunnel driver.
type WintunView struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Detail  string `json:"detail"`
}

// StatusView is the connection state as the interface sees it.
type StatusView struct {
	State       string     `json:"state"`
	Connected   bool       `json:"connected"`
	Since       *time.Time `json:"since"`
	UptimeSec   int64      `json:"uptimeSec"`
	Server      string     `json:"server"`
	ProfileName string     `json:"profileName"`
	Mode        string     `json:"mode"`
	AssignedIP  string     `json:"assignedIP"`
	RxBytes     uint64     `json:"rxBytes"`
	TxBytes     uint64     `json:"txBytes"`
	RxRate      float64    `json:"rxRate"`
	TxRate      float64    `json:"txRate"`
	RttMs       *int       `json:"rttMs"`
	Error       string     `json:"error"`
	SelfTest    bool       `json:"selfTest"`
}

// ProfileView is a profile as exposed to the interface. The private key is
// replaced by a flag and by the derived public key.
type ProfileView struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Host            string    `json:"host"`
	PublicKey       string    `json:"publicKey"`
	PrivateKeySet   bool      `json:"privateKeySet"`
	ClientPublicKey string    `json:"clientPublicKey"`
	TunnelAddress   string    `json:"tunnelAddress"`
	Gateway         string    `json:"gateway"`
	MTU             int       `json:"mtu"`
	CreatedAt       time.Time `json:"createdAt"`
	LatencyMs       *int      `json:"latencyMs"`
}

// StateView is the full state returned by the API.
type StateView struct {
	Version         string        `json:"version"`
	OS              string        `json:"os"`
	Elevated        bool          `json:"elevated"`
	DataDir         string        `json:"dataDir"`
	Wintun          WintunView    `json:"wintun"`
	Status          StatusView    `json:"status"`
	Profiles        []ProfileView `json:"profiles"`
	ActiveProfileID string        `json:"activeProfileId"`
	Settings        Settings      `json:"settings"`
	Logs            []LogEntry    `json:"logs"`
	Busy            string        `json:"busy"`
}

// Options configures an App.
type Options struct {
	// Version is the version string of the binary.
	Version string
	// DataDir is where the profiles and the logs are stored.
	DataDir string
	// LogLevel is the minimum level kept in the log.
	LogLevel slog.Level
}

// App is the application layer of the desktop client.
type App struct {
	version  string
	dataDir  string
	store    *Store
	logs     *LogBuffer
	logger   *slog.Logger
	elevated bool

	mu         sync.Mutex
	conn       *connection
	connecting bool
	status     StatusView
	busy       string
	wintun     WintunView
	latencies  map[string]int
	proxyOn    bool
	// proxyBefore holds the proxy settings that were active before the
	// application changed them, so they can be restored exactly.
	proxyBefore *systemProxyState

	subsMu     sync.Mutex
	subs       map[chan Event]struct{}
	subsClosed bool

	closed   bool
	shutdown chan struct{}
}

// connection is one running instance of the tunnel client.
type connection struct {
	client    *client.Client
	cancel    context.CancelFunc
	done      chan struct{}
	profileID string
	mode      string
	started   time.Time
	// failure records why the application stopped the tunnel itself, for
	// example because the handshake never completed.
	failure string
}

const (
	// connectTimeout is how long a fresh connection may stay in the handshake
	// state before the application gives up. In the tunnel mode the routes
	// are already installed at that point, so a stuck handshake means no
	// internet until they are rolled back.
	connectTimeout = 25 * time.Second
	// staleTimeout is how long a connected session may go without a
	// successful handshake (they happen every two minutes) before the tunnel
	// is considered dead.
	staleTimeout = 6 * time.Minute
)

// ErrBusy is returned when an operation is already in progress.
var ErrBusy = errors.New("app: another operation is in progress")

// ErrNotConnected is returned by Disconnect when the tunnel is down.
var ErrNotConnected = errors.New("app: the tunnel is not connected")

// ErrElevation is returned when the tunnel mode is requested without the
// administrator rights the Wintun driver needs.
var ErrElevation = errors.New("app: the tunnel mode needs administrator rights")

// New creates the application, loads the store and prepares the log.
func New(opts Options) (*App, error) {
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.DataDir == "" {
		dir, err := DefaultDataDir()
		if err != nil {
			return nil, err
		}
		opts.DataDir = dir
	}
	store, err := OpenStore(opts.DataDir)
	if err != nil {
		return nil, err
	}
	logFile, err := OpenLogFile(filepath.Join(opts.DataDir, "logs", "w1nwire.log"))
	if err != nil {
		logFile = nil
	}
	buf := NewLogBuffer(500, logFile)
	logger := slog.New(NewHandler(buf, opts.LogLevel))

	a := &App{
		version:   opts.Version,
		dataDir:   opts.DataDir,
		store:     store,
		logs:      buf,
		logger:    logger,
		elevated:  isElevated(),
		latencies: make(map[string]int),
		subs:      make(map[chan Event]struct{}),
		shutdown:  make(chan struct{}),
	}
	a.status = StatusView{State: "disconnected", Mode: store.Settings().Mode}
	a.wintun = wintunInfo()
	logger.Info("W1nWire started", "version", opts.Version, "os", runtime.GOOS, "elevated", a.elevated, "data", opts.DataDir)
	if !a.elevated && runtime.GOOS == "windows" {
		logger.Info("running without administrator rights: the tunnel mode will ask for an elevation")
	}
	if a.elevated {
		// A previous run may have been killed while the tunnel was up. The
		// half-default routes it installed would swallow all traffic until
		// they are removed, so they are removed before anything else.
		if err := cleanupStaleTunnel(); err != nil {
			logger.Warn("the leftover tunnel routes could not be cleaned up", "error", err)
		}
	}
	return a, nil
}

// DefaultDataDir returns the directory where the application keeps its files.
func DefaultDataDir() (string, error) {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "W1nWire"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("app: locate the data directory: %w", err)
	}
	return filepath.Join(home, ".w1nwire"), nil
}

// Logger returns the logger of the application.
func (a *App) Logger() *slog.Logger { return a.logger }

// Version returns the version of the binary.
func (a *App) Version() string { return a.version }

// DataDir returns the directory where the application keeps its files.
func (a *App) DataDir() string { return a.dataDir }

// Logs returns the log buffer.
func (a *App) Logs() *LogBuffer { return a.logs }

// Subscribe registers a listener for the events pushed to the interface.
func (a *App) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	a.subsMu.Lock()
	a.subs[ch] = struct{}{}
	a.subsMu.Unlock()
	return ch, func() {
		a.subsMu.Lock()
		delete(a.subs, ch)
		a.subsMu.Unlock()
	}
}

func (a *App) broadcast(name string, data any) {
	a.subsMu.Lock()
	defer a.subsMu.Unlock()
	if a.subsClosed {
		return
	}
	for ch := range a.subs {
		select {
		case ch <- Event{Name: name, Data: data}:
		default:
		}
	}
}

// Snapshot returns the full state of the application.
func (a *App) Snapshot() StateView {
	settings := a.store.Settings()

	a.mu.Lock()
	status := a.status
	busy := a.busy
	wintun := a.wintun
	lat := make(map[string]int, len(a.latencies))
	for k, v := range a.latencies {
		lat[k] = v
	}
	a.mu.Unlock()

	if status.Connected && status.Since != nil {
		status.UptimeSec = int64(time.Since(*status.Since).Seconds())
	}

	profiles := a.store.Profiles()
	views := make([]ProfileView, 0, len(profiles))
	for _, p := range profiles {
		view := ProfileView{
			ID:            p.ID,
			Name:          p.Name,
			Host:          p.Host,
			PublicKey:     p.ServerKey,
			PrivateKeySet: p.PrivateKey != "",
			TunnelAddress: p.TunnelAddress,
			Gateway:       p.Gateway,
			MTU:           p.MTU,
			CreatedAt:     p.CreatedAt,
		}
		if pub, err := p.PublicKey(); err == nil {
			view.ClientPublicKey = pub
		}
		if ms, ok := lat[p.ID]; ok {
			v := ms
			view.LatencyMs = &v
		}
		views = append(views, view)
	}

	entries := a.logs.Entries()
	if len(entries) > 200 {
		entries = entries[len(entries)-200:]
	}

	return StateView{
		Version:         a.version,
		OS:              runtime.GOOS,
		Elevated:        a.elevated,
		DataDir:         a.dataDir,
		Wintun:          wintun,
		Status:          status,
		Profiles:        views,
		ActiveProfileID: settings.ActiveProfile,
		Settings:        settings,
		Logs:            entries,
		Busy:            busy,
	}
}

// Connect starts the tunnel with the given profile.
func (a *App) Connect(profileID, mode string) error {
	// An empty mode means "use the mode from the settings", so the default
	// has to be applied before the mode is validated.
	if mode == "" {
		mode = a.store.Settings().Mode
	}
	if mode != "tunnel" && mode != "proxy" {
		return fmt.Errorf("app: unknown mode %q", mode)
	}

	a.mu.Lock()
	if a.conn != nil || a.connecting {
		a.mu.Unlock()
		return errors.New("app: the tunnel is already running")
	}
	if a.busy != "" {
		a.mu.Unlock()
		return ErrBusy
	}
	// The slot is taken before the slow part of the connection: downloading
	// the driver and creating the client can take seconds, and two parallel
	// requests would otherwise both pass the check and the first tunnel
	// would be left running without anybody watching it.
	a.connecting = true
	a.mu.Unlock()

	// Every early return has to give the slot back.
	connected := false
	defer func() {
		if connected {
			return
		}
		a.mu.Lock()
		a.connecting = false
		a.mu.Unlock()
	}()

	profile, found := a.store.Profile(profileID)
	if !found {
		return fmt.Errorf("app: profile %q not found", profileID)
	}
	if profile.PrivateKey == "" {
		return fmt.Errorf("app: the profile has no private key, import an invite code or add the key")
	}

	settings := a.store.Settings()
	if mode == "tunnel" {
		if err := a.prepareTunnel(); err != nil {
			return err
		}
	} else {
		// Fail before the tunnel comes up when the SOCKS5 port is taken: an
		// established tunnel without its listener looks like a working VPN
		// and silently sends nothing through it.
		listen := net.JoinHostPort("127.0.0.1", strconv.Itoa(settings.ProxyPort))
		probe, err := net.Listen("tcp", listen)
		if err != nil {
			return fmt.Errorf("app: the SOCKS5 port %d is busy: %w", settings.ProxyPort, err)
		}
		probe.Close()
	}

	cfg := buildClientConfig(profile, mode, settings)
	if cfg.TUN.Enabled {
		cfg.TUN.MTU = profile.MTU
	}

	registry := prometheus.NewRegistry()
	m := metrics.New(registry)
	c, err := client.New(cfg, m, a.logger)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	conn := &connection{
		client:    c,
		cancel:    cancel,
		done:      make(chan struct{}),
		profileID: profile.ID,
		mode:      mode,
		started:   time.Now(),
	}

	a.mu.Lock()
	a.conn = conn
	a.connecting = false
	a.status = StatusView{
		State:       "connecting",
		Mode:        mode,
		Server:      profile.Host,
		ProfileName: profile.Name,
	}
	a.mu.Unlock()
	connected = true

	a.store.SetSettings(withActive(settings, profile.ID, mode))
	a.broadcast("state", a.Snapshot().Status)
	a.logger.Info("connecting", "server", profile.Host, "mode", mode, "profile", profile.Name)

	go func() {
		err := c.Run(ctx)
		close(conn.done)
		a.finishConnection(conn, err)
	}()
	// The monitor must not run on the caller's goroutine: Connect is called
	// from an HTTP handler and has to return while the tunnel keeps running.
	go a.monitor(conn)
	return nil
}

// Disconnect stops the tunnel.
func (a *App) Disconnect() error {
	a.mu.Lock()
	conn := a.conn
	if conn == nil {
		a.mu.Unlock()
		return ErrNotConnected
	}
	a.status.State = "disconnecting"
	a.mu.Unlock()
	a.broadcast("state", a.Snapshot().Status)

	a.logger.Info("disconnecting")
	conn.cancel()

	select {
	case <-conn.done:
	case <-time.After(5 * time.Second):
		a.logger.Warn("the client did not stop in time")
	}
	return nil
}

// monitor polls the client once a second and pushes the traffic and the state
// changes to the interface.
func (a *App) monitor(conn *connection) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var lastRx, lastTx uint64
	var everConnected bool
	last := time.Now()
	for {
		select {
		case <-conn.done:
			return
		case <-ticker.C:
		}

		st := conn.client.Status()
		now := time.Now()
		elapsed := now.Sub(last).Seconds()
		if elapsed <= 0 {
			elapsed = 1
		}
		last = now

		// A handshake that never completes, or a session that stopped
		// rekeying, would leave the machine without internet in the tunnel
		// mode. Give up and roll back instead.
		if !st.Connected && !everConnected && now.Sub(conn.started) > connectTimeout {
			a.failConnection(conn, "the server did not answer within 25 seconds: check the address, the port, the firewall and the invite code")
			return
		}
		if st.Connected && !st.LastHandshake.IsZero() && now.Sub(st.LastHandshake) > staleTimeout {
			a.failConnection(conn, "the server stopped answering for six minutes: the tunnel was closed to restore the connection")
			return
		}

		var rxRate, txRate float64
		if st.RxBytes >= lastRx {
			rxRate = float64(st.RxBytes-lastRx) / elapsed
		}
		if st.TxBytes >= lastTx {
			txRate = float64(st.TxBytes-lastTx) / elapsed
		}
		lastRx, lastTx = st.RxBytes, st.TxBytes

		a.mu.Lock()
		if a.conn != conn {
			a.mu.Unlock()
			return
		}
		wasConnected := a.status.Connected
		a.status.RxBytes = st.RxBytes
		a.status.TxBytes = st.TxBytes
		a.status.RxRate = rxRate
		a.status.TxRate = txRate
		a.status.AssignedIP = st.AssignedIP
		switch st.State {
		case "connected":
			a.status.State = "connected"
			a.status.Connected = true
			everConnected = true
			if !wasConnected {
				since := st.Since
				if since.IsZero() {
					since = now
				}
				a.status.Since = &since
			}
			if st.RttMs > 0 {
				rtt := st.RttMs
				a.status.RttMs = &rtt
			}
		case "handshake":
			if a.status.Connected {
				a.status.State = "reconnecting"
			} else {
				a.status.State = "connecting"
			}
		default:
			if !a.status.Connected {
				a.status.State = "connecting"
			}
		}
		status := a.status
		settings := a.store.Settings()
		a.mu.Unlock()

		if st.State == "connected" && !wasConnected {
			a.logger.Info("tunnel established", "server", status.Server, "address", st.AssignedIP)
			if conn.mode == "proxy" && settings.SystemProxy {
				a.enableSystemProxy()
			}
		}
		if wasConnected && st.State != "connected" && status.State == "reconnecting" {
			a.logger.Warn("the session is being renegotiated")
		}

		uptime := int64(0)
		if status.Since != nil && status.Connected {
			uptime = int64(time.Since(*status.Since).Seconds())
		}
		a.broadcast("traffic", map[string]any{
			"rxBytes":   st.RxBytes,
			"txBytes":   st.TxBytes,
			"rxRate":    rxRate,
			"txRate":    txRate,
			"uptimeSec": uptime,
			"rxPackets": st.RxPackets,
			"txPackets": st.TxPackets,
		})
		if wasConnected != st.Connected || status.State != "connected" {
			a.broadcast("state", status)
		}
	}
}

// failConnection stops the client because of a problem the application
// detected itself, and keeps the reason for the final state.
func (a *App) failConnection(conn *connection, message string) {
	a.mu.Lock()
	if a.conn != conn {
		a.mu.Unlock()
		return
	}
	conn.failure = message
	a.mu.Unlock()

	a.logger.Error("the tunnel is being rolled back", "reason", message)
	conn.cancel()
}

// finishConnection is called when the client loop returns.
func (a *App) finishConnection(conn *connection, err error) {
	a.mu.Lock()
	if a.conn != conn {
		a.mu.Unlock()
		return
	}
	a.conn = nil
	proxyEnabled := a.proxyOn
	a.proxyOn = false
	failure := ""
	switch {
	case conn.failure != "":
		failure = conn.failure
	case err != nil && !errors.Is(err, context.Canceled):
		failure = err.Error()
	case conn.client.Status().LastError != "":
		failure = conn.client.Status().LastError
	}
	status := StatusView{State: "disconnected", Mode: conn.mode, Server: a.status.Server, ProfileName: a.status.ProfileName}
	if failure != "" {
		status.State = "error"
		status.Error = failure
	}
	a.status = status
	a.mu.Unlock()

	if proxyEnabled {
		a.disableSystemProxy()
	}
	if failure != "" {
		a.logger.Error("the tunnel stopped", "error", failure)
	} else {
		a.logger.Info("the tunnel is down")
	}
	a.broadcast("state", status)
}

// ClearLogs empties the log shown in the interface.
func (a *App) ClearLogs() {
	a.logs.Clear()
	a.broadcast("logs-cleared", nil)
}

// Shutdown stops everything and releases the resources.
func (a *App) Shutdown() {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.closed = true
	conn := a.conn
	a.mu.Unlock()

	if conn != nil {
		conn.cancel()
		select {
		case <-conn.done:
		case <-time.After(5 * time.Second):
		}
	}
	a.mu.Lock()
	proxyEnabled := a.proxyOn
	a.proxyOn = false
	a.mu.Unlock()
	if proxyEnabled {
		a.disableSystemProxy()
	}

	a.subsMu.Lock()
	a.subsClosed = true
	for ch := range a.subs {
		close(ch)
		delete(a.subs, ch)
	}
	a.subsMu.Unlock()
	a.logs.Close()
}

// buildClientConfig turns a profile into the configuration of the client.
func buildClientConfig(p Profile, mode string, s Settings) *config.ClientConfig {
	cfg := &config.ClientConfig{
		Server: config.ServerEndpoint{
			Address:   p.Host,
			PublicKey: p.ServerKey,
		},
		PrivateKey: p.PrivateKey,
		DNS:        s.DNS,
		HTTP:       config.HTTPConfig{Metrics: config.MetricsConfig{Enabled: false}},
	}
	if mode == "tunnel" {
		cfg.TUN = config.ClientTUNConfig{
			Enabled:         true,
			Name:            "W1nWire",
			Address:         p.TunnelAddress,
			Gateway:         p.Gateway,
			MTU:             p.MTU,
			ConfigureRoutes: true,
			RouteAll:        false,
			AllowIPv6:       !s.BlockIPv6,
		}
	} else {
		cfg.SOCKS5 = config.SOCKS5Config{
			Enabled: true,
			Listen:  net.JoinHostPort("127.0.0.1", strconv.Itoa(s.ProxyPort)),
		}
	}
	return cfg
}

func withActive(s Settings, profileID, mode string) Settings {
	s.ActiveProfile = profileID
	s.Mode = mode
	return s
}

// ImportInvite adds a profile from an invite code.
func (a *App) ImportInvite(code string) (ProfileView, error) {
	inv, err := invite.Decode(code)
	if err != nil {
		return ProfileView{}, err
	}
	if inv.PrivateKey == "" {
		return ProfileView{}, fmt.Errorf("app: the code does not contain the client key, add the server manually and put the public key on the server")
	}
	p := Profile{
		Name:          firstNonEmpty(inv.Name, "server"),
		Host:          inv.Host,
		ServerKey:     inv.ServerKey,
		PrivateKey:    inv.PrivateKey,
		TunnelAddress: inv.Address,
		Gateway:       inv.Gateway,
		MTU:           inv.MTU,
	}
	if p.TunnelAddress != "" && !strings.Contains(p.TunnelAddress, "/") {
		p.TunnelAddress += "/24"
	}
	saved, err := a.store.AddProfile(p)
	if err != nil {
		return ProfileView{}, err
	}
	a.logger.Info("profile imported", "name", saved.Name, "server", saved.Host)
	return a.profileView(saved), nil
}

// AddProfile adds a profile with a freshly generated client key.
func (a *App) AddProfile(p Profile) (ProfileView, string, error) {
	norm, err := NormalizeProfile(p)
	if err != nil {
		return ProfileView{}, "", err
	}
	if norm.PrivateKey == "" {
		key, _, err := generateKey()
		if err != nil {
			return ProfileView{}, "", err
		}
		norm.PrivateKey = key
	}
	pub, err := norm.PublicKey()
	if err != nil {
		return ProfileView{}, "", err
	}
	if norm.ServerKey == "" {
		return ProfileView{}, "", fmt.Errorf("app: the public key of the server is required")
	}
	saved, err := a.store.AddProfile(norm)
	if err != nil {
		return ProfileView{}, "", err
	}
	a.logger.Info("profile added", "name", saved.Name, "server", saved.Host)
	a.broadcast("profiles", a.Snapshot().Profiles)
	return a.profileView(saved), pub, nil
}

// UpdateProfile changes the editable fields of a profile.
func (a *App) UpdateProfile(p Profile) (ProfileView, error) {
	existing, ok := a.store.Profile(p.ID)
	if !ok {
		return ProfileView{}, fmt.Errorf("app: profile %q not found", p.ID)
	}
	p.PrivateKey = firstNonEmpty(p.PrivateKey, existing.PrivateKey)
	norm, err := NormalizeProfile(p)
	if err != nil {
		return ProfileView{}, err
	}
	saved, err := a.store.UpdateProfile(norm)
	if err != nil {
		return ProfileView{}, err
	}
	a.broadcast("profiles", a.Snapshot().Profiles)
	return a.profileView(saved), nil
}

// DeleteProfile removes a profile.
func (a *App) DeleteProfile(id string) error {
	a.mu.Lock()
	running := a.conn != nil && a.conn.profileID == id
	a.mu.Unlock()
	if running {
		return fmt.Errorf("app: disconnect before deleting the profile in use")
	}
	if err := a.store.DeleteProfile(id); err != nil {
		return err
	}
	a.mu.Lock()
	delete(a.latencies, id)
	a.mu.Unlock()
	a.logger.Info("profile deleted", "id", id)
	a.broadcast("profiles", a.Snapshot().Profiles)
	return nil
}

// ActivateProfile selects the profile used by the interface.
func (a *App) ActivateProfile(id string) error {
	if err := a.store.ActivateProfile(id); err != nil {
		return err
	}
	a.broadcast("profiles", a.Snapshot().Profiles)
	return nil
}

func (a *App) profileView(p Profile) ProfileView {
	pub, _ := p.PublicKey()
	view := ProfileView{
		ID:              p.ID,
		Name:            p.Name,
		Host:            p.Host,
		PublicKey:       p.ServerKey,
		PrivateKeySet:   p.PrivateKey != "",
		ClientPublicKey: pub,
		TunnelAddress:   p.TunnelAddress,
		Gateway:         p.Gateway,
		MTU:             p.MTU,
		CreatedAt:       p.CreatedAt,
	}
	a.mu.Lock()
	if ms, ok := a.latencies[p.ID]; ok {
		v := ms
		view.LatencyMs = &v
	}
	a.mu.Unlock()
	return view
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
