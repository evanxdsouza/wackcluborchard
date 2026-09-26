package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/runtime"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
	"github.com/evanxdsouza/wackcluborchard/internal/ws"
)

type sse struct {
	w  http.ResponseWriter
	f  http.Flusher
	mu sync.Mutex
}

func newSSE(w http.ResponseWriter) (*sse, bool) {
	f, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "streaming unsupported")
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	f.Flush()
	return &sse{w: w, f: f}, true
}

func (s *sse) send(id int64, event string, data any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(data)
	if id > 0 {
		fmt.Fprintf(s.w, "id: %d\n", id)
	}
	if event != "" {
		fmt.Fprintf(s.w, "event: %s\n", event)
	}
	_, err := fmt.Fprintf(s.w, "data: %s\n\n", b)
	s.f.Flush()
	return err
}

func (s *sse) ping() {
	s.mu.Lock()
	fmt.Fprint(s.w, ": ping\n\n")
	s.f.Flush()
	s.mu.Unlock()
}

// canSeeTopic authorizes one event-bus topic for the user.
func (s *Server) canSeeTopic(d *store.Data, u *store.User, topic string) bool {
	kind, id, ok := strings.Cut(topic, ":")
	if !ok {
		return false
	}
	switch kind {
	case "project":
		return canViewProject(d, u, d.Projects[id])
	case "app":
		if a := d.Apps[id]; a != nil {
			return canViewProject(d, u, d.Projects[a.ProjectID])
		}
	case "database":
		if a := d.Databases[id]; a != nil {
			return canViewProject(d, u, d.Projects[a.ProjectID])
		}
	case "deploy":
		if dep := d.Deploys[id]; dep != nil {
			if a := d.Apps[dep.AppID]; a != nil {
				return canViewProject(d, u, d.Projects[a.ProjectID])
			}
		}
	case "job":
		if j := d.Jobs[id]; j != nil {
			return canViewProject(d, u, d.Projects[j.ProjectID])
		}
	case "run":
		if r := d.JobRuns[id]; r != nil {
			if j := d.Jobs[r.JobID]; j != nil {
				return canViewProject(d, u, d.Projects[j.ProjectID])
			}
		}
	case "org":
		return orgRole(d, u, id) != ""
	case "sandbox":
		if sb := d.Sandboxes[id]; sb != nil {
			return sb.OwnerID == u.ID || store.RoleRank(orgRole(d, u, sb.OrgID)) >= store.RoleRank(store.RoleAdmin)
		}
	}
	return false
}

// events streams bus events for the requested topics. A reconnect with
// Last-Event-ID replays what was missed.
func (s *Server) events(w http.ResponseWriter, r *http.Request, u *store.User) {
	var topics []string
	s.P.Store.Read(func(d *store.Data) {
		for _, t := range strings.Split(r.URL.Query().Get("topics"), ",") {
			t = strings.TrimSpace(t)
			if t != "" && s.canSeeTopic(d, u, t) {
				topics = append(topics, t)
			}
		}
	})
	if len(topics) == 0 {
		writeErr(w, 400, "no topics you can see")
		return
	}
	last, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if last == 0 {
		last, _ = strconv.ParseInt(r.URL.Query().Get("lastEventId"), 10, 64)
	}
	// exact topic match: "app:app_x" must not also match "app:app_xy"
	prefixes := make([]string, len(topics))
	copy(prefixes, topics)
	sub, replay := s.P.Bus.Subscribe(last, prefixes...)
	defer s.P.Bus.Unsubscribe(sub)
	st, ok := newSSE(w)
	if !ok {
		return
	}
	exact := map[string]bool{}
	for _, t := range topics {
		exact[t] = true
	}
	for _, ev := range replay {
		if exact[ev.Topic] {
			st.send(ev.ID, ev.Type, ev)
		}
	}
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			st.ping()
		case ev := <-sub.C:
			if exact[ev.Topic] {
				if err := st.send(ev.ID, ev.Type, ev); err != nil {
					return
				}
			}
		}
	}
}

// appLogs streams live logs, optionally for one pod, a time window, or
// the previous (dead) container.
func (s *Server) appLogs(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	if ok, _ := s.checkProject(w, u, s.projectOfApp(id), view); !ok {
		return
	}
	var ns, name string
	s.P.Store.Read(func(d *store.Data) {
		a := d.Apps[id]
		ns, name = d.Namespace(a.ProjectID), a.Name
	})
	q := r.URL.Query()
	since, _ := time.ParseDuration(q.Get("since"))
	previous := q.Get("previous") == "1" || q.Get("previous") == "true"
	st, ok := newSSE(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				st.ping()
			}
		}
	}()
	err := s.P.Driver.Logs(ctx, ns, name, q.Get("pod"), previous, since, func(pod, line string) {
		st.send(0, "log", map[string]any{"pod": pod, "text": line, "at": time.Now()})
	})
	if err != nil && ctx.Err() == nil {
		st.send(0, "log", map[string]any{"text": "(logs unavailable: " + err.Error() + ")", "at": time.Now()})
	}
	st.send(0, "end", map[string]bool{"end": true})
}

func (s *Server) deployLogs(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	var appID, status string
	s.P.Store.Read(func(d *store.Data) {
		if dep := d.Deploys[id]; dep != nil {
			appID, status = dep.AppID, dep.Status
		}
	})
	if ok, _ := s.checkProject(w, u, s.projectOfApp(appID), view); !ok {
		return
	}
	st, ok := newSSE(w)
	if !ok {
		return
	}
	c, tail := s.P.Logs.Subscribe("deploy:" + id)
	defer s.P.Logs.Unsubscribe("deploy:"+id, c)
	for _, l := range tail {
		if strings.HasPrefix(l.Text, "::done::") {
			st.send(0, "end", l)
			return
		}
		st.send(0, "log", l)
	}
	if status == "succeeded" || status == "failed" || status == "superseded" {
		if len(tail) == 0 {
			st.send(0, "log", map[string]any{"text": "(build logs are kept in memory and were cleared by a control-plane restart)"})
		}
		st.send(0, "end", map[string]bool{"end": true})
		return
	}
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			st.ping()
		case l := <-c:
			if strings.HasPrefix(l.Text, "::done::") {
				st.send(0, "end", l)
				return
			}
			st.send(0, "log", l)
		}
	}
}

// ---- websocket terminals ----

// wsStdio bridges a websocket to a line-oriented process: each text
// message is a chunk of stdin; output is sent back as text messages.
type wsStdio struct {
	conn *ws.Conn
	pr   *io.PipeReader
	pw   *io.PipeWriter
}

func (x *wsStdio) Write(b []byte) (int, error) {
	if err := x.conn.WriteMessage(ws.OpText, b); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (s *Server) terminal(w http.ResponseWriter, r *http.Request, run func(ctx context.Context, io runtime.Stdio) error, banner string) {
	conn, err := ws.Upgrade(w, r)
	if err != nil {
		return
	}
	defer conn.Close()
	pr, pw := io.Pipe()
	x := &wsStdio{conn: conn, pr: pr, pw: pw}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		defer pw.Close()
		defer cancel()
		for {
			op, msg, err := conn.ReadMessage()
			if err != nil || op == ws.OpClose {
				return
			}
			if _, err := pw.Write(msg); err != nil {
				return
			}
		}
	}()
	if banner != "" {
		x.Write([]byte(banner))
	}
	if err := run(ctx, runtime.Stdio{Stdin: pr, Stdout: x}); err != nil && ctx.Err() == nil {
		x.Write([]byte("\r\n[" + err.Error() + "]\r\n"))
	}
	x.Write([]byte("\r\n[session ended]\r\n"))
	conn.CloseWithMessage(1000, "done")
}

func (s *Server) appShell(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	ok, orgID := s.checkProject(w, u, s.projectOfApp(id), edit)
	if !ok {
		return
	}
	var ns, name string
	s.P.Store.Read(func(d *store.Data) {
		a := d.Apps[id]
		ns, name = d.Namespace(a.ProjectID), a.Name
	})
	pod := r.URL.Query().Get("pod")
	s.P.Audit(orgID, u, "app.shell", name, map[string]string{"pod": pod})
	s.terminal(w, r, func(ctx context.Context, io runtime.Stdio) error {
		return s.P.Driver.Exec(ctx, ns, name, pod, nil, io)
	}, "Connected to "+name+". Changes are lost on the next restart.\r\n")
}
