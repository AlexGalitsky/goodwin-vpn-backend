#!/usr/bin/env node
/**
 * Compile geo allowlists into the canonical pack JSON served by the plane.
 *
 *   node tools/build_geo_packs.mjs           # print ads pack + sha256
 *   node tools/build_geo_packs.mjs --check   # fail if allowlist has invalid entries
 *
 * Plane embeds internal/geo/allowlist/ and hashes at startup. This script is
 * the operator/CI check: no GitHub geosite.dat, no runtime download.
 */
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..");
const allowDir = path.join(root, "internal", "geo", "allowlist");
const hostRe =
  /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$/;
const maxBytes = 512 * 1024;
const maxEntries = 2000;

const check = process.argv.includes("--check");

function loadJSON(name) {
  return JSON.parse(readFileSync(path.join(allowDir, name), "utf8"));
}

function normalizeHost(raw, suffix) {
  let s = String(raw ?? "")
    .trim()
    .toLowerCase();
  if (s.startsWith(".")) s = s.slice(1);
  if (!s) throw new Error("empty host");
  if (/[ /:*]/.test(s) || s.includes("geosite:") || s.includes("geoip:")) {
    throw new Error(`forbidden pattern ${raw}`);
  }
  if (!hostRe.test(s)) throw new Error(`not a hostname: ${raw}`);
  return suffix ? `.${s}` : s;
}

function uniqueSorted(xs) {
  return [...new Set(xs)].sort();
}

function compilePack(id, src) {
  if (!/^[a-z][a-z0-9-]{0,31}$/.test(id)) {
    throw new Error(`invalid pack id ${id}`);
  }
  const domains = uniqueSorted((src.domains ?? []).map((d) => normalizeHost(d, false)));
  const suffixes = uniqueSorted((src.suffixes ?? []).map((d) => normalizeHost(d, true)));
  const cidrs = uniqueSorted(src.cidrs ?? []);
  const n = domains.length + suffixes.length + cidrs.length;
  if (n === 0) throw new Error(`pack ${id}: empty allowlist`);
  if (n > maxEntries) throw new Error(`pack ${id}: ${n} entries`);
  const body = JSON.stringify({ id, domains, suffixes, cidrs });
  const buf = Buffer.from(body, "utf8");
  if (buf.length > maxBytes) throw new Error(`pack ${id}: ${buf.length} bytes`);
  return {
    body: buf,
    sha256: createHash("sha256").update(buf).digest("hex"),
    bytes: buf.length,
  };
}

const catalog = loadJSON("catalog.json");
const ads = loadJSON("ads.json");
const compiled = compilePack("ads", ads);
const manifest = {
  version: String(catalog.version || "").trim(),
  packs: [
    {
      id: "ads",
      url: "/gw/v1/geo/packs/ads",
      sha256: compiled.sha256,
      bytes: compiled.bytes,
    },
  ],
};
if (!manifest.version) throw new Error("catalog.json: empty version");

if (!ads.suffixes?.some((s) => String(s).toLowerCase().includes("doubleclick.net"))) {
  throw new Error("ads allowlist must include doubleclick.net");
}

if (check) {
  process.stdout.write(
    `ok ads ${compiled.bytes} bytes sha256=${compiled.sha256} version=${manifest.version}\n`,
  );
  process.exit(0);
}

process.stdout.write(`${JSON.stringify(manifest, null, 2)}\n`);
process.stdout.write(`${compiled.body.toString("utf8")}\n`);
