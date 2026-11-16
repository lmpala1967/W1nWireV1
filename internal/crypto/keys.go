// Package crypto provides the cryptographic primitives used by the W1nWire
// protocol: X25519 key pairs, HKDF-SHA256 key derivation and ChaCha20-Poly1305
// authenticated encryption with counter based nonces.
//
// The package intentionally depends only on the Go standard library so that
// the security relevant code can be audited in one place.
package crypto

import (
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// KeySize is the length in bytes of every symmetric key and of every
// X25519 public or private key handled by the protocol.
const KeySize = 32

// ErrInvalidKey is returned when a key cannot be parsed or is malformed.
var ErrInvalidKey = errors.New("crypto: invalid key")

// PrivateKey is an X25519 private key.
type PrivateKey [KeySize]byte

// PublicKey is an X25519 public key.
type PublicKey [KeySize]byte

// GenerateKey creates a fresh X25519 key pair using the system CSPRNG.
func GenerateKey() (PrivateKey, PublicKey, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return PrivateKey{}, PublicKey{}, fmt.Errorf("crypto: generate key: %w", err)
	}
	var out PrivateKey
	copy(out[:], priv.Bytes())
	return out, out.Public(), nil
}

// Public derives the public key that belongs to the private key.
func (p PrivateKey) Public() PublicKey {
	priv, err := ecdh.X25519().NewPrivateKey(p[:])
	if err != nil {
		// NewPrivateKey only fails for a wrong length, which cannot happen
		// because the array length is fixed at compile time.
		panic("crypto: unreachable: " + err.Error())
	}
	var out PublicKey
	copy(out[:], priv.PublicKey().Bytes())
	return out
}

// DH computes the X25519 shared secret between the private key and the given
// public key. All-zero output is rejected: it indicates a low order public key
// and would silently destroy the security of the derived session keys.
func (p PrivateKey) DH(pub PublicKey) ([]byte, error) {
	priv, err := ecdh.X25519().NewPrivateKey(p[:])
	if err != nil {
		return nil, fmt.Errorf("crypto: private key: %w", err)
	}
	remote, err := ecdh.X25519().NewPublicKey(pub[:])
	if err != nil {
		return nil, fmt.Errorf("crypto: public key: %w", err)
	}
	secret, err := priv.ECDH(remote)
	if err != nil {
		return nil, fmt.Errorf("crypto: dh: %w", err)
	}
	if isAllZero(secret) {
		return nil, errors.New("crypto: dh: degenerate (all zero) shared secret")
	}
	return secret, nil
}

// String returns the base64 (raw, unpadded) representation of the key.
func (p PrivateKey) String() string {
	return base64.RawStdEncoding.EncodeToString(p[:])
}

// String returns the base64 (raw, unpadded) representation of the key.
func (p PublicKey) String() string {
	return base64.RawStdEncoding.EncodeToString(p[:])
}

// ParsePrivateKey decodes a base64 encoded private key.
func ParsePrivateKey(s string) (PrivateKey, error) {
	var key PrivateKey
	raw, err := decodeKey(s)
	if err != nil {
		return key, err
	}
	copy(key[:], raw)
	return key, nil
}

// ParsePublicKey decodes a base64 encoded public key.
func ParsePublicKey(s string) (PublicKey, error) {
	var key PublicKey
	raw, err := decodeKey(s)
	if err != nil {
		return key, err
	}
	copy(key[:], raw)
	return key, nil
}

func decodeKey(s string) ([]byte, error) {
	if s == "" {
		return nil, fmt.Errorf("%w: empty", ErrInvalidKey)
	}
	raw, err := base64.RawStdEncoding.DecodeString(s)
	if err != nil {
		// Tolerate padded base64 as well, it is what most CLI tools print.
		raw, err = base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("%w: not base64: %v", ErrInvalidKey, err)
		}
	}
	if len(raw) != KeySize {
		return nil, fmt.Errorf("%w: expected %d bytes, got %d", ErrInvalidKey, KeySize, len(raw))
	}
	return raw, nil
}

// Hex returns the hex encoding of the key, used for diagnostics.
func (p PublicKey) Hex() string { return hex.EncodeToString(p[:]) }

// MAC computes HMAC-SHA256 over the concatenation of the given messages.
func MAC(key []byte, messages ...[]byte) []byte {
	mac := hmac.New(sha256.New, key)
	for _, m := range messages {
		mac.Write(m)
	}
	return mac.Sum(nil)
}

// ConstantTimeEqual compares two byte slices in constant time.
func ConstantTimeEqual(a, b []byte) bool {
	return hmac.Equal(a, b)
}

// RandomBytes returns n cryptographically random bytes.
func RandomBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return nil, fmt.Errorf("crypto: random: %w", err)
	}
	return buf, nil
}

func isAllZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}
