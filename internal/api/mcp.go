package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/platform"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

// The MCP endpoint speaks JSON-RPC 2.0 over streamable HTTP (single JSON
// responses). Agents authenticate with a personal API token and can do
// what that person can do in the dashboard.

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	run         func(ctx context.Context, u *store.User, a args) (any, error)
}

type args map[string]any

func (a args) str(k string) string {
	v, _ := a[k].(string)
	return strings.TrimSpace(v)
}

func (a args) num(k string, def int) int {
	if v, ok := a[k].(float64); ok {
		return int(v)
	}
	return def
}

func obj(props map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func sp(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func np(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }

func (s *Server) mcpTools() []mcpTool {
	return []mcpTool{
		{Name: "list_organizations", Description: "List the organizations you belong to.", InputSchema: obj(map[string]any{}),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				var out []map[string]string
				s.P.Store.Read(func(d *store.Data) {
					for _, o := range d.UserOrgs(u.ID) {
						out = append(out, map[string]string{"id": o.ID, "slug": o.Slug, "name": o.Name, "role": orgRole(d, u, o.ID)})
					}
				})
				return out, nil
			}},
		{Name: "list_projects", Description: "List projects with their apps and databases.", InputSchema: obj(map[string]any{"org": sp("organization slug or id; defaults to your first")}),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				var out []map[string]any
				s.P.Store.Read(func(d *store.Data) {
					org := s.resolveOrg(d, u, a.str("org"))
					for _, p := range d.Projects {
						if org != nil && p.OrgID != org.ID || !canViewProject(d, u, p) {
							continue
						}
						var apps, dbs []map[string]string
						for _, x := range d.AppsIn(p.ID) {
							apps = append(apps, map[string]string{"id": x.ID, "name": x.Name, "status": x.Status, "image": x.Image})
						}
						for _, x := range d.DatabasesIn(p.ID) {
							dbs = append(dbs, map[string]string{"id": x.ID, "name": x.Name, "status": x.Status})
						}
						out = append(out, map[string]any{"id": p.ID, "name": p.Name, "apps": apps, "databases": dbs})
					}
				})
				return out, nil
			}},
		{Name: "get_app", Description: "Status, pods, domains, resources and the latest deploy of an app.", InputSchema: obj(map[string]any{"app": sp("app id or name")}, "app"),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				id, err := s.resolveApp(u, a.str("app"), view)
				if err != nil {
					return nil, err
				}
				var out map[string]any
				s.P.Store.Read(func(d *store.Data) {
					x := d.Apps[id]
					out = map[string]any{"app": store.Clone(x)}
					if dep := d.Deploys[x.CurrentDeploy]; dep != nil {
						out["latestDeploy"] = store.Clone(dep)
					}
				})
				return out, nil
			}},
		{Name: "create_app", Description: "Create an app from a container image in a project and deploy it.",
			InputSchema: obj(map[string]any{"project": sp("project id or name"), "name": sp("app name"), "image": sp("container image"), "port": np("HTTP port the container listens on (0 for none)"), "replicas": np("replica count")}, "project", "name", "image"),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				pid, err := s.resolveProject(u, a.str("project"), edit)
				if err != nil {
					return nil, err
				}
				reps := a.num("replicas", 1)
				in := platform.AppInput{Name: a.str("name"), Source: store.Source{Type: "image", Image: a.str("image")}, Replicas: &reps, Deploy: true}
				if port := a.num("port", 8080); port > 0 {
					in.Ports = []store.Port{{Name: "http", Port: port, Protocol: "http"}}
				} else {
					in.Ports = []store.Port{}
				}
				app, err := s.P.CreateApp(u, pid, in)
				if err == nil {
					s.P.Audit(s.orgOfProject(pid), u, "app.created", app.Name, map[string]string{"via": "mcp"})
				}
				return app, err
			}},
		{Name: "deploy_app", Description: "Deploy an app: rebuild from its repo, or roll out its image (or a given image).", InputSchema: obj(map[string]any{"app": sp("app id or name"), "image": sp("optional image to deploy instead")}, "app"),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				id, err := s.resolveApp(u, a.str("app"), edit)
				if err != nil {
					return nil, err
				}
				return s.P.Deploy(u, id, platform.DeployOpts{Trigger: "mcp", Image: a.str("image")})
			}},
		{Name: "restart_app", Description: "Recreate an app's pods on the same image.", InputSchema: obj(map[string]any{"app": sp("app id or name")}, "app"),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				id, err := s.resolveApp(u, a.str("app"), edit)
				if err != nil {
					return nil, err
				}
				return map[string]bool{"ok": true}, s.P.Restart(u, id)
			}},
		{Name: "scale_app", Description: "Set an app's replica count.", InputSchema: obj(map[string]any{"app": sp("app id or name"), "replicas": np("replicas")}, "app", "replicas"),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				id, err := s.resolveApp(u, a.str("app"), edit)
				if err != nil {
					return nil, err
				}
				n := a.num("replicas", 1)
				return s.P.UpdateApp(u, id, platform.AppPatch{Replicas: &n})
			}},
		{Name: "list_deploys", Description: "An app's deploy history, newest first.", InputSchema: obj(map[string]any{"app": sp("app id or name")}, "app"),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				id, err := s.resolveApp(u, a.str("app"), view)
				if err != nil {
					return nil, err
				}
				var out []*store.Deploy
				s.P.Store.Read(func(d *store.Data) {
					for _, dep := range d.DeploysOf(id) {
						out = append(out, store.Clone(dep))
					}
				})
				return out, nil
			}},
		{Name: "rollback_app", Description: "Roll an app back to an earlier deploy's image (the previous successful one by default).", InputSchema: obj(map[string]any{"app": sp("app id or name"), "deploy": sp("deploy id to roll back to")}, "app"),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				id, err := s.resolveApp(u, a.str("app"), edit)
				if err != nil {
					return nil, err
				}
				target := a.str("deploy")
				if target == "" {
					s.P.Store.Read(func(d *store.Data) {
						cur := d.Apps[id].Image
						for _, dep := range d.DeploysOf(id) {
							if dep.Status == "succeeded" && dep.Image != "" && dep.Image != cur {
								target = dep.ID
								break
							}
						}
					})
				}
				if target == "" {
					return nil, platform.Invalid("no earlier successful deploy to roll back to")
				}
				return s.P.Rollback(u, id, target)
			}},
		{Name: "get_logs", Description: "Recent log lines from an app. previous=true reads the container that crashed.", InputSchema: obj(map[string]any{"app": sp("app id or name"), "lines": np("max lines, default 200"), "previous": map[string]any{"type": "boolean"}}, "app"),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				id, err := s.resolveApp(u, a.str("app"), view)
				if err != nil {
					return nil, err
				}
				var ns, name string
				s.P.Store.Read(func(d *store.Data) {
					x := d.Apps[id]
					ns, name = d.Namespace(x.ProjectID), x.Name
				})
				prev, _ := a["previous"].(bool)
				lctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
				defer cancel()
				var mu sync.Mutex
				var lines []string
				s.P.Driver.Logs(lctx, ns, name, "", prev, 0, func(pod, line string) {
					mu.Lock()
					lines = append(lines, line)
					mu.Unlock()
				})
				n := a.num("lines", 200)
				mu.Lock()
				defer mu.Unlock()
				if len(lines) > n {
					lines = lines[len(lines)-n:]
				}
				var crashes []*store.CrashReport
				s.P.Store.Read(func(d *store.Data) {
					for i := len(d.Crashes) - 1; i >= 0 && len(crashes) < 3; i-- {
						if d.Crashes[i].OwnerID == id {
							crashes = append(crashes, d.Crashes[i])
						}
					}
				})
				return map[string]any{"lines": strings.Join(lines, "\n"), "recentCrashes": crashes}, nil
			}},
		{Name: "set_variables", Description: "Set environment variables on a project (shared) or one app. Affected apps roll out.", InputSchema: obj(map[string]any{"project": sp("project id or name"), "app": sp("optional app to scope to"), "variables": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}, "secret": map[string]any{"type": "boolean"}}, "project", "variables"),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				pid, err := s.resolveProject(u, a.str("project"), edit)
				if err != nil {
					return nil, err
				}
				appID := ""
				if a.str("app") != "" {
					if appID, err = s.resolveApp(u, a.str("app"), edit); err != nil {
						return nil, err
					}
				}
				vars, _ := a["variables"].(map[string]any)
				secret, _ := a["secret"].(bool)
				n := 0
				s.P.Store.Write(func(d *store.Data) error {
					for k, v := range vars {
						if !validKey(k) {
							continue
						}
						val := fmt.Sprint(v)
						found := false
						for _, x := range d.Variables {
							if x.ProjectID == pid && x.AppID == appID && x.EnvID == "" && x.Key == k {
								x.Value, x.Secret, x.UpdatedAt = val, secret, time.Now()
								found = true
							}
						}
						if !found {
							id := store.NewID("var")
							d.Variables[id] = &store.Variable{ID: id, ProjectID: pid, AppID: appID, Key: k, Value: val, Secret: secret, UpdatedAt: time.Now()}
						}
						n++
					}
					return nil
				})
				var apps []string
				s.P.Store.Read(func(d *store.Data) {
					for _, x := range d.AppsIn(pid) {
						if appID == "" || x.ID == appID {
							apps = append(apps, x.ID)
						}
					}
				})
				for _, id := range apps {
					s.P.EnqueueApp(id)
				}
				return map[string]int{"set": n, "appsUpdated": len(apps)}, nil
			}},
		{Name: "create_database", Description: "Create a managed PostgreSQL database in a project.", InputSchema: obj(map[string]any{"project": sp("project id or name"), "name": sp("database name"), "version": np("PostgreSQL major version, default 17")}, "project", "name"),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				pid, err := s.resolveProject(u, a.str("project"), edit)
				if err != nil {
					return nil, err
				}
				db, err := s.P.CreateDatabase(u, pid, platform.DatabaseInput{Name: a.str("name"), Version: a.num("version", 17)})
				if err != nil {
					return nil, err
				}
				return map[string]any{"id": db.ID, "name": db.Name, "status": db.Status, "reference": "${{ " + db.Name + ".DATABASE_URL }}"}, nil
			}},
		{Name: "query_database", Description: "Run SQL against a managed database and return rows.", InputSchema: obj(map[string]any{"database": sp("database id or name"), "sql": sp("SQL to run")}, "database", "sql"),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				id, err := s.resolveDB(u, a.str("database"))
				if err != nil {
					return nil, err
				}
				qctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				return s.P.Query(qctx, id, a.str("sql"))
			}},
		{Name: "list_jobs", Description: "List jobs and their last run.", InputSchema: obj(map[string]any{}),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				var out []map[string]any
				s.P.Store.Read(func(d *store.Data) {
					for _, j := range d.Jobs {
						if !canViewProject(d, u, d.Projects[j.ProjectID]) {
							continue
						}
						row := map[string]any{"id": j.ID, "name": j.Name, "schedule": j.Schedule}
						if runs := d.RunsOf(j.ID); len(runs) > 0 {
							row["lastRun"] = map[string]any{"id": runs[0].ID, "number": runs[0].Number, "status": runs[0].Status}
						}
						out = append(out, row)
					}
				})
				return out, nil
			}},
		{Name: "run_job", Description: "Trigger a job run now.", InputSchema: obj(map[string]any{"job": sp("job id or name")}, "job"),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				id, err := s.resolveJob(u, a.str("job"))
				if err != nil {
					return nil, err
				}
				return s.P.TriggerJob(u, id, "mcp")
			}},
		{Name: "get_job_run", Description: "A job run with per-step status, exit codes and output.", InputSchema: obj(map[string]any{"run": sp("run id")}, "run"),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				var out *store.JobRun
				ok := false
				s.P.Store.Read(func(d *store.Data) {
					if r := d.JobRuns[a.str("run")]; r != nil {
						if j := d.Jobs[r.JobID]; j != nil && canViewProject(d, u, d.Projects[j.ProjectID]) {
							out, ok = store.Clone(r), true
						}
					}
				})
				if !ok {
					return nil, platform.Invalid("run not found")
				}
				return out, nil
			}},
		{Name: "list_templates", Description: "Templates available in The Grove.", InputSchema: obj(map[string]any{}),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				var out []map[string]string
				for _, t := range platform.Templates {
					out = append(out, map[string]string{"id": t.ID, "name": t.Name, "description": t.Description})
				}
				return out, nil
			}},
		{Name: "deploy_template", Description: "Deploy a Grove template into a project.", InputSchema: obj(map[string]any{"project": sp("project id or name"), "template": sp("template id"), "name": sp("instance name")}, "project", "template"),
			run: func(ctx context.Context, u *store.User, a args) (any, error) {
				pid, err := s.resolveProject(u, a.str("project"), edit)
				if err != nil {
					return nil, err
				}
				var env string
				s.P.Store.Read(func(d *store.Data) { env = d.Projects[pid].Environments[0].ID })
				return s.P.InstantiateTemplate(u, pid, env, a.str("template"), a.str("name"))
			}},
	}
}

func (s *Server) resolveOrg(d *store.Data, u *store.User, ref string) *store.Org {
	if ref == "" {
		return nil
	}
	o := d.Orgs[ref]
	if o == nil {
		o = d.OrgBySlug(ref)
	}
	if o != nil && orgRole(d, u, o.ID) == "" {
		return nil
	}
	return o
}

func (s *Server) orgOfProject(pid string) (org string) {
	s.P.Store.Read(func(d *store.Data) {
		if p := d.Projects[pid]; p != nil {
			org = p.OrgID
		}
	})
	return
}

func (s *Server) resolveProject(u *store.User, ref string, lvl accessLevel) (string, error) {
	var id string
	var matches int
	s.P.Store.Read(func(d *store.Data) {
		for _, p := range d.Projects {
			if (p.ID == ref || strings.EqualFold(p.Name, ref) || p.Slug == ref) && canViewProject(d, u, p) && (lvl == view || canEditProject(d, u, p)) {
				id = p.ID
				matches++
			}
		}
	})
	if matches == 0 {
		return "", platform.Invalid("project %q not found", ref)
	}
	if matches > 1 {
		return "", platform.Invalid("%q matches several projects; use its id", ref)
	}
	return id, nil
}

func (s *Server) resolveApp(u *store.User, ref string, lvl accessLevel) (string, error) {
	var id string
	var matches int
	s.P.Store.Read(func(d *store.Data) {
		for _, a := range d.Apps {
			if a.ID == ref || a.Name == ref {
				p := d.Projects[a.ProjectID]
				if canViewProject(d, u, p) && (lvl == view || canEditProject(d, u, p)) {
					id = a.ID
					matches++
				}
			}
		}
	})
	if matches == 0 {
		return "", platform.Invalid("app %q not found", ref)
	}
	if matches > 1 {
		return "", platform.Invalid("%q matches apps in several projects; use its id", ref)
	}
	return id, nil
}

func (s *Server) resolveDB(u *store.User, ref string) (string, error) {
	var id string
	s.P.Store.Read(func(d *store.Data) {
		for _, x := range d.Databases {
			if (x.ID == ref || x.Name == ref) && canEditProject(d, u, d.Projects[x.ProjectID]) {
				id = x.ID
			}
		}
	})
	if id == "" {
		return "", platform.Invalid("database %q not found", ref)
	}
	return id, nil
}

func (s *Server) resolveJob(u *store.User, ref string) (string, error) {
	var id string
	s.P.Store.Read(func(d *store.Data) {
		for _, x := range d.Jobs {
			if (x.ID == ref || x.Name == ref) && canEditProject(d, u, d.Projects[x.ProjectID]) {
				id = x.ID
			}
		}
	})
	if id == "" {
		return "", platform.Invalid("job %q not found", ref)
	}
	return id, nil
}

func (s *Server) mcp(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// no server-initiated stream
		w.Header().Set("Allow", "POST")
		writeErr(w, http.StatusMethodNotAllowed, "POST JSON-RPC to this endpoint")
		return
	case http.MethodDelete:
		w.WriteHeader(204)
		return
	}
	u := userOf(r)
	if u == nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="orchard", error="invalid_token"`)
		writeErr(w, 401, "authenticate with a personal API token: Authorization: Bearer orch_…")
		return
	}
	var enabled bool
	s.P.Store.Read(func(d *store.Data) { enabled = d.Settings.MCPEnabled })
	if !enabled {
		writeErr(w, 403, "the MCP endpoint is turned off on this instance")
		return
	}
	var raw json.RawMessage
	if err := readJSON(r, &raw); err != nil {
		s.fail(w, err)
		return
	}
	// batches are arrays
	if len(raw) > 0 && raw[0] == '[' {
		var reqs []rpcReq
		json.Unmarshal(raw, &reqs)
		var out []any
		for _, rq := range reqs {
			if res := s.mcpHandle(r.Context(), u, rq); res != nil {
				out = append(out, res)
			}
		}
		if len(out) == 0 {
			w.WriteHeader(202)
			return
		}
		writeJSON(w, 200, out)
		return
	}
	var rq rpcReq
	if err := json.Unmarshal(raw, &rq); err != nil {
		writeJSON(w, 200, map[string]any{"jsonrpc": "2.0", "id": nil, "error": map[string]any{"code": -32700, "message": "parse error"}})
		return
	}
	res := s.mcpHandle(r.Context(), u, rq)
	if res == nil {
		w.WriteHeader(202)
		return
	}
	w.Header().Set("Mcp-Session-Id", "orchard")
	writeJSON(w, 200, res)
}

func (s *Server) mcpHandle(ctx context.Context, u *store.User, rq rpcReq) any {
	if len(rq.ID) == 0 {
		return nil // notification
	}
	reply := func(result any) any {
		return map[string]any{"jsonrpc": "2.0", "id": rq.ID, "result": result}
	}
	fail := func(code int, msg string) any {
		return map[string]any{"jsonrpc": "2.0", "id": rq.ID, "error": map[string]any{"code": code, "message": msg}}
	}
	switch rq.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(rq.Params, &p)
		ver := p.ProtocolVersion
		if ver == "" {
			ver = "2025-06-18"
		}
		return reply(map[string]any{
			"protocolVersion": ver,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]string{"name": "wack-club-orchard", "version": s.P.Cfg.Version},
			"instructions":    "Wack Club Orchard is a Kubernetes platform. Projects hold apps (deployments), managed PostgreSQL databases and jobs. Refer to things by id or unique name. Deploys and builds are asynchronous: check get_app or list_deploys for progress. Variables may reference databases with ${{ dbname.DATABASE_URL }}.",
		})
	case "ping":
		return reply(map[string]any{})
	case "tools/list":
		var tools []mcpTool
		tools = s.mcpTools()
		return reply(map[string]any{"tools": tools})
	case "tools/call":
		var p struct {
			Name      string `json:"name"`
			Arguments args   `json:"arguments"`
		}
		json.Unmarshal(rq.Params, &p)
		for _, t := range s.mcpTools() {
			if t.Name != p.Name {
				continue
			}
			if p.Arguments == nil {
				p.Arguments = args{}
			}
			res, err := t.run(ctx, u, p.Arguments)
			if err != nil {
				return reply(map[string]any{"isError": true, "content": []map[string]string{{"type": "text", "text": err.Error()}}})
			}
			b, _ := json.MarshalIndent(res, "", "  ")
			return reply(map[string]any{"content": []map[string]string{{"type": "text", "text": string(b)}}, "structuredContent": map[string]any{"result": res}})
		}
		return fail(-32602, "unknown tool "+p.Name)
	case "resources/list":
		return reply(map[string]any{"resources": []any{}})
	case "prompts/list":
		return reply(map[string]any{"prompts": []any{}})
	}
	return fail(-32601, "method not found: "+rq.Method)
}
