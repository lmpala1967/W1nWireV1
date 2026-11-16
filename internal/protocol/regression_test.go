package protocol

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/w1nchi/w1nwire/internal/crypto"
)

// establish runs a full handshake and returns the two sessions.
func establish(t *testing.T) (*Responder, *Initiator, *Session, *Session, *net.UDPAddr, time.Time) {
	t.Helper()
	clientPriv, clientPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	serverPriv, serverPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	responder, err := NewResponder(serverPriv, []Peer{{ID: "peer1", Public: clientPub, AssignedIP: [4]byte{10, 66, 0, 2}}})
	if err != nil {
		t.Fatal(err)
	}
	initiator, err := NewInitiator(clientPriv, serverPub, 0x11223344)
	if err != nil {
		t.Fatal(err)
	}
	src := testAddr()
	cookieResp, err := responder.HandleCookieRequest(src, initiator.CookieRequestBytes(), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := initiator.ApplyCookieResponse(cookieResp); err != nil {
		t.Fatal(err)
	}
	initBytes, err := initiator.HandshakeInitBytes(now)
	if err != nil {
		t.Fatal(err)
	}
	handshakeResp, serverSession, err := responder.HandleHandshakeInit(src, initBytes, now)
	if err != nil {
		t.Fatal(err)
	}
	clientSession, err := initiator.ApplyHandshakeResponse(handshakeResp, now)
	if err != nil {
		t.Fatal(err)
	}
	return responder, initiator, serverSession, clientSession, src, now
}

// TestEncodeAddrKeepsEveryIPv4Octet guards the encoding of an IPv4 address
// inside the cookie. The address is stored as the IPv4 mapped IPv6 form
// ::ffff:a.b.c.d, so the four octets live in the last four bytes. An
// encoding that drops the two middle octets would let a cookie minted for
// one address be used from another one.
func TestEncodeAddrKeepsEveryIPv4Octet(t *testing.T) {
	first := encodeAddr(&net.UDPAddr{IP: net.ParseIP("10.0.0.2"), Port: 4242})
	second := encodeAddr(&net.UDPAddr{IP: net.ParseIP("10.5.7.2"), Port: 4242})
	if bytes.Equal(first, second) {
		t.Fatal("two different addresses were encoded the same way")
	}

	// The mapped prefix has to be in place.
	if first[10] != 0xff || first[11] != 0xff {
		t.Errorf("the IPv4 mapped prefix is missing: %x", first[:16])
	}
	if !bytes.Equal(first[12:16], []byte{10, 0, 0, 2}) {
		t.Errorf("the address bytes are %x, want 0a000002", first[12:16])
	}
}

// TestCookieIsBoundToTheFullAddress checks that a cookie issued for one
// address is refused from an address that differs only in the two octets
// that a broken encoding would drop.
//
// The peer key is the real one of the client, so the address check is the
// only thing that can refuse the handshake.
func TestCookieIsBoundToTheFullAddress(t *testing.T) {
	clientPriv, clientPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	serverPriv, serverPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	responder, err := NewResponder(serverPriv, []Peer{{ID: "peer1", Public: clientPub}})
	if err != nil {
		t.Fatal(err)
	}
	initiator, err := NewInitiator(clientPriv, serverPub, 13)
	if err != nil {
		t.Fatal(err)
	}
	src := &net.UDPAddr{IP: net.ParseIP("10.0.0.2"), Port: 4242}
	cookieResp, err := responder.HandleCookieRequest(src, initiator.CookieRequestBytes(), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := initiator.ApplyCookieResponse(cookieResp); err != nil {
		t.Fatal(err)
	}
	initBytes, err := initiator.HandshakeInitBytes(now)
	if err != nil {
		t.Fatal(err)
	}

	// The same handshake from the address it was issued for is accepted,
	// so the address is the only possible reason for a refusal.
	if _, _, err := responder.HandleHandshakeInit(src, initBytes, now); err != nil {
		t.Fatalf("the handshake from the right address was refused: %v", err)
	}

	other := &net.UDPAddr{IP: net.ParseIP("10.5.7.2"), Port: 4242}
	if _, _, err := responder.HandleHandshakeInit(other, initBytes, now); err == nil {
		t.Fatal("a cookie was accepted from an address that differs in the dropped octets")
	}
}

// TestForgedCounterDoesNotMoveTheReplayWindow makes sure that a packet that
// cannot be authenticated cannot shift the replay window. A window that
// moves before the authentication would let a forged packet with a huge
// counter lock out every legitimate packet of the session.
func TestForgedCounterDoesNotMoveTheReplayWindow(t *testing.T) {
	_, _, serverSession, clientSession, _, _ := establish(t)

	// A forged packet with a counter far in the future.
	if _, err := serverSession.Decrypt(1<<40, []byte("forged")); err == nil {
		t.Fatal("a forged packet was accepted")
	}

	// The legitimate packet with a low counter must still go through.
	datagram, err := clientSession.Encrypt([]byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	pkt, err := DecodeData(datagram)
	if err != nil {
		t.Fatal(err)
	}
	out, err := serverSession.Decrypt(pkt.Counter, pkt.Ciphertext)
	if err != nil {
		t.Fatalf("the legitimate packet was rejected after a forged one: %v", err)
	}
	if string(out) != "payload" {
		t.Errorf("payload = %q, want %q", out, "payload")
	}
}

// TestEmptyPayloadDoesNotBreakTheSession covers a peer that seals an empty
// plaintext. The server used to read the first byte of the payload before
// it checked the length, which killed the whole process.
func TestEmptyPayloadDoesNotBreakTheSession(t *testing.T) {
	_, _, serverSession, clientSession, _, _ := establish(t)

	datagram, err := clientSession.Encrypt(nil)
	if err != nil {
		t.Fatal(err)
	}
	pkt, err := DecodeData(datagram)
	if err != nil {
		t.Fatal(err)
	}
	out, err := serverSession.Decrypt(pkt.Counter, pkt.Ciphertext)
	if err != nil {
		t.Fatalf("the empty payload was refused: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("payload = %x, want an empty payload", out)
	}

	// The session still works afterwards.
	datagram, err = clientSession.Encrypt([]byte("still here"))
	if err != nil {
		t.Fatal(err)
	}
	pkt, err = DecodeData(datagram)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := serverSession.Decrypt(pkt.Counter, pkt.Ciphertext); err != nil {
		t.Fatalf("the session broke after an empty payload: %v", err)
	}
}

// TestHandshakeReplayIsRejected makes sure that a captured handshake cannot
// be used a second time to replace the session of the victim.
func TestHandshakeReplayIsRejected(t *testing.T) {
	responder, initiator, _, _, src, now := establish(t)

	// A second, fresh handshake from the same peer is accepted: the client
	// rekeys every two minutes.
	if err := initiator.ApplyCookieResponse(mustCookieResponse(t, responder, initiator, src, now)); err != nil {
		t.Fatal(err)
	}
	fresh, err := initiator.HandshakeInitBytes(now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := responder.HandleHandshakeInit(src, fresh, now.Add(time.Second)); err != nil {
		t.Fatalf("a fresh handshake was refused: %v", err)
	}

	// The old handshake, captured earlier, must not be usable again.
	captured, err := initiator.HandshakeInitBytes(now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := responder.HandleHandshakeInit(src, captured, now.Add(2*time.Second)); err == nil {
		t.Fatal("a replayed handshake was accepted")
	}
}

// TestHandshakeWithAnOutOfSyncClockIsRejected covers the clock check of
// the server.
func TestHandshakeWithAnOutOfSyncClockIsRejected(t *testing.T) {
	clientPriv, _, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	serverPriv, serverPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	responder, err := NewResponder(serverPriv, []Peer{{ID: "peer1", Public: mustRandomPub(t)}})
	if err != nil {
		t.Fatal(err)
	}
	initiator, err := NewInitiator(clientPriv, serverPub, 29)
	if err != nil {
		t.Fatal(err)
	}
	src := testAddr()
	cookieResp, err := responder.HandleCookieRequest(src, initiator.CookieRequestBytes(), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := initiator.ApplyCookieResponse(cookieResp); err != nil {
		t.Fatal(err)
	}
	// The client claims a timestamp far away from the clock of the server.
	initBytes, err := initiator.HandshakeInitBytes(now.Add(-2 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := responder.HandleHandshakeInit(src, initBytes, now); err == nil {
		t.Fatal("a handshake with an out of sync clock was accepted")
	}
}

// mustCookieResponse asks the responder for a cookie for the given source.
func mustCookieResponse(t *testing.T, r *Responder, i *Initiator, src *net.UDPAddr, now time.Time) []byte {
	t.Helper()
	response, err := r.HandleCookieRequest(src, i.CookieRequestBytes(), now)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
