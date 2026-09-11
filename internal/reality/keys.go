package reality

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"

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

func FillMissing(k Keys) Keys {
	if strings.TrimSpace(k.Dest) == "" {
		k.Dest = DefaultDest
	}
	if strings.TrimSpace(k.SNI) == "" {
		k.SNI = DefaultSNI
	}
	return k
}

func NormalizeDest(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimSuffix(s, "/")
	if s == "" {
		return "", fmt.Errorf("dest required")
	}
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		host, port = s, "443"
	}
	host = strings.TrimSpace(host)
	if host == "" || strings.ContainsAny(host, "/ ") {
		return "", fmt.Errorf("dest host")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p <= 0 || p > 65535 {
		return "", fmt.Errorf("dest port")
	}
	return net.JoinHostPort(host, strconv.Itoa(p)), nil
}

func NormalizeSNI(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimSuffix(s, "/")
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || strings.ContainsAny(s, "/ :") {
		return "", fmt.Errorf("sni required")
	}
	return s, nil
}
