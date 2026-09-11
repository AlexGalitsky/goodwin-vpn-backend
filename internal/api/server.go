package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"website.goodwin.vpn/plane/internal/agentclient"
	"website.goodwin.vpn/plane/internal/desired"
	"website.goodwin.vpn/plane/internal/reality"
	"website.goodwin.vpn/plane/internal/stack"
	"website.goodwin.vpn/plane/internal/store"
	"website.goodwin.vpn/plane/internal/sub"
	"website.goodwin.vpn/plane/internal/xrayconf"
)

type Config struct {
	AdminPassword string
	SessionSecret string
	PublicSubBase string
	AdminDir      string
}

type Server struct {
	store *store.Store
	cfg   Config
	mux   *http.ServeMux
}

func New(st *store.Store, cfg Config) *Server {
	s := &Server{store: st, cfg: cfg, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.cors(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /v1/auth/login", s.login)
	s.mux.HandleFunc("POST /v1/auth/logout", s.logout)
	s.mux.HandleFunc("GET /v1/me", s.withAuth(s.me))
	s.mux.HandleFunc("GET /v1/overview", s.withAuth(s.overview))
	s.mux.HandleFunc("GET /v1/settings", s.withAuth(s.getSettings))
	s.mux.HandleFunc("PATCH /v1/settings", s.withAuth(s.patchSettings))
	s.mux.HandleFunc("GET /v1/groups", s.withAuth(s.listGroups))
	s.mux.HandleFunc("POST /v1/groups", s.withAuth(s.createGroup))
	s.mux.HandleFunc("PATCH /v1/groups/{id}", s.withAuth(s.patchGroup))
	s.mux.HandleFunc("DELETE /v1/groups/{id}", s.withAuth(s.deleteGroup))
	s.mux.HandleFunc("GET /v1/users", s.withAuth(s.listUsers))
	s.mux.HandleFunc("POST /v1/users", s.withAuth(s.createUser))
	s.mux.HandleFunc("PATCH /v1/users/{id}", s.withAuth(s.patchUser))
	s.mux.HandleFunc("DELETE /v1/users/{id}", s.withAuth(s.deleteUser))
	s.mux.HandleFunc("POST /v1/users/{id}/revoke", s.withAuth(s.revokeUser))
	s.mux.HandleFunc("POST /v1/users/{id}/rotate", s.withAuth(s.rotateUser))
	s.mux.HandleFunc("GET /v1/audit", s.withAuth(s.listAudit))
	s.mux.HandleFunc("POST /v1/traffic/collect", s.withAuth(s.collectTrafficNow))
	s.mux.HandleFunc("GET /v1/nodes", s.withAuth(s.listNodes))
	s.mux.HandleFunc("POST /v1/nodes", s.withAuth(s.createNode))
	s.mux.HandleFunc("DELETE /v1/nodes/{id}", s.withAuth(s.deleteNode))
	s.mux.HandleFunc("POST /v1/nodes/{id}/enroll", s.withAuth(s.enrollNode))
	s.mux.HandleFunc("PUT /v1/nodes/{id}/groups", s.withAuth(s.setNodeGroups))
	s.mux.HandleFunc("PUT /v1/nodes/{id}/stack", s.withAuth(s.setNodeStack))
	s.mux.HandleFunc("GET /v1/nodes/{id}/health", s.withAuth(s.nodeHealth))
	s.mux.HandleFunc("POST /v1/nodes/{id}/exec", s.withAuth(s.nodeExec))
	s.mux.HandleFunc("POST /v1/nodes/{id}/apply", s.withAuth(s.applyNode))
	s.mux.HandleFunc("GET /v1/users/{id}/preview", s.withAuth(s.previewUser))
	s.mux.HandleFunc("GET /sub/{token}", s.subscription)
	if strings.TrimSpace(s.cfg.AdminDir) != "" {
		s.mux.HandleFunc("GET /{path...}", s.adminStatic)
	}
}

func corsOrigins(publicSubBase string) map[string]struct{} {
	out := map[string]struct{}{
		"http://127.0.0.1:5173": {},
		"http://localhost:5173": {},
	}
	base := strings.TrimRight(strings.TrimSpace(publicSubBase), "/")
	if base == "" {
		return out
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return out
	}
	out[u.Scheme+"://"+u.Host] = struct{}{}
	return out
}

func (s *Server) cors(next http.Handler) http.Handler {
	allowed := corsOrigins(s.cfg.PublicSubBase)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Vary", "Origin")
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	return dec.Decode(dst)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.cfg.AdminPassword)) != 1 {
		writeErr(w, http.StatusUnauthorized, "bad password")
		return
	}
	http.SetCookie(w, s.sessionCookie(s.signSession(), 7*24*3600))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) sessionCookie(value string, maxAge int) *http.Cookie {
	c := &http.Cookie{
		Name:     "plane_session",
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	}
	if strings.HasPrefix(strings.ToLower(s.cfg.PublicSubBase), "https://") {
		c.Secure = true
	}
	return c
}

func (s *Server) adminStatic(w http.ResponseWriter, r *http.Request) {
	root := filepath.Clean(s.cfg.AdminDir)
	rel := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	target := filepath.Join(root, rel)
	if rel == "" || rel == "." {
		target = filepath.Join(root, "index.html")
	}
	if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
		http.NotFound(w, r)
		return
	}
	st, err := os.Stat(target)
	if err != nil || st.IsDir() {
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
		return
	}
	http.ServeFile(w, r, target)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, s.sessionCookie("", -1))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"role":            "admin",
		"public_sub_base": s.cfg.PublicSubBase,
	})
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	groups, err := s.store.ListGroups(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now()
	uc := map[string]int{"total": len(users), "active": 0, "disabled": 0, "revoked": 0, "expired": 0}
	for _, u := range users {
		switch u.Status {
		case "disabled":
			uc["disabled"]++
		case "revoked":
			uc["revoked"]++
		default:
			if !sub.Entitled(u.Status, u.Expire, u.Upload, u.Download, u.Total, now) {
				uc["expired"]++
			} else {
				uc["active"]++
			}
		}
	}
	fleet := s.probeFleet(ctx, nodes)
	nc := map[string]int{"total": len(fleet)}
	alerts := []OverviewAlert{}
	for _, fn := range fleet {
		st := fn.Status
		if st == "" {
			st = "unknown"
		}
		nc[st]++
		alerts = append(alerts, fleetAlerts(fn)...)
	}
	var last any
	if ev, err := s.store.LastAudit(ctx, "apply"); err == nil {
		last = ev
	}
	dest, _ := s.store.Setting(ctx, "reality_dest")
	sni, _ := s.store.Setting(ctx, "reality_sni")
	if dest == "" {
		dest = reality.DefaultDest
	}
	if sni == "" {
		sni = reality.DefaultSNI
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"public_sub_base": s.cfg.PublicSubBase,
		"https_ok":        strings.HasPrefix(strings.ToLower(s.cfg.PublicSubBase), "https://"),
		"users":           uc,
		"nodes":           nc,
		"groups":          len(groups),
		"last_apply":      last,
		"reality_dest":    dest,
		"reality_sni":     sni,
		"alerts":          alerts,
		"fleet":           fleet,
	})
}

func (s *Server) probeFleet(ctx context.Context, nodes []store.Node) []FleetNode {
	if len(nodes) == 0 {
		return []FleetNode{}
	}
	type probe struct {
		i     int
		n     store.Node
		alive bool
		h     agentclient.Health
	}
	ch := make(chan probe, len(nodes))
	for i, n := range nodes {
		go func(i int, n store.Node) {
			p := probe{i: i, n: n}
			if strings.TrimSpace(n.IPv4) != "" && strings.TrimSpace(n.AgentToken) != "" {
				cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
				c := agentclient.New(fmt.Sprintf("http://%s:%d", n.IPv4, n.ControlPort), n.AgentToken)
				h, err := c.Health(cctx)
				cancel()
				if err == nil {
					p.alive = h.OK
					p.h = h
				}
			}
			ch <- p
		}(i, n)
	}
	out := make([]FleetNode, len(nodes))
	for range nodes {
		p := <-ch
		s.noteNodeAlive(ctx, &p.n, p.alive)
		out[p.i] = fleetFromHealth(p.n.ID.String(), p.n.Name, p.n.Status, p.alive, p.h)
	}
	return out
}

func (s *Server) settingValue(ctx context.Context, key string) string {
	v, err := s.store.Setting(ctx, key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dest := s.settingValue(ctx, "reality_dest")
	sni := s.settingValue(ctx, "reality_sni")
	if dest == "" {
		dest = reality.DefaultDest
	}
	if sni == "" {
		sni = reality.DefaultSNI
	}
	pub := s.settingValue(ctx, "reality_public")
	writeJSON(w, http.StatusOK, map[string]any{
		"reality_dest":   dest,
		"reality_sni":    sni,
		"reality_public": pub,
		"keys_ready":     pub != "",
	})
}

func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RealityDest *string `json:"reality_dest"`
		RealitySNI  *string `json:"reality_sni"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.RealityDest == nil && req.RealitySNI == nil {
		writeErr(w, http.StatusBadRequest, "no fields")
		return
	}
	ctx := r.Context()
	dest := s.settingValue(ctx, "reality_dest")
	sni := s.settingValue(ctx, "reality_sni")
	if dest == "" {
		dest = reality.DefaultDest
	}
	if sni == "" {
		sni = reality.DefaultSNI
	}
	if req.RealityDest != nil {
		got, err := reality.NormalizeDest(*req.RealityDest)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		dest = got
	}
	if req.RealitySNI != nil {
		got, err := reality.NormalizeSNI(*req.RealitySNI)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		sni = got
	}
	if err := s.store.SetSetting(ctx, "reality_dest", dest); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.SetSetting(ctx, "reality_sni", sni); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.store.Audit(ctx, "admin", "settings", nil, dest+" "+sni)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"reality_dest": dest,
		"reality_sni":  sni,
	})
}

type handler func(http.ResponseWriter, *http.Request)

func (s *Server) withAuth(next handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("plane_session")
		if err != nil || !s.validSession(c.Value) {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

func (s *Server) signSession() string {
	exp := time.Now().Add(7 * 24 * time.Hour).Unix()
	payload := fmt.Sprintf("admin.%d", exp)
	mac := hmac.New(sha256.New, []byte(s.cfg.SessionSecret))
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig
}

func (s *Server) validSession(val string) bool {
	parts := strings.Split(val, ".")
	if len(parts) != 2 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(s.cfg.SessionSecret))
	mac.Write(raw)
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(want), []byte(parts[1])) != 1 {
		return false
	}
	payload := strings.SplitN(string(raw), ".", 2)
	if len(payload) != 2 || payload[0] != "admin" {
		return false
	}
	exp, err := strconv.ParseInt(payload[1], 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix() < exp
}

func (s *Server) listGroups(w http.ResponseWriter, r *http.Request) {
	gs, err := s.store.ListGroups(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if gs == nil {
		gs = []store.Group{}
	}
	writeJSON(w, http.StatusOK, gs)
}

func (s *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name               string   `json:"name"`
		Protocols          []string `json:"protocols"`
		QuotaBytes         int64    `json:"quota_bytes"`
		ExpireDefaultHours int      `json:"expire_default_hours"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if len(req.Protocols) == 0 {
		req.Protocols = []string{"vless", "hy2", "tt"}
	}
	protos, err := stack.NormalizeFamilies(req.Protocols)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Protocols = protos
	if req.QuotaBytes < 0 {
		req.QuotaBytes = 0
	}
	if req.ExpireDefaultHours < 0 {
		req.ExpireDefaultHours = 0
	}
	g, err := s.store.CreateGroup(r.Context(), req.Name, req.Protocols, req.QuotaBytes, req.ExpireDefaultHours)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) patchGroup(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id")
		return
	}
	g, err := s.store.Group(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "group")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req struct {
		Name               *string  `json:"name"`
		QuotaBytes         *int64   `json:"quota_bytes"`
		ExpireDefaultHours *int     `json:"expire_default_hours"`
		Protocols          []string `json:"protocols"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeErr(w, http.StatusBadRequest, "name")
			return
		}
		g.Name = name
	}
	if req.QuotaBytes != nil {
		if *req.QuotaBytes < 0 {
			writeErr(w, http.StatusBadRequest, "quota_bytes")
			return
		}
		g.QuotaBytes = *req.QuotaBytes
	}
	if req.ExpireDefaultHours != nil {
		if *req.ExpireDefaultHours < 0 {
			writeErr(w, http.StatusBadRequest, "expire_default_hours")
			return
		}
		g.ExpireDefaultHours = *req.ExpireDefaultHours
	}
	if req.Protocols != nil {
		protos, err := stack.NormalizeFamilies(req.Protocols)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		g.Protocols = protos
	}
	if err := s.store.UpdateGroup(r.Context(), g); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id")
		return
	}
	if err := s.store.DeleteGroup(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "group")
			return
		}
		if errors.Is(err, store.ErrConflict) {
			writeErr(w, http.StatusConflict, "group still has users")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.store.Audit(r.Context(), "admin", "group_delete", nil, id.String())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	us, err := s.store.ListUsers(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	type row struct {
		store.User
		SubURL    string `json:"sub_url"`
		ImportURL string `json:"import_url"`
	}
	out := make([]row, 0, len(us))
	for _, u := range us {
		sub := s.subURL(u.SubToken)
		out = append(out, row{
			User:      u,
			SubURL:    sub,
			ImportURL: "goodwin://import?url=" + url.QueryEscape(sub),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GroupID     string `json:"group_id"`
		DisplayName string `json:"display_name"`
		Note        string `json:"note"`
		Total       *int64 `json:"total"`
		ExpireUnix  *int64 `json:"expire_unix"`
		ExpireHours *int   `json:"expire_hours"`
		Count       int    `json:"count"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	gid, err := uuid.Parse(req.GroupID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "group_id")
		return
	}
	g, err := s.store.Group(r.Context(), gid)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusBadRequest, "group_id")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	n := req.Count
	if n <= 0 {
		n = 1
	}
	if n > 20 {
		writeErr(w, http.StatusBadRequest, "count max 20")
		return
	}
	baseName := strings.TrimSpace(req.DisplayName)
	created := make([]store.User, 0, n)
	var lastURL string
	for i := 0; i < n; i++ {
		token, err := randomToken(16)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		name := baseName
		if n > 1 {
			if name == "" {
				name = fmt.Sprintf("user-%d", i+1)
			} else {
				name = fmt.Sprintf("%s %d", name, i+1)
			}
		}
		u := store.User{
			GroupID:     gid,
			DisplayName: name,
			Note:        req.Note,
			VlessUUID:   uuid.NewString(),
			Hy2Password: randomHex(12),
			TTUser:      "u" + randomHex(4),
			TTPassword:  randomHex(12),
			Total:       g.QuotaBytes,
			Status:      "active",
			SubToken:    token,
		}
		if req.Total != nil {
			if *req.Total < 0 {
				writeErr(w, http.StatusBadRequest, "total")
				return
			}
			u.Total = *req.Total
		}
		switch {
		case req.ExpireUnix != nil:
			if *req.ExpireUnix > 0 {
				t := time.Unix(*req.ExpireUnix, 0).UTC()
				u.Expire = &t
			}
		case req.ExpireHours != nil:
			if *req.ExpireHours > 0 {
				t := time.Now().UTC().Add(time.Duration(*req.ExpireHours) * time.Hour)
				u.Expire = &t
			}
		case g.ExpireDefaultHours > 0:
			t := time.Now().UTC().Add(time.Duration(g.ExpireDefaultHours) * time.Hour)
			u.Expire = &t
		}
		u, err = s.store.CreateUser(r.Context(), u)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		created = append(created, u)
		lastURL = s.subURL(u.SubToken)
	}
	out := map[string]any{
		"count":      len(created),
		"sub_url":    lastURL,
		"import_url": "goodwin://import?url=" + url.QueryEscape(lastURL),
	}
	if len(created) == 1 {
		out["user"] = created[0]
	} else {
		out["users"] = created
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) patchUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id")
		return
	}
	u, err := s.store.User(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "user")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req struct {
		Status      *string `json:"status"`
		DisplayName *string `json:"display_name"`
		Note        *string `json:"note"`
		Upload      *int64  `json:"upload"`
		Download    *int64  `json:"download"`
		Total       *int64  `json:"total"`
		ExpireUnix  *int64  `json:"expire_unix"`
		ExtendHours *int    `json:"extend_hours"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Status == nil && req.DisplayName == nil && req.Note == nil && req.Upload == nil && req.Download == nil && req.Total == nil && req.ExpireUnix == nil && req.ExtendHours == nil {
		writeErr(w, http.StatusBadRequest, "no fields")
		return
	}
	if req.DisplayName != nil {
		u.DisplayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.Note != nil {
		u.Note = strings.TrimSpace(*req.Note)
	}
	if req.Status != nil {
		st := strings.ToLower(strings.TrimSpace(*req.Status))
		if st != "active" && st != "disabled" {
			writeErr(w, http.StatusBadRequest, "status must be active or disabled")
			return
		}
		u.Status = st
	}
	if req.Upload != nil {
		if *req.Upload < 0 {
			writeErr(w, http.StatusBadRequest, "upload")
			return
		}
		u.Upload = *req.Upload
	}
	if req.Download != nil {
		if *req.Download < 0 {
			writeErr(w, http.StatusBadRequest, "download")
			return
		}
		u.Download = *req.Download
	}
	if req.Total != nil {
		if *req.Total < 0 {
			writeErr(w, http.StatusBadRequest, "total")
			return
		}
		u.Total = *req.Total
	}
	if req.ExpireUnix != nil {
		if *req.ExpireUnix <= 0 {
			u.Expire = nil
		} else {
			t := time.Unix(*req.ExpireUnix, 0).UTC()
			u.Expire = &t
		}
	}
	if req.ExtendHours != nil {
		h := *req.ExtendHours
		if h <= 0 {
			writeErr(w, http.StatusBadRequest, "extend_hours")
			return
		}
		base := time.Now().UTC()
		if u.Expire != nil && u.Expire.After(base) {
			base = u.Expire.UTC()
		}
		t := base.Add(time.Duration(h) * time.Hour)
		u.Expire = &t
	}
	if err := s.store.UpdateUser(r.Context(), u); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.store.Audit(r.Context(), "admin", "user_patch", &id, u.Status)
	out := map[string]any{"ok": true, "user": u}
	if req.Status != nil || req.Upload != nil || req.Download != nil || req.Total != nil || req.ExpireUnix != nil || req.ExtendHours != nil {
		out["applied_nodes"] = s.applyGroupNodes(r.Context(), u.GroupID)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) revokeUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id")
		return
	}
	u, err := s.store.User(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "user")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	token, err := randomToken(16)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.RevokeUser(r.Context(), id, token); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.store.DeleteTTLinksForUser(r.Context(), id)
	_ = s.store.Audit(r.Context(), "admin", "revoke", &id, u.DisplayName)
	applied := s.applyGroupNodes(r.Context(), u.GroupID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "applied_nodes": applied})
}

func (s *Server) rotateUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id")
		return
	}
	u, err := s.store.User(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "user")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if u.Status == "revoked" {
		writeErr(w, http.StatusConflict, "user is revoked")
		return
	}
	token, err := randomToken(16)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.RotateSubToken(r.Context(), id, token); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.store.Audit(r.Context(), "admin", "rotate", &id, u.DisplayName)
	subURL := s.subURL(token)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"sub_url":    subURL,
		"import_url": "goodwin://import?url=" + url.QueryEscape(subURL),
	})
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id")
		return
	}
	u, err := s.store.User(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "user")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.DeleteUser(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.store.Audit(r.Context(), "admin", "user_delete", &id, u.DisplayName)
	applied := s.applyGroupNodes(r.Context(), u.GroupID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "applied_nodes": applied})
}

func (s *Server) applyGroupNodes(ctx context.Context, groupID uuid.UUID) int {
	applied := 0
	nids, err := s.store.NodeIDsForGroup(ctx, groupID)
	if err != nil {
		return 0
	}
	for _, nid := range nids {
		n, err := s.store.Node(ctx, nid)
		if err != nil {
			continue
		}
		if _, _, err := s.applyStoredNode(ctx, n, true); err == nil {
			applied++
		}
	}
	return applied
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	ev, err := s.store.ListAudit(r.Context(), 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ev == nil {
		ev = []store.AuditEvent{}
	}
	writeJSON(w, http.StatusOK, ev)
}

func (s *Server) listNodes(w http.ResponseWriter, r *http.Request) {
	ns, err := s.store.ListNodes(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	type row struct {
		store.Node
		GroupIDs []uuid.UUID `json:"group_ids"`
	}
	out := make([]row, 0, len(ns))
	for _, n := range ns {
		ids, err := s.store.GroupIDsForNode(r.Context(), n.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		n.AgentToken = ""
		if ids == nil {
			ids = []uuid.UUID{}
		}
		out = append(out, row{Node: n, GroupIDs: ids})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteNode(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id")
		return
	}
	n, err := s.store.Node(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "node")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.DeleteNode(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.store.Audit(r.Context(), "admin", "node_delete", &id, n.Name)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) createNode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		IPv4        string   `json:"ipv4"`
		Hostname    string   `json:"hostname"`
		ControlPort int      `json:"control_port"`
		Token       string   `json:"token"`
		Preset      string   `json:"preset"`
		GroupIDs    []string `json:"group_ids"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if req.ControlPort == 0 {
		req.ControlPort = 19400
	}
	preset := req.Preset
	if preset == "" {
		preset = "max"
	}
	spec := stack.Spec{Families: stack.FamiliesForPreset(preset), Ports: stack.DefaultPorts(preset), Preset: preset}
	if err := stack.Validate(spec); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if stack.NeedsHostname(spec.Families) && strings.TrimSpace(req.Hostname) == "" {
		writeErr(w, http.StatusBadRequest, "hostname required for hy2/tt (Let's Encrypt SAN)")
		return
	}
	ports, _ := json.Marshal(spec.Ports)
	n, err := s.store.CreateNode(r.Context(), store.Node{
		Name:        req.Name,
		IPv4:        req.IPv4,
		Hostname:    req.Hostname,
		ControlPort: req.ControlPort,
		Families:    spec.Families,
		PortsJSON:   ports,
		Status:      "pending",
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(req.GroupIDs) > 0 {
		ids := make([]uuid.UUID, 0, len(req.GroupIDs))
		for _, raw := range req.GroupIDs {
			id, err := uuid.Parse(raw)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "group_ids")
				return
			}
			ids = append(ids, id)
		}
		if err := s.store.SetNodeGroups(r.Context(), n.ID, ids); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if req.Token != "" && req.IPv4 != "" {
		if err := s.doEnroll(r.Context(), &n, req.IPv4, req.ControlPort, req.Token); err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
	}
	n.AgentToken = ""
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) enrollNode(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id")
		return
	}
	var req struct {
		IPv4        string `json:"ipv4"`
		ControlPort int    `json:"control_port"`
		Token       string `json:"token"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	n, err := s.store.Node(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "node")
		return
	}
	if req.ControlPort == 0 {
		req.ControlPort = n.ControlPort
	}
	if err := s.doEnroll(r.Context(), &n, req.IPv4, req.ControlPort, req.Token); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	n.AgentToken = ""
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) doEnroll(ctx context.Context, n *store.Node, ipv4 string, port int, token string) error {
	ipv4 = strings.TrimSpace(ipv4)
	token = strings.TrimSpace(token)
	if ipv4 == "" || token == "" {
		return errors.New("ipv4 and token required")
	}
	if port == 0 {
		port = 19400
	}
	c := agentclient.New(fmt.Sprintf("http://%s:%d", ipv4, port), token)
	h, err := c.Health(ctx)
	if err != nil {
		return fmt.Errorf("agent health: %w", err)
	}
	if !h.OK {
		return errors.New("agent not ok")
	}
	n.IPv4 = ipv4
	n.ControlPort = port
	n.AgentToken = token
	n.Status = "enrolled"
	if err := s.store.UpdateNode(ctx, *n); err != nil {
		return err
	}
	_ = s.store.Audit(ctx, "admin", "enroll", &n.ID, ipv4)
	return nil
}

func (s *Server) setNodeGroups(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id")
		return
	}
	var req struct {
		GroupIDs []string `json:"group_ids"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	ids := make([]uuid.UUID, 0, len(req.GroupIDs))
	for _, raw := range req.GroupIDs {
		gid, err := uuid.Parse(raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "group_ids")
			return
		}
		ids = append(ids, gid)
	}
	if err := s.store.SetNodeGroups(r.Context(), id, ids); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) setNodeStack(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id")
		return
	}
	var spec stack.Spec
	if err := readJSON(r, &spec); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := stack.Validate(spec); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	n, err := s.store.Node(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "node")
		return
	}
	n.Families = spec.Families
	n.PortsJSON, _ = json.Marshal(spec.Ports)
	if err := s.store.UpdateNode(r.Context(), n); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, spec)
}

func (s *Server) nodeHealth(w http.ResponseWriter, r *http.Request) {
	c, n, err := s.agentFor(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	h, err := c.Health(ctx)
	if err != nil {
		s.noteNodeAlive(r.Context(), &n, false)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	s.noteNodeAlive(r.Context(), &n, h.OK)
	writeJSON(w, http.StatusOK, map[string]any{"node_id": n.ID, "health": h, "node_status": n.Status})
}

func (s *Server) nodeExec(w http.ResponseWriter, r *http.Request) {
	c, n, err := s.agentFor(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		Shell      string `json:"shell"`
		TimeoutSec int    `json:"timeout_sec"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	timeout := time.Duration(req.TimeoutSec) * time.Second
	res, err := c.Exec(r.Context(), req.Shell, timeout)
	detail := req.Shell
	if len(detail) > 200 {
		detail = detail[:200]
	}
	_ = s.store.Audit(r.Context(), "admin", "exec", &n.ID, detail)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) applyNode(w http.ResponseWriter, r *http.Request) {
	_, n, err := s.agentFor(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	out, code, err := s.applyStoredNode(r.Context(), n, false)
	if err != nil {
		writeErr(w, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) applyStoredNode(ctx context.Context, n store.Node, allowEmpty bool) (map[string]any, int, error) {
	if strings.TrimSpace(n.IPv4) == "" || strings.TrimSpace(n.AgentToken) == "" {
		return nil, http.StatusBadRequest, errors.New("node not enrolled")
	}
	c := agentclient.New(fmt.Sprintf("http://%s:%d", n.IPv4, n.ControlPort), n.AgentToken)
	var ports stack.Ports
	_ = json.Unmarshal(n.PortsJSON, &ports)
	gids, err := s.store.GroupIDsForNode(ctx, n.ID)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if len(gids) == 0 {
		return nil, http.StatusBadRequest, errors.New("assign this node to a group before Apply")
	}

	wantVLESS := stack.HasFamily(n.Families, stack.FamilyVLESS)
	wantHy2 := stack.HasFamily(n.Families, stack.FamilyHy2)
	wantTT := stack.HasFamily(n.Families, stack.FamilyTT)
	if (wantHy2 || wantTT) && strings.TrimSpace(n.Hostname) == "" {
		return nil, http.StatusBadRequest, errors.New("hostname required for hy2/tt (Let's Encrypt SAN)")
	}

	now := time.Now()
	clients := []desired.VLESSClient{}
	hy2Users := []desired.Hy2User{}
	ttUsers := []desired.TTUser{}
	seenVless := map[string]bool{}
	seenHy2 := map[string]bool{}
	seenTT := map[string]bool{}
	for _, gid := range gids {
		g, err := s.store.Group(ctx, gid)
		if err != nil {
			continue
		}
		users, err := s.store.UsersInGroup(ctx, gid)
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
		if wantVLESS && stack.HasFamily(g.Protocols, stack.FamilyVLESS) {
			for _, u := range users {
				if !sub.Entitled(u.Status, u.Expire, u.Upload, u.Download, u.Total, now) {
					continue
				}
				if seenVless[u.VlessUUID] {
					continue
				}
				seenVless[u.VlessUUID] = true
				clients = append(clients, desired.VLESSClient{ID: u.VlessUUID, Email: u.ID.String()})
			}
		}
		if wantHy2 && stack.HasFamily(g.Protocols, stack.FamilyHy2) {
			for _, u := range users {
				if !sub.Entitled(u.Status, u.Expire, u.Upload, u.Download, u.Total, now) {
					continue
				}
				if strings.TrimSpace(u.Hy2Password) == "" || seenHy2[u.Hy2Password] {
					continue
				}
				seenHy2[u.Hy2Password] = true
				hy2Users = append(hy2Users, desired.Hy2User{ID: u.ID.String(), Password: u.Hy2Password})
			}
		}
		if wantTT && stack.HasFamily(g.Protocols, stack.FamilyTT) {
			for _, u := range users {
				if !sub.Entitled(u.Status, u.Expire, u.Upload, u.Download, u.Total, now) {
					continue
				}
				if strings.TrimSpace(u.TTUser) == "" || strings.TrimSpace(u.TTPassword) == "" || seenTT[u.TTUser] {
					continue
				}
				seenTT[u.TTUser] = true
				ttUsers = append(ttUsers, desired.TTUser{ID: u.ID.String(), Username: u.TTUser, Password: u.TTPassword})
			}
		}
	}

	st := desired.State{}
	if wantVLESS {
		keys, err := s.ensureReality(ctx)
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
		port := ports.VlessTCP
		if port <= 0 {
			port = 443
		}
		st.VLESS = &desired.VLESS{
			Port:        port,
			Network:     "grpc",
			ServiceName: xrayconf.GRPCService,
			Reality:     keys,
			Clients:     clients,
		}
	}
	if wantHy2 {
		hport := ports.Hy2UDP
		if hport <= 0 {
			hport = 443
		}
		st.Hy2 = &desired.Hy2{
			Port:     hport,
			Hostname: n.Hostname,
			Users:    hy2Users,
		}
	}
	if wantTT {
		tport := ports.TT
		if tport <= 0 {
			tport = 8443
		}
		advHost := strings.TrimSpace(n.Hostname)
		if advHost == "" {
			advHost = n.IPv4
		}
		st.TT = &desired.TT{
			Port:      tport,
			Hostname:  n.Hostname,
			Advertise: fmt.Sprintf("%s:%d", advHost, tport),
			Name:      n.Name,
			Users:     ttUsers,
		}
	}
	if st.VLESS == nil && st.Hy2 == nil && st.TT == nil {
		return nil, http.StatusBadRequest, errors.New("node has no protocol families")
	}

	res, err := c.Apply(ctx, st)
	if err != nil {
		n.Status = "failed"
		_ = s.store.UpdateNode(ctx, n)
		return nil, http.StatusBadGateway, err
	}
	fams := []string{}
	if res.XrayListen {
		fams = append(fams, stack.FamilyVLESS)
	}
	if res.Hy2Listen {
		fams = append(fams, stack.FamilyHy2)
	}
	if res.TTListen && len(res.TTLinks) > 0 {
		fams = append(fams, stack.FamilyTT)
	}
	n.AppliedFamilies = fams
	switch {
	case !res.OK && len(fams) == 0:
		n.Status = "failed"
	case !res.OK:
		n.Status = "degraded"
	default:
		n.Status = "ready"
	}
	if err := s.store.UpdateNode(ctx, n); err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if res.TTListen && len(res.TTLinks) > 0 {
		links := map[uuid.UUID]string{}
		for _, l := range res.TTLinks {
			id, err := uuid.Parse(l.UserID)
			if err != nil {
				continue
			}
			links[id] = l.Link
		}
		_ = s.store.ReplaceTTLinks(ctx, n.ID, links)
	} else {
		_ = s.store.DeleteTTLinksForNode(ctx, n.ID)
	}
	detail, _ := json.Marshal(map[string]any{
		"status":   n.Status,
		"families": fams,
		"vless":    len(clients),
		"hy2":      len(hy2Users),
		"tt":       len(ttUsers),
		"agent":    res.Detail,
	})
	_ = s.store.Audit(ctx, "admin", "apply", &n.ID, string(detail))
	return map[string]any{
		"node_status": n.Status,
		"apply":       res,
		"clients":     len(clients),
		"hy2_users":   len(hy2Users),
		"tt_users":    len(ttUsers),
	}, 0, nil
}

func (s *Server) ensureReality(ctx context.Context) (reality.Keys, error) {
	pub := s.settingValue(ctx, "reality_public")
	priv := s.settingValue(ctx, "reality_private")
	sid := s.settingValue(ctx, "reality_short_id")
	dest := s.settingValue(ctx, "reality_dest")
	sni := s.settingValue(ctx, "reality_sni")
	if pub != "" && priv != "" && sid != "" {
		k := reality.FillMissing(reality.Keys{
			PrivateKey: priv,
			PublicKey:  pub,
			ShortID:    sid,
			Dest:       dest,
			SNI:        sni,
		})
		return k, k.Validate()
	}
	k, err := reality.Generate()
	if err != nil {
		return reality.Keys{}, err
	}
	if dest != "" {
		k.Dest = dest
	}
	if sni != "" {
		k.SNI = sni
	}
	k = reality.FillMissing(k)
	pairs := map[string]string{
		"reality_private":  k.PrivateKey,
		"reality_public":   k.PublicKey,
		"reality_short_id": k.ShortID,
		"reality_dest":     k.Dest,
		"reality_sni":      k.SNI,
	}
	for key, val := range pairs {
		if err := s.store.SetSetting(ctx, key, val); err != nil {
			return reality.Keys{}, err
		}
	}
	return k, nil
}

func (s *Server) realityShare(ctx context.Context) (*sub.Reality, error) {
	pub, err := s.store.Setting(ctx, "reality_public")
	if err != nil || pub == "" {
		return nil, err
	}
	sid, _ := s.store.Setting(ctx, "reality_short_id")
	sni, _ := s.store.Setting(ctx, "reality_sni")
	if sni == "" {
		sni = reality.DefaultSNI
	}
	return &sub.Reality{
		SNI:         sni,
		PublicKey:   pub,
		ShortID:     sid,
		FP:          "chrome",
		Network:     "grpc",
		ServiceName: xrayconf.GRPCService,
	}, nil
}

func (s *Server) agentFor(r *http.Request) (*agentclient.Client, store.Node, error) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		return nil, store.Node{}, errors.New("id")
	}
	n, err := s.store.Node(r.Context(), id)
	if err != nil {
		return nil, store.Node{}, errors.New("node not found")
	}
	if n.IPv4 == "" || n.AgentToken == "" {
		return nil, n, errors.New("node not enrolled")
	}
	return agentclient.New(fmt.Sprintf("http://%s:%d", n.IPv4, n.ControlPort), n.AgentToken), n, nil
}

func (s *Server) previewUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id")
		return
	}
	u, err := s.store.User(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "user")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	body, hdr, status, err := s.renderUser(r.Context(), u)
	if err != nil {
		writeErr(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"body": body, "headers": hdr, "sub_url": s.subURL(u.SubToken)})
}

func (s *Server) subscription(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	u, err := s.store.UserBySubToken(r.Context(), token)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body, hdr, status, err := s.renderUser(r.Context(), u)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	h := map[string]string{}
	sub.WriteHeaders(h, hdr)
	for k, v := range h {
		w.Header().Set(k, v)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

func (s *Server) renderUser(ctx context.Context, u store.User) (string, sub.Headers, int, error) {
	hdr := sub.Headers{
		Title:         strings.TrimSpace(u.DisplayName),
		IntervalHours: 24,
		Upload:        u.Upload,
		Download:      u.Download,
		Total:         u.Total,
	}
	if hdr.Title == "" {
		hdr.Title = "Goodwin"
	}
	if u.Expire != nil && !u.Expire.IsZero() {
		hdr.ExpireUnix = u.Expire.UTC().Unix()
	}
	if !sub.Entitled(u.Status, u.Expire, u.Upload, u.Download, u.Total, time.Now()) {
		return "", hdr, http.StatusOK, nil
	}
	groups, err := s.store.ListGroups(ctx)
	if err != nil {
		return "", sub.Headers{}, http.StatusInternalServerError, err
	}
	var g store.Group
	for _, x := range groups {
		if x.ID == u.GroupID {
			g = x
			break
		}
	}
	nodeIDs, err := s.store.NodeIDsForGroup(ctx, u.GroupID)
	if err != nil {
		return "", sub.Headers{}, http.StatusInternalServerError, err
	}
	type probed struct {
		n     store.Node
		alive bool
	}
	probes := make([]probed, len(nodeIDs))
	var wg sync.WaitGroup
	for i, nid := range nodeIDs {
		wg.Add(1)
		go func(i int, nid uuid.UUID) {
			defer wg.Done()
			n, err := s.store.Node(ctx, nid)
			if err != nil {
				return
			}
			alive := s.agentAlive(ctx, n)
			s.noteNodeAlive(ctx, &n, alive)
			probes[i] = probed{n: n, alive: alive}
		}(i, nid)
	}
	wg.Wait()
	var lines []sub.NodeLine
	for _, p := range probes {
		n := p.n
		if n.ID == uuid.Nil || !sub.IncludeNode(n.Status, n.AppliedFamilies, p.alive) {
			continue
		}
		host := n.Hostname
		if host == "" {
			host = n.IPv4
		}
		var ports stack.Ports
		_ = json.Unmarshal(n.PortsJSON, &ports)
		rk, _ := s.realityShare(ctx)
		fams := n.AppliedFamilies
		if len(fams) == 0 {
			continue
		}
		for _, fam := range fams {
			if !stack.HasFamily(g.Protocols, fam) {
				continue
			}
			port := 443
			switch fam {
			case stack.FamilyVLESS:
				port = ports.VlessTCP
			case stack.FamilyHy2:
				port = ports.Hy2UDP
			case stack.FamilyTT:
				port = ports.TT
			}
			line := sub.NodeLine{
				Name:   n.Name,
				Host:   host,
				Family: fam,
				Port:   port,
				Hy2SNI: n.Hostname,
			}
			if fam == stack.FamilyVLESS && rk != nil {
				line.Reality = rk
			}
			if fam == stack.FamilyTT {
				link, err := s.store.TTLink(ctx, n.ID, u.ID)
				if err != nil || strings.TrimSpace(link) == "" {
					continue
				}
				line.TTLink = link
			}
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return "", hdr, http.StatusOK, nil
	}
	res, err := sub.Render(sub.User{
		DisplayName: u.DisplayName,
		VlessUUID:   u.VlessUUID,
		Hy2Password: u.Hy2Password,
		TTUser:      u.TTUser,
		TTPassword:  u.TTPassword,
		Upload:      u.Upload,
		Download:    u.Download,
		Total:       u.Total,
		Expire:      u.Expire,
		Status:      u.Status,
	}, lines)
	if err != nil {
		if strings.Contains(err.Error(), "no share links") {
			return "", hdr, http.StatusOK, nil
		}
		return "", sub.Headers{}, http.StatusInternalServerError, err
	}
	return res.Body, res.Headers, http.StatusOK, nil
}

func (s *Server) subURL(token string) string {
	base := strings.TrimRight(s.cfg.PublicSubBase, "/")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}
	return base + "/sub/" + token
}

func (s *Server) agentAlive(ctx context.Context, n store.Node) bool {
	if strings.TrimSpace(n.IPv4) == "" || strings.TrimSpace(n.AgentToken) == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	c := agentclient.New(fmt.Sprintf("http://%s:%d", n.IPv4, n.ControlPort), n.AgentToken)
	h, err := c.Health(ctx)
	return err == nil && h.OK
}

func (s *Server) noteNodeAlive(ctx context.Context, n *store.Node, alive bool) {
	next := n.Status
	if !alive {
		if n.Status == "ready" || n.Status == "degraded" {
			next = "offline"
		}
	} else if n.Status == "offline" {
		next = "ready"
	}
	if next == n.Status {
		return
	}
	n.Status = next
	_ = s.store.UpdateNode(ctx, *n)
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	const hex = "0123456789abcdef"
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = hex[int(b[i])%16]
	}
	return string(out)
}

func Log(msg string, args ...any) {
	log.Printf(msg, args...)
}

func ParseListen(v string) string {
	if v == "" {
		return ":8080"
	}
	if _, err := strconv.Atoi(v); err == nil {
		return ":" + v
	}
	return v
}
