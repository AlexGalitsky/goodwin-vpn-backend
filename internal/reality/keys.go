package reality

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const (
	DefaultDest = "www.microsoft.com:443"
	DefaultSNI  = "www.microsoft.com"
)

type Keys struct {
	PrivateKey string
	PublicKey  string
	ShortID    string
	Dest       string
	SNI        string
}

func Generate() (Keys, error) {
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return Keys{}, err
	}
	sid := make([]byte, 4)
	if _, err := rand.Read(sid); err != nil {
		return Keys{}, err
	}
	return Keys{
		PrivateKey: base64.RawURLEncoding.EncodeToString(k.Bytes()),
		PublicKey:  base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes()),
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
