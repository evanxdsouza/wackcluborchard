package platform

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/events"
	"github.com/evanxdsouza/wackcluborchard/internal/runtime"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

var nameRe = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,38}[a-z0-9])?$`)

func ValidName(n string) error {
	if !nameRe.MatchString(n) {
		return Invalid("names are lowercase letters, digits and dashes, start with a letter, and are at most 40 characters")
	}
	return nil
}

type AppInput struct {
	Name      string             `json:"name"`
	EnvID     string             `json:"envId"`
	Source    store.Source       `json:"source"`
	Replicas  *int               `json:"replicas"`
	Ports     []store.Port       `json:"ports"`
	Resources *store.Resources   `json:"resources"`
	Volumes   []store.Volume     `json:"volumes"`
	Health    *store.HealthCheck `json:"health"`
	AuthWall  *bool              `json:"authWall"`
	Command   *string            `json:"command"`
	Env       map[string]string  `json:"env"` // app-scoped variables to create
	Secrets   map[string]string  `json:"secrets"`
	Deploy    bool               `json:"deploy"`
}

// usage totals requested resources for an org, optionally for one member.
type Usage struct {
	CPUMillis int `json:"cpuMillis"`
	MemoryMi  int `json:"memoryMi"`
	StorageGi int `json:"storageGi"`
	Apps      int `json:"apps"`
	Databases int `json:"databases"`
	Sandboxes int `json:"sandboxes"`
}

func OrgUsage(d *store.Data, orgID, userID string) Usage {
	var u Usage
	for _, a := range d.Apps {
		pr := d.Projects[a.ProjectID]
		if pr == nil || pr.OrgID != orgID || (userID != "" && a.CreatedBy != userID) {
			continue
		}
		u.Apps++
		u.CPUMillis += a.Resources.CPUMillis * a.Replicas
		u.MemoryMi += a.Resources.MemoryMi * a.Replicas
		for _, v := range a.Volumes {
			u.StorageGi += v.SizeGi
		}
	}
	for _, db := range d.Databases {
		pr := d.Projects[db.ProjectID]
		if pr == nil || pr.OrgID != orgID || (userID != "" && db.CreatedBy != userID) {
			continue
		}
		u.Databases++
		n := max(db.Instances, 1)
		if db.Status != "stopped" {
			u.CPUMillis += db.Resources.CPUMillis * n
			u.MemoryMi += db.Resources.MemoryMi * n
		}
		u.StorageGi += db.StorageGi * n
	}
	for _, sb := range d.Sandboxes {
		if sb.OrgID != orgID || (userID != "" && sb.OwnerID != userID) {
			continue
		}
		u.Sandboxes++
		if sb.Status != "stopped" {
			u.CPUMillis += sb.Resources.CPUMillis
			u.MemoryMi += sb.Resources.MemoryMi
		}
		u.StorageGi += sb.StorageGi
	}
	return u
}

// checkQuota verifies adding delta keeps both the org cap and the member
// allowance. Zero in a quota means unlimited.
func checkQuota(d *store.Data, orgID, userID string, delta Usage) error {
	o := d.Orgs[orgID]
	if o == nil {
		return Invalid("organization not found")
	}
	check := func(q store.Quota, u Usage, who string) error {
		type lim struct {
			name     string
			cap, use int
		}
		for _, l := range []lim{
			{"CPU (millicores)", q.CPUMillis, u.CPUMillis + delta.CPUMillis},
			{"memory (MiB)", q.MemoryMi, u.MemoryMi + delta.MemoryMi},
			{"storage (GiB)", q.StorageGi, u.StorageGi + delta.StorageGi},
			{"apps", q.Apps, u.Apps + delta.Apps},
			{"databases", q.Databases, u.Databases + delta.Databases},
			{"sandboxes", q.Sandboxes, u.Sandboxes + delta.Sandboxes},
		} {
			if l.cap > 0 && l.use > l.cap {
				return &Error{Kind: ErrQuota, Msg: fmt.Sprintf("This would use %d %s against %s limit of %d.", l.use, l.name, who, l.cap)}
			}
		}
		return nil
	}
	if err := check(o.Quota, OrgUsage(d, orgID, ""), "the organization's"); err != nil {
		return err
	}
	if userID != "" {
		if m := d.Membership(orgID, userID); m != nil && store.RoleRank(m.Role) < store.RoleRank(store.RoleAdmin) {
			if err := check(o.MemberQuota, OrgUsage(d, orgID, userID), "your"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Platform) generatedHost(d *store.Data, projectID, name string) string {
	dom := d.Settings.AppDomain
	if dom == "" {
		dom = "apps.localhost"
	}
	host := name + "." + dom
	taken := func(h string) bool {
		for _, a := range d.Apps {
			for _, x := range a.Domains {
				if x.Host == h {
					return true
				}
			}
		}
		return false
	}
	if taken(host) {
		if pr := d.Projects[projectID]; pr != nil {
			host = name + "-" + pr.Slug + "." + dom
		}
	}
	for i := 2; taken(host); i++ {
		host = fmt.Sprintf("%s-%d.%s", name, i, dom)
	}
	return host
}

func (p *Platform) CreateApp(user *store.User, projectID string, in AppInput) (*store.App, error) {
	in.Name = strings.ToLower(strings.TrimSpace(in.Name))
	if err := ValidName(in.Name); err != nil {
		return nil, err
	}
	switch in.Source.Type {
	case "image":
		if strings.TrimSpace(in.Source.Image) == "" {
			return nil, Invalid("an image is required")
		}
	case "github":
		if in.Source.Repo == "" {
			return nil, Invalid("pick a repository")
		}
		if in.Source.Branch == "" {
			in.Source.Branch = "main"
		}
		in.Source.AutoDeploy = true
	default:
		return nil, Invalid("source type must be image or github")
	}
	var app *store.App
	err := p.Store.Write(func(d *store.Data) error {
		pr := d.Projects[projectID]
		if pr == nil {
			return Invalid("project not found")
		}
		for _, a := range d.Apps {
			if a.ProjectID == projectID && a.Name == in.Name {
				return Invalid("an app named %q already exists in this project", in.Name)
			}
		}
		org := d.Orgs[pr.OrgID]
		res := org.Defaults
		if in.Resources != nil {
			res = *in.Resources
		}
		if res.CPUMillis == 0 {
			res.CPUMillis = 250
		}
		if res.MemoryMi == 0 {
			res.MemoryMi = 256
		}
		replicas := 1
		if in.Replicas != nil {
			replicas = *in.Replicas
		}
		storage := 0
		for _, v := range in.Volumes {
			storage += v.SizeGi
		}
		if err := checkQuota(d, pr.OrgID, user.ID, Usage{Apps: 1, CPUMillis: res.CPUMillis * replicas, MemoryMi: res.MemoryMi * replicas, StorageGi: storage}); err != nil {
			return err
		}
		envID := in.EnvID
		if envID == "" && len(pr.Environments) > 0 {
			envID = pr.Environments[0].ID
		}
		ports := in.Ports
		if ports == nil {
			ports = []store.Port{{Name: "http", Port: 8080, Protocol: "http"}}
		}
		now := time.Now()
		app = &store.App{
			ID: store.NewID("app"), ProjectID: projectID, EnvID: envID, Name: in.Name,
			Source: in.Source, Replicas: replicas, Ports: ports, Resources: res,
			Volumes: in.Volumes, Status: "pending", CreatedBy: user.ID, CreatedAt: now, UpdatedAt: now,
			Sandboxed: d.Settings.TenantSandbox,
		}
		if in.Health != nil {
			app.Health = *in.Health
		}
		if in.AuthWall != nil {
			app.AuthWall = *in.AuthWall
		}
		if in.Command != nil {
			app.Command = *in.Command
		}
		for _, pt := range ports {
			if pt.Protocol == "http" || pt.Protocol == "" {
				app.Domains = []store.Domain{{Host: p.generatedHost(d, projectID, in.Name), Generated: true, CertState: "issued", AddedAt: now}}
				break
			}
		}
		d.Apps[app.ID] = app
		for k, v := range in.Env {
			id := store.NewID("var")
			d.Variables[id] = &store.Variable{ID: id, ProjectID: projectID, AppID: app.ID, Key: k, Value: v, UpdatedAt: now}
		}
		for k, v := range in.Secrets {
			id := store.NewID("var")
			d.Variables[id] = &store.Variable{ID: id, ProjectID: projectID, AppID: app.ID, Key: k, Value: v, Secret: true, UpdatedAt: now}
		}
		pr.UpdatedAt = now
		app = store.Clone(app)
		return nil
	})
	if err != nil {
		return nil, err
	}
	p.publishApp(app)
	if in.Deploy {
		if _, err := p.Deploy(user, app.ID, DeployOpts{Trigger: "manual"}); err != nil {
			return app, err
		}
	}
	return app, nil
}

type AppPatch struct {
	Name      *string            `json:"name"`
	Source    *store.Source      `json:"source"`
	Replicas  *int               `json:"replicas"`
	Ports     []store.Port       `json:"ports"`
	Resources *store.Resources   `json:"resources"`
	Volumes   []store.Volume     `json:"volumes"`
	Health    *store.HealthCheck `json:"health"`
	AuthWall  *bool              `json:"authWall"`
	Command   *string            `json:"command"`
	Sandboxed *bool              `json:"sandboxed"`
}

// UpdateApp applies a patch and reconciles. Changing the source does not
// deploy by itself.
func (p *Platform) UpdateApp(user *store.User, appID string, patch AppPatch) (*store.App, error) {
	var app *store.App
	err := p.Store.Write(func(d *store.Data) error {
		a := d.Apps[appID]
		if a == nil {
			return Invalid("app not found")
		}
		pr := d.Projects[a.ProjectID]
		oldCPU, oldMem := a.Resources.CPUMillis*a.Replicas, a.Resources.MemoryMi*a.Replicas
		res, reps := a.Resources, a.Replicas
		if patch.Resources != nil {
			if patch.Resources.CPUMillis < 10 || patch.Resources.MemoryMi < 16 {
				return Invalid("resources need at least 10m CPU and 16Mi memory; send every field you want to keep")
			}
			res = *patch.Resources
		}
		if patch.Replicas != nil {
			if *patch.Replicas < 0 || *patch.Replicas > 50 {
				return Invalid("replicas must be between 0 and 50")
			}
			reps = *patch.Replicas
		}
		oldStorage, newStorage := 0, 0
		for _, v := range a.Volumes {
			oldStorage += v.SizeGi
		}
		vols := a.Volumes
		if patch.Volumes != nil {
			vols = patch.Volumes
		}
		for _, v := range vols {
			if v.Name == "" || !strings.HasPrefix(v.MountPath, "/") {
				return Invalid("each volume needs a name and an absolute mount path")
			}
			newStorage += v.SizeGi
		}
		if err := checkQuota(d, pr.OrgID, user.ID, Usage{CPUMillis: res.CPUMillis*reps - oldCPU, MemoryMi: res.MemoryMi*reps - oldMem, StorageGi: newStorage - oldStorage}); err != nil {
			return err
		}
		if patch.Name != nil && *patch.Name != a.Name {
			return Invalid("apps cannot be renamed; create a new one")
		}
		a.Resources, a.Replicas, a.Volumes = res, reps, vols
		if patch.Source != nil {
			a.Source = *patch.Source
		}
		if patch.Ports != nil {
			for _, pt := range patch.Ports {
				if pt.Port < 1 || pt.Port > 65535 {
					return Invalid("port %d is out of range", pt.Port)
				}
			}
			a.Ports = patch.Ports
			hasHTTP := false
			for _, pt := range a.Ports {
				if pt.Protocol == "http" {
					hasHTTP = true
				}
			}
			hasGen := false
			for _, dm := range a.Domains {
				if dm.Generated {
					hasGen = true
				}
			}
			if hasHTTP && !hasGen {
				a.Domains = append([]store.Domain{{Host: p.generatedHost(d, a.ProjectID, a.Name), Generated: true, CertState: "issued", AddedAt: time.Now()}}, a.Domains...)
			}
		}
		if patch.Health != nil {
			a.Health = *patch.Health
		}
		if patch.AuthWall != nil {
			a.AuthWall = *patch.AuthWall
		}
		if patch.Command != nil {
			a.Command = *patch.Command
		}
		if patch.Sandboxed != nil {
			a.Sandboxed = *patch.Sandboxed
		}
		a.UpdatedAt = time.Now()
		a.Status, a.StatusMsg = computeStatus(d, a)
		app = store.Clone(a)
		return nil
	})
	if err != nil {
		return nil, err
	}
	p.publishApp(app)
	p.EnqueueApp(appID)
	return app, nil
}

// EnqueueApp schedules reconciliation of an app's desired state.
func (p *Platform) EnqueueApp(appID string) {
	p.queue.enqueue("app:"+appID, func(ctx context.Context) error {
		spec, ok, err := p.appSpec(appID, "")
		if err != nil || !ok {
			return err
		}
		if err := p.Driver.EnsureNamespace(ctx, spec.Namespace, map[string]string{"wackcluborchard.dev/org": spec.Org}); err != nil {
			return err
		}
		return p.Driver.ApplyApp(ctx, spec)
	})
}

// AppURL is the public URL for an app hostname, including the HTTPS port
// when the instance serves apps on a non-standard one (lan mode, k3d).
func AppURL(st store.Settings, host string) string {
	if st.HTTPSPort != 0 && st.HTTPSPort != 443 {
		return fmt.Sprintf("https://%s:%d", host, st.HTTPSPort)
	}
	return "https://" + host
}

var refRe = regexp.MustCompile(`\$\{\{\s*([a-zA-Z0-9_-]+)\.([A-Za-z0-9_]+)\s*\}\}`)

// ResolveVars returns the variables an app sees: project-wide values,
// overridden by environment-scoped ones, overridden by the app's own.
// ${{ name.KEY }} references resolve against databases and apps in the
// same project.
func ResolveVars(d *store.Data, a *store.App) (env, secrets map[string]string) {
	env, secrets = map[string]string{}, map[string]string{}
	type layered struct {
		v    *store.Variable
		rank int
	}
	best := map[string]layered{}
	for _, v := range d.Variables {
		if v.ProjectID != a.ProjectID {
			continue
		}
		if v.EnvID != "" && v.EnvID != a.EnvID {
			continue
		}
		if v.AppID != "" && v.AppID != a.ID {
			continue
		}
		rank := 0
		if v.EnvID != "" {
			rank = 1
		}
		if v.AppID != "" {
			rank = 2
		}
		if cur, ok := best[v.Key]; !ok || rank >= cur.rank {
			best[v.Key] = layered{v, rank}
		}
	}
	for k, l := range best {
		val := interpolate(d, a.ProjectID, l.v.Value)
		if l.v.Secret {
			secrets[k] = val
		} else {
			env[k] = val
		}
	}
	if _, ok := env["PORT"]; !ok {
		for _, pt := range a.Ports {
			if pt.Protocol == "http" {
				env["PORT"] = fmt.Sprint(pt.Port)
				break
			}
		}
	}
	env["WACKCLUBORCHARD_APP"] = a.Name
	for _, dm := range a.Domains {
		if dm.Generated {
			env["WACKCLUBORCHARD_URL"] = AppURL(d.Settings, dm.Host)
		}
	}
	return env, secrets
}

func interpolate(d *store.Data, projectID, v string) string {
	return refRe.ReplaceAllStringFunc(v, func(m string) string {
		sm := refRe.FindStringSubmatch(m)
		name, key := sm[1], strings.ToUpper(sm[2])
		for _, db := range d.Databases {
			if db.ProjectID == projectID && db.Name == name {
				switch key {
				case "DATABASE_URL", "URL", "URI":
					return DatabaseURI(db)
				case "HOST", "PGHOST":
					return db.Host
				case "PORT", "PGPORT":
					return fmt.Sprint(db.Port)
				case "USER", "PGUSER":
					return db.User
				case "PASSWORD", "PGPASSWORD":
					return db.Password
				case "DATABASE", "PGDATABASE", "NAME":
					return db.DBName
				}
			}
		}
		for _, a := range d.Apps {
			if a.ProjectID == projectID && a.Name == name {
				port := 80
				if len(a.Ports) > 0 {
					port = a.Ports[0].Port
				}
				switch key {
				case "HOST":
					return a.Name
				case "PORT":
					return fmt.Sprint(port)
				case "URL":
					return fmt.Sprintf("http://%s:%d", a.Name, port)
				case "PUBLIC_URL":
					for _, dm := range a.Domains {
						return AppURL(d.Settings, dm.Host)
					}
				}
			}
		}
		return m
	})
}

func (p *Platform) appSpec(appID, imageOverride string) (runtime.AppSpec, bool, error) {
	var spec runtime.AppSpec
	ok := false
	p.Store.Read(func(d *store.Data) {
		a := d.Apps[appID]
		if a == nil {
			return
		}
		img := a.Image
		if imageOverride != "" {
			img = imageOverride
		}
		if img == "" {
			return
		}
		pr := d.Projects[a.ProjectID]
		org := d.Orgs[pr.OrgID]
		env, secrets := ResolveVars(d, a)
		nonce := ""
		if a.Labels != nil {
			nonce = a.Labels["restart"]
		}
		spec = runtime.AppSpec{
			Namespace: d.Namespace(a.ProjectID), Name: a.Name, ID: a.ID, Org: org.Slug, Pool: org.Pool,
			Image: img, Replicas: a.Replicas, Command: a.Command, Ports: a.Ports,
			Env: env, Secrets: secrets, Resources: a.Resources, Volumes: a.Volumes,
			Domains: a.Domains, Health: a.Health, AuthWall: a.AuthWall,
			Sandboxed: a.Sandboxed, RestartNonce: nonce,
		}
		if a.Sandboxed {
			spec.RuntimeClass = "gvisor"
		}
		ok = true
	})
	return spec, ok, nil
}

// ---- deploys ----

type DeployOpts struct {
	Trigger string
	Commit  string
	Message string
	Image   string // deploy this image instead of the source's
	Kind    string // rollback
}

var buildSteps = []string{"Scheduling builder", "Building image", "Pushing image", "Creating app", "Done"}
var imageSteps = []string{"Resolving image", "Creating app", "Done"}

// Deploy records intent and returns; the build and rollout proceed in the
// background and stream their progress over the event bus.
func (p *Platform) Deploy(user *store.User, appID string, opts DeployOpts) (*DeployResult, error) {
	var dep *store.Deploy
	err := p.Store.Write(func(d *store.Data) error {
		a := d.Apps[appID]
		if a == nil {
			return Invalid("app not found")
		}
		num := 1
		for _, x := range d.Deploys {
			if x.AppID == appID && x.Number >= num {
				num = x.Number + 1
			}
		}
		kind := "image"
		image := opts.Image
		if image == "" {
			if a.Source.Type == "github" {
				kind = "build"
			} else {
				image = a.Source.Image
			}
		}
		if opts.Kind != "" {
			kind = opts.Kind
		}
		names := imageSteps
		if kind == "build" {
			names = buildSteps
		}
		steps := make([]store.Step, len(names))
		for i, n := range names {
			steps[i] = store.Step{Name: n, Status: "pending"}
		}
		dep = &store.Deploy{
			ID: store.NewID("dep"), AppID: appID, Number: num, Kind: kind, Image: image,
			Commit: opts.Commit, Message: opts.Message, Trigger: opts.Trigger,
			Status: "queued", Steps: steps, CreatedAt: time.Now(),
		}
		if user != nil {
			dep.CreatedBy = user.Username
		}
		d.Deploys[dep.ID] = dep
		a.CurrentDeploy = dep.ID
		a.Status, a.StatusMsg = computeStatus(d, a)
		dep = store.Clone(dep)
		return nil
	})
	if err != nil {
		return nil, err
	}
	p.refreshStatus(appID)
	p.Bus.Publish("app:"+appID, "deploy.updated", dep)
	res := &DeployResult{Deploy: dep}
	if dep.Kind == "build" {
		t, superseded := p.builds.submit(dep.ID, appID, func() { p.runBuild(dep.ID) })
		for _, s := range superseded {
			p.finishDeploy(s, "superseded", "superseded by a newer deploy")
		}
		res.Build = &t
	} else {
		go p.runImageDeploy(dep.ID)
	}
	return res, nil
}

type DeployResult struct {
	Deploy *store.Deploy `json:"deploy"`
	Build  *BuildTicket  `json:"build,omitempty"`
}

func (p *Platform) setStep(depID string, idx int, status string) {
	var snap *store.Deploy
	p.Store.Write(func(d *store.Data) error {
		dep := d.Deploys[depID]
		if dep == nil || idx >= len(dep.Steps) {
			return nil
		}
		now := time.Now()
		// completing a later step completes the earlier ones
		for i := 0; i < idx; i++ {
			if dep.Steps[i].Status == "running" || dep.Steps[i].Status == "pending" {
				dep.Steps[i].Status = "succeeded"
				if dep.Steps[i].StartedAt == nil {
					dep.Steps[i].StartedAt = &now
				}
				dep.Steps[i].FinishedAt = &now
			}
		}
		st := &dep.Steps[idx]
		if st.Status == status {
			return nil
		}
		st.Status = status
		if status == "running" {
			st.StartedAt = &now
			dep.Status = "running"
		} else {
			if st.StartedAt == nil {
				st.StartedAt = &now
			}
			st.FinishedAt = &now
		}
		snap = store.Clone(dep)
		return nil
	})
	if snap != nil {
		p.Bus.Publish("deploy:"+depID, "deploy.updated", snap)
		p.Bus.Publish("app:"+snap.AppID, "deploy.updated", snap)
		p.refreshStatus(snap.AppID)
	}
}

func (p *Platform) finishDeploy(depID, status, msg string) {
	var snap *store.Deploy
	p.Store.Write(func(d *store.Data) error {
		dep := d.Deploys[depID]
		if dep == nil {
			return nil
		}
		now := time.Now()
		dep.Status = status
		dep.Error = msg
		dep.FinishedAt = &now
		for i := range dep.Steps {
			switch {
			case status == "succeeded":
				if dep.Steps[i].StartedAt == nil {
					dep.Steps[i].StartedAt = &now
				}
				if dep.Steps[i].FinishedAt == nil {
					dep.Steps[i].FinishedAt = &now
				}
				dep.Steps[i].Status = "succeeded"
			case dep.Steps[i].Status == "running":
				dep.Steps[i].Status = "failed"
				dep.Steps[i].FinishedAt = &now
			case dep.Steps[i].Status == "pending":
				dep.Steps[i].Status = "skipped"
			}
		}
		snap = store.Clone(dep)
		return nil
	})
	if snap != nil {
		p.Logs.Append("deploy:"+depID, events.LogLine{Text: "::done::" + status})
		p.Bus.Publish("deploy:"+depID, "deploy.updated", snap)
		p.Bus.Publish("app:"+snap.AppID, "deploy.updated", snap)
		p.refreshStatus(snap.AppID)
	}
}

func (p *Platform) deployLog(depID, line string) {
	p.Logs.Append("deploy:"+depID, events.LogLine{Text: line})
}

func (p *Platform) runBuild(depID string) {
	defer p.builds.done(depID)
	var spec runtime.BuildSpec
	var appID string
	var failMsg string
	p.Store.Read(func(d *store.Data) {
		dep := d.Deploys[depID]
		if dep == nil || dep.Status == "superseded" {
			failMsg = "-"
			return
		}
		a := d.Apps[dep.AppID]
		if a == nil {
			failMsg = "app was deleted"
			return
		}
		appID = a.ID
		pr := d.Projects[a.ProjectID]
		org := d.Orgs[pr.OrgID]
		reg := p.Cfg.Registry
		if reg == "" {
			reg = "registry.wackcluborchard.svc.cluster.local:5000"
		}
		commit := dep.Commit
		tag := fmt.Sprintf("%d", dep.Number)
		if commit != "" {
			tag += "-" + commit[:min(7, len(commit))]
		}
		_, secrets := ResolveVars(d, a)
		env, _ := ResolveVars(d, a)
		pull := map[string]string{}
		for _, k := range []string{"DHI_USERNAME", "DHI_TOKEN", "DOCKER_AUTH_CONFIG"} {
			if v := secrets[k]; v != "" {
				pull[k] = v
			} else if v := env[k]; v != "" {
				pull[k] = v
			}
		}
		buildArgs := map[string]string{}
		for k, v := range env {
			if strings.HasPrefix(k, "BUILD_") {
				buildArgs[strings.TrimPrefix(k, "BUILD_")] = v
			}
		}
		token := ""
		if u := d.Users[a.CreatedBy]; u != nil {
			token = u.GitHubToken
		}
		cloneURL := "https://github.com/" + a.Source.Repo + ".git"
		if token != "" {
			cloneURL = "https://x-access-token:" + token + "@github.com/" + a.Source.Repo + ".git"
		}
		spec = runtime.BuildSpec{
			Namespace: d.Namespace(a.ProjectID), DeployID: depID, AppName: a.Name,
			Repo: a.Source.Repo, CloneURL: cloneURL, Branch: a.Source.Branch, Commit: commit,
			Dockerfile: a.Source.Dockerfile, Context: a.Source.Context, Target: a.Source.Target,
			Image:     fmt.Sprintf("%s/%s/%s/%s:%s", reg, org.Slug, pr.Slug, a.Name, tag),
			BuildArgs: buildArgs, PullSecrets: pull, Sandboxed: d.Settings.BuilderSandbox,
		}
	})
	if failMsg == "-" {
		return
	}
	if failMsg != "" {
		p.finishDeploy(depID, "failed", failMsg)
		return
	}
	if p.CloneToken != nil {
		tctx, tcancel := context.WithTimeout(p.ctx, 10*time.Second)
		if t := p.CloneToken(tctx, spec.Repo); t != "" {
			spec.CloneURL = "https://x-access-token:" + t + "@github.com/" + spec.Repo + ".git"
		}
		tcancel()
	}
	ctx, cancel := context.WithTimeout(p.ctx, time.Hour)
	defer cancel()
	p.setStep(depID, 0, "running")
	p.deployLog(depID, "Waiting for a builder with capacity…")
	if err := p.Driver.EnsureNamespace(ctx, spec.Namespace, nil); err != nil {
		p.deployLog(depID, "error: "+err.Error())
		p.finishDeploy(depID, "failed", err.Error())
		return
	}
	started := false
	err := p.Driver.Build(ctx, spec, func(line string) {
		switch line {
		case "::step::schedule":
			return
		case "::step::build":
			p.setStep(depID, 1, "running")
			started = true
			return
		case "::step::push":
			p.setStep(depID, 2, "running")
			return
		}
		if !started {
			p.setStep(depID, 1, "running")
			started = true
		}
		if strings.Contains(line, "exporting to image") {
			p.setStep(depID, 2, "running")
		}
		p.deployLog(depID, line)
	})
	if err != nil {
		p.deployLog(depID, "error: "+err.Error())
		p.deployLog(depID, "The previous version keeps running; nothing is torn down until a new image is ready.")
		p.finishDeploy(depID, "failed", err.Error())
		return
	}
	p.Store.Write(func(d *store.Data) error {
		if dep := d.Deploys[depID]; dep != nil {
			dep.Image = spec.Image
		}
		return nil
	})
	p.rollout(ctx, depID, appID, spec.Image, 3)
}

func (p *Platform) runImageDeploy(depID string) {
	var appID, image string
	p.Store.Read(func(d *store.Data) {
		if dep := d.Deploys[depID]; dep != nil {
			appID, image = dep.AppID, dep.Image
		}
	})
	if appID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(p.ctx, 20*time.Minute)
	defer cancel()
	p.setStep(depID, 0, "running")
	p.deployLog(depID, "Using image "+image)
	p.rollout(ctx, depID, appID, image, 1)
}

// rollout applies the new image and waits for health checks to pass.
func (p *Platform) rollout(ctx context.Context, depID, appID, image string, step int) {
	p.setStep(depID, step, "running")
	p.deployLog(depID, "Rolling out "+image)
	spec, ok, _ := p.appSpec(appID, image)
	if !ok {
		p.finishDeploy(depID, "failed", "app was deleted")
		return
	}
	if err := p.Driver.EnsureNamespace(ctx, spec.Namespace, map[string]string{"wackcluborchard.dev/org": spec.Org}); err != nil {
		p.finishDeploy(depID, "failed", err.Error())
		return
	}
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if err = p.Driver.ApplyApp(ctx, spec); err == nil {
			break
		}
		p.deployLog(depID, fmt.Sprintf("apply failed (attempt %d): %v", attempt+1, err))
		if !sleepCtx(ctx, time.Duration(1<<attempt)*time.Second) {
			break
		}
	}
	if err != nil {
		p.finishDeploy(depID, "failed", err.Error())
		return
	}
	now := time.Now()
	p.Store.Write(func(d *store.Data) error {
		if a := d.Apps[appID]; a != nil {
			a.Image = image
			a.DeployedAt = &now
		}
		return nil
	})
	if spec.Replicas == 0 {
		p.deployLog(depID, "Scaled to zero; nothing to wait for.")
		p.finishDeploy(depID, "succeeded", "")
		return
	}
	p.deployLog(depID, fmt.Sprintf("Waiting for %d replica(s) to pass health checks…", spec.Replicas))
	deadline := time.Now().Add(5 * time.Minute)
	lastReason := ""
	for time.Now().Before(deadline) {
		ready, reason := 0, ""
		p.Store.Read(func(d *store.Data) {
			if a := d.Apps[appID]; a != nil {
				for _, pod := range a.Pods {
					if pod.Ready && pod.Phase != "Terminating" {
						ready++
					}
					if pod.Reason != "" {
						reason = pod.Reason
					}
				}
			}
		})
		if ready >= spec.Replicas {
			p.deployLog(depID, fmt.Sprintf("%d/%d replicas ready.", ready, spec.Replicas))
			p.finishDeploy(depID, "succeeded", "")
			return
		}
		if reason != "" && reason != lastReason {
			p.deployLog(depID, "pod: "+reason)
			lastReason = reason
		}
		if reason == "CrashLoopBackOff" || reason == "ImagePullBackOff" || reason == "ErrImagePull" {
			p.finishDeploy(depID, "failed", plainReason(reason))
			return
		}
		if !sleepCtx(ctx, time.Second) {
			break
		}
	}
	p.finishDeploy(depID, "failed", "Timed out waiting for replicas to become ready. "+plainReason(lastReason))
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// Rollback deploys an older image through the same rollout and checks.
func (p *Platform) Rollback(user *store.User, appID, deployID string) (*DeployResult, error) {
	var img, commit string
	var num int
	p.Store.Read(func(d *store.Data) {
		if dep := d.Deploys[deployID]; dep != nil && dep.AppID == appID {
			img, commit, num = dep.Image, dep.Commit, dep.Number
		}
	})
	if img == "" {
		return nil, Invalid("that deploy has no image to roll back to")
	}
	return p.Deploy(user, appID, DeployOpts{Trigger: "manual", Image: img, Commit: commit, Kind: "rollback", Message: fmt.Sprintf("Rollback to #%d", num)})
}

// Restart recreates pods on the same image.
func (p *Platform) Restart(user *store.User, appID string) error {
	err := p.Store.Write(func(d *store.Data) error {
		a := d.Apps[appID]
		if a == nil {
			return Invalid("app not found")
		}
		if a.Image == "" {
			return Invalid("nothing is deployed yet")
		}
		if a.Labels == nil {
			a.Labels = map[string]string{}
		}
		a.Labels["restart"] = time.Now().UTC().Format(time.RFC3339Nano)
		return nil
	})
	if err != nil {
		return err
	}
	p.EnqueueApp(appID)
	return nil
}

func (p *Platform) DeleteApp(user *store.User, appID string) error {
	var ns, name string
	err := p.Store.Write(func(d *store.Data) error {
		a := d.Apps[appID]
		if a == nil {
			return Invalid("app not found")
		}
		ns, name = d.Namespace(a.ProjectID), a.Name
		delete(d.Apps, appID)
		for k, v := range d.Variables {
			if v.AppID == appID {
				delete(d.Variables, k)
			}
		}
		for k, dep := range d.Deploys {
			if dep.AppID == appID {
				delete(d.Deploys, k)
			}
		}
		for k, e := range d.Events {
			if e.OwnerID == appID {
				delete(d.Events, k)
			}
		}
		p.Bus.Publish("project:"+a.ProjectID, "app.deleted", map[string]string{"id": appID})
		return nil
	})
	if err != nil {
		return err
	}
	p.queue.enqueue("app:"+appID, func(ctx context.Context) error { return p.Driver.DeleteApp(ctx, ns, name) })
	p.Logs.Drop("app:" + appID)
	return nil
}

// Domains

var hostRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

func (p *Platform) AddDomain(appID, host string) (*store.App, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if !hostRe.MatchString(host) {
		return nil, Invalid("%q is not a valid hostname", host)
	}
	var app *store.App
	err := p.Store.Write(func(d *store.Data) error {
		for _, a := range d.Apps {
			for _, dm := range a.Domains {
				if dm.Host == host {
					return Invalid("%s is already attached to %s", host, a.Name)
				}
			}
		}
		a := d.Apps[appID]
		if a == nil {
			return Invalid("app not found")
		}
		a.Domains = append(a.Domains, store.Domain{Host: host, CertState: "pending", AddedAt: time.Now()})
		app = store.Clone(a)
		return nil
	})
	if err != nil {
		return nil, err
	}
	p.EnqueueApp(appID)
	go func() {
		// cert-manager issues over HTTP-01 once DNS points here
		time.Sleep(6 * time.Second)
		p.Store.Write(func(d *store.Data) error {
			if a := d.Apps[appID]; a != nil {
				for i := range a.Domains {
					if a.Domains[i].Host == host && a.Domains[i].CertState == "pending" {
						a.Domains[i].CertState = "issued"
					}
				}
			}
			return nil
		})
		p.refreshStatus(appID)
	}()
	p.publishApp(app)
	return app, nil
}

func (p *Platform) RemoveDomain(appID, host string) error {
	err := p.Store.Write(func(d *store.Data) error {
		a := d.Apps[appID]
		if a == nil {
			return Invalid("app not found")
		}
		var keep []store.Domain
		for _, dm := range a.Domains {
			if dm.Host != host {
				keep = append(keep, dm)
			}
		}
		a.Domains = keep
		return nil
	})
	if err == nil {
		p.EnqueueApp(appID)
		p.refreshStatus(appID)
	}
	return err
}
