package hy2run

import "testing"

func TestListeningVacant(t *testing.T) {
	if Listening(59991) {
		t.Skip("port 59991 already in use")
	}
}

func TestBinName(t *testing.T) {
	n := BinName()
	if n != "hysteria-linux-amd64" && n != "hysteria-linux-arm64" {
		t.Fatalf("bin %s", n)
	}
}
