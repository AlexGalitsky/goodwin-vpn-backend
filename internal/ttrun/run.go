package ttrun

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"website.goodwin.vpn/plane/internal/sysd"
)

const (
	Version = "1.1.0"
	release = "https://github.com/TrustTunnel/TrustTunnel/releases/download/v" + Version + "/"
)

func ArchiveName() string {
	if runtime.GOARCH == "arm64" {
		return "trusttunnel-v" + Version + "-linux-aarch64.tar.gz"
	}
	return "trusttunnel-v" + Version + "-linux-x86_64.tar.gz"
}

func EnsureBinary(ctx context.Context, dir string) (string, error) {
	bin := filepath.Join(dir, "trusttunnel_endpoint")
	if st, err := os.Stat(bin); err == nil && st.Mode().IsRegular() {
		return bin, nil
	}
	url := release + ArchiveName()
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
		return "", fmt.Errorf("download trusttunnel: %s", res.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 80<<20))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := extractEndpoint(raw, bin); err != nil {
		return "", err
	}
	_ = os.Chmod(bin, 0o755)
	return bin, nil
}

func extractEndpoint(archive []byte, dest string) error {
	gr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.FileInfo().IsDir() || filepath.Base(hdr.Name) != "trusttunnel_endpoint" {
			continue
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, io.LimitReader(tr, 80<<20))
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
	return fmt.Errorf("trusttunnel_endpoint missing in archive")
}

func WriteFile(path string, raw []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, raw, mode)
}

func UnitFile(bin, workDir, vpnPath, hostsPath string) string {
	return fmt.Sprintf(`[Unit]
Description=Goodwin TrustTunnel
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s %s %s
Restart=on-failure
RestartSec=2
LimitNOFILE=1048576
%s
[Install]
WantedBy=multi-user.target
`, workDir, bin, vpnPath, hostsPath, sysd.Extra)
}

func InstallUnit(bin, workDir, vpnPath, hostsPath string) error {
	if err := os.WriteFile("/etc/systemd/system/goodwin-trusttunnel.service", []byte(UnitFile(bin, workDir, vpnPath, hostsPath)), 0o644); err != nil {
		return err
	}
	cmds := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "--now", "goodwin-trusttunnel"},
		{"systemctl", "restart", "goodwin-trusttunnel"},
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
	cmd := exec.Command("systemctl", "disable", "--now", "goodwin-trusttunnel")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.ToLower(string(out) + " " + err.Error())
		if strings.Contains(msg, "not found") || strings.Contains(msg, "not loaded") || strings.Contains(msg, "could not be found") {
			return nil
		}
		return fmt.Errorf("systemctl disable goodwin-trusttunnel: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func Listening() bool {
	return runtime.GOOS == "linux" && exec.Command("systemctl", "is-active", "--quiet", "goodwin-trusttunnel").Run() == nil
}

func VersionBin(bin string) string {
	out, err := exec.Command(bin, "-v").Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(line)
}

func MintDeeplink(ctx context.Context, bin, vpnPath, hostsPath, username, advertise, displayName string) (string, error) {
	args := []string{vpnPath, hostsPath, "-c", username, "-a", advertise, "--format", "deeplink"}
	if strings.TrimSpace(displayName) != "" {
		args = append(args, "--name", displayName)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = filepath.Dir(vpnPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("deeplink %s: %v (%s)", username, err, strings.TrimSpace(string(out)))
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "tt://") {
			return line, nil
		}
	}
	return "", fmt.Errorf("deeplink %s: no tt:// in output (%s)", username, strings.TrimSpace(string(out)))
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
