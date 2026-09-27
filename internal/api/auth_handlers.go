package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/auth"
	"github.com/evanxdsouza/wackcluborchard/internal/platform"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

type PublicUser struct {
	ID          string          `json:"id"`
	Username    string          `json:"username"`
	Name        string          `json:"name"`
	Email       string          `json:"email,omitempty"`
	Superadmin  bool            `json:"superadmin"`
	AvatarSeed  string          `json:"avatarSeed"`
	GitHubLogin string          `json:"githubLogin,omitempty"`
	HasPassword bool            `json:"hasPassword"`
	Passkeys    []PublicPasskey `json:"passkeys"`
	SSHKeys     []store.SSHKey  `json:"sshKeys"`
	CreatedAt   time.Time       `json:"createdAt"`
}

type PublicPasskey struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

func publicUser(u *store.User) PublicUser {
	p := PublicUser{ID: u.ID, Username: u.Username, Name: u.Name, Email: u.Email, Superadmin: u.Superadmin, AvatarSeed: u.AvatarSeed,
		GitHubLogin: u.GitHubLogin, HasPassword: u.PasswordHash != "", SSHKeys: u.SSHKeys, CreatedAt: u.CreatedAt, Passkeys: []PublicPasskey{}}
	if p.SSHKeys == nil {
		p.SSHKeys = []store.SSHKey{}
	}
	for _, k := range u.Passkeys {
		p.Passkeys = append(p.Passkeys, PublicPasskey{k.ID, k.Name, k.CreatedAt})
	}
	return p
}

// brief is how other people are shown: no email for non-admins.
type Brief struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	Name       string `json:"name"`
	AvatarSeed string `json:"avatarSeed"`
}

func brief(u *store.User) Brief {
	if u == nil {
		return Brief{Username: "unknown", Name: "Unknown"}
	}
	return Brief{u.ID, u.Username, u.Name, u.AvatarSeed}
}

var usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

func (s *Server) authState(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	var out struct {
		User         *PublicUser `json:"user"`
		NeedsClaim   bool        `json:"needsClaim"`
		FirstUser    bool        `json:"firstUser"`
		SignupMode   string      `json:"signupMode"`
		InstanceName string      `json:"instanceName"`
		GitHub       bool        `json:"github"`
		SSO          bool        `json:"sso"`
		Demo         bool        `json:"demo"`
		Runtime      string      `json:"runtime"`
		Version      string      `json:"version"`
		AppDomain    string      `json:"appDomain"`
		NeedsPasskey bool        `json:"needsCredential"`
		Secure       bool        `json:"secureContext"`
	}
	s.P.Store.Read(func(d *store.Data) {
		out.SignupMode = d.Settings.SignupMode
		out.InstanceName = d.Settings.InstanceName
		out.GitHub = d.Settings.GitHub.ClientID != ""
		out.FirstUser = len(d.Users) == 0
		out.AppDomain = d.Settings.AppDomain
		hasSuper := false
		for _, x := range d.Users {
			if x.Superadmin {
				hasSuper = true
			}
		}
		out.NeedsClaim = !hasSuper
		for _, o := range d.Orgs {
			if o.SSO.Enabled {
				out.SSO = true
			}
		}
		if u != nil {
			if cur := d.Users[u.ID]; cur != nil {
				pu := publicUser(cur)
				out.User = &pu
				out.NeedsPasskey = cur.Superadmin && cur.PasswordHash == "" && len(cur.Passkeys) == 0
			}
		}
	})
	out.Demo = s.Cfg.Demo
	out.Runtime = s.P.Driver.Name()
	out.Version = s.P.Cfg.Version
	out.Secure = strings.HasPrefix(s.Cfg.FrontendURL, "https://") || strings.Contains(s.Cfg.FrontendURL, "localhost")
	writeJSON(w, 200, out)
}

func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	if !s.limiter.allow("signup:"+clientIP(r), 10, time.Hour) {
		writeErr(w, 429, "too many signups from this address; try again later")
		return
	}
	var in struct {
		Username string `json:"username"`
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Invite   string `json:"invite"`
	}
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	if !usernameRe.MatchString(in.Username) {
		writeErr(w, 400, "usernames are 2 to 32 lowercase letters, digits, dashes or underscores")
		return
	}
	if len(in.Password) < 10 {
		writeErr(w, 400, "use a password of at least 10 characters")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		s.fail(w, err)
		return
	}
	var user *store.User
	first := false
	err = s.P.Store.Write(func(d *store.Data) error {
		first = len(d.Users) == 0
		var inv *store.Invite
		if in.Invite != "" {
			for _, x := range d.Invites {
				if auth.Equal(x.Token, in.Invite) {
					inv = x
				}
			}
			if inv == nil {
				return platform.Invalid("that invite link is not valid")
			}
		}
		if !first && inv == nil {
			switch d.Settings.SignupMode {
			case "closed":
				return platform.Invalid("signups are closed on this instance")
			case "invite":
				return platform.Invalid("this instance is invite-only; ask an admin for an invite link")
			}
		}
		if d.UserByName(in.Username) != nil {
			return platform.Invalid("that username is taken")
		}
		if in.Email != "" && d.UserByName(in.Email) != nil {
			return platform.Invalid("an account with that email exists")
		}
		name := strings.TrimSpace(in.Name)
		if name == "" {
			name = in.Username
		}
		user = &store.User{ID: store.NewID("usr"), Username: in.Username, Name: name, Email: strings.TrimSpace(in.Email), PasswordHash: hash, AvatarSeed: platform.RandHex(4), CreatedAt: time.Now()}
		d.Users[user.ID] = user
		if inv != nil {
			m := &store.Membership{ID: store.NewID("mem"), OrgID: inv.OrgID, UserID: user.ID, Role: inv.Role, CreatedAt: time.Now()}
			d.Memberships[m.ID] = m
			delete(d.Invites, inv.ID)
			d.AddAudit(&store.AuditEntry{OrgID: inv.OrgID, ActorID: user.ID, Actor: user.Username, Action: "member.joined", Target: user.Username})
		}
		user = store.Clone(user)
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	// every user gets a personal organization
	org, _ := s.P.CreateOrg(user, user.Name+"'s Wack Club Orchard", user.Username)
	if s.Cfg.Demo && first && org != nil {
		s.P.SeedDemo(user, org)
	}
	s.startSession(w, user.ID)
	writeJSON(w, 201, publicUser(user))
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	if !s.limiter.allow("login:"+clientIP(r), 20, 10*time.Minute) || !s.limiter.allow("login-user:"+strings.ToLower(in.Username), 10, 10*time.Minute) {
		writeErr(w, 429, "too many attempts; wait a few minutes")
		return
	}
	var user *store.User
	s.P.Store.Read(func(d *store.Data) {
		if u := d.UserByName(strings.TrimSpace(in.Username)); u != nil && !u.Disabled {
			user = store.Clone(u)
		}
	})
	if user == nil || user.PasswordHash == "" || !auth.CheckPassword(user.PasswordHash, in.Password) {
		writeErr(w, 401, "wrong username or password")
		return
	}
	s.startSession(w, user.ID)
	writeJSON(w, 200, publicUser(user))
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.P.Store.Write(func(d *store.Data) error {
			delete(d.Sessions, auth.HashToken(c.Value))
			return nil
		})
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// MintSetupToken creates the single-use claim token for an instance with
// no superadmin. It is valid for 24 hours.
func MintSetupToken(st *store.Store) (string, bool) {
	tok := auth.Token("", 24)
	minted := false
	st.Write(func(d *store.Data) error {
		for _, u := range d.Users {
			if u.Superadmin {
				return nil
			}
		}
		exp := time.Now().Add(24 * time.Hour)
		d.Settings.SetupTokenHash = auth.HashToken(tok)
		d.Settings.SetupExpires = &exp
		minted = true
		return nil
	})
	return tok, minted
}

// MintRecoveryToken always mints a claim token, even with a superadmin;
// used by wackcluborchardctl claim when someone is locked out.
func MintRecoveryToken(st *store.Store) string {
	tok := auth.Token("", 24)
	st.Write(func(d *store.Data) error {
		exp := time.Now().Add(24 * time.Hour)
		d.Settings.SetupTokenHash = auth.HashToken(tok)
		d.Settings.SetupExpires = &exp
		return nil
	})
	return tok
}

// claim redeems the setup token. Signed in, it makes the current account
// superadmin; signed out (recovery), it signs in as the first superadmin.
func (s *Server) claim(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	if !s.limiter.allow("claim:"+clientIP(r), 10, time.Hour) {
		writeErr(w, 429, "too many attempts")
		return
	}
	u := userOf(r)
	var target string
	err := s.P.Store.Write(func(d *store.Data) error {
		st := d.Settings
		if st.SetupTokenHash == "" || st.SetupExpires == nil || time.Now().After(*st.SetupExpires) || !auth.Equal(st.SetupTokenHash, auth.HashToken(in.Token)) {
			return platform.Invalid("that claim link is invalid, used or expired; run `wackcluborchardctl claim` for a new one")
		}
		d.Settings.SetupTokenHash = ""
		d.Settings.SetupExpires = nil
		if u != nil {
			d.Users[u.ID].Superadmin = true
			target = u.ID
		} else {
			for _, x := range d.Users {
				if x.Superadmin && (target == "" || x.CreatedAt.Before(d.Users[target].CreatedAt)) {
					target = x.ID
				}
			}
			if target == "" {
				return platform.Invalid("sign up first, then open the claim link")
			}
		}
		d.AddAudit(&store.AuditEntry{Actor: "system", Action: "instance.claimed", Target: d.Users[target].Username})
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	if u == nil {
		s.startSession(w, target)
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---- me ----

func (s *Server) me(w http.ResponseWriter, r *http.Request, u *store.User) {
	var out struct {
		PublicUser
		Orgs []map[string]any `json:"orgs"`
	}
	s.P.Store.Read(func(d *store.Data) {
		out.PublicUser = publicUser(d.Users[u.ID])
		for _, o := range d.UserOrgs(u.ID) {
			out.Orgs = append(out.Orgs, map[string]any{"id": o.ID, "slug": o.Slug, "name": o.Name, "role": orgRole(d, u, o.ID)})
		}
		if u.Superadmin {
			seen := map[string]bool{}
			for _, o := range out.Orgs {
				seen[o["id"].(string)] = true
			}
			for _, o := range store.Values(d.Orgs, func(a, b *store.Org) bool { return a.CreatedAt.Before(b.CreatedAt) }) {
				if !seen[o.ID] {
					out.Orgs = append(out.Orgs, map[string]any{"id": o.ID, "slug": o.Slug, "name": o.Name, "role": "owner", "viaSuperadmin": true})
				}
			}
		}
	})
	if out.Orgs == nil {
		out.Orgs = []map[string]any{}
	}
	writeJSON(w, 200, out)
}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct {
		Name       *string `json:"name"`
		Email      *string `json:"email"`
		AvatarSeed *string `json:"avatarSeed"`
	}
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	var out PublicUser
	s.P.Store.Write(func(d *store.Data) error {
		x := d.Users[u.ID]
		if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
			x.Name = strings.TrimSpace(*in.Name)
		}
		if in.Email != nil {
			x.Email = strings.TrimSpace(*in.Email)
		}
		if in.AvatarSeed != nil {
			x.AvatarSeed = *in.AvatarSeed
		}
		out = publicUser(x)
		return nil
	})
	writeJSON(w, 200, out)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	if len(in.New) < 10 {
		writeErr(w, 400, "use a password of at least 10 characters")
		return
	}
	var cur string
	s.P.Store.Read(func(d *store.Data) { cur = d.Users[u.ID].PasswordHash })
	if cur != "" && !auth.CheckPassword(cur, in.Current) {
		writeErr(w, 400, "the current password is wrong")
		return
	}
	h, _ := auth.HashPassword(in.New)
	s.P.Store.Write(func(d *store.Data) error {
		d.Users[u.ID].PasswordHash = h
		return nil
	})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request, u *store.User) {
	out := []store.APIToken{}
	s.P.Store.Read(func(d *store.Data) {
		for _, t := range store.Values(d.APITokens, func(a, b *store.APIToken) bool { return a.CreatedAt.After(b.CreatedAt) }) {
			if t.UserID == u.ID {
				c := *t
				c.Hash = ""
				out = append(out, c)
			}
		}
	})
	writeJSON(w, 200, out)
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct {
		Name string `json:"name"`
	}
	readJSON(r, &in)
	if in.Name == "" {
		in.Name = "token"
	}
	tok := auth.Token("wackclubwackcluborchard_", 30)
	t := &store.APIToken{ID: store.NewID("tok"), UserID: u.ID, Name: in.Name, Hash: auth.HashToken(tok), Prefix: tok[:10], CreatedAt: time.Now()}
	s.P.Store.Write(func(d *store.Data) error {
		d.APITokens[t.ID] = t
		return nil
	})
	writeJSON(w, 201, map[string]any{"id": t.ID, "name": t.Name, "token": tok, "prefix": t.Prefix})
}

func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	s.P.Store.Write(func(d *store.Data) error {
		if t := d.APITokens[id]; t != nil && t.UserID == u.ID {
			delete(d.APITokens, id)
		}
		return nil
	})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

var sshKeyRe = regexp.MustCompile(`^(ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp256|ecdsa-sha2-nistp384|ecdsa-sha2-nistp521|sk-ssh-ed25519@openssh.com|sk-ecdsa-sha2-nistp256@openssh.com) ([A-Za-z0-9+/=]+)( .*)?$`)

func (s *Server) addSSHKey(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct {
		Name      string `json:"name"`
		PublicKey string `json:"publicKey"`
	}
	readJSON(r, &in)
	in.PublicKey = strings.TrimSpace(in.PublicKey)
	m := sshKeyRe.FindStringSubmatch(in.PublicKey)
	if m == nil {
		writeErr(w, 400, "that does not look like an OpenSSH public key")
		return
	}
	raw, err := base64.StdEncoding.DecodeString(m[2])
	if err != nil {
		writeErr(w, 400, "the key's base64 is invalid")
		return
	}
	sum := sha256.Sum256(raw)
	fp := "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
	if in.Name == "" {
		in.Name = strings.TrimSpace(m[3])
		if in.Name == "" {
			in.Name = m[1]
		}
	}
	k := store.SSHKey{ID: store.NewID("ssh"), Name: in.Name, PublicKey: in.PublicKey, Fingerprint: fp, CreatedAt: time.Now()}
	err = s.P.Store.Write(func(d *store.Data) error {
		x := d.Users[u.ID]
		for _, e := range x.SSHKeys {
			if e.Fingerprint == fp {
				return platform.Invalid("that key is already added")
			}
		}
		x.SSHKeys = append(x.SSHKeys, k)
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 201, k)
}

func (s *Server) deleteSSHKey(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	s.P.Store.Write(func(d *store.Data) error {
		x := d.Users[u.ID]
		var keep []store.SSHKey
		for _, k := range x.SSHKeys {
			if k.ID != id {
				keep = append(keep, k)
			}
		}
		x.SSHKeys = keep
		return nil
	})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---- passkeys ----

func (s *Server) rp(r *http.Request) (id string, origins []string) {
	host := r.Host
	origin := "http://" + r.Host
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		origin = "https://" + r.Host
	}
	if s.Cfg.FrontendURL != "" {
		if u, err := url.Parse(s.Cfg.FrontendURL); err == nil {
			origins = append(origins, u.Scheme+"://"+u.Host)
		}
	}
	origins = append(origins, origin)
	if i := strings.LastIndex(host, ":"); i > 0 && !strings.HasSuffix(host, "]") {
		host = host[:i]
	}
	return host, origins
}

func (s *Server) newChallenge(userID, extra string) string {
	c := auth.Token("", 32)
	s.chMu.Lock()
	defer s.chMu.Unlock()
	now := time.Now()
	for k, v := range s.challenges {
		if now.After(v.expires) {
			delete(s.challenges, k)
		}
	}
	s.challenges[c] = challenge{value: c, userID: userID, expires: now.Add(5 * time.Minute), extra: extra}
	return c
}

func (s *Server) takeChallenge(c string) (challenge, bool) {
	s.chMu.Lock()
	defer s.chMu.Unlock()
	ch, ok := s.challenges[c]
	delete(s.challenges, c)
	if !ok || time.Now().After(ch.expires) {
		return challenge{}, false
	}
	return ch, true
}

func (s *Server) passkeyRegisterBegin(w http.ResponseWriter, r *http.Request, u *store.User) {
	rpID, _ := s.rp(r)
	var exclude []map[string]string
	s.P.Store.Read(func(d *store.Data) {
		for _, k := range d.Users[u.ID].Passkeys {
			exclude = append(exclude, map[string]string{"type": "public-key", "id": k.ID})
		}
	})
	writeJSON(w, 200, map[string]any{
		"challenge":          s.newChallenge(u.ID, ""),
		"rp":                 map[string]string{"id": rpID, "name": "Wack Club Orchard"},
		"user":               map[string]string{"id": base64.RawURLEncoding.EncodeToString([]byte(u.ID)), "name": u.Username, "displayName": u.Name},
		"excludeCredentials": exclude,
	})
}

func (s *Server) passkeyRegisterFinish(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct {
		auth.Registration
		Challenge string `json:"challenge"`
		Name      string `json:"name"`
	}
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	ch, ok := s.takeChallenge(in.Challenge)
	if !ok || ch.userID != u.ID {
		writeErr(w, 400, "the passkey challenge expired; try again")
		return
	}
	rpID, origins := s.rp(r)
	pk, count, err := auth.VerifyRegistration(in.Registration, in.Challenge, rpID, origins)
	if err != nil {
		writeErr(w, 400, "passkey rejected: "+err.Error())
		return
	}
	if in.Name == "" {
		in.Name = "Passkey"
	}
	s.P.Store.Write(func(d *store.Data) error {
		x := d.Users[u.ID]
		x.Passkeys = append(x.Passkeys, store.Passkey{ID: in.ID, Name: in.Name, PublicKey: pk, SignCount: count, CreatedAt: time.Now()})
		return nil
	})
	writeJSON(w, 201, map[string]bool{"ok": true})
}

func (s *Server) passkeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	rpID, _ := s.rp(r)
	writeJSON(w, 200, map[string]any{"challenge": s.newChallenge("", ""), "rpId": rpID})
}

func (s *Server) passkeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	var in struct {
		auth.Assertion
		Challenge string `json:"challenge"`
	}
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	if _, ok := s.takeChallenge(in.Challenge); !ok {
		writeErr(w, 400, "the passkey challenge expired; try again")
		return
	}
	rpID, origins := s.rp(r)
	var userID string
	var key store.Passkey
	s.P.Store.Read(func(d *store.Data) {
		for _, u := range d.Users {
			for _, k := range u.Passkeys {
				if k.ID == in.ID && !u.Disabled {
					userID, key = u.ID, k
				}
			}
		}
	})
	if userID == "" {
		writeErr(w, 401, "that passkey is not registered here")
		return
	}
	count, err := auth.VerifyAssertion(in.Assertion, key.PublicKey, key.SignCount, in.Challenge, rpID, origins)
	if err != nil {
		writeErr(w, 401, "passkey rejected: "+err.Error())
		return
	}
	s.P.Store.Write(func(d *store.Data) error {
		for i := range d.Users[userID].Passkeys {
			if d.Users[userID].Passkeys[i].ID == in.ID {
				d.Users[userID].Passkeys[i].SignCount = count
			}
		}
		return nil
	})
	s.startSession(w, userID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) deletePasskey(w http.ResponseWriter, r *http.Request, u *store.User) {
	id := r.PathValue("id")
	err := s.P.Store.Write(func(d *store.Data) error {
		x := d.Users[u.ID]
		if x.PasswordHash == "" && len(x.Passkeys) <= 1 {
			return platform.Invalid("this is your only way to sign in; add a password or another passkey first")
		}
		var keep []store.Passkey
		for _, k := range x.Passkeys {
			if k.ID != id {
				keep = append(keep, k)
			}
		}
		x.Passkeys = keep
		return nil
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---- SSO (OpenID Connect) ----

type oidcDiscovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

func discover(ctx context.Context, issuer string) (*oidcDiscovery, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(issuer, "/")+"/.well-known/openid-configuration", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var d oidcDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, err
	}
	if d.AuthorizationEndpoint == "" || d.TokenEndpoint == "" {
		return nil, fmt.Errorf("issuer %s has no usable OpenID configuration", issuer)
	}
	return &d, nil
}

func (s *Server) ssoStart(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("email")))
	domain := email
	if i := strings.LastIndex(email, "@"); i >= 0 {
		domain = email[i+1:]
	}
	var org *store.Org
	s.P.Store.Read(func(d *store.Data) {
		for _, o := range d.Orgs {
			if o.SSO.Enabled && strings.EqualFold(o.SSO.Domain, domain) {
				org = store.Clone(o)
			}
		}
	})
	if org == nil {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("No organization uses single sign-on for "+domain), http.StatusFound)
		return
	}
	disc, err := discover(r.Context(), org.SSO.Issuer)
	if err != nil {
		http.Redirect(w, r, "/login?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	state := s.newChallenge("", org.ID)
	q := url.Values{"client_id": {org.SSO.ClientID}, "response_type": {"code"}, "scope": {"openid email profile"},
		"redirect_uri": {s.baseURL(r) + "/api/auth/sso/callback"}, "state": {state}, "login_hint": {email}}
	http.Redirect(w, r, disc.AuthorizationEndpoint+"?"+q.Encode(), http.StatusFound)
}

func (s *Server) baseURL(r *http.Request) string {
	if s.Cfg.FrontendURL != "" {
		return strings.TrimRight(s.Cfg.FrontendURL, "/")
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (s *Server) ssoCallback(w http.ResponseWriter, r *http.Request) {
	ch, ok := s.takeChallenge(r.URL.Query().Get("state"))
	if !ok {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("Sign-in expired; try again"), http.StatusFound)
		return
	}
	var org *store.Org
	s.P.Store.Read(func(d *store.Data) {
		if o := d.Orgs[ch.extra]; o != nil {
			org = store.Clone(o)
		}
	})
	fail := func(msg string) {
		http.Redirect(w, r, "/login?error="+url.QueryEscape(msg), http.StatusFound)
	}
	if org == nil {
		fail("organization not found")
		return
	}
	disc, err := discover(r.Context(), org.SSO.Issuer)
	if err != nil {
		fail(err.Error())
		return
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {r.URL.Query().Get("code")}, "redirect_uri": {s.baseURL(r) + "/api/auth/sso/callback"},
		"client_id": {org.SSO.ClientID}, "client_secret": {org.SSO.ClientSecret}}
	resp, err := http.PostForm(disc.TokenEndpoint, form)
	if err != nil {
		fail(err.Error())
		return
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(resp.Body).Decode(&tok)
	resp.Body.Close()
	if tok.AccessToken == "" {
		fail("the identity provider did not return a token")
		return
	}
	req, _ := http.NewRequestWithContext(r.Context(), "GET", disc.UserinfoEndpoint, nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	uresp, err := http.DefaultClient.Do(req)
	if err != nil {
		fail(err.Error())
		return
	}
	var info struct {
		Sub      string `json:"sub"`
		Email    string `json:"email"`
		Verified *bool  `json:"email_verified"`
		Name     string `json:"name"`
		Username string `json:"preferred_username"`
	}
	json.NewDecoder(uresp.Body).Decode(&info)
	uresp.Body.Close()
	if info.Email == "" || (info.Verified != nil && !*info.Verified) {
		fail("the identity provider did not return a verified email")
		return
	}
	if !strings.HasSuffix(strings.ToLower(info.Email), "@"+strings.ToLower(org.SSO.Domain)) {
		fail("that email is not in " + org.SSO.Domain)
		return
	}
	var userID string
	s.P.Store.Write(func(d *store.Data) error {
		u := d.UserByName(info.Email)
		if u == nil {
			base := store.Slugify(strings.Split(info.Email, "@")[0])
			name := base
			for i := 2; d.UserByName(name) != nil; i++ {
				name = fmt.Sprintf("%s%d", base, i)
			}
			u = &store.User{ID: store.NewID("usr"), Username: name, Name: info.Name, Email: info.Email, AvatarSeed: platform.RandHex(4), ExternalID: info.Sub, CreatedAt: time.Now()}
			if u.Name == "" {
				u.Name = name
			}
			d.Users[u.ID] = u
		}
		if u.Disabled {
			return nil
		}
		if d.Membership(org.ID, u.ID) == nil {
			role := org.SSO.DefaultRole
			if role == "" {
				role = store.RoleMember
			}
			m := &store.Membership{ID: store.NewID("mem"), OrgID: org.ID, UserID: u.ID, Role: role, CreatedAt: time.Now()}
			d.Memberships[m.ID] = m
			d.AddAudit(&store.AuditEntry{OrgID: org.ID, ActorID: u.ID, Actor: u.Username, Action: "member.joined", Target: u.Username, Meta: map[string]string{"via": "sso"}})
		}
		userID = u.ID
		return nil
	})
	if userID == "" {
		fail("this account is disabled")
		return
	}
	s.startSession(w, userID)
	http.Redirect(w, r, "/", http.StatusFound)
}

// ---- auth wall (Traefik forwardAuth) ----

func (s *Server) wallSecret() []byte {
	var sec string
	s.P.Store.Write(func(d *store.Data) error {
		if d.Settings.Secret == "" {
			d.Settings.Secret = platform.RandHex(32)
		}
		sec = d.Settings.Secret
		return nil
	})
	return []byte(sec)
}

func (s *Server) signWall(parts ...string) string {
	payload := strings.Join(parts, "|")
	m := hmac.New(sha256.New, s.wallSecret())
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + hex.EncodeToString(m.Sum(nil))
}

func (s *Server) verifyWall(v string) ([]string, bool) {
	i := strings.LastIndex(v, ".")
	if i < 0 {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(v[:i])
	if err != nil {
		return nil, false
	}
	m := hmac.New(sha256.New, s.wallSecret())
	m.Write(payload)
	if !hmac.Equal([]byte(hex.EncodeToString(m.Sum(nil))), []byte(v[i+1:])) {
		return nil, false
	}
	parts := strings.Split(string(payload), "|")
	if len(parts) < 3 {
		return nil, false
	}
	exp, err := time.Parse(time.RFC3339, parts[len(parts)-1])
	if err != nil || time.Now().After(exp) {
		return nil, false
	}
	return parts, true
}

// forwardAuth is called by the ingress for every request to a walled app.
func (s *Server) forwardAuth(w http.ResponseWriter, r *http.Request) {
	appID := r.URL.Query().Get("app")
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "https"
	}
	orig := proto + "://" + r.Header.Get("X-Forwarded-Host") + r.Header.Get("X-Forwarded-Uri")
	// a fresh ticket from the dashboard: trade it for a cookie on this host
	if ou, err := url.Parse(orig); err == nil {
		if t := ou.Query().Get("__wackcluborchard_ticket"); t != "" {
			if parts, ok := s.verifyWall(t); ok && parts[1] == appID {
				q := ou.Query()
				q.Del("__wackcluborchard_ticket")
				ou.RawQuery = q.Encode()
				http.SetCookie(w, &http.Cookie{Name: "wackcluborchard_wall", Value: s.signWall(parts[0], appID, time.Now().Add(12*time.Hour).Format(time.RFC3339)), Path: "/", HttpOnly: true, Secure: proto == "https", SameSite: http.SameSiteLaxMode})
				http.Redirect(w, r, ou.String(), http.StatusFound)
				return
			}
		}
	}
	if c, err := r.Cookie("wackcluborchard_wall"); err == nil {
		if parts, ok := s.verifyWall(c.Value); ok && parts[1] == appID {
			var user *store.User
			s.P.Store.Read(func(d *store.Data) {
				if u := d.Users[parts[0]]; u != nil && !u.Disabled {
					user = store.Clone(u)
				}
			})
			if user != nil {
				w.Header().Set("X-Wackclubwackcluborchard-User", user.Username)
				w.Header().Set("X-Wackclubwackcluborchard-Email", user.Email)
				w.WriteHeader(200)
				return
			}
		}
	}
	http.Redirect(w, r, s.baseURL(r)+"/api/auth/wall?app="+url.QueryEscape(appID)+"&rd="+url.QueryEscape(orig), http.StatusFound)
}

// wall runs on the dashboard host: a signed-in project member gets a
// ticket and is sent back to the app.
func (s *Server) wall(w http.ResponseWriter, r *http.Request) {
	appID := r.URL.Query().Get("app")
	rd := r.URL.Query().Get("rd")
	u := userOf(r)
	if u == nil {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
		return
	}
	ok := false
	var hosts []string
	s.P.Store.Read(func(d *store.Data) {
		if a := d.Apps[appID]; a != nil {
			ok = canViewProject(d, u, d.Projects[a.ProjectID])
			for _, dm := range a.Domains {
				hosts = append(hosts, dm.Host)
			}
		}
	})
	target, err := url.Parse(rd)
	if !ok || err != nil {
		writeErr(w, 403, "you do not have access to this app")
		return
	}
	valid := false
	for _, h := range hosts {
		if target.Hostname() == h {
			valid = true
		}
	}
	if !valid {
		writeErr(w, 400, "redirect target is not one of this app's domains")
		return
	}
	q := target.Query()
	q.Set("__wackcluborchard_ticket", s.signWall(u.ID, appID, time.Now().Add(time.Minute).Format(time.RFC3339)))
	target.RawQuery = q.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}
