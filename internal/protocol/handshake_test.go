package protocol

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/w1nchi/w1nwire/internal/crypto"
)

func testAddr() *net.UDPAddr {
	return &net.UDPAddr{IP: net.ParseIP("203.0.113.5"), Port: 4242}
}

func TestHandshakeEstablishesSession(t *testing.T) {
	clientPriv, clientPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	serverPriv, serverPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	_ = serverPriv

	peers := []Peer{{ID: "peer1", Public: clientPub, AssignedIP: [4]byte{10, 66, 0, 2}}}
	_ = peers

	// The responder needs the real server private key, so the helper above is
	// replaced by a direct construction in this test.
	now := time.Now()
	responder, err := NewResponder(serverPriv, peers)
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
		t.Fatalf("cookie request: %v", err)
	}
	if err := initiator.ApplyCookieResponse(cookieResp); err != nil {
		t.Fatalf("apply cookie: %v", err)
	}
	initBytes, err := initiator.HandshakeInitBytes(now)
	if err != nil {
		t.Fatalf("handshake init: %v", err)
	}
	handshakeResp, serverSession, err := responder.HandleHandshakeInit(src, initBytes, now)
	if err != nil {
		t.Fatalf("handle handshake init: %v", err)
	}
	clientSession, err := initiator.ApplyHandshakeResponse(handshakeResp, now)
	if err != nil {
		t.Fatalf("apply handshake response: %v", err)
	}

	if serverSession.PeerID != "peer1" {
		t.Errorf("peer id = %q, want %q", serverSession.PeerID, "peer1")
	}
	if clientSession.AssignedIP != [4]byte{10, 66, 0, 2} {
		t.Errorf("assigned ip = %v, want 10.66.0.2", clientSession.AssignedIP)
	}
	if clientSession.RemoteIndex != serverSession.LocalIndex {
		t.Errorf("client remote index %d != server local index %d", clientSession.RemoteIndex, serverSession.LocalIndex)
	}
	if serverSession.RemoteIndex != clientSession.LocalIndex {
		t.Errorf("server remote index %d != client local index %d", serverSession.RemoteIndex, clientSession.LocalIndex)
	}
}

func TestDataRoundTrip(t *testing.T) {
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

	cookieResp, _ := responder.HandleCookieRequest(src, initiator.CookieRequestBytes(), now)
	if err := initiator.ApplyCookieResponse(cookieResp); err != nil {
		t.Fatal(err)
	}
	initBytes, _ := initiator.HandshakeInitBytes(now)
	handshakeResp, serverSession, err := responder.HandleHandshakeInit(src, initBytes, now)
	if err != nil {
		t.Fatal(err)
	}
	clientSession, err := initiator.ApplyHandshakeResponse(handshakeResp, now)
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte("hello w1nwire")
	datagram, err := clientSession.Encrypt(payload)
	if err != nil {
		t.Fatal(err)
	}
	pkt, err := DecodeData(datagram)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := serverSession.Decrypt(pkt.Counter, pkt.Ciphertext)
	if err != nil {
		t.Fatalf("server decrypt: %v", err)
	}
	if !bytes.Equal(plain, payload) {
		t.Errorf("payload = %q, want %q", plain, payload)
	}

	// The reverse direction must work as well.
	back, err := serverSession.Encrypt([]byte("pong"))
	if err != nil {
		t.Fatal(err)
	}
	pkt, err = DecodeData(back)
	if err != nil {
		t.Fatal(err)
	}
	plain, err = clientSession.Decrypt(pkt.Counter, pkt.Ciphertext)
	if err != nil {
		t.Fatalf("client decrypt: %v", err)
	}
	if string(plain) != "pong" {
		t.Errorf("payload = %q, want %q", plain, "pong")
	}
}

func TestReplayIsRejected(t *testing.T) {
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
	initiator, err := NewInitiator(clientPriv, serverPub, 7)
	if err != nil {
		t.Fatal(err)
	}
	src := testAddr()
	cookieResp, _ := responder.HandleCookieRequest(src, initiator.CookieRequestBytes(), now)
	_ = initiator.ApplyCookieResponse(cookieResp)
	initBytes, _ := initiator.HandshakeInitBytes(now)
	handshakeResp, serverSession, err := responder.HandleHandshakeInit(src, initBytes, now)
	if err != nil {
		t.Fatal(err)
	}
	clientSession, err := initiator.ApplyHandshakeResponse(handshakeResp, now)
	if err != nil {
		t.Fatal(err)
	}

	datagram, _ := clientSession.Encrypt([]byte("payload"))
	pkt, _ := DecodeData(datagram)
	if _, err := serverSession.Decrypt(pkt.Counter, pkt.Ciphertext); err != nil {
		t.Fatalf("first delivery must succeed: %v", err)
	}
	if _, err := serverSession.Decrypt(pkt.Counter, pkt.Ciphertext); err == nil {
		t.Fatal("replayed packet was accepted")
	}
}

func TestUnknownPeerIsRejected(t *testing.T) {
	clientPriv, _, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	serverPriv, serverPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	responder, err := NewResponder(serverPriv, []Peer{{ID: "other", Public: mustRandomPub(t)}})
	if err != nil {
		t.Fatal(err)
	}
	initiator, err := NewInitiator(clientPriv, serverPub, 9)
	if err != nil {
		t.Fatal(err)
	}
	src := testAddr()
	cookieResp, _ := responder.HandleCookieRequest(src, initiator.CookieRequestBytes(), now)
	_ = initiator.ApplyCookieResponse(cookieResp)
	initBytes, _ := initiator.HandshakeInitBytes(now)
	if _, _, err := responder.HandleHandshakeInit(src, initBytes, now); err == nil {
		t.Fatal("handshake with an unknown peer was accepted")
	}
}

func TestClientRejectsImpostorServer(t *testing.T) {
	clientPriv, _, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	_, realServerPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	impostorPriv, _, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	responder, err := NewResponder(impostorPriv, []Peer{{ID: "peer1", Public: mustRandomPub(t)}})
	if err != nil {
		t.Fatal(err)
	}
	initiator, err := NewInitiator(clientPriv, realServerPub, 11)
	if err != nil {
		t.Fatal(err)
	}
	src := testAddr()

	// The impostor does not know the private key of the server the client
	// expects, therefore it cannot even produce a valid mac1 and the
	// handshake dies before any key material is exchanged.
	if _, err := responder.HandleCookieRequest(src, initiator.CookieRequestBytes(), now); err == nil {
		t.Fatal("impostor server accepted a cookie request")
	}
}

func TestClientRejectsTamperedResponse(t *testing.T) {
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
	initiator, err := NewInitiator(clientPriv, serverPub, 21)
	if err != nil {
		t.Fatal(err)
	}
	src := testAddr()
	cookieResp, _ := responder.HandleCookieRequest(src, initiator.CookieRequestBytes(), now)
	if err := initiator.ApplyCookieResponse(cookieResp); err != nil {
		t.Fatal(err)
	}
	initBytes, _ := initiator.HandshakeInitBytes(now)
	handshakeResp, _, err := responder.HandleHandshakeInit(src, initBytes, now)
	if err != nil {
		t.Fatal(err)
	}

	tampered := append([]byte(nil), handshakeResp...)
	tampered[len(tampered)-1] ^= 0xff
	if _, err := initiator.ApplyHandshakeResponse(tampered, now); err == nil {
		t.Fatal("client accepted a tampered handshake response")
	}
}

func TestCookieIsBoundToAddress(t *testing.T) {
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
	initiator, err := NewInitiator(clientPriv, serverPub, 13)
	if err != nil {
		t.Fatal(err)
	}
	src := testAddr()
	cookieResp, _ := responder.HandleCookieRequest(src, initiator.CookieRequestBytes(), now)
	_ = initiator.ApplyCookieResponse(cookieResp)
	initBytes, _ := initiator.HandshakeInitBytes(now)

	other := &net.UDPAddr{IP: net.ParseIP("198.51.100.9"), Port: 5555}
	if _, _, err := responder.HandleHandshakeInit(other, initBytes, now); err == nil {
		t.Fatal("cookie was accepted from a different source address")
	}
}

func TestExpiredCookieIsRejected(t *testing.T) {
	clientPriv, _, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	serverPriv, serverPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	responder, err := NewResponder(serverPriv, []Peer{{ID: "peer1", Public: mustRandomPub(t)}})
	if err != nil {
		t.Fatal(err)
	}
	responder.SetCookieTTL(-time.Minute)

	initiator, err := NewInitiator(clientPriv, serverPub, 17)
	if err != nil {
		t.Fatal(err)
	}
	src := testAddr()
	now := time.Now()
	cookieResp, _ := responder.HandleCookieRequest(src, initiator.CookieRequestBytes(), now)
	_ = initiator.ApplyCookieResponse(cookieResp)
	initBytes, _ := initiator.HandshakeInitBytes(now)
	if _, _, err := responder.HandleHandshakeInit(src, initBytes, now.Add(2*time.Minute)); err == nil {
		t.Fatal("expired cookie was accepted")
	}
}

func mustRandomPub(t *testing.T) crypto.PublicKey {
	t.Helper()
	_, pub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return pub
}
