import { h, Fragment, render, useEffect } from "./lib/sprout.js";
import { match, navigate, useLocation } from "./lib/router.js";
import { session, use, loadSession, applyTheme, currentOrgSlug } from "./lib/state.js";
import { ModalHost, Toasts, Loading } from "./ui/kit.js";
import { Shell } from "./ui/layout.js";
import { LoginPage, SignupPage, ClaimPage, InvitePage } from "./pages/auth.js";
import { AppsPage } from "./pages/apps.js";
import { ProjectPage } from "./pages/project.js";
import { AppPage } from "./pages/app.js";
import { DeployPage } from "./pages/deploy.js";
import { DatabasePage } from "./pages/database.js";
import { JobsPage, JobPage, NewJobPage, RunPage } from "./pages/jobs.js";
import { NewAppPage, NewDatabasePage } from "./pages/create.js";
import { UsagePage, GrovePage, GreenhousePage, SandboxPage } from "./pages/misc.js";
import { OrgSettingsPage, MembersPage, AccountPage, AdminPage } from "./pages/settings.js";
const orgRoutes = [
    ["/o/:org", (p) => h(AppsPage, { org: p.org }), "Your Apps"],
    ["/o/:org/projects/:id", (p) => h(ProjectPage, { key: p.id, org: p.org, id: p.id }), "Project"],
    ["/o/:org/apps/:id", (p) => h(AppPage, { key: p.id, org: p.org, id: p.id }), "App"],
    ["/o/:org/deploys/:id", (p) => h(DeployPage, { key: p.id, org: p.org, id: p.id }), "Deploy"],
    ["/o/:org/databases/:id", (p) => h(DatabasePage, { key: p.id, org: p.org, id: p.id }), "Database"],
    ["/o/:org/jobs", (p) => h(JobsPage, { org: p.org }), "Jobs"],
    ["/o/:org/jobs/:id", (p) => h(JobPage, { key: p.id, org: p.org, id: p.id }), "Job"],
    ["/o/:org/runs/:id", (p) => h(RunPage, { key: p.id, org: p.org, id: p.id }), "Run"],
    ["/o/:org/new/app", (p) => h(NewAppPage, { org: p.org }), "New app"],
    ["/o/:org/new/database", (p) => h(NewDatabasePage, { org: p.org }), "New database"],
    ["/o/:org/new/job", (p) => h(NewJobPage, { org: p.org }), "New job"],
    ["/o/:org/usage", (p) => h(UsagePage, { org: p.org }), "Usage"],
    ["/o/:org/grove", (p) => h(GrovePage, { org: p.org }), "The Grove"],
    ["/o/:org/greenhouse", (p) => h(GreenhousePage, { org: p.org }), "Greenhouse"],
    ["/o/:org/greenhouse/:id", (p) => h(SandboxPage, { key: p.id, org: p.org, id: p.id }), "Sandbox"],
    ["/o/:org/settings", (p) => h(OrgSettingsPage, { org: p.org }), "Settings"],
    ["/o/:org/members", (p) => h(MembersPage, { org: p.org }), "Members"],
];
function NotFound() {
    return (h("div", { class: "page-mid", style: { paddingTop: 60, textAlign: "center" } },
        h("div", { class: "page-title" }, "Nothing grows here."),
        h("p", { class: "muted" },
            "That page does not exist. ",
            h("a", { href: "/", onClick: (e) => { e.preventDefault(); navigate("/"); } }, "Go home"),
            ".")));
}
function Redirect({ to }) {
    useEffect(() => navigate(to, true), [to]);
    return h(Loading, null);
}
function App() {
    const s = use(session);
    const path = useLocation().split("?")[0];
    if (!s.loaded)
        return h("div", { class: "boot" },
            h("span", { class: "spinner lg" }));
    const me = s.me;
    if (path === "/login")
        return me ? h(Redirect, { to: "/" }) : s.auth?.firstUser ? h(Redirect, { to: "/signup" + location.search }) : h(LoginPage, null);
    if (path === "/signup")
        return me ? h(Redirect, { to: "/" }) : h(SignupPage, null);
    if (path === "/claim")
        return h(ClaimPage, null);
    const inv = match("/invite/:token", path);
    if (inv)
        return h(InvitePage, { token: inv.token });
    if (!me) {
        const next = location.pathname + location.search;
        return h(Redirect, { to: (s.auth?.firstUser ? "/signup" : "/login") + (next !== "/" ? "?next=" + encodeURIComponent(next) : "") });
    }
    if (path === "/" || path === "/o" || path === "") {
        const slug = currentOrgSlug();
        const org = me.orgs.find((o) => o.slug === slug) || me.orgs[0];
        return org ? h(Redirect, { to: "/o/" + org.slug }) : h(Shell, { org: "" },
            h(NoOrg, null));
    }
    const orgSlug = currentOrgSlug() || me.orgs[0]?.slug || "";
    if (path === "/account")
        return h(Shell, { org: orgSlug },
            h(AccountPage, null));
    if (path === "/admin")
        return me.superadmin ? h(Shell, { org: orgSlug },
            h(AdminPage, null)) : h(Shell, { org: orgSlug },
            h(NotFound, null));
    for (const [pattern, view, title] of orgRoutes) {
        const params = match(pattern, path);
        if (params) {
            document.title = (title ? title + " · " : "") + "Wack Club Orchard";
            if (!me.orgs.find((o) => o.slug === params.org || o.id === params.org)) {
                return h(Shell, { org: me.orgs[0]?.slug || "" },
                    h(NotFound, null));
            }
            return h(Shell, { org: params.org }, view(params));
        }
    }
    return h(Shell, { org: orgSlug },
        h(NotFound, null));
}
function NoOrg() {
    return (h("div", { class: "page-mid", style: { paddingTop: 40 } },
        h("div", { class: "page-title" }, "You are not in an organization yet"),
        h("p", { class: "muted" }, "Create one from the switcher in the top left, or ask someone for an invite.")));
}
function Root() {
    return (h(Fragment, null,
        h(App, null),
        h(ModalHost, null),
        h(Toasts, null)));
}
applyTheme();
loadSession().catch(() => session.set({ loaded: true }));
const root = document.getElementById("app");
root.textContent = "";
render(h(Root, null), root);
