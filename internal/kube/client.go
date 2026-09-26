// Package kube is a minimal Kubernetes API client built on net/http:
// server-side apply, get/list/delete, watch streams, pod logs and exec over
// WebSocket. Objects are plain maps so any kind, including CRDs such as
// CloudNativePG clusters and Gateway API routes, can be handled uniformly.
package kube

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/ws"
	"github.com/evanxdsouza/wackcluborchard/internal/yaml"
)

type Obj = map[string]any

type Client struct {
	Server string
	token  string
	tls    *tls.Config
	http   *http.Client
	stream *http.Client
	Field  string // field manager for server-side apply
}

type StatusError struct {
	Code    int
	Reason  string
	Message string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("kubernetes: %d %s: %s", e.Code, e.Reason, e.Message)
}

func IsNotFound(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == 404
}

func IsConflict(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == 409
}

func IsGone(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == 410
}

// Config loads in-cluster credentials when running in a pod, otherwise the
// kubeconfig at $KUBECONFIG or ~/.kube/config.
func Config() (*Client, error) {
	if host := os.Getenv("KUBERNETES_SERVICE_HOST"); host != "" {
		const sa = "/var/run/secrets/kubernetes.io/serviceaccount/"
		tok, err := os.ReadFile(sa + "token")
		if err != nil {
			return nil, err
		}
		ca, err := os.ReadFile(sa + "ca.crt")
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(ca)
		port := os.Getenv("KUBERNETES_SERVICE_PORT")
		if port == "" {
			port = "443"
		}
		return newClient("https://"+host+":"+port, strings.TrimSpace(string(tok)), &tls.Config{RootCAs: pool}), nil
	}
	path := os.Getenv("KUBECONFIG")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".kube", "config")
	}
	return FromKubeconfig(path)
}

func FromKubeconfig(path string) (*Client, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := yaml.Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("kubeconfig: %w", err)
	}
	root := yaml.Map(doc)
	ctxName := yaml.String(root["current-context"])
	find := func(list, name string) map[string]any {
		for _, e := range yaml.List(root[list]) {
			m := yaml.Map(e)
			if yaml.String(m["name"]) == name {
				return m
			}
		}
		return nil
	}
	c := yaml.Map(find("contexts", ctxName)["context"])
	if c == nil {
		return nil, fmt.Errorf("kubeconfig: context %q not found", ctxName)
	}
	cluster := yaml.Map(find("clusters", yaml.String(c["cluster"]))["cluster"])
	user := yaml.Map(find("users", yaml.String(c["user"]))["user"])
	conf := &tls.Config{}
	if ca := yaml.String(cluster["certificate-authority-data"]); ca != "" {
		pem, err := base64.StdEncoding.DecodeString(ca)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(pem)
		conf.RootCAs = pool
	} else if caf := yaml.String(cluster["certificate-authority"]); caf != "" {
		pem, err := os.ReadFile(caf)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(pem)
		conf.RootCAs = pool
	}
	if v, _ := cluster["insecure-skip-tls-verify"].(bool); v {
		conf.InsecureSkipVerify = true
	}
	certPEM, keyPEM := yaml.String(user["client-certificate-data"]), yaml.String(user["client-key-data"])
	if certPEM != "" && keyPEM != "" {
		cb, _ := base64.StdEncoding.DecodeString(certPEM)
		kb, _ := base64.StdEncoding.DecodeString(keyPEM)
		cert, err := tls.X509KeyPair(cb, kb)
		if err != nil {
			return nil, fmt.Errorf("kubeconfig client cert: %w", err)
		}
		conf.Certificates = []tls.Certificate{cert}
	}
	return newClient(strings.TrimRight(yaml.String(cluster["server"]), "/"), yaml.String(user["token"]), conf), nil
}

func newClient(server, token string, conf *tls.Config) *Client {
	tr := &http.Transport{TLSClientConfig: conf, MaxIdleConnsPerHost: 32, IdleConnTimeout: 90 * time.Second}
	return &Client{
		Server: server,
		token:  token,
		tls:    conf,
		http:   &http.Client{Transport: tr, Timeout: 60 * time.Second},
		stream: &http.Client{Transport: tr},
		Field:  "orchard",
	}
}

// ---- resource paths ----

type kind struct {
	group, version, plural string
	namespaced             bool
}

var kinds = map[string]kind{
	"Namespace":             {"", "v1", "namespaces", false},
	"Node":                  {"", "v1", "nodes", false},
	"Pod":                   {"", "v1", "pods", true},
	"Service":               {"", "v1", "services", true},
	"Secret":                {"", "v1", "secrets", true},
	"ConfigMap":             {"", "v1", "configmaps", true},
	"PersistentVolumeClaim": {"", "v1", "persistentvolumeclaims", true},
	"Event":                 {"", "v1", "events", true},
	"ResourceQuota":         {"", "v1", "resourcequotas", true},
	"LimitRange":            {"", "v1", "limitranges", true},
	"ServiceAccount":        {"", "v1", "serviceaccounts", true},
	"Deployment":            {"apps", "v1", "deployments", true},
	"ReplicaSet":            {"apps", "v1", "replicasets", true},
	"Job":                   {"batch", "v1", "jobs", true},
	"CronJob":               {"batch", "v1", "cronjobs", true},
	"Ingress":               {"networking.k8s.io", "v1", "ingresses", true},
	"NetworkPolicy":         {"networking.k8s.io", "v1", "networkpolicies", true},
	"HTTPRoute":             {"gateway.networking.k8s.io", "v1", "httproutes", true},
	"Cluster":               {"postgresql.cnpg.io", "v1", "clusters", true},
	"Backup":                {"postgresql.cnpg.io", "v1", "backups", true},
	"ScheduledBackup":       {"postgresql.cnpg.io", "v1", "scheduledbackups", true},
	"PodMetrics":            {"metrics.k8s.io", "v1beta1", "pods", true},
	"NodeMetrics":           {"metrics.k8s.io", "v1beta1", "nodes", false},
	"Middleware":            {"traefik.io", "v1alpha1", "middlewares", true},
	"IngressRouteTCP":       {"traefik.io", "v1alpha1", "ingressroutetcps", true},
	"Certificate":           {"cert-manager.io", "v1", "certificates", true},
	"RuntimeClass":          {"node.k8s.io", "v1", "runtimeclasses", false},
}

func (c *Client) path(kindName, ns, name string) (string, error) {
	k, ok := kinds[kindName]
	if !ok {
		return "", fmt.Errorf("kube: unknown kind %s", kindName)
	}
	var b strings.Builder
	if k.group == "" {
		b.WriteString("/api/" + k.version)
	} else {
		b.WriteString("/apis/" + k.group + "/" + k.version)
	}
	if k.namespaced && ns != "" {
		b.WriteString("/namespaces/" + ns)
	}
	b.WriteString("/" + k.plural)
	if name != "" {
		b.WriteString("/" + name)
	}
	return b.String(), nil
}

// APIVersion returns the apiVersion string for a kind.
func APIVersion(kindName string) string {
	k := kinds[kindName]
	if k.group == "" {
		return k.version
	}
	return k.group + "/" + k.version
}

func (c *Client) do(ctx context.Context, method, path, contentType string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.Server+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var st struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		}
		json.Unmarshal(data, &st)
		if st.Message == "" {
			st.Message = strings.TrimSpace(string(data))
		}
		return &StatusError{Code: resp.StatusCode, Reason: st.Reason, Message: st.Message}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Apply server-side-applies obj, forcing ownership of the fields Orchard
// manages. obj must carry apiVersion, kind and metadata.name.
func (c *Client) Apply(ctx context.Context, obj Obj) (Obj, error) {
	kindName, _ := obj["kind"].(string)
	meta, _ := obj["metadata"].(map[string]any)
	name, _ := meta["name"].(string)
	ns, _ := meta["namespace"].(string)
	if _, ok := obj["apiVersion"]; !ok {
		obj["apiVersion"] = APIVersion(kindName)
	}
	p, err := c.path(kindName, ns, name)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var out Obj
	err = c.do(ctx, http.MethodPatch, p+"?fieldManager="+url.QueryEscape(c.Field)+"&force=true", "application/apply-patch+yaml", body, &out)
	return out, err
}

func (c *Client) Get(ctx context.Context, kindName, ns, name string) (Obj, error) {
	p, err := c.path(kindName, ns, name)
	if err != nil {
		return nil, err
	}
	var out Obj
	return out, c.do(ctx, http.MethodGet, p, "", nil, &out)
}

type ListOpts struct {
	LabelSelector string
	FieldSelector string
}

func (o ListOpts) query() url.Values {
	q := url.Values{}
	if o.LabelSelector != "" {
		q.Set("labelSelector", o.LabelSelector)
	}
	if o.FieldSelector != "" {
		q.Set("fieldSelector", o.FieldSelector)
	}
	return q
}

// List returns items and the list's resourceVersion (for starting a watch).
func (c *Client) List(ctx context.Context, kindName, ns string, opts ListOpts) ([]Obj, string, error) {
	p, err := c.path(kindName, ns, "")
	if err != nil {
		return nil, "", err
	}
	if q := opts.query().Encode(); q != "" {
		p += "?" + q
	}
	var out struct {
		Metadata struct {
			ResourceVersion string `json:"resourceVersion"`
		} `json:"metadata"`
		Items []Obj `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, p, "", nil, &out); err != nil {
		return nil, "", err
	}
	return out.Items, out.Metadata.ResourceVersion, nil
}

func (c *Client) Delete(ctx context.Context, kindName, ns, name string) error {
	p, err := c.path(kindName, ns, name)
	if err != nil {
		return err
	}
	body := []byte(`{"kind":"DeleteOptions","apiVersion":"v1","propagationPolicy":"Background"}`)
	err = c.do(ctx, http.MethodDelete, p, "application/json", body, nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}

// DeleteCollection removes every object of a kind matching a selector.
func (c *Client) DeleteCollection(ctx context.Context, kindName, ns string, opts ListOpts) error {
	p, err := c.path(kindName, ns, "")
	if err != nil {
		return err
	}
	if q := opts.query().Encode(); q != "" {
		p += "?" + q
	}
	err = c.do(ctx, http.MethodDelete, p, "", nil, nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}

// Patch sends a merge patch.
func (c *Client) Patch(ctx context.Context, kindName, ns, name string, patch any) error {
	p, err := c.path(kindName, ns, name)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(patch)
	return c.do(ctx, http.MethodPatch, p, "application/merge-patch+json", body, nil)
}

type WatchEvent struct {
	Type   string `json:"type"` // ADDED, MODIFIED, DELETED, BOOKMARK, ERROR
	Object Obj    `json:"object"`
}

// Watch streams events until ctx ends or the server closes the stream.
func (c *Client) Watch(ctx context.Context, kindName, ns string, opts ListOpts, resourceVersion string, fn func(WatchEvent)) error {
	p, err := c.path(kindName, ns, "")
	if err != nil {
		return err
	}
	q := opts.query()
	q.Set("watch", "1")
	q.Set("allowWatchBookmarks", "true")
	q.Set("timeoutSeconds", "600")
	if resourceVersion != "" {
		q.Set("resourceVersion", resourceVersion)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Server+p+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.stream.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return &StatusError{Code: resp.StatusCode, Message: string(b)}
	}
	dec := json.NewDecoder(resp.Body)
	for {
		var ev WatchEvent
		if err := dec.Decode(&ev); err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				return nil
			}
			return err
		}
		if ev.Type == "ERROR" {
			code, _ := ev.Object["code"].(float64)
			msg, _ := ev.Object["message"].(string)
			return &StatusError{Code: int(code), Message: msg}
		}
		fn(ev)
	}
}

// Informer lists then watches a kind, calling fn for every object and
// re-listing when the watch expires. It runs until ctx ends.
func (c *Client) Informer(ctx context.Context, kindName, ns string, opts ListOpts, fn func(typ string, o Obj)) {
	backoff := time.Second
	for ctx.Err() == nil {
		items, rv, err := c.List(ctx, kindName, ns, opts)
		if err != nil {
			if !IsNotFound(err) {
				fmt.Fprintf(os.Stderr, "informer %s: list: %v\n", kindName, err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		for _, it := range items {
			fn("SYNC", it)
		}
		for ctx.Err() == nil {
			err := c.Watch(ctx, kindName, ns, opts, rv, func(ev WatchEvent) {
				if meta, ok := ev.Object["metadata"].(map[string]any); ok {
					if v, ok := meta["resourceVersion"].(string); ok {
						rv = v
					}
				}
				if ev.Type != "BOOKMARK" {
					fn(ev.Type, ev.Object)
				}
			})
			if err != nil {
				if !IsGone(err) {
					fmt.Fprintf(os.Stderr, "informer %s: watch: %v\n", kindName, err)
				}
				break
			}
		}
	}
}

// Logs streams a container's logs line by line.
func (c *Client) Logs(ctx context.Context, ns, pod, container string, follow, previous bool, sinceSeconds int, tail int, fn func(string)) error {
	q := url.Values{}
	if container != "" {
		q.Set("container", container)
	}
	if follow {
		q.Set("follow", "true")
	}
	if previous {
		q.Set("previous", "true")
	}
	if sinceSeconds > 0 {
		q.Set("sinceSeconds", fmt.Sprint(sinceSeconds))
	}
	if tail > 0 {
		q.Set("tailLines", fmt.Sprint(tail))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Server+"/api/v1/namespaces/"+ns+"/pods/"+pod+"/log?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.stream.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		var st struct {
			Message string `json:"message"`
		}
		json.Unmarshal(b, &st)
		return &StatusError{Code: resp.StatusCode, Message: st.Message}
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		fn(sc.Text())
	}
	if ctx.Err() != nil {
		return nil
	}
	return sc.Err()
}

// Exec runs cmd in a container over the v4 channel protocol. Channel 0 is
// stdin, 1 stdout, 2 stderr, 3 the status/error channel.
func (c *Client) Exec(ctx context.Context, ns, pod, container string, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	q := url.Values{}
	if container != "" {
		q.Set("container", container)
	}
	for _, a := range cmd {
		q.Add("command", a)
	}
	q.Set("stdout", "true")
	q.Set("stderr", "true")
	if stdin != nil {
		q.Set("stdin", "true")
	}
	u := strings.Replace(c.Server, "https://", "wss://", 1)
	u = strings.Replace(u, "http://", "ws://", 1)
	u += "/api/v1/namespaces/" + ns + "/pods/" + pod + "/exec?" + q.Encode()
	h := http.Header{}
	if c.token != "" {
		h.Set("Authorization", "Bearer "+c.token)
	}
	conn, err := ws.Dial(u, h, c.tls, "v4.channel.k8s.io")
	if err != nil {
		return err
	}
	defer conn.Close()
	go func() {
		<-ctx.Done()
		conn.Close()
	}()
	if stdin != nil {
		go func() {
			buf := make([]byte, 8192)
			for {
				n, err := stdin.Read(buf)
				if n > 0 {
					if werr := conn.WriteMessage(ws.OpBinary, append([]byte{0}, buf[:n]...)); werr != nil {
						return
					}
				}
				if err != nil {
					return
				}
			}
		}()
	}
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if len(msg) == 0 {
			continue
		}
		switch msg[0] {
		case 1:
			if stdout != nil {
				stdout.Write(msg[1:])
			}
		case 2:
			if stderr != nil {
				stderr.Write(msg[1:])
			}
		case 3:
			var st struct {
				Status  string `json:"status"`
				Message string `json:"message"`
			}
			if json.Unmarshal(msg[1:], &st) == nil && st.Status == "Failure" {
				return &ExitError{Message: st.Message}
			}
			return nil
		}
	}
}

type ExitError struct{ Message string }

func (e *ExitError) Error() string { return e.Message }

// ---- object helpers ----

func Str(o Obj, path ...string) string {
	v := Get(o, path...)
	s, _ := v.(string)
	return s
}

func Num(o Obj, path ...string) float64 {
	v := Get(o, path...)
	f, _ := v.(float64)
	return f
}

func Get(o Obj, path ...string) any {
	var cur any = o
	for _, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[p]
	}
	return cur
}

func Items(v any) []Obj {
	l, _ := v.([]any)
	out := make([]Obj, 0, len(l))
	for _, x := range l {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func Labels(o Obj) map[string]string {
	out := map[string]string{}
	m, _ := Get(o, "metadata", "labels").(map[string]any)
	for k, v := range m {
		out[k], _ = v.(string)
	}
	return out
}

// ParseQuantity converts CPU ("250m", "2") to millicores and memory
// ("512Mi", "1Gi", "123456Ki", bytes) to MiB.
func ParseCPU(s string) int {
	if s == "" {
		return 0
	}
	if strings.HasSuffix(s, "n") {
		var n float64
		fmt.Sscan(strings.TrimSuffix(s, "n"), &n)
		return int(n / 1e6)
	}
	if strings.HasSuffix(s, "u") {
		var n float64
		fmt.Sscan(strings.TrimSuffix(s, "u"), &n)
		return int(n / 1e3)
	}
	if strings.HasSuffix(s, "m") {
		var n float64
		fmt.Sscan(strings.TrimSuffix(s, "m"), &n)
		return int(n)
	}
	var n float64
	fmt.Sscan(s, &n)
	return int(n * 1000)
}

func ParseMem(s string) int {
	units := []struct {
		suf string
		mul float64
	}{{"Ki", 1.0 / 1024}, {"Mi", 1}, {"Gi", 1024}, {"Ti", 1024 * 1024}, {"k", 1000.0 / 1048576}, {"M", 1e6 / 1048576}, {"G", 1e9 / 1048576}}
	for _, u := range units {
		if strings.HasSuffix(s, u.suf) {
			var n float64
			fmt.Sscan(strings.TrimSuffix(s, u.suf), &n)
			return int(n * u.mul)
		}
	}
	var n float64
	fmt.Sscan(s, &n)
	return int(n / 1048576)
}
