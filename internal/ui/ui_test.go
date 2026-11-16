package ui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/w1nchi/w1nwire/internal/app"
	"github.com/w1nchi/w1nwire/internal/crypto"
	"github.com/w1nchi/w1nwire/internal/invite"
)

// startServer creates an application and an interface server on a random
// port, and returns the base URL and a client that already holds the cookie.
func startServer(t *testing.T) (base string, client *http.Client) {
	t.Helper()

	a, err := app.New(app.Options{Version: "test", DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(a.Shutdown)

	s, err := New(a)
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	link, err := s.Start()
	if err != nil {
		t.Fatalf("ui.Start: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client = &http.Client{Jar: jar, Timeout: 60 * time.Second}

	// The token swaps itself for a cookie and redirects.
	resp, err := client.Get(link)
	if err != nil {
		t.Fatalf("open the interface: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the interface returned %d", resp.StatusCode)
	}

	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	return "http://" + u.Host, client
}

func getState(t *testing.T, base string, client *http.Client) app.StateView {
	t.Helper()
	resp, err := client.Get(base + "/api/state")
	if err != nil {
		t.Fatalf("GET /api/state: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/state returned %d", resp.StatusCode)
	}
	var state app.StateView
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatalf("decode the state: %v", err)
	}
	return state
}

func postJSON(t *testing.T, base string, client *http.Client, path string, body any) map[string]any {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Post(base+path, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("POST %s: decode: %v", path, err)
	}
	return out
}

func TestAPIRootServesTheInterface(t *testing.T) {
	base, client := startServer(t)
	resp, err := client.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / returned %d", resp.StatusCode)
	}
	if !strings.Contains(strings.ToLower(string(body)), "<html") {
		t.Fatalf("GET / did not return HTML: %q", truncate(string(body)))
	}
	if got := resp.Header.Get("Content-Security-Policy"); got == "" {
		t.Fatal("the interface is served without a CSP")
	}
}

func TestAPIRequiresTheToken(t *testing.T) {
	base, _ := startServer(t)
	// A fresh client without the cookie.
	resp, err := http.Get(base + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("an unauthenticated request returned %d, want 401", resp.StatusCode)
	}
}

func TestStateShape(t *testing.T) {
	base, client := startServer(t)
	state := getState(t, base, client)
	if state.Version != "test" {
		t.Fatalf("version = %q", state.Version)
	}
	if state.Status.State == "" {
		t.Fatal("the status has no state")
	}
	if state.Settings.Language == "" {
		t.Fatal("the settings have no language")
	}
	if state.Logs == nil {
		t.Fatal("the logs must be an array, not null")
	}
	if state.Profiles == nil {
		t.Fatal("the profiles must be an array, not null")
	}
}

func TestImportProfileThroughTheAPI(t *testing.T) {
	base, client := startServer(t)

	clientPriv, _, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	_, serverPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	code, err := invite.New("oracle", "198.51.100.9:51820", serverPub.String(), clientPriv.String(), "10.66.0.2", "10.66.0.1").Encode()
	if err != nil {
		t.Fatal(err)
	}

	out := postJSON(t, base, client, "/api/profiles/import", map[string]string{"code": code})
	if out["ok"] != true {
		t.Fatalf("import failed: %v", out)
	}
	profile, ok := out["profile"].(map[string]any)
	if !ok {
		t.Fatalf("no profile in the response: %v", out)
	}
	if profile["name"] != "oracle" {
		t.Fatalf("unexpected profile: %v", profile)
	}
	if _, leaked := profile["privateKey"]; leaked {
		t.Fatal("the private key leaked into the API")
	}

	state := getState(t, base, client)
	if len(state.Profiles) != 1 {
		t.Fatalf("profiles = %d", len(state.Profiles))
	}

	// A garbage code is refused with a readable error.
	bad := postJSON(t, base, client, "/api/profiles/import", map[string]string{"code": "nope"})
	if bad["ok"] != false || bad["error"] == "" {
		t.Fatalf("a garbage code was accepted: %v", bad)
	}
}

func TestSelfTestThroughTheAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("the self test takes a few seconds")
	}
	base, client := startServer(t)
	out := postJSON(t, base, client, "/api/selftest", map[string]string{})
	if out["ok"] != true {
		t.Fatalf("the self test failed: %v", out)
	}
}

func TestSettingsThroughTheAPI(t *testing.T) {
	base, client := startServer(t)
	out := postJSON(t, base, client, "/api/settings", map[string]any{"language": "en", "mtu": 1380})
	if out["ok"] != true {
		t.Fatalf("the settings update failed: %v", out)
	}
	state := getState(t, base, client)
	if state.Settings.Language != "en" || state.Settings.MTU != 1380 {
		t.Fatalf("the settings were not applied: %+v", state.Settings)
	}
}

func truncate(s string) string {
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}
