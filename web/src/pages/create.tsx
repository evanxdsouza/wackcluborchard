import { h, Fragment, useEffect, useState } from "../lib/sprout.js";
import { Link, navigate, query } from "../lib/router.js";
import { useApi, post, get, act } from "../lib/api.js";
import { cpu, mem, timeAgo } from "../lib/format.js";
import { session, use } from "../lib/state.js";
import { Button, Loading, Card, Field, Input, Select, Toggle, Callout, cx, IconButton, Spinner } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";

function useProjects(org: string) {
  const r = useApi<any[]>(`/orgs/${org}/overview`);
  return (r.data || []).filter((p) => p.canEdit);
}

function ProjectEnvPicker({ org, projectId, setProjectId, envId, setEnvId }: { org: string; projectId: string; setProjectId: (v: string) => void; envId: string; setEnvId: (v: string) => void }) {
  const projects = useProjects(org);
  useEffect(() => {
    if (!projectId && projects.length) setProjectId(projects[0].id);
  }, [projects.length]);
  const p = projects.find((x) => x.id === projectId);
  useEffect(() => {
    if (p && !p.environments.find((e: any) => e.id === envId)) setEnvId(p.environments[0].id);
  }, [projectId, p?.id]);
  if (!projects.length) return <Callout kind="amber">Create a project first: <Link href={`/o/${org}?new=project`}>new project</Link>.</Callout>;
  return (
    <div class="form-grid">
      <Field label="Project"><Select value={projectId} onChange={setProjectId} options={projects.map((x) => ({ value: x.id, label: `${x.icon} ${x.name}` }))} /></Field>
      <Field label="Environment"><Select value={envId} onChange={setEnvId} options={(p?.environments || []).map((e: any) => ({ value: e.id, label: e.name }))} /></Field>
    </div>
  );
}

const sizes = [
  { id: "xs", label: "Tiny", cpu: 100, mem: 128 },
  { id: "s", label: "Small", cpu: 250, mem: 256 },
  { id: "m", label: "Medium", cpu: 500, mem: 512 },
  { id: "l", label: "Large", cpu: 1000, mem: 1024 },
  { id: "xl", label: "X-Large", cpu: 2000, mem: 4096 },
];

function SizePicker({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <div class="row row-wrap" style={{ gap: 8 }}>
      {sizes.map((s) => (
        <button key={s.id} type="button" class={cx("choice", value === s.id && "active")} style={{ flex: "1 1 110px", padding: 10 }} onClick={() => onChange(s.id)}>
          <div>
            <div class="choice-title">{s.label}</div>
            <div class="choice-desc mono">{cpu(s.cpu)} · {mem(s.mem)}</div>
          </div>
        </button>
      ))}
    </div>
  );
}

function EnvRows({ rows, setRows }: { rows: { k: string; v: string }[]; setRows: (r: { k: string; v: string }[]) => void }) {
  return (
    <div class="col" style={{ gap: 8 }}>
      {rows.map((r, i) => (
        <div class="row" key={i}>
          <Input value={r.k} onInput={(x) => setRows(rows.map((y, j) => (j === i ? { ...y, k: x.toUpperCase().replace(/[^A-Z0-9_]/g, "_") } : y)))} placeholder="KEY" mono />
          <Input value={r.v} onInput={(x) => setRows(rows.map((y, j) => (j === i ? { ...y, v: x } : y)))} placeholder="value" mono />
          <IconButton icon="x" title="Remove" onClick={() => setRows(rows.filter((_, j) => j !== i))} />
        </div>
      ))}
      <div><Button size="sm" kind="ghost" icon="plus" onClick={() => setRows([...rows, { k: "", v: "" }])}>Add variable</Button></div>
    </div>
  );
}

export function NewAppPage({ org }: { org: string }) {
  const s = use(session);
  const q = query();
  const [source, setSource] = useState<"github" | "image">((q.get("source") as any) === "image" ? "image" : "github");
  const [projectId, setProjectId] = useState(q.get("project") || "");
  const [envId, setEnvId] = useState("");
  const [name, setName] = useState("");
  const [nameTouched, setNameTouched] = useState(false);
  const [image, setImage] = useState(q.get("image") || "");
  const [repo, setRepo] = useState<any>(null);
  const [branch, setBranch] = useState("");
  const [branches, setBranches] = useState<string[]>([]);
  const [inspect, setInspect] = useState<any>(null);
  const [inspecting, setInspecting] = useState(false);
  const [target, setTarget] = useState("");
  const [dockerfile, setDockerfile] = useState("Dockerfile");
  const [port, setPort] = useState("8080");
  const [protocol, setProtocol] = useState("http");
  const [replicas, setReplicas] = useState("1");
  const [size, setSize] = useState("s");
  const [env, setEnv] = useState<{ k: string; v: string }[]>([]);
  const [deploy, setDeploy] = useState(true);
  const [repoFilter, setRepoFilter] = useState("");
  const gh = useApi<any>("/github/status");
  const repos = useApi<any[]>(source === "github" && gh.data?.login ? "/github/repos" : null, [source, gh.data?.login]);

  const autoName = (x: string) => {
    if (nameTouched) return;
    const base = x.split("/").pop()!.split(":")[0].split("@")[0].toLowerCase().replace(/[^a-z0-9-]/g, "-").replace(/^-+|-+$/g, "").slice(0, 40);
    setName(base || "");
  };
  const pickRepo = async (r: any) => {
    setRepo(r);
    autoName(r.fullName);
    setBranch(r.defaultBranch);
    get<string[]>("/github/branches?repo=" + encodeURIComponent(r.fullName)).then(setBranches).catch(() => setBranches([r.defaultBranch]));
  };
  useEffect(() => {
    if (!repo || !branch) return;
    setInspecting(true);
    get(`/github/inspect?repo=${encodeURIComponent(repo.fullName)}&branch=${encodeURIComponent(branch)}&dockerfile=${encodeURIComponent(dockerfile)}`)
      .then((i) => {
        setInspect(i);
        setTarget(i.target || "");
        if (i.port) setPort(String(i.port));
        if (i.env?.length) setEnv(i.env.filter((k: string) => !["PATH", "HOME"].includes(k)).map((k: string) => ({ k, v: "" })));
      })
      .catch(() => setInspect(null))
      .finally(() => setInspecting(false));
  }, [repo?.fullName, branch, dockerfile]);

  const sz = sizes.find((x) => x.id === size)!;
  const canSubmit = name && projectId && (source === "image" ? image.trim() : repo && branch);
  const submit = async () => {
    const vars: Record<string, string> = {};
    for (const r of env) if (r.k && r.v !== "") vars[r.k] = r.v;
    const portN = Number(port);
    const body = {
      name,
      envId,
      source: source === "image" ? { type: "image", image: image.trim() } : { type: "github", repo: repo.fullName, branch, dockerfile, target, autoDeploy: true },
      replicas: Number(replicas) || 1,
      ports: portN > 0 ? [{ name: protocol === "http" ? "http" : protocol, port: portN, protocol }] : [],
      resources: { cpuMillis: sz.cpu, memoryMi: sz.mem },
      env: vars,
      deploy,
    };
    const a = await act(() => post(`/projects/${projectId}/apps`, body), deploy ? "Created; deploying" : "Created");
    if (a) navigate(`/o/${org}/apps/${a.id}`);
  };
  const filteredRepos = (repos.data || []).filter((r) => !repoFilter || r.fullName.toLowerCase().includes(repoFilter.toLowerCase()));
  return (
    <div class="page-mid" style={{ maxWidth: 820 }}>
      <Link href={projectId ? `/o/${org}/projects/${projectId}` : `/o/${org}`} class="back-link"><Icon name="arrow-left" size={14} /> Back</Link>
      <div class="page-head"><div><h1 class="page-title" style={{ fontSize: 26 }}>New app</h1><div class="page-sub">Build a repository on the cluster, or run an image that already exists.</div></div></div>
      <div class="col gap-lg">
        <div class="choice-grid">
          <button type="button" class={cx("choice", source === "github" && "active")} onClick={() => setSource("github")}>
            <div class="choice-icon"><Icon name="github" size={18} /></div>
            <div><div class="choice-title">GitHub repository</div><div class="choice-desc">Wack Club Orchard reads the Dockerfile, builds on the cluster and redeploys on every push.</div></div>
          </button>
          <button type="button" class={cx("choice", source === "image" && "active")} onClick={() => setSource("image")}>
            <div class="choice-icon"><Icon name="box" size={18} /></div>
            <div><div class="choice-title">Container image</div><div class="choice-desc">Any image from a registry, like <span class="mono">nginx:alpine</span>.</div></div>
          </button>
        </div>

        <Card class="form-card">
          <ProjectEnvPicker org={org} projectId={projectId} setProjectId={setProjectId} envId={envId} setEnvId={setEnvId} />
        </Card>

        {source === "github" ? (
          <Card class="form-card">
            <div class="panel-title">Repository</div>
            {!gh.data ? <Loading /> : !gh.data.configured ? (
              <Callout kind="amber" title="GitHub is not connected to this instance yet.">
                {s.me?.superadmin ? <>Create the GitHub App from <Link href="/admin?tab=github">Instance admin → GitHub</Link>; it takes one click.</> : "Ask an instance admin to set up the GitHub App."}
              </Callout>
            ) : !gh.data.login ? (
              <div class="row">
                <div class="grow muted">Link your GitHub account. Wack Club Orchard only ever sees the repositories you grant it.</div>
                <Button kind="primary" icon="github" href={`/api/github/connect?next=${encodeURIComponent(location.pathname + location.search)}`}>Connect GitHub</Button>
              </div>
            ) : repo ? (
              <>
                <div class="row">
                  <Icon name="github" />
                  <strong class="grow mono">{repo.fullName}</strong>
                  <Button size="sm" kind="ghost" onClick={() => { setRepo(null); setInspect(null); }}>Change</Button>
                </div>
                <div class="form-grid">
                  <Field label="Branch"><Select value={branch} onChange={setBranch} options={(branches.length ? branches : [branch]).map((b) => ({ value: b, label: b }))} /></Field>
                  <Field label="Dockerfile path"><Input value={dockerfile} onInput={setDockerfile} mono /></Field>
                  <Field label="Build stage" hint={inspect?.stages?.length > 1 ? "Multi-stage build: we picked the last stage. A stage like base has no app in it." : "The final stage is built by default."}>
                    {inspect?.stages?.length ? (
                      <Select value={target} onChange={setTarget} options={[{ value: "", label: "(final stage)" }, ...inspect.stages.filter((x: string) => !x.startsWith("stage-")).map((x: string) => ({ value: x, label: x }))]} />
                    ) : <Input value={target} onInput={setTarget} placeholder="(final stage)" mono />}
                  </Field>
                  <Field label="Detected">
                    <div class="muted" style={{ fontSize: 13, paddingTop: 8 }}>
                      {inspecting ? <><Spinner size="sm" /> reading Dockerfile…</> : inspect?.found ? <>{inspect.stages.length} stage{inspect.stages.length === 1 ? "" : "s"}{inspect.port ? `, port ${inspect.port}` : ""}{inspect.env?.length ? `, ${inspect.env.length} variables` : ""}</> : <span style={{ color: "var(--amber)" }}>No Dockerfile at {dockerfile}</span>}
                    </div>
                  </Field>
                </div>
              </>
            ) : (
              <>
                <Input value={repoFilter} onInput={setRepoFilter} placeholder="Search your repositories…" />
                {!repos.data ? <Loading /> : (
                  <div class="repo-list">
                    {filteredRepos.length === 0 ? <div class="muted" style={{ padding: 16 }}>No repositories. <a href={gh.data.installUrl} target="_blank" rel="noopener">Grant access to more</a>.</div> : filteredRepos.map((r) => (
                      <button key={r.fullName} type="button" class="repo-item" onClick={() => pickRepo(r)}>
                        <Icon name={r.private ? "lock" : "git"} size={15} class="muted" />
                        <span class="grow"><span class="mono" style={{ fontSize: 13.5 }}>{r.fullName}</span> <span class="muted" style={{ fontSize: 12.5 }}>{r.language}</span></span>
                        <span class="muted" style={{ fontSize: 12.5 }}>{timeAgo(r.pushedAt)}</span>
                      </button>
                    ))}
                  </div>
                )}
              </>
            )}
          </Card>
        ) : (
          <Card class="form-card">
            <Field label="Image" hint="Registries that need credentials: add DOCKER_AUTH_CONFIG as a secret variable.">
              <Input value={image} onInput={(v) => { setImage(v); autoName(v); }} placeholder="ghcr.io/you/app:latest" mono autofocus />
            </Field>
            <div class="row row-wrap" style={{ gap: 6 }}>
              {["nginx:alpine", "traefik/whoami", "redis:7-alpine", "ghcr.io/umami-software/umami:postgresql-latest"].map((x) => (
                <button key={x} type="button" class="tag mono" style={{ cursor: "pointer" }} onClick={() => { setImage(x); autoName(x); if (x.includes("nginx") || x.includes("whoami")) setPort("80"); if (x.includes("redis")) { setPort("6379"); setProtocol("tcp"); } if (x.includes("umami")) setPort("3000"); }}>{x}</button>
              ))}
            </div>
          </Card>
        )}

        <Card class="form-card">
          <div class="form-grid">
            <Field label="App name" hint="Lowercase; also the hostname inside the project.">
              <Input value={name} onInput={(v) => { setName(v.toLowerCase().replace(/[^a-z0-9-]/g, "-")); setNameTouched(true); }} mono placeholder="api" />
            </Field>
            <Field label="Replicas"><Input value={replicas} type="number" min={0} max={20} onInput={setReplicas} /></Field>
            <Field label="Port" hint="0 for a worker with no port.">
              <Input value={port} type="number" onInput={setPort} mono />
            </Field>
            <Field label="Protocol">
              <Select value={protocol} onChange={setProtocol} options={[{ value: "http", label: "HTTP (gets an HTTPS URL)" }, { value: "tcp", label: "TCP (internal)" }, { value: "udp", label: "UDP (internal)" }]} />
            </Field>
            <div class="span-2"><Field label="Size" hint="Requests equal limits: what you pick is what you get and what you are capped at."><SizePicker value={size} onChange={setSize} /></Field></div>
          </div>
        </Card>

        <Card class="form-card">
          <div>
            <div class="panel-title">Variables</div>
            <div class="panel-desc">{inspect?.env?.length ? "Declared in the Dockerfile; fill in the ones you need." : "App-scoped values. Reference databases with ${{ dbname.DATABASE_URL }}."}</div>
          </div>
          <EnvRows rows={env} setRows={setEnv} />
        </Card>

        <div class="row">
          <Toggle checked={deploy} onChange={setDeploy} label="Deploy right away" />
          <span class="grow" />
          <Button kind="primary" icon="rocket" onClick={submit} disabled={!canSubmit}>{deploy ? "Create and deploy" : "Create app"}</Button>
        </div>
      </div>
    </div>
  );
}

export function NewDatabasePage({ org }: { org: string }) {
  const [projectId, setProjectId] = useState(query().get("project") || "");
  const [envId, setEnvId] = useState("");
  const [name, setName] = useState("postgres");
  const [version, setVersion] = useState("17");
  const [storage, setStorage] = useState("10");
  const [size, setSize] = useState("m");
  const sz = sizes.find((x) => x.id === size)!;
  const submit = async () => {
    const db = await act(() => post(`/projects/${projectId}/databases`, { name, envId, version: Number(version), storageGi: Number(storage), resources: { cpuMillis: sz.cpu, memoryMi: sz.mem } }), "Provisioning PostgreSQL");
    if (db) navigate(`/o/${org}/databases/${db.id}`);
  };
  return (
    <div class="page-mid" style={{ maxWidth: 760 }}>
      <Link href={`/o/${org}`} class="back-link"><Icon name="arrow-left" size={14} /> Back</Link>
      <div class="page-head"><div><h1 class="page-title" style={{ fontSize: 26 }}>New database</h1><div class="page-sub">Managed PostgreSQL on CloudNativePG, with backups, extensions, read replicas and a query console.</div></div></div>
      <div class="col gap-lg">
        <Card class="form-card"><ProjectEnvPicker org={org} projectId={projectId} setProjectId={setProjectId} envId={envId} setEnvId={setEnvId} /></Card>
        <Card class="form-card">
          <div class="form-grid">
            <Field label="Name" hint="Referenced from apps as ${{ name.DATABASE_URL }}."><Input value={name} onInput={(v) => setName(v.toLowerCase().replace(/[^a-z0-9-]/g, "-"))} mono autofocus /></Field>
            <Field label="Version"><Select value={version} onChange={setVersion} options={["18", "17", "16", "15", "14"].map((v) => ({ value: v, label: "PostgreSQL " + v }))} /></Field>
            <Field label="Storage (GiB)" hint="Can grow later, never shrink."><Input value={storage} type="number" onInput={setStorage} /></Field>
            <div />
            <div class="span-2"><Field label="Size"><SizePicker value={size} onChange={setSize} /></Field></div>
          </div>
        </Card>
        <div class="row"><span class="grow" /><Button kind="primary" icon="database" onClick={submit} disabled={!name || !projectId}>Create database</Button></div>
      </div>
    </div>
  );
}
