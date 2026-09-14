package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"website.goodwin.vpn/plane/internal/store"
)

func TestPublicUserOmitsSecrets(t *testing.T) {
	s := New(nil, Config{PublicSubBase: "https://saturn.example"})
	u := store.User{
		ID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		DisplayName: "n",
		VlessUUID:   "secret-vless-uuid",
		Hy2Password: "secret-hy2",
		TTUser:      "secret-tt-user",
		TTPassword:  "secret-tt-pass",
		SubToken:    "tok-visible",
		Status:      "active",
	}
	raw, err := json.Marshal(s.publicUser(u))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, secret := range []string{"secret-vless-uuid", "secret-hy2", "secret-tt-user", "secret-tt-pass"} {
		if strings.Contains(body, secret) {
			t.Fatalf("list leaked %q in %s", secret, body)
		}
	}
	if !strings.Contains(body, "/sub/tok-visible") {
		t.Fatalf("missing sub url in %s", body)
	}
}
