import { h, Fragment, useEffect, useState } from "../lib/sprout.js";
import { Link, navigate, query, setQuery } from "../lib/router.js";
import { useApi, useEvents, post, patch, del, act, get } from "../lib/api.js";
import { timeAgo, duration, cpu, mem, plural, shortSha, dateTime, appURL } from "../lib/format.js";
import { Button, Loading, ErrorBox, Empty, Pill, Tag, Tabs, Card, Chart, Slider, Field, Input, Select, Toggle, confirm, Callout, IconButton, openModal, ModalHeader, Table, CopyButton, Menu, MenuItem, MenuSep, cx, Dot, DefList } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";
import { ImageBadge } from "../ui/art.js";
import { LogView, Terminal, OutputView } from "../ui/term.js";
import { appImageLabel } from "./apps.js";
export function AppPage({ org, id }) {
    const r = useApi(`/apps/${id}`);
    const [tab, setTabState] = useState(query().get("tab") || "overview");
    const [metrics, setMetrics] = useState([]);
    const [deployTick, setDeployTick] = useState(0);
    useEffect(() => {
        get(`/apps/${id}/metrics`).then(setMetrics).catch(() => { });
    }, [id]);
    useEvents([`app:${id}`], (e) => {
        if (e.type === "app.updated")
            r.set((d) => ({ ...d, ...e.data }));
        else if (e.type === "metrics")
            setMetrics((m) => [...m.slice(-359), e.data]);
        else if (e.type === "deploy.updated") {
            setDeployTick((t) => t + 1);
            r.set((d) => (d.latestDeploy && d.latestDeploy.id === e.data.id) || e.data.id === d.currentDeploy ? { ...d, latestDeploy: e.data } : d);
        }
        else if (e.type === "event" || e.type === "crash")
            r.reload();
    });
    const setTab = (t) => {
        setTabState(t);
        setQuery("tab", t === "overview" ? null : t);
    };
    if (r.error)
        return h("div", { class: "page" },
            h(ErrorBox, { error: r.error, onRetry: r.reload }));
    if (!r.data)
        return h("div", { class: "page" },
            h(Loading, null));
    const a = r.data;
    const url = a.domains?.length ? appURL((a.domains.find((d) => !d.generated) || a.domains[0]).host) : "";
    const deploy = async () => {
        const res = await act(() => post(`/apps/${id}/deploy`));
        if (res) {
            if (res.build && !res.build.started)
                (await import("../lib/state.js")).toast(res.build.message, "info");
            navigate(`/o/${org}/deploys/${res.deploy.id}`);
        }
    };
    const building = a.status === "building" || a.status === "deploying";
    return (h("div", { class: "page" },
        h(Link, { href: `/o/${org}/projects/${a.project.id}`, class: "back-link" },
            h(Icon, { name: "arrow-left", size: 14 }),
            " ",
            a.project.icon,
            " ",
            a.project.name,
            a.envName ? " · " + a.envName : ""),
        h(Card, { pad: false },
            h("div", { class: "detail-head" },
                h(ImageBadge, { image: appImageLabel(a), name: a.name, size: 48 }),
                h("div", { class: "grow" },
                    h("div", { class: "detail-title" },
                        a.name,
                        " ",
                        h(Pill, { status: a.status })),
                    h("div", { class: "detail-tags" },
                        h(Tag, { mono: true, title: "Source" }, a.source.type === "github" ? h(Fragment, null,
                            h(Icon, { name: "github", size: 12 }),
                            " ",
                            a.source.repo,
                            "@",
                            a.source.branch) : a.source.image),
                        h(Tag, { mono: true }, plural(a.replicas, "replica")),
                        h(Tag, { mono: true },
                            cpu(a.resources.cpuMillis),
                            " \u00B7 ",
                            mem(a.resources.memoryMi)),
                        a.authWall ? h(Tag, null,
                            h(Icon, { name: "lock", size: 12 }),
                            " auth wall") : null,
                        a.sandboxed ? h(Tag, null,
                            h(Icon, { name: "shield", size: 12 }),
                            " gVisor") : null,
                        url ? h("a", { href: url, target: "_blank", rel: "noopener", class: "tag mono", style: { color: "var(--accent-text)" } },
                            h(Icon, { name: "external", size: 12 }),
                            " ",
                            url.replace("https://", "")) : null)),
                a.canEdit ? (h("div", { class: "detail-actions" },
                    h(Button, { kind: "primary", icon: "rocket", onClick: deploy, loading: building && a.latestDeploy?.status === "running" }, a.image ? "Redeploy" : "Deploy"),
                    h(Button, { kind: "secondary", icon: "restart", disabled: !a.image, onClick: async () => { await act(() => post(`/apps/${id}/restart`), "Restarting pods on the same image"); } }, "Restart"),
                    h(Button, { kind: "secondary", icon: "terminal", disabled: !a.pods?.some((p) => p.phase === "Running"), onClick: () => openShell(a) }, "Shell"),
                    h(Menu, { align: "right", trigger: (o, toggle) => h(IconButton, { icon: "more", title: "More", class: "boxed", onClick: toggle }) }, (close) => (h(Fragment, null,
                        a.replicas > 0 ? (h(MenuItem, { icon: "square", onClick: async () => { close(); await act(() => patch(`/apps/${id}`, { replicas: 0 }), "Scaled to zero"); } }, "Stop (scale to 0)")) : (h(MenuItem, { icon: "play", onClick: async () => { close(); await act(() => patch(`/apps/${id}`, { replicas: 1 }), "Starting"); } }, "Start")),
                        h(MenuItem, { icon: "copy", onClick: () => { close(); navigator.clipboard?.writeText(a.internalHost); }, hint: "internal DNS" }, "Copy hostname"),
                        h(MenuSep, null),
                        h(MenuItem, { icon: "trash", danger: true, onClick: async () => {
                                close();
                                if (await confirm({ title: `Delete ${a.name}?`, body: "Deletes the deployment, its service, routes and volumes. Images are kept in the registry.", danger: true, confirm: "Delete app", typeToConfirm: a.name })) {
                                    if (await act(() => del(`/apps/${id}`), "App deleted"))
                                        navigate(`/o/${org}/projects/${a.project.id}`);
                                }
                            } }, "Delete app")))))) : null)),
        h("div", { style: { margin: "16px 0" } },
            h(Tabs, { value: tab, onChange: setTab, tabs: [
                    { id: "overview", label: "Overview", icon: "activity" },
                    { id: "deploys", label: "Deploys", icon: "history" },
                    { id: "variables", label: "Variables", icon: "key" },
                    { id: "domains", label: "Domains", icon: "globe", count: a.domains?.length || 0 },
                    { id: "observe", label: "Observe", icon: "alert" },
                    { id: "settings", label: "Settings", icon: "settings" },
                ] })),
        tab === "overview" ? h(Overview, { a: a, metrics: metrics, org: org }) : null,
        tab === "deploys" ? h(Deploys, { a: a, org: org, tick: deployTick }) : null,
        tab === "variables" ? h(AppVariables, { a: a, org: org }) : null,
        tab === "domains" ? h(Domains, { a: a, reload: r.reload }) : null,
        tab === "observe" ? h(Observe, { a: a }) : null,
        tab === "settings" ? h(Settings, { a: a, reload: r.reload }) : null));
}
function openShell(a) {
    openModal((close) => h(ShellModal, { a: a, close: close }), { wide: true });
}
function ShellModal({ a, close }) {
    const running = a.pods.filter((p) => p.phase === "Running");
    const [pod, setPod] = useState(running[0]?.name || "");
    return (h(Fragment, null,
        h(ModalHeader, { title: `Shell · ${a.name}`, subtitle: "A real process in a real pod: changes are lost on the next restart, and a command that eats the container's memory will get it killed.", onClose: close, icon: "terminal" }),
        h("div", { class: "modal-body", style: { paddingBottom: 18 } },
            running.length > 1 ? h(Select, { value: pod, onChange: setPod, options: running.map((p) => ({ value: p.name, label: p.name })) }) : null,
            h(Terminal, { path: `/apps/${a.id}/shell?pod=${encodeURIComponent(pod)}`, title: pod || a.name, height: 420, prompt: "#" }))));
}
function Overview({ a, metrics, org }) {
    const [replicas, setReplicas] = useState(a.replicas);
    useEffect(() => setReplicas(a.replicas), [a.replicas]);
    const cpuSeries = metrics.map((m) => ({ t: new Date(m.at).getTime(), v: m.cpuMillis }));
    const memSeries = metrics.map((m) => ({ t: new Date(m.at).getTime(), v: m.memoryMi }));
    const pods = (a.pods || []).filter((p) => p.phase !== "Terminating");
    const failing = a.status === "failed" || a.status === "degraded";
    const dep = a.latestDeploy;
    return (h(Fragment, null,
        a.statusMessage && failing ? (h("div", { class: cx("alert-banner", a.status === "failed" && "red") },
            h(Icon, { name: "alert", size: 16 }),
            h("div", null,
                h("strong", null, a.status === "failed" ? "This app is not running." : "Some replicas are unhealthy."),
                " ",
                a.statusMessage,
                a.alerts?.length ? h("div", { class: "muted", style: { marginTop: 4 } }, a.alerts.map((e) => e.plain).join(" · ")) : null))) : null,
        dep && (dep.status === "running" || dep.status === "queued") ? (h(Callout, { kind: "info", icon: "rocket", title: `Deploy #${dep.number} in progress` },
            dep.steps.find((s) => s.status === "running")?.name || "Queued",
            "\u2026 ",
            h(Link, { href: `/o/${org}/deploys/${dep.id}` }, "Watch it"))) : dep && dep.status === "failed" ? (h("div", { class: "alert-banner red" },
            h(Icon, { name: "alert", size: 16 }),
            h("div", null,
                h("strong", null,
                    "Deploy #",
                    dep.number,
                    " failed."),
                " ",
                dep.error,
                " ",
                a.image ? "The previous version is still running." : "",
                " ",
                h(Link, { href: `/o/${org}/deploys/${dep.id}` }, "Read the build log")))) : null,
        h("div", { class: "overview-grid", style: { marginTop: dep && (dep.status !== "succeeded") ? 14 : 0 } },
            a.image ? h(LogView, { appId: a.id, pods: pods }) : (h(Card, null,
                h(Empty, { icon: "rocket", title: "Not deployed yet", action: a.canEdit ? h(Button, { kind: "primary", icon: "rocket", onClick: async () => { const r = await act(() => post(`/apps/${a.id}/deploy`)); if (r)
                            navigate(`/o/${org}/deploys/${r.deploy.id}`); } }, "Deploy now") : null }, a.source.type === "github" ? "Wack Club Orchard will build the repository on the cluster and roll it out." : "Wack Club Orchard will pull the image and roll it out."))),
            h("div", { class: "side-stack" },
                h(Card, { class: "metric-card" },
                    h("div", { class: "metric-head" },
                        h(Icon, { name: "cpu", size: 15 }),
                        " CPU ",
                        h("span", { class: "muted" },
                            "limit ",
                            cpu(a.resources.cpuMillis * a.replicas))),
                    h(Chart, { series: cpuSeries, height: 96, format: (v) => cpu(v), max: 0 })),
                h(Card, { class: "metric-card" },
                    h("div", { class: "metric-head" },
                        h(Icon, { name: "memory", size: 15 }),
                        " Memory ",
                        h("span", { class: "muted" },
                            "limit ",
                            mem(a.resources.memoryMi * a.replicas))),
                    h(Chart, { series: memSeries, height: 96, format: (v) => mem(v), color: "var(--blue)" })),
                h(Card, { class: "metric-card" },
                    h("div", { class: "metric-head" },
                        h(Icon, { name: "layers", size: 15 }),
                        " Replicas ",
                        h("span", { class: "muted" },
                            pods.filter((p) => p.ready).length,
                            "/",
                            a.replicas,
                            " ready")),
                    a.canEdit ? (h("div", { class: "row", style: { gap: 12 } },
                        h(Slider, { value: replicas, min: 0, max: 10, onChange: setReplicas, onCommit: async (v) => { if (v !== a.replicas)
                                await act(() => patch(`/apps/${a.id}`, { replicas: v }), `Scaling to ${plural(v, "replica")}`); } }),
                        h("strong", { style: { width: 22, textAlign: "right" } }, replicas))) : null,
                    h("div", { class: "pod-list" }, pods.length === 0 ? h("div", { class: "muted", style: { fontSize: 13 } }, "No pods.") : pods.map((p) => (h("div", { class: "pod-row", key: p.name },
                        h(Dot, { status: p.ready ? "running" : p.reason === "CrashLoopBackOff" ? "failed" : "deploying" }),
                        h("span", { class: "truncate" }, p.name),
                        h("span", { class: "right" },
                            p.restarts ? `${p.restarts} restarts · ` : "",
                            p.reason && !p.ready ? p.reason : timeAgo(p.startedAt))))))),
                h(Card, { class: "metric-card" },
                    h("div", { class: "metric-head" },
                        h(Icon, { name: "network", size: 15 }),
                        " Networking"),
                    h(DefList, { rows: [
                            ["Internal", h(Fragment, null,
                                    h("span", { class: "truncate" }, a.name),
                                    h(CopyButton, { text: a.internalHost, size: "sm", label: "" }))],
                            ...(a.ports || []).map((p) => [p.name || "port", `${p.port}/${p.protocol}${p.public && p.nodePort ? ` → ${a.publicIp || "public"}:${p.nodePort}` : ""}`]),
                            ...(a.domains || []).slice(0, 2).map((d) => ["URL", h("a", { href: appURL(d.host), target: "_blank", rel: "noopener" }, d.host)]),
                        ] }))))));
}
function Deploys({ a, org, tick }) {
    const r = useApi(`/apps/${a.id}/deploys`, [tick]);
    if (!r.data)
        return h(Loading, null);
    const list = r.data.deploys;
    if (!list.length)
        return h(Empty, { icon: "history", title: "No deploys yet" }, "Every build and rollout lands here, with its image, so rolling back is one click.");
    return (h(Fragment, null,
        h(Callout, { kind: "info" }, "A rollback is an ordinary deploy of an older image: same rolling update, same health checks. It does not revert your repository or undo database migrations."),
        h("div", { style: { height: 12 } }),
        h(Table, { head: ["", "Deploy", "Source", "Trigger", "When", "Took", ""] }, list.map((d) => {
            const current = d.image && d.image === r.data.currentImage && d.status === "succeeded";
            return (h("tr", { key: d.id, class: "clickable", onClick: () => navigate(`/o/${org}/deploys/${d.id}`) },
                h("td", { style: { width: 30 } },
                    h(Dot, { status: d.status === "running" ? "deploying" : d.status })),
                h("td", null,
                    h("div", { style: { fontWeight: 600 } },
                        "#",
                        d.number,
                        " ",
                        current ? h("span", { class: "pill pill-green", style: { marginLeft: 6 } }, "current") : null),
                    h("div", { class: "muted", style: { fontSize: 12.5 } }, d.kind === "rollback" ? d.message : d.message || d.kind)),
                h("td", { class: "mono", style: { fontSize: 12.5, maxWidth: 320 } },
                    d.commit ? h("span", { class: "row", style: { gap: 5 } },
                        h(Icon, { name: "commit", size: 13 }),
                        " ",
                        shortSha(d.commit)) : null,
                    h("span", { class: "muted truncate", style: { display: "block", maxWidth: 320 } }, d.image || "—")),
                h("td", null,
                    h("span", { class: "tag" }, d.trigger),
                    " ",
                    h("span", { class: "muted", style: { fontSize: 12.5 } }, d.createdBy)),
                h("td", { class: "nowrap", title: dateTime(d.createdAt) }, timeAgo(d.createdAt)),
                h("td", { class: "mono nowrap", style: { fontSize: 12.5 } }, duration(d.createdAt, d.finishedAt)),
                h("td", { style: { textAlign: "right" } }, a.canEdit && d.status === "succeeded" && d.image && !current ? (h(Button, { size: "sm", kind: "secondary", icon: "history", onClick: async (e) => {
                        e.stopPropagation();
                        if (await confirm({ title: `Roll back to #${d.number}?`, body: h(Fragment, null,
                                "Deploys ",
                                h("code", null, d.image),
                                " through the normal rollout. Database schema changes made since are not undone."), confirm: "Roll back" })) {
                            const res = await act(() => post(`/apps/${a.id}/rollback/${d.id}`));
                            if (res)
                                navigate(`/o/${org}/deploys/${res.deploy.id}`);
                        }
                    } }, "Roll back")) : null)));
        }))));
}
function AppVariables({ a, org }) {
    const r = useApi(`/projects/${a.project.id}/variables`);
    if (!r.data)
        return h(Loading, null);
    const own = r.data.filter((v) => v.appId === a.id);
    const shared = r.data.filter((v) => !v.appId);
    const row = (v) => (h("tr", { key: v.id },
        h("td", { class: "mono", style: { fontWeight: 500 } }, v.key),
        h("td", { class: "mono muted truncate", style: { maxWidth: 380 } }, v.secret ? "••••••••" : v.value),
        h("td", null, v.secret ? h(Tag, null,
            h(Icon, { name: "lock", size: 11 }),
            " secret") : null)));
    return (h("div", { class: "col gap-lg" },
        h("div", { class: "row" },
            h("div", { class: "grow muted" }, "Variables resolve in layers: shared project values, then environment-scoped, then this app's own. Changing them rolls the app."),
            h(Button, { kind: "primary", icon: "pencil", href: `/o/${org}/projects/${a.project.id}?tab=variables` }, "Edit variables")),
        h("div", null,
            h("div", { class: "section-title", style: { marginBottom: 10 } }, "This app"),
            own.length ? h(Table, { head: ["Key", "Value", ""] }, own.map(row)) : h("div", { class: "muted" }, "None.")),
        h("div", null,
            h("div", { class: "section-title", style: { marginBottom: 10 } },
                "Shared by ",
                a.project.name),
            shared.length ? h(Table, { head: ["Key", "Value", ""] }, shared.map(row)) : h("div", { class: "muted" }, "None.")),
        h(Callout, { kind: "info" },
            "Wack Club Orchard also sets ",
            h("code", null, "PORT"),
            ", ",
            h("code", null, "WACKCLUBORCHARD_APP"),
            " and ",
            h("code", null, "WACKCLUBORCHARD_URL"),
            ".")));
}
function Domains({ a, reload }) {
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
    const httpPort = a.ports.find((p) => p.protocol === "http");
    return (h("div", { class: "col gap-lg", style: { maxWidth: 820 } },
        !httpPort ? h(Callout, { kind: "amber" }, "This app exposes no HTTP port, so it has no URL. Add one under Settings \u2192 Ports.") : null,
        h(Card, { pad: false }, a.domains.length === 0 ? h("div", { class: "muted", style: { padding: 18 } }, "No domains.") : a.domains.map((d) => (h("div", { class: "domain-row", key: d.host, style: { padding: "12px 16px" } },
            h(Icon, { name: d.generated ? "globe" : "link", class: "muted" }),
            h("a", { href: appURL(d.host), target: "_blank", rel: "noopener", class: "grow mono", style: { fontSize: 13.5 } }, d.host),
            d.generated ? h(Tag, null, "generated") : null,
            h(Pill, { status: d.certState === "issued" ? "active" : d.certState === "failed" ? "failed" : "provisioning", label: d.certState === "issued" ? "HTTPS" : d.certState === "failed" ? "Certificate failed" : "Issuing certificate" }),
            a.canEdit && !d.generated ? h(IconButton, { icon: "trash", title: "Remove domain", onClick: async () => {
                    if (await confirm({ title: `Remove ${d.host}?`, danger: true, confirm: "Remove" })) {
                        await act(() => del(`/apps/${a.id}/domains/${encodeURIComponent(d.host)}`), "Removed");
                        reload();
                    }
                } }) : null)))),
        a.canEdit && httpPort ? (h(Card, { class: "form-card" },
            h("div", null,
                h("div", { class: "panel-title" }, "Add a custom domain"),
                h("div", { class: "panel-desc" }, "Point a CNAME at the instance, add the domain here, and cert-manager issues a certificate automatically.")),
            h("div", { class: "row" },
                h(Input, { value: host, onInput: (v) => setHost(v.trim().toLowerCase()), placeholder: "www.example.com", mono: true, onEnter: add }),
                h(Button, { kind: "primary", icon: "plus", onClick: add, disabled: !host }, "Add")),
            cname ? h(Callout, { kind: "info", title: "Now add this DNS record" },
                h("code", null,
                    "CNAME ",
                    a.domains[a.domains.length - 1]?.host,
                    " \u2192 ",
                    cname),
                ". The certificate is issued over HTTP-01, so it arrives once the record resolves here.") : null)) : null));
}
function Observe({ a }) {
    const ev = useApi(`/apps/${a.id}/events`);
    const cr = useApi(`/apps/${a.id}/crashes`);
    const [open, setOpen] = useState(null);
    return (h("div", { class: "col gap-lg" },
        h("div", null,
            h("div", { class: "section-title" }, "Crash reports"),
            h("p", { class: "section-desc", style: { marginBottom: 12 } }, "When a container dies, its final output is captured here, including OOM kills. This is what turns \u201CCrashLoopBackOff\u201D into the actual error."),
            !cr.data ? h(Loading, null) : cr.data.length === 0 ? h(Empty, { icon: "check", title: "Nothing has crashed" }, "Containers that exit or get killed show up here with their last logs.") : (h("div", { class: "col" }, cr.data.map((c) => (h(Card, { key: c.id, pad: false },
                h("div", { class: "row", style: { padding: "12px 16px", cursor: "pointer" }, onClick: () => setOpen(open === c.id ? null : c.id) },
                    h(Icon, { name: "skull", class: "muted" }),
                    h("div", { class: "grow" },
                        h("div", null,
                            h("strong", null, c.reason || "Exited"),
                            " ",
                            h("span", { class: "muted" },
                                "exit ",
                                c.exitCode,
                                c.exitCode === 137 ? " · out of memory" : "")),
                        h("div", { class: "muted mono", style: { fontSize: 12 } }, c.pod)),
                    h("span", { class: "muted", style: { fontSize: 13 } }, timeAgo(c.createdAt)),
                    h(Icon, { name: open === c.id ? "chevron-up" : "chevron-down", class: "muted" })),
                open === c.id ? h("div", { style: { padding: "0 12px 12px" } },
                    h(OutputView, { text: c.logs, title: "last output", height: 320 })) : null)))))),
        h("div", null,
            h("div", { class: "section-title" }, "Kubernetes events"),
            h("p", { class: "section-desc", style: { marginBottom: 12 } }, "Warnings the cluster recorded for this app's pods, newest first, translated into plain English. Kubernetes keeps these for about an hour after the problem clears."),
            !ev.data ? h(Loading, null) : ev.data.length === 0 ? h(Empty, { icon: "check", title: "No warnings" }, "Scheduling problems, image pulls, probe failures and volume mounts show up here.") : (h(Table, { head: ["", "What happened", "Object", "Count", "Last seen"] }, ev.data.map((e) => (h("tr", { key: e.id },
                h("td", { style: { width: 30 } },
                    h(Icon, { name: "alert", size: 15, class: "muted" })),
                h("td", null,
                    h("div", { style: { fontWeight: 500 } }, e.plain),
                    h("div", { class: "muted", style: { fontSize: 12.5 } },
                        h("code", null, e.reason),
                        " ",
                        e.message)),
                h("td", { class: "mono muted", style: { fontSize: 12 } }, e.object),
                h("td", null,
                    e.count,
                    "\u00D7"),
                h("td", { class: "nowrap" }, timeAgo(e.lastSeen))))))))));
}
const cpuPresets = [50, 100, 250, 500, 1000, 2000, 4000];
const memPresets = [64, 128, 256, 512, 1024, 2048, 4096, 8192];
function Settings({ a, reload }) {
    const [src, setSrc] = useState({ ...a.source });
    const [res, setRes] = useState({ ...a.resources });
    const [ports, setPorts] = useState(a.ports.map((p) => ({ ...p })));
    const [vols, setVols] = useState((a.volumes || []).map((v) => ({ ...v })));
    const [health, setHealth] = useState({ enabled: false, path: "/", port: 0, initialDelaySeconds: 5, ...a.health });
    const [command, setCommand] = useState(a.command || "");
    const dis = !a.canEdit;
    const save = async (body, msg = "Saved; rolling out") => {
        if (await act(() => patch(`/apps/${a.id}`, body), msg))
            reload();
    };
    return (h("div", { class: "col gap-lg", style: { maxWidth: 860 } },
        h(Card, { class: "form-card" },
            h("div", { class: "panel-title" }, "Source"),
            src.type === "github" ? (h("div", { class: "form-grid" },
                h(Field, { label: "Repository" },
                    h(Input, { value: src.repo, onInput: (v) => setSrc({ ...src, repo: v }), mono: true, disabled: dis })),
                h(Field, { label: "Branch" },
                    h(Input, { value: src.branch, onInput: (v) => setSrc({ ...src, branch: v }), mono: true, disabled: dis })),
                h(Field, { label: "Dockerfile" },
                    h(Input, { value: src.dockerfile || "", onInput: (v) => setSrc({ ...src, dockerfile: v }), placeholder: "Dockerfile", mono: true, disabled: dis })),
                h(Field, { label: "Build stage", hint: "A multi-stage build aimed at an early stage produces an image with no app in it." },
                    h(Input, { value: src.target || "", onInput: (v) => setSrc({ ...src, target: v }), placeholder: "final stage", mono: true, disabled: dis })),
                h(Field, { label: "Build context" },
                    h(Input, { value: src.context || "", onInput: (v) => setSrc({ ...src, context: v }), placeholder: ".", mono: true, disabled: dis })),
                h("div", { class: "span-2" },
                    h(Toggle, { checked: src.autoDeploy, onChange: (v) => setSrc({ ...src, autoDeploy: v }), label: "Deploy on push", hint: `Every push to ${src.branch} builds and deploys itself.`, disabled: dis })))) : (h(Field, { label: "Image", hint: "Saving does not deploy; press Deploy to roll out a new image." },
                h(Input, { value: src.image, onInput: (v) => setSrc({ ...src, image: v }), mono: true, disabled: dis }))),
            !dis ? h("div", { class: "form-card-foot" },
                h(Button, { kind: "primary", onClick: () => save({ source: src }, "Source saved") }, "Save source")) : null),
        h(Card, { class: "form-card" },
            h("div", null,
                h("div", { class: "panel-title" }, "Resources"),
                h("div", { class: "panel-desc" }, "Requests equal limits: what you pick is what the app gets and what it is capped at. Size honestly; quotas count requests.")),
            h("div", { class: "form-grid" },
                h(Field, { label: "CPU" },
                    h(Select, { value: String(res.cpuMillis), onChange: (v) => setRes({ ...res, cpuMillis: Number(v) }), disabled: dis, options: [...new Set([...cpuPresets, res.cpuMillis])].sort((x, y) => x - y).map((c) => ({ value: String(c), label: cpu(c) })) })),
                h(Field, { label: "Memory" },
                    h(Select, { value: String(res.memoryMi), onChange: (v) => setRes({ ...res, memoryMi: Number(v) }), disabled: dis, options: [...new Set([...memPresets, res.memoryMi])].sort((x, y) => x - y).map((m) => ({ value: String(m), label: mem(m) })) }))),
            !dis ? h("div", { class: "form-card-foot" },
                h(Button, { kind: "primary", onClick: () => save({ resources: res }) }, "Save resources")) : null),
        h(Card, { class: "form-card" },
            h("div", null,
                h("div", { class: "panel-title" }, "Ports"),
                h("div", { class: "panel-desc" }, "The first HTTP port gets the app's URL. TCP and UDP ports are reachable inside the project; mark them public to publish them on the organization's IP.")),
            ports.map((p, i) => (h("div", { class: "row", key: i },
                h(Input, { value: p.name, onInput: (v) => setPorts(ports.map((x, j) => (j === i ? { ...x, name: v } : x))), placeholder: "name", disabled: dis }),
                h(Input, { value: p.port, type: "number", onInput: (v) => setPorts(ports.map((x, j) => (j === i ? { ...x, port: Number(v) } : x))), placeholder: "8080", mono: true, disabled: dis }),
                h(Select, { value: p.protocol, onChange: (v) => setPorts(ports.map((x, j) => (j === i ? { ...x, protocol: v } : x))), disabled: dis, options: [{ value: "http", label: "HTTP" }, { value: "tcp", label: "TCP" }, { value: "udp", label: "UDP" }] }),
                p.protocol !== "http" ? h("label", { class: "row nowrap", style: { fontSize: 13 } },
                    h("input", { type: "checkbox", checked: p.public, disabled: dis, onChange: (e) => setPorts(ports.map((x, j) => (j === i ? { ...x, public: e.target.checked } : x))) }),
                    " public") : null,
                !dis ? h(IconButton, { icon: "trash", title: "Remove port", onClick: () => setPorts(ports.filter((_, j) => j !== i)) }) : null))),
            !dis ? (h("div", { class: "form-card-foot" },
                h(Button, { kind: "ghost", icon: "plus", onClick: () => setPorts([...ports, { name: "", port: 8080, protocol: ports.some((p) => p.protocol === "http") ? "tcp" : "http", public: false }]) }, "Add port"),
                h("span", { class: "grow" }),
                h(Button, { kind: "primary", onClick: () => save({ ports }) }, "Save ports"))) : null),
        h(Card, { class: "form-card" },
            h("div", null,
                h("div", { class: "panel-title" }, "Volumes"),
                h("div", { class: "panel-desc" }, "Persistent volumes survive restarts and deploys. They are single-writer: an app with volumes rolls out by recreating its pod.")),
            vols.map((v, i) => (h("div", { class: "row", key: i },
                h(Input, { value: v.name, onInput: (x) => setVols(vols.map((y, j) => (j === i ? { ...y, name: x } : y))), placeholder: "data", disabled: dis }),
                h(Input, { value: v.mountPath, onInput: (x) => setVols(vols.map((y, j) => (j === i ? { ...y, mountPath: x } : y))), placeholder: "/data", mono: true, disabled: dis }),
                h(Input, { value: v.sizeGi, type: "number", onInput: (x) => setVols(vols.map((y, j) => (j === i ? { ...y, sizeGi: Number(x) } : y))), placeholder: "1", disabled: dis }),
                h("span", { class: "muted" }, "GiB"),
                !dis ? h(IconButton, { icon: "trash", title: "Remove volume", onClick: () => setVols(vols.filter((_, j) => j !== i)) }) : null))),
            !dis ? (h("div", { class: "form-card-foot" },
                h(Button, { kind: "ghost", icon: "plus", onClick: () => setVols([...vols, { name: "data", mountPath: "/data", sizeGi: 1 }]) }, "Add volume"),
                h("span", { class: "grow" }),
                h(Button, { kind: "primary", onClick: () => save({ volumes: vols }) }, "Save volumes"))) : null),
        h(Card, { class: "form-card" },
            h("div", { class: "panel-title" }, "Health and startup"),
            h(Toggle, { checked: health.enabled, onChange: (v) => setHealth({ ...health, enabled: v }), label: "HTTP health check", hint: "Pods only get traffic once this passes, and are restarted if it keeps failing. Re-applied on every rebuild.", disabled: dis }),
            health.enabled ? (h("div", { class: "form-grid" },
                h(Field, { label: "Path" },
                    h(Input, { value: health.path, onInput: (v) => setHealth({ ...health, path: v }), mono: true, disabled: dis })),
                h(Field, { label: "Initial delay (s)" },
                    h(Input, { value: health.initialDelaySeconds, type: "number", onInput: (v) => setHealth({ ...health, initialDelaySeconds: Number(v) }), disabled: dis })))) : null,
            h(Field, { label: "Start command", hint: "Overrides the image's entrypoint, run with sh -c. Leave empty to use the image's own." },
                h(Input, { value: command, onInput: setCommand, mono: true, placeholder: "node dist/server.js", disabled: dis })),
            !dis ? h("div", { class: "form-card-foot" },
                h(Button, { kind: "primary", onClick: () => save({ health, command }) }, "Save")) : null),
        h(Card, { class: "form-card" },
            h("div", { class: "panel-title" }, "Access and isolation"),
            h(Toggle, { checked: a.authWall, onChange: (v) => save({ authWall: v }, v ? "Auth wall on" : "Auth wall off"), disabled: dis, label: "Auth wall", hint: "Only signed-in members of this project can reach the app's URLs. The app sees X-Wackclubwackcluborchard-User." }),
            h(Toggle, { checked: a.sandboxed, onChange: (v) => save({ sandboxed: v }), disabled: dis, label: "gVisor sandbox", hint: "Runs pods under a user-space kernel. Needs the gvisor RuntimeClass on the nodes; use it for code you do not trust." }))));
}
