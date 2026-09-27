// Command wackcluborchard is the Wack Club Orchard CLI: deploy, scale, tail logs,
// query databases and run jobs from a terminal.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

var version = "dev"

type config struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	Org   string `json:"org,omitempty"`
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "wackcluborchard", "config.json")
}

func loadConfig() config {
	var c config
	if b, err := os.ReadFile(configPath()); err == nil {
		json.Unmarshal(b, &c)
	}
	if v := os.Getenv("WACKCLUBORCHARD_URL"); v != "" {
		c.URL = v
	}
	if v := os.Getenv("WACKCLUBORCHARD_TOKEN"); v != "" {
		c.Token = v
	}
	return c
}

func saveConfig(c config) error {
	p := configPath()
	os.MkdirAll(filepath.Dir(p), 0o700)
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(p, b, 0o600)
}

type client struct {
	cfg config
	h   *http.Client
}

func (c *client) do(method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = strings.NewReader(string(b))
	}
	req, err := http.NewRequest(method, strings.TrimRight(c.cfg.URL, "/")+"/api"+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.h.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return errors.New(e.Error)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

func (c *client) stream(path string, fn func(event string, data []byte) bool) error {
	req, _ := http.NewRequest("GET", strings.TrimRight(c.cfg.URL, "/")+"/api"+path, nil)
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s", resp.Status)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	event := ""
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			if !fn(event, []byte(strings.TrimPrefix(line, "data: "))) {
				return nil
			}
		case line == "":
			event = ""
		}
	}
	return sc.Err()
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "wackcluborchard: "+format+"\n", a...)
	os.Exit(1)
}

func must(err error) {
	if err != nil {
		die("%v", err)
	}
}

const usage = `Wack Club Orchard CLI

Usage:
  wackcluborchard login <url>              sign in and save a token
  wackcluborchard whoami                   show the signed-in account
  wackcluborchard orgs                     list organizations
  wackcluborchard apps                     list apps and databases
  wackcluborchard status <app>             pods, deploy and URLs for an app
  wackcluborchard deploy <app> [--image]   build or roll out an app
  wackcluborchard logs <app> [-f] [--previous] [--pod p] [--since 15m]
  wackcluborchard restart <app>            recreate pods on the same image
  wackcluborchard scale <app> <n>          set replicas
  wackcluborchard rollback <app> [#n]      roll back to an earlier deploy
  wackcluborchard open <app>               open the app's URL
  wackcluborchard env <project> [K=V ...] [--app a] [--secret]
  wackcluborchard db query <db> "<sql>"    run SQL
  wackcluborchard jobs                     list jobs
  wackcluborchard run <job> [--no-wait]    run a job and stream its output
  wackcluborchard mcp                      print MCP configuration for agents
  wackcluborchard version

Set WACKCLUBORCHARD_URL and WACKCLUBORCHARD_TOKEN to skip login (CI).
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		return
	}
	cmd, args := os.Args[1], os.Args[2:]
	if cmd == "version" || cmd == "--version" {
		fmt.Println("wackcluborchard", version)
		return
	}
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		fmt.Print(usage)
		return
	}
	if cmd == "login" {
		login(args)
		return
	}
	cfg := loadConfig()
	if cfg.URL == "" || cfg.Token == "" {
		die("not signed in; run `wackcluborchard login <url>`")
	}
	c := &client{cfg: cfg, h: &http.Client{Timeout: 60 * time.Second}}
	switch cmd {
	case "whoami":
		var me struct {
			Username   string `json:"username"`
			Name       string `json:"name"`
			Superadmin bool   `json:"superadmin"`
		}
		must(c.do("GET", "/me", nil, &me))
		fmt.Printf("%s (@%s) on %s%s\n", me.Name, me.Username, cfg.URL, map[bool]string{true: " · superadmin"}[me.Superadmin])
	case "orgs":
		var me struct {
			Orgs []struct{ Slug, Name, Role string } `json:"orgs"`
		}
		must(c.do("GET", "/me", nil, &me))
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "SLUG\tNAME\tROLE")
		for _, o := range me.Orgs {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", o.Slug, o.Name, o.Role)
		}
		tw.Flush()
	case "apps", "ls":
		listApps(c)
	case "status":
		need(args, 1, "wackcluborchard status <app>")
		status(c, resolveApp(c, args[0]))
	case "deploy":
		fs := flag.NewFlagSet("deploy", flag.ExitOnError)
		image := fs.String("image", "", "deploy this image")
		detach := fs.Bool("detach", false, "do not follow the build")
		pos := parse(fs, args)
		need(pos, 1, "wackcluborchard deploy <app>")
		name := pos[0]
		deploy(c, resolveApp(c, name), *image, *detach)
	case "logs":
		fs := flag.NewFlagSet("logs", flag.ExitOnError)
		follow := fs.Bool("f", false, "follow")
		prev := fs.Bool("previous", false, "logs of the container that died")
		pod := fs.String("pod", "", "one pod")
		since := fs.String("since", "", "time window, like 15m")
		pos := parse(fs, args)
		need(pos, 1, "wackcluborchard logs <app>")
		name := pos[0]
		logs(c, resolveApp(c, name), *follow, *prev, *pod, *since)
	case "restart":
		need(args, 1, "wackcluborchard restart <app>")
		must(c.do("POST", "/apps/"+resolveApp(c, args[0]).ID+"/restart", nil, nil))
		fmt.Println("restarting")
	case "scale":
		need(args, 2, "wackcluborchard scale <app> <replicas>")
		var n int
		fmt.Sscan(args[1], &n)
		must(c.do("PATCH", "/apps/"+resolveApp(c, args[0]).ID, map[string]int{"replicas": n}, nil))
		fmt.Printf("scaled %s to %d\n", args[0], n)
	case "rollback":
		need(args, 1, "wackcluborchard rollback <app> [#number]")
		rollback(c, resolveApp(c, args[0]), args[1:])
	case "open":
		need(args, 1, "wackcluborchard open <app>")
		a := resolveApp(c, args[0])
		if len(a.Domains) == 0 {
			die("%s has no URL", a.Name)
		}
		u := appURL(c, a.Domains[0].Host)
		fmt.Println(u)
		openBrowser(u)
	case "env":
		env(c, args)
	case "db":
		if len(args) < 3 || args[0] != "query" {
			die("usage: wackcluborchard db query <db> \"<sql>\"")
		}
		dbQuery(c, args[1], args[2])
	case "jobs":
		jobs(c)
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		noWait := fs.Bool("no-wait", false, "trigger and return")
		pos := parse(fs, args)
		need(pos, 1, "wackcluborchard run <job>")
		name := pos[0]
		runJob(c, name, !*noWait)
	case "mcp":
		fmt.Printf("claude mcp add --transport http wackcluborchard %s/mcp --header \"Authorization: Bearer %s\"\n", strings.TrimRight(cfg.URL, "/"), cfg.Token)
	case "logout":
		os.Remove(configPath())
		fmt.Println("signed out")
	default:
		fmt.Print(usage)
		os.Exit(2)
	}
}

// parse lets flags appear anywhere among positional arguments.
func parse(fs *flag.FlagSet, args []string) []string {
	var pos []string
	for {
		fs.Parse(args)
		if fs.NArg() == 0 {
			return pos
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

func need(args []string, n int, u string) {
	if len(args) < n || args[0] == "" {
		die("usage: %s", u)
	}
}

func login(args []string) {
	if len(args) < 1 {
		die("usage: wackcluborchard login <url>")
	}
	u := args[0]
	if !strings.Contains(u, "://") {
		u = "https://" + u
	}
	u = strings.TrimRight(u, "/")
	in := bufio.NewReader(os.Stdin)
	fmt.Printf("Create a token at %s/account?tab=tokens and paste it (or press enter to use a password): ", u)
	tok, _ := in.ReadString('\n')
	tok = strings.TrimSpace(tok)
	if tok == "" {
		fmt.Print("Username: ")
		user, _ := in.ReadString('\n')
		fmt.Print("Password (echoed): ")
		pw, _ := in.ReadString('\n')
		jar := &cookieJar{}
		h := &http.Client{Jar: jar, Timeout: 30 * time.Second}
		b, _ := json.Marshal(map[string]string{"username": strings.TrimSpace(user), "password": strings.TrimSpace(pw)})
		req, _ := http.NewRequest("POST", u+"/api/auth/login", strings.NewReader(string(b)))
		req.Header.Set("Content-Type", "application/json")
		resp, err := h.Do(req)
		must(err)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			die("sign-in failed: %s", resp.Status)
		}
		host, _ := os.Hostname()
		req, _ = http.NewRequest("POST", u+"/api/me/tokens", strings.NewReader(`{"name":"cli on `+host+`"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err = h.Do(req)
		must(err)
		var out struct {
			Token string `json:"token"`
		}
		json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		tok = out.Token
	}
	c := &client{cfg: config{URL: u, Token: tok}, h: &http.Client{Timeout: 30 * time.Second}}
	var me struct {
		Username string `json:"username"`
	}
	must(c.do("GET", "/me", nil, &me))
	must(saveConfig(c.cfg))
	fmt.Printf("signed in as @%s\n", me.Username)
}

type cookieJar struct{ cookies []*http.Cookie }

func (j *cookieJar) SetCookies(u *url.URL, cs []*http.Cookie) { j.cookies = append(j.cookies, cs...) }
func (j *cookieJar) Cookies(u *url.URL) []*http.Cookie        { return j.cookies }

// ---- resolution ----

type app struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Replicas int    `json:"replicas"`
	Image    string `json:"image"`
	Source   struct {
		Type  string `json:"type"`
		Image string `json:"image"`
		Repo  string `json:"repo"`
	} `json:"source"`
	Domains []struct {
		Host string `json:"host"`
	} `json:"domains"`
	Pods []struct {
		Name     string `json:"name"`
		Phase    string `json:"phase"`
		Ready    bool   `json:"ready"`
		Restarts int    `json:"restarts"`
		Reason   string `json:"reason"`
	} `json:"pods"`
	StatusMessage string `json:"statusMessage"`
}

type project struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Apps      []app  `json:"apps"`
	Databases []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
		DBName string `json:"dbName"`
	} `json:"databases"`
}

func orgs(c *client) []string {
	var me struct {
		Orgs []struct{ Slug string } `json:"orgs"`
	}
	must(c.do("GET", "/me", nil, &me))
	var out []string
	for _, o := range me.Orgs {
		if c.cfg.Org == "" || o.Slug == c.cfg.Org {
			out = append(out, o.Slug)
		}
	}
	return out
}

func allProjects(c *client) []project {
	var out []project
	for _, o := range orgs(c) {
		var ps []project
		if c.do("GET", "/orgs/"+o+"/overview", nil, &ps) == nil {
			out = append(out, ps...)
		}
	}
	return out
}

func resolveApp(c *client, ref string) app {
	var found []app
	for _, p := range allProjects(c) {
		for _, a := range p.Apps {
			if a.ID == ref || a.Name == ref || p.Name+"/"+a.Name == ref {
				found = append(found, a)
			}
		}
	}
	if len(found) == 0 {
		die("no app called %q", ref)
	}
	if len(found) > 1 {
		die("%q matches %d apps; use project/app or the app id", ref, len(found))
	}
	return found[0]
}

func listApps(c *client) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "PROJECT\tNAME\tKIND\tSTATUS\tSOURCE\tURL")
	for _, p := range allProjects(c) {
		for _, a := range p.Apps {
			src := a.Source.Image
			if a.Source.Type == "github" {
				src = a.Source.Repo
			}
			u := ""
			if len(a.Domains) > 0 {
				u = appURL(c, a.Domains[0].Host)
			}
			fmt.Fprintf(tw, "%s\t%s\tapp\t%s\t%s\t%s\n", p.Name, a.Name, a.Status, src, u)
		}
		for _, d := range p.Databases {
			fmt.Fprintf(tw, "%s\t%s\tpostgres\t%s\t%s\t\n", p.Name, d.Name, d.Status, d.DBName)
		}
	}
	tw.Flush()
}

func status(c *client, a app) {
	var full app
	must(c.do("GET", "/apps/"+a.ID, nil, &full))
	fmt.Printf("%s  %s  %d replica(s)\n", full.Name, full.Status, full.Replicas)
	if full.StatusMessage != "" {
		fmt.Println("  " + full.StatusMessage)
	}
	fmt.Println("  image:", full.Image)
	for _, d := range full.Domains {
		fmt.Println("  url:   https://" + d.Host)
	}
	for _, p := range full.Pods {
		state := p.Phase
		if !p.Ready && p.Reason != "" {
			state = p.Reason
		}
		fmt.Printf("  pod %s  %s  restarts=%d\n", p.Name, state, p.Restarts)
	}
}

func deploy(c *client, a app, image string, detach bool) {
	var res struct {
		Deploy struct {
			ID     string `json:"id"`
			Number int    `json:"number"`
			Kind   string `json:"kind"`
		} `json:"deploy"`
		Build *struct {
			Started bool   `json:"started"`
			Message string `json:"message"`
		} `json:"build"`
	}
	must(c.do("POST", "/apps/"+a.ID+"/deploy", map[string]string{"image": image}, &res))
	fmt.Printf("deploy #%d of %s started (%s)\n", res.Deploy.Number, a.Name, res.Deploy.Kind)
	if res.Build != nil && !res.Build.Started {
		fmt.Println(res.Build.Message)
	}
	if detach {
		return
	}
	c.stream("/deploys/"+res.Deploy.ID+"/logs", func(ev string, data []byte) bool {
		var l struct {
			Text string `json:"text"`
		}
		json.Unmarshal(data, &l)
		if ev == "end" {
			return false
		}
		fmt.Println(l.Text)
		return true
	})
	var dep struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	for i := 0; i < 600; i++ {
		c.do("GET", "/deploys/"+res.Deploy.ID, nil, &dep)
		if dep.Status == "succeeded" || dep.Status == "failed" || dep.Status == "superseded" {
			break
		}
		time.Sleep(time.Second)
	}
	if dep.Status == "failed" {
		die("deploy failed: %s", dep.Error)
	}
	fmt.Println("deploy", dep.Status)
}

func logs(c *client, a app, follow, prev bool, pod, since string) {
	q := url.Values{}
	if prev {
		q.Set("previous", "1")
	}
	if pod != "" {
		q.Set("pod", pod)
	}
	if since != "" {
		q.Set("since", since)
	}
	idle := time.AfterFunc(2*time.Second, func() {
		if !follow {
			os.Exit(0)
		}
	})
	c.stream("/apps/"+a.ID+"/logs?"+q.Encode(), func(ev string, data []byte) bool {
		if ev == "end" {
			return false
		}
		var l struct {
			Pod  string `json:"pod"`
			Text string `json:"text"`
		}
		json.Unmarshal(data, &l)
		if l.Pod != "" && len(a.Pods) > 1 {
			parts := strings.Split(l.Pod, "-")
			fmt.Printf("\x1b[90m%s\x1b[0m %s\n", parts[len(parts)-1], l.Text)
		} else {
			fmt.Println(l.Text)
		}
		if !follow {
			idle.Reset(700 * time.Millisecond)
		}
		return true
	})
}

func rollback(c *client, a app, args []string) {
	var res struct {
		Deploys []struct {
			ID     string `json:"id"`
			Number int    `json:"number"`
			Status string `json:"status"`
			Image  string `json:"image"`
		} `json:"deploys"`
		Current string `json:"currentImage"`
	}
	must(c.do("GET", "/apps/"+a.ID+"/deploys", nil, &res))
	target := ""
	want := 0
	if len(args) > 0 {
		fmt.Sscan(strings.TrimPrefix(args[0], "#"), &want)
	}
	for _, d := range res.Deploys {
		if want > 0 && d.Number == want || want == 0 && d.Status == "succeeded" && d.Image != "" && d.Image != res.Current {
			target = d.ID
			fmt.Printf("rolling back to #%d (%s)\n", d.Number, d.Image)
			break
		}
	}
	if target == "" {
		die("no earlier successful deploy to roll back to")
	}
	must(c.do("POST", "/apps/"+a.ID+"/rollback/"+target, nil, nil))
}

func env(c *client, args []string) {
	fs := flag.NewFlagSet("env", flag.ExitOnError)
	appName := fs.String("app", "", "scope to one app")
	secret := fs.Bool("secret", false, "mark values secret")
	pos := parse(fs, args)
	projectRef := ""
	if len(pos) > 0 {
		projectRef = pos[0]
		pos = pos[1:]
	}
	if projectRef == "" {
		die("usage: wackcluborchard env <project> [K=V ...] [--app a] [--secret]")
	}
	var pid string
	var appID string
	for _, p := range allProjects(c) {
		if p.ID == projectRef || strings.EqualFold(p.Name, projectRef) {
			pid = p.ID
			for _, a := range p.Apps {
				if a.Name == *appName {
					appID = a.ID
				}
			}
		}
	}
	if pid == "" {
		die("no project called %q", projectRef)
	}
	if *appName != "" && appID == "" {
		die("no app %q in %s", *appName, projectRef)
	}
	if len(pos) == 0 {
		var vars []struct {
			Key     string `json:"key"`
			Value   string `json:"value"`
			Secret  bool   `json:"secret"`
			AppName string `json:"appName"`
		}
		must(c.do("GET", "/projects/"+pid+"/variables", nil, &vars))
		sort.Slice(vars, func(i, j int) bool { return vars[i].Key < vars[j].Key })
		for _, v := range vars {
			if *appName != "" && v.AppName != *appName {
				continue
			}
			val := v.Value
			if v.Secret {
				val = "••••••••"
			}
			scope := "shared"
			if v.AppName != "" {
				scope = v.AppName
			}
			fmt.Printf("%s=%s\t# %s\n", v.Key, val, scope)
		}
		return
	}
	var set []map[string]any
	for _, kv := range pos {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			die("expected KEY=value, got %q", kv)
		}
		set = append(set, map[string]any{"key": k, "value": v, "appId": appID, "secret": *secret})
	}
	var res struct {
		Redeployed int `json:"redeployed"`
	}
	must(c.do("PUT", "/projects/"+pid+"/variables", map[string]any{"variables": set}, &res))
	fmt.Printf("set %d variable(s); %d app(s) rolling out\n", len(set), res.Redeployed)
}

func dbQuery(c *client, ref, sql string) {
	var id string
	for _, p := range allProjects(c) {
		for _, d := range p.Databases {
			if d.ID == ref || d.Name == ref {
				id = d.ID
			}
		}
	}
	if id == "" {
		die("no database called %q", ref)
	}
	var res struct {
		Columns []string   `json:"columns"`
		Rows    [][]string `json:"rows"`
		Command string     `json:"command"`
		Error   string     `json:"error"`
	}
	must(c.do("POST", "/databases/"+id+"/query", map[string]string{"sql": sql}, &res))
	if res.Error != "" {
		die("%s", res.Error)
	}
	if len(res.Columns) == 0 {
		fmt.Println(res.Command)
		return
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(res.Columns, "\t"))
	for _, r := range res.Rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	tw.Flush()
	fmt.Printf("(%d rows)\n", len(res.Rows))
}

type jobRow struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProjectName  string `json:"projectName"`
	ScheduleText string `json:"scheduleText"`
	LastRun      *struct {
		Number int    `json:"number"`
		Status string `json:"status"`
	} `json:"lastRun"`
}

func allJobs(c *client) []jobRow {
	var out []jobRow
	for _, o := range orgs(c) {
		var js []jobRow
		if c.do("GET", "/orgs/"+o+"/jobs", nil, &js) == nil {
			out = append(out, js...)
		}
	}
	return out
}

func jobs(c *client) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "PROJECT\tJOB\tSCHEDULE\tLAST RUN")
	for _, j := range allJobs(c) {
		last := "never"
		if j.LastRun != nil {
			last = fmt.Sprintf("#%d %s", j.LastRun.Number, j.LastRun.Status)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", j.ProjectName, j.Name, j.ScheduleText, last)
	}
	tw.Flush()
}

func runJob(c *client, ref string, wait bool) {
	var id string
	for _, j := range allJobs(c) {
		if j.ID == ref || j.Name == ref {
			id = j.ID
		}
	}
	if id == "" {
		die("no job called %q", ref)
	}
	var run struct {
		ID     string `json:"id"`
		Number int    `json:"number"`
	}
	must(c.do("POST", "/jobs/"+id+"/runs", nil, &run))
	fmt.Printf("run #%d started\n", run.Number)
	if !wait {
		return
	}
	printed := map[int]int{}
	for {
		var r struct {
			Status string `json:"status"`
			Steps  []struct {
				Name     string `json:"name"`
				Status   string `json:"status"`
				Output   string `json:"output"`
				ExitCode *int   `json:"exitCode"`
			} `json:"steps"`
		}
		must(c.do("GET", "/runs/"+run.ID, nil, &r))
		for i, s := range r.Steps {
			if len(s.Output) > printed[i] {
				if printed[i] == 0 {
					fmt.Printf("\x1b[1m▸ %s\x1b[0m\n", s.Name)
				}
				fmt.Print(s.Output[printed[i]:])
				printed[i] = len(s.Output)
			}
		}
		if r.Status != "running" && r.Status != "queued" {
			for _, s := range r.Steps {
				code := "-"
				if s.ExitCode != nil {
					code = fmt.Sprint(*s.ExitCode)
				}
				fmt.Printf("  %-9s %s (exit %s)\n", s.Status, s.Name, code)
			}
			if r.Status != "succeeded" {
				die("run %s", r.Status)
			}
			fmt.Println("run succeeded")
			return
		}
		time.Sleep(time.Second)
	}
}

func openBrowser(u string) {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "explorer"
	default:
		cmd = "xdg-open"
	}
	exec.Command(cmd, u).Start()
}

// appURL adds the instance's HTTPS port when it is not 443 (lan mode, k3d).
func appURL(c *client, host string) string {
	var st struct {
		HTTPSPort int `json:"httpsPort"`
	}
	if c.do("GET", "/auth/state", nil, &st) == nil && st.HTTPSPort != 0 && st.HTTPSPort != 443 {
		return fmt.Sprintf("https://%s:%d", host, st.HTTPSPort)
	}
	return "https://" + host
}
