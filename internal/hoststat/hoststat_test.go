package hoststat

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCertsDaysLeft(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "titan.goodwin.website")
	if err := os.Mkdir(live, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	raw, err := selfSignedPEM(now.Add(11*24*time.Hour + 3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "fullchain.pem"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got := Certs(dir, now)
	if len(got) != 1 || got[0].Name != "titan.goodwin.website" {
		t.Fatalf("%+v", got)
	}
	if got[0].DaysLeft != 11 {
		t.Fatalf("days %d", got[0].DaysLeft)
	}
}

func TestSnapshotDisk(t *testing.T) {
	s := Snapshot(t.TempDir())
	if s.CPUN < 1 {
		t.Fatalf("cpu %d", s.CPUN)
	}
	if s.DiskTotal <= 0 || s.DiskUsed < 0 || s.DiskUsed > s.DiskTotal {
		t.Fatalf("disk %+v", s)
	}
}

func selfSignedPEM(notAfter time.Time) ([]byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "titan.goodwin.website"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
}
