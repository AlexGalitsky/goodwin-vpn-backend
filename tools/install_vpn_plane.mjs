#!/usr/bin/env node
/**
 * Install or update Goodwin VPN plane (API + admin) on Linux.
 * Idempotent: rebuild binaries, keep /etc/goodwin-vpn-plane.env secrets, restart units.
 *
 *   sudo node tools/install_vpn_plane.mjs --src /opt/goodwin-vpn-src
 */
import { randomBytes } from "node:crypto";
import {
  chmodSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const defaultSrc = path.resolve(here, "..");

function arg(name, fallback = "") {
  const i = process.argv.indexOf(name);
  if (i >= 0 && process.argv[i + 1]) return process.argv[i + 1];
  return fallback;
}

function sh(cmd, args, opts = {}) {
  const r = spawnSync(cmd, args, {
    encoding: "utf8",
    stdio: opts.stdio ?? "pipe",
    env: { ...process.env, HOME: process.env.HOME || "/root", ...opts.env },
    cwd: opts.cwd,
  });
  if (r.status !== 0) {
    const err = (r.stderr || r.stdout || "").trim() || `${cmd} exit ${r.status}`;
    throw new Error(err);
  }
  return r.stdout || "";
}

function parseEnvFile(p) {
  const out = {};
  if (!existsSync(p)) return out;
  for (const line of readFileSync(p, "utf8").split("\n")) {
    const t = line.trim();
    if (!t || t.startsWith("#")) continue;
    const i = t.indexOf("=");
    if (i < 0) continue;
    out[t.slice(0, i)] = t.slice(i + 1);
  }
  return out;
}

const src = path.resolve(arg("--src", process.env.PLANE_SRC || defaultSrc));
const prefix = "/opt/goodwin-vpn-plane";
const envPath = "/etc/goodwin-vpn-plane.env";
const certDomain = process.env.CERT_DOMAIN || "";
const publicBase =
  process.env.PUBLIC_SUB_BASE ||
  (certDomain ? `https://${certDomain}` : "http://127.0.0.1:8080");

if (typeof process.getuid === "function" && process.getuid() !== 0) {
  console.error("run as root");
  process.exit(1);
}

mkdirSync(prefix, { recursive: true });
mkdirSync(path.join(prefix, "admin"), { recursive: true });

const prev = parseEnvFile(envPath);
const adminPassword =
  process.env.ADMIN_PASSWORD || prev.ADMIN_PASSWORD || randomBytes(12).toString("hex");
const sessionSecret =
  process.env.SESSION_SECRET || prev.SESSION_SECRET || randomBytes(24).toString("hex");
const dbPass = process.env.POSTGRES_PASSWORD || prev.POSTGRES_PASSWORD || randomBytes(12).toString("hex");

const envBody = `LISTEN=127.0.0.1:8080
ADMIN_DIR=${prefix}/admin
PUBLIC_SUB_BASE=${publicBase}
ADMIN_PASSWORD=${adminPassword}
SESSION_SECRET=${sessionSecret}
SEED_DEV=${process.env.SEED_DEV || prev.SEED_DEV || "0"}
POSTGRES_PASSWORD=${dbPass}
DATABASE_URL=postgres://plane:${dbPass}@127.0.0.1:5432/plane?sslmode=disable
HOME=/root
`;
writeFileSync(envPath, envBody, { mode: 0o600 });

const composeOverride = `services:
  db:
    environment:
      POSTGRES_PASSWORD: ${dbPass}
    ports:
      - "127.0.0.1:5432:5432"
`;
writeFileSync(path.join(prefix, "compose.override.yml"), composeOverride);
copyFileSync(path.join(src, "docker-compose.yml"), path.join(prefix, "docker-compose.yml"));

function dockerCompose(args) {
  const base = ["-f", path.join(prefix, "docker-compose.yml"), "-f", path.join(prefix, "compose.override.yml")];
  try {
    return sh("docker", ["compose", ...base, ...args], { stdio: "inherit" });
  } catch {
    return sh("docker-compose", [...base, ...args], { stdio: "inherit" });
  }
}

dockerCompose(["up", "-d", "db"]);

const planeBin = path.join(prefix, "plane");
const tmpBin = "/tmp/goodwin-plane";
sh("go", ["build", "-o", tmpBin, "./cmd/plane"], {
  cwd: src,
  env: {
    CGO_ENABLED: "0",
    GOCACHE: process.env.GOCACHE || "/root/.cache/go-build",
  },
  stdio: "inherit",
});
copyFileSync(tmpBin, planeBin);
chmodSync(planeBin, 0o755);

sh("npm", ["ci"], { cwd: path.join(src, "admin"), stdio: "inherit" });
sh("npm", ["run", "build"], { cwd: path.join(src, "admin"), stdio: "inherit" });
const dist = path.join(src, "admin/dist");
rmSync(path.join(prefix, "admin"), { recursive: true, force: true });
mkdirSync(path.join(prefix, "admin"), { recursive: true });
sh("cp", ["-a", dist + "/.", path.join(prefix, "admin")]);

const unit = `[Unit]
Description=Goodwin VPN control plane
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=${envPath}
Environment=HOME=/root
ExecStart=${planeBin}
Restart=on-failure
RestartSec=2
WorkingDirectory=${prefix}

[Install]
WantedBy=multi-user.target
`;
writeFileSync("/etc/systemd/system/goodwin-vpn-plane.service", unit);
sh("systemctl", ["daemon-reload"], { stdio: "inherit" });
sh("systemctl", ["enable", "--now", "goodwin-vpn-plane"], { stdio: "inherit" });
sh("systemctl", ["restart", "goodwin-vpn-plane"], { stdio: "inherit" });

if (certDomain) {
  const caddy = `${certDomain} {
	encode gzip
	reverse_proxy 127.0.0.1:8080
}
`;
  writeFileSync("/etc/caddy/Caddyfile", caddy);
  sh("systemctl", ["enable", "--now", "caddy"], { stdio: "inherit" });
  sh("systemctl", ["reload", "caddy"], { stdio: "inherit" });
}

console.log(`
Plane installed/updated
  binary:  ${planeBin}
  admin:   ${prefix}/admin
  env:     ${envPath}
  public:  ${publicBase}
  login:   ${publicBase.replace(/\/$/, "")}/
`);
if (!publicBase.startsWith("https://")) {
  console.log("WARNING: PUBLIC_SUB_BASE is not HTTPS — the Flutter app will reject /sub URLs.");
}
console.log(`Admin password is in ${envPath} (ADMIN_PASSWORD).`);
