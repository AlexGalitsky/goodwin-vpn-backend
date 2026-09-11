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
	return cors(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /v1/auth/login", s.login)
	s.mux.HandleFunc("POST /v1/auth/logout", s.logout)
	s.mux.HandleFunc("GET /v1/me", s.withAuth(s.me))
	s.mux.HandleFunc("GET /v1/groups", s.withAuth(s.listGroups))
	s.mux.HandleFunc("POST /v1/groups", s.withAuth(s.createGroup))
	s.mux.HandleFunc("GET /v1/users", s.withAuth(s.listUsers))
	s.mux.HandleFunc("POST /v1/users", s.withAuth(s.createUser))
	s.mux.HandleFunc("PATCH /v1/users/{id}", s.withAuth(s.patchUser))
	s.mux.HandleFunc("GET /v1/nodes", s.withAuth(s.listNodes))
	s.mux.HandleFunc("POST /v1/nodes", s.withAuth(s.createNode))
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

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
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
	writeJSON(w, http.StatusOK, map[string]any{"role": "admin"})
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
		Name      string   `json:"name"`
		Protocols []string `json:"protocols"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if len(req.Protocols) == 0 {
		req.Protocols = []string{"vless", "hy2", "tt"}
	}
	g, err := s.store.CreateGroup(r.Context(), req.Name, req.Protocols)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, g)
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
	token, err := randomToken(16)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	u, err := s.store.CreateUser(r.Context(), store.User{
		GroupID:     gid,
		DisplayName: req.DisplayName,
		VlessUUID:   uuid.NewString(),
		Hy2Password: randomHex(12),
		TTUser:      "u" + randomHex(4),
		TTPassword:  randomHex(12),
		Status:      "active",
		SubToken:    token,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": u, "sub_url": s.subURL(u.SubToken), "import_url": "goodwin://import?url=" + url.QueryEscape(s.subURL(u.SubToken))})
}

func (s *Server) patchUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id")
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	st := strings.ToLower(strings.TrimSpace(req.Status))
	if st != "active" && st != "disabled" {
		writeErr(w, http.StatusBadRequest, "status must be active or disabled")
		return
	}
	if err := s.store.SetUserStatus(r.Context(), id, st); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "user")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.store.Audit(r.Context(), "admin", "user_status", &id, st)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": st})
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
	c, n, err := s.agentFor(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	keys, err := s.ensureReality(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var ports stack.Ports
	_ = json.Unmarshal(n.PortsJSON, &ports)
	port := ports.VlessTCP
	if port <= 0 {
		port = 443
	}
	clients := []desired.VLESSClient{}
	gids, err := s.store.GroupIDsForNode(r.Context(), n.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(gids) == 0 {
		writeErr(w, http.StatusBadRequest, "assign this node to a group before Apply")
		return
	}
	seen := map[string]bool{}
	for _, gid := range gids {
		g, err := s.store.Group(r.Context(), gid)
		if err != nil {
			continue
		}
		if !stack.HasFamily(g.Protocols, stack.FamilyVLESS) {
			continue
		}
		users, err := s.store.UsersInGroup(r.Context(), gid)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, u := range users {
			if seen[u.VlessUUID] {
				continue
			}
			seen[u.VlessUUID] = true
			clients = append(clients, desired.VLESSClient{
				ID:    u.VlessUUID,
				Email: u.ID.String(),
			})
		}
	}
	if len(clients) == 0 {
		writeErr(w, http.StatusBadRequest, "no vless users in this node's groups — create a user, then Apply again")
		return
	}
	st := desired.State{VLESS: &desired.VLESS{
		Port:        port,
		Network:     "grpc",
		ServiceName: xrayconf.GRPCService,
		Reality:     keys,
		Clients:     clients,
	}}
	res, err := c.Apply(r.Context(), st)
	if err != nil {
		n.Status = "failed"
		_ = s.store.UpdateNode(r.Context(), n)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if res.XrayListen {
		n.Status = "ready"
		n.AppliedFamilies = []string{stack.FamilyVLESS}
	} else {
		n.Status = "degraded"
		n.AppliedFamilies = []string{stack.FamilyVLESS}
	}
	if err := s.store.UpdateNode(r.Context(), n); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.store.Audit(r.Context(), "admin", "apply", &n.ID, res.Detail)
	writeJSON(w, http.StatusOK, map[string]any{"node_status": n.Status, "apply": res, "clients": len(clients)})
}

func (s *Server) ensureReality(ctx context.Context) (reality.Keys, error) {
	pub, err := s.store.Setting(ctx, "reality_public")
	if err == nil && pub != "" {
		priv, _ := s.store.Setting(ctx, "reality_private")
		sid, _ := s.store.Setting(ctx, "reality_short_id")
		dest, _ := s.store.Setting(ctx, "reality_dest")
		sni, _ := s.store.Setting(ctx, "reality_sni")
		k := reality.Keys{PrivateKey: priv, PublicKey: pub, ShortID: sid, Dest: dest, SNI: sni}
		if k.Dest == "" {
			k.Dest = reality.DefaultDest
		}
		if k.SNI == "" {
			k.SNI = reality.DefaultSNI
		}
		if k.MatchesDefaults() {
			return k, k.Validate()
		}
	}
	k, err := reality.Generate()
	if err != nil {
		return reality.Keys{}, err
	}
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
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var u store.User
	found := false
	for _, x := range users {
		if x.ID == id {
			u = x
			found = true
			break
		}
	}
	if !found {
		writeErr(w, http.StatusNotFound, "user")
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
	if u.Status != "active" {
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
			lines = append(lines, line)
		}
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
		Status:      u.Status,
	}, lines)
	if err != nil {
		return "", sub.Headers{}, http.StatusNotFound, err
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
