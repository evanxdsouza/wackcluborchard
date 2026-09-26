package platform

import (
	"context"
	"strings"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/cron"
	"github.com/evanxdsouza/wackcluborchard/internal/runtime"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

// Stock images for script steps. Deliberately small: debian:12-slim has
// no curl or wget.
var ScriptImages = map[string]string{
	"bash":   "debian:12-slim",
	"python": "python:3.13-slim",
	"node":   "node:22-slim",
}

type JobInput struct {
	Name        string          `json:"name"`
	EnvID       string          `json:"envId"`
	Schedule    string          `json:"schedule"`
	Concurrency string          `json:"concurrency"`
	Steps       []store.JobStep `json:"steps"`
	Paused      bool            `json:"paused"`
}

func validateJob(d *store.Data, projectID string, in *JobInput) error {
	in.Name = strings.ToLower(strings.TrimSpace(in.Name))
	if err := ValidName(in.Name); err != nil {
		return err
	}
	if in.Schedule != "" {
		if _, err := cron.Parse(in.Schedule); err != nil {
			return Invalid("%v", err)
		}
	}
	switch in.Concurrency {
	case "":
		in.Concurrency = "skip"
	case "skip", "queue", "allow":
	default:
		return Invalid("concurrency must be skip, queue or allow")
	}
	if len(in.Steps) == 0 {
		return Invalid("a job needs at least one step")
	}
	for i := range in.Steps {
		st := &in.Steps[i]
		if st.Name == "" {
			st.Name = "step " + string(rune('1'+i))
		}
		switch st.Type {
		case "script":
			if st.Lang == "" {
				st.Lang = "bash"
			}
			if ScriptImages[st.Lang] == "" {
				return Invalid("step %q: language must be bash, python or node", st.Name)
			}
			if strings.TrimSpace(st.Source) == "" {
				return Invalid("step %q has no script", st.Name)
			}
		case "app":
			a := d.Apps[st.AppID]
			if a == nil || a.ProjectID != projectID {
				return Invalid("step %q: pick an app in this project", st.Name)
			}
			if st.Command == "" {
				return Invalid("step %q needs a command", st.Name)
			}
		case "image":
			if st.Image == "" || st.Command == "" {
				return Invalid("step %q needs an image and a command", st.Name)
			}
		default:
			return Invalid("step type must be script, app or image")
		}
	}
	return nil
}

func (p *Platform) CreateJob(user *store.User, projectID string, in JobInput) (*store.Job, error) {
	var job *store.Job
	err := p.Store.Write(func(d *store.Data) error {
		pr := d.Projects[projectID]
		if pr == nil {
			return Invalid("project not found")
		}
		if err := validateJob(d, projectID, &in); err != nil {
			return err
		}
		for _, j := range d.Jobs {
			if j.ProjectID == projectID && j.Name == in.Name {
				return Invalid("a job named %q already exists in this project", in.Name)
			}
		}
		envID := in.EnvID
		if envID == "" && len(pr.Environments) > 0 {
			envID = pr.Environments[0].ID
		}
		job = &store.Job{ID: store.NewID("job"), ProjectID: projectID, EnvID: envID, Name: in.Name, Schedule: in.Schedule,
			Concurrency: in.Concurrency, Steps: in.Steps, Paused: in.Paused, CreatedBy: user.ID, CreatedAt: time.Now()}
		setNext(job)
		d.Jobs[job.ID] = job
		job = store.Clone(job)
		return nil
	})
	return job, err
}

func setNext(j *store.Job) {
	j.NextRunAt = nil
	if j.Schedule == "" || j.Paused {
		return
	}
	if s, err := cron.Parse(j.Schedule); err == nil {
		n := s.Next(time.Now().UTC())
		j.NextRunAt = &n
	}
}

func (p *Platform) UpdateJob(jobID string, in JobInput) (*store.Job, error) {
	var job *store.Job
	err := p.Store.Write(func(d *store.Data) error {
		j := d.Jobs[jobID]
		if j == nil {
			return Invalid("job not found")
		}
		if err := validateJob(d, j.ProjectID, &in); err != nil {
			return err
		}
		j.Name, j.Schedule, j.Concurrency, j.Steps, j.Paused = in.Name, in.Schedule, in.Concurrency, in.Steps, in.Paused
		setNext(j)
		job = store.Clone(j)
		return nil
	})
	return job, err
}

func (p *Platform) DeleteJob(jobID string) error {
	return p.Store.Write(func(d *store.Data) error {
		if d.Jobs[jobID] == nil {
			return Invalid("job not found")
		}
		delete(d.Jobs, jobID)
		for k, r := range d.JobRuns {
			if r.JobID == jobID {
				delete(d.JobRuns, k)
			}
		}
		return nil
	})
}

// TriggerJob starts a run, honoring the job's concurrency policy.
func (p *Platform) TriggerJob(user *store.User, jobID, trigger string) (*store.JobRun, error) {
	var run *store.JobRun
	start := true
	err := p.Store.Write(func(d *store.Data) error {
		j := d.Jobs[jobID]
		if j == nil {
			return Invalid("job not found")
		}
		busy := false
		num := 0
		for _, r := range d.JobRuns {
			if r.JobID != jobID {
				continue
			}
			if r.Number > num {
				num = r.Number
			}
			if r.Status == "running" || r.Status == "queued" {
				busy = true
			}
		}
		if busy {
			switch j.Concurrency {
			case "skip":
				if trigger == "schedule" {
					return Invalid("skipped: the previous run is still going")
				}
				return Invalid("the previous run is still going and this job's policy is to skip")
			case "queue":
				start = false
			}
		}
		now := time.Now()
		run = &store.JobRun{ID: store.NewID("run"), JobID: jobID, Number: num + 1, Trigger: trigger, Status: "queued", CreatedAt: now}
		if user != nil {
			run.CreatedBy = user.Name
			if run.CreatedBy == "" {
				run.CreatedBy = user.Username
			}
		}
		for _, st := range j.Steps {
			img := st.Image
			switch st.Type {
			case "script":
				img = ScriptImages[st.Lang]
			case "app":
				if a := d.Apps[st.AppID]; a != nil {
					img = a.Image
					if img == "" {
						img = a.Source.Image
					}
				}
			}
			run.Steps = append(run.Steps, store.RunStep{Name: st.Name, Type: st.Type, Image: img, Status: "pending"})
		}
		j.LastRunAt = &now
		d.JobRuns[run.ID] = run
		// keep the last 100 runs per job
		runs := d.RunsOf(jobID)
		for _, old := range runs[min(len(runs), 100):] {
			delete(d.JobRuns, old.ID)
		}
		run = store.Clone(run)
		return nil
	})
	if err != nil {
		return nil, err
	}
	p.Bus.Publish("job:"+jobID, "run.updated", run)
	if start {
		go p.executeRun(run.ID)
	}
	return run, nil
}

func (p *Platform) CancelRun(runID string) {
	p.runMu.Lock()
	if c := p.running[runID]; c != nil {
		c()
	}
	p.runMu.Unlock()
}

func (p *Platform) executeRun(runID string) {
	var spec runtime.JobSpec
	var jobID string
	ok := false
	p.Store.Write(func(d *store.Data) error {
		r := d.JobRuns[runID]
		if r == nil {
			return nil
		}
		j := d.Jobs[r.JobID]
		if j == nil {
			return nil
		}
		jobID = j.ID
		now := time.Now()
		r.Status = "running"
		r.StartedAt = &now
		spec = runtime.JobSpec{Namespace: d.Namespace(j.ProjectID), Name: j.Name, RunID: r.ID, Env: map[string]string{}}
		for k, v := range d.Variables {
			_ = k
			if v.ProjectID == j.ProjectID && v.AppID == "" && (v.EnvID == "" || v.EnvID == j.EnvID) {
				spec.Env[v.Key] = interpolate(d, j.ProjectID, v.Value)
			}
		}
		for i, st := range j.Steps {
			rs := runtime.ResolvedStep{Name: st.Name, Image: r.Steps[i].Image}
			switch st.Type {
			case "script":
				rs.Script, rs.Lang = st.Source, st.Lang
			default:
				rs.Command = []string{"sh", "-c", st.Command}
			}
			if st.Type == "app" {
				if a := d.Apps[st.AppID]; a != nil {
					env, sec := ResolveVars(d, a)
					for k, v := range env {
						spec.Env[k] = v
					}
					for k, v := range sec {
						spec.Env[k] = v
					}
				}
			}
			spec.Steps = append(spec.Steps, rs)
		}
		ok = true
		return nil
	})
	if !ok {
		return
	}
	p.publishRun(runID)
	ctx, cancel := context.WithTimeout(p.ctx, 6*time.Hour)
	p.runMu.Lock()
	p.running[runID] = cancel
	p.runMu.Unlock()
	defer func() {
		cancel()
		p.runMu.Lock()
		delete(p.running, runID)
		p.runMu.Unlock()
	}()
	p.Driver.EnsureNamespace(ctx, spec.Namespace, nil)
	err := p.Driver.RunJob(ctx, spec, func(u runtime.StepUpdate) {
		p.Store.Write(func(d *store.Data) error {
			r := d.JobRuns[runID]
			if r == nil || u.Index >= len(r.Steps) {
				return nil
			}
			st := &r.Steps[u.Index]
			now := time.Now()
			if u.Status != "" {
				st.Status = u.Status
				if u.Status == "running" {
					st.StartedAt = &now
				} else {
					st.FinishedAt = &now
				}
			}
			if u.ExitCode != nil {
				st.ExitCode = u.ExitCode
			}
			if u.Output != "" {
				st.Output += u.Output
				if len(st.Output) > 256<<10 {
					st.Output = "…(truncated)…\n" + st.Output[len(st.Output)-200<<10:]
				}
			}
			return nil
		})
		p.publishRun(runID)
	})
	p.Store.Write(func(d *store.Data) error {
		r := d.JobRuns[runID]
		if r == nil {
			return nil
		}
		now := time.Now()
		r.FinishedAt = &now
		switch {
		case ctx.Err() == context.Canceled:
			r.Status = "cancelled"
		case err != nil:
			r.Status = "failed"
		default:
			r.Status = "succeeded"
		}
		for i := range r.Steps {
			if r.Steps[i].Status == "running" || r.Steps[i].Status == "pending" {
				if err != nil {
					r.Steps[i].Status = "skipped"
				}
			}
		}
		return nil
	})
	p.publishRun(runID)
	// start the next queued run, if any
	var next string
	p.Store.Read(func(d *store.Data) {
		var oldest *store.JobRun
		for _, r := range d.JobRuns {
			if r.JobID == jobID && r.Status == "queued" && (oldest == nil || r.Number < oldest.Number) {
				oldest = r
			}
		}
		if oldest != nil {
			next = oldest.ID
		}
	})
	if next != "" {
		go p.executeRun(next)
	}
}

func (p *Platform) publishRun(runID string) {
	var r *store.JobRun
	p.Store.Read(func(d *store.Data) {
		if x := d.JobRuns[runID]; x != nil {
			r = store.Clone(x)
		}
	})
	if r != nil {
		p.Bus.Publish("run:"+runID, "run.updated", r)
		p.Bus.Publish("job:"+r.JobID, "run.updated", r)
	}
}

// scheduler fires cron jobs. There is one scheduler per control plane;
// its state lives in the store so a restart picks up where it left off.
func (p *Platform) scheduler() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-t.C:
		}
		now := time.Now().UTC()
		var due []string
		p.Store.Write(func(d *store.Data) error {
			for _, j := range d.Jobs {
				if j.Paused || j.Schedule == "" || j.NextRunAt == nil {
					continue
				}
				if !j.NextRunAt.After(now) {
					due = append(due, j.ID)
					setNext(j)
				}
			}
			return nil
		})
		for _, id := range due {
			p.TriggerJob(nil, id, "schedule")
		}
	}
}
