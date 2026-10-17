#!/usr/bin/env bash
# Bootstrap / update the control plane (API + admin) on a dedicated VPS.
# Do not run this on an exit node (titan). Idempotent: clone or git pull, then rebuild.
#
#   curl -fsSL https://raw.githubusercontent.com/AlexGalitsky/goodwin-vpn-backend/main/tools/bootstrap_vpn_plane.sh \
#     | sudo env CERT_DOMAIN=saturn.goodwin.website bash
#
# Optional env:
#   REPO BRANCH WORKDIR
#   CERT_DOMAIN     public hostname for Caddy + PUBLIC_SUB_BASE
#   ADMIN_PASSWORD  kept if /etc/goodwin-vpn-plane.env already exists
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "run as root (sudo)" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
export HOME="${HOME:-/root}"
export GOCACHE="${GOCACHE:-${HOME}/.cache/go-build}"
mkdir -p "${GOCACHE}"

REPO="${REPO:-https://github.com/AlexGalitsky/goodwin-vpn-backend.git}"
BRANCH="${BRANCH:-main}"
WORKDIR="${WORKDIR:-/opt/goodwin-vpn-src}"
CERT_DOMAIN="${CERT_DOMAIN:-}"

if [[ -n "${GITHUB_TOKEN:-}" && "${REPO}" == https://github.com/* ]]; then
  REPO="https://${GITHUB_TOKEN}@github.com/${REPO#https://github.com/}"
fi

apt-get update -y
apt-get install -y --no-install-recommends ca-certificates curl git tar xz-utils

if ! command -v node >/dev/null 2>&1 || ! node -e 'process.exit(Number(process.versions.node.split(".")[0])<20)'; then
  curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
  apt-get install -y nodejs
fi

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

if ! command -v docker >/dev/null 2>&1; then
  apt-get install -y docker.io
fi
systemctl enable --now docker >/dev/null 2>&1 || true

# Debian docker.io has no Compose v2 plugin. Need docker-compose (v1) or the plugin.
if ! docker compose version >/dev/null 2>&1 && ! command -v docker-compose >/dev/null 2>&1; then
  apt-get install -y docker-compose docker-compose-v2 docker-compose-plugin 2>/dev/null || apt-get install -y docker-compose || true
fi

if ! command -v caddy >/dev/null 2>&1; then
  apt-get install -y caddy
fi

if [[ -d "${WORKDIR}/.git" ]]; then
  git -C "${WORKDIR}" remote set-url origin "${REPO}"
  git -C "${WORKDIR}" fetch --depth 1 origin "${BRANCH}"
  git -C "${WORKDIR}" checkout -B "${BRANCH}" "origin/${BRANCH}"
else
  rm -rf "${WORKDIR}"
  git clone --depth 1 --branch "${BRANCH}" "${REPO}" "${WORKDIR}"
fi

cd "${WORKDIR}"
node "${WORKDIR}/tools/install_vpn_plane.mjs" --src "${WORKDIR}"
