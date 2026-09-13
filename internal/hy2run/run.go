package hy2run

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"website.goodwin.vpn/plane/internal/releasebin"
	"website.goodwin.vpn/plane/internal/sysd"
)

const (
	Release     = "2.12.2"
	releaseBase = "https://github.com/apernet/hysteria/releases/download/app/v" + Release + "/"
)

var binSHA256 = map[string]string{
	"hysteria-linux-amd64": "6493dfffd55b5883f64c76c63880ecc32988f0c568c9ca9014907877b4d55f94",
	"hysteria-linux-arm64": "ebfacc1ec3a0edfd742cd68ce17f292a6092e606b9d11f99b035c1d888f3d709",
}

func BinName() string {
	if runtime.GOARCH == "arm64" {
		return "hysteria-linux-arm64"
	}
	return "hysteria-linux-amd64"
}

func EnsureBinary(ctx context.Context, dir string) (string, error) {
	bin := filepath.Join(dir, "hysteria")
	pin := filepath.Join(dir, "PIN")
	if st, err := os.Stat(bin); err == nil && st.Mode().IsRegular() && releasebin.PinMatches(pin, Release) {
		return bin, nil
	}
	name := BinName()
	sum, ok := binSHA256[name]
	if !ok {
		return "", fmt.Errorf("no sha256 for %s", name)
	}
	raw, err := releasebin.Get(ctx, releaseBase+name, sum, 80<<20)
	if err != nil {
		return "", fmt.Errorf("download hysteria: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := releasebin.WriteFile(bin, raw, 0o755); err != nil {
		return "", err
	}
	if err := releasebin.WriteFile(pin, []byte(Release+"\n"), 0o644); err != nil {
		return "", err
	}
	return bin, nil
}

func WriteFile(path string, raw []byte, mode os.FileMode) error {
	return releasebin.WriteFile(path, raw, mode)
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
%s
[Install]
WantedBy=multi-user.target
`, bin, cfgPath, sysd.Extra)
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
	if port <= 0 {
		return false
	}
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err == nil {
		_ = c.Close()
		return false
	}
	return addrInUse(err)
}

func addrInUse(err error) bool {
	var op *net.OpError
	if errors.As(err, &op) {
		err = op.Err
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.EADDRINUSE
	}
	return strings.Contains(strings.ToLower(err.Error()), "address already in use")
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
