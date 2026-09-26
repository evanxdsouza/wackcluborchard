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

type Route = [string, (p: Record<string, string>) => any, string?];

const orgRoutes: Route[] = [
  ["/o/:org", (p) => <AppsPage org={p.org} />, "Your Apps"],
  ["/o/:org/projects/:id", (p) => <ProjectPage key={p.id} org={p.org} id={p.id} />, "Project"],
  ["/o/:org/apps/:id", (p) => <AppPage key={p.id} org={p.org} id={p.id} />, "App"],
  ["/o/:org/deploys/:id", (p) => <DeployPage key={p.id} org={p.org} id={p.id} />, "Deploy"],
  ["/o/:org/databases/:id", (p) => <DatabasePage key={p.id} org={p.org} id={p.id} />, "Database"],
  ["/o/:org/jobs", (p) => <JobsPage org={p.org} />, "Jobs"],
  ["/o/:org/jobs/:id", (p) => <JobPage key={p.id} org={p.org} id={p.id} />, "Job"],
  ["/o/:org/runs/:id", (p) => <RunPage key={p.id} org={p.org} id={p.id} />, "Run"],
  ["/o/:org/new/app", (p) => <NewAppPage org={p.org} />, "New app"],
  ["/o/:org/new/database", (p) => <NewDatabasePage org={p.org} />, "New database"],
  ["/o/:org/new/job", (p) => <NewJobPage org={p.org} />, "New job"],
  ["/o/:org/usage", (p) => <UsagePage org={p.org} />, "Usage"],
  ["/o/:org/grove", (p) => <GrovePage org={p.org} />, "The Grove"],
  ["/o/:org/greenhouse", (p) => <GreenhousePage org={p.org} />, "Greenhouse"],
  ["/o/:org/greenhouse/:id", (p) => <SandboxPage key={p.id} org={p.org} id={p.id} />, "Sandbox"],
  ["/o/:org/settings", (p) => <OrgSettingsPage org={p.org} />, "Settings"],
  ["/o/:org/members", (p) => <MembersPage org={p.org} />, "Members"],
];

function NotFound() {
  return (
    <div class="page-mid" style={{ paddingTop: 60, textAlign: "center" }}>
      <div class="page-title">Nothing grows here.</div>
      <p class="muted">That page does not exist. <a href="/" onClick={(e: Event) => { e.preventDefault(); navigate("/"); }}>Go home</a>.</p>
    </div>
  );
}

function Redirect({ to }: { to: string }) {
  useEffect(() => navigate(to, true), [to]);
  return <Loading />;
}

function App() {
  const s = use(session);
  const path = useLocation().split("?")[0];
  if (!s.loaded) return <div class="boot"><span class="spinner lg" /></div>;
  const me = s.me;

  // public routes
  if (path === "/login") return me ? <Redirect to="/" /> : s.auth?.firstUser ? <Redirect to={"/signup" + location.search} /> : <LoginPage />;
  if (path === "/signup") return me ? <Redirect to="/" /> : <SignupPage />;
  if (path === "/claim") return <ClaimPage />;
  const inv = match("/invite/:token", path);
  if (inv) return <InvitePage token={inv.token} />;

  if (!me) {
    const next = location.pathname + location.search;
    return <Redirect to={(s.auth?.firstUser ? "/signup" : "/login") + (next !== "/" ? "?next=" + encodeURIComponent(next) : "")} />;
  }

  if (path === "/" || path === "/o" || path === "") {
    const slug = currentOrgSlug();
    const org = me.orgs.find((o) => o.slug === slug) || me.orgs[0];
    return org ? <Redirect to={"/o/" + org.slug} /> : <Shell org=""><NoOrg /></Shell>;
  }

  const orgSlug = currentOrgSlug() || me.orgs[0]?.slug || "";
  if (path === "/account") return <Shell org={orgSlug}><AccountPage /></Shell>;
  if (path === "/admin") return me.superadmin ? <Shell org={orgSlug}><AdminPage /></Shell> : <Shell org={orgSlug}><NotFound /></Shell>;

  for (const [pattern, view, title] of orgRoutes) {
    const params = match(pattern, path);
    if (params) {
      document.title = (title ? title + " · " : "") + "Wack Club Orchard";
      if (!me.orgs.find((o) => o.slug === params.org || o.id === params.org)) {
        return <Shell org={me.orgs[0]?.slug || ""}><NotFound /></Shell>;
      }
      return <Shell org={params.org}>{view(params)}</Shell>;
    }
  }
  return <Shell org={orgSlug}><NotFound /></Shell>;
}

function NoOrg() {
  return (
    <div class="page-mid" style={{ paddingTop: 40 }}>
      <div class="page-title">You are not in an organization yet</div>
      <p class="muted">Create one from the switcher in the top left, or ask someone for an invite.</p>
    </div>
  );
}

function Root() {
  return (
    <>
      <App />
      <ModalHost />
      <Toasts />
    </>
  );
}

applyTheme();
loadSession().catch(() => session.set({ loaded: true }));
const root = document.getElementById("app")!;
root.textContent = "";
render(<Root />, root);
