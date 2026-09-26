package platform

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/cron"
	"github.com/evanxdsouza/wackcluborchard/internal/runtime"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

func DatabaseURI(db *store.Database) string {
	u := url.URL{Scheme: "postgresql", User: url.UserPassword(db.User, db.Password), Host: fmt.Sprintf("%s:%d", db.Host, db.Port), Path: "/" + db.DBName}
	return u.String()
}

// Extensions offered in the UI. CloudNativePG's images ship these.
var AvailableExtensions = []struct {
	Name, Description string
}{
	{"pgvector", "Vector similarity search for embeddings"},
	{"pg_stat_statements", "Track planning and execution statistics of SQL"},
	{"pgcrypto", "Cryptographic functions"},
	{"uuid-ossp", "Generate UUIDs"},
	{"citext", "Case-insensitive text type"},
	{"hstore", "Key/value pairs in a single column"},
	{"pg_trgm", "Trigram matching for fuzzy text search"},
	{"postgis", "Spatial and geographic objects"},
	{"btree_gin", "GIN operator classes for common types"},
	{"unaccent", "Text search dictionary that removes accents"},
	{"tablefunc", "Crosstab and other table functions"},
	{"ltree", "Hierarchical tree-like labels"},
}

type DatabaseInput struct {
	Name      string          `json:"name"`
	EnvID     string          `json:"envId"`
	Version   int             `json:"version"`
	StorageGi int             `json:"storageGi"`
	Resources store.Resources `json:"resources"`
	Mode      string          `json:"mode"`
}

func (p *Platform) CreateDatabase(user *store.User, projectID string, in DatabaseInput) (*store.Database, error) {
	in.Name = strings.ToLower(strings.TrimSpace(in.Name))
	if err := ValidName(in.Name); err != nil {
		return nil, err
	}
	if in.Version == 0 {
		in.Version = 17
	}
	if in.Version < 13 || in.Version > 18 {
		return nil, Invalid("PostgreSQL %d is not offered; pick 13 to 18", in.Version)
	}
	if in.StorageGi <= 0 {
		in.StorageGi = 10
	}
	if in.Resources.CPUMillis == 0 {
		in.Resources.CPUMillis = 500
	}
	if in.Resources.MemoryMi == 0 {
		in.Resources.MemoryMi = 512
	}
	if in.Mode == "" {
		in.Mode = "shared"
	}
	var db *store.Database
	err := p.Store.Write(func(d *store.Data) error {
		pr := d.Projects[projectID]
		if pr == nil {
			return Invalid("project not found")
		}
		for _, x := range d.Databases {
			if x.ProjectID == projectID && x.Name == in.Name {
				return Invalid("a database named %q already exists in this project", in.Name)
			}
		}
		if err := checkQuota(d, pr.OrgID, user.ID, Usage{Databases: 1, CPUMillis: in.Resources.CPUMillis, MemoryMi: in.Resources.MemoryMi, StorageGi: in.StorageGi}); err != nil {
			return err
		}
		org := d.Orgs[pr.OrgID]
		ident := strings.ReplaceAll(fmt.Sprintf("orchard_%s_%s_%s", org.Slug, pr.Slug, in.Name), "-", "_")
		if len(ident) > 60 {
			ident = ident[:60]
		}
		ns := d.Namespace(projectID)
		envID := in.EnvID
		if envID == "" && len(pr.Environments) > 0 {
			envID = pr.Environments[0].ID
		}
		now := time.Now()
		db = &store.Database{
			ID: store.NewID("db"), ProjectID: projectID, EnvID: envID, Name: in.Name,
			Version: in.Version, StorageGi: in.StorageGi, Resources: in.Resources, Mode: in.Mode,
			DBName: ident, User: "u_" + ident, Password: randString(24),
			Host: fmt.Sprintf("pg-%s-rw.%s.svc.cluster.local", in.Name, ns), Port: 5432,
			Status: "provisioning", Instances: 1, BackupCron: "0 3 * * *",
			CreatedBy: user.ID, CreatedAt: now, UpdatedAt: now,
		}
		d.Databases[db.ID] = db
		pr.UpdatedAt = now
		db = store.Clone(db)
		return nil
	})
	if err != nil {
		return nil, err
	}
	p.publishDB(db)
	p.EnqueueDatabase(db.ID)
	return db, nil
}

func (p *Platform) dbSpec(id string) (runtime.DatabaseSpec, bool) {
	var s runtime.DatabaseSpec
	ok := false
	p.Store.Read(func(d *store.Data) {
		db := d.Databases[id]
		if db == nil {
			return
		}
		s = runtime.DatabaseSpec{
			Namespace: d.Namespace(db.ProjectID), Name: db.Name, ID: db.ID, Version: db.Version,
			Instances: db.Instances, StorageGi: db.StorageGi, Resources: db.Resources,
			DBName: db.DBName, User: db.User, Password: db.Password, Extensions: db.Extensions,
			BackupCron: db.BackupCron, PublicPort: db.PublicPort, Stopped: db.Status == "stopped",
		}
		ok = true
	})
	return s, ok
}

func (p *Platform) EnqueueDatabase(id string) {
	p.queue.enqueue("db:"+id, func(ctx context.Context) error {
		s, ok := p.dbSpec(id)
		if !ok {
			return nil
		}
		if err := p.Driver.EnsureNamespace(ctx, s.Namespace, nil); err != nil {
			return err
		}
		return p.Driver.ApplyDatabase(ctx, s)
	})
}

type DatabasePatch struct {
	Resources  *store.Resources `json:"resources"`
	StorageGi  *int             `json:"storageGi"`
	Instances  *int             `json:"instances"`
	Extensions []string         `json:"extensions"`
	BackupCron *string          `json:"backupSchedule"`
	Stopped    *bool            `json:"stopped"`
	Public     *bool            `json:"public"`
}

func (p *Platform) UpdateDatabase(user *store.User, id string, patch DatabasePatch) (*store.Database, error) {
	var db *store.Database
	var newExt []string
	err := p.Store.Write(func(d *store.Data) error {
		x := d.Databases[id]
		if x == nil {
			return Invalid("database not found")
		}
		pr := d.Projects[x.ProjectID]
		res, inst, sto := x.Resources, x.Instances, x.StorageGi
		if patch.Resources != nil {
			res = *patch.Resources
		}
		if patch.Instances != nil {
			if *patch.Instances < 1 || *patch.Instances > 5 {
				return Invalid("a database has 1 primary and up to 4 read replicas")
			}
			inst = *patch.Instances
		}
		if patch.StorageGi != nil {
			if *patch.StorageGi < x.StorageGi {
				return Invalid("volumes can grow but not shrink")
			}
			sto = *patch.StorageGi
		}
		delta := Usage{
			CPUMillis: res.CPUMillis*inst - x.Resources.CPUMillis*max(x.Instances, 1),
			MemoryMi:  res.MemoryMi*inst - x.Resources.MemoryMi*max(x.Instances, 1),
			StorageGi: sto*inst - x.StorageGi*max(x.Instances, 1),
		}
		if err := checkQuota(d, pr.OrgID, user.ID, delta); err != nil {
			return err
		}
		x.Resources, x.Instances, x.StorageGi = res, inst, sto
		if patch.Extensions != nil {
			have := map[string]bool{}
			for _, e := range x.Extensions {
				have[e] = true
			}
			for _, e := range patch.Extensions {
				if !have[e] {
					newExt = append(newExt, e)
				}
			}
			x.Extensions = patch.Extensions
		}
		if patch.BackupCron != nil {
			if *patch.BackupCron != "" {
				if _, err := cron.Parse(*patch.BackupCron); err != nil {
					return Invalid("%v", err)
				}
			}
			x.BackupCron = *patch.BackupCron
		}
		if patch.Stopped != nil {
			if *patch.Stopped {
				x.Status = "stopped"
			} else if x.Status == "stopped" {
				x.Status = "provisioning"
			}
		}
		if patch.Public != nil {
			if *patch.Public && x.PublicPort == 0 {
				org := d.Orgs[pr.OrgID]
				if org.PublicIP == "" && d.Settings.PublicIP == "" {
					return Invalid("no shared public IP is configured; set one in organization settings")
				}
				used := map[int]bool{}
				for _, o := range d.Databases {
					used[o.PublicPort] = true
				}
				for _, a := range d.Apps {
					for _, pt := range a.Ports {
						used[pt.NodePort] = true
					}
				}
				for {
					port := 30000 + rand.Intn(2767)
					if !used[port] {
						x.PublicPort = port
						break
					}
				}
			} else if !*patch.Public {
				x.PublicPort = 0
			}
		}
		x.UpdatedAt = time.Now()
		db = store.Clone(x)
		return nil
	})
	if err != nil {
		return nil, err
	}
	p.publishDB(db)
	p.EnqueueDatabase(id)
	if len(newExt) > 0 && db.Status == "ready" {
		go func() {
			spec, _ := p.dbSpec(id)
			for _, e := range newExt {
				p.Driver.Query(p.ctx, spec, fmt.Sprintf(`CREATE EXTENSION IF NOT EXISTS "%s"`, e))
			}
		}()
	}
	return db, nil
}

func (p *Platform) RestartDatabase(id string) {
	p.DatabaseState(id, "provisioning", -1, -1)
	go func() {
		time.Sleep(2 * time.Second)
		p.EnqueueDatabase(id)
		s, _ := p.dbSpec(id)
		if !s.Stopped {
			p.DatabaseState(id, "ready", -1, -1)
		}
	}()
}

func (p *Platform) DeleteDatabase(id string) error {
	var ns, name string
	err := p.Store.Write(func(d *store.Data) error {
		db := d.Databases[id]
		if db == nil {
			return Invalid("database not found")
		}
		ns, name = d.Namespace(db.ProjectID), db.Name
		delete(d.Databases, id)
		p.Bus.Publish("project:"+db.ProjectID, "database.deleted", map[string]string{"id": id})
		return nil
	})
	if err != nil {
		return err
	}
	p.queue.enqueue("db:"+id, func(ctx context.Context) error { return p.Driver.DeleteDatabase(ctx, ns, name) })
	return nil
}

func (p *Platform) Query(ctx context.Context, id, sql string) (*runtime.QueryResult, error) {
	s, ok := p.dbSpec(id)
	if !ok {
		return nil, Invalid("database not found")
	}
	if s.Stopped {
		return nil, Invalid("the database is stopped")
	}
	return p.Driver.Query(ctx, s, sql)
}

func (p *Platform) BackupNow(id, method string) (*store.Backup, error) {
	s, ok := p.dbSpec(id)
	if !ok {
		return nil, Invalid("database not found")
	}
	b := &store.Backup{ID: store.NewID("bak"), Status: "running", Method: method, CreatedAt: time.Now()}
	p.Store.Write(func(d *store.Data) error {
		if db := d.Databases[id]; db != nil {
			db.Backups = append([]store.Backup{*b}, db.Backups...)
			if len(db.Backups) > 30 {
				db.Backups = db.Backups[:30]
			}
		}
		return nil
	})
	go func() {
		ctx, cancel := context.WithTimeout(p.ctx, 30*time.Minute)
		defer cancel()
		size, err := p.Driver.Backup(ctx, s)
		var snap *store.Database
		p.Store.Write(func(d *store.Data) error {
			db := d.Databases[id]
			if db == nil {
				return nil
			}
			for i := range db.Backups {
				if db.Backups[i].ID == b.ID {
					if err != nil {
						db.Backups[i].Status = "failed"
					} else {
						db.Backups[i].Status = "completed"
						db.Backups[i].SizeBytes = size
					}
				}
			}
			snap = store.Clone(db)
			return nil
		})
		if snap != nil {
			p.publishDB(snap)
		}
	}()
	return b, nil
}

func (p *Platform) backupLoop() {
	last := map[string]time.Time{}
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-t.C:
		}
		type due struct{ id string }
		var ds []due
		now := time.Now()
		p.Store.Read(func(d *store.Data) {
			for _, db := range d.Databases {
				if db.BackupCron == "" || db.Status != "ready" {
					continue
				}
				s, err := cron.Parse(db.BackupCron)
				if err != nil {
					continue
				}
				prev, ok := last[db.ID]
				if !ok {
					prev = now.Add(-time.Minute)
				}
				if next := s.Next(prev); !next.After(now) {
					ds = append(ds, due{db.ID})
				}
			}
		})
		for _, x := range ds {
			last[x.id] = now
			p.BackupNow(x.id, "scheduled")
		}
	}
}
