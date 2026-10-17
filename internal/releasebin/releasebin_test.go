package releasebin

import (
	"os"
	"testing"
)

func TestVerifySHA256(t *testing.T) {
	if err := VerifySHA256([]byte("abc"), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"); err != nil {
		t.Fatal(err)
	}
	if err := VerifySHA256([]byte("abc"), "deadbeef"); err == nil {
		t.Fatal("expected mismatch")
	}
	if err := VerifySHA256([]byte("abc"), ""); err == nil {
		t.Fatal("expected missing")
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/bin"
	if err := WriteFile(path, []byte("ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("v2"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "v2" {
		t.Fatalf("got %q", raw)
	}
}
