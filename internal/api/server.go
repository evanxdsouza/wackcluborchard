// Package api is Orchard's HTTP surface: the JSON API the dashboard and
// CLI use, server-sent events, WebSocket terminals, SCIM, the GitHub
// webhook, the MCP endpoint, and the static frontend.
package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/auth"
	"github.com/evanxdsouza/wackcluborchard/internal/platform"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

type Config struct {
	FrontendURL  string // public base URL of the dashboard
	SecureCookie bool
	Web          fs.FS // built frontend
	Demo         bool
}

type Server struct {
	P   *platform.Platform
	Cfg Config
	mux *http.ServeMux

	chMu       sync.Mutex
	challenges map[string]challenge // webauthn and oauth state
	limiter    *rateLimiter
}

type challenge struct {
	value   string
	userID  string
	expires time.Time
	extra   string
}

type ctxKey int

const (
	userKey ctxKey = iota
	tokenKey
)

func New(p *platform.Platform, cfg Config) *Server {
	s := &Server{P: p, Cfg: cfg, mux: http.NewServeMux(), challenges: map[string]challenge{}, limiter: newRateLimiter()}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	r = s.authenticate(r)
	// CSRF: cookie-authenticated writes must come from our own origin.
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && r.Context().Value(tokenKey) == nil {
		if o := r.Header.Get("Origin"); o != "" && !s.sameOrigin(r, o) && !strings.HasPrefix(r.URL.Path, "/api/github/webhook") && !strings.HasPrefix(r.URL.Path, "/scim/") && r.URL.Path != "/mcp" {
			writeErr(w, http.StatusForbidden, "cross-origin request refused")
			return
		}
	}
	sw := &statusWriter{ResponseWriter: w, code: 200}
	s.mux.ServeHTTP(sw, r)
	if strings.HasPrefix(r.URL.Path, "/api/") && !strings.Contains(r.URL.Path, "/logs") && !strings.HasSuffix(r.URL.Path, "/events") && os.Getenv("ORCHARD_QUIET") == "" {
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.code, time.Since(start).Round(time.Millisecond))
	}
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(c int) { w.code = c; w.ResponseWriter.WriteHeader(c) }
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijack unsupported")
	}
	w.code = 101
	return h.Hijack()
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (s *Server) sameOrigin(r *http.Request, origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Host == r.Host {
		return true
	}
	if s.Cfg.FrontendURL != "" {
		if f, err := url.Parse(s.Cfg.FrontendURL); err == nil && f.Host == u.Host {
			return true
		}
	}
	return false
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	var pe *platform.Error
	switch {
	case errors.As(err, &pe) && errors.Is(pe.Kind, platform.ErrQuota):
		writeErr(w, http.StatusUnprocessableEntity, pe.Msg)
	case errors.As(err, &pe) && errors.Is(pe.Kind, platform.ErrForbidden):
		writeErr(w, http.StatusForbidden, pe.Msg)
	case errors.As(err, &pe):
		writeErr(w, http.StatusBadRequest, pe.Msg)
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<20))
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return platform.Invalid("bad JSON: %v", err)
	}
	return nil
}

func userOf(r *http.Request) *store.User {
	u, _ := r.Context().Value(userKey).(*store.User)
	return u
}

func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return strings.TrimSpace(strings.Split(f, ",")[0])
	}
	h, _, _ := net.SplitHostPort(r.RemoteAddr)
	return h
}

// ---- authentication ----

const cookieName = "orchard_session"

func (s *Server) authenticate(r *http.Request) *http.Request {
	var user *store.User
	viaToken := false
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		tok := strings.TrimPrefix(h, "Bearer ")
		hash := auth.HashToken(tok)
		s.P.Store.Write(func(d *store.Data) error {
			for _, t := range d.APITokens {
				if auth.Equal(t.Hash, hash) {
					if u := d.Users[t.UserID]; u != nil && !u.Disabled {
						now := time.Now()
						if t.LastUsed == nil || now.Sub(*t.LastUsed) > time.Minute {
							t.LastUsed = &now
						}
						user = store.Clone(u)
						viaToken = true
					}
				}
			}
			return nil
		})
	} else if c, err := r.Cookie(cookieName); err == nil {
		s.P.Store.Read(func(d *store.Data) {
			if sess := d.Sessions[auth.HashToken(c.Value)]; sess != nil && time.Now().Before(sess.ExpiresAt) {
				if u := d.Users[sess.UserID]; u != nil && !u.Disabled {
					user = store.Clone(u)
				}
			}
		})
	}
	if user == nil {
		return r
	}
	ctx := context.WithValue(r.Context(), userKey, user)
	if viaToken {
		ctx = context.WithValue(ctx, tokenKey, true)
	}
	return r.WithContext(ctx)
}

func (s *Server) startSession(w http.ResponseWriter, userID string) {
	tok := auth.Token("", 32)
	s.P.Store.Write(func(d *store.Data) error {
		now := time.Now()
		d.Sessions[auth.HashToken(tok)] = &store.Session{ID: auth.HashToken(tok), UserID: userID, CreatedAt: now, ExpiresAt: now.Add(30 * 24 * time.Hour)}
		for k, sess := range d.Sessions {
			if now.After(sess.ExpiresAt) {
				delete(d.Sessions, k)
			}
		}
		return nil
	})
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: tok, Path: "/", HttpOnly: true, Secure: s.Cfg.SecureCookie, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600})
}

// ---- access control ----

type handler func(w http.ResponseWriter, r *http.Request, u *store.User)

func (s *Server) authed(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := userOf(r)
		if u == nil {
			writeErr(w, http.StatusUnauthorized, "sign in first")
			return
		}
		h(w, r, u)
	}
}

func (s *Server) superadmin(h handler) http.HandlerFunc {
	return s.authed(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		if !u.Superadmin {
			writeErr(w, http.StatusForbidden, "instance admins only")
			return
		}
		h(w, r, u)
	})
}

// orgRole returns the user's role in an org; superadmins act as owners.
func orgRole(d *store.Data, u *store.User, orgID string) string {
	if u.Superadmin {
		return store.RoleOwner
	}
	if m := d.Membership(orgID, u.ID); m != nil {
		return m.Role
	}
	return ""
}

func canViewProject(d *store.Data, u *store.User, p *store.Project) bool {
	if p == nil {
		return false
	}
	role := orgRole(d, u, p.OrgID)
	if store.RoleRank(role) >= store.RoleRank(store.RoleAdmin) {
		return true
	}
	if role == "" {
		return false
	}
	for _, m := range p.Members {
		if m == u.ID {
			return true
		}
	}
	return false
}

func canEditProject(d *store.Data, u *store.User, p *store.Project) bool {
	return canViewProject(d, u, p) && store.RoleRank(orgRole(d, u, p.OrgID)) >= store.RoleRank(store.RoleMember)
}

// access resolves the project an object belongs to and checks it.
type accessLevel int

const (
	view accessLevel = iota
	edit
)

func (s *Server) checkProject(w http.ResponseWriter, u *store.User, projectID string, lvl accessLevel) (ok bool, orgID string) {
	s.P.Store.Read(func(d *store.Data) {
		p := d.Projects[projectID]
		if p == nil {
			return
		}
		orgID = p.OrgID
		if lvl == edit {
			ok = canEditProject(d, u, p)
		} else {
			ok = canViewProject(d, u, p)
		}
	})
	if !ok {
		writeErr(w, http.StatusNotFound, "not found")
	}
	return ok, orgID
}

func (s *Server) projectOfApp(id string) (pid string) {
	s.P.Store.Read(func(d *store.Data) {
		if a := d.Apps[id]; a != nil {
			pid = a.ProjectID
		}
	})
	return
}

func (s *Server) projectOfDB(id string) (pid string) {
	s.P.Store.Read(func(d *store.Data) {
		if a := d.Databases[id]; a != nil {
			pid = a.ProjectID
		}
	})
	return
}

func (s *Server) projectOfJob(id string) (pid string) {
	s.P.Store.Read(func(d *store.Data) {
		if a := d.Jobs[id]; a != nil {
			pid = a.ProjectID
		}
	})
	return
}

func (s *Server) checkOrg(w http.ResponseWriter, u *store.User, orgRef string, min string) (*store.Org, bool) {
	var org *store.Org
	ok := false
	s.P.Store.Read(func(d *store.Data) {
		o := d.Orgs[orgRef]
		if o == nil {
			o = d.OrgBySlug(orgRef)
		}
		if o == nil {
			return
		}
		role := orgRole(d, u, o.ID)
		if role != "" && store.RoleRank(role) >= store.RoleRank(min) {
			org = store.Clone(o)
			ok = true
		}
	})
	if !ok {
		writeErr(w, http.StatusNotFound, "organization not found")
	}
	return org, ok
}

// ---- rate limiting for credential endpoints ----

type rateLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newRateLimiter() *rateLimiter { return &rateLimiter{hits: map[string][]time.Time{}} }

func (l *rateLimiter) allow(key string, n int, per time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	var keep []time.Time
	for _, t := range l.hits[key] {
		if now.Sub(t) < per {
			keep = append(keep, t)
		}
	}
	if len(keep) >= n {
		l.hits[key] = keep
		return false
	}
	l.hits[key] = append(keep, now)
	return true
}

// ---- static frontend ----

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "no such endpoint")
		return
	}
	if s.Cfg.Web == nil {
		writeErr(w, http.StatusNotFound, "frontend not built")
		return
	}
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" {
		p = "index.html"
	}
	f, err := s.Cfg.Web.Open(p)
	if err == nil {
		st, _ := f.Stat()
		f.Close()
		if st != nil && !st.IsDir() {
			if strings.HasPrefix(p, "assets/") || strings.HasSuffix(p, ".woff2") {
				w.Header().Set("Cache-Control", "public, max-age=3600")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			http.ServeFileFS(w, r, s.Cfg.Web, p)
			return
		}
	}
	// SPA fallback
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, s.Cfg.Web, "index.html")
}
