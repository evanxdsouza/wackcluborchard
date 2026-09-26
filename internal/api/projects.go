package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/platform"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

func (s *Server) createProject(w http.ResponseWriter, r *http.Request, u *store.User) {
	org, ok := s.checkOrg(w, u, r.PathValue("org"), store.RoleMember)
	if !ok {
		return
	}
	var in struct {
		Name       string `json:"name"`
		Icon       string `json:"icon"`
		Background string `json:"background"`
	}
	readJSON(r, &in)
	p, err := s.P.CreateProject(u, org.ID, in.Name, in.Icon, in.Background)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(org.ID, u, "project.created", p.Name, nil)
	writeJSON(w, 201, p)
}

type projectDetail struct {
	*store.Project
	Org       map[string]string         `json:"org"`
	Apps      []appCard                 `json:"apps"`
	Databases []*store.Database         `json:"databases"`
	Templates []*store.TemplateInstance `json:"templateInstances"`
	Jobs      []*store.Job              `json:"jobs"`
	People    []memberBrief             `json:"people"`
	CanEdit   bool                      `json:"canEdit"`
	Role      string                    `json:"role"`
	Namespace string                    `json:"namespace"`
}

type memberBrief struct {
	Brief
	Role string `json:"role"`
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	if ok, _ := s.checkProject(w, u, id, view); !ok {
		return
	}
	live := s.P.LatestMetrics()
	var out projectDetail
	s.P.Store.Read(func(d *store.Data) {
		p := d.Projects[id]
		o := d.Orgs[p.OrgID]
		out.Project = store.Clone(p)
		out.Org = map[string]string{"id": o.ID, "slug": o.Slug, "name": o.Name}
		out.CanEdit = canEditProject(d, u, p)
		out.Role = orgRole(d, u, p.OrgID)
		out.Namespace = d.Namespace(p.ID)
		out.Apps = []appCard{}
		out.Databases = []*store.Database{}
		out.Templates = []*store.TemplateInstance{}
		out.Jobs = []*store.Job{}
		for _, a := range d.AppsIn(id) {
			c := appCard{App: store.Clone(a), Creator: brief(d.Users[a.CreatedBy])}
			if m, ok := live[a.ID]; ok {
				c.Metrics = &m
			}
			out.Apps = append(out.Apps, c)
		}
		for _, db := range d.DatabasesIn(id) {
			out.Databases = append(out.Databases, sanitizeDB(db, out.CanEdit))
		}
		for _, t := range d.TemplateInstances {
			if t.ProjectID == id {
				out.Templates = append(out.Templates, store.Clone(t))
			}
		}
		sort.Slice(out.Templates, func(i, j int) bool { return out.Templates[i].CreatedAt.Before(out.Templates[j].CreatedAt) })
		for _, j := range d.Jobs {
			if j.ProjectID == id {
				out.Jobs = append(out.Jobs, store.Clone(j))
			}
		}
		for _, uid := range p.Members {
			if x := d.Users[uid]; x != nil {
				role := ""
				if m := d.Membership(p.OrgID, uid); m != nil {
					role = m.Role
				}
				out.People = append(out.People, memberBrief{brief(x), role})
			}
		}
	})
	writeJSON(w, 200, out)
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, id, edit)
	if !ok {
		return
	}
	var in struct {
		Name       *string `json:"name"`
		Icon       *string `json:"icon"`
		Background *string `json:"background"`
	}
	readJSON(r, &in)
	var p *store.Project
	s.P.Store.Write(func(d *store.Data) error {
		x := d.Projects[id]
		if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
			x.Name = strings.TrimSpace(*in.Name)
		}
		if in.Icon != nil {
			x.Icon = *in.Icon
		}
		if in.Background != nil {
			x.Background = *in.Background
		}
		x.UpdatedAt = time.Now()
		p = store.Clone(x)
		return nil
	})
	s.P.Audit(orgID, u, "project.updated", p.Name, nil)
	s.P.Bus.Publish("project:"+id, "project.updated", p)
	writeJSON(w, 200, p)
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, id, edit)
	if !ok {
		return
	}
	var name string
	var allowed bool
	s.P.Store.Read(func(d *store.Data) {
		p := d.Projects[id]
		name = p.Name
		allowed = p.CreatedBy == u.ID || store.RoleRank(orgRole(d, u, orgID)) >= store.RoleRank(store.RoleAdmin)
	})
	if !allowed {
		writeErr(w, 403, "only the project's creator or an org admin can delete it")
		return
	}
	if err := s.P.DeleteProject(u, id); err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "project.deleted", name, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) addEnvironment(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	if ok, _ := s.checkProject(w, u, id, edit); !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	readJSON(r, &in)
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeErr(w, 400, "name the environment")
		return
	}
	env := store.Environment{ID: store.NewID("env"), Name: in.Name}
	err := s.P.Store.Write(func(d *store.Data) error {
		p := d.Projects[id]
		for _, e := range p.Environments {
			if strings.EqualFold(e.Name, in.Name) {
				return platform.Invalid("an environment called %s exists", in.Name)
			}
		}
		p.Environments = append(p.Environments, env)
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 201, env)
}

func (s *Server) deleteEnvironment(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	if ok, _ := s.checkProject(w, u, id, edit); !ok {
		return
	}
	envID := r.PathValue("env")
	err := s.P.Store.Write(func(d *store.Data) error {
		p := d.Projects[id]
		if len(p.Environments) <= 1 {
			return platform.Invalid("a project needs at least one environment")
		}
		for _, a := range d.Apps {
			if a.ProjectID == id && a.EnvID == envID {
				return platform.Invalid("move or delete the apps in this environment first")
			}
		}
		var keep []store.Environment
		for _, e := range p.Environments {
			if e.ID != envID {
				keep = append(keep, e)
			}
		}
		p.Environments = keep
		for k, v := range d.Variables {
			if v.EnvID == envID {
				delete(d.Variables, k)
			}
		}
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) addProjectMember(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, id, edit)
	if !ok {
		return
	}
	var in struct {
		UserID   string `json:"userId"`
		Username string `json:"username"`
	}
	readJSON(r, &in)
	err := s.P.Store.Write(func(d *store.Data) error {
		target := d.Users[in.UserID]
		if target == nil {
			target = d.UserByName(in.Username)
		}
		if target == nil || d.Membership(orgID, target.ID) == nil {
			return platform.Invalid("add them to the organization first")
		}
		p := d.Projects[id]
		for _, m := range p.Members {
			if m == target.ID {
				return nil
			}
		}
		p.Members = append(p.Members, target.ID)
		in.Username = target.Username
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "project.member_added", in.Username, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) removeProjectMember(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, id, edit)
	if !ok {
		return
	}
	target := r.PathValue("user")
	s.P.Store.Write(func(d *store.Data) error {
		p := d.Projects[id]
		var keep []string
		for _, m := range p.Members {
			if m != target {
				keep = append(keep, m)
			}
		}
		p.Members = keep
		return nil
	})
	s.P.Audit(orgID, u, "project.member_removed", target, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---- variables ----

type varView struct {
	*store.Variable
	AppName string `json:"appName,omitempty"`
}

func (s *Server) listVariables(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	if ok, _ := s.checkProject(w, u, id, view); !ok {
		return
	}
	appFilter := r.URL.Query().Get("app")
	out := []varView{}
	s.P.Store.Read(func(d *store.Data) {
		reveal := canEditProject(d, u, d.Projects[id])
		for _, v := range d.Variables {
			if v.ProjectID != id {
				continue
			}
			if appFilter != "" && v.AppID != appFilter {
				continue
			}
			c := store.Clone(v)
			if c.Secret && !reveal {
				c.Value = ""
			}
			vv := varView{Variable: c}
			if a := d.Apps[v.AppID]; a != nil {
				vv.AppName = a.Name
			}
			out = append(out, vv)
		}
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].AppID != out[j].AppID {
			return out[i].AppID < out[j].AppID
		}
		return out[i].Key < out[j].Key
	})
	writeJSON(w, 200, out)
}

func validKey(k string) bool {
	if k == "" || len(k) > 128 {
		return false
	}
	for i, r := range k {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9' || i > 0 && (r == '.' || r == '-')) {
			return false
		}
	}
	return true
}

// setVariables upserts a batch. Changing variables redeploys nothing by
// itself; affected apps are reconciled so the new values roll out.
func (s *Server) setVariables(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, id, edit)
	if !ok {
		return
	}
	var in struct {
		Variables []store.Variable `json:"variables"`
		Delete    []string         `json:"delete"`
		// Raw .env text for bulk import into one scope
		Env    string `json:"env"`
		EnvID  string `json:"envId"`
		AppID  string `json:"appId"`
		Secret bool   `json:"secret"`
	}
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	if in.Env != "" {
		for _, line := range strings.Split(in.Env, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			line = strings.TrimPrefix(line, "export ")
			kv := strings.SplitN(line, "=", 2)
			if len(kv) != 2 {
				continue
			}
			v := strings.TrimSpace(kv[1])
			if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			}
			in.Variables = append(in.Variables, store.Variable{Key: strings.TrimSpace(kv[0]), Value: v, EnvID: in.EnvID, AppID: in.AppID, Secret: in.Secret})
		}
	}
	affected := map[string]bool{}
	err := s.P.Store.Write(func(d *store.Data) error {
		for _, v := range in.Variables {
			if !validKey(v.Key) {
				return platform.Invalid("%q is not a valid variable name", v.Key)
			}
			if v.AppID != "" {
				if a := d.Apps[v.AppID]; a == nil || a.ProjectID != id {
					return platform.Invalid("unknown app for variable %s", v.Key)
				}
			}
		}
		for _, delID := range in.Delete {
			if v := d.Variables[delID]; v != nil && v.ProjectID == id {
				delete(d.Variables, delID)
				affected[v.AppID] = true
			}
		}
		for _, v := range in.Variables {
			var existing *store.Variable
			if v.ID != "" {
				existing = d.Variables[v.ID]
			}
			if existing == nil {
				for _, x := range d.Variables {
					if x.ProjectID == id && x.Key == v.Key && x.EnvID == v.EnvID && x.AppID == v.AppID {
						existing = x
					}
				}
			}
			if existing != nil && existing.ProjectID == id {
				existing.Key = v.Key
				// a masked secret sent back unchanged keeps its value
				if !(existing.Secret && v.Value == "") {
					existing.Value = v.Value
				}
				existing.Secret = v.Secret
				existing.EnvID = v.EnvID
				existing.UpdatedAt = time.Now()
			} else {
				nid := store.NewID("var")
				d.Variables[nid] = &store.Variable{ID: nid, ProjectID: id, EnvID: v.EnvID, AppID: v.AppID, Key: v.Key, Value: v.Value, Secret: v.Secret, UpdatedAt: time.Now()}
			}
			affected[v.AppID] = true
		}
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	// project-wide changes touch every app in the project
	var apps []string
	s.P.Store.Read(func(d *store.Data) {
		for _, a := range d.AppsIn(id) {
			if affected[""] || affected[a.ID] {
				apps = append(apps, a.ID)
			}
		}
	})
	for _, a := range apps {
		s.P.EnqueueApp(a)
	}
	s.P.Audit(orgID, u, "variables.updated", id, map[string]string{"count": itoa(len(in.Variables) + len(in.Delete))})
	writeJSON(w, 200, map[string]any{"ok": true, "redeployed": len(apps)})
}

func itoa(n int) string { return strconv.Itoa(n) }

// ---- compose ----

func (s *Server) getCompose(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	if ok, _ := s.checkProject(w, u, id, view); !ok {
		return
	}
	env := r.URL.Query().Get("env")
	var out *store.ComposeStack
	s.P.Store.Read(func(d *store.Data) {
		for _, c := range d.Compose {
			if c.ProjectID == id && (env == "" || c.EnvID == env) {
				out = store.Clone(c)
			}
		}
	})
	if out == nil {
		out = &store.ComposeStack{ProjectID: id, EnvID: env, AppIDs: []string{}}
	}
	writeJSON(w, 200, out)
}

func (s *Server) applyCompose(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, id, edit)
	if !ok {
		return
	}
	var in struct {
		EnvID  string `json:"envId"`
		Source string `json:"source"`
		DryRun bool   `json:"dryRun"`
	}
	readJSON(r, &in)
	if in.EnvID == "" {
		s.P.Store.Read(func(d *store.Data) { in.EnvID = d.Projects[id].Environments[0].ID })
	}
	res, err := s.P.ApplyCompose(u, id, in.EnvID, in.Source, in.DryRun)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !in.DryRun {
		s.P.Audit(orgID, u, "compose.applied", id, nil)
	}
	writeJSON(w, 200, res)
}

// ---- templates ----

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request, u *store.User) {
	writeJSON(w, 200, platform.Templates)
}

func (s *Server) createTemplateInstance(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, id, edit)
	if !ok {
		return
	}
	var in struct {
		TemplateID string `json:"templateId"`
		Name       string `json:"name"`
		EnvID      string `json:"envId"`
	}
	readJSON(r, &in)
	if in.EnvID == "" {
		s.P.Store.Read(func(d *store.Data) { in.EnvID = d.Projects[id].Environments[0].ID })
	}
	inst, err := s.P.InstantiateTemplate(u, id, in.EnvID, in.TemplateID, in.Name)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "template.deployed", inst.Name, map[string]string{"template": in.TemplateID})
	writeJSON(w, 201, inst)
}

func (s *Server) deleteTemplateInstance(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	var pid string
	s.P.Store.Read(func(d *store.Data) {
		if t := d.TemplateInstances[id]; t != nil {
			pid = t.ProjectID
		}
	})
	if ok, _ := s.checkProject(w, u, pid, edit); !ok {
		return
	}
	if err := s.P.DeleteTemplateInstance(u, id); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
