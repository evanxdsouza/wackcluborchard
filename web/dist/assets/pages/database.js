import { h, Fragment, useEffect, useState } from "../lib/sprout.js";
import { Link, navigate, query, setQuery } from "../lib/router.js";
import { useApi, useEvents, post, patch, del, act } from "../lib/api.js";
import { bytes, timeAgo, cpu, mem, dateTime } from "../lib/format.js";
import { Button, Loading, ErrorBox, Empty, Pill, Tag, Tabs, Card, Field, Input, Select, Toggle, confirm, Callout, CopyButton, Table, Segmented, Stat, DefList, IconButton } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";
import { Terminal, OutputView } from "../ui/term.js";
export function DatabasePage({ org, id }) {
    const r = useApi(`/databases/${id}`);
    const [tab, setTabState] = useState(query().get("tab") || "connection");
    useEvents([`database:${id}`], (e) => {
        if (e.type === "database.updated")
            r.set((d) => ({ ...d, ...e.data, password: e.data.password || d.password }));
    });
    const setTab = (t) => {
        setTabState(t);
        setQuery("tab", t === "connection" ? null : t);
    };
    if (r.error)
        return h("div", { class: "page-mid" },
            h(ErrorBox, { error: r.error, onRetry: r.reload }));
    if (!r.data)
        return h("div", { class: "page-mid" },
            h(Loading, null));
    const d = r.data;
    const stopped = d.status === "stopped";
    return (h("div", { class: "page-mid", style: { maxWidth: 760 } },
        h(Link, { href: `/o/${org}/projects/${d.project.id}`, class: "back-link" },
            h(Icon, { name: "arrow-left", size: 14 }),
            " ",
            d.project.icon,
            " ",
            d.project.name),
        h(Card, { pad: false },
            h("div", { class: "detail-head db-head" },
                h("div", { class: "detail-icon" },
                    h(Icon, { name: "database", size: 22 })),
                h("div", { class: "grow" },
                    h("div", { class: "detail-title" },
                        d.name,
                        " ",
                        h(Pill, { status: d.status })),
                    h("div", { class: "detail-tags" },
                        h(Tag, { mono: true },
                            "PG ",
                            d.version),
                        h(Tag, { mono: true },
                            d.storageGi,
                            "Gi"),
                        h(Tag, { mono: true }, d.namespace))),
                d.canEdit ? (h("div", { class: "detail-actions" },
                    h(Button, { size: "sm", kind: "secondary", icon: "restart", disabled: stopped, onClick: () => act(() => post(`/databases/${id}/restart`), "Restarting") }, "Restart"),
                    h(Button, { size: "sm", kind: "secondary", icon: stopped ? "play" : "square", onClick: () => act(() => patch(`/databases/${id}`, { stopped: !stopped }), stopped ? "Starting" : "Stopping (hibernating the cluster)") }, stopped ? "Start" : "Stop"),
                    h(Button, { size: "sm", kind: "danger", icon: "trash", onClick: async () => {
                            if (await confirm({ title: `Delete ${d.name}?`, body: "The CloudNativePG cluster and its volumes are deleted. Take a backup first if you need the data.", danger: true, confirm: "Delete database", typeToConfirm: d.name })) {
                                if (await act(() => del(`/databases/${id}`), "Database deleted"))
                                    navigate(`/o/${org}/projects/${d.project.id}`);
                            }
                        } }, "Delete"))) : null)),
        h("div", { class: "stats" },
            h(Stat, { label: "Size", value: bytes(d.sizeBytes) }),
            h(Stat, { label: "Connections", value: d.connections }),
            h(Stat, { label: "Port", value: d.port }),
            d.instances > 1 ? h(Stat, { label: "Replicas", value: d.instances - 1 }) : null),
        h("div", { style: { marginBottom: 16 } },
            h(Tabs, { value: tab, onChange: setTab, tabs: [
                    { id: "connection", label: "Connection" },
                    { id: "backups", label: "Backups" },
                    { id: "terminal", label: "Terminal" },
                    { id: "extensions", label: "Extensions" },
                    { id: "queries", label: "Queries" },
                    { id: "metrics", label: "Metrics" },
                    { id: "resources", label: "Resources" },
                    { id: "settings", label: "Settings" },
                ] })),
        tab === "connection" ? h(Connection, { d: d, org: org }) : null,
        tab === "backups" ? h(Backups, { d: d }) : null,
        tab === "terminal" ? h(DbTerminal, { d: d }) : null,
        tab === "extensions" ? h(Extensions, { d: d }) : null,
        tab === "queries" ? h(Queries, { d: d }) : null,
        tab === "metrics" ? h(DbMetrics, { d: d }) : null,
        tab === "resources" ? h(Resources, { d: d }) : null,
        tab === "settings" ? h(DbSettings, { d: d }) : null));
}
function Connection({ d, org }) {
    const [fmt, setFmt] = useState("uri");
    const [show, setShow] = useState(false);
    const [pwShown, setPwShown] = useState(false);
    const [enabling, setEnabling] = useState(false);
    if (!d.canEdit)
        return h(Callout, { kind: "info" }, "Viewers cannot see database credentials.");
    const pw = d.password;
    const mask = "••••••••";
    const uri = (reveal) => `postgresql://${d.user}:${reveal ? encodeURIComponent(pw) : mask}@${d.host}:${d.port}/${d.dbName}`;
    const text = (reveal) => {
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
    return (h("div", { class: "col gap-lg" },
        h(Card, { pad: false, class: "conn-box" },
            h("div", { class: "conn-tabs" },
                h(Segmented, { value: fmt, onChange: setFmt, options: [{ id: "uri", label: "URI" }, { id: "psql", label: "psql" }, { id: "env", label: ".env" }, { id: "jdbc", label: "JDBC" }] })),
            h("div", { class: "conn-uri" },
                h("pre", null, text(show)),
                h(IconButton, { icon: show ? "eye-off" : "eye", title: show ? "Hide password" : "Show password", onClick: () => setShow(!show) }),
                h(CopyButton, { text: text(true) })),
            h("div", { class: "conn-rows" },
                h(DefList, { rows: [
                        ["Host", d.host],
                        ["Port", String(d.port)],
                        ["Database", d.dbName],
                        ["User", d.user],
                        ["Password", h(Fragment, null,
                                h("span", null, pwShown ? pw : mask),
                                h(IconButton, { icon: pwShown ? "eye-off" : "eye", class: "tiny", title: "Reveal", onClick: () => setPwShown(!pwShown) }))],
                    ] }))),
        h(Callout, { kind: "info" },
            "From an app in this project, reference it instead of pasting: ",
            h("code", null, `\${{ ${d.name}.DATABASE_URL }}`),
            " in the project's ",
            h(Link, { href: `/o/${org}/projects/${d.project.id}?tab=variables` }, "variables"),
            "."),
        h(Card, null,
            h("div", { class: "row", style: { alignItems: "flex-start" } },
                h("div", { class: "grow" },
                    h("div", { class: "panel-title" }, "Public access"),
                    h("div", { class: "panel-desc" }, "Expose this database on a shared organization IP with a unique port.")),
                h(Button, { kind: d.publicPort ? "secondary" : "primary", size: "sm", loading: enabling, onClick: async () => {
                        if (d.publicPort && !(await confirm({ title: "Turn off public access?", body: "Clients outside the cluster lose access immediately.", confirm: "Turn off" })))
                            return;
                        setEnabling(true);
                        await act(() => patch(`/databases/${d.id}`, { public: !d.publicPort }), d.publicPort ? "No longer public" : "Public port assigned");
                        setEnabling(false);
                    } }, d.publicPort ? "Disable" : "Enable")),
            h("div", { style: { marginTop: 14 } }, d.publicPort ? (h("div", { class: "col", style: { gap: 8 } },
                h("div", { class: "callout callout-green", style: { alignItems: "center" } },
                    h(Icon, { name: "globe" }),
                    h("div", { class: "grow mono" },
                        d.publicHost || "<public ip>",
                        ":",
                        d.publicPort),
                    h(CopyButton, { text: `postgresql://${d.user}:${encodeURIComponent(pw)}@${d.publicHost}:${d.publicPort}/${d.dbName}?sslmode=prefer`, label: "Copy public URI", size: "sm" })),
                h("div", { class: "muted", style: { fontSize: 12.5 } }, "Anyone with the password can connect from the internet. Rotate it if it leaks, and prefer the internal host from apps."))) : (h("div", { class: "callout", style: { background: "var(--panel-3)", border: "1px solid var(--border)", color: "var(--text-2)" } },
                h("div", null,
                    "Not publicly exposed. Click ",
                    h("strong", null, "Enable"),
                    " to assign a random free port on your organization's shared database IP. If no shared IP is configured, set one in ",
                    h(Link, { href: `/o/${org}/settings` }, "organization settings"),
                    ".")))))));
}
function Backups({ d }) {
    const [sched, setSched] = useState(d.backupSchedule || "");
    return (h("div", { class: "col gap-lg" },
        h(Card, { class: "form-card" },
            h("div", { class: "row" },
                h("div", { class: "grow" },
                    h("div", { class: "panel-title" }, "Backups"),
                    h("div", { class: "panel-desc" }, "Logical dumps (pg_dump, custom format) kept on the database volume. The last 30 are listed.")),
                d.canEdit ? h(Button, { kind: "primary", icon: "download", disabled: d.status !== "ready", onClick: () => act(() => post(`/databases/${d.id}/backups`), "Backup started") }, "Back up now") : null),
            h("div", { class: "row" },
                h(Field, { label: "Schedule (cron, UTC)", hint: "Empty turns scheduled backups off. Default is 03:00 every day." },
                    h(Input, { value: sched, onInput: setSched, mono: true, placeholder: "0 3 * * *", disabled: !d.canEdit })),
                d.canEdit ? h(Button, { kind: "secondary", onClick: () => act(() => patch(`/databases/${d.id}`, { backupSchedule: sched }), "Schedule saved"), class: "" }, "Save") : null)),
        !d.backups?.length ? h(Empty, { icon: "download", title: "No backups yet" }, "Scheduled backups appear here once the first one runs.") : (h(Table, { head: ["", "Taken", "Method", "Size", "Status"] }, d.backups.map((b) => (h("tr", { key: b.id },
            h("td", { style: { width: 30 } },
                h(Icon, { name: "database", size: 15, class: "muted" })),
            h("td", { title: dateTime(b.createdAt) }, timeAgo(b.createdAt)),
            h("td", null,
                h("span", { class: "tag" }, b.method)),
            h("td", { class: "mono" }, b.sizeBytes ? bytes(b.sizeBytes) : "—"),
            h("td", null,
                h(Pill, { status: b.status === "running" ? "provisioning" : b.status, label: b.status === "running" ? "Running" : undefined })))))))));
}
function DbTerminal({ d }) {
    const [sub, setSub] = useState("psql");
    const cr = useApi(sub === "killed" ? `/databases/${d.id}/crashes` : null, [sub]);
    return (h("div", { class: "col" },
        h(Segmented, { value: sub, onChange: setSub, options: [{ id: "psql", label: "psql", icon: "terminal" }, { id: "killed", label: "Killed", icon: "skull" }] }),
        sub === "psql" ? (d.canEdit ? (d.status === "ready" ? h(Terminal, { path: `/databases/${d.id}/terminal`, title: `psql · ${d.dbName}`, prompt: d.dbName + "=>", height: 440 }) : h(Callout, { kind: "amber" }, "The database must be running to open a terminal.")) : h(Callout, { kind: "info" }, "Only project members can open a terminal.")) : !cr.data ? h(Loading, null) : cr.data.length === 0 ? (h(Empty, { icon: "check", title: "Nothing killed" }, "A Postgres container that gets OOMKilled is replaced fast enough that its logs are usually gone before anyone looks. Wack Club Orchard keeps them here.")) : cr.data.map((c) => (h(Card, { key: c.id },
            h("div", { class: "row", style: { marginBottom: 10 } },
                h("strong", null, c.reason),
                h("span", { class: "muted" },
                    "exit ",
                    c.exitCode,
                    " \u00B7 ",
                    c.pod,
                    " \u00B7 ",
                    timeAgo(c.createdAt))),
            h(OutputView, { text: c.logs, title: "last output", height: 300 }))))));
}
function Extensions({ d }) {
    const [on, setOn] = useState(d.extensions || []);
    useEffect(() => setOn(d.extensions || []), [d.extensions?.join(",")]);
    return (h(Card, { pad: false },
        h("div", { style: { padding: "14px 18px 6px" } },
            h("div", { class: "panel-title" }, "Extensions"),
            h("div", { class: "panel-desc" }, "Enabled with CREATE EXTENSION in the application database. Turning one off keeps its objects until you drop them.")),
        h("div", { style: { padding: "0 18px 8px" } }, d.availableExtensions.map((e) => (h("div", { class: "ext-row", key: e.Name },
            h("div", { class: "grow" },
                h("div", { class: "mono", style: { fontWeight: 500, fontSize: 13.5 } }, e.Name),
                h("div", { class: "muted", style: { fontSize: 13 } }, e.Description)),
            h(Toggle, { checked: on.includes(e.Name), disabled: !d.canEdit, onChange: async (v) => {
                    const next = v ? [...on, e.Name] : on.filter((x) => x !== e.Name);
                    setOn(next);
                    await act(() => patch(`/databases/${d.id}`, { extensions: next }), v ? `${e.Name} enabled` : `${e.Name} removed from the list`);
                } })))))));
}
function Queries({ d }) {
    const [sql, setSql] = useState("select now();");
    const [res, setRes] = useState(null);
    const [busy, setBusy] = useState(false);
    const [history, setHistory] = useState(() => {
        try {
            return JSON.parse(localStorage.getItem("wackcluborchard-sql-" + d.id) || "[]");
        }
        catch {
            return [];
        }
    });
    const run = async () => {
        if (!sql.trim())
            return;
        setBusy(true);
        const r = await act(() => post(`/databases/${d.id}/query`, { sql }));
        setBusy(false);
        setRes(r);
        const h = [sql, ...history.filter((x) => x !== sql)].slice(0, 15);
        setHistory(h);
        try {
            localStorage.setItem("wackcluborchard-sql-" + d.id, JSON.stringify(h));
        }
        catch { }
    };
    if (!d.canEdit)
        return h(Callout, { kind: "info" }, "Only project members can run queries.");
    return (h("div", { class: "col" },
        h(Card, { pad: false },
            h("textarea", { class: "editor sql-editor", spellcheck: "false", value: sql, onInput: (e) => setSql(e.target.value), onKeyDown: (e) => {
                    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
                        e.preventDefault();
                        run();
                    }
                } }),
            h("div", { class: "row", style: { padding: "8px 10px", borderTop: "1px solid var(--border)" } },
                h("span", { class: "muted", style: { fontSize: 12.5 } },
                    "\u2318\u21B5 to run \u00B7 ",
                    h("code", null, "\\dt"),
                    " lists tables"),
                h("span", { class: "grow" }),
                history.length ? h(Select, { value: "", onChange: (v) => v && setSql(v), options: [{ value: "", label: "History…" }, ...history.map((x) => ({ value: x, label: x.slice(0, 60) }))] }) : null,
                h(Button, { kind: "primary", icon: "play", loading: busy, onClick: run, disabled: d.status !== "ready" }, "Run"))),
        res ? (res.error ? h(Callout, { kind: "red", title: "ERROR" }, res.error) : (h(Fragment, null,
            h("div", { class: "result-meta" },
                h("span", null, res.command),
                h("span", null,
                    res.durationMs?.toFixed(1),
                    " ms"),
                res.rows ? h("span", null,
                    res.rows.length,
                    " rows") : null),
            res.columns?.length ? (h(Table, { head: res.columns }, res.rows.map((row, i) => h("tr", { key: i }, row.map((c, j) => h("td", { key: j, class: "mono", style: { fontSize: 12.5 } }, c === "" ? h("span", { class: "faint" }, "null") : c)))))) : null))) : null));
}
function DbMetrics({ d }) {
    return (h("div", { class: "grid-2" },
        h(Card, null,
            h("div", { class: "stat-label" }, "Database size"),
            h("div", { class: "stat-value", style: { fontSize: 24 } }, bytes(d.sizeBytes)),
            h("div", { class: "muted", style: { fontSize: 12.5 } },
                "of ",
                d.storageGi,
                " GiB volume"),
            h("div", { class: "meter", style: { marginTop: 12 } },
                h("div", { class: "meter-bar" },
                    h("div", { class: "meter-fill tone-ok", style: { width: Math.min(100, (d.sizeBytes / (d.storageGi * 1073741824)) * 100) + "%" } })))),
        h(Card, null,
            h("div", { class: "stat-label" }, "Active connections"),
            h("div", { class: "stat-value", style: { fontSize: 24 } }, d.connections),
            h("div", { class: "muted", style: { fontSize: 12.5 } }, "client backends right now")),
        h(Card, null,
            h("div", { class: "stat-label" }, "Compute"),
            h("div", { class: "stat-value" },
                cpu(d.resources.cpuMillis),
                " \u00B7 ",
                mem(d.resources.memoryMi)),
            h("div", { class: "muted", style: { fontSize: 12.5 } }, "per instance, requests equal limits")),
        h(Card, null,
            h("div", { class: "stat-label" }, "Topology"),
            h("div", { class: "stat-value" },
                "1 primary",
                d.instances > 1 ? ` + ${d.instances - 1} read replica${d.instances > 2 ? "s" : ""}` : ""),
            h("div", { class: "muted", style: { fontSize: 12.5 } },
                "CloudNativePG cluster pg-",
                d.name))));
}
function Resources({ d }) {
    const [res, setRes] = useState({ ...d.resources });
    const [storage, setStorage] = useState(d.storageGi);
    const [inst, setInst] = useState(d.instances);
    return (h(Card, { class: "form-card" },
        h("div", { class: "form-grid" },
            h(Field, { label: "CPU per instance" },
                h(Select, { value: String(res.cpuMillis), onChange: (v) => setRes({ ...res, cpuMillis: Number(v) }), disabled: !d.canEdit, options: [250, 500, 1000, 2000, 4000].map((c) => ({ value: String(c), label: cpu(c) })) })),
            h(Field, { label: "Memory per instance" },
                h(Select, { value: String(res.memoryMi), onChange: (v) => setRes({ ...res, memoryMi: Number(v) }), disabled: !d.canEdit, options: [256, 512, 1024, 2048, 4096, 8192].map((m) => ({ value: String(m), label: mem(m) })) })),
            h(Field, { label: "Storage (GiB)", hint: "Volumes can grow but not shrink." },
                h(Input, { type: "number", value: storage, min: d.storageGi, onInput: (v) => setStorage(Number(v)), disabled: !d.canEdit })),
            h(Field, { label: "Read replicas", hint: "Replicas stream from the primary and serve read-only queries on the -ro service." },
                h(Select, { value: String(inst - 1), onChange: (v) => setInst(Number(v) + 1), disabled: !d.canEdit, options: [0, 1, 2, 3, 4].map((n) => ({ value: String(n), label: n === 0 ? "None" : String(n) })) }))),
        d.canEdit ? h("div", { class: "form-card-foot" },
            h(Button, { kind: "primary", onClick: () => act(() => patch(`/databases/${d.id}`, { resources: res, storageGi: storage, instances: inst }), "Saved; the cluster is rolling") }, "Save")) : null));
}
function DbSettings({ d }) {
    return (h(Card, { class: "form-card" },
        h("div", { class: "panel-title" }, "Details"),
        h(DefList, { rows: [
                ["Engine", `PostgreSQL ${d.version} (CloudNativePG)`],
                ["Cluster", `pg-${d.name}`],
                ["Namespace", d.namespace],
                ["Read-write service", d.host],
                ["Read-only service", d.host.replace("-rw.", "-ro.")],
                ["Created", dateTime(d.createdAt)],
            ] })));
}
