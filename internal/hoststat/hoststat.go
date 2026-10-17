package hoststat

import (
	"bufio"
	"crypto/x509"
	"encoding/pem"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Cert struct {
	Name     string `json:"name"`
	NotAfter string `json:"not_after,omitempty"`
	DaysLeft int    `json:"days_left"`
}

type Snap struct {
	Load1     float64
	CPUN      int
	MemUsed   int64
	MemTotal  int64
	DiskUsed  int64
	DiskTotal int64
}

func Snapshot(root string) Snap {
	s := Snap{CPUN: runtime.NumCPU(), Load1: load1()}
	s.MemUsed, s.MemTotal = memUsage()
	s.DiskUsed, s.DiskTotal = diskUsage(root)
	return s
}

func Certs(liveDir string, now time.Time) []Cert {
	if strings.TrimSpace(liveDir) == "" {
		liveDir = "/etc/letsencrypt/live"
	}
	ents, err := os.ReadDir(liveDir)
	if err != nil {
		return nil
	}
	var out []Cert
	for _, e := range ents {
		name := e.Name()
		if name == "" || name[0] == '.' || name == "README" {
			continue
		}
		path := filepath.Join(liveDir, name)
		st, err := os.Stat(path)
		if err != nil || !st.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(path, "fullchain.pem"))
		if err != nil {
			continue
		}
		block, _ := pem.Decode(raw)
		if block == nil {
			continue
		}
		crt, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		days := int(math.Floor(crt.NotAfter.Sub(now).Hours() / 24))
		out = append(out, Cert{
			Name:     name,
			NotAfter: crt.NotAfter.UTC().Format(time.RFC3339),
			DaysLeft: days,
		})
	}
	return out
}

func load1() float64 {
	raw, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0
	}
	field, _, _ := strings.Cut(strings.TrimSpace(string(raw)), " ")
	v, err := strconv.ParseFloat(field, 64)
	if err != nil {
		return 0
	}
	return v
}

func memUsage() (used, total int64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	var avail int64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total = parseKiB(line) * 1024
		case strings.HasPrefix(line, "MemAvailable:"):
			avail = parseKiB(line) * 1024
		}
	}
	if total > 0 && avail >= 0 && avail <= total {
		used = total - avail
	}
	return used, total
}

func parseKiB(line string) int64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	n, _ := strconv.ParseInt(fields[1], 10, 64)
	return n
}

func diskUsage(root string) (used, total int64) {
	if root == "" {
		root = "/"
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(root, &st); err != nil {
		return 0, 0
	}
	bsize := int64(st.Bsize)
	if bsize <= 0 {
		return 0, 0
	}
	total = int64(st.Blocks) * bsize
	free := int64(st.Bavail) * bsize
	if total > 0 && free >= 0 && free <= total {
		used = total - free
	}
	return used, total
}
