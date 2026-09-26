import { h, useSubscribe } from "./sprout.js";

type Listener = () => void;
const listeners = new Set<Listener>();

export function subscribeRoute(fn: Listener) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

function emit() {
  for (const l of [...listeners]) l();
}

window.addEventListener("popstate", emit);

export function navigate(to: string, replace = false) {
  if (to === location.pathname + location.search) return;
  if (replace) history.replaceState(null, "", to);
  else history.pushState(null, "", to);
  window.scrollTo(0, 0);
  emit();
}

export function useLocation() {
  return useSubscribe(() => location.pathname + location.search, subscribeRoute);
}

export function query(): URLSearchParams {
  return new URLSearchParams(location.search);
}

export function setQuery(k: string, v: string | null) {
  const q = query();
  if (v == null || v === "") q.delete(k);
  else q.set(k, v);
  const s = q.toString();
  history.replaceState(null, "", location.pathname + (s ? "?" + s : ""));
  emit();
}

/** Match "/o/:org/apps/:id" against a path. */
export function match(pattern: string, path: string): Record<string, string> | null {
  const pp = pattern.split("/").filter(Boolean);
  const xs = path.split("?")[0].split("/").filter(Boolean);
  if (pp.length !== xs.length) return null;
  const out: Record<string, string> = {};
  for (let i = 0; i < pp.length; i++) {
    if (pp[i].startsWith(":")) out[pp[i].slice(1)] = decodeURIComponent(xs[i]);
    else if (pp[i] !== xs[i]) return null;
  }
  return out;
}

export function Link(props: { href: string; class?: string; children?: any; title?: string; onClick?: (e: Event) => void; style?: any; target?: string }) {
  return h("a", {
    ...props,
    onClick: (e: MouseEvent) => {
      props.onClick?.(e);
      if (e.defaultPrevented || props.target || e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
      if (!props.href.startsWith("/") || props.href.startsWith("/api/")) return;
      e.preventDefault();
      navigate(props.href);
    },
  });
}
