// Package store is Wack Club Orchard's control-plane state: a typed document store
// held in memory and snapshotted atomically to disk. Reads never touch the
// cluster; the runtime driver mirrors cluster state into it.
package store

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("not found")

type Data struct {
	Settings          Settings                     `json:"settings"`
	Users             map[string]*User             `json:"users"`
	Sessions          map[string]*Session          `json:"sessions"`
	APITokens         map[string]*APIToken         `json:"apiTokens"`
	Orgs              map[string]*Org              `json:"orgs"`
	Memberships       map[string]*Membership       `json:"memberships"`
	Invites           map[string]*Invite           `json:"invites"`
	Projects          map[string]*Project          `json:"projects"`
	Variables         map[string]*Variable         `json:"variables"`
	Apps              map[string]*App              `json:"apps"`
	Deploys           map[string]*Deploy           `json:"deploys"`
	Databases         map[string]*Database         `json:"databases"`
	Jobs              map[string]*Job              `json:"jobs"`
	JobRuns           map[string]*JobRun           `json:"jobRuns"`
	TemplateInstances map[string]*TemplateInstance `json:"templateInstances"`
	Compose           map[string]*ComposeStack     `json:"compose"`
	Sandboxes         map[string]*Sandbox          `json:"sandboxes"`
	Audit             []*AuditEntry                `json:"audit"`
	Crashes           []*CrashReport               `json:"crashes"`
	Events            map[string]*KubeEvent        `json:"events"`
	Pools             map[string]*Pool             `json:"pools"`
}

func newData() *Data {
	d := &Data{}
	d.ensure()
	return d
}

func (d *Data) ensure() {
	if d.Users == nil {
		d.Users = map[string]*User{}
	}
	if d.Sessions == nil {
		d.Sessions = map[string]*Session{}
	}
	if d.APITokens == nil {
		d.APITokens = map[string]*APIToken{}
	}
	if d.Orgs == nil {
		d.Orgs = map[string]*Org{}
	}
	if d.Memberships == nil {
		d.Memberships = map[string]*Membership{}
	}
	if d.Invites == nil {
		d.Invites = map[string]*Invite{}
	}
	if d.Projects == nil {
		d.Projects = map[string]*Project{}
	}
	if d.Variables == nil {
		d.Variables = map[string]*Variable{}
	}
	if d.Apps == nil {
		d.Apps = map[string]*App{}
	}
	if d.Deploys == nil {
		d.Deploys = map[string]*Deploy{}
	}
	if d.Databases == nil {
		d.Databases = map[string]*Database{}
	}
	if d.Jobs == nil {
		d.Jobs = map[string]*Job{}
	}
	if d.JobRuns == nil {
		d.JobRuns = map[string]*JobRun{}
	}
	if d.TemplateInstances == nil {
		d.TemplateInstances = map[string]*TemplateInstance{}
	}
	if d.Compose == nil {
		d.Compose = map[string]*ComposeStack{}
	}
	if d.Sandboxes == nil {
		d.Sandboxes = map[string]*Sandbox{}
	}
	if d.Events == nil {
		d.Events = map[string]*KubeEvent{}
	}
	if d.Pools == nil {
		d.Pools = map[string]*Pool{}
	}
}

type Store struct {
	mu    sync.RWMutex
	data  *Data
	path  string
	dirty bool
	stop  chan struct{}
	done  chan struct{}
}

// Open loads the snapshot at path, or starts empty. An empty path keeps
// everything in memory.
func Open(path string) (*Store, error) {
	s := &Store{data: newData(), path: path, stop: make(chan struct{}), done: make(chan struct{})}
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		b, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := json.Unmarshal(b, s.data); err != nil {
				return nil, fmt.Errorf("parse %s: %w", path, err)
			}
			s.data.ensure()
		case errors.Is(err, os.ErrNotExist):
		default:
			return nil, err
		}
	}
	go s.flushLoop()
	return s, nil
}

func (s *Store) flushLoop() {
	defer close(s.done)
	t := time.NewTicker(750 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if err := s.Flush(); err != nil {
				log.Printf("store: flush: %v", err)
			}
		case <-s.stop:
			if err := s.Flush(); err != nil {
				log.Printf("store: final flush: %v", err)
			}
			return
		}
	}
}

// Flush writes the snapshot if anything changed since the last write.
func (s *Store) Flush() error {
	if s.path == "" {
		return nil
	}
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return nil
	}
	b, err := json.Marshal(s.data)
	s.dirty = false
	s.mu.Unlock()
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) Close() error {
	close(s.stop)
	<-s.done
	return nil
}

// Read runs fn with a consistent read-only view.
func (s *Store) Read(fn func(d *Data)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fn(s.data)
}

// Write runs fn with exclusive access. Returning an error still keeps any
// mutations fn already made, so validate before mutating.
func (s *Store) Write(fn func(d *Data) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := fn(s.data)
	s.dirty = true
	return err
}

// Clone deep-copies v so it can be used outside the lock.
func Clone[T any](v T) T {
	var out T
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	return out
}

const idAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

// NewID returns a short random id with a type prefix, like "app_k3j9x2m1q8".
func NewID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = idAlphabet[int(b[i])%len(idAlphabet)]
	}
	return prefix + "_" + string(b)
}

// Values returns map values sorted by the given less function.
func Values[T any](m map[string]*T, less func(a, b *T) bool) []*T {
	out := make([]*T, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	if less != nil {
		sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	}
	return out
}

// Slugify lowercases s and keeps it DNS-label safe.
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 40 {
		out = strings.Trim(out[:40], "-")
	}
	if out == "" {
		out = "x"
	}
	return out
}

// ---- common lookups (call with the lock held) ----

func (d *Data) Membership(orgID, userID string) *Membership {
	for _, m := range d.Memberships {
		if m.OrgID == orgID && m.UserID == userID {
			return m
		}
	}
	return nil
}

func (d *Data) UserOrgs(userID string) []*Org {
	var out []*Org
	for _, m := range d.Memberships {
		if m.UserID == userID {
			if o := d.Orgs[m.OrgID]; o != nil {
				out = append(out, o)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (d *Data) OrgBySlug(slug string) *Org {
	for _, o := range d.Orgs {
		if o.Slug == slug {
			return o
		}
	}
	return nil
}

func (d *Data) UserByName(username string) *User {
	for _, u := range d.Users {
		if strings.EqualFold(u.Username, username) || (u.Email != "" && strings.EqualFold(u.Email, username)) {
			return u
		}
	}
	return nil
}

func (d *Data) AppsIn(projectID string) []*App {
	var out []*App
	for _, a := range d.Apps {
		if a.ProjectID == projectID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (d *Data) DatabasesIn(projectID string) []*Database {
	var out []*Database
	for _, a := range d.Databases {
		if a.ProjectID == projectID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (d *Data) DeploysOf(appID string) []*Deploy {
	var out []*Deploy
	for _, x := range d.Deploys {
		if x.AppID == appID {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number > out[j].Number })
	return out
}

func (d *Data) RunsOf(jobID string) []*JobRun {
	var out []*JobRun
	for _, x := range d.JobRuns {
		if x.JobID == jobID {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number > out[j].Number })
	return out
}

// Namespace is the Kubernetes namespace for a project: one per project per
// organization, so tenants are separated at the cluster level.
func (d *Data) Namespace(projectID string) string {
	p := d.Projects[projectID]
	if p == nil {
		return ""
	}
	o := d.Orgs[p.OrgID]
	if o == nil {
		return ""
	}
	ns := "wackcluborchard-" + o.Slug + "-" + p.Slug
	if len(ns) > 63 {
		ns = strings.Trim(ns[:63], "-")
	}
	return ns
}

func (d *Data) AddAudit(e *AuditEntry) {
	e.ID = NewID("aud")
	e.CreatedAt = time.Now()
	d.Audit = append(d.Audit, e)
	if len(d.Audit) > 20000 {
		d.Audit = d.Audit[len(d.Audit)-20000:]
	}
}

func (d *Data) AddCrash(c *CrashReport) {
	c.ID = NewID("crash")
	c.CreatedAt = time.Now()
	d.Crashes = append(d.Crashes, c)
	if len(d.Crashes) > 2000 {
		d.Crashes = d.Crashes[len(d.Crashes)-2000:]
	}
}
