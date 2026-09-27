package platform

import (
	"fmt"
	"strings"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/store"
	"github.com/evanxdsouza/wackcluborchard/internal/yaml"
)

// A Template is a ready-made stack from The Grove: one or more apps and
// databases wired together with variable references.
type Template struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Label       string        `json:"label"` // shown under instance names
	Description string        `json:"description"`
	Category    string        `json:"category"`
	Icon        string        `json:"icon"`
	Apps        []TemplateApp `json:"apps"`
	Databases   []string      `json:"databases"` // database names, suffixed with the instance name
}

type TemplateApp struct {
	Suffix    string            `json:"suffix"` // appended to the instance name ("" for the main app)
	Image     string            `json:"image"`
	Command   string            `json:"command,omitempty"`
	Ports     []store.Port      `json:"ports"`
	Env       map[string]string `json:"env,omitempty"`
	Secrets   []string          `json:"secrets,omitempty"` // generated random secrets
	Volumes   []store.Volume    `json:"volumes,omitempty"`
	Resources store.Resources   `json:"resources"`
	Health    string            `json:"health,omitempty"`
}

func httpPort(port int) []store.Port {
	return []store.Port{{Name: "http", Port: port, Protocol: "http"}}
}

var Templates = []Template{
	{ID: "static-site", Name: "Static Site", Label: "Static Site", Category: "Web", Icon: "globe",
		Description: "nginx serving a static site. Swap the image for your own build.",
		Apps:        []TemplateApp{{Image: "nginx:alpine", Ports: httpPort(80), Resources: store.Resources{CPUMillis: 100, MemoryMi: 64}, Health: "/"}}},
	{ID: "redis", Name: "Redis Cache", Label: "Redis Cache", Category: "Data", Icon: "zap",
		Description: "An in-memory Redis for caching and queues, reachable inside the project.",
		Apps:        []TemplateApp{{Image: "redis:7-alpine", Ports: []store.Port{{Name: "redis", Port: 6379, Protocol: "tcp"}}, Resources: store.Resources{CPUMillis: 100, MemoryMi: 128}}}},
	{ID: "node-postgres", Name: "Node.js + PostgreSQL", Label: "Node.js + PostgreSQL", Category: "Starters", Icon: "server",
		Description: "A Node.js API with a managed PostgreSQL wired in through DATABASE_URL.",
		Apps: []TemplateApp{{Image: "node:22-alpine", Ports: httpPort(3000),
			Command:   `node -e "require('http').createServer((q,s)=>{s.setHeader('content-type','application/json');s.end(JSON.stringify({hello:'wack club wackcluborchard',db:!!process.env.DATABASE_URL}))}).listen(process.env.PORT||3000)"`,
			Env:       map[string]string{"DATABASE_URL": "${{ {db}.DATABASE_URL }}", "NODE_ENV": "production"},
			Resources: store.Resources{CPUMillis: 250, MemoryMi: 256}, Health: "/"}},
		Databases: []string{"db"}},
	{ID: "whoami", Name: "whoami", Label: "Echo server", Category: "Web", Icon: "user",
		Description: "Traefik's tiny HTTP server that prints request details. Handy for testing routing.",
		Apps:        []TemplateApp{{Image: "traefik/whoami", Ports: httpPort(80), Resources: store.Resources{CPUMillis: 50, MemoryMi: 32}, Health: "/health"}}},
	{ID: "uptime-kuma", Name: "Uptime Kuma", Label: "Monitoring", Category: "Tools", Icon: "activity",
		Description: "Self-hosted uptime monitoring with a status page.",
		Apps:        []TemplateApp{{Image: "louislam/uptime-kuma:1", Ports: httpPort(3001), Volumes: []store.Volume{{Name: "data", MountPath: "/app/data", SizeGi: 2}}, Resources: store.Resources{CPUMillis: 250, MemoryMi: 256}}}},
	{ID: "umami", Name: "Umami", Label: "Analytics", Category: "Tools", Icon: "chart",
		Description: "Privacy-friendly web analytics backed by PostgreSQL.",
		Apps: []TemplateApp{{Image: "ghcr.io/umami-software/umami:postgresql-latest", Ports: httpPort(3000),
			Env: map[string]string{"DATABASE_URL": "${{ {db}.DATABASE_URL }}"}, Secrets: []string{"APP_SECRET"},
			Resources: store.Resources{CPUMillis: 250, MemoryMi: 512}}},
		Databases: []string{"db"}},
	{ID: "n8n", Name: "n8n", Label: "Automation", Category: "Tools", Icon: "workflow",
		Description: "Workflow automation with hundreds of integrations.",
		Apps: []TemplateApp{{Image: "n8nio/n8n", Ports: httpPort(5678), Volumes: []store.Volume{{Name: "data", MountPath: "/home/node/.n8n", SizeGi: 2}},
			Env:     map[string]string{"DB_TYPE": "postgresdb", "DB_POSTGRESDB_HOST": "${{ {db}.HOST }}", "DB_POSTGRESDB_DATABASE": "${{ {db}.DATABASE }}", "DB_POSTGRESDB_USER": "${{ {db}.USER }}", "DB_POSTGRESDB_PASSWORD": "${{ {db}.PASSWORD }}"},
			Secrets: []string{"N8N_ENCRYPTION_KEY"}, Resources: store.Resources{CPUMillis: 500, MemoryMi: 512}}},
		Databases: []string{"db"}},
	{ID: "vaultwarden", Name: "Vaultwarden", Label: "Passwords", Category: "Tools", Icon: "lock",
		Description: "A lightweight Bitwarden-compatible password manager.",
		Apps:        []TemplateApp{{Image: "vaultwarden/server:latest", Ports: httpPort(80), Volumes: []store.Volume{{Name: "data", MountPath: "/data", SizeGi: 1}}, Resources: store.Resources{CPUMillis: 100, MemoryMi: 128}, Health: "/alive"}}},
	{ID: "gitea", Name: "Gitea", Label: "Git hosting", Category: "Dev", Icon: "git",
		Description: "Painless self-hosted Git with issues, PRs and a container registry.",
		Apps: []TemplateApp{{Image: "gitea/gitea:1.22", Ports: []store.Port{{Name: "http", Port: 3000, Protocol: "http"}, {Name: "ssh", Port: 22, Protocol: "tcp"}},
			Env:     map[string]string{"GITEA__database__DB_TYPE": "postgres", "GITEA__database__HOST": "${{ {db}.HOST }}:5432", "GITEA__database__NAME": "${{ {db}.DATABASE }}", "GITEA__database__USER": "${{ {db}.USER }}", "GITEA__database__PASSWD": "${{ {db}.PASSWORD }}"},
			Volumes: []store.Volume{{Name: "data", MountPath: "/data", SizeGi: 5}}, Resources: store.Resources{CPUMillis: 500, MemoryMi: 512}}},
		Databases: []string{"db"}},
	{ID: "ghost", Name: "Ghost", Label: "Blog", Category: "Web", Icon: "pen",
		Description: "A publishing platform for blogs and newsletters.",
		Apps: []TemplateApp{{Image: "ghost:5-alpine", Ports: httpPort(2368), Env: map[string]string{"url": "${{ {self}.PUBLIC_URL }}", "database__client": "sqlite3", "database__connection__filename": "/var/lib/ghost/content/data/ghost.db"},
			Volumes: []store.Volume{{Name: "content", MountPath: "/var/lib/ghost/content", SizeGi: 2}}, Resources: store.Resources{CPUMillis: 250, MemoryMi: 384}}}},
	{ID: "excalidraw", Name: "Excalidraw", Label: "Whiteboard", Category: "Tools", Icon: "pen",
		Description: "A virtual whiteboard for sketching hand-drawn diagrams.",
		Apps:        []TemplateApp{{Image: "excalidraw/excalidraw:latest", Ports: httpPort(80), Resources: store.Resources{CPUMillis: 100, MemoryMi: 64}}}},
	{ID: "minio", Name: "MinIO", Label: "Object storage", Category: "Data", Icon: "box",
		Description: "S3-compatible object storage.",
		Apps: []TemplateApp{{Image: "minio/minio:latest", Command: "minio server /data --console-address :9001", Ports: []store.Port{{Name: "console", Port: 9001, Protocol: "http"}, {Name: "s3", Port: 9000, Protocol: "tcp"}},
			Env: map[string]string{"MINIO_ROOT_USER": "wackcluborchard"}, Secrets: []string{"MINIO_ROOT_PASSWORD"},
			Volumes: []store.Volume{{Name: "data", MountPath: "/data", SizeGi: 10}}, Resources: store.Resources{CPUMillis: 250, MemoryMi: 512}}}},
	{ID: "metabase", Name: "Metabase", Label: "BI", Category: "Data", Icon: "chart",
		Description: "Dashboards and questions over your data, stored in PostgreSQL.",
		Apps: []TemplateApp{{Image: "metabase/metabase:latest", Ports: httpPort(3000),
			Env:       map[string]string{"MB_DB_TYPE": "postgres", "MB_DB_CONNECTION_URI": "jdbc:postgresql://${{ {db}.HOST }}:5432/${{ {db}.DATABASE }}?user=${{ {db}.USER }}&password=${{ {db}.PASSWORD }}"},
			Resources: store.Resources{CPUMillis: 500, MemoryMi: 1024}}},
		Databases: []string{"db"}},
}

func TemplateByID(id string) *Template {
	for i := range Templates {
		if Templates[i].ID == id {
			return &Templates[i]
		}
	}
	return nil
}

// InstantiateTemplate creates everything a template describes and records
// a template instance grouping them.
func (p *Platform) InstantiateTemplate(user *store.User, projectID, envID, templateID, name string) (*store.TemplateInstance, error) {
	t := TemplateByID(templateID)
	if t == nil {
		return nil, Invalid("unknown template %q", templateID)
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = t.ID
	}
	if err := ValidName(name); err != nil {
		return nil, err
	}
	inst := &store.TemplateInstance{ID: store.NewID("tpl"), ProjectID: projectID, EnvID: envID, TemplateID: t.ID, Name: name, Label: t.Label, Status: "active", CreatedAt: time.Now()}
	if user != nil {
		inst.CreatedBy = user.ID
	}
	dbNames := map[string]string{}
	for _, dn := range t.Databases {
		full := name + "-" + dn
		db, err := p.CreateDatabase(user, projectID, DatabaseInput{Name: full, EnvID: envID})
		if err != nil {
			return nil, fmt.Errorf("database %s: %w", full, err)
		}
		p.Store.Write(func(d *store.Data) error {
			if x := d.Databases[db.ID]; x != nil {
				x.TemplateID = inst.ID
			}
			return nil
		})
		dbNames[dn] = full
	}
	for _, ta := range t.Apps {
		appName := name
		if ta.Suffix != "" {
			appName += "-" + ta.Suffix
		}
		env := map[string]string{}
		for k, v := range ta.Env {
			for dn, full := range dbNames {
				v = strings.ReplaceAll(v, "{"+dn+"}", full)
			}
			v = strings.ReplaceAll(v, "{self}", appName)
			env[k] = v
		}
		secrets := map[string]string{}
		for _, s := range ta.Secrets {
			secrets[s] = RandHex(24)
		}
		res := ta.Resources
		cmd := ta.Command
		in := AppInput{Name: appName, EnvID: envID, Source: store.Source{Type: "image", Image: ta.Image}, Ports: ta.Ports, Resources: &res, Volumes: ta.Volumes, Env: env, Secrets: secrets, Deploy: true}
		if cmd != "" {
			in.Command = &cmd
		}
		if ta.Health != "" {
			in.Health = &store.HealthCheck{Enabled: true, Path: ta.Health, Initial: 5}
		}
		a, err := p.CreateApp(user, projectID, in)
		if err != nil {
			return nil, fmt.Errorf("app %s: %w", appName, err)
		}
		p.Store.Write(func(d *store.Data) error {
			if x := d.Apps[a.ID]; x != nil {
				x.TemplateID = inst.ID
			}
			return nil
		})
	}
	p.Store.Write(func(d *store.Data) error {
		d.TemplateInstances[inst.ID] = inst
		return nil
	})
	p.Bus.Publish("project:"+projectID, "template.created", inst)
	return inst, nil
}

func (p *Platform) DeleteTemplateInstance(user *store.User, id string) error {
	var apps, dbs []string
	err := p.Store.Write(func(d *store.Data) error {
		if d.TemplateInstances[id] == nil {
			return Invalid("template instance not found")
		}
		for _, a := range d.Apps {
			if a.TemplateID == id {
				apps = append(apps, a.ID)
			}
		}
		for _, db := range d.Databases {
			if db.TemplateID == id {
				dbs = append(dbs, db.ID)
			}
		}
		delete(d.TemplateInstances, id)
		return nil
	})
	if err != nil {
		return err
	}
	for _, a := range apps {
		p.DeleteApp(user, a)
	}
	for _, db := range dbs {
		p.DeleteDatabase(db)
	}
	return nil
}

// ---- compose ----

type ComposeResult struct {
	Created  []string `json:"created"`
	Updated  []string `json:"updated"`
	Removed  []string `json:"removed"`
	Warnings []string `json:"warnings"`
}

// ApplyCompose turns a docker-compose file into apps in one environment.
// Services map to apps; named volumes become persistent volumes; ports
// become service ports. Re-applying reconciles: services that vanished
// from the file are removed.
func (p *Platform) ApplyCompose(user *store.User, projectID, envID, src string, dryRun bool) (*ComposeResult, error) {
	doc, err := yaml.Parse(src)
	if err != nil {
		return nil, Invalid("compose file: %v", err)
	}
	services := yaml.Map(yaml.Map(doc)["services"])
	if len(services) == 0 {
		return nil, Invalid("compose file has no services")
	}
	res := &ComposeResult{}
	type planned struct {
		name string
		in   AppInput
	}
	var plan []planned
	for svcName, raw := range services {
		svc := yaml.Map(raw)
		name := store.Slugify(svcName)
		if svc["build"] != nil && svc["image"] == nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: build: is not supported in compose; create it from a GitHub repo instead", svcName))
			continue
		}
		in := AppInput{Name: name, EnvID: envID, Source: store.Source{Type: "image", Image: yaml.String(svc["image"])}, Env: map[string]string{}, Deploy: true}
		switch env := svc["environment"].(type) {
		case map[string]any:
			for k, v := range env {
				in.Env[k] = yaml.String(v)
			}
		case []any:
			for _, e := range env {
				kv := strings.SplitN(yaml.String(e), "=", 2)
				if len(kv) == 2 {
					in.Env[kv[0]] = kv[1]
				} else {
					in.Env[kv[0]] = ""
				}
			}
		}
		var ports []store.Port
		for _, pr := range yaml.List(svc["ports"]) {
			spec := yaml.String(pr)
			proto := "http"
			if strings.HasSuffix(spec, "/udp") {
				proto = "udp"
				spec = strings.TrimSuffix(spec, "/udp")
			}
			spec = strings.TrimSuffix(spec, "/tcp")
			parts := strings.Split(spec, ":")
			var port int
			fmt.Sscan(parts[len(parts)-1], &port)
			if port > 0 {
				if len(ports) > 0 && proto == "http" {
					proto = "tcp"
				}
				ports = append(ports, store.Port{Name: fmt.Sprintf("p%d", port), Port: port, Protocol: proto})
			}
		}
		for _, e := range yaml.List(svc["expose"]) {
			var port int
			fmt.Sscan(yaml.String(e), &port)
			if port > 0 {
				ports = append(ports, store.Port{Name: fmt.Sprintf("p%d", port), Port: port, Protocol: "tcp"})
			}
		}
		if ports == nil {
			ports = []store.Port{}
		}
		in.Ports = ports
		for _, v := range yaml.List(svc["volumes"]) {
			parts := strings.Split(yaml.String(v), ":")
			if len(parts) >= 2 && !strings.HasPrefix(parts[0], ".") && !strings.HasPrefix(parts[0], "/") {
				in.Volumes = append(in.Volumes, store.Volume{Name: store.Slugify(parts[0]), MountPath: parts[1], SizeGi: 1})
			} else if len(parts) >= 2 {
				res.Warnings = append(res.Warnings, fmt.Sprintf("%s: bind mount %s skipped; only named volumes are supported", svcName, parts[0]))
			}
		}
		switch c := svc["command"].(type) {
		case string:
			cmd := c
			in.Command = &cmd
		case []any:
			cmd := strings.Join(yaml.Strings(c), " ")
			in.Command = &cmd
		}
		if rep, ok := yaml.Map(svc["deploy"])["replicas"].(int); ok {
			in.Replicas = &rep
		}
		plan = append(plan, planned{name, in})
	}
	var stack *store.ComposeStack
	existing := map[string]string{} // name -> app id
	p.Store.Read(func(d *store.Data) {
		for _, c := range d.Compose {
			if c.ProjectID == projectID && c.EnvID == envID {
				stack = store.Clone(c)
			}
		}
		if stack != nil {
			for _, id := range stack.AppIDs {
				if a := d.Apps[id]; a != nil {
					existing[a.Name] = id
				}
			}
		}
	})
	keep := map[string]bool{}
	var ids []string
	for _, pl := range plan {
		keep[pl.name] = true
		if id, ok := existing[pl.name]; ok {
			res.Updated = append(res.Updated, pl.name)
			if dryRun {
				continue
			}
			src := pl.in.Source
			reps := pl.in.Replicas
			_, err := p.UpdateApp(user, id, AppPatch{Source: &src, Ports: pl.in.Ports, Volumes: pl.in.Volumes, Command: pl.in.Command, Replicas: reps})
			if err != nil {
				return nil, fmt.Errorf("%s: %w", pl.name, err)
			}
			p.setAppVars(projectID, id, pl.in.Env)
			p.Deploy(user, id, DeployOpts{Trigger: "compose"})
			ids = append(ids, id)
			continue
		}
		res.Created = append(res.Created, pl.name)
		if dryRun {
			continue
		}
		a, err := p.CreateApp(user, projectID, pl.in)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", pl.name, err)
		}
		ids = append(ids, a.ID)
	}
	for name, id := range existing {
		if !keep[name] {
			res.Removed = append(res.Removed, name)
			if !dryRun {
				p.DeleteApp(user, id)
			}
		}
	}
	if dryRun {
		return res, nil
	}
	p.Store.Write(func(d *store.Data) error {
		if stack == nil {
			stack = &store.ComposeStack{ID: store.NewID("cmp"), ProjectID: projectID, EnvID: envID}
		}
		stack.Source = src
		stack.AppIDs = ids
		stack.UpdatedBy = user.Username
		stack.UpdatedAt = time.Now()
		d.Compose[stack.ID] = stack
		for _, id := range ids {
			if a := d.Apps[id]; a != nil {
				a.ComposeID = stack.ID
			}
		}
		return nil
	})
	return res, nil
}

func (p *Platform) setAppVars(projectID, appID string, env map[string]string) {
	p.Store.Write(func(d *store.Data) error {
		for k, v := range d.Variables {
			if v.AppID == appID && !v.Secret {
				delete(d.Variables, k)
			}
		}
		for k, v := range env {
			id := store.NewID("var")
			d.Variables[id] = &store.Variable{ID: id, ProjectID: projectID, AppID: appID, Key: k, Value: v, UpdatedAt: time.Now()}
		}
		return nil
	})
}
