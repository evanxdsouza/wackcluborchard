import { h, Fragment, useEffect, useRef, useState } from "../lib/sprout.js";
import { Link, navigate, query, setQuery } from "../lib/router.js";
import { useApi, useEvents, get, post, put, patch, del, act } from "../lib/api.js";
import { plural, timeAgo } from "../lib/format.js";
import { Button, Loading, ErrorBox, Empty, Dot, Pill, cx, SectionLabel, Tabs, Field, Input, Textarea, Select, Toggle, confirm, Callout, IconButton, Card, Menu, MenuItem, CodeBlock } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";
import { Sky, Avatar } from "../ui/art.js";
import { NewMenu, BackgroundPicker, appImageLabel } from "./apps.js";
export function ProjectPage({ org, id }) {
    const [tick, setTick] = useState(0);
    const r = useApi(`/projects/${id}`, [tick]);
    const [tab, setTabState] = useState(query().get("tab") || "apps");
    const [env, setEnv] = useState(query().get("env") || "");
    const pending = useRef(null);
    useEvents([`project:${id}`], () => {
        if (pending.current)
            return;
        pending.current = setTimeout(() => {
            pending.current = null;
            setTick((t) => t + 1);
        }, 300);
    });
    const setTab = (t) => {
        setTabState(t);
        setQuery("tab", t === "apps" ? null : t);
    };
    if (r.error)
        return h("div", { class: "page" },
            h(ErrorBox, { error: r.error, onRetry: r.reload }));
    if (!r.data)
        return h("div", { class: "page" },
            h(Loading, null));
    const p = r.data;
    const envId = env || p.environments[0]?.id;
    const inEnv = (x) => !x.envId || x.envId === envId;
    const apps = p.apps.filter(inEnv);
    const dbs = p.databases.filter(inEnv);
    const tpls = p.templateInstances.filter(inEnv);
    const jobs = p.jobs.filter(inEnv);
    return (h("div", { class: "page" },
        h(Link, { href: `/o/${org}`, class: "crumb-pill" },
            h(Icon, { name: "chevron-left", size: 15 }),
            " Your Apps"),
        h("div", { class: "hero-wrap" },
            h("div", { class: "hero" },
                h(Sky, { seed: p.id, preset: p.background }),
                h("div", { class: "hero-title" },
                    h("span", { class: "emoji" }, p.icon),
                    " ",
                    p.name),
                h("div", { class: "hero-actions" },
                    h(Menu, { align: "right", trigger: (open, toggle) => h("button", { type: "button", class: "hero-btn", onClick: toggle, title: "Project actions" },
                            h(Icon, { name: "more", size: 16 })) }, (close) => (h(Fragment, null,
                        h(MenuItem, { icon: "settings", onClick: () => { close(); setTab("settings"); } }, "Project settings"),
                        h(MenuItem, { icon: "users", onClick: () => { close(); setTab("members"); } }, "Members"),
                        h(MenuItem, { icon: "copy", onClick: () => { close(); navigator.clipboard?.writeText(p.namespace); }, hint: p.namespace }, "Copy namespace"))))))),
        h("div", { class: "project-bar-tabs" },
            h("label", { class: "env-select" },
                h("select", { value: envId, onChange: (e) => { setEnv(e.target.value); setQuery("env", e.target.value); } }, p.environments.map((e) => h("option", { key: e.id, value: e.id, selected: e.id === envId }, e.name))),
                h(Icon, { name: "chevron-down", size: 15, class: "muted" })),
            h(Tabs, { value: tab, onChange: setTab, tabs: [
                    { id: "apps", label: "Apps", icon: "server", count: apps.length + dbs.length + tpls.length },
                    { id: "variables", label: "Variables", icon: "key" },
                    { id: "compose", label: "Compose", icon: "file-text" },
                    { id: "members", label: "Members", icon: "users", count: p.people.length },
                    { id: "settings", label: "Settings", icon: "settings" },
                ] }),
            h("span", { class: "grow" }),
            p.canEdit ? h(NewMenu, { org: org, projectId: p.id }) : null),
        tab === "apps" ? h(AppsTab, { org: org, p: p, apps: apps, dbs: dbs, tpls: tpls, jobs: jobs }) : null,
        tab === "variables" ? h(VariablesTab, { p: p, envId: envId }) : null,
        tab === "compose" ? h(ComposeTab, { p: p, envId: envId, org: org }) : null,
        tab === "members" ? h(MembersTab, { p: p, org: org, reload: r.reload }) : null,
        tab === "settings" ? h(SettingsTab, { p: p, org: org, reload: r.reload }) : null));
}
function letter(n) {
    return (n[0] || "?").toUpperCase();
}
function AppsTab({ org, p, apps, dbs, tpls, jobs }) {
    if (!apps.length && !dbs.length && !tpls.length && !jobs.length) {
        return (h(Empty, { icon: "sprout", title: "An empty plot", action: p.canEdit ? h(NewMenu, { org: org, projectId: p.id, label: "Add to this project" }) : null }, "Connect a GitHub repo, run a container image, spin up PostgreSQL, or grab a template from The Grove."));
    }
    return (h(Fragment, null,
        apps.length ? (h("div", { class: "grid-3" }, apps.map((a) => (h(Link, { key: a.id, href: `/o/${org}/apps/${a.id}`, class: "card svc-card card-link" },
            h("div", { class: "svc-top" },
                h("div", { class: "svc-letter" },
                    letter(a.name),
                    h("span", { class: "status-dot dot-" + (a.status === "running" ? "green" : a.status === "failed" ? "red" : a.status === "degraded" ? "amber" : a.status === "stopped" || a.status === "pending" ? "gray" : "blue") })),
                h("div", { class: "svc-name" }, a.name),
                h(Pill, { status: a.status })),
            h("div", { class: "svc-image" }, appImageLabel(a)),
            h("div", { class: "svc-meta" },
                h("span", null, plural(a.replicas, "replica")),
                h("span", { class: "muted" },
                    "\u00B7 ",
                    a.creator.name))))))) : null,
        dbs.length ? (h(Fragment, null,
            h(SectionLabel, null, "Databases"),
            h("div", { class: "grid-3" }, dbs.map((d) => (h(Link, { key: d.id, href: `/o/${org}/databases/${d.id}`, class: "card db-card card-link" },
                h("div", { class: "row" },
                    h("div", { class: "db-icon" },
                        h(Icon, { name: "database", size: 20 })),
                    h(Pill, { status: d.status })),
                h("div", { class: "db-name" }, d.name),
                h("div", { class: "db-ident" }, d.dbName),
                h("div", { class: "db-foot" },
                    "PostgreSQL ",
                    d.version,
                    " \u00B7 ",
                    d.mode,
                    d.instances > 1 ? ` · ${d.instances - 1} replica${d.instances > 2 ? "s" : ""}` : ""))))))) : null,
        jobs.length ? (h(Fragment, null,
            h(SectionLabel, null, "Jobs"),
            h("div", { class: "grid-3" }, jobs.map((j) => (h(Link, { key: j.id, href: `/o/${org}/jobs/${j.id}`, class: "card db-card card-link" },
                h("div", { class: "row" },
                    h("div", { class: "tpl-icon", style: { background: "var(--amber-soft)", color: "var(--amber)" } },
                        h(Icon, { name: "zap", size: 20 })),
                    j.paused ? h(Pill, { status: "paused" }) : j.schedule ? h(Pill, { status: "scheduled", label: "Scheduled" }) : h(Pill, { status: "pending", label: "Manual" })),
                h("div", { class: "db-name" }, j.name),
                h("div", { class: "db-ident" }, j.schedule || "run on demand"),
                h("div", { class: "db-foot" },
                    plural(j.steps.length, "step"),
                    j.lastRunAt ? ` · ran ${timeAgo(j.lastRunAt)}` : ""))))))) : null,
        tpls.length ? (h(Fragment, null,
            h(SectionLabel, null, "Template instances"),
            h("div", { class: "grid-3" }, tpls.map((t) => (h("div", { key: t.id, class: "card db-card" },
                h("div", { class: "row" },
                    h("div", { class: "tpl-icon" },
                        h(Icon, { name: "template", size: 20 })),
                    h(Pill, { status: t.status }),
                    h("span", { class: "grow" }),
                    p.canEdit ? (h(IconButton, { icon: "trash", title: "Remove the template and everything it created", onClick: async () => {
                            if (await confirm({ title: `Remove ${t.name}?`, body: "This deletes the apps and databases this template created, including their data.", danger: true, confirm: "Remove", typeToConfirm: t.name })) {
                                await act(() => del(`/template-instances/${t.id}`), "Template removed");
                            }
                        } })) : null),
                h("div", { class: "db-name" }, t.name),
                h("div", { class: "db-ident" }, t.label))))))) : null));
}
function VariablesTab({ p, envId }) {
    const r = useApi(`/projects/${p.id}/variables`);
    const [rows, setRows] = useState([]);
    const [deleted, setDeleted] = useState([]);
    const [scope, setScope] = useState("all");
    const [bulk, setBulk] = useState(null);
    const [dirty, setDirty] = useState(false);
    useEffect(() => {
        if (r.data) {
            setRows(r.data.map((v) => ({ ...v })));
            setDeleted([]);
            setDirty(false);
        }
    }, [r.data]);
    const envName = (id) => p.environments.find((e) => e.id === id)?.name || "all environments";
    const shown = rows.filter((v) => scope === "all" || (scope === "shared" ? !v.appId : v.appId === scope));
    const update = (i, k, v) => {
        setRows(rows.map((row) => (row === shown[i] ? { ...row, [k]: v } : row)));
        setDirty(true);
    };
    const save = async () => {
        const res = await act(() => put(`/projects/${p.id}/variables`, { variables: rows.filter((v) => v.key.trim()), delete: deleted }));
        if (res) {
            await r.reload();
            if (res.redeployed)
                (await import("../lib/state.js")).toast(`Saved. ${plural(res.redeployed, "app")} rolling out with the new values.`, "ok");
        }
    };
    const refs = [...p.databases.map((d) => `\${{ ${d.name}.DATABASE_URL }}`), ...p.apps.slice(0, 3).map((a) => `\${{ ${a.name}.URL }}`)];
    if (!r.data)
        return h(Loading, null);
    return (h("div", { class: "col gap-lg" },
        h("div", { class: "row row-wrap" },
            h(Select, { value: scope, onChange: setScope, options: [{ value: "all", label: "All variables" }, { value: "shared", label: "Shared by the project" }, ...p.apps.map((a) => ({ value: a.id, label: "App: " + a.name }))], class: "role-select" }),
            h("span", { class: "grow" }),
            p.canEdit ? h(Button, { kind: "ghost", icon: "upload", onClick: () => setBulk(bulk == null ? "" : null) }, "Import .env") : null,
            p.canEdit ? h(Button, { kind: "secondary", icon: "plus", onClick: () => { setRows([...rows, { key: "", value: "", secret: false, envId: "", appId: scope !== "all" && scope !== "shared" ? scope : "" }]); setDirty(true); } }, "Add variable") : null,
            p.canEdit ? h(Button, { kind: "primary", icon: "save", disabled: !dirty, onClick: save }, "Save changes") : null),
        bulk != null ? (h(Card, null,
            h("div", { class: "form-stack" },
                h(Field, { label: "Paste a .env file", hint: "KEY=value lines. Existing keys in the same scope are overwritten." },
                    h(Textarea, { value: bulk, onInput: setBulk, rows: 6, mono: true, placeholder: "DATABASE_URL=postgres://…\nNODE_ENV=production" })),
                h("div", { class: "row" },
                    h("span", { class: "grow" }),
                    h(Button, { kind: "ghost", onClick: () => setBulk(null) }, "Cancel"),
                    h(Button, { kind: "primary", onClick: async () => {
                            const ok = await act(() => put(`/projects/${p.id}/variables`, { env: bulk, envId: "", appId: scope !== "all" && scope !== "shared" ? scope : "" }), "Imported");
                            if (ok) {
                                setBulk(null);
                                r.reload();
                            }
                        } }, "Import"))))) : null,
        shown.length === 0 ? (h(Empty, { icon: "key", title: "No variables here" }, "Shared variables reach every app in the project; app variables override them; environment-scoped ones apply only there.")) : (h(Card, null,
            h("div", { class: "col" }, shown.map((v, i) => (h("div", { class: "kv-row", key: v.id || "new" + i },
                h(Input, { value: v.key, onInput: (x) => update(i, "key", x.toUpperCase().replace(/[^A-Z0-9_.-]/g, "_")), placeholder: "KEY", mono: true, disabled: !p.canEdit }),
                h(Input, { value: v.value, onInput: (x) => update(i, "value", x), placeholder: v.secret && v.id ? "•••••••• (unchanged)" : "value", mono: true, type: v.secret ? "password" : "text", disabled: !p.canEdit }),
                h("div", { class: "row", style: { gap: 6 } },
                    h("select", { class: "input select", style: { width: 150, height: 34, fontSize: 12.5 }, disabled: !p.canEdit, onChange: (e) => update(i, "envId", e.target.value) },
                        h("option", { value: "", selected: !v.envId }, "All environments"),
                        p.environments.map((e) => h("option", { key: e.id, value: e.id, selected: v.envId === e.id }, e.name))),
                    h("select", { class: "input select", style: { width: 130, height: 34, fontSize: 12.5 }, disabled: !p.canEdit, onChange: (e) => update(i, "appId", e.target.value) },
                        h("option", { value: "", selected: !v.appId }, "Shared"),
                        p.apps.map((a) => h("option", { key: a.id, value: a.id, selected: v.appId === a.id }, a.name)))),
                h("div", { class: "row", style: { gap: 2 } },
                    h(IconButton, { icon: v.secret ? "lock" : "eye", title: v.secret ? "Secret: hidden from viewers and logs. Click to make plain" : "Plain. Click to make secret", onClick: () => p.canEdit && update(i, "secret", !v.secret) }),
                    p.canEdit ? h(IconButton, { icon: "trash", title: "Delete", onClick: () => { if (v.id)
                            setDeleted([...deleted, v.id]); setRows(rows.filter((x) => x !== v)); setDirty(true); } }) : null))))))),
        h(Callout, { kind: "info", title: "Reference other things in this project" },
            "Values can point at databases and apps, resolved when the app rolls out: ",
            refs.map((x, i) => h(Fragment, null,
                h("code", { key: i }, x),
                i < refs.length - 1 ? " " : "")),
            ". Variables prefixed ",
            h("code", null, "BUILD_"),
            " become Docker build arguments. Currently viewing ",
            h("strong", null, envName(envId)),
            ".")));
}
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
function ComposeTab({ p, envId, org }) {
    const r = useApi(`/projects/${p.id}/compose?env=${envId}`, [envId]);
    const [src, setSrc] = useState("");
    const [plan, setPlan] = useState(null);
    useEffect(() => {
        if (r.data)
            setSrc(r.data.source || "");
    }, [r.data]);
    if (!r.data)
        return h(Loading, null);
    const run = async (dryRun) => {
        const res = await act(() => put(`/projects/${p.id}/compose`, { envId, source: src, dryRun }));
        if (res) {
            setPlan({ ...res, dryRun });
            if (!dryRun)
                r.reload();
        }
    };
    return (h("div", { class: "overview-grid" },
        h(Card, { pad: false },
            h("div", { class: "ws-head" },
                h(Icon, { name: "file-text", size: 14 }),
                " docker-compose.yml ",
                h("span", { class: "grow" }),
                r.data.updatedAt ? h("span", { class: "muted", style: { fontWeight: 400 } },
                    "applied ",
                    timeAgo(r.data.updatedAt),
                    " by ",
                    r.data.updatedBy) : null),
            h("textarea", { class: "editor", style: { minHeight: 440 }, spellcheck: "false", value: src, placeholder: composeExample, onInput: (e) => setSrc(e.target.value), disabled: !p.canEdit })),
        h("div", { class: "side-stack" },
            h(Card, null,
                h("div", { class: "panel-title" }, "Deploy a Compose stack"),
                h("p", { class: "panel-desc" }, "Each service becomes an app in this environment. Named volumes become persistent volumes; the first published port gets an HTTPS URL. Re-applying reconciles: services removed from the file are removed."),
                p.canEdit ? (h("div", { class: "row", style: { marginTop: 14 } },
                    !src ? h(Button, { kind: "ghost", onClick: () => setSrc(composeExample) }, "Use example") : null,
                    h("span", { class: "grow" }),
                    h(Button, { kind: "secondary", onClick: () => run(true), disabled: !src.trim() }, "Preview"),
                    h(Button, { kind: "primary", icon: "rocket", onClick: () => run(false), disabled: !src.trim() }, "Apply"))) : null),
            plan ? (h(Card, null,
                h("div", { class: "panel-title" }, plan.dryRun ? "What would change" : "Applied"),
                h("div", { class: "col", style: { gap: 6, marginTop: 8, fontSize: 13.5 } },
                    plan.created?.map((n) => h("div", { key: "c" + n },
                        h("span", { class: "pill pill-green" }, "create"),
                        " ",
                        n)),
                    plan.updated?.map((n) => h("div", { key: "u" + n },
                        h("span", { class: "pill pill-blue" }, "update"),
                        " ",
                        n)),
                    plan.removed?.map((n) => h("div", { key: "r" + n },
                        h("span", { class: "pill pill-red" }, "remove"),
                        " ",
                        n)),
                    plan.warnings?.map((w, i) => h(Callout, { key: "w" + i, kind: "amber" }, w)),
                    !plan.created?.length && !plan.updated?.length && !plan.removed?.length ? h("div", { class: "muted" }, "No changes.") : null))) : null,
            r.data.appIds?.length ? (h(Card, null,
                h("div", { class: "panel-title" }, "Stack apps"),
                h("div", { class: "col", style: { gap: 6, marginTop: 8 } }, r.data.appIds.map((aid) => {
                    const a = p.apps.find((x) => x.id === aid);
                    return a ? h(Link, { key: aid, href: `/o/${org}/apps/${aid}`, class: "row" },
                        h(Dot, { status: a.status }),
                        " ",
                        a.name) : null;
                })))) : null)));
}
function MembersTab({ p, org, reload }) {
    const m = useApi(`/orgs/${org}/members`);
    const [pick, setPick] = useState("");
    const candidates = (m.data?.members || []).filter((x) => !p.people.find((y) => y.id === x.id));
    return (h("div", { class: "col gap-lg", style: { maxWidth: 760 } },
        h(Callout, { kind: "info" }, "Project membership is what gates access: members see and change the apps inside. Organization owners and admins can see every project."),
        p.canEdit && candidates.length ? (h(Card, null,
            h("div", { class: "row" },
                h(Select, { value: pick, onChange: setPick, class: "grow", options: [{ value: "", label: "Add a member of the organization…" }, ...candidates.map((c) => ({ value: c.id, label: `${c.name} (@${c.username}) · ${c.role}` }))] }),
                h(Button, { kind: "primary", disabled: !pick, onClick: async () => {
                        if (await act(() => post(`/projects/${p.id}/members`, { userId: pick }), "Added")) {
                            setPick("");
                            reload();
                        }
                    } }, "Add")))) : null,
        h(Card, { pad: false }, p.people.length === 0 ? h("div", { class: "muted", style: { padding: 18 } }, "No explicit members.") : p.people.map((u) => (h("div", { key: u.id, class: "row", style: { padding: "12px 16px", borderBottom: "1px solid var(--border)" } },
            h(Avatar, { seed: u.avatarSeed, size: 32 }),
            h("div", { class: "grow" },
                h("div", { style: { fontWeight: 600 } }, u.name),
                h("div", { class: "muted mono", style: { fontSize: 12.5 } },
                    "@",
                    u.username)),
            h("span", { class: "tag" }, u.role || "member"),
            p.canEdit ? h(IconButton, { icon: "x", title: "Remove from project", onClick: async () => {
                    if (await confirm({ title: `Remove ${u.name} from ${p.name}?`, confirm: "Remove", danger: true })) {
                        await act(() => del(`/projects/${p.id}/members/${u.id}`), "Removed");
                        reload();
                    }
                } }) : null))))));
}
const emojis = ["🌱", "🍎", "🍒", "🍋", "🌳", "🌸", "🍄", "🐝", "🦔", "🚀", "🛰️", "🧪", "🎮", "📚", "🔧", "🏠"];
function SettingsTab({ p, org, reload }) {
    const [name, setName] = useState(p.name);
    const [icon, setIcon] = useState(p.icon);
    const [bg, setBg] = useState(p.background);
    const [envName, setEnvName] = useState("");
    return (h("div", { class: "col gap-lg", style: { maxWidth: 760 } },
        h(Card, { class: "form-card" },
            h("div", { class: "panel-title" }, "General"),
            h(Field, { label: "Name" },
                h(Input, { value: name, onInput: setName, disabled: !p.canEdit })),
            h(Field, { label: "Icon" },
                h("div", { class: "emoji-row" }, emojis.map((e) => h("button", { key: e, type: "button", class: cx("emoji-btn", e === icon && "active"), onClick: () => setIcon(e) }, e)))),
            h(Field, { label: "Sky" },
                h(BackgroundPicker, { value: bg, onChange: setBg, seed: p.id })),
            h(Field, { label: "Kubernetes namespace", hint: "Every project gets its own namespace, network-isolated from the others." },
                h(Input, { value: p.namespace, onInput: () => { }, disabled: true, mono: true })),
            p.canEdit ? h("div", { class: "form-card-foot" },
                h(Button, { kind: "primary", onClick: async () => { if (await act(() => patch(`/projects/${p.id}`, { name, icon, background: bg }), "Saved"))
                        reload(); } }, "Save")) : null),
        h(Card, { class: "form-card" },
            h("div", null,
                h("div", { class: "panel-title" }, "Environments"),
                h("div", { class: "panel-desc" }, "Named slices of the project. Variables can be scoped to one, so staging and production differ without two projects.")),
            p.environments.map((e) => (h("div", { key: e.id, class: "row" },
                h(Icon, { name: "layers", class: "muted" }),
                h("span", { class: "grow" }, e.name),
                p.canEdit && p.environments.length > 1 ? h(IconButton, { icon: "trash", title: "Delete environment", onClick: async () => {
                        if (await confirm({ title: `Delete ${e.name}?`, body: "Its scoped variables are deleted too.", danger: true, confirm: "Delete" })) {
                            if (await act(() => del(`/projects/${p.id}/environments/${e.id}`), "Deleted"))
                                reload();
                        }
                    } }) : null))),
            p.canEdit ? (h("div", { class: "row" },
                h(Input, { value: envName, onInput: setEnvName, placeholder: "Staging", onEnter: async () => { if (envName && (await act(() => post(`/projects/${p.id}/environments`, { name: envName })))) {
                        setEnvName("");
                        reload();
                    } } }),
                h(Button, { kind: "secondary", icon: "plus", disabled: !envName.trim(), onClick: async () => { if (await act(() => post(`/projects/${p.id}/environments`, { name: envName }))) {
                        setEnvName("");
                        reload();
                    } } }, "Add"))) : null),
        p.canEdit ? (h(Card, { class: "form-card danger-zone" },
            h("div", null,
                h("div", { class: "panel-title" }, "Delete project"),
                h("div", { class: "panel-desc" }, "Deletes every app, database, job and volume in it, then the namespace. Database data is not recoverable.")),
            h("div", { class: "form-card-foot" },
                h(Button, { kind: "danger", icon: "trash", onClick: async () => {
                        if (await confirm({ title: `Delete ${p.name}?`, body: `This removes ${plural(p.apps.length, "app")} and ${plural(p.databases.length, "database")} permanently.`, danger: true, confirm: "Delete project", typeToConfirm: p.name })) {
                            if (await act(() => del(`/projects/${p.id}`), "Project deleted"))
                                navigate(`/o/${org}`);
                        }
                    } }, "Delete project")))) : null));
}
export { CodeBlock, Toggle, get };
