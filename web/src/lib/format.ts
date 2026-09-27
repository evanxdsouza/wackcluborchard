import { session } from "./state.js";

export function timeAgo(iso?: string | null, now = Date.now()): string {
  if (!iso) return "never";
  const t = new Date(iso).getTime();
  if (!t || t < 0) return "never";
  let s = Math.round((now - t) / 1000);
  const future = s < 0;
  s = Math.abs(s);
  let out: string;
  if (s < 45) out = s + "s";
  else if (s < 3600) out = Math.round(s / 60) + "m";
  else if (s < 86400) out = Math.round(s / 3600) + "h";
  else if (s < 86400 * 30) out = Math.round(s / 86400) + "d";
  else return new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
  return future ? "in " + out : out + " ago";
}

export function duration(from?: string | null, to?: string | null): string {
  if (!from) return "";
  const a = new Date(from).getTime();
  const b = to ? new Date(to).getTime() : Date.now();
  let s = Math.max(0, Math.round((b - a) / 1000));
  if (s < 60) return s + "s";
  const m = Math.floor(s / 60);
  s %= 60;
  if (m < 60) return `${m}m ${s}s`;
  return `${Math.floor(m / 60)}h ${m % 60}m`;
}

export function bytes(n: number): string {
  if (!n) return "0 B";
  const units = ["B", "kB", "MB", "GB", "TB"];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return (n >= 100 || i === 0 ? Math.round(n) : n.toFixed(1)) + " " + units[i];
}

export function cpu(m: number): string {
  if (m >= 1000) return (m / 1000).toFixed(m % 1000 === 0 ? 0 : 1) + " vCPU";
  return Math.round(m) + "m";
}

export function mem(mi: number): string {
  if (mi >= 1024) return (mi / 1024).toFixed(mi % 1024 === 0 ? 0 : 1) + " GiB";
  return Math.round(mi) + " MiB";
}

export function plural(n: number, word: string, pl?: string) {
  return `${n} ${n === 1 ? word : pl || word + "s"}`;
}

export function dateTime(iso?: string) {
  if (!iso) return "";
  return new Date(iso).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

export function shortSha(s?: string) {
  return s ? s.slice(0, 7) : "";
}

export function copyText(text: string) {
  if (navigator.clipboard && window.isSecureContext) return navigator.clipboard.writeText(text);
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.style.position = "fixed";
  ta.style.opacity = "0";
  document.body.appendChild(ta);
  ta.select();
  document.execCommand("copy");
  ta.remove();
  return Promise.resolve();
}

/** Public URL for an app hostname, with the instance's HTTPS port if it is not 443. */
export function appURL(host: string): string {
  const port = session.get().auth?.httpsPort;
  return port && port !== 443 ? `https://${host}:${port}` : `https://${host}`;
}
