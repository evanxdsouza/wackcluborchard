// Package github talks to GitHub as a GitHub App: the manifest flow that
// creates the app, user sign-in, repository listing, Dockerfile
// inspection, installation tokens for cloning, and webhook verification.
package github

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

const api = "https://api.github.com"

var client = &http.Client{Timeout: 20 * time.Second}

func do(ctx context.Context, method, u, auth string, body any, out any) error {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		json.Unmarshal(data, &e)
		return fmt.Errorf("github %s %s: %d %s", method, u, resp.StatusCode, e.Message)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Manifest builds the GitHub App manifest for this instance.
func Manifest(name, baseURL string) map[string]any {
	return map[string]any{
		"name":          name,
		"url":           baseURL,
		"redirect_url":  baseURL + "/api/github/manifest/callback",
		"callback_urls": []string{baseURL + "/api/github/callback"},
		"setup_url":     baseURL + "/api/github/setup",
		"hook_attributes": map[string]any{
			"url": baseURL + "/api/github/webhook",
		},
		"public": false,
		"default_permissions": map[string]string{
			"contents": "read", "metadata": "read", "pull_requests": "write", "statuses": "write",
		},
		"default_events":           []string{"push", "pull_request"},
		"request_oauth_on_install": true,
	}
}

// ConvertManifest exchanges the one-time manifest code for credentials.
func ConvertManifest(ctx context.Context, code string) (store.GitHubAppConfig, error) {
	var out struct {
		ID            int    `json:"id"`
		Slug          string `json:"slug"`
		ClientID      string `json:"client_id"`
		ClientSecret  string `json:"client_secret"`
		PEM           string `json:"pem"`
		WebhookSecret string `json:"webhook_secret"`
	}
	if err := do(ctx, "POST", api+"/app-manifests/"+url.PathEscape(code)+"/conversions", "", nil, &out); err != nil {
		return store.GitHubAppConfig{}, err
	}
	return store.GitHubAppConfig{AppID: strconv.Itoa(out.ID), Slug: out.Slug, ClientID: out.ClientID, ClientSecret: out.ClientSecret, PrivateKey: out.PEM, WebhookSecret: out.WebhookSecret}, nil
}

func AuthorizeURL(cfg store.GitHubAppConfig, redirect, state string) string {
	q := url.Values{"client_id": {cfg.ClientID}, "redirect_uri": {redirect}, "state": {state}}
	return "https://github.com/login/oauth/authorize?" + q.Encode()
}

func ExchangeCode(ctx context.Context, cfg store.GitHubAppConfig, code string) (string, error) {
	form := url.Values{"client_id": {cfg.ClientID}, "client_secret": {cfg.ClientSecret}, "code": {code}}
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://github.com/login/oauth/access_token", strings.NewReader(form.Encode()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error_description"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if out.AccessToken == "" {
		return "", fmt.Errorf("github: %s", out.Error)
	}
	return out.AccessToken, nil
}

func Login(ctx context.Context, token string) (string, error) {
	var u struct {
		Login string `json:"login"`
	}
	err := do(ctx, "GET", api+"/user", "Bearer "+token, nil, &u)
	return u.Login, err
}

type Repo struct {
	FullName      string    `json:"fullName"`
	Private       bool      `json:"private"`
	DefaultBranch string    `json:"defaultBranch"`
	Description   string    `json:"description"`
	PushedAt      time.Time `json:"pushedAt"`
	Language      string    `json:"language"`
}

// Repos lists repositories the user granted to the app.
func Repos(ctx context.Context, token string) ([]Repo, error) {
	var inst struct {
		Installations []struct {
			ID int `json:"id"`
		} `json:"installations"`
	}
	if err := do(ctx, "GET", api+"/user/installations?per_page=100", "Bearer "+token, nil, &inst); err != nil {
		return nil, err
	}
	var out []Repo
	for _, in := range inst.Installations {
		for page := 1; page <= 10; page++ {
			var rs struct {
				Repositories []struct {
					FullName      string    `json:"full_name"`
					Private       bool      `json:"private"`
					DefaultBranch string    `json:"default_branch"`
					Description   string    `json:"description"`
					PushedAt      time.Time `json:"pushed_at"`
					Language      string    `json:"language"`
				} `json:"repositories"`
			}
			if err := do(ctx, "GET", fmt.Sprintf("%s/user/installations/%d/repositories?per_page=100&page=%d", api, in.ID, page), "Bearer "+token, nil, &rs); err != nil {
				return nil, err
			}
			for _, r := range rs.Repositories {
				out = append(out, Repo{r.FullName, r.Private, r.DefaultBranch, r.Description, r.PushedAt, r.Language})
			}
			if len(rs.Repositories) < 100 {
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PushedAt.After(out[j].PushedAt) })
	return out, nil
}

func Branches(ctx context.Context, token, repo string) ([]string, error) {
	var bs []struct {
		Name string `json:"name"`
	}
	if err := do(ctx, "GET", api+"/repos/"+repo+"/branches?per_page=100", "Bearer "+token, nil, &bs); err != nil {
		return nil, err
	}
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.Name
	}
	return out, nil
}

type Commit struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
}

func LatestCommit(ctx context.Context, token, repo, branch string) (Commit, error) {
	var c struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	}
	err := do(ctx, "GET", api+"/repos/"+repo+"/commits/"+url.PathEscape(branch), "Bearer "+token, nil, &c)
	return Commit{c.SHA, strings.SplitN(c.Commit.Message, "\n", 2)[0]}, err
}

func FileContent(ctx context.Context, token, repo, path, ref string) (string, error) {
	var f struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := do(ctx, "GET", api+"/repos/"+repo+"/contents/"+path+"?ref="+url.QueryEscape(ref), "Bearer "+token, nil, &f); err != nil {
		return "", err
	}
	b, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(f.Content, "\n", ""))
	return string(b), err
}

// Inspection is what Wack Club Orchard learns from a repository's Dockerfile.
type Inspection struct {
	Dockerfile string   `json:"dockerfile"`
	Found      bool     `json:"found"`
	Stages     []string `json:"stages"`
	Target     string   `json:"target"` // suggested: the last stage
	Port       int      `json:"port"`
	Env        []string `json:"env"`
}

var (
	fromRe   = regexp.MustCompile(`(?i)^FROM\s+(?:--platform=\S+\s+)?(\S+)(?:\s+AS\s+(\S+))?`)
	exposeRe = regexp.MustCompile(`(?i)^EXPOSE\s+(\d+)`)
	envRe    = regexp.MustCompile(`(?i)^(?:ENV|ARG)\s+([A-Za-z_][A-Za-z0-9_]*)`)
)

// ParseDockerfile finds build stages, the exposed port and declared
// variables. It reports the last stage as the target, since a multi-stage
// build targeted at an early stage produces an image with no app in it.
func ParseDockerfile(src string) Inspection {
	in := Inspection{Found: true}
	seen := map[string]bool{}
	// join continuation lines
	src = strings.ReplaceAll(src, "\\\n", " ")
	for _, l := range strings.Split(src, "\n") {
		l = strings.TrimSpace(l)
		if m := fromRe.FindStringSubmatch(l); m != nil {
			name := m[2]
			if name == "" {
				name = fmt.Sprintf("stage-%d", len(in.Stages))
			}
			in.Stages = append(in.Stages, name)
		}
		if m := exposeRe.FindStringSubmatch(l); m != nil {
			in.Port, _ = strconv.Atoi(m[1])
		}
		if m := envRe.FindStringSubmatch(l); m != nil && !seen[m[1]] {
			seen[m[1]] = true
			in.Env = append(in.Env, m[1])
		}
	}
	if len(in.Stages) > 0 {
		in.Target = in.Stages[len(in.Stages)-1]
		if strings.HasPrefix(in.Target, "stage-") {
			in.Target = ""
		}
	}
	return in
}

func Inspect(ctx context.Context, token, repo, branch, path string) (Inspection, error) {
	if path == "" {
		path = "Dockerfile"
	}
	src, err := FileContent(ctx, token, repo, path, branch)
	if err != nil {
		return Inspection{Dockerfile: path}, nil
	}
	in := ParseDockerfile(src)
	in.Dockerfile = path
	return in, nil
}

// ---- app authentication ----

func appJWT(cfg store.GitHubAppConfig) (string, error) {
	block, _ := pem.Decode([]byte(cfg.PrivateKey))
	if block == nil {
		return "", errors.New("github app private key is not PEM")
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		key = k
	} else if k8, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		key, _ = k8.(*rsa.PrivateKey)
	}
	if key == nil {
		return "", errors.New("github app private key is not RSA")
	}
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	now := time.Now()
	claims, _ := json.Marshal(map[string]any{"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(9 * time.Minute).Unix(), "iss": cfg.AppID})
	signing := header + "." + enc.EncodeToString(claims)
	h := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h[:])
	if err != nil {
		return "", err
	}
	return signing + "." + enc.EncodeToString(sig), nil
}

// InstallationToken mints a short-lived token able to clone repo.
func InstallationToken(ctx context.Context, cfg store.GitHubAppConfig, repo string) (string, error) {
	jwt, err := appJWT(cfg)
	if err != nil {
		return "", err
	}
	var inst struct {
		ID int `json:"id"`
	}
	if err := do(ctx, "GET", api+"/repos/"+repo+"/installation", "Bearer "+jwt, nil, &inst); err != nil {
		return "", err
	}
	var tok struct {
		Token string `json:"token"`
	}
	err = do(ctx, "POST", fmt.Sprintf("%s/app/installations/%d/access_tokens", api, inst.ID), "Bearer "+jwt, map[string]any{}, &tok)
	return tok.Token, err
}

func VerifyWebhook(secret string, body []byte, sigHeader string) bool {
	if secret == "" || !strings.HasPrefix(sigHeader, "sha256=") {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(strings.TrimPrefix(sigHeader, "sha256=")))
}

type PushEvent struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Deleted    bool   `json:"deleted"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	HeadCommit *struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	} `json:"head_commit"`
	Pusher struct {
		Name string `json:"name"`
	} `json:"pusher"`
}
