package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/auth"
	"github.com/evanxdsouza/wackcluborchard/internal/github"
	"github.com/evanxdsouza/wackcluborchard/internal/platform"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

// ---- instance admin ----

func (s *Server) adminSettings(w http.ResponseWriter, r *http.Request, u *store.User) {
	var st store.Settings
	s.P.Store.Read(func(d *store.Data) { st = d.Settings })
	st.SetupTokenHash = ""
	st.Secret = ""
	if st.GitHub.ClientSecret != "" {
		st.GitHub.ClientSecret = "••••••••"
	}
	st.GitHub.PrivateKey = ""
	st.GitHub.WebhookSecret = ""
	if st.AnthropicKey != "" {
		st.AnthropicKey = "••••••••"
	}
	run, wait, slots := s.P.BuildStats()
	writeJSON(w, 200, map[string]any{"settings": st, "runtime": s.P.Driver.Name(), "builds": map[string]int{"running": run, "waiting": wait, "slots": slots}, "version": s.P.Cfg.Version})
}

func (s *Server) updateAdminSettings(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in map[string]json.RawMessage
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	err := s.P.Store.Write(func(d *store.Data) error {
		st := &d.Settings
		str := func(k string, dst *string) {
			if v, ok := in[k]; ok {
				var x string
				json.Unmarshal(v, &x)
				*dst = strings.TrimSpace(x)
			}
		}
		str("instanceName", &st.InstanceName)
		str("domain", &st.Domain)
		str("appDomain", &st.AppDomain)
		str("mcpDomain", &st.MCPDomain)
		str("ingressCname", &st.IngressCNAME)
		str("publicIp", &st.PublicIP)
		str("publicDbDomain", &st.PublicDBDomain)
		if v, ok := in["signupMode"]; ok {
			var x string
			json.Unmarshal(v, &x)
			if x != "open" && x != "invite" && x != "closed" {
				return platform.Invalid("signup mode must be open, invite or closed")
			}
			st.SignupMode = x
		}
		if v, ok := in["anthropicKey"]; ok {
			var x string
			json.Unmarshal(v, &x)
			if !strings.HasPrefix(x, "••") {
				st.AnthropicKey = x
			}
		}
		for k, dst := range map[string]*bool{"mcpEnabled": &st.MCPEnabled, "tenantSandbox": &st.TenantSandbox, "builderSandbox": &st.BuilderSandbox} {
			if v, ok := in[k]; ok {
				json.Unmarshal(v, dst)
			}
		}
		if v, ok := in["buildSlots"]; ok {
			var n int
			json.Unmarshal(v, &n)
			if n < 1 || n > 32 {
				return platform.Invalid("build slots must be 1 to 32")
			}
			st.BuildSlots = n
		}
		if v, ok := in["hostnames"]; ok {
			var hs []string
			json.Unmarshal(v, &hs)
			st.Hostnames = hs
		}
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit("", u, "instance.settings_updated", "", nil)
	s.adminSettings(w, r, u)
}

func (s *Server) adminOrgs(w http.ResponseWriter, r *http.Request, u *store.User) {
	type row struct {
		*store.Org
		Members  int            `json:"members"`
		Projects int            `json:"projects"`
		Used     platform.Usage `json:"used"`
	}
	out := []row{}
	s.P.Store.Read(func(d *store.Data) {
		for _, o := range store.Values(d.Orgs, func(a, b *store.Org) bool { return a.CreatedAt.Before(b.CreatedAt) }) {
			c := store.Clone(o)
			c.SCIMToken = ""
			c.SSO.ClientSecret = ""
			x := row{Org: c, Used: platform.OrgUsage(d, o.ID, "")}
			for _, m := range d.Memberships {
				if m.OrgID == o.ID {
					x.Members++
				}
			}
			for _, p := range d.Projects {
				if p.OrgID == o.ID {
					x.Projects++
				}
			}
			out = append(out, x)
		}
	})
	writeJSON(w, 200, out)
}

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request, u *store.User) {
	type row struct {
		PublicUser
		Disabled bool     `json:"disabled"`
		Orgs     []string `json:"orgs"`
	}
	out := []row{}
	s.P.Store.Read(func(d *store.Data) {
		for _, x := range store.Values(d.Users, func(a, b *store.User) bool { return a.CreatedAt.Before(b.CreatedAt) }) {
			rw := row{PublicUser: publicUser(x), Disabled: x.Disabled}
			for _, o := range d.UserOrgs(x.ID) {
				rw.Orgs = append(rw.Orgs, o.Slug)
			}
			out = append(out, rw)
		}
	})
	writeJSON(w, 200, out)
}

func (s *Server) adminUpdateUser(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	var in struct {
		Superadmin *bool `json:"superadmin"`
		Disabled   *bool `json:"disabled"`
	}
	readJSON(r, &in)
	err := s.P.Store.Write(func(d *store.Data) error {
		x := d.Users[id]
		if x == nil {
			return platform.Invalid("user not found")
		}
		if id == u.ID && ((in.Superadmin != nil && !*in.Superadmin) || (in.Disabled != nil && *in.Disabled)) {
			return platform.Invalid("you cannot demote or disable yourself")
		}
		if in.Superadmin != nil {
			x.Superadmin = *in.Superadmin
		}
		if in.Disabled != nil {
			x.Disabled = *in.Disabled
			if x.Disabled {
				for k, sess := range d.Sessions {
					if sess.UserID == id {
						delete(d.Sessions, k)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit("", u, "instance.user_updated", id, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) adminNodes(w http.ResponseWriter, r *http.Request, u *store.User) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	nodes, err := s.P.Nodes(ctx)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	var pools []*store.Pool
	s.P.Store.Read(func(d *store.Data) {
		for _, p := range store.Values(d.Pools, func(a, b *store.Pool) bool { return a.Name < b.Name }) {
			pools = append(pools, store.Clone(p))
		}
	})
	if pools == nil {
		pools = []*store.Pool{}
	}
	writeJSON(w, 200, map[string]any{"nodes": nodes, "pools": pools})
}

func (s *Server) adminSavePool(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in store.Pool
	readJSON(r, &in)
	in.Name = store.Slugify(in.Name)
	if in.Name == "" {
		writeErr(w, 400, "name the pool")
		return
	}
	var old *store.Pool
	s.P.Store.Write(func(d *store.Data) error {
		if in.ID == "" {
			in.ID = store.NewID("pool")
		} else if p := d.Pools[in.ID]; p != nil {
			old = store.Clone(p)
		}
		d.Pools[in.ID] = &in
		// organizations mapped to this pool schedule onto it
		for _, o := range d.Orgs {
			mapped := false
			for _, id := range in.OrgIDs {
				if id == o.ID {
					mapped = true
				}
			}
			if mapped {
				o.Pool = in.Name
			} else if o.Pool == in.Name {
				o.Pool = ""
			}
		}
		return nil
	})
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	keep := map[string]bool{}
	for _, n := range in.Nodes {
		keep[n] = true
		if err := s.P.Driver.SetNodePool(ctx, n, in.Name, in.Taint); err != nil {
			writeErr(w, 502, err.Error())
			return
		}
	}
	if old != nil {
		for _, n := range old.Nodes {
			if !keep[n] {
				s.P.Driver.SetNodePool(ctx, n, "", false)
			}
		}
	}
	// re-apply apps so their scheduling follows the pool
	var apps []string
	s.P.Store.Read(func(d *store.Data) {
		for _, a := range d.Apps {
			apps = append(apps, a.ID)
		}
	})
	for _, a := range apps {
		s.P.EnqueueApp(a)
	}
	s.P.Audit("", u, "instance.pool_saved", in.Name, nil)
	writeJSON(w, 200, in)
}

func (s *Server) adminDeletePool(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	var p *store.Pool
	s.P.Store.Write(func(d *store.Data) error {
		if x := d.Pools[id]; x != nil {
			p = store.Clone(x)
			delete(d.Pools, id)
			for _, o := range d.Orgs {
				if o.Pool == x.Name {
					o.Pool = ""
				}
			}
		}
		return nil
	})
	if p != nil {
		for _, n := range p.Nodes {
			s.P.Driver.SetNodePool(r.Context(), n, "", false)
		}
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// adminNetworking reports what each of the instance's hostnames resolves
// to, so DNS problems are visible before certificates fail quietly.
func (s *Server) adminNetworking(w http.ResponseWriter, r *http.Request, u *store.User) {
	var st store.Settings
	s.P.Store.Read(func(d *store.Data) { st = d.Settings })
	type check struct {
		Name    string   `json:"name"`
		Purpose string   `json:"purpose"`
		Records []string `json:"records"`
		Error   string   `json:"error,omitempty"`
		OK      bool     `json:"ok"`
	}
	var names []check
	add := func(n, purpose string) {
		if n != "" {
			names = append(names, check{Name: n, Purpose: purpose})
		}
	}
	add(st.Domain, "Dashboard")
	if st.AppDomain != "" {
		add("probe."+st.AppDomain, "App wildcard (*."+st.AppDomain+")")
	}
	if st.MCPEnabled {
		add(st.MCPDomain, "MCP endpoint")
	}
	for _, h := range st.Hostnames {
		add(h, "Extra hostname")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	for i := range names {
		addrs, err := net.DefaultResolver.LookupHost(ctx, names[i].Name)
		if err != nil {
			names[i].Error = err.Error()
			continue
		}
		names[i].Records = addrs
		names[i].OK = st.PublicIP == "" || contains(addrs, st.PublicIP)
		if !names[i].OK {
			names[i].Error = "does not point at the public IP " + st.PublicIP
		}
	}
	writeJSON(w, 200, map[string]any{"mode": st.IngressMode, "publicIp": st.PublicIP, "checks": names})
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func (s *Server) adminAudit(w http.ResponseWriter, r *http.Request, u *store.User) {
	out := []*store.AuditEntry{}
	s.P.Store.Read(func(d *store.Data) {
		for i := len(d.Audit) - 1; i >= 0 && len(out) < 300; i-- {
			out = append(out, d.Audit[i])
		}
	})
	writeJSON(w, 200, out)
}

// ---- GitHub ----

func (s *Server) githubStatus(w http.ResponseWriter, r *http.Request, u *store.User) {
	var st store.GitHubAppConfig
	var login string
	s.P.Store.Read(func(d *store.Data) {
		st = d.Settings.GitHub
		login = d.Users[u.ID].GitHubLogin
	})
	out := map[string]any{"configured": st.ClientID != "", "slug": st.Slug, "login": login}
	if st.Slug != "" {
		out["installUrl"] = "https://github.com/apps/" + st.Slug + "/installations/new"
	}
	writeJSON(w, 200, out)
}

// githubManifest returns the form an admin posts to GitHub to create the
// app: GitHub generates the credentials and hands them back, so there is
// no step copying six values between two tabs.
func (s *Server) githubManifest(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct {
		Org string `json:"org"` // create under a GitHub organization
	}
	readJSON(r, &in)
	name := "Wack Club Orchard"
	s.P.Store.Read(func(d *store.Data) {
		if d.Settings.InstanceName != "" {
			name = d.Settings.InstanceName
		}
	})
	manifest := github.Manifest(name+" "+platform.RandHex(2), s.baseURL(r))
	b, _ := json.Marshal(manifest)
	state := s.newChallenge(u.ID, "manifest")
	action := "https://github.com/settings/apps/new?state=" + url.QueryEscape(state)
	if in.Org != "" {
		action = "https://github.com/organizations/" + url.PathEscape(in.Org) + "/settings/apps/new?state=" + url.QueryEscape(state)
	}
	writeJSON(w, 200, map[string]string{"action": action, "manifest": string(b)})
}

func (s *Server) githubManifestCallback(w http.ResponseWriter, r *http.Request, u *store.User) {
	if ch, ok := s.takeChallenge(r.URL.Query().Get("state")); !ok || ch.userID != u.ID || ch.extra != "manifest" {
		http.Redirect(w, r, "/admin?error="+url.QueryEscape("GitHub setup expired; start again"), http.StatusFound)
		return
	}
	cfg, err := github.ConvertManifest(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Redirect(w, r, "/admin?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	s.P.Store.Write(func(d *store.Data) error {
		d.Settings.GitHub = cfg
		return nil
	})
	s.P.Audit("", u, "instance.github_app_created", cfg.Slug, nil)
	http.Redirect(w, r, "https://github.com/apps/"+cfg.Slug+"/installations/new", http.StatusFound)
}

func (s *Server) githubConnect(w http.ResponseWriter, r *http.Request, u *store.User) {
	var cfg store.GitHubAppConfig
	s.P.Store.Read(func(d *store.Data) { cfg = d.Settings.GitHub })
	if cfg.ClientID == "" {
		writeErr(w, 400, "an instance admin needs to set up the GitHub App first")
		return
	}
	state := s.newChallenge(u.ID, r.URL.Query().Get("next"))
	http.Redirect(w, r, github.AuthorizeURL(cfg, s.baseURL(r)+"/api/github/callback", state), http.StatusFound)
}

func (s *Server) githubCallback(w http.ResponseWriter, r *http.Request, u *store.User) {
	ch, ok := s.takeChallenge(r.URL.Query().Get("state"))
	if !ok || ch.userID != u.ID {
		// installs started from GitHub arrive without our state
		if r.URL.Query().Get("installation_id") == "" {
			http.Redirect(w, r, "/account?error="+url.QueryEscape("GitHub sign-in expired"), http.StatusFound)
			return
		}
	}
	var cfg store.GitHubAppConfig
	s.P.Store.Read(func(d *store.Data) { cfg = d.Settings.GitHub })
	tok, err := github.ExchangeCode(r.Context(), cfg, r.URL.Query().Get("code"))
	if err != nil {
		http.Redirect(w, r, "/account?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	login, _ := github.Login(r.Context(), tok)
	s.P.Store.Write(func(d *store.Data) error {
		d.Users[u.ID].GitHubToken = tok
		d.Users[u.ID].GitHubLogin = login
		return nil
	})
	next := ch.extra
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		next = "/account"
	}
	http.Redirect(w, r, next, http.StatusFound)
}

func (s *Server) userToken(u *store.User) string {
	var t string
	s.P.Store.Read(func(d *store.Data) { t = d.Users[u.ID].GitHubToken })
	return t
}

func (s *Server) githubRepos(w http.ResponseWriter, r *http.Request, u *store.User) {
	tok := s.userToken(u)
	if tok == "" {
		writeErr(w, 409, "link your GitHub account first")
		return
	}
	repos, err := github.Repos(r.Context(), tok)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	if repos == nil {
		repos = []github.Repo{}
	}
	writeJSON(w, 200, repos)
}

func (s *Server) githubBranches(w http.ResponseWriter, r *http.Request, u *store.User) {
	bs, err := github.Branches(r.Context(), s.userToken(u), r.URL.Query().Get("repo"))
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, bs)
}

// githubInspect reads the Dockerfile: build stages, the exposed port and
// declared variables, so the new-app form can fill itself in.
func (s *Server) githubInspect(w http.ResponseWriter, r *http.Request, u *store.User) {
	q := r.URL.Query()
	in, err := github.Inspect(r.Context(), s.userToken(u), q.Get("repo"), q.Get("branch"), q.Get("dockerfile"))
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, in)
}

// githubWebhook deploys every auto-deploying app that tracks the pushed
// branch.
func (s *Server) githubWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 25<<20))
	if err != nil {
		writeErr(w, 400, "bad body")
		return
	}
	var secret string
	s.P.Store.Read(func(d *store.Data) { secret = d.Settings.GitHub.WebhookSecret })
	if !github.VerifyWebhook(secret, body, r.Header.Get("X-Hub-Signature-256")) {
		writeErr(w, 401, "bad signature")
		return
	}
	if r.Header.Get("X-GitHub-Event") != "push" {
		writeJSON(w, 200, map[string]string{"ignored": r.Header.Get("X-GitHub-Event")})
		return
	}
	var ev github.PushEvent
	json.Unmarshal(body, &ev)
	if ev.Deleted || !strings.HasPrefix(ev.Ref, "refs/heads/") {
		writeJSON(w, 200, map[string]string{"ignored": "not a branch push"})
		return
	}
	branch := strings.TrimPrefix(ev.Ref, "refs/heads/")
	type target struct {
		appID string
		user  *store.User
	}
	var targets []target
	s.P.Store.Read(func(d *store.Data) {
		for _, a := range d.Apps {
			if a.Source.Type == "github" && a.Source.AutoDeploy && strings.EqualFold(a.Source.Repo, ev.Repository.FullName) && a.Source.Branch == branch {
				targets = append(targets, target{a.ID, store.Clone(d.Users[a.CreatedBy])})
			}
		}
	})
	msg := ""
	if ev.HeadCommit != nil {
		msg = strings.SplitN(ev.HeadCommit.Message, "\n", 2)[0]
	}
	var ids []string
	for _, t := range targets {
		res, err := s.P.Deploy(&store.User{Username: ev.Pusher.Name, Name: ev.Pusher.Name}, t.appID, platform.DeployOpts{Trigger: "push", Commit: ev.After, Message: msg})
		if err == nil {
			ids = append(ids, res.Deploy.ID)
		}
	}
	writeJSON(w, 200, map[string]any{"deploys": ids})
}

// ---- SCIM 2.0 (Users) ----

func (s *Server) scimOrg(w http.ResponseWriter, r *http.Request) *store.Org {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	hash := auth.HashToken(tok)
	var org *store.Org
	s.P.Store.Read(func(d *store.Data) {
		for _, o := range d.Orgs {
			if o.SCIMToken != "" && auth.Equal(o.SCIMToken, hash) {
				org = store.Clone(o)
			}
		}
	})
	if org == nil {
		w.Header().Set("Content-Type", "application/scim+json")
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]any{"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:Error"}, "status": "401", "detail": "bad token"})
	}
	return org
}

func scimUser(u *store.User, active bool) map[string]any {
	return map[string]any{
		"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		"id":          u.ID,
		"externalId":  u.ExternalID,
		"userName":    u.Email,
		"name":        map[string]string{"formatted": u.Name},
		"displayName": u.Name,
		"emails":      []map[string]any{{"value": u.Email, "primary": true}},
		"active":      active && !u.Disabled,
		"meta":        map[string]string{"resourceType": "User", "created": u.CreatedAt.Format(time.RFC3339)},
	}
}

func scimJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) scimUsers(w http.ResponseWriter, r *http.Request) {
	org := s.scimOrg(w, r)
	if org == nil {
		return
	}
	filter := r.URL.Query().Get("filter") // userName eq "x"
	var want string
	if i := strings.Index(filter, `"`); i >= 0 {
		want = strings.Trim(filter[i:], `"`)
	}
	var res []map[string]any
	s.P.Store.Read(func(d *store.Data) {
		for _, m := range d.Memberships {
			if m.OrgID != org.ID {
				continue
			}
			u := d.Users[m.UserID]
			if u == nil || (want != "" && !strings.EqualFold(u.Email, want) && !strings.EqualFold(u.Username, want)) {
				continue
			}
			res = append(res, scimUser(u, true))
		}
	})
	sort.Slice(res, func(i, j int) bool { return fmt.Sprint(res[i]["userName"]) < fmt.Sprint(res[j]["userName"]) })
	if res == nil {
		res = []map[string]any{}
	}
	scimJSON(w, 200, map[string]any{"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"}, "totalResults": len(res), "itemsPerPage": len(res), "startIndex": 1, "Resources": res})
}

func (s *Server) scimCreateUser(w http.ResponseWriter, r *http.Request) {
	org := s.scimOrg(w, r)
	if org == nil {
		return
	}
	var in struct {
		UserName    string `json:"userName"`
		ExternalID  string `json:"externalId"`
		DisplayName string `json:"displayName"`
		Name        struct {
			Formatted  string `json:"formatted"`
			GivenName  string `json:"givenName"`
			FamilyName string `json:"familyName"`
		} `json:"name"`
		Emails []struct {
			Value string `json:"value"`
		} `json:"emails"`
	}
	readJSON(r, &in)
	email := in.UserName
	if !strings.Contains(email, "@") && len(in.Emails) > 0 {
		email = in.Emails[0].Value
	}
	var out *store.User
	s.P.Store.Write(func(d *store.Data) error {
		u := d.UserByName(email)
		if u == nil {
			base := store.Slugify(strings.Split(email, "@")[0])
			uname := base
			for i := 2; d.UserByName(uname) != nil; i++ {
				uname = fmt.Sprintf("%s%d", base, i)
			}
			name := in.DisplayName
			if name == "" {
				name = strings.TrimSpace(in.Name.GivenName + " " + in.Name.FamilyName)
			}
			if name == "" {
				name = uname
			}
			u = &store.User{ID: store.NewID("usr"), Username: uname, Name: name, Email: email, ExternalID: in.ExternalID, AvatarSeed: platform.RandHex(4), CreatedAt: time.Now()}
			d.Users[u.ID] = u
		}
		u.Disabled = false
		if d.Membership(org.ID, u.ID) == nil {
			role := org.SSO.DefaultRole
			if role == "" {
				role = store.RoleMember
			}
			m := &store.Membership{ID: store.NewID("mem"), OrgID: org.ID, UserID: u.ID, Role: role, CreatedAt: time.Now()}
			d.Memberships[m.ID] = m
			d.AddAudit(&store.AuditEntry{OrgID: org.ID, Actor: "scim", Action: "member.provisioned", Target: u.Username})
		}
		out = store.Clone(u)
		return nil
	})
	scimJSON(w, 201, scimUser(out, true))
}

func (s *Server) scimUser(w http.ResponseWriter, r *http.Request) {
	org := s.scimOrg(w, r)
	if org == nil {
		return
	}
	id := r.PathValue("id")
	var u *store.User
	s.P.Store.Read(func(d *store.Data) {
		if x := d.Users[id]; x != nil && d.Membership(org.ID, id) != nil {
			u = store.Clone(x)
		}
	})
	if u == nil {
		scimJSON(w, 404, map[string]any{"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:Error"}, "status": "404"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		scimJSON(w, 200, scimUser(u, true))
	case http.MethodDelete:
		s.deprovision(org, id)
		w.WriteHeader(204)
	default: // PATCH / PUT: we honor "active"
		body, _ := io.ReadAll(r.Body)
		active := true
		var put struct {
			Active *bool `json:"active"`
		}
		var patch struct {
			Operations []struct {
				Op    string          `json:"op"`
				Path  string          `json:"path"`
				Value json.RawMessage `json:"value"`
			} `json:"Operations"`
		}
		if json.Unmarshal(body, &patch) == nil && len(patch.Operations) > 0 {
			for _, op := range patch.Operations {
				var b bool
				if strings.EqualFold(op.Path, "active") && json.Unmarshal(op.Value, &b) == nil {
					active = b
				}
				var m map[string]any
				if json.Unmarshal(op.Value, &m) == nil {
					if v, ok := m["active"].(bool); ok {
						active = v
					}
				}
			}
		} else if json.Unmarshal(body, &put) == nil && put.Active != nil {
			active = *put.Active
		}
		if !active {
			s.deprovision(org, id)
		}
		scimJSON(w, 200, scimUser(u, active))
	}
}

// deprovision removes the member from the org and ends their sessions if
// they belong to no other organization.
func (s *Server) deprovision(org *store.Org, userID string) {
	s.P.Store.Write(func(d *store.Data) error {
		if m := d.Membership(org.ID, userID); m != nil {
			delete(d.Memberships, m.ID)
		}
		if len(d.UserOrgs(userID)) == 0 {
			if u := d.Users[userID]; u != nil && !u.Superadmin {
				u.Disabled = true
			}
			for k, sess := range d.Sessions {
				if sess.UserID == userID {
					delete(d.Sessions, k)
				}
			}
		}
		d.AddAudit(&store.AuditEntry{OrgID: org.ID, Actor: "scim", Action: "member.deprovisioned", Target: userID})
		return nil
	})
}
