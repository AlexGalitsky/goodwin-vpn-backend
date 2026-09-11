package ttconf

import (
	"strings"
	"testing"

	"website.goodwin.vpn/plane/internal/desired"
)

func TestVPN(t *testing.T) {
	raw := string(VPN(8443, "/opt/goodwin-vpn-agent/trusttunnel/credentials.toml"))
	if !strings.Contains(raw, `listen_address = "0.0.0.0:8443"`) {
		t.Fatal(raw)
	}
	if !strings.Contains(raw, "[listen_protocols.quic]") {
		t.Fatal("quic required")
	}
}

func TestHosts(t *testing.T) {
	raw := string(Hosts("titan.goodwin.website", "/etc/letsencrypt/live/titan.goodwin.website/fullchain.pem", "/etc/letsencrypt/live/titan.goodwin.website/privkey.pem"))
	if !strings.Contains(raw, "titan.goodwin.website") {
		t.Fatal(raw)
	}
}

func TestCredentials(t *testing.T) {
	raw, err := Credentials([]desired.TTUser{{Username: "udev", Password: "s3cret"}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "[[client]]") || !strings.Contains(s, "udev") {
		t.Fatal(s)
	}
}

func TestCredentialsEmpty(t *testing.T) {
	if _, err := Credentials(nil); err == nil {
		t.Fatal("expected error")
	}
}
