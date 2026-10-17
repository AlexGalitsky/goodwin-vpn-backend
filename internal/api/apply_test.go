package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"website.goodwin.vpn/plane/internal/desired"
	"website.goodwin.vpn/plane/internal/stack"
	"website.goodwin.vpn/plane/internal/store"
)

func TestApplyDisableExpireRevoke(t *testing.T) {
	st := openApplyTestStore(t)
	ctx := context.Background()

	agent := &fakeAgent{listen: true}
	ts := httptest.NewServer(agent.handler())
	t.Cleanup(ts.Close)
	host, port := mustHostPort(t, ts.URL)

	g, err := st.CreateGroup(ctx, "apply-"+uuid.NewString()[:8], []string{stack.FamilyVLESS}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteGroup(ctx, g.ID) })

	u, err := st.CreateUser(ctx, store.User{
		GroupID:     g.ID,
		DisplayName: "panel",
		VlessUUID:   uuid.NewString(),
		Hy2Password: "hy2pass",
		TTUser:      "u1",
		TTPassword:  "ttpass",
		Status:      "active",
		SubToken:    "tok-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteUser(ctx, u.ID) })

	n, err := st.CreateNode(ctx, store.Node{
		ID:              uuid.New(),
		Name:            "fake-" + uuid.NewString()[:8],
		IPv4:            host,
		ControlPort:     port,
		Families:        []string{stack.FamilyVLESS},
		AppliedFamilies: []string{},
		PortsJSON:       json.RawMessage(`{"vless_tcp":443}`),
		Status:          "enrolled",
		AgentToken:      "test-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteNode(ctx, n.ID) })
	if err := st.SetNodeGroups(ctx, n.ID, []uuid.UUID{g.ID}); err != nil {
		t.Fatal(err)
	}

	s := New(st, Config{
		AdminPassword: "x",
		SessionSecret: "y",
		PublicSubBase: "https://saturn.example",
	})

	if _, _, err := s.applyStoredNode(ctx, n, true); err != nil {
		t.Fatalf("create apply: %v", err)
	}
	if got := agent.vlessIDs(); len(got) != 1 || got[0] != u.VlessUUID {
		t.Fatalf("create desired %v want %s", got, u.VlessUUID)
	}
	assertSub(t, s, u.SubToken, http.StatusOK, false)

	u.Status = "disabled"
	if err := st.UpdateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	n, _ = st.Node(ctx, n.ID)
	if _, _, err := s.applyStoredNode(ctx, n, true); err != nil {
		t.Fatalf("disable apply: %v", err)
	}
	if got := agent.vlessIDs(); len(got) != 0 {
		t.Fatalf("disable left uuid on node: %v", got)
	}
	assertSub(t, s, u.SubToken, http.StatusOK, true)

	u.Status = "active"
	past := time.Now().UTC().Add(-time.Hour)
	u.Expire = &past
	if err := st.UpdateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	n, _ = st.Node(ctx, n.ID)
	if _, _, err := s.applyStoredNode(ctx, n, true); err != nil {
		t.Fatalf("expire apply: %v", err)
	}
	if got := agent.vlessIDs(); len(got) != 0 {
		t.Fatalf("expire left uuid on node: %v", got)
	}
	assertSub(t, s, u.SubToken, http.StatusOK, true)

	u.Expire = nil
	if err := st.UpdateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	n, _ = st.Node(ctx, n.ID)
	if _, _, err := s.applyStoredNode(ctx, n, true); err != nil {
		t.Fatalf("re-enable apply: %v", err)
	}
	if got := agent.vlessIDs(); len(got) != 1 {
		t.Fatalf("re-enable desired %v", got)
	}

	old := u.SubToken
	if err := st.RevokeUser(ctx, u.ID, "revoked-"+uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	n, _ = st.Node(ctx, n.ID)
	if _, _, err := s.applyStoredNode(ctx, n, true); err != nil {
		t.Fatalf("revoke apply: %v", err)
	}
	if got := agent.vlessIDs(); len(got) != 0 {
		t.Fatalf("revoke left uuid on node: %v", got)
	}
	assertSub(t, s, old, http.StatusNotFound, true)
}

func TestCollectTrafficHy2QuotaKick(t *testing.T) {
	st := openApplyTestStore(t)
	ctx := context.Background()
	uid := uuid.New()
	agent := &fakeAgent{
		listen: true,
		hy2Stats: []map[string]any{
			{"email": uid.String(), "uplink": 60, "downlink": 50},
		},
	}
	ts := httptest.NewServer(agent.handler())
	t.Cleanup(ts.Close)
	host, port := mustHostPort(t, ts.URL)

	g, err := st.CreateGroup(ctx, "hy2q-"+uuid.NewString()[:8], []string{stack.FamilyHy2}, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteGroup(ctx, g.ID) })
	u, err := st.CreateUser(ctx, store.User{
		ID:          uid,
		GroupID:     g.ID,
		DisplayName: "hy2",
		VlessUUID:   uuid.NewString(),
		Hy2Password: "hy2pass",
		TTUser:      "u1",
		TTPassword:  "ttpass",
		Status:      "active",
		Total:       100,
		SubToken:    "tok-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteUser(ctx, u.ID) })
	n, err := st.CreateNode(ctx, store.Node{
		ID:              uuid.New(),
		Name:            "fake-" + uuid.NewString()[:8],
		IPv4:            host,
		Hostname:        "hy2.example",
		ControlPort:     port,
		Families:        []string{stack.FamilyHy2},
		AppliedFamilies: []string{stack.FamilyHy2},
		PortsJSON:       json.RawMessage(`{"hy2_udp":443}`),
		Status:          "enrolled",
		AgentToken:      "test-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteNode(ctx, n.ID) })
	if err := st.SetNodeGroups(ctx, n.ID, []uuid.UUID{g.ID}); err != nil {
		t.Fatal(err)
	}

	s := New(st, Config{AdminPassword: "x", SessionSecret: "y", PublicSubBase: "https://saturn.example"})
	if _, _, err := s.applyStoredNode(ctx, n, true); err != nil {
		t.Fatalf("create apply: %v", err)
	}
	if got := agent.hy2IDs(); len(got) != 1 || got[0] != uid.String() {
		t.Fatalf("hy2 desired %v", got)
	}

	out := s.CollectTraffic(ctx)
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("collect %v", out)
	}
	got, err := st.User(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if got.Upload+got.Download < 100 {
		t.Fatalf("hy2 not credited %d/%d", got.Upload, got.Download)
	}
	if len(agent.hy2IDs()) != 0 {
		t.Fatalf("quota left hy2 user on node: %v", agent.hy2IDs())
	}
	assertSub(t, s, u.SubToken, http.StatusOK, true)
}

func TestSweepExpireKicksOnce(t *testing.T) {
	st := openApplyTestStore(t)
	ctx := context.Background()
	agent := &fakeAgent{listen: true}
	ts := httptest.NewServer(agent.handler())
	t.Cleanup(ts.Close)
	host, port := mustHostPort(t, ts.URL)

	g, err := st.CreateGroup(ctx, "sweep-"+uuid.NewString()[:8], []string{stack.FamilyVLESS}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteGroup(ctx, g.ID) })
	u, err := st.CreateUser(ctx, store.User{
		GroupID:     g.ID,
		DisplayName: "exp",
		VlessUUID:   uuid.NewString(),
		Hy2Password: "hy2pass",
		TTUser:      "u1",
		TTPassword:  "ttpass",
		Status:      "active",
		SubToken:    "tok-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteUser(ctx, u.ID) })
	n, err := st.CreateNode(ctx, store.Node{
		ID:              uuid.New(),
		Name:            "fake-" + uuid.NewString()[:8],
		IPv4:            host,
		ControlPort:     port,
		Families:        []string{stack.FamilyVLESS},
		AppliedFamilies: []string{},
		PortsJSON:       json.RawMessage(`{"vless_tcp":443}`),
		Status:          "enrolled",
		AgentToken:      "test-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteNode(ctx, n.ID) })
	if err := st.SetNodeGroups(ctx, n.ID, []uuid.UUID{g.ID}); err != nil {
		t.Fatal(err)
	}

	s := New(st, Config{AdminPassword: "x", SessionSecret: "y", PublicSubBase: "https://saturn.example"})
	if _, _, err := s.applyStoredNode(ctx, n, true); err != nil {
		t.Fatalf("create apply: %v", err)
	}
	if got := agent.vlessIDs(); len(got) != 1 {
		t.Fatalf("create desired %v", got)
	}
	putsAfterCreate := agent.putCount()

	past := time.Now().UTC().Add(-time.Hour)
	u.Expire = &past
	if err := st.UpdateUser(ctx, u); err != nil {
		t.Fatal(err)
	}

	out := s.SweepEntitlement(ctx)
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("sweep %v", out)
	}
	if got := agent.vlessIDs(); len(got) != 0 {
		t.Fatalf("sweep left uuid on node: %v", got)
	}
	putsAfterSweep := agent.putCount()
	if putsAfterSweep <= putsAfterCreate {
		t.Fatal("sweep did not Apply")
	}

	s.SweepEntitlement(ctx)
	if agent.putCount() != putsAfterSweep {
		t.Fatalf("second sweep re-applied, puts %d want %d", agent.putCount(), putsAfterSweep)
	}
}

func TestSweepRetriesFailedKick(t *testing.T) {
	st := openApplyTestStore(t)
	ctx := context.Background()
	agent := &fakeAgent{listen: true}
	ts := httptest.NewServer(agent.handler())
	t.Cleanup(ts.Close)
	host, port := mustHostPort(t, ts.URL)

	g, err := st.CreateGroup(ctx, "retry-"+uuid.NewString()[:8], []string{stack.FamilyVLESS}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteGroup(ctx, g.ID) })
	u, err := st.CreateUser(ctx, store.User{
		GroupID:     g.ID,
		DisplayName: "retry",
		VlessUUID:   uuid.NewString(),
		Hy2Password: "hy2pass",
		TTUser:      "u1",
		TTPassword:  "ttpass",
		Status:      "active",
		SubToken:    "tok-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteUser(ctx, u.ID) })
	n, err := st.CreateNode(ctx, store.Node{
		ID:              uuid.New(),
		Name:            "fake-" + uuid.NewString()[:8],
		IPv4:            host,
		ControlPort:     port,
		Families:        []string{stack.FamilyVLESS},
		AppliedFamilies: []string{},
		PortsJSON:       json.RawMessage(`{"vless_tcp":443}`),
		Status:          "enrolled",
		AgentToken:      "test-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteNode(ctx, n.ID) })
	if err := st.SetNodeGroups(ctx, n.ID, []uuid.UUID{g.ID}); err != nil {
		t.Fatal(err)
	}

	s := New(st, Config{AdminPassword: "x", SessionSecret: "y", PublicSubBase: "https://saturn.example"})
	if _, _, err := s.applyStoredNode(ctx, n, true); err != nil {
		t.Fatalf("create apply: %v", err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	u.Expire = &past
	if err := st.UpdateUser(ctx, u); err != nil {
		t.Fatal(err)
	}

	agent.setFailPut(true)
	s.SweepEntitlement(ctx)
	if got := agent.vlessIDs(); len(got) != 1 {
		t.Fatalf("failed kick should leave uuid, got %v", got)
	}

	agent.setFailPut(false)
	s.SweepEntitlement(ctx)
	if got := agent.vlessIDs(); len(got) != 0 {
		t.Fatalf("retry left uuid on node: %v", got)
	}
}

func TestPatchNodeHostnameAndStack(t *testing.T) {
	st := openApplyTestStore(t)
	ctx := context.Background()
	agent := &fakeAgent{listen: true}
	tsAgent := httptest.NewServer(agent.handler())
	t.Cleanup(tsAgent.Close)
	host, port := mustHostPort(t, tsAgent.URL)

	g, err := st.CreateGroup(ctx, "stack-"+uuid.NewString()[:8], []string{stack.FamilyVLESS, stack.FamilyHy2}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteGroup(ctx, g.ID) })
	n, err := st.CreateNode(ctx, store.Node{
		ID:              uuid.New(),
		Name:            "fake-" + uuid.NewString()[:8],
		IPv4:            host,
		ControlPort:     port,
		Families:        []string{stack.FamilyVLESS},
		AppliedFamilies: []string{stack.FamilyVLESS},
		PortsJSON:       json.RawMessage(`{"vless_tcp":443}`),
		Status:          "enrolled",
		AgentToken:      "test-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteNode(ctx, n.ID) })
	if err := st.SetNodeGroups(ctx, n.ID, []uuid.UUID{g.ID}); err != nil {
		t.Fatal(err)
	}

	s := New(st, Config{
		AdminPassword: "x",
		SessionSecret: "y",
		PublicSubBase: "https://saturn.example",
	})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	cookie := loginCookie(t, ts)

	res := doJSON(t, http.MethodPut, ts.URL+"/v1/nodes/"+n.ID.String()+"/stack",
		`{"families":["hy2"],"ports":{"hy2_udp":443}}`, cookie)
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("stack without hostname status %d body %s", res.StatusCode, body)
	}

	res = doJSON(t, http.MethodPatch, ts.URL+"/v1/nodes/"+n.ID.String(),
		`{"hostname":"titan.example"}`, cookie)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("patch hostname %d %s", res.StatusCode, body)
	}
	got, err := st.Node(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hostname != "titan.example" {
		t.Fatalf("hostname %q", got.Hostname)
	}

	res = doJSON(t, http.MethodPut, ts.URL+"/v1/nodes/"+n.ID.String()+"/stack",
		`{"families":["hy2"],"ports":{"hy2_udp":443}}`, cookie)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("stack hy2 %d %s", res.StatusCode, body)
	}
	got, err = st.Node(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stack.HasFamily(got.Families, stack.FamilyHy2) || stack.HasFamily(got.Families, stack.FamilyVLESS) {
		t.Fatalf("families %v", got.Families)
	}

	res = doJSON(t, http.MethodPost, ts.URL+"/v1/nodes/apply-all", `{}`, cookie)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("apply-all %d %s", res.StatusCode, body)
	}
	var out struct {
		Applied int `json:"applied_nodes"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Applied < 1 {
		t.Fatalf("apply-all applied %d", out.Applied)
	}
}

func loginCookie(t *testing.T, ts *httptest.Server) *http.Cookie {
	t.Helper()
	res, err := http.Post(ts.URL+"/v1/auth/login", "application/json", strings.NewReader(`{"password":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	for _, c := range res.Cookies() {
		if c.Name == "plane_session" {
			return c
		}
	}
	t.Fatal("no session cookie")
	return nil
}

func doJSON(t *testing.T, method, rawURL, body string, cookie *http.Cookie) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, rawURL, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func assertSub(t *testing.T, s *Server, token string, wantStatus int, wantEmpty bool) {
	t.Helper()
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	res, err := http.Get(ts.URL + "/sub/" + token)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != wantStatus {
		t.Fatalf("sub status %d want %d body %q", res.StatusCode, wantStatus, body)
	}
	if wantStatus == http.StatusOK && wantEmpty && strings.TrimSpace(string(body)) != "" {
		t.Fatalf("sub should be empty, got %q", body)
	}
}

type fakeAgent struct {
	mu        sync.Mutex
	last      desired.State
	listen    bool
	puts      int
	failPut   bool
	hy2Stats  []map[string]any
	xrayStats []map[string]any
}

func (a *fakeAgent) putCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.puts
}

func (a *fakeAgent) setFailPut(v bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failPut = v
}

func (a *fakeAgent) hy2IDs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.last.Hy2 == nil {
		return nil
	}
	out := make([]string, 0, len(a.last.Hy2.Users))
	for _, u := range a.last.Hy2.Users {
		out = append(out, u.ID)
	}
	return out
}

func (a *fakeAgent) vlessIDs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.last.VLESS == nil {
		return nil
	}
	out := make([]string, 0, len(a.last.VLESS.Clients))
	for _, c := range a.last.VLESS.Clients {
		out = append(out, c.ID)
	}
	return out
}

func (a *fakeAgent) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "xray_listen": a.listen, "hy2_listen": a.listen})
	})
	mux.HandleFunc("GET /v1/stats", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		xray := a.xrayStats
		hy2 := a.hy2Stats
		a.mu.Unlock()
		if xray == nil {
			xray = []map[string]any{}
		}
		if hy2 == nil {
			hy2 = []map[string]any{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "users": xray, "hy2_users": hy2})
	})
	mux.HandleFunc("PUT /v1/desired", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.puts++
		fail := a.failPut
		a.mu.Unlock()
		if fail {
			http.Error(w, "agent down", http.StatusInternalServerError)
			return
		}
		var st desired.State
		if err := json.NewDecoder(r.Body).Decode(&st); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		a.mu.Lock()
		a.last = st
		a.mu.Unlock()
		writeJSON(w, http.StatusOK, desired.ApplyResult{OK: true, XrayListen: a.listen, Hy2Listen: a.listen, Detail: "fake"})
	})
	return mux
}

func mustHostPort(t *testing.T, raw string) (string, int) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	if host == "" || host == "::" {
		host = "127.0.0.1"
	}
	return host, port
}

func openApplyTestStore(t *testing.T) *store.Store {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		base = "postgres://plane:plane@127.0.0.1:5432/plane?sslmode=disable"
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Skipf("postgres: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("postgres: %v", err)
	}
	name := "plane_apply_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	if _, err := admin.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", name)); err != nil {
		admin.Close()
		t.Skipf("create database: %v", err)
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	st, err := store.Open(ctx, u.String())
	if err != nil {
		_, _ = admin.Exec(ctx, fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", name))
		admin.Close()
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		st.Close()
		_, _ = admin.Exec(context.Background(), fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", name))
		admin.Close()
	})
	return st
}
