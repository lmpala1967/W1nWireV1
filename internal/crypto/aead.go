package crypto

import (
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

// Sizes of the ChaCha20-Poly1305 construction.
const (
	AEADKeySize   = chacha20poly1305.KeySize
	AEADNonceSize = chacha20poly1305.NonceSize
	AEADOverhead  = 16
)

// AEAD wraps ChaCha20-Poly1305 with a counter based nonce scheme.
//
// The nonce is 12 bytes: four zero bytes followed by a little endian 64 bit
// counter. Every direction of a session owns its own key, therefore the
// counter space of one direction never overlaps with the other one.
type AEAD struct {
	key    [AEADKeySize]byte
	cipher cipher.AEAD
}

// NewAEAD builds an AEAD from a 32 byte key.
func NewAEAD(key []byte) (*AEAD, error) {
	if len(key) != AEADKeySize {
		return nil, fmt.Errorf("crypto: aead: key must be %d bytes, got %d", AEADKeySize, len(key))
	}
	c, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: aead: %w", err)
	}
	a := &AEAD{cipher: c}
	copy(a.key[:], key)
	return a, nil
}

// Nonce returns the nonce that belongs to the given counter.
func Nonce(counter uint64) []byte {
	nonce := make([]byte, AEADNonceSize)
	binary.LittleEndian.PutUint64(nonce[4:], counter)
	return nonce
}

// Seal encrypts the plaintext and appends the authentication tag.
func (a *AEAD) Seal(counter uint64, plaintext []byte) []byte {
	out := make([]byte, 0, len(plaintext)+AEADOverhead)
	return a.cipher.Seal(out, Nonce(counter), plaintext, nil)
}

// Open verifies and decrypts a ciphertext produced by Seal.
func (a *AEAD) Open(counter uint64, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < AEADOverhead {
		return nil, errors.New("crypto: aead: ciphertext too short")
	}
	out, err := a.cipher.Open(nil, Nonce(counter), ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: aead: %w", err)
	}
	return out, nil
}

// SealAt encrypts with an explicit nonce, used for the cookie message where
// the nonce is derived from random bytes instead of from a counter.
func (a *AEAD) SealAt(nonce []byte, plaintext []byte) ([]byte, error) {
	return a.cipher.Seal(nil, nonce, plaintext, nil), nil
}

// OpenAt decrypts with an explicit nonce.
func (a *AEAD) OpenAt(nonce []byte, ciphertext []byte) ([]byte, error) {
	out, err := a.cipher.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: aead: %w", err)
	}
	return out, nil
}
