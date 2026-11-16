// Package app implements the application layer of the desktop client: the
// stored profiles, the settings, the lifecycle of the tunnel and the
// operations the user interface can trigger.
//
// The package is deliberately free of any user interface code. It is used by
// the local HTTP server that serves the window of the desktop application, and
// by the tests.
package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/w1nchi/w1nwire/internal/crypto"
)

// DefaultTunnelAddress is the address handed out to the first client.
const DefaultTunnelAddress = "10.66.0.2/24"

// DefaultGateway is the address of the server inside the tunnel.
const DefaultGateway = "10.66.0.1"

// DefaultMTU is the tunnel MTU, chosen so that a full sized TCP segment plus
// the W1nWire header fits into a 1500 byte path MTU.
const DefaultMTU = 1420

// DefaultProxyPort is the port of the local SOCKS5 proxy.
const DefaultProxyPort = 1080

// Profile is one configured server, together with the static key of the
// client. It is stored in the data directory of the application.
type Profile struct {
	ID string `json:"id"`
	// Name is the name shown in the interface.
	Name string `json:"name"`
	// Host is the address of the server, host:port.
	Host string `json:"host"`
	// ServerKey is the base64 static public key of the server.
	ServerKey string `json:"public_key"`
	// PrivateKey is the base64 static private key of the client. It never
	// leaves the process: the API exposes only its public half.
	PrivateKey string `json:"private_key"`
	// TunnelAddress is the address of the client inside the tunnel, CIDR.
	TunnelAddress string `json:"tunnel_address"`
	// Gateway is the address of the server inside the tunnel.
	Gateway string `json:"gateway"`
	// MTU is the MTU of the tunnel device.
	MTU int `json:"mtu"`
	// CreatedAt is when the profile was added.
	CreatedAt time.Time `json:"created_at"`
}

// PublicKey returns the static public key of the client, derived from the
// stored private key.
func (p Profile) PublicKey() (string, error) {
	priv, err := crypto.ParsePrivateKey(p.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("app: profile %q: %w", p.Name, err)
	}
	return priv.Public().String(), nil
}

// Settings is the persisted configuration of the application itself.
type Settings struct {
	Language       string `json:"language"`
	DNS            string `json:"dns"`
	MTU            int    `json:"mtu"`
	ProxyPort      int    `json:"proxyPort"`
	BlockIPv6      bool   `json:"blockIPv6"`
	AutoStart      bool   `json:"autoStart"`
	AutoConnect    bool   `json:"autoConnect"`
	MinimizeToTray bool   `json:"minimizeToTray"`
	SystemProxy    bool   `json:"systemProxy"`
	// Mode is the transport selected the last time, "tunnel" or "proxy".
	Mode string `json:"mode"`
	// ActiveProfile is the id of the profile the user selected.
	ActiveProfile string `json:"activeProfile"`
}

// DefaultSettings returns the settings of a fresh installation.
func DefaultSettings() Settings {
	return Settings{
		Language:       "ru",
		DNS:            "1.1.1.1",
		MTU:            DefaultMTU,
		ProxyPort:      DefaultProxyPort,
		BlockIPv6:      true,
		MinimizeToTray: false,
		Mode:           "tunnel",
	}
}

// store is the on disk representation of the application state.
type store struct {
	Version  int       `json:"version"`
	Settings Settings  `json:"settings"`
	Profiles []Profile `json:"profiles"`
}

// Store manages the JSON file that holds the profiles and the settings.
type Store struct {
	path   string
	mu     chan struct{}
	state  store
	loaded bool
}

// OpenStore loads the store from the data directory, creating it when it does
// not exist yet.
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("app: create the data directory: %w", err)
	}
	s := &Store{
		path: filepath.Join(dir, "config.json"),
		mu:   make(chan struct{}, 1),
	}
	s.mu <- struct{}{}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	<-s.mu
	defer func() { s.mu <- struct{}{} }()

	s.state = store{Version: 1, Settings: DefaultSettings()}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.loaded = true
			return nil
		}
		return fmt.Errorf("app: read the configuration: %w", err)
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &s.state); err != nil {
			return fmt.Errorf("app: parse %s: %w", s.path, err)
		}
	}
	s.state.Settings = withDefaults(s.state.Settings)
	s.loaded = true
	return nil
}

// withDefaults fills the fields that are missing in an old configuration.
func withDefaults(s Settings) Settings {
	d := DefaultSettings()
	if s.Language == "" {
		s.Language = d.Language
	}
	if s.DNS == "" {
		s.DNS = d.DNS
	}
	if s.MTU <= 0 {
		s.MTU = d.MTU
	}
	if s.ProxyPort <= 0 || s.ProxyPort > 65535 {
		s.ProxyPort = d.ProxyPort
	}
	if s.Mode != "tunnel" && s.Mode != "proxy" {
		s.Mode = d.Mode
	}
	return s
}

func (s *Store) saveLocked() error {
	s.state.Version = 1
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("app: encode the configuration: %w", err)
	}
	data = append(data, '\n')

	// Write to a temporary file in the same directory and rename it, so a
	// crash in the middle of the write cannot leave a truncated file that
	// would lose every profile.
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".config-*.json")
	if err != nil {
		return fmt.Errorf("app: write the configuration: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("app: write the configuration: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("app: write the configuration: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("app: write the configuration: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("app: write the configuration: %w", err)
	}
	if err := os.Rename(name, s.path); err != nil {
		return fmt.Errorf("app: write the configuration: %w", err)
	}
	return nil
}

// Settings returns a copy of the current settings.
func (s *Store) Settings() Settings {
	<-s.mu
	defer func() { s.mu <- struct{}{} }()
	return s.state.Settings
}

// SetSettings replaces the settings and persists them.
func (s *Store) SetSettings(v Settings) error {
	<-s.mu
	defer func() { s.mu <- struct{}{} }()
	s.state.Settings = withDefaults(v)
	return s.saveLocked()
}

// Profiles returns a copy of the configured profiles.
func (s *Store) Profiles() []Profile {
	<-s.mu
	defer func() { s.mu <- struct{}{} }()
	out := make([]Profile, len(s.state.Profiles))
	copy(out, s.state.Profiles)
	return out
}

// Profile returns the profile with the given id.
func (s *Store) Profile(id string) (Profile, bool) {
	<-s.mu
	defer func() { s.mu <- struct{}{} }()
	for _, p := range s.state.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return Profile{}, false
}

// AddProfile appends a profile and makes it active when it is the first one.
func (s *Store) AddProfile(p Profile) (Profile, error) {
	<-s.mu
	defer func() { s.mu <- struct{}{} }()
	if p.ID == "" {
		p.ID = newID()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	for _, existing := range s.state.Profiles {
		if existing.Host == p.Host && existing.ServerKey == p.ServerKey {
			return Profile{}, fmt.Errorf("app: a server with the same address and key is already configured")
		}
	}
	s.state.Profiles = append(s.state.Profiles, p)
	if s.state.Settings.ActiveProfile == "" {
		s.state.Settings.ActiveProfile = p.ID
	}
	if err := s.saveLocked(); err != nil {
		return Profile{}, err
	}
	return p, nil
}

// UpdateProfile replaces the profile with the same id.
func (s *Store) UpdateProfile(p Profile) (Profile, error) {
	<-s.mu
	defer func() { s.mu <- struct{}{} }()
	for i := range s.state.Profiles {
		if s.state.Profiles[i].ID == p.ID {
			if p.PrivateKey == "" {
				p.PrivateKey = s.state.Profiles[i].PrivateKey
			}
			if p.CreatedAt.IsZero() {
				p.CreatedAt = s.state.Profiles[i].CreatedAt
			}
			s.state.Profiles[i] = p
			return p, s.saveLocked()
		}
	}
	return Profile{}, fmt.Errorf("app: profile %q not found", p.ID)
}

// DeleteProfile removes the profile with the given id.
func (s *Store) DeleteProfile(id string) error {
	<-s.mu
	defer func() { s.mu <- struct{}{} }()
	kept := s.state.Profiles[:0]
	found := false
	for _, p := range s.state.Profiles {
		if p.ID == id {
			found = true
			continue
		}
		kept = append(kept, p)
	}
	if !found {
		return fmt.Errorf("app: profile %q not found", id)
	}
	s.state.Profiles = kept
	if s.state.Settings.ActiveProfile == id {
		s.state.Settings.ActiveProfile = ""
		if len(s.state.Profiles) > 0 {
			s.state.Settings.ActiveProfile = s.state.Profiles[0].ID
		}
	}
	return s.saveLocked()
}

// ActivateProfile marks a profile as the selected one.
func (s *Store) ActivateProfile(id string) error {
	<-s.mu
	defer func() { s.mu <- struct{}{} }()
	for _, p := range s.state.Profiles {
		if p.ID == id {
			s.state.Settings.ActiveProfile = id
			return s.saveLocked()
		}
	}
	return fmt.Errorf("app: profile %q not found", id)
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("p%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// NormalizeProfile fills the defaults and validates a profile supplied by the
// user interface or by an invite code.
func NormalizeProfile(p Profile) (Profile, error) {
	p.Name = strings.TrimSpace(p.Name)
	p.Host = strings.TrimSpace(p.Host)
	p.TunnelAddress = strings.TrimSpace(p.TunnelAddress)
	p.Gateway = strings.TrimSpace(p.Gateway)

	if p.Name == "" {
		p.Name = p.Host
	}
	if p.Host == "" {
		return Profile{}, fmt.Errorf("app: the server address is required")
	}
	if !strings.Contains(p.Host, ":") {
		p.Host += ":51820"
	}
	if p.TunnelAddress == "" {
		p.TunnelAddress = DefaultTunnelAddress
	}
	if !strings.Contains(p.TunnelAddress, "/") {
		p.TunnelAddress += "/24"
	}
	if p.Gateway == "" {
		p.Gateway = DefaultGateway
	}
	if p.MTU <= 0 {
		p.MTU = DefaultMTU
	}
	if p.MTU > 1500 {
		return Profile{}, fmt.Errorf("app: the MTU cannot exceed 1500")
	}
	return p, nil
}
