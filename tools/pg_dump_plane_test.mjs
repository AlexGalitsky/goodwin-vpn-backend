#!/usr/bin/env node
/**
 * Round-trip: pg_dump live plane → restore into plane_restore → probe row present.
 * Prefers DATABASE_URL + host pg_dump (CI). Falls back to docker compose in this repo.
 */
import { existsSync, mkdtempSync, rmSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..");
const script = path.join(here, "pg_dump_plane.mjs");
const url = process.env.TEST_DATABASE_URL || process.env.DATABASE_URL || "";
const composeYml = path.join(root, "docker-compose.yml");
const overrideYml = path.join(root, "compose.override.yml");

function have(bin) {
  return spawnSync(bin, ["--version"], { encoding: "utf8" }).status === 0;
}

function dockerCompose() {
  if (spawnSync("docker", ["compose", "version"], { encoding: "utf8" }).status === 0) {
    return ["docker", "compose"];
  }
  if (spawnSync("docker-compose", ["version"], { encoding: "utf8" }).status === 0) {
    return ["docker-compose"];
  }
  return null;
}

function die(msg) {
  console.error(msg);
  process.exit(1);
}

function urlOnDb(raw, db) {
  const u = new URL(raw);
  u.pathname = "/" + db;
  return u.href;
}

function composeCmd() {
  const exe = dockerCompose();
  if (!exe) return null;
  const files = ["-f", composeYml];
  if (existsSync(overrideYml)) files.push("-f", overrideYml);
  return { cmd: exe[0], args: [...exe.slice(1), ...files] };
}

const canURL = Boolean(url) && have("pg_dump") && have("psql");
const canCompose = existsSync(composeYml) && Boolean(dockerCompose());
if (!canURL && !canCompose) {
  console.log("skip pg_dump_plane_test: need DATABASE_URL+pg_dump or docker compose");
  process.exit(0);
}

const mode = canURL ? "url" : "compose";
const marker = "p25_probe_" + String(Date.now());

function psql(db, sql) {
  if (mode === "url") {
    const dbUrl = db === "plane" ? url : urlOnDb(url, db);
    const r = spawnSync("psql", [dbUrl, "-v", "ON_ERROR_STOP=1", "-t", "-A", "-c", sql], {
      encoding: "utf8",
    });
    if (r.status !== 0) die(r.stderr || r.stdout || `psql exit ${r.status}`);
    return (r.stdout || "").trim();
  }
  const c = composeCmd();
  const r = spawnSync(
    c.cmd,
    [...c.args, "exec", "-T", "db", "psql", "-U", "plane", "-d", db, "-v", "ON_ERROR_STOP=1", "-t", "-A", "-c", sql],
    { encoding: "utf8" },
  );
  if (r.status !== 0) die(r.stderr || r.stdout || `compose psql exit ${r.status}`);
  return (r.stdout || "").trim();
}

psql("plane", `CREATE TABLE ${marker} (note text)`);
psql("plane", `INSERT INTO ${marker}(note) VALUES ('reality-keys')`);

const dir = mkdtempSync(path.join(tmpdir(), "plane-dump-"));
const env = {
  ...process.env,
  PLANE_BACKUP_DIR: dir,
};
if (mode === "url") {
  env.DATABASE_URL = url;
  env.PLANE_PREFIX = dir;
} else {
  env.PLANE_PREFIX = root;
  delete env.DATABASE_URL;
  delete env.TEST_DATABASE_URL;
}

const dumped = spawnSync(process.execPath, [script], { encoding: "utf8", env });
if (dumped.status !== 0) {
  die(dumped.stderr || dumped.stdout || `dump exit ${dumped.status}`);
}
const file = dumped.stdout.trim().split("\n").filter(Boolean).pop();
if (!file || !file.endsWith(".sql.gz")) die(`dump path missing: ${dumped.stdout}`);

const restored = spawnSync(process.execPath, [script, "--restore", file], { encoding: "utf8", env });
if (restored.status !== 0) {
  die(restored.stderr || restored.stdout || `restore exit ${restored.status}`);
}

const note = psql("plane_restore", `SELECT note FROM ${marker}`);
if (note !== "reality-keys") die(`copy missing probe row: ${note}`);
const live = psql("plane", `SELECT note FROM ${marker}`);
if (live !== "reality-keys") die(`live plane was clobbered: ${live}`);

psql("plane", `DROP TABLE ${marker}`);
psql("postgres", `DROP DATABASE IF EXISTS plane_restore`);
rmSync(dir, { recursive: true, force: true });
console.log("ok dump → restore copy", file);
