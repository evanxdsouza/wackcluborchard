// Package platform is Wack Club Orchard's control plane: it owns desired state in the
// store, drives the runtime through a retrying apply queue, runs builds
// within a bounded number of slots, schedules jobs, and mirrors observed
// cluster state back so reads never have to ask the cluster.
package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/events"
	"github.com/evanxdsouza/wackcluborchard/internal/runtime"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

type Config struct {
	FrontendURL string
	Registry    string
	Version     string
}

type Platform struct {
	Store  *store.Store
	Bus    *events.Bus
	Logs   *events.LogHub
	Driver runtime.Driver
	Cfg    Config

	queue  *applyQueue
	builds *buildQueue
	ctx    context.Context

	runMu   sync.Mutex
	running map[string]context.CancelFunc // job run id -> cancel

	// CloneToken mints a credential able to clone a repository, such as a
	// GitHub App installation token. Builds fall back to the app creator's
	// own GitHub token.
	CloneToken func(ctx context.Context, repo string) string

	metricsMu sync.Mutex
	metrics   map[string][]MetricPoint // app id -> recent samples
}

type MetricPoint struct {
	At        time.Time `json:"at"`
	CPUMillis float64   `json:"cpuMillis"`
	MemoryMi  float64   `json:"memoryMi"`
	Replicas  int       `json:"replicas"`
}

var (
	ErrQuota     = errors.New("quota exceeded")
	ErrForbidden = errors.New("forbidden")
	ErrInvalid   = errors.New("invalid")
)

// Error wraps a user-facing message with a kind for status mapping.
type Error struct {
	Kind error
	Msg  string
}

func (e *Error) Error() string { return e.Msg }
func (e *Error) Unwrap() error { return e.Kind }

func Invalid(format string, a ...any) error {
	return &Error{Kind: ErrInvalid, Msg: fmt.Sprintf(format, a...)}
}

func New(ctx context.Context, s *store.Store, d runtime.Driver, cfg Config) *Platform {
	p := &Platform{
		Store:   s,
		Bus:     events.New(4096),
		Logs:    events.NewLogHub(),
		Driver:  d,
		Cfg:     cfg,
		ctx:     ctx,
		running: map[string]context.CancelFunc{},
		metrics: map[string][]MetricPoint{},
	}
	p.queue = newApplyQueue(ctx, p)
	p.builds = newBuildQueue(p)
	return p
}

// Start connects the runtime's informers and begins background loops.
func (p *Platform) Start() error {
	if err := p.Driver.Start(p.ctx, p); err != nil {
		return err
	}
	p.recover()
	go p.scheduler()
	go p.metricsLoop()
	go p.backupLoop()
	return nil
}

// recover re-applies desired state after a restart and fails builds that
// were in flight when the process died.
func (p *Platform) recover() {
	var appIDs, dbIDs, sbIDs []string
	p.Store.Write(func(d *store.Data) error {
		now := time.Now()
		for _, dep := range d.Deploys {
			if dep.Status == "running" || dep.Status == "queued" {
				dep.Status = "failed"
				dep.Error = "interrupted by a control-plane restart; redeploy to try again"
				dep.FinishedAt = &now
				for i := range dep.Steps {
					if dep.Steps[i].Status == "running" {
						dep.Steps[i].Status = "failed"
					}
				}
			}
		}
		for _, r := range d.JobRuns {
			if r.Status == "running" || r.Status == "queued" {
				r.Status = "failed"
				r.FinishedAt = &now
			}
		}
		for _, a := range d.Apps {
			if a.Image != "" {
				appIDs = append(appIDs, a.ID)
			}
		}
		for _, db := range d.Databases {
			dbIDs = append(dbIDs, db.ID)
		}
		for _, sb := range d.Sandboxes {
			if sb.Status != "stopped" {
				sbIDs = append(sbIDs, sb.ID)
			}
		}
		return nil
	})
	for _, id := range appIDs {
		p.EnqueueApp(id)
	}
	for _, id := range dbIDs {
		p.EnqueueDatabase(id)
	}
	for _, id := range sbIDs {
		p.enqueueSandbox(id)
	}
}

func (p *Platform) Settings() store.Settings {
	var s store.Settings
	p.Store.Read(func(d *store.Data) { s = d.Settings })
	return s
}

// ---- helpers ----

func randString(n int) string {
	const a = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, n)
	for i := range b {
		x, _ := rand.Int(rand.Reader, big.NewInt(int64(len(a))))
		b[i] = a[x.Int64()]
	}
	return string(b)
}

func RandHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (p *Platform) publishApp(a *store.App) {
	p.Bus.Publish("project:"+a.ProjectID, "app.updated", a)
	p.Bus.Publish("app:"+a.ID, "app.updated", a)
}

func (p *Platform) publishDB(db *store.Database) {
	p.Bus.Publish("project:"+db.ProjectID, "database.updated", db)
	p.Bus.Publish("database:"+db.ID, "database.updated", db)
}

// Audit records who did what.
func (p *Platform) Audit(orgID string, actor *store.User, action, target string, meta map[string]string) {
	p.Store.Write(func(d *store.Data) error {
		e := &store.AuditEntry{OrgID: orgID, Action: action, Target: target, Meta: meta}
		if actor != nil {
			e.ActorID = actor.ID
			e.Actor = actor.Username
		} else {
			e.Actor = "system"
		}
		d.AddAudit(e)
		return nil
	})
}

// ---- status ----

// computeStatus derives an app's status from what the cluster reports.
// A build in flight dominates.
func computeStatus(d *store.Data, a *store.App) (string, string) {
	if a.CurrentDeploy != "" {
		if dep := d.Deploys[a.CurrentDeploy]; dep != nil && (dep.Status == "running" || dep.Status == "queued") {
			if dep.Kind == "build" {
				for _, s := range dep.Steps[:3] {
					if s.Status == "running" || s.Status == "pending" {
						return "building", ""
					}
				}
			}
			return "deploying", ""
		}
	}
	if a.Image == "" {
		if a.CurrentDeploy != "" {
			if dep := d.Deploys[a.CurrentDeploy]; dep != nil && dep.Status == "failed" {
				return "failed", dep.Error
			}
		}
		return "pending", ""
	}
	if a.Replicas == 0 {
		return "stopped", ""
	}
	ready, total := 0, 0
	var reason string
	for _, pod := range a.Pods {
		if pod.Phase == "Terminating" {
			continue
		}
		total++
		if pod.Ready {
			ready++
		} else if pod.Reason != "" {
			reason = pod.Reason
		}
	}
	switch {
	case ready >= a.Replicas:
		return "running", ""
	case reason == "CrashLoopBackOff" || reason == "Error" || reason == "OOMKilled" || reason == "ImagePullBackOff" || reason == "ErrImagePull" || reason == "CreateContainerConfigError":
		if ready > 0 {
			return "degraded", plainReason(reason)
		}
		return "failed", plainReason(reason)
	case ready > 0:
		return "degraded", fmt.Sprintf("%d of %d replicas ready", ready, a.Replicas)
	case total == 0:
		return "deploying", "waiting for pods"
	default:
		if reason != "" && reason != "ContainerCreating" {
			return "deploying", plainReason(reason)
		}
		return "deploying", "starting"
	}
}

func plainReason(r string) string {
	switch r {
	case "CrashLoopBackOff":
		return "The container keeps exiting and Kubernetes is backing off restarting it. The crash report has its last output."
	case "OOMKilled":
		return "Killed for using more memory than its limit (exit 137). Raise the memory limit or fix the leak."
	case "ImagePullBackOff", "ErrImagePull":
		return "The image could not be pulled. Check the image name and registry credentials."
	case "CreateContainerConfigError":
		return "The container could not be configured, usually a missing secret or config reference."
	case "Error":
		return "The container exited with an error."
	}
	if strings.Contains(r, "Insufficient cpu") || strings.Contains(r, "Insufficient memory") {
		return "No node has enough allocatable CPU or memory for this app's request. " + r
	}
	if strings.Contains(r, "untolerated taint") {
		return "Every node that could run this has a taint the app does not tolerate. " + r
	}
	return r
}

// TranslateEvent turns a Kubernetes warning into plain English, keeping the
// raw reason alongside.
func TranslateEvent(reason, msg string) string {
	switch reason {
	case "FailedScheduling":
		switch {
		case strings.Contains(msg, "Insufficient cpu"):
			return "Waiting for a node with enough free CPU for this request."
		case strings.Contains(msg, "Insufficient memory"):
			return "Waiting for a node with enough free memory for this request."
		case strings.Contains(msg, "taint"):
			return "No node without a matching taint is available."
		case strings.Contains(msg, "volume node affinity") || strings.Contains(msg, "Multi-Attach"):
			return "Its volume is attached to another node, so it cannot start elsewhere."
		}
		return "Kubernetes could not find a node to run this on."
	case "BackOff":
		if strings.Contains(msg, "pulling image") {
			return "Backing off retrying the image pull."
		}
		return "The container keeps crashing; Kubernetes is waiting longer between restarts."
	case "Failed":
		if strings.Contains(msg, "pull") {
			return "The image could not be pulled."
		}
		return "A container failed to start."
	case "Unhealthy":
		if strings.Contains(msg, "Readiness") {
			return "The readiness check is failing, so this pod gets no traffic."
		}
		return "The liveness check is failing; the container will be restarted."
	case "FailedMount", "FailedAttachVolume":
		return "A volume could not be mounted."
	case "OOMKilling", "OOMKilled":
		return "A container was killed for exceeding its memory limit."
	case "FailedCreate":
		if strings.Contains(msg, "exceeded quota") {
			return "Creating a pod would exceed the namespace quota."
		}
		return "Kubernetes could not create a pod."
	case "Evicted":
		return "The pod was evicted because its node ran low on resources."
	}
	return msg
}

// ---- observer (mirror) ----

func (p *Platform) AppPods(appID string, pods []store.Pod) {
	var snapshot *store.App
	p.Store.Write(func(d *store.Data) error {
		a := d.Apps[appID]
		if a == nil {
			return nil
		}
		a.Pods = pods
		a.Status, a.StatusMsg = computeStatus(d, a)
		snapshot = store.Clone(a)
		return nil
	})
	if snapshot != nil {
		p.publishApp(snapshot)
	}
}

func (p *Platform) refreshStatus(appID string) {
	var snapshot *store.App
	p.Store.Write(func(d *store.Data) error {
		a := d.Apps[appID]
		if a == nil {
			return nil
		}
		a.Status, a.StatusMsg = computeStatus(d, a)
		snapshot = store.Clone(a)
		return nil
	})
	if snapshot != nil {
		p.publishApp(snapshot)
	}
}

func (p *Platform) DatabaseState(dbID, status string, size int64, conns int) {
	var snap *store.Database
	p.Store.Write(func(d *store.Data) error {
		db := d.Databases[dbID]
		if db == nil {
			return nil
		}
		changed := db.Status != status
		db.Status = status
		if size >= 0 {
			changed = changed || db.SizeBytes != size
			db.SizeBytes = size
		}
		if conns >= 0 {
			changed = changed || db.Connections != conns
			db.Connections = conns
		}
		if changed {
			db.UpdatedAt = time.Now()
			snap = store.Clone(db)
		}
		return nil
	})
	if snap != nil {
		p.publishDB(snap)
	}
}

func (p *Platform) Crash(ownerID string, c store.CrashReport) {
	c.OwnerID = ownerID
	p.Store.Write(func(d *store.Data) error {
		d.AddCrash(&c)
		return nil
	})
	p.Bus.Publish("app:"+ownerID, "crash", c)
	p.Bus.Publish("database:"+ownerID, "crash", c)
}

func (p *Platform) Warning(ownerID string, e store.KubeEvent) {
	var appID string
	p.Store.Write(func(d *store.Data) error {
		if strings.HasPrefix(ownerID, "name:") {
			// ns/app-name reference from events on Deployments/ReplicaSets
			ref := strings.TrimPrefix(ownerID, "name:")
			for _, a := range d.Apps {
				if d.Namespace(a.ProjectID)+"/"+a.Name == ref || strings.HasPrefix(ref, d.Namespace(a.ProjectID)+"/"+a.Name) {
					ownerID = a.ID
				}
			}
			if strings.HasPrefix(ownerID, "name:") {
				return nil
			}
		}
		key := ownerID + "|" + e.Object + "|" + e.Reason + "|" + e.Message
		now := time.Now()
		if e.LastSeen.IsZero() {
			e.LastSeen = now
		}
		if ex := d.Events[key]; ex != nil {
			ex.Count++
			if e.Count > ex.Count {
				ex.Count = e.Count
			}
			ex.LastSeen = e.LastSeen
		} else {
			e.ID = store.NewID("evt")
			e.OwnerID = ownerID
			e.Plain = TranslateEvent(e.Reason, e.Message)
			if e.FirstSeen.IsZero() {
				e.FirstSeen = now
			}
			if e.Count == 0 {
				e.Count = 1
			}
			d.Events[key] = &e
		}
		// Kubernetes keeps warnings for about an hour; so do we.
		for k, ev := range d.Events {
			if now.Sub(ev.LastSeen) > 3*time.Hour {
				delete(d.Events, k)
			}
		}
		appID = ownerID
		return nil
	})
	if appID != "" {
		p.Bus.Publish("app:"+appID, "event", e)
	}
}

func (p *Platform) AppLog(appID, pod, line string) {
	p.Logs.Append("app:"+appID, events.LogLine{Pod: pod, Text: line})
}

func (p *Platform) SandboxState(id, status string, progress int, stage string) {
	var snap *store.Sandbox
	p.Store.Write(func(d *store.Data) error {
		sb := d.Sandboxes[id]
		if sb == nil {
			return nil
		}
		sb.Status = status
		sb.BootProgress = progress
		sb.BootStage = stage
		snap = store.Clone(sb)
		return nil
	})
	if snap != nil {
		p.Bus.Publish("org:"+snap.OrgID, "sandbox.updated", snap)
		p.Bus.Publish("sandbox:"+snap.ID, "sandbox.updated", snap)
	}
}

// ---- metrics ----

func (p *Platform) metricsLoop() {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-t.C:
		}
		type target struct {
			id, ns, name string
			replicas     int
		}
		var ts []target
		p.Store.Read(func(d *store.Data) {
			for _, a := range d.Apps {
				if a.Image != "" && a.Replicas > 0 {
					ts = append(ts, target{a.ID, d.Namespace(a.ProjectID), a.Name, a.Replicas})
				}
			}
		})
		for _, t := range ts {
			ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
			m, err := p.Driver.AppMetrics(ctx, t.ns, t.name)
			cancel()
			if err != nil {
				continue
			}
			pt := MetricPoint{At: time.Now(), CPUMillis: m.CPUMillis, MemoryMi: m.MemoryMi, Replicas: t.replicas}
			p.metricsMu.Lock()
			s := append(p.metrics[t.id], pt)
			if len(s) > 360 { // one hour at 10s
				s = s[len(s)-360:]
			}
			p.metrics[t.id] = s
			p.metricsMu.Unlock()
			p.Bus.Publish("app:"+t.id, "metrics", pt)
		}
	}
}

func (p *Platform) Metrics(appID string) []MetricPoint {
	p.metricsMu.Lock()
	defer p.metricsMu.Unlock()
	return append([]MetricPoint(nil), p.metrics[appID]...)
}

// LatestMetrics returns the most recent sample for each app.
func (p *Platform) LatestMetrics() map[string]MetricPoint {
	p.metricsMu.Lock()
	defer p.metricsMu.Unlock()
	out := map[string]MetricPoint{}
	for id, s := range p.metrics {
		if len(s) > 0 {
			out[id] = s[len(s)-1]
		}
	}
	return out
}

// ---- nodes & pools ----

func (p *Platform) Nodes(ctx context.Context) ([]runtime.Node, error) {
	nodes, err := p.Driver.Nodes(ctx)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	return nodes, err
}
