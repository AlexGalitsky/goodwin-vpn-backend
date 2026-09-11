package stack

import (
	"fmt"
	"strings"
)

const (
	FamilyVLESS = "vless"
	FamilyHy2   = "hy2"
	FamilyTT    = "tt"
)

type Ports struct {
	VlessTCP int `json:"vless_tcp,omitempty"`
	Hy2UDP   int `json:"hy2_udp,omitempty"`
	TT       int `json:"tt,omitempty"`
}

type Spec struct {
	Families []string `json:"families"`
	Ports    Ports    `json:"ports"`
	Preset   string   `json:"preset,omitempty"`
}

func DefaultPorts(preset string) Ports {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "tt-first":
		return Ports{VlessTCP: 8443, Hy2UDP: 8443, TT: 443}
	case "stealth":
		return Ports{VlessTCP: 443}
	case "hy2":
		return Ports{Hy2UDP: 443}
	case "tt":
		return Ports{TT: 443}
	default:
		return Ports{VlessTCP: 443, Hy2UDP: 443, TT: 8443}
	}
}

func FamiliesForPreset(preset string) []string {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "stealth":
		return []string{FamilyVLESS}
	case "hy2":
		return []string{FamilyHy2}
	case "tt":
		return []string{FamilyTT}
	case "tt-first", "max", "max-stack", "":
		return []string{FamilyVLESS, FamilyHy2, FamilyTT}
	default:
		return []string{FamilyVLESS, FamilyHy2, FamilyTT}
	}
}

func HasFamily(families []string, want string) bool {
	for _, f := range families {
		if f == want {
			return true
		}
	}
	return false
}

func NeedsHostname(families []string) bool {
	return HasFamily(families, FamilyHy2) || HasFamily(families, FamilyTT)
}

func Validate(spec Spec) error {
	vless := HasFamily(spec.Families, FamilyVLESS)
	hy2 := HasFamily(spec.Families, FamilyHy2)
	tt := HasFamily(spec.Families, FamilyTT)
	if !vless && !hy2 && !tt {
		return fmt.Errorf("select at least one family")
	}
	if vless && spec.Ports.VlessTCP <= 0 {
		return fmt.Errorf("vless needs a TCP port")
	}
	if hy2 && spec.Ports.Hy2UDP <= 0 {
		return fmt.Errorf("hy2 needs a UDP port")
	}
	if tt && spec.Ports.TT <= 0 {
		return fmt.Errorf("trusttunnel needs a TCP+UDP port")
	}
	if vless && tt && spec.Ports.VlessTCP == spec.Ports.TT {
		return fmt.Errorf("vless TCP and trusttunnel share port %d — split them", spec.Ports.TT)
	}
	if hy2 && tt && spec.Ports.Hy2UDP == spec.Ports.TT {
		return fmt.Errorf("hy2 UDP and trusttunnel share port %d — split them", spec.Ports.TT)
	}
	return nil
}
