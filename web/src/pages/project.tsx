import { h, Fragment, useEffect, useRef, useState } from "../lib/sprout.js";
import { Link, navigate, query, setQuery } from "../lib/router.js";
import { useApi, useEvents, get, post, put, patch, del, act } from "../lib/api.js";
import { plural, timeAgo } from "../lib/format.js";
import { Button, Loading, ErrorBox, Empty, Dot, Pill, cx, SectionLabel, Tabs, Field, Input, Textarea, Select, Toggle, confirm, Callout, IconButton, Card, Menu, MenuItem, CodeBlock } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";
import { Sky, Avatar } from "../ui/art.js";
import { NewMenu, BackgroundPicker, appImageLabel } from "./apps.js";

type Tab = "apps" | "variables" | "compose" | "members" | "settings";

export function ProjectPage({ org, id }: { org: string; id: string }) {
  const [tick, setTick] = useState(0);
  const r = useApi<any>(`/projects/${id}`, [tick]);
  const [tab, setTabState] = useState<Tab>((query().get("tab") as Tab) || "apps");
  const [env, setEnv] = useState(query().get("env") || "");
  const pending = useRef<any>(null);
  useEvents([`project:${id}`], () => {
    if (pending.current) return;
    pending.current = setTimeout(() => {
      pending.current = null;
      setTick((t) => t + 1);
    }, 300);
  });
  const setTab = (t: Tab) => {
    setTabState(t);
    setQuery("tab", t === "apps" ? null : t);
  };
  if (r.error) return <div class="page"><ErrorBox error={r.error} onRetry={r.reload} /></div>;
  if (!r.data) return <div class="page"><Loading /></div>;
  const p = r.data;
  const envId = env || p.environments[0]?.id;
  const inEnv = (x: any) => !x.envId || x.envId === envId;
  const apps = p.apps.filter(inEnv);
  const dbs = p.databases.filter(inEnv);
  const tpls = p.templateInstances.filter(inEnv);
  const jobs = p.jobs.filter(inEnv);
  return (
    <div class="page">
      <Link href={`/o/${org}`} class="crumb-pill"><Icon name="chevron-left" size={15} /> Your Apps</Link>
      <div class="hero-wrap">
        <div class="hero">
          <Sky seed={p.id} preset={p.background} />
          <div class="hero-title"><span class="emoji">{p.icon}</span> {p.name}</div>
          <div class="hero-actions">
            <Menu align="right" trigger={(open, toggle) => <button type="button" class="hero-btn" onClick={toggle} title="Project actions"><Icon name="more" size={16} /></button>}>
              {(close) => (
                <>
                  <MenuItem icon="settings" onClick={() => { close(); setTab("settings"); }}>Project settings</MenuItem>
                  <MenuItem icon="users" onClick={() => { close(); setTab("members"); }}>Members</MenuItem>
                  <MenuItem icon="copy" onClick={() => { close(); navigator.clipboard?.writeText(p.namespace); }} hint={p.namespace}>Copy namespace</MenuItem>
                </>
              )}
            </Menu>
          </div>
        </div>
      </div>
      <div class="project-bar-tabs">
        <label class="env-select">
          <select value={envId} onChange={(e: any) => { setEnv(e.target.value); setQuery("env", e.target.value); }}>
            {p.environments.map((e: any) => <option key={e.id} value={e.id} selected={e.id === envId}>{e.name}</option>)}
          </select>
          <Icon name="chevron-down" size={15} class="muted" />
        </label>
        <Tabs<Tab>
          value={tab}
          onChange={setTab}
          tabs={[
            { id: "apps", label: "Apps", icon: "server", count: apps.length + dbs.length + tpls.length },
            { id: "variables", label: "Variables", icon: "key" },
            { id: "compose", label: "Compose", icon: "file-text" },
            { id: "members", label: "Members", icon: "users", count: p.people.length },
            { id: "settings", label: "Settings", icon: "settings" },
          ]}
        />
        <span class="grow" />
        {p.canEdit ? <NewMenu org={org} projectId={p.id} /> : null}
      </div>
      {tab === "apps" ? <AppsTab org={org} p={p} apps={apps} dbs={dbs} tpls={tpls} jobs={jobs} /> : null}
      {tab === "variables" ? <VariablesTab p={p} envId={envId} /> : null}
      {tab === "compose" ? <ComposeTab p={p} envId={envId} org={org} /> : null}
      {tab === "members" ? <MembersTab p={p} org={org} reload={r.reload} /> : null}
      {tab === "settings" ? <SettingsTab p={p} org={org} reload={r.reload} /> : null}
    </div>
  );
}

function letter(n: string) {
  return (n[0] || "?").toUpperCase();
}

function AppsTab({ org, p, apps, dbs, tpls, jobs }: { org: string; p: any; apps: any[]; dbs: any[]; tpls: any[]; jobs: any[] }) {
  if (!apps.length && !dbs.length && !tpls.length && !jobs.length) {
    return (
      <Empty icon="sprout" title="An empty plot" action={p.canEdit ? <NewMenu org={org} projectId={p.id} label="Add to this project" /> : null}>
        Connect a GitHub repo, run a container image, spin up PostgreSQL, or grab a template from The Grove.
      </Empty>
    );
  }
  return (
    <>
      {apps.length ? (
        <div class="grid-3">
          {apps.map((a) => (
            <Link key={a.id} href={`/o/${org}/apps/${a.id}`} class="card svc-card card-link">
              <div class="svc-top">
                <div class="svc-letter">{letter(a.name)}<span class={"status-dot dot-" + (a.status === "running" ? "green" : a.status === "failed" ? "red" : a.status === "degraded" ? "amber" : a.status === "stopped" || a.status === "pending" ? "gray" : "blue")} /></div>
                <div class="svc-name">{a.name}</div>
                <Pill status={a.status} />
              </div>
              <div class="svc-image">{appImageLabel(a)}</div>
              <div class="svc-meta">
                <span>{plural(a.replicas, "replica")}</span>
                <span class="muted">· {a.creator.name}</span>
              </div>
            </Link>
          ))}
        </div>
      ) : null}
      {dbs.length ? (
        <>
          <SectionLabel>Databases</SectionLabel>
          <div class="grid-3">
            {dbs.map((d) => (
              <Link key={d.id} href={`/o/${org}/databases/${d.id}`} class="card db-card card-link">
                <div class="row">
                  <div class="db-icon"><Icon name="database" size={20} /></div>
                  <Pill status={d.status} />
                </div>
                <div class="db-name">{d.name}</div>
                <div class="db-ident">{d.dbName}</div>
                <div class="db-foot">PostgreSQL {d.version} · {d.mode}{d.instances > 1 ? ` · ${d.instances - 1} replica${d.instances > 2 ? "s" : ""}` : ""}</div>
              </Link>
            ))}
          </div>
        </>
      ) : null}
      {jobs.length ? (
        <>
          <SectionLabel>Jobs</SectionLabel>
          <div class="grid-3">
            {jobs.map((j) => (
              <Link key={j.id} href={`/o/${org}/jobs/${j.id}`} class="card db-card card-link">
                <div class="row">
                  <div class="tpl-icon" style={{ background: "var(--amber-soft)", color: "var(--amber)" }}><Icon name="zap" size={20} /></div>
                  {j.paused ? <Pill status="paused" /> : j.schedule ? <Pill status="scheduled" label="Scheduled" /> : <Pill status="pending" label="Manual" />}
                </div>
                <div class="db-name">{j.name}</div>
                <div class="db-ident">{j.schedule || "run on demand"}</div>
                <div class="db-foot">{plural(j.steps.length, "step")}{j.lastRunAt ? ` · ran ${timeAgo(j.lastRunAt)}` : ""}</div>
              </Link>
            ))}
          </div>
        </>
      ) : null}
      {tpls.length ? (
        <>
          <SectionLabel>Template instances</SectionLabel>
          <div class="grid-3">
            {tpls.map((t) => (
              <div key={t.id} class="card db-card">
                <div class="row">
                  <div class="tpl-icon"><Icon name="template" size={20} /></div>
                  <Pill status={t.status} />
                  <span class="grow" />
                  {p.canEdit ? (
                    <IconButton icon="trash" title="Remove the template and everything it created" onClick={async () => {
                      if (await confirm({ title: `Remove ${t.name}?`, body: "This deletes the apps and databases this template created, including their data.", danger: true, confirm: "Remove", typeToConfirm: t.name })) {
                        await act(() => del(`/template-instances/${t.id}`), "Template removed");
                      }
                    }} />
                  ) : null}
                </div>
                <div class="db-name">{t.name}</div>
                <div class="db-ident">{t.label}</div>
              </div>
            ))}
          </div>
        </>
      ) : null}
    </>
  );
}

// ---- variables ----

function VariablesTab({ p, envId }: { p: any; envId: string }) {
  const r = useApi<any[]>(`/projects/${p.id}/variables`);
  const [rows, setRows] = useState<any[]>([]);
  const [deleted, setDeleted] = useState<string[]>([]);
  const [scope, setScope] = useState("all");
  const [bulk, setBulk] = useState<string | null>(null);
  const [dirty, setDirty] = useState(false);
  useEffect(() => {
    if (r.data) {
      setRows(r.data.map((v) => ({ ...v })));
      setDeleted([]);
      setDirty(false);
    }
  }, [r.data]);
  const envName = (id: string) => p.environments.find((e: any) => e.id === id)?.name || "all environments";
  const shown = rows.filter((v) => scope === "all" || (scope === "shared" ? !v.appId : v.appId === scope));
  const update = (i: number, k: string, v: any) => {
    setRows(rows.map((row) => (row === shown[i] ? { ...row, [k]: v } : row)));
    setDirty(true);
  };
  const save = async () => {
    const res = await act(() => put(`/projects/${p.id}/variables`, { variables: rows.filter((v) => v.key.trim()), delete: deleted }));
    if (res) {
      await r.reload();
      if (res.redeployed) (await import("../lib/state.js")).toast(`Saved. ${plural(res.redeployed, "app")} rolling out with the new values.`, "ok");
    }
  };
  const refs = [...p.databases.map((d: any) => `\${{ ${d.name}.DATABASE_URL }}`), ...p.apps.slice(0, 3).map((a: any) => `\${{ ${a.name}.URL }}`)];
  if (!r.data) return <Loading />;
  return (
    <div class="col gap-lg">
      <div class="row row-wrap">
        <Select value={scope} onChange={setScope} options={[{ value: "all", label: "All variables" }, { value: "shared", label: "Shared by the project" }, ...p.apps.map((a: any) => ({ value: a.id, label: "App: " + a.name }))]} class="role-select" />
        <span class="grow" />
        {p.canEdit ? <Button kind="ghost" icon="upload" onClick={() => setBulk(bulk == null ? "" : null)}>Import .env</Button> : null}
        {p.canEdit ? <Button kind="secondary" icon="plus" onClick={() => { setRows([...rows, { key: "", value: "", secret: false, envId: "", appId: scope !== "all" && scope !== "shared" ? scope : "" }]); setDirty(true); }}>Add variable</Button> : null}
        {p.canEdit ? <Button kind="primary" icon="save" disabled={!dirty} onClick={save}>Save changes</Button> : null}
      </div>
      {bulk != null ? (
        <Card>
          <div class="form-stack">
            <Field label="Paste a .env file" hint="KEY=value lines. Existing keys in the same scope are overwritten.">
              <Textarea value={bulk} onInput={setBulk} rows={6} mono placeholder={"DATABASE_URL=postgres://…\nNODE_ENV=production"} />
            </Field>
            <div class="row">
              <span class="grow" />
              <Button kind="ghost" onClick={() => setBulk(null)}>Cancel</Button>
              <Button kind="primary" onClick={async () => {
                const ok = await act(() => put(`/projects/${p.id}/variables`, { env: bulk, envId: "", appId: scope !== "all" && scope !== "shared" ? scope : "" }), "Imported");
                if (ok) {
                  setBulk(null);
                  r.reload();
                }
              }}>Import</Button>
            </div>
          </div>
        </Card>
      ) : null}
      {shown.length === 0 ? (
        <Empty icon="key" title="No variables here">
          Shared variables reach every app in the project; app variables override them; environment-scoped ones apply only there.
        </Empty>
      ) : (
        <Card>
          <div class="col">
            {shown.map((v, i) => (
              <div class="kv-row" key={v.id || "new" + i}>
                <Input value={v.key} onInput={(x) => update(i, "key", x.toUpperCase().replace(/[^A-Z0-9_.-]/g, "_"))} placeholder="KEY" mono disabled={!p.canEdit} />
                <Input value={v.value} onInput={(x) => update(i, "value", x)} placeholder={v.secret && v.id ? "•••••••• (unchanged)" : "value"} mono type={v.secret ? "password" : "text"} disabled={!p.canEdit} />
                <div class="row" style={{ gap: 6 }}>
                  <select class="input select" style={{ width: 150, height: 34, fontSize: 12.5 }} disabled={!p.canEdit} onChange={(e: any) => update(i, "envId", e.target.value)}>
                    <option value="" selected={!v.envId}>All environments</option>
                    {p.environments.map((e: any) => <option key={e.id} value={e.id} selected={v.envId === e.id}>{e.name}</option>)}
                  </select>
                  <select class="input select" style={{ width: 130, height: 34, fontSize: 12.5 }} disabled={!p.canEdit} onChange={(e: any) => update(i, "appId", e.target.value)}>
                    <option value="" selected={!v.appId}>Shared</option>
                    {p.apps.map((a: any) => <option key={a.id} value={a.id} selected={v.appId === a.id}>{a.name}</option>)}
                  </select>
                </div>
                <div class="row" style={{ gap: 2 }}>
                  <IconButton icon={v.secret ? "lock" : "eye"} title={v.secret ? "Secret: hidden from viewers and logs. Click to make plain" : "Plain. Click to make secret"} onClick={() => p.canEdit && update(i, "secret", !v.secret)} />
                  {p.canEdit ? <IconButton icon="trash" title="Delete" onClick={() => { if (v.id) setDeleted([...deleted, v.id]); setRows(rows.filter((x) => x !== v)); setDirty(true); }} /> : null}
                </div>
              </div>
            ))}
          </div>
        </Card>
      )}
      <Callout kind="info" title="Reference other things in this project">
        Values can point at databases and apps, resolved when the app rolls out: {refs.map((x, i) => <><code key={i}>{x}</code>{i < refs.length - 1 ? " " : ""}</>)}. Variables prefixed <code>BUILD_</code> become Docker build arguments. Currently viewing <strong>{envName(envId)}</strong>.
      </Callout>
    </div>
  );
}

// ---- compose ----

const composeExample = `services:
  web:
    image: nginx:alpine
    ports:
      - "80"
  worker:
    image: redis:7-alpine
    command: redis-server --appendonly yes
    volumes:
      - data:/data
volumes:
  data:
`;

function ComposeTab({ p, envId, org }: { p: any; envId: string; org: string }) {
  const r = useApi<any>(`/projects/${p.id}/compose?env=${envId}`, [envId]);
  const [src, setSrc] = useState("");
  const [plan, setPlan] = useState<any>(null);
  useEffect(() => {
    if (r.data) setSrc(r.data.source || "");
  }, [r.data]);
  if (!r.data) return <Loading />;
  const run = async (dryRun: boolean) => {
    const res = await act(() => put(`/projects/${p.id}/compose`, { envId, source: src, dryRun }));
    if (res) {
      setPlan({ ...res, dryRun });
      if (!dryRun) r.reload();
    }
  };
  return (
    <div class="overview-grid">
      <Card pad={false}>
        <div class="ws-head"><Icon name="file-text" size={14} /> docker-compose.yml <span class="grow" />{r.data.updatedAt ? <span class="muted" style={{ fontWeight: 400 }}>applied {timeAgo(r.data.updatedAt)} by {r.data.updatedBy}</span> : null}</div>
        <textarea class="editor" style={{ minHeight: 440 }} spellcheck="false" value={src} placeholder={composeExample} onInput={(e: any) => setSrc(e.target.value)} disabled={!p.canEdit} />
      </Card>
      <div class="side-stack">
        <Card>
          <div class="panel-title">Deploy a Compose stack</div>
          <p class="panel-desc">Each service becomes an app in this environment. Named volumes become persistent volumes; the first published port gets an HTTPS URL. Re-applying reconciles: services removed from the file are removed.</p>
          {p.canEdit ? (
            <div class="row" style={{ marginTop: 14 }}>
              {!src ? <Button kind="ghost" onClick={() => setSrc(composeExample)}>Use example</Button> : null}
              <span class="grow" />
              <Button kind="secondary" onClick={() => run(true)} disabled={!src.trim()}>Preview</Button>
              <Button kind="primary" icon="rocket" onClick={() => run(false)} disabled={!src.trim()}>Apply</Button>
            </div>
          ) : null}
        </Card>
        {plan ? (
          <Card>
            <div class="panel-title">{plan.dryRun ? "What would change" : "Applied"}</div>
            <div class="col" style={{ gap: 6, marginTop: 8, fontSize: 13.5 }}>
              {plan.created?.map((n: string) => <div key={"c" + n}><span class="pill pill-green">create</span> {n}</div>)}
              {plan.updated?.map((n: string) => <div key={"u" + n}><span class="pill pill-blue">update</span> {n}</div>)}
              {plan.removed?.map((n: string) => <div key={"r" + n}><span class="pill pill-red">remove</span> {n}</div>)}
              {plan.warnings?.map((w: string, i: number) => <Callout key={"w" + i} kind="amber">{w}</Callout>)}
              {!plan.created?.length && !plan.updated?.length && !plan.removed?.length ? <div class="muted">No changes.</div> : null}
            </div>
          </Card>
        ) : null}
        {r.data.appIds?.length ? (
          <Card>
            <div class="panel-title">Stack apps</div>
            <div class="col" style={{ gap: 6, marginTop: 8 }}>
              {r.data.appIds.map((aid: string) => {
                const a = p.apps.find((x: any) => x.id === aid);
                return a ? <Link key={aid} href={`/o/${org}/apps/${aid}`} class="row"><Dot status={a.status} /> {a.name}</Link> : null;
              })}
            </div>
          </Card>
        ) : null}
      </div>
    </div>
  );
}

// ---- members ----

function MembersTab({ p, org, reload }: { p: any; org: string; reload: () => void }) {
  const m = useApi<any>(`/orgs/${org}/members`);
  const [pick, setPick] = useState("");
  const candidates = (m.data?.members || []).filter((x: any) => !p.people.find((y: any) => y.id === x.id));
  return (
    <div class="col gap-lg" style={{ maxWidth: 760 }}>
      <Callout kind="info">Project membership is what gates access: members see and change the apps inside. Organization owners and admins can see every project.</Callout>
      {p.canEdit && candidates.length ? (
        <Card>
          <div class="row">
            <Select value={pick} onChange={setPick} class="grow" options={[{ value: "", label: "Add a member of the organization…" }, ...candidates.map((c: any) => ({ value: c.id, label: `${c.name} (@${c.username}) · ${c.role}` }))]} />
            <Button kind="primary" disabled={!pick} onClick={async () => {
              if (await act(() => post(`/projects/${p.id}/members`, { userId: pick }), "Added")) {
                setPick("");
                reload();
              }
            }}>Add</Button>
          </div>
        </Card>
      ) : null}
      <Card pad={false}>
        {p.people.length === 0 ? <div class="muted" style={{ padding: 18 }}>No explicit members.</div> : p.people.map((u: any) => (
          <div key={u.id} class="row" style={{ padding: "12px 16px", borderBottom: "1px solid var(--border)" }}>
            <Avatar seed={u.avatarSeed} size={32} />
            <div class="grow">
              <div style={{ fontWeight: 600 }}>{u.name}</div>
              <div class="muted mono" style={{ fontSize: 12.5 }}>@{u.username}</div>
            </div>
            <span class="tag">{u.role || "member"}</span>
            {p.canEdit ? <IconButton icon="x" title="Remove from project" onClick={async () => {
              if (await confirm({ title: `Remove ${u.name} from ${p.name}?`, confirm: "Remove", danger: true })) {
                await act(() => del(`/projects/${p.id}/members/${u.id}`), "Removed");
                reload();
              }
            }} /> : null}
          </div>
        ))}
      </Card>
    </div>
  );
}

// ---- settings ----

const emojis = ["🌱", "🍎", "🍒", "🍋", "🌳", "🌸", "🍄", "🐝", "🦔", "🚀", "🛰️", "🧪", "🎮", "📚", "🔧", "🏠"];

function SettingsTab({ p, org, reload }: { p: any; org: string; reload: () => void }) {
  const [name, setName] = useState(p.name);
  const [icon, setIcon] = useState(p.icon);
  const [bg, setBg] = useState(p.background);
  const [envName, setEnvName] = useState("");
  return (
    <div class="col gap-lg" style={{ maxWidth: 760 }}>
      <Card class="form-card">
        <div class="panel-title">General</div>
        <Field label="Name"><Input value={name} onInput={setName} disabled={!p.canEdit} /></Field>
        <Field label="Icon"><div class="emoji-row">{emojis.map((e) => <button key={e} type="button" class={cx("emoji-btn", e === icon && "active")} onClick={() => setIcon(e)}>{e}</button>)}</div></Field>
        <Field label="Sky"><BackgroundPicker value={bg} onChange={setBg} seed={p.id} /></Field>
        <Field label="Kubernetes namespace" hint="Every project gets its own namespace, network-isolated from the others."><Input value={p.namespace} onInput={() => {}} disabled mono /></Field>
        {p.canEdit ? <div class="form-card-foot"><Button kind="primary" onClick={async () => { if (await act(() => patch(`/projects/${p.id}`, { name, icon, background: bg }), "Saved")) reload(); }}>Save</Button></div> : null}
      </Card>
      <Card class="form-card">
        <div>
          <div class="panel-title">Environments</div>
          <div class="panel-desc">Named slices of the project. Variables can be scoped to one, so staging and production differ without two projects.</div>
        </div>
        {p.environments.map((e: any) => (
          <div key={e.id} class="row">
            <Icon name="layers" class="muted" />
            <span class="grow">{e.name}</span>
            {p.canEdit && p.environments.length > 1 ? <IconButton icon="trash" title="Delete environment" onClick={async () => {
              if (await confirm({ title: `Delete ${e.name}?`, body: "Its scoped variables are deleted too.", danger: true, confirm: "Delete" })) {
                if (await act(() => del(`/projects/${p.id}/environments/${e.id}`), "Deleted")) reload();
              }
            }} /> : null}
          </div>
        ))}
        {p.canEdit ? (
          <div class="row">
            <Input value={envName} onInput={setEnvName} placeholder="Staging" onEnter={async () => { if (envName && (await act(() => post(`/projects/${p.id}/environments`, { name: envName })))) { setEnvName(""); reload(); } }} />
            <Button kind="secondary" icon="plus" disabled={!envName.trim()} onClick={async () => { if (await act(() => post(`/projects/${p.id}/environments`, { name: envName }))) { setEnvName(""); reload(); } }}>Add</Button>
          </div>
        ) : null}
      </Card>
      {p.canEdit ? (
        <Card class="form-card danger-zone">
          <div>
            <div class="panel-title">Delete project</div>
            <div class="panel-desc">Deletes every app, database, job and volume in it, then the namespace. Database data is not recoverable.</div>
          </div>
          <div class="form-card-foot">
            <Button kind="danger" icon="trash" onClick={async () => {
              if (await confirm({ title: `Delete ${p.name}?`, body: `This removes ${plural(p.apps.length, "app")} and ${plural(p.databases.length, "database")} permanently.`, danger: true, confirm: "Delete project", typeToConfirm: p.name })) {
                if (await act(() => del(`/projects/${p.id}`), "Project deleted")) navigate(`/o/${org}`);
              }
            }}>Delete project</Button>
          </div>
        </Card>
      ) : null}
    </div>
  );
}

export { CodeBlock, Toggle, get };
