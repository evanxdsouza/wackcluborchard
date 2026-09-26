import { h, Fragment, useEffect, useRef, useState, Child } from "../lib/sprout.js";
import { Store, use, toasts } from "../lib/state.js";
import { copyText } from "../lib/format.js";
import { Icon } from "./icons.js";
import { Link } from "../lib/router.js";

export function cx(...xs: any[]) {
  return xs.filter(Boolean).join(" ");
}

// ---- buttons ----

export function Button(p: {
  children?: Child;
  onClick?: (e: MouseEvent) => any;
  kind?: "primary" | "secondary" | "ghost" | "danger" | "soft";
  size?: "sm" | "md";
  icon?: string;
  iconRight?: string;
  disabled?: boolean;
  type?: string;
  href?: string;
  title?: string;
  class?: string;
  loading?: boolean;
  target?: string;
}) {
  const [busy, setBusy] = useState(false);
  const cls = cx("btn", "btn-" + (p.kind || "secondary"), p.size === "sm" && "btn-sm", p.class, (busy || p.loading) && "is-busy");
  const inner = (
    <>
      {busy || p.loading ? <span class="spinner sm" /> : p.icon ? <Icon name={p.icon} size={p.size === "sm" ? 14 : 16} /> : null}
      {p.children != null && p.children !== "" ? <span>{p.children}</span> : null}
      {p.iconRight ? <Icon name={p.iconRight} size={14} /> : null}
    </>
  );
  if (p.href) {
    if (p.href.startsWith("/") && !p.href.startsWith("/api/") && !p.target) return <Link href={p.href} class={cls} title={p.title}>{inner}</Link>;
    return <a href={p.href} class={cls} title={p.title} target={p.target} rel={p.target ? "noopener" : undefined}>{inner}</a>;
  }
  return (
    <button
      type={p.type || "button"}
      class={cls}
      title={p.title}
      disabled={p.disabled || busy || p.loading}
      onClick={async (e: MouseEvent) => {
        if (!p.onClick) return;
        const r = p.onClick(e);
        if (r && typeof r.then === "function") {
          setBusy(true);
          try {
            await r;
          } finally {
            setBusy(false);
          }
        }
      }}
    >
      {inner}
    </button>
  );
}

export function IconButton(p: { icon: string; onClick?: (e: MouseEvent) => any; title: string; class?: string; size?: number; href?: string }) {
  if (p.href) return <Link href={p.href} class={cx("icon-btn", p.class)} title={p.title}><Icon name={p.icon} size={p.size || 16} /></Link>;
  return (
    <button type="button" class={cx("icon-btn", p.class)} title={p.title} aria-label={p.title} onClick={p.onClick}>
      <Icon name={p.icon} size={p.size || 16} />
    </button>
  );
}

// ---- status ----

const statusKinds: Record<string, string> = {
  running: "green", ready: "green", active: "green", succeeded: "green", completed: "green", issued: "green", healthy: "green",
  building: "blue", deploying: "blue", provisioning: "blue", booting: "blue", queued: "gray", pending: "gray", scheduled: "blue",
  degraded: "amber", stopped: "gray", paused: "gray", skipped: "gray", superseded: "gray", cancelled: "gray",
  failed: "red", crashing: "red", error: "red",
};

export function statusKind(s: string) {
  return statusKinds[s] || "gray";
}

export function Pill({ status, label, pulse }: { status: string; label?: string; pulse?: boolean }) {
  const k = statusKind(status);
  const live = pulse ?? (["building", "deploying", "provisioning", "booting", "running"].includes(status) && k === "blue");
  return (
    <span class={cx("pill", "pill-" + k)}>
      <span class={cx("dot", live && "pulse")} />
      {label || status[0].toUpperCase() + status.slice(1)}
    </span>
  );
}

export function Dot({ status }: { status: string }) {
  return <span class={cx("status-dot", "dot-" + statusKind(status))} />;
}

export function Tag({ children, mono, title }: { children: Child; mono?: boolean; title?: string }) {
  return <span class={cx("tag", mono && "mono")} title={title}>{children}</span>;
}

export function Kbd({ children }: { children: Child }) {
  return <kbd class="kbd">{children}</kbd>;
}

export function Spinner({ size }: { size?: "sm" | "lg" }) {
  return <span class={cx("spinner", size)} />;
}

export function Loading({ label }: { label?: string }) {
  return (
    <div class="loading">
      <Spinner /> <span>{label || "Loading…"}</span>
    </div>
  );
}

export function ErrorBox({ error, onRetry }: { error: string; onRetry?: () => void }) {
  return (
    <div class="callout callout-red">
      <Icon name="alert" />
      <div>
        <strong>Something went wrong.</strong> {error}
        {onRetry ? <> <a href="#" onClick={(e: Event) => { e.preventDefault(); onRetry(); }}>Try again</a></> : null}
      </div>
    </div>
  );
}

export function Callout({ kind = "info", icon, children, title }: { kind?: "info" | "red" | "amber" | "green"; icon?: string; children: Child; title?: string }) {
  return (
    <div class={cx("callout", "callout-" + kind)}>
      <Icon name={icon || (kind === "red" || kind === "amber" ? "alert" : "info")} />
      <div>
        {title ? <strong class="callout-title">{title}</strong> : null}
        {children}
      </div>
    </div>
  );
}

export function Empty({ icon, title, children, action }: { icon?: string; title: string; children?: Child; action?: Child }) {
  return (
    <div class="empty">
      <div class="empty-icon"><Icon name={icon || "sprout"} size={22} /></div>
      <div class="empty-title">{title}</div>
      {children ? <div class="empty-body">{children}</div> : null}
      {action ? <div class="empty-action">{action}</div> : null}
    </div>
  );
}

// ---- layout ----

export function Card({ children, class: cls, pad = true, onClick, href }: { children: Child; class?: string; pad?: boolean; onClick?: () => void; href?: string }) {
  if (href) return <Link href={href} class={cx("card", pad && "card-pad", "card-link", cls)}>{children}</Link>;
  return <div class={cx("card", pad && "card-pad", cls, onClick && "card-link")} onClick={onClick}>{children}</div>;
}

export function Section({ title, children, actions, description }: { title?: Child; children: Child; actions?: Child; description?: Child }) {
  return (
    <section class="section">
      {title || actions ? (
        <div class="section-head">
          <div>
            {title ? <h2 class="section-title">{title}</h2> : null}
            {description ? <p class="section-desc">{description}</p> : null}
          </div>
          {actions ? <div class="section-actions">{actions}</div> : null}
        </div>
      ) : null}
      {children}
    </section>
  );
}

export function SectionLabel({ children }: { children: Child }) {
  return <div class="section-label">{children}</div>;
}

export function Tabs<T extends string>({ tabs, value, onChange, class: cls }: { tabs: { id: T; label: Child; icon?: string; count?: number }[]; value: T; onChange: (t: T) => void; class?: string }) {
  return (
    <div class={cx("tabs", cls)} role="tablist">
      {tabs.map((t) => (
        <button key={t.id} role="tab" type="button" class={cx("tab", t.id === value && "active")} onClick={() => onChange(t.id)}>
          {t.icon ? <Icon name={t.icon} size={15} /> : null}
          <span>{t.label}</span>
          {t.count != null ? <span class="tab-count">{t.count}</span> : null}
        </button>
      ))}
    </div>
  );
}

export function Segmented<T extends string>({ options, value, onChange, size }: { options: { id: T; label: Child; icon?: string; title?: string }[]; value: T; onChange: (v: T) => void; size?: "sm" }) {
  return (
    <div class={cx("segmented", size === "sm" && "seg-sm")}>
      {options.map((o) => (
        <button key={o.id} type="button" title={o.title} class={cx("seg", o.id === value && "active")} onClick={() => onChange(o.id)}>
          {o.icon ? <Icon name={o.icon} size={14} /> : null}
          {o.label ? <span>{o.label}</span> : null}
        </button>
      ))}
    </div>
  );
}

// ---- forms ----

export function Field({ label, hint, children, error }: { label?: Child; hint?: Child; children: Child; error?: string }) {
  return (
    <label class="field">
      {label ? <span class="field-label">{label}</span> : null}
      {children}
      {error ? <span class="field-error">{error}</span> : hint ? <span class="field-hint">{hint}</span> : null}
    </label>
  );
}

export function Input(p: { value: string | number; onInput: (v: string) => void; placeholder?: string; type?: string; mono?: boolean; autofocus?: boolean; disabled?: boolean; min?: number; max?: number; step?: number; onEnter?: () => void; class?: string; spellcheck?: boolean; autocomplete?: string; name?: string }) {
  return (
    <input
      class={cx("input", p.mono && "mono", p.class)}
      type={p.type || "text"}
      value={String(p.value ?? "")}
      placeholder={p.placeholder}
      autofocus={p.autofocus}
      disabled={p.disabled}
      min={p.min}
      max={p.max}
      step={p.step}
      name={p.name}
      autocomplete={p.autocomplete}
      spellcheck={p.spellcheck === false ? "false" : undefined}
      onInput={(e: any) => p.onInput(e.target.value)}
      onKeyDown={(e: KeyboardEvent) => {
        if (e.key === "Enter" && p.onEnter) {
          e.preventDefault();
          p.onEnter();
        }
      }}
    />
  );
}

export function Textarea(p: { value: string; onInput: (v: string) => void; placeholder?: string; rows?: number; mono?: boolean; class?: string; spellcheck?: boolean; onKeyDown?: (e: KeyboardEvent) => void }) {
  return (
    <textarea
      class={cx("input textarea", p.mono && "mono", p.class)}
      rows={p.rows || 4}
      placeholder={p.placeholder}
      value={p.value}
      spellcheck={p.spellcheck === false || p.mono ? "false" : undefined}
      onInput={(e: any) => p.onInput(e.target.value)}
      onKeyDown={p.onKeyDown}
    />
  );
}

export function Select<T extends string>(p: { value: T; onChange: (v: T) => void; options: { value: T; label: string }[]; class?: string; disabled?: boolean }) {
  return (
    <div class={cx("select-wrap", p.class)}>
      <select class="input select" value={p.value} disabled={p.disabled} onChange={(e: any) => p.onChange(e.target.value)}>
        {p.options.map((o) => <option key={o.value} value={o.value} selected={o.value === p.value}>{o.label}</option>)}
      </select>
      <Icon name="chevron-down" size={14} class="select-caret" />
    </div>
  );
}

export function Toggle({ checked, onChange, label, hint, disabled }: { checked: boolean; onChange: (v: boolean) => void; label?: Child; hint?: Child; disabled?: boolean }) {
  return (
    <label class={cx("toggle-row", disabled && "disabled")}>
      <span class="toggle-text">
        {label ? <span class="toggle-label">{label}</span> : null}
        {hint ? <span class="field-hint">{hint}</span> : null}
      </span>
      <button type="button" role="switch" aria-checked={checked ? "true" : "false"} disabled={disabled} class={cx("toggle", checked && "on")} onClick={(e: Event) => { e.preventDefault(); onChange(!checked); }}>
        <span class="knob" />
      </button>
    </label>
  );
}

export function Slider({ value, min, max, step = 1, onChange, onCommit }: { value: number; min: number; max: number; step?: number; onChange: (v: number) => void; onCommit?: (v: number) => void }) {
  const pct = ((value - min) / (max - min)) * 100;
  return (
    <input
      type="range"
      class="slider"
      min={min}
      max={max}
      step={step}
      value={String(value)}
      style={{ "--pct": pct + "%" }}
      onInput={(e: any) => onChange(Number(e.target.value))}
      onChange={(e: any) => onCommit?.(Number(e.target.value))}
    />
  );
}

// ---- copy / code ----

export function CopyButton({ text, label, size }: { text: string; label?: string; size?: "sm" }) {
  const [done, setDone] = useState(false);
  return (
    <button
      type="button"
      class={cx("btn btn-secondary copy-btn", size === "sm" && "btn-sm")}
      onClick={async () => {
        await copyText(text);
        setDone(true);
        setTimeout(() => setDone(false), 1400);
      }}
    >
      <Icon name={done ? "check" : "copy"} size={14} />
      {label !== "" ? <span>{done ? "Copied" : label || "Copy"}</span> : null}
    </button>
  );
}

export function Secret({ value, mono = true }: { value: string; mono?: boolean }) {
  const [shown, setShown] = useState(false);
  return (
    <span class={cx("secret", mono && "mono")}>
      <span>{shown ? value : "••••••••"}</span>
      <button type="button" class="icon-btn tiny" title={shown ? "Hide" : "Reveal"} onClick={() => setShown(!shown)}>
        <Icon name={shown ? "eye-off" : "eye"} size={14} />
      </button>
    </span>
  );
}

export function CodeBlock({ code, label, copy = true }: { code: string; label?: string; copy?: boolean }) {
  return (
    <div class="codeblock">
      {label ? <div class="codeblock-head"><span>{label}</span>{copy ? <CopyButton text={code} size="sm" /> : null}</div> : null}
      <pre class="mono">{code}</pre>
      {!label && copy ? <div class="codeblock-copy"><CopyButton text={code} size="sm" label="" /></div> : null}
    </div>
  );
}

// ---- overlays ----

interface ModalState {
  id: number;
  render: (close: () => void) => Child;
  wide?: boolean;
}
export const modals = new Store<ModalState[]>([]);
let modalSeq = 0;

export function openModal(render: (close: () => void) => Child, opts: { wide?: boolean } = {}) {
  const id = ++modalSeq;
  modals.set((m) => [...m, { id, render, wide: opts.wide }]);
  return () => modals.set((m) => m.filter((x) => x.id !== id));
}

export function ModalHost() {
  const list = use(modals);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && modals.get().length) modals.set((m) => m.slice(0, -1));
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  return (
    <>
      {list.map((m) => {
        const close = () => modals.set((ms) => ms.filter((x) => x.id !== m.id));
        return (
          <div key={m.id} class="modal-backdrop" onMouseDown={(e: MouseEvent) => { if (e.target === e.currentTarget) close(); }}>
            <div class={cx("modal", m.wide && "modal-wide")} role="dialog">{m.render(close)}</div>
          </div>
        );
      })}
    </>
  );
}

export function ModalHeader({ title, subtitle, onClose, icon }: { title: Child; subtitle?: Child; onClose: () => void; icon?: string }) {
  return (
    <div class="modal-head">
      {icon ? <div class="modal-icon"><Icon name={icon} size={18} /></div> : null}
      <div class="modal-titles">
        <h3>{title}</h3>
        {subtitle ? <p>{subtitle}</p> : null}
      </div>
      <IconButton icon="x" title="Close" onClick={onClose} />
    </div>
  );
}

export function confirm(opts: { title: string; body?: Child; confirm?: string; danger?: boolean; typeToConfirm?: string }): Promise<boolean> {
  return new Promise((resolve) => {
    openModal((close) => <ConfirmBody opts={opts} done={(v) => { close(); resolve(v); }} />);
  });
}

function ConfirmBody({ opts, done }: { opts: { title: string; body?: Child; confirm?: string; danger?: boolean; typeToConfirm?: string }; done: (v: boolean) => void }) {
  const [typed, setTyped] = useState("");
  const ok = !opts.typeToConfirm || typed === opts.typeToConfirm;
  return (
    <>
      <ModalHeader title={opts.title} onClose={() => done(false)} icon={opts.danger ? "alert" : undefined} />
      <div class="modal-body">
        {opts.body ? <div class="muted-body">{opts.body}</div> : null}
        {opts.typeToConfirm ? (
          <Field label={<>Type <code>{opts.typeToConfirm}</code> to confirm</>}>
            <Input value={typed} onInput={setTyped} autofocus mono onEnter={() => ok && done(true)} />
          </Field>
        ) : null}
      </div>
      <div class="modal-foot">
        <Button kind="ghost" onClick={() => done(false)}>Cancel</Button>
        <Button kind={opts.danger ? "danger" : "primary"} disabled={!ok} onClick={() => done(true)}>{opts.confirm || "Confirm"}</Button>
      </div>
    </>
  );
}

// ---- menus ----

export function Menu({ trigger, children, align = "left", class: cls }: { trigger: (open: boolean, toggle: () => void) => Child; children: (close: () => void) => Child; align?: "left" | "right"; class?: string }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLElement | null>(null);
  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);
  return (
    <div class={cx("menu-wrap", cls)} ref={ref}>
      {trigger(open, () => setOpen(!open))}
      {open ? <div class={cx("menu", "menu-" + align)}>{children(() => setOpen(false))}</div> : null}
    </div>
  );
}

export function MenuItem({ icon, children, onClick, href, danger, hint }: { icon?: string; children: Child; onClick?: () => void; href?: string; danger?: boolean; hint?: Child }) {
  const inner = (
    <>
      {icon ? <Icon name={icon} size={15} /> : <span class="menu-noicon" />}
      <span class="menu-label">{children}</span>
      {hint ? <span class="menu-hint">{hint}</span> : null}
    </>
  );
  if (href) return <Link href={href} class={cx("menu-item", danger && "danger")} onClick={onClick}>{inner}</Link>;
  return <button type="button" class={cx("menu-item", danger && "danger")} onClick={onClick}>{inner}</button>;
}

export function MenuSep() {
  return <div class="menu-sep" />;
}

// ---- toasts ----

export function Toasts() {
  const list = use(toasts);
  return (
    <div class="toasts" aria-live="polite">
      {list.map((t) => (
        <div key={t.id} class={cx("toast", "toast-" + t.kind)}>
          <Icon name={t.kind === "error" ? "alert" : t.kind === "ok" ? "check" : "info"} size={15} />
          <span>{t.text}</span>
        </div>
      ))}
    </div>
  );
}

// ---- data display ----

export function Stat({ label, value, sub }: { label: Child; value: Child; sub?: Child }) {
  return (
    <div class="stat">
      <div class="stat-label">{label}</div>
      <div class="stat-value">{value}</div>
      {sub ? <div class="stat-sub">{sub}</div> : null}
    </div>
  );
}

export function Meter({ used, cap, label, format }: { used: number; cap: number; label: Child; format: (n: number) => string }) {
  const pct = cap > 0 ? Math.min(100, (used / cap) * 100) : 0;
  const tone = pct > 90 ? "red" : pct > 70 ? "amber" : "ok";
  return (
    <div class="meter">
      <div class="meter-top">
        <span class="meter-label">{label}</span>
        <span class="meter-val"><strong>{format(used)}</strong> <span class="muted">/ {cap > 0 ? format(cap) : "unlimited"}</span></span>
      </div>
      <div class="meter-bar"><div class={cx("meter-fill", "tone-" + tone)} style={{ width: (cap > 0 ? pct : 0) + "%" }} /></div>
    </div>
  );
}

/** Minimal SVG line/area chart. */
export function Chart({ series, height = 120, format, max: fixedMax, color = "var(--accent)" }: { series: { t: number; v: number }[]; height?: number; format: (v: number) => string; max?: number; color?: string }) {
  const [hover, setHover] = useState<number | null>(null);
  const W = 600, H = height;
  if (series.length < 2) {
    return <div class="chart-empty" style={{ height }}>Collecting samples…</div>;
  }
  const t0 = series[0].t, t1 = series[series.length - 1].t;
  const maxV = Math.max(fixedMax || 0, ...series.map((p) => p.v)) * 1.15 || 1;
  const x = (t: number) => ((t - t0) / Math.max(1, t1 - t0)) * W;
  const y = (v: number) => H - 4 - (v / maxV) * (H - 12);
  const line = series.map((p, i) => (i ? "L" : "M") + x(p.t).toFixed(1) + " " + y(p.v).toFixed(1)).join(" ");
  const area = line + ` L ${W} ${H} L 0 ${H} Z`;
  const hp = hover != null ? series[hover] : null;
  return (
    <div class="chart" style={{ height }}>
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" width="100%" height={H}
        onMouseMove={(e: MouseEvent) => {
          const r = (e.currentTarget as SVGElement).getBoundingClientRect();
          const tx = t0 + ((e.clientX - r.left) / r.width) * (t1 - t0);
          let best = 0;
          for (let i = 0; i < series.length; i++) if (Math.abs(series[i].t - tx) < Math.abs(series[best].t - tx)) best = i;
          setHover(best);
        }}
        onMouseLeave={() => setHover(null)}>
        {[0.25, 0.5, 0.75].map((f) => <line key={f} x1="0" x2={W} y1={H * f} y2={H * f} class="chart-grid" />)}
        <path d={area} fill={color} opacity="0.12" />
        <path d={line} fill="none" stroke={color} stroke-width="2" vector-effect="non-scaling-stroke" stroke-linejoin="round" />
        {hp ? <line x1={x(hp.t)} x2={x(hp.t)} y1="0" y2={H} class="chart-cursor" /> : null}
      </svg>
      <div class="chart-label">{hp ? <>{format(hp.v)} <span class="muted">· {new Date(hp.t).toLocaleTimeString()}</span></> : <>{format(series[series.length - 1].v)} <span class="muted">now</span></>}</div>
    </div>
  );
}

export function Sparkline({ values, width = 80, height = 22 }: { values: number[]; width?: number; height?: number }) {
  if (values.length < 2) return null;
  const max = Math.max(...values) || 1;
  const d = values.map((v, i) => (i ? "L" : "M") + ((i / (values.length - 1)) * width).toFixed(1) + " " + (height - 2 - (v / max) * (height - 4)).toFixed(1)).join(" ");
  return <svg width={width} height={height} class="sparkline"><path d={d} fill="none" stroke="currentColor" stroke-width="1.5" /></svg>;
}

export function Table({ head, children, class: cls }: { head: Child[]; children: Child; class?: string }) {
  return (
    <div class={cx("table-wrap", cls)}>
      <table class="table">
        <thead><tr>{head.map((c, i) => <th key={i}>{c}</th>)}</tr></thead>
        <tbody>{children}</tbody>
      </table>
    </div>
  );
}

export function DefList({ rows }: { rows: [Child, Child][] }) {
  return (
    <div class="deflist">
      {rows.map(([k, v], i) => (
        <div class="def-row" key={i}>
          <div class="def-k">{k}</div>
          <div class="def-v">{v}</div>
        </div>
      ))}
    </div>
  );
}
