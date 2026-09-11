import { FormEvent, useEffect, useState } from "react";

type Group = {
  ID: string;
  Name: string;
  Protocols: string[];
  QuotaBytes: number;
  ExpireDefaultHours: number;
};
type UserRow = {
  ID: string;
  DisplayName: string;
  SubToken: string;
  Status: string;
  GroupID: string;
  Upload: number;
  Download: number;
  Total: number;
  Expire: string | null;
  sub_url: string;
  import_url: string;
};
type AuditRow = {
  ID: string;
  At: string;
  Actor: string;
  Action: string;
  NodeID: string | null;
  Detail: string;
};

const GiB = 1024 * 1024 * 1024;

function bytesToGiB(n: number): string {
  if (!n) return "0";
  const v = n / GiB;
  return Number.isInteger(v) ? String(v) : v.toFixed(2);
}

function giBToBytes(raw: string): number {
  const n = Number(raw);
  if (!Number.isFinite(n) || n <= 0) return 0;
  return Math.round(n * GiB);
}

function toLocalInput(rfc: string | null): string {
  if (!rfc) return "";
  const d = new Date(rfc);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function localInputToUnix(v: string): number {
  if (!v) return 0;
  const t = new Date(v).getTime();
  if (Number.isNaN(t)) return 0;
  return Math.floor(t / 1000);
}
type NodeRow = {
  ID: string;
  Name: string;
  IPv4: string;
  Hostname: string;
  ControlPort: number;
  Status: string;
  Families: string[];
  group_ids: string[];
};

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: "include",
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) {
    throw new Error(data?.error || res.statusText);
  }
  return data as T;
}

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  const [tab, setTab] = useState<"nodes" | "users" | "groups" | "exec">("nodes");
  const [err, setErr] = useState("");

  async function ping() {
    try {
      await api("/v1/me");
      setAuthed(true);
    } catch {
      setAuthed(false);
    }
  }

  useEffect(() => {
    void ping();
  }, []);

  if (authed === null) return <div className="wrap muted">Loading…</div>;
  if (!authed) return <Login onOk={() => setAuthed(true)} />;

  return (
    <div className="wrap">
      <div className="row" style={{ justifyContent: "space-between" }}>
        <h1>Goodwin plane</h1>
        <button
          onClick={async () => {
            await api("/v1/auth/logout", { method: "POST", body: "{}" });
            setAuthed(false);
          }}
        >
          Log out
        </button>
      </div>
      <nav className="row" style={{ marginBottom: 16 }}>
        {(["nodes", "users", "groups", "exec"] as const).map((id) => (
          <button key={id} className={tab === id ? "on" : ""} onClick={() => setTab(id)}>
            {id}
          </button>
        ))}
      </nav>
      {err ? <p className="err">{err}</p> : null}
      {tab === "nodes" && <Nodes onError={setErr} />}
      {tab === "users" && <Users onError={setErr} />}
      {tab === "groups" && <Groups onError={setErr} />}
      {tab === "exec" && (
        <div className="grid">
          <Exec onError={setErr} />
          <AuditLog onError={setErr} />
        </div>
      )}
    </div>
  );
}

function Login({ onOk }: { onOk: () => void }) {
  const [password, setPassword] = useState("");
  const [err, setErr] = useState("");
  async function submit(e: FormEvent) {
    e.preventDefault();
    setErr("");
    try {
      await api("/v1/auth/login", {
        method: "POST",
        body: JSON.stringify({ password }),
      });
      onOk();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "login failed");
    }
  }
  return (
    <form className="wrap grid" style={{ maxWidth: 360 }} onSubmit={submit}>
      <h1>Admin</h1>
      <label>
        Password
        <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
      </label>
      {err ? <p className="err">{err}</p> : null}
      <button className="primary" type="submit">
        Sign in
      </button>
    </form>
  );
}

function Nodes({ onError }: { onError: (s: string) => void }) {
  const [nodes, setNodes] = useState<NodeRow[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [name, setName] = useState("");
  const [ipv4, setIpv4] = useState("");
  const [hostname, setHostname] = useState("");
  const [port, setPort] = useState("19400");
  const [token, setToken] = useState("");
  const [preset, setPreset] = useState("max");
  const [groupIds, setGroupIds] = useState<string[]>([]);

  async function load() {
    try {
      setNodes(await api("/v1/nodes"));
      setGroups(await api("/v1/groups"));
    } catch (e) {
      onError(e instanceof Error ? e.message : "load");
    }
  }
  useEffect(() => {
    void load();
  }, []);

  async function create(e: FormEvent) {
    e.preventDefault();
    onError("");
    try {
      await api("/v1/nodes", {
        method: "POST",
        body: JSON.stringify({
          name,
          ipv4,
          hostname,
          control_port: Number(port) || 19400,
          token,
          preset,
          group_ids: groupIds,
        }),
      });
      setName("");
      setToken("");
      await load();
    } catch (e) {
      onError(e instanceof Error ? e.message : "create");
    }
  }

  return (
    <div className="grid">
      <form className="card grid" onSubmit={create}>
        <h2>New node</h2>
        <p className="muted">Paste IPv4 and token from the VPS installer. Stack defaults to max (VLESS 443/tcp, Hy2 443/udp, TT 8443).</p>
        <label>
          Name
          <input value={name} onChange={(e) => setName(e.target.value)} required />
        </label>
        <label>
          IPv4
          <input value={ipv4} onChange={(e) => setIpv4(e.target.value)} placeholder="1.2.3.4" />
        </label>
        <label>
          Hostname (Hy2 / TT)
          <input
            value={hostname}
            onChange={(e) => setHostname(e.target.value)}
            required={preset !== "stealth"}
          />
        </label>
        <label>
          Control port
          <input value={port} onChange={(e) => setPort(e.target.value)} />
        </label>
        <label>
          Node token
          <input value={token} onChange={(e) => setToken(e.target.value)} />
        </label>
        <label>
          Preset
          <select value={preset} onChange={(e) => setPreset(e.target.value)}>
            <option value="max">max stack</option>
            <option value="tt-first">TT-first</option>
            <option value="stealth">stealth (VLESS)</option>
            <option value="hy2">hy2 only</option>
            <option value="tt">tt only</option>
          </select>
        </label>
        <label>
          Groups
          <select
            multiple
            value={groupIds}
            onChange={(e) =>
              setGroupIds(Array.from(e.target.selectedOptions).map((o) => o.value))
            }
          >
            {groups.map((g) => (
              <option key={g.ID} value={g.ID}>
                {g.Name}
              </option>
            ))}
          </select>
        </label>
        <button className="primary" type="submit">
          Enroll
        </button>
      </form>
      <div className="card">
        <h2>Nodes</h2>
        <p className="muted">
          One group per region: each user only sees nodes saved on their group. Apply never
          attaches every group. After Save groups, click Apply (VLESS TCP 443 + Hy2 UDP 443 +
          TT TCP/UDP 8443). TT cannot share 443 with REALITY/Hy2. Health / dead agent →{" "}
          <code>offline</code>, that node disappears from <code>/sub</code> until it answers again.
        </p>
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>IP</th>
              <th>Status</th>
              <th>Groups</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {nodes.map((n) => (
              <tr key={n.ID}>
                <td>
                  {n.Name}
                  <div className="muted">{n.Hostname || "—"}</div>
                </td>
                <td>{n.IPv4 || "—"}</td>
                <td>{n.Status}</td>
                <td>
                  <select
                    multiple
                    value={n.group_ids || []}
                    onChange={(e) => {
                      const ids = Array.from(e.target.selectedOptions).map((o) => o.value);
                      setNodes((prev) =>
                        prev.map((x) => (x.ID === n.ID ? { ...x, group_ids: ids } : x)),
                      );
                    }}
                  >
                    {groups.map((g) => (
                      <option key={g.ID} value={g.ID}>
                        {g.Name}
                      </option>
                    ))}
                  </select>
                </td>
                <td>
                  <div className="row">
                    <button
                      type="button"
                      onClick={async () => {
                        onError("");
                        try {
                          await api(`/v1/nodes/${n.ID}/groups`, {
                            method: "PUT",
                            body: JSON.stringify({ group_ids: n.group_ids || [] }),
                          });
                          await load();
                        } catch (e) {
                          onError(e instanceof Error ? e.message : "groups");
                        }
                      }}
                    >
                      Save groups
                    </button>
                    <button
                      type="button"
                      disabled={n.Status === "pending"}
                      onClick={async () => {
                        onError("");
                        try {
                          await api(`/v1/nodes/${n.ID}/apply`, { method: "POST", body: "{}" });
                          await load();
                        } catch (e) {
                          onError(e instanceof Error ? e.message : "apply");
                        }
                      }}
                    >
                      Apply
                    </button>
                    <button
                      type="button"
                      disabled={n.Status === "pending"}
                      onClick={async () => {
                        onError("");
                        try {
                          await api(`/v1/nodes/${n.ID}/health`);
                          await load();
                        } catch (e) {
                          onError(e instanceof Error ? e.message : "offline");
                          await load();
                        }
                      }}
                    >
                      Health
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function Users({ onError }: { onError: (s: string) => void }) {
  const [users, setUsers] = useState<UserRow[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [name, setName] = useState("");
  const [groupId, setGroupId] = useState("");
  const [preview, setPreview] = useState<string | null>(null);

  async function load() {
    try {
      const gs = await api<Group[]>("/v1/groups");
      setGroups(gs);
      if (!groupId && gs[0]) setGroupId(gs[0].ID);
      setUsers(await api("/v1/users"));
    } catch (e) {
      onError(e instanceof Error ? e.message : "load");
    }
  }
  useEffect(() => {
    void load();
  }, []);

  async function create(e: FormEvent) {
    e.preventDefault();
    try {
      await api("/v1/users", {
        method: "POST",
        body: JSON.stringify({ group_id: groupId, display_name: name }),
      });
      setName("");
      await load();
    } catch (e) {
      onError(e instanceof Error ? e.message : "create");
    }
  }

  return (
    <div className="grid">
      <form className="card grid" onSubmit={create}>
        <h2>New user</h2>
        <label>
          Name
          <input value={name} onChange={(e) => setName(e.target.value)} required />
        </label>
        <label>
          Group
          <select value={groupId} onChange={(e) => setGroupId(e.target.value)}>
            {groups.map((g) => (
              <option key={g.ID} value={g.ID}>
                {g.Name}
              </option>
            ))}
          </select>
        </label>
        <button className="primary" type="submit">
          Create
        </button>
      </form>
      <div className="card">
        <h2>Users</h2>
        {!users[0]?.sub_url.startsWith("https://") ? (
          <p className="err">
            PUBLIC_SUB_BASE is not HTTPS. The app rejects http:// subscription URLs. Set it to
            https://your-panel-host before handing out links.
          </p>
        ) : null}
        {users.map((u) => (
          <UserCard
            key={u.ID}
            u={u}
            groups={groups}
            onError={onError}
            onReload={load}
            onPreview={(body) => setPreview(body)}
          />
        ))}
        {preview !== null ? <pre>{preview || "(empty — disabled, expired, over quota, or no ready nodes)"}</pre> : null}
      </div>
    </div>
  );
}

function UserCard({
  u,
  groups,
  onError,
  onReload,
  onPreview,
}: {
  u: UserRow;
  groups: Group[];
  onError: (s: string) => void;
  onReload: () => Promise<void>;
  onPreview: (body: string) => void;
}) {
  const [quotaGiB, setQuotaGiB] = useState(bytesToGiB(u.Total));
  const [expireLocal, setExpireLocal] = useState(toLocalInput(u.Expire));
  useEffect(() => {
    setQuotaGiB(bytesToGiB(u.Total));
    setExpireLocal(toLocalInput(u.Expire));
  }, [u.Total, u.Expire]);
  const groupName = groups.find((g) => g.ID === u.GroupID)?.Name;
  const used = (u.Upload || 0) + (u.Download || 0);
  return (
    <div style={{ marginBottom: 16 }}>
      <div>
        <strong>{u.DisplayName || u.ID}</strong>{" "}
        <span className="muted">
          {u.Status}
          {groupName ? ` · ${groupName}` : ""}
          {u.Expire ? ` · expire ${new Date(u.Expire).toLocaleString()}` : " · no expire"}
          {` · ${used} / ${u.Total || "∞"} bytes`}
        </span>
      </div>
      {u.Status !== "revoked" ? <div className="muted">{u.sub_url}</div> : (
        <div className="muted">Revoked — old URL returns 404. Restore to issue a new link.</div>
      )}
      <div className="row" style={{ marginTop: 8 }}>
        <label>
          Quota GiB
          <input
            value={quotaGiB}
            onChange={(e) => setQuotaGiB(e.target.value)}
            style={{ width: 88 }}
          />
        </label>
        <label>
          Expire
          <input
            type="datetime-local"
            value={expireLocal}
            onChange={(e) => setExpireLocal(e.target.value)}
          />
        </label>
        <button
          type="button"
          onClick={async () => {
            try {
              await api(`/v1/users/${u.ID}`, {
                method: "PATCH",
                body: JSON.stringify({
                  total: giBToBytes(quotaGiB),
                  expire_unix: localInputToUnix(expireLocal),
                }),
              });
              await onReload();
            } catch (e) {
              onError(e instanceof Error ? e.message : "limits");
            }
          }}
        >
          Save limits
        </button>
        {u.Status !== "revoked" ? (
          <button
            type="button"
            onClick={() => void navigator.clipboard.writeText(u.sub_url)}
          >
            Copy HTTPS URL
          </button>
        ) : null}
        {u.Status !== "revoked" ? (
          <button
            type="button"
            onClick={() => void navigator.clipboard.writeText(u.import_url)}
          >
            Copy goodwin://import
          </button>
        ) : null}
        <button
          type="button"
          onClick={async () => {
            try {
              const next = u.Status === "active" ? "disabled" : "active";
              await api(`/v1/users/${u.ID}`, {
                method: "PATCH",
                body: JSON.stringify({ status: next }),
              });
              await onReload();
            } catch (e) {
              onError(e instanceof Error ? e.message : "status");
            }
          }}
        >
          {u.Status === "active" ? "Disable" : "Enable"}
        </button>
        {u.Status !== "revoked" ? (
          <button
            type="button"
            onClick={async () => {
              if (!window.confirm(`Revoke ${u.DisplayName || u.ID}? Old URL will 404.`)) return;
              try {
                await api(`/v1/users/${u.ID}/revoke`, { method: "POST", body: "{}" });
                await onReload();
              } catch (e) {
                onError(e instanceof Error ? e.message : "revoke");
              }
            }}
          >
            Revoke
          </button>
        ) : null}
        <button
          type="button"
          onClick={async () => {
            try {
              const p = await api<{ body: string }>(`/v1/users/${u.ID}/preview`);
              onPreview(p.body);
            } catch (e) {
              onError(e instanceof Error ? e.message : "preview");
            }
          }}
        >
          Preview body
        </button>
      </div>
    </div>
  );
}

function Groups({ onError }: { onError: (s: string) => void }) {
  const [groups, setGroups] = useState<Group[]>([]);
  const [name, setName] = useState("");
  const [quotaGiB, setQuotaGiB] = useState("0");
  const [expireDays, setExpireDays] = useState("0");
  async function load() {
    try {
      setGroups(await api("/v1/groups"));
    } catch (e) {
      onError(e instanceof Error ? e.message : "load");
    }
  }
  useEffect(() => {
    void load();
  }, []);
  return (
    <div className="grid">
      <form
        className="card grid"
        onSubmit={async (e) => {
          e.preventDefault();
          try {
            await api("/v1/groups", {
              method: "POST",
              body: JSON.stringify({
                name,
                protocols: ["vless", "hy2", "tt"],
                quota_bytes: giBToBytes(quotaGiB),
                expire_default_hours: Math.round(Number(expireDays) * 24) || 0,
              }),
            });
            setName("");
            setQuotaGiB("0");
            setExpireDays("0");
            await load();
          } catch (err) {
            onError(err instanceof Error ? err.message : "create");
          }
        }}
      >
        <h2>New group</h2>
        <p className="muted">Quota and expire apply to users created in this group. 0 = unlimited / no expiry.</p>
        <label>
          Name
          <input value={name} onChange={(e) => setName(e.target.value)} required />
        </label>
        <label>
          Default quota GiB
          <input value={quotaGiB} onChange={(e) => setQuotaGiB(e.target.value)} />
        </label>
        <label>
          Default expire (days)
          <input value={expireDays} onChange={(e) => setExpireDays(e.target.value)} />
        </label>
        <button className="primary" type="submit">
          Create
        </button>
      </form>
      <div className="card grid">
        {groups.map((g) => (
          <GroupRow key={g.ID} g={g} onError={onError} onReload={load} />
        ))}
      </div>
    </div>
  );
}

function GroupRow({
  g,
  onError,
  onReload,
}: {
  g: Group;
  onError: (s: string) => void;
  onReload: () => Promise<void>;
}) {
  const [quotaGiB, setQuotaGiB] = useState(bytesToGiB(g.QuotaBytes));
  const [expireDays, setExpireDays] = useState(
    g.ExpireDefaultHours ? String(g.ExpireDefaultHours / 24) : "0",
  );
  useEffect(() => {
    setQuotaGiB(bytesToGiB(g.QuotaBytes));
    setExpireDays(g.ExpireDefaultHours ? String(g.ExpireDefaultHours / 24) : "0");
  }, [g.QuotaBytes, g.ExpireDefaultHours]);
  return (
    <div>
      <div>
        {g.Name} <span className="muted">{(g.Protocols || []).join(", ")}</span>
      </div>
      <div className="row" style={{ marginTop: 8 }}>
        <label>
          Quota GiB
          <input
            value={quotaGiB}
            onChange={(e) => setQuotaGiB(e.target.value)}
            style={{ width: 88 }}
          />
        </label>
        <label>
          Expire days
          <input
            value={expireDays}
            onChange={(e) => setExpireDays(e.target.value)}
            style={{ width: 88 }}
          />
        </label>
        <button
          type="button"
          onClick={async () => {
            try {
              await api(`/v1/groups/${g.ID}`, {
                method: "PATCH",
                body: JSON.stringify({
                  quota_bytes: giBToBytes(quotaGiB),
                  expire_default_hours: Math.round(Number(expireDays) * 24) || 0,
                }),
              });
              await onReload();
            } catch (e) {
              onError(e instanceof Error ? e.message : "group");
            }
          }}
        >
          Save
        </button>
      </div>
    </div>
  );
}

function Exec({ onError }: { onError: (s: string) => void }) {
  const [nodes, setNodes] = useState<NodeRow[]>([]);
  const [nodeId, setNodeId] = useState("");
  const [shell, setShell] = useState("uname -a && ss -lntp | head");
  const [out, setOut] = useState("");

  useEffect(() => {
    api<NodeRow[]>("/v1/nodes")
      .then((ns) => {
        setNodes(ns);
        if (ns[0]) setNodeId(ns[0].ID);
      })
      .catch((e) => onError(e instanceof Error ? e.message : "load"));
  }, [onError]);

  return (
    <form
      className="card grid"
      onSubmit={async (e) => {
        e.preventDefault();
        setOut("");
        try {
          const res = await api<{ stdout: string; stderr: string; exit_code: number }>(
            `/v1/nodes/${nodeId}/exec`,
            { method: "POST", body: JSON.stringify({ shell, timeout_sec: 30 }) },
          );
          setOut(`exit ${res.exit_code}\n--- stdout ---\n${res.stdout}\n--- stderr ---\n${res.stderr}`);
        } catch (err) {
          onError(err instanceof Error ? err.message : "exec");
        }
      }}
    >
      <h2>Run on VPS</h2>
      <p className="muted">Dev only. Goes through the agent on :19400. Logged in audit_events.</p>
      <label>
        Node
        <select value={nodeId} onChange={(e) => setNodeId(e.target.value)}>
          {nodes.map((n) => (
            <option key={n.ID} value={n.ID}>
              {n.Name} ({n.Status})
            </option>
          ))}
        </select>
      </label>
      <label>
        Command
        <textarea rows={3} value={shell} onChange={(e) => setShell(e.target.value)} />
      </label>
      <button className="primary" type="submit">
        Run
      </button>
      {out ? <pre>{out}</pre> : null}
    </form>
  );
}

function AuditLog({ onError }: { onError: (s: string) => void }) {
  const [rows, setRows] = useState<AuditRow[]>([]);
  async function load() {
    try {
      setRows(await api("/v1/audit"));
    } catch (e) {
      onError(e instanceof Error ? e.message : "audit");
    }
  }
  useEffect(() => {
    void load();
  }, []);
  return (
    <div className="card">
      <div className="row" style={{ justifyContent: "space-between" }}>
        <h2>Audit</h2>
        <button type="button" onClick={() => void load()}>
          Refresh
        </button>
      </div>
      <p className="muted">Apply, enroll, exec, revoke, user patch.</p>
      <table>
        <thead>
          <tr>
            <th>When</th>
            <th>Action</th>
            <th>Detail</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((e) => (
            <tr key={e.ID}>
              <td className="muted">{new Date(e.At).toLocaleString()}</td>
              <td>{e.Action}</td>
              <td className="muted">{e.Detail}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
