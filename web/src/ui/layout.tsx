import { h, Fragment, useEffect, useRef, useState, Child } from "../lib/sprout.js";
import { Link, navigate, useLocation } from "../lib/router.js";
import { session, use, sidebarCollapsed, palette, theme, Theme, rememberOrg, OrgRef } from "../lib/state.js";
import { get, post } from "../lib/api.js";
import { Icon } from "./icons.js";
import { Avatar, Logo } from "./art.js";
import { cx, Kbd, Menu, MenuItem, MenuSep, openModal, Dot } from "./kit.js";

export function useOrg(slug: string): OrgRef | undefined {
  const s = use(session);
  const org = s.me?.orgs.find((o) => o.slug === slug || o.id === slug);
  useEffect(() => {
    if (org) rememberOrg(org.slug);
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

function activeSection(path: string): string {
  const m = path.match(/^\/o\/[^/]+(\/[^/?]+)?/);
  const seg = m?.[1] || "";
  if (seg === "/jobs" || seg === "/runs") return "jobs";
  if (seg === "/usage") return "usage";
  if (seg === "/greenhouse") return "greenhouse";
  if (seg === "/grove") return "grove";
  if (seg === "/settings") return "settings";
  if (seg === "/members") return "members";
  if (path.startsWith("/account")) return "account";
  if (path.startsWith("/admin")) return "admin";
  return "apps";
}

export function Shell({ org, children }: { org: string; children: Child }) {
  const s = use(session);
  const collapsed = use(sidebarCollapsed);
  const path = useLocation();
  const [mobileOpen, setMobileOpen] = useState(false);
  const me = s.me!;
  const current = me.orgs.find((o) => o.slug === org) || me.orgs[0];
  const section = activeSection(path);
  useEffect(() => setMobileOpen(false), [path]);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
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
  return (
    <div class="shell">
      <aside class={cx("sidebar", collapsed && "collapsed", mobileOpen && "mobile-open")}>
        <Menu
          class="org-menu"
          trigger={(open, toggle) => (
            <button type="button" class="org-switch" onClick={toggle} title="Switch organization">
              <Logo size={36} />
              <span class="org-name hide-collapsed">{current?.name.replace(/'s Orchard$/, "") === me.name ? "Orchard" : current?.name || "Orchard"}</span>
              <Icon name="chevrons-up-down" size={15} class="muted hide-collapsed" />
            </button>
          )}
        >
          {(close) => (
            <>
              <div class="menu-heading">Organizations</div>
              {me.orgs.map((o) => (
                <MenuItem key={o.id} icon={o.slug === current?.slug ? "check" : "building"} href={"/o/" + o.slug} onClick={close} hint={o.viaSuperadmin ? "admin" : o.role}>
                  {o.name}
                </MenuItem>
              ))}
              <MenuSep />
              <MenuItem icon="plus" onClick={() => { close(); newOrgModal(); }}>New organization</MenuItem>
              {isAdmin ? <MenuItem icon="settings" href={base + "/settings"} onClick={close}>Organization settings</MenuItem> : null}
            </>
          )}
        </Menu>
        <button type="button" class="search-btn" onClick={() => palette.set(true)} title="Search (⌘K)">
          <Icon name="search" size={16} />
          <span class="hide-collapsed">Search</span>
          <span class="hide-collapsed" style={{ marginLeft: "auto" }}><Kbd>⌘K</Kbd></span>
        </button>
        <div class="nav-label hide-collapsed">Platform</div>
        <nav class="nav">
          {nav.map((n) => (
            <Link key={n.id} href={base + n.path} class={cx("nav-item", section === n.id && "active")} title={n.label}>
              <Icon name={n.icon} size={18} />
              <span class="hide-collapsed">{n.label}</span>
            </Link>
          ))}
        </nav>
        <div class="nav-label hide-collapsed">Organization</div>
        <nav class="nav">
          <Link href={base + "/members"} class={cx("nav-item", section === "members" && "active")} title="Members">
            <Icon name="users" size={18} />
            <span class="hide-collapsed">Members</span>
          </Link>
          <Link href={base + "/settings"} class={cx("nav-item", section === "settings" && "active")} title="Settings">
            <Icon name="settings" size={18} />
            <span class="hide-collapsed">Settings</span>
          </Link>
          {me.superadmin ? (
            <Link href="/admin" class={cx("nav-item", section === "admin" && "active")} title="Instance admin">
              <Icon name="shield" size={18} />
              <span class="hide-collapsed">Instance admin</span>
            </Link>
          ) : null}
        </nav>
        <div class="sidebar-foot">
          <Menu
            align="left"
            class="grow"
            trigger={(open, toggle) => (
              <button type="button" class="user-chip" onClick={toggle} title={me.name}>
                <Avatar seed={me.avatarSeed} size={30} />
                <span class="user-name hide-collapsed">{me.username}</span>
              </button>
            )}
          >
            {(close) => <UserMenu close={close} />}
          </Menu>
          <button type="button" class="icon-btn" title={collapsed ? "Expand sidebar" : "Collapse sidebar"} onClick={() => sidebarCollapsed.set(!collapsed)}>
            <Icon name="panel-left" size={16} />
          </button>
        </div>
      </aside>
      <main class="main">
        <div class="mobile-bar">
          <button type="button" class="icon-btn boxed" onClick={() => setMobileOpen(true)} title="Menu"><Icon name="list" /></button>
          <Logo size={28} />
          <strong>Wack Club Orchard</strong>
        </div>
        {s.auth?.demo ? (
          <div class="demo-banner">
            <Icon name="sparkles" size={14} />
            <span>Running on the <strong>simulated runtime</strong>: nothing is scheduled on a real cluster. Point the server at a Kubernetes API to go live.</span>
          </div>
        ) : null}
        {s.auth?.needsCredential ? (
          <div class="demo-banner" style={{ background: "var(--red-soft)" }}>
            <Icon name="key" size={14} />
            <span>Add a passkey or password now: the claim link was single use. <Link href="/account">Secure your account →</Link></span>
          </div>
        ) : null}
        {children}
      </main>
      {mobileOpen ? <div class="modal-backdrop" style={{ zIndex: 15 }} onClick={() => setMobileOpen(false)} /> : null}
      <Palette org={current?.slug || org} />
    </div>
  );
}

function UserMenu({ close }: { close: () => void }) {
  const t = use(theme);
  const s = use(session);
  const setTheme = (x: Theme) => theme.set(x);
  return (
    <>
      <div class="menu-heading">{s.me?.name}</div>
      <MenuItem icon="user" href="/account" onClick={close}>Account</MenuItem>
      {s.me?.superadmin ? <MenuItem icon="shield" href="/admin" onClick={close}>Instance admin</MenuItem> : null}
      <MenuSep />
      <div class="menu-heading">Theme</div>
      <div style={{ padding: "2px 8px 6px" }}>
        <div class="segmented seg-sm" style={{ width: "100%" }}>
          {(["light", "dark", "system"] as Theme[]).map((x) => (
            <button key={x} type="button" class={cx("seg", t === x && "active")} style={{ flex: 1, justifyContent: "center" }} onClick={() => setTheme(x)}>
              <Icon name={x === "light" ? "sun" : x === "dark" ? "moon" : "monitor"} size={13} />
            </button>
          ))}
        </div>
      </div>
      <MenuSep />
      <MenuItem icon="logout" onClick={async () => { close(); await post("/auth/logout"); location.href = "/login"; }}>Sign out</MenuItem>
    </>
  );
}

import { Button, Field, Input, ModalHeader } from "./kit.js";
import { act } from "../lib/api.js";
import { loadSession } from "../lib/state.js";

export function newOrgModal() {
  openModal((close) => <NewOrg close={close} />);
}

function NewOrg({ close }: { close: () => void }) {
  const [name, setName] = useState("");
  const submit = async () => {
    const o = await act(() => post("/orgs", { name }));
    if (o) {
      await loadSession();
      close();
      navigate("/o/" + o.slug);
    }
  };
  return (
    <>
      <ModalHeader title="New organization" subtitle="Organizations hold members, quotas and projects. Each project gets its own namespace." onClose={close} icon="building" />
      <div class="modal-body">
        <Field label="Name"><Input value={name} onInput={setName} autofocus placeholder="Wack Club" onEnter={submit} /></Field>
      </div>
      <div class="modal-foot">
        <Button kind="ghost" onClick={close}>Cancel</Button>
        <Button kind="primary" onClick={submit} disabled={!name.trim()}>Create</Button>
      </div>
    </>
  );
}

// ---- command palette ----

interface Hit {
  kind: string;
  id: string;
  title: string;
  subtitle: string;
  href: string;
  status?: string;
}

function Palette({ org }: { org: string }) {
  const open = use(palette);
  if (!open) return null;
  return (
    <div class="modal-backdrop" onMouseDown={(e: MouseEvent) => { if (e.target === e.currentTarget) palette.set(false); }}>
      <div class="modal palette"><PaletteBody org={org} /></div>
    </div>
  );
}

function PaletteBody({ org }: { org: string }) {
  const [q, setQ] = useState("");
  const [hits, setHits] = useState<Hit[]>([]);
  const [sel, setSel] = useState(0);
  const listRef = useRef<HTMLElement | null>(null);
  const base = "/o/" + org;
  const actions: Hit[] = [
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
        const r = await get<Hit[]>("/search?q=" + encodeURIComponent(q));
        if (alive) {
          setHits(r);
          setSel(0);
        }
      } catch {}
    }, 90);
    return () => {
      alive = false;
      clearTimeout(t);
    };
  }, [q]);
  const lower = q.toLowerCase();
  const all = [...hits, ...actions.filter((a) => !q || a.title.toLowerCase().includes(lower))];
  const go = (hh: Hit) => {
    palette.set(false);
    navigate(hh.href);
  };
  return (
    <>
      <div class="palette-input">
        <Icon name="search" size={18} class="muted" />
        <input
          autofocus
          placeholder="Search apps, databases, jobs, projects…"
          value={q}
          onInput={(e: any) => setQ(e.target.value)}
          onKeyDown={(e: KeyboardEvent) => {
            if (e.key === "Escape") palette.set(false);
            else if (e.key === "ArrowDown") {
              e.preventDefault();
              setSel(Math.min(all.length - 1, sel + 1));
            } else if (e.key === "ArrowUp") {
              e.preventDefault();
              setSel(Math.max(0, sel - 1));
            } else if (e.key === "Enter" && all[sel]) go(all[sel]);
          }}
        />
        <Kbd>esc</Kbd>
      </div>
      <div class="palette-list" ref={listRef}>
        {all.length === 0 ? <div class="muted" style={{ padding: 16, textAlign: "center" }}>Nothing matches “{q}”.</div> : null}
        {all.map((hh, i) => (
          <a key={hh.kind + hh.id} href={hh.href} class={cx("palette-item", i === sel && "sel")} onMouseEnter={() => setSel(i)} onClick={(e: Event) => { e.preventDefault(); go(hh); }}>
            <span class="palette-kind">{hh.kind}</span>
            <Icon name={{ app: "box", database: "database", job: "zap", project: "folder", new: "plus", go: "arrow-right" }[hh.kind] || "box"} size={16} class="muted" />
            <span class="grow truncate"><strong style={{ fontWeight: 500 }}>{hh.title}</strong> <span class="muted">{hh.subtitle}</span></span>
            {hh.status ? <Dot status={hh.status} /> : null}
          </a>
        ))}
      </div>
      <div class="palette-foot"><span><Kbd>↑↓</Kbd> move</span><span><Kbd>↵</Kbd> open</span><span><Kbd>⌘K</Kbd> toggle</span></div>
    </>
  );
}
