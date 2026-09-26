import { h, useSubscribe } from "./sprout.js";
const listeners = new Set();
export function subscribeRoute(fn) {
    listeners.add(fn);
    return () => listeners.delete(fn);
}
function emit() {
    for (const l of [...listeners])
        l();
}
window.addEventListener("popstate", emit);
export function navigate(to, replace = false) {
    if (to === location.pathname + location.search)
        return;
    if (replace)
        history.replaceState(null, "", to);
    else
        history.pushState(null, "", to);
    window.scrollTo(0, 0);
    emit();
}
export function useLocation() {
    return useSubscribe(() => location.pathname + location.search, subscribeRoute);
}
export function query() {
    return new URLSearchParams(location.search);
}
export function setQuery(k, v) {
    const q = query();
    if (v == null || v === "")
        q.delete(k);
    else
        q.set(k, v);
    const s = q.toString();
    history.replaceState(null, "", location.pathname + (s ? "?" + s : ""));
    emit();
}
export function match(pattern, path) {
    const pp = pattern.split("/").filter(Boolean);
    const xs = path.split("?")[0].split("/").filter(Boolean);
    if (pp.length !== xs.length)
        return null;
    const out = {};
    for (let i = 0; i < pp.length; i++) {
        if (pp[i].startsWith(":"))
            out[pp[i].slice(1)] = decodeURIComponent(xs[i]);
        else if (pp[i] !== xs[i])
            return null;
    }
    return out;
}
export function Link(props) {
    return h("a", {
        ...props,
        onClick: (e) => {
            props.onClick?.(e);
            if (e.defaultPrevented || props.target || e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0)
                return;
            if (!props.href.startsWith("/") || props.href.startsWith("/api/"))
                return;
            e.preventDefault();
            navigate(props.href);
        },
    });
}
