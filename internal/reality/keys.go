package reality

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

const (
	DefaultDest = "www.cloudflare.com:443"
	DefaultSNI  = "www.cloudflare.com"
)

type Keys struct {
	PrivateKey string
	PublicKey  string
	ShortID    string
	Dest       string
	SNI        string
}

func Generate() (Keys, error) {
	var priv [32]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return Keys{}, err
	}
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	pub, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return Keys{}, err
	}
	sid := make([]byte, 4)
	if _, err := rand.Read(sid); err != nil {
		return Keys{}, err
	}
	return Keys{
		PrivateKey: base64.RawURLEncoding.EncodeToString(priv[:]),
		PublicKey:  base64.RawURLEncoding.EncodeToString(pub),
		ShortID:    hex.EncodeToString(sid),
		Dest:       DefaultDest,
		SNI:        DefaultSNI,
	}, nil
}

func (k Keys) Validate() error {
	if k.PrivateKey == "" || k.PublicKey == "" || k.ShortID == "" {
		return fmt.Errorf("incomplete REALITY keys")
	}
	if k.Dest == "" || k.SNI == "" {
		return fmt.Errorf("REALITY dest/sni required")
	}
	return nil
}

func (k Keys) MatchesDefaults() bool {
	return k.Dest == DefaultDest && k.SNI == DefaultSNI
}
