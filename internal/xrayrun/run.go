package xrayrun

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"website.goodwin.vpn/plane/internal/releasebin"
	"website.goodwin.vpn/plane/internal/sysd"
)

const (
	Release     = "26.3.27"
	xrayRelease = "https://github.com/XTLS/Xray-core/releases/download/v" + Release + "/"
)

var zipSHA256 = map[string]string{
	"Xray-linux-64.zip":        "23cd9af937744d97776ee35ecad4972cf4b2109d1e0fe6be9930467608f7c8ae",
	"Xray-linux-arm64-v8a.zip": "4d30283ae614e3057f730f67cd088a42be6fdf91f8639d82cb69e48cde80413c",
}

func ZipName() string {
	if runtime.GOARCH == "arm64" {
		return "Xray-linux-arm64-v8a.zip"
	}
	return "Xray-linux-64.zip"
}

func EnsureBinary(ctx context.Context, dir string) (string, error) {
	bin := filepath.Join(dir, "xray")
	pin := filepath.Join(dir, "PIN")
	if st, err := os.Stat(bin); err == nil && st.Mode().IsRegular() && releasebin.PinMatches(pin, Release) {
		return bin, nil
	}
	name := ZipName()
	sum, ok := zipSHA256[name]
	if !ok {
		return "", fmt.Errorf("no sha256 for %s", name)
	}
	raw, err := releasebin.Get(ctx, xrayRelease+name, sum, 80<<20)
	if err != nil {
		return "", fmt.Errorf("download xray: %w", err)
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
		mode := os.FileMode(0o644)
		if base == "xray" {
			mode = 0o755
		}
		if err := writeZipFile(f, dst, mode); err != nil {
			return "", err
		}
	}
	if _, err := os.Stat(bin); err != nil {
		return "", fmt.Errorf("xray binary missing after unzip")
	}
	if err := releasebin.WriteFile(pin, []byte(Release+"\n"), 0o644); err != nil {
		return "", err
	}
	return bin, nil
}

func writeZipFile(f *zip.File, dst string, mode os.FileMode) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, 80<<20))
	if err != nil {
		return err
	}
	return releasebin.WriteFile(dst, raw, mode)
}

func WriteConfig(path string, raw []byte) error {
	return releasebin.WriteFile(path, raw, 0o600)
}

func ReadPortFile(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return 0
	}
	return n
}

func UnitFile(bin, cfgPath string) string {
	return fmt.Sprintf(`[Unit]
Description=Goodwin Xray (VLESS REALITY)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s run -c %s
Restart=on-failure
RestartSec=2
LimitNOFILE=1048576
%s
[Install]
WantedBy=multi-user.target
`, bin, cfgPath, sysd.Extra)
}

func InstallUnit(bin, cfgPath string) error {
	if err := os.WriteFile("/etc/systemd/system/goodwin-xray.service", []byte(UnitFile(bin, cfgPath)), 0o644); err != nil {
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
	if port <= 0 {
		return false
	}
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 400*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func ListeningWait(port, tries int) bool {
	if tries < 1 {
		tries = 1
	}
	for i := 0; i < tries; i++ {
		if Listening(port) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func Version(bin string) string {
	out, err := exec.Command(bin, "version").Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(line)
}
