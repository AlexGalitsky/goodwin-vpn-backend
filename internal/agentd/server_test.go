package agentd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthAndExec(t *testing.T) {
	s := New(Config{Token: "secret", AllowExec: true, Version: "test"})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/health", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", res.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/v1/health", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	res.Body.Close()

	body, _ := json.Marshal(map[string]any{"shell": "echo plane-exec", "timeout_sec": 5})
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/v1/exec", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("exec status %d", res.StatusCode)
	}
	var out struct {
		Stdout   string `json:"stdout"`
		ExitCode int    `json:"exit_code"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.ExitCode != 0 || !bytes.Contains([]byte(out.Stdout), []byte("plane-exec")) {
		t.Fatalf("%+v", out)
	}

	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/v1/desired", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty desired status %d", res.StatusCode)
	}

	body, _ = json.Marshal(map[string]any{
		"hy2": map[string]any{
			"port":     443,
			"hostname": "no-such.example",
			"users":    []map[string]string{{"id": "u", "password": "p"}},
		},
	})
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/v1/desired", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("missing cert status %d body %s", res.StatusCode, raw)
	}
	if !bytes.Contains(raw, []byte("cert missing")) {
		t.Fatalf("body %s", raw)
	}

	body, _ = json.Marshal(map[string]any{
		"tt": map[string]any{
			"port":     8443,
			"hostname": "no-such.example",
			"users":    []map[string]string{{"id": "u", "username": "udev", "password": "p"}},
		},
	})
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/v1/desired", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("tt missing cert status %d body %s", res.StatusCode, raw)
	}
	if !bytes.Contains(raw, []byte("cert missing")) {
		t.Fatalf("tt body %s", raw)
	}
}

func TestEmptyHy2AndTTStopWithoutCert(t *testing.T) {
	dir := t.TempDir()
	s := New(Config{Token: "secret", Prefix: dir, Version: "test"})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	put := func(body []byte) (int, []byte) {
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/desired", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer secret")
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return res.StatusCode, raw
	}

	body, _ := json.Marshal(map[string]any{
		"hy2": map[string]any{
			"port":     443,
			"hostname": "no-such.example",
			"users":    []any{},
		},
	})
	code, raw := put(body)
	if code != 200 {
		t.Fatalf("empty hy2 status %d body %s", code, raw)
	}
	var hy2Out struct {
		OK        bool   `json:"ok"`
		Hy2Listen bool   `json:"hy2_listen"`
		Detail    string `json:"detail"`
	}
	if err := json.Unmarshal(raw, &hy2Out); err != nil {
		t.Fatal(err)
	}
	if !hy2Out.OK || hy2Out.Hy2Listen || !bytes.Contains([]byte(hy2Out.Detail), []byte("no users")) {
		t.Fatalf("empty hy2 %+v", hy2Out)
	}

	body, _ = json.Marshal(map[string]any{
		"tt": map[string]any{
			"port":     8443,
			"hostname": "no-such.example",
			"users":    []any{},
		},
	})
	code, raw = put(body)
	if code != 200 {
		t.Fatalf("empty tt status %d body %s", code, raw)
	}
	var ttOut struct {
		OK       bool   `json:"ok"`
		TTListen bool   `json:"tt_listen"`
		Detail   string `json:"detail"`
	}
	if err := json.Unmarshal(raw, &ttOut); err != nil {
		t.Fatal(err)
	}
	if !ttOut.OK || ttOut.TTListen || !bytes.Contains([]byte(ttOut.Detail), []byte("no users")) {
		t.Fatalf("empty tt %+v", ttOut)
	}
}
