import { h, Fragment, useEffect, useState } from "../lib/sprout.js";
import { Link, navigate, query } from "../lib/router.js";
import { useApi, post, get, act } from "../lib/api.js";
import { cpu, mem, timeAgo } from "../lib/format.js";
import { session, use } from "../lib/state.js";
import { Button, Loading, Card, Field, Input, Select, Toggle, Callout, cx, IconButton, Spinner } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";
function useProjects(org) {
    const r = useApi(`/orgs/${org}/overview`);
    return (r.data || []).filter((p) => p.canEdit);
}
function ProjectEnvPicker({ org, projectId, setProjectId, envId, setEnvId }) {
    const projects = useProjects(org);
    useEffect(() => {
        if (!projectId && projects.length)
            setProjectId(projects[0].id);
    }, [projects.length]);
    const p = projects.find((x) => x.id === projectId);
    useEffect(() => {
        if (p && !p.environments.find((e) => e.id === envId))
            setEnvId(p.environments[0].id);
    }, [projectId, p?.id]);
    if (!projects.length)
        return h(Callout, { kind: "amber" },
            "Create a project first: ",
            h(Link, { href: `/o/${org}?new=project` }, "new project"),
            ".");
    return (h("div", { class: "form-grid" },
        h(Field, { label: "Project" },
            h(Select, { value: projectId, onChange: setProjectId, options: projects.map((x) => ({ value: x.id, label: `${x.icon} ${x.name}` })) })),
        h(Field, { label: "Environment" },
            h(Select, { value: envId, onChange: setEnvId, options: (p?.environments || []).map((e) => ({ value: e.id, label: e.name })) }))));
}
const sizes = [
    { id: "xs", label: "Tiny", cpu: 100, mem: 128 },
    { id: "s", label: "Small", cpu: 250, mem: 256 },
    { id: "m", label: "Medium", cpu: 500, mem: 512 },
    { id: "l", label: "Large", cpu: 1000, mem: 1024 },
    { id: "xl", label: "X-Large", cpu: 2000, mem: 4096 },
];
function SizePicker({ value, onChange }) {
    return (h("div", { class: "row row-wrap", style: { gap: 8 } }, sizes.map((s) => (h("button", { key: s.id, type: "button", class: cx("choice", value === s.id && "active"), style: { flex: "1 1 110px", padding: 10 }, onClick: () => onChange(s.id) },
        h("div", null,
            h("div", { class: "choice-title" }, s.label),
            h("div", { class: "choice-desc mono" },
                cpu(s.cpu),
                " \u00B7 ",
                mem(s.mem))))))));
}
function EnvRows({ rows, setRows }) {
    return (h("div", { class: "col", style: { gap: 8 } },
        rows.map((r, i) => (h("div", { class: "row", key: i },
            h(Input, { value: r.k, onInput: (x) => setRows(rows.map((y, j) => (j === i ? { ...y, k: x.toUpperCase().replace(/[^A-Z0-9_]/g, "_") } : y))), placeholder: "KEY", mono: true }),
            h(Input, { value: r.v, onInput: (x) => setRows(rows.map((y, j) => (j === i ? { ...y, v: x } : y))), placeholder: "value", mono: true }),
            h(IconButton, { icon: "x", title: "Remove", onClick: () => setRows(rows.filter((_, j) => j !== i)) })))),
        h("div", null,
            h(Button, { size: "sm", kind: "ghost", icon: "plus", onClick: () => setRows([...rows, { k: "", v: "" }]) }, "Add variable"))));
}
export function NewAppPage({ org }) {
    const s = use(session);
    const q = query();
    const [source, setSource] = useState(q.get("source") === "image" ? "image" : "github");
    const [projectId, setProjectId] = useState(q.get("project") || "");
    const [envId, setEnvId] = useState("");
    const [name, setName] = useState("");
    const [nameTouched, setNameTouched] = useState(false);
    const [image, setImage] = useState(q.get("image") || "");
    const [repo, setRepo] = useState(null);
    const [branch, setBranch] = useState("");
    const [branches, setBranches] = useState([]);
    const [inspect, setInspect] = useState(null);
    const [inspecting, setInspecting] = useState(false);
    const [target, setTarget] = useState("");
    const [dockerfile, setDockerfile] = useState("Dockerfile");
    const [port, setPort] = useState("8080");
    const [protocol, setProtocol] = useState("http");
    const [replicas, setReplicas] = useState("1");
    const [size, setSize] = useState("s");
    const [env, setEnv] = useState([]);
    const [deploy, setDeploy] = useState(true);
    const [repoFilter, setRepoFilter] = useState("");
    const gh = useApi("/github/status");
    const repos = useApi(source === "github" && gh.data?.login ? "/github/repos" : null, [source, gh.data?.login]);
    const autoName = (x) => {
        if (nameTouched)
            return;
        const base = x.split("/").pop().split(":")[0].split("@")[0].toLowerCase().replace(/[^a-z0-9-]/g, "-").replace(/^-+|-+$/g, "").slice(0, 40);
        setName(base || "");
    };
    const pickRepo = async (r) => {
        setRepo(r);
        autoName(r.fullName);
        setBranch(r.defaultBranch);
        get("/github/branches?repo=" + encodeURIComponent(r.fullName)).then(setBranches).catch(() => setBranches([r.defaultBranch]));
    };
    useEffect(() => {
        if (!repo || !branch)
            return;
        setInspecting(true);
        get(`/github/inspect?repo=${encodeURIComponent(repo.fullName)}&branch=${encodeURIComponent(branch)}&dockerfile=${encodeURIComponent(dockerfile)}`)
            .then((i) => {
            setInspect(i);
            setTarget(i.target || "");
            if (i.port)
                setPort(String(i.port));
            if (i.env?.length)
                setEnv(i.env.filter((k) => !["PATH", "HOME"].includes(k)).map((k) => ({ k, v: "" })));
        })
            .catch(() => setInspect(null))
            .finally(() => setInspecting(false));
    }, [repo?.fullName, branch, dockerfile]);
    const sz = sizes.find((x) => x.id === size);
    const canSubmit = name && projectId && (source === "image" ? image.trim() : repo && branch);
    const submit = async () => {
        const vars = {};
        for (const r of env)
            if (r.k && r.v !== "")
                vars[r.k] = r.v;
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
        if (a)
            navigate(`/o/${org}/apps/${a.id}`);
    };
    const filteredRepos = (repos.data || []).filter((r) => !repoFilter || r.fullName.toLowerCase().includes(repoFilter.toLowerCase()));
    return (h("div", { class: "page-mid", style: { maxWidth: 820 } },
        h(Link, { href: projectId ? `/o/${org}/projects/${projectId}` : `/o/${org}`, class: "back-link" },
            h(Icon, { name: "arrow-left", size: 14 }),
            " Back"),
        h("div", { class: "page-head" },
            h("div", null,
                h("h1", { class: "page-title", style: { fontSize: 26 } }, "New app"),
                h("div", { class: "page-sub" }, "Build a repository on the cluster, or run an image that already exists."))),
        h("div", { class: "col gap-lg" },
            h("div", { class: "choice-grid" },
                h("button", { type: "button", class: cx("choice", source === "github" && "active"), onClick: () => setSource("github") },
                    h("div", { class: "choice-icon" },
                        h(Icon, { name: "github", size: 18 })),
                    h("div", null,
                        h("div", { class: "choice-title" }, "GitHub repository"),
                        h("div", { class: "choice-desc" }, "Orchard reads the Dockerfile, builds on the cluster and redeploys on every push."))),
                h("button", { type: "button", class: cx("choice", source === "image" && "active"), onClick: () => setSource("image") },
                    h("div", { class: "choice-icon" },
                        h(Icon, { name: "box", size: 18 })),
                    h("div", null,
                        h("div", { class: "choice-title" }, "Container image"),
                        h("div", { class: "choice-desc" },
                            "Any image from a registry, like ",
                            h("span", { class: "mono" }, "nginx:alpine"),
                            ".")))),
            h(Card, { class: "form-card" },
                h(ProjectEnvPicker, { org: org, projectId: projectId, setProjectId: setProjectId, envId: envId, setEnvId: setEnvId })),
            source === "github" ? (h(Card, { class: "form-card" },
                h("div", { class: "panel-title" }, "Repository"),
                !gh.data ? h(Loading, null) : !gh.data.configured ? (h(Callout, { kind: "amber", title: "GitHub is not connected to this instance yet." }, s.me?.superadmin ? h(Fragment, null,
                    "Create the GitHub App from ",
                    h(Link, { href: "/admin?tab=github" }, "Instance admin \u2192 GitHub"),
                    "; it takes one click.") : "Ask an instance admin to set up the GitHub App.")) : !gh.data.login ? (h("div", { class: "row" },
                    h("div", { class: "grow muted" }, "Link your GitHub account. Orchard only ever sees the repositories you grant it."),
                    h(Button, { kind: "primary", icon: "github", href: `/api/github/connect?next=${encodeURIComponent(location.pathname + location.search)}` }, "Connect GitHub"))) : repo ? (h(Fragment, null,
                    h("div", { class: "row" },
                        h(Icon, { name: "github" }),
                        h("strong", { class: "grow mono" }, repo.fullName),
                        h(Button, { size: "sm", kind: "ghost", onClick: () => { setRepo(null); setInspect(null); } }, "Change")),
                    h("div", { class: "form-grid" },
                        h(Field, { label: "Branch" },
                            h(Select, { value: branch, onChange: setBranch, options: (branches.length ? branches : [branch]).map((b) => ({ value: b, label: b })) })),
                        h(Field, { label: "Dockerfile path" },
                            h(Input, { value: dockerfile, onInput: setDockerfile, mono: true })),
                        h(Field, { label: "Build stage", hint: inspect?.stages?.length > 1 ? "Multi-stage build: we picked the last stage. A stage like base has no app in it." : "The final stage is built by default." }, inspect?.stages?.length ? (h(Select, { value: target, onChange: setTarget, options: [{ value: "", label: "(final stage)" }, ...inspect.stages.filter((x) => !x.startsWith("stage-")).map((x) => ({ value: x, label: x }))] })) : h(Input, { value: target, onInput: setTarget, placeholder: "(final stage)", mono: true })),
                        h(Field, { label: "Detected" },
                            h("div", { class: "muted", style: { fontSize: 13, paddingTop: 8 } }, inspecting ? h(Fragment, null,
                                h(Spinner, { size: "sm" }),
                                " reading Dockerfile\u2026") : inspect?.found ? h(Fragment, null,
                                inspect.stages.length,
                                " stage",
                                inspect.stages.length === 1 ? "" : "s",
                                inspect.port ? `, port ${inspect.port}` : "",
                                inspect.env?.length ? `, ${inspect.env.length} variables` : "") : h("span", { style: { color: "var(--amber)" } },
                                "No Dockerfile at ",
                                dockerfile)))))) : (h(Fragment, null,
                    h(Input, { value: repoFilter, onInput: setRepoFilter, placeholder: "Search your repositories\u2026" }),
                    !repos.data ? h(Loading, null) : (h("div", { class: "repo-list" }, filteredRepos.length === 0 ? h("div", { class: "muted", style: { padding: 16 } },
                        "No repositories. ",
                        h("a", { href: gh.data.installUrl, target: "_blank", rel: "noopener" }, "Grant access to more"),
                        ".") : filteredRepos.map((r) => (h("button", { key: r.fullName, type: "button", class: "repo-item", onClick: () => pickRepo(r) },
                        h(Icon, { name: r.private ? "lock" : "git", size: 15, class: "muted" }),
                        h("span", { class: "grow" },
                            h("span", { class: "mono", style: { fontSize: 13.5 } }, r.fullName),
                            " ",
                            h("span", { class: "muted", style: { fontSize: 12.5 } }, r.language)),
                        h("span", { class: "muted", style: { fontSize: 12.5 } }, timeAgo(r.pushedAt))))))))))) : (h(Card, { class: "form-card" },
                h(Field, { label: "Image", hint: "Registries that need credentials: add DOCKER_AUTH_CONFIG as a secret variable." },
                    h(Input, { value: image, onInput: (v) => { setImage(v); autoName(v); }, placeholder: "ghcr.io/you/app:latest", mono: true, autofocus: true })),
                h("div", { class: "row row-wrap", style: { gap: 6 } }, ["nginx:alpine", "traefik/whoami", "redis:7-alpine", "ghcr.io/umami-software/umami:postgresql-latest"].map((x) => (h("button", { key: x, type: "button", class: "tag mono", style: { cursor: "pointer" }, onClick: () => { setImage(x); autoName(x); if (x.includes("nginx") || x.includes("whoami"))
                        setPort("80"); if (x.includes("redis")) {
                        setPort("6379");
                        setProtocol("tcp");
                    } if (x.includes("umami"))
                        setPort("3000"); } }, x)))))),
            h(Card, { class: "form-card" },
                h("div", { class: "form-grid" },
                    h(Field, { label: "App name", hint: "Lowercase; also the hostname inside the project." },
                        h(Input, { value: name, onInput: (v) => { setName(v.toLowerCase().replace(/[^a-z0-9-]/g, "-")); setNameTouched(true); }, mono: true, placeholder: "api" })),
                    h(Field, { label: "Replicas" },
                        h(Input, { value: replicas, type: "number", min: 0, max: 20, onInput: setReplicas })),
                    h(Field, { label: "Port", hint: "0 for a worker with no port." },
                        h(Input, { value: port, type: "number", onInput: setPort, mono: true })),
                    h(Field, { label: "Protocol" },
                        h(Select, { value: protocol, onChange: setProtocol, options: [{ value: "http", label: "HTTP (gets an HTTPS URL)" }, { value: "tcp", label: "TCP (internal)" }, { value: "udp", label: "UDP (internal)" }] })),
                    h("div", { class: "span-2" },
                        h(Field, { label: "Size", hint: "Requests equal limits: what you pick is what you get and what you are capped at." },
                            h(SizePicker, { value: size, onChange: setSize }))))),
            h(Card, { class: "form-card" },
                h("div", null,
                    h("div", { class: "panel-title" }, "Variables"),
                    h("div", { class: "panel-desc" }, inspect?.env?.length ? "Declared in the Dockerfile; fill in the ones you need." : "App-scoped values. Reference databases with ${{ dbname.DATABASE_URL }}.")),
                h(EnvRows, { rows: env, setRows: setEnv })),
            h("div", { class: "row" },
                h(Toggle, { checked: deploy, onChange: setDeploy, label: "Deploy right away" }),
                h("span", { class: "grow" }),
                h(Button, { kind: "primary", icon: "rocket", onClick: submit, disabled: !canSubmit }, deploy ? "Create and deploy" : "Create app")))));
}
export function NewDatabasePage({ org }) {
    const [projectId, setProjectId] = useState(query().get("project") || "");
    const [envId, setEnvId] = useState("");
    const [name, setName] = useState("postgres");
    const [version, setVersion] = useState("17");
    const [storage, setStorage] = useState("10");
    const [size, setSize] = useState("m");
    const sz = sizes.find((x) => x.id === size);
    const submit = async () => {
        const db = await act(() => post(`/projects/${projectId}/databases`, { name, envId, version: Number(version), storageGi: Number(storage), resources: { cpuMillis: sz.cpu, memoryMi: sz.mem } }), "Provisioning PostgreSQL");
        if (db)
            navigate(`/o/${org}/databases/${db.id}`);
    };
    return (h("div", { class: "page-mid", style: { maxWidth: 760 } },
        h(Link, { href: `/o/${org}`, class: "back-link" },
            h(Icon, { name: "arrow-left", size: 14 }),
            " Back"),
        h("div", { class: "page-head" },
            h("div", null,
                h("h1", { class: "page-title", style: { fontSize: 26 } }, "New database"),
                h("div", { class: "page-sub" }, "Managed PostgreSQL on CloudNativePG, with backups, extensions, read replicas and a query console."))),
        h("div", { class: "col gap-lg" },
            h(Card, { class: "form-card" },
                h(ProjectEnvPicker, { org: org, projectId: projectId, setProjectId: setProjectId, envId: envId, setEnvId: setEnvId })),
            h(Card, { class: "form-card" },
                h("div", { class: "form-grid" },
                    h(Field, { label: "Name", hint: "Referenced from apps as ${{ name.DATABASE_URL }}." },
                        h(Input, { value: name, onInput: (v) => setName(v.toLowerCase().replace(/[^a-z0-9-]/g, "-")), mono: true, autofocus: true })),
                    h(Field, { label: "Version" },
                        h(Select, { value: version, onChange: setVersion, options: ["18", "17", "16", "15", "14"].map((v) => ({ value: v, label: "PostgreSQL " + v })) })),
                    h(Field, { label: "Storage (GiB)", hint: "Can grow later, never shrink." },
                        h(Input, { value: storage, type: "number", onInput: setStorage })),
                    h("div", null),
                    h("div", { class: "span-2" },
                        h(Field, { label: "Size" },
                            h(SizePicker, { value: size, onChange: setSize }))))),
            h("div", { class: "row" },
                h("span", { class: "grow" }),
                h(Button, { kind: "primary", icon: "database", onClick: submit, disabled: !name || !projectId }, "Create database")))));
}
