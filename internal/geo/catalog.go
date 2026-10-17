package geo

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
)

// FeatureGeoPacks is advertised on GET /gw/v1/service when packs are served.
const FeatureGeoPacks = "geo-packs"

const (
	maxPackBytes   = 512 << 10
	maxPackEntries = 2000
	packURLPrefix  = "/gw/v1/geo/packs/"
)

//go:embed allowlist/catalog.json allowlist/ads.json
var allowlistFS embed.FS

var hostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)

// Manifest is GET /gw/v1/geo/manifest.
type Manifest struct {
	Version string     `json:"version"`
	Packs   []PackMeta `json:"packs"`
}

// PackMeta is one entry in the manifest. SHA256 is of the exact pack body bytes.
type PackMeta struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

// PackBody is GET /gw/v1/geo/packs/{id}.
type PackBody struct {
	ID       string   `json:"id"`
	Domains  []string `json:"domains"`
	Suffixes []string `json:"suffixes"`
	CIDRs    []string `json:"cidrs"`
}

type allowlistFile struct {
	Domains  []string `json:"domains"`
	Suffixes []string `json:"suffixes"`
	CIDRs    []string `json:"cidrs"`
}

type catalogFile struct {
	Version string `json:"version"`
}

// Catalog is the compiled, hashed pack set served by the plane.
type Catalog struct {
	Manifest Manifest
	bodies   map[string][]byte
	meta     map[string]PackMeta
}

var defaultCatalog = mustLoad()

func mustLoad() *Catalog {
	c, err := Load()
	if err != nil {
		panic("geo allowlist: " + err.Error())
	}
	return c
}

// Default is the embedded ads pack compiled from allowlist/.
func Default() *Catalog { return defaultCatalog }

// HasPacks is true when GET /service should advertise geo-packs.
func HasPacks() bool { return len(defaultCatalog.Manifest.Packs) > 0 }

// ServiceFeatures is the features[] slice for GET /gw/v1/service.
func ServiceFeatures() []string {
	if !HasPacks() {
		return []string{}
	}
	return []string{FeatureGeoPacks}
}

// Load compiles embedded allowlists into canonical JSON bodies + sha256.
func Load() (*Catalog, error) {
	rawCat, err := allowlistFS.ReadFile("allowlist/catalog.json")
	if err != nil {
		return nil, err
	}
	var cat catalogFile
	if err := json.Unmarshal(rawCat, &cat); err != nil {
		return nil, fmt.Errorf("catalog.json: %w", err)
	}
	version := strings.TrimSpace(cat.Version)
	if version == "" {
		return nil, fmt.Errorf("catalog.json: empty version")
	}

	rawAds, err := allowlistFS.ReadFile("allowlist/ads.json")
	if err != nil {
		return nil, err
	}
	var src allowlistFile
	if err := json.Unmarshal(rawAds, &src); err != nil {
		return nil, fmt.Errorf("ads.json: %w", err)
	}
	body, meta, err := CompilePack("ads", src.Domains, src.Suffixes, src.CIDRs)
	if err != nil {
		return nil, err
	}
	return newCatalog(version, map[string][]byte{"ads": body}, []PackMeta{meta})
}

func newCatalog(version string, bodies map[string][]byte, metas []PackMeta) (*Catalog, error) {
	c := &Catalog{
		Manifest: Manifest{Version: version, Packs: metas},
		bodies:   bodies,
		meta:     map[string]PackMeta{},
	}
	for _, m := range metas {
		c.meta[m.ID] = m
	}
	return c, nil
}

// Pack returns the canonical JSON body for id.
func (c *Catalog) Pack(id string) ([]byte, PackMeta, bool) {
	body, ok := c.bodies[id]
	if !ok {
		return nil, PackMeta{}, false
	}
	return body, c.meta[id], true
}

// CompilePack canonicalizes allowlist entries into the served JSON body.
func CompilePack(id string, domains, suffixes, cidrs []string) ([]byte, PackMeta, error) {
	if err := validatePackID(id); err != nil {
		return nil, PackMeta{}, err
	}
	out := PackBody{
		ID:       id,
		Domains:  []string{},
		Suffixes: []string{},
		CIDRs:    []string{},
	}
	seenDom := map[string]struct{}{}
	seenSuf := map[string]struct{}{}
	seenCIDR := map[string]struct{}{}

	for _, raw := range domains {
		h, err := normalizeHost(raw, false)
		if err != nil {
			return nil, PackMeta{}, fmt.Errorf("pack %s domain %q: %w", id, raw, err)
		}
		if _, ok := seenDom[h]; ok {
			continue
		}
		seenDom[h] = struct{}{}
		out.Domains = append(out.Domains, h)
	}
	for _, raw := range suffixes {
		h, err := normalizeHost(raw, true)
		if err != nil {
			return nil, PackMeta{}, fmt.Errorf("pack %s suffix %q: %w", id, raw, err)
		}
		if _, ok := seenSuf[h]; ok {
			continue
		}
		seenSuf[h] = struct{}{}
		out.Suffixes = append(out.Suffixes, h)
	}
	for _, raw := range cidrs {
		n, err := normalizeCIDR(raw)
		if err != nil {
			return nil, PackMeta{}, fmt.Errorf("pack %s cidr %q: %w", id, raw, err)
		}
		if _, ok := seenCIDR[n]; ok {
			continue
		}
		seenCIDR[n] = struct{}{}
		out.CIDRs = append(out.CIDRs, n)
	}
	sort.Strings(out.Domains)
	sort.Strings(out.Suffixes)
	sort.Strings(out.CIDRs)
	n := len(out.Domains) + len(out.Suffixes) + len(out.CIDRs)
	if n == 0 {
		return nil, PackMeta{}, fmt.Errorf("pack %s: empty allowlist", id)
	}
	if n > maxPackEntries {
		return nil, PackMeta{}, fmt.Errorf("pack %s: %d entries (max %d)", id, n, maxPackEntries)
	}
	body, err := json.Marshal(out)
	if err != nil {
		return nil, PackMeta{}, err
	}
	if len(body) > maxPackBytes {
		return nil, PackMeta{}, fmt.Errorf("pack %s: %d bytes (max %d)", id, len(body), maxPackBytes)
	}
	sum := sha256.Sum256(body)
	meta := PackMeta{
		ID:     id,
		URL:    packURLPrefix + id,
		SHA256: hex.EncodeToString(sum[:]),
		Bytes:  len(body),
	}
	return body, meta, nil
}

func validatePackID(id string) error {
	if id == "" || len(id) > 32 {
		return fmt.Errorf("invalid pack id %q", id)
	}
	for _, c := range id {
		ok := c == '-' || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if !ok {
			return fmt.Errorf("invalid pack id %q", id)
		}
	}
	if id[0] < 'a' || id[0] > 'z' {
		return fmt.Errorf("invalid pack id %q", id)
	}
	return nil
}

func normalizeHost(raw string, suffix bool) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.TrimPrefix(s, ".")
	if s == "" {
		return "", fmt.Errorf("empty")
	}
	if strings.ContainsAny(s, " /:*") || strings.Contains(s, "geosite:") || strings.Contains(s, "geoip:") {
		return "", fmt.Errorf("forbidden pattern")
	}
	if net.ParseIP(s) != nil {
		return "", fmt.Errorf("ip belongs in cidrs")
	}
	if !hostRe.MatchString(s) {
		return "", fmt.Errorf("not a hostname")
	}
	if suffix {
		return "." + s, nil
	}
	return s, nil
}

func normalizeCIDR(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		return "", err
	}
	return n.String(), nil
}
