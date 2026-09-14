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
}

func TestNormalizeFamilies(t *testing.T) {
	got, err := NormalizeFamilies([]string{" VLESS ", "hy2", "hy2", "tt"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != FamilyVLESS {
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
