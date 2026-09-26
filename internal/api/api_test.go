package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/platform"
	"github.com/evanxdsouza/wackcluborchard/internal/runtime"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

type harness struct {
	t   *testing.T
	srv *httptest.Server
	st  *store.Store
	p   *platform.Platform
}

func newHarness(t *testing.T) *harness {
	t.Setenv("ORCHARD_QUIET", "1")
	st, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	st.Write(func(d *store.Data) error {
		d.Settings.SignupMode = "open"
		d.Settings.AppDomain = "apps.test"
		d.Settings.MCPEnabled = true
		d.Settings.BuildSlots = 1
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	p := platform.New(ctx, st, runtime.NewSim(), platform.Config{Version: "test"})
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	s := New(p, Config{})
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return &harness{t: t, srv: srv, st: st, p: p}
}

type user struct {
	h     *harness
	c     *http.Client
	token string
}

func (h *harness) signup(name string) *user {
	jar, _ := cookiejar.New(nil)
	u := &user{h: h, c: &http.Client{Jar: jar}}
	u.call("POST", "/api/auth/signup", map[string]string{"username": name, "name": name, "password": "correcthorse1"}, nil, 201)
	return u
}

func (u *user) call(method, path string, body any, out any, want int) {
	u.h.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, u.h.srv.URL+path, r)
	req.Header.Set("Content-Type", "application/json")
	if u.token != "" {
		req.Header.Set("Authorization", "Bearer "+u.token)
	}
	resp, err := u.c.Do(req)
	if err != nil {
		u.h.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if want != 0 && resp.StatusCode != want {
		u.h.t.Fatalf("%s %s: status %d, want %d: %s", method, path, resp.StatusCode, want, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			u.h.t.Fatalf("%s %s: decode: %v: %s", method, path, err, data)
		}
	}
}

func eventually(t *testing.T, what string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestDeployFlowAndIsolation(t *testing.T) {
	h := newHarness(t)
	alice := h.signup("alice")
	bob := h.signup("bob")

	var prj struct{ ID string }
	alice.call("POST", "/api/orgs/alice/projects", map[string]string{"name": "Homelab"}, &prj, 201)

	// bob cannot see alice's project, even by id
	bob.call("GET", "/api/projects/"+prj.ID, nil, nil, 404)
	bob.call("POST", "/api/orgs/alice/projects", map[string]string{"name": "x"}, nil, 404)

	var db struct{ ID, Name string }
	alice.call("POST", "/api/projects/"+prj.ID+"/databases", map[string]any{"name": "main"}, &db, 201)

	var app struct {
		ID      string
		Domains []struct{ Host string }
	}
	alice.call("POST", "/api/projects/"+prj.ID+"/apps", map[string]any{
		"name":   "web",
		"source": map[string]string{"type": "image", "image": "nginx:alpine"},
		"ports":  []map[string]any{{"name": "http", "port": 80, "protocol": "http"}},
		"env":    map[string]string{"DATABASE_URL": "${{ main.DATABASE_URL }}"},
		"deploy": true,
	}, &app, 201)
	if len(app.Domains) != 1 || app.Domains[0].Host != "web.apps.test" {
		t.Fatalf("generated domain: %+v", app.Domains)
	}
	eventually(t, "app running", func() bool {
		var a struct{ Status string }
		alice.call("GET", "/api/apps/"+app.ID, nil, &a, 200)
		return a.Status == "running"
	})

	// the database reference resolves to a real URI
	h.st.Read(func(d *store.Data) {
		env, _ := platform.ResolveVars(d, d.Apps[app.ID])
		if !strings.HasPrefix(env["DATABASE_URL"], "postgresql://u_orchard_alice_homelab_main:") || !strings.Contains(env["DATABASE_URL"], "pg-main-rw.orchard-alice-homelab.svc.cluster.local:5432") {
			t.Fatalf("DATABASE_URL = %q", env["DATABASE_URL"])
		}
		if env["PORT"] != "80" {
			t.Fatalf("PORT = %q", env["PORT"])
		}
	})

	// scale, then roll back to the first deploy after a second one
	alice.call("PATCH", "/api/apps/"+app.ID, map[string]any{"replicas": 2}, nil, 200)
	var res struct {
		Deploy struct{ ID string }
	}
	alice.call("POST", "/api/apps/"+app.ID+"/deploy", map[string]string{"image": "nginx:1.27"}, &res, 202)
	eventually(t, "second deploy", func() bool {
		var dep struct{ Status string }
		alice.call("GET", "/api/deploys/"+res.Deploy.ID, nil, &dep, 200)
		return dep.Status == "succeeded"
	})
	var list struct {
		Deploys []struct {
			ID     string
			Number int
			Image  string
		}
		CurrentImage string
	}
	alice.call("GET", "/api/apps/"+app.ID+"/deploys", nil, &list, 200)
	if list.CurrentImage != "nginx:1.27" || len(list.Deploys) != 2 {
		t.Fatalf("deploys: %+v", list)
	}
	alice.call("POST", "/api/apps/"+app.ID+"/rollback/"+list.Deploys[1].ID, nil, &res, 202)
	eventually(t, "rollback", func() bool {
		var a struct{ Image, Status string }
		alice.call("GET", "/api/apps/"+app.ID, nil, &a, 200)
		return a.Image == "nginx:alpine" && a.Status == "running"
	})

	// bob can't touch the app either
	bob.call("POST", "/api/apps/"+app.ID+"/restart", nil, nil, 404)
	bob.call("DELETE", "/api/apps/"+app.ID, nil, nil, 404)

	// viewers can read but not write
	alice.call("POST", "/api/orgs/alice/members", map[string]string{"username": "bob", "role": "viewer"}, nil, 201)
	alice.call("POST", "/api/projects/"+prj.ID+"/members", map[string]string{"username": "bob"}, nil, 200)
	bob.call("GET", "/api/apps/"+app.ID, nil, nil, 200)
	bob.call("POST", "/api/apps/"+app.ID+"/restart", nil, nil, 404)
	var dbView struct{ Password string }
	bob.call("GET", "/api/databases/"+db.ID, nil, &dbView, 200)
	if dbView.Password != "" {
		t.Fatal("viewer saw the database password")
	}
}

func TestQuotaAndClaim(t *testing.T) {
	h := newHarness(t)
	alice := h.signup("alice")
	h.st.Write(func(d *store.Data) error {
		for _, o := range d.Orgs {
			o.Quota = store.Quota{Apps: 1}
		}
		return nil
	})
	var prj struct{ ID string }
	alice.call("POST", "/api/orgs/alice/projects", map[string]string{"name": "P"}, &prj, 201)
	mk := func(name string, want int) {
		alice.call("POST", "/api/projects/"+prj.ID+"/apps", map[string]any{"name": name, "source": map[string]string{"type": "image", "image": "nginx"}}, nil, want)
	}
	mk("one", 201)
	mk("two", 422)

	// claiming: a wrong token fails, the minted one works once
	alice.call("POST", "/api/auth/claim", map[string]string{"token": "nope"}, nil, 400)
	tok, minted := MintSetupToken(h.st)
	if !minted {
		t.Fatal("no setup token minted on an unclaimed instance")
	}
	alice.call("POST", "/api/auth/claim", map[string]string{"token": tok}, nil, 200)
	alice.call("POST", "/api/auth/claim", map[string]string{"token": tok}, nil, 400)
	alice.call("GET", "/api/admin/settings", nil, nil, 200)
	if _, minted := MintSetupToken(h.st); minted {
		t.Fatal("setup token minted although a superadmin exists")
	}
}

func TestMCP(t *testing.T) {
	h := newHarness(t)
	alice := h.signup("alice")
	var tok struct{ Token string }
	alice.call("POST", "/api/me/tokens", map[string]string{"name": "agent"}, &tok, 201)
	agent := &user{h: h, c: http.DefaultClient, token: tok.Token}
	var prj struct{ ID string }
	agent.call("POST", "/api/orgs/alice/projects", map[string]string{"name": "Agents"}, &prj, 201)

	var list struct {
		Result struct {
			Tools []struct{ Name string }
		}
	}
	agent.call("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}, &list, 200)
	if len(list.Result.Tools) < 10 {
		t.Fatalf("tools: %+v", list)
	}
	var call struct {
		Result struct {
			IsError bool
			Content []struct{ Text string }
		}
	}
	agent.call("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{
		"name": "create_app", "arguments": map[string]any{"project": "Agents", "name": "hello", "image": "traefik/whoami", "port": 80},
	}}, &call, 200)
	if call.Result.IsError || !strings.Contains(call.Result.Content[0].Text, `"name": "hello"`) {
		t.Fatalf("create_app: %+v", call)
	}
	// unauthenticated requests are refused
	anon := &user{h: h, c: http.DefaultClient}
	anon.call("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}, nil, 401)
}

func TestComposeAndJobs(t *testing.T) {
	h := newHarness(t)
	alice := h.signup("alice")
	var prj struct {
		ID           string
		Environments []struct{ ID string }
	}
	alice.call("POST", "/api/orgs/alice/projects", map[string]string{"name": "Stack"}, &prj, 201)
	var plan struct{ Created, Removed []string }
	src := "services:\n  web:\n    image: nginx:alpine\n    ports: [\"80\"]\n  worker:\n    image: redis:7\n    volumes:\n      - data:/data\n"
	alice.call("PUT", "/api/projects/"+prj.ID+"/compose", map[string]any{"source": src}, &plan, 200)
	if len(plan.Created) != 2 {
		t.Fatalf("compose created %v", plan.Created)
	}
	alice.call("PUT", "/api/projects/"+prj.ID+"/compose", map[string]any{"source": "services:\n  web:\n    image: nginx:alpine\n"}, &plan, 200)
	if len(plan.Removed) != 1 || plan.Removed[0] != "worker" {
		t.Fatalf("compose removed %v", plan.Removed)
	}

	var job struct{ ID string }
	alice.call("POST", "/api/projects/"+prj.ID+"/jobs", map[string]any{
		"name": "hello", "concurrency": "skip",
		"steps": []map[string]any{
			{"name": "one", "type": "script", "lang": "bash", "source": "echo first"},
			{"name": "two", "type": "script", "lang": "bash", "source": "echo boom\nexit 1"},
			{"name": "three", "type": "script", "lang": "bash", "source": "echo never"},
		},
	}, &job, 201)
	var run struct{ ID string }
	alice.call("POST", "/api/jobs/"+job.ID+"/runs", nil, &run, 202)
	var got struct {
		Status string
		Steps  []struct {
			Status   string
			ExitCode *int
			Output   string
		}
	}
	eventually(t, "run finishes", func() bool {
		alice.call("GET", "/api/runs/"+run.ID, nil, &got, 200)
		return got.Status == "failed" || got.Status == "succeeded"
	})
	if got.Status != "failed" || got.Steps[0].Status != "succeeded" || got.Steps[1].Status != "failed" || got.Steps[2].Status != "skipped" {
		t.Fatalf("run: %+v", got)
	}
	if got.Steps[1].ExitCode == nil || *got.Steps[1].ExitCode != 1 || !strings.Contains(got.Steps[0].Output, "first") {
		t.Fatalf("step detail: %+v", got.Steps)
	}
}
