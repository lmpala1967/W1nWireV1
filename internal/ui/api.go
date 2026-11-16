package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/w1nchi/w1nwire/internal/app"
)

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// fail reports an error to the interface. The HTTP status is 200 on purpose:
// the interface distinguishes success from failure by the "ok" field, and a
// transport level error would only hide the message of the application.
func fail(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
}

func ok(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func okWith(w http.ResponseWriter, extra map[string]any) {
	out := map[string]any{"ok": true}
	for k, v := range extra {
		out[k] = v
	}
	writeJSON(w, http.StatusOK, out)
}

// writeError is used for protocol level errors.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": msg})
}

// decode reads a JSON body, tolerating an empty one.
func decode(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("the request body is not valid JSON: %w", err)
	}
	return nil
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, s.app.Snapshot())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": s.app.Version()})
}

// handleEvents streams the state changes, the traffic counters and the log
// lines over server sent events.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	flusher, canFlush := w.(http.Flusher)
	if !canFlush {
		writeError(w, http.StatusInternalServerError, "streaming is not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	send := func(name string, data any) bool {
		payload, err := json.Marshal(data)
		if err != nil {
			return true
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	events, cancelEvents := s.app.Subscribe()
	defer cancelEvents()
	logs, cancelLogs := s.app.Logs().Subscribe()
	defer cancelLogs()

	state := s.app.Snapshot()
	if !send("state", state.Status) {
		return
	}
	for _, entry := range state.Logs {
		if !send("log", entry) {
			return
		}
	}
	if state.Busy != "" {
		send("busy", map[string]string{"busy": state.Busy})
	}

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case ev, open := <-events:
			if !open {
				return
			}
			if !send(ev.Name, ev.Data) {
				return
			}
		case entry, open := <-logs:
			if !open {
				return
			}
			if !send("log", entry) {
				return
			}
		}
	}
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProfileID string `json:"profileId"`
		Mode      string `json:"mode"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if req.ProfileID == "" {
		req.ProfileID = s.app.Snapshot().ActiveProfileID
	}
	if err := s.app.Connect(req.ProfileID, req.Mode); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (s *Server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := s.app.Disconnect(); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (s *Server) handleProfileAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          string `json:"name"`
		Host          string `json:"host"`
		PublicKey     string `json:"publicKey"`
		TunnelAddress string `json:"tunnelAddress"`
		Gateway       string `json:"gateway"`
		MTU           int    `json:"mtu"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	view, clientKey, err := s.app.AddProfile(app.Profile{
		Name:          req.Name,
		Host:          req.Host,
		ServerKey:     req.PublicKey,
		TunnelAddress: req.TunnelAddress,
		Gateway:       req.Gateway,
		MTU:           req.MTU,
	})
	if err != nil {
		fail(w, err)
		return
	}
	okWith(w, map[string]any{"profile": view, "publicKey": clientKey})
}

func (s *Server) handleProfileUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Host          string `json:"host"`
		PublicKey     string `json:"publicKey"`
		TunnelAddress string `json:"tunnelAddress"`
		Gateway       string `json:"gateway"`
		MTU           int    `json:"mtu"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	view, err := s.app.UpdateProfile(app.Profile{
		ID:            req.ID,
		Name:          req.Name,
		Host:          req.Host,
		ServerKey:     req.PublicKey,
		TunnelAddress: req.TunnelAddress,
		Gateway:       req.Gateway,
		MTU:           req.MTU,
	})
	if err != nil {
		fail(w, err)
		return
	}
	okWith(w, map[string]any{"profile": view})
}

func (s *Server) handleProfileDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if err := s.app.DeleteProfile(req.ID); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (s *Server) handleProfileActivate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if err := s.app.ActivateProfile(req.ID); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (s *Server) handleProfileImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	view, err := s.app.ImportInvite(req.Code)
	if err != nil {
		fail(w, err)
		return
	}
	okWith(w, map[string]any{"profile": view})
}

func (s *Server) handleProfileTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	rtt, err := s.app.TestProfile(req.ID)
	if err != nil {
		fail(w, err)
		return
	}
	okWith(w, map[string]any{"rttMs": rtt})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	var patch app.SettingsPatch
	if err := decode(r, &patch); err != nil {
		fail(w, err)
		return
	}
	if err := s.app.UpdateSettings(patch); err != nil {
		fail(w, err)
		return
	}
	okWith(w, map[string]any{"settings": s.app.Snapshot().Settings})
}

func (s *Server) handleSystemProxy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if err := s.app.SetSystemProxy(req.Enabled); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (s *Server) handleOpenExternal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if err := s.app.OpenExternal(req.URL); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (s *Server) handleOpenFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Which string `json:"which"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if err := s.app.OpenFolder(req.Which); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (s *Server) handleWintunInstall(w http.ResponseWriter, r *http.Request) {
	if err := s.app.InstallWintun(); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (s *Server) handleRelaunch(w http.ResponseWriter, r *http.Request) {
	if err := s.app.RelaunchAdmin(); err != nil {
		fail(w, err)
		return
	}
	ok(w)
}

func (s *Server) handleSelfTest(w http.ResponseWriter, r *http.Request) {
	if err := s.app.SelfTest(); err != nil {
		fail(w, err)
		return
	}
	okWith(w, map[string]any{"message": "self test passed"})
}

func (s *Server) handleLogsClear(w http.ResponseWriter, r *http.Request) {
	s.app.ClearLogs()
	ok(w)
}
