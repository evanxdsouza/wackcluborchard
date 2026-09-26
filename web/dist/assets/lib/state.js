import { useSubscribe } from "./sprout.js";
export class Store {
    value;
    listeners = new Set();
    constructor(value) {
        this.value = value;
    }
    get = () => this.value;
    set = (v) => {
        this.value = typeof v === "function" ? v(this.value) : v;
        for (const l of [...this.listeners])
            l();
    };
    subscribe = (fn) => {
        this.listeners.add(fn);
        return () => {
            this.listeners.delete(fn);
        };
    };
}
export function use(s) {
    return useSubscribe(s.get, s.subscribe);
}
export const session = new Store({ loaded: false });
export async function loadSession() {
    const auth = await fetch("/api/auth/state").then((r) => r.json());
    let me;
    if (auth.user) {
        me = await fetch("/api/me").then((r) => (r.ok ? r.json() : undefined));
    }
    session.set({ loaded: true, auth, me });
}
export const toasts = new Store([]);
let toastSeq = 0;
export function toast(text, kind = "info") {
    const id = ++toastSeq;
    toasts.set((ts) => [...ts, { id, text, kind }]);
    setTimeout(() => toasts.set((ts) => ts.filter((t) => t.id !== id)), kind === "error" ? 6500 : 3500);
}
function readTheme() {
    try {
        return localStorage.getItem("orchard-theme") || "system";
    }
    catch {
        return "system";
    }
}
export const theme = new Store(readTheme());
export function applyTheme() {
    const t = theme.get();
    const dark = t === "dark" || (t === "system" && matchMedia("(prefers-color-scheme: dark)").matches);
    document.documentElement.dataset.theme = dark ? "dark" : "light";
}
theme.subscribe(() => {
    try {
        localStorage.setItem("orchard-theme", theme.get());
    }
    catch { }
    applyTheme();
});
matchMedia("(prefers-color-scheme: dark)").addEventListener("change", applyTheme);
function readCollapsed() {
    try {
        return localStorage.getItem("orchard-sidebar") === "collapsed";
    }
    catch {
        return false;
    }
}
export const sidebarCollapsed = new Store(readCollapsed());
sidebarCollapsed.subscribe(() => {
    try {
        localStorage.setItem("orchard-sidebar", sidebarCollapsed.get() ? "collapsed" : "open");
    }
    catch { }
});
export const palette = new Store(false);
export function currentOrgSlug() {
    const m = location.pathname.match(/^\/o\/([^/]+)/);
    if (m)
        return decodeURIComponent(m[1]);
    try {
        return localStorage.getItem("orchard-org");
    }
    catch {
        return null;
    }
}
export function rememberOrg(slug) {
    try {
        localStorage.setItem("orchard-org", slug);
    }
    catch { }
}
