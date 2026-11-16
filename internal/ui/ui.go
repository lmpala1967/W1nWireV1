// Package ui serves the interface of the desktop client and the JSON API
// that drives it.
//
// The interface is a small single page application embedded into the binary.
// It is served on a loopback address with a random port and a random token:
// the token is passed in the URL that the window is opened with, and stored
// as an HttpOnly cookie, so that no other local process can talk to the API
// just by guessing the port.
package ui

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/w1nchi/w1nwire/internal/app"
)

//go:embed assets
var assets embed.FS

// cookieName is the name of the session cookie.
const cookieName = "ww_token"

// Server is the local HTTP server of the interface.
type Server struct {
	app   *app.App
	token string
	ln    net.Listener
	srv   *http.Server
	mux   *http.ServeMux
}

// New creates the server and prepares the routes.
func New(a *app.App) (*Server, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	s := &Server{app: a, token: token, mux: http.NewServeMux()}
	s.routes()
	return s, nil
}

// Start binds a random loopback port and serves in the background. It returns
// the URL the interface must be opened with.
func (s *Server) Start() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("ui: listen: %w", err)
	}
	s.ln = ln
	s.srv = &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := s.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.app.Logger().Error("the interface server stopped", "error", err)
		}
	}()
	return fmt.Sprintf("http://%s/?t=%s", ln.Addr().String(), s.token), nil
}

// Addr returns the address the server listens on.
func (s *Server) Addr() string {
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// Close stops the server.
func (s *Server) Close() error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Close()
}

// routes registers the handlers.
func (s *Server) routes() {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(sub))

	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(w, r) {
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			data, err := assets.ReadFile("assets/index.html")
			if err != nil {
				http.Error(w, "the interface is missing", http.StatusInternalServerError)
				return
			}
			w.Write(data)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})

	s.mux.HandleFunc("/api/state", s.handleState)
	s.mux.HandleFunc("/api/events", s.handleEvents)
	s.mux.HandleFunc("/api/health", s.handleHealth)

	s.mux.HandleFunc("/api/connect", s.post(s.handleConnect))
	s.mux.HandleFunc("/api/disconnect", s.post(s.handleDisconnect))
	s.mux.HandleFunc("/api/profiles/add", s.post(s.handleProfileAdd))
	s.mux.HandleFunc("/api/profiles/update", s.post(s.handleProfileUpdate))
	s.mux.HandleFunc("/api/profiles/delete", s.post(s.handleProfileDelete))
	s.mux.HandleFunc("/api/profiles/activate", s.post(s.handleProfileActivate))
	s.mux.HandleFunc("/api/profiles/import", s.post(s.handleProfileImport))
	s.mux.HandleFunc("/api/profiles/test", s.post(s.handleProfileTest))
	s.mux.HandleFunc("/api/settings", s.post(s.handleSettings))
	s.mux.HandleFunc("/api/system-proxy", s.post(s.handleSystemProxy))
	s.mux.HandleFunc("/api/open-external", s.post(s.handleOpenExternal))
	s.mux.HandleFunc("/api/open-folder", s.post(s.handleOpenFolder))
	s.mux.HandleFunc("/api/wintun/install", s.post(s.handleWintunInstall))
	s.mux.HandleFunc("/api/relaunch-admin", s.post(s.handleRelaunch))
	s.mux.HandleFunc("/api/selftest", s.post(s.handleSelfTest))
	s.mux.HandleFunc("/api/logs/clear", s.post(s.handleLogsClear))
}

// authorized checks the session cookie and, on the first request, exchanges
// the token of the URL for the cookie.
func (s *Server) authorized(w http.ResponseWriter, r *http.Request) bool {
	if c, err := r.Cookie(cookieName); err == nil && s.tokenEqual(c.Value) {
		return true
	}
	if token := r.URL.Query().Get("t"); token != "" && s.tokenEqual(token) {
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    s.token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   12 * 60 * 60,
		})
		// Drop the token from the address bar so that it does not end up
		// in a screenshot or in a browser history.
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/", http.StatusFound)
			return false
		}
		return true
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
	return false
}

func (s *Server) tokenEqual(got string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

// post wraps a handler with the method and the origin checks.
func (s *Server) post(h func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !s.authorized(w, r) {
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			if !strings.HasSuffix(origin, s.Addr()) {
				writeError(w, http.StatusForbidden, "cross origin request")
				return
			}
		}
		h(w, r)
	}
}

func randomToken() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("ui: generate the token: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
