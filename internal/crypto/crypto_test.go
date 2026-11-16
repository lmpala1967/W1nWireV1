package crypto

import (
	"bytes"
	"testing"
)

func TestGenerateKeyAndParse(t *testing.T) {
	priv, pub, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if priv.Public() != pub {
		t.Fatal("the derived public key does not match")
	}

	parsedPriv, err := ParsePrivateKey(priv.String())
	if err != nil {
		t.Fatal(err)
	}
	if parsedPriv != priv {
		t.Fatal("the parsed private key differs")
	}

	parsedPub, err := ParsePublicKey(pub.String())
	if err != nil {
		t.Fatal(err)
	}
	if parsedPub != pub {
		t.Fatal("the parsed public key differs")
	}
}

func TestParseKeyRejectsGarbage(t *testing.T) {
	if _, err := ParsePublicKey(""); err == nil {
		t.Error("an empty key was accepted")
	}
	if _, err := ParsePublicKey("not base64!"); err == nil {
		t.Error("a key that is not base64 was accepted")
	}
	if _, err := ParsePublicKey("c2hvcnQ="); err == nil {
		t.Error("a key of the wrong length was accepted")
	}
}

func TestDHIsSymmetric(t *testing.T) {
	privA, pubA, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	privB, pubB, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	secretA, err := privA.DH(pubB)
	if err != nil {
		t.Fatal(err)
	}
	secretB, err := privB.DH(pubA)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(secretA, secretB) {
		t.Fatal("the shared secrets differ")
	}
}

func TestAEADRoundTrip(t *testing.T) {
	key, err := RandomBytes(KeySize)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := NewAEAD(key)
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("the quick brown fox jumps over the lazy dog")
	ciphertext := aead.Seal(7, plaintext)
	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("the plaintext was not encrypted")
	}
	opened, err := aead.Open(7, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(opened, plaintext) {
		t.Fatal("the decrypted payload differs")
	}
	if _, err := aead.Open(8, ciphertext); err == nil {
		t.Fatal("a packet was accepted with the wrong counter")
	}
}

func TestAEADRejectsTamperedCiphertext(t *testing.T) {
	key, _ := RandomBytes(KeySize)
	aead, _ := NewAEAD(key)
	ciphertext := aead.Seal(0, []byte("payload"))
	ciphertext[0] ^= 0xff
	if _, err := aead.Open(0, ciphertext); err == nil {
		t.Fatal("a tampered ciphertext was accepted")
	}
}

func TestHKDFIsDeterministicAndSeparated(t *testing.T) {
	ikm := []byte("input keying material")
	salt := []byte("salt")

	first, err := Derive(ikm, salt, "label-a", 32)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Derive(ikm, salt, "label-a", 32)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("the derivation is not deterministic")
	}

	other, err := Derive(ikm, salt, "label-b", 32)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, other) {
		t.Fatal("two different labels produced the same key")
	}
}

func TestDeriveTwoIsIndependentOfTheOrder(t *testing.T) {
	ikm := []byte("input keying material")
	salt := []byte("salt")

	a1, b1, err := DeriveTwo(ikm, salt, "one", "two")
	if err != nil {
		t.Fatal(err)
	}
	b2, a2, err := DeriveTwo(ikm, salt, "two", "one")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a1, a2) || !bytes.Equal(b1, b2) {
		t.Fatal("the keys depend on the order of the labels")
	}
}

func TestConstantTimeEqual(t *testing.T) {
	if !ConstantTimeEqual([]byte("abc"), []byte("abc")) {
		t.Error("equal slices were reported as different")
	}
	if ConstantTimeEqual([]byte("abc"), []byte("abd")) {
		t.Error("different slices were reported as equal")
	}
}
