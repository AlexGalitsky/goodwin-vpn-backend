package api

import (
	"fmt"

	"website.goodwin.vpn/plane/internal/agentclient"
)

const certWarnDays = 14

type OverviewAlert struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

type FleetNode struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Status      string             `json:"status"`
	Alive       bool               `json:"alive"`
	XrayListen  bool               `json:"xray_listen"`
	Hy2Listen   bool               `json:"hy2_listen"`
	TTListen    bool               `json:"tt_listen"`
	XrayVersion string             `json:"xray_version,omitempty"`
	Hy2Version  string             `json:"hy2_version,omitempty"`
	TTVersion   string             `json:"tt_version,omitempty"`
	CPULoad1    float64            `json:"cpu_load1,omitempty"`
	CPUN        int                `json:"cpu_n,omitempty"`
	MemUsed     int64              `json:"mem_used,omitempty"`
	MemTotal    int64              `json:"mem_total,omitempty"`
	DiskUsed    int64              `json:"disk_used,omitempty"`
	DiskTotal   int64              `json:"disk_total,omitempty"`
	Certs       []agentclient.Cert `json:"certs,omitempty"`
}

func fleetFromHealth(id, name, status string, alive bool, h agentclient.Health) FleetNode {
	n := FleetNode{ID: id, Name: name, Status: status, Alive: alive}
	if !alive {
		return n
	}
	n.XrayListen = h.XrayListen
	n.Hy2Listen = h.Hy2Listen
	n.TTListen = h.TTListen
	n.XrayVersion = h.XrayVersion
	n.Hy2Version = h.Hy2Version
	n.TTVersion = h.TTVersion
	n.CPULoad1 = h.CPULoad1
	n.CPUN = h.CPUN
	n.MemUsed = h.MemUsed
	n.MemTotal = h.MemTotal
	n.DiskUsed = h.DiskUsed
	n.DiskTotal = h.DiskTotal
	n.Certs = h.Certs
	return n
}

func fleetAlerts(n FleetNode) []OverviewAlert {
	name := n.Name
	if name == "" {
		name = n.ID
	}
	var out []OverviewAlert
	if !n.Alive {
		if n.Status == "pending" {
			return nil
		}
		return []OverviewAlert{{Level: "bad", Text: name + " офлайн"}}
	}
	if n.Status == "ready" && !n.XrayListen && !n.Hy2Listen && !n.TTListen {
		out = append(out, OverviewAlert{Level: "bad", Text: name + " ядра не слушаются"})
	}
	if n.DiskTotal > 0 {
		pct := n.DiskUsed * 100 / n.DiskTotal
		if pct >= 90 {
			out = append(out, OverviewAlert{Level: "bad", Text: fmt.Sprintf("%s диск %d%%", name, pct)})
		}
	}
	for _, c := range n.Certs {
		label := c.Name
		if label == "" {
			label = name
		}
		switch {
		case c.DaysLeft < 0:
			out = append(out, OverviewAlert{Level: "bad", Text: label + " сертификат истёк"})
		case c.DaysLeft < certWarnDays:
			out = append(out, OverviewAlert{Level: "bad", Text: fmt.Sprintf("%s сертификат %d дней", label, c.DaysLeft)})
		}
	}
	return out
}
