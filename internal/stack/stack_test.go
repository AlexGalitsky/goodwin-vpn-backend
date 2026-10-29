package stack

import "testing"

func TestValidateMaxStack(t *testing.T) {
	err := Validate(Spec{
		Families: []string{FamilyVLESS, FamilyHy2, FamilyTT},
		Ports:    DefaultPorts("max"),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateTTConflictOn443(t *testing.T) {
	err := Validate(Spec{
		Families: []string{FamilyVLESS, FamilyHy2, FamilyTT},
		Ports:    Ports{VlessTCP: 443, Hy2UDP: 443, TT: 443},
	})
	if err == nil {
		t.Fatal("expected conflict")
	}
}

func TestNeedsHostname(t *testing.T) {
	if !NeedsHostname([]string{FamilyVLESS, FamilyHy2}) {
		t.Fatal("hy2 needs hostname")
	}
	if NeedsHostname([]string{FamilyTT}) != true {
		t.Fatal("tt needs hostname")
	}
	if NeedsHostname([]string{FamilyVLESS}) {
		t.Fatal("stealth does not")
	}
	if !NeedsHostname([]string{FamilyVMess}) {
		t.Fatal("vmess needs hostname")
	}
}

func TestDetectPreset(t *testing.T) {
	if got := DetectPreset([]string{FamilyVLESS}, DefaultPorts("stealth")); got != "stealth" {
		t.Fatalf("stealth: %s", got)
	}
	if got := DetectPreset([]string{FamilyVLESS, FamilyHy2, FamilyTT}, DefaultPorts("max")); got != "max" {
		t.Fatalf("max: %s", got)
	}
	if got := DetectPreset([]string{FamilyVLESS, FamilyHy2, FamilyTT}, DefaultPorts("tt-first")); got != "tt-first" {
		t.Fatalf("tt-first: %s", got)
	}
	if got := DetectPreset([]string{FamilyHy2}, DefaultPorts("hy2")); got != "hy2" {
		t.Fatalf("hy2: %s", got)
	}
	if got := DetectPreset(FamiliesForPreset("review"), DefaultPorts("review")); got != "review" {
		t.Fatalf("review: %s", got)
	}
	if got := DetectPreset([]string{FamilyVLESS}, DefaultPorts("stealth-dual")); got != "stealth-dual" {
		t.Fatalf("stealth-dual: %s", got)
	}
	if got := DetectPreset([]string{FamilyVLESS, FamilyHy2, FamilyTT}, DefaultPorts("max-full")); got != "max-full" {
		t.Fatalf("max-full: %s", got)
	}
}

func TestNormalizeFamilies(t *testing.T) {
	got, err := NormalizeFamilies([]string{" VLESS ", "hy2", "hy2", "tt", "ss", "shadowsocks", "vmess", "trojan"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 6 || got[0] != FamilyVLESS {
		t.Fatalf("%v", got)
	}
	if _, err := NormalizeFamilies(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateStealth(t *testing.T) {
	err := Validate(Spec{
		Families: FamiliesForPreset("stealth"),
		Ports:    DefaultPorts("stealth"),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateReview(t *testing.T) {
	err := Validate(Spec{
		Families: FamiliesForPreset("review"),
		Ports:    DefaultPorts("review"),
	})
	if err != nil {
		t.Fatal(err)
	}
	listens := ResolveXray(DefaultPorts("review"))
	if len(listens) != 7 {
		t.Fatalf("listens %d", len(listens))
	}
}

func TestResolveXrayLegacy(t *testing.T) {
	got := ResolveXray(Ports{VlessTCP: 443})
	if len(got) != 1 || got[0].Network != "grpc" {
		t.Fatalf("%+v", got)
	}
}
