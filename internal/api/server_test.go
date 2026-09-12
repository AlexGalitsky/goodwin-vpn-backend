package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdminStaticDoesNotPanicOnRegister(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("admin-ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(nil, Config{
		AdminPassword: "x",
		SessionSecret: "y",
		AdminDir:      dir,
	})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || string(body) != "admin-ok" {
		t.Fatalf("status %d body %q", res.StatusCode, body)
	}

	privacy, err := http.Get(ts.URL + "/privacy")
	if err != nil {
		t.Fatal(err)
	}
	defer privacy.Body.Close()
	html, _ := io.ReadAll(privacy.Body)
	if privacy.StatusCode != 200 {
		t.Fatalf("privacy status %d", privacy.StatusCode)
	}
	if privacy.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("privacy content-type %q", privacy.Header.Get("Content-Type"))
	}
	if !strings.Contains(string(html), "GoodWin VPN Privacy Policy") {
		t.Fatalf("privacy body missing title: %q", html[:min(len(html), 120)])
	}
	if !strings.Contains(string(html), "Политика конфиденциальности") {
		t.Fatal("privacy body missing Russian section")
	}
}

func TestCORSAllowlist(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("admin-ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(nil, Config{
		AdminPassword: "x",
		SessionSecret: "y",
		AdminDir:      dir,
		PublicSubBase: "https://saturn.goodwin.website",
	})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	get := func(origin string) *http.Response {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	res := get("https://saturn.goodwin.website")
	res.Body.Close()
	if res.Header.Get("Access-Control-Allow-Origin") != "https://saturn.goodwin.website" {
		t.Fatalf("allowed origin ACAO %q", res.Header.Get("Access-Control-Allow-Origin"))
	}

	res = get("http://127.0.0.1:5173")
	res.Body.Close()
	if res.Header.Get("Access-Control-Allow-Origin") != "http://127.0.0.1:5173" {
		t.Fatalf("vite origin ACAO %q", res.Header.Get("Access-Control-Allow-Origin"))
	}

	res = get("https://evil.example")
	res.Body.Close()
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("evil origin ACAO %q", got)
	}

	res = get("")
	res.Body.Close()
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("no origin ACAO %q", got)
	}
}
