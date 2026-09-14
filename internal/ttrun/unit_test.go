package ttrun

import (
	"testing"

	"website.goodwin.vpn/plane/internal/sysd"
)

func TestUnitFileHardened(t *testing.T) {
	u := UnitFile(
		"/opt/goodwin-vpn-agent/trusttunnel/trusttunnel_endpoint",
		"/opt/goodwin-vpn-agent/trusttunnel",
		"/opt/goodwin-vpn-agent/trusttunnel/vpn.toml",
		"/opt/goodwin-vpn-agent/trusttunnel/hosts.toml",
	)
	if !sysd.Hardened(u) {
		t.Fatal("goodwin-trusttunnel unit is bare root without ProtectSystem")
	}
}
