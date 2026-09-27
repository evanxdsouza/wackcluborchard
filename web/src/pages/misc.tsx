import { h, Fragment, useEffect, useRef, useState } from "../lib/sprout.js";
import { Link, navigate, query } from "../lib/router.js";
import { useApi, useEvents, post, put, patch, del, act, get } from "../lib/api.js";
import { cpu, mem, timeAgo, plural } from "../lib/format.js";
import { Button, Loading, ErrorBox, Empty, Pill, Card, Field, Input, Select, Meter, confirm, Callout, Table, IconButton, cx, openModal, ModalHeader, Segmented, Tag, Textarea } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";
import { Sky, Avatar, ImageBadge } from "../ui/art.js";
import { Terminal } from "../ui/term.js";

// ---- Usage ----

export function UsagePage({ org }: { org: string }) {
  const r = useApi<any>(`/orgs/${org}/usage`);
  if (r.error) return <div class="page"><ErrorBox error={r.error} /></div>;
  if (!r.data) return <div class="page"><Loading /></div>;
  const u = r.data;
  const gi = (n: number) => n + " GiB";
  const count = (n: number) => String(n);
  return (
    <div class="page">
      <div class="page-head">
        <div>
          <h1 class="page-title">Usage</h1>
          <div class="page-sub">Quotas are enforced against requests. The effective limit for anything new is the smaller of your allowance and the organization's remaining cap.</div>
        </div>
      </div>
      <div class="grid-2">
        <Card class="form-card">
          <div class="panel-title">Organization</div>
          <Meter label="CPU" used={u.used.cpuMillis} cap={u.quota.cpuMillis} format={cpu} />
          <Meter label="Memory" used={u.used.memoryMi} cap={u.quota.memoryMi} format={mem} />
          <Meter label="Storage" used={u.used.storageGi} cap={u.quota.storageGi} format={gi} />
          <div class="grid-3">
            <Meter label="Apps" used={u.used.apps} cap={u.quota.apps} format={count} />
            <Meter label="Databases" used={u.used.databases} cap={u.quota.databases} format={count} />
            <Meter label="Sandboxes" used={u.used.sandboxes} cap={u.quota.sandboxes} format={count} />
          </div>
        </Card>
        <Card class="form-card">
          <div class="panel-title">Your allowance</div>
          <Meter label="CPU" used={u.memberUsed.cpuMillis} cap={u.memberQuota.cpuMillis} format={cpu} />
          <Meter label="Memory" used={u.memberUsed.memoryMi} cap={u.memberQuota.memoryMi} format={mem} />
          <Meter label="Storage" used={u.memberUsed.storageGi} cap={u.memberQuota.storageGi} format={gi} />
          <div class="grid-3">
            <Meter label="Apps" used={u.memberUsed.apps} cap={u.memberQuota.apps} format={count} />
            <Meter label="Databases" used={u.memberUsed.databases} cap={u.memberQuota.databases} format={count} />
            <Meter label="Sandboxes" used={u.memberUsed.sandboxes} cap={u.memberQuota.sandboxes} format={count} />
          </div>
          <div class="muted" style={{ fontSize: 12.5 }}>Owners and admins are only bound by the organization cap.</div>
        </Card>
      </div>
      <div class="section">
        <div class="section-head"><h2 class="section-title">By project</h2><span class="muted" style={{ fontSize: 13 }}>Builds: {u.builds.running}/{u.builds.slots} slots busy{u.builds.waiting ? `, ${u.builds.waiting} queued` : ""}</span></div>
        {!u.projects?.length ? <Empty title="No projects yet" /> : (
          <Table head={["Project", "Apps", "Databases", "CPU requested", "Memory requested", "Storage", "Live CPU / memory"]}>
            {u.projects.map((p: any) => (
              <tr key={p.id} class="clickable" onClick={() => navigate(`/o/${org}/projects/${p.id}`)}>
                <td><strong>{p.icon} {p.name}</strong></td>
                <td>{p.apps}</td>
                <td>{p.databases}</td>
                <td class="mono">{cpu(p.cpuMillis)}</td>
                <td class="mono">{mem(p.memoryMi)}</td>
                <td class="mono">{p.storageGi} GiB</td>
                <td class="mono muted">{cpu(p.liveCpuMillis)} · {mem(p.liveMemoryMi)}</td>
              </tr>
            ))}
          </Table>
        )}
      </div>
    </div>
  );
}

// ---- The Grove (templates) ----

export function GrovePage({ org }: { org: string }) {
  const r = useApi<any[]>("/templates");
  const [cat, setCat] = useState("All");
  const [q, setQ] = useState("");
  const cats = ["All", ...new Set((r.data || []).map((t) => t.category))];
  const shown = (r.data || []).filter((t) => (cat === "All" || t.category === cat) && (!q || (t.name + t.description).toLowerCase().includes(q.toLowerCase())));
  return (
    <div class="page">
      <div class="grove-hero">
        <Sky seed="the-grove" preset="meadow" />
        <div class="grove-hero-text">
          <h1>The Grove</h1>
          <p>Ready-made stacks. Apps, databases and the variables between them, planted in one go.</p>
        </div>
      </div>
      <div class="row row-wrap" style={{ marginBottom: 16 }}>
        <Segmented value={cat} onChange={setCat} options={cats.map((c) => ({ id: c, label: c }))} />
        <span class="grow" />
        <div class="search-input"><Icon name="search" size={15} /><input class="input" placeholder="Search templates…" value={q} onInput={(e: any) => setQ(e.target.value)} /></div>
      </div>
      {!r.data ? <Loading /> : (
        <div class="grid-3">
          {shown.map((t) => (
            <Card key={t.id} class="tpl-card">
              <div class="row">
                <ImageBadge image={t.apps[0]?.image || ""} name={t.name} size={40} />
                <div class="grow">
                  <div style={{ fontWeight: 600, fontSize: 15.5 }}>{t.name}</div>
                  <div class="tpl-cat">{t.category}</div>
                </div>
              </div>
              <div class="tpl-desc">{t.description}</div>
              <div class="row row-wrap" style={{ gap: 5 }}>
                {t.apps.map((a: any, i: number) => <Tag key={i} mono>{a.image.split("/").pop()}</Tag>)}
                {t.databases?.map((d: string) => <Tag key={d}><Icon name="database" size={11} /> postgres</Tag>)}
              </div>
              <Button kind="secondary" icon="sprout" onClick={() => openModal((close) => <PlantTemplate org={org} t={t} close={close} />)}>Deploy</Button>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}

function PlantTemplate({ org, t, close }: { org: string; t: any; close: () => void }) {
  const r = useApi<any[]>(`/orgs/${org}/overview`);
  const projects = (r.data || []).filter((p) => p.canEdit);
  const [pid, setPid] = useState(query().get("project") || "");
  const [name, setName] = useState(t.id.split("-")[0]);
  useEffect(() => {
    if (!pid && projects.length) setPid(projects[0].id);
  }, [projects.length]);
  return (
    <>
      <ModalHeader title={`Deploy ${t.name}`} subtitle={t.description} onClose={close} icon="trees" />
      <div class="modal-body">
        <Field label="Project"><Select value={pid} onChange={setPid} options={projects.map((p) => ({ value: p.id, label: `${p.icon} ${p.name}` }))} /></Field>
        <Field label="Instance name" hint={t.databases?.length ? `Creates the app ${name} and the database ${name}-db, wired together.` : `Creates the app ${name}.`}><Input value={name} onInput={(v) => setName(v.toLowerCase().replace(/[^a-z0-9-]/g, "-"))} mono autofocus /></Field>
      </div>
      <div class="modal-foot">
        <Button kind="ghost" onClick={close}>Cancel</Button>
        <Button kind="primary" icon="sprout" disabled={!pid || !name} onClick={async () => {
          const inst = await act(() => post(`/projects/${pid}/templates`, { templateId: t.id, name }), `${t.name} is being planted`);
          if (inst) {
            close();
            navigate(`/o/${org}/projects/${pid}`);
          }
        }}>Deploy</Button>
      </div>
    </>
  );
}

// ---- Greenhouse (sandboxes) ----

export function GreenhousePage({ org }: { org: string }) {
  const r = useApi<any[]>(`/orgs/${org}/sandboxes`);
  const o = useApi<any>(`/orgs/${org}`);
  useEvents(o.data ? [`org:${o.data.id}`] : [], (e) => {
    if (e.type === "sandbox.updated") r.set((list) => list.map((s) => (s.id === e.data.id ? { ...s, ...e.data, conversations: undefined } : s)));
  });
  return (
    <div class="page">
      <div class="page-head">
        <div>
          <h1 class="page-title">Greenhouse</h1>
          <div class="page-sub">Isolated cloud dev environments: a persistent workspace with an editor, a terminal, git, and an agent.</div>
        </div>
        <div class="page-actions"><Button kind="primary" icon="plus" onClick={() => openModal((close) => <NewSandbox org={org} close={close} />)}>New sandbox</Button></div>
      </div>
      <Callout kind="info" icon="shield">Sandboxes run as Kata micro-VMs with internet egress only: they can reach out, but not into the rest of your cluster. Deleting one deletes its workspace, so push before you delete.</Callout>
      <div style={{ height: 16 }} />
      {!r.data ? <Loading /> : r.data.length === 0 ? (
        <Empty icon="sprout" title="Nothing growing yet" action={<Button kind="primary" icon="plus" onClick={() => openModal((close) => <NewSandbox org={org} close={close} />)}>New sandbox</Button>}>
          Start a sandbox from a repository and work on it from the browser, or let the agent take a first pass.
        </Empty>
      ) : (
        <div class="grid-3">
          {r.data.map((s) => (
            <Link key={s.id} href={`/o/${org}/greenhouse/${s.id}`} class="card card-link" style={{ padding: 16, display: "flex", flexDirection: "column", gap: 10 }}>
              <div class="row">
                <div class="tpl-icon" style={{ width: 40, height: 40 }}><Icon name="sprout" size={18} /></div>
                <div class="grow">
                  <div style={{ fontWeight: 600 }}>{s.name}</div>
                  <div class="muted mono truncate" style={{ fontSize: 12 }}>{s.repo || s.image || "blank workspace"}</div>
                </div>
                <Pill status={s.status} />
              </div>
              {s.status === "booting" ? (
                <div>
                  <div class="boot-bar"><div class="boot-fill" style={{ width: s.bootProgress + "%" }} /></div>
                  <div class="muted" style={{ fontSize: 12, marginTop: 5 }}>{s.bootStage}</div>
                </div>
              ) : null}
              <div class="row muted" style={{ fontSize: 12.5 }}>
                <Avatar seed={s.owner.avatarSeed} size={18} /> {s.owner.username} · {cpu(s.resources.cpuMillis)} · {mem(s.resources.memoryMi)} · {s.storageGi} GiB · {timeAgo(s.createdAt)}
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}

function NewSandbox({ org, close }: { org: string; close: () => void }) {
  const [name, setName] = useState("");
  const [repo, setRepo] = useState("");
  const [image, setImage] = useState("");
  return (
    <>
      <ModalHeader title="New sandbox" subtitle="A persistent workspace in its own micro-VM." onClose={close} icon="sprout" />
      <div class="modal-body">
        <Field label="Name"><Input value={name} onInput={(v) => setName(v.toLowerCase().replace(/[^a-z0-9-]/g, "-"))} autofocus mono placeholder="scratch" /></Field>
        <Field label="Repository" hint="Optional. owner/name, cloned into /workspace on first boot."><Input value={repo} onInput={setRepo} mono placeholder="hackclub/wackcluborchard" /></Field>
        <Field label="Image" hint="Optional. Defaults to a devcontainer with common toolchains."><Input value={image} onInput={setImage} mono placeholder="mcr.microsoft.com/devcontainers/universal:2-linux" /></Field>
      </div>
      <div class="modal-foot">
        <Button kind="ghost" onClick={close}>Cancel</Button>
        <Button kind="primary" disabled={!name} onClick={async () => {
          const s = await act(() => post(`/orgs/${org}/sandboxes`, { name, repo, image }), "Booting");
          if (s) {
            close();
            navigate(`/o/${org}/greenhouse/${s.id}`);
          }
        }}>Create</Button>
      </div>
    </>
  );
}

export function SandboxPage({ org, id }: { org: string; id: string }) {
  const r = useApi<any>(`/sandboxes/${id}`);
  const [pane, setPane] = useState<"editor" | "terminal" | "git">("editor");
  useEvents([`sandbox:${id}`], (e) => {
    if (e.type === "sandbox.updated") r.set((d) => ({ ...d, sandbox: { ...d.sandbox, ...e.data, conversations: d.sandbox.conversations } }));
    if (e.type === "conversation.updated") r.set((d) => ({ ...d, sandbox: { ...d.sandbox, conversations: d.sandbox.conversations.map((c: any) => (c.id === e.data.id ? e.data : c)) } }));
  });
  if (r.error) return <div class="page"><ErrorBox error={r.error} /></div>;
  if (!r.data) return <div class="page"><Loading /></div>;
  const s = r.data.sandbox;
  const running = s.status === "running";
  return (
    <div class="page" style={{ maxWidth: 1500 }}>
      <Link href={`/o/${org}/greenhouse`} class="back-link"><Icon name="arrow-left" size={14} /> Greenhouse</Link>
      <div class="page-head" style={{ marginBottom: 12 }}>
        <h1 class="page-title" style={{ fontSize: 22 }}>{s.name}</h1>
        <Pill status={s.status} />
        {s.repo ? <Tag mono><Icon name="github" size={12} /> {s.repo}</Tag> : null}
        <div class="page-actions">
          <Segmented value={pane} onChange={setPane} options={[{ id: "editor", label: "Files", icon: "folder" }, { id: "terminal", label: "Terminal", icon: "terminal" }, { id: "git", label: "Git", icon: "branch" }]} />
          <Button kind="secondary" icon="restart" onClick={() => act(() => post(`/sandboxes/${id}/restart`), "Restarting")}>Restart</Button>
          <Button kind="danger" icon="trash" onClick={async () => {
            if (await confirm({ title: `Delete ${s.name}?`, body: "Its persistent workspace is deleted. Anything not committed and pushed is gone.", danger: true, confirm: "Delete sandbox", typeToConfirm: s.name })) {
              if (await act(() => del(`/sandboxes/${id}`), "Deleted")) navigate(`/o/${org}/greenhouse`);
            }
          }}>Delete</Button>
        </div>
      </div>
      {!running ? (
        <Card>
          <div class="panel-title">{s.status === "failed" ? "The sandbox failed to boot" : "Booting…"}</div>
          <div class="boot-bar" style={{ margin: "12px 0 6px" }}><div class="boot-fill" style={{ width: s.bootProgress + "%" }} /></div>
          <div class="muted" style={{ fontSize: 13 }}>{s.bootStage}</div>
        </Card>
      ) : (
        <div class="workspace">
          {pane === "editor" ? <Files id={id} /> : pane === "terminal" ? <div class="ws-panel" style={{ gridColumn: "span 2" }}><Terminal path={`/sandboxes/${id}/shell`} title="/workspace" height="100%" /></div> : <Git id={id} />}
          <Agent id={id} s={s} />
        </div>
      )}
    </div>
  );
}

function Files({ id }: { id: string }) {
  const files = useApi<string[]>(`/sandboxes/${id}/files`);
  const [open, setOpen] = useState<string | null>(null);
  const [content, setContent] = useState("");
  const [orig, setOrig] = useState("");
  const load = async (p: string) => {
    const r = await act(() => get(`/sandboxes/${id}/file?path=${encodeURIComponent(p)}`));
    if (r) {
      setOpen(p);
      setContent(r.content);
      setOrig(r.content);
    }
  };
  const save = async () => {
    if (open && (await act(() => put(`/sandboxes/${id}/file`, { path: open, content }), "Saved"))) setOrig(content);
  };
  return (
    <>
      <div class="ws-panel">
        <div class="ws-head"><Icon name="folder" size={14} /> workspace <span class="grow" /><IconButton icon="plus" class="tiny" title="New file" onClick={async () => {
          const p = prompt("New file path");
          if (p && (await act(() => put(`/sandboxes/${id}/file`, { path: p, content: "" })))) {
            await files.reload();
            load(p);
          }
        }} /><IconButton icon="restart" class="tiny" title="Refresh" onClick={files.reload} /></div>
        <div class="ws-body file-tree">
          {!files.data ? <Loading /> : files.data.map((f) => (
            <button key={f} type="button" class={cx("file-item", open === f && "active")} title={f} onClick={() => load(f)}>
              <Icon name="file" size={13} />
              <span style={{ paddingLeft: (f.split("/").length - 1) * 10 }}>{f}</span>
            </button>
          ))}
        </div>
      </div>
      <div class="ws-panel">
        <div class="ws-head"><Icon name="code" size={14} /> {open || "No file open"}{open && content !== orig ? <span style={{ color: "var(--amber)" }}>● modified</span> : null}<span class="grow" />{open ? <Button size="sm" kind="primary" icon="save" onClick={save} disabled={content === orig}>Save</Button> : null}</div>
        {open ? (
          <textarea class="editor" spellcheck="false" value={content} onInput={(e: any) => setContent(e.target.value)} onKeyDown={(e: KeyboardEvent) => {
            if ((e.metaKey || e.ctrlKey) && e.key === "s") {
              e.preventDefault();
              save();
            }
            if (e.key === "Tab") {
              e.preventDefault();
              const t = e.target as HTMLTextAreaElement;
              const st = t.selectionStart;
              setContent(content.slice(0, st) + "  " + content.slice(t.selectionEnd));
              requestAnimationFrame(() => (t.selectionStart = t.selectionEnd = st + 2));
            }
          }} />
        ) : <div class="ws-body"><Empty icon="file" title="Pick a file">Edits are written straight into the sandbox's workspace. ⌘S saves.</Empty></div>}
      </div>
    </>
  );
}

function Git({ id }: { id: string }) {
  const [out, setOut] = useState("");
  const [msg, setMsg] = useState("");
  const [branch, setBranch] = useState("");
  const run = async (op: string, extra: any = {}) => {
    const r = await act(() => post(`/sandboxes/${id}/git`, { op, ...extra }));
    if (r) setOut(`$ git ${op}\n` + (r.output || "") + (r.error ? "\nerror: " + r.error : ""));
  };
  useEffect(() => {
    run("status");
  }, []);
  return (
    <>
      <div class="ws-panel">
        <div class="ws-head"><Icon name="branch" size={14} /> git</div>
        <div class="ws-body" style={{ padding: 12, display: "flex", flexDirection: "column", gap: 10 }}>
          <div class="row row-wrap" style={{ gap: 6 }}>
            {["status", "branches", "diff", "log", "fetch"].map((o) => <Button key={o} size="sm" kind="secondary" onClick={() => run(o)}>{o}</Button>)}
          </div>
          <Field label="Branch"><Input value={branch} onInput={setBranch} mono placeholder="feature/thing" /></Field>
          <div class="row" style={{ gap: 6 }}>
            <Button size="sm" kind="secondary" onClick={() => run("checkout", { branch })} disabled={!branch}>Switch</Button>
            <Button size="sm" kind="secondary" onClick={() => run("checkout", { branch, create: true })} disabled={!branch}>Create</Button>
          </div>
          <Field label="Commit message"><Textarea value={msg} onInput={setMsg} rows={3} /></Field>
          <div class="row row-wrap" style={{ gap: 6 }}>
            <Button size="sm" kind="primary" icon="check" onClick={() => run("commit", { message: msg })} disabled={!msg}>Commit all</Button>
            <Button size="sm" kind="ghost" onClick={() => run("uncommit")}>Uncommit</Button>
          </div>
          <div class="row row-wrap" style={{ gap: 6 }}>
            <Button size="sm" kind="secondary" icon="download" onClick={() => run("pull", { rebase: true })}>Pull --rebase</Button>
            <Button size="sm" kind="secondary" icon="upload" onClick={() => run("push")}>Push</Button>
            <Button size="sm" kind="ghost" onClick={async () => { if (await confirm({ title: "Force push?", body: "Uses --force-with-lease: it refuses if the remote moved under you, instead of overwriting someone else's work.", confirm: "Force push" })) run("push", { force: true }); }}>Force</Button>
          </div>
          <Button size="sm" kind="secondary" icon="github" onClick={() => run("pr", { title: msg })}>Open pull request</Button>
        </div>
      </div>
      <div class="ws-panel" style={{ background: "var(--terminal)" }}>
        <div class="ws-head" style={{ color: "#8b8b93", borderColor: "#1d1d21" }}>output</div>
        <pre class="ws-body mono" style={{ margin: 0, padding: 14, color: "#ddd", fontSize: 12.5, whiteSpace: "pre-wrap" }}>{out}</pre>
      </div>
    </>
  );
}

function Agent({ id, s }: { id: string; s: any }) {
  const convs = s.conversations || [];
  const [cur, setCur] = useState<string | null>(convs[0]?.id || null);
  const [text, setText] = useState("");
  const c = convs.find((x: any) => x.id === cur);
  const scroll = useRef<HTMLElement | null>(null);
  useEffect(() => {
    if (scroll.current) scroll.current.scrollTop = scroll.current.scrollHeight;
  }, [c?.messages?.length]);
  const newConv = async () => {
    const n = await act(() => post(`/sandboxes/${id}/conversations`, { cwd: s.repo ? s.repo.split("/").pop() : "" }));
    if (n) {
      s.conversations = [n, ...convs];
      setCur(n.id);
    }
  };
  const send = async () => {
    if (!text.trim()) return;
    let target = cur;
    if (!target) {
      const n = await act(() => post(`/sandboxes/${id}/conversations`, { cwd: s.repo ? s.repo.split("/").pop() : "" }));
      if (!n) return;
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
  return (
    <div class="ws-panel ws-chat">
      <div class="ws-head">
        <Icon name="bot" size={14} /> Agent
        <span class="grow" />
        {convs.length ? (
          <select class="input select" style={{ height: 26, width: 150, fontSize: 12 }} onChange={(e: any) => setCur(e.target.value)}>
            {convs.map((x: any) => <option key={x.id} value={x.id} selected={x.id === cur}>{x.title}</option>)}
          </select>
        ) : null}
        <IconButton icon="plus" class="tiny" title="New conversation" onClick={newConv} />
        {c ? <IconButton icon="trash" class="tiny" title="Delete conversation" onClick={async () => {
          if (await act(() => del(`/sandboxes/${id}/conversations/${c.id}`))) {
            s.conversations = convs.filter((x: any) => x.id !== c.id);
            setCur(s.conversations[0]?.id || null);
          }
        }} /> : null}
      </div>
      <div class="ws-body chat" ref={scroll}>
        {!c || !c.messages.length ? (
          <div class="muted" style={{ fontSize: 13, textAlign: "center", padding: "30px 10px" }}>
            <Icon name="bot" size={22} /><br />Ask Claude to work in this workspace. It reads and writes real files and runs real commands.{c?.cwd ? <><br />Working in <code>{c.cwd}</code>.</> : null}
          </div>
        ) : c.messages.map((m: any, i: number) => <div key={i} class={"msg " + m.role}>{m.text}</div>)}
        {thinking ? <div class="msg assistant muted"><span class="spinner sm" /> working…</div> : null}
      </div>
      <div class="chat-input">
        <Textarea value={text} onInput={setText} rows={2} placeholder="Add a /healthz endpoint and a test…" onKeyDown={(e: KeyboardEvent) => {
          if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            send();
          }
        }} />
        <IconButton icon="send" title="Send" onClick={send} class="boxed" />
      </div>
    </div>
  );
}

export { patch, plural };
