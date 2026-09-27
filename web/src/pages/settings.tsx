import { h, Fragment, useEffect, useState } from "../lib/sprout.js";
import { Link, navigate, query, setQuery } from "../lib/router.js";
import { useApi, post, put, patch, del, act, get } from "../lib/api.js";
import { cpu, mem, timeAgo, dateTime, plural } from "../lib/format.js";
import { session, use, loadSession, toast } from "../lib/state.js";
import { Button, Loading, ErrorBox, Empty, Pill, Card, Field, Input, Select, Toggle, confirm, Callout, Table, IconButton, cx, openModal, ModalHeader, Tag, CopyButton, CodeBlock, Textarea, Meter } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";
import { Avatar } from "../ui/art.js";
import { passkeysSupported, registerPasskey } from "./auth.js";

function SettingsLayout<T extends string>({ title, sub, tabs, tab, setTab, children }: { title: string; sub?: string; tabs: { id: T; label: string; icon: string }[]; tab: T; setTab: (t: T) => void; children: any }) {
  return (
    <div class="page" style={{ maxWidth: 1080 }}>
      <div class="page-head"><div><h1 class="page-title">{title}</h1>{sub ? <div class="page-sub">{sub}</div> : null}</div></div>
      <div class="settings">
        <nav class="settings-nav">
          {tabs.map((t) => <button key={t.id} type="button" class={cx(t.id === tab && "active")} onClick={() => setTab(t.id)}><Icon name={t.icon} size={15} /> {t.label}</button>)}
        </nav>
        <div class="col gap-lg">{children}</div>
      </div>
    </div>
  );
}

function useTab<T extends string>(def: T): [T, (t: T) => void] {
  const [t, set] = useState<T>((query().get("tab") as T) || def);
  return [t, (x: T) => { set(x); setQuery("tab", x === def ? null : x); }];
}

// ---- organization ----

type OrgTab = "general" | "members" | "quotas" | "sso" | "audit" | "danger";

export function MembersPage({ org }: { org: string }) {
  const o = useApi<any>(`/orgs/${org}`);
  if (!o.data) return <div class="page"><Loading /></div>;
  return (
    <div class="page" style={{ maxWidth: 980 }}>
      <div class="page-head"><div><h1 class="page-title">Members</h1><div class="page-sub">People in {o.data.name}. Project membership decides which apps each person can reach.</div></div></div>
      <Members org={org} o={o.data} />
    </div>
  );
}

export function OrgSettingsPage({ org }: { org: string }) {
  const o = useApi<any>(`/orgs/${org}`);
  const [tab, setTab] = useTab<OrgTab>("general");
  if (o.error) return <div class="page"><ErrorBox error={o.error} /></div>;
  if (!o.data) return <div class="page"><Loading /></div>;
  const admin = o.data.role === "owner" || o.data.role === "admin";
  return (
    <SettingsLayout
      title="Organization settings"
      sub={o.data.name}
      tab={tab}
      setTab={setTab}
      tabs={[
        { id: "general", label: "General", icon: "settings" },
        { id: "members", label: "Members", icon: "users" },
        { id: "quotas", label: "Quotas", icon: "activity" },
        { id: "sso", label: "SSO & SCIM", icon: "key" },
        { id: "audit", label: "Audit log", icon: "history" },
        { id: "danger", label: "Danger zone", icon: "alert" },
      ]}
    >
      {!admin && tab !== "members" ? <Callout kind="info">Only owners and admins can change organization settings.</Callout> : null}
      {tab === "general" ? <OrgGeneral o={o.data} reload={o.reload} admin={admin} /> : null}
      {tab === "members" ? <Members org={org} o={o.data} /> : null}
      {tab === "quotas" ? <Quotas o={o.data} reload={o.reload} admin={admin} /> : null}
      {tab === "sso" ? <SSO o={o.data} reload={o.reload} admin={admin} /> : null}
      {tab === "audit" ? (admin ? <Audit org={org} /> : null) : null}
      {tab === "danger" ? <OrgDanger o={o.data} /> : null}
    </SettingsLayout>
  );
}

function OrgGeneral({ o, reload, admin }: { o: any; reload: () => void; admin: boolean }) {
  const [name, setName] = useState(o.name);
  const [ip, setIp] = useState(o.publicIp || "");
  const [def, setDef] = useState({ ...o.defaults });
  return (
    <>
      <Card class="form-card">
        <div class="panel-title">General</div>
        <div class="form-grid">
          <Field label="Name"><Input value={name} onInput={setName} disabled={!admin} /></Field>
          <Field label="Slug" hint="Used in URLs and namespaces."><Input value={o.slug} onInput={() => {}} disabled mono /></Field>
        </div>
        {admin ? <div class="form-card-foot"><Button kind="primary" onClick={async () => { if (await act(() => patch(`/orgs/${o.slug}`, { name }), "Saved")) { reload(); loadSession(); } }}>Save</Button></div> : null}
      </Card>
      <Card class="form-card">
        <div>
          <div class="panel-title">Shared public IP</div>
          <div class="panel-desc">Raw TCP/UDP ports and public databases are published on this address. Leave empty to inherit the instance default.</div>
        </div>
        <Field label="Public IP"><Input value={ip} onInput={setIp} placeholder="203.0.113.10" mono disabled={!admin} /></Field>
        {admin ? <div class="form-card-foot"><Button kind="primary" onClick={() => act(() => patch(`/orgs/${o.slug}`, { publicIp: ip }), "Saved")}>Save</Button></div> : null}
      </Card>
      <Card class="form-card">
        <div>
          <div class="panel-title">Default app size</div>
          <div class="panel-desc">Applied when an app is created without explicit resources.</div>
        </div>
        <div class="form-grid">
          <Field label="CPU (millicores)"><Input type="number" value={def.cpuMillis} onInput={(v) => setDef({ ...def, cpuMillis: Number(v) })} disabled={!admin} /></Field>
          <Field label="Memory (MiB)"><Input type="number" value={def.memoryMi} onInput={(v) => setDef({ ...def, memoryMi: Number(v) })} disabled={!admin} /></Field>
        </div>
        {admin ? <div class="form-card-foot"><Button kind="primary" onClick={() => act(() => patch(`/orgs/${o.slug}`, { defaults: def }), "Saved")}>Save</Button></div> : null}
      </Card>
    </>
  );
}

const roles = [
  { value: "owner", label: "Owner" },
  { value: "admin", label: "Admin" },
  { value: "member", label: "Member" },
  { value: "viewer", label: "Viewer" },
];

function Members({ org, o }: { org: string; o: any }) {
  const r = useApi<any>(`/orgs/${org}/members`);
  const s = use(session);
  const admin = o.role === "owner" || o.role === "admin";
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("member");
  const [link, setLink] = useState("");
  if (!r.data) return <Loading />;
  return (
    <>
      {admin ? (
        <Card class="form-card">
          <div>
            <div class="panel-title">Invite people</div>
            <div class="panel-desc">Enter a username to add someone who already has an account, or an email to make an invite link.</div>
          </div>
          <div class="row">
            <Input value={email} onInput={setEmail} placeholder="username or someone@example.com" />
            <Select value={role} onChange={setRole} options={roles} class="role-select" />
            <Button kind="primary" icon="plus" disabled={!email} onClick={async () => {
              const isEmail = email.includes("@");
              const res = await act(() => post(`/orgs/${org}/members`, isEmail ? { email, role } : { username: email, role }), isEmail ? "Invite created" : "Added");
              if (res) {
                setEmail("");
                if (res.url) setLink(res.url);
                r.reload();
              }
            }}>Invite</Button>
          </div>
          {link ? <Callout kind="green" title="Share this invite link">It works once. <CodeBlock code={link} /></Callout> : null}
          <div class="muted" style={{ fontSize: 12.5 }}>Owners: everything, including deleting the organization. Admins: members, projects, quotas, settings. Members: resources in projects they belong to. Viewers: read-only.</div>
        </Card>
      ) : null}
      <Table head={["Person", "Role", "Projects", "Using", "Joined", ""]}>
        {r.data.members.map((m: any) => (
          <tr key={m.id}>
            <td>
              <div class="row">
                <Avatar seed={m.avatarSeed} size={30} />
                <div><div style={{ fontWeight: 600 }}>{m.name}{m.id === s.me?.id ? <span class="muted"> (you)</span> : null}</div><div class="muted mono" style={{ fontSize: 12 }}>@{m.username}{m.email ? " · " + m.email : ""}</div></div>
              </div>
            </td>
            <td>{admin && m.id !== s.me?.id ? <Select value={m.role} onChange={async (v) => { if (await act(() => patch(`/orgs/${org}/members/${m.id}`, { role: v }), "Role changed")) r.reload(); }} options={roles} class="role-select" /> : <span class="tag">{m.role}</span>}</td>
            <td>{m.projects}</td>
            <td class="mono muted" style={{ fontSize: 12.5 }}>{cpu(m.usage.cpuMillis)} · {mem(m.usage.memoryMi)}</td>
            <td class="muted">{timeAgo(m.joinedAt)}</td>
            <td style={{ textAlign: "right" }}>
              {(admin && m.id !== s.me?.id) || m.id === s.me?.id ? (
                <IconButton icon={m.id === s.me?.id ? "logout" : "x"} title={m.id === s.me?.id ? "Leave organization" : "Remove"} onClick={async () => {
                  if (await confirm({ title: m.id === s.me?.id ? `Leave ${o.name}?` : `Remove ${m.name}?`, body: "Their apps keep running; they lose access.", danger: true, confirm: m.id === s.me?.id ? "Leave" : "Remove" })) {
                    if (await act(() => del(`/orgs/${org}/members/${m.id}`), "Done")) {
                      if (m.id === s.me?.id) {
                        await loadSession();
                        navigate("/");
                      } else r.reload();
                    }
                  }
                }} />
              ) : null}
            </td>
          </tr>
        ))}
      </Table>
      {r.data.invites.length ? (
        <Card>
          <div class="panel-title" style={{ marginBottom: 8 }}>Pending invites</div>
          {r.data.invites.map((i: any) => (
            <div key={i.id} class="row" style={{ padding: "8px 0", borderTop: "1px solid var(--border)" }}>
              <Icon name="message" class="muted" />
              <span class="grow">{i.email || "link invite"} <span class="tag">{i.role}</span></span>
              <CopyButton text={location.origin + "/invite/" + i.token} size="sm" label="Copy link" />
              <IconButton icon="x" title="Revoke" onClick={async () => { if (await act(() => del(`/orgs/${org}/invites/${i.id}`))) r.reload(); }} />
            </div>
          ))}
        </Card>
      ) : null}
    </>
  );
}

function QuotaEditor({ q, set, disabled }: { q: any; set: (q: any) => void; disabled: boolean }) {
  const f = (k: string, label: string, hint?: string) => (
    <Field label={label} hint={hint}><Input type="number" value={q[k]} onInput={(v) => set({ ...q, [k]: Number(v) })} disabled={disabled} /></Field>
  );
  return (
    <div class="form-grid">
      {f("cpuMillis", "CPU (millicores)", "0 means unlimited")}
      {f("memoryMi", "Memory (MiB)")}
      {f("storageGi", "Storage (GiB)")}
      {f("apps", "Apps")}
      {f("databases", "Databases")}
      {f("sandboxes", "Sandboxes")}
    </div>
  );
}

function Quotas({ o, reload, admin }: { o: any; reload: () => void; admin: boolean }) {
  const s = use(session);
  const [q, setQ] = useState({ ...o.quota });
  const [mq, setMq] = useState({ ...o.memberQuota });
  return (
    <>
      <Card class="form-card">
        <div>
          <div class="panel-title">Organization cap</div>
          <div class="panel-desc">{s.me?.superadmin ? "The ceiling for everything in this organization." : "Set by the instance admins."}</div>
        </div>
        <QuotaEditor q={q} set={setQ} disabled={!s.me?.superadmin} />
        {s.me?.superadmin ? <div class="form-card-foot"><Button kind="primary" onClick={async () => { if (await act(() => patch(`/orgs/${o.slug}`, { quota: q }), "Saved")) reload(); }}>Save cap</Button></div> : null}
      </Card>
      <Card class="form-card">
        <div>
          <div class="panel-title">Per-member allowance</div>
          <div class="panel-desc">Each member and viewer can use up to this, inside the organization cap. Owners and admins are bound only by the cap.</div>
        </div>
        <QuotaEditor q={mq} set={setMq} disabled={!admin} />
        {admin ? <div class="form-card-foot"><Button kind="primary" onClick={async () => { if (await act(() => patch(`/orgs/${o.slug}`, { memberQuota: mq }), "Saved")) reload(); }}>Save allowance</Button></div> : null}
      </Card>
    </>
  );
}

function SSO({ o, reload, admin }: { o: any; reload: () => void; admin: boolean }) {
  const [sso, setSso] = useState({ enabled: false, issuer: "", clientId: "", clientSecret: "", domain: "", defaultRole: "member", ...o.sso });
  const [scim, setScim] = useState<any>(null);
  return (
    <>
      <Card class="form-card">
        <div>
          <div class="panel-title">Single sign-on (OpenID Connect)</div>
          <div class="panel-desc">People with an email at your domain sign in through your identity provider and join this organization automatically.</div>
        </div>
        <Toggle checked={sso.enabled} onChange={(v) => setSso({ ...sso, enabled: v })} label="Enable SSO" disabled={!admin} />
        <div class="form-grid">
          <Field label="Issuer URL"><Input value={sso.issuer} onInput={(v) => setSso({ ...sso, issuer: v })} placeholder="https://accounts.google.com" mono disabled={!admin} /></Field>
          <Field label="Email domain"><Input value={sso.domain} onInput={(v) => setSso({ ...sso, domain: v })} placeholder="hackclub.com" mono disabled={!admin} /></Field>
          <Field label="Client ID"><Input value={sso.clientId} onInput={(v) => setSso({ ...sso, clientId: v })} mono disabled={!admin} /></Field>
          <Field label="Client secret"><Input value={sso.clientSecret} onInput={(v) => setSso({ ...sso, clientSecret: v })} type="password" mono disabled={!admin} /></Field>
          <Field label="Role for new members"><Select value={sso.defaultRole} onChange={(v) => setSso({ ...sso, defaultRole: v })} options={roles.filter((r) => r.value !== "owner")} disabled={!admin} /></Field>
          <Field label="Redirect URI" hint="Register this with your identity provider."><Input value={location.origin + "/api/auth/sso/callback"} onInput={() => {}} disabled mono /></Field>
        </div>
        {admin ? <div class="form-card-foot"><Button kind="primary" onClick={async () => { if (await act(() => patch(`/orgs/${o.slug}`, { sso }), "SSO saved")) reload(); }}>Save</Button></div> : null}
      </Card>
      <Card class="form-card">
        <div>
          <div class="panel-title">SCIM provisioning</div>
          <div class="panel-desc">Let your identity provider create and deactivate members. Deactivated people lose access immediately.</div>
        </div>
        <div class="row">
          <Pill status={o.scimConfigured ? "active" : "pending"} label={o.scimConfigured ? "Token set" : "Not configured"} />
          <span class="grow" />
          {o.role === "owner" ? <Button kind="secondary" icon="key" onClick={async () => {
            if (o.scimConfigured && !(await confirm({ title: "Rotate the SCIM token?", body: "The old token stops working immediately.", confirm: "Rotate" }))) return;
            const r = await act(() => post(`/orgs/${o.slug}/scim-token`));
            if (r) {
              setScim(r);
              reload();
            }
          }}>{o.scimConfigured ? "Rotate token" : "Generate token"}</Button> : null}
        </div>
        {scim ? (
          <Callout kind="green" title="Copy this now; it will not be shown again.">
            <div class="col" style={{ gap: 8, marginTop: 6 }}>
              <CodeBlock code={scim.baseUrl} label="SCIM base URL" />
              <CodeBlock code={scim.token} label="Bearer token" />
            </div>
          </Callout>
        ) : null}
      </Card>
    </>
  );
}

function Audit({ org }: { org: string }) {
  const [q, setQ] = useState("");
  const r = useApi<any[]>(`/orgs/${org}/audit?q=${encodeURIComponent(q)}`, [q]);
  return (
    <>
      <div class="search-input" style={{ maxWidth: 360 }}><Icon name="search" size={15} /><input class="input" placeholder="Filter by action, person or target" value={q} onInput={(e: any) => setQ(e.target.value)} /></div>
      {!r.data ? <Loading /> : r.data.length === 0 ? <Empty icon="history" title="Nothing recorded" /> : (
        <Table head={["When", "Who", "Action", "Target", ""]}>
          {r.data.map((e) => (
            <tr key={e.id}>
              <td class="nowrap" title={dateTime(e.createdAt)}>{timeAgo(e.createdAt)}</td>
              <td class="mono">{e.actor}</td>
              <td><code>{e.action}</code></td>
              <td class="mono muted truncate" style={{ maxWidth: 240 }}>{e.target}</td>
              <td class="muted" style={{ fontSize: 12 }}>{e.meta ? Object.entries(e.meta).map(([k, v]) => `${k}=${v}`).join(" ") : ""}</td>
            </tr>
          ))}
        </Table>
      )}
    </>
  );
}

function OrgDanger({ o }: { o: any }) {
  if (o.role !== "owner") return <Callout kind="info">Only owners can delete the organization.</Callout>;
  return (
    <Card class="form-card danger-zone">
      <div>
        <div class="panel-title">Delete organization</div>
        <div class="panel-desc">Deletes every project, app, database, job and sandbox in {o.name}, and every namespace they live in.</div>
      </div>
      <div class="form-card-foot">
        <Button kind="danger" icon="trash" onClick={async () => {
          if (await confirm({ title: `Delete ${o.name}?`, body: "This cannot be undone.", danger: true, confirm: "Delete organization", typeToConfirm: o.slug })) {
            if (await act(() => del(`/orgs/${o.slug}`), "Deleted")) {
              await loadSession();
              navigate("/");
            }
          }
        }}>Delete organization</Button>
      </div>
    </Card>
  );
}

// ---- account ----

type AccTab = "profile" | "security" | "tokens" | "ssh" | "github" | "mcp";

export function AccountPage() {
  const s = use(session);
  const [tab, setTab] = useTab<AccTab>(query().get("setup") ? "security" : "profile");
  const me = s.me!;
  return (
    <SettingsLayout
      title="Account"
      sub={`@${me.username}`}
      tab={tab}
      setTab={setTab}
      tabs={[
        { id: "profile", label: "Profile", icon: "user" },
        { id: "security", label: "Passkeys & password", icon: "fingerprint" },
        { id: "tokens", label: "API tokens", icon: "key" },
        { id: "ssh", label: "SSH identities", icon: "terminal" },
        { id: "github", label: "GitHub", icon: "github" },
        { id: "mcp", label: "CLI & MCP", icon: "bot" },
      ]}
    >
      {query().get("error") ? <Callout kind="red">{query().get("error")}</Callout> : null}
      {tab === "profile" ? <Profile /> : null}
      {tab === "security" ? <Security /> : null}
      {tab === "tokens" ? <Tokens /> : null}
      {tab === "ssh" ? <SSHKeys /> : null}
      {tab === "github" ? <GitHubLink /> : null}
      {tab === "mcp" ? <CliMcp /> : null}
    </SettingsLayout>
  );
}

function Profile() {
  const s = use(session);
  const me = s.me!;
  const [name, setName] = useState(me.name);
  const [email, setEmail] = useState(me.email || "");
  return (
    <Card class="form-card">
      <div class="row">
        <Avatar seed={me.avatarSeed} size={56} />
        <div class="grow"><div style={{ fontWeight: 600, fontSize: 16 }}>{me.name}</div><div class="muted mono">@{me.username}</div></div>
        <Button kind="secondary" icon="sparkles" onClick={async () => { await act(() => patch("/me", { avatarSeed: Math.random().toString(16).slice(2, 10) })); loadSession(); }}>New critter</Button>
      </div>
      <div class="form-grid">
        <Field label="Display name"><Input value={name} onInput={setName} /></Field>
        <Field label="Email"><Input value={email} onInput={setEmail} type="email" /></Field>
      </div>
      <div class="form-card-foot"><Button kind="primary" onClick={async () => { if (await act(() => patch("/me", { name, email }), "Saved")) loadSession(); }}>Save</Button></div>
    </Card>
  );
}

function Security() {
  const s = use(session);
  const me = s.me!;
  const [cur, setCur] = useState("");
  const [nw, setNw] = useState("");
  const [pkName, setPkName] = useState("");
  return (
    <>
      {s.auth?.needsCredential ? <Callout kind="red" title="Add a way back in.">The claim link was single use. Until you add a passkey or password, a cleared cookie jar leaves nothing to sign in with.</Callout> : null}
      <Card class="form-card">
        <div>
          <div class="panel-title">Passkeys</div>
          <div class="panel-desc">Sign in with Touch ID, Windows Hello, a phone or a security key. Passkeys need HTTPS (or localhost).</div>
        </div>
        {me.passkeys.map((k) => (
          <div key={k.id} class="row" style={{ padding: "6px 0" }}>
            <Icon name="fingerprint" class="muted" />
            <span class="grow">{k.name} <span class="muted" style={{ fontSize: 12.5 }}>added {timeAgo(k.createdAt)}</span></span>
            <IconButton icon="trash" title="Remove passkey" onClick={async () => { if (await confirm({ title: `Remove ${k.name}?`, danger: true, confirm: "Remove" })) { await act(() => del(`/me/passkeys/${k.id}`), "Removed"); loadSession(); } }} />
          </div>
        ))}
        {passkeysSupported() ? (
          <div class="row">
            <Input value={pkName} onInput={setPkName} placeholder="MacBook Touch ID" />
            <Button kind="primary" icon="plus" onClick={async () => {
              try {
                await registerPasskey(pkName || "Passkey");
                toast("Passkey added", "ok");
                setPkName("");
                loadSession();
              } catch (e) {
                if ((e as Error).name !== "NotAllowedError") toast((e as Error).message, "error");
              }
            }}>Add passkey</Button>
          </div>
        ) : <Callout kind="amber">This page is not served over HTTPS, so the browser does not offer passkeys. Use a password here (typical for LAN installs).</Callout>}
      </Card>
      <Card class="form-card">
        <div class="panel-title">{me.hasPassword ? "Change password" : "Set a password"}</div>
        <div class="form-grid">
          {me.hasPassword ? <Field label="Current password"><Input value={cur} onInput={setCur} type="password" autocomplete="current-password" /></Field> : null}
          <Field label="New password" hint="At least 10 characters."><Input value={nw} onInput={setNw} type="password" autocomplete="new-password" /></Field>
        </div>
        <div class="form-card-foot"><Button kind="primary" disabled={nw.length < 10} onClick={async () => { if (await act(() => post("/me/password", { current: cur, new: nw }), "Password saved")) { setCur(""); setNw(""); loadSession(); } }}>Save password</Button></div>
      </Card>
    </>
  );
}

function Tokens() {
  const r = useApi<any[]>("/me/tokens");
  const [name, setName] = useState("");
  const [fresh, setFresh] = useState<string | null>(null);
  return (
    <>
      <Card class="form-card">
        <div>
          <div class="panel-title">Personal API tokens</div>
          <div class="panel-desc">For the CLI, scripts, CI and agents over MCP. A token can do everything you can.</div>
        </div>
        <div class="row">
          <Input value={name} onInput={setName} placeholder="laptop cli" />
          <Button kind="primary" icon="plus" onClick={async () => { const t = await act(() => post("/me/tokens", { name })); if (t) { setFresh(t.token); setName(""); r.reload(); } }}>Create token</Button>
        </div>
        {fresh ? <Callout kind="green" title="Copy it now; it is shown once."><CodeBlock code={fresh} /></Callout> : null}
      </Card>
      {!r.data ? <Loading /> : r.data.length === 0 ? null : (
        <Table head={["Name", "Token", "Created", "Last used", ""]}>
          {r.data.map((t) => (
            <tr key={t.id}>
              <td>{t.name}</td>
              <td class="mono muted">{t.prefix}…</td>
              <td>{timeAgo(t.createdAt)}</td>
              <td class="muted">{t.lastUsed ? timeAgo(t.lastUsed) : "never"}</td>
              <td style={{ textAlign: "right" }}><IconButton icon="trash" title="Revoke" onClick={async () => { if (await confirm({ title: `Revoke ${t.name}?`, danger: true, confirm: "Revoke" })) { await act(() => del(`/me/tokens/${t.id}`), "Revoked"); r.reload(); } }} /></td>
            </tr>
          ))}
        </Table>
      )}
    </>
  );
}

function SSHKeys() {
  const s = use(session);
  const [key, setKey] = useState("");
  const [name, setName] = useState("");
  return (
    <Card class="form-card">
      <div>
        <div class="panel-title">SSH identities</div>
        <div class="panel-desc">Attach a public key to reach Greenhouse sandboxes from your own terminal or point a local editor at them.</div>
      </div>
      {s.me!.sshKeys.map((k) => (
        <div key={k.id} class="row" style={{ padding: "6px 0" }}>
          <Icon name="key" class="muted" />
          <span class="grow">{k.name} <span class="mono muted" style={{ fontSize: 12 }}>{k.fingerprint}</span></span>
          <IconButton icon="trash" title="Remove" onClick={async () => { await act(() => del(`/me/ssh-keys/${k.id}`), "Removed"); loadSession(); }} />
        </div>
      ))}
      <Field label="Name"><Input value={name} onInput={setName} placeholder="laptop" /></Field>
      <Field label="Public key"><Textarea value={key} onInput={setKey} rows={3} mono placeholder="ssh-ed25519 AAAA… you@laptop" /></Field>
      <div class="form-card-foot"><Button kind="primary" disabled={!key.trim()} onClick={async () => { if (await act(() => post("/me/ssh-keys", { name, publicKey: key }), "Key added")) { setKey(""); setName(""); loadSession(); } }}>Add key</Button></div>
    </Card>
  );
}

function GitHubLink() {
  const r = useApi<any>("/github/status");
  if (!r.data) return <Loading />;
  return (
    <Card class="form-card">
      <div>
        <div class="panel-title">GitHub</div>
        <div class="panel-desc">Link your account to deploy repositories. Wack Club Orchard only ever sees the repositories you grant the app.</div>
      </div>
      {!r.data.configured ? <Callout kind="amber">The instance has no GitHub App yet. An instance admin creates it under Instance admin → GitHub.</Callout> : (
        <div class="row">
          {r.data.login ? <><Icon name="github" /> <span class="grow">Linked as <strong>@{r.data.login}</strong></span></> : <span class="grow muted">Not linked.</span>}
          {r.data.installUrl ? <Button kind="ghost" href={r.data.installUrl} target="_blank" iconRight="external">Grant repositories</Button> : null}
          <Button kind={r.data.login ? "secondary" : "primary"} icon="github" href="/api/github/connect?next=/account?tab=github">{r.data.login ? "Relink" : "Link GitHub"}</Button>
        </div>
      )}
    </Card>
  );
}

function CliMcp() {
  const host = location.origin;
  return (
    <>
      <Card class="form-card">
        <div class="panel-title">The wackcluborchard CLI</div>
        <CodeBlock label="sign in" code={`wackcluborchard login ${host}\nwackcluborchard apps\nwackcluborchard deploy api\nwackcluborchard logs api -f\nwackcluborchard run warm-site`} />
      </Card>
      <Card class="form-card">
        <div>
          <div class="panel-title">MCP for agents</div>
          <div class="panel-desc">Agents like Claude can list, deploy, scale, roll back, read logs, query databases and run jobs. Authenticate with a personal API token.</div>
        </div>
        <CodeBlock label="claude code" code={`claude mcp add --transport http wackcluborchard ${host}/mcp \\\n  --header "Authorization: Bearer wackclubwackcluborchard_…"`} />
        <CodeBlock label="mcp.json" code={JSON.stringify({ mcpServers: { wackcluborchard: { type: "http", url: host + "/mcp", headers: { Authorization: "Bearer wackclubwackcluborchard_…" } } } }, null, 2)} />
      </Card>
    </>
  );
}

// ---- instance admin ----

type AdminTab = "settings" | "github" | "orgs" | "users" | "nodes" | "networking" | "audit";

export function AdminPage() {
  const [tab, setTab] = useTab<AdminTab>("settings");
  return (
    <SettingsLayout
      title="Instance admin"
      sub="Superadmin only: the platform itself."
      tab={tab}
      setTab={setTab}
      tabs={[
        { id: "settings", label: "Settings", icon: "settings" },
        { id: "github", label: "GitHub App", icon: "github" },
        { id: "orgs", label: "Organizations", icon: "building" },
        { id: "users", label: "Users", icon: "users" },
        { id: "nodes", label: "Nodes & pools", icon: "server" },
        { id: "networking", label: "Networking", icon: "globe" },
        { id: "audit", label: "Audit", icon: "history" },
      ]}
    >
      {query().get("error") ? <Callout kind="red">{query().get("error")}</Callout> : null}
      {tab === "settings" ? <AdminSettings /> : null}
      {tab === "github" ? <AdminGitHub /> : null}
      {tab === "orgs" ? <AdminOrgs /> : null}
      {tab === "users" ? <AdminUsers /> : null}
      {tab === "nodes" ? <AdminNodes /> : null}
      {tab === "networking" ? <AdminNetworking /> : null}
      {tab === "audit" ? <AdminAudit /> : null}
    </SettingsLayout>
  );
}

function AdminSettings() {
  const r = useApi<any>("/admin/settings");
  const [st, setSt] = useState<any>(null);
  useEffect(() => {
    if (r.data) setSt({ ...r.data.settings });
  }, [r.data]);
  if (!st) return <Loading />;
  const f = (k: string, label: string, hint?: string, ph?: string) => <Field label={label} hint={hint}><Input value={st[k] || ""} onInput={(v) => setSt({ ...st, [k]: v })} mono placeholder={ph} /></Field>;
  const save = async () => {
    if (await act(() => patch("/admin/settings", st), "Settings saved")) r.reload();
  };
  return (
    <>
      <Card class="form-card">
        <div class="row">
          <div class="grow"><div class="panel-title">Instance</div><div class="panel-desc">Runtime: <strong>{r.data.runtime}</strong> · version {r.data.version} · builds {r.data.builds.running}/{r.data.builds.slots} busy, {r.data.builds.waiting} queued</div></div>
        </div>
        <div class="form-grid">
          {f("instanceName", "Instance name")}
          <Field label="Signups"><Select value={st.signupMode} onChange={(v) => setSt({ ...st, signupMode: v })} options={[{ value: "open", label: "Open to anyone" }, { value: "invite", label: "Invite only" }, { value: "closed", label: "Closed" }]} /></Field>
          {f("domain", "Dashboard domain")}
          {f("appDomain", "App domain", "Apps get <name>.<app domain>.")}
          {f("ingressCname", "Custom domain CNAME target")}
          {f("publicIp", "Default public IP", "Inherited by every organization for TCP/UDP and public databases.")}
          {f("publicDbDomain", "Public database domain", "Optional wildcard for exposed databases.")}
          <Field label="Build slots" hint="Concurrent builds. Extra builds queue."><Input type="number" value={st.buildSlots} onInput={(v) => setSt({ ...st, buildSlots: Number(v) })} /></Field>
        </div>
        <div class="form-card-foot"><Button kind="primary" onClick={save}>Save</Button></div>
      </Card>
      <Card class="form-card">
        <div class="panel-title">MCP endpoint</div>
        <Toggle checked={st.mcpEnabled} onChange={(v) => setSt({ ...st, mcpEnabled: v })} label="Enable /mcp" hint={`Agents connect at ${st.mcpDomain ? "https://" + st.mcpDomain : location.origin + "/mcp"} with a personal token.`} />
        {f("mcpDomain", "MCP hostname", "Needs a DNS record before its certificate can be issued.")}
        <div class="form-card-foot"><Button kind="primary" onClick={save}>Save</Button></div>
      </Card>
      <Card class="form-card">
        <div>
          <div class="panel-title">Isolation</div>
          <div class="panel-desc">Deployments are network-isolated by default but share the host kernel. Turn these on once the node runtimes are installed.</div>
        </div>
        <Toggle checked={st.tenantSandbox} onChange={(v) => setSt({ ...st, tenantSandbox: v })} label="gVisor for new tenant apps" hint="RuntimeClass gvisor." />
        <Toggle checked={st.builderSandbox} onChange={(v) => setSt({ ...st, builderSandbox: v })} label="Kata VMs for image builds" hint="Without it, builds run with host-level privileges: fine when the builder owns the box." />
        <div class="form-card-foot"><Button kind="primary" onClick={save}>Save</Button></div>
      </Card>
      <Card class="form-card">
        <div>
          <div class="panel-title">Greenhouse agent</div>
          <div class="panel-desc">An Anthropic API key lets sandboxes run Claude against their workspace.</div>
        </div>
        <Field label="Anthropic API key"><Input value={st.anthropicKey || ""} onInput={(v) => setSt({ ...st, anthropicKey: v })} type="password" mono placeholder="sk-ant-…" /></Field>
        <div class="form-card-foot"><Button kind="primary" onClick={save}>Save</Button></div>
      </Card>
    </>
  );
}

function AdminGitHub() {
  const r = useApi<any>("/github/status");
  const [ghOrg, setGhOrg] = useState("");
  if (!r.data) return <Loading />;
  const create = async () => {
    const m = await act(() => post("/github/manifest", { org: ghOrg }));
    if (!m) return;
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
  return (
    <Card class="form-card">
      <div>
        <div class="panel-title">GitHub App</div>
        <div class="panel-desc">Wack Club Orchard generates the app manifest, GitHub creates the app, and the credentials come back automatically. Then each person links their own account.</div>
      </div>
      {r.data.configured ? (
        <Callout kind="green" title={`Connected as ${r.data.slug}`}>
          Pushes to tracked branches deploy automatically. <a href={r.data.installUrl} target="_blank" rel="noopener">Install on more accounts ↗</a>
        </Callout>
      ) : null}
      <Field label="GitHub organization" hint="Optional. Leave empty to create the app under your personal account."><Input value={ghOrg} onInput={setGhOrg} placeholder="hackclub" mono /></Field>
      <div class="form-card-foot"><Button kind="primary" icon="github" onClick={create}>{r.data.configured ? "Create a new GitHub App" : "Create GitHub App"}</Button></div>
    </Card>
  );
}

function AdminOrgs() {
  const r = useApi<any[]>("/admin/orgs");
  if (!r.data) return <Loading />;
  return (
    <Table head={["Organization", "Members", "Projects", "CPU", "Memory", "Pool", ""]}>
      {r.data.map((o) => (
        <tr key={o.id}>
          <td><strong>{o.name}</strong> <span class="muted mono" style={{ fontSize: 12 }}>{o.slug}</span></td>
          <td>{o.members}</td>
          <td>{o.projects}</td>
          <td class="mono" style={{ fontSize: 12.5 }}>{cpu(o.used.cpuMillis)} / {o.quota.cpuMillis ? cpu(o.quota.cpuMillis) : "∞"}</td>
          <td class="mono" style={{ fontSize: 12.5 }}>{mem(o.used.memoryMi)} / {o.quota.memoryMi ? mem(o.quota.memoryMi) : "∞"}</td>
          <td>{o.pool ? <Tag>{o.pool}</Tag> : <span class="muted">any</span>}</td>
          <td style={{ textAlign: "right" }}><Button size="sm" kind="secondary" href={`/o/${o.slug}/settings?tab=quotas`}>Quotas</Button></td>
        </tr>
      ))}
    </Table>
  );
}

function AdminUsers() {
  const r = useApi<any[]>("/admin/users");
  const s = use(session);
  if (!r.data) return <Loading />;
  return (
    <Table head={["User", "Organizations", "Sign-in", "Joined", ""]}>
      {r.data.map((u) => (
        <tr key={u.id} style={u.disabled ? { opacity: 0.55 } : undefined}>
          <td><div class="row"><Avatar seed={u.avatarSeed} size={28} /><div><strong>{u.name}</strong> {u.superadmin ? <Tag><Icon name="shield" size={11} /> superadmin</Tag> : null}<div class="muted mono" style={{ fontSize: 12 }}>@{u.username}{u.email ? " · " + u.email : ""}</div></div></div></td>
          <td class="muted" style={{ fontSize: 12.5 }}>{(u.orgs || []).join(", ")}</td>
          <td class="muted" style={{ fontSize: 12.5 }}>{[u.hasPassword && "password", u.passkeys.length && plural(u.passkeys.length, "passkey"), u.githubLogin && "github"].filter(Boolean).join(", ") || "sso"}</td>
          <td class="muted">{timeAgo(u.createdAt)}</td>
          <td style={{ textAlign: "right" }}>
            {u.id !== s.me?.id ? (
              <div class="row" style={{ justifyContent: "flex-end", gap: 6 }}>
                <Button size="sm" kind="ghost" onClick={async () => { if (await act(() => patch(`/admin/users/${u.id}`, { superadmin: !u.superadmin }), "Updated")) r.reload(); }}>{u.superadmin ? "Demote" : "Make superadmin"}</Button>
                <Button size="sm" kind="secondary" onClick={async () => { if (await act(() => patch(`/admin/users/${u.id}`, { disabled: !u.disabled }), u.disabled ? "Enabled" : "Disabled and signed out")) r.reload(); }}>{u.disabled ? "Enable" : "Disable"}</Button>
              </div>
            ) : null}
          </td>
        </tr>
      ))}
    </Table>
  );
}

function AdminNodes() {
  const r = useApi<any>("/admin/nodes");
  const orgs = useApi<any[]>("/admin/orgs");
  if (r.error) return <ErrorBox error={r.error} />;
  if (!r.data) return <Loading />;
  const editPool = (p?: any) => openModal((close) => <PoolEditor pool={p} nodes={r.data.nodes} orgs={orgs.data || []} close={close} done={r.reload} />);
  return (
    <>
      <div class="grid-2">
        {r.data.nodes.map((n: any) => (
          <Card key={n.name} class="node-card">
            <div class="row">
              <Icon name="server" />
              <strong class="grow mono">{n.name}</strong>
              <Pill status={n.ready ? "ready" : "failed"} label={n.ready ? "Ready" : "NotReady"} />
            </div>
            <div class="row row-wrap" style={{ gap: 5 }}>
              {(n.roles || []).map((x: string) => <Tag key={x}>{x}</Tag>)}
              <Tag mono>{n.arch}</Tag>
              <Tag mono>{n.kubelet}</Tag>
              {n.labels?.["wackcluborchard.dev/pool"] ? <Tag><Icon name="layers" size={11} /> {n.labels["wackcluborchard.dev/pool"]}</Tag> : null}
            </div>
            <Meter label="CPU requested" used={n.requestedCpuMillis} cap={n.allocatableCpuMillis} format={cpu} />
            <Meter label="Memory requested" used={n.requestedMemoryMi} cap={n.allocatableMemoryMi} format={mem} />
            <div class="muted" style={{ fontSize: 12.5 }}>{n.pods} pods · allocatable is smaller than capacity ({cpu(n.cpuMillis)}, {mem(n.memoryMi)}) because the system takes a share.{n.taints?.length ? " Taints: " + n.taints.join(", ") : ""}</div>
          </Card>
        ))}
      </div>
      <Card class="form-card">
        <div class="row">
          <div class="grow">
            <div class="panel-title">Pools</div>
            <div class="panel-desc">Group nodes with a purpose and assign organizations to them. Pools use ordinary taints and node selectors, so kubectl explains placement the usual way.</div>
          </div>
          <Button kind="primary" icon="plus" onClick={() => editPool()}>New pool</Button>
        </div>
        {r.data.pools.length === 0 ? <div class="muted">No pools: every organization schedules anywhere.</div> : r.data.pools.map((p: any) => (
          <div key={p.id} class="row" style={{ padding: "8px 0", borderTop: "1px solid var(--border)" }}>
            <Icon name="layers" class="muted" />
            <div class="grow"><strong>{p.name}</strong> <span class="muted">{p.description}</span><div class="muted" style={{ fontSize: 12.5 }}>{plural(p.nodes.length, "node")} · {plural(p.orgIds.length, "organization")}{p.taint ? " · tainted" : ""}</div></div>
            <Button size="sm" kind="secondary" onClick={() => editPool(p)}>Edit</Button>
            <IconButton icon="trash" title="Delete pool" onClick={async () => { if (await confirm({ title: `Delete pool ${p.name}?`, body: "Its nodes are untainted and its organizations schedule anywhere again.", danger: true, confirm: "Delete" })) { await act(() => del(`/admin/pools/${p.id}`), "Deleted"); r.reload(); } }} />
          </div>
        ))}
      </Card>
    </>
  );
}

function PoolEditor({ pool, nodes, orgs, close, done }: { pool?: any; nodes: any[]; orgs: any[]; close: () => void; done: () => void }) {
  const [p, setP] = useState<any>(pool ? { ...pool } : { name: "", description: "", nodes: [], orgIds: [], taint: true });
  const toggle = (k: string, v: string) => setP({ ...p, [k]: p[k].includes(v) ? p[k].filter((x: string) => x !== v) : [...p[k], v] });
  return (
    <>
      <ModalHeader title={pool ? `Edit ${pool.name}` : "New pool"} onClose={close} icon="layers" />
      <div class="modal-body">
        <div class="form-grid">
          <Field label="Name"><Input value={p.name} onInput={(v) => setP({ ...p, name: v })} mono /></Field>
          <Field label="Purpose"><Input value={p.description} onInput={(v) => setP({ ...p, description: v })} placeholder="GPU box Sam paid for" /></Field>
        </div>
        <Field label="Nodes">{nodes.map((n) => <label key={n.name} class="row"><input type="checkbox" checked={p.nodes.includes(n.name)} onChange={() => toggle("nodes", n.name)} /> <span class="mono">{n.name}</span></label>)}</Field>
        <Field label="Organizations">{orgs.map((o) => <label key={o.id} class="row"><input type="checkbox" checked={p.orgIds.includes(o.id)} onChange={() => toggle("orgIds", o.id)} /> {o.name}</label>)}</Field>
        <Toggle checked={p.taint} onChange={(v) => setP({ ...p, taint: v })} label="Taint the nodes" hint="Keep everyone else off them. Without a taint, the pool only attracts its organizations." />
      </div>
      <div class="modal-foot">
        <Button kind="ghost" onClick={close}>Cancel</Button>
        <Button kind="primary" disabled={!p.name} onClick={async () => { if (await act(() => post("/admin/pools", p), "Pool saved")) { close(); done(); } }}>Save</Button>
      </div>
    </>
  );
}

function AdminNetworking() {
  const r = useApi<any>("/admin/networking");
  if (r.error) return <ErrorBox error={r.error} />;
  if (!r.data) return <Loading label="Resolving hostnames…" />;
  return (
    <>
      <Card>
        <div class="panel-title">Traffic</div>
        <div class="panel-desc">Mode <strong>{r.data.mode}</strong>{r.data.publicIp ? <> · public IP <code>{r.data.publicIp}</code></> : null}. In public mode certificates come from Let's Encrypt over HTTP-01, so every name must point here before its certificate can be issued.</div>
      </Card>
      <Table head={["Name", "Used for", "Resolves to", ""]}>
        {r.data.checks.map((c: any) => (
          <tr key={c.name}>
            <td class="mono">{c.name}</td>
            <td class="muted">{c.purpose}</td>
            <td class="mono muted" style={{ fontSize: 12.5 }}>{c.records?.join(", ") || c.error}</td>
            <td><Pill status={c.ok ? "active" : "failed"} label={c.ok ? "OK" : "Check DNS"} /></td>
          </tr>
        ))}
      </Table>
    </>
  );
}

function AdminAudit() {
  const r = useApi<any[]>("/admin/audit");
  if (!r.data) return <Loading />;
  return (
    <Table head={["When", "Who", "Action", "Target"]}>
      {r.data.map((e) => (
        <tr key={e.id}>
          <td class="nowrap">{timeAgo(e.createdAt)}</td>
          <td class="mono">{e.actor}</td>
          <td><code>{e.action}</code></td>
          <td class="mono muted">{e.target}</td>
        </tr>
      ))}
    </Table>
  );
}

export { get, put, Link };
