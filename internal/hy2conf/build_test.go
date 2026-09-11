package hy2conf

import (
	"strings"
	"testing"

	"website.goodwin.vpn/plane/internal/desired"
)

func TestBuild(t *testing.T) {
	raw, err := Build(desired.Hy2{
		Port:     443,
		Hostname: "titan.example.com",
		Users:    []desired.Hy2User{{ID: "u1", Password: "secret"}},
	}, "/opt/goodwin-vpn-agent/hy2-auth")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "listen: 0.0.0.0:443") {
		t.Fatalf("listen %s", s)
	}
	if !strings.Contains(s, "titan.example.com") {
		t.Fatalf("cert host %s", s)
	}
	if !strings.Contains(s, "type: command") {
		t.Fatalf("auth %s", s)
	}
	if strings.Contains(s, "obfs") {
		t.Fatal("must not set obfs")
	}
	if !strings.Contains(s, "reject(all, tcp/25)") || !strings.Contains(s, "tcp/587") {
		t.Fatalf("smtp acl %s", s)
	}
}

func TestCertPaths(t *testing.T) {
	c, k := CertPaths("mimas.goodwin.website")
	if !strings.HasSuffix(c, "/mimas.goodwin.website/fullchain.pem") {
		t.Fatal(c)
	}
	if !strings.HasSuffix(k, "/privkey.pem") {
		t.Fatal(k)
	}
}
