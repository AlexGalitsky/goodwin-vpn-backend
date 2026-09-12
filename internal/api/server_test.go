package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

func TestGoodwinService(t *testing.T) {
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

	res, err := http.Get(ts.URL + "/gw/v1/service")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type %q", ct)
	}
	var body struct {
		Protocol string   `json:"protocol"`
		Version  int      `json:"version"`
		Name     string   `json:"name"`
		Privacy  string   `json:"privacy"`
		Features []string `json:"features"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Protocol != "goodwin-vpn" || body.Version != 1 || body.Name == "" {
		t.Fatalf("catalog %+v", body)
	}
	if body.Privacy != "https://saturn.goodwin.website/privacy" {
		t.Fatalf("privacy %q", body.Privacy)
	}
	if len(body.Features) != 1 || body.Features[0] != "geo-packs" {
		t.Fatalf("features %v", body.Features)
	}
}

func TestGeoPacksPublic(t *testing.T) {
	s := New(nil, Config{
		AdminPassword: "x",
		SessionSecret: "y",
	})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	man, err := http.Get(ts.URL + "/gw/v1/geo/manifest")
	if err != nil {
		t.Fatal(err)
	}
	defer man.Body.Close()
	if man.StatusCode != 200 {
		t.Fatalf("manifest status %d", man.StatusCode)
	}
	var manifest struct {
		Version string `json:"version"`
		Packs   []struct {
			ID     string `json:"id"`
			URL    string `json:"url"`
			SHA256 string `json:"sha256"`
			Bytes  int    `json:"bytes"`
		} `json:"packs"`
	}
	if err := json.NewDecoder(man.Body).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version == "" || len(manifest.Packs) != 1 || manifest.Packs[0].ID != "ads" {
		t.Fatalf("manifest %+v", manifest)
	}
	meta := manifest.Packs[0]
	if meta.URL != "/gw/v1/geo/packs/ads" || meta.SHA256 == "" || meta.Bytes <= 0 {
		t.Fatalf("pack meta %+v", meta)
	}

	packRes, err := http.Get(ts.URL + meta.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer packRes.Body.Close()
	if packRes.StatusCode != 200 {
		t.Fatalf("pack status %d", packRes.StatusCode)
	}
	raw, err := io.ReadAll(packRes.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != meta.Bytes {
		t.Fatalf("bytes header %d body %d", meta.Bytes, len(raw))
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != meta.SHA256 {
		t.Fatalf("sha256 body != manifest")
	}
	if packRes.Header.Get("ETag") != `"`+meta.SHA256+`"` {
		t.Fatalf("etag %q", packRes.Header.Get("ETag"))
	}
	var pack struct {
		ID       string   `json:"id"`
		Domains  []string `json:"domains"`
		Suffixes []string `json:"suffixes"`
		CIDRs    []string `json:"cidrs"`
	}
	if err := json.Unmarshal(raw, &pack); err != nil {
		t.Fatal(err)
	}
	if pack.ID != "ads" || len(pack.Suffixes) == 0 {
		t.Fatalf("pack %+v", pack)
	}

	missing, err := http.Get(ts.URL + "/gw/v1/geo/packs/nope")
	if err != nil {
		t.Fatal(err)
	}
	defer missing.Body.Close()
	if missing.StatusCode != 404 {
		t.Fatalf("unknown pack status %d", missing.StatusCode)
	}
}
