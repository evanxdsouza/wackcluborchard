import { h, Fragment, useEffect, useRef, useState } from "../lib/sprout.js";
import { Link, navigate, query } from "../lib/router.js";
import { useApi, useEvents, post, put, patch, del, act, get } from "../lib/api.js";
import { cpu, mem, timeAgo, plural } from "../lib/format.js";
import { Button, Loading, ErrorBox, Empty, Pill, Card, Field, Input, Select, Meter, confirm, Callout, Table, IconButton, cx, openModal, ModalHeader, Segmented, Tag, Textarea } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";
import { Sky, Avatar, ImageBadge } from "../ui/art.js";
import { Terminal } from "../ui/term.js";
export function UsagePage({ org }) {
    const r = useApi(`/orgs/${org}/usage`);
    if (r.error)
        return h("div", { class: "page" },
            h(ErrorBox, { error: r.error }));
    if (!r.data)
        return h("div", { class: "page" },
            h(Loading, null));
    const u = r.data;
    const gi = (n) => n + " GiB";
    const count = (n) => String(n);
    return (h("div", { class: "page" },
        h("div", { class: "page-head" },
            h("div", null,
                h("h1", { class: "page-title" }, "Usage"),
                h("div", { class: "page-sub" }, "Quotas are enforced against requests. The effective limit for anything new is the smaller of your allowance and the organization's remaining cap."))),
        h("div", { class: "grid-2" },
            h(Card, { class: "form-card" },
                h("div", { class: "panel-title" }, "Organization"),
                h(Meter, { label: "CPU", used: u.used.cpuMillis, cap: u.quota.cpuMillis, format: cpu }),
                h(Meter, { label: "Memory", used: u.used.memoryMi, cap: u.quota.memoryMi, format: mem }),
                h(Meter, { label: "Storage", used: u.used.storageGi, cap: u.quota.storageGi, format: gi }),
                h("div", { class: "grid-3" },
                    h(Meter, { label: "Apps", used: u.used.apps, cap: u.quota.apps, format: count }),
                    h(Meter, { label: "Databases", used: u.used.databases, cap: u.quota.databases, format: count }),
                    h(Meter, { label: "Sandboxes", used: u.used.sandboxes, cap: u.quota.sandboxes, format: count }))),
            h(Card, { class: "form-card" },
                h("div", { class: "panel-title" }, "Your allowance"),
                h(Meter, { label: "CPU", used: u.memberUsed.cpuMillis, cap: u.memberQuota.cpuMillis, format: cpu }),
                h(Meter, { label: "Memory", used: u.memberUsed.memoryMi, cap: u.memberQuota.memoryMi, format: mem }),
                h(Meter, { label: "Storage", used: u.memberUsed.storageGi, cap: u.memberQuota.storageGi, format: gi }),
                h("div", { class: "grid-3" },
                    h(Meter, { label: "Apps", used: u.memberUsed.apps, cap: u.memberQuota.apps, format: count }),
                    h(Meter, { label: "Databases", used: u.memberUsed.databases, cap: u.memberQuota.databases, format: count }),
                    h(Meter, { label: "Sandboxes", used: u.memberUsed.sandboxes, cap: u.memberQuota.sandboxes, format: count })),
                h("div", { class: "muted", style: { fontSize: 12.5 } }, "Owners and admins are only bound by the organization cap."))),
        h("div", { class: "section" },
            h("div", { class: "section-head" },
                h("h2", { class: "section-title" }, "By project"),
                h("span", { class: "muted", style: { fontSize: 13 } },
                    "Builds: ",
                    u.builds.running,
                    "/",
                    u.builds.slots,
                    " slots busy",
                    u.builds.waiting ? `, ${u.builds.waiting} queued` : "")),
            !u.projects?.length ? h(Empty, { title: "No projects yet" }) : (h(Table, { head: ["Project", "Apps", "Databases", "CPU requested", "Memory requested", "Storage", "Live CPU / memory"] }, u.projects.map((p) => (h("tr", { key: p.id, class: "clickable", onClick: () => navigate(`/o/${org}/projects/${p.id}`) },
                h("td", null,
                    h("strong", null,
                        p.icon,
                        " ",
                        p.name)),
                h("td", null, p.apps),
                h("td", null, p.databases),
                h("td", { class: "mono" }, cpu(p.cpuMillis)),
                h("td", { class: "mono" }, mem(p.memoryMi)),
                h("td", { class: "mono" },
                    p.storageGi,
                    " GiB"),
                h("td", { class: "mono muted" },
                    cpu(p.liveCpuMillis),
                    " \u00B7 ",
                    mem(p.liveMemoryMi))))))))));
}
export function GrovePage({ org }) {
    const r = useApi("/templates");
    const [cat, setCat] = useState("All");
    const [q, setQ] = useState("");
    const cats = ["All", ...new Set((r.data || []).map((t) => t.category))];
    const shown = (r.data || []).filter((t) => (cat === "All" || t.category === cat) && (!q || (t.name + t.description).toLowerCase().includes(q.toLowerCase())));
    return (h("div", { class: "page" },
        h("div", { class: "grove-hero" },
            h(Sky, { seed: "the-grove", preset: "meadow" }),
            h("div", { class: "grove-hero-text" },
                h("h1", null, "The Grove"),
                h("p", null, "Ready-made stacks. Apps, databases and the variables between them, planted in one go."))),
        h("div", { class: "row row-wrap", style: { marginBottom: 16 } },
            h(Segmented, { value: cat, onChange: setCat, options: cats.map((c) => ({ id: c, label: c })) }),
            h("span", { class: "grow" }),
            h("div", { class: "search-input" },
                h(Icon, { name: "search", size: 15 }),
                h("input", { class: "input", placeholder: "Search templates\u2026", value: q, onInput: (e) => setQ(e.target.value) }))),
        !r.data ? h(Loading, null) : (h("div", { class: "grid-3" }, shown.map((t) => (h(Card, { key: t.id, class: "tpl-card" },
            h("div", { class: "row" },
                h(ImageBadge, { image: t.apps[0]?.image || "", name: t.name, size: 40 }),
                h("div", { class: "grow" },
                    h("div", { style: { fontWeight: 600, fontSize: 15.5 } }, t.name),
                    h("div", { class: "tpl-cat" }, t.category))),
            h("div", { class: "tpl-desc" }, t.description),
            h("div", { class: "row row-wrap", style: { gap: 5 } },
                t.apps.map((a, i) => h(Tag, { key: i, mono: true }, a.image.split("/").pop())),
                t.databases?.map((d) => h(Tag, { key: d },
                    h(Icon, { name: "database", size: 11 }),
                    " postgres"))),
            h(Button, { kind: "secondary", icon: "sprout", onClick: () => openModal((close) => h(PlantTemplate, { org: org, t: t, close: close })) }, "Deploy"))))))));
}
function PlantTemplate({ org, t, close }) {
    const r = useApi(`/orgs/${org}/overview`);
    const projects = (r.data || []).filter((p) => p.canEdit);
    const [pid, setPid] = useState(query().get("project") || "");
    const [name, setName] = useState(t.id.split("-")[0]);
    useEffect(() => {
        if (!pid && projects.length)
            setPid(projects[0].id);
    }, [projects.length]);
    return (h(Fragment, null,
        h(ModalHeader, { title: `Deploy ${t.name}`, subtitle: t.description, onClose: close, icon: "trees" }),
        h("div", { class: "modal-body" },
            h(Field, { label: "Project" },
                h(Select, { value: pid, onChange: setPid, options: projects.map((p) => ({ value: p.id, label: `${p.icon} ${p.name}` })) })),
            h(Field, { label: "Instance name", hint: t.databases?.length ? `Creates the app ${name} and the database ${name}-db, wired together.` : `Creates the app ${name}.` },
                h(Input, { value: name, onInput: (v) => setName(v.toLowerCase().replace(/[^a-z0-9-]/g, "-")), mono: true, autofocus: true }))),
        h("div", { class: "modal-foot" },
            h(Button, { kind: "ghost", onClick: close }, "Cancel"),
            h(Button, { kind: "primary", icon: "sprout", disabled: !pid || !name, onClick: async () => {
                    const inst = await act(() => post(`/projects/${pid}/templates`, { templateId: t.id, name }), `${t.name} is being planted`);
                    if (inst) {
                        close();
                        navigate(`/o/${org}/projects/${pid}`);
                    }
                } }, "Deploy"))));
}
export function GreenhousePage({ org }) {
    const r = useApi(`/orgs/${org}/sandboxes`);
    const o = useApi(`/orgs/${org}`);
    useEvents(o.data ? [`org:${o.data.id}`] : [], (e) => {
        if (e.type === "sandbox.updated")
            r.set((list) => list.map((s) => (s.id === e.data.id ? { ...s, ...e.data, conversations: undefined } : s)));
    });
    return (h("div", { class: "page" },
        h("div", { class: "page-head" },
            h("div", null,
                h("h1", { class: "page-title" }, "Greenhouse"),
                h("div", { class: "page-sub" }, "Isolated cloud dev environments: a persistent workspace with an editor, a terminal, git, and an agent.")),
            h("div", { class: "page-actions" },
                h(Button, { kind: "primary", icon: "plus", onClick: () => openModal((close) => h(NewSandbox, { org: org, close: close })) }, "New sandbox"))),
        h(Callout, { kind: "info", icon: "shield" }, "Sandboxes run as Kata micro-VMs with internet egress only: they can reach out, but not into the rest of your cluster. Deleting one deletes its workspace, so push before you delete."),
        h("div", { style: { height: 16 } }),
        !r.data ? h(Loading, null) : r.data.length === 0 ? (h(Empty, { icon: "sprout", title: "Nothing growing yet", action: h(Button, { kind: "primary", icon: "plus", onClick: () => openModal((close) => h(NewSandbox, { org: org, close: close })) }, "New sandbox") }, "Start a sandbox from a repository and work on it from the browser, or let the agent take a first pass.")) : (h("div", { class: "grid-3" }, r.data.map((s) => (h(Link, { key: s.id, href: `/o/${org}/greenhouse/${s.id}`, class: "card card-link", style: { padding: 16, display: "flex", flexDirection: "column", gap: 10 } },
            h("div", { class: "row" },
                h("div", { class: "tpl-icon", style: { width: 40, height: 40 } },
                    h(Icon, { name: "sprout", size: 18 })),
                h("div", { class: "grow" },
                    h("div", { style: { fontWeight: 600 } }, s.name),
                    h("div", { class: "muted mono truncate", style: { fontSize: 12 } }, s.repo || s.image || "blank workspace")),
                h(Pill, { status: s.status })),
            s.status === "booting" ? (h("div", null,
                h("div", { class: "boot-bar" },
                    h("div", { class: "boot-fill", style: { width: s.bootProgress + "%" } })),
                h("div", { class: "muted", style: { fontSize: 12, marginTop: 5 } }, s.bootStage))) : null,
            h("div", { class: "row muted", style: { fontSize: 12.5 } },
                h(Avatar, { seed: s.owner.avatarSeed, size: 18 }),
                " ",
                s.owner.username,
                " \u00B7 ",
                cpu(s.resources.cpuMillis),
                " \u00B7 ",
                mem(s.resources.memoryMi),
                " \u00B7 ",
                s.storageGi,
                " GiB \u00B7 ",
                timeAgo(s.createdAt)))))))));
}
function NewSandbox({ org, close }) {
    const [name, setName] = useState("");
    const [repo, setRepo] = useState("");
    const [image, setImage] = useState("");
    return (h(Fragment, null,
        h(ModalHeader, { title: "New sandbox", subtitle: "A persistent workspace in its own micro-VM.", onClose: close, icon: "sprout" }),
        h("div", { class: "modal-body" },
            h(Field, { label: "Name" },
                h(Input, { value: name, onInput: (v) => setName(v.toLowerCase().replace(/[^a-z0-9-]/g, "-")), autofocus: true, mono: true, placeholder: "scratch" })),
            h(Field, { label: "Repository", hint: "Optional. owner/name, cloned into /workspace on first boot." },
                h(Input, { value: repo, onInput: setRepo, mono: true, placeholder: "hackclub/orchard" })),
            h(Field, { label: "Image", hint: "Optional. Defaults to a devcontainer with common toolchains." },
                h(Input, { value: image, onInput: setImage, mono: true, placeholder: "mcr.microsoft.com/devcontainers/universal:2-linux" }))),
        h("div", { class: "modal-foot" },
            h(Button, { kind: "ghost", onClick: close }, "Cancel"),
            h(Button, { kind: "primary", disabled: !name, onClick: async () => {
                    const s = await act(() => post(`/orgs/${org}/sandboxes`, { name, repo, image }), "Booting");
                    if (s) {
                        close();
                        navigate(`/o/${org}/greenhouse/${s.id}`);
                    }
                } }, "Create"))));
}
export function SandboxPage({ org, id }) {
    const r = useApi(`/sandboxes/${id}`);
    const [pane, setPane] = useState("editor");
    useEvents([`sandbox:${id}`], (e) => {
        if (e.type === "sandbox.updated")
            r.set((d) => ({ ...d, sandbox: { ...d.sandbox, ...e.data, conversations: d.sandbox.conversations } }));
        if (e.type === "conversation.updated")
            r.set((d) => ({ ...d, sandbox: { ...d.sandbox, conversations: d.sandbox.conversations.map((c) => (c.id === e.data.id ? e.data : c)) } }));
    });
    if (r.error)
        return h("div", { class: "page" },
            h(ErrorBox, { error: r.error }));
    if (!r.data)
        return h("div", { class: "page" },
            h(Loading, null));
    const s = r.data.sandbox;
    const running = s.status === "running";
    return (h("div", { class: "page", style: { maxWidth: 1500 } },
        h(Link, { href: `/o/${org}/greenhouse`, class: "back-link" },
            h(Icon, { name: "arrow-left", size: 14 }),
            " Greenhouse"),
        h("div", { class: "page-head", style: { marginBottom: 12 } },
            h("h1", { class: "page-title", style: { fontSize: 22 } }, s.name),
            h(Pill, { status: s.status }),
            s.repo ? h(Tag, { mono: true },
                h(Icon, { name: "github", size: 12 }),
                " ",
                s.repo) : null,
            h("div", { class: "page-actions" },
                h(Segmented, { value: pane, onChange: setPane, options: [{ id: "editor", label: "Files", icon: "folder" }, { id: "terminal", label: "Terminal", icon: "terminal" }, { id: "git", label: "Git", icon: "branch" }] }),
                h(Button, { kind: "secondary", icon: "restart", onClick: () => act(() => post(`/sandboxes/${id}/restart`), "Restarting") }, "Restart"),
                h(Button, { kind: "danger", icon: "trash", onClick: async () => {
                        if (await confirm({ title: `Delete ${s.name}?`, body: "Its persistent workspace is deleted. Anything not committed and pushed is gone.", danger: true, confirm: "Delete sandbox", typeToConfirm: s.name })) {
                            if (await act(() => del(`/sandboxes/${id}`), "Deleted"))
                                navigate(`/o/${org}/greenhouse`);
                        }
                    } }, "Delete"))),
        !running ? (h(Card, null,
            h("div", { class: "panel-title" }, s.status === "failed" ? "The sandbox failed to boot" : "Booting…"),
            h("div", { class: "boot-bar", style: { margin: "12px 0 6px" } },
                h("div", { class: "boot-fill", style: { width: s.bootProgress + "%" } })),
            h("div", { class: "muted", style: { fontSize: 13 } }, s.bootStage))) : (h("div", { class: "workspace" },
            pane === "editor" ? h(Files, { id: id }) : pane === "terminal" ? h("div", { class: "ws-panel", style: { gridColumn: "span 2" } },
                h(Terminal, { path: `/sandboxes/${id}/shell`, title: "/workspace", height: "100%" })) : h(Git, { id: id }),
            h(Agent, { id: id, s: s })))));
}
function Files({ id }) {
    const files = useApi(`/sandboxes/${id}/files`);
    const [open, setOpen] = useState(null);
    const [content, setContent] = useState("");
    const [orig, setOrig] = useState("");
    const load = async (p) => {
        const r = await act(() => get(`/sandboxes/${id}/file?path=${encodeURIComponent(p)}`));
        if (r) {
            setOpen(p);
            setContent(r.content);
            setOrig(r.content);
        }
    };
    const save = async () => {
        if (open && (await act(() => put(`/sandboxes/${id}/file`, { path: open, content }), "Saved")))
            setOrig(content);
    };
    return (h(Fragment, null,
        h("div", { class: "ws-panel" },
            h("div", { class: "ws-head" },
                h(Icon, { name: "folder", size: 14 }),
                " workspace ",
                h("span", { class: "grow" }),
                h(IconButton, { icon: "plus", class: "tiny", title: "New file", onClick: async () => {
                        const p = prompt("New file path");
                        if (p && (await act(() => put(`/sandboxes/${id}/file`, { path: p, content: "" })))) {
                            await files.reload();
                            load(p);
                        }
                    } }),
                h(IconButton, { icon: "restart", class: "tiny", title: "Refresh", onClick: files.reload })),
            h("div", { class: "ws-body file-tree" }, !files.data ? h(Loading, null) : files.data.map((f) => (h("button", { key: f, type: "button", class: cx("file-item", open === f && "active"), title: f, onClick: () => load(f) },
                h(Icon, { name: "file", size: 13 }),
                h("span", { style: { paddingLeft: (f.split("/").length - 1) * 10 } }, f)))))),
        h("div", { class: "ws-panel" },
            h("div", { class: "ws-head" },
                h(Icon, { name: "code", size: 14 }),
                " ",
                open || "No file open",
                open && content !== orig ? h("span", { style: { color: "var(--amber)" } }, "\u25CF modified") : null,
                h("span", { class: "grow" }),
                open ? h(Button, { size: "sm", kind: "primary", icon: "save", onClick: save, disabled: content === orig }, "Save") : null),
            open ? (h("textarea", { class: "editor", spellcheck: "false", value: content, onInput: (e) => setContent(e.target.value), onKeyDown: (e) => {
                    if ((e.metaKey || e.ctrlKey) && e.key === "s") {
                        e.preventDefault();
                        save();
                    }
                    if (e.key === "Tab") {
                        e.preventDefault();
                        const t = e.target;
                        const st = t.selectionStart;
                        setContent(content.slice(0, st) + "  " + content.slice(t.selectionEnd));
                        requestAnimationFrame(() => (t.selectionStart = t.selectionEnd = st + 2));
                    }
                } })) : h("div", { class: "ws-body" },
                h(Empty, { icon: "file", title: "Pick a file" }, "Edits are written straight into the sandbox's workspace. \u2318S saves.")))));
}
function Git({ id }) {
    const [out, setOut] = useState("");
    const [msg, setMsg] = useState("");
    const [branch, setBranch] = useState("");
    const run = async (op, extra = {}) => {
        const r = await act(() => post(`/sandboxes/${id}/git`, { op, ...extra }));
        if (r)
            setOut(`$ git ${op}\n` + (r.output || "") + (r.error ? "\nerror: " + r.error : ""));
    };
    useEffect(() => {
        run("status");
    }, []);
    return (h(Fragment, null,
        h("div", { class: "ws-panel" },
            h("div", { class: "ws-head" },
                h(Icon, { name: "branch", size: 14 }),
                " git"),
            h("div", { class: "ws-body", style: { padding: 12, display: "flex", flexDirection: "column", gap: 10 } },
                h("div", { class: "row row-wrap", style: { gap: 6 } }, ["status", "branches", "diff", "log", "fetch"].map((o) => h(Button, { key: o, size: "sm", kind: "secondary", onClick: () => run(o) }, o))),
                h(Field, { label: "Branch" },
                    h(Input, { value: branch, onInput: setBranch, mono: true, placeholder: "feature/thing" })),
                h("div", { class: "row", style: { gap: 6 } },
                    h(Button, { size: "sm", kind: "secondary", onClick: () => run("checkout", { branch }), disabled: !branch }, "Switch"),
                    h(Button, { size: "sm", kind: "secondary", onClick: () => run("checkout", { branch, create: true }), disabled: !branch }, "Create")),
                h(Field, { label: "Commit message" },
                    h(Textarea, { value: msg, onInput: setMsg, rows: 3 })),
                h("div", { class: "row row-wrap", style: { gap: 6 } },
                    h(Button, { size: "sm", kind: "primary", icon: "check", onClick: () => run("commit", { message: msg }), disabled: !msg }, "Commit all"),
                    h(Button, { size: "sm", kind: "ghost", onClick: () => run("uncommit") }, "Uncommit")),
                h("div", { class: "row row-wrap", style: { gap: 6 } },
                    h(Button, { size: "sm", kind: "secondary", icon: "download", onClick: () => run("pull", { rebase: true }) }, "Pull --rebase"),
                    h(Button, { size: "sm", kind: "secondary", icon: "upload", onClick: () => run("push") }, "Push"),
                    h(Button, { size: "sm", kind: "ghost", onClick: async () => { if (await confirm({ title: "Force push?", body: "Uses --force-with-lease: it refuses if the remote moved under you, instead of overwriting someone else's work.", confirm: "Force push" }))
                            run("push", { force: true }); } }, "Force")),
                h(Button, { size: "sm", kind: "secondary", icon: "github", onClick: () => run("pr", { title: msg }) }, "Open pull request"))),
        h("div", { class: "ws-panel", style: { background: "var(--terminal)" } },
            h("div", { class: "ws-head", style: { color: "#8b8b93", borderColor: "#1d1d21" } }, "output"),
            h("pre", { class: "ws-body mono", style: { margin: 0, padding: 14, color: "#ddd", fontSize: 12.5, whiteSpace: "pre-wrap" } }, out))));
}
function Agent({ id, s }) {
    const convs = s.conversations || [];
    const [cur, setCur] = useState(convs[0]?.id || null);
    const [text, setText] = useState("");
    const c = convs.find((x) => x.id === cur);
    const scroll = useRef(null);
    useEffect(() => {
        if (scroll.current)
            scroll.current.scrollTop = scroll.current.scrollHeight;
    }, [c?.messages?.length]);
    const newConv = async () => {
        const n = await act(() => post(`/sandboxes/${id}/conversations`, { cwd: s.repo ? s.repo.split("/").pop() : "" }));
        if (n) {
            s.conversations = [n, ...convs];
            setCur(n.id);
        }
    };
    const send = async () => {
        if (!text.trim())
            return;
        let target = cur;
        if (!target) {
            const n = await act(() => post(`/sandboxes/${id}/conversations`, { cwd: s.repo ? s.repo.split("/").pop() : "" }));
            if (!n)
                return;
            s.conversations = [n, ...convs];
            target = n.id;
            setCur(n.id);
        }
        const t = text;
        setText("");
        await act(() => post(`/sandboxes/${id}/conversations/${target}/messages`, { text: t }));
    };
    const last = c?.messages?.[c.messages.length - 1];
    const thinking = last && last.role !== "assistant" && last.role !== "error";
    return (h("div", { class: "ws-panel ws-chat" },
        h("div", { class: "ws-head" },
            h(Icon, { name: "bot", size: 14 }),
            " Agent",
            h("span", { class: "grow" }),
            convs.length ? (h("select", { class: "input select", style: { height: 26, width: 150, fontSize: 12 }, onChange: (e) => setCur(e.target.value) }, convs.map((x) => h("option", { key: x.id, value: x.id, selected: x.id === cur }, x.title)))) : null,
            h(IconButton, { icon: "plus", class: "tiny", title: "New conversation", onClick: newConv }),
            c ? h(IconButton, { icon: "trash", class: "tiny", title: "Delete conversation", onClick: async () => {
                    if (await act(() => del(`/sandboxes/${id}/conversations/${c.id}`))) {
                        s.conversations = convs.filter((x) => x.id !== c.id);
                        setCur(s.conversations[0]?.id || null);
                    }
                } }) : null),
        h("div", { class: "ws-body chat", ref: scroll },
            !c || !c.messages.length ? (h("div", { class: "muted", style: { fontSize: 13, textAlign: "center", padding: "30px 10px" } },
                h(Icon, { name: "bot", size: 22 }),
                h("br", null),
                "Ask Claude to work in this workspace. It reads and writes real files and runs real commands.",
                c?.cwd ? h(Fragment, null,
                    h("br", null),
                    "Working in ",
                    h("code", null, c.cwd),
                    ".") : null)) : c.messages.map((m, i) => h("div", { key: i, class: "msg " + m.role }, m.text)),
            thinking ? h("div", { class: "msg assistant muted" },
                h("span", { class: "spinner sm" }),
                " working\u2026") : null),
        h("div", { class: "chat-input" },
            h(Textarea, { value: text, onInput: setText, rows: 2, placeholder: "Add a /healthz endpoint and a test\u2026", onKeyDown: (e) => {
                    if (e.key === "Enter" && !e.shiftKey) {
                        e.preventDefault();
                        send();
                    }
                } }),
            h(IconButton, { icon: "send", title: "Send", onClick: send, class: "boxed" }))));
}
export { patch, plural };
