package hy2run

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const releaseBase = "https://github.com/apernet/hysteria/releases/latest/download/"

func BinName() string {
	if runtime.GOARCH == "arm64" {
		return "hysteria-linux-arm64"
	}
	return "hysteria-linux-amd64"
}

func EnsureBinary(ctx context.Context, dir string) (string, error) {
	bin := filepath.Join(dir, "hysteria")
	if st, err := os.Stat(bin); err == nil && st.Mode().IsRegular() {
		return bin, nil
	}
	url := releaseBase + BinName()
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
		return "", fmt.Errorf("download hysteria: %s", res.Status)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 80<<20))
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(bin, raw, 0o755); err != nil {
		return "", err
	}
	return bin, nil
}

func WriteFile(path string, raw []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, raw, mode)
}

func InstallUnit(bin, cfgPath string) error {
	unit := fmt.Sprintf(`[Unit]
Description=Goodwin Hysteria2
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s server -c %s
Restart=on-failure
RestartSec=2
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
`, bin, cfgPath)
	if err := os.WriteFile("/etc/systemd/system/goodwin-hysteria.service", []byte(unit), 0o644); err != nil {
		return err
	}
	cmds := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "--now", "goodwin-hysteria"},
		{"systemctl", "restart", "goodwin-hysteria"},
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

func StopUnit() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	cmd := exec.Command("systemctl", "disable", "--now", "goodwin-hysteria")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.ToLower(string(out) + " " + err.Error())
		if strings.Contains(msg, "not found") || strings.Contains(msg, "not loaded") || strings.Contains(msg, "could not be found") {
			return nil
		}
		return fmt.Errorf("systemctl disable goodwin-hysteria: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func Listening(port int) bool {
	if runtime.GOOS == "linux" && exec.Command("systemctl", "is-active", "--quiet", "goodwin-hysteria").Run() == nil {
		return true
	}
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		return true
	}
	_ = c.Close()
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
