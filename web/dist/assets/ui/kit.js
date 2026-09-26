import { h, Fragment, useEffect, useRef, useState } from "../lib/sprout.js";
import { Store, use, toasts } from "../lib/state.js";
import { copyText } from "../lib/format.js";
import { Icon } from "./icons.js";
import { Link } from "../lib/router.js";
export function cx(...xs) {
    return xs.filter(Boolean).join(" ");
}
export function Button(p) {
    const [busy, setBusy] = useState(false);
    const cls = cx("btn", "btn-" + (p.kind || "secondary"), p.size === "sm" && "btn-sm", p.class, (busy || p.loading) && "is-busy");
    const inner = (h(Fragment, null,
        busy || p.loading ? h("span", { class: "spinner sm" }) : p.icon ? h(Icon, { name: p.icon, size: p.size === "sm" ? 14 : 16 }) : null,
        p.children != null && p.children !== "" ? h("span", null, p.children) : null,
        p.iconRight ? h(Icon, { name: p.iconRight, size: 14 }) : null));
    if (p.href) {
        if (p.href.startsWith("/") && !p.href.startsWith("/api/") && !p.target)
            return h(Link, { href: p.href, class: cls, title: p.title }, inner);
        return h("a", { href: p.href, class: cls, title: p.title, target: p.target, rel: p.target ? "noopener" : undefined }, inner);
    }
    return (h("button", { type: p.type || "button", class: cls, title: p.title, disabled: p.disabled || busy || p.loading, onClick: async (e) => {
            if (!p.onClick)
                return;
            const r = p.onClick(e);
            if (r && typeof r.then === "function") {
                setBusy(true);
                try {
                    await r;
                }
                finally {
                    setBusy(false);
                }
            }
        } }, inner));
}
export function IconButton(p) {
    if (p.href)
        return h(Link, { href: p.href, class: cx("icon-btn", p.class), title: p.title },
            h(Icon, { name: p.icon, size: p.size || 16 }));
    return (h("button", { type: "button", class: cx("icon-btn", p.class), title: p.title, "aria-label": p.title, onClick: p.onClick },
        h(Icon, { name: p.icon, size: p.size || 16 })));
}
const statusKinds = {
    running: "green", ready: "green", active: "green", succeeded: "green", completed: "green", issued: "green", healthy: "green",
    building: "blue", deploying: "blue", provisioning: "blue", booting: "blue", queued: "gray", pending: "gray", scheduled: "blue",
    degraded: "amber", stopped: "gray", paused: "gray", skipped: "gray", superseded: "gray", cancelled: "gray",
    failed: "red", crashing: "red", error: "red",
};
export function statusKind(s) {
    return statusKinds[s] || "gray";
}
export function Pill({ status, label, pulse }) {
    const k = statusKind(status);
    const live = pulse ?? (["building", "deploying", "provisioning", "booting", "running"].includes(status) && k === "blue");
    return (h("span", { class: cx("pill", "pill-" + k) },
        h("span", { class: cx("dot", live && "pulse") }),
        label || status[0].toUpperCase() + status.slice(1)));
}
export function Dot({ status }) {
    return h("span", { class: cx("status-dot", "dot-" + statusKind(status)) });
}
export function Tag({ children, mono, title }) {
    return h("span", { class: cx("tag", mono && "mono"), title: title }, children);
}
export function Kbd({ children }) {
    return h("kbd", { class: "kbd" }, children);
}
export function Spinner({ size }) {
    return h("span", { class: cx("spinner", size) });
}
export function Loading({ label }) {
    return (h("div", { class: "loading" },
        h(Spinner, null),
        " ",
        h("span", null, label || "Loading…")));
}
export function ErrorBox({ error, onRetry }) {
    return (h("div", { class: "callout callout-red" },
        h(Icon, { name: "alert" }),
        h("div", null,
            h("strong", null, "Something went wrong."),
            " ",
            error,
            onRetry ? h(Fragment, null,
                " ",
                h("a", { href: "#", onClick: (e) => { e.preventDefault(); onRetry(); } }, "Try again")) : null)));
}
export function Callout({ kind = "info", icon, children, title }) {
    return (h("div", { class: cx("callout", "callout-" + kind) },
        h(Icon, { name: icon || (kind === "red" || kind === "amber" ? "alert" : "info") }),
        h("div", null,
            title ? h("strong", { class: "callout-title" }, title) : null,
            children)));
}
export function Empty({ icon, title, children, action }) {
    return (h("div", { class: "empty" },
        h("div", { class: "empty-icon" },
            h(Icon, { name: icon || "sprout", size: 22 })),
        h("div", { class: "empty-title" }, title),
        children ? h("div", { class: "empty-body" }, children) : null,
        action ? h("div", { class: "empty-action" }, action) : null));
}
export function Card({ children, class: cls, pad = true, onClick, href }) {
    if (href)
        return h(Link, { href: href, class: cx("card", pad && "card-pad", "card-link", cls) }, children);
    return h("div", { class: cx("card", pad && "card-pad", cls, onClick && "card-link"), onClick: onClick }, children);
}
export function Section({ title, children, actions, description }) {
    return (h("section", { class: "section" },
        title || actions ? (h("div", { class: "section-head" },
            h("div", null,
                title ? h("h2", { class: "section-title" }, title) : null,
                description ? h("p", { class: "section-desc" }, description) : null),
            actions ? h("div", { class: "section-actions" }, actions) : null)) : null,
        children));
}
export function SectionLabel({ children }) {
    return h("div", { class: "section-label" }, children);
}
export function Tabs({ tabs, value, onChange, class: cls }) {
    return (h("div", { class: cx("tabs", cls), role: "tablist" }, tabs.map((t) => (h("button", { key: t.id, role: "tab", type: "button", class: cx("tab", t.id === value && "active"), onClick: () => onChange(t.id) },
        t.icon ? h(Icon, { name: t.icon, size: 15 }) : null,
        h("span", null, t.label),
        t.count != null ? h("span", { class: "tab-count" }, t.count) : null)))));
}
export function Segmented({ options, value, onChange, size }) {
    return (h("div", { class: cx("segmented", size === "sm" && "seg-sm") }, options.map((o) => (h("button", { key: o.id, type: "button", title: o.title, class: cx("seg", o.id === value && "active"), onClick: () => onChange(o.id) },
        o.icon ? h(Icon, { name: o.icon, size: 14 }) : null,
        o.label ? h("span", null, o.label) : null)))));
}
export function Field({ label, hint, children, error }) {
    return (h("label", { class: "field" },
        label ? h("span", { class: "field-label" }, label) : null,
        children,
        error ? h("span", { class: "field-error" }, error) : hint ? h("span", { class: "field-hint" }, hint) : null));
}
export function Input(p) {
    return (h("input", { class: cx("input", p.mono && "mono", p.class), type: p.type || "text", value: String(p.value ?? ""), placeholder: p.placeholder, autofocus: p.autofocus, disabled: p.disabled, min: p.min, max: p.max, step: p.step, name: p.name, autocomplete: p.autocomplete, spellcheck: p.spellcheck === false ? "false" : undefined, onInput: (e) => p.onInput(e.target.value), onKeyDown: (e) => {
            if (e.key === "Enter" && p.onEnter) {
                e.preventDefault();
                p.onEnter();
            }
        } }));
}
export function Textarea(p) {
    return (h("textarea", { class: cx("input textarea", p.mono && "mono", p.class), rows: p.rows || 4, placeholder: p.placeholder, value: p.value, spellcheck: p.spellcheck === false || p.mono ? "false" : undefined, onInput: (e) => p.onInput(e.target.value), onKeyDown: p.onKeyDown }));
}
export function Select(p) {
    return (h("div", { class: cx("select-wrap", p.class) },
        h("select", { class: "input select", value: p.value, disabled: p.disabled, onChange: (e) => p.onChange(e.target.value) }, p.options.map((o) => h("option", { key: o.value, value: o.value, selected: o.value === p.value }, o.label))),
        h(Icon, { name: "chevron-down", size: 14, class: "select-caret" })));
}
export function Toggle({ checked, onChange, label, hint, disabled }) {
    return (h("label", { class: cx("toggle-row", disabled && "disabled") },
        h("span", { class: "toggle-text" },
            label ? h("span", { class: "toggle-label" }, label) : null,
            hint ? h("span", { class: "field-hint" }, hint) : null),
        h("button", { type: "button", role: "switch", "aria-checked": checked ? "true" : "false", disabled: disabled, class: cx("toggle", checked && "on"), onClick: (e) => { e.preventDefault(); onChange(!checked); } },
            h("span", { class: "knob" }))));
}
export function Slider({ value, min, max, step = 1, onChange, onCommit }) {
    const pct = ((value - min) / (max - min)) * 100;
    return (h("input", { type: "range", class: "slider", min: min, max: max, step: step, value: String(value), style: { "--pct": pct + "%" }, onInput: (e) => onChange(Number(e.target.value)), onChange: (e) => onCommit?.(Number(e.target.value)) }));
}
export function CopyButton({ text, label, size }) {
    const [done, setDone] = useState(false);
    return (h("button", { type: "button", class: cx("btn btn-secondary copy-btn", size === "sm" && "btn-sm"), onClick: async () => {
            await copyText(text);
            setDone(true);
            setTimeout(() => setDone(false), 1400);
        } },
        h(Icon, { name: done ? "check" : "copy", size: 14 }),
        label !== "" ? h("span", null, done ? "Copied" : label || "Copy") : null));
}
export function Secret({ value, mono = true }) {
    const [shown, setShown] = useState(false);
    return (h("span", { class: cx("secret", mono && "mono") },
        h("span", null, shown ? value : "••••••••"),
        h("button", { type: "button", class: "icon-btn tiny", title: shown ? "Hide" : "Reveal", onClick: () => setShown(!shown) },
            h(Icon, { name: shown ? "eye-off" : "eye", size: 14 }))));
}
export function CodeBlock({ code, label, copy = true }) {
    return (h("div", { class: "codeblock" },
        label ? h("div", { class: "codeblock-head" },
            h("span", null, label),
            copy ? h(CopyButton, { text: code, size: "sm" }) : null) : null,
        h("pre", { class: "mono" }, code),
        !label && copy ? h("div", { class: "codeblock-copy" },
            h(CopyButton, { text: code, size: "sm", label: "" })) : null));
}
export const modals = new Store([]);
let modalSeq = 0;
export function openModal(render, opts = {}) {
    const id = ++modalSeq;
    modals.set((m) => [...m, { id, render, wide: opts.wide }]);
    return () => modals.set((m) => m.filter((x) => x.id !== id));
}
export function ModalHost() {
    const list = use(modals);
    useEffect(() => {
        const onKey = (e) => {
            if (e.key === "Escape" && modals.get().length)
                modals.set((m) => m.slice(0, -1));
        };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    }, []);
    return (h(Fragment, null, list.map((m) => {
        const close = () => modals.set((ms) => ms.filter((x) => x.id !== m.id));
        return (h("div", { key: m.id, class: "modal-backdrop", onMouseDown: (e) => { if (e.target === e.currentTarget)
                close(); } },
            h("div", { class: cx("modal", m.wide && "modal-wide"), role: "dialog" }, m.render(close))));
    })));
}
export function ModalHeader({ title, subtitle, onClose, icon }) {
    return (h("div", { class: "modal-head" },
        icon ? h("div", { class: "modal-icon" },
            h(Icon, { name: icon, size: 18 })) : null,
        h("div", { class: "modal-titles" },
            h("h3", null, title),
            subtitle ? h("p", null, subtitle) : null),
        h(IconButton, { icon: "x", title: "Close", onClick: onClose })));
}
export function confirm(opts) {
    return new Promise((resolve) => {
        openModal((close) => h(ConfirmBody, { opts: opts, done: (v) => { close(); resolve(v); } }));
    });
}
function ConfirmBody({ opts, done }) {
    const [typed, setTyped] = useState("");
    const ok = !opts.typeToConfirm || typed === opts.typeToConfirm;
    return (h(Fragment, null,
        h(ModalHeader, { title: opts.title, onClose: () => done(false), icon: opts.danger ? "alert" : undefined }),
        h("div", { class: "modal-body" },
            opts.body ? h("div", { class: "muted-body" }, opts.body) : null,
            opts.typeToConfirm ? (h(Field, { label: h(Fragment, null,
                    "Type ",
                    h("code", null, opts.typeToConfirm),
                    " to confirm") },
                h(Input, { value: typed, onInput: setTyped, autofocus: true, mono: true, onEnter: () => ok && done(true) }))) : null),
        h("div", { class: "modal-foot" },
            h(Button, { kind: "ghost", onClick: () => done(false) }, "Cancel"),
            h(Button, { kind: opts.danger ? "danger" : "primary", disabled: !ok, onClick: () => done(true) }, opts.confirm || "Confirm"))));
}
export function Menu({ trigger, children, align = "left", class: cls }) {
    const [open, setOpen] = useState(false);
    const ref = useRef(null);
    useEffect(() => {
        if (!open)
            return;
        const onDoc = (e) => {
            if (ref.current && !ref.current.contains(e.target))
                setOpen(false);
        };
        const onKey = (e) => e.key === "Escape" && setOpen(false);
        document.addEventListener("mousedown", onDoc);
        document.addEventListener("keydown", onKey);
        return () => {
            document.removeEventListener("mousedown", onDoc);
            document.removeEventListener("keydown", onKey);
        };
    }, [open]);
    return (h("div", { class: cx("menu-wrap", cls), ref: ref },
        trigger(open, () => setOpen(!open)),
        open ? h("div", { class: cx("menu", "menu-" + align) }, children(() => setOpen(false))) : null));
}
export function MenuItem({ icon, children, onClick, href, danger, hint }) {
    const inner = (h(Fragment, null,
        icon ? h(Icon, { name: icon, size: 15 }) : h("span", { class: "menu-noicon" }),
        h("span", { class: "menu-label" }, children),
        hint ? h("span", { class: "menu-hint" }, hint) : null));
    if (href)
        return h(Link, { href: href, class: cx("menu-item", danger && "danger"), onClick: onClick }, inner);
    return h("button", { type: "button", class: cx("menu-item", danger && "danger"), onClick: onClick }, inner);
}
export function MenuSep() {
    return h("div", { class: "menu-sep" });
}
export function Toasts() {
    const list = use(toasts);
    return (h("div", { class: "toasts", "aria-live": "polite" }, list.map((t) => (h("div", { key: t.id, class: cx("toast", "toast-" + t.kind) },
        h(Icon, { name: t.kind === "error" ? "alert" : t.kind === "ok" ? "check" : "info", size: 15 }),
        h("span", null, t.text))))));
}
export function Stat({ label, value, sub }) {
    return (h("div", { class: "stat" },
        h("div", { class: "stat-label" }, label),
        h("div", { class: "stat-value" }, value),
        sub ? h("div", { class: "stat-sub" }, sub) : null));
}
export function Meter({ used, cap, label, format }) {
    const pct = cap > 0 ? Math.min(100, (used / cap) * 100) : 0;
    const tone = pct > 90 ? "red" : pct > 70 ? "amber" : "ok";
    return (h("div", { class: "meter" },
        h("div", { class: "meter-top" },
            h("span", { class: "meter-label" }, label),
            h("span", { class: "meter-val" },
                h("strong", null, format(used)),
                " ",
                h("span", { class: "muted" },
                    "/ ",
                    cap > 0 ? format(cap) : "unlimited"))),
        h("div", { class: "meter-bar" },
            h("div", { class: cx("meter-fill", "tone-" + tone), style: { width: (cap > 0 ? pct : 0) + "%" } }))));
}
export function Chart({ series, height = 120, format, max: fixedMax, color = "var(--accent)" }) {
    const [hover, setHover] = useState(null);
    const W = 600, H = height;
    if (series.length < 2) {
        return h("div", { class: "chart-empty", style: { height } }, "Collecting samples\u2026");
    }
    const t0 = series[0].t, t1 = series[series.length - 1].t;
    const maxV = Math.max(fixedMax || 0, ...series.map((p) => p.v)) * 1.15 || 1;
    const x = (t) => ((t - t0) / Math.max(1, t1 - t0)) * W;
    const y = (v) => H - 4 - (v / maxV) * (H - 12);
    const line = series.map((p, i) => (i ? "L" : "M") + x(p.t).toFixed(1) + " " + y(p.v).toFixed(1)).join(" ");
    const area = line + ` L ${W} ${H} L 0 ${H} Z`;
    const hp = hover != null ? series[hover] : null;
    return (h("div", { class: "chart", style: { height } },
        h("svg", { viewBox: `0 0 ${W} ${H}`, preserveAspectRatio: "none", width: "100%", height: H, onMouseMove: (e) => {
                const r = e.currentTarget.getBoundingClientRect();
                const tx = t0 + ((e.clientX - r.left) / r.width) * (t1 - t0);
                let best = 0;
                for (let i = 0; i < series.length; i++)
                    if (Math.abs(series[i].t - tx) < Math.abs(series[best].t - tx))
                        best = i;
                setHover(best);
            }, onMouseLeave: () => setHover(null) },
            [0.25, 0.5, 0.75].map((f) => h("line", { key: f, x1: "0", x2: W, y1: H * f, y2: H * f, class: "chart-grid" })),
            h("path", { d: area, fill: color, opacity: "0.12" }),
            h("path", { d: line, fill: "none", stroke: color, "stroke-width": "2", "vector-effect": "non-scaling-stroke", "stroke-linejoin": "round" }),
            hp ? h("line", { x1: x(hp.t), x2: x(hp.t), y1: "0", y2: H, class: "chart-cursor" }) : null),
        h("div", { class: "chart-label" }, hp ? h(Fragment, null,
            format(hp.v),
            " ",
            h("span", { class: "muted" },
                "\u00B7 ",
                new Date(hp.t).toLocaleTimeString())) : h(Fragment, null,
            format(series[series.length - 1].v),
            " ",
            h("span", { class: "muted" }, "now")))));
}
export function Sparkline({ values, width = 80, height = 22 }) {
    if (values.length < 2)
        return null;
    const max = Math.max(...values) || 1;
    const d = values.map((v, i) => (i ? "L" : "M") + ((i / (values.length - 1)) * width).toFixed(1) + " " + (height - 2 - (v / max) * (height - 4)).toFixed(1)).join(" ");
    return h("svg", { width: width, height: height, class: "sparkline" },
        h("path", { d: d, fill: "none", stroke: "currentColor", "stroke-width": "1.5" }));
}
export function Table({ head, children, class: cls }) {
    return (h("div", { class: cx("table-wrap", cls) },
        h("table", { class: "table" },
            h("thead", null,
                h("tr", null, head.map((c, i) => h("th", { key: i }, c)))),
            h("tbody", null, children))));
}
export function DefList({ rows }) {
    return (h("div", { class: "deflist" }, rows.map(([k, v], i) => (h("div", { class: "def-row", key: i },
        h("div", { class: "def-k" }, k),
        h("div", { class: "def-v" }, v))))));
}
