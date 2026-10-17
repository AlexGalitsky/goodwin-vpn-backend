package hy2run

import (
	"net"
	"testing"
)

func TestListeningVacantIsFalse(t *testing.T) {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Skip(err)
	}
	port := c.LocalAddr().(*net.UDPAddr).Port
	_ = c.Close()
	if Listening(port) {
		t.Fatalf("vacant %d reported listening", port)
	}
}

func TestListeningInUse(t *testing.T) {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Skip(err)
	}
	defer c.Close()
	port := c.LocalAddr().(*net.UDPAddr).Port
	if !Listening(port) {
		t.Fatalf("in-use %d reported down", port)
	}
}

func TestBinName(t *testing.T) {
	n := BinName()
	if n != "hysteria-linux-amd64" && n != "hysteria-linux-arm64" {
		t.Fatalf("bin %s", n)
	}
}
