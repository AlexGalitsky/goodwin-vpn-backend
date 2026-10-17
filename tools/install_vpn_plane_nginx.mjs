#!/usr/bin/env node
/**
 * Install or update Goodwin VPN plane behind nginx, with distro Postgres.
 * Does not install Docker or Caddy and does not rewrite other nginx vhosts.
 *
 *   sudo node tools/install_vpn_plane_nginx.mjs --src /opt/goodwin-vpn-src
 *
 * CERT_DOMAIN is required the first time (or left in PUBLIC_SUB_BASE).
 */
import { randomBytes } from "node:crypto";
import {
  chmodSync,
  copyFileSync,
  existsSync,
  lstatSync,
  mkdirSync,
  readFileSync,
  renameSync,
  rmSync,
  symlinkSync,
  unlinkSync,
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
  const env = {
    ...process.env,
    HOME: process.env.HOME || "/root",
    PATH: `${process.env.PATH || ""}:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin`,
    DEBIAN_FRONTEND: "noninteractive",
    ...opts.env,
  };
  const r = spawnSync(cmd, args, {
    encoding: "utf8",
    stdio: opts.stdio ?? "pipe",
    env,
    cwd: opts.cwd,
    input: opts.input,
  });
  if (r.status !== 0) {
    const why = r.error?.message || r.stderr || r.stdout || "";
    throw new Error(`${cmd} ${args.join(" ")}: ${String(why).trim() || `exit ${r.status}`}`);
  }
  return r.stdout || "";
}

function ok(cmd, args) {
  const r = spawnSync(cmd, args, {
    encoding: "utf8",
    env: {
      ...process.env,
      PATH: `${process.env.PATH || ""}:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin`,
    },
  });
  return r.status === 0;
}

function aptInstall(pkgs) {
  sh("apt-get", ["update", "-y"], { stdio: "inherit" });
  sh("apt-get", ["install", "-y", "--no-install-recommends", ...pkgs], { stdio: "inherit" });
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

function nodeBin() {
  const r = spawnSync("sh", ["-c", "command -v node"], {
    encoding: "utf8",
    env: {
      ...process.env,
      PATH: `${process.env.PATH || ""}:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin`,
    },
  });
  const bin = (r.stdout || "").trim();
  return bin || "/usr/bin/node";
}

function psqlAsPostgres(db, sql) {
  return sh("runuser", ["-u", "postgres", "--", "psql", "-v", "ON_ERROR_STOP=1", "-d", db, "-c", sql]);
}

function sqlQuote(value) {
  return `'${String(value).replaceAll("'", "''")}'`;
}

function ensureHostPostgres(dbPass) {
  if (!/^[a-f0-9]+$/.test(dbPass)) {
    throw new Error("POSTGRES_PASSWORD must be hex (refusing to interpolate it into SQL)");
  }
  aptInstall(["postgresql", "postgresql-contrib"]);
  if (!ok("systemctl", ["is-active", "--quiet", "postgresql"])) {
    const listeners = spawnSync("ss", ["-ltn", "sport", "=", ":5432"], { encoding: "utf8" });
    const busy = (listeners.stdout || "").split("\n").some((line) => line.includes(":5432"));
    if (busy) {
      throw new Error(
        "127.0.0.1:5432 is already taken and systemd postgresql is not active. Free the port before installing plane.",
      );
    }
  }
  sh("systemctl", ["enable", "--now", "postgresql"], { stdio: "inherit" });
  for (let i = 0; i < 30; i++) {
    if (ok("pg_isready", ["-q"])) break;
    spawnSync("sleep", ["1"]);
  }
  if (!ok("pg_isready", ["-q"])) {
    throw new Error("postgresql did not become ready");
  }

  const pass = sqlQuote(dbPass);
  psqlAsPostgres(
    "postgres",
    `DO $$ BEGIN
       IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'plane') THEN
         CREATE ROLE plane LOGIN PASSWORD ${pass};
       ELSE
         ALTER ROLE plane WITH LOGIN PASSWORD ${pass};
       END IF;
     END $$`,
  );
  psqlAsPostgres("postgres", "ALTER ROLE plane CREATEDB");
  const exists = psqlAsPostgres("postgres", "SELECT 1 FROM pg_database WHERE datname = 'plane'");
  if (!exists.includes("1")) {
    psqlAsPostgres("postgres", "CREATE DATABASE plane OWNER plane");
  } else {
    psqlAsPostgres("postgres", "ALTER DATABASE plane OWNER TO plane");
  }
  psqlAsPostgres("plane", "ALTER SCHEMA public OWNER TO plane");
  psqlAsPostgres("plane", "GRANT ALL ON SCHEMA public TO plane");

  const check = spawnSync(
    "psql",
    ["postgres://plane@127.0.0.1:5432/plane?sslmode=disable", "-v", "ON_ERROR_STOP=1", "-c", "SELECT 1"],
    {
      encoding: "utf8",
      env: { ...process.env, PGPASSWORD: dbPass },
    },
  );
  if (check.status !== 0) {
    throw new Error(
      `plane cannot log in via 127.0.0.1:5432. Host pg_hba.conf needs "host plane plane 127.0.0.1/32 scram-sha-256" (do not replace other apps' lines). ${String(check.stderr || "").trim()}`,
    );
  }
}

const siteAvail = "/etc/nginx/sites-available/goodwin-plane.conf";
const siteEnabled = "/etc/nginx/sites-enabled/goodwin-plane.conf";

function nginxSite(domain, tls) {
  const acme = `    location ^~ /.well-known/acme-challenge/ {
        root /var/www/letsencrypt;
    }
`;
  const proxy = `        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 120s;`;
  if (!tls) {
    return `server {
    listen 80;
    server_name ${domain};
${acme}
    location / {
${proxy}
    }
}
`;
  }
  const live = `/etc/letsencrypt/live/${domain}`;
  return `server {
    listen 80;
    server_name ${domain};
${acme}
    location / {
        return 301 https://$host$request_uri;
    }
}

server {
    listen 443 ssl;
    server_name ${domain};
    ssl_certificate ${live}/fullchain.pem;
    ssl_certificate_key ${live}/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
${acme}
    location / {
${proxy.replace("$scheme", "https")}
    }
}
`;
}

function linkSite(body) {
  mkdirSync("/etc/nginx/sites-available", { recursive: true });
  mkdirSync("/etc/nginx/sites-enabled", { recursive: true });
  writeFileSync(siteAvail, body);
  try {
    const st = lstatSync(siteEnabled);
    if (st.isSymbolicLink() || st.isFile()) unlinkSync(siteEnabled);
  } catch {
    /* absent */
  }
  symlinkSync(siteAvail, siteEnabled);
}

function nginxKnowsSites() {
  const r = spawnSync("nginx", ["-T"], { encoding: "utf8" });
  return `${r.stdout || ""}${r.stderr || ""}`.includes("sites-enabled");
}

function reloadNginx() {
  try {
    sh("nginx", ["-t"], { stdio: "inherit" });
  } catch (err) {
    try {
      unlinkSync(siteEnabled);
    } catch {
      /* keep the available file for inspection */
    }
    throw err;
  }
  if (ok("systemctl", ["is-active", "--quiet", "nginx"])) {
    sh("systemctl", ["reload", "nginx"], { stdio: "inherit" });
  } else {
    sh("systemctl", ["enable", "--now", "nginx"], { stdio: "inherit" });
  }
}

function ensureTls(domain) {
  if (!/^[A-Za-z0-9.-]+$/.test(domain) || domain.includes("..")) {
    throw new Error(`CERT_DOMAIN is not a hostname: ${domain}`);
  }
  aptInstall(["nginx", "certbot"]);
  mkdirSync("/var/www/letsencrypt", { recursive: true });
  if (!nginxKnowsSites()) {
    throw new Error(
      "nginx is not including /etc/nginx/sites-enabled. Add that include yourself; this installer will not edit the main nginx.conf.",
    );
  }
  linkSite(nginxSite(domain, false));
  reloadNginx();

  const live = `/etc/letsencrypt/live/${domain}/fullchain.pem`;
  if (!existsSync(live)) {
    sh(
      "certbot",
      [
        "certonly",
        "--webroot",
        "-w",
        "/var/www/letsencrypt",
        "-d",
        domain,
        "--non-interactive",
        "--agree-tos",
        "--register-unsafely-without-email",
        "--keep-until-expiring",
      ],
      { stdio: "inherit" },
    );
  }
  if (!existsSync(live)) {
    throw new Error(`certbot did not write ${live}`);
  }
  linkSite(nginxSite(domain, true));
  reloadNginx();

  const hookDir = "/etc/letsencrypt/renewal-hooks/deploy";
  mkdirSync(hookDir, { recursive: true });
  writeFileSync(
    path.join(hookDir, "goodwin-plane-nginx"),
    `#!/bin/sh
nginx -t && systemctl reload nginx
exit 0
`,
    { mode: 0o755 },
  );
  spawnSync("systemctl", ["enable", "--now", "certbot.timer"], { stdio: "inherit" });
}

const src = path.resolve(arg("--src", process.env.PLANE_SRC || defaultSrc));
const prefix = "/opt/goodwin-vpn-plane";
const envPath = "/etc/goodwin-vpn-plane.env";

if (typeof process.getuid === "function" && process.getuid() !== 0) {
  console.error("run as root");
  process.exit(1);
}

mkdirSync(prefix, { recursive: true });
mkdirSync(path.join(prefix, "admin"), { recursive: true });

const prev = parseEnvFile(envPath);
let certDomain = process.env.CERT_DOMAIN || "";
if (!certDomain && prev.PUBLIC_SUB_BASE?.startsWith("https://")) {
  try {
    certDomain = new URL(prev.PUBLIC_SUB_BASE).hostname;
  } catch {
    certDomain = "";
  }
}
if (!certDomain) {
  console.error("CERT_DOMAIN is required (example: gw-plane.goodwin.website)");
  process.exit(1);
}
const publicBase = process.env.PUBLIC_SUB_BASE || prev.PUBLIC_SUB_BASE || `https://${certDomain}`;
const adminPassword = process.env.ADMIN_PASSWORD || prev.ADMIN_PASSWORD || randomBytes(12).toString("hex");
const sessionSecret = process.env.SESSION_SECRET || prev.SESSION_SECRET || randomBytes(24).toString("hex");
const dbPass = process.env.POSTGRES_PASSWORD || prev.POSTGRES_PASSWORD || randomBytes(12).toString("hex");

const envBody = `LISTEN=127.0.0.1:8080
ADMIN_DIR=${prefix}/admin
PUBLIC_SUB_BASE=${publicBase}
ADMIN_PASSWORD=${adminPassword}
SESSION_SECRET=${sessionSecret}
SEED_DEV=${process.env.SEED_DEV || prev.SEED_DEV || "0"}
ALERT_WEBHOOK=${process.env.ALERT_WEBHOOK || prev.ALERT_WEBHOOK || ""}
POSTGRES_PASSWORD=${dbPass}
DATABASE_URL=postgres://plane:${dbPass}@127.0.0.1:5432/plane?sslmode=disable
HOME=/root
`;
writeFileSync(envPath, envBody, { mode: 0o600 });

ensureHostPostgres(dbPass);

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
spawnSync("systemctl", ["stop", "goodwin-vpn-plane"], { encoding: "utf8" });
const staged = planeBin + ".new";
copyFileSync(tmpBin, staged);
chmodSync(staged, 0o755);
renameSync(staged, planeBin);

sh("npm", ["ci"], { cwd: path.join(src, "admin"), stdio: "inherit" });
sh("npm", ["run", "build"], { cwd: path.join(src, "admin"), stdio: "inherit" });
const dist = path.join(src, "admin/dist");
rmSync(path.join(prefix, "admin"), { recursive: true, force: true });
mkdirSync(path.join(prefix, "admin"), { recursive: true });
sh("cp", ["-a", dist + "/.", path.join(prefix, "admin")]);

const unit = `[Unit]
Description=Goodwin VPN control plane
After=network-online.target postgresql.service
Wants=network-online.target postgresql.service

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
spawnSync("systemctl", ["reset-failed", "goodwin-vpn-plane"], { encoding: "utf8" });
sh("systemctl", ["enable", "--now", "goodwin-vpn-plane"], { stdio: "inherit" });
sh("systemctl", ["restart", "goodwin-vpn-plane"], { stdio: "inherit" });

const dumpScript = path.join(prefix, "pg_dump_plane.mjs");
copyFileSync(path.join(src, "tools/pg_dump_plane.mjs"), dumpScript);
chmodSync(dumpScript, 0o755);
mkdirSync("/var/backups/goodwin-plane", { recursive: true, mode: 0o750 });
writeFileSync(
  "/etc/systemd/system/goodwin-plane-pgdump.service",
  `[Unit]
Description=Goodwin plane Postgres dump
After=postgresql.service

[Service]
Type=oneshot
EnvironmentFile=${envPath}
Environment=PLANE_PREFIX=${prefix}
ExecStart=${nodeBin()} ${dumpScript}
`,
);
writeFileSync(
  "/etc/systemd/system/goodwin-plane-pgdump.timer",
  `[Unit]
Description=Daily Goodwin plane Postgres dump

[Timer]
OnCalendar=*-*-* 03:17:00
Persistent=true

[Install]
WantedBy=timers.target
`,
);
sh("systemctl", ["daemon-reload"], { stdio: "inherit" });
sh("systemctl", ["enable", "--now", "goodwin-plane-pgdump.timer"], { stdio: "inherit" });

ensureTls(certDomain);

console.log(`
Plane installed/updated (nginx + host Postgres)
  binary:  ${planeBin}
  admin:   ${prefix}/admin
  env:     ${envPath}
  public:  ${publicBase}
  login:   ${publicBase.replace(/\/$/, "")}/
  vhost:   ${siteAvail}
  backup:  /var/backups/goodwin-plane (timer 03:17)
`);
console.log(`Admin password is in ${envPath} (ADMIN_PASSWORD).`);
