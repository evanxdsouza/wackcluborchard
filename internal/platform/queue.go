package platform

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

// applyQueue reconciles desired state against the cluster outside the HTTP
// request. Work is keyed per object and coalesced: enqueuing an object that
// is already waiting does nothing, so the latest desired state always wins.
// Failures retry with exponential backoff instead of surfacing as a failed
// request the user has to repeat.
type applyQueue struct {
	p       *Platform
	mu      sync.Mutex
	pending map[string]func(context.Context) error
	order   []string
	active  map[string]bool
	wake    chan struct{}
}

func newApplyQueue(ctx context.Context, p *Platform) *applyQueue {
	q := &applyQueue{p: p, pending: map[string]func(context.Context) error{}, active: map[string]bool{}, wake: make(chan struct{}, 1)}
	for i := 0; i < 4; i++ {
		go q.worker(ctx)
	}
	return q
}

func (q *applyQueue) enqueue(key string, fn func(context.Context) error) {
	q.mu.Lock()
	if _, ok := q.pending[key]; !ok {
		q.order = append(q.order, key)
	}
	q.pending[key] = fn
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *applyQueue) next() (string, func(context.Context) error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, k := range q.order {
		if q.active[k] {
			continue
		}
		fn := q.pending[k]
		delete(q.pending, k)
		q.order = append(q.order[:i:i], q.order[i+1:]...)
		q.active[k] = true
		return k, fn
	}
	return "", nil
}

func (q *applyQueue) worker(ctx context.Context) {
	for {
		key, fn := q.next()
		if fn == nil {
			select {
			case <-ctx.Done():
				return
			case <-q.wake:
			case <-time.After(time.Second):
			}
			continue
		}
		var err error
		backoff := 500 * time.Millisecond
		for attempt := 1; attempt <= 6; attempt++ {
			actx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			err = fn(actx)
			cancel()
			if err == nil || ctx.Err() != nil {
				break
			}
			log.Printf("apply %s: attempt %d: %v", key, attempt, err)
			// a newer desired state supersedes this retry
			q.mu.Lock()
			_, superseded := q.pending[key]
			q.mu.Unlock()
			if superseded {
				err = nil
				break
			}
			select {
			case <-ctx.Done():
			case <-time.After(backoff):
			}
			backoff *= 2
		}
		q.mu.Lock()
		delete(q.active, key)
		q.mu.Unlock()
		if err != nil {
			q.p.applyFailed(key, err)
		}
		select {
		case q.wake <- struct{}{}:
		default:
		}
	}
}

func (p *Platform) applyFailed(key string, err error) {
	log.Printf("apply %s gave up: %v", key, err)
	p.Store.Write(func(d *store.Data) error {
		if a := d.Apps[key[len("app:"):]]; len(key) > 4 && key[:4] == "app:" && a != nil {
			a.StatusMsg = "Could not apply to the cluster: " + err.Error()
		}
		return nil
	})
}

// buildQueue bounds concurrent builds. A queued build is accepted, not
// ignored; it starts on its own when a slot frees. A second request for
// the same app supersedes the waiting one instead of queueing twice.
type buildQueue struct {
	p       *Platform
	mu      sync.Mutex
	running map[string]bool // deploy ids
	waiting []queuedBuild
}

type queuedBuild struct {
	deployID string
	appID    string
	start    func()
}

type BuildTicket struct {
	Accepted bool   `json:"accepted"`
	Started  bool   `json:"started"`
	Status   string `json:"status"`
	Running  int    `json:"running"`
	Slots    int    `json:"slots"`
	Waiting  int    `json:"waiting"`
	Message  string `json:"message"`
	DeployID string `json:"deployId"`
}

func newBuildQueue(p *Platform) *buildQueue {
	return &buildQueue{p: p, running: map[string]bool{}}
}

func (b *buildQueue) slots() int {
	n := b.p.Settings().BuildSlots
	if n <= 0 {
		n = 2
	}
	return n
}

func (b *buildQueue) submit(deployID, appID string, start func()) (BuildTicket, []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var superseded []string
	kept := b.waiting[:0]
	for _, w := range b.waiting {
		if w.appID == appID {
			superseded = append(superseded, w.deployID)
			continue
		}
		kept = append(kept, w)
	}
	b.waiting = kept
	slots := b.slots()
	t := BuildTicket{Accepted: true, Slots: slots, DeployID: deployID}
	if len(b.running) < slots {
		b.running[deployID] = true
		t.Started = true
		t.Status = "running"
		t.Running = len(b.running)
		t.Message = "Build started."
		go start()
		return t, superseded
	}
	b.waiting = append(b.waiting, queuedBuild{deployID, appID, start})
	t.Status = "queued"
	t.Running = len(b.running)
	t.Waiting = len(b.waiting)
	t.Message = fmt.Sprintf("All %d build slots are busy on this instance. This build is queued and will start on its own when a slot frees up; do not retry, a second request supersedes this one rather than moving it up the queue.", slots)
	return t, superseded
}

func (b *buildQueue) done(deployID string) {
	b.mu.Lock()
	delete(b.running, deployID)
	var next *queuedBuild
	if len(b.waiting) > 0 && len(b.running) < b.slots() {
		n := b.waiting[0]
		b.waiting = b.waiting[1:]
		b.running[n.deployID] = true
		next = &n
	}
	b.mu.Unlock()
	if next != nil {
		go next.start()
	}
}

func (b *buildQueue) Stats() (running, waiting, slots int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.running), len(b.waiting), b.slots()
}

func (p *Platform) BuildStats() (int, int, int) { return p.builds.Stats() }
