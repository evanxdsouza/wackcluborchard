package api

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/cron"
	"github.com/evanxdsouza/wackcluborchard/internal/platform"
	"github.com/evanxdsouza/wackcluborchard/internal/runtime"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

// ---- databases ----

func (s *Server) createDatabase(w http.ResponseWriter, r *http.Request, u *store.User) {
	pid := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, pid, edit)
	if !ok {
		return
	}
	var in platform.DatabaseInput
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	db, err := s.P.CreateDatabase(u, pid, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "database.created", db.Name, nil)
	writeJSON(w, 201, db)
}

type dbDetail struct {
	*store.Database
	Project    map[string]string `json:"project"`
	Org        map[string]string `json:"org"`
	CanEdit    bool              `json:"canEdit"`
	URI        string            `json:"uri,omitempty"`
	PublicHost string            `json:"publicHost,omitempty"`
	Namespace  string            `json:"namespace"`
	Available  any               `json:"availableExtensions"`
}

func (s *Server) getDatabase(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	if ok, _ := s.checkProject(w, u, s.projectOfDB(id), view); !ok {
		return
	}
	var out dbDetail
	s.P.Store.Read(func(d *store.Data) {
		db := d.Databases[id]
		p := d.Projects[db.ProjectID]
		o := d.Orgs[p.OrgID]
		out.CanEdit = canEditProject(d, u, p)
		out.Database = sanitizeDB(db, out.CanEdit)
		out.Project = map[string]string{"id": p.ID, "name": p.Name, "icon": p.Icon}
		out.Org = map[string]string{"id": o.ID, "slug": o.Slug, "name": o.Name}
		out.Namespace = d.Namespace(p.ID)
		if out.CanEdit {
			out.URI = platform.DatabaseURI(db)
		}
		if db.PublicPort > 0 {
			host := o.PublicIP
			if host == "" {
				host = d.Settings.PublicIP
			}
			if d.Settings.PublicDBDomain != "" {
				host = db.Name + "-" + p.Slug + "." + d.Settings.PublicDBDomain
			}
			out.PublicHost = host
		}
	})
	out.Available = platform.AvailableExtensions
	writeJSON(w, 200, out)
}

func (s *Server) updateDatabase(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, s.projectOfDB(id), edit)
	if !ok {
		return
	}
	var in platform.DatabasePatch
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	db, err := s.P.UpdateDatabase(u, id, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "database.updated", db.Name, nil)
	writeJSON(w, 200, db)
}

func (s *Server) restartDatabase(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, s.projectOfDB(id), edit)
	if !ok {
		return
	}
	s.P.RestartDatabase(id)
	s.P.Audit(orgID, u, "database.restarted", id, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) deleteDatabase(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, s.projectOfDB(id), edit)
	if !ok {
		return
	}
	if err := s.P.DeleteDatabase(id); err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "database.deleted", id, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) queryDatabase(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, s.projectOfDB(id), edit)
	if !ok {
		return
	}
	var in struct {
		SQL string `json:"sql"`
	}
	readJSON(r, &in)
	if strings.TrimSpace(in.SQL) == "" {
		writeErr(w, 400, "write a query")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := s.P.Query(ctx, id, in.SQL)
	if err != nil {
		writeJSON(w, 200, map[string]string{"error": err.Error()})
		return
	}
	first := strings.ToLower(strings.Fields(in.SQL)[0])
	if first != "select" && first != "\\dt" && first != "\\dx" && first != "show" && first != "explain" {
		s.P.Audit(orgID, u, "database.query", id, map[string]string{"command": res.Command})
	}
	writeJSON(w, 200, res)
}

func (s *Server) backupDatabase(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, s.projectOfDB(id), edit)
	if !ok {
		return
	}
	b, err := s.P.BackupNow(id, "manual")
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "database.backup", id, nil)
	writeJSON(w, 202, b)
}

func (s *Server) dbCrashes(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	s.crashes(w, r, u, id, s.projectOfDB(id))
}

func (s *Server) dbTerminal(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, s.projectOfDB(id), edit)
	if !ok {
		return
	}
	var spec runtime.DatabaseSpec
	s.P.Store.Read(func(d *store.Data) {
		db := d.Databases[id]
		spec = runtime.DatabaseSpec{Namespace: d.Namespace(db.ProjectID), Name: db.Name, ID: db.ID, Version: db.Version, DBName: db.DBName, User: db.User, Password: db.Password, Extensions: db.Extensions}
	})
	s.P.Audit(orgID, u, "database.terminal", spec.Name, nil)
	s.terminal(w, r, func(ctx context.Context, io runtime.Stdio) error {
		return s.P.Driver.DatabaseShell(ctx, spec, io)
	}, "")
}

// ---- jobs ----

type jobView struct {
	*store.Job
	ProjectName string        `json:"projectName"`
	LastRun     *store.JobRun `json:"lastRun,omitempty"`
	Describe    string        `json:"scheduleText"`
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request, u *store.User) {
	org, ok := s.checkOrg(w, u, r.PathValue("org"), store.RoleViewer)
	if !ok {
		return
	}
	out := []jobView{}
	s.P.Store.Read(func(d *store.Data) {
		for _, j := range d.Jobs {
			p := d.Projects[j.ProjectID]
			if p == nil || p.OrgID != org.ID || !canViewProject(d, u, p) {
				continue
			}
			v := jobView{Job: store.Clone(j), ProjectName: p.Name, Describe: describeCron(j.Schedule)}
			if runs := d.RunsOf(j.ID); len(runs) > 0 {
				v.LastRun = store.Clone(runs[0])
				for i := range v.LastRun.Steps {
					v.LastRun.Steps[i].Output = ""
				}
			}
			out = append(out, v)
		}
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	writeJSON(w, 200, out)
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request, u *store.User) {
	pid := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, pid, edit)
	if !ok {
		return
	}
	var in platform.JobInput
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	j, err := s.P.CreateJob(u, pid, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "job.created", j.Name, nil)
	writeJSON(w, 201, j)
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	if ok, _ := s.checkProject(w, u, s.projectOfJob(id), view); !ok {
		return
	}
	var out struct {
		jobView
		Runs    []*store.JobRun     `json:"runs"`
		Org     map[string]string   `json:"org"`
		CanEdit bool                `json:"canEdit"`
		Apps    []map[string]string `json:"apps"`
	}
	s.P.Store.Read(func(d *store.Data) {
		j := d.Jobs[id]
		p := d.Projects[j.ProjectID]
		o := d.Orgs[p.OrgID]
		out.jobView = jobView{Job: store.Clone(j), ProjectName: p.Name, Describe: describeCron(j.Schedule)}
		out.Org = map[string]string{"id": o.ID, "slug": o.Slug, "name": o.Name}
		out.CanEdit = canEditProject(d, u, p)
		out.Runs = []*store.JobRun{}
		for _, run := range d.RunsOf(id) {
			c := store.Clone(run)
			for i := range c.Steps {
				c.Steps[i].Output = ""
			}
			out.Runs = append(out.Runs, c)
		}
		for _, a := range d.AppsIn(p.ID) {
			out.Apps = append(out.Apps, map[string]string{"id": a.ID, "name": a.Name, "image": a.Image})
		}
	})
	writeJSON(w, 200, out)
}

func (s *Server) updateJob(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, s.projectOfJob(id), edit)
	if !ok {
		return
	}
	var in platform.JobInput
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	j, err := s.P.UpdateJob(id, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "job.updated", j.Name, nil)
	writeJSON(w, 200, j)
}

func (s *Server) deleteJob(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, s.projectOfJob(id), edit)
	if !ok {
		return
	}
	if err := s.P.DeleteJob(id); err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "job.deleted", id, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) runJob(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, s.projectOfJob(id), edit)
	if !ok {
		return
	}
	trigger := "manual"
	if r.Context().Value(tokenKey) != nil {
		trigger = "cli"
	}
	run, err := s.P.TriggerJob(u, id, trigger)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "job.run", id, map[string]string{"run": run.ID})
	writeJSON(w, 202, run)
}

func (s *Server) runOwner(id string) (jobID, projectID string) {
	s.P.Store.Read(func(d *store.Data) {
		if r := d.JobRuns[id]; r != nil {
			jobID = r.JobID
			if j := d.Jobs[r.JobID]; j != nil {
				projectID = j.ProjectID
			}
		}
	})
	return
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	_, pid := s.runOwner(id)
	if ok, _ := s.checkProject(w, u, pid, view); !ok {
		return
	}
	var out struct {
		*store.JobRun
		JobName string            `json:"jobName"`
		Org     map[string]string `json:"org"`
	}
	s.P.Store.Read(func(d *store.Data) {
		run := d.JobRuns[id]
		j := d.Jobs[run.JobID]
		o := d.Orgs[d.Projects[j.ProjectID].OrgID]
		out.JobRun = store.Clone(run)
		out.JobName = j.Name
		out.Org = map[string]string{"id": o.ID, "slug": o.Slug}
	})
	writeJSON(w, 200, out)
}

func (s *Server) cancelRun(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	_, pid := s.runOwner(id)
	if ok, _ := s.checkProject(w, u, pid, edit); !ok {
		return
	}
	s.P.CancelRun(id)
	s.P.Store.Write(func(d *store.Data) error {
		if run := d.JobRuns[id]; run != nil && run.Status == "queued" {
			run.Status = "cancelled"
			now := time.Now()
			run.FinishedAt = &now
		}
		return nil
	})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---- sandboxes (Greenhouse) ----

func (s *Server) sandboxAccess(w http.ResponseWriter, u *store.User, id string) (*store.Sandbox, bool) {
	var sb *store.Sandbox
	s.P.Store.Read(func(d *store.Data) {
		x := d.Sandboxes[id]
		if x == nil {
			return
		}
		if x.OwnerID == u.ID || store.RoleRank(orgRole(d, u, x.OrgID)) >= store.RoleRank(store.RoleAdmin) {
			sb = store.Clone(x)
		}
	})
	if sb == nil {
		writeErr(w, 404, "sandbox not found")
		return nil, false
	}
	return sb, true
}

type sandboxView struct {
	*store.Sandbox
	Owner Brief `json:"owner"`
}

// listSandboxes: owners and admins see every sandbox; others their own.
func (s *Server) listSandboxes(w http.ResponseWriter, r *http.Request, u *store.User) {
	org, ok := s.checkOrg(w, u, r.PathValue("org"), store.RoleViewer)
	if !ok {
		return
	}
	out := []sandboxView{}
	s.P.Store.Read(func(d *store.Data) {
		admin := store.RoleRank(orgRole(d, u, org.ID)) >= store.RoleRank(store.RoleAdmin)
		for _, sb := range d.Sandboxes {
			if sb.OrgID == org.ID && (admin || sb.OwnerID == u.ID) {
				c := store.Clone(sb)
				c.Conversations = nil
				out = append(out, sandboxView{c, brief(d.Users[sb.OwnerID])})
			}
		}
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	writeJSON(w, 200, out)
}

func (s *Server) createSandbox(w http.ResponseWriter, r *http.Request, u *store.User) {
	org, ok := s.checkOrg(w, u, r.PathValue("org"), store.RoleMember)
	if !ok {
		return
	}
	var in platform.SandboxInput
	readJSON(r, &in)
	sb, err := s.P.CreateSandbox(u, org.ID, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(org.ID, u, "sandbox.created", sb.Name, nil)
	writeJSON(w, 201, sb)
}

func (s *Server) getSandbox(w http.ResponseWriter, r *http.Request, u *store.User) {
	sb, ok := s.sandboxAccess(w, u, r.PathValue("id"))
	if !ok {
		return
	}
	var owner Brief
	var org map[string]string
	s.P.Store.Read(func(d *store.Data) {
		owner = brief(d.Users[sb.OwnerID])
		o := d.Orgs[sb.OrgID]
		org = map[string]string{"id": o.ID, "slug": o.Slug, "name": o.Name}
	})
	if sb.Conversations == nil {
		sb.Conversations = []store.Conversation{}
	}
	writeJSON(w, 200, map[string]any{"sandbox": sb, "owner": owner, "org": org})
}

func (s *Server) deleteSandbox(w http.ResponseWriter, r *http.Request, u *store.User) {
	sb, ok := s.sandboxAccess(w, u, r.PathValue("id"))
	if !ok {
		return
	}
	if err := s.P.DeleteSandbox(sb.ID); err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(sb.OrgID, u, "sandbox.deleted", sb.Name, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) restartSandbox(w http.ResponseWriter, r *http.Request, u *store.User) {
	sb, ok := s.sandboxAccess(w, u, r.PathValue("id"))
	if !ok {
		return
	}
	s.P.RestartSandbox(sb.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) sandboxFiles(w http.ResponseWriter, r *http.Request, u *store.User) {
	sb, ok := s.sandboxAccess(w, u, r.PathValue("id"))
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	out, err := s.P.SandboxRun(ctx, sb.ID, []string{"wackcluborchard-ls"}, "")
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	files := []string{}
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, l)
		}
	}
	writeJSON(w, 200, files)
}

func cleanPath(p string) (string, bool) {
	p = strings.TrimPrefix(strings.TrimSpace(p), "/")
	if p == "" || strings.Contains(p, "..") || strings.ContainsAny(p, "\x00\n") {
		return "", false
	}
	return p, true
}

func (s *Server) sandboxReadFile(w http.ResponseWriter, r *http.Request, u *store.User) {
	sb, ok := s.sandboxAccess(w, u, r.PathValue("id"))
	if !ok {
		return
	}
	p, ok := cleanPath(r.URL.Query().Get("path"))
	if !ok {
		writeErr(w, 400, "bad path")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	out, err := s.P.SandboxRun(ctx, sb.ID, []string{"wackcluborchard-read", p}, "")
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"path": p, "content": out})
}

func (s *Server) sandboxWriteFile(w http.ResponseWriter, r *http.Request, u *store.User) {
	sb, ok := s.sandboxAccess(w, u, r.PathValue("id"))
	if !ok {
		return
	}
	var in struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Delete  bool   `json:"delete"`
	}
	readJSON(r, &in)
	p, ok := cleanPath(in.Path)
	if !ok {
		writeErr(w, 400, "bad path")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	cmd := []string{"wackcluborchard-write", p}
	if in.Delete {
		cmd = []string{"wackcluborchard-rm", p}
	}
	if _, err := s.P.SandboxRun(ctx, sb.ID, cmd, in.Content); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// sandboxGit runs one of the supported git operations.
func (s *Server) sandboxGit(w http.ResponseWriter, r *http.Request, u *store.User) {
	sb, ok := s.sandboxAccess(w, u, r.PathValue("id"))
	if !ok {
		return
	}
	var in struct {
		Op      string   `json:"op"`
		Branch  string   `json:"branch"`
		Base    string   `json:"base"`
		Message string   `json:"message"`
		Files   []string `json:"files"`
		File    string   `json:"file"`
		Rebase  bool     `json:"rebase"`
		Force   bool     `json:"force"`
		Create  bool     `json:"create"`
		Title   string   `json:"title"`
	}
	readJSON(r, &in)
	var args []string
	switch in.Op {
	case "branches":
		args = []string{"branch", "-a", "-vv"}
	case "status":
		args = []string{"status", "--porcelain=v1", "-b"}
	case "diff":
		args = []string{"diff", "HEAD"}
		if in.File != "" {
			args = append(args, "--", in.File)
		}
	case "log":
		args = []string{"log", "--oneline", "-n", "30"}
	case "checkout":
		if in.Create {
			args = []string{"checkout", "-b", in.Branch}
			if in.Base != "" {
				args = append(args, in.Base)
			}
		} else {
			args = []string{"checkout", in.Branch}
		}
	case "commit":
		if strings.TrimSpace(in.Message) == "" {
			writeErr(w, 400, "write a commit message")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		add := append([]string{"git", "add"}, in.Files...)
		if len(in.Files) == 0 {
			add = []string{"git", "add", "-A"}
		}
		s.P.SandboxRun(ctx, sb.ID, add, "")
		cancel()
		args = []string{"commit", "-m", in.Message}
	case "uncommit":
		args = []string{"reset", "--soft", "HEAD~1"}
	case "fetch":
		args = []string{"fetch", "--all", "--prune"}
	case "pull":
		args = []string{"pull"}
		if in.Rebase {
			args = append(args, "--rebase")
		}
	case "push":
		args = []string{"push", "-u", "origin", "HEAD"}
		if in.Force {
			// refuses when the remote moved under us
			args = append(args, "--force-with-lease")
		}
	case "pr":
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		title := in.Title
		if title == "" {
			title = "Changes from the greenhouse"
		}
		out, err := s.P.SandboxRun(ctx, sb.ID, []string{"sh", "-c", "cd /workspace && gh pr create --fill --title \"$1\" 2>&1", "sh", title}, "")
		if err != nil {
			writeJSON(w, 200, map[string]string{"output": out, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"output": out})
		return
	default:
		writeErr(w, 400, "unknown git operation")
		return
	}
	for _, a := range args {
		if strings.HasPrefix(a, "--upload-pack") || strings.HasPrefix(a, "--exec") {
			writeErr(w, 400, "refused")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	out, err := s.P.SandboxRun(ctx, sb.ID, append([]string{"git"}, args...), "")
	res := map[string]string{"output": out}
	if err != nil {
		res["error"] = err.Error()
	}
	writeJSON(w, 200, res)
}

func (s *Server) sandboxShell(w http.ResponseWriter, r *http.Request, u *store.User) {
	sb, ok := s.sandboxAccess(w, u, r.PathValue("id"))
	if !ok {
		return
	}
	s.terminal(w, r, func(ctx context.Context, io runtime.Stdio) error {
		return s.P.SandboxShell(ctx, sb.ID, io)
	}, "")
}

func (s *Server) createConversation(w http.ResponseWriter, r *http.Request, u *store.User) {
	sb, ok := s.sandboxAccess(w, u, r.PathValue("id"))
	if !ok {
		return
	}
	var in struct {
		Cwd string `json:"cwd"`
	}
	readJSON(r, &in)
	c := store.Conversation{ID: store.NewID("conv"), Title: "New conversation", Cwd: in.Cwd, Messages: []store.Message{}, CreatedAt: time.Now()}
	s.P.Store.Write(func(d *store.Data) error {
		x := d.Sandboxes[sb.ID]
		x.Conversations = append([]store.Conversation{c}, x.Conversations...)
		return nil
	})
	writeJSON(w, 201, c)
}

func (s *Server) updateConversation(w http.ResponseWriter, r *http.Request, u *store.User) {
	sb, ok := s.sandboxAccess(w, u, r.PathValue("id"))
	if !ok {
		return
	}
	cid := r.PathValue("conv")
	if r.Method == http.MethodDelete {
		s.P.Store.Write(func(d *store.Data) error {
			x := d.Sandboxes[sb.ID]
			var keep []store.Conversation
			for _, c := range x.Conversations {
				if c.ID != cid {
					keep = append(keep, c)
				}
			}
			x.Conversations = keep
			return nil
		})
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	var in struct {
		Title string `json:"title"`
	}
	readJSON(r, &in)
	s.P.Store.Write(func(d *store.Data) error {
		x := d.Sandboxes[sb.ID]
		for i := range x.Conversations {
			if x.Conversations[i].ID == cid && strings.TrimSpace(in.Title) != "" {
				x.Conversations[i].Title = strings.TrimSpace(in.Title)
			}
		}
		return nil
	})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request, u *store.User) {
	sb, ok := s.sandboxAccess(w, u, r.PathValue("id"))
	if !ok {
		return
	}
	var in struct {
		Text string `json:"text"`
	}
	readJSON(r, &in)
	if strings.TrimSpace(in.Text) == "" {
		writeErr(w, 400, "say something")
		return
	}
	if sb.Status != "running" {
		writeErr(w, 409, "the sandbox is not running yet")
		return
	}
	if err := s.P.AgentMessage(sb.ID, r.PathValue("conv"), in.Text); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 202, map[string]bool{"ok": true})
}

func describeCron(expr string) string {
	return cron.Describe(expr)
}
