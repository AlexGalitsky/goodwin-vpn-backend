package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"website.goodwin.vpn/plane/internal/store"
)

const (
	alertResend = 6 * time.Hour
	alertRetry  = 5 * time.Minute
)

type alertSent struct {
	at time.Time
	ok bool
}

func quotaAlerts(users []store.User) []OverviewAlert {
	var out []OverviewAlert
	for _, u := range users {
		if u.Status != "active" || u.Total <= 0 {
			continue
		}
		used := u.Upload + u.Download
		if used >= u.Total || used*10 < u.Total*9 {
			continue
		}
		name := strings.TrimSpace(u.DisplayName)
		if name == "" {
			name = u.ID.String()
			if len(name) > 8 {
				name = name[:8]
			}
		}
		out = append(out, OverviewAlert{
			Level: "warn",
			Text:  name + " квота 90%",
			Key:   "quota:" + u.ID.String(),
		})
	}
	return out
}

func parseWebhookURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("webhook url")
	}
	host := u.Hostname()
	if u.Scheme != "https" && host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return "", errors.New("webhook must be https")
	}
	return u.String(), nil
}

func webhookAudit(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		if raw == "" {
			return "cleared"
		}
		return "set"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = ""
	u.RawPath = ""
	return u.String()
}

func (s *Server) webhookURL(ctx context.Context) string {
	if s.store != nil {
		if v := s.settingValue(ctx, "alert_webhook"); v != "" {
			return v
		}
	}
	return strings.TrimSpace(s.cfg.AlertWebhook)
}

func (s *Server) wantAlert(key string, now time.Time) bool {
	s.alertMu.Lock()
	defer s.alertMu.Unlock()
	prev, ok := s.alertSent[key]
	if !ok {
		return true
	}
	wait := alertRetry
	if prev.ok {
		wait = alertResend
	}
	return now.Sub(prev.at) >= wait
}

func (s *Server) noteAlertSent(key string, ok bool, now time.Time) {
	s.alertMu.Lock()
	defer s.alertMu.Unlock()
	s.alertSent[key] = alertSent{at: now, ok: ok}
}

func (s *Server) pruneAlerts(live map[string]struct{}) {
	s.alertMu.Lock()
	defer s.alertMu.Unlock()
	for k := range s.alertSent {
		if _, ok := live[k]; !ok {
			delete(s.alertSent, k)
		}
	}
}

func postWebhook(rawURL string, a OverviewAlert) error {
	body, err := json.Marshal(map[string]string{
		"level":  a.Level,
		"text":   a.Text,
		"source": "goodwin-plane",
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "goodwin-plane")
	client := &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("webhook %d", res.StatusCode)
	}
	return nil
}

func (s *Server) collectAlerts(ctx context.Context) []OverviewAlert {
	if s.store == nil {
		return nil
	}
	var out []OverviewAlert
	nodes, err := s.store.ListNodes(ctx)
	if err == nil {
		for _, fn := range s.probeFleet(ctx, nodes) {
			out = append(out, fleetAlerts(fn)...)
		}
	}
	users, err := s.store.ListUsers(ctx)
	if err == nil {
		out = append(out, quotaAlerts(users)...)
	}
	return out
}

func (s *Server) PushAlerts(ctx context.Context) map[string]any {
	hook := s.webhookURL(ctx)
	if hook == "" {
		return map[string]any{"ok": true, "skipped": true}
	}
	if _, err := parseWebhookURL(hook); err != nil {
		log.Printf("webhook url: %v", err)
		return map[string]any{"ok": false, "error": err.Error()}
	}
	alerts := s.collectAlerts(ctx)
	live := map[string]struct{}{}
	sent := 0
	now := time.Now()
	for _, a := range alerts {
		key := a.Key
		if key == "" {
			key = a.Level + "|" + a.Text
		}
		live[key] = struct{}{}
		if !s.wantAlert(key, now) {
			continue
		}
		if err := postWebhook(hook, a); err != nil {
			log.Printf("webhook: %v", err)
			s.noteAlertSent(key, false, now)
			continue
		}
		s.noteAlertSent(key, true, now)
		sent++
	}
	s.pruneAlerts(live)
	return map[string]any{"ok": true, "sent": sent, "alerts": len(alerts)}
}

func (s *Server) AlertLoop(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = time.Minute
	}
	t := time.NewTicker(every)
	defer t.Stop()
	s.PushAlerts(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.PushAlerts(context.Background())
		}
	}
}

func (s *Server) testAlert(w http.ResponseWriter, r *http.Request) {
	hook := s.webhookURL(r.Context())
	if hook == "" {
		writeErr(w, http.StatusBadRequest, "alert_webhook")
		return
	}
	if _, err := parseWebhookURL(hook); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := postWebhook(hook, OverviewAlert{Level: "info", Text: "проверка goodwin-plane"}); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
