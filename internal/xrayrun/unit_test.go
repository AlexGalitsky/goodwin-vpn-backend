package xrayrun

import (
	"strings"
	"testing"

	"website.goodwin.vpn/plane/internal/sysd"
)

func TestUnitFileHardened(t *testing.T) {
	u := UnitFile("/opt/goodwin-vpn-agent/xray/xray", "/opt/goodwin-vpn-agent/xray.json")
	if !sysd.Hardened(u) {
		t.Fatal("goodwin-xray unit is bare root without ProtectSystem")
	}
	if !strings.Contains(u, "Restart=on-failure") {
		t.Fatal("missing Restart=on-failure")
	}
	if !strings.Contains(u, "ReadWritePaths=/opt/goodwin-vpn-agent") {
		t.Fatal("missing ReadWritePaths")
	}
}
