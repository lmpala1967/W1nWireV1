// Package invite encodes and decodes the one line "invite code" that
// connects a fresh client to a server.
//
// An invite code carries everything a client needs to reach a server and to
// authenticate itself in the handshake: the address of the server, its static
// public key, and the static private key of the client. It is therefore a
// credential, not a bookmark, and it must be treated as a secret.
//
// The code is intentionally short enough to be pasted into a chat window:
//
//	ww1.eyJ2IjoxLCJuIjoiZGVza3RvcCIsImgiOiIxLjIuMy40OjUxODIwIiwia...
//
// The payload is compact JSON encoded with the URL safe base64 alphabet
// without padding, so the code survives copy and paste and does not need
// escaping in a URL or in a QR code.
package invite

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Prefix identifies an invite code.
const Prefix = "ww1."

// Version is the current version of the payload.
const Version = 1

// ErrInvalid is returned when a string is not an invite code.
var ErrInvalid = errors.New("invite: invalid code")

// Invite is the decoded payload of an invite code.
type Invite struct {
	// Version is the layout version of the payload.
	Version int `json:"v"`
	// Name is the human readable name of the client, for example "desktop".
	Name string `json:"n"`
	// Host is the address of the server, host:port.
	Host string `json:"h"`
	// ServerKey is the base64 static public key of the server.
	ServerKey string `json:"k"`
	// PrivateKey is the base64 static private key of the client.
	PrivateKey string `json:"p"`
	// Address is the address of the client inside the tunnel, without mask.
	Address string `json:"a"`
	// Gateway is the address of the server inside the tunnel.
	Gateway string `json:"g"`
	// MTU is the optional tunnel MTU.
	MTU int `json:"m,omitempty"`
	// Created is the optional creation time, in RFC 3339.
	Created string `json:"t,omitempty"`
}

// New builds an invite from its parts, filling the defaults.
func New(name, host, serverKey, privateKey, address, gateway string) *Invite {
	if address == "" {
		address = "10.66.0.2"
	}
	if gateway == "" {
		gateway = "10.66.0.1"
	}
	return &Invite{
		Version:    Version,
		Name:       name,
		Host:       host,
		ServerKey:  serverKey,
		PrivateKey: privateKey,
		Address:    address,
		Gateway:    gateway,
		MTU:        1420,
		Created:    time.Now().UTC().Format(time.RFC3339),
	}
}

// Encode returns the invite code of the invite.
func (i *Invite) Encode() (string, error) {
	if err := i.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(i)
	if err != nil {
		return "", fmt.Errorf("invite: encode: %w", err)
	}
	return Prefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

// String returns the invite code, or an empty string when the invite is
// incomplete.
func (i *Invite) String() string {
	code, err := i.Encode()
	if err != nil {
		return ""
	}
	return code
}

// Validate checks that the invite can be used to connect.
func (i *Invite) Validate() error {
	if i.Version != Version {
		return fmt.Errorf("%w: unsupported version %d", ErrInvalid, i.Version)
	}
	if i.Host == "" {
		return fmt.Errorf("%w: the server address is missing", ErrInvalid)
	}
	if _, _, err := net.SplitHostPort(i.Host); err != nil {
		return fmt.Errorf("%w: invalid server address %q", ErrInvalid, i.Host)
	}
	if host, _, err := net.SplitHostPort(i.Host); err == nil {
		if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
			return fmt.Errorf("%w: %q is not a reachable server address", ErrInvalid, host)
		}
	}
	if i.ServerKey == "" {
		return fmt.Errorf("%w: the server key is missing", ErrInvalid)
	}
	if i.PrivateKey == "" {
		return fmt.Errorf("%w: the client key is missing", ErrInvalid)
	}
	if i.Address != "" && net.ParseIP(i.Address) == nil {
		return fmt.Errorf("%w: invalid tunnel address %q", ErrInvalid, i.Address)
	}
	if i.Gateway != "" && net.ParseIP(i.Gateway) == nil {
		return fmt.Errorf("%w: invalid gateway %q", ErrInvalid, i.Gateway)
	}
	return nil
}

// Decode parses an invite code. It accepts the canonical form, the same code
// behind a w1nwire:// scheme, and a human readable "host|serverkey|name"
// triple that can be typed by hand.
func Decode(s string) (*Invite, error) {
	code := strings.TrimSpace(s)
	code = strings.Trim(code, "\"'`")
	code = strings.Join(strings.Fields(code), "")
	if code == "" {
		return nil, fmt.Errorf("%w: empty code", ErrInvalid)
	}

	const scheme = "w1nwire://"
	if len(code) >= len(scheme) && strings.EqualFold(code[:len(scheme)], scheme) {
		code = code[len(scheme):]
	}
	if len(code) >= len(Prefix) && strings.EqualFold(code[:len(Prefix)], Prefix) {
		code = code[len(Prefix):]
	}
	if code == "" {
		return nil, fmt.Errorf("%w: empty code", ErrInvalid)
	}

	if strings.Contains(code, "|") {
		return decodePlain(code)
	}

	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(code, "="))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	var inv Invite
	if err := json.Unmarshal(payload, &inv); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := inv.Validate(); err != nil {
		return nil, err
	}
	return &inv, nil
}

// decodePlain parses the legacy "host|serverkey|name" form. It is useful
// when the code has to be dictated or typed by hand.
func decodePlain(s string) (*Invite, error) {
	parts := strings.Split(s, "|")
	if len(parts) < 2 {
		return nil, fmt.Errorf("%w: expected host|serverkey|name", ErrInvalid)
	}
	host := strings.TrimSpace(parts[0])
	key := strings.TrimSpace(parts[1])
	name := "server"
	if len(parts) > 2 && strings.TrimSpace(parts[2]) != "" {
		name = strings.TrimSpace(parts[2])
	}
	// The private key is generated by the caller: the plain form cannot
	// carry it. Validate is therefore not called here.
	return New(name, host, key, "", "10.66.0.2", "10.66.0.1"), nil
}

// ParseAddress splits an address into host and port, filling the default
// port when it is missing.
func ParseAddress(s string) (host string, port int, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0, fmt.Errorf("%w: empty address", ErrInvalid)
	}
	if _, _, splitErr := net.SplitHostPort(s); splitErr == nil {
		h, p, err := net.SplitHostPort(s)
		if err != nil {
			return "", 0, err
		}
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 || n > 65535 {
			return "", 0, fmt.Errorf("%w: invalid port %q", ErrInvalid, p)
		}
		return h, n, nil
	}
	if net.ParseIP(s) != nil {
		return s, 51820, nil
	}
	// A bare host name is accepted, the default port is appended.
	if strings.Contains(s, ":") {
		return "", 0, fmt.Errorf("%w: invalid address %q", ErrInvalid, s)
	}
	return s, 51820, nil
}
