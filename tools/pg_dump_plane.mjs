#!/usr/bin/env node
/**
 * Dump the plane Postgres volume to /var/backups/goodwin-plane.
 *
 * Usage (on Saturn, as root):
 *   node tools/pg_dump_plane.mjs
 *   node tools/pg_dump_plane.mjs --restore /var/backups/goodwin-plane/plane-YYYYMMDD.sql.gz
 */
import {
  existsSync,
  mkdirSync,
  readdirSync,
  unlinkSync,
  createWriteStream,
  createReadStream,
} from "node:fs";
import { spawn, spawnSync } from "node:child_process";
import { createGzip, createGunzip } from "node:zlib";
import { pipeline } from "node:stream/promises";
import path from "node:path";

const prefix = process.env.PLANE_PREFIX || "/opt/goodwin-vpn-plane";
const backupDir = process.env.PLANE_BACKUP_DIR || "/var/backups/goodwin-plane";
const keepDays = Number(process.env.PLANE_BACKUP_KEEP || 14);
const composeYml = path.join(prefix, "docker-compose.yml");
const overrideYml = path.join(prefix, "compose.override.yml");

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

function stamp() {
  const d = new Date();
  const p = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}`;
}

async function dump() {
  if (!existsSync(composeYml)) die(`missing ${composeYml}`);
  mkdirSync(backupDir, { recursive: true, mode: 0o750 });
  const dest = path.join(backupDir, `plane-${stamp()}.sql.gz`);
  const { cmd, args } = composeArgs(["exec", "-T", "db", "pg_dump", "-U", "plane", "-d", "plane", "--no-owner"]);
  const child = spawn(cmd, args, { stdio: ["ignore", "pipe", "inherit"] });
  await pipeline(child.stdout, createGzip(), createWriteStream(dest, { mode: 0o600 }));
  const code = await new Promise((resolve) => child.on("close", resolve));
  if (code !== 0) {
    try {
      unlinkSync(dest);
    } catch {
      /* ignore */
    }
    die(`pg_dump exit ${code}`);
  }
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

async function restore(file) {
  if (!file || !existsSync(file)) die("restore file missing");
  if (!existsSync(composeYml)) die(`missing ${composeYml}`);
  const { cmd, args } = composeArgs([
    "exec",
    "-T",
    "db",
    "psql",
    "-U",
    "plane",
    "-d",
    "plane",
    "-v",
    "ON_ERROR_STOP=1",
  ]);
  const child = spawn(cmd, args, { stdio: ["pipe", "inherit", "inherit"] });
  await pipeline(createReadStream(file), createGunzip(), child.stdin);
  const code = await new Promise((resolve) => child.on("close", resolve));
  if (code !== 0) die(`psql exit ${code}`);
  console.log("restore ok:", file);
}

const restoreAt = process.argv.indexOf("--restore");
if (restoreAt >= 0) {
  await restore(process.argv[restoreAt + 1]);
} else {
  await dump();
}
