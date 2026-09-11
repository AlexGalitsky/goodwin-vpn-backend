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
