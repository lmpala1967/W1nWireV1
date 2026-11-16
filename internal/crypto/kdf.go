package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
)

// HKDF is the HMAC-SHA256 based key derivation function (RFC 5869).
//
// It is implemented locally instead of pulling a dependency so that the whole
// key schedule of the protocol can be reviewed in a single file.
type HKDF struct {
	secret  []byte
	prk     []byte
	info    []byte
	counter byte
	block   []byte
}

// NewHKDF performs the extract step: it combines the input keying material
// with the optional salt into a pseudo random key.
func NewHKDF(ikm, salt []byte) *HKDF {
	if salt == nil {
		salt = make([]byte, sha256.Size)
	}
	extractor := hmac.New(sha256.New, salt)
	extractor.Write(ikm)
	return &HKDF{secret: append([]byte(nil), ikm...), prk: extractor.Sum(nil)}
}

// Expand derives the requested number of bytes. It may be called several
// times; subsequent calls continue the byte stream.
func (h *HKDF) Expand(length int, info []byte) ([]byte, error) {
	if length < 0 || length > 255*sha256.Size {
		return nil, errors.New("crypto: hkdf: invalid requested length")
	}
	out := make([]byte, 0, length)
	for len(out) < length {
		h.counter++
		mac := hmac.New(sha256.New, h.prk)
		mac.Write(h.block)
		mac.Write(info)
		mac.Write([]byte{h.counter})
		h.block = mac.Sum(nil)
		out = append(out, h.block...)
	}
	return out[:length], nil
}

// Derive is a convenience wrapper that performs extract and expand in one
// call. The salt may be nil, in which case a zero salt is used.
func Derive(ikm, salt []byte, info string, length int) ([]byte, error) {
	return NewHKDF(ikm, salt).Expand(length, []byte(info))
}

// DeriveTwo derives two independent keys from the same input keying material.
//
// The two outputs are separated by different info labels. Each label is
// expanded from its own HKDF instance, so the result of a derivation never
// depends on the order in which the caller asks for the keys.
func DeriveTwo(ikm, salt []byte, labelA, labelB string) (keyA, keyB []byte, err error) {
	if keyA, err = Derive(ikm, salt, labelA, KeySize); err != nil {
		return nil, nil, err
	}
	if keyB, err = Derive(ikm, salt, labelB, KeySize); err != nil {
		return nil, nil, err
	}
	return keyA, keyB, nil
}
