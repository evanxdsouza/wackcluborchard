import { h, Fragment, useEffect, useState } from "../lib/sprout.js";
import { Link, navigate, query, setQuery } from "../lib/router.js";
import { useApi, post, put, patch, del, act, get } from "../lib/api.js";
import { cpu, mem, timeAgo, dateTime, plural } from "../lib/format.js";
import { session, use, loadSession, toast } from "../lib/state.js";
import { Button, Loading, ErrorBox, Empty, Pill, Card, Field, Input, Select, Toggle, confirm, Callout, Table, IconButton, cx, openModal, ModalHeader, Tag, CopyButton, CodeBlock, Textarea, Meter } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";
import { Avatar } from "../ui/art.js";
import { passkeysSupported, registerPasskey } from "./auth.js";
function SettingsLayout({ title, sub, tabs, tab, setTab, children }) {
    return (h("div", { class: "page", style: { maxWidth: 1080 } },
        h("div", { class: "page-head" },
            h("div", null,
                h("h1", { class: "page-title" }, title),
                sub ? h("div", { class: "page-sub" }, sub) : null)),
        h("div", { class: "settings" },
            h("nav", { class: "settings-nav" }, tabs.map((t) => h("button", { key: t.id, type: "button", class: cx(t.id === tab && "active"), onClick: () => setTab(t.id) },
                h(Icon, { name: t.icon, size: 15 }),
                " ",
                t.label))),
            h("div", { class: "col gap-lg" }, children))));
}
function useTab(def) {
    const [t, set] = useState(query().get("tab") || def);
    return [t, (x) => { set(x); setQuery("tab", x === def ? null : x); }];
}
export function MembersPage({ org }) {
    const o = useApi(`/orgs/${org}`);
    if (!o.data)
        return h("div", { class: "page" },
            h(Loading, null));
    return (h("div", { class: "page", style: { maxWidth: 980 } },
        h("div", { class: "page-head" },
            h("div", null,
                h("h1", { class: "page-title" }, "Members"),
                h("div", { class: "page-sub" },
                    "People in ",
                    o.data.name,
                    ". Project membership decides which apps each person can reach."))),
        h(Members, { org: org, o: o.data })));
}
export function OrgSettingsPage({ org }) {
    const o = useApi(`/orgs/${org}`);
    const [tab, setTab] = useTab("general");
    if (o.error)
        return h("div", { class: "page" },
            h(ErrorBox, { error: o.error }));
    if (!o.data)
        return h("div", { class: "page" },
            h(Loading, null));
    const admin = o.data.role === "owner" || o.data.role === "admin";
    return (h(SettingsLayout, { title: "Organization settings", sub: o.data.name, tab: tab, setTab: setTab, tabs: [
            { id: "general", label: "General", icon: "settings" },
            { id: "members", label: "Members", icon: "users" },
            { id: "quotas", label: "Quotas", icon: "activity" },
            { id: "sso", label: "SSO & SCIM", icon: "key" },
            { id: "audit", label: "Audit log", icon: "history" },
            { id: "danger", label: "Danger zone", icon: "alert" },
        ] },
        !admin && tab !== "members" ? h(Callout, { kind: "info" }, "Only owners and admins can change organization settings.") : null,
        tab === "general" ? h(OrgGeneral, { o: o.data, reload: o.reload, admin: admin }) : null,
        tab === "members" ? h(Members, { org: org, o: o.data }) : null,
        tab === "quotas" ? h(Quotas, { o: o.data, reload: o.reload, admin: admin }) : null,
        tab === "sso" ? h(SSO, { o: o.data, reload: o.reload, admin: admin }) : null,
        tab === "audit" ? (admin ? h(Audit, { org: org }) : null) : null,
        tab === "danger" ? h(OrgDanger, { o: o.data }) : null));
}
function OrgGeneral({ o, reload, admin }) {
    const [name, setName] = useState(o.name);
    const [ip, setIp] = useState(o.publicIp || "");
    const [def, setDef] = useState({ ...o.defaults });
    return (h(Fragment, null,
        h(Card, { class: "form-card" },
            h("div", { class: "panel-title" }, "General"),
            h("div", { class: "form-grid" },
                h(Field, { label: "Name" },
                    h(Input, { value: name, onInput: setName, disabled: !admin })),
                h(Field, { label: "Slug", hint: "Used in URLs and namespaces." },
                    h(Input, { value: o.slug, onInput: () => { }, disabled: true, mono: true }))),
            admin ? h("div", { class: "form-card-foot" },
                h(Button, { kind: "primary", onClick: async () => { if (await act(() => patch(`/orgs/${o.slug}`, { name }), "Saved")) {
                        reload();
                        loadSession();
                    } } }, "Save")) : null),
        h(Card, { class: "form-card" },
            h("div", null,
                h("div", { class: "panel-title" }, "Shared public IP"),
                h("div", { class: "panel-desc" }, "Raw TCP/UDP ports and public databases are published on this address. Leave empty to inherit the instance default.")),
            h(Field, { label: "Public IP" },
                h(Input, { value: ip, onInput: setIp, placeholder: "203.0.113.10", mono: true, disabled: !admin })),
            admin ? h("div", { class: "form-card-foot" },
                h(Button, { kind: "primary", onClick: () => act(() => patch(`/orgs/${o.slug}`, { publicIp: ip }), "Saved") }, "Save")) : null),
        h(Card, { class: "form-card" },
            h("div", null,
                h("div", { class: "panel-title" }, "Default app size"),
                h("div", { class: "panel-desc" }, "Applied when an app is created without explicit resources.")),
            h("div", { class: "form-grid" },
                h(Field, { label: "CPU (millicores)" },
                    h(Input, { type: "number", value: def.cpuMillis, onInput: (v) => setDef({ ...def, cpuMillis: Number(v) }), disabled: !admin })),
                h(Field, { label: "Memory (MiB)" },
                    h(Input, { type: "number", value: def.memoryMi, onInput: (v) => setDef({ ...def, memoryMi: Number(v) }), disabled: !admin }))),
            admin ? h("div", { class: "form-card-foot" },
                h(Button, { kind: "primary", onClick: () => act(() => patch(`/orgs/${o.slug}`, { defaults: def }), "Saved") }, "Save")) : null)));
}
const roles = [
    { value: "owner", label: "Owner" },
    { value: "admin", label: "Admin" },
    { value: "member", label: "Member" },
    { value: "viewer", label: "Viewer" },
];
function Members({ org, o }) {
    const r = useApi(`/orgs/${org}/members`);
    const s = use(session);
    const admin = o.role === "owner" || o.role === "admin";
    const [email, setEmail] = useState("");
    const [role, setRole] = useState("member");
    const [link, setLink] = useState("");
    if (!r.data)
        return h(Loading, null);
    return (h(Fragment, null,
        admin ? (h(Card, { class: "form-card" },
            h("div", null,
                h("div", { class: "panel-title" }, "Invite people"),
                h("div", { class: "panel-desc" }, "Enter a username to add someone who already has an account, or an email to make an invite link.")),
            h("div", { class: "row" },
                h(Input, { value: email, onInput: setEmail, placeholder: "username or someone@example.com" }),
                h(Select, { value: role, onChange: setRole, options: roles, class: "role-select" }),
                h(Button, { kind: "primary", icon: "plus", disabled: !email, onClick: async () => {
                        const isEmail = email.includes("@");
                        const res = await act(() => post(`/orgs/${org}/members`, isEmail ? { email, role } : { username: email, role }), isEmail ? "Invite created" : "Added");
                        if (res) {
                            setEmail("");
                            if (res.url)
                                setLink(res.url);
                            r.reload();
                        }
                    } }, "Invite")),
            link ? h(Callout, { kind: "green", title: "Share this invite link" },
                "It works once. ",
                h(CodeBlock, { code: link })) : null,
            h("div", { class: "muted", style: { fontSize: 12.5 } }, "Owners: everything, including deleting the organization. Admins: members, projects, quotas, settings. Members: resources in projects they belong to. Viewers: read-only."))) : null,
        h(Table, { head: ["Person", "Role", "Projects", "Using", "Joined", ""] }, r.data.members.map((m) => (h("tr", { key: m.id },
            h("td", null,
                h("div", { class: "row" },
                    h(Avatar, { seed: m.avatarSeed, size: 30 }),
                    h("div", null,
                        h("div", { style: { fontWeight: 600 } },
                            m.name,
                            m.id === s.me?.id ? h("span", { class: "muted" }, " (you)") : null),
                        h("div", { class: "muted mono", style: { fontSize: 12 } },
                            "@",
                            m.username,
                            m.email ? " · " + m.email : "")))),
            h("td", null, admin && m.id !== s.me?.id ? h(Select, { value: m.role, onChange: async (v) => { if (await act(() => patch(`/orgs/${org}/members/${m.id}`, { role: v }), "Role changed"))
                    r.reload(); }, options: roles, class: "role-select" }) : h("span", { class: "tag" }, m.role)),
            h("td", null, m.projects),
            h("td", { class: "mono muted", style: { fontSize: 12.5 } },
                cpu(m.usage.cpuMillis),
                " \u00B7 ",
                mem(m.usage.memoryMi)),
            h("td", { class: "muted" }, timeAgo(m.joinedAt)),
            h("td", { style: { textAlign: "right" } }, (admin && m.id !== s.me?.id) || m.id === s.me?.id ? (h(IconButton, { icon: m.id === s.me?.id ? "logout" : "x", title: m.id === s.me?.id ? "Leave organization" : "Remove", onClick: async () => {
                    if (await confirm({ title: m.id === s.me?.id ? `Leave ${o.name}?` : `Remove ${m.name}?`, body: "Their apps keep running; they lose access.", danger: true, confirm: m.id === s.me?.id ? "Leave" : "Remove" })) {
                        if (await act(() => del(`/orgs/${org}/members/${m.id}`), "Done")) {
                            if (m.id === s.me?.id) {
                                await loadSession();
                                navigate("/");
                            }
                            else
                                r.reload();
                        }
                    }
                } })) : null))))),
        r.data.invites.length ? (h(Card, null,
            h("div", { class: "panel-title", style: { marginBottom: 8 } }, "Pending invites"),
            r.data.invites.map((i) => (h("div", { key: i.id, class: "row", style: { padding: "8px 0", borderTop: "1px solid var(--border)" } },
                h(Icon, { name: "message", class: "muted" }),
                h("span", { class: "grow" },
                    i.email || "link invite",
                    " ",
                    h("span", { class: "tag" }, i.role)),
                h(CopyButton, { text: location.origin + "/invite/" + i.token, size: "sm", label: "Copy link" }),
                h(IconButton, { icon: "x", title: "Revoke", onClick: async () => { if (await act(() => del(`/orgs/${org}/invites/${i.id}`)))
                        r.reload(); } })))))) : null));
}
function QuotaEditor({ q, set, disabled }) {
    const f = (k, label, hint) => (h(Field, { label: label, hint: hint },
        h(Input, { type: "number", value: q[k], onInput: (v) => set({ ...q, [k]: Number(v) }), disabled: disabled })));
    return (h("div", { class: "form-grid" },
        f("cpuMillis", "CPU (millicores)", "0 means unlimited"),
        f("memoryMi", "Memory (MiB)"),
        f("storageGi", "Storage (GiB)"),
        f("apps", "Apps"),
        f("databases", "Databases"),
        f("sandboxes", "Sandboxes")));
}
function Quotas({ o, reload, admin }) {
    const s = use(session);
    const [q, setQ] = useState({ ...o.quota });
    const [mq, setMq] = useState({ ...o.memberQuota });
    return (h(Fragment, null,
        h(Card, { class: "form-card" },
            h("div", null,
                h("div", { class: "panel-title" }, "Organization cap"),
                h("div", { class: "panel-desc" }, s.me?.superadmin ? "The ceiling for everything in this organization." : "Set by the instance admins.")),
            h(QuotaEditor, { q: q, set: setQ, disabled: !s.me?.superadmin }),
            s.me?.superadmin ? h("div", { class: "form-card-foot" },
                h(Button, { kind: "primary", onClick: async () => { if (await act(() => patch(`/orgs/${o.slug}`, { quota: q }), "Saved"))
                        reload(); } }, "Save cap")) : null),
        h(Card, { class: "form-card" },
            h("div", null,
                h("div", { class: "panel-title" }, "Per-member allowance"),
                h("div", { class: "panel-desc" }, "Each member and viewer can use up to this, inside the organization cap. Owners and admins are bound only by the cap.")),
            h(QuotaEditor, { q: mq, set: setMq, disabled: !admin }),
            admin ? h("div", { class: "form-card-foot" },
                h(Button, { kind: "primary", onClick: async () => { if (await act(() => patch(`/orgs/${o.slug}`, { memberQuota: mq }), "Saved"))
                        reload(); } }, "Save allowance")) : null)));
}
function SSO({ o, reload, admin }) {
    const [sso, setSso] = useState({ enabled: false, issuer: "", clientId: "", clientSecret: "", domain: "", defaultRole: "member", ...o.sso });
    const [scim, setScim] = useState(null);
    return (h(Fragment, null,
        h(Card, { class: "form-card" },
            h("div", null,
                h("div", { class: "panel-title" }, "Single sign-on (OpenID Connect)"),
                h("div", { class: "panel-desc" }, "People with an email at your domain sign in through your identity provider and join this organization automatically.")),
            h(Toggle, { checked: sso.enabled, onChange: (v) => setSso({ ...sso, enabled: v }), label: "Enable SSO", disabled: !admin }),
            h("div", { class: "form-grid" },
                h(Field, { label: "Issuer URL" },
                    h(Input, { value: sso.issuer, onInput: (v) => setSso({ ...sso, issuer: v }), placeholder: "https://accounts.google.com", mono: true, disabled: !admin })),
                h(Field, { label: "Email domain" },
                    h(Input, { value: sso.domain, onInput: (v) => setSso({ ...sso, domain: v }), placeholder: "hackclub.com", mono: true, disabled: !admin })),
                h(Field, { label: "Client ID" },
                    h(Input, { value: sso.clientId, onInput: (v) => setSso({ ...sso, clientId: v }), mono: true, disabled: !admin })),
                h(Field, { label: "Client secret" },
                    h(Input, { value: sso.clientSecret, onInput: (v) => setSso({ ...sso, clientSecret: v }), type: "password", mono: true, disabled: !admin })),
                h(Field, { label: "Role for new members" },
                    h(Select, { value: sso.defaultRole, onChange: (v) => setSso({ ...sso, defaultRole: v }), options: roles.filter((r) => r.value !== "owner"), disabled: !admin })),
                h(Field, { label: "Redirect URI", hint: "Register this with your identity provider." },
                    h(Input, { value: location.origin + "/api/auth/sso/callback", onInput: () => { }, disabled: true, mono: true }))),
            admin ? h("div", { class: "form-card-foot" },
                h(Button, { kind: "primary", onClick: async () => { if (await act(() => patch(`/orgs/${o.slug}`, { sso }), "SSO saved"))
                        reload(); } }, "Save")) : null),
        h(Card, { class: "form-card" },
            h("div", null,
                h("div", { class: "panel-title" }, "SCIM provisioning"),
                h("div", { class: "panel-desc" }, "Let your identity provider create and deactivate members. Deactivated people lose access immediately.")),
            h("div", { class: "row" },
                h(Pill, { status: o.scimConfigured ? "active" : "pending", label: o.scimConfigured ? "Token set" : "Not configured" }),
                h("span", { class: "grow" }),
                o.role === "owner" ? h(Button, { kind: "secondary", icon: "key", onClick: async () => {
                        if (o.scimConfigured && !(await confirm({ title: "Rotate the SCIM token?", body: "The old token stops working immediately.", confirm: "Rotate" })))
                            return;
                        const r = await act(() => post(`/orgs/${o.slug}/scim-token`));
                        if (r) {
                            setScim(r);
                            reload();
                        }
                    } }, o.scimConfigured ? "Rotate token" : "Generate token") : null),
            scim ? (h(Callout, { kind: "green", title: "Copy this now; it will not be shown again." },
                h("div", { class: "col", style: { gap: 8, marginTop: 6 } },
                    h(CodeBlock, { code: scim.baseUrl, label: "SCIM base URL" }),
                    h(CodeBlock, { code: scim.token, label: "Bearer token" })))) : null)));
}
function Audit({ org }) {
    const [q, setQ] = useState("");
    const r = useApi(`/orgs/${org}/audit?q=${encodeURIComponent(q)}`, [q]);
    return (h(Fragment, null,
        h("div", { class: "search-input", style: { maxWidth: 360 } },
            h(Icon, { name: "search", size: 15 }),
            h("input", { class: "input", placeholder: "Filter by action, person or target", value: q, onInput: (e) => setQ(e.target.value) })),
        !r.data ? h(Loading, null) : r.data.length === 0 ? h(Empty, { icon: "history", title: "Nothing recorded" }) : (h(Table, { head: ["When", "Who", "Action", "Target", ""] }, r.data.map((e) => (h("tr", { key: e.id },
            h("td", { class: "nowrap", title: dateTime(e.createdAt) }, timeAgo(e.createdAt)),
            h("td", { class: "mono" }, e.actor),
            h("td", null,
                h("code", null, e.action)),
            h("td", { class: "mono muted truncate", style: { maxWidth: 240 } }, e.target),
            h("td", { class: "muted", style: { fontSize: 12 } }, e.meta ? Object.entries(e.meta).map(([k, v]) => `${k}=${v}`).join(" ") : ""))))))));
}
function OrgDanger({ o }) {
    if (o.role !== "owner")
        return h(Callout, { kind: "info" }, "Only owners can delete the organization.");
    return (h(Card, { class: "form-card danger-zone" },
        h("div", null,
            h("div", { class: "panel-title" }, "Delete organization"),
            h("div", { class: "panel-desc" },
                "Deletes every project, app, database, job and sandbox in ",
                o.name,
                ", and every namespace they live in.")),
        h("div", { class: "form-card-foot" },
            h(Button, { kind: "danger", icon: "trash", onClick: async () => {
                    if (await confirm({ title: `Delete ${o.name}?`, body: "This cannot be undone.", danger: true, confirm: "Delete organization", typeToConfirm: o.slug })) {
                        if (await act(() => del(`/orgs/${o.slug}`), "Deleted")) {
                            await loadSession();
                            navigate("/");
                        }
                    }
                } }, "Delete organization"))));
}
export function AccountPage() {
    const s = use(session);
    const [tab, setTab] = useTab(query().get("setup") ? "security" : "profile");
    const me = s.me;
    return (h(SettingsLayout, { title: "Account", sub: `@${me.username}`, tab: tab, setTab: setTab, tabs: [
            { id: "profile", label: "Profile", icon: "user" },
            { id: "security", label: "Passkeys & password", icon: "fingerprint" },
            { id: "tokens", label: "API tokens", icon: "key" },
            { id: "ssh", label: "SSH identities", icon: "terminal" },
            { id: "github", label: "GitHub", icon: "github" },
            { id: "mcp", label: "CLI & MCP", icon: "bot" },
        ] },
        query().get("error") ? h(Callout, { kind: "red" }, query().get("error")) : null,
        tab === "profile" ? h(Profile, null) : null,
        tab === "security" ? h(Security, null) : null,
        tab === "tokens" ? h(Tokens, null) : null,
        tab === "ssh" ? h(SSHKeys, null) : null,
        tab === "github" ? h(GitHubLink, null) : null,
        tab === "mcp" ? h(CliMcp, null) : null));
}
function Profile() {
    const s = use(session);
    const me = s.me;
    const [name, setName] = useState(me.name);
    const [email, setEmail] = useState(me.email || "");
    return (h(Card, { class: "form-card" },
        h("div", { class: "row" },
            h(Avatar, { seed: me.avatarSeed, size: 56 }),
            h("div", { class: "grow" },
                h("div", { style: { fontWeight: 600, fontSize: 16 } }, me.name),
                h("div", { class: "muted mono" },
                    "@",
                    me.username)),
            h(Button, { kind: "secondary", icon: "sparkles", onClick: async () => { await act(() => patch("/me", { avatarSeed: Math.random().toString(16).slice(2, 10) })); loadSession(); } }, "New critter")),
        h("div", { class: "form-grid" },
            h(Field, { label: "Display name" },
                h(Input, { value: name, onInput: setName })),
            h(Field, { label: "Email" },
                h(Input, { value: email, onInput: setEmail, type: "email" }))),
        h("div", { class: "form-card-foot" },
            h(Button, { kind: "primary", onClick: async () => { if (await act(() => patch("/me", { name, email }), "Saved"))
                    loadSession(); } }, "Save"))));
}
function Security() {
    const s = use(session);
    const me = s.me;
    const [cur, setCur] = useState("");
    const [nw, setNw] = useState("");
    const [pkName, setPkName] = useState("");
    return (h(Fragment, null,
        s.auth?.needsCredential ? h(Callout, { kind: "red", title: "Add a way back in." }, "The claim link was single use. Until you add a passkey or password, a cleared cookie jar leaves nothing to sign in with.") : null,
        h(Card, { class: "form-card" },
            h("div", null,
                h("div", { class: "panel-title" }, "Passkeys"),
                h("div", { class: "panel-desc" }, "Sign in with Touch ID, Windows Hello, a phone or a security key. Passkeys need HTTPS (or localhost).")),
            me.passkeys.map((k) => (h("div", { key: k.id, class: "row", style: { padding: "6px 0" } },
                h(Icon, { name: "fingerprint", class: "muted" }),
                h("span", { class: "grow" },
                    k.name,
                    " ",
                    h("span", { class: "muted", style: { fontSize: 12.5 } },
                        "added ",
                        timeAgo(k.createdAt))),
                h(IconButton, { icon: "trash", title: "Remove passkey", onClick: async () => { if (await confirm({ title: `Remove ${k.name}?`, danger: true, confirm: "Remove" })) {
                        await act(() => del(`/me/passkeys/${k.id}`), "Removed");
                        loadSession();
                    } } })))),
            passkeysSupported() ? (h("div", { class: "row" },
                h(Input, { value: pkName, onInput: setPkName, placeholder: "MacBook Touch ID" }),
                h(Button, { kind: "primary", icon: "plus", onClick: async () => {
                        try {
                            await registerPasskey(pkName || "Passkey");
                            toast("Passkey added", "ok");
                            setPkName("");
                            loadSession();
                        }
                        catch (e) {
                            if (e.name !== "NotAllowedError")
                                toast(e.message, "error");
                        }
                    } }, "Add passkey"))) : h(Callout, { kind: "amber" }, "This page is not served over HTTPS, so the browser does not offer passkeys. Use a password here (typical for LAN installs).")),
        h(Card, { class: "form-card" },
            h("div", { class: "panel-title" }, me.hasPassword ? "Change password" : "Set a password"),
            h("div", { class: "form-grid" },
                me.hasPassword ? h(Field, { label: "Current password" },
                    h(Input, { value: cur, onInput: setCur, type: "password", autocomplete: "current-password" })) : null,
                h(Field, { label: "New password", hint: "At least 10 characters." },
                    h(Input, { value: nw, onInput: setNw, type: "password", autocomplete: "new-password" }))),
            h("div", { class: "form-card-foot" },
                h(Button, { kind: "primary", disabled: nw.length < 10, onClick: async () => { if (await act(() => post("/me/password", { current: cur, new: nw }), "Password saved")) {
                        setCur("");
                        setNw("");
                        loadSession();
                    } } }, "Save password")))));
}
function Tokens() {
    const r = useApi("/me/tokens");
    const [name, setName] = useState("");
    const [fresh, setFresh] = useState(null);
    return (h(Fragment, null,
        h(Card, { class: "form-card" },
            h("div", null,
                h("div", { class: "panel-title" }, "Personal API tokens"),
                h("div", { class: "panel-desc" }, "For the CLI, scripts, CI and agents over MCP. A token can do everything you can.")),
            h("div", { class: "row" },
                h(Input, { value: name, onInput: setName, placeholder: "laptop cli" }),
                h(Button, { kind: "primary", icon: "plus", onClick: async () => { const t = await act(() => post("/me/tokens", { name })); if (t) {
                        setFresh(t.token);
                        setName("");
                        r.reload();
                    } } }, "Create token")),
            fresh ? h(Callout, { kind: "green", title: "Copy it now; it is shown once." },
                h(CodeBlock, { code: fresh })) : null),
        !r.data ? h(Loading, null) : r.data.length === 0 ? null : (h(Table, { head: ["Name", "Token", "Created", "Last used", ""] }, r.data.map((t) => (h("tr", { key: t.id },
            h("td", null, t.name),
            h("td", { class: "mono muted" },
                t.prefix,
                "\u2026"),
            h("td", null, timeAgo(t.createdAt)),
            h("td", { class: "muted" }, t.lastUsed ? timeAgo(t.lastUsed) : "never"),
            h("td", { style: { textAlign: "right" } },
                h(IconButton, { icon: "trash", title: "Revoke", onClick: async () => { if (await confirm({ title: `Revoke ${t.name}?`, danger: true, confirm: "Revoke" })) {
                        await act(() => del(`/me/tokens/${t.id}`), "Revoked");
                        r.reload();
                    } } })))))))));
}
function SSHKeys() {
    const s = use(session);
    const [key, setKey] = useState("");
    const [name, setName] = useState("");
    return (h(Card, { class: "form-card" },
        h("div", null,
            h("div", { class: "panel-title" }, "SSH identities"),
            h("div", { class: "panel-desc" }, "Attach a public key to reach Greenhouse sandboxes from your own terminal or point a local editor at them.")),
        s.me.sshKeys.map((k) => (h("div", { key: k.id, class: "row", style: { padding: "6px 0" } },
            h(Icon, { name: "key", class: "muted" }),
            h("span", { class: "grow" },
                k.name,
                " ",
                h("span", { class: "mono muted", style: { fontSize: 12 } }, k.fingerprint)),
            h(IconButton, { icon: "trash", title: "Remove", onClick: async () => { await act(() => del(`/me/ssh-keys/${k.id}`), "Removed"); loadSession(); } })))),
        h(Field, { label: "Name" },
            h(Input, { value: name, onInput: setName, placeholder: "laptop" })),
        h(Field, { label: "Public key" },
            h(Textarea, { value: key, onInput: setKey, rows: 3, mono: true, placeholder: "ssh-ed25519 AAAA\u2026 you@laptop" })),
        h("div", { class: "form-card-foot" },
            h(Button, { kind: "primary", disabled: !key.trim(), onClick: async () => { if (await act(() => post("/me/ssh-keys", { name, publicKey: key }), "Key added")) {
                    setKey("");
                    setName("");
                    loadSession();
                } } }, "Add key"))));
}
function GitHubLink() {
    const r = useApi("/github/status");
    if (!r.data)
        return h(Loading, null);
    return (h(Card, { class: "form-card" },
        h("div", null,
            h("div", { class: "panel-title" }, "GitHub"),
            h("div", { class: "panel-desc" }, "Link your account to deploy repositories. Wack Club Orchard only ever sees the repositories you grant the app.")),
        !r.data.configured ? h(Callout, { kind: "amber" }, "The instance has no GitHub App yet. An instance admin creates it under Instance admin \u2192 GitHub.") : (h("div", { class: "row" },
            r.data.login ? h(Fragment, null,
                h(Icon, { name: "github" }),
                " ",
                h("span", { class: "grow" },
                    "Linked as ",
                    h("strong", null,
                        "@",
                        r.data.login))) : h("span", { class: "grow muted" }, "Not linked."),
            r.data.installUrl ? h(Button, { kind: "ghost", href: r.data.installUrl, target: "_blank", iconRight: "external" }, "Grant repositories") : null,
            h(Button, { kind: r.data.login ? "secondary" : "primary", icon: "github", href: "/api/github/connect?next=/account?tab=github" }, r.data.login ? "Relink" : "Link GitHub")))));
}
function CliMcp() {
    const host = location.origin;
    return (h(Fragment, null,
        h(Card, { class: "form-card" },
            h("div", { class: "panel-title" }, "The wackcluborchard CLI"),
            h(CodeBlock, { label: "sign in", code: `wackcluborchard login ${host}\nwackcluborchard apps\nwackcluborchard deploy api\nwackcluborchard logs api -f\nwackcluborchard run warm-site` })),
        h(Card, { class: "form-card" },
            h("div", null,
                h("div", { class: "panel-title" }, "MCP for agents"),
                h("div", { class: "panel-desc" }, "Agents like Claude can list, deploy, scale, roll back, read logs, query databases and run jobs. Authenticate with a personal API token.")),
            h(CodeBlock, { label: "claude code", code: `claude mcp add --transport http wackcluborchard ${host}/mcp \\\n  --header "Authorization: Bearer wackclubwackcluborchard_…"` }),
            h(CodeBlock, { label: "mcp.json", code: JSON.stringify({ mcpServers: { wackcluborchard: { type: "http", url: host + "/mcp", headers: { Authorization: "Bearer wackclubwackcluborchard_…" } } } }, null, 2) }))));
}
export function AdminPage() {
    const [tab, setTab] = useTab("settings");
    return (h(SettingsLayout, { title: "Instance admin", sub: "Superadmin only: the platform itself.", tab: tab, setTab: setTab, tabs: [
            { id: "settings", label: "Settings", icon: "settings" },
            { id: "github", label: "GitHub App", icon: "github" },
            { id: "orgs", label: "Organizations", icon: "building" },
            { id: "users", label: "Users", icon: "users" },
            { id: "nodes", label: "Nodes & pools", icon: "server" },
            { id: "networking", label: "Networking", icon: "globe" },
            { id: "audit", label: "Audit", icon: "history" },
        ] },
        query().get("error") ? h(Callout, { kind: "red" }, query().get("error")) : null,
        tab === "settings" ? h(AdminSettings, null) : null,
        tab === "github" ? h(AdminGitHub, null) : null,
        tab === "orgs" ? h(AdminOrgs, null) : null,
        tab === "users" ? h(AdminUsers, null) : null,
        tab === "nodes" ? h(AdminNodes, null) : null,
        tab === "networking" ? h(AdminNetworking, null) : null,
        tab === "audit" ? h(AdminAudit, null) : null));
}
function AdminSettings() {
    const r = useApi("/admin/settings");
    const [st, setSt] = useState(null);
    useEffect(() => {
        if (r.data)
            setSt({ ...r.data.settings });
    }, [r.data]);
    if (!st)
        return h(Loading, null);
    const f = (k, label, hint, ph) => h(Field, { label: label, hint: hint },
        h(Input, { value: st[k] || "", onInput: (v) => setSt({ ...st, [k]: v }), mono: true, placeholder: ph }));
    const save = async () => {
        if (await act(() => patch("/admin/settings", st), "Settings saved"))
            r.reload();
    };
    return (h(Fragment, null,
        h(Card, { class: "form-card" },
            h("div", { class: "row" },
                h("div", { class: "grow" },
                    h("div", { class: "panel-title" }, "Instance"),
                    h("div", { class: "panel-desc" },
                        "Runtime: ",
                        h("strong", null, r.data.runtime),
                        " \u00B7 version ",
                        r.data.version,
                        " \u00B7 builds ",
                        r.data.builds.running,
                        "/",
                        r.data.builds.slots,
                        " busy, ",
                        r.data.builds.waiting,
                        " queued"))),
            h("div", { class: "form-grid" },
                f("instanceName", "Instance name"),
                h(Field, { label: "Signups" },
                    h(Select, { value: st.signupMode, onChange: (v) => setSt({ ...st, signupMode: v }), options: [{ value: "open", label: "Open to anyone" }, { value: "invite", label: "Invite only" }, { value: "closed", label: "Closed" }] })),
                f("domain", "Dashboard domain"),
                f("appDomain", "App domain", "Apps get <name>.<app domain>."),
                f("ingressCname", "Custom domain CNAME target"),
                f("publicIp", "Default public IP", "Inherited by every organization for TCP/UDP and public databases."),
                f("publicDbDomain", "Public database domain", "Optional wildcard for exposed databases."),
                h(Field, { label: "Build slots", hint: "Concurrent builds. Extra builds queue." },
                    h(Input, { type: "number", value: st.buildSlots, onInput: (v) => setSt({ ...st, buildSlots: Number(v) }) }))),
            h("div", { class: "form-card-foot" },
                h(Button, { kind: "primary", onClick: save }, "Save"))),
        h(Card, { class: "form-card" },
            h("div", { class: "panel-title" }, "MCP endpoint"),
            h(Toggle, { checked: st.mcpEnabled, onChange: (v) => setSt({ ...st, mcpEnabled: v }), label: "Enable /mcp", hint: `Agents connect at ${st.mcpDomain ? "https://" + st.mcpDomain : location.origin + "/mcp"} with a personal token.` }),
            f("mcpDomain", "MCP hostname", "Needs a DNS record before its certificate can be issued."),
            h("div", { class: "form-card-foot" },
                h(Button, { kind: "primary", onClick: save }, "Save"))),
        h(Card, { class: "form-card" },
            h("div", null,
                h("div", { class: "panel-title" }, "Isolation"),
                h("div", { class: "panel-desc" }, "Deployments are network-isolated by default but share the host kernel. Turn these on once the node runtimes are installed.")),
            h(Toggle, { checked: st.tenantSandbox, onChange: (v) => setSt({ ...st, tenantSandbox: v }), label: "gVisor for new tenant apps", hint: "RuntimeClass gvisor." }),
            h(Toggle, { checked: st.builderSandbox, onChange: (v) => setSt({ ...st, builderSandbox: v }), label: "Kata VMs for image builds", hint: "Without it, builds run with host-level privileges: fine when the builder owns the box." }),
            h("div", { class: "form-card-foot" },
                h(Button, { kind: "primary", onClick: save }, "Save"))),
        h(Card, { class: "form-card" },
            h("div", null,
                h("div", { class: "panel-title" }, "Greenhouse agent"),
                h("div", { class: "panel-desc" }, "An Anthropic API key lets sandboxes run Claude against their workspace.")),
            h(Field, { label: "Anthropic API key" },
                h(Input, { value: st.anthropicKey || "", onInput: (v) => setSt({ ...st, anthropicKey: v }), type: "password", mono: true, placeholder: "sk-ant-\u2026" })),
            h("div", { class: "form-card-foot" },
                h(Button, { kind: "primary", onClick: save }, "Save")))));
}
function AdminGitHub() {
    const r = useApi("/github/status");
    const [ghOrg, setGhOrg] = useState("");
    if (!r.data)
        return h(Loading, null);
    const create = async () => {
        const m = await act(() => post("/github/manifest", { org: ghOrg }));
        if (!m)
            return;
        const form = document.createElement("form");
        form.method = "post";
        form.action = m.action;
        const input = document.createElement("input");
        input.type = "hidden";
        input.name = "manifest";
        input.value = m.manifest;
        form.appendChild(input);
        document.body.appendChild(form);
        form.submit();
    };
    return (h(Card, { class: "form-card" },
        h("div", null,
            h("div", { class: "panel-title" }, "GitHub App"),
            h("div", { class: "panel-desc" }, "Wack Club Orchard generates the app manifest, GitHub creates the app, and the credentials come back automatically. Then each person links their own account.")),
        r.data.configured ? (h(Callout, { kind: "green", title: `Connected as ${r.data.slug}` },
            "Pushes to tracked branches deploy automatically. ",
            h("a", { href: r.data.installUrl, target: "_blank", rel: "noopener" }, "Install on more accounts \u2197"))) : null,
        h(Field, { label: "GitHub organization", hint: "Optional. Leave empty to create the app under your personal account." },
            h(Input, { value: ghOrg, onInput: setGhOrg, placeholder: "hackclub", mono: true })),
        h("div", { class: "form-card-foot" },
            h(Button, { kind: "primary", icon: "github", onClick: create }, r.data.configured ? "Create a new GitHub App" : "Create GitHub App"))));
}
function AdminOrgs() {
    const r = useApi("/admin/orgs");
    if (!r.data)
        return h(Loading, null);
    return (h(Table, { head: ["Organization", "Members", "Projects", "CPU", "Memory", "Pool", ""] }, r.data.map((o) => (h("tr", { key: o.id },
        h("td", null,
            h("strong", null, o.name),
            " ",
            h("span", { class: "muted mono", style: { fontSize: 12 } }, o.slug)),
        h("td", null, o.members),
        h("td", null, o.projects),
        h("td", { class: "mono", style: { fontSize: 12.5 } },
            cpu(o.used.cpuMillis),
            " / ",
            o.quota.cpuMillis ? cpu(o.quota.cpuMillis) : "∞"),
        h("td", { class: "mono", style: { fontSize: 12.5 } },
            mem(o.used.memoryMi),
            " / ",
            o.quota.memoryMi ? mem(o.quota.memoryMi) : "∞"),
        h("td", null, o.pool ? h(Tag, null, o.pool) : h("span", { class: "muted" }, "any")),
        h("td", { style: { textAlign: "right" } },
            h(Button, { size: "sm", kind: "secondary", href: `/o/${o.slug}/settings?tab=quotas` }, "Quotas")))))));
}
function AdminUsers() {
    const r = useApi("/admin/users");
    const s = use(session);
    if (!r.data)
        return h(Loading, null);
    return (h(Table, { head: ["User", "Organizations", "Sign-in", "Joined", ""] }, r.data.map((u) => (h("tr", { key: u.id, style: u.disabled ? { opacity: 0.55 } : undefined },
        h("td", null,
            h("div", { class: "row" },
                h(Avatar, { seed: u.avatarSeed, size: 28 }),
                h("div", null,
                    h("strong", null, u.name),
                    " ",
                    u.superadmin ? h(Tag, null,
                        h(Icon, { name: "shield", size: 11 }),
                        " superadmin") : null,
                    h("div", { class: "muted mono", style: { fontSize: 12 } },
                        "@",
                        u.username,
                        u.email ? " · " + u.email : "")))),
        h("td", { class: "muted", style: { fontSize: 12.5 } }, (u.orgs || []).join(", ")),
        h("td", { class: "muted", style: { fontSize: 12.5 } }, [u.hasPassword && "password", u.passkeys.length && plural(u.passkeys.length, "passkey"), u.githubLogin && "github"].filter(Boolean).join(", ") || "sso"),
        h("td", { class: "muted" }, timeAgo(u.createdAt)),
        h("td", { style: { textAlign: "right" } }, u.id !== s.me?.id ? (h("div", { class: "row", style: { justifyContent: "flex-end", gap: 6 } },
            h(Button, { size: "sm", kind: "ghost", onClick: async () => { if (await act(() => patch(`/admin/users/${u.id}`, { superadmin: !u.superadmin }), "Updated"))
                    r.reload(); } }, u.superadmin ? "Demote" : "Make superadmin"),
            h(Button, { size: "sm", kind: "secondary", onClick: async () => { if (await act(() => patch(`/admin/users/${u.id}`, { disabled: !u.disabled }), u.disabled ? "Enabled" : "Disabled and signed out"))
                    r.reload(); } }, u.disabled ? "Enable" : "Disable"))) : null))))));
}
function AdminNodes() {
    const r = useApi("/admin/nodes");
    const orgs = useApi("/admin/orgs");
    if (r.error)
        return h(ErrorBox, { error: r.error });
    if (!r.data)
        return h(Loading, null);
    const editPool = (p) => openModal((close) => h(PoolEditor, { pool: p, nodes: r.data.nodes, orgs: orgs.data || [], close: close, done: r.reload }));
    return (h(Fragment, null,
        h("div", { class: "grid-2" }, r.data.nodes.map((n) => (h(Card, { key: n.name, class: "node-card" },
            h("div", { class: "row" },
                h(Icon, { name: "server" }),
                h("strong", { class: "grow mono" }, n.name),
                h(Pill, { status: n.ready ? "ready" : "failed", label: n.ready ? "Ready" : "NotReady" })),
            h("div", { class: "row row-wrap", style: { gap: 5 } },
                (n.roles || []).map((x) => h(Tag, { key: x }, x)),
                h(Tag, { mono: true }, n.arch),
                h(Tag, { mono: true }, n.kubelet),
                n.labels?.["wackcluborchard.dev/pool"] ? h(Tag, null,
                    h(Icon, { name: "layers", size: 11 }),
                    " ",
                    n.labels["wackcluborchard.dev/pool"]) : null),
            h(Meter, { label: "CPU requested", used: n.requestedCpuMillis, cap: n.allocatableCpuMillis, format: cpu }),
            h(Meter, { label: "Memory requested", used: n.requestedMemoryMi, cap: n.allocatableMemoryMi, format: mem }),
            h("div", { class: "muted", style: { fontSize: 12.5 } },
                n.pods,
                " pods \u00B7 allocatable is smaller than capacity (",
                cpu(n.cpuMillis),
                ", ",
                mem(n.memoryMi),
                ") because the system takes a share.",
                n.taints?.length ? " Taints: " + n.taints.join(", ") : ""))))),
        h(Card, { class: "form-card" },
            h("div", { class: "row" },
                h("div", { class: "grow" },
                    h("div", { class: "panel-title" }, "Pools"),
                    h("div", { class: "panel-desc" }, "Group nodes with a purpose and assign organizations to them. Pools use ordinary taints and node selectors, so kubectl explains placement the usual way.")),
                h(Button, { kind: "primary", icon: "plus", onClick: () => editPool() }, "New pool")),
            r.data.pools.length === 0 ? h("div", { class: "muted" }, "No pools: every organization schedules anywhere.") : r.data.pools.map((p) => (h("div", { key: p.id, class: "row", style: { padding: "8px 0", borderTop: "1px solid var(--border)" } },
                h(Icon, { name: "layers", class: "muted" }),
                h("div", { class: "grow" },
                    h("strong", null, p.name),
                    " ",
                    h("span", { class: "muted" }, p.description),
                    h("div", { class: "muted", style: { fontSize: 12.5 } },
                        plural(p.nodes.length, "node"),
                        " \u00B7 ",
                        plural(p.orgIds.length, "organization"),
                        p.taint ? " · tainted" : "")),
                h(Button, { size: "sm", kind: "secondary", onClick: () => editPool(p) }, "Edit"),
                h(IconButton, { icon: "trash", title: "Delete pool", onClick: async () => { if (await confirm({ title: `Delete pool ${p.name}?`, body: "Its nodes are untainted and its organizations schedule anywhere again.", danger: true, confirm: "Delete" })) {
                        await act(() => del(`/admin/pools/${p.id}`), "Deleted");
                        r.reload();
                    } } })))))));
}
function PoolEditor({ pool, nodes, orgs, close, done }) {
    const [p, setP] = useState(pool ? { ...pool } : { name: "", description: "", nodes: [], orgIds: [], taint: true });
    const toggle = (k, v) => setP({ ...p, [k]: p[k].includes(v) ? p[k].filter((x) => x !== v) : [...p[k], v] });
    return (h(Fragment, null,
        h(ModalHeader, { title: pool ? `Edit ${pool.name}` : "New pool", onClose: close, icon: "layers" }),
        h("div", { class: "modal-body" },
            h("div", { class: "form-grid" },
                h(Field, { label: "Name" },
                    h(Input, { value: p.name, onInput: (v) => setP({ ...p, name: v }), mono: true })),
                h(Field, { label: "Purpose" },
                    h(Input, { value: p.description, onInput: (v) => setP({ ...p, description: v }), placeholder: "GPU box Sam paid for" }))),
            h(Field, { label: "Nodes" }, nodes.map((n) => h("label", { key: n.name, class: "row" },
                h("input", { type: "checkbox", checked: p.nodes.includes(n.name), onChange: () => toggle("nodes", n.name) }),
                " ",
                h("span", { class: "mono" }, n.name)))),
            h(Field, { label: "Organizations" }, orgs.map((o) => h("label", { key: o.id, class: "row" },
                h("input", { type: "checkbox", checked: p.orgIds.includes(o.id), onChange: () => toggle("orgIds", o.id) }),
                " ",
                o.name))),
            h(Toggle, { checked: p.taint, onChange: (v) => setP({ ...p, taint: v }), label: "Taint the nodes", hint: "Keep everyone else off them. Without a taint, the pool only attracts its organizations." })),
        h("div", { class: "modal-foot" },
            h(Button, { kind: "ghost", onClick: close }, "Cancel"),
            h(Button, { kind: "primary", disabled: !p.name, onClick: async () => { if (await act(() => post("/admin/pools", p), "Pool saved")) {
                    close();
                    done();
                } } }, "Save"))));
}
function AdminNetworking() {
    const r = useApi("/admin/networking");
    if (r.error)
        return h(ErrorBox, { error: r.error });
    if (!r.data)
        return h(Loading, { label: "Resolving hostnames\u2026" });
    return (h(Fragment, null,
        h(Card, null,
            h("div", { class: "panel-title" }, "Traffic"),
            h("div", { class: "panel-desc" },
                "Mode ",
                h("strong", null, r.data.mode),
                r.data.publicIp ? h(Fragment, null,
                    " \u00B7 public IP ",
                    h("code", null, r.data.publicIp)) : null,
                ". In public mode certificates come from Let's Encrypt over HTTP-01, so every name must point here before its certificate can be issued.")),
        h(Table, { head: ["Name", "Used for", "Resolves to", ""] }, r.data.checks.map((c) => (h("tr", { key: c.name },
            h("td", { class: "mono" }, c.name),
            h("td", { class: "muted" }, c.purpose),
            h("td", { class: "mono muted", style: { fontSize: 12.5 } }, c.records?.join(", ") || c.error),
            h("td", null,
                h(Pill, { status: c.ok ? "active" : "failed", label: c.ok ? "OK" : "Check DNS" }))))))));
}
function AdminAudit() {
    const r = useApi("/admin/audit");
    if (!r.data)
        return h(Loading, null);
    return (h(Table, { head: ["When", "Who", "Action", "Target"] }, r.data.map((e) => (h("tr", { key: e.id },
        h("td", { class: "nowrap" }, timeAgo(e.createdAt)),
        h("td", { class: "mono" }, e.actor),
        h("td", null,
            h("code", null, e.action)),
        h("td", { class: "mono muted" }, e.target))))));
}
export { get, put, Link };
