import { h, Fragment, useEffect, useState } from "../lib/sprout.js";
import { Link, navigate, query, setQuery } from "../lib/router.js";
import { useApi, useEvents, post, patch, del, act } from "../lib/api.js";
import { bytes, timeAgo, cpu, mem, dateTime } from "../lib/format.js";
import { Button, Loading, ErrorBox, Empty, Pill, Tag, Tabs, Card, Field, Input, Select, Toggle, confirm, Callout, CopyButton, Table, Segmented, Stat, DefList, cx, IconButton } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";
import { Terminal, OutputView } from "../ui/term.js";

type Tab = "connection" | "backups" | "terminal" | "extensions" | "queries" | "metrics" | "resources" | "settings";

export function DatabasePage({ org, id }: { org: string; id: string }) {
  const r = useApi<any>(`/databases/${id}`);
  const [tab, setTabState] = useState<Tab>((query().get("tab") as Tab) || "connection");
  useEvents([`database:${id}`], (e) => {
    if (e.type === "database.updated") r.set((d) => ({ ...d, ...e.data, password: e.data.password || d.password }));
  });
  const setTab = (t: Tab) => {
    setTabState(t);
    setQuery("tab", t === "connection" ? null : t);
  };
  if (r.error) return <div class="page-mid"><ErrorBox error={r.error} onRetry={r.reload} /></div>;
  if (!r.data) return <div class="page-mid"><Loading /></div>;
  const d = r.data;
  const stopped = d.status === "stopped";
  return (
    <div class="page-mid" style={{ maxWidth: 760 }}>
      <Link href={`/o/${org}/projects/${d.project.id}`} class="back-link"><Icon name="arrow-left" size={14} /> {d.project.icon} {d.project.name}</Link>
      <Card pad={false}>
        <div class="detail-head db-head">
          <div class="detail-icon"><Icon name="database" size={22} /></div>
          <div class="grow">
            <div class="detail-title">{d.name} <Pill status={d.status} /></div>
            <div class="detail-tags">
              <Tag mono>PG {d.version}</Tag>
              <Tag mono>{d.storageGi}Gi</Tag>
              <Tag mono>{d.namespace}</Tag>
            </div>
          </div>
          {d.canEdit ? (
            <div class="detail-actions">
              <Button size="sm" kind="secondary" icon="restart" disabled={stopped} onClick={() => act(() => post(`/databases/${id}/restart`), "Restarting")}>Restart</Button>
              <Button size="sm" kind="secondary" icon={stopped ? "play" : "square"} onClick={() => act(() => patch(`/databases/${id}`, { stopped: !stopped }), stopped ? "Starting" : "Stopping (hibernating the cluster)")}>{stopped ? "Start" : "Stop"}</Button>
              <Button size="sm" kind="danger" icon="trash" onClick={async () => {
                if (await confirm({ title: `Delete ${d.name}?`, body: "The CloudNativePG cluster and its volumes are deleted. Take a backup first if you need the data.", danger: true, confirm: "Delete database", typeToConfirm: d.name })) {
                  if (await act(() => del(`/databases/${id}`), "Database deleted")) navigate(`/o/${org}/projects/${d.project.id}`);
                }
              }}>Delete</Button>
            </div>
          ) : null}
        </div>
      </Card>
      <div class="stats">
        <Stat label="Size" value={bytes(d.sizeBytes)} />
        <Stat label="Connections" value={d.connections} />
        <Stat label="Port" value={d.port} />
        {d.instances > 1 ? <Stat label="Replicas" value={d.instances - 1} /> : null}
      </div>
      <div style={{ marginBottom: 16 }}>
        <Tabs<Tab>
          value={tab}
          onChange={setTab}
          tabs={[
            { id: "connection", label: "Connection" },
            { id: "backups", label: "Backups" },
            { id: "terminal", label: "Terminal" },
            { id: "extensions", label: "Extensions" },
            { id: "queries", label: "Queries" },
            { id: "metrics", label: "Metrics" },
            { id: "resources", label: "Resources" },
            { id: "settings", label: "Settings" },
          ]}
        />
      </div>
      {tab === "connection" ? <Connection d={d} org={org} /> : null}
      {tab === "backups" ? <Backups d={d} /> : null}
      {tab === "terminal" ? <DbTerminal d={d} /> : null}
      {tab === "extensions" ? <Extensions d={d} /> : null}
      {tab === "queries" ? <Queries d={d} /> : null}
      {tab === "metrics" ? <DbMetrics d={d} /> : null}
      {tab === "resources" ? <Resources d={d} /> : null}
      {tab === "settings" ? <DbSettings d={d} /> : null}
    </div>
  );
}

type Fmt = "uri" | "psql" | "env" | "jdbc";

function Connection({ d, org }: { d: any; org: string }) {
  const [fmt, setFmt] = useState<Fmt>("uri");
  const [show, setShow] = useState(false);
  const [pwShown, setPwShown] = useState(false);
  const [enabling, setEnabling] = useState(false);
  if (!d.canEdit) return <Callout kind="info">Viewers cannot see database credentials.</Callout>;
  const pw = d.password;
  const mask = "••••••••";
  const uri = (reveal: boolean) => `postgresql://${d.user}:${reveal ? encodeURIComponent(pw) : mask}@${d.host}:${d.port}/${d.dbName}`;
  const text = (reveal: boolean) => {
    const p = reveal ? pw : mask;
    switch (fmt) {
      case "psql":
        return `PGPASSWORD='${p}' psql -h ${d.host} -p ${d.port} -U ${d.user} -d ${d.dbName}`;
      case "env":
        return `DATABASE_URL=${uri(reveal)}\nPGHOST=${d.host}\nPGPORT=${d.port}\nPGDATABASE=${d.dbName}\nPGUSER=${d.user}\nPGPASSWORD=${p}`;
      case "jdbc":
        return `jdbc:postgresql://${d.host}:${d.port}/${d.dbName}?user=${d.user}&password=${reveal ? encodeURIComponent(pw) : mask}`;
    }
    return uri(reveal);
  };
  return (
    <div class="col gap-lg">
      <Card pad={false} class="conn-box">
        <div class="conn-tabs">
          <Segmented<Fmt> value={fmt} onChange={setFmt} options={[{ id: "uri", label: "URI" }, { id: "psql", label: "psql" }, { id: "env", label: ".env" }, { id: "jdbc", label: "JDBC" }]} />
        </div>
        <div class="conn-uri">
          <pre>{text(show)}</pre>
          <IconButton icon={show ? "eye-off" : "eye"} title={show ? "Hide password" : "Show password"} onClick={() => setShow(!show)} />
          <CopyButton text={text(true)} />
        </div>
        <div class="conn-rows">
          <DefList rows={[
            ["Host", d.host],
            ["Port", String(d.port)],
            ["Database", d.dbName],
            ["User", d.user],
            ["Password", <><span>{pwShown ? pw : mask}</span><IconButton icon={pwShown ? "eye-off" : "eye"} class="tiny" title="Reveal" onClick={() => setPwShown(!pwShown)} /></>],
          ]} />
        </div>
      </Card>
      <Callout kind="info">
        From an app in this project, reference it instead of pasting: <code>{`\${{ ${d.name}.DATABASE_URL }}`}</code> in the project's <Link href={`/o/${org}/projects/${d.project.id}?tab=variables`}>variables</Link>.
      </Callout>
      <Card>
        <div class="row" style={{ alignItems: "flex-start" }}>
          <div class="grow">
            <div class="panel-title">Public access</div>
            <div class="panel-desc">Expose this database on a shared organization IP with a unique port.</div>
          </div>
          <Button kind={d.publicPort ? "secondary" : "primary"} size="sm" loading={enabling} onClick={async () => {
            if (d.publicPort && !(await confirm({ title: "Turn off public access?", body: "Clients outside the cluster lose access immediately.", confirm: "Turn off" }))) return;
            setEnabling(true);
            await act(() => patch(`/databases/${d.id}`, { public: !d.publicPort }), d.publicPort ? "No longer public" : "Public port assigned");
            setEnabling(false);
          }}>{d.publicPort ? "Disable" : "Enable"}</Button>
        </div>
        <div style={{ marginTop: 14 }}>
          {d.publicPort ? (
            <div class="col" style={{ gap: 8 }}>
              <div class="callout callout-green" style={{ alignItems: "center" }}>
                <Icon name="globe" />
                <div class="grow mono">{d.publicHost || "<public ip>"}:{d.publicPort}</div>
                <CopyButton text={`postgresql://${d.user}:${encodeURIComponent(pw)}@${d.publicHost}:${d.publicPort}/${d.dbName}?sslmode=prefer`} label="Copy public URI" size="sm" />
              </div>
              <div class="muted" style={{ fontSize: 12.5 }}>Anyone with the password can connect from the internet. Rotate it if it leaks, and prefer the internal host from apps.</div>
            </div>
          ) : (
            <div class="callout" style={{ background: "var(--panel-3)", border: "1px solid var(--border)", color: "var(--text-2)" }}>
              <div>Not publicly exposed. Click <strong>Enable</strong> to assign a random free port on your organization's shared database IP. If no shared IP is configured, set one in <Link href={`/o/${org}/settings`}>organization settings</Link>.</div>
            </div>
          )}
        </div>
      </Card>
    </div>
  );
}

function Backups({ d }: { d: any }) {
  const [sched, setSched] = useState(d.backupSchedule || "");
  return (
    <div class="col gap-lg">
      <Card class="form-card">
        <div class="row">
          <div class="grow">
            <div class="panel-title">Backups</div>
            <div class="panel-desc">Logical dumps (pg_dump, custom format) kept on the database volume. The last 30 are listed.</div>
          </div>
          {d.canEdit ? <Button kind="primary" icon="download" disabled={d.status !== "ready"} onClick={() => act(() => post(`/databases/${d.id}/backups`), "Backup started")}>Back up now</Button> : null}
        </div>
        <div class="row">
          <Field label="Schedule (cron, UTC)" hint="Empty turns scheduled backups off. Default is 03:00 every day.">
            <Input value={sched} onInput={setSched} mono placeholder="0 3 * * *" disabled={!d.canEdit} />
          </Field>
          {d.canEdit ? <Button kind="secondary" onClick={() => act(() => patch(`/databases/${d.id}`, { backupSchedule: sched }), "Schedule saved")} class="" >Save</Button> : null}
        </div>
      </Card>
      {!d.backups?.length ? <Empty icon="download" title="No backups yet">Scheduled backups appear here once the first one runs.</Empty> : (
        <Table head={["", "Taken", "Method", "Size", "Status"]}>
          {d.backups.map((b: any) => (
            <tr key={b.id}>
              <td style={{ width: 30 }}><Icon name="database" size={15} class="muted" /></td>
              <td title={dateTime(b.createdAt)}>{timeAgo(b.createdAt)}</td>
              <td><span class="tag">{b.method}</span></td>
              <td class="mono">{b.sizeBytes ? bytes(b.sizeBytes) : "—"}</td>
              <td><Pill status={b.status === "running" ? "provisioning" : b.status} label={b.status === "running" ? "Running" : undefined} /></td>
            </tr>
          ))}
        </Table>
      )}
    </div>
  );
}

function DbTerminal({ d }: { d: any }) {
  const [sub, setSub] = useState<"psql" | "killed">("psql");
  const cr = useApi<any[]>(sub === "killed" ? `/databases/${d.id}/crashes` : null, [sub]);
  return (
    <div class="col">
      <Segmented value={sub} onChange={setSub} options={[{ id: "psql", label: "psql", icon: "terminal" }, { id: "killed", label: "Killed", icon: "skull" }]} />
      {sub === "psql" ? (
        d.canEdit ? (d.status === "ready" ? <Terminal path={`/databases/${d.id}/terminal`} title={`psql · ${d.dbName}`} prompt={d.dbName + "=>"} height={440} /> : <Callout kind="amber">The database must be running to open a terminal.</Callout>) : <Callout kind="info">Only project members can open a terminal.</Callout>
      ) : !cr.data ? <Loading /> : cr.data.length === 0 ? (
        <Empty icon="check" title="Nothing killed">A Postgres container that gets OOMKilled is replaced fast enough that its logs are usually gone before anyone looks. Orchard keeps them here.</Empty>
      ) : cr.data.map((c) => (
        <Card key={c.id}>
          <div class="row" style={{ marginBottom: 10 }}><strong>{c.reason}</strong><span class="muted">exit {c.exitCode} · {c.pod} · {timeAgo(c.createdAt)}</span></div>
          <OutputView text={c.logs} title="last output" height={300} />
        </Card>
      ))}
    </div>
  );
}

function Extensions({ d }: { d: any }) {
  const [on, setOn] = useState<string[]>(d.extensions || []);
  useEffect(() => setOn(d.extensions || []), [d.extensions?.join(",")]);
  return (
    <Card pad={false}>
      <div style={{ padding: "14px 18px 6px" }}>
        <div class="panel-title">Extensions</div>
        <div class="panel-desc">Enabled with CREATE EXTENSION in the application database. Turning one off keeps its objects until you drop them.</div>
      </div>
      <div style={{ padding: "0 18px 8px" }}>
        {d.availableExtensions.map((e: any) => (
          <div class="ext-row" key={e.Name}>
            <div class="grow">
              <div class="mono" style={{ fontWeight: 500, fontSize: 13.5 }}>{e.Name}</div>
              <div class="muted" style={{ fontSize: 13 }}>{e.Description}</div>
            </div>
            <Toggle checked={on.includes(e.Name)} disabled={!d.canEdit} onChange={async (v) => {
              const next = v ? [...on, e.Name] : on.filter((x) => x !== e.Name);
              setOn(next);
              await act(() => patch(`/databases/${d.id}`, { extensions: next }), v ? `${e.Name} enabled` : `${e.Name} removed from the list`);
            }} />
          </div>
        ))}
      </div>
    </Card>
  );
}

function Queries({ d }: { d: any }) {
  const [sql, setSql] = useState("select now();");
  const [res, setRes] = useState<any>(null);
  const [busy, setBusy] = useState(false);
  const [history, setHistory] = useState<string[]>(() => {
    try {
      return JSON.parse(localStorage.getItem("orchard-sql-" + d.id) || "[]");
    } catch {
      return [];
    }
  });
  const run = async () => {
    if (!sql.trim()) return;
    setBusy(true);
    const r = await act(() => post(`/databases/${d.id}/query`, { sql }));
    setBusy(false);
    setRes(r);
    const h = [sql, ...history.filter((x) => x !== sql)].slice(0, 15);
    setHistory(h);
    try {
      localStorage.setItem("orchard-sql-" + d.id, JSON.stringify(h));
    } catch {}
  };
  if (!d.canEdit) return <Callout kind="info">Only project members can run queries.</Callout>;
  return (
    <div class="col">
      <Card pad={false}>
        <textarea class="editor sql-editor" spellcheck="false" value={sql} onInput={(e: any) => setSql(e.target.value)} onKeyDown={(e: KeyboardEvent) => {
          if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
            e.preventDefault();
            run();
          }
        }} />
        <div class="row" style={{ padding: "8px 10px", borderTop: "1px solid var(--border)" }}>
          <span class="muted" style={{ fontSize: 12.5 }}>⌘↵ to run · <code>\dt</code> lists tables</span>
          <span class="grow" />
          {history.length ? <Select value="" onChange={(v) => v && setSql(v)} options={[{ value: "", label: "History…" }, ...history.map((x) => ({ value: x, label: x.slice(0, 60) }))]} /> : null}
          <Button kind="primary" icon="play" loading={busy} onClick={run} disabled={d.status !== "ready"}>Run</Button>
        </div>
      </Card>
      {res ? (
        res.error ? <Callout kind="red" title="ERROR">{res.error}</Callout> : (
          <>
            <div class="result-meta"><span>{res.command}</span><span>{res.durationMs?.toFixed(1)} ms</span>{res.rows ? <span>{res.rows.length} rows</span> : null}</div>
            {res.columns?.length ? (
              <Table head={res.columns}>
                {res.rows.map((row: string[], i: number) => <tr key={i}>{row.map((c, j) => <td key={j} class="mono" style={{ fontSize: 12.5 }}>{c === "" ? <span class="faint">null</span> : c}</td>)}</tr>)}
              </Table>
            ) : null}
          </>
        )
      ) : null}
    </div>
  );
}

function DbMetrics({ d }: { d: any }) {
  return (
    <div class="grid-2">
      <Card><div class="stat-label">Database size</div><div class="stat-value" style={{ fontSize: 24 }}>{bytes(d.sizeBytes)}</div><div class="muted" style={{ fontSize: 12.5 }}>of {d.storageGi} GiB volume</div>
        <div class="meter" style={{ marginTop: 12 }}><div class="meter-bar"><div class="meter-fill tone-ok" style={{ width: Math.min(100, (d.sizeBytes / (d.storageGi * 1073741824)) * 100) + "%" }} /></div></div>
      </Card>
      <Card><div class="stat-label">Active connections</div><div class="stat-value" style={{ fontSize: 24 }}>{d.connections}</div><div class="muted" style={{ fontSize: 12.5 }}>client backends right now</div></Card>
      <Card><div class="stat-label">Compute</div><div class="stat-value">{cpu(d.resources.cpuMillis)} · {mem(d.resources.memoryMi)}</div><div class="muted" style={{ fontSize: 12.5 }}>per instance, requests equal limits</div></Card>
      <Card><div class="stat-label">Topology</div><div class="stat-value">1 primary{d.instances > 1 ? ` + ${d.instances - 1} read replica${d.instances > 2 ? "s" : ""}` : ""}</div><div class="muted" style={{ fontSize: 12.5 }}>CloudNativePG cluster pg-{d.name}</div></Card>
    </div>
  );
}

function Resources({ d }: { d: any }) {
  const [res, setRes] = useState({ ...d.resources });
  const [storage, setStorage] = useState(d.storageGi);
  const [inst, setInst] = useState(d.instances);
  return (
    <Card class="form-card">
      <div class="form-grid">
        <Field label="CPU per instance">
          <Select value={String(res.cpuMillis)} onChange={(v) => setRes({ ...res, cpuMillis: Number(v) })} disabled={!d.canEdit} options={[250, 500, 1000, 2000, 4000].map((c) => ({ value: String(c), label: cpu(c) }))} />
        </Field>
        <Field label="Memory per instance">
          <Select value={String(res.memoryMi)} onChange={(v) => setRes({ ...res, memoryMi: Number(v) })} disabled={!d.canEdit} options={[256, 512, 1024, 2048, 4096, 8192].map((m) => ({ value: String(m), label: mem(m) }))} />
        </Field>
        <Field label="Storage (GiB)" hint="Volumes can grow but not shrink."><Input type="number" value={storage} min={d.storageGi} onInput={(v) => setStorage(Number(v))} disabled={!d.canEdit} /></Field>
        <Field label="Read replicas" hint="Replicas stream from the primary and serve read-only queries on the -ro service.">
          <Select value={String(inst - 1)} onChange={(v) => setInst(Number(v) + 1)} disabled={!d.canEdit} options={[0, 1, 2, 3, 4].map((n) => ({ value: String(n), label: n === 0 ? "None" : String(n) }))} />
        </Field>
      </div>
      {d.canEdit ? <div class="form-card-foot"><Button kind="primary" onClick={() => act(() => patch(`/databases/${d.id}`, { resources: res, storageGi: storage, instances: inst }), "Saved; the cluster is rolling")}>Save</Button></div> : null}
    </Card>
  );
}

function DbSettings({ d }: { d: any }) {
  return (
    <Card class="form-card">
      <div class="panel-title">Details</div>
      <DefList rows={[
        ["Engine", `PostgreSQL ${d.version} (CloudNativePG)`],
        ["Cluster", `pg-${d.name}`],
        ["Namespace", d.namespace],
        ["Read-write service", d.host],
        ["Read-only service", d.host.replace("-rw.", "-ro.")],
        ["Created", dateTime(d.createdAt)],
      ]} />
    </Card>
  );
}
