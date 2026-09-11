package execcmd

import (
	"context"
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
