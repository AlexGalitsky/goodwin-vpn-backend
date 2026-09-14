package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"website.goodwin.vpn/plane/internal/stack"
	"website.goodwin.vpn/plane/internal/store"
	"website.goodwin.vpn/plane/internal/sub"
)

func TestSweepResetsDailyQuotaAndRestoresNode(t *testing.T) {
	st := openApplyTestStore(t)
	ctx := context.Background()

	agent := &fakeAgent{listen: true}
	ts := httptest.NewServer(agent.handler())
	t.Cleanup(ts.Close)
	host, port := mustHostPort(t, ts.URL)

	g, err := st.CreateGroupReset(ctx, "quota-"+uuid.NewString()[:8], []string{stack.FamilyVLESS}, 100, 0, sub.QuotaResetDay)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteGroup(ctx, g.ID) })

	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	u, err := st.CreateUser(ctx, store.User{
		GroupID:          g.ID,
		DisplayName:      "quota",
		VlessUUID:        uuid.NewString(),
		Hy2Password:      "hy2pass",
		TTUser:           "u1",
		TTPassword:       "ttpass",
		Upload:           60,
		Download:         40,
		Total:            100,
		Status:           "active",
		SubToken:         "tok-" + uuid.NewString(),
		QuotaReset:       sub.QuotaResetDay,
		QuotaPeriodStart: &yesterday,
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
		t.Fatalf("over-quota apply: %v", err)
	}
	if got := agent.vlessIDs(); len(got) != 0 {
		t.Fatalf("over-quota still on node: %v", got)
	}
	assertSub(t, s, u.SubToken, 200, true)

	out := s.SweepEntitlement(ctx)
	if reset, _ := out["users_reset"].(int); reset != 1 {
		t.Fatalf("sweep %v", out)
	}
	got, err := st.User(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Upload != 0 || got.Download != 0 {
		t.Fatalf("counters %d/%d want 0", got.Upload, got.Download)
	}
	if got.QuotaPeriodStart == nil || got.QuotaPeriodStart.After(time.Now().UTC()) {
		t.Fatalf("period start %v", got.QuotaPeriodStart)
	}
	if ids := agent.vlessIDs(); len(ids) != 1 || ids[0] != u.VlessUUID {
		t.Fatalf("after reset desired %v want %s", ids, u.VlessUUID)
	}
	assertSub(t, s, u.SubToken, 200, false)
}

func TestQuotaResetAdoptsWithoutZeroing(t *testing.T) {
	st := openApplyTestStore(t)
	ctx := context.Background()
	g, err := st.CreateGroup(ctx, "adopt-"+uuid.NewString()[:8], []string{stack.FamilyVLESS}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteGroup(ctx, g.ID) })
	u, err := st.CreateUser(ctx, store.User{
		GroupID:     g.ID,
		DisplayName: "adopt",
		VlessUUID:   uuid.NewString(),
		Status:      "active",
		SubToken:    "tok-" + uuid.NewString(),
		Upload:      10,
		Download:    20,
		Total:       100,
		QuotaReset:  sub.QuotaResetDay,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteUser(ctx, u.ID) })

	s := New(st, Config{AdminPassword: "x", SessionSecret: "y", PublicSubBase: "https://saturn.example"})
	out := s.SweepEntitlement(ctx)
	if adopted, _ := out["users_adopted"].(int); adopted != 1 {
		t.Fatalf("sweep %v", out)
	}
	got, err := st.User(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Upload != 10 || got.Download != 20 {
		t.Fatalf("adopt zeroed counters %d/%d", got.Upload, got.Download)
	}
	if got.QuotaPeriodStart == nil {
		t.Fatal("adopt left period_start nil")
	}
}

func TestQuotaResetKeepsTrafficCursor(t *testing.T) {
	st := openApplyTestStore(t)
	ctx := context.Background()
	g, err := st.CreateGroup(ctx, "cursor-"+uuid.NewString()[:8], []string{stack.FamilyVLESS}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteGroup(ctx, g.ID) })
	n, err := st.CreateNode(ctx, store.Node{
		ID:         uuid.New(),
		Name:       "n",
		IPv4:       "127.0.0.1",
		Status:     "enrolled",
		AgentToken: "t",
		Families:   []string{stack.FamilyVLESS},
		PortsJSON:  json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteNode(ctx, n.ID) })
	u, err := st.CreateUser(ctx, store.User{
		GroupID:     g.ID,
		DisplayName: "c",
		VlessUUID:   uuid.NewString(),
		Status:      "active",
		SubToken:    "tok-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteUser(ctx, u.ID) })

	if _, _, _, err := st.ApplyTraffic(ctx, n.ID, u.ID, store.TrafficXray, 1000, 2000); err != nil {
		t.Fatal(err)
	}
	start := sub.PeriodStart(time.Now().UTC(), sub.QuotaResetDay)
	if err := st.ResetQuotaPeriod(ctx, u.ID, start, true); err != nil {
		t.Fatal(err)
	}
	up, down, after, err := st.ApplyTraffic(ctx, n.ID, u.ID, store.TrafficXray, 1000, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if up != 0 || down != 0 || after.Upload != 0 || after.Download != 0 {
		t.Fatalf("cursor dump up=%d down=%d user=%d/%d", up, down, after.Upload, after.Download)
	}
	up, down, after, err = st.ApplyTraffic(ctx, n.ID, u.ID, store.TrafficXray, 1001, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if up != 1 || down != 0 || after.Upload != 1 {
		t.Fatalf("delta up=%d down=%d user=%d", up, down, after.Upload)
	}
}
