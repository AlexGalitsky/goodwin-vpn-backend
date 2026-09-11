package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAdminStaticDoesNotPanicOnRegister(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("admin-ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(nil, Config{
		AdminPassword: "x",
		SessionSecret: "y",
		AdminDir:      dir,
	})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || string(body) != "admin-ok" {
		t.Fatalf("status %d body %q", res.StatusCode, body)
	}
}
