import { h, Fragment, useEffect, useRef, useState } from "../lib/sprout.js";
import { Link, navigate, query, setQuery } from "../lib/router.js";
import { useApi, useEvents, post, act } from "../lib/api.js";
import { timeAgo, cpu, mem, plural } from "../lib/format.js";
import { Button, Menu, MenuItem, MenuSep, Loading, ErrorBox, Empty, Dot, cx, openModal, ModalHeader, Field, Input, Segmented, statusKind } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";
import { Sky, ImageBadge, drawSky, palettes } from "../ui/art.js";
import { useOrg } from "../ui/layout.js";

export function appImageLabel(a: any): string {
  if (a.source?.type === "github") return a.source.repo;
  return a.source?.image || a.image || "";
}

export function appSubtitle(a: any): string {
  const gen = (a.domains || []).find((d: any) => d.generated);
  const custom = (a.domains || []).find((d: any) => !d.generated);
  return (custom || gen)?.host || appImageLabel(a);
}

function statusText(s: string) {
  return s[0].toUpperCase() + s.slice(1);
}

export function NewMenu({ org, projectId, label = "New", kind = "primary" }: { org: string; projectId?: string; label?: string; kind?: "primary" | "glass" }) {
  const q = projectId ? "?project=" + projectId : "";
  return (
    <Menu
      align="right"
      trigger={(open, toggle) =>
        kind === "glass" ? (
          <button type="button" class="glass-btn" onClick={(e: Event) => { e.stopPropagation(); toggle(); }}><Icon name="plus" size={13} /> {label}</button>
        ) : (
          <Button kind="primary" icon="plus" iconRight="chevron-down" onClick={toggle}>{label}</Button>
        )
      }
    >
      {(close) => (
        <div onClick={(e: Event) => e.stopPropagation()}>
          {!projectId ? <MenuItem icon="folder" onClick={() => { close(); newProjectModal(org); }}>Project</MenuItem> : null}
          {!projectId ? <MenuSep /> : null}
          <MenuItem icon="github" href={`/o/${org}/new/app${q}${q ? "&" : "?"}source=github`} onClick={close}>GitHub repository</MenuItem>
          <MenuItem icon="box" href={`/o/${org}/new/app${q}${q ? "&" : "?"}source=image`} onClick={close}>Container image</MenuItem>
          <MenuItem icon="database" href={`/o/${org}/new/database${q}`} onClick={close}>PostgreSQL database</MenuItem>
          <MenuItem icon="zap" href={`/o/${org}/new/job${q}`} onClick={close}>Job</MenuItem>
          <MenuSep />
          <MenuItem icon="trees" href={`/o/${org}/grove${q}`} onClick={close}>Template from The Grove</MenuItem>
          {projectId ? <MenuItem icon="layers" href={`/o/${org}/projects/${projectId}?tab=compose`} onClick={close}>Docker Compose</MenuItem> : null}
        </div>
      )}
    </Menu>
  );
}

const emojis = ["🌱", "🍎", "🍒", "🍋", "🌳", "🌸", "🍄", "🐝", "🦔", "🚀", "🛰️", "🧪", "🎮", "📚", "🔧", "🏠"];

export function newProjectModal(org: string, onCreated?: (p: any) => void) {
  openModal((close) => <NewProject org={org} close={close} onCreated={onCreated} />);
}

export function BackgroundPicker({ value, onChange, seed }: { value: string; onChange: (v: string) => void; seed: string }) {
  return (
    <div class="row row-wrap">
      {Object.keys(palettes).map((p) => (
        <button key={p} type="button" class={cx("bg-swatch", value === p && "active")} title={p} onClick={() => onChange(p)}>
          <canvas ref={(el: HTMLCanvasElement | null) => el && !el.dataset.drawn && (drawSky(el, seed, p), (el.dataset.drawn = "1"))} />
        </button>
      ))}
    </div>
  );
}

function NewProject({ org, close, onCreated }: { org: string; close: () => void; onCreated?: (p: any) => void }) {
  const [name, setName] = useState("");
  const [icon, setIcon] = useState("🌱");
  const [bg, setBg] = useState("clouds");
  const submit = async () => {
    const p = await act(() => post(`/orgs/${org}/projects`, { name, icon, background: bg }), "Project created");
    if (p) {
      close();
      onCreated?.(p);
      navigate(`/o/${org}/projects/${p.id}`);
    }
  };
  return (
    <>
      <ModalHeader title="New project" subtitle="A project groups apps, databases and jobs, and gets its own Kubernetes namespace." onClose={close} icon="folder" />
      <div class="modal-body">
        <Field label="Name"><Input value={name} onInput={setName} placeholder="Homelab" autofocus onEnter={submit} /></Field>
        <Field label="Icon">
          <div class="emoji-row">{emojis.map((e) => <button key={e} type="button" class={cx("emoji-btn", e === icon && "active")} onClick={() => setIcon(e)}>{e}</button>)}</div>
        </Field>
        <Field label="Sky"><BackgroundPicker value={bg} onChange={setBg} seed={name || "new"} /></Field>
      </div>
      <div class="modal-foot">
        <Button kind="ghost" onClick={close}>Cancel</Button>
        <Button kind="primary" onClick={submit} disabled={!name.trim()}>Create project</Button>
      </div>
    </>
  );
}

function AppTile({ a, org }: { a: any; org: string }) {
  const m = a.metrics;
  return (
    <Link href={`/o/${org}/apps/${a.id}`} class="app-tile">
      <div class="app-tile-top">
        <ImageBadge image={appImageLabel(a)} name={a.name} />
        <div class="grow">
          <div class="app-tile-name truncate">{a.name}</div>
          <div class="app-tile-sub truncate">{appSubtitle(a)}</div>
        </div>
      </div>
      <div class="app-tile-foot">
        <Dot status={a.status} />
        <span class="status-txt">{statusText(a.status)}</span>
        <span class="right">
          {m ? <span class="nowrap">{Math.round(m.cpuMillis)}m · {Math.round(m.memoryMi)} MiB</span> : null}
          <span class="nowrap">{timeAgo(a.deployedAt || a.createdAt)}</span>
        </span>
      </div>
    </Link>
  );
}

function DbTile({ d, org }: { d: any; org: string }) {
  return (
    <Link href={`/o/${org}/databases/${d.id}`} class="app-tile">
      <div class="app-tile-top">
        <ImageBadge image="postgres" name={d.name} />
        <div class="grow">
          <div class="app-tile-name truncate">{d.name}</div>
          <div class="app-tile-sub truncate">{d.dbName}</div>
        </div>
      </div>
      <div class="app-tile-foot">
        <Dot status={d.status} />
        <span class="status-txt">{statusText(d.status)}</span>
        <span class="right"><span>{timeAgo(d.createdAt)}</span></span>
      </div>
    </Link>
  );
}

function ListRow({ href, image, name, sub, status, right }: { href: string; image: string; name: string; sub: string; status: string; right: string }) {
  return (
    <Link href={href} class="list-row">
      <ImageBadge image={image} name={name} size={30} />
      <div class="grow">
        <div style={{ fontWeight: 600 }}>{name}</div>
        <div class="muted truncate" style={{ fontSize: 13 }}>{sub}</div>
      </div>
      <Dot status={status} />
      <span style={{ width: 90, fontSize: 13 }}>{statusText(status)}</span>
      <span class="muted nowrap" style={{ width: 80, textAlign: "right", fontSize: 13 }}>{right}</span>
    </Link>
  );
}

function ProjectGroup({ p, org, filter, view }: { p: any; org: string; filter: string; view: string }) {
  const key = "orchard-collapsed-" + p.id;
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem(key) === "1";
    } catch {
      return false;
    }
  });
  const toggle = () => {
    const n = !collapsed;
    setCollapsed(n);
    try {
      localStorage.setItem(key, n ? "1" : "0");
    } catch {}
  };
  const f = filter.toLowerCase();
  const apps = p.apps.filter((a: any) => !f || a.name.includes(f) || appImageLabel(a).toLowerCase().includes(f));
  const dbs = p.databases.filter((d: any) => !f || d.name.includes(f) || d.dbName.includes(f));
  if (f && !apps.length && !dbs.length && !p.name.toLowerCase().includes(f)) return null;
  const healthy = p.apps.filter((a: any) => a.status === "running").length + p.databases.filter((d: any) => d.status === "ready").length;
  const bad = p.apps.filter((a: any) => statusKind(a.status) === "red").length;
  const last = [...p.apps.map((a: any) => a.deployedAt || a.createdAt), p.updatedAt].filter(Boolean).sort().pop();
  return (
    <div class={cx("project-group", collapsed && "collapsed-group")}>
      <div class="project-bar" onClick={() => navigate(`/o/${org}/projects/${p.id}`)}>
        <Sky seed={p.id} preset={p.background} />
        <button type="button" class="collapse-btn" title={collapsed ? "Expand" : "Collapse"} onClick={(e: Event) => { e.stopPropagation(); toggle(); }}>
          <Icon name="chevron-down" size={16} />
        </button>
        <div class="project-icon">{p.icon}</div>
        <div class="project-meta">
          <div class="project-name">{p.name}</div>
          <div class="project-sub">
            <span>{p.owner.username}</span>
            <span>{plural(p.apps.length, "deployment")}</span>
            <span>{plural(p.databases.length, "database")}</span>
            <span>{timeAgo(last)}</span>
          </div>
        </div>
        <span class="glass-count" title={`${healthy} healthy${bad ? `, ${bad} failing` : ""}`}>
          <span class={cx("status-dot", bad ? "dot-red" : "dot-green")} />
          {bad || healthy}
        </span>
        {p.canEdit ? <NewMenu org={org} projectId={p.id} kind="glass" /> : null}
        <Link href={`/o/${org}/projects/${p.id}`} class="glass-btn glass-round" title="Open project" onClick={(e: Event) => e.stopPropagation()}>
          <Icon name="arrow-right" size={14} />
        </Link>
      </div>
      {collapsed ? null : apps.length + dbs.length === 0 ? (
        <div style={{ padding: 16 }}>
          <Empty icon="box" title="Nothing planted yet" action={p.canEdit ? <NewMenu org={org} projectId={p.id} label="Add something" /> : null}>
            Deploy a GitHub repo or an image, or add a database.
          </Empty>
        </div>
      ) : view === "list" ? (
        <div class="project-apps list">
          {apps.map((a: any) => <ListRow key={a.id} href={`/o/${org}/apps/${a.id}`} image={appImageLabel(a)} name={a.name} sub={appSubtitle(a)} status={a.status} right={timeAgo(a.deployedAt || a.createdAt)} />)}
          {dbs.map((d: any) => <ListRow key={d.id} href={`/o/${org}/databases/${d.id}`} image="postgres" name={d.name} sub={d.dbName} status={d.status} right={timeAgo(d.createdAt)} />)}
        </div>
      ) : (
        <div class="project-apps">
          {apps.map((a: any) => <AppTile key={a.id} a={a} org={org} />)}
          {dbs.map((d: any) => <DbTile key={d.id} d={d} org={org} />)}
        </div>
      )}
    </div>
  );
}

function RecentDeploys({ org, tick }: { org: string; tick: number }) {
  const r = useApi<any[]>(`/orgs/${org}/deploys/recent`, [tick]);
  return (
    <div class="card card-pad recent">
      <div class="panel-title" style={{ marginBottom: 8 }}>Recent deployments</div>
      {!r.data ? <div class="muted" style={{ padding: "18px 0", textAlign: "center", fontSize: 13.5 }}>Loading…</div> : r.data.length === 0 ? (
        <div class="muted" style={{ padding: "22px 0", textAlign: "center", fontSize: 13.5 }}>No recent deployments</div>
      ) : (
        r.data.map((d) => (
          <Link key={d.id} href={`/o/${org}/deploys/${d.id}`} class="recent-item">
            <Dot status={d.status === "running" ? "deploying" : d.status} />
            <div class="grow">
              <div><strong style={{ fontWeight: 600 }}>{d.appName}</strong> <span class="muted">#{d.number}</span></div>
              <div class="muted truncate" style={{ fontSize: 12.5 }}>{d.message || (d.kind === "rollback" ? "Rollback" : d.image?.split("/").pop() || d.kind)}</div>
            </div>
            <span class="muted nowrap" style={{ fontSize: 12.5 }}>{timeAgo(d.createdAt)}</span>
          </Link>
        ))
      )}
    </div>
  );
}

export function AppsPage({ org }: { org: string }) {
  const o = useOrg(org);
  const [tick, setTick] = useState(0);
  const r = useApi<any[]>(`/orgs/${org}/overview`, [tick]);
  const [filter, setFilter] = useState("");
  const [view, setView] = useState(() => {
    try {
      return localStorage.getItem("orchard-view") || "grid";
    } catch {
      return "grid";
    }
  });
  const pending = useRef<any>(null);
  useEvents(r.data ? r.data.map((p) => "project:" + p.id) : [], () => {
    // coalesce bursts of updates into one refetch
    if (pending.current) return;
    pending.current = setTimeout(() => {
      pending.current = null;
      setTick((t) => t + 1);
    }, 400);
  });
  useEffect(() => {
    if (query().get("new") === "project") {
      setQuery("new", null);
      newProjectModal(org);
    }
  }, []);
  const canCreate = o && o.role !== "viewer";
  return (
    <div class="page">
      <div class="page-head">
        <h1 class="page-title">Your Apps</h1>
        <div class="page-actions">{canCreate ? <NewMenu org={org} /> : null}</div>
      </div>
      <div class="apps-layout">
        <div>
          <div class="apps-toolbar">
            <div class="search-input">
              <Icon name="search" size={15} />
              <input class="input" placeholder="Search apps..." value={filter} onInput={(e: any) => setFilter(e.target.value)} />
            </div>
            <span class="grow" />
            <Segmented
              size="sm"
              value={view}
              onChange={(v) => {
                setView(v);
                try {
                  localStorage.setItem("orchard-view", v);
                } catch {}
              }}
              options={[{ id: "list", label: "", icon: "list", title: "List" }, { id: "grid", label: "", icon: "grid", title: "Cards" }]}
            />
          </div>
          {r.error ? <ErrorBox error={r.error} onRetry={r.reload} /> : !r.data ? <Loading /> : r.data.length === 0 ? (
            <Empty icon="sprout" title="Plant your first project" action={canCreate ? <Button kind="primary" icon="plus" onClick={() => newProjectModal(org)}>New project</Button> : null}>
              Projects hold apps, databases and jobs that belong together. Start one, then deploy a GitHub repo or a container image into it.
            </Empty>
          ) : (
            r.data.map((p) => <ProjectGroup key={p.id} p={p} org={org} filter={filter} view={view} />)
          )}
        </div>
        <RecentDeploys org={org} tick={tick} />
      </div>
    </div>
  );
}
