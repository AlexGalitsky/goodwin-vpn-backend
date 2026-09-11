package sub

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRenderMatchesGolden(t *testing.T) {
	got, err := Render(User{
		DisplayName: "Panel",
		VlessUUID:   "11111111-2222-3333-4444-555555555555",
		Status:      "active",
	}, []NodeLine{
		{Name: "NodeA", Host: "a.example.com", Family: "vless", Port: 443},
		{Name: "NodeB", Host: "b.example.com", Family: "vless", Port: 443},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "subscriptions", "p0-plaintext.txt")
	want, err := os.ReadFile(root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got.Body) != strings.TrimSpace(string(want)) {
		t.Fatalf("body mismatch\n got: %q\nwant: %q", got.Body, want)
	}
	if got.Headers.Title != "Panel" {
		t.Fatalf("title %q", got.Headers.Title)
	}
}

func TestRenderRejectsEmpty(t *testing.T) {
	_, err := Render(User{Status: "active"}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHy2Link(t *testing.T) {
	got, err := Render(User{
		Hy2Password: "secret",
		Status:      "active",
	}, []NodeLine{{Name: "NL", Host: "nl.example.com", Family: "hy2", Port: 443, Hy2SNI: "nl.example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Body, "hysteria2://secret@nl.example.com:443") {
		t.Fatalf("body %s", got.Body)
	}
	if strings.Contains(got.Body, "obfs") {
		t.Fatal("must not emit obfs")
	}
}

func TestRealityGRPCLink(t *testing.T) {
	got, err := Render(User{
		VlessUUID: "11111111-2222-3333-4444-555555555555",
		Status:    "active",
	}, []NodeLine{{
		Name:   "Titan",
		Host:   "titan.example.com",
		Family: "vless",
		Port:   443,
		Reality: &Reality{
			SNI:         "www.cloudflare.com",
			PublicKey:   "PUBLIC",
			ShortID:     "abcd1234",
			Network:     "grpc",
			ServiceName: "goodwin",
			FP:          "chrome",
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Body, "type=grpc") || !strings.Contains(got.Body, "serviceName=goodwin") {
		t.Fatalf("body %s", got.Body)
	}
	if strings.Contains(got.Body, "flow=") {
		t.Fatal("grpc REALITY must not set vision flow")
	}
	if !strings.Contains(got.Body, "sni=www.cloudflare.com") {
		t.Fatalf("sni %s", got.Body)
	}
}

func TestWriteHeadersUserinfo(t *testing.T) {
	h := map[string]string{}
	WriteHeaders(h, Headers{
		Title:         "Panel",
		IntervalHours: 24,
		Upload:        1,
		Download:      2,
		Total:         3,
		ExpireUnix:    1700000000,
	})
	if h["profile-title"] != "Panel" {
		t.Fatalf("title %q", h["profile-title"])
	}
	if h["profile-update-interval"] != "24" {
		t.Fatalf("interval %q", h["profile-update-interval"])
	}
	want := "upload=1; download=2; total=3; expire=1700000000"
	if h["subscription-userinfo"] != want {
		t.Fatalf("userinfo %q", h["subscription-userinfo"])
	}
}

func TestTTLink(t *testing.T) {
	got, err := Render(User{Status: "active"}, []NodeLine{{
		Name:   "Titan",
		Host:   "titan.goodwin.website",
		Family: "tt",
		Port:   8443,
		TTLink: "tt://?AAAA",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got.Body) != "tt://?AAAA" {
		t.Fatalf("body %s", got.Body)
	}
}
