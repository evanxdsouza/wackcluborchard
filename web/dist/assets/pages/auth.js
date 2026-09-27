import { h, Fragment, useEffect, useState } from "../lib/sprout.js";
import { Link, navigate, query } from "../lib/router.js";
import { post, get, act, ApiError } from "../lib/api.js";
import { session, use, loadSession, toast } from "../lib/state.js";
import { Button, Field, Input, Callout } from "../ui/kit.js";
import { Logo, Sky } from "../ui/art.js";
import { Icon } from "../ui/icons.js";
function b64urlToBytes(s) {
    s = s.replace(/-/g, "+").replace(/_/g, "/");
    while (s.length % 4)
        s += "=";
    const bin = atob(s);
    const out = new Uint8Array(new ArrayBuffer(bin.length));
    for (let i = 0; i < bin.length; i++)
        out[i] = bin.charCodeAt(i);
    return out;
}
function bytesToB64url(b) {
    const u = b instanceof Uint8Array ? b : new Uint8Array(b);
    let s = "";
    for (let i = 0; i < u.length; i++)
        s += String.fromCharCode(u[i]);
    return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}
export function passkeysSupported() {
    return typeof window.PublicKeyCredential !== "undefined" && window.isSecureContext;
}
export async function registerPasskey(name) {
    const opts = await post("/auth/passkey/register/begin");
    const cred = (await navigator.credentials.create({
        publicKey: {
            challenge: b64urlToBytes(opts.challenge),
            rp: opts.rp,
            user: { id: b64urlToBytes(opts.user.id), name: opts.user.name, displayName: opts.user.displayName },
            pubKeyCredParams: [{ type: "public-key", alg: -7 }, { type: "public-key", alg: -8 }, { type: "public-key", alg: -257 }],
            authenticatorSelection: { residentKey: "preferred", userVerification: "preferred" },
            excludeCredentials: (opts.excludeCredentials || []).map((c) => ({ type: "public-key", id: b64urlToBytes(c.id) })),
            attestation: "none",
            timeout: 60000,
        },
    }));
    if (!cred)
        throw new Error("cancelled");
    const r = cred.response;
    const pk = r.getPublicKey?.();
    if (!pk)
        throw new Error("this browser does not expose the passkey's public key; try a newer browser");
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
    }));
    if (!cred)
        throw new Error("cancelled");
    const r = cred.response;
    await post("/auth/passkey/login/finish", {
        challenge: opts.challenge,
        id: bytesToB64url(cred.rawId),
        clientDataJSON: bytesToB64url(r.clientDataJSON),
        authenticatorData: bytesToB64url(r.authenticatorData),
        signature: bytesToB64url(r.signature),
    });
}
function AuthLayout({ children, headline, sub }) {
    return (h("div", { class: "auth-page" },
        h("div", { class: "auth-art" },
            h(Sky, { seed: "wackcluborchard", preset: "clouds" }),
            h("div", { class: "auth-art-text" },
                h("h1", null, headline || "Ship things to your own cluster."),
                h("p", null, sub || "Wack Club Orchard builds your repos, runs them on Kubernetes, gives them HTTPS and a database, and shows you the logs when they break."))),
        h("div", { class: "auth-panel" },
            h("div", { class: "auth-box" },
                h("div", { class: "auth-brand" },
                    h(Logo, { size: 34 }),
                    " Wack Club Orchard"),
                children))));
}
async function afterAuth() {
    await loadSession();
    const next = query().get("next");
    if (next && next.startsWith("/") && !next.startsWith("//")) {
        if (next.startsWith("/api/"))
            location.href = next;
        else
            navigate(next, true);
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
        }
        catch (e) {
            setErr(e.message);
        }
    };
    const next = query().get("next");
    return (h(AuthLayout, null,
        h("div", null,
            h("div", { class: "auth-title" }, "Welcome back"),
            h("div", { class: "auth-sub", style: { marginTop: 4 } },
                "Sign in to ",
                s.auth?.instanceName || "Wack Club Orchard",
                ".")),
        err ? h(Callout, { kind: "red" }, err) : null,
        passkeysSupported() ? (h(Button, { kind: "secondary", class: "btn-block", icon: "fingerprint", onClick: async () => {
                setErr("");
                try {
                    await loginWithPasskey();
                    await afterAuth();
                }
                catch (e) {
                    if (e.name !== "NotAllowedError")
                        setErr(e.message);
                }
            } }, "Sign in with a passkey")) : null,
        s.auth?.sso ? (sso ? (h("div", { class: "form-stack" },
            h(Field, { label: "Work email" },
                h(Input, { value: email, onInput: setEmail, placeholder: "you@company.com", autofocus: true, onEnter: () => (location.href = "/api/auth/sso/start?email=" + encodeURIComponent(email)) })),
            h(Button, { kind: "secondary", class: "btn-block", href: "/api/auth/sso/start?email=" + encodeURIComponent(email) }, "Continue with SSO"))) : h(Button, { kind: "secondary", class: "btn-block", icon: "building", onClick: () => setSso(true) }, "Sign in with SSO")) : null,
        h("div", { class: "or" }, "or with a password"),
        h("form", { class: "form-stack", onSubmit: (e) => { e.preventDefault(); submit(); } },
            h(Field, { label: "Username or email" },
                h(Input, { value: username, onInput: setUsername, autofocus: true, autocomplete: "username", name: "username" })),
            h(Field, { label: "Password" },
                h(Input, { type: "password", value: password, onInput: setPassword, autocomplete: "current-password", name: "password", onEnter: submit })),
            h(Button, { kind: "primary", type: "submit", class: "btn-block", onClick: submit }, "Sign in")),
        s.auth?.signupMode !== "closed" || s.auth?.firstUser ? (h("div", { class: "muted", style: { fontSize: 13.5 } },
            "New here? ",
            h(Link, { href: "/signup" + (next ? "?next=" + encodeURIComponent(next) : "") }, "Create an account"))) : null));
}
export function SignupPage({ invite }) {
    const s = use(session);
    const [f, setF] = useState({ username: "", name: "", email: "", password: "" });
    const [err, setErr] = useState("");
    const set = (k) => (v) => setF({ ...f, [k]: v });
    const first = s.auth?.firstUser;
    const inviteToken = invite || query().get("invite") || "";
    const closed = !first && !inviteToken && s.auth?.signupMode !== "open";
    const submit = async () => {
        setErr("");
        try {
            await post("/auth/signup", { ...f, invite: inviteToken });
            await afterAuth();
        }
        catch (e) {
            setErr(e.message);
        }
    };
    return (h(AuthLayout, { headline: first ? "Plant the first tree." : undefined, sub: first ? "You are the first person here. Create your account, then open the claim link from the server logs to become the instance admin." : undefined },
        h("div", null,
            h("div", { class: "auth-title" }, first ? "Set up this instance" : "Create your account")),
        closed ? h(Callout, { kind: "amber" }, s.auth?.signupMode === "invite" ? "This instance is invite-only. Ask an admin for an invite link." : "Signups are closed on this instance.") : null,
        err ? h(Callout, { kind: "red" }, err) : null,
        h("form", { class: "form-stack", onSubmit: (e) => { e.preventDefault(); submit(); } },
            h("div", { class: "form-grid" },
                h(Field, { label: "Username" },
                    h(Input, { value: f.username, onInput: (v) => set("username")(v.toLowerCase()), autofocus: true, autocomplete: "username", placeholder: "orpheus", mono: true })),
                h(Field, { label: "Display name" },
                    h(Input, { value: f.name, onInput: set("name"), placeholder: "Orpheus" }))),
            h(Field, { label: "Email", hint: "Optional. Used for SSO matching and invites." },
                h(Input, { type: "email", value: f.email, onInput: set("email"), autocomplete: "email" })),
            h(Field, { label: "Password", hint: "At least 10 characters. You can add a passkey afterwards." },
                h(Input, { type: "password", value: f.password, onInput: set("password"), autocomplete: "new-password", onEnter: submit })),
            h(Button, { kind: "primary", class: "btn-block", type: "submit", disabled: closed, onClick: submit }, "Create account")),
        h("div", { class: "muted", style: { fontSize: 13.5 } },
            "Already have one? ",
            h(Link, { href: "/login" }, "Sign in"))));
}
export function ClaimPage() {
    const s = use(session);
    const token = query().get("token") || "";
    const [state, setState] = useState("idle");
    const [err, setErr] = useState("");
    const claim = async () => {
        setState("busy");
        try {
            await post("/auth/claim", { token });
            await loadSession();
            setState("done");
        }
        catch (e) {
            setErr(e.message);
            setState("error");
        }
    };
    if (!s.me && !s.auth?.firstUser && !s.auth?.needsClaim) {
    }
    if (!s.me && s.auth?.needsClaim) {
        return (h(AuthLayout, { headline: "Claim this instance.", sub: "The claim link makes you the superadmin. It works once." },
            h("div", { class: "auth-title" }, "Create your account first"),
            h("div", { class: "muted" }, "Sign up with the account that should own this instance, then this page claims it."),
            h(Button, { kind: "primary", class: "btn-block", href: "/signup?next=" + encodeURIComponent("/claim?token=" + token) }, "Sign up"),
            h(Button, { kind: "secondary", class: "btn-block", href: "/login?next=" + encodeURIComponent("/claim?token=" + token) }, "I already have an account")));
    }
    return (h(AuthLayout, { headline: "Claim this instance.", sub: "The claim link is the credential: single use, and it makes you the instance superadmin." }, state === "done" ? (h(Fragment, null,
        h("div", { class: "auth-title" }, "It is yours."),
        h(Callout, { kind: "green", title: "You are the instance superadmin." }, "The claim link is spent. Add a passkey or keep your password safe: an instance you cannot sign into again is not set up."),
        h(Button, { kind: "primary", class: "btn-block", href: "/account?setup=1" }, "Secure your account"))) : (h(Fragment, null,
        h("div", { class: "auth-title" }, s.me ? `Claim as ${s.me.username}` : "Recover access"),
        h("div", { class: "muted" }, s.me ? "Your account becomes the instance superadmin." : "This signs you in as the instance superadmin."),
        err ? h(Callout, { kind: "red" }, err) : null,
        h(Button, { kind: "primary", class: "btn-block", icon: "shield", loading: state === "busy", disabled: !token, onClick: claim }, "Claim instance")))));
}
export function InvitePage({ token }) {
    const s = use(session);
    const [inv, setInv] = useState(null);
    const [err, setErr] = useState("");
    useEffect(() => {
        get("/invites/" + token).then(setInv).catch((e) => setErr(e.message));
    }, [token]);
    if (err)
        return h(AuthLayout, null,
            h(Callout, { kind: "red" }, err),
            h(Link, { href: "/" }, "Go home"));
    if (!inv)
        return h(AuthLayout, null,
            h("div", { class: "loading" },
                h("span", { class: "spinner" })));
    return (h(AuthLayout, { headline: `Join ${inv.org}.`, sub: `${inv.invitedBy.name} invited you to join as ${inv.role}.` },
        h("div", { class: "auth-title" },
            "You are invited to ",
            inv.org),
        h("div", { class: "muted" },
            "Role: ",
            h("strong", null, inv.role)),
        s.me ? (h(Button, { kind: "primary", class: "btn-block", onClick: async () => {
                const r = await act(() => post(`/invites/${token}/accept`));
                if (r) {
                    await loadSession();
                    toast("Welcome to " + inv.org, "ok");
                    navigate("/o/" + r.org);
                }
            } },
            "Join as ",
            s.me.username)) : (h(Fragment, null,
            h(Button, { kind: "primary", class: "btn-block", href: "/signup?invite=" + token }, "Create an account"),
            h(Button, { kind: "secondary", class: "btn-block", href: "/login?next=" + encodeURIComponent("/invite/" + token) }, "Sign in")))));
}
export { ApiError, Icon };
