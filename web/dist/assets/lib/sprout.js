export const Fragment = Symbol.for("sprout.fragment");
const TEXT = "#text";
export function h(type, props, ...children) {
    props = props ? { ...props } : {};
    const key = props.key;
    delete props.key;
    if (children.length)
        props.children = children.length === 1 ? children[0] : children;
    return { type, props, key };
}
function normalize(c, out = []) {
    if (c == null || c === false || c === true)
        return out;
    if (Array.isArray(c)) {
        for (const x of c)
            normalize(x, out);
        return out;
    }
    if (typeof c === "object") {
        out.push(c);
        return out;
    }
    out.push({ type: TEXT, props: { nodeValue: String(c) }, key: undefined });
    return out;
}
let current = null;
let hookIndex = 0;
function hook() {
    const inst = current;
    if (!inst)
        throw new Error("hooks can only be called inside components");
    if (!inst.hooks[hookIndex])
        inst.hooks[hookIndex] = {};
    return inst.hooks[hookIndex++];
}
export function useState(init) {
    const hk = hook();
    const inst = current;
    if (!("value" in hk)) {
        hk.value = [typeof init === "function" ? init() : init, (v) => {
                const next = typeof v === "function" ? v(hk.value[0]) : v;
                if (Object.is(next, hk.value[0]))
                    return;
                hk.value = [next, hk.value[1]];
                schedule(inst);
            }];
    }
    return hk.value;
}
export function useReducer(reducer, init) {
    const [s, set] = useState(init);
    return [s, (a) => set((prev) => reducer(prev, a))];
}
function depsChanged(a, b) {
    return !a || !b || a.length !== b.length || a.some((x, i) => !Object.is(x, b[i]));
}
export function useEffect(effect, deps) {
    const hk = hook();
    if (depsChanged(hk.deps, deps)) {
        hk.deps = deps;
        hk.effect = effect;
        hk.pending = true;
        pendingEffects.add(current);
    }
}
export function useRef(init) {
    const hk = hook();
    if (!("value" in hk))
        hk.value = { current: init };
    return hk.value;
}
export function useMemo(fn, deps) {
    const hk = hook();
    if (depsChanged(hk.deps, deps)) {
        hk.deps = deps;
        hk.value = fn();
    }
    return hk.value;
}
export function useCallback(fn, deps) {
    return useMemo(() => fn, deps);
}
export function useSubscribe(get, subscribe) {
    const [, force] = useState(0);
    useEffect(() => subscribe(() => force((n) => n + 1)), []);
    return get();
}
const dirty = new Set();
const pendingEffects = new Set();
let scheduled = false;
function schedule(inst) {
    if (inst.unmounted)
        return;
    inst.dirty = true;
    dirty.add(inst);
    if (!scheduled) {
        scheduled = true;
        queueMicrotask(flush);
    }
}
function flush() {
    scheduled = false;
    const list = [...dirty].sort((a, b) => a.depth - b.depth);
    dirty.clear();
    for (const inst of list) {
        if (inst.dirty && !inst.unmounted)
            rerender(inst);
    }
    runEffects();
}
function runEffects() {
    const list = [...pendingEffects];
    pendingEffects.clear();
    for (const inst of list) {
        if (inst.unmounted)
            continue;
        for (const hk of inst.hooks) {
            if (hk.pending) {
                hk.pending = false;
                if (typeof hk.cleanup === "function")
                    hk.cleanup();
                try {
                    hk.cleanup = hk.effect();
                }
                catch (e) {
                    console.error(e);
                }
            }
        }
    }
}
function rerender(inst) {
    const v = inst.vnode;
    const nodes = domNodes(v);
    const first = nodes[0];
    const parent = first?.parentNode;
    if (!parent)
        return;
    const anchor = nodes[nodes.length - 1].nextSibling;
    renderComponent(v, inst, parent, anchor);
}
function domNodes(v, out = []) {
    if (v._dom) {
        out.push(v._dom);
        return out;
    }
    if (v._kids && v._kids.length) {
        for (const k of v._kids)
            domNodes(k, out);
    }
    else if (v._placeholder) {
        out.push(v._placeholder);
    }
    return out;
}
function renderComponent(v, inst, parentDom, anchor) {
    inst.vnode = v;
    inst.dirty = false;
    const prev = current;
    const prevIdx = hookIndex;
    current = inst;
    hookIndex = 0;
    let out;
    try {
        out = v.type(v.props);
    }
    catch (e) {
        console.error(e);
        out = h("div", { class: "render-error" }, "Something broke rendering this part of the page: " + String(e.message));
    }
    finally {
        current = prev;
        hookIndex = prevIdx;
    }
    const kids = normalize(out);
    diffChildren(parentDom, kids, v._kids || [], anchor, inst.depth + 1, inst.svg, v);
    v._kids = kids;
}
function diffChildren(parentDom, next, prev, anchor, depth, svg, owner) {
    const keyed = new Map();
    const unkeyed = [];
    for (const o of prev) {
        if (o.key != null)
            keyed.set(o.key, o);
        else
            unkeyed.push(o);
    }
    let ui = 0;
    const used = new Set();
    for (let i = 0; i < next.length; i++) {
        const n = next[i];
        let o;
        if (n.key != null) {
            o = keyed.get(n.key);
            if (o && o.type !== n.type)
                o = undefined;
        }
        else {
            while (ui < unkeyed.length && used.has(unkeyed[ui]))
                ui++;
            if (ui < unkeyed.length && unkeyed[ui].type === n.type) {
                o = unkeyed[ui];
                ui++;
            }
        }
        if (o)
            used.add(o);
        if (o === n) {
            next[i] = { ...n };
        }
        diff(parentDom, next[i], o, depth, svg);
    }
    for (const o of prev)
        if (!used.has(o))
            unmount(o, true);
    let a = anchor;
    for (let i = next.length - 1; i >= 0; i--) {
        const nodes = domNodes(next[i]);
        for (let j = nodes.length - 1; j >= 0; j--) {
            const node = nodes[j];
            if (node.parentNode !== parentDom || node.nextSibling !== a)
                parentDom.insertBefore(node, a);
            a = node;
        }
    }
    if (owner && next.length === 0) {
        if (!owner._placeholder)
            owner._placeholder = document.createTextNode("");
        if (owner._placeholder.parentNode !== parentDom || owner._placeholder.nextSibling !== anchor)
            parentDom.insertBefore(owner._placeholder, anchor);
    }
    else if (owner && owner._placeholder) {
        owner._placeholder.remove();
        owner._placeholder = undefined;
    }
}
function diff(parentDom, n, o, depth, svg) {
    const t = n.type;
    if (typeof t === "function") {
        let inst = o?._c;
        if (!inst) {
            inst = { vnode: n, hooks: [], parentDom, dirty: false, unmounted: false, depth, svg };
        }
        n._c = inst;
        n._kids = o?._kids;
        n._placeholder = o?._placeholder;
        let anchor = null;
        if (o) {
            const nodes = domNodes(o);
            if (nodes.length)
                anchor = nodes[nodes.length - 1].nextSibling;
            if (nodes[0]?.parentNode)
                parentDom = nodes[0].parentNode;
        }
        renderComponent(n, inst, parentDom, anchor);
        return;
    }
    if (t === Fragment) {
        n._kids = o?._kids;
        n._placeholder = o?._placeholder;
        let anchor = null;
        if (o) {
            const nodes = domNodes(o);
            if (nodes.length)
                anchor = nodes[nodes.length - 1].nextSibling;
        }
        const kids = normalize(n.props.children);
        diffChildren(parentDom, kids, o?._kids || [], anchor, depth, svg, n);
        n._kids = kids;
        return;
    }
    if (t === TEXT) {
        if (o && o._dom) {
            n._dom = o._dom;
            if (o.props.nodeValue !== n.props.nodeValue)
                n._dom.nodeValue = n.props.nodeValue;
        }
        else {
            n._dom = document.createTextNode(n.props.nodeValue);
        }
        return;
    }
    const isSvg = svg || t === "svg";
    let dom = o?._dom;
    if (!dom) {
        dom = isSvg ? document.createElementNS("http://www.w3.org/2000/svg", t) : document.createElement(t);
    }
    n._dom = dom;
    setProps(dom, n.props, o ? o.props : {}, isSvg);
    if (n.props.innerHTML == null) {
        const kids = normalize(n.props.children);
        diffChildren(dom, kids, o?._kids || [], null, depth, isSvg && t !== "foreignObject");
        n._kids = kids;
        if (t === "select" && n.props.value != null)
            dom.value = n.props.value;
    }
    const ref = n.props.ref;
    if (ref) {
        if (typeof ref === "function")
            ref(dom);
        else
            ref.current = dom;
    }
}
const listenerKey = "__sprout";
function setProps(dom, props, old, svg) {
    for (const k in old) {
        if (k === "children" || k === "ref" || k in props)
            continue;
        setProp(dom, k, undefined, old[k], svg);
    }
    for (const k in props) {
        if (k === "children" || k === "ref")
            continue;
        if (k === "value" || k === "checked" ? dom[k] !== props[k] : old[k] !== props[k])
            setProp(dom, k, props[k], old[k], svg);
    }
}
function setProp(dom, k, v, ov, svg) {
    if (k.startsWith("on") && k.length > 2) {
        const name = k.slice(2).toLowerCase();
        const store = (dom[listenerKey] ||= {});
        if (!store[name] && v)
            dom.addEventListener(name, (e) => store[name]?.(e));
        store[name] = v;
        return;
    }
    if (k === "style") {
        if (typeof v === "string") {
            dom.style.cssText = v;
            return;
        }
        if (typeof ov === "string")
            dom.style.cssText = "";
        const o = typeof ov === "object" && ov ? ov : {};
        const nv = v || {};
        for (const s in o)
            if (!(s in nv))
                setStyle(dom.style, s, "");
        for (const s in nv)
            if (o[s] !== nv[s])
                setStyle(dom.style, s, nv[s]);
        return;
    }
    if (k === "class" || k === "className") {
        const cls = Array.isArray(v) ? v.filter(Boolean).join(" ") : typeof v === "object" && v ? Object.keys(v).filter((x) => v[x]).join(" ") : v || "";
        if (svg)
            dom.setAttribute("class", cls);
        else
            dom.className = cls;
        return;
    }
    if (k === "innerHTML") {
        if (dom.innerHTML !== v)
            dom.innerHTML = v ?? "";
        return;
    }
    if (!svg && (k === "value" || k === "checked" || k === "selected" || k === "disabled" || k === "indeterminate" || k === "readOnly" || k === "multiple")) {
        dom[k] = k === "value" ? (v ?? "") : !!v;
        return;
    }
    if (k === "autofocus") {
        if (v)
            queueMicrotask(() => dom.focus());
        return;
    }
    if (v == null || v === false)
        dom.removeAttribute(k === "htmlFor" ? "for" : k);
    else
        dom.setAttribute(k === "htmlFor" ? "for" : k, v === true ? "" : String(v));
}
const unitless = new Set(["opacity", "zIndex", "flex", "flexGrow", "flexShrink", "fontWeight", "lineHeight", "order", "zoom", "gridRow", "gridColumn"]);
function setStyle(style, k, v) {
    if (k.startsWith("--"))
        style.setProperty(k, v);
    else
        style[k] = typeof v === "number" && !unitless.has(k) ? v + "px" : v;
}
function unmount(v, remove) {
    if (v._c) {
        v._c.unmounted = true;
        for (const hk of v._c.hooks)
            if (typeof hk.cleanup === "function")
                hk.cleanup();
    }
    const ref = v.props?.ref;
    if (ref && v._dom) {
        if (typeof ref === "function")
            ref(null);
        else
            ref.current = null;
    }
    if (v._kids)
        for (const k of v._kids)
            unmount(k, remove && !v._dom);
    if (remove) {
        if (v._dom)
            v._dom.remove();
        if (v._placeholder)
            v._placeholder.remove();
    }
}
const roots = new WeakMap();
export function render(v, container) {
    const next = h(Fragment, null, v);
    const prev = roots.get(container);
    diff(container, next, prev, 0, false);
    const nodes = domNodes(next);
    for (const n of nodes)
        if (n.parentNode !== container)
            container.appendChild(n);
    roots.set(container, next);
    runEffects();
}
