package reality

import (
	"encoding/base64"
	"testing"
)

func TestGenerate(t *testing.T) {
	k, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if err := k.Validate(); err != nil {
		t.Fatal(err)
	}
	priv, err := base64.RawURLEncoding.DecodeString(k.PrivateKey)
	if err != nil || len(priv) != 32 {
		t.Fatalf("private %v len=%d", err, len(priv))
	}
	pub, err := base64.RawURLEncoding.DecodeString(k.PublicKey)
	if err != nil || len(pub) != 32 {
		t.Fatalf("public %v len=%d", err, len(pub))
	}
	if len(k.ShortID) != 8 {
		t.Fatalf("shortId %q", k.ShortID)
	}
	if k.Dest != DefaultDest || k.SNI != DefaultSNI {
		t.Fatalf("dest %s sni %s", k.Dest, k.SNI)
	}
}

func TestNormalizeDestSNI(t *testing.T) {
	dest, err := NormalizeDest("www.cloudflare.com")
	if err != nil || dest != DefaultDest {
		t.Fatalf("dest %q %v", dest, err)
	}
	dest, err = NormalizeDest("https://www.microsoft.com/")
	if err != nil || dest != "www.microsoft.com:443" {
		t.Fatalf("ms dest %q %v", dest, err)
	}
	sni, err := NormalizeSNI("www.Cloudflare.com:443")
	if err != nil || sni != DefaultSNI {
		t.Fatalf("sni %q %v", sni, err)
	}
	if _, err := NormalizeDest(""); err == nil {
		t.Fatal("empty dest")
	}
	if _, err := NormalizeSNI("foo/bar"); err == nil {
		t.Fatal("slash sni")
	}
}
