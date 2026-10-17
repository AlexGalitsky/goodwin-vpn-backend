package lehook

import (
	"os"
	"path/filepath"
	"runtime"

	"website.goodwin.vpn/plane/internal/releasebin"
)

const Path = "/etc/letsencrypt/renewal-hooks/deploy/goodwin-vpn"

const Script = `#!/bin/sh
systemctl try-restart goodwin-hysteria.service goodwin-trusttunnel.service >/dev/null 2>&1 || true
exit 0
`

func Install() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(Path), 0o755); err != nil {
		return err
	}
	return releasebin.WriteFile(Path, []byte(Script), 0o755)
}
