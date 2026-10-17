package api

import (
	"testing"

	"website.goodwin.vpn/plane/internal/agentclient"
)

func TestFleetAlerts(t *testing.T) {
	offline := fleetAlerts(FleetNode{Name: "mimas", Alive: false})
	if len(offline) != 1 || offline[0].Text != "mimas офлайн" {
		t.Fatalf("%+v", offline)
	}
	if pending := fleetAlerts(FleetNode{Name: "new", Status: "pending", Alive: false}); len(pending) != 0 {
		t.Fatalf("pending %+v", pending)
	}
	cert := fleetAlerts(FleetNode{
		Name:  "titan",
		Alive: true,
		Certs: []agentclient.Cert{{Name: "titan.goodwin.website", DaysLeft: 11}},
	})
	if len(cert) != 1 || cert[0].Text != "titan.goodwin.website сертификат 11 дней" {
		t.Fatalf("%+v", cert)
	}
	expired := fleetAlerts(FleetNode{
		Name:  "titan",
		Alive: true,
		Certs: []agentclient.Cert{{Name: "titan.goodwin.website", DaysLeft: -1}},
	})
	if len(expired) != 1 || expired[0].Text != "titan.goodwin.website сертификат истёк" {
		t.Fatalf("%+v", expired)
	}
	quiet := fleetAlerts(FleetNode{Name: "titan", Status: "ready", Alive: true})
	if len(quiet) != 1 || quiet[0].Text != "titan ядра не слушаются" {
		t.Fatalf("%+v", quiet)
	}
	disk := fleetAlerts(FleetNode{Name: "titan", Alive: true, DiskUsed: 93, DiskTotal: 100})
	if len(disk) != 1 || disk[0].Text != "titan диск 93%" {
		t.Fatalf("%+v", disk)
	}
	ok := fleetAlerts(FleetNode{
		Name:       "titan",
		Status:     "ready",
		Alive:      true,
		XrayListen: true,
		Certs:      []agentclient.Cert{{Name: "titan.goodwin.website", DaysLeft: 80}},
		DiskUsed:   10,
		DiskTotal:  100,
	})
	if len(ok) != 0 {
		t.Fatalf("%+v", ok)
	}
}
