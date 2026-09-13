package xrayrun

import "testing"

func TestZipNameHasPin(t *testing.T) {
	n := ZipName()
	if zipSHA256[n] == "" {
		t.Fatalf("no sha256 for %s", n)
	}
	if Release == "" || Release == "latest" {
		t.Fatal("release must be pinned")
	}
}
