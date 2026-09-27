// Global app state: session, current org, toasts, theme. Small observable
// stores that components subscribe to with useSubscribe.

import { useSubscribe } from "./sprout.js";

export class Store<T> {
  private listeners = new Set<() => void>();
  constructor(private value: T) {}
  get = () => this.value;
  set = (v: T | ((p: T) => T)) => {
    this.value = typeof v === "function" ? (v as any)(this.value) : v;
    for (const l of [...this.listeners]) l();
  };
  subscribe = (fn: () => void) => {
    this.listeners.add(fn);
    return () => {
      this.listeners.delete(fn);
    };
  };
}

export function use<T>(s: Store<T>): T {
  return useSubscribe(s.get, s.subscribe);
}

export interface OrgRef {
  id: string;
  slug: string;
  name: string;
  role: string;
  viaSuperadmin?: boolean;
}

export interface Me {
  id: string;
  username: string;
  name: string;
  email?: string;
  superadmin: boolean;
  avatarSeed: string;
  githubLogin?: string;
  hasPassword: boolean;
  passkeys: { id: string; name: string; createdAt: string }[];
  sshKeys: { id: string; name: string; fingerprint: string; createdAt: string }[];
  orgs: OrgRef[];
}

export interface AuthState {
  user: Me | null;
  needsClaim: boolean;
  firstUser: boolean;
  signupMode: string;
  instanceName: string;
  github: boolean;
  sso: boolean;
  demo: boolean;
  runtime: string;
  version: string;
  appDomain: string;
  httpsPort: number;
  needsCredential: boolean;
  secureContext: boolean;
}

export const session = new Store<{ loaded: boolean; auth?: AuthState; me?: Me }>({ loaded: false });

export async function loadSession() {
  const auth: AuthState = await fetch("/api/auth/state").then((r) => r.json());
  let me: Me | undefined;
  if (auth.user) {
    me = await fetch("/api/me").then((r) => (r.ok ? r.json() : undefined));
  }
  session.set({ loaded: true, auth, me });
}

// ---- toasts ----

export interface Toast {
  id: number;
  text: string;
  kind: "ok" | "error" | "info";
}
export const toasts = new Store<Toast[]>([]);
let toastSeq = 0;

export function toast(text: string, kind: Toast["kind"] = "info") {
  const id = ++toastSeq;
  toasts.set((ts) => [...ts, { id, text, kind }]);
  setTimeout(() => toasts.set((ts) => ts.filter((t) => t.id !== id)), kind === "error" ? 6500 : 3500);
}

// ---- theme ----

export type Theme = "light" | "dark" | "system";
function readTheme(): Theme {
  try {
    return (localStorage.getItem("wackcluborchard-theme") as Theme) || "system";
  } catch {
    return "system";
  }
}
export const theme = new Store<Theme>(readTheme());
export function applyTheme() {
  const t = theme.get();
  const dark = t === "dark" || (t === "system" && matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.dataset.theme = dark ? "dark" : "light";
}
theme.subscribe(() => {
  try {
    localStorage.setItem("wackcluborchard-theme", theme.get());
  } catch {}
  applyTheme();
});
matchMedia("(prefers-color-scheme: dark)").addEventListener("change", applyTheme);

// ---- sidebar ----

function readCollapsed() {
  try {
    return localStorage.getItem("wackcluborchard-sidebar") === "collapsed";
  } catch {
    return false;
  }
}
export const sidebarCollapsed = new Store<boolean>(readCollapsed());
sidebarCollapsed.subscribe(() => {
  try {
    localStorage.setItem("wackcluborchard-sidebar", sidebarCollapsed.get() ? "collapsed" : "open");
  } catch {}
});

export const palette = new Store<boolean>(false);

export function currentOrgSlug(): string | null {
  const m = location.pathname.match(/^\/o\/([^/]+)/);
  if (m) return decodeURIComponent(m[1]);
  try {
    return localStorage.getItem("wackcluborchard-org");
  } catch {
    return null;
  }
}

export function rememberOrg(slug: string) {
  try {
    localStorage.setItem("wackcluborchard-org", slug);
  } catch {}
}
