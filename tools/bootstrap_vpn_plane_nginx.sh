#!/usr/bin/env bash
# Bootstrap / update the control plane on a shared Ubuntu host.
# TLS is an nginx vhost (other sites are left alone). Postgres is the distro
# package, not a container. Idempotent: clone or git pull, then rebuild.
#
#   curl -fsSL https://raw.githubusercontent.com/AlexGalitsky/goodwin-vpn-backend/main/tools/bootstrap_vpn_plane_nginx.sh \
#     | sudo env CERT_DOMAIN=gw-plane.goodwin.website bash
#
# Optional env:
#   REPO BRANCH WORKDIR
#   CERT_DOMAIN     public hostname (required on first run)
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
apt-get install -y --no-install-recommends \
  ca-certificates curl git tar xz-utils \
  nginx certbot \
  postgresql postgresql-contrib

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

if [[ -d "${WORKDIR}/.git" ]]; then
  git -C "${WORKDIR}" remote set-url origin "${REPO}"
  git -C "${WORKDIR}" fetch --depth 1 origin "${BRANCH}"
  git -C "${WORKDIR}" checkout -B "${BRANCH}" "origin/${BRANCH}"
else
  rm -rf "${WORKDIR}"
  git clone --depth 1 --branch "${BRANCH}" "${REPO}" "${WORKDIR}"
fi

cd "${WORKDIR}"
node "${WORKDIR}/tools/install_vpn_plane_nginx.mjs" --src "${WORKDIR}"
