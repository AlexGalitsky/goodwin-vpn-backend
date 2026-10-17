#!/usr/bin/env node
/**
 * Dump the plane Postgres volume to /var/backups/goodwin-plane.
 *
 * Usage (on Saturn, as root):
 *   node /opt/goodwin-vpn-plane/pg_dump_plane.mjs
 *   node /opt/goodwin-vpn-plane/pg_dump_plane.mjs --restore FILE.sql.gz
 *
 * --restore loads into database plane_restore (a copy). Live plane is untouched.
 * To replace production: --restore FILE --i-mean-live (after a fresh dump).
 */
import {
  existsSync,
  mkdirSync,
  readdirSync,
  unlinkSync,
  writeFileSync,
  readFileSync,
} from "node:fs";
import { spawnSync } from "node:child_process";
import { gzipSync, gunzipSync } from "node:zlib";
import path from "node:path";

const prefix = process.env.PLANE_PREFIX || "/opt/goodwin-vpn-plane";
const backupDir = process.env.PLANE_BACKUP_DIR || "/var/backups/goodwin-plane";
const keepDays = Number(process.env.PLANE_BACKUP_KEEP || 14);
const composeYml = path.join(prefix, "docker-compose.yml");
const overrideYml = path.join(prefix, "compose.override.yml");
const COPY_DB = "plane_restore";

function die(msg) {
  console.error(msg);
  process.exit(1);
}

function dockerCompose() {
  if (spawnSync("docker", ["compose", "version"], { encoding: "utf8" }).status === 0) {
    return ["docker", "compose"];
  }
  if (spawnSync("docker-compose", ["version"], { encoding: "utf8" }).status === 0) {
    return ["docker-compose"];
  }
  die("docker compose not found");
}

function composeArgs(rest) {
  const exe = dockerCompose();
  const files = ["-f", composeYml];
  if (existsSync(overrideYml)) files.push("-f", overrideYml);
  return { cmd: exe[0], args: [...exe.slice(1), ...files, ...rest] };
}

function databaseURL() {
  return process.env.DATABASE_URL || process.env.TEST_DATABASE_URL || "";
}

function useCompose() {
  return existsSync(composeYml);
}

function parsePgURL(raw) {
  let u;
  try {
    u = new URL(raw);
  } catch {
    die("DATABASE_URL is not a URL");
  }
  const db = decodeURIComponent(u.pathname.replace(/^\//, "")).split("/")[0] || "plane";
  return { raw, db };
}

function urlOnDb(raw, db) {
  const u = new URL(raw);
  u.pathname = "/" + db;
  return u.href;
}

function ident(name) {
  if (!/^[a-z][a-z0-9_]*$/.test(name)) die(`bad database name ${name}`);
  return name;
}

function stamp() {
  const d = new Date();
  const p = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}`;
}

function run(cmd, args, opts = {}) {
  const r = spawnSync(cmd, args, { encoding: "utf8", ...opts });
  if (r.status !== 0) {
    die(`${cmd} ${args.join(" ")}: ${(r.stderr || r.stdout || `exit ${r.status}`).trim()}`);
  }
  return r.stdout || "";
}

function pgDumpCmd(sourceDb) {
  if (useCompose()) {
    return composeArgs(["exec", "-T", "db", "pg_dump", "-U", "plane", "-d", sourceDb, "--no-owner"]);
  }
  const raw = databaseURL();
  if (!raw) die("need docker compose at PLANE_PREFIX or DATABASE_URL");
  return { cmd: "pg_dump", args: [urlOnDb(raw, sourceDb), "--no-owner"] };
}

function psqlCmd(db) {
  if (useCompose()) {
    return composeArgs(["exec", "-T", "db", "psql", "-U", "plane", "-d", db, "-v", "ON_ERROR_STOP=1"]);
  }
  const raw = databaseURL();
  if (!raw) die("need docker compose at PLANE_PREFIX or DATABASE_URL");
  return { cmd: "psql", args: [urlOnDb(raw, db), "-v", "ON_ERROR_STOP=1"] };
}

function pgOwner() {
  if (useCompose()) return "plane";
  const raw = databaseURL();
  if (!raw) return "plane";
  try {
    return ident(decodeURIComponent(new URL(raw).username || "plane") || "plane");
  } catch {
    return "plane";
  }
}

function recreateCopy(destDb) {
  ident(destDb);
  const owner = pgOwner();
  const { cmd, args } = psqlCmd("postgres");
  run(cmd, [
    ...args,
    "-c",
    `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '${destDb}' AND pid <> pg_backend_pid()`,
  ]);
  run(cmd, [...args, "-c", `DROP DATABASE IF EXISTS ${destDb}`]);
  run(cmd, [...args, "-c", `CREATE DATABASE ${destDb} OWNER ${owner}`]);
}

function dump() {
  mkdirSync(backupDir, { recursive: true, mode: 0o750 });
  const dest = path.join(backupDir, `plane-${stamp()}.sql.gz`);
  const sourceDb = useCompose() ? "plane" : parsePgURL(databaseURL()).db;
  const { cmd, args } = pgDumpCmd(sourceDb);
  const r = spawnSync(cmd, args, { encoding: "buffer", maxBuffer: 64 * 1024 * 1024 });
  if (r.status !== 0) {
    die(`pg_dump exit ${r.status}: ${String(r.stderr || "").trim()}`);
  }
  writeFileSync(dest, gzipSync(r.stdout), { mode: 0o600 });
  const cutoff = Date.now() - keepDays * 86400000;
  for (const name of readdirSync(backupDir)) {
    if (!/^plane-\d{8}\.sql\.gz$/.test(name)) continue;
    const full = path.join(backupDir, name);
    const m = name.match(/plane-(\d{4})(\d{2})(\d{2})/);
    if (!m) continue;
    const t = Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
    if (t < cutoff) unlinkSync(full);
  }
  console.log(dest);
}

function restore(file, destDb) {
  if (!file || !existsSync(file)) die("restore file missing");
  recreateCopy(destDb);
  const sql = gunzipSync(readFileSync(file));
  const { cmd, args } = psqlCmd(destDb);
  const r = spawnSync(cmd, args, { encoding: "buffer", input: sql, maxBuffer: 64 * 1024 * 1024 });
  if (r.status !== 0) {
    die(`psql exit ${r.status}: ${String(r.stderr || "").trim()}`);
  }
  console.log("restore ok:", file, "→", destDb);
}

const restoreAt = process.argv.indexOf("--restore");
if (restoreAt >= 0) {
  const live = process.argv.includes("--i-mean-live");
  const dbAt = process.argv.indexOf("--db");
  let dest = COPY_DB;
  if (dbAt >= 0 && process.argv[dbAt + 1]) dest = ident(process.argv[dbAt + 1]);
  if (live) {
    dest = useCompose() ? "plane" : parsePgURL(databaseURL()).db;
  }
  if (dest === "plane" && !live) {
    die("restoring onto live plane needs --i-mean-live (take a fresh dump first)");
  }
  restore(process.argv[restoreAt + 1], dest);
} else {
  dump();
}
