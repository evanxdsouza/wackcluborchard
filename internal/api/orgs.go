package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/auth"
	"github.com/evanxdsouza/wackcluborchard/internal/platform"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

func (s *Server) createOrg(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	readJSON(r, &in)
	o, err := s.P.CreateOrg(u, in.Name, in.Slug)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(o.ID, u, "org.created", o.Name, nil)
	writeJSON(w, 201, o)
}

type orgView struct {
	*store.Org
	Role     string `json:"role"`
	SCIMSet  bool   `json:"scimConfigured"`
	Members  int    `json:"memberCount"`
	Projects int    `json:"projectCount"`
}

func (s *Server) getOrg(w http.ResponseWriter, r *http.Request, u *store.User) {
	org, ok := s.checkOrg(w, u, r.PathValue("org"), store.RoleViewer)
	if !ok {
		return
	}
	v := orgView{Org: org}
	s.P.Store.Read(func(d *store.Data) {
		v.Role = orgRole(d, u, org.ID)
		for _, m := range d.Memberships {
			if m.OrgID == org.ID {
				v.Members++
			}
		}
		for _, p := range d.Projects {
			if p.OrgID == org.ID {
				v.Projects++
			}
		}
	})
	v.SCIMSet = org.SCIMToken != ""
	v.Org.SCIMToken = ""
	if store.RoleRank(v.Role) < store.RoleRank(store.RoleAdmin) {
		v.Org.SSO.ClientSecret = ""
	} else if v.Org.SSO.ClientSecret != "" {
		v.Org.SSO.ClientSecret = "••••••••"
	}
	writeJSON(w, 200, v)
}

func (s *Server) updateOrg(w http.ResponseWriter, r *http.Request, u *store.User) {
	org, ok := s.checkOrg(w, u, r.PathValue("org"), store.RoleAdmin)
	if !ok {
		return
	}
	var in struct {
		Name        *string          `json:"name"`
		Quota       *store.Quota     `json:"quota"`
		MemberQuota *store.Quota     `json:"memberQuota"`
		Defaults    *store.Resources `json:"defaults"`
		PublicIP    *string          `json:"publicIp"`
		SSO         *store.SSOConfig `json:"sso"`
		Pool        *string          `json:"pool"`
	}
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	if (in.Quota != nil || in.Pool != nil) && !u.Superadmin {
		writeErr(w, 403, "only instance admins change organization caps and node pools")
		return
	}
	s.P.Store.Write(func(d *store.Data) error {
		o := d.Orgs[org.ID]
		if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
			o.Name = strings.TrimSpace(*in.Name)
		}
		if in.Quota != nil {
			o.Quota = *in.Quota
		}
		if in.MemberQuota != nil {
			o.MemberQuota = *in.MemberQuota
		}
		if in.Defaults != nil {
			o.Defaults = *in.Defaults
		}
		if in.PublicIP != nil {
			o.PublicIP = strings.TrimSpace(*in.PublicIP)
		}
		if in.Pool != nil {
			o.Pool = *in.Pool
		}
		if in.SSO != nil {
			sec := o.SSO.ClientSecret
			o.SSO = *in.SSO
			if in.SSO.ClientSecret == "" || strings.HasPrefix(in.SSO.ClientSecret, "••") {
				o.SSO.ClientSecret = sec
			}
		}
		return nil
	})
	s.P.Audit(org.ID, u, "org.updated", org.Name, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) deleteOrg(w http.ResponseWriter, r *http.Request, u *store.User) {
	org, ok := s.checkOrg(w, u, r.PathValue("org"), store.RoleOwner)
	if !ok {
		return
	}
	var projects []string
	s.P.Store.Read(func(d *store.Data) {
		for _, p := range d.Projects {
			if p.OrgID == org.ID {
				projects = append(projects, p.ID)
			}
		}
	})
	for _, p := range projects {
		s.P.DeleteProject(u, p)
	}
	s.P.Store.Write(func(d *store.Data) error {
		for k, m := range d.Memberships {
			if m.OrgID == org.ID {
				delete(d.Memberships, k)
			}
		}
		for k, sb := range d.Sandboxes {
			if sb.OrgID == org.ID {
				delete(d.Sandboxes, k)
			}
		}
		delete(d.Orgs, org.ID)
		return nil
	})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type memberView struct {
	Brief
	Email    string         `json:"email,omitempty"`
	Role     string         `json:"role"`
	JoinedAt time.Time      `json:"joinedAt"`
	Projects int            `json:"projects"`
	Usage    platform.Usage `json:"usage"`
}

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request, u *store.User) {
	org, ok := s.checkOrg(w, u, r.PathValue("org"), store.RoleViewer)
	if !ok {
		return
	}
	var out struct {
		Members []memberView    `json:"members"`
		Invites []*store.Invite `json:"invites"`
	}
	out.Members = []memberView{}
	out.Invites = []*store.Invite{}
	s.P.Store.Read(func(d *store.Data) {
		admin := store.RoleRank(orgRole(d, u, org.ID)) >= store.RoleRank(store.RoleAdmin)
		for _, m := range d.Memberships {
			if m.OrgID != org.ID {
				continue
			}
			x := d.Users[m.UserID]
			if x == nil {
				continue
			}
			mv := memberView{Brief: brief(x), Role: m.Role, JoinedAt: m.CreatedAt, Usage: platform.OrgUsage(d, org.ID, x.ID)}
			if admin {
				mv.Email = x.Email
			}
			for _, p := range d.Projects {
				if p.OrgID == org.ID {
					for _, id := range p.Members {
						if id == x.ID {
							mv.Projects++
						}
					}
				}
			}
			out.Members = append(out.Members, mv)
		}
		if admin {
			for _, i := range d.Invites {
				if i.OrgID == org.ID {
					out.Invites = append(out.Invites, store.Clone(i))
				}
			}
		}
	})
	sort.Slice(out.Members, func(i, j int) bool {
		if store.RoleRank(out.Members[i].Role) != store.RoleRank(out.Members[j].Role) {
			return store.RoleRank(out.Members[i].Role) > store.RoleRank(out.Members[j].Role)
		}
		return out.Members[i].Username < out.Members[j].Username
	})
	writeJSON(w, 200, out)
}

func validRole(r string) bool {
	return r == store.RoleOwner || r == store.RoleAdmin || r == store.RoleMember || r == store.RoleViewer
}

func (s *Server) invite(w http.ResponseWriter, r *http.Request, u *store.User) {
	org, ok := s.checkOrg(w, u, r.PathValue("org"), store.RoleAdmin)
	if !ok {
		return
	}
	var in struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	readJSON(r, &in)
	if in.Role == "" {
		in.Role = store.RoleMember
	}
	if !validRole(in.Role) {
		writeErr(w, 400, "unknown role")
		return
	}
	var myRole string
	s.P.Store.Read(func(d *store.Data) { myRole = orgRole(d, u, org.ID) })
	if in.Role == store.RoleOwner && myRole != store.RoleOwner {
		writeErr(w, 403, "only owners can add owners")
		return
	}
	// an existing user is added directly
	if in.Username != "" {
		err := s.P.Store.Write(func(d *store.Data) error {
			x := d.UserByName(in.Username)
			if x == nil {
				return platform.Invalid("no user called %q", in.Username)
			}
			if d.Membership(org.ID, x.ID) != nil {
				return platform.Invalid("%s is already a member", x.Username)
			}
			m := &store.Membership{ID: store.NewID("mem"), OrgID: org.ID, UserID: x.ID, Role: in.Role, CreatedAt: time.Now()}
			d.Memberships[m.ID] = m
			return nil
		})
		if err != nil {
			s.fail(w, err)
			return
		}
		s.P.Audit(org.ID, u, "member.added", in.Username, map[string]string{"role": in.Role})
		writeJSON(w, 201, map[string]bool{"ok": true})
		return
	}
	inv := &store.Invite{ID: store.NewID("inv"), OrgID: org.ID, Email: in.Email, Role: in.Role, Token: auth.Token("", 18), CreatedBy: u.ID, CreatedAt: time.Now()}
	s.P.Store.Write(func(d *store.Data) error {
		d.Invites[inv.ID] = inv
		return nil
	})
	s.P.Audit(org.ID, u, "member.invited", in.Email, map[string]string{"role": in.Role})
	writeJSON(w, 201, map[string]any{"invite": inv, "url": s.baseURL(r) + "/invite/" + inv.Token})
}

func (s *Server) deleteInvite(w http.ResponseWriter, r *http.Request, u *store.User) {
	org, ok := s.checkOrg(w, u, r.PathValue("org"), store.RoleAdmin)
	if !ok {
		return
	}
	s.P.Store.Write(func(d *store.Data) error {
		if i := d.Invites[r.PathValue("id")]; i != nil && i.OrgID == org.ID {
			delete(d.Invites, i.ID)
		}
		return nil
	})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) getInvite(w http.ResponseWriter, r *http.Request) {
	tok := r.PathValue("token")
	var out map[string]any
	s.P.Store.Read(func(d *store.Data) {
		for _, i := range d.Invites {
			if auth.Equal(i.Token, tok) {
				o := d.Orgs[i.OrgID]
				out = map[string]any{"org": o.Name, "role": i.Role, "email": i.Email, "invitedBy": brief(d.Users[i.CreatedBy])}
			}
		}
	})
	if out == nil {
		writeErr(w, 404, "that invite link is not valid")
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request, u *store.User) {
	tok := r.PathValue("token")
	var orgSlug string
	err := s.P.Store.Write(func(d *store.Data) error {
		for _, i := range d.Invites {
			if auth.Equal(i.Token, tok) {
				if d.Membership(i.OrgID, u.ID) == nil {
					m := &store.Membership{ID: store.NewID("mem"), OrgID: i.OrgID, UserID: u.ID, Role: i.Role, CreatedAt: time.Now()}
					d.Memberships[m.ID] = m
					d.AddAudit(&store.AuditEntry{OrgID: i.OrgID, ActorID: u.ID, Actor: u.Username, Action: "member.joined", Target: u.Username})
				}
				orgSlug = d.Orgs[i.OrgID].Slug
				delete(d.Invites, i.ID)
				return nil
			}
		}
		return platform.Invalid("that invite link is not valid")
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"org": orgSlug})
}

func (s *Server) updateMember(w http.ResponseWriter, r *http.Request, u *store.User) {
	org, ok := s.checkOrg(w, u, r.PathValue("org"), store.RoleAdmin)
	if !ok {
		return
	}
	var in struct {
		Role string `json:"role"`
	}
	readJSON(r, &in)
	if !validRole(in.Role) {
		writeErr(w, 400, "unknown role")
		return
	}
	target := r.PathValue("user")
	err := s.P.Store.Write(func(d *store.Data) error {
		me := orgRole(d, u, org.ID)
		m := d.Membership(org.ID, target)
		if m == nil {
			return platform.Invalid("not a member")
		}
		if (m.Role == store.RoleOwner || in.Role == store.RoleOwner) && me != store.RoleOwner {
			return platform.Invalid("only owners can change owners")
		}
		if m.Role == store.RoleOwner && in.Role != store.RoleOwner {
			owners := 0
			for _, x := range d.Memberships {
				if x.OrgID == org.ID && x.Role == store.RoleOwner {
					owners++
				}
			}
			if owners <= 1 {
				return platform.Invalid("an organization needs at least one owner")
			}
		}
		m.Role = in.Role
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(org.ID, u, "member.role_changed", target, map[string]string{"role": in.Role})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request, u *store.User) {
	target := r.PathValue("user")
	min := store.RoleAdmin
	if target == u.ID {
		min = store.RoleViewer // anyone can leave
	}
	org, ok := s.checkOrg(w, u, r.PathValue("org"), min)
	if !ok {
		return
	}
	err := s.P.Store.Write(func(d *store.Data) error {
		m := d.Membership(org.ID, target)
		if m == nil {
			return platform.Invalid("not a member")
		}
		if m.Role == store.RoleOwner {
			owners := 0
			for _, x := range d.Memberships {
				if x.OrgID == org.ID && x.Role == store.RoleOwner {
					owners++
				}
			}
			if owners <= 1 {
				return platform.Invalid("an organization needs at least one owner")
			}
			if orgRole(d, u, org.ID) != store.RoleOwner {
				return platform.Invalid("only owners can remove owners")
			}
		}
		delete(d.Memberships, m.ID)
		for _, p := range d.Projects {
			if p.OrgID == org.ID {
				var keep []string
				for _, id := range p.Members {
					if id != target {
						keep = append(keep, id)
					}
				}
				p.Members = keep
			}
		}
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(org.ID, u, "member.removed", target, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) audit(w http.ResponseWriter, r *http.Request, u *store.User) {
	org, ok := s.checkOrg(w, u, r.PathValue("org"), store.RoleAdmin)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	q := strings.ToLower(r.URL.Query().Get("q"))
	out := []*store.AuditEntry{}
	s.P.Store.Read(func(d *store.Data) {
		for i := len(d.Audit) - 1; i >= 0 && len(out) < limit; i-- {
			e := d.Audit[i]
			if e.OrgID != org.ID {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(e.Action+" "+e.Actor+" "+e.Target), q) {
				continue
			}
			out = append(out, e)
		}
	})
	writeJSON(w, 200, out)
}

func (s *Server) rotateSCIM(w http.ResponseWriter, r *http.Request, u *store.User) {
	org, ok := s.checkOrg(w, u, r.PathValue("org"), store.RoleOwner)
	if !ok {
		return
	}
	tok := auth.Token("scim_", 30)
	s.P.Store.Write(func(d *store.Data) error {
		d.Orgs[org.ID].SCIMToken = auth.HashToken(tok)
		return nil
	})
	s.P.Audit(org.ID, u, "scim.token_rotated", org.Name, nil)
	writeJSON(w, 200, map[string]string{"token": tok, "baseUrl": s.baseURL(r) + "/scim/v2"})
}

// usage: the org's caps and use, the member's own allowance, and a
// per-project breakdown.
func (s *Server) usage(w http.ResponseWriter, r *http.Request, u *store.User) {
	org, ok := s.checkOrg(w, u, r.PathValue("org"), store.RoleViewer)
	if !ok {
		return
	}
	type projectUsage struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		Icon      string  `json:"icon"`
		CPUMillis int     `json:"cpuMillis"`
		MemoryMi  int     `json:"memoryMi"`
		StorageGi int     `json:"storageGi"`
		Apps      int     `json:"apps"`
		Databases int     `json:"databases"`
		LiveCPU   float64 `json:"liveCpuMillis"`
		LiveMem   float64 `json:"liveMemoryMi"`
	}
	var out struct {
		Quota       store.Quota    `json:"quota"`
		Used        platform.Usage `json:"used"`
		MemberQuota store.Quota    `json:"memberQuota"`
		MemberUsed  platform.Usage `json:"memberUsed"`
		Projects    []projectUsage `json:"projects"`
		Builds      map[string]int `json:"builds"`
	}
	live := s.P.LatestMetrics()
	s.P.Store.Read(func(d *store.Data) {
		out.Quota = org.Quota
		out.MemberQuota = org.MemberQuota
		out.Used = platform.OrgUsage(d, org.ID, "")
		out.MemberUsed = platform.OrgUsage(d, org.ID, u.ID)
		for _, p := range d.Projects {
			if p.OrgID != org.ID || !canViewProject(d, u, p) {
				continue
			}
			pu := projectUsage{ID: p.ID, Name: p.Name, Icon: p.Icon}
			for _, a := range d.AppsIn(p.ID) {
				pu.Apps++
				pu.CPUMillis += a.Resources.CPUMillis * a.Replicas
				pu.MemoryMi += a.Resources.MemoryMi * a.Replicas
				for _, v := range a.Volumes {
					pu.StorageGi += v.SizeGi
				}
				pu.LiveCPU += live[a.ID].CPUMillis
				pu.LiveMem += live[a.ID].MemoryMi
			}
			for _, db := range d.DatabasesIn(p.ID) {
				pu.Databases++
				n := max(db.Instances, 1)
				if db.Status != "stopped" {
					pu.CPUMillis += db.Resources.CPUMillis * n
					pu.MemoryMi += db.Resources.MemoryMi * n
				}
				pu.StorageGi += db.StorageGi * n
			}
			out.Projects = append(out.Projects, pu)
		}
	})
	run, wait, slots := s.P.BuildStats()
	out.Builds = map[string]int{"running": run, "waiting": wait, "slots": slots}
	sort.Slice(out.Projects, func(i, j int) bool { return out.Projects[i].CPUMillis > out.Projects[j].CPUMillis })
	writeJSON(w, 200, out)
}

// ---- overview (Your Apps) ----

type appCard struct {
	*store.App
	Metrics *platform.MetricPoint `json:"metrics,omitempty"`
	Creator Brief                 `json:"creator"`
}

type projectCard struct {
	*store.Project
	Apps      []appCard         `json:"apps"`
	Databases []*store.Database `json:"databases"`
	Owner     Brief             `json:"owner"`
	CanEdit   bool              `json:"canEdit"`
}

func sanitizeDB(db *store.Database, canEdit bool) *store.Database {
	c := store.Clone(db)
	if !canEdit {
		c.Password = ""
	}
	return c
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request, u *store.User) {
	org, ok := s.checkOrg(w, u, r.PathValue("org"), store.RoleViewer)
	if !ok {
		return
	}
	live := s.P.LatestMetrics()
	out := []projectCard{}
	s.P.Store.Read(func(d *store.Data) {
		for _, p := range store.Values(d.Projects, func(a, b *store.Project) bool { return a.UpdatedAt.After(b.UpdatedAt) }) {
			if p.OrgID != org.ID || !canViewProject(d, u, p) {
				continue
			}
			edit := canEditProject(d, u, p)
			pc := projectCard{Project: store.Clone(p), Apps: []appCard{}, Databases: []*store.Database{}, Owner: brief(d.Users[p.CreatedBy]), CanEdit: edit}
			for _, a := range d.AppsIn(p.ID) {
				c := appCard{App: store.Clone(a), Creator: brief(d.Users[a.CreatedBy])}
				if m, ok := live[a.ID]; ok {
					c.Metrics = &m
				}
				pc.Apps = append(pc.Apps, c)
			}
			for _, db := range d.DatabasesIn(p.ID) {
				pc.Databases = append(pc.Databases, sanitizeDB(db, edit))
			}
			out = append(out, pc)
		}
	})
	writeJSON(w, 200, out)
}

type deployView struct {
	*store.Deploy
	AppName     string `json:"appName"`
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"projectName"`
}

func (s *Server) recentDeploys(w http.ResponseWriter, r *http.Request, u *store.User) {
	org, ok := s.checkOrg(w, u, r.PathValue("org"), store.RoleViewer)
	if !ok {
		return
	}
	out := []deployView{}
	cutoff := time.Now().Add(-24 * time.Hour)
	s.P.Store.Read(func(d *store.Data) {
		for _, dep := range d.Deploys {
			a := d.Apps[dep.AppID]
			if a == nil || dep.CreatedAt.Before(cutoff) {
				continue
			}
			p := d.Projects[a.ProjectID]
			if p == nil || p.OrgID != org.ID || !canViewProject(d, u, p) {
				continue
			}
			out = append(out, deployView{Deploy: store.Clone(dep), AppName: a.Name, ProjectID: p.ID, ProjectName: p.Name})
		}
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > 12 {
		out = out[:12]
	}
	writeJSON(w, 200, out)
}

// search powers the ⌘K palette.
func (s *Server) search(w http.ResponseWriter, r *http.Request, u *store.User) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	type hit struct {
		Kind     string `json:"kind"`
		ID       string `json:"id"`
		Title    string `json:"title"`
		Subtitle string `json:"subtitle"`
		Href     string `json:"href"`
		Status   string `json:"status,omitempty"`
	}
	out := []hit{}
	match := func(parts ...string) bool {
		if q == "" {
			return true
		}
		for _, p := range parts {
			if strings.Contains(strings.ToLower(p), q) {
				return true
			}
		}
		return false
	}
	s.P.Store.Read(func(d *store.Data) {
		for _, p := range d.Projects {
			if !canViewProject(d, u, p) {
				continue
			}
			org := d.Orgs[p.OrgID]
			if match(p.Name) {
				out = append(out, hit{"project", p.ID, p.Name, org.Name, "/o/" + org.Slug + "/projects/" + p.ID, ""})
			}
			for _, a := range d.AppsIn(p.ID) {
				img := a.Source.Image
				if a.Source.Type == "github" {
					img = a.Source.Repo
				}
				if match(a.Name, img) {
					out = append(out, hit{"app", a.ID, a.Name, p.Name + " · " + img, "/o/" + org.Slug + "/apps/" + a.ID, a.Status})
				}
			}
			for _, db := range d.DatabasesIn(p.ID) {
				if match(db.Name, db.DBName) {
					out = append(out, hit{"database", db.ID, db.Name, p.Name + " · PostgreSQL " + strconv.Itoa(db.Version), "/o/" + org.Slug + "/databases/" + db.ID, db.Status})
				}
			}
			for _, j := range d.Jobs {
				if j.ProjectID == p.ID && match(j.Name) {
					out = append(out, hit{"job", j.ID, j.Name, p.Name + " · job", "/o/" + org.Slug + "/jobs/" + j.ID, ""})
				}
			}
		}
	})
	if len(out) > 40 {
		out = out[:40]
	}
	writeJSON(w, 200, out)
}
