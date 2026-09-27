package runtime

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/events"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

// Sim is an in-process fake cluster. Pods start and become ready, images
// produce plausible logs, databases provision, builds stream BuildKit-ish
// output. Set WACKCLUBORCHARD_SIM_EXEC=1 to run job script steps for real with the
// local interpreters (development only: there is no isolation).
type Sim struct {
	mu        sync.Mutex
	obs       Observer
	apps      map[string]*simApp
	dbs       map[string]*simDB
	sandboxes map[string]*simSandbox
	logs      *events.LogHub
	prevLogs  map[string]string
	nodes     []Node
	realExec  bool
	ctx       context.Context
}

type simApp struct {
	spec   AppSpec
	pods   []store.Pod
	cancel context.CancelFunc
	cpu    float64
	mem    float64
	gen    int
}

type simDB struct {
	spec   DatabaseSpec
	size   int64
	tables map[string][][]string
	cols   map[string][]string
}

type simSandbox struct {
	spec  SandboxSpec
	files map[string]string
}

func NewSim() *Sim {
	return &Sim{
		apps:      map[string]*simApp{},
		dbs:       map[string]*simDB{},
		sandboxes: map[string]*simSandbox{},
		logs:      events.NewLogHub(),
		prevLogs:  map[string]string{},
		realExec:  os.Getenv("WACKCLUBORCHARD_SIM_EXEC") == "1",
		nodes: []Node{
			{Name: "wackcluborchard-control-1", Ready: true, Roles: []string{"control-plane", "master"}, CPUMillis: 8000, MemoryMi: 32768, AllocCPU: 7800, AllocMem: 31200, Arch: "amd64", Kubelet: "v1.31.4+k3s1", Labels: map[string]string{"kubernetes.io/arch": "amd64"}},
			{Name: "wackcluborchard-worker-1", Ready: true, Roles: []string{"worker"}, CPUMillis: 16000, MemoryMi: 65536, AllocCPU: 15800, AllocMem: 63900, Arch: "amd64", Kubelet: "v1.31.4+k3s1", Labels: map[string]string{"kubernetes.io/arch": "amd64"}},
			{Name: "wackcluborchard-worker-2", Ready: true, Roles: []string{"worker"}, CPUMillis: 16000, MemoryMi: 65536, AllocCPU: 15800, AllocMem: 63900, Arch: "amd64", Kubelet: "v1.31.4+k3s1", Labels: map[string]string{"kubernetes.io/arch": "amd64"}},
		},
	}
}

func (s *Sim) Name() string { return "sim" }

func (s *Sim) Start(ctx context.Context, obs Observer) error {
	s.obs = obs
	s.ctx = ctx
	go s.tick(ctx)
	return nil
}

func key(ns, name string) string { return ns + "/" + name }

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func randSuffix(n int) string {
	const a = "bcdfghjklmnpqrstvwxz2456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = a[rand.Intn(len(a))]
	}
	return string(b)
}

// ---- builds ----

func (s *Sim) Build(ctx context.Context, spec BuildSpec, log func(string)) error {
	lines := []string{
		fmt.Sprintf("#0 cloning https://github.com/%s (branch %s)", spec.Repo, spec.Branch),
		fmt.Sprintf("#0 HEAD is now at %s", firstN(spec.Commit, 7)),
		"#1 [internal] load build definition from " + orDefault(spec.Dockerfile, "Dockerfile"),
		"#1 transferring dockerfile: 1.21kB done",
		"#2 [internal] load metadata for docker.io/library/node:22-alpine",
		"#2 DONE 0.8s",
		"#3 [internal] load .dockerignore",
		"#4 [base 1/4] FROM docker.io/library/node:22-alpine@sha256:6e80991f69cc7722c561e5d14d5e72ab47c0d6b6cfb3ae50fb9cf9a7b30fdf97",
		"#4 CACHED",
		"#5 [base 2/4] WORKDIR /app",
		"#5 CACHED",
		"#6 [deps 3/4] COPY package.json package-lock.json ./",
		"#6 DONE 0.1s",
		"#7 [deps 4/4] RUN npm ci --omit=dev",
		"#7 4.112 added 214 packages, and audited 215 packages in 4s",
		"#7 4.113 found 0 vulnerabilities",
		"#7 DONE 4.6s",
		"#8 [build 1/2] COPY . .",
		"#8 DONE 0.2s",
		"#9 [build 2/2] RUN npm run build",
		"#9 1.402 > build",
		"#9 1.403 > tsc -p . && node scripts/bundle.mjs",
		"#9 6.019 bundled 312 modules in 1.9s",
		"#9 DONE 6.4s",
	}
	target := orDefault(spec.Target, "runner")
	lines = append(lines,
		fmt.Sprintf("#10 [%s 1/2] COPY --from=build /app/dist ./dist", target),
		"#10 DONE 0.1s",
		"#11 exporting to image",
		"#11 exporting layers 0.9s done",
		"#11 exporting manifest sha256:"+randHex(64)+" done",
		"#11 naming to "+spec.Image+" done",
		"#11 DONE 1.1s",
	)
	for i, l := range lines {
		if strings.Contains(spec.Repo, "fail") && i == 16 {
			log("#7 4.201 npm ERR! code ERESOLVE")
			log("#7 4.202 npm ERR! ERESOLVE could not resolve dependency tree")
			log("#7 ERROR: process \"/bin/sh -c npm ci --omit=dev\" did not complete successfully: exit code: 1")
			return fmt.Errorf("build step 7 failed: npm ci exited 1")
		}
		log(l)
		if !sleep(ctx, time.Duration(80+rand.Intn(220))*time.Millisecond) {
			return ctx.Err()
		}
	}
	return nil
}

func randHex(n int) string {
	const h = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = h[rand.Intn(16)]
	}
	return string(b)
}

func firstN(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// ---- apps ----

func (s *Sim) ApplyApp(ctx context.Context, spec AppSpec) error {
	s.mu.Lock()
	a := s.apps[key(spec.Namespace, spec.Name)]
	if a == nil {
		a = &simApp{cpu: 5 + rand.Float64()*20, mem: 10 + rand.Float64()*30}
		s.apps[key(spec.Namespace, spec.Name)] = a
	}
	changed := a.spec.Image != spec.Image || a.spec.RestartNonce != spec.RestartNonce || a.spec.Command != spec.Command || !sameEnv(a.spec.Env, spec.Env) || a.spec.Resources != spec.Resources
	a.spec = spec
	if a.cancel != nil && changed {
		a.cancel()
		a.cancel = nil
	}
	s.mu.Unlock()
	s.reconcile(spec.Namespace, spec.Name, changed)
	return nil
}

func sameEnv(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// reconcile moves the pod set towards spec.Replicas, rolling pods if the
// template changed.
func (s *Sim) reconcile(ns, name string, roll bool) {
	s.mu.Lock()
	a := s.apps[key(ns, name)]
	if a == nil {
		s.mu.Unlock()
		return
	}
	if roll || a.cancel == nil {
		a.gen++
		hash := randSuffix(10)
		ctx, cancel := context.WithCancel(s.ctx)
		a.cancel = cancel
		old := a.pods
		a.pods = nil
		for i := 0; i < a.spec.Replicas; i++ {
			a.pods = append(a.pods, store.Pod{Name: name + "-" + hash + "-" + randSuffix(5), Phase: "Pending", Node: s.nodes[rand.Intn(len(s.nodes))].Name, StartedAt: time.Now()})
		}
		// keep old pods briefly for a rolling update
		for i := range old {
			old[i].Ready = false
			old[i].Phase = "Terminating"
		}
		a.pods = append(a.pods, old...)
		spec := a.spec
		gen := a.gen
		s.mu.Unlock()
		s.publish(ns, name)
		go s.runPods(ctx, spec, gen, hash)
		return
	}
	// scale only
	hash := ""
	var alive []store.Pod
	for _, p := range a.pods {
		if p.Phase != "Terminating" {
			alive = append(alive, p)
			if hash == "" {
				parts := strings.Split(p.Name, "-")
				if len(parts) >= 3 {
					hash = parts[len(parts)-2]
				}
			}
		}
	}
	if hash == "" {
		hash = randSuffix(10)
	}
	for len(alive) < a.spec.Replicas {
		alive = append(alive, store.Pod{Name: name + "-" + hash + "-" + randSuffix(5), Phase: "Pending", Node: s.nodes[rand.Intn(len(s.nodes))].Name, StartedAt: time.Now()})
	}
	if len(alive) > a.spec.Replicas {
		alive = alive[:a.spec.Replicas]
	}
	a.pods = alive
	s.mu.Unlock()
	s.publish(ns, name)
	go func() {
		sleep(s.ctx, 900*time.Millisecond)
		s.mu.Lock()
		if a := s.apps[key(ns, name)]; a != nil {
			for i := range a.pods {
				if a.pods[i].Phase == "Pending" {
					a.pods[i].Phase = "Running"
					a.pods[i].Ready = !strings.Contains(a.spec.Image, "crash")
				}
			}
		}
		s.mu.Unlock()
		s.publish(ns, name)
	}()
}

func (s *Sim) publish(ns, name string) {
	s.mu.Lock()
	a := s.apps[key(ns, name)]
	if a == nil {
		s.mu.Unlock()
		return
	}
	pods := append([]store.Pod(nil), a.pods...)
	id := a.spec.ID
	s.mu.Unlock()
	if s.obs != nil {
		s.obs.AppPods(id, pods)
	}
}

func (s *Sim) runPods(ctx context.Context, spec AppSpec, gen int, hash string) {
	if !sleep(ctx, 700*time.Millisecond) {
		return
	}
	crashing := strings.Contains(spec.Image, "crash") || strings.Contains(spec.Command, "exit 1")
	s.mu.Lock()
	a := s.apps[key(spec.Namespace, spec.Name)]
	if a == nil || a.gen != gen {
		s.mu.Unlock()
		return
	}
	for i := range a.pods {
		if a.pods[i].Phase == "Pending" {
			a.pods[i].Phase = "Running"
		}
	}
	s.mu.Unlock()
	s.publish(spec.Namespace, spec.Name)
	for _, l := range bootLines(spec) {
		s.emit(spec, "", l)
	}
	if crashing {
		s.crashLoop(ctx, spec, gen)
		return
	}
	if !sleep(ctx, time.Duration(spec.Health.Initial)*100*time.Millisecond+600*time.Millisecond) {
		return
	}
	s.mu.Lock()
	if a := s.apps[key(spec.Namespace, spec.Name)]; a != nil && a.gen == gen {
		var keep []store.Pod
		for _, p := range a.pods {
			if p.Phase == "Terminating" {
				continue
			}
			p.Ready = true
			keep = append(keep, p)
		}
		a.pods = keep
	}
	s.mu.Unlock()
	s.publish(spec.Namespace, spec.Name)
	// steady-state traffic
	for {
		if !sleep(ctx, time.Duration(900+rand.Intn(2600))*time.Millisecond) {
			return
		}
		s.mu.Lock()
		a := s.apps[key(spec.Namespace, spec.Name)]
		if a == nil || a.gen != gen {
			s.mu.Unlock()
			return
		}
		var pods []string
		for _, p := range a.pods {
			if p.Ready {
				pods = append(pods, p.Name)
			}
		}
		s.mu.Unlock()
		if len(pods) == 0 {
			continue
		}
		s.emit(spec, pods[rand.Intn(len(pods))], trafficLine(spec))
	}
}

func (s *Sim) crashLoop(ctx context.Context, spec AppSpec, gen int) {
	restarts := 0
	for {
		errLines := []string{
			"node:internal/modules/cjs/loader:1228",
			"  throw err;",
			"  ^",
			"",
			"Error: Cannot find module 'pg'",
			"Require stack:",
			"- /app/dist/server.js",
			"    at Module._resolveFilename (node:internal/modules/cjs/loader:1225:15)",
			"    at Object.<anonymous> (/app/dist/server.js:3:18)",
			"  code: 'MODULE_NOT_FOUND'",
			"",
			"Node.js v22.11.0",
		}
		for _, l := range errLines {
			s.emit(spec, "", l)
		}
		restarts++
		s.mu.Lock()
		a := s.apps[key(spec.Namespace, spec.Name)]
		if a == nil || a.gen != gen {
			s.mu.Unlock()
			return
		}
		var podName string
		for i := range a.pods {
			if a.pods[i].Phase == "Terminating" {
				continue
			}
			a.pods[i].Restarts = restarts
			a.pods[i].Ready = false
			a.pods[i].Reason = "CrashLoopBackOff"
			podName = a.pods[i].Name
		}
		s.prevLogs[key(spec.Namespace, spec.Name)] = strings.Join(errLines, "\n")
		s.mu.Unlock()
		s.publish(spec.Namespace, spec.Name)
		if s.obs != nil && restarts <= 3 {
			s.obs.Crash(spec.ID, store.CrashReport{Pod: podName, Container: spec.Name, Reason: "Error", ExitCode: 1, Logs: strings.Join(errLines, "\n")})
			s.obs.Warning(spec.ID, store.KubeEvent{Object: "Pod/" + podName, Reason: "BackOff", Message: "Back-off restarting failed container " + spec.Name + " in pod " + podName})
		}
		backoff := time.Duration(math.Min(float64(restarts)*2, 10)) * time.Second
		if !sleep(ctx, backoff) {
			return
		}
	}
}

func (s *Sim) emit(spec AppSpec, pod, line string) {
	if pod == "" {
		s.mu.Lock()
		if a := s.apps[key(spec.Namespace, spec.Name)]; a != nil && len(a.pods) > 0 {
			pod = a.pods[0].Name
		}
		s.mu.Unlock()
	}
	s.logs.Append(key(spec.Namespace, spec.Name), events.LogLine{Pod: pod, Text: line})
	if s.obs != nil {
		s.obs.AppLog(spec.ID, pod, line)
	}
}

func bootLines(spec AppSpec) []string {
	img := spec.Image
	now := time.Now().UTC()
	port := 8080
	if len(spec.Ports) > 0 {
		port = spec.Ports[0].Port
	}
	switch {
	case strings.Contains(img, "nginx"):
		return []string{
			"/docker-entrypoint.sh: /docker-entrypoint.d/ is not empty, will attempt to perform configuration",
			"/docker-entrypoint.sh: Looking for shell scripts in /docker-entrypoint.d/",
			"10-listen-on-ipv6-by-default.sh: info: Enabled listen on IPv6 in /etc/nginx/conf.d/default.conf",
			"/docker-entrypoint.sh: Configuration complete; ready for start up",
			now.Format("2006/01/02 15:04:05") + " [notice] 1#1: nginx/1.27.3",
			now.Format("2006/01/02 15:04:05") + " [notice] 1#1: start worker processes",
		}
	case strings.Contains(img, "redis"):
		return []string{
			"1:C " + now.Format("02 Jan 2006 15:04:05.000") + " # oO0OoO0OoO0Oo Redis is starting oO0OoO0OoO0Oo",
			"1:C " + now.Format("02 Jan 2006 15:04:05.000") + " # Redis version=7.4.1, bits=64, commit=00000000, modified=0, pid=1, just started",
			"1:M " + now.Format("02 Jan 2006 15:04:05.000") + " * monotonic clock: POSIX clock_gettime",
			"1:M " + now.Format("02 Jan 2006 15:04:05.000") + " * Running mode=standalone, port=6379.",
			"1:M " + now.Format("02 Jan 2006 15:04:05.000") + " * Server initialized",
			"1:M " + now.Format("02 Jan 2006 15:04:05.000") + " * Ready to accept connections tcp",
		}
	case strings.Contains(img, "whoami"):
		return []string{fmt.Sprintf("%s Starting up on port %d", now.Format("2006/01/02 15:04:05"), port)}
	case strings.Contains(img, "postgres"):
		return []string{"LOG:  database system is ready to accept connections"}
	default:
		return []string{
			"> start",
			"> node dist/server.js",
			fmt.Sprintf("{\"level\":\"info\",\"time\":%d,\"msg\":\"listening on 0.0.0.0:%d\"}", now.UnixMilli(), port),
		}
	}
}

var paths = []string{"/", "/", "/", "/health", "/api/items", "/favicon.ico", "/assets/app.js", "/api/me", "/login"}

func trafficLine(spec AppSpec) string {
	now := time.Now().UTC()
	ip := fmt.Sprintf("10.42.%d.%d", rand.Intn(4), 2+rand.Intn(250))
	p := paths[rand.Intn(len(paths))]
	code := 200
	if rand.Intn(25) == 0 {
		code = 404
	}
	if rand.Intn(80) == 0 {
		code = 500
	}
	switch {
	case strings.Contains(spec.Image, "nginx"):
		return fmt.Sprintf(`%s - - [%s] "GET %s HTTP/1.1" %d %d "-" "Mozilla/5.0" "-"`, ip, now.Format("02/Jan/2006:15:04:05 -0700"), p, code, 200+rand.Intn(9000))
	case strings.Contains(spec.Image, "redis"):
		if rand.Intn(3) == 0 {
			return "1:M " + now.Format("02 Jan 2006 15:04:05.000") + " * 100 changes in 300 seconds. Saving..."
		}
		return "1:M " + now.Format("02 Jan 2006 15:04:05.000") + " * Background saving terminated with success"
	case strings.Contains(spec.Image, "whoami"):
		return fmt.Sprintf("%s %s GET %s", now.Format("2006/01/02 15:04:05"), ip, p)
	default:
		return fmt.Sprintf(`{"level":"info","time":%d,"req":{"method":"GET","url":"%s"},"res":{"statusCode":%d},"responseTime":%d}`, now.UnixMilli(), p, code, 2+rand.Intn(80))
	}
}

func (s *Sim) DeleteApp(ctx context.Context, ns, name string) error {
	s.mu.Lock()
	if a := s.apps[key(ns, name)]; a != nil && a.cancel != nil {
		a.cancel()
	}
	delete(s.apps, key(ns, name))
	s.mu.Unlock()
	s.logs.Drop(key(ns, name))
	return nil
}

// Scale is fast-pathed by ApplyApp noticing only replicas changed.

func (s *Sim) Logs(ctx context.Context, ns, name, pod string, previous bool, since time.Duration, out func(pod, line string)) error {
	if previous {
		s.mu.Lock()
		prev := s.prevLogs[key(ns, name)]
		s.mu.Unlock()
		if prev == "" {
			out("", "(no previous container: nothing has restarted)")
			return nil
		}
		for _, l := range strings.Split(prev, "\n") {
			out(pod, l)
		}
		return nil
	}
	c, tail := s.logs.Subscribe(key(ns, name))
	defer s.logs.Unsubscribe(key(ns, name), c)
	cutoff := time.Time{}
	if since > 0 {
		cutoff = time.Now().Add(-since)
	}
	for _, l := range tail {
		if (pod == "" || l.Pod == pod) && l.At.After(cutoff) {
			out(l.Pod, l.Text)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case l := <-c:
			if pod == "" || l.Pod == pod {
				out(l.Pod, l.Text)
			}
		}
	}
}

func (s *Sim) AppMetrics(ctx context.Context, ns, name string) (Metrics, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.apps[key(ns, name)]
	if a == nil {
		return Metrics{}, nil
	}
	ready := 0
	for _, p := range a.pods {
		if p.Ready {
			ready++
		}
	}
	a.cpu = clamp(a.cpu+(rand.Float64()-0.5)*6, 1, float64(a.spec.Resources.CPUMillis)*0.9+1)
	a.mem = clamp(a.mem+(rand.Float64()-0.48)*2, 4, float64(a.spec.Resources.MemoryMi)*0.9+1)
	return Metrics{CPUMillis: a.cpu * float64(ready), MemoryMi: a.mem * float64(ready)}, nil
}

func clamp(v, lo, hi float64) float64 {
	if hi < lo {
		hi = lo
	}
	return math.Max(lo, math.Min(hi, v))
}

func (s *Sim) Exec(ctx context.Context, ns, name, pod string, cmd []string, io Stdio) error {
	s.mu.Lock()
	a := s.apps[key(ns, name)]
	var spec AppSpec
	if a != nil {
		spec = a.spec
		if pod == "" && len(a.pods) > 0 {
			pod = a.pods[0].Name
		}
	}
	s.mu.Unlock()
	if a == nil {
		return fmt.Errorf("app %s is not running", name)
	}
	env := map[string]string{"HOSTNAME": pod, "HOME": "/root", "PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	for k, v := range spec.Env {
		env[k] = v
	}
	for k := range spec.Secrets {
		env[k] = spec.Secrets[k]
	}
	files := map[string]string{
		"/etc/hostname":   pod + "\n",
		"/etc/os-release": "PRETTY_NAME=\"Alpine Linux v3.20\"\nNAME=\"Alpine Linux\"\nID=alpine\nVERSION_ID=3.20.3\n",
	}
	return fakeShell(ctx, io, pod, env, files)
}

// fakeShell is a tiny line-oriented shell so the terminal UI has something
// real-feeling to talk to without a cluster.
func fakeShell(ctx context.Context, io Stdio, host string, env map[string]string, files map[string]string) error {
	w := io.Stdout
	cwd := "/app"
	dirs := map[string][]string{
		"/":    {"app", "bin", "etc", "home", "root", "tmp", "usr", "var"},
		"/app": {"dist", "node_modules", "package.json"},
		"/etc": {"hostname", "hosts", "os-release", "passwd"},
	}
	sc := bufio.NewScanner(io.Stdin)
	for sc.Scan() {
		if ctx.Err() != nil {
			return nil
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		switch f[0] {
		case "exit", "logout":
			return nil
		case "pwd":
			fmt.Fprintln(w, cwd)
		case "whoami":
			fmt.Fprintln(w, "root")
		case "hostname":
			fmt.Fprintln(w, host)
		case "uname":
			fmt.Fprintln(w, "Linux "+host+" 6.8.0-49-generic #49-Ubuntu SMP x86_64 Linux")
		case "date":
			fmt.Fprintln(w, time.Now().UTC().Format("Mon Jan  2 15:04:05 UTC 2006"))
		case "echo":
			out := strings.Join(f[1:], " ")
			for k, v := range env {
				out = strings.ReplaceAll(out, "$"+k, v)
			}
			fmt.Fprintln(w, out)
		case "env", "printenv":
			keys := make([]string, 0, len(env))
			for k := range env {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if len(f) > 1 && f[0] == "printenv" && f[1] != k {
					continue
				}
				if len(f) > 1 && f[0] == "printenv" {
					fmt.Fprintln(w, env[k])
				} else {
					fmt.Fprintf(w, "%s=%s\n", k, env[k])
				}
			}
		case "ls":
			d := cwd
			if len(f) > 1 && !strings.HasPrefix(f[len(f)-1], "-") {
				d = f[len(f)-1]
			}
			if entries, ok := dirs[d]; ok {
				fmt.Fprintln(w, strings.Join(entries, "  "))
			} else {
				fmt.Fprintf(w, "ls: %s: No such file or directory\n", d)
			}
		case "cd":
			if len(f) < 2 {
				cwd = "/root"
			} else if _, ok := dirs[f[1]]; ok {
				cwd = f[1]
			} else {
				fmt.Fprintf(w, "sh: cd: can't cd to %s: No such file or directory\n", f[1])
			}
		case "cat":
			for _, p := range f[1:] {
				if c, ok := files[p]; ok {
					fmt.Fprint(w, c)
				} else {
					fmt.Fprintf(w, "cat: can't open '%s': No such file or directory\n", p)
				}
			}
		case "ps":
			fmt.Fprintln(w, "PID   USER     TIME  COMMAND\n    1 root      0:02 node dist/server.js\n   42 root      0:00 sh\n   48 root      0:00 ps")
		case "df":
			fmt.Fprintln(w, "Filesystem     1K-blocks    Used Available Use% Mounted on\noverlay         61202244 18234120  39830184  32% /")
		case "free":
			fmt.Fprintln(w, "              total        used        free\nMem:         262144       41212      220932")
		case "help":
			fmt.Fprintln(w, "simulated shell: ls cd pwd cat env printenv echo ps df free uname hostname whoami date exit")
		default:
			fmt.Fprintf(w, "sh: %s: not found\n", f[0])
		}
	}
	return nil
}

// ---- databases ----

func (s *Sim) ApplyDatabase(ctx context.Context, spec DatabaseSpec) error {
	s.mu.Lock()
	d := s.dbs[key(spec.Namespace, spec.Name)]
	fresh := d == nil
	if fresh {
		d = &simDB{size: 7_600_000 + rand.Int63n(200_000), tables: map[string][][]string{}, cols: map[string][]string{}}
		s.dbs[key(spec.Namespace, spec.Name)] = d
	}
	d.spec = spec
	s.mu.Unlock()
	go func() {
		if fresh {
			s.obs.DatabaseState(spec.ID, "provisioning", 0, 0)
			if !sleep(s.ctx, 2500*time.Millisecond) {
				return
			}
		}
		if spec.Stopped {
			s.obs.DatabaseState(spec.ID, "stopped", d.size, 0)
			return
		}
		s.obs.DatabaseState(spec.ID, "ready", d.size, 0)
	}()
	return nil
}

func (s *Sim) DeleteDatabase(ctx context.Context, ns, name string) error {
	s.mu.Lock()
	delete(s.dbs, key(ns, name))
	s.mu.Unlock()
	return nil
}

func (s *Sim) Backup(ctx context.Context, spec DatabaseSpec) (int64, error) {
	if !sleep(ctx, 1500*time.Millisecond) {
		return 0, ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if d := s.dbs[key(spec.Namespace, spec.Name)]; d != nil {
		return d.size / 3, nil
	}
	return 0, fmt.Errorf("database not found")
}

func (s *Sim) Query(ctx context.Context, spec DatabaseSpec, q string) (*QueryResult, error) {
	s.mu.Lock()
	d := s.dbs[key(spec.Namespace, spec.Name)]
	s.mu.Unlock()
	if d == nil {
		return nil, fmt.Errorf("database is not running")
	}
	start := time.Now()
	res, err := d.exec(spec, q)
	if res != nil {
		res.Duration = float64(time.Since(start).Microseconds())/1000 + 0.3 + rand.Float64()
	}
	return res, err
}

func (s *Sim) DatabaseShell(ctx context.Context, spec DatabaseSpec, io Stdio) error {
	w := io.Stdout
	fmt.Fprintf(w, "psql (%d.2)\nType \"help\" for help.\n\n", spec.Version)
	sc := bufio.NewScanner(io.Stdin)
	var buf strings.Builder
	for sc.Scan() {
		line := sc.Text()
		t := strings.TrimSpace(line)
		if t == `\q` || t == "exit" || t == "quit" {
			return nil
		}
		if strings.HasPrefix(t, `\`) {
			res, err := s.Query(ctx, spec, t)
			printResult(w, res, err)
			continue
		}
		buf.WriteString(line + "\n")
		if strings.HasSuffix(t, ";") {
			res, err := s.Query(ctx, spec, buf.String())
			printResult(w, res, err)
			buf.Reset()
		}
	}
	return nil
}

func printResult(w interface{ Write([]byte) (int, error) }, r *QueryResult, err error) {
	if err != nil {
		fmt.Fprintf(w, "ERROR:  %v\n", err)
		return
	}
	if len(r.Columns) == 0 {
		fmt.Fprintln(w, r.Command)
		return
	}
	widths := make([]int, len(r.Columns))
	for i, c := range r.Columns {
		widths[i] = len(c)
	}
	for _, row := range r.Rows {
		for i, v := range row {
			if i < len(widths) && len(v) > widths[i] {
				widths[i] = len(v)
			}
		}
	}
	var hdr, sep []string
	for i, c := range r.Columns {
		hdr = append(hdr, " "+pad(c, widths[i])+" ")
		sep = append(sep, strings.Repeat("-", widths[i]+2))
	}
	fmt.Fprintln(w, strings.Join(hdr, "|"))
	fmt.Fprintln(w, strings.Join(sep, "+"))
	for _, row := range r.Rows {
		var cells []string
		for i, v := range row {
			cells = append(cells, " "+pad(v, widths[i])+" ")
		}
		fmt.Fprintln(w, strings.Join(cells, "|"))
	}
	fmt.Fprintf(w, "(%d row%s)\n\n", len(r.Rows), plural(len(r.Rows)))
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// exec understands just enough SQL to make the console feel alive:
// CREATE TABLE, INSERT, SELECT * / count(*), DROP TABLE, and a few
// catalog queries.
func (d *simDB) exec(spec DatabaseSpec, q string) (*QueryResult, error) {
	q = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(q), ";"))
	lq := strings.ToLower(q)
	switch {
	case lq == `\dt` || strings.Contains(lq, "information_schema.tables") || strings.Contains(lq, "pg_tables"):
		r := &QueryResult{Columns: []string{"schema", "name", "type", "owner"}, Command: "SELECT"}
		names := make([]string, 0, len(d.tables))
		for n := range d.tables {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			r.Rows = append(r.Rows, []string{"public", n, "table", spec.User})
		}
		return r, nil
	case lq == `\dx` || strings.Contains(lq, "pg_extension"):
		r := &QueryResult{Columns: []string{"name", "version", "schema"}, Command: "SELECT"}
		r.Rows = append(r.Rows, []string{"plpgsql", "1.0", "pg_catalog"})
		for _, e := range spec.Extensions {
			r.Rows = append(r.Rows, []string{e, "1.0", "public"})
		}
		return r, nil
	case lq == `\l`:
		return &QueryResult{Columns: []string{"Name", "Owner", "Encoding"}, Rows: [][]string{{spec.DBName, spec.User, "UTF8"}}, Command: "SELECT"}, nil
	case strings.HasPrefix(lq, `\`):
		return nil, fmt.Errorf("invalid command %s", q)
	case strings.HasPrefix(lq, "select version()"):
		return &QueryResult{Columns: []string{"version"}, Rows: [][]string{{fmt.Sprintf("PostgreSQL %d.2 on x86_64-pc-linux-gnu, compiled by gcc (Debian 12.2.0-14) 12.2.0, 64-bit", spec.Version)}}, Command: "SELECT 1"}, nil
	case strings.HasPrefix(lq, "select now()"), strings.HasPrefix(lq, "select current_timestamp"):
		return &QueryResult{Columns: []string{"now"}, Rows: [][]string{{time.Now().UTC().Format("2006-01-02 15:04:05.000000-07")}}, Command: "SELECT 1"}, nil
	case strings.HasPrefix(lq, "select current_user"):
		return &QueryResult{Columns: []string{"current_user"}, Rows: [][]string{{spec.User}}, Command: "SELECT 1"}, nil
	case strings.HasPrefix(lq, "create table"):
		rest := strings.TrimSpace(q[len("create table"):])
		rest = strings.TrimPrefix(strings.TrimPrefix(rest, "if not exists "), "IF NOT EXISTS ")
		open := strings.Index(rest, "(")
		if open < 0 {
			return nil, fmt.Errorf("syntax error at end of input")
		}
		name := strings.ToLower(strings.TrimSpace(rest[:open]))
		body := strings.TrimSuffix(strings.TrimSpace(rest[open+1:]), ")")
		var cols []string
		for _, c := range splitTop(body) {
			f := strings.Fields(strings.TrimSpace(c))
			if len(f) > 0 && !strings.EqualFold(f[0], "primary") && !strings.EqualFold(f[0], "constraint") {
				cols = append(cols, strings.ToLower(strings.Trim(f[0], `"`)))
			}
		}
		if _, ok := d.tables[name]; ok {
			return nil, fmt.Errorf("relation \"%s\" already exists", name)
		}
		d.tables[name] = nil
		d.cols[name] = cols
		d.size += 8192
		return &QueryResult{Command: "CREATE TABLE"}, nil
	case strings.HasPrefix(lq, "drop table"):
		name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(lq, "drop table"), " if exists")))
		if _, ok := d.tables[name]; !ok {
			return nil, fmt.Errorf("table \"%s\" does not exist", name)
		}
		delete(d.tables, name)
		delete(d.cols, name)
		return &QueryResult{Command: "DROP TABLE"}, nil
	case strings.HasPrefix(lq, "insert into"):
		rest := strings.TrimSpace(q[len("insert into"):])
		sp := strings.IndexAny(rest, " (")
		if sp < 0 {
			return nil, fmt.Errorf("syntax error")
		}
		name := strings.ToLower(rest[:sp])
		cols, ok := d.cols[name]
		if !ok {
			return nil, fmt.Errorf("relation \"%s\" does not exist", name)
		}
		vi := strings.Index(strings.ToLower(rest), "values")
		if vi < 0 {
			return nil, fmt.Errorf("syntax error: expected VALUES")
		}
		target := cols
		if head := strings.TrimSpace(rest[sp:vi]); strings.HasPrefix(head, "(") {
			target = nil
			for _, c := range splitTop(strings.Trim(head, "()")) {
				target = append(target, strings.ToLower(strings.TrimSpace(c)))
			}
		}
		n := 0
		for _, tuple := range splitTuples(rest[vi+6:]) {
			vals := splitTop(tuple)
			row := make([]string, len(cols))
			for i, c := range cols {
				row[i] = ""
				if c == "id" {
					row[i] = fmt.Sprint(len(d.tables[name]) + 1)
				}
				if c == "created_at" {
					row[i] = time.Now().UTC().Format("2006-01-02 15:04:05")
				}
			}
			for i, v := range vals {
				if i < len(target) {
					for j, c := range cols {
						if c == target[i] {
							row[j] = strings.Trim(strings.TrimSpace(v), "'")
						}
					}
				}
			}
			d.tables[name] = append(d.tables[name], row)
			d.size += 64
			n++
		}
		return &QueryResult{Command: fmt.Sprintf("INSERT 0 %d", n)}, nil
	case strings.HasPrefix(lq, "select"):
		fi := strings.Index(lq, " from ")
		if fi < 0 {
			// SELECT <literal>
			expr := strings.TrimSpace(q[6:])
			return &QueryResult{Columns: []string{"?column?"}, Rows: [][]string{{strings.Trim(expr, "'")}}, Command: "SELECT 1"}, nil
		}
		what := strings.TrimSpace(lq[6:fi])
		tail := strings.Fields(lq[fi+6:])
		if len(tail) == 0 {
			return nil, fmt.Errorf("syntax error")
		}
		name := tail[0]
		rows, ok := d.tables[name]
		if !ok {
			return nil, fmt.Errorf("relation \"%s\" does not exist", name)
		}
		limit := len(rows)
		for i, t := range tail {
			if t == "limit" && i+1 < len(tail) {
				fmt.Sscan(tail[i+1], &limit)
			}
		}
		if what == "count(*)" {
			return &QueryResult{Columns: []string{"count"}, Rows: [][]string{{fmt.Sprint(len(rows))}}, Command: "SELECT 1"}, nil
		}
		if limit > len(rows) {
			limit = len(rows)
		}
		r := &QueryResult{Columns: d.cols[name], Rows: rows[:limit], Command: fmt.Sprintf("SELECT %d", limit)}
		return r, nil
	case strings.HasPrefix(lq, "create extension"):
		return &QueryResult{Command: "CREATE EXTENSION"}, nil
	case strings.HasPrefix(lq, "begin"), strings.HasPrefix(lq, "commit"):
		return &QueryResult{Command: strings.ToUpper(strings.Fields(lq)[0])}, nil
	}
	return nil, fmt.Errorf("the simulated runtime only understands a small SQL subset; connect a cluster for the real thing")
}

func splitTop(s string) []string {
	var out []string
	depth, start := 0, 0
	inQ := false
	for i, r := range s {
		switch {
		case r == '\'':
			inQ = !inQ
		case inQ:
		case r == '(':
			depth++
		case r == ')':
			depth--
		case r == ',' && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if strings.TrimSpace(s[start:]) != "" {
		out = append(out, s[start:])
	}
	return out
}

func splitTuples(s string) []string {
	var out []string
	depth, start := 0, -1
	inQ := false
	for i, r := range s {
		switch {
		case r == '\'':
			inQ = !inQ
		case inQ:
		case r == '(':
			if depth == 0 {
				start = i + 1
			}
			depth++
		case r == ')':
			depth--
			if depth == 0 && start >= 0 {
				out = append(out, s[start:i])
			}
		}
	}
	return out
}

// ---- jobs ----

func (s *Sim) RunJob(ctx context.Context, spec JobSpec, update func(StepUpdate)) error {
	failed := false
	for i, st := range spec.Steps {
		if failed {
			update(StepUpdate{Index: i, Status: "skipped"})
			continue
		}
		update(StepUpdate{Index: i, Status: "running"})
		code := 0
		if s.realExec && st.Script != "" {
			code = s.execLocal(ctx, st, spec.Env, func(l string) { update(StepUpdate{Index: i, Output: l + "\n"}) })
		} else {
			for _, l := range simulateStep(st) {
				if !sleep(ctx, time.Duration(300+rand.Intn(600))*time.Millisecond) {
					return ctx.Err()
				}
				update(StepUpdate{Index: i, Output: l + "\n"})
			}
			if strings.Contains(st.Script, "exit 1") || strings.Contains(strings.Join(st.Command, " "), "exit 1") {
				code = 1
			}
		}
		c := code
		status := "succeeded"
		if code != 0 {
			status = "failed"
			failed = true
		}
		update(StepUpdate{Index: i, Status: status, ExitCode: &c})
	}
	if failed {
		return fmt.Errorf("a step failed")
	}
	return nil
}

func (s *Sim) execLocal(ctx context.Context, st ResolvedStep, env map[string]string, out func(string)) int {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	var cmd *exec.Cmd
	switch st.Lang {
	case "python":
		cmd = exec.CommandContext(ctx, "python3", "-c", st.Script)
	case "node":
		cmd = exec.CommandContext(ctx, "node", "-e", st.Script)
	default:
		cmd = exec.CommandContext(ctx, "bash", "-c", st.Script)
	}
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	b, err := cmd.CombinedOutput()
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if l != "" {
			out(l)
		}
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		out(err.Error())
		return 127
	}
	return 0
}

// simulateStep produces believable output for a step without running it:
// echo lines are echoed, and a few idioms (curl loops) are recognized.
func simulateStep(st ResolvedStep) []string {
	src := st.Script
	if src == "" {
		src = strings.Join(st.Command, " ")
	}
	var out []string
	for _, raw := range strings.Split(src, "\n") {
		l := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(l, "echo "):
			out = append(out, strings.Trim(strings.TrimPrefix(l, "echo "), `"'`))
		case strings.HasPrefix(l, "print("):
			out = append(out, strings.Trim(strings.TrimSuffix(strings.TrimPrefix(l, "print("), ")"), `"'`))
		case strings.HasPrefix(l, "console.log("):
			out = append(out, strings.Trim(strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(l, "console.log("), ";"), ")"), "`\"'"))
		}
	}
	if len(out) == 0 {
		out = append(out, fmt.Sprintf("[%s] %s: completed", st.Image, st.Name))
	}
	return out
}

// ---- sandboxes ----

var bootStages = []struct {
	pct   int
	stage string
}{
	{5, "Scheduling onto a Kata-capable node"},
	{20, "Starting micro-VM"},
	{40, "Attaching persistent workspace"},
	{60, "Pulling sandbox image"},
	{80, "Cloning repository"},
	{95, "Starting agent and SSH"},
	{100, "Ready"},
}

func (s *Sim) ApplySandbox(ctx context.Context, spec SandboxSpec) error {
	s.mu.Lock()
	sb := s.sandboxes[key(spec.Namespace, spec.Name)]
	if sb == nil {
		sb = &simSandbox{files: map[string]string{
			"README.md":   "# " + spec.Name + "\n\nThis is your Wack Club Orchard greenhouse.\n",
			"src/main.go": "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello from the greenhouse\")\n}\n",
			".gitignore":  "node_modules\n.env\n",
		}}
		s.sandboxes[key(spec.Namespace, spec.Name)] = sb
	}
	sb.spec = spec
	s.mu.Unlock()
	go func() {
		for _, st := range bootStages {
			if !sleep(s.ctx, time.Duration(500+rand.Intn(700))*time.Millisecond) {
				return
			}
			status := "booting"
			if st.pct == 100 {
				status = "running"
			}
			s.obs.SandboxState(spec.ID, status, st.pct, st.stage)
		}
	}()
	return nil
}

func (s *Sim) DeleteSandbox(ctx context.Context, ns, name string) error {
	s.mu.Lock()
	delete(s.sandboxes, key(ns, name))
	s.mu.Unlock()
	return nil
}

// SandboxExec answers the handful of commands the workspace UI issues
// (file listing, reading, writing, git) against an in-memory tree, and
// falls back to the fake shell for interactive use.
func (s *Sim) SandboxExec(ctx context.Context, ns, name string, cmd []string, io Stdio) error {
	s.mu.Lock()
	sb := s.sandboxes[key(ns, name)]
	s.mu.Unlock()
	if sb == nil {
		return fmt.Errorf("sandbox is not running")
	}
	w := io.Stdout
	if len(cmd) == 0 {
		return fakeShell(ctx, io, name, map[string]string{"HOME": "/workspace", "USER": "wackcluborchard"}, map[string]string{})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch cmd[0] {
	case "wackcluborchard-ls":
		paths := make([]string, 0, len(sb.files))
		for p := range sb.files {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		fmt.Fprint(w, strings.Join(paths, "\n"))
	case "wackcluborchard-read":
		c, ok := sb.files[cmd[1]]
		if !ok {
			return fmt.Errorf("no such file: %s", cmd[1])
		}
		fmt.Fprint(w, c)
	case "wackcluborchard-write":
		b, _ := readAll(io.Stdin)
		sb.files[cmd[1]] = string(b)
	case "wackcluborchard-rm":
		delete(sb.files, cmd[1])
	case "git":
		fmt.Fprint(w, simGit(cmd[1:], sb))
	default:
		fmt.Fprintf(w, "%s: simulated sandbox\n", cmd[0])
	}
	return nil
}

func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	var out []byte
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			return out, nil
		}
	}
}

func simGit(args []string, sb *simSandbox) string {
	if len(args) == 0 {
		return ""
	}
	switch args[0] {
	case "branch":
		return "* main\n  remotes/origin/main\n"
	case "status":
		return "## main...origin/main\n M README.md\n"
	case "diff":
		return "diff --git a/README.md b/README.md\n--- a/README.md\n+++ b/README.md\n@@ -1,3 +1,3 @@\n # " + sb.spec.Name + "\n \n-This is your Wack Club Orchard greenhouse.\n+This is your Wack Club Orchard greenhouse, now with edits.\n"
	case "log":
		return "a1b2c3d Initial commit\n"
	default:
		return "ok: git " + strings.Join(args, " ") + "\n"
	}
}

// ---- cluster ----

func (s *Sim) EnsureNamespace(ctx context.Context, ns string, labels map[string]string) error {
	return nil
}
func (s *Sim) DeleteNamespace(ctx context.Context, ns string) error { return nil }

func (s *Sim) Nodes(ctx context.Context) ([]Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Node, len(s.nodes))
	copy(out, s.nodes)
	counts := map[string]int{}
	cpu := map[string]int{}
	mem := map[string]int{}
	for _, a := range s.apps {
		for _, p := range a.pods {
			counts[p.Node]++
			cpu[p.Node] += a.spec.Resources.CPUMillis
			mem[p.Node] += a.spec.Resources.MemoryMi
		}
	}
	for i := range out {
		out[i].Pods = counts[out[i].Name] + 9
		out[i].UsedCPU = cpu[out[i].Name] + 850
		out[i].UsedMem = mem[out[i].Name] + 1800
	}
	return out, nil
}

func (s *Sim) SetNodePool(ctx context.Context, node, pool string, taint bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.nodes {
		if s.nodes[i].Name == node {
			if s.nodes[i].Labels == nil {
				s.nodes[i].Labels = map[string]string{}
			}
			s.nodes[i].Taints = nil
			if pool == "" {
				delete(s.nodes[i].Labels, "wackcluborchard.dev/pool")
			} else {
				s.nodes[i].Labels["wackcluborchard.dev/pool"] = pool
				if taint {
					s.nodes[i].Taints = []string{"wackcluborchard.dev/pool=" + pool + ":NoSchedule"}
				}
			}
			return nil
		}
	}
	return fmt.Errorf("node %s not found", node)
}

func (s *Sim) tick(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.mu.Lock()
			type st struct {
				id    string
				size  int64
				conns int
			}
			var up []st
			for _, d := range s.dbs {
				if d.spec.Stopped {
					continue
				}
				d.size += rand.Int63n(4096)
				up = append(up, st{d.spec.ID, d.size, rand.Intn(4)})
			}
			s.mu.Unlock()
			for _, u := range up {
				s.obs.DatabaseState(u.id, "ready", u.size, u.conns)
			}
		}
	}
}
