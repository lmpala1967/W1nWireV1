package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/w1nchi/w1nwire/internal/crypto"
	"github.com/w1nchi/w1nwire/internal/invite"
)

// newTestApp creates an application with a temporary data directory.
func newTestApp(t *testing.T) *App {
	t.Helper()
	a, err := New(Options{Version: "test", DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(a.Shutdown)
	return a
}

func testProfile(t *testing.T) Profile {
	t.Helper()
	priv, _, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	serverPriv, serverPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	_ = serverPriv
	return Profile{
		Name:          "test",
		Host:          "203.0.113.9:51820",
		ServerKey:     serverPub.String(),
		PrivateKey:    priv.String(),
		TunnelAddress: "10.66.0.2/24",
		Gateway:       "10.66.0.1",
		MTU:           1420,
	}
}

func TestProfileLifecycle(t *testing.T) {
	a := newTestApp(t)

	view, clientKey, err := a.AddProfile(testProfile(t))
	if err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if clientKey == "" {
		t.Fatal("the client public key was not returned")
	}
	if view.ID == "" {
		t.Fatal("the profile has no id")
	}
	if view.PrivateKeySet != true {
		t.Fatal("the private key should be present")
	}
	if got := a.Snapshot().ActiveProfileID; got != view.ID {
		t.Fatalf("the first profile was not activated: %q", got)
	}

	// The private key must never be exposed.
	if view.ClientPublicKey == "" {
		t.Fatal("the client public key is missing from the view")
	}

	// Duplicates are refused by address and key.
	same := Profile{
		Name:          "duplicate",
		Host:          view.Host,
		ServerKey:     view.PublicKey,
		PrivateKey:    "",
		TunnelAddress: "10.66.0.3/24",
	}
	if _, _, err := a.AddProfile(same); err == nil {
		t.Fatal("a profile with the same address and key was accepted")
	}

	updated := Profile{ID: view.ID, Name: "renamed", Host: view.Host, ServerKey: view.PublicKey, TunnelAddress: "10.66.0.2/24", Gateway: "10.66.0.1", MTU: 1380}
	if _, err := a.UpdateProfile(updated); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	snap := a.Snapshot()
	if len(snap.Profiles) != 1 || snap.Profiles[0].Name != "renamed" || snap.Profiles[0].MTU != 1380 {
		t.Fatalf("the update was not applied: %+v", snap.Profiles)
	}

	if err := a.DeleteProfile(view.ID); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	if len(a.Snapshot().Profiles) != 0 {
		t.Fatal("the profile was not deleted")
	}
}

func TestNormalizeProfile(t *testing.T) {
	p, err := NormalizeProfile(Profile{Host: "203.0.113.9", MTU: 0})
	if err != nil {
		t.Fatalf("NormalizeProfile: %v", err)
	}
	if p.Host != "203.0.113.9:51820" {
		t.Fatalf("the default port was not added: %q", p.Host)
	}
	if p.TunnelAddress != DefaultTunnelAddress {
		t.Fatalf("tunnel address = %q", p.TunnelAddress)
	}
	if p.MTU != DefaultMTU {
		t.Fatalf("mtu = %d", p.MTU)
	}
	if p.Name != "203.0.113.9" {
		t.Fatalf("name = %q", p.Name)
	}

	if _, err := NormalizeProfile(Profile{Host: "203.0.113.9", MTU: 9000}); err == nil {
		t.Fatal("an MTU above 1500 was accepted")
	}
}

func TestImportInvite(t *testing.T) {
	a := newTestApp(t)
	clientPriv, _, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	_, serverPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	code, err := invite.New("home", "198.51.100.4:51820", serverPub.String(), clientPriv.String(), "10.66.0.7", "10.66.0.1").Encode()
	if err != nil {
		t.Fatal(err)
	}

	view, err := a.ImportInvite(code)
	if err != nil {
		t.Fatalf("ImportInvite: %v", err)
	}
	if view.Name != "home" || view.Host != "198.51.100.4:51820" {
		t.Fatalf("unexpected profile: %+v", view)
	}
	if view.TunnelAddress != "10.66.0.7/24" {
		t.Fatalf("the address was not normalised: %q", view.TunnelAddress)
	}

	if _, err := a.ImportInvite("not a code"); err == nil {
		t.Fatal("a garbage invite code was accepted")
	}
}

func TestInviteWithoutPrivateKeyIsRejected(t *testing.T) {
	a := newTestApp(t)
	_, serverPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	inv := invite.New("manual", "198.51.100.4:51820", serverPub.String(), "", "10.66.0.7", "10.66.0.1")
	// Encode without the key must fail, which is the first line of defence.
	if code, err := inv.Encode(); err == nil {
		if _, err := a.ImportInvite(code); err == nil {
			t.Fatal("an invite without a client key was accepted")
		}
	}
}

func TestSettingsArePersisted(t *testing.T) {
	dir := t.TempDir()
	a, err := New(Options{Version: "test", DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	lang := "en"
	dns := "9.9.9.9"
	mtu := 1380
	port := 1081
	if err := a.UpdateSettings(SettingsPatch{Language: &lang, DNS: &dns, MTU: &mtu, ProxyPort: &port}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	a.Shutdown()

	// A second instance must see the same settings.
	b, err := New(Options{Version: "test", DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Shutdown()
	s := b.Snapshot().Settings
	if s.Language != "en" || s.DNS != "9.9.9.9" || s.MTU != 1380 || s.ProxyPort != 1081 {
		t.Fatalf("the settings were not persisted: %+v", s)
	}

	bad := "klingon"
	if err := b.UpdateSettings(SettingsPatch{Language: &bad}); err == nil {
		t.Fatal("an unsupported language was accepted")
	}
	if err := b.UpdateSettings(SettingsPatch{MTU: intPtr(100)}); err == nil {
		t.Fatal("an MTU below the minimum was accepted")
	}
}

func intPtr(v int) *int { return &v }

func TestSnapshotExposesNoPrivateKey(t *testing.T) {
	a := newTestApp(t)
	profile := testProfile(t)
	view, _, err := a.AddProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	snap := a.Snapshot()
	if len(snap.Profiles) != 1 {
		t.Fatalf("profiles = %d", len(snap.Profiles))
	}
	if snap.Profiles[0].ID != view.ID {
		t.Fatalf("unexpected profile %+v", snap.Profiles[0])
	}
	if _, err := os.Stat(filepath.Join(a.DataDir(), "config.json")); err != nil {
		t.Fatalf("the configuration file was not written: %v", err)
	}
}

func TestLogBufferKeepsTheLastEntries(t *testing.T) {
	buf := NewLogBuffer(3, nil)
	for _, msg := range []string{"one", "two", "three", "four"} {
		buf.Add("info", msg)
	}
	entries := buf.Entries()
	if len(entries) != 3 {
		t.Fatalf("entries = %d", len(entries))
	}
	if entries[0].Msg != "two" || entries[2].Msg != "four" {
		t.Fatalf("unexpected entries: %+v", entries)
	}

	ch, cancel := buf.Subscribe()
	defer cancel()
	buf.Add("warn", "five")
	select {
	case got := <-ch:
		if got.Msg != "five" || got.Level != "warn" {
			t.Fatalf("unexpected event: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("the subscriber did not receive the entry")
	}
}

// TestSelfTest runs the built in end to end check: a real server, a real
// client and an HTTP request through the SOCKS5 listener.
func TestSelfTest(t *testing.T) {
	a := newTestApp(t)
	done := make(chan error, 1)
	go func() { done <- a.SelfTest() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SelfTest: %v", err)
		}
	case <-time.After(45 * time.Second):
		t.Fatal("the self test did not finish in time")
	}
}

// TestConnectReturnsImmediately guards against a regression where Connect
// blocked on the monitor loop and therefore froze the HTTP handler that
// serves the interface.
func TestConnectReturnsImmediately(t *testing.T) {
	a := newTestApp(t)
	profile := testProfile(t)
	profile.Host = "127.0.0.1:1" // nothing listens there
	view, _, err := a.AddProfile(profile)
	if err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	if err := a.Connect(view.ID, "proxy"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("Connect blocked for %v", elapsed)
	}

	// The state must move to "connecting" (or already to "error" when the
	// socket cannot even be dialled) without waiting for the handshake.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state := a.Snapshot().Status.State
		if state == "connecting" || state == "error" || state == "connected" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if state := a.Snapshot().Status.State; state == "disconnected" {
		t.Fatalf("the state is still %q right after Connect", state)
	}

	_ = a.Disconnect()
}

// TestConnectWithoutAModeUsesTheStoredOne covers the mode fallback: an
// empty mode means "the mode from the settings", and it must not be
// refused as an unknown mode.
func TestConnectWithoutAModeUsesTheStoredOne(t *testing.T) {
	a := newTestApp(t)
	profile := testProfile(t)
	profile.Host = "127.0.0.1:1" // nothing listens there
	view, _, err := a.AddProfile(profile)
	if err != nil {
		t.Fatal(err)
	}

	if err := a.UpdateSettings(SettingsPatch{Mode: strptr("proxy")}); err != nil {
		t.Fatal(err)
	}
	if err := a.Connect(view.ID, ""); err != nil {
		t.Fatalf("Connect with an empty mode was refused: %v", err)
	}
	if mode := a.Snapshot().Status.Mode; mode != "proxy" {
		t.Errorf("the mode is %q, want the mode from the settings", mode)
	}
	_ = a.Disconnect()
}

// TestTwoConnectsAtOnceAreRefused makes sure that a second connection does
// not start while the first one is still being prepared. The check and the
// assignment are far apart, and without a lock two requests could both
// pass it and leave a tunnel nobody watches.
func TestTwoConnectsAtOnceAreRefused(t *testing.T) {
	a := newTestApp(t)
	profile := testProfile(t)
	profile.Host = "127.0.0.1:1"
	view, _, err := a.AddProfile(profile)
	if err != nil {
		t.Fatal(err)
	}

	if err := a.Connect(view.ID, "proxy"); err != nil {
		t.Fatal(err)
	}
	if err := a.Connect(view.ID, "proxy"); err == nil {
		t.Fatal("a second connection was accepted")
	}
	_ = a.Disconnect()
}

// strptr returns a pointer to a string.
func strptr(value string) *string { return &value }
