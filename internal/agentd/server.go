package agentd

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"website.goodwin.vpn/plane/internal/desired"
	"website.goodwin.vpn/plane/internal/execcmd"
	"website.goodwin.vpn/plane/internal/xrayconf"
	"website.goodwin.vpn/plane/internal/xrayrun"
)

type Config struct {
	Listen    string
	Token     string
	AllowExec bool
	Version   string
	Prefix    string
}

type Server struct {
	cfg Config
	mux *http.ServeMux
}

func New(cfg Config) *Server {
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	if cfg.Prefix == "" {
		cfg.Prefix = "/opt/goodwin-vpn-agent"
	}
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /v1/health", s.auth(s.health))
	s.mux.HandleFunc("POST /v1/exec", s.auth(s.exec))
	s.mux.HandleFunc("PUT /v1/desired", s.auth(s.desired))
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
	port := 443
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"hostname":    host,
		"allow_exec":  s.cfg.AllowExec,
		"version":     s.cfg.Version,
		"xray_listen": xrayrun.Listening(port),
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

func (s *Server) desired(w http.ResponseWriter, r *http.Request) {
	var st desired.State
	defer r.Body.Close()
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&st); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if st.VLESS == nil {
		http.Error(w, "vless required", http.StatusBadRequest)
		return
	}
	res, err := applyVLESS(r.Context(), s.cfg.Prefix, *st.VLESS)
	if err != nil {
		log.Printf("apply vless: %v", err)
		writeJSON(w, http.StatusInternalServerError, desired.ApplyResult{OK: false, Detail: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func applyVLESS(ctx context.Context, prefix string, v desired.VLESS) (desired.ApplyResult, error) {
	if v.Port <= 0 {
		v.Port = 443
	}
	dir := filepath.Join(prefix, "xray")
	bin, err := xrayrun.EnsureBinary(ctx, dir)
	if err != nil {
		return desired.ApplyResult{}, err
	}
	raw, err := xrayconf.Build(v)
	if err != nil {
		return desired.ApplyResult{}, err
	}
	cfgPath := filepath.Join(prefix, "xray.json")
	if err := xrayrun.WriteConfig(cfgPath, raw); err != nil {
		return desired.ApplyResult{}, err
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		if err := xrayrun.InstallUnit(bin, cfgPath); err != nil {
			return desired.ApplyResult{}, err
		}
	}
	time.Sleep(800 * time.Millisecond)
	listen := xrayrun.Listening(v.Port)
	return desired.ApplyResult{
		OK:          listen,
		XrayListen:  listen,
		XrayVersion: xrayrun.Version(bin),
		Detail:      cfgPath,
	}, nil
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
