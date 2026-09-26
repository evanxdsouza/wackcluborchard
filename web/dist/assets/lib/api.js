import { useEffect, useRef, useState } from "./sprout.js";
import { toast } from "./state.js";
export class ApiError extends Error {
    status;
    constructor(status, message) {
        super(message);
        this.status = status;
    }
}
export async function api(method, path, body) {
    const res = await fetch("/api" + path, {
        method,
        headers: body !== undefined ? { "Content-Type": "application/json" } : {},
        body: body !== undefined ? JSON.stringify(body) : undefined,
        credentials: "same-origin",
    });
    const text = await res.text();
    let data = null;
    try {
        data = text ? JSON.parse(text) : null;
    }
    catch {
        data = { error: text };
    }
    if (!res.ok) {
        throw new ApiError(res.status, data?.error || res.statusText);
    }
    return data;
}
export const get = (p) => api("GET", p);
export const post = (p, b = {}) => api("POST", p, b);
export const put = (p, b = {}) => api("PUT", p, b);
export const patch = (p, b = {}) => api("PATCH", p, b);
export const del = (p) => api("DELETE", p);
export async function act(fn, success) {
    try {
        const r = await fn();
        if (success)
            toast(success, "ok");
        return r;
    }
    catch (e) {
        toast(e.message, "error");
        return undefined;
    }
}
export function useApi(path, deps = []) {
    const [state, setState] = useState({ loading: !!path });
    const seq = useRef(0);
    const load = async () => {
        if (!path)
            return;
        const n = ++seq.current;
        try {
            const d = await get(path);
            if (n === seq.current)
                setState({ data: d, loading: false });
        }
        catch (e) {
            if (n === seq.current)
                setState((s) => ({ ...s, error: e.message, loading: false }));
        }
    };
    useEffect(() => {
        setState((s) => ({ data: s.data, loading: true }));
        load();
    }, [path, ...deps]);
    return {
        data: state.data,
        error: state.error,
        loading: state.loading,
        reload: load,
        set: (fn) => setState((s) => ({ ...s, data: s.data !== undefined ? fn(s.data) : s.data })),
    };
}
export function useEvents(topics, onEvent) {
    const handler = useRef(onEvent);
    handler.current = onEvent;
    const key = topics.filter(Boolean).join(",");
    useEffect(() => {
        if (!key)
            return;
        const es = new EventSource("/api/events?topics=" + encodeURIComponent(key));
        const on = (m) => {
            try {
                handler.current(JSON.parse(m.data));
            }
            catch { }
        };
        es.onmessage = on;
        for (const t of ["app.updated", "app.deleted", "database.updated", "database.deleted", "deploy.updated", "run.updated", "metrics", "crash", "event", "sandbox.updated", "conversation.updated", "template.created", "project.updated"]) {
            es.addEventListener(t, on);
        }
        return () => es.close();
    }, [key]);
}
export function useLogStream(path, max = 3000) {
    const [lines, setLines] = useState([]);
    const [ended, setEnded] = useState(false);
    useEffect(() => {
        setLines([]);
        setEnded(false);
        if (!path)
            return;
        const es = new EventSource("/api" + path);
        let buf = [];
        let timer = null;
        const flushBuf = () => {
            timer = null;
            const b = buf;
            buf = [];
            setLines((ls) => {
                const n = ls.concat(b);
                return n.length > max ? n.slice(n.length - max) : n;
            });
        };
        es.addEventListener("log", (m) => {
            buf.push(JSON.parse(m.data));
            if (!timer)
                timer = setTimeout(flushBuf, 60);
        });
        es.addEventListener("end", () => {
            if (timer)
                clearTimeout(timer);
            flushBuf();
            setEnded(true);
            es.close();
        });
        es.onerror = () => {
            if (es.readyState === EventSource.CLOSED)
                setEnded(true);
        };
        return () => {
            es.close();
            if (timer)
                clearTimeout(timer);
        };
    }, [path]);
    return { lines, ended };
}
