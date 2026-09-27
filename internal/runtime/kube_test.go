package runtime

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/evanxdsouza/wackcluborchard/internal/kube"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

// fakeAPI records server-side-apply patches.
type fakeAPI struct {
	mu      sync.Mutex
	applied map[string]map[string]any // path -> object
	deleted []string
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Method {
	case http.MethodPatch:
		if r.Header.Get("Content-Type") != "application/apply-patch+yaml" || !strings.Contains(r.URL.RawQuery, "fieldManager=wackcluborchard") {
			http.Error(w, "expected server-side apply", 400)
			return
		}
		var obj map[string]any
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &obj)
		f.applied[r.URL.Path] = obj
		w.Write(b)
	case http.MethodDelete:
		f.deleted = append(f.deleted, r.URL.Path)
		w.Write([]byte(`{}`))
	default:
		w.Write([]byte(`{"items":[],"metadata":{"resourceVersion":"1"}}`))
	}
}

func newFake(t *testing.T) (*fakeAPI, *Kube) {
	f := &fakeAPI{applied: map[string]map[string]any{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	cfgPath := filepath.Join(t.TempDir(), "kubeconfig")
	os.WriteFile(cfgPath, []byte(`apiVersion: v1
clusters:
- cluster:
    server: `+srv.URL+`
  name: fake
contexts:
- context:
    cluster: fake
    user: fake
  name: fake
current-context: fake
users:
- name: fake
  user:
    token: t
`), 0o600)
	c, err := kube.FromKubeconfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	return f, NewKube(c, DefaultKubeConfig())
}

func TestApplyAppManifests(t *testing.T) {
	f, k := newFake(t)
	err := k.ApplyApp(context.Background(), AppSpec{
		Namespace: "wackcluborchard-wc-homelab", Name: "api", ID: "app_1", Org: "wc", Pool: "gpu",
		Image: "registry/api:3", Replicas: 2,
		Ports:     []store.Port{{Name: "http", Port: 3000, Protocol: "http"}, {Name: "game", Port: 25565, Protocol: "tcp", Public: true}},
		Env:       map[string]string{"PORT": "3000"},
		Secrets:   map[string]string{"TOKEN": "x"},
		Resources: store.Resources{CPUMillis: 250, MemoryMi: 256},
		Domains:   []store.Domain{{Host: "api.apps.test", Generated: true}, {Host: "api.example.com"}},
		Health:    store.HealthCheck{Enabled: true, Path: "/healthz", Initial: 5},
		AuthWall:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	dep := f.applied["/apis/apps/v1/namespaces/wackcluborchard-wc-homelab/deployments/api"]
	if dep == nil {
		t.Fatalf("no deployment applied; got %v", keys(f.applied))
	}
	c := kube.Items(kube.Get(dep, "spec", "template", "spec", "containers"))[0]
	if kube.Str(c, "resources", "requests", "cpu") != "250m" || kube.Str(c, "resources", "limits", "cpu") != "250m" || kube.Str(c, "resources", "limits", "memory") != "256Mi" {
		t.Fatalf("requests must equal limits: %v", c["resources"])
	}
	if kube.Str(c, "readinessProbe", "httpGet", "path") != "/healthz" {
		t.Fatalf("readiness probe: %v", c["readinessProbe"])
	}
	if kube.Str(dep, "spec", "template", "spec", "nodeSelector", "wackcluborchard.dev/pool") != "gpu" {
		t.Fatal("pool node selector missing")
	}
	route := f.applied["/apis/gateway.networking.k8s.io/v1/namespaces/wackcluborchard-wc-homelab/httproutes/api"]
	if route == nil || kube.Get(route, "spec", "hostnames").([]any)[0] != "api.apps.test" {
		t.Fatalf("httproute: %v", route)
	}
	if kube.Get(route, "spec", "rules").([]any)[0].(map[string]any)["filters"] == nil {
		t.Fatal("auth wall filter missing on the route")
	}
	ing := f.applied["/apis/networking.k8s.io/v1/namespaces/wackcluborchard-wc-homelab/ingresses/api"]
	if ing == nil || kube.Str(ing, "metadata", "annotations", "cert-manager.io/cluster-issuer") == "" {
		t.Fatalf("custom domain ingress: %v", ing)
	}
	pub := f.applied["/api/v1/namespaces/wackcluborchard-wc-homelab/services/api-public"]
	if pub == nil || kube.Str(pub, "spec", "type") != "NodePort" {
		t.Fatalf("public tcp service: %v", pub)
	}
	sec := f.applied["/api/v1/namespaces/wackcluborchard-wc-homelab/secrets/api-env"]
	if kube.Str(sec, "stringData", "TOKEN") != "x" || kube.Str(sec, "stringData", "PORT") != "3000" {
		t.Fatalf("env secret: %v", sec)
	}
}

func TestApplyDatabaseManifests(t *testing.T) {
	f, k := newFake(t)
	err := k.ApplyDatabase(context.Background(), DatabaseSpec{Namespace: "ns", Name: "main", ID: "db_1", Version: 17, Instances: 2, StorageGi: 10,
		Resources: store.Resources{CPUMillis: 500, MemoryMi: 512}, DBName: "app", User: "u_app", Password: "pw", Extensions: []string{"pgvector"}, PublicPort: 31000})
	if err != nil {
		t.Fatal(err)
	}
	cl := f.applied["/apis/postgresql.cnpg.io/v1/namespaces/ns/clusters/pg-main"]
	if cl == nil {
		t.Fatalf("no cluster; got %v", keys(f.applied))
	}
	if kube.Num(cl, "spec", "instances") != 2 || kube.Str(cl, "spec", "imageName") != "ghcr.io/cloudnative-pg/postgresql:17" {
		t.Fatalf("cluster spec: %v", cl["spec"])
	}
	sql := kube.Get(cl, "spec", "bootstrap", "initdb", "postInitApplicationSQL").([]any)
	if len(sql) != 1 || !strings.Contains(sql[0].(string), "pgvector") {
		t.Fatalf("extensions: %v", sql)
	}
	svc := f.applied["/api/v1/namespaces/ns/services/pg-main-public"]
	if svc == nil || kube.Num(kube.Items(kube.Get(svc, "spec", "ports"))[0], "nodePort") != 31000 {
		t.Fatalf("public service: %v", svc)
	}
}

func keys(m map[string]map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
