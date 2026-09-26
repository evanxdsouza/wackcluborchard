import { h, Fragment, useEffect, useRef, useState } from "../lib/sprout.js";
import { useLogStream } from "../lib/api.js";
import { Icon } from "./icons.js";
import { cx } from "./kit.js";
const ansiColors = { "30": "gray", "31": "red", "32": "green", "33": "yellow", "34": "blue", "35": "magenta", "36": "cyan", "37": "", "90": "gray", "91": "red", "92": "green", "93": "yellow", "94": "blue", "95": "magenta", "96": "cyan" };
export function Ansi({ text }) {
    if (!text.includes("\x1b["))
        return h(Fragment, null, text);
    const out = [];
    let cls = [];
    const re = /\x1b\[([0-9;]*)m/g;
    let last = 0;
    let m;
    while ((m = re.exec(text))) {
        if (m.index > last)
            out.push(h("span", { class: cls.join(" ") }, text.slice(last, m.index)));
        for (const code of m[1].split(";")) {
            if (code === "0" || code === "")
                cls = [];
            else if (code === "1")
                cls.push("ansi-bold");
            else if (ansiColors[code])
                cls = cls.filter((c) => !c.startsWith("ansi-") || c === "ansi-bold").concat("ansi-" + ansiColors[code]);
        }
        last = re.lastIndex;
    }
    if (last < text.length)
        out.push(h("span", { class: cls.join(" ") }, text.slice(last)));
    return h(Fragment, null, out);
}
function stripControl(s) {
    return s.replace(/\x1b\[[0-9;?]*[ABCDEFGHJKSTfhlnsu]/g, "").replace(/\x1b\][^\x07]*\x07/g, "").replace(/\r(?!\n)/g, "");
}
export function useAutoScroll(dep) {
    const ref = useRef(null);
    const stick = useRef(true);
    useEffect(() => {
        const el = ref.current;
        if (el && stick.current)
            el.scrollTop = el.scrollHeight;
    }, [dep]);
    const onScroll = () => {
        const el = ref.current;
        if (el)
            stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    };
    return { ref, onScroll };
}
export function LogView({ appId, pods, height }) {
    const [pod, setPod] = useState("");
    const [since, setSince] = useState("");
    const [previous, setPrevious] = useState(false);
    const [filter, setFilter] = useState("");
    const [nonce, setNonce] = useState(0);
    const params = new URLSearchParams();
    if (pod)
        params.set("pod", pod);
    if (since)
        params.set("since", since);
    if (previous)
        params.set("previous", "1");
    params.set("n", String(nonce));
    const { lines, ended } = useLogStream(`/apps/${appId}/logs?${params}`);
    const shown = filter ? lines.filter((l) => l.text.toLowerCase().includes(filter.toLowerCase())) : lines;
    const scroll = useAutoScroll(shown.length);
    const multi = pods.length > 1;
    return (h("div", { class: "term", style: height ? { height } : undefined },
        h("div", { class: "term-head" },
            h("span", null, previous ? "previous container" : "logs"),
            h("span", { class: "grow" }),
            h("input", { class: "term-filter", placeholder: "filter", value: filter, onInput: (e) => setFilter(e.target.value), style: { background: "#151518", border: "1px solid #2a2a2f", borderRadius: 6, color: "#ddd", height: 24, padding: "0 8px", width: 120, fontFamily: "var(--mono)", fontSize: 11 } }),
            multi ? (h("select", { value: pod, onChange: (e) => setPod(e.target.value), title: "Pod" },
                h("option", { value: "" }, "all pods"),
                pods.map((p) => h("option", { key: p.name, value: p.name, selected: p.name === pod }, p.name.split("-").slice(-1)[0])))) : null,
            h("select", { value: since, onChange: (e) => setSince(e.target.value), title: "Time range" },
                h("option", { value: "" }, "all"),
                h("option", { value: "5m", selected: since === "5m" }, "5 min"),
                h("option", { value: "15m", selected: since === "15m" }, "15 min"),
                h("option", { value: "1h", selected: since === "1h" }, "1 hour")),
            h("button", { type: "button", class: cx("icon-btn", previous && "active"), title: "Previous container (the one that died)", onClick: () => setPrevious(!previous), style: previous ? { color: "#ff8a8a" } : undefined },
                h(Icon, { name: "skull", size: 14 })),
            h("button", { type: "button", class: "icon-btn", title: "Reconnect", onClick: () => setNonce(nonce + 1) },
                h(Icon, { name: "restart", size: 14 }))),
        h("div", { class: "term-body", ref: scroll.ref, onScroll: scroll.onScroll },
            shown.length === 0 ? h("div", { class: "term-empty" }, ended ? "No output." : "Waiting for output…") : null,
            shown.map((l, i) => (h("div", { class: "term-line", key: i },
                multi && !pod && l.pod ? h("span", { class: "pod" }, l.pod.split("-").slice(-1)[0]) : null,
                h("span", null,
                    h(Ansi, { text: l.text }))))))));
}
export function OutputView({ text, title = "stdout", onRefresh, height }) {
    const scroll = useAutoScroll(text.length);
    return (h("div", { class: "term", style: height ? { maxHeight: height } : undefined },
        h("div", { class: "term-head" },
            h("span", null, title),
            h("span", { class: "grow" }),
            onRefresh ? h("button", { type: "button", class: "icon-btn", title: "Refresh", onClick: onRefresh },
                h(Icon, { name: "restart", size: 14 })) : null),
        h("div", { class: "term-body", ref: scroll.ref, onScroll: scroll.onScroll }, text ? text.replace(/\n$/, "").split("\n").map((l, i) => h("div", { key: i },
            h(Ansi, { text: l }))) : h("span", { class: "term-empty" }, "No output."))));
}
export function DeployLog({ deployId, height = 420 }) {
    const { lines, ended } = useLogStream(`/deploys/${deployId}/logs`);
    const scroll = useAutoScroll(lines.length);
    return (h("div", { class: "term", style: { height } },
        h("div", { class: "term-head" },
            h("span", null, "build log"),
            h("span", { class: "grow" }),
            ended ? null : h("span", { class: "spinner sm" })),
        h("div", { class: "term-body", ref: scroll.ref, onScroll: scroll.onScroll },
            lines.length === 0 ? h("div", { class: "term-empty" }, ended ? "No output." : "Waiting for the builder…") : null,
            lines.map((l, i) => h("div", { key: i, class: cx("term-line", /error|ERR!/i.test(l.text) && "err") },
                h(Ansi, { text: l.text }))))));
}
export function Terminal({ path, title = "terminal", height = 460, prompt = "$" }) {
    const [out, setOut] = useState("");
    const [line, setLine] = useState("");
    const [status, setStatus] = useState("connecting");
    const [nonce, setNonce] = useState(0);
    const ws = useRef(null);
    const hist = useRef([]);
    const hpos = useRef(-1);
    const input = useRef(null);
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
        if (!ws.current || ws.current.readyState !== 1)
            return;
        ws.current.send(line + "\n");
        setOut((o) => o + prompt + " " + line + "\n");
        if (line.trim())
            hist.current.push(line);
        hpos.current = -1;
        setLine("");
    };
    return (h("div", { class: "term", style: { height }, onClick: () => input.current?.focus() },
        h("div", { class: "term-head" },
            h("span", null, title),
            h("span", { class: "grow" }),
            h("span", { style: { color: status === "open" ? "#6ee7a0" : status === "closed" ? "#ff8a8a" : "#f5c46b" } },
                "\u25CF ",
                status),
            h("button", { type: "button", class: "icon-btn", title: "Reconnect", onClick: () => setNonce(nonce + 1) },
                h(Icon, { name: "restart", size: 14 }))),
        h("div", { class: "term-body", ref: scroll.ref, onScroll: scroll.onScroll }, out.split("\n").map((l, i) => h("div", { key: i },
            h(Ansi, { text: l })))),
        h("div", { class: "term-input-row" },
            h("span", { class: "prompt" }, prompt),
            h("input", { ref: input, class: "term-input", value: line, disabled: status !== "open", spellcheck: "false", autocomplete: "off", placeholder: status === "open" ? "" : status === "closed" ? "disconnected" : "connecting…", onInput: (e) => setLine(e.target.value), onKeyDown: (e) => {
                    if (e.key === "Enter")
                        send();
                    else if (e.key === "ArrowUp") {
                        e.preventDefault();
                        const hs = hist.current;
                        if (!hs.length)
                            return;
                        hpos.current = hpos.current < 0 ? hs.length - 1 : Math.max(0, hpos.current - 1);
                        setLine(hs[hpos.current]);
                    }
                    else if (e.key === "ArrowDown") {
                        e.preventDefault();
                        const hs = hist.current;
                        if (hpos.current < 0)
                            return;
                        hpos.current++;
                        if (hpos.current >= hs.length) {
                            hpos.current = -1;
                            setLine("");
                        }
                        else
                            setLine(hs[hpos.current]);
                    }
                    else if (e.key === "l" && e.ctrlKey) {
                        e.preventDefault();
                        setOut("");
                    }
                } }))));
}
