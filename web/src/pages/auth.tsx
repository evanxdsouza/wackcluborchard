import { h, Fragment, useEffect, useState } from "../lib/sprout.js";
import { Link, navigate, query } from "../lib/router.js";
import { post, get, act, ApiError } from "../lib/api.js";
import { session, use, loadSession, toast } from "../lib/state.js";
import { Button, Field, Input, Callout } from "../ui/kit.js";
import { Logo, Sky } from "../ui/art.js";
import { Icon } from "../ui/icons.js";

// ---- WebAuthn helpers ----

function b64urlToBytes(s: string): Uint8Array<ArrayBuffer> {
  s = s.replace(/-/g, "+").replace(/_/g, "/");
  while (s.length % 4) s += "=";
  const bin = atob(s);
  const out = new Uint8Array(new ArrayBuffer(bin.length));
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function bytesToB64url(b: ArrayBuffer | Uint8Array): string {
  const u = b instanceof Uint8Array ? b : new Uint8Array(b);
  let s = "";
  for (let i = 0; i < u.length; i++) s += String.fromCharCode(u[i]);
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export function passkeysSupported() {
  return typeof window.PublicKeyCredential !== "undefined" && window.isSecureContext;
}

export async function registerPasskey(name: string) {
  const opts = await post("/auth/passkey/register/begin");
  const cred = (await navigator.credentials.create({
    publicKey: {
      challenge: b64urlToBytes(opts.challenge),
      rp: opts.rp,
      user: { id: b64urlToBytes(opts.user.id), name: opts.user.name, displayName: opts.user.displayName },
      pubKeyCredParams: [{ type: "public-key", alg: -7 }, { type: "public-key", alg: -8 }, { type: "public-key", alg: -257 }],
      authenticatorSelection: { residentKey: "preferred", userVerification: "preferred" },
      excludeCredentials: (opts.excludeCredentials || []).map((c: any) => ({ type: "public-key", id: b64urlToBytes(c.id) })),
      attestation: "none",
      timeout: 60000,
    },
  })) as PublicKeyCredential | null;
  if (!cred) throw new Error("cancelled");
  const r = cred.response as AuthenticatorAttestationResponse;
  const pk = r.getPublicKey?.();
  if (!pk) throw new Error("this browser does not expose the passkey's public key; try a newer browser");
  await post("/auth/passkey/register/finish", {
    challenge: opts.challenge,
    name,
    id: bytesToB64url(cred.rawId),
    clientDataJSON: bytesToB64url(r.clientDataJSON),
    authenticatorData: bytesToB64url(r.getAuthenticatorData()),
    publicKey: bytesToB64url(pk),
  });
}

async function loginWithPasskey() {
  const opts = await post("/auth/passkey/login/begin");
  const cred = (await navigator.credentials.get({
    publicKey: { challenge: b64urlToBytes(opts.challenge), rpId: opts.rpId, userVerification: "preferred", timeout: 60000 },
  })) as PublicKeyCredential | null;
  if (!cred) throw new Error("cancelled");
  const r = cred.response as AuthenticatorAssertionResponse;
  await post("/auth/passkey/login/finish", {
    challenge: opts.challenge,
    id: bytesToB64url(cred.rawId),
    clientDataJSON: bytesToB64url(r.clientDataJSON),
    authenticatorData: bytesToB64url(r.authenticatorData),
    signature: bytesToB64url(r.signature),
  });
}

// ---- layout ----

function AuthLayout({ children, headline, sub }: { children: any; headline?: string; sub?: string }) {
  return (
    <div class="auth-page">
      <div class="auth-art">
        <Sky seed="wack-club-orchard" preset="clouds" />
        <div class="auth-art-text">
          <h1>{headline || "Ship things to your own cluster."}</h1>
          <p>{sub || "Wack Club Orchard builds your repos, runs them on Kubernetes, gives them HTTPS and a database, and shows you the logs when they break."}</p>
        </div>
      </div>
      <div class="auth-panel">
        <div class="auth-box">
          <div class="auth-brand"><Logo size={34} /> Wack Club Orchard</div>
          {children}
        </div>
      </div>
    </div>
  );
}

async function afterAuth() {
  await loadSession();
  const next = query().get("next");
  if (next && next.startsWith("/") && !next.startsWith("//")) {
    if (next.startsWith("/api/")) location.href = next;
    else navigate(next, true);
    return;
  }
  const me = session.get().me;
  navigate(me && me.orgs[0] ? "/o/" + me.orgs[0].slug : "/", true);
}

export function LoginPage() {
  const s = use(session);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState(query().get("error") || "");
  const [sso, setSso] = useState(false);
  const [email, setEmail] = useState("");
  const submit = async () => {
    setErr("");
    try {
      await post("/auth/login", { username, password });
      await afterAuth();
    } catch (e) {
      setErr((e as Error).message);
    }
  };
  const next = query().get("next");
  return (
    <AuthLayout>
      <div>
        <div class="auth-title">Welcome back</div>
        <div class="auth-sub" style={{ marginTop: 4 }}>Sign in to {s.auth?.instanceName || "Wack Club Orchard"}.</div>
      </div>
      {err ? <Callout kind="red">{err}</Callout> : null}
      {passkeysSupported() ? (
        <Button kind="secondary" class="btn-block" icon="fingerprint" onClick={async () => {
          setErr("");
          try {
            await loginWithPasskey();
            await afterAuth();
          } catch (e) {
            if ((e as Error).name !== "NotAllowedError") setErr((e as Error).message);
          }
        }}>Sign in with a passkey</Button>
      ) : null}
      {s.auth?.sso ? (
        sso ? (
          <div class="form-stack">
            <Field label="Work email"><Input value={email} onInput={setEmail} placeholder="you@company.com" autofocus onEnter={() => (location.href = "/api/auth/sso/start?email=" + encodeURIComponent(email))} /></Field>
            <Button kind="secondary" class="btn-block" href={"/api/auth/sso/start?email=" + encodeURIComponent(email)}>Continue with SSO</Button>
          </div>
        ) : <Button kind="secondary" class="btn-block" icon="building" onClick={() => setSso(true)}>Sign in with SSO</Button>
      ) : null}
      <div class="or">or with a password</div>
      <form class="form-stack" onSubmit={(e: Event) => { e.preventDefault(); submit(); }}>
        <Field label="Username or email"><Input value={username} onInput={setUsername} autofocus autocomplete="username" name="username" /></Field>
        <Field label="Password"><Input type="password" value={password} onInput={setPassword} autocomplete="current-password" name="password" onEnter={submit} /></Field>
        <Button kind="primary" type="submit" class="btn-block" onClick={submit}>Sign in</Button>
      </form>
      {s.auth?.signupMode !== "closed" || s.auth?.firstUser ? (
        <div class="muted" style={{ fontSize: 13.5 }}>New here? <Link href={"/signup" + (next ? "?next=" + encodeURIComponent(next) : "")}>Create an account</Link></div>
      ) : null}
    </AuthLayout>
  );
}

export function SignupPage({ invite }: { invite?: string }) {
  const s = use(session);
  const [f, setF] = useState({ username: "", name: "", email: "", password: "" });
  const [err, setErr] = useState("");
  const set = (k: string) => (v: string) => setF({ ...f, [k]: v });
  const first = s.auth?.firstUser;
  const inviteToken = invite || query().get("invite") || "";
  const closed = !first && !inviteToken && s.auth?.signupMode !== "open";
  const submit = async () => {
    setErr("");
    try {
      await post("/auth/signup", { ...f, invite: inviteToken });
      await afterAuth();
    } catch (e) {
      setErr((e as Error).message);
    }
  };
  return (
    <AuthLayout headline={first ? "Plant the first tree." : undefined} sub={first ? "You are the first person here. Create your account, then open the claim link from the server logs to become the instance admin." : undefined}>
      <div>
        <div class="auth-title">{first ? "Set up this instance" : "Create your account"}</div>
      </div>
      {closed ? <Callout kind="amber">{s.auth?.signupMode === "invite" ? "This instance is invite-only. Ask an admin for an invite link." : "Signups are closed on this instance."}</Callout> : null}
      {err ? <Callout kind="red">{err}</Callout> : null}
      <form class="form-stack" onSubmit={(e: Event) => { e.preventDefault(); submit(); }}>
        <div class="form-grid">
          <Field label="Username"><Input value={f.username} onInput={(v) => set("username")(v.toLowerCase())} autofocus autocomplete="username" placeholder="orpheus" mono /></Field>
          <Field label="Display name"><Input value={f.name} onInput={set("name")} placeholder="Orpheus" /></Field>
        </div>
        <Field label="Email" hint="Optional. Used for SSO matching and invites."><Input type="email" value={f.email} onInput={set("email")} autocomplete="email" /></Field>
        <Field label="Password" hint="At least 10 characters. You can add a passkey afterwards."><Input type="password" value={f.password} onInput={set("password")} autocomplete="new-password" onEnter={submit} /></Field>
        <Button kind="primary" class="btn-block" type="submit" disabled={closed} onClick={submit}>Create account</Button>
      </form>
      <div class="muted" style={{ fontSize: 13.5 }}>Already have one? <Link href="/login">Sign in</Link></div>
    </AuthLayout>
  );
}

export function ClaimPage() {
  const s = use(session);
  const token = query().get("token") || "";
  const [state, setState] = useState<"idle" | "busy" | "done" | "error">("idle");
  const [err, setErr] = useState("");
  const claim = async () => {
    setState("busy");
    try {
      await post("/auth/claim", { token });
      await loadSession();
      setState("done");
    } catch (e) {
      setErr((e as Error).message);
      setState("error");
    }
  };
  if (!s.me && !s.auth?.firstUser && !s.auth?.needsClaim) {
    // recovery: signed out with a fresh claim link
  }
  if (!s.me && s.auth?.needsClaim) {
    return (
      <AuthLayout headline="Claim this instance." sub="The claim link makes you the superadmin. It works once.">
        <div class="auth-title">Create your account first</div>
        <div class="muted">Sign up with the account that should own this instance, then this page claims it.</div>
        <Button kind="primary" class="btn-block" href={"/signup?next=" + encodeURIComponent("/claim?token=" + token)}>Sign up</Button>
        <Button kind="secondary" class="btn-block" href={"/login?next=" + encodeURIComponent("/claim?token=" + token)}>I already have an account</Button>
      </AuthLayout>
    );
  }
  return (
    <AuthLayout headline="Claim this instance." sub="The claim link is the credential: single use, and it makes you the instance superadmin.">
      {state === "done" ? (
        <>
          <div class="auth-title">It is yours.</div>
          <Callout kind="green" title="You are the instance superadmin.">
            The claim link is spent. Add a passkey or keep your password safe: an instance you cannot sign into again is not set up.
          </Callout>
          <Button kind="primary" class="btn-block" href="/account?setup=1">Secure your account</Button>
        </>
      ) : (
        <>
          <div class="auth-title">{s.me ? `Claim as ${s.me.username}` : "Recover access"}</div>
          <div class="muted">{s.me ? "Your account becomes the instance superadmin." : "This signs you in as the instance superadmin."}</div>
          {err ? <Callout kind="red">{err}</Callout> : null}
          <Button kind="primary" class="btn-block" icon="shield" loading={state === "busy"} disabled={!token} onClick={claim}>Claim instance</Button>
        </>
      )}
    </AuthLayout>
  );
}

export function InvitePage({ token }: { token: string }) {
  const s = use(session);
  const [inv, setInv] = useState<any>(null);
  const [err, setErr] = useState("");
  useEffect(() => {
    get("/invites/" + token).then(setInv).catch((e) => setErr(e.message));
  }, [token]);
  if (err) return <AuthLayout><Callout kind="red">{err}</Callout><Link href="/">Go home</Link></AuthLayout>;
  if (!inv) return <AuthLayout><div class="loading"><span class="spinner" /></div></AuthLayout>;
  return (
    <AuthLayout headline={`Join ${inv.org}.`} sub={`${inv.invitedBy.name} invited you to join as ${inv.role}.`}>
      <div class="auth-title">You are invited to {inv.org}</div>
      <div class="muted">Role: <strong>{inv.role}</strong></div>
      {s.me ? (
        <Button kind="primary" class="btn-block" onClick={async () => {
          const r = await act(() => post(`/invites/${token}/accept`));
          if (r) {
            await loadSession();
            toast("Welcome to " + inv.org, "ok");
            navigate("/o/" + r.org);
          }
        }}>Join as {s.me.username}</Button>
      ) : (
        <>
          <Button kind="primary" class="btn-block" href={"/signup?invite=" + token}>Create an account</Button>
          <Button kind="secondary" class="btn-block" href={"/login?next=" + encodeURIComponent("/invite/" + token)}>Sign in</Button>
        </>
      )}
    </AuthLayout>
  );
}

export { ApiError, Icon };
