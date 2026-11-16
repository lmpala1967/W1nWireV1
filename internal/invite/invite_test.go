package invite

import (
	"strings"
	"testing"

	"github.com/w1nchi/w1nwire/internal/crypto"
)

func testInvite(t *testing.T) *Invite {
	t.Helper()
	priv, _, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the client key: %v", err)
	}
	serverPriv, serverPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the server key: %v", err)
	}
	_ = priv
	return New("desktop", "203.0.113.7:51820", serverPub.String(), serverPriv.String(), "10.66.0.2", "10.66.0.1")
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	in := testInvite(t)
	code, err := in.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.HasPrefix(code, Prefix) {
		t.Fatalf("the code does not carry the prefix: %q", code)
	}
	if strings.ContainsAny(code, "+/=") {
		t.Fatalf("the code is not URL safe: %q", code)
	}

	out, err := Decode(code)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Host != in.Host || out.ServerKey != in.ServerKey || out.PrivateKey != in.PrivateKey {
		t.Fatalf("the payload changed:\n got %+v\nwant %+v", out, in)
	}
	if out.Address != "10.66.0.2" || out.Gateway != "10.66.0.1" {
		t.Fatalf("the addresses changed: %+v", out)
	}
}

func TestDecodeAcceptsLenientForms(t *testing.T) {
	in := testInvite(t)
	code, err := in.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	for name, input := range map[string]string{
		"leading and trailing spaces": "  " + code + "\n",
		"quoted":                      `"` + code + `"`,
		"scheme":                      "w1nwire://" + code,
		"scheme without prefix":       "w1nwire://" + strings.TrimPrefix(code, Prefix),
		"wrapped over two lines":      code[:20] + "\n" + code[20:],
	} {
		t.Run(name, func(t *testing.T) {
			out, err := Decode(input)
			if err != nil {
				t.Fatalf("decode %q: %v", input, err)
			}
			if out.Host != in.Host {
				t.Fatalf("host = %q, want %q", out.Host, in.Host)
			}
		})
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	for _, input := range []string{"", "   ", "ww1.", "ww1.not-base64!!", "hello world"} {
		if _, err := Decode(input); err == nil {
			t.Fatalf("decode %q: expected an error", input)
		}
	}
}

func TestPlainForm(t *testing.T) {
	serverPriv, serverPub, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	_ = serverPriv
	inv, err := Decode("203.0.113.7:51820|" + serverPub.String() + "|home")
	if err != nil {
		t.Fatalf("decode the plain form: %v", err)
	}
	if inv.Host != "203.0.113.7:51820" {
		t.Fatalf("host = %q", inv.Host)
	}
	if inv.Name != "home" {
		t.Fatalf("name = %q", inv.Name)
	}
	if inv.PrivateKey != "" {
		t.Fatalf("the plain form must not invent a private key")
	}
}

func TestValidateRejectsIncompleteInvites(t *testing.T) {
	base := testInvite(t)
	cases := map[string]func(*Invite){
		"no host":   func(i *Invite) { i.Host = "" },
		"bad host":  func(i *Invite) { i.Host = "1.2.3.4" },
		"no key":    func(i *Invite) { i.ServerKey = "" },
		"no client": func(i *Invite) { i.PrivateKey = "" },
		"version":   func(i *Invite) { i.Version = 99 },
		"address":   func(i *Invite) { i.Address = "not-an-ip" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			inv := *base
			mutate(&inv)
			if err := inv.Validate(); err == nil {
				t.Fatalf("expected an error")
			}
		})
	}
}

func TestParseAddress(t *testing.T) {
	cases := []struct {
		in   string
		host string
		port int
		bad  bool
	}{
		{"203.0.113.7:51820", "203.0.113.7", 51820, false},
		{"203.0.113.7", "203.0.113.7", 51820, false},
		{"vpn.example.com", "vpn.example.com", 51820, false},
		{"vpn.example.com:443", "vpn.example.com", 443, false},
		{"", "", 0, true},
		{"address:port", "", 0, true},
		{"1.2.3.4:0", "", 0, true},
	}
	for _, c := range cases {
		host, port, err := ParseAddress(c.in)
		if c.bad {
			if err == nil {
				t.Fatalf("ParseAddress(%q): expected an error", c.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseAddress(%q): %v", c.in, err)
		}
		if host != c.host || port != c.port {
			t.Fatalf("ParseAddress(%q) = %q,%d want %q,%d", c.in, host, port, c.host, c.port)
		}
	}
}
