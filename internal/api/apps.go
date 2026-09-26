package api

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/github"
	"github.com/evanxdsouza/wackcluborchard/internal/platform"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

func (s *Server) createApp(w http.ResponseWriter, r *http.Request, u *store.User) {
	pid := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, pid, edit)
	if !ok {
		return
	}
	var in platform.AppInput
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	a, err := s.P.CreateApp(u, pid, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "app.created", a.Name, map[string]string{"source": a.Source.Type})
	writeJSON(w, 201, a)
}

type appDetail struct {
	*store.App
	Project   map[string]string  `json:"project"`
	Org       map[string]string  `json:"org"`
	EnvName   string             `json:"envName"`
	Creator   Brief              `json:"creator"`
	CanEdit   bool               `json:"canEdit"`
	Namespace string             `json:"namespace"`
	Latest    *store.Deploy      `json:"latestDeploy,omitempty"`
	Alerts    []*store.KubeEvent `json:"alerts"`
	Internal  string             `json:"internalHost"`
	PublicIP  string             `json:"publicIp,omitempty"`
}

func (s *Server) getApp(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	if ok, _ := s.checkProject(w, u, s.projectOfApp(id), view); !ok {
		return
	}
	var out appDetail
	s.P.Store.Read(func(d *store.Data) {
		a := d.Apps[id]
		p := d.Projects[a.ProjectID]
		o := d.Orgs[p.OrgID]
		out.App = store.Clone(a)
		out.Project = map[string]string{"id": p.ID, "name": p.Name, "icon": p.Icon, "background": p.Background}
		out.Org = map[string]string{"id": o.ID, "slug": o.Slug, "name": o.Name}
		for _, e := range p.Environments {
			if e.ID == a.EnvID {
				out.EnvName = e.Name
			}
		}
		out.Creator = brief(d.Users[a.CreatedBy])
		out.CanEdit = canEditProject(d, u, p)
		out.Namespace = d.Namespace(p.ID)
		out.Internal = a.Name + "." + out.Namespace + ".svc.cluster.local"
		out.PublicIP = o.PublicIP
		if out.PublicIP == "" {
			out.PublicIP = d.Settings.PublicIP
		}
		if a.CurrentDeploy != "" {
			if dep := d.Deploys[a.CurrentDeploy]; dep != nil {
				out.Latest = store.Clone(dep)
			}
		}
		// the banner shows only what is wrong now, not the hour of history
		out.Alerts = []*store.KubeEvent{}
		if a.Status != "running" && a.Status != "stopped" {
			for _, e := range d.Events {
				if e.OwnerID == id && time.Since(e.LastSeen) < 2*time.Minute {
					out.Alerts = append(out.Alerts, store.Clone(e))
				}
			}
		}
	})
	writeJSON(w, 200, out)
}

func (s *Server) updateApp(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, s.projectOfApp(id), edit)
	if !ok {
		return
	}
	var in platform.AppPatch
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	a, err := s.P.UpdateApp(u, id, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	meta := map[string]string{}
	if in.Replicas != nil {
		meta["replicas"] = strconv.Itoa(*in.Replicas)
	}
	s.P.Audit(orgID, u, "app.updated", a.Name, meta)
	writeJSON(w, 200, a)
}

func (s *Server) deleteApp(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, s.projectOfApp(id), edit)
	if !ok {
		return
	}
	var name string
	s.P.Store.Read(func(d *store.Data) { name = d.Apps[id].Name })
	if err := s.P.DeleteApp(u, id); err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "app.deleted", name, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) deployApp(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, s.projectOfApp(id), edit)
	if !ok {
		return
	}
	var in struct {
		Image  string `json:"image"`
		Commit string `json:"commit"`
	}
	readJSON(r, &in)
	opts := platform.DeployOpts{Trigger: "manual", Image: in.Image, Commit: in.Commit}
	if r.Context().Value(tokenKey) != nil {
		opts.Trigger = "api"
	}
	// resolve the branch head so the deploy records what it built
	var src store.Source
	var token string
	s.P.Store.Read(func(d *store.Data) {
		src = d.Apps[id].Source
		token = d.Users[u.ID].GitHubToken
	})
	if src.Type == "github" && in.Image == "" && in.Commit == "" && token != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		if c, err := github.LatestCommit(ctx, token, src.Repo, src.Branch); err == nil {
			opts.Commit, opts.Message = c.SHA, c.Message
		}
		cancel()
	}
	res, err := s.P.Deploy(u, id, opts)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "app.deployed", id, map[string]string{"deploy": res.Deploy.ID})
	writeJSON(w, 202, res)
}

func (s *Server) restartApp(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, s.projectOfApp(id), edit)
	if !ok {
		return
	}
	if err := s.P.Restart(u, id); err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "app.restarted", id, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) rollbackApp(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, s.projectOfApp(id), edit)
	if !ok {
		return
	}
	res, err := s.P.Rollback(u, id, r.PathValue("dep"))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "app.rolled_back", id, map[string]string{"to": r.PathValue("dep")})
	writeJSON(w, 202, res)
}

func (s *Server) listDeploys(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	if ok, _ := s.checkProject(w, u, s.projectOfApp(id), view); !ok {
		return
	}
	out := []*store.Deploy{}
	var current string
	s.P.Store.Read(func(d *store.Data) {
		current = d.Apps[id].Image
		for _, dep := range d.DeploysOf(id) {
			out = append(out, store.Clone(dep))
		}
	})
	writeJSON(w, 200, map[string]any{"deploys": out, "currentImage": current})
}

func (s *Server) getDeploy(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	var dep *store.Deploy
	var appID string
	s.P.Store.Read(func(d *store.Data) {
		if x := d.Deploys[id]; x != nil {
			dep = store.Clone(x)
			appID = x.AppID
		}
	})
	if ok, _ := s.checkProject(w, u, s.projectOfApp(appID), view); !ok {
		return
	}
	writeJSON(w, 200, dep)
}

func (s *Server) appMetrics(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	if ok, _ := s.checkProject(w, u, s.projectOfApp(id), view); !ok {
		return
	}
	pts := s.P.Metrics(id)
	if pts == nil {
		pts = []platform.MetricPoint{}
	}
	writeJSON(w, 200, pts)
}

func (s *Server) appEvents(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	if ok, _ := s.checkProject(w, u, s.projectOfApp(id), view); !ok {
		return
	}
	out := []*store.KubeEvent{}
	s.P.Store.Read(func(d *store.Data) {
		for _, e := range d.Events {
			if e.OwnerID == id {
				out = append(out, store.Clone(e))
			}
		}
	})
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
	writeJSON(w, 200, out)
}

func (s *Server) crashes(w http.ResponseWriter, r *http.Request, u *store.User, ownerID, projectID string) {
	if ok, _ := s.checkProject(w, u, projectID, view); !ok {
		return
	}
	out := []*store.CrashReport{}
	s.P.Store.Read(func(d *store.Data) {
		for i := len(d.Crashes) - 1; i >= 0 && len(out) < 50; i-- {
			if d.Crashes[i].OwnerID == ownerID {
				out = append(out, d.Crashes[i])
			}
		}
	})
	writeJSON(w, 200, out)
}

func (s *Server) appCrashes(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	s.crashes(w, r, u, id, s.projectOfApp(id))
}

func (s *Server) addDomain(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, s.projectOfApp(id), edit)
	if !ok {
		return
	}
	var in struct {
		Host string `json:"host"`
	}
	readJSON(r, &in)
	a, err := s.P.AddDomain(id, in.Host)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "domain.added", in.Host, map[string]string{"app": a.Name})
	var cname string
	s.P.Store.Read(func(d *store.Data) {
		cname = d.Settings.IngressCNAME
		if cname == "" {
			cname = d.Settings.Domain
		}
	})
	writeJSON(w, 201, map[string]any{"app": a, "cname": cname})
}

func (s *Server) removeDomain(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, s.projectOfApp(id), edit)
	if !ok {
		return
	}
	host := r.PathValue("host")
	if err := s.P.RemoveDomain(id, host); err != nil {
		s.fail(w, err)
		return
	}
	s.P.Audit(orgID, u, "domain.removed", host, nil)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
