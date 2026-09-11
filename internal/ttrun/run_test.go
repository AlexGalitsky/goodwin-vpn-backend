package ttrun

import (
	"strings"
	"testing"
)

func TestArchiveName(t *testing.T) {
	n := ArchiveName()
	if !strings.Contains(n, "trusttunnel-v"+Version) || !strings.HasSuffix(n, ".tar.gz") {
		t.Fatalf("archive %s", n)
	}
}
