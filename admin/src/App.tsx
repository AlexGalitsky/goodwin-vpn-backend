import { FormEvent, useEffect, useMemo, useState } from "react";

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
  Status: string;
  GroupID: string;
  Upload: number;
  Download: number;
  Total: number;
  Expire: string | null;
  Note: string;
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
  applied_families?: string[];
  group_ids: string[];
};
type AuditRow = {
  ID: string;
  At: string;
  Actor: string;
  Action: string;
  NodeID: string | null;
  Detail: string;
};
type Overview = {
  public_sub_base: string;
  https_ok: boolean;
  users: { total: number; active: number; disabled: number; revoked: number; expired: number };
  nodes: Record<string, number>;
  groups: number;
  last_apply: AuditRow | null;
  reality_dest: string;
  reality_sni: string;
};

const GiB = 1024 * 1024 * 1024;
const FAMILIES = [
  { id: "vless", label: "VLESS" },
  { id: "hy2", label: "Hysteria2" },
  { id: "tt", label: "TrustTunnel" },
] as const;
const TABS = [
  { id: "overview", label: "Обзор" },
  { id: "nodes", label: "Ноды" },
  { id: "users", label: "Пользователи" },
  { id: "groups", label: "Группы" },
  { id: "tools", label: "Инструменты" },
] as const;
type Tab = (typeof TABS)[number]["id"];
type ExpireMode = "inherit" | "none" | "1d" | "7d";

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
function expireHoursPayload(mode: ExpireMode): { expire_hours?: number } {
  if (mode === "inherit") return {};
  if (mode === "none") return { expire_hours: 0 };
  if (mode === "1d") return { expire_hours: 24 };
  return { expire_hours: 168 };
}
function hoursLabel(h: number): string {
  if (!h) return "без срока";
  if (h === 24) return "сутки";
  if (h === 168) return "неделя";
  if (h % 24 === 0) return `${h / 24} дн.`;
  return `${h} ч.`;
}
function expireLabel(rfc: string | null): string {
  if (!rfc) return "без срока";
  const d = new Date(rfc);
  if (Number.isNaN(d.getTime())) return "—";
  const ms = d.getTime() - Date.now();
  if (ms <= 0) return "истёк";
  const hours = Math.ceil(ms / 3600000);
  if (hours < 24) return `${hours} ч.`;
  return `${Math.ceil(hours / 24)} дн.`;
}
function statusPill(status: string): { cls: string; text: string } {
  switch (status) {
    case "ready":
    case "active":
    case "enrolled":
      return { cls: "ok", text: status === "active" ? "активен" : status === "ready" ? "готова" : "enrolled" };
    case "disabled":
    case "degraded":
    case "pending":
      return { cls: "warn", text: status === "disabled" ? "выкл." : status === "degraded" ? "деград." : "ожидает" };
    case "revoked":
    case "offline":
    case "failed":
      return { cls: "bad", text: status === "revoked" ? "отозван" : status === "offline" ? "офлайн" : "ошибка" };
    default:
      return { cls: "", text: status };
  }
}
function catchErr(e: unknown, fallback: string) {
  return e instanceof Error ? e.message : fallback;
}

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
  const [tab, setTab] = useState<Tab>("overview");
  const [err, setErr] = useState("");

  useEffect(() => {
    api("/v1/me")
      .then(() => setAuthed(true))
      .catch(() => setAuthed(false));
  }, []);

  if (authed === null) return <div className="wrap muted">Загрузка…</div>;
  if (!authed) return <Login onOk={() => setAuthed(true)} />;

  return (
    <div className="wrap">
      <div className="row" style={{ justifyContent: "space-between" }}>
        <h1>Goodwin</h1>
        <button
          type="button"
          onClick={async () => {
            await api("/v1/auth/logout", { method: "POST", body: "{}" });
            setAuthed(false);
          }}
        >
          Выйти
        </button>
      </div>
      <nav className="row" style={{ marginBottom: 16 }}>
        {TABS.map((t) => (
          <button key={t.id} className={tab === t.id ? "on" : ""} type="button" onClick={() => setTab(t.id)}>
            {t.label}
          </button>
        ))}
      </nav>
      {err ? <p className="err">{err}</p> : null}
      {tab === "overview" && <OverviewPage onError={setErr} />}
      {tab === "nodes" && <Nodes onError={setErr} />}
      {tab === "users" && <Users onError={setErr} />}
      {tab === "groups" && <Groups onError={setErr} />}
      {tab === "tools" && (
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
      await api("/v1/auth/login", { method: "POST", body: JSON.stringify({ password }) });
      onOk();
    } catch (e) {
      setErr(catchErr(e, "не вошли"));
    }
  }
  return (
    <form className="wrap grid" style={{ maxWidth: 360 }} onSubmit={submit}>
      <h1>Админка</h1>
      <label>
        Пароль
        <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
      </label>
      {err ? <p className="err">{err}</p> : null}
      <button className="primary" type="submit">
        Войти
      </button>
    </form>
  );
}

function OverviewPage({ onError }: { onError: (s: string) => void }) {
  const [ov, setOv] = useState<Overview | null>(null);
  useEffect(() => {
    api<Overview>("/v1/overview")
      .then(setOv)
      .catch((e) => onError(catchErr(e, "обзор")));
  }, [onError]);
  if (!ov) return <p className="muted">Загрузка…</p>;
  return (
    <div className="grid">
      {!ov.https_ok ? (
        <p className="err">
          PUBLIC_SUB_BASE не HTTPS ({ov.public_sub_base || "пусто"}). Приложение отклонит ссылки.
        </p>
      ) : null}
      <div className="stats">
        <div className="stat">
          <b>{ov.users.active}</b>
          <span className="muted">активных</span>
        </div>
        <div className="stat">
          <b>{ov.users.expired}</b>
          <span className="muted">срок / квота</span>
        </div>
        <div className="stat">
          <b>{ov.users.disabled + ov.users.revoked}</b>
          <span className="muted">выкл. + отозв.</span>
        </div>
        <div className="stat">
          <b>{ov.nodes.ready || 0}</b>
          <span className="muted">нод ready</span>
        </div>
        <div className="stat">
          <b>{ov.nodes.offline || 0}</b>
          <span className="muted">офлайн</span>
        </div>
        <div className="stat">
          <b>{ov.groups}</b>
          <span className="muted">групп</span>
        </div>
      </div>
      <div className="card">
        <h2>Подписка и REALITY</h2>
        <p className="muted">{ov.public_sub_base || "—"}</p>
        <p className="muted">
          dest {ov.reality_dest || "ещё не Apply"} · SNI {ov.reality_sni || "—"}
        </p>
        {ov.last_apply ? (
          <p className="muted">
            Последний Apply: {new Date(ov.last_apply.At).toLocaleString()} — {ov.last_apply.Detail}
          </p>
        ) : (
          <p className="muted">Apply ещё не было.</p>
        )}
      </div>
    </div>
  );
}

function GroupChecks({
  groups,
  selected,
  onChange,
}: {
  groups: Group[];
  selected: string[];
  onChange: (ids: string[]) => void;
}) {
  return (
    <div className="checks">
      {groups.map((g) => (
        <label key={g.ID} className="check">
          <input
            type="checkbox"
            checked={selected.includes(g.ID)}
            onChange={(e) => {
              if (e.target.checked) onChange([...selected, g.ID]);
              else onChange(selected.filter((id) => id !== g.ID));
            }}
          />
          {g.Name}
        </label>
      ))}
    </div>
  );
}

function ProtocolChecks({
  value,
  onChange,
}: {
  value: string[];
  onChange: (v: string[]) => void;
}) {
  return (
    <div className="checks">
      {FAMILIES.map((f) => (
        <label key={f.id} className="check">
          <input
            type="checkbox"
            checked={value.includes(f.id)}
            onChange={(e) => {
              if (e.target.checked) onChange([...value, f.id]);
              else onChange(value.filter((x) => x !== f.id));
            }}
          />
          {f.label}
        </label>
      ))}
    </div>
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
  const [applyLog, setApplyLog] = useState<Record<string, string>>({});

  async function load() {
    try {
      setNodes(await api("/v1/nodes"));
      setGroups(await api("/v1/groups"));
    } catch (e) {
      onError(catchErr(e, "загрузка"));
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
      onError(catchErr(e, "создать"));
    }
  }

  return (
    <div className="grid">
      <form className="card grid" onSubmit={create}>
        <h2>Новая нода</h2>
        <p className="muted">IPv4 и токен с VPS. Пресет max: VLESS 443/tcp, Hy2 443/udp, TT 8443.</p>
        <label>
          Имя
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
          Control-порт
          <input value={port} onChange={(e) => setPort(e.target.value)} />
        </label>
        <label>
          Токен ноды
          <input value={token} onChange={(e) => setToken(e.target.value)} />
        </label>
        <label>
          Пресет
          <select value={preset} onChange={(e) => setPreset(e.target.value)}>
            <option value="max">max (все три)</option>
            <option value="tt-first">TT-first</option>
            <option value="stealth">только VLESS</option>
            <option value="hy2">только Hy2</option>
            <option value="tt">только TT</option>
          </select>
        </label>
        <div>
          <div className="muted" style={{ marginBottom: 6 }}>
            Группы
          </div>
          <GroupChecks groups={groups} selected={groupIds} onChange={setGroupIds} />
        </div>
        <button className="primary" type="submit">
          Enroll
        </button>
      </form>
      <div className="card">
        <h2>Ноды</h2>
        <p className="muted">
          Пользователь видит только ноды своей группы. После смены групп — Apply. Мёртвый agent
          пропадает из подписки.
        </p>
        <table>
          <thead>
            <tr>
              <th>Нода</th>
              <th>Статус</th>
              <th>Стек</th>
              <th>Группы</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {nodes.map((n) => {
              const st = statusPill(n.Status);
              return (
                <tr key={n.ID}>
                  <td>
                    {n.Name}
                    <div className="muted">
                      {n.IPv4 || "—"} · {n.Hostname || "без hostname"}
                    </div>
                    {applyLog[n.ID] ? <div className="apply-log">{applyLog[n.ID]}</div> : null}
                  </td>
                  <td>
                    <span className={`pill ${st.cls}`}>{st.text}</span>
                  </td>
                  <td className="muted">
                    {(n.applied_families || n.Families || []).join(", ") || "—"}
                  </td>
                  <td>
                    <GroupChecks
                      groups={groups}
                      selected={n.group_ids || []}
                      onChange={(ids) =>
                        setNodes((prev) => prev.map((x) => (x.ID === n.ID ? { ...x, group_ids: ids } : x)))
                      }
                    />
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
                            onError(catchErr(e, "группы"));
                          }
                        }}
                      >
                        Сохранить
                      </button>
                      <button
                        type="button"
                        disabled={n.Status === "pending"}
                        onClick={async () => {
                          onError("");
                          try {
                            const res = await api<{ node_status: string; apply?: { detail?: string } }>(
                              `/v1/nodes/${n.ID}/apply`,
                              { method: "POST", body: "{}" },
                            );
                            setApplyLog((p) => ({
                              ...p,
                              [n.ID]: res.apply?.detail || res.node_status,
                            }));
                            await load();
                          } catch (e) {
                            onError(catchErr(e, "apply"));
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
                            onError(catchErr(e, "офлайн"));
                            await load();
                          }
                        }}
                      >
                        Health
                      </button>
                      <button
                        type="button"
                        className="danger"
                        onClick={async () => {
                          if (!window.confirm(`Убрать ноду ${n.Name} из панели? Ядра на VPS не останавливаются.`))
                            return;
                          try {
                            await api(`/v1/nodes/${n.ID}`, { method: "DELETE" });
                            await load();
                          } catch (e) {
                            onError(catchErr(e, "удалить"));
                          }
                        }}
                      >
                        Удалить
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
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
  const [expireMode, setExpireMode] = useState<ExpireMode>("inherit");
  const [count, setCount] = useState("1");
  const [q, setQ] = useState("");
  const [status, setStatus] = useState("all");
  const [filterGroup, setFilterGroup] = useState("all");
  const [preview, setPreview] = useState<string | null>(null);
  const [openId, setOpenId] = useState<string | null>(null);

  async function load() {
    try {
      const gs = await api<Group[]>("/v1/groups");
      setGroups(gs);
      if (!groupId && gs[0]) setGroupId(gs[0].ID);
      setUsers(await api("/v1/users"));
    } catch (e) {
      onError(catchErr(e, "загрузка"));
    }
  }
  useEffect(() => {
    void load();
  }, []);

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return users.filter((u) => {
      if (status !== "all" && u.Status !== status) return false;
      if (filterGroup !== "all" && u.GroupID !== filterGroup) return false;
      if (!needle) return true;
      const g = groups.find((x) => x.ID === u.GroupID)?.Name || "";
      return `${u.DisplayName} ${u.Note} ${g}`.toLowerCase().includes(needle);
    });
  }, [users, groups, q, status, filterGroup]);

  async function create(e: FormEvent) {
    e.preventDefault();
    try {
      await api("/v1/users", {
        method: "POST",
        body: JSON.stringify({
          group_id: groupId,
          display_name: name,
          count: Number(count) || 1,
          ...expireHoursPayload(expireMode),
        }),
      });
      setName("");
      await load();
    } catch (e) {
      onError(catchErr(e, "создать"));
    }
  }

  return (
    <div className="grid">
      <form className="card grid" onSubmit={create}>
        <h2>Новый пользователь</h2>
        <label>
          Имя
          <input value={name} onChange={(e) => setName(e.target.value)} required />
        </label>
        <label>
          Группа
          <select value={groupId} onChange={(e) => setGroupId(e.target.value)}>
            {groups.map((g) => (
              <option key={g.ID} value={g.ID}>
                {g.Name} · {hoursLabel(g.ExpireDefaultHours)}
              </option>
            ))}
          </select>
        </label>
        <label>
          Срок
          <select value={expireMode} onChange={(e) => setExpireMode(e.target.value as ExpireMode)}>
            <option value="inherit">как у группы</option>
            <option value="1d">сутки</option>
            <option value="7d">неделя</option>
            <option value="none">без срока</option>
          </select>
        </label>
        <label>
          Сколько
          <input value={count} onChange={(e) => setCount(e.target.value)} style={{ width: 80 }} />
        </label>
        <button className="primary" type="submit">
          Создать
        </button>
      </form>
      <div className="card">
        <h2>Пользователи</h2>
        {!users[0]?.sub_url.startsWith("https://") && users.length > 0 ? (
          <p className="err">PUBLIC_SUB_BASE не HTTPS — приложение не примет ссылки.</p>
        ) : null}
        <div className="row" style={{ marginBottom: 12 }}>
          <input placeholder="поиск" value={q} onChange={(e) => setQ(e.target.value)} />
          <select value={status} onChange={(e) => setStatus(e.target.value)}>
            <option value="all">все статусы</option>
            <option value="active">активен</option>
            <option value="disabled">выкл.</option>
            <option value="revoked">отозван</option>
          </select>
          <select value={filterGroup} onChange={(e) => setFilterGroup(e.target.value)}>
            <option value="all">все группы</option>
            {groups.map((g) => (
              <option key={g.ID} value={g.ID}>
                {g.Name}
              </option>
            ))}
          </select>
        </div>
        <table>
          <thead>
            <tr>
              <th>Имя</th>
              <th>Группа</th>
              <th>Статус</th>
              <th>Срок</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((u) => {
              const st = statusPill(u.Status);
              const expired = u.Status === "active" && expireLabel(u.Expire) === "истёк";
              return (
                <tr key={u.ID}>
                  <td>
                    <strong>{u.DisplayName || u.ID.slice(0, 8)}</strong>
                    {u.Note ? <div className="muted">{u.Note}</div> : null}
                  </td>
                  <td className="muted">{groups.find((g) => g.ID === u.GroupID)?.Name || "—"}</td>
                  <td>
                    <span className={`pill ${expired ? "warn" : st.cls}`}>{expired ? "истёк" : st.text}</span>
                  </td>
                  <td className="muted">{expireLabel(u.Expire)}</td>
                  <td>
                    <button type="button" onClick={() => setOpenId(openId === u.ID ? null : u.ID)}>
                      {openId === u.ID ? "Свернуть" : "Открыть"}
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
        {openId
          ? (() => {
              const u = users.find((x) => x.ID === openId);
              if (!u) return null;
              return (
                <UserDetail
                  u={u}
                  onError={onError}
                  onReload={load}
                  onPreview={setPreview}
                />
              );
            })()
          : null}
        {preview !== null ? (
          <pre>{preview || "(пусто — выкл., истёк срок, квота или нет готовых нод)"}</pre>
        ) : null}
      </div>
    </div>
  );
}

function UserDetail({
  u,
  onError,
  onReload,
  onPreview,
}: {
  u: UserRow;
  onError: (s: string) => void;
  onReload: () => Promise<void>;
  onPreview: (body: string) => void;
}) {
  const [display, setDisplay] = useState(u.DisplayName);
  const [note, setNote] = useState(u.Note || "");
  const [quotaGiB, setQuotaGiB] = useState(bytesToGiB(u.Total));
  useEffect(() => {
    setDisplay(u.DisplayName);
    setNote(u.Note || "");
    setQuotaGiB(bytesToGiB(u.Total));
  }, [u.ID, u.DisplayName, u.Note, u.Total]);

  return (
    <div className="card grid" style={{ marginTop: 12 }}>
      <div className="muted">{u.Status === "revoked" ? "Отозван — старый URL даёт 404." : u.sub_url}</div>
      <div className="row">
        <label>
          Имя
          <input value={display} onChange={(e) => setDisplay(e.target.value)} />
        </label>
        <label>
          Заметка
          <input value={note} onChange={(e) => setNote(e.target.value)} />
        </label>
        <label>
          Квота GiB
          <input value={quotaGiB} onChange={(e) => setQuotaGiB(e.target.value)} style={{ width: 88 }} />
        </label>
        <button
          type="button"
          onClick={async () => {
            try {
              await api(`/v1/users/${u.ID}`, {
                method: "PATCH",
                body: JSON.stringify({
                  display_name: display,
                  note,
                  total: giBToBytes(quotaGiB),
                }),
              });
              await onReload();
            } catch (e) {
              onError(catchErr(e, "сохранить"));
            }
          }}
        >
          Сохранить
        </button>
      </div>
      <div className="row">
        <button
          type="button"
          onClick={async () => {
            try {
              await api(`/v1/users/${u.ID}`, { method: "PATCH", body: JSON.stringify({ extend_hours: 24 }) });
              await onReload();
            } catch (e) {
              onError(catchErr(e, "срок"));
            }
          }}
        >
          + сутки
        </button>
        <button
          type="button"
          onClick={async () => {
            try {
              await api(`/v1/users/${u.ID}`, { method: "PATCH", body: JSON.stringify({ extend_hours: 168 }) });
              await onReload();
            } catch (e) {
              onError(catchErr(e, "срок"));
            }
          }}
        >
          + неделя
        </button>
        <button
          type="button"
          onClick={async () => {
            try {
              await api(`/v1/users/${u.ID}`, { method: "PATCH", body: JSON.stringify({ expire_unix: 0 }) });
              await onReload();
            } catch (e) {
              onError(catchErr(e, "срок"));
            }
          }}
        >
          Без срока
        </button>
        {u.Status !== "revoked" ? (
          <button type="button" onClick={() => void navigator.clipboard.writeText(u.sub_url)}>
            Копировать URL
          </button>
        ) : null}
        {u.Status !== "revoked" ? (
          <button
            type="button"
            onClick={async () => {
              if (!window.confirm("Сменить ссылку? Старый URL станет 404, доступ сохранится.")) return;
              try {
                await api(`/v1/users/${u.ID}/rotate`, { method: "POST", body: "{}" });
                await onReload();
              } catch (e) {
                onError(catchErr(e, "ротация"));
              }
            }}
          >
            Новая ссылка
          </button>
        ) : null}
        <button
          type="button"
          onClick={async () => {
            try {
              const next = u.Status === "active" ? "disabled" : "active";
              await api(`/v1/users/${u.ID}`, { method: "PATCH", body: JSON.stringify({ status: next }) });
              await onReload();
            } catch (e) {
              onError(catchErr(e, "статус"));
            }
          }}
        >
          {u.Status === "active" ? "Выключить" : "Включить"}
        </button>
        {u.Status !== "revoked" ? (
          <button
            type="button"
            className="danger"
            onClick={async () => {
              if (!window.confirm(`Отозвать ${u.DisplayName}? Старый URL — 404, учётка уйдёт с нод.`)) return;
              try {
                await api(`/v1/users/${u.ID}/revoke`, { method: "POST", body: "{}" });
                await onReload();
              } catch (e) {
                onError(catchErr(e, "revoke"));
              }
            }}
          >
            Отозвать
          </button>
        ) : null}
        <button
          type="button"
          onClick={async () => {
            try {
              const p = await api<{ body: string }>(`/v1/users/${u.ID}/preview`);
              onPreview(p.body);
            } catch (e) {
              onError(catchErr(e, "preview"));
            }
          }}
        >
          Preview
        </button>
        <button
          type="button"
          className="danger"
          onClick={async () => {
            if (!window.confirm(`Удалить ${u.DisplayName} безвозвратно?`)) return;
            try {
              await api(`/v1/users/${u.ID}`, { method: "DELETE" });
              await onReload();
            } catch (e) {
              onError(catchErr(e, "удалить"));
            }
          }}
        >
          Удалить
        </button>
      </div>
    </div>
  );
}

function Groups({ onError }: { onError: (s: string) => void }) {
  const [groups, setGroups] = useState<Group[]>([]);
  const [name, setName] = useState("");
  const [quotaGiB, setQuotaGiB] = useState("0");
  const [expire, setExpire] = useState<"none" | "1d" | "7d">("none");
  const [protocols, setProtocols] = useState<string[]>(["vless", "hy2", "tt"]);

  async function load() {
    try {
      setGroups(await api("/v1/groups"));
    } catch (e) {
      onError(catchErr(e, "загрузка"));
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
                protocols,
                quota_bytes: giBToBytes(quotaGiB),
                expire_default_hours: expire === "1d" ? 24 : expire === "7d" ? 168 : 0,
              }),
            });
            setName("");
            setQuotaGiB("0");
            setExpire("none");
            setProtocols(["vless", "hy2", "tt"]);
            await load();
          } catch (err) {
            onError(catchErr(err, "создать"));
          }
        }}
      >
        <h2>Новая группа</h2>
        <p className="muted">Срок по умолчанию — сутки или неделя для новых пользователей, не привязка к протоколу.</p>
        <label>
          Имя
          <input value={name} onChange={(e) => setName(e.target.value)} required />
        </label>
        <div>
          <div className="muted" style={{ marginBottom: 6 }}>
            Протоколы в подписке
          </div>
          <ProtocolChecks value={protocols} onChange={setProtocols} />
        </div>
        <label>
          Квота GiB (0 = без лимита)
          <input value={quotaGiB} onChange={(e) => setQuotaGiB(e.target.value)} />
        </label>
        <label>
          Срок новых пользователей
          <select value={expire} onChange={(e) => setExpire(e.target.value as "none" | "1d" | "7d")}>
            <option value="none">без срока</option>
            <option value="1d">сутки</option>
            <option value="7d">неделя</option>
          </select>
        </label>
        <button className="primary" type="submit">
          Создать
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
  const [name, setName] = useState(g.Name);
  const [quotaGiB, setQuotaGiB] = useState(bytesToGiB(g.QuotaBytes));
  const [expire, setExpire] = useState(
    g.ExpireDefaultHours === 24 ? "1d" : g.ExpireDefaultHours === 168 ? "7d" : g.ExpireDefaultHours ? "custom" : "none",
  );
  const [customDays, setCustomDays] = useState(
    g.ExpireDefaultHours && g.ExpireDefaultHours !== 24 && g.ExpireDefaultHours !== 168
      ? String(g.ExpireDefaultHours / 24)
      : "3",
  );
  const [protocols, setProtocols] = useState(g.Protocols || ["vless", "hy2", "tt"]);
  useEffect(() => {
    setName(g.Name);
    setQuotaGiB(bytesToGiB(g.QuotaBytes));
    setExpire(
      g.ExpireDefaultHours === 24 ? "1d" : g.ExpireDefaultHours === 168 ? "7d" : g.ExpireDefaultHours ? "custom" : "none",
    );
    setProtocols(g.Protocols || ["vless", "hy2", "tt"]);
  }, [g]);

  function hours(): number {
    if (expire === "1d") return 24;
    if (expire === "7d") return 168;
    if (expire === "custom") return Math.round(Number(customDays) * 24) || 0;
    return 0;
  }

  return (
    <div>
      <div className="row" style={{ marginBottom: 8 }}>
        <input value={name} onChange={(e) => setName(e.target.value)} />
        <span className="muted">{hoursLabel(g.ExpireDefaultHours)}</span>
      </div>
      <ProtocolChecks value={protocols} onChange={setProtocols} />
      <div className="row" style={{ marginTop: 8 }}>
        <label>
          Квота GiB
          <input value={quotaGiB} onChange={(e) => setQuotaGiB(e.target.value)} style={{ width: 88 }} />
        </label>
        <label>
          Срок
          <select value={expire} onChange={(e) => setExpire(e.target.value)}>
            <option value="none">без срока</option>
            <option value="1d">сутки</option>
            <option value="7d">неделя</option>
            <option value="custom">другое</option>
          </select>
        </label>
        {expire === "custom" ? (
          <label>
            Дней
            <input value={customDays} onChange={(e) => setCustomDays(e.target.value)} style={{ width: 72 }} />
          </label>
        ) : null}
        <button
          type="button"
          onClick={async () => {
            try {
              await api(`/v1/groups/${g.ID}`, {
                method: "PATCH",
                body: JSON.stringify({
                  name,
                  protocols,
                  quota_bytes: giBToBytes(quotaGiB),
                  expire_default_hours: hours(),
                }),
              });
              await onReload();
            } catch (e) {
              onError(catchErr(e, "группа"));
            }
          }}
        >
          Сохранить
        </button>
        <button
          type="button"
          className="danger"
          onClick={async () => {
            if (!window.confirm(`Удалить группу ${g.Name}? Только если в ней нет пользователей.`)) return;
            try {
              await api(`/v1/groups/${g.ID}`, { method: "DELETE" });
              await onReload();
            } catch (e) {
              onError(catchErr(e, "удалить"));
            }
          }}
        >
          Удалить
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
      .catch((e) => onError(catchErr(e, "загрузка")));
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
          onError(catchErr(err, "exec"));
        }
      }}
    >
      <h2>Команда на VPS</h2>
      <p className="muted">Через агент на :19400. Пишется в аудит.</p>
      <label>
        Нода
        <select value={nodeId} onChange={(e) => setNodeId(e.target.value)}>
          {nodes.map((n) => (
            <option key={n.ID} value={n.ID}>
              {n.Name} ({n.Status})
            </option>
          ))}
        </select>
      </label>
      <label>
        Команда
        <textarea rows={3} value={shell} onChange={(e) => setShell(e.target.value)} />
      </label>
      <button className="primary" type="submit">
        Выполнить
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
      onError(catchErr(e, "аудит"));
    }
  }
  useEffect(() => {
    void load();
  }, []);
  return (
    <div className="card">
      <div className="row" style={{ justifyContent: "space-between" }}>
        <h2>Аудит</h2>
        <button type="button" onClick={() => void load()}>
          Обновить
        </button>
      </div>
      <table>
        <thead>
          <tr>
            <th>Когда</th>
            <th>Действие</th>
            <th>Детали</th>
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
