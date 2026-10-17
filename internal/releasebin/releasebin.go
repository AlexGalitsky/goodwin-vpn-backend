package releasebin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func Get(ctx context.Context, url, wantSHA256 string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("download: %s", res.Status)
	}
	if maxBytes <= 0 {
		maxBytes = 80 << 20
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxBytes))
	if err != nil {
		return nil, err
	}
	if err := VerifySHA256(raw, wantSHA256); err != nil {
		return nil, err
	}
	return raw, nil
}

func VerifySHA256(raw []byte, want string) error {
	want = strings.TrimSpace(strings.ToLower(want))
	if want == "" {
		return fmt.Errorf("missing sha256")
	}
	sum := sha256.Sum256(raw)
	got := hex.EncodeToString(sum[:])
	if !bytes.Equal([]byte(got), []byte(want)) {
		return fmt.Errorf("sha256 mismatch: got %s want %s", got, want)
	}
	return nil
}

func WriteFile(path string, raw []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".new"
	if err := os.WriteFile(tmp, raw, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func PinMatches(path, version string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(raw)) == strings.TrimSpace(version)
}
