package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"website.goodwin.vpn/plane/internal/store"
)

func TestParseWebhookURL(t *testing.T) {
	if _, err := parseWebhookURL("http://evil.example/hook"); err == nil {
		t.Fatal("plain http remote")
	}
	got, err := parseWebhookURL("https://hooks.example/a")
	if err != nil || got != "https://hooks.example/a" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := parseWebhookURL("http://127.0.0.1:9/x"); err != nil {
		t.Fatal(err)
	}
	empty, err := parseWebhookURL("  ")
	if err != nil || empty != "" {
		t.Fatalf("empty %q %v", empty, err)
	}
}

func TestWebhookAuditStripsQuery(t *testing.T) {
	got := webhookAudit("https://api.telegram.org/botSECRET/sendMessage?chat_id=1")
	if strings.Contains(got, "SECRET") || strings.Contains(got, "sendMessage") || strings.Contains(got, "chat_id") {
		t.Fatalf("%s", got)
	}
	if webhookAudit("") != "cleared" {
		t.Fatal(webhookAudit(""))
	}
}

func TestQuotaAlertsNinetyPercent(t *testing.T) {
	id := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	low := quotaAlerts([]store.User{{ID: id, DisplayName: "ann", Status: "active", Upload: 8, Total: 10}})
	if len(low) != 0 {
		t.Fatalf("80%% %+v", low)
	}
	hit := quotaAlerts([]store.User{{ID: id, DisplayName: "ann", Status: "active", Upload: 9, Total: 10}})
	if len(hit) != 1 || hit[0].Text != "ann квота 90%" || hit[0].Key != "quota:"+id.String() {
		t.Fatalf("%+v", hit)
	}
	full := quotaAlerts([]store.User{{ID: id, DisplayName: "ann", Status: "active", Upload: 10, Total: 10}})
	if len(full) != 0 {
		t.Fatalf("full %+v", full)
	}
}

func TestAlertDedupe(t *testing.T) {
	s := New(nil, Config{})
	now := time.Now()
	if !s.wantAlert("k", now) {
		t.Fatal("first")
	}
	s.noteAlertSent("k", true, now)
	if s.wantAlert("k", now.Add(time.Minute)) {
		t.Fatal("dedupe")
	}
	if !s.wantAlert("k", now.Add(alertResend)) {
		t.Fatal("resend")
	}
	s.noteAlertSent("k", false, now)
	if s.wantAlert("k", now.Add(time.Minute)) {
		t.Fatal("retry wait")
	}
	if !s.wantAlert("k", now.Add(alertRetry)) {
		t.Fatal("retry")
	}
}

func TestPostWebhookAndPushOnce(t *testing.T) {
	var hits atomic.Int32
	var last map[string]string
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &last)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(hs.Close)

	st := openApplyTestStore(t)
	ctx := context.Background()
	n, err := st.CreateNode(ctx, store.Node{
		ID:          uuid.New(),
		Name:        "titan-test",
		IPv4:        "127.0.0.1",
		ControlPort: 1,
		Status:      "enrolled",
		AgentToken:  "x",
		Families:    []string{"vless"},
		PortsJSON:   json.RawMessage(`{"vless_tcp":443}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteNode(ctx, n.ID) })

	s := New(st, Config{AlertWebhook: hs.URL})
	out := s.PushAlerts(ctx)
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("%v", out)
	}
	if hits.Load() < 1 {
		t.Fatal("no webhook")
	}
	if last["source"] != "goodwin-plane" || last["text"] != "titan-test офлайн" {
		t.Fatalf("%v", last)
	}
	s.PushAlerts(ctx)
	if hits.Load() != 1 {
		t.Fatalf("dedupe hits %d", hits.Load())
	}
}
