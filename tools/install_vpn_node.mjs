#!/usr/bin/env node
/**
 * Install Goodwin VPN node agent on a VPS.
 *
 *   sudo node tools/install_vpn_node.mjs --bin ./bin/agent
 *
 * Prints NODE_TOKEN, guessed IPv4, control port. Does not start Xray/Hy2/TT.
 * Re-run keeps the existing token and replaces the agent binary (stop + rename).
 */
import { spawnSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import {
  chmodSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  readFileSync,
  renameSync,
  writeFileSync,
} from "node:fs";
import { homedir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

function arg(name, fallback = "") {
  const i = process.argv.indexOf(name);
  if (i >= 0 && process.argv[i + 1]) return process.argv[i + 1];
  return fallback;
}

const binSrc = arg("--bin", process.env.AGENT_BIN || "");
const listen = arg("--listen", "0.0.0.0:19400");
const uid = typeof process.getuid === "function" ? process.getuid() : 1;
const prefix =
  uid === 0
    ? "/opt/goodwin-vpn-agent"
    : path.join(homedir(), ".local/share/goodwin-vpn-agent");
const allowExec = process.argv.includes("--no-exec") ? false : true;

if (!binSrc || !existsSync(binSrc)) {
  console.error("Pass --bin /path/to/agent (go build -o bin/agent ./cmd/agent)");
  process.exit(1);
}

mkdirSync(prefix, { recursive: true });
const destBin = path.join(prefix, "agent");
const cfgPath = path.join(prefix, "agent.json");

let token = "";
let keptToken = false;
if (existsSync(cfgPath)) {
  try {
    const prev = JSON.parse(readFileSync(cfgPath, "utf8"));
    if (typeof prev.token === "string" && prev.token.trim()) {
      token = prev.token.trim();
      keptToken = true;
    }
  } catch {
    token = "";
  }
}
if (!token) {
  token = randomBytes(32).toString("hex");
}

const cfg = {
  listen,
  token,
  allow_exec: allowExec,
};
writeFileSync(cfgPath, JSON.stringify(cfg, null, 2) + "\n");

if (uid === 0 && process.platform === "linux") {
  spawnSync("systemctl", ["stop", "goodwin-vpn-agent"], { encoding: "utf8" });
}
const staged = destBin + ".new";
copyFileSync(binSrc, staged);
chmodSync(staged, 0o755);
renameSync(staged, destBin);

const unit = `[Unit]
Description=Goodwin VPN node agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=HOME=/root
ExecStart=${destBin} -config ${cfgPath}
Restart=on-failure
RestartSec=2

[Install]
WantedBy=multi-user.target
`;

let systemd = false;
if (uid === 0 && process.platform === "linux") {
  writeFileSync("/etc/systemd/system/goodwin-vpn-agent.service", unit);
  spawnSync("systemctl", ["daemon-reload"], { stdio: "inherit" });
  spawnSync("systemctl", ["enable", "--now", "goodwin-vpn-agent"], {
    stdio: "inherit",
  });
  systemd = true;
}

const port = listen.includes(":") ? listen.split(":").pop() : "19400";
const ipGuess =
  spawnSync("sh", ["-c", "hostname -I 2>/dev/null | awk '{print $1}'"], {
    encoding: "utf8",
  }).stdout.trim() || "(set IPv4 in admin)";

console.log(`
Agent installed
  prefix:  ${prefix}
  config:  ${cfgPath}
  systemd: ${systemd}
  exec:    ${allowExec}
  token:   ${keptToken ? "kept existing" : "new"}

Paste into admin:
  IPv4:  ${ipGuess}
  port:  ${port}
  token: ${token}
`);

if (!systemd) {
  console.log(`Start locally:\n  ${destBin} -config ${cfgPath}\n`);
}

void root;
