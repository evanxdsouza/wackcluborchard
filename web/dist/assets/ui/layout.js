import { h, Fragment, useEffect, useRef, useState } from "../lib/sprout.js";
import { Link, navigate, useLocation } from "../lib/router.js";
import { session, use, sidebarCollapsed, palette, theme, rememberOrg } from "../lib/state.js";
import { get, post } from "../lib/api.js";
import { Icon } from "./icons.js";
import { Avatar, Logo } from "./art.js";
import { cx, Kbd, Menu, MenuItem, MenuSep, openModal, Dot } from "./kit.js";
export function useOrg(slug) {
    const s = use(session);
    const org = s.me?.orgs.find((o) => o.slug === slug || o.id === slug);
    useEffect(() => {
        if (org)
            rememberOrg(org.slug);
    }, [org?.slug]);
    return org;
}
const nav = [
    { id: "apps", label: "Your Apps", icon: "leaf", path: "" },
    { id: "jobs", label: "Jobs", icon: "zap", path: "/jobs" },
    { id: "usage", label: "Usage", icon: "activity", path: "/usage" },
    { id: "greenhouse", label: "Greenhouse", icon: "sprout", path: "/greenhouse" },
    { id: "grove", label: "The Grove", icon: "trees", path: "/grove" },
];
function activeSection(path) {
    const m = path.match(/^\/o\/[^/]+(\/[^/?]+)?/);
    const seg = m?.[1] || "";
    if (seg === "/jobs" || seg === "/runs")
        return "jobs";
    if (seg === "/usage")
        return "usage";
    if (seg === "/greenhouse")
        return "greenhouse";
    if (seg === "/grove")
        return "grove";
    if (seg === "/settings")
        return "settings";
    if (seg === "/members")
        return "members";
    if (path.startsWith("/account"))
        return "account";
    if (path.startsWith("/admin"))
        return "admin";
    return "apps";
}
export function Shell({ org, children }) {
    const s = use(session);
    const collapsed = use(sidebarCollapsed);
    const path = useLocation();
    const [mobileOpen, setMobileOpen] = useState(false);
    const me = s.me;
    const current = me.orgs.find((o) => o.slug === org) || me.orgs[0];
    const section = activeSection(path);
    useEffect(() => setMobileOpen(false), [path]);
    useEffect(() => {
        const onKey = (e) => {
            if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
                e.preventDefault();
                palette.set(true);
            }
        };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    }, []);
    const base = "/o/" + (current?.slug || org);
    const isAdmin = current && (current.role === "owner" || current.role === "admin");
    return (h("div", { class: "shell" },
        h("aside", { class: cx("sidebar", collapsed && "collapsed", mobileOpen && "mobile-open") },
            h(Menu, { class: "org-menu", trigger: (open, toggle) => (h("button", { type: "button", class: "org-switch", onClick: toggle, title: "Switch organization" },
                    h(Logo, { size: 36 }),
                    h("span", { class: "org-name hide-collapsed" }, current?.name.replace(/'s Orchard$/, "") === me.name ? "Orchard" : current?.name || "Orchard"),
                    h(Icon, { name: "chevrons-up-down", size: 15, class: "muted hide-collapsed" }))) }, (close) => (h(Fragment, null,
                h("div", { class: "menu-heading" }, "Organizations"),
                me.orgs.map((o) => (h(MenuItem, { key: o.id, icon: o.slug === current?.slug ? "check" : "building", href: "/o/" + o.slug, onClick: close, hint: o.viaSuperadmin ? "admin" : o.role }, o.name))),
                h(MenuSep, null),
                h(MenuItem, { icon: "plus", onClick: () => { close(); newOrgModal(); } }, "New organization"),
                isAdmin ? h(MenuItem, { icon: "settings", href: base + "/settings", onClick: close }, "Organization settings") : null))),
            h("button", { type: "button", class: "search-btn", onClick: () => palette.set(true), title: "Search (\u2318K)" },
                h(Icon, { name: "search", size: 16 }),
                h("span", { class: "hide-collapsed" }, "Search"),
                h("span", { class: "hide-collapsed", style: { marginLeft: "auto" } },
                    h(Kbd, null, "\u2318K"))),
            h("div", { class: "nav-label hide-collapsed" }, "Platform"),
            h("nav", { class: "nav" }, nav.map((n) => (h(Link, { key: n.id, href: base + n.path, class: cx("nav-item", section === n.id && "active"), title: n.label },
                h(Icon, { name: n.icon, size: 18 }),
                h("span", { class: "hide-collapsed" }, n.label))))),
            h("div", { class: "nav-label hide-collapsed" }, "Organization"),
            h("nav", { class: "nav" },
                h(Link, { href: base + "/members", class: cx("nav-item", section === "members" && "active"), title: "Members" },
                    h(Icon, { name: "users", size: 18 }),
                    h("span", { class: "hide-collapsed" }, "Members")),
                h(Link, { href: base + "/settings", class: cx("nav-item", section === "settings" && "active"), title: "Settings" },
                    h(Icon, { name: "settings", size: 18 }),
                    h("span", { class: "hide-collapsed" }, "Settings")),
                me.superadmin ? (h(Link, { href: "/admin", class: cx("nav-item", section === "admin" && "active"), title: "Instance admin" },
                    h(Icon, { name: "shield", size: 18 }),
                    h("span", { class: "hide-collapsed" }, "Instance admin"))) : null),
            h("div", { class: "sidebar-foot" },
                h(Menu, { align: "left", class: "grow", trigger: (open, toggle) => (h("button", { type: "button", class: "user-chip", onClick: toggle, title: me.name },
                        h(Avatar, { seed: me.avatarSeed, size: 30 }),
                        h("span", { class: "user-name hide-collapsed" }, me.username))) }, (close) => h(UserMenu, { close: close })),
                h("button", { type: "button", class: "icon-btn", title: collapsed ? "Expand sidebar" : "Collapse sidebar", onClick: () => sidebarCollapsed.set(!collapsed) },
                    h(Icon, { name: "panel-left", size: 16 })))),
        h("main", { class: "main" },
            h("div", { class: "mobile-bar" },
                h("button", { type: "button", class: "icon-btn boxed", onClick: () => setMobileOpen(true), title: "Menu" },
                    h(Icon, { name: "list" })),
                h(Logo, { size: 28 }),
                h("strong", null, "Wack Club Orchard")),
            s.auth?.demo ? (h("div", { class: "demo-banner" },
                h(Icon, { name: "sparkles", size: 14 }),
                h("span", null,
                    "Running on the ",
                    h("strong", null, "simulated runtime"),
                    ": nothing is scheduled on a real cluster. Point the server at a Kubernetes API to go live."))) : null,
            s.auth?.needsCredential ? (h("div", { class: "demo-banner", style: { background: "var(--red-soft)" } },
                h(Icon, { name: "key", size: 14 }),
                h("span", null,
                    "Add a passkey or password now: the claim link was single use. ",
                    h(Link, { href: "/account" }, "Secure your account \u2192")))) : null,
            children),
        mobileOpen ? h("div", { class: "modal-backdrop", style: { zIndex: 15 }, onClick: () => setMobileOpen(false) }) : null,
        h(Palette, { org: current?.slug || org })));
}
function UserMenu({ close }) {
    const t = use(theme);
    const s = use(session);
    const setTheme = (x) => theme.set(x);
    return (h(Fragment, null,
        h("div", { class: "menu-heading" }, s.me?.name),
        h(MenuItem, { icon: "user", href: "/account", onClick: close }, "Account"),
        s.me?.superadmin ? h(MenuItem, { icon: "shield", href: "/admin", onClick: close }, "Instance admin") : null,
        h(MenuSep, null),
        h("div", { class: "menu-heading" }, "Theme"),
        h("div", { style: { padding: "2px 8px 6px" } },
            h("div", { class: "segmented seg-sm", style: { width: "100%" } }, ["light", "dark", "system"].map((x) => (h("button", { key: x, type: "button", class: cx("seg", t === x && "active"), style: { flex: 1, justifyContent: "center" }, onClick: () => setTheme(x) },
                h(Icon, { name: x === "light" ? "sun" : x === "dark" ? "moon" : "monitor", size: 13 })))))),
        h(MenuSep, null),
        h(MenuItem, { icon: "logout", onClick: async () => { close(); await post("/auth/logout"); location.href = "/login"; } }, "Sign out")));
}
import { Button, Field, Input, ModalHeader } from "./kit.js";
import { act } from "../lib/api.js";
import { loadSession } from "../lib/state.js";
export function newOrgModal() {
    openModal((close) => h(NewOrg, { close: close }));
}
function NewOrg({ close }) {
    const [name, setName] = useState("");
    const submit = async () => {
        const o = await act(() => post("/orgs", { name }));
        if (o) {
            await loadSession();
            close();
            navigate("/o/" + o.slug);
        }
    };
    return (h(Fragment, null,
        h(ModalHeader, { title: "New organization", subtitle: "Organizations hold members, quotas and projects. Each project gets its own namespace.", onClose: close, icon: "building" }),
        h("div", { class: "modal-body" },
            h(Field, { label: "Name" },
                h(Input, { value: name, onInput: setName, autofocus: true, placeholder: "Wack Club", onEnter: submit }))),
        h("div", { class: "modal-foot" },
            h(Button, { kind: "ghost", onClick: close }, "Cancel"),
            h(Button, { kind: "primary", onClick: submit, disabled: !name.trim() }, "Create"))));
}
function Palette({ org }) {
    const open = use(palette);
    if (!open)
        return null;
    return (h("div", { class: "modal-backdrop", onMouseDown: (e) => { if (e.target === e.currentTarget)
            palette.set(false); } },
        h("div", { class: "modal palette" },
            h(PaletteBody, { org: org }))));
}
function PaletteBody({ org }) {
    const [q, setQ] = useState("");
    const [hits, setHits] = useState([]);
    const [sel, setSel] = useState(0);
    const listRef = useRef(null);
    const base = "/o/" + org;
    const actions = [
        { kind: "go", id: "a1", title: "Your Apps", subtitle: "", href: base },
        { kind: "go", id: "a2", title: "Jobs", subtitle: "", href: base + "/jobs" },
        { kind: "go", id: "a3", title: "Usage", subtitle: "", href: base + "/usage" },
        { kind: "go", id: "a4", title: "Greenhouse", subtitle: "Sandboxes", href: base + "/greenhouse" },
        { kind: "go", id: "a5", title: "The Grove", subtitle: "Templates", href: base + "/grove" },
        { kind: "new", id: "n1", title: "New project", subtitle: "", href: base + "?new=project" },
        { kind: "new", id: "n2", title: "New app from an image", subtitle: "", href: base + "/new/app?source=image" },
        { kind: "new", id: "n3", title: "New app from GitHub", subtitle: "", href: base + "/new/app?source=github" },
        { kind: "new", id: "n4", title: "New database", subtitle: "", href: base + "/new/database" },
        { kind: "go", id: "a6", title: "Account", subtitle: "Passkeys, tokens, SSH keys", href: "/account" },
    ];
    useEffect(() => {
        let alive = true;
        const t = setTimeout(async () => {
            try {
                const r = await get("/search?q=" + encodeURIComponent(q));
                if (alive) {
                    setHits(r);
                    setSel(0);
                }
            }
            catch { }
        }, 90);
        return () => {
            alive = false;
            clearTimeout(t);
        };
    }, [q]);
    const lower = q.toLowerCase();
    const all = [...hits, ...actions.filter((a) => !q || a.title.toLowerCase().includes(lower))];
    const go = (hh) => {
        palette.set(false);
        navigate(hh.href);
    };
    return (h(Fragment, null,
        h("div", { class: "palette-input" },
            h(Icon, { name: "search", size: 18, class: "muted" }),
            h("input", { autofocus: true, placeholder: "Search apps, databases, jobs, projects\u2026", value: q, onInput: (e) => setQ(e.target.value), onKeyDown: (e) => {
                    if (e.key === "Escape")
                        palette.set(false);
                    else if (e.key === "ArrowDown") {
                        e.preventDefault();
                        setSel(Math.min(all.length - 1, sel + 1));
                    }
                    else if (e.key === "ArrowUp") {
                        e.preventDefault();
                        setSel(Math.max(0, sel - 1));
                    }
                    else if (e.key === "Enter" && all[sel])
                        go(all[sel]);
                } }),
            h(Kbd, null, "esc")),
        h("div", { class: "palette-list", ref: listRef },
            all.length === 0 ? h("div", { class: "muted", style: { padding: 16, textAlign: "center" } },
                "Nothing matches \u201C",
                q,
                "\u201D.") : null,
            all.map((hh, i) => (h("a", { key: hh.kind + hh.id, href: hh.href, class: cx("palette-item", i === sel && "sel"), onMouseEnter: () => setSel(i), onClick: (e) => { e.preventDefault(); go(hh); } },
                h("span", { class: "palette-kind" }, hh.kind),
                h(Icon, { name: { app: "box", database: "database", job: "zap", project: "folder", new: "plus", go: "arrow-right" }[hh.kind] || "box", size: 16, class: "muted" }),
                h("span", { class: "grow truncate" },
                    h("strong", { style: { fontWeight: 500 } }, hh.title),
                    " ",
                    h("span", { class: "muted" }, hh.subtitle)),
                hh.status ? h(Dot, { status: hh.status }) : null)))),
        h("div", { class: "palette-foot" },
            h("span", null,
                h(Kbd, null, "\u2191\u2193"),
                " move"),
            h("span", null,
                h(Kbd, null, "\u21B5"),
                " open"),
            h("span", null,
                h(Kbd, null, "\u2318K"),
                " toggle"))));
}
