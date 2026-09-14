package execcmd

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestRunEcho(t *testing.T) {
	res, err := Run(context.Background(), "echo hello", 2*time.Second, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "hello") {
		t.Fatalf("%+v", res)
	}
}

func TestRunDisabled(t *testing.T) {
	_, err := Run(context.Background(), "echo x", time.Second, false)
	if err != ErrDisabled {
		t.Fatalf("got %v", err)
	}
}

func TestRunKillsProcessGroup(t *testing.T) {
	start := time.Now()
	res, err := Run(context.Background(), "sleep 30", 200*time.Millisecond, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode == 0 {
		t.Fatalf("expected killed, got %+v", res)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("process group still running after %s", time.Since(start))
	}
}

func TestRunKillsProcessGroupChildren(t *testing.T) {
	if _, err := exec.LookPath("pgrep"); err != nil {
		t.Skip("pgrep not available")
	}
	// Unique argv so pgrep cannot match this test binary. sh stays parent of sleep.
	_, err := Run(context.Background(), "sleep 91 & wait", 400*time.Millisecond, true)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	out, _ := exec.Command("pgrep", "-f", "sleep 91").Output()
	if ids := strings.TrimSpace(string(out)); ids != "" {
		t.Fatalf("child still running after process-group kill: %s", ids)
	}
}

func TestRunFillsHomeWhenUnset(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("GOCACHE", "")
	t.Setenv("GOPATH", "")
	res, err := Run(context.Background(), `printf '%s' "$HOME"`, 2*time.Second, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 || strings.TrimSpace(res.Stdout) == "" {
		t.Fatalf("expected HOME, got %+v", res)
	}
}
