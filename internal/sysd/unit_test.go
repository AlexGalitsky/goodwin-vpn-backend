package sysd

import "testing"

func TestExtraHardensCores(t *testing.T) {
	if !Hardened("[Service]\nRestart=on-failure\n" + Extra) {
		t.Fatal("Extra missing ProtectSystem / NoNewPrivileges / Restart / KillMode")
	}
	if Hardened("[Service]\nUser=root\nExecStart=/usr/bin/xray\n") {
		t.Fatal("bare root unit must not count as hardened")
	}
}
