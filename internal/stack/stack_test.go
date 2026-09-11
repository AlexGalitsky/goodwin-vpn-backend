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

func TestValidateStealth(t *testing.T) {
	err := Validate(Spec{
		Families: FamiliesForPreset("stealth"),
		Ports:    DefaultPorts("stealth"),
	})
	if err != nil {
		t.Fatal(err)
	}
}
