#!/usr/bin/env bash
# Bootstrap a VPN *node* (agent only). Does not install the admin panel.
# Requires root on Debian/Ubuntu. Installs Node.js, then runs tools/install_vpn_node.mjs
#
#   curl -fsSL https://raw.githubusercontent.com/AlexGalitsky/goodwin-vpn-backend/main/tools/bootstrap_vpn_node.sh | sudo bash
#
# Optional env:
#   REPO          git URL (default: this GitHub repo)
#   BRANCH        git branch (default: main)
#   CERT_DOMAIN   e.g. titan.goodwin.website — issue Let's Encrypt cert (standalone :80)
#   ISSUE_CERT    1/0 (default 1 if CERT_DOMAIN is set)
#   AGENT_LISTEN  default 0.0.0.0:19400
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
WORKDIR="${WORKDIR:-/opt/goodwin-vpn-src}"

if [[ -n "${GITHUB_TOKEN:-}" && "${REPO}" == https://github.com/* ]]; then
  REPO="https://${GITHUB_TOKEN}@github.com/${REPO#https://github.com/}"
fi

apt-get update -y
apt-get install -y --no-install-recommends ca-certificates curl git tar xz-utils

if ! command -v node >/dev/null 2>&1 || ! node -e 'process.exit(Number(process.versions.node.split(".")[0])<20)'; then
  curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
  apt-get install -y nodejs
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

node "${WORKDIR}/tools/install_vpn_node.mjs" --bin "${agent_bin}" --listen "${AGENT_LISTEN}"

if [[ -z "${ISSUE_CERT}" && -n "${CERT_DOMAIN}" ]]; then
  ISSUE_CERT=1
fi
if [[ "${ISSUE_CERT}" == "1" && -n "${CERT_DOMAIN}" ]]; then
  apt-get install -y certbot
  certbot certonly --standalone --non-interactive --agree-tos \
    --register-unsafely-without-email -d "${CERT_DOMAIN}"
  echo "TLS cert: /etc/letsencrypt/live/${CERT_DOMAIN}/"
fi

echo
echo "Node hostname for Hy2/TT later: ${CERT_DOMAIN:-titan.goodwin.website}"
echo "Control port 19400 must be reachable from the plane host."
