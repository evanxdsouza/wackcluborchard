import { h, Fragment, useEffect, useRef, useState } from "../lib/sprout.js";
import { Link, navigate, query, setQuery } from "../lib/router.js";
import { useApi, useEvents, post, act } from "../lib/api.js";
import { timeAgo, plural } from "../lib/format.js";
import { Button, Menu, MenuItem, MenuSep, Loading, ErrorBox, Empty, Dot, cx, openModal, ModalHeader, Field, Input, Segmented, statusKind } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";
import { Sky, ImageBadge, drawSky, palettes } from "../ui/art.js";
import { useOrg } from "../ui/layout.js";
export function appImageLabel(a) {
    if (a.source?.type === "github")
        return a.source.repo;
    return a.source?.image || a.image || "";
}
export function appSubtitle(a) {
    const gen = (a.domains || []).find((d) => d.generated);
    const custom = (a.domains || []).find((d) => !d.generated);
    return (custom || gen)?.host || appImageLabel(a);
}
function statusText(s) {
    return s[0].toUpperCase() + s.slice(1);
}
export function NewMenu({ org, projectId, label = "New", kind = "primary" }) {
    const q = projectId ? "?project=" + projectId : "";
    return (h(Menu, { align: "right", trigger: (open, toggle) => kind === "glass" ? (h("button", { type: "button", class: "glass-btn", onClick: (e) => { e.stopPropagation(); toggle(); } },
            h(Icon, { name: "plus", size: 13 }),
            " ",
            label)) : (h(Button, { kind: "primary", icon: "plus", iconRight: "chevron-down", onClick: toggle }, label)) }, (close) => (h("div", { onClick: (e) => e.stopPropagation() },
        !projectId ? h(MenuItem, { icon: "folder", onClick: () => { close(); newProjectModal(org); } }, "Project") : null,
        !projectId ? h(MenuSep, null) : null,
        h(MenuItem, { icon: "github", href: `/o/${org}/new/app${q}${q ? "&" : "?"}source=github`, onClick: close }, "GitHub repository"),
        h(MenuItem, { icon: "box", href: `/o/${org}/new/app${q}${q ? "&" : "?"}source=image`, onClick: close }, "Container image"),
        h(MenuItem, { icon: "database", href: `/o/${org}/new/database${q}`, onClick: close }, "PostgreSQL database"),
        h(MenuItem, { icon: "zap", href: `/o/${org}/new/job${q}`, onClick: close }, "Job"),
        h(MenuSep, null),
        h(MenuItem, { icon: "trees", href: `/o/${org}/grove${q}`, onClick: close }, "Template from The Grove"),
        projectId ? h(MenuItem, { icon: "layers", href: `/o/${org}/projects/${projectId}?tab=compose`, onClick: close }, "Docker Compose") : null))));
}
const emojis = ["🌱", "🍎", "🍒", "🍋", "🌳", "🌸", "🍄", "🐝", "🦔", "🚀", "🛰️", "🧪", "🎮", "📚", "🔧", "🏠"];
export function newProjectModal(org, onCreated) {
    openModal((close) => h(NewProject, { org: org, close: close, onCreated: onCreated }));
}
export function BackgroundPicker({ value, onChange, seed }) {
    return (h("div", { class: "row row-wrap" }, Object.keys(palettes).map((p) => (h("button", { key: p, type: "button", class: cx("bg-swatch", value === p && "active"), title: p, onClick: () => onChange(p) },
        h("canvas", { ref: (el) => el && !el.dataset.drawn && (drawSky(el, seed, p), (el.dataset.drawn = "1")) }))))));
}
function NewProject({ org, close, onCreated }) {
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
    return (h(Fragment, null,
        h(ModalHeader, { title: "New project", subtitle: "A project groups apps, databases and jobs, and gets its own Kubernetes namespace.", onClose: close, icon: "folder" }),
        h("div", { class: "modal-body" },
            h(Field, { label: "Name" },
                h(Input, { value: name, onInput: setName, placeholder: "Homelab", autofocus: true, onEnter: submit })),
            h(Field, { label: "Icon" },
                h("div", { class: "emoji-row" }, emojis.map((e) => h("button", { key: e, type: "button", class: cx("emoji-btn", e === icon && "active"), onClick: () => setIcon(e) }, e)))),
            h(Field, { label: "Sky" },
                h(BackgroundPicker, { value: bg, onChange: setBg, seed: name || "new" }))),
        h("div", { class: "modal-foot" },
            h(Button, { kind: "ghost", onClick: close }, "Cancel"),
            h(Button, { kind: "primary", onClick: submit, disabled: !name.trim() }, "Create project"))));
}
function AppTile({ a, org }) {
    const m = a.metrics;
    return (h(Link, { href: `/o/${org}/apps/${a.id}`, class: "app-tile" },
        h("div", { class: "app-tile-top" },
            h(ImageBadge, { image: appImageLabel(a), name: a.name }),
            h("div", { class: "grow" },
                h("div", { class: "app-tile-name truncate" }, a.name),
                h("div", { class: "app-tile-sub truncate" }, appSubtitle(a)))),
        h("div", { class: "app-tile-foot" },
            h(Dot, { status: a.status }),
            h("span", { class: "status-txt" }, statusText(a.status)),
            h("span", { class: "right" },
                m ? h("span", { class: "nowrap" },
                    Math.round(m.cpuMillis),
                    "m \u00B7 ",
                    Math.round(m.memoryMi),
                    " MiB") : null,
                h("span", { class: "nowrap" }, timeAgo(a.deployedAt || a.createdAt))))));
}
function DbTile({ d, org }) {
    return (h(Link, { href: `/o/${org}/databases/${d.id}`, class: "app-tile" },
        h("div", { class: "app-tile-top" },
            h(ImageBadge, { image: "postgres", name: d.name }),
            h("div", { class: "grow" },
                h("div", { class: "app-tile-name truncate" }, d.name),
                h("div", { class: "app-tile-sub truncate" }, d.dbName))),
        h("div", { class: "app-tile-foot" },
            h(Dot, { status: d.status }),
            h("span", { class: "status-txt" }, statusText(d.status)),
            h("span", { class: "right" },
                h("span", null, timeAgo(d.createdAt))))));
}
function ListRow({ href, image, name, sub, status, right }) {
    return (h(Link, { href: href, class: "list-row" },
        h(ImageBadge, { image: image, name: name, size: 30 }),
        h("div", { class: "grow" },
            h("div", { style: { fontWeight: 600 } }, name),
            h("div", { class: "muted truncate", style: { fontSize: 13 } }, sub)),
        h(Dot, { status: status }),
        h("span", { style: { width: 90, fontSize: 13 } }, statusText(status)),
        h("span", { class: "muted nowrap", style: { width: 80, textAlign: "right", fontSize: 13 } }, right)));
}
function ProjectGroup({ p, org, filter, view }) {
    const key = "orchard-collapsed-" + p.id;
    const [collapsed, setCollapsed] = useState(() => {
        try {
            return localStorage.getItem(key) === "1";
        }
        catch {
            return false;
        }
    });
    const toggle = () => {
        const n = !collapsed;
        setCollapsed(n);
        try {
            localStorage.setItem(key, n ? "1" : "0");
        }
        catch { }
    };
    const f = filter.toLowerCase();
    const apps = p.apps.filter((a) => !f || a.name.includes(f) || appImageLabel(a).toLowerCase().includes(f));
    const dbs = p.databases.filter((d) => !f || d.name.includes(f) || d.dbName.includes(f));
    if (f && !apps.length && !dbs.length && !p.name.toLowerCase().includes(f))
        return null;
    const healthy = p.apps.filter((a) => a.status === "running").length + p.databases.filter((d) => d.status === "ready").length;
    const bad = p.apps.filter((a) => statusKind(a.status) === "red").length;
    const last = [...p.apps.map((a) => a.deployedAt || a.createdAt), p.updatedAt].filter(Boolean).sort().pop();
    return (h("div", { class: cx("project-group", collapsed && "collapsed-group") },
        h("div", { class: "project-bar", onClick: () => navigate(`/o/${org}/projects/${p.id}`) },
            h(Sky, { seed: p.id, preset: p.background }),
            h("button", { type: "button", class: "collapse-btn", title: collapsed ? "Expand" : "Collapse", onClick: (e) => { e.stopPropagation(); toggle(); } },
                h(Icon, { name: "chevron-down", size: 16 })),
            h("div", { class: "project-icon" }, p.icon),
            h("div", { class: "project-meta" },
                h("div", { class: "project-name" }, p.name),
                h("div", { class: "project-sub" },
                    h("span", null, p.owner.username),
                    h("span", null, plural(p.apps.length, "deployment")),
                    h("span", null, plural(p.databases.length, "database")),
                    h("span", null, timeAgo(last)))),
            h("span", { class: "glass-count", title: `${healthy} healthy${bad ? `, ${bad} failing` : ""}` },
                h("span", { class: cx("status-dot", bad ? "dot-red" : "dot-green") }),
                bad || healthy),
            p.canEdit ? h(NewMenu, { org: org, projectId: p.id, kind: "glass" }) : null,
            h(Link, { href: `/o/${org}/projects/${p.id}`, class: "glass-btn glass-round", title: "Open project", onClick: (e) => e.stopPropagation() },
                h(Icon, { name: "arrow-right", size: 14 }))),
        collapsed ? null : apps.length + dbs.length === 0 ? (h("div", { style: { padding: 16 } },
            h(Empty, { icon: "box", title: "Nothing planted yet", action: p.canEdit ? h(NewMenu, { org: org, projectId: p.id, label: "Add something" }) : null }, "Deploy a GitHub repo or an image, or add a database."))) : view === "list" ? (h("div", { class: "project-apps list" },
            apps.map((a) => h(ListRow, { key: a.id, href: `/o/${org}/apps/${a.id}`, image: appImageLabel(a), name: a.name, sub: appSubtitle(a), status: a.status, right: timeAgo(a.deployedAt || a.createdAt) })),
            dbs.map((d) => h(ListRow, { key: d.id, href: `/o/${org}/databases/${d.id}`, image: "postgres", name: d.name, sub: d.dbName, status: d.status, right: timeAgo(d.createdAt) })))) : (h("div", { class: "project-apps" },
            apps.map((a) => h(AppTile, { key: a.id, a: a, org: org })),
            dbs.map((d) => h(DbTile, { key: d.id, d: d, org: org }))))));
}
function RecentDeploys({ org, tick }) {
    const r = useApi(`/orgs/${org}/deploys/recent`, [tick]);
    return (h("div", { class: "card card-pad recent" },
        h("div", { class: "panel-title", style: { marginBottom: 8 } }, "Recent deployments"),
        !r.data ? h("div", { class: "muted", style: { padding: "18px 0", textAlign: "center", fontSize: 13.5 } }, "Loading\u2026") : r.data.length === 0 ? (h("div", { class: "muted", style: { padding: "22px 0", textAlign: "center", fontSize: 13.5 } }, "No recent deployments")) : (r.data.map((d) => (h(Link, { key: d.id, href: `/o/${org}/deploys/${d.id}`, class: "recent-item" },
            h(Dot, { status: d.status === "running" ? "deploying" : d.status }),
            h("div", { class: "grow" },
                h("div", null,
                    h("strong", { style: { fontWeight: 600 } }, d.appName),
                    " ",
                    h("span", { class: "muted" },
                        "#",
                        d.number)),
                h("div", { class: "muted truncate", style: { fontSize: 12.5 } }, d.message || (d.kind === "rollback" ? "Rollback" : d.image?.split("/").pop() || d.kind))),
            h("span", { class: "muted nowrap", style: { fontSize: 12.5 } }, timeAgo(d.createdAt))))))));
}
export function AppsPage({ org }) {
    const o = useOrg(org);
    const [tick, setTick] = useState(0);
    const r = useApi(`/orgs/${org}/overview`, [tick]);
    const [filter, setFilter] = useState("");
    const [view, setView] = useState(() => {
        try {
            return localStorage.getItem("orchard-view") || "grid";
        }
        catch {
            return "grid";
        }
    });
    const pending = useRef(null);
    useEvents(r.data ? r.data.map((p) => "project:" + p.id) : [], () => {
        if (pending.current)
            return;
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
    return (h("div", { class: "page" },
        h("div", { class: "page-head" },
            h("h1", { class: "page-title" }, "Your Apps"),
            h("div", { class: "page-actions" }, canCreate ? h(NewMenu, { org: org }) : null)),
        h("div", { class: "apps-layout" },
            h("div", null,
                h("div", { class: "apps-toolbar" },
                    h("div", { class: "search-input" },
                        h(Icon, { name: "search", size: 15 }),
                        h("input", { class: "input", placeholder: "Search apps...", value: filter, onInput: (e) => setFilter(e.target.value) })),
                    h("span", { class: "grow" }),
                    h(Segmented, { size: "sm", value: view, onChange: (v) => {
                            setView(v);
                            try {
                                localStorage.setItem("orchard-view", v);
                            }
                            catch { }
                        }, options: [{ id: "list", label: "", icon: "list", title: "List" }, { id: "grid", label: "", icon: "grid", title: "Cards" }] })),
                r.error ? h(ErrorBox, { error: r.error, onRetry: r.reload }) : !r.data ? h(Loading, null) : r.data.length === 0 ? (h(Empty, { icon: "sprout", title: "Plant your first project", action: canCreate ? h(Button, { kind: "primary", icon: "plus", onClick: () => newProjectModal(org) }, "New project") : null }, "Projects hold apps, databases and jobs that belong together. Start one, then deploy a GitHub repo or a container image into it.")) : (r.data.map((p) => h(ProjectGroup, { key: p.id, p: p, org: org, filter: filter, view: view })))),
            h(RecentDeploys, { org: org, tick: tick }))));
}
