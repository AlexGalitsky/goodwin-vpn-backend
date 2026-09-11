import { FormEvent, useEffect, useState } from "react";

type Group = { ID: string; Name: string; Protocols: string[] };
type UserRow = {
  ID: string;
  DisplayName: string;
  SubToken: string;
  Status: string;
  sub_url: string;
  import_url: string;
};
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
      {tab === "exec" && <Exec onError={setErr} />}
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
          <input value={hostname} onChange={(e) => setHostname(e.target.value)} />
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
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>IP</th>
              <th>Status</th>
              <th>Families</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {nodes.map((n) => (
              <tr key={n.ID}>
                <td>{n.Name}</td>
                <td>{n.IPv4 || "—"}</td>
                <td>{n.Status}</td>
                <td>{(n.Families || []).join(", ")}</td>
                <td>
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
          <div key={u.ID} style={{ marginBottom: 16 }}>
            <div>
              <strong>{u.DisplayName || u.ID}</strong>{" "}
              <span className="muted">{u.Status}</span>
            </div>
            <div className="muted">{u.sub_url}</div>
            <div className="row" style={{ marginTop: 8 }}>
              <button
                type="button"
                onClick={() => void navigator.clipboard.writeText(u.sub_url)}
              >
                Copy HTTPS URL
              </button>
              <button
                type="button"
                onClick={() => void navigator.clipboard.writeText(u.import_url)}
              >
                Copy goodwin://import
              </button>
              <button
                type="button"
                onClick={async () => {
                  try {
                    const next = u.Status === "disabled" ? "active" : "disabled";
                    await api(`/v1/users/${u.ID}`, {
                      method: "PATCH",
                      body: JSON.stringify({ status: next }),
                    });
                    await load();
                  } catch (e) {
                    onError(e instanceof Error ? e.message : "status");
                  }
                }}
              >
                {u.Status === "disabled" ? "Enable" : "Disable"}
              </button>
              <button
                type="button"
                onClick={async () => {
                  try {
                    const p = await api<{ body: string }>(`/v1/users/${u.ID}/preview`);
                    setPreview(p.body);
                  } catch (e) {
                    onError(e instanceof Error ? e.message : "preview");
                  }
                }}
              >
                Preview body
              </button>
            </div>
          </div>
        ))}
        {preview !== null ? <pre>{preview || "(empty — disabled or no ready nodes)"}</pre> : null}
      </div>
    </div>
  );
}

function Groups({ onError }: { onError: (s: string) => void }) {
  const [groups, setGroups] = useState<Group[]>([]);
  const [name, setName] = useState("");
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
              body: JSON.stringify({ name, protocols: ["vless", "hy2", "tt"] }),
            });
            setName("");
            await load();
          } catch (err) {
            onError(err instanceof Error ? err.message : "create");
          }
        }}
      >
        <h2>New group</h2>
        <label>
          Name
          <input value={name} onChange={(e) => setName(e.target.value)} required />
        </label>
        <button className="primary" type="submit">
          Create
        </button>
      </form>
      <div className="card">
        {groups.map((g) => (
          <div key={g.ID}>
            {g.Name} <span className="muted">{(g.Protocols || []).join(", ")}</span>
          </div>
        ))}
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
