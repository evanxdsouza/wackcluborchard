import { useEffect, useRef, useState } from "./sprout.js";
import { toast } from "./state.js";

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

export async function api<T = any>(method: string, path: string, body?: any): Promise<T> {
  const res = await fetch("/api" + path, {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json" } : {},
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: "same-origin",
  });
  const text = await res.text();
  let data: any = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = { error: text };
  }
  if (!res.ok) {
    throw new ApiError(res.status, data?.error || res.statusText);
  }
  return data as T;
}

export const get = <T = any>(p: string) => api<T>("GET", p);
export const post = <T = any>(p: string, b: any = {}) => api<T>("POST", p, b);
export const put = <T = any>(p: string, b: any = {}) => api<T>("PUT", p, b);
export const patch = <T = any>(p: string, b: any = {}) => api<T>("PATCH", p, b);
export const del = <T = any>(p: string) => api<T>("DELETE", p);

/** Run an action, toasting failures. Returns undefined on error. */
export async function act<T>(fn: () => Promise<T>, success?: string): Promise<T | undefined> {
  try {
    const r = await fn();
    if (success) toast(success, "ok");
    return r;
  } catch (e) {
    toast((e as Error).message, "error");
    return undefined;
  }
}

export interface Resource<T> {
  data: T | undefined;
  error: string | undefined;
  loading: boolean;
  reload: () => Promise<void>;
  set: (fn: (d: T) => T) => void;
}

/** Fetch a GET endpoint and keep it in component state. */
export function useApi<T = any>(path: string | null, deps: any[] = []): Resource<T> {
  const [state, setState] = useState<{ data?: T; error?: string; loading: boolean }>({ loading: !!path });
  const seq = useRef(0);
  const load = async () => {
    if (!path) return;
    const n = ++seq.current;
    try {
      const d = await get<T>(path);
      if (n === seq.current) setState({ data: d, loading: false });
    } catch (e) {
      if (n === seq.current) setState((s) => ({ ...s, error: (e as Error).message, loading: false }));
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

export interface BusEvent {
  id: number;
  topic: string;
  type: string;
  data: any;
}

/** Subscribe to server-sent bus events for some topics. */
export function useEvents(topics: (string | null | undefined)[], onEvent: (e: BusEvent) => void) {
  const handler = useRef(onEvent);
  handler.current = onEvent;
  const key = topics.filter(Boolean).join(",");
  useEffect(() => {
    if (!key) return;
    const es = new EventSource("/api/events?topics=" + encodeURIComponent(key));
    const on = (m: MessageEvent) => {
      try {
        handler.current(JSON.parse(m.data));
      } catch {}
    };
    es.onmessage = on;
    for (const t of ["app.updated", "app.deleted", "database.updated", "database.deleted", "deploy.updated", "run.updated", "metrics", "crash", "event", "sandbox.updated", "conversation.updated", "template.created", "project.updated"]) {
      es.addEventListener(t, on as any);
    }
    return () => es.close();
  }, [key]);
}

/** Stream an SSE endpoint of log lines. */
export function useLogStream(path: string | null, max = 3000) {
  const [lines, setLines] = useState<{ pod?: string; text: string; at?: string }[]>([]);
  const [ended, setEnded] = useState(false);
  useEffect(() => {
    setLines([]);
    setEnded(false);
    if (!path) return;
    const es = new EventSource("/api" + path);
    let buf: any[] = [];
    let timer: any = null;
    const flushBuf = () => {
      timer = null;
      const b = buf;
      buf = [];
      setLines((ls) => {
        const n = ls.concat(b);
        return n.length > max ? n.slice(n.length - max) : n;
      });
    };
    es.addEventListener("log", (m: MessageEvent) => {
      buf.push(JSON.parse(m.data));
      if (!timer) timer = setTimeout(flushBuf, 60);
    });
    es.addEventListener("end", () => {
      if (timer) clearTimeout(timer);
      flushBuf();
      setEnded(true);
      es.close();
    });
    es.onerror = () => {
      if (es.readyState === EventSource.CLOSED) setEnded(true);
    };
    return () => {
      es.close();
      if (timer) clearTimeout(timer);
    };
  }, [path]);
  return { lines, ended };
}
