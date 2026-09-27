package platform

import (
	"context"
	"strings"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

var Backgrounds = []string{"clouds", "dusk", "meadow", "wackcluborchard", "night", "sunrise", "ocean", "lavender"}

func DefaultQuota() store.Quota {
	return store.Quota{CPUMillis: 16000, MemoryMi: 32768, StorageGi: 200, Apps: 50, Databases: 10, Sandboxes: 5}
}

func (p *Platform) CreateOrg(user *store.User, name, slug string) (*store.Org, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, Invalid("organization name is required")
	}
	if slug == "" {
		slug = store.Slugify(name)
	}
	slug = store.Slugify(slug)
	var org *store.Org
	err := p.Store.Write(func(d *store.Data) error {
		if d.OrgBySlug(slug) != nil {
			return Invalid("the slug %q is taken", slug)
		}
		now := time.Now()
		org = &store.Org{ID: store.NewID("org"), Slug: slug, Name: name, Quota: DefaultQuota(),
			MemberQuota: store.Quota{CPUMillis: 4000, MemoryMi: 8192, StorageGi: 50, Apps: 15, Databases: 4, Sandboxes: 2},
			Defaults:    store.Resources{CPUMillis: 250, MemoryMi: 256}, CreatedAt: now}
		d.Orgs[org.ID] = org
		if user != nil {
			m := &store.Membership{ID: store.NewID("mem"), OrgID: org.ID, UserID: user.ID, Role: store.RoleOwner, CreatedAt: now}
			d.Memberships[m.ID] = m
		}
		org = store.Clone(org)
		return nil
	})
	return org, err
}

func (p *Platform) CreateProject(user *store.User, orgID, name, icon, background string) (*store.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, Invalid("project name is required")
	}
	if icon == "" {
		icon = "🌱"
	}
	if background == "" {
		background = "clouds"
	}
	var pr *store.Project
	err := p.Store.Write(func(d *store.Data) error {
		if d.Orgs[orgID] == nil {
			return Invalid("organization not found")
		}
		slug := store.Slugify(name)
		base := slug
		for i := 2; ; i++ {
			taken := false
			for _, x := range d.Projects {
				if x.OrgID == orgID && x.Slug == slug {
					taken = true
				}
			}
			if !taken {
				break
			}
			slug = base + "-" + string(rune('0'+i%10))
		}
		now := time.Now()
		pr = &store.Project{ID: store.NewID("prj"), OrgID: orgID, Name: name, Slug: slug, Icon: icon, Background: background,
			Environments: []store.Environment{{ID: store.NewID("env"), Name: "Production"}},
			CreatedAt:    now, UpdatedAt: now}
		if user != nil {
			pr.CreatedBy = user.ID
			pr.Members = []string{user.ID}
		}
		d.Projects[pr.ID] = pr
		pr = store.Clone(pr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	ns := ""
	p.Store.Read(func(d *store.Data) { ns = d.Namespace(pr.ID) })
	p.queue.enqueue("ns:"+ns, func(ctx context.Context) error {
		return p.Driver.EnsureNamespace(ctx, ns, nil)
	})
	return pr, nil
}

func (p *Platform) DeleteProject(user *store.User, projectID string) error {
	var apps, dbs, jobs, tpls []string
	var ns string
	p.Store.Read(func(d *store.Data) {
		ns = d.Namespace(projectID)
		for _, a := range d.Apps {
			if a.ProjectID == projectID {
				apps = append(apps, a.ID)
			}
		}
		for _, x := range d.Databases {
			if x.ProjectID == projectID {
				dbs = append(dbs, x.ID)
			}
		}
		for _, x := range d.Jobs {
			if x.ProjectID == projectID {
				jobs = append(jobs, x.ID)
			}
		}
		for _, x := range d.TemplateInstances {
			if x.ProjectID == projectID {
				tpls = append(tpls, x.ID)
			}
		}
	})
	if ns == "" {
		return Invalid("project not found")
	}
	for _, id := range apps {
		p.DeleteApp(user, id)
	}
	for _, id := range dbs {
		p.DeleteDatabase(id)
	}
	for _, id := range jobs {
		p.DeleteJob(id)
	}
	p.Store.Write(func(d *store.Data) error {
		for _, id := range tpls {
			delete(d.TemplateInstances, id)
		}
		for k, v := range d.Variables {
			if v.ProjectID == projectID {
				delete(d.Variables, k)
			}
		}
		for k, c := range d.Compose {
			if c.ProjectID == projectID {
				delete(d.Compose, k)
			}
		}
		delete(d.Projects, projectID)
		return nil
	})
	p.queue.enqueue("ns:"+ns, func(ctx context.Context) error { return p.Driver.DeleteNamespace(ctx, ns) })
	return nil
}

// SeedDemo fills an empty instance with the "Homelab" project from the
// screenshots so a fresh dev server has something to look at.
func (p *Platform) SeedDemo(user *store.User, org *store.Org) {
	pr, err := p.CreateProject(user, org.ID, "Homelab", "🌱", "clouds")
	if err != nil {
		return
	}
	three := 3
	one := 1
	site, _ := p.CreateApp(user, pr.ID, AppInput{Name: "site", Source: store.Source{Type: "image", Image: "nginx:alpine"}, Replicas: &one,
		Ports: []store.Port{{Name: "http", Port: 80, Protocol: "http"}}, Resources: &store.Resources{CPUMillis: 100, MemoryMi: 64},
		Health: &store.HealthCheck{Enabled: true, Path: "/", Initial: 2}, Deploy: true})
	p.CreateApp(user, pr.ID, AppInput{Name: "cache", Source: store.Source{Type: "image", Image: "redis:7-alpine"}, Replicas: &one,
		Ports: []store.Port{{Name: "redis", Port: 6379, Protocol: "tcp"}}, Resources: &store.Resources{CPUMillis: 100, MemoryMi: 128}, Deploy: true})
	p.CreateApp(user, pr.ID, AppInput{Name: "api", Source: store.Source{Type: "image", Image: "traefik/whoami"}, Replicas: &three,
		Ports: []store.Port{{Name: "http", Port: 80, Protocol: "http"}}, Resources: &store.Resources{CPUMillis: 50, MemoryMi: 32},
		Env: map[string]string{"DATABASE_URL": "${{ api-db.DATABASE_URL }}", "REDIS_URL": "redis://${{ cache.HOST }}:6379"}, Deploy: true})
	p.CreateDatabase(user, pr.ID, DatabaseInput{Name: "api-db"})
	p.CreateDatabase(user, pr.ID, DatabaseInput{Name: "postgres"})
	p.Store.Write(func(d *store.Data) error {
		id := store.NewID("var")
		d.Variables[id] = &store.Variable{ID: id, ProjectID: pr.ID, Key: "LOG_LEVEL", Value: "info", UpdatedAt: time.Now()}
		id = store.NewID("var")
		d.Variables[id] = &store.Variable{ID: id, ProjectID: pr.ID, Key: "SESSION_SECRET", Value: RandHex(16), Secret: true, UpdatedAt: time.Now()}
		return nil
	})
	if site != nil {
		job, err := p.CreateJob(user, pr.ID, JobInput{Name: "warm-site", Schedule: "*/30 * * * *", Concurrency: "skip", Steps: []store.JobStep{{
			Name: "hit the site", Type: "script", Lang: "bash",
			Source: "apt-get update -qq && apt-get install -y -qq curl >/dev/null\nfor i in $(seq 150); do curl -s -o /dev/null http://site/; done\necho \"sent 150 requests to http://site/\"",
		}}})
		if err == nil {
			p.TriggerJob(user, job.ID, "manual")
		}
	}
	p.CreateJob(user, pr.ID, JobInput{Name: "nightly-report", Schedule: "0 6 * * *", Concurrency: "queue", Steps: []store.JobStep{
		{Name: "collect", Type: "script", Lang: "python", Source: "import datetime\nprint(\"collecting stats for\", datetime.date.today())\nprint(\"12 signups, 3 new projects\")"},
		{Name: "publish", Type: "script", Lang: "node", Source: "console.log('report published to #wackcluborchard-stats')"},
	}})
	for _, t := range []struct{ tpl, name string }{{"node-postgres", "api"}, {"redis", "cache"}, {"static-site", "site"}} {
		p.Store.Write(func(d *store.Data) error {
			inst := &store.TemplateInstance{ID: store.NewID("tpl"), ProjectID: pr.ID, EnvID: pr.Environments[0].ID, TemplateID: t.tpl, Name: t.name, Label: TemplateByID(t.tpl).Label, Status: "active", CreatedBy: user.ID, CreatedAt: time.Now()}
			d.TemplateInstances[inst.ID] = inst
			for _, a := range d.Apps {
				if a.ProjectID == pr.ID && a.Name == t.name {
					a.TemplateID = inst.ID
				}
			}
			return nil
		})
	}
}
