package agentd

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
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
	"website.goodwin.vpn/plane/internal/hy2conf"
	"website.goodwin.vpn/plane/internal/hy2run"
	"website.goodwin.vpn/plane/internal/ttconf"
	"website.goodwin.vpn/plane/internal/ttrun"
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

func tokenOK(want, got string) bool {
	if want == "" {
		return false
	}
	a := []byte(want)
	b := []byte(got)
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if got == "" {
			got = r.Header.Get("X-Node-Token")
		}
		if !tokenOK(s.cfg.Token, got) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	hy2Port := hy2run.ReadPortFile(filepath.Join(s.cfg.Prefix, "hy2.port"))
	ttPort := ttrun.ReadPortFile(filepath.Join(s.cfg.Prefix, "trusttunnel", "tt.port"))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"hostname":    host,
		"allow_exec":  s.cfg.AllowExec,
		"version":     s.cfg.Version,
		"xray_listen": xrayrun.Listening(443),
		"hy2_listen":  hy2Port > 0 && hy2run.Listening(hy2Port),
		"tt_listen":   ttPort > 0 && ttrun.Listening(),
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
	if st.VLESS == nil && st.Hy2 == nil && st.TT == nil {
		http.Error(w, "vless, hy2, or tt required", http.StatusBadRequest)
		return
	}
	res := desired.ApplyResult{}
	var parts []string
	if st.VLESS != nil {
		got, err := applyVLESS(r.Context(), s.cfg.Prefix, *st.VLESS)
		if err != nil {
			log.Printf("apply vless: %v", err)
			writeJSON(w, http.StatusInternalServerError, desired.ApplyResult{OK: false, Detail: err.Error()})
			return
		}
		res.XrayListen = got.XrayListen
		res.XrayVersion = got.XrayVersion
		if got.Detail != "" {
			parts = append(parts, got.Detail)
		}
	}
	if st.Hy2 != nil {
		got, err := applyHy2(r.Context(), s.cfg.Prefix, *st.Hy2)
		if err != nil {
			log.Printf("apply hy2: %v", err)
			if st.VLESS == nil && st.TT == nil {
				writeJSON(w, http.StatusInternalServerError, desired.ApplyResult{OK: false, Detail: err.Error()})
				return
			}
			parts = append(parts, "hy2: "+err.Error())
		} else {
			res.Hy2Listen = got.Hy2Listen
			res.Hy2Version = got.Hy2Version
			if got.Detail != "" {
				parts = append(parts, got.Detail)
			}
		}
	}
	if st.TT != nil {
		got, err := applyTT(r.Context(), s.cfg.Prefix, *st.TT)
		if err != nil {
			log.Printf("apply tt: %v", err)
			if st.VLESS == nil && st.Hy2 == nil {
				writeJSON(w, http.StatusInternalServerError, desired.ApplyResult{OK: false, Detail: err.Error()})
				return
			}
			parts = append(parts, "tt: "+err.Error())
		} else {
			res.TTListen = got.TTListen
			res.TTVersion = got.TTVersion
			res.TTLinks = got.TTLinks
			if got.Detail != "" {
				parts = append(parts, got.Detail)
			}
		}
	}
	res.Detail = strings.Join(parts, "; ")
	hy2OK := st.Hy2 == nil || res.Hy2Listen || len(st.Hy2.Users) == 0
	ttOK := st.TT == nil || res.TTListen || len(st.TT.Users) == 0
	res.OK = (st.VLESS == nil || res.XrayListen) && hy2OK && ttOK
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

func applyHy2(ctx context.Context, prefix string, h desired.Hy2) (desired.ApplyResult, error) {
	if h.Port <= 0 {
		h.Port = 443
	}
	if len(h.Users) == 0 {
		if err := hy2run.WriteFile(filepath.Join(prefix, "hy2-users.json"), []byte("[]\n"), 0o600); err != nil {
			return desired.ApplyResult{}, err
		}
		if runtime.GOOS == "linux" && os.Geteuid() == 0 {
			if err := hy2run.StopUnit(); err != nil {
				return desired.ApplyResult{}, err
			}
		}
		return desired.ApplyResult{OK: true, Hy2Listen: false, Detail: "hy2 stopped (no users)"}, nil
	}
	if strings.TrimSpace(h.Hostname) == "" {
		return desired.ApplyResult{}, fmt.Errorf("hy2 hostname required (Let's Encrypt SAN)")
	}
	cert, key := h.Cert, h.Key
	if cert == "" || key == "" {
		c, k := hy2conf.CertPaths(h.Hostname)
		if cert == "" {
			cert = c
		}
		if key == "" {
			key = k
		}
	}
	if _, err := os.Stat(cert); err != nil {
		return desired.ApplyResult{}, fmt.Errorf("hy2 cert missing %s — issue Let's Encrypt with CERT_DOMAIN=%s", cert, h.Hostname)
	}
	if _, err := os.Stat(key); err != nil {
		return desired.ApplyResult{}, fmt.Errorf("hy2 key missing %s", key)
	}
	h.Cert, h.Key = cert, key
	dir := filepath.Join(prefix, "hysteria")
	bin, err := hy2run.EnsureBinary(ctx, dir)
	if err != nil {
		return desired.ApplyResult{}, err
	}
	authCmd := filepath.Join(prefix, "hy2-auth")
	if err := hy2run.WriteFile(authCmd, []byte(hy2conf.AuthScript), 0o755); err != nil {
		return desired.ApplyResult{}, err
	}
	usersRaw, err := hy2conf.UsersJSON(h.Users)
	if err != nil {
		return desired.ApplyResult{}, err
	}
	if err := hy2run.WriteFile(filepath.Join(prefix, "hy2-users.json"), usersRaw, 0o600); err != nil {
		return desired.ApplyResult{}, err
	}
	raw, err := hy2conf.Build(h, authCmd)
	if err != nil {
		return desired.ApplyResult{}, err
	}
	cfgPath := filepath.Join(prefix, "hy2.yaml")
	if err := hy2run.WriteFile(cfgPath, raw, 0o600); err != nil {
		return desired.ApplyResult{}, err
	}
	if err := hy2run.WriteFile(filepath.Join(prefix, "hy2.port"), []byte(fmt.Sprintf("%d\n", h.Port)), 0o644); err != nil {
		return desired.ApplyResult{}, err
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		if err := hy2run.InstallUnit(bin, cfgPath); err != nil {
			return desired.ApplyResult{}, err
		}
	}
	time.Sleep(800 * time.Millisecond)
	listen := hy2run.Listening(h.Port)
	return desired.ApplyResult{
		OK:         listen,
		Hy2Listen:  listen,
		Hy2Version: hy2run.Version(bin),
		Detail:     cfgPath,
	}, nil
}

func applyTT(ctx context.Context, prefix string, t desired.TT) (desired.ApplyResult, error) {
	if t.Port <= 0 {
		t.Port = 8443
	}
	if len(t.Users) == 0 {
		dir := filepath.Join(prefix, "trusttunnel")
		if err := ttrun.WriteFile(filepath.Join(dir, "credentials.toml"), []byte("# no clients\n"), 0o600); err != nil {
			return desired.ApplyResult{}, err
		}
		if runtime.GOOS == "linux" && os.Geteuid() == 0 {
			if err := ttrun.StopUnit(); err != nil {
				return desired.ApplyResult{}, err
			}
		}
		return desired.ApplyResult{OK: true, TTListen: false, Detail: "tt stopped (no users)"}, nil
	}
	if strings.TrimSpace(t.Hostname) == "" {
		return desired.ApplyResult{}, fmt.Errorf("tt hostname required (Let's Encrypt SAN)")
	}
	cert, key := t.Cert, t.Key
	if cert == "" || key == "" {
		c, k := ttconf.CertPaths(t.Hostname)
		if cert == "" {
			cert = c
		}
		if key == "" {
			key = k
		}
	}
	if _, err := os.Stat(cert); err != nil {
		return desired.ApplyResult{}, fmt.Errorf("tt cert missing %s — issue Let's Encrypt with CERT_DOMAIN=%s", cert, t.Hostname)
	}
	if _, err := os.Stat(key); err != nil {
		return desired.ApplyResult{}, fmt.Errorf("tt key missing %s", key)
	}
	dir := filepath.Join(prefix, "trusttunnel")
	bin, err := ttrun.EnsureBinary(ctx, dir)
	if err != nil {
		return desired.ApplyResult{}, err
	}
	credPath := filepath.Join(dir, "credentials.toml")
	cred, err := ttconf.Credentials(t.Users)
	if err != nil {
		return desired.ApplyResult{}, err
	}
	if err := ttrun.WriteFile(credPath, cred, 0o600); err != nil {
		return desired.ApplyResult{}, err
	}
	vpnPath := filepath.Join(dir, "vpn.toml")
	if err := ttrun.WriteFile(vpnPath, ttconf.VPN(t.Port, credPath), 0o600); err != nil {
		return desired.ApplyResult{}, err
	}
	hostsPath := filepath.Join(dir, "hosts.toml")
	if err := ttrun.WriteFile(hostsPath, ttconf.Hosts(t.Hostname, cert, key), 0o600); err != nil {
		return desired.ApplyResult{}, err
	}
	if err := ttrun.WriteFile(filepath.Join(dir, "tt.port"), []byte(fmt.Sprintf("%d\n", t.Port)), 0o644); err != nil {
		return desired.ApplyResult{}, err
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		if err := ttrun.InstallUnit(bin, dir, vpnPath, hostsPath); err != nil {
			return desired.ApplyResult{}, err
		}
	}
	time.Sleep(800 * time.Millisecond)
	listen := ttrun.Listening()
	if !listen && runtime.GOOS == "linux" && os.Geteuid() == 0 {
		return desired.ApplyResult{}, fmt.Errorf("trusttunnel did not become active")
	}
	advertise := strings.TrimSpace(t.Advertise)
	if advertise == "" {
		advertise = fmt.Sprintf("%s:%d", t.Hostname, t.Port)
	}
	display := strings.TrimSpace(t.Name)
	if display == "" {
		display = t.Hostname
	}
	var links []desired.TTLink
	for _, u := range t.Users {
		link, err := ttrun.MintDeeplink(ctx, bin, vpnPath, hostsPath, u.Username, advertise, display)
		if err != nil {
			return desired.ApplyResult{}, err
		}
		links = append(links, desired.TTLink{UserID: u.ID, Username: u.Username, Link: link})
	}
	ok := listen || runtime.GOOS != "linux" || os.Geteuid() != 0
	return desired.ApplyResult{
		OK:        ok,
		TTListen:  ok,
		TTVersion: ttrun.VersionBin(bin),
		TTLinks:   links,
		Detail:    vpnPath,
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
