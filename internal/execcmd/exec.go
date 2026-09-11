package execcmd

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
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

func clip(s string) string {
	if len(s) > maxOut {
		s = s[:maxOut]
	}
	if !utf8.ValidString(s) {
		return strings.ToValidUTF8(s, "�")
	}
	return s
}
