package xrayrun

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const xrayRelease = "https://github.com/XTLS/Xray-core/releases/latest/download/"

func ZipName() string {
	if runtime.GOARCH == "arm64" {
		return "Xray-linux-arm64-v8a.zip"
	}
	return "Xray-linux-64.zip"
}

func EnsureBinary(ctx context.Context, dir string) (string, error) {
	bin := filepath.Join(dir, "xray")
	if st, err := os.Stat(bin); err == nil && st.Mode().IsRegular() {
		return bin, nil
	}
	url := xrayRelease + ZipName()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("download xray: %s", res.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 80<<20))
	if err != nil {
		return "", err
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		if base != "xray" && !strings.HasSuffix(base, ".dat") {
			continue
		}
		dst := filepath.Join(dir, base)
		if err := writeZipFile(f, dst); err != nil {
			return "", err
		}
		if base == "xray" {
			_ = os.Chmod(dst, 0o755)
		}
	}
	if _, err := os.Stat(bin); err != nil {
		return "", fmt.Errorf("xray binary missing after unzip")
	}
	return bin, nil
}

func writeZipFile(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

func WriteConfig(path string, raw []byte) error {
	return os.WriteFile(path, raw, 0o600)
}

func InstallUnit(bin, cfgPath string) error {
	unit := fmt.Sprintf(`[Unit]
Description=Goodwin Xray (VLESS REALITY)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s run -c %s
Restart=on-failure
RestartSec=2
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
`, bin, cfgPath)
	if err := os.WriteFile("/etc/systemd/system/goodwin-xray.service", []byte(unit), 0o644); err != nil {
		return err
	}
	cmds := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "--now", "goodwin-xray"},
		{"systemctl", "restart", "goodwin-xray"},
	}
	for _, c := range cmds {
		cmd := exec.Command(c[0], c[1:]...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %v (%s)", strings.Join(c, " "), err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func Listening(port int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func Version(bin string) string {
	out, err := exec.Command(bin, "version").Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(line)
}
