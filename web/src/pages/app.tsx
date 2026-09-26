import { h, Fragment, useEffect, useRef, useState } from "../lib/sprout.js";
import { Link, navigate, query, setQuery } from "../lib/router.js";
import { useApi, useEvents, post, patch, del, act, get } from "../lib/api.js";
import { timeAgo, duration, cpu, mem, plural, shortSha, dateTime } from "../lib/format.js";
import { Button, Loading, ErrorBox, Empty, Pill, Tag, Tabs, Card, Chart, Slider, Field, Input, Select, Toggle, confirm, Callout, IconButton, openModal, ModalHeader, Table, CopyButton, Menu, MenuItem, MenuSep, cx, Dot, DefList } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";
import { ImageBadge } from "../ui/art.js";
import { LogView, Terminal, OutputView } from "../ui/term.js";
import { appImageLabel } from "./apps.js";

type Tab = "overview" | "deploys" | "variables" | "domains" | "observe" | "settings";

export function AppPage({ org, id }: { org: string; id: string }) {
  const r = useApi<any>(`/apps/${id}`);
  const [tab, setTabState] = useState<Tab>((query().get("tab") as Tab) || "overview");
  const [metrics, setMetrics] = useState<any[]>([]);
  const [deployTick, setDeployTick] = useState(0);
  useEffect(() => {
    get<any[]>(`/apps/${id}/metrics`).then(setMetrics).catch(() => {});
  }, [id]);
  useEvents([`app:${id}`], (e) => {
    if (e.type === "app.updated") r.set((d) => ({ ...d, ...e.data }));
    else if (e.type === "metrics") setMetrics((m) => [...m.slice(-359), e.data]);
    else if (e.type === "deploy.updated") {
      setDeployTick((t) => t + 1);
      r.set((d) => (d.latestDeploy && d.latestDeploy.id === e.data.id) || e.data.id === d.currentDeploy ? { ...d, latestDeploy: e.data } : d);
    } else if (e.type === "event" || e.type === "crash") r.reload();
  });
  const setTab = (t: Tab) => {
    setTabState(t);
    setQuery("tab", t === "overview" ? null : t);
  };
  if (r.error) return <div class="page"><ErrorBox error={r.error} onRetry={r.reload} /></div>;
  if (!r.data) return <div class="page"><Loading /></div>;
  const a = r.data;
  const url = a.domains?.length ? "https://" + (a.domains.find((d: any) => !d.generated) || a.domains[0]).host : "";
  const deploy = async () => {
    const res = await act(() => post(`/apps/${id}/deploy`));
    if (res) {
      if (res.build && !res.build.started) (await import("../lib/state.js")).toast(res.build.message, "info");
      navigate(`/o/${org}/deploys/${res.deploy.id}`);
    }
  };
  const building = a.status === "building" || a.status === "deploying";
  return (
    <div class="page">
      <Link href={`/o/${org}/projects/${a.project.id}`} class="back-link"><Icon name="arrow-left" size={14} /> {a.project.icon} {a.project.name}{a.envName ? " · " + a.envName : ""}</Link>
      <Card pad={false}>
        <div class="detail-head">
          <ImageBadge image={appImageLabel(a)} name={a.name} size={48} />
          <div class="grow">
            <div class="detail-title">{a.name} <Pill status={a.status} /></div>
            <div class="detail-tags">
              <Tag mono title="Source">{a.source.type === "github" ? <><Icon name="github" size={12} /> {a.source.repo}@{a.source.branch}</> : a.source.image}</Tag>
              <Tag mono>{plural(a.replicas, "replica")}</Tag>
              <Tag mono>{cpu(a.resources.cpuMillis)} · {mem(a.resources.memoryMi)}</Tag>
              {a.authWall ? <Tag><Icon name="lock" size={12} /> auth wall</Tag> : null}
              {a.sandboxed ? <Tag><Icon name="shield" size={12} /> gVisor</Tag> : null}
              {url ? <a href={url} target="_blank" rel="noopener" class="tag mono" style={{ color: "var(--accent-text)" }}><Icon name="external" size={12} /> {url.replace("https://", "")}</a> : null}
            </div>
          </div>
          {a.canEdit ? (
            <div class="detail-actions">
              <Button kind="primary" icon="rocket" onClick={deploy} loading={building && a.latestDeploy?.status === "running"}>{a.image ? "Redeploy" : "Deploy"}</Button>
              <Button kind="secondary" icon="restart" disabled={!a.image} onClick={async () => { await act(() => post(`/apps/${id}/restart`), "Restarting pods on the same image"); }}>Restart</Button>
              <Button kind="secondary" icon="terminal" disabled={!a.pods?.some((p: any) => p.phase === "Running")} onClick={() => openShell(a)}>Shell</Button>
              <Menu align="right" trigger={(o, toggle) => <IconButton icon="more" title="More" class="boxed" onClick={toggle} />}>
                {(close) => (
                  <>
                    {a.replicas > 0 ? (
                      <MenuItem icon="square" onClick={async () => { close(); await act(() => patch(`/apps/${id}`, { replicas: 0 }), "Scaled to zero"); }}>Stop (scale to 0)</MenuItem>
                    ) : (
                      <MenuItem icon="play" onClick={async () => { close(); await act(() => patch(`/apps/${id}`, { replicas: 1 }), "Starting"); }}>Start</MenuItem>
                    )}
                    <MenuItem icon="copy" onClick={() => { close(); navigator.clipboard?.writeText(a.internalHost); }} hint="internal DNS">Copy hostname</MenuItem>
                    <MenuSep />
                    <MenuItem icon="trash" danger onClick={async () => {
                      close();
                      if (await confirm({ title: `Delete ${a.name}?`, body: "Deletes the deployment, its service, routes and volumes. Images are kept in the registry.", danger: true, confirm: "Delete app", typeToConfirm: a.name })) {
                        if (await act(() => del(`/apps/${id}`), "App deleted")) navigate(`/o/${org}/projects/${a.project.id}`);
                      }
                    }}>Delete app</MenuItem>
                  </>
                )}
              </Menu>
            </div>
          ) : null}
        </div>
      </Card>
      <div style={{ margin: "16px 0" }}>
        <Tabs<Tab>
          value={tab}
          onChange={setTab}
          tabs={[
            { id: "overview", label: "Overview", icon: "activity" },
            { id: "deploys", label: "Deploys", icon: "history" },
            { id: "variables", label: "Variables", icon: "key" },
            { id: "domains", label: "Domains", icon: "globe", count: a.domains?.length || 0 },
            { id: "observe", label: "Observe", icon: "alert" },
            { id: "settings", label: "Settings", icon: "settings" },
          ]}
        />
      </div>
      {tab === "overview" ? <Overview a={a} metrics={metrics} org={org} /> : null}
      {tab === "deploys" ? <Deploys a={a} org={org} tick={deployTick} /> : null}
      {tab === "variables" ? <AppVariables a={a} org={org} /> : null}
      {tab === "domains" ? <Domains a={a} reload={r.reload} /> : null}
      {tab === "observe" ? <Observe a={a} /> : null}
      {tab === "settings" ? <Settings a={a} reload={r.reload} /> : null}
    </div>
  );
}

function openShell(a: any) {
  openModal((close) => <ShellModal a={a} close={close} />, { wide: true });
}

function ShellModal({ a, close }: { a: any; close: () => void }) {
  const running = a.pods.filter((p: any) => p.phase === "Running");
  const [pod, setPod] = useState(running[0]?.name || "");
  return (
    <>
      <ModalHeader title={`Shell · ${a.name}`} subtitle="A real process in a real pod: changes are lost on the next restart, and a command that eats the container's memory will get it killed." onClose={close} icon="terminal" />
      <div class="modal-body" style={{ paddingBottom: 18 }}>
        {running.length > 1 ? <Select value={pod} onChange={setPod} options={running.map((p: any) => ({ value: p.name, label: p.name }))} /> : null}
        <Terminal path={`/apps/${a.id}/shell?pod=${encodeURIComponent(pod)}`} title={pod || a.name} height={420} prompt="#" />
      </div>
    </>
  );
}

// ---- overview ----

function Overview({ a, metrics, org }: { a: any; metrics: any[]; org: string }) {
  const [replicas, setReplicas] = useState(a.replicas);
  useEffect(() => setReplicas(a.replicas), [a.replicas]);
  const cpuSeries = metrics.map((m) => ({ t: new Date(m.at).getTime(), v: m.cpuMillis }));
  const memSeries = metrics.map((m) => ({ t: new Date(m.at).getTime(), v: m.memoryMi }));
  const pods = (a.pods || []).filter((p: any) => p.phase !== "Terminating");
  const failing = a.status === "failed" || a.status === "degraded";
  const dep = a.latestDeploy;
  return (
    <>
      {a.statusMessage && failing ? (
        <div class={cx("alert-banner", a.status === "failed" && "red")}>
          <Icon name="alert" size={16} />
          <div>
            <strong>{a.status === "failed" ? "This app is not running." : "Some replicas are unhealthy."}</strong> {a.statusMessage}
            {a.alerts?.length ? <div class="muted" style={{ marginTop: 4 }}>{a.alerts.map((e: any) => e.plain).join(" · ")}</div> : null}
          </div>
        </div>
      ) : null}
      {dep && (dep.status === "running" || dep.status === "queued") ? (
        <Callout kind="info" icon="rocket" title={`Deploy #${dep.number} in progress`}>
          {dep.steps.find((s: any) => s.status === "running")?.name || "Queued"}… <Link href={`/o/${org}/deploys/${dep.id}`}>Watch it</Link>
        </Callout>
      ) : dep && dep.status === "failed" ? (
        <div class="alert-banner red">
          <Icon name="alert" size={16} />
          <div><strong>Deploy #{dep.number} failed.</strong> {dep.error} {a.image ? "The previous version is still running." : ""} <Link href={`/o/${org}/deploys/${dep.id}`}>Read the build log</Link></div>
        </div>
      ) : null}
      <div class="overview-grid" style={{ marginTop: dep && (dep.status !== "succeeded") ? 14 : 0 }}>
        {a.image ? <LogView appId={a.id} pods={pods} /> : (
          <Card>
            <Empty icon="rocket" title="Not deployed yet" action={a.canEdit ? <Button kind="primary" icon="rocket" onClick={async () => { const r = await act(() => post(`/apps/${a.id}/deploy`)); if (r) navigate(`/o/${org}/deploys/${r.deploy.id}`); }}>Deploy now</Button> : null}>
              {a.source.type === "github" ? "Orchard will build the repository on the cluster and roll it out." : "Orchard will pull the image and roll it out."}
            </Empty>
          </Card>
        )}
        <div class="side-stack">
          <Card class="metric-card">
            <div class="metric-head"><Icon name="cpu" size={15} /> CPU <span class="muted">limit {cpu(a.resources.cpuMillis * a.replicas)}</span></div>
            <Chart series={cpuSeries} height={96} format={(v) => cpu(v)} max={0} />
          </Card>
          <Card class="metric-card">
            <div class="metric-head"><Icon name="memory" size={15} /> Memory <span class="muted">limit {mem(a.resources.memoryMi * a.replicas)}</span></div>
            <Chart series={memSeries} height={96} format={(v) => mem(v)} color="var(--blue)" />
          </Card>
          <Card class="metric-card">
            <div class="metric-head"><Icon name="layers" size={15} /> Replicas <span class="muted">{pods.filter((p: any) => p.ready).length}/{a.replicas} ready</span></div>
            {a.canEdit ? (
              <div class="row" style={{ gap: 12 }}>
                <Slider value={replicas} min={0} max={10} onChange={setReplicas} onCommit={async (v) => { if (v !== a.replicas) await act(() => patch(`/apps/${a.id}`, { replicas: v }), `Scaling to ${plural(v, "replica")}`); }} />
                <strong style={{ width: 22, textAlign: "right" }}>{replicas}</strong>
              </div>
            ) : null}
            <div class="pod-list">
              {pods.length === 0 ? <div class="muted" style={{ fontSize: 13 }}>No pods.</div> : pods.map((p: any) => (
                <div class="pod-row" key={p.name}>
                  <Dot status={p.ready ? "running" : p.reason === "CrashLoopBackOff" ? "failed" : "deploying"} />
                  <span class="truncate">{p.name}</span>
                  <span class="right">{p.restarts ? `${p.restarts} restarts · ` : ""}{p.reason && !p.ready ? p.reason : timeAgo(p.startedAt)}</span>
                </div>
              ))}
            </div>
          </Card>
          <Card class="metric-card">
            <div class="metric-head"><Icon name="network" size={15} /> Networking</div>
            <DefList rows={[
              ["Internal", <><span class="truncate">{a.name}</span><CopyButton text={a.internalHost} size="sm" label="" /></>],
              ...(a.ports || []).map((p: any) => [p.name || "port", `${p.port}/${p.protocol}${p.public && p.nodePort ? ` → ${a.publicIp || "public"}:${p.nodePort}` : ""}`] as [any, any]),
              ...(a.domains || []).slice(0, 2).map((d: any) => ["URL", <a href={"https://" + d.host} target="_blank" rel="noopener">{d.host}</a>] as [any, any]),
            ]} />
          </Card>
        </div>
      </div>
    </>
  );
}

// ---- deploys ----

function Deploys({ a, org, tick }: { a: any; org: string; tick: number }) {
  const r = useApi<any>(`/apps/${a.id}/deploys`, [tick]);
  if (!r.data) return <Loading />;
  const list = r.data.deploys;
  if (!list.length) return <Empty icon="history" title="No deploys yet">Every build and rollout lands here, with its image, so rolling back is one click.</Empty>;
  return (
    <>
      <Callout kind="info">A rollback is an ordinary deploy of an older image: same rolling update, same health checks. It does not revert your repository or undo database migrations.</Callout>
      <div style={{ height: 12 }} />
      <Table head={["", "Deploy", "Source", "Trigger", "When", "Took", ""]}>
        {list.map((d: any) => {
          const current = d.image && d.image === r.data.currentImage && d.status === "succeeded";
          return (
            <tr key={d.id} class="clickable" onClick={() => navigate(`/o/${org}/deploys/${d.id}`)}>
              <td style={{ width: 30 }}><Dot status={d.status === "running" ? "deploying" : d.status} /></td>
              <td>
                <div style={{ fontWeight: 600 }}>#{d.number} {current ? <span class="pill pill-green" style={{ marginLeft: 6 }}>current</span> : null}</div>
                <div class="muted" style={{ fontSize: 12.5 }}>{d.kind === "rollback" ? d.message : d.message || d.kind}</div>
              </td>
              <td class="mono" style={{ fontSize: 12.5, maxWidth: 320 }}>
                {d.commit ? <span class="row" style={{ gap: 5 }}><Icon name="commit" size={13} /> {shortSha(d.commit)}</span> : null}
                <span class="muted truncate" style={{ display: "block", maxWidth: 320 }}>{d.image || "—"}</span>
              </td>
              <td><span class="tag">{d.trigger}</span> <span class="muted" style={{ fontSize: 12.5 }}>{d.createdBy}</span></td>
              <td class="nowrap" title={dateTime(d.createdAt)}>{timeAgo(d.createdAt)}</td>
              <td class="mono nowrap" style={{ fontSize: 12.5 }}>{duration(d.createdAt, d.finishedAt)}</td>
              <td style={{ textAlign: "right" }}>
                {a.canEdit && d.status === "succeeded" && d.image && !current ? (
                  <Button size="sm" kind="secondary" icon="history" onClick={async (e: MouseEvent) => {
                    e.stopPropagation();
                    if (await confirm({ title: `Roll back to #${d.number}?`, body: <>Deploys <code>{d.image}</code> through the normal rollout. Database schema changes made since are not undone.</>, confirm: "Roll back" })) {
                      const res = await act(() => post(`/apps/${a.id}/rollback/${d.id}`));
                      if (res) navigate(`/o/${org}/deploys/${res.deploy.id}`);
                    }
                  }}>Roll back</Button>
                ) : null}
              </td>
            </tr>
          );
        })}
      </Table>
    </>
  );
}

// ---- variables (app scope) ----

function AppVariables({ a, org }: { a: any; org: string }) {
  const r = useApi<any[]>(`/projects/${a.project.id}/variables`);
  if (!r.data) return <Loading />;
  const own = r.data.filter((v) => v.appId === a.id);
  const shared = r.data.filter((v) => !v.appId);
  const row = (v: any) => (
    <tr key={v.id}>
      <td class="mono" style={{ fontWeight: 500 }}>{v.key}</td>
      <td class="mono muted truncate" style={{ maxWidth: 380 }}>{v.secret ? "••••••••" : v.value}</td>
      <td>{v.secret ? <Tag><Icon name="lock" size={11} /> secret</Tag> : null}</td>
    </tr>
  );
  return (
    <div class="col gap-lg">
      <div class="row">
        <div class="grow muted">Variables resolve in layers: shared project values, then environment-scoped, then this app's own. Changing them rolls the app.</div>
        <Button kind="primary" icon="pencil" href={`/o/${org}/projects/${a.project.id}?tab=variables`}>Edit variables</Button>
      </div>
      <div>
        <div class="section-title" style={{ marginBottom: 10 }}>This app</div>
        {own.length ? <Table head={["Key", "Value", ""]}>{own.map(row)}</Table> : <div class="muted">None.</div>}
      </div>
      <div>
        <div class="section-title" style={{ marginBottom: 10 }}>Shared by {a.project.name}</div>
        {shared.length ? <Table head={["Key", "Value", ""]}>{shared.map(row)}</Table> : <div class="muted">None.</div>}
      </div>
      <Callout kind="info">Orchard also sets <code>PORT</code>, <code>ORCHARD_APP</code> and <code>ORCHARD_URL</code>.</Callout>
    </div>
  );
}

// ---- domains ----

function Domains({ a, reload }: { a: any; reload: () => void }) {
  const [host, setHost] = useState("");
  const [cname, setCname] = useState("");
  const add = async () => {
    const res = await act(() => post(`/apps/${a.id}/domains`, { host }), "Domain added; a certificate is on its way");
    if (res) {
      setCname(res.cname);
      setHost("");
      reload();
    }
  };
  const httpPort = a.ports.find((p: any) => p.protocol === "http");
  return (
    <div class="col gap-lg" style={{ maxWidth: 820 }}>
      {!httpPort ? <Callout kind="amber">This app exposes no HTTP port, so it has no URL. Add one under Settings → Ports.</Callout> : null}
      <Card pad={false}>
        {a.domains.length === 0 ? <div class="muted" style={{ padding: 18 }}>No domains.</div> : a.domains.map((d: any) => (
          <div class="domain-row" key={d.host} style={{ padding: "12px 16px" }}>
            <Icon name={d.generated ? "globe" : "link"} class="muted" />
            <a href={"https://" + d.host} target="_blank" rel="noopener" class="grow mono" style={{ fontSize: 13.5 }}>{d.host}</a>
            {d.generated ? <Tag>generated</Tag> : null}
            <Pill status={d.certState === "issued" ? "active" : d.certState === "failed" ? "failed" : "provisioning"} label={d.certState === "issued" ? "HTTPS" : d.certState === "failed" ? "Certificate failed" : "Issuing certificate"} />
            {a.canEdit && !d.generated ? <IconButton icon="trash" title="Remove domain" onClick={async () => {
              if (await confirm({ title: `Remove ${d.host}?`, danger: true, confirm: "Remove" })) {
                await act(() => del(`/apps/${a.id}/domains/${encodeURIComponent(d.host)}`), "Removed");
                reload();
              }
            }} /> : null}
          </div>
        ))}
      </Card>
      {a.canEdit && httpPort ? (
        <Card class="form-card">
          <div>
            <div class="panel-title">Add a custom domain</div>
            <div class="panel-desc">Point a CNAME at the instance, add the domain here, and cert-manager issues a certificate automatically.</div>
          </div>
          <div class="row">
            <Input value={host} onInput={(v) => setHost(v.trim().toLowerCase())} placeholder="www.example.com" mono onEnter={add} />
            <Button kind="primary" icon="plus" onClick={add} disabled={!host}>Add</Button>
          </div>
          {cname ? <Callout kind="info" title="Now add this DNS record">
            <code>CNAME {a.domains[a.domains.length - 1]?.host} → {cname}</code>. The certificate is issued over HTTP-01, so it arrives once the record resolves here.
          </Callout> : null}
        </Card>
      ) : null}
    </div>
  );
}

// ---- observe ----

function Observe({ a }: { a: any }) {
  const ev = useApi<any[]>(`/apps/${a.id}/events`);
  const cr = useApi<any[]>(`/apps/${a.id}/crashes`);
  const [open, setOpen] = useState<string | null>(null);
  return (
    <div class="col gap-lg">
      <div>
        <div class="section-title">Crash reports</div>
        <p class="section-desc" style={{ marginBottom: 12 }}>When a container dies, its final output is captured here, including OOM kills. This is what turns “CrashLoopBackOff” into the actual error.</p>
        {!cr.data ? <Loading /> : cr.data.length === 0 ? <Empty icon="check" title="Nothing has crashed">Containers that exit or get killed show up here with their last logs.</Empty> : (
          <div class="col">
            {cr.data.map((c) => (
              <Card key={c.id} pad={false}>
                <div class="row" style={{ padding: "12px 16px", cursor: "pointer" }} onClick={() => setOpen(open === c.id ? null : c.id)}>
                  <Icon name="skull" class="muted" />
                  <div class="grow">
                    <div><strong>{c.reason || "Exited"}</strong> <span class="muted">exit {c.exitCode}{c.exitCode === 137 ? " · out of memory" : ""}</span></div>
                    <div class="muted mono" style={{ fontSize: 12 }}>{c.pod}</div>
                  </div>
                  <span class="muted" style={{ fontSize: 13 }}>{timeAgo(c.createdAt)}</span>
                  <Icon name={open === c.id ? "chevron-up" : "chevron-down"} class="muted" />
                </div>
                {open === c.id ? <div style={{ padding: "0 12px 12px" }}><OutputView text={c.logs} title="last output" height={320} /></div> : null}
              </Card>
            ))}
          </div>
        )}
      </div>
      <div>
        <div class="section-title">Kubernetes events</div>
        <p class="section-desc" style={{ marginBottom: 12 }}>Warnings the cluster recorded for this app's pods, newest first, translated into plain English. Kubernetes keeps these for about an hour after the problem clears.</p>
        {!ev.data ? <Loading /> : ev.data.length === 0 ? <Empty icon="check" title="No warnings">Scheduling problems, image pulls, probe failures and volume mounts show up here.</Empty> : (
          <Table head={["", "What happened", "Object", "Count", "Last seen"]}>
            {ev.data.map((e) => (
              <tr key={e.id}>
                <td style={{ width: 30 }}><Icon name="alert" size={15} class="muted" /></td>
                <td>
                  <div style={{ fontWeight: 500 }}>{e.plain}</div>
                  <div class="muted" style={{ fontSize: 12.5 }}><code>{e.reason}</code> {e.message}</div>
                </td>
                <td class="mono muted" style={{ fontSize: 12 }}>{e.object}</td>
                <td>{e.count}×</td>
                <td class="nowrap">{timeAgo(e.lastSeen)}</td>
              </tr>
            ))}
          </Table>
        )}
      </div>
    </div>
  );
}

// ---- settings ----

const cpuPresets = [50, 100, 250, 500, 1000, 2000, 4000];
const memPresets = [64, 128, 256, 512, 1024, 2048, 4096, 8192];

function Settings({ a, reload }: { a: any; reload: () => void }) {
  const [src, setSrc] = useState({ ...a.source });
  const [res, setRes] = useState({ ...a.resources });
  const [ports, setPorts] = useState<any[]>(a.ports.map((p: any) => ({ ...p })));
  const [vols, setVols] = useState<any[]>((a.volumes || []).map((v: any) => ({ ...v })));
  const [health, setHealth] = useState({ enabled: false, path: "/", port: 0, initialDelaySeconds: 5, ...a.health });
  const [command, setCommand] = useState(a.command || "");
  const dis = !a.canEdit;
  const save = async (body: any, msg = "Saved; rolling out") => {
    if (await act(() => patch(`/apps/${a.id}`, body), msg)) reload();
  };
  return (
    <div class="col gap-lg" style={{ maxWidth: 860 }}>
      <Card class="form-card">
        <div class="panel-title">Source</div>
        {src.type === "github" ? (
          <div class="form-grid">
            <Field label="Repository"><Input value={src.repo} onInput={(v) => setSrc({ ...src, repo: v })} mono disabled={dis} /></Field>
            <Field label="Branch"><Input value={src.branch} onInput={(v) => setSrc({ ...src, branch: v })} mono disabled={dis} /></Field>
            <Field label="Dockerfile"><Input value={src.dockerfile || ""} onInput={(v) => setSrc({ ...src, dockerfile: v })} placeholder="Dockerfile" mono disabled={dis} /></Field>
            <Field label="Build stage" hint="A multi-stage build aimed at an early stage produces an image with no app in it."><Input value={src.target || ""} onInput={(v) => setSrc({ ...src, target: v })} placeholder="final stage" mono disabled={dis} /></Field>
            <Field label="Build context"><Input value={src.context || ""} onInput={(v) => setSrc({ ...src, context: v })} placeholder="." mono disabled={dis} /></Field>
            <div class="span-2"><Toggle checked={src.autoDeploy} onChange={(v) => setSrc({ ...src, autoDeploy: v })} label="Deploy on push" hint={`Every push to ${src.branch} builds and deploys itself.`} disabled={dis} /></div>
          </div>
        ) : (
          <Field label="Image" hint="Saving does not deploy; press Deploy to roll out a new image."><Input value={src.image} onInput={(v) => setSrc({ ...src, image: v })} mono disabled={dis} /></Field>
        )}
        {!dis ? <div class="form-card-foot"><Button kind="primary" onClick={() => save({ source: src }, "Source saved")}>Save source</Button></div> : null}
      </Card>

      <Card class="form-card">
        <div>
          <div class="panel-title">Resources</div>
          <div class="panel-desc">Requests equal limits: what you pick is what the app gets and what it is capped at. Size honestly; quotas count requests.</div>
        </div>
        <div class="form-grid">
          <Field label="CPU">
            <Select value={String(res.cpuMillis)} onChange={(v) => setRes({ ...res, cpuMillis: Number(v) })} disabled={dis} options={[...new Set([...cpuPresets, res.cpuMillis])].sort((x, y) => x - y).map((c) => ({ value: String(c), label: cpu(c) }))} />
          </Field>
          <Field label="Memory">
            <Select value={String(res.memoryMi)} onChange={(v) => setRes({ ...res, memoryMi: Number(v) })} disabled={dis} options={[...new Set([...memPresets, res.memoryMi])].sort((x, y) => x - y).map((m) => ({ value: String(m), label: mem(m) }))} />
          </Field>
        </div>
        {!dis ? <div class="form-card-foot"><Button kind="primary" onClick={() => save({ resources: res })}>Save resources</Button></div> : null}
      </Card>

      <Card class="form-card">
        <div>
          <div class="panel-title">Ports</div>
          <div class="panel-desc">The first HTTP port gets the app's URL. TCP and UDP ports are reachable inside the project; mark them public to publish them on the organization's IP.</div>
        </div>
        {ports.map((p, i) => (
          <div class="row" key={i}>
            <Input value={p.name} onInput={(v) => setPorts(ports.map((x, j) => (j === i ? { ...x, name: v } : x)))} placeholder="name" disabled={dis} />
            <Input value={p.port} type="number" onInput={(v) => setPorts(ports.map((x, j) => (j === i ? { ...x, port: Number(v) } : x)))} placeholder="8080" mono disabled={dis} />
            <Select value={p.protocol} onChange={(v) => setPorts(ports.map((x, j) => (j === i ? { ...x, protocol: v } : x)))} disabled={dis} options={[{ value: "http", label: "HTTP" }, { value: "tcp", label: "TCP" }, { value: "udp", label: "UDP" }]} />
            {p.protocol !== "http" ? <label class="row nowrap" style={{ fontSize: 13 }}><input type="checkbox" checked={p.public} disabled={dis} onChange={(e: any) => setPorts(ports.map((x, j) => (j === i ? { ...x, public: e.target.checked } : x)))} /> public</label> : null}
            {!dis ? <IconButton icon="trash" title="Remove port" onClick={() => setPorts(ports.filter((_, j) => j !== i))} /> : null}
          </div>
        ))}
        {!dis ? (
          <div class="form-card-foot">
            <Button kind="ghost" icon="plus" onClick={() => setPorts([...ports, { name: "", port: 8080, protocol: ports.some((p) => p.protocol === "http") ? "tcp" : "http", public: false }])}>Add port</Button>
            <span class="grow" />
            <Button kind="primary" onClick={() => save({ ports })}>Save ports</Button>
          </div>
        ) : null}
      </Card>

      <Card class="form-card">
        <div>
          <div class="panel-title">Volumes</div>
          <div class="panel-desc">Persistent volumes survive restarts and deploys. They are single-writer: an app with volumes rolls out by recreating its pod.</div>
        </div>
        {vols.map((v, i) => (
          <div class="row" key={i}>
            <Input value={v.name} onInput={(x) => setVols(vols.map((y, j) => (j === i ? { ...y, name: x } : y)))} placeholder="data" disabled={dis} />
            <Input value={v.mountPath} onInput={(x) => setVols(vols.map((y, j) => (j === i ? { ...y, mountPath: x } : y)))} placeholder="/data" mono disabled={dis} />
            <Input value={v.sizeGi} type="number" onInput={(x) => setVols(vols.map((y, j) => (j === i ? { ...y, sizeGi: Number(x) } : y)))} placeholder="1" disabled={dis} />
            <span class="muted">GiB</span>
            {!dis ? <IconButton icon="trash" title="Remove volume" onClick={() => setVols(vols.filter((_, j) => j !== i))} /> : null}
          </div>
        ))}
        {!dis ? (
          <div class="form-card-foot">
            <Button kind="ghost" icon="plus" onClick={() => setVols([...vols, { name: "data", mountPath: "/data", sizeGi: 1 }])}>Add volume</Button>
            <span class="grow" />
            <Button kind="primary" onClick={() => save({ volumes: vols })}>Save volumes</Button>
          </div>
        ) : null}
      </Card>

      <Card class="form-card">
        <div class="panel-title">Health and startup</div>
        <Toggle checked={health.enabled} onChange={(v) => setHealth({ ...health, enabled: v })} label="HTTP health check" hint="Pods only get traffic once this passes, and are restarted if it keeps failing. Re-applied on every rebuild." disabled={dis} />
        {health.enabled ? (
          <div class="form-grid">
            <Field label="Path"><Input value={health.path} onInput={(v) => setHealth({ ...health, path: v })} mono disabled={dis} /></Field>
            <Field label="Initial delay (s)"><Input value={health.initialDelaySeconds} type="number" onInput={(v) => setHealth({ ...health, initialDelaySeconds: Number(v) })} disabled={dis} /></Field>
          </div>
        ) : null}
        <Field label="Start command" hint="Overrides the image's entrypoint, run with sh -c. Leave empty to use the image's own."><Input value={command} onInput={setCommand} mono placeholder="node dist/server.js" disabled={dis} /></Field>
        {!dis ? <div class="form-card-foot"><Button kind="primary" onClick={() => save({ health, command })}>Save</Button></div> : null}
      </Card>

      <Card class="form-card">
        <div class="panel-title">Access and isolation</div>
        <Toggle checked={a.authWall} onChange={(v) => save({ authWall: v }, v ? "Auth wall on" : "Auth wall off")} disabled={dis} label="Auth wall" hint="Only signed-in members of this project can reach the app's URLs. The app sees X-Orchard-User." />
        <Toggle checked={a.sandboxed} onChange={(v) => save({ sandboxed: v })} disabled={dis} label="gVisor sandbox" hint="Runs pods under a user-space kernel. Needs the gvisor RuntimeClass on the nodes; use it for code you do not trust." />
      </Card>
    </div>
  );
}
