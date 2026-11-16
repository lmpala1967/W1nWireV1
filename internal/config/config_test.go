package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadServerRejectsBrokenFiles(t *testing.T) {
	if _, err := LoadServer(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("a missing file was accepted")
	}
	if _, err := LoadServer(writeFile(t, "broken.json", "{not json")); err == nil {
		t.Error("a broken file was accepted")
	}
	if _, err := LoadServer(writeFile(t, "empty.json", "{}")); err == nil {
		t.Error("an empty configuration was accepted")
	}
}

func TestLoadServerAcceptsAValidFile(t *testing.T) {
	content := `{
      "listen": "0.0.0.0:51820",
      "server_private_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
      "tun": {"enabled": false},
      "peers": [
        {"id": "phone", "public_key": "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB", "tunnel_address": "10.66.0.2"}
      ]
    }`
	cfg, err := LoadServer(writeFile(t, "server.json", content))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Peers) != 1 || cfg.Peers[0].ID != "phone" {
		t.Fatalf("unexpected peers: %+v", cfg.Peers)
	}
}

func TestLoadServerRejectsUnknownFields(t *testing.T) {
	content := `{"listen": "0.0.0.0:51820", "server_private_key": "x", "peers": [], "typo": true}`
	if _, err := LoadServer(writeFile(t, "server.json", content)); err == nil {
		t.Error("a configuration with an unknown field was accepted")
	}
}

func TestLoadClientValidatesTheMode(t *testing.T) {
	content := `{
      "server": {"address": "1.2.3.4:51820", "public_key": "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"},
      "private_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
      "tun": {"enabled": false},
      "socks5": {"enabled": false}
    }`
	if _, err := LoadClient(writeFile(t, "client.json", content)); err == nil {
		t.Error("a client without any mode was accepted")
	}
}

func TestLoadClientRejectsAnInvalidEndpoint(t *testing.T) {
	content := `{
      "server": {"address": "no-port", "public_key": "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"},
      "private_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
      "socks5": {"enabled": true, "listen": "127.0.0.1:1080"}
    }`
	if _, err := LoadClient(writeFile(t, "client.json", content)); err == nil {
		t.Error("an endpoint without a port was accepted")
	}
}
