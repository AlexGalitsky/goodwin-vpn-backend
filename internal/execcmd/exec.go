package execcmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

const maxOut = 1 << 20

var ErrDisabled = errors.New("exec disabled")

type Result struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

func Run(ctx context.Context, shell string, timeout time.Duration, allowed bool) (Result, error) {
	if !allowed {
		return Result{}, ErrDisabled
	}
	shell = strings.TrimSpace(shell)
	if shell == "" {
		return Result{}, errors.New("empty command")
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if timeout > 120*time.Second {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", shell)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = withBuildEnv(os.Environ())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := Result{
		Stdout:   clip(stdout.String()),
		Stderr:   clip(stderr.String()),
		ExitCode: 0,
	}
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			res.ExitCode = -1
			if res.Stderr == "" {
				res.Stderr = "timeout"
			}
			return res, nil
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.ExitCode = ee.ExitCode()
			return res, nil
		}
		return res, err
	}
	return res, nil
}

// systemd services often have no HOME; go then refuses to build.
func withBuildEnv(env []string) []string {
	have := map[string]string{}
	out := make([]string, 0, len(env)+8)
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		have[k] = v
		if v != "" {
			out = append(out, e)
		}
	}
	home := have["HOME"]
	if home == "" {
		if os.Getuid() == 0 {
			home = "/root"
		} else {
			home = os.TempDir()
		}
		out = append(out, "HOME="+home)
	}
	cache := have["XDG_CACHE_HOME"]
	if cache == "" {
		cache = filepath.Join(home, ".cache")
		out = append(out, "XDG_CACHE_HOME="+cache)
	}
	if have["GOCACHE"] == "" {
		out = append(out, "GOCACHE="+filepath.Join(cache, "go-build"))
	}
	if have["GOPATH"] == "" {
		out = append(out, "GOPATH="+filepath.Join(home, "go"))
	}
	if have["PATH"] == "" {
		out = append(out, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	}
	return out
}

func clip(s string) string {
	if len(s) > maxOut {
		s = s[:maxOut]
	}
	if !utf8.ValidString(s) {
		return strings.ToValidUTF8(s, "�")
	}
	return s
}
