#!/usr/bin/env bash
# Bootstrap a VPN *node* (agent only). Does not install the admin panel.
# Requires root on Debian/Ubuntu. Installs Node.js, then runs tools/install_vpn_node.mjs
#
#   curl -fsSL https://raw.githubusercontent.com/AlexGalitsky/goodwin-vpn-backend/main/tools/bootstrap_vpn_node.sh | sudo bash
#
# Optional env:
#   REPO          git URL (default: this GitHub repo)
#   BRANCH        git branch (default: main)
#   CERT_DOMAIN   e.g. gw-node1.goodwin.website — Let's Encrypt via standalone :80
#   ISSUE_CERT    1/0 (default 1 if CERT_DOMAIN is set)
#   AGENT_LISTEN  default 0.0.0.0:19400 (needs SATURN_IP)
#   SATURN_IP     plane IPv4 allowed to :19400 (required for public listen)
#
# Ubuntu 26: Node 22 comes from NodeSource when that repo knows the release,
# otherwise from the official nodejs.org tarball. nftables is installed so
# :19400 is limited to SATURN_IP.
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "run as root (sudo)" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
REPO="${REPO:-https://github.com/AlexGalitsky/goodwin-vpn-backend.git}"
BRANCH="${BRANCH:-main}"
CERT_DOMAIN="${CERT_DOMAIN:-}"
ISSUE_CERT="${ISSUE_CERT:-}"
AGENT_LISTEN="${AGENT_LISTEN:-0.0.0.0:19400}"
SATURN_IP="${SATURN_IP:-}"
WORKDIR="${WORKDIR:-/opt/goodwin-vpn-src}"

if [[ -n "${GITHUB_TOKEN:-}" && "${REPO}" == https://github.com/* ]]; then
  REPO="https://${GITHUB_TOKEN}@github.com/${REPO#https://github.com/}"
fi

apt-get update -y
apt-get install -y --no-install-recommends ca-certificates curl git tar xz-utils nftables

node_ok() {
  command -v node >/dev/null 2>&1 && node -e 'process.exit(Number(process.versions.node.split(".")[0])<20)'
}

if ! node_ok; then
  if curl -fsSL https://deb.nodesource.com/setup_22.x | bash -; then
    apt-get install -y nodejs || true
  fi
fi
if ! node_ok; then
  case "$(dpkg --print-architecture)" in
    amd64) nodearch=x64 ;;
    arm64) nodearch=arm64 ;;
    *) echo "unsupported arch: $(dpkg --print-architecture)" >&2; exit 1 ;;
  esac
  tarball="$(curl -fsSL https://nodejs.org/dist/latest-v22.x/ | grep -oE "node-v22\\.[0-9]+\\.[0-9]+-linux-${nodearch}\\.tar\\.xz" | head -n1 || true)"
  if [[ -z "${tarball}" ]]; then
    echo "could not resolve Node.js 22 tarball" >&2
    exit 1
  fi
  curl -fsSL "https://nodejs.org/dist/latest-v22.x/${tarball}" -o /tmp/node.tar.xz
  tar -C /usr/local --strip-components=1 -xJf /tmp/node.tar.xz
  rm -f /tmp/node.tar.xz
  hash -r
fi
node --version
npm --version >/dev/null

arch="$(dpkg --print-architecture)"
case "${arch}" in
  amd64) goarch=amd64 ;;
  arm64) goarch=arm64 ;;
  *) echo "unsupported arch: ${arch}" >&2; exit 1 ;;
esac

if ! command -v go >/dev/null 2>&1 || ! go version | grep -Eq 'go1\.(2[3-9]|[3-9][0-9])'; then
  gover="$(curl -fsSL https://go.dev/VERSION?m=text | head -n1)"
  curl -fsSL "https://go.dev/dl/${gover}.linux-${goarch}.tar.gz" -o /tmp/go.tgz
  rm -rf /usr/local/go
  tar -C /usr/local -xzf /tmp/go.tgz
  rm -f /tmp/go.tgz
  ln -sfn /usr/local/go/bin/go /usr/local/bin/go
fi
go version

rm -rf "${WORKDIR}"
git clone --depth 1 --branch "${BRANCH}" "${REPO}" "${WORKDIR}"
cd "${WORKDIR}"

agent_bin=/tmp/goodwin-vpn-agent
CGO_ENABLED=0 go build -o "${agent_bin}" ./cmd/agent

node "${WORKDIR}/tools/install_vpn_node.mjs" --bin "${agent_bin}" --listen "${AGENT_LISTEN}" ${SATURN_IP:+--allow-from "${SATURN_IP}"}

if [[ -z "${ISSUE_CERT}" && -n "${CERT_DOMAIN}" ]]; then
  ISSUE_CERT=1
fi
if [[ "${ISSUE_CERT}" == "1" && -n "${CERT_DOMAIN}" ]]; then
  apt-get install -y certbot
  certbot certonly --standalone --non-interactive --agree-tos \
    --register-unsafely-without-email -d "${CERT_DOMAIN}"
  mkdir -p /etc/letsencrypt/renewal-hooks/deploy
  cat > /etc/letsencrypt/renewal-hooks/deploy/goodwin-vpn <<'EOF'
#!/bin/sh
systemctl try-restart goodwin-hysteria.service goodwin-trusttunnel.service >/dev/null 2>&1 || true
exit 0
EOF
  chmod 755 /etc/letsencrypt/renewal-hooks/deploy/goodwin-vpn
  systemctl enable --now certbot.timer >/dev/null 2>&1 || true
  echo "TLS cert: /etc/letsencrypt/live/${CERT_DOMAIN}/"
fi

echo
echo "Node hostname for Hy2/TT later: ${CERT_DOMAIN:-titan.goodwin.website}"
echo "Control port 19400 must be reachable from the plane host."
