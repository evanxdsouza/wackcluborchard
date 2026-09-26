import { h, Fragment, useEffect, useRef, useState, Child } from "../lib/sprout.js";
import { useLogStream } from "../lib/api.js";
import { Icon } from "./icons.js";
import { cx } from "./kit.js";

const ansiColors: Record<string, string> = { "30": "gray", "31": "red", "32": "green", "33": "yellow", "34": "blue", "35": "magenta", "36": "cyan", "37": "", "90": "gray", "91": "red", "92": "green", "93": "yellow", "94": "blue", "95": "magenta", "96": "cyan" };

/** Render a line with basic SGR color codes. */
export function Ansi({ text }: { text: string }) {
  if (!text.includes("\x1b[")) return <>{text}</>;
  const out: Child[] = [];
  let cls: string[] = [];
  const re = /\x1b\[([0-9;]*)m/g;
  let last = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text))) {
    if (m.index > last) out.push(<span class={cls.join(" ")}>{text.slice(last, m.index)}</span>);
    for (const code of m[1].split(";")) {
      if (code === "0" || code === "") cls = [];
      else if (code === "1") cls.push("ansi-bold");
      else if (ansiColors[code]) cls = cls.filter((c) => !c.startsWith("ansi-") || c === "ansi-bold").concat("ansi-" + ansiColors[code]);
    }
    last = re.lastIndex;
  }
  if (last < text.length) out.push(<span class={cls.join(" ")}>{text.slice(last)}</span>);
  return <>{out}</>;
}

function stripControl(s: string) {
  // drop cursor movement and other non-color escapes from interactive shells
  return s.replace(/\x1b\[[0-9;?]*[ABCDEFGHJKSTfhlnsu]/g, "").replace(/\x1b\][^\x07]*\x07/g, "").replace(/\r(?!\n)/g, "");
}

export function useAutoScroll(dep: any) {
  const ref = useRef<HTMLElement | null>(null);
  const stick = useRef(true);
  useEffect(() => {
    const el = ref.current;
    if (el && stick.current) el.scrollTop = el.scrollHeight;
  }, [dep]);
  const onScroll = () => {
    const el = ref.current;
    if (el) stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
  };
  return { ref, onScroll };
}

/** Live logs for an app: filter by pod and time, or read the dead container. */
export function LogView({ appId, pods, height }: { appId: string; pods: { name: string }[]; height?: number }) {
  const [pod, setPod] = useState("");
  const [since, setSince] = useState("");
  const [previous, setPrevious] = useState(false);
  const [filter, setFilter] = useState("");
  const [nonce, setNonce] = useState(0);
  const params = new URLSearchParams();
  if (pod) params.set("pod", pod);
  if (since) params.set("since", since);
  if (previous) params.set("previous", "1");
  params.set("n", String(nonce));
  const { lines, ended } = useLogStream(`/apps/${appId}/logs?${params}`);
  const shown = filter ? lines.filter((l) => l.text.toLowerCase().includes(filter.toLowerCase())) : lines;
  const scroll = useAutoScroll(shown.length);
  const multi = pods.length > 1;
  return (
    <div class="term" style={height ? { height } : undefined}>
      <div class="term-head">
        <span>{previous ? "previous container" : "logs"}</span>
        <span class="grow" />
        <input class="term-filter" placeholder="filter" value={filter} onInput={(e: any) => setFilter(e.target.value)}
          style={{ background: "#151518", border: "1px solid #2a2a2f", borderRadius: 6, color: "#ddd", height: 24, padding: "0 8px", width: 120, fontFamily: "var(--mono)", fontSize: 11 }} />
        {multi ? (
          <select value={pod} onChange={(e: any) => setPod(e.target.value)} title="Pod">
            <option value="">all pods</option>
            {pods.map((p) => <option key={p.name} value={p.name} selected={p.name === pod}>{p.name.split("-").slice(-1)[0]}</option>)}
          </select>
        ) : null}
        <select value={since} onChange={(e: any) => setSince(e.target.value)} title="Time range">
          <option value="">all</option>
          <option value="5m" selected={since === "5m"}>5 min</option>
          <option value="15m" selected={since === "15m"}>15 min</option>
          <option value="1h" selected={since === "1h"}>1 hour</option>
        </select>
        <button type="button" class={cx("icon-btn", previous && "active")} title="Previous container (the one that died)" onClick={() => setPrevious(!previous)} style={previous ? { color: "#ff8a8a" } : undefined}>
          <Icon name="skull" size={14} />
        </button>
        <button type="button" class="icon-btn" title="Reconnect" onClick={() => setNonce(nonce + 1)}>
          <Icon name="restart" size={14} />
        </button>
      </div>
      <div class="term-body" ref={scroll.ref} onScroll={scroll.onScroll}>
        {shown.length === 0 ? <div class="term-empty">{ended ? "No output." : "Waiting for output…"}</div> : null}
        {shown.map((l, i) => (
          <div class="term-line" key={i}>
            {multi && !pod && l.pod ? <span class="pod">{l.pod.split("-").slice(-1)[0]}</span> : null}
            <span><Ansi text={l.text} /></span>
          </div>
        ))}
      </div>
    </div>
  );
}

/** Static output block, like a job step's stdout. */
export function OutputView({ text, title = "stdout", onRefresh, height }: { text: string; title?: string; onRefresh?: () => void; height?: number }) {
  const scroll = useAutoScroll(text.length);
  return (
    <div class="term" style={height ? { maxHeight: height } : undefined}>
      <div class="term-head">
        <span>{title}</span>
        <span class="grow" />
        {onRefresh ? <button type="button" class="icon-btn" title="Refresh" onClick={onRefresh}><Icon name="restart" size={14} /></button> : null}
      </div>
      <div class="term-body" ref={scroll.ref} onScroll={scroll.onScroll}>
        {text ? text.replace(/\n$/, "").split("\n").map((l, i) => <div key={i}><Ansi text={l} /></div>) : <span class="term-empty">No output.</span>}
      </div>
    </div>
  );
}

/** Streamed build log for a deploy. */
export function DeployLog({ deployId, height = 420 }: { deployId: string; height?: number }) {
  const { lines, ended } = useLogStream(`/deploys/${deployId}/logs`);
  const scroll = useAutoScroll(lines.length);
  return (
    <div class="term" style={{ height }}>
      <div class="term-head"><span>build log</span><span class="grow" />{ended ? null : <span class="spinner sm" />}</div>
      <div class="term-body" ref={scroll.ref} onScroll={scroll.onScroll}>
        {lines.length === 0 ? <div class="term-empty">{ended ? "No output." : "Waiting for the builder…"}</div> : null}
        {lines.map((l, i) => <div key={i} class={cx("term-line", /error|ERR!/i.test(l.text) && "err")}><Ansi text={l.text} /></div>)}
      </div>
    </div>
  );
}

/**
 * A line-oriented terminal over WebSocket. Each line typed is sent to the
 * process's stdin; output streams back. Up/down walk the history.
 */
export function Terminal({ path, title = "terminal", height = 460, prompt = "$" }: { path: string; title?: string; height?: number | string; prompt?: string }) {
  const [out, setOut] = useState("");
  const [line, setLine] = useState("");
  const [status, setStatus] = useState<"connecting" | "open" | "closed">("connecting");
  const [nonce, setNonce] = useState(0);
  const ws = useRef<WebSocket | null>(null);
  const hist = useRef<string[]>([]);
  const hpos = useRef(-1);
  const input = useRef<HTMLInputElement | null>(null);
  useEffect(() => {
    setOut("");
    setStatus("connecting");
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    const sock = new WebSocket(`${proto}//${location.host}/api${path}`);
    ws.current = sock;
    sock.onopen = () => setStatus("open");
    sock.onmessage = (m) => {
      const text = typeof m.data === "string" ? m.data : "";
      setOut((o) => {
        const n = o + stripControl(text);
        return n.length > 200000 ? n.slice(n.length - 150000) : n;
      });
    };
    sock.onclose = () => setStatus("closed");
    return () => sock.close();
  }, [path, nonce]);
  const scroll = useAutoScroll(out.length);
  const send = () => {
    if (!ws.current || ws.current.readyState !== 1) return;
    ws.current.send(line + "\n");
    setOut((o) => o + prompt + " " + line + "\n");
    if (line.trim()) hist.current.push(line);
    hpos.current = -1;
    setLine("");
  };
  return (
    <div class="term" style={{ height }} onClick={() => input.current?.focus()}>
      <div class="term-head">
        <span>{title}</span>
        <span class="grow" />
        <span style={{ color: status === "open" ? "#6ee7a0" : status === "closed" ? "#ff8a8a" : "#f5c46b" }}>● {status}</span>
        <button type="button" class="icon-btn" title="Reconnect" onClick={() => setNonce(nonce + 1)}><Icon name="restart" size={14} /></button>
      </div>
      <div class="term-body" ref={scroll.ref} onScroll={scroll.onScroll}>
        {out.split("\n").map((l, i) => <div key={i}><Ansi text={l} /></div>)}
      </div>
      <div class="term-input-row">
        <span class="prompt">{prompt}</span>
        <input
          ref={input}
          class="term-input"
          value={line}
          disabled={status !== "open"}
          spellcheck="false"
          autocomplete="off"
          placeholder={status === "open" ? "" : status === "closed" ? "disconnected" : "connecting…"}
          onInput={(e: any) => setLine(e.target.value)}
          onKeyDown={(e: KeyboardEvent) => {
            if (e.key === "Enter") send();
            else if (e.key === "ArrowUp") {
              e.preventDefault();
              const hs = hist.current;
              if (!hs.length) return;
              hpos.current = hpos.current < 0 ? hs.length - 1 : Math.max(0, hpos.current - 1);
              setLine(hs[hpos.current]);
            } else if (e.key === "ArrowDown") {
              e.preventDefault();
              const hs = hist.current;
              if (hpos.current < 0) return;
              hpos.current++;
              if (hpos.current >= hs.length) {
                hpos.current = -1;
                setLine("");
              } else setLine(hs[hpos.current]);
            } else if (e.key === "l" && e.ctrlKey) {
              e.preventDefault();
              setOut("");
            }
          }}
        />
      </div>
    </div>
  );
}
