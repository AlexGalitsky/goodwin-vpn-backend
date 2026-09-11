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
}
