package agentd

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"website.goodwin.vpn/plane/internal/execcmd"
)

type Config struct {
	Listen    string
	Token     string
	AllowExec bool
	Version   string
}

type Server struct {
	cfg Config
	mux *http.ServeMux
}

func New(cfg Config) *Server {
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /v1/health", s.auth(s.health))
	s.mux.HandleFunc("POST /v1/exec", s.auth(s.exec))
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if got == "" {
			got = r.Header.Get("X-Node-Token")
		}
		if s.cfg.Token == "" || got != s.cfg.Token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"hostname":   host,
		"allow_exec": s.cfg.AllowExec,
		"version":    s.cfg.Version,
	})
}

func (s *Server) exec(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Shell      string `json:"shell"`
		TimeoutSec int    `json:"timeout_sec"`
	}
	defer r.Body.Close()
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	timeout := time.Duration(req.TimeoutSec) * time.Second
	res, err := execcmd.Run(r.Context(), req.Shell, timeout, s.cfg.AllowExec)
	if err != nil {
		status := http.StatusBadRequest
		if err == execcmd.ErrDisabled {
			status = http.StatusForbidden
		}
		http.Error(w, err.Error(), status)
		return
	}
	log.Printf("exec exit=%d cmd=%q", res.ExitCode, clip(req.Shell, 80))
	writeJSON(w, http.StatusOK, res)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
