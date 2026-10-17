package geo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultAdsPack(t *testing.T) {
	c := Default()
	if c.Manifest.Version != "2026.09.12" {
		t.Fatalf("version %q", c.Manifest.Version)
	}
	if !HasPacks() {
		t.Fatal("expected ads pack")
	}
	feats := ServiceFeatures()
	if len(feats) != 1 || feats[0] != FeatureGeoPacks {
		t.Fatalf("features %v", feats)
	}
	body, meta, ok := c.Pack("ads")
	if !ok {
		t.Fatal("missing ads")
	}
	if meta.URL != "/gw/v1/geo/packs/ads" {
		t.Fatalf("url %q", meta.URL)
	}
	if meta.Bytes != len(body) {
		t.Fatalf("bytes %d != %d", meta.Bytes, len(body))
	}
	sum := sha256.Sum256(body)
	if meta.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("sha256 mismatch %s", meta.SHA256)
	}
	if len(body) > maxPackBytes {
		t.Fatalf("pack too large: %d", len(body))
	}
	var pack PackBody
	if err := json.Unmarshal(body, &pack); err != nil {
		t.Fatal(err)
	}
	if pack.ID != "ads" {
		t.Fatalf("id %q", pack.ID)
	}
	if pack.Domains == nil || pack.Suffixes == nil || pack.CIDRs == nil {
		t.Fatal("arrays must not be null")
	}
	if len(pack.CIDRs) != 0 {
		t.Fatalf("ads cidrs should be empty: %v", pack.CIDRs)
	}
	if len(pack.Suffixes) < 10 {
		t.Fatalf("too few suffixes: %d", len(pack.Suffixes))
	}
	if !contains(pack.Suffixes, ".doubleclick.net") {
		t.Fatal("missing .doubleclick.net (client tiny-list superset)")
	}
	if !contains(pack.Suffixes, ".googleadservices.com") {
		t.Fatal("missing .googleadservices.com")
	}
	if contains(pack.Suffixes, ".google.com") || contains(pack.Suffixes, ".facebook.com") {
		t.Fatal("suffix too broad")
	}
	if !sortedUnique(pack.Suffixes) || !sortedUnique(pack.Domains) || !sortedUnique(pack.CIDRs) {
		t.Fatal("arrays must be sorted unique")
	}
}

func TestCompilePackRejectsGeositeAndEmpty(t *testing.T) {
	_, _, err := CompilePack("ads", nil, nil, nil)
	if err == nil {
		t.Fatal("empty pack")
	}
	_, _, err = CompilePack("ads", nil, []string{"geosite:ads"}, nil)
	if err == nil {
		t.Fatal("geosite")
	}
	_, _, err = CompilePack("Ads", nil, []string{"doubleclick.net"}, nil)
	if err == nil {
		t.Fatal("uppercase id")
	}
	_, _, err = CompilePack("ads", nil, []string{"*.doubleclick.net"}, nil)
	if err == nil {
		t.Fatal("wildcard")
	}
	_, _, err = CompilePack("ads", []string{"8.8.8.8"}, nil, nil)
	if err == nil {
		t.Fatal("ip as domain")
	}
}

func TestCompilePackNormalizes(t *testing.T) {
	body, meta, err := CompilePack("ads", []string{" Example.COM "}, []string{"doubleclick.net", ".DoubleClick.net"}, []string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	var pack PackBody
	if err := json.Unmarshal(body, &pack); err != nil {
		t.Fatal(err)
	}
	if len(pack.Domains) != 1 || pack.Domains[0] != "example.com" {
		t.Fatalf("domains %v", pack.Domains)
	}
	if len(pack.Suffixes) != 1 || pack.Suffixes[0] != ".doubleclick.net" {
		t.Fatalf("suffixes %v", pack.Suffixes)
	}
	if len(pack.CIDRs) != 1 || pack.CIDRs[0] != "10.0.0.0/8" {
		t.Fatalf("cidrs %v", pack.CIDRs)
	}
	if meta.SHA256 == "" || meta.Bytes != len(body) {
		t.Fatalf("meta %+v", meta)
	}
}

func TestPackLookupMissing(t *testing.T) {
	_, _, ok := Default().Pack("nope")
	if ok {
		t.Fatal("unknown pack")
	}
}

func TestNodeAllowlistCheckMatchesGo(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not in PATH")
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..")
	cmd := exec.Command("node", "tools/build_geo_packs.mjs", "--check")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node --check: %v\n%s", err, out)
	}
	_, meta, ok := Default().Pack("ads")
	if !ok {
		t.Fatal("missing ads")
	}
	if !strings.Contains(string(out), "sha256="+meta.SHA256) {
		t.Fatalf("node sha256 != go:\n%s\ngo %s", out, meta.SHA256)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func sortedUnique(xs []string) bool {
	for i := 1; i < len(xs); i++ {
		if xs[i-1] >= xs[i] {
			return false
		}
	}
	return true
}
