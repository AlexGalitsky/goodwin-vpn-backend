package hy2run

import (
	"testing"

	"website.goodwin.vpn/plane/internal/sysd"
)

func TestUnitFileHardened(t *testing.T) {
	u := UnitFile("/opt/goodwin-vpn-agent/hysteria/hysteria", "/opt/goodwin-vpn-agent/hy2.yaml")
	if !sysd.Hardened(u) {
		t.Fatal("goodwin-hysteria unit is bare root without ProtectSystem")
	}
}
