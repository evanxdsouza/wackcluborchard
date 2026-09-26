package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/kube"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

// KubeConfig carries the cluster-facing settings the driver needs.
type KubeConfig struct {
	ControlNamespace string // where Orchard itself runs
	GatewayName      string // shared Gateway for generated app hostnames
	GatewayNamespace string
	IngressClass     string // for custom-domain Ingresses
	ClusterIssuer    string // cert-manager issuer for custom domains
	StorageClass     string
	Registry         string // e.g. registry.orchard.svc:5000
	RegistryInsecure bool
	BuildkitImage    string
	GitImage         string
	PostgresImage    string // prefix, version appended
	SandboxImage     string
	ForwardAuthURL   string // auth wall endpoint on the Orchard server
	BuilderRuntime   string // RuntimeClass for build jobs, empty for none
	BuilderNodeSel   map[string]string
}

func DefaultKubeConfig() KubeConfig {
	env := func(k, d string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return d
	}
	return KubeConfig{
		ControlNamespace: env("ORCHARD_NAMESPACE", "orchard"),
		GatewayName:      env("GATEWAY_NAME", "orchard"),
		GatewayNamespace: env("GATEWAY_NAMESPACE", "orchard"),
		IngressClass:     env("TENANT_INGRESS_CLASS", "traefik"),
		ClusterIssuer:    env("TENANT_CERT_ISSUER", "letsencrypt-prod"),
		StorageClass:     os.Getenv("TENANT_STORAGE_CLASS"),
		Registry:         env("REGISTRY_HOST", "registry.orchard.svc.cluster.local:5000"),
		RegistryInsecure: env("REGISTRY_INSECURE", "true") == "true",
		BuildkitImage:    env("BUILDKIT_IMAGE", "moby/buildkit:v0.18.2-rootless"),
		GitImage:         env("GIT_IMAGE", "alpine/git:2.47.1"),
		PostgresImage:    env("POSTGRES_IMAGE", "ghcr.io/cloudnative-pg/postgresql"),
		SandboxImage:     env("SANDBOX_IMAGE", "mcr.microsoft.com/devcontainers/universal:2-linux"),
		ForwardAuthURL:   env("FORWARD_AUTH_URL", "http://server.orchard.svc.cluster.local:8080/api/auth/forward"),
		BuilderRuntime:   os.Getenv("BUILDER_RUNTIME_CLASS"),
	}
}

type Kube struct {
	c   *kube.Client
	cfg KubeConfig
	obs Observer

	mu        sync.Mutex
	podsByApp map[string]map[string]store.Pod // app id -> pod name -> pod
	podOwner  map[string]string               // ns/pod -> app or db id
	seenCrash map[string]bool                 // ns/pod/container/restartCount
	seenEvent map[string]bool
}

func NewKube(c *kube.Client, cfg KubeConfig) *Kube {
	return &Kube{c: c, cfg: cfg, podsByApp: map[string]map[string]store.Pod{}, podOwner: map[string]string{}, seenCrash: map[string]bool{}, seenEvent: map[string]bool{}}
}

func (k *Kube) Name() string { return "kubernetes" }

const (
	lblManaged = "orchard.dev/managed"
	lblApp     = "orchard.dev/app"
	lblAppName = "app.kubernetes.io/name"
	lblDB      = "orchard.dev/database"
	lblRun     = "orchard.dev/run"
	lblBuild   = "orchard.dev/build"
	lblSandbox = "orchard.dev/sandbox"
	lblPool    = "orchard.dev/pool"
	lblOrg     = "orchard.dev/org"
)

func meta(name, ns string, labels map[string]string) map[string]any {
	m := map[string]any{"name": name}
	if ns != "" {
		m["namespace"] = ns
	}
	if labels != nil {
		l := map[string]any{lblManaged: "true"}
		for k, v := range labels {
			l[k] = v
		}
		m["labels"] = l
	}
	return m
}

func res(r store.Resources) map[string]any {
	cpu := fmt.Sprintf("%dm", max(r.CPUMillis, 10))
	mem := fmt.Sprintf("%dMi", max(r.MemoryMi, 16))
	// Requests equal limits: what you pick is what you get and what you
	// are capped at.
	return map[string]any{
		"requests": map[string]any{"cpu": cpu, "memory": mem},
		"limits":   map[string]any{"cpu": cpu, "memory": mem},
	}
}

func poolScheduling(pool string, spec map[string]any) {
	if pool == "" {
		return
	}
	spec["nodeSelector"] = map[string]any{lblPool: pool}
	spec["tolerations"] = []any{map[string]any{"key": lblPool, "operator": "Equal", "value": pool, "effect": "NoSchedule"}}
}

// ---- lifecycle ----

func (k *Kube) Start(ctx context.Context, obs Observer) error {
	k.obs = obs
	if _, _, err := k.c.List(ctx, "Namespace", "", kube.ListOpts{LabelSelector: "kubernetes.io/metadata.name=default"}); err != nil {
		return fmt.Errorf("cannot reach the Kubernetes API at %s: %w", k.c.Server, err)
	}
	go k.c.Informer(ctx, "Pod", "", kube.ListOpts{LabelSelector: lblManaged + "=true"}, k.onPod)
	go k.c.Informer(ctx, "Event", "", kube.ListOpts{FieldSelector: "type=Warning"}, k.onEvent)
	go k.c.Informer(ctx, "Cluster", "", kube.ListOpts{LabelSelector: lblManaged + "=true"}, k.onCluster)
	return nil
}

func (k *Kube) onPod(typ string, o kube.Obj) {
	labels := kube.Labels(o)
	ns := kube.Str(o, "metadata", "namespace")
	name := kube.Str(o, "metadata", "name")
	owner := labels[lblApp]
	if owner == "" {
		owner = labels[lblDB]
	}
	k.mu.Lock()
	if owner != "" {
		k.podOwner[ns+"/"+name] = owner
	}
	k.mu.Unlock()

	// crash capture: a container with a terminated last state has just
	// restarted; grab its final output before it is gone for good.
	for _, cs := range kube.Items(kube.Get(o, "status", "containerStatuses")) {
		term, _ := kube.Get(cs, "lastState", "terminated").(map[string]any)
		if term == nil || owner == "" {
			continue
		}
		cname, _ := cs["name"].(string)
		restarts := int(kube.Num(cs, "restartCount"))
		key := fmt.Sprintf("%s/%s/%s/%d", ns, name, cname, restarts)
		k.mu.Lock()
		seen := k.seenCrash[key]
		k.seenCrash[key] = true
		k.mu.Unlock()
		if seen {
			continue
		}
		reason, _ := term["reason"].(string)
		code := int(kube.Num(term, "exitCode"))
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			var buf strings.Builder
			k.c.Logs(ctx, ns, name, cname, false, true, 0, 300, func(l string) { buf.WriteString(l + "\n") })
			k.obs.Crash(owner, store.CrashReport{Pod: name, Container: cname, Reason: reason, ExitCode: code, Logs: buf.String()})
		}()
	}

	appID := labels[lblApp]
	if appID == "" {
		return
	}
	k.mu.Lock()
	pods := k.podsByApp[appID]
	if pods == nil {
		pods = map[string]store.Pod{}
		k.podsByApp[appID] = pods
	}
	if typ == "DELETED" {
		delete(pods, name)
	} else {
		pods[name] = podFrom(o)
	}
	list := make([]store.Pod, 0, len(pods))
	for _, p := range pods {
		list = append(list, p)
	}
	k.mu.Unlock()
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	k.obs.AppPods(appID, list)
}

func podFrom(o kube.Obj) store.Pod {
	p := store.Pod{
		Name:  kube.Str(o, "metadata", "name"),
		Phase: kube.Str(o, "status", "phase"),
		Node:  kube.Str(o, "spec", "nodeName"),
	}
	if kube.Get(o, "metadata", "deletionTimestamp") != nil {
		p.Phase = "Terminating"
	}
	if t, err := time.Parse(time.RFC3339, kube.Str(o, "status", "startTime")); err == nil {
		p.StartedAt = t
	}
	for _, c := range kube.Items(kube.Get(o, "status", "conditions")) {
		if c["type"] == "Ready" {
			p.Ready = c["status"] == "True"
		}
		if c["type"] == "PodScheduled" && c["status"] == "False" {
			p.Reason, _ = c["message"].(string)
		}
	}
	for _, cs := range kube.Items(kube.Get(o, "status", "containerStatuses")) {
		p.Restarts += int(kube.Num(cs, "restartCount"))
		if w := kube.Str(cs, "state", "waiting", "reason"); w != "" {
			p.Reason = w
		}
	}
	return p
}

func (k *Kube) onEvent(typ string, o kube.Obj) {
	if typ == "DELETED" {
		return
	}
	ns := kube.Str(o, "metadata", "namespace")
	objName := kube.Str(o, "involvedObject", "name")
	objKind := kube.Str(o, "involvedObject", "kind")
	k.mu.Lock()
	owner := k.podOwner[ns+"/"+objName]
	k.mu.Unlock()
	if owner == "" {
		// Deployments and ReplicaSets are named after the app; attribute
		// by prefix within managed namespaces.
		if !strings.HasPrefix(ns, "orchard-") {
			return
		}
		owner = "name:" + ns + "/" + strings.SplitN(objName, "-", 2)[0]
	}
	reason := kube.Str(o, "reason")
	msg := kube.Str(o, "message")
	count := int(kube.Num(o, "count"))
	if count == 0 {
		count = 1
	}
	e := store.KubeEvent{Object: objKind + "/" + objName, Reason: reason, Message: msg, Count: count}
	if t, err := time.Parse(time.RFC3339, kube.Str(o, "lastTimestamp")); err == nil {
		e.LastSeen = t
	}
	if t, err := time.Parse(time.RFC3339, kube.Str(o, "firstTimestamp")); err == nil {
		e.FirstSeen = t
	}
	k.obs.Warning(owner, e)
}

func (k *Kube) onCluster(typ string, o kube.Obj) {
	id := kube.Labels(o)[lblDB]
	if id == "" {
		return
	}
	if typ == "DELETED" {
		return
	}
	phase := kube.Str(o, "status", "phase")
	status := "provisioning"
	switch {
	case kube.Get(o, "metadata", "annotations", "cnpg.io/hibernation") == "on":
		status = "stopped"
	case strings.Contains(phase, "healthy"):
		status = "ready"
	case strings.Contains(strings.ToLower(phase), "fail"):
		status = "failed"
	}
	k.obs.DatabaseState(id, status, -1, -1)
}

// ---- namespaces ----

func (k *Kube) EnsureNamespace(ctx context.Context, ns string, labels map[string]string) error {
	l := map[string]string{"pod-security.kubernetes.io/enforce": "baseline"}
	for a, b := range labels {
		l[a] = b
	}
	if _, err := k.c.Apply(ctx, kube.Obj{"kind": "Namespace", "metadata": meta(ns, "", l)}); err != nil {
		return err
	}
	// Tenants are network-isolated from each other and from the platform:
	// only same-namespace pods and the ingress layer may connect in.
	np := kube.Obj{
		"kind":     "NetworkPolicy",
		"metadata": meta("orchard-isolation", ns, map[string]string{}),
		"spec": map[string]any{
			"podSelector": map[string]any{},
			"policyTypes": []any{"Ingress"},
			"ingress": []any{
				map[string]any{"from": []any{
					map[string]any{"podSelector": map[string]any{}},
					map[string]any{"namespaceSelector": map[string]any{"matchExpressions": []any{map[string]any{
						"key": "kubernetes.io/metadata.name", "operator": "In",
						"values": []any{"kube-system", "traefik", "ingress-nginx", k.cfg.ControlNamespace, k.cfg.GatewayNamespace},
					}}}},
				}},
			},
		},
	}
	_, err := k.c.Apply(ctx, np)
	return err
}

func (k *Kube) DeleteNamespace(ctx context.Context, ns string) error {
	return k.c.Delete(ctx, "Namespace", "", ns)
}

// ---- apps ----

func (k *Kube) ApplyApp(ctx context.Context, s AppSpec) error {
	labels := map[string]string{lblApp: s.ID, lblAppName: s.Name, lblOrg: s.Org}
	secretData := map[string]any{}
	for kk, v := range s.Env {
		secretData[kk] = v
	}
	for kk, v := range s.Secrets {
		secretData[kk] = v
	}
	if _, err := k.c.Apply(ctx, kube.Obj{"kind": "Secret", "metadata": meta(s.Name+"-env", s.Namespace, labels), "type": "Opaque", "stringData": secretData}); err != nil {
		return fmt.Errorf("env secret: %w", err)
	}
	for _, v := range s.Volumes {
		pvc := kube.Obj{
			"kind":     "PersistentVolumeClaim",
			"metadata": meta(s.Name+"-"+v.Name, s.Namespace, labels),
			"spec": map[string]any{
				"accessModes": []any{"ReadWriteOnce"},
				"resources":   map[string]any{"requests": map[string]any{"storage": fmt.Sprintf("%dGi", max(v.SizeGi, 1))}},
			},
		}
		if k.cfg.StorageClass != "" {
			pvc["spec"].(map[string]any)["storageClassName"] = k.cfg.StorageClass
		}
		if _, err := k.c.Apply(ctx, pvc); err != nil {
			return fmt.Errorf("volume %s: %w", v.Name, err)
		}
	}

	container := map[string]any{
		"name":            s.Name,
		"image":           s.Image,
		"imagePullPolicy": "IfNotPresent",
		"envFrom":         []any{map[string]any{"secretRef": map[string]any{"name": s.Name + "-env"}}},
		"resources":       res(s.Resources),
	}
	if s.Command != "" {
		container["command"] = []any{"sh", "-c", s.Command}
	}
	var ports []any
	for _, p := range s.Ports {
		proto := "TCP"
		if p.Protocol == "udp" {
			proto = "UDP"
		}
		ports = append(ports, map[string]any{"name": portName(p), "containerPort": p.Port, "protocol": proto})
	}
	if len(ports) > 0 {
		container["ports"] = ports
	}
	if s.Health.Enabled && s.Health.Path != "" {
		port := s.Health.Port
		if port == 0 && len(s.Ports) > 0 {
			port = s.Ports[0].Port
		}
		container["readinessProbe"] = map[string]any{
			"httpGet":             map[string]any{"path": s.Health.Path, "port": port},
			"initialDelaySeconds": s.Health.Initial,
			"periodSeconds":       5,
			"failureThreshold":    3,
		}
		container["livenessProbe"] = map[string]any{
			"httpGet":             map[string]any{"path": s.Health.Path, "port": port},
			"initialDelaySeconds": s.Health.Initial + 20,
			"periodSeconds":       10,
			"failureThreshold":    6,
		}
	}
	var vols, mounts []any
	for _, v := range s.Volumes {
		vols = append(vols, map[string]any{"name": v.Name, "persistentVolumeClaim": map[string]any{"claimName": s.Name + "-" + v.Name}})
		mounts = append(mounts, map[string]any{"name": v.Name, "mountPath": v.MountPath})
	}
	if len(mounts) > 0 {
		container["volumeMounts"] = mounts
	}
	podSpec := map[string]any{
		"containers":                    []any{container},
		"enableServiceLinks":            false,
		"automountServiceAccountToken":  false,
		"terminationGracePeriodSeconds": 20,
	}
	if len(vols) > 0 {
		podSpec["volumes"] = vols
	}
	if s.Sandboxed && s.RuntimeClass != "" {
		podSpec["runtimeClassName"] = s.RuntimeClass
	}
	poolScheduling(s.Pool, podSpec)
	strategy := map[string]any{"type": "RollingUpdate", "rollingUpdate": map[string]any{"maxUnavailable": 0, "maxSurge": 1}}
	if len(s.Volumes) > 0 {
		// single-writer volumes cannot be attached to two pods at once
		strategy = map[string]any{"type": "Recreate"}
	}
	dep := kube.Obj{
		"kind":     "Deployment",
		"metadata": meta(s.Name, s.Namespace, labels),
		"spec": map[string]any{
			"replicas":             s.Replicas,
			"revisionHistoryLimit": 5,
			"strategy":             strategy,
			"selector":             map[string]any{"matchLabels": map[string]any{lblApp: s.ID}},
			"template": map[string]any{
				"metadata": map[string]any{
					"labels":      map[string]any{lblManaged: "true", lblApp: s.ID, lblAppName: s.Name, lblOrg: s.Org},
					"annotations": map[string]any{"orchard.dev/restart": s.RestartNonce},
				},
				"spec": podSpec,
			},
		},
	}
	if _, err := k.c.Apply(ctx, dep); err != nil {
		return fmt.Errorf("deployment: %w", err)
	}

	// Services: cluster-internal for every port, NodePort for public raw ports.
	var svcPorts, pubPorts []any
	for _, p := range s.Ports {
		proto := "TCP"
		if p.Protocol == "udp" {
			proto = "UDP"
		}
		sp := map[string]any{"name": portName(p), "port": p.Port, "targetPort": p.Port, "protocol": proto}
		svcPorts = append(svcPorts, sp)
		if p.Public && p.Protocol != "http" {
			pp := map[string]any{"name": portName(p), "port": p.Port, "targetPort": p.Port, "protocol": proto}
			if p.NodePort > 0 {
				pp["nodePort"] = p.NodePort
			}
			pubPorts = append(pubPorts, pp)
		}
	}
	if len(svcPorts) > 0 {
		if _, err := k.c.Apply(ctx, kube.Obj{"kind": "Service", "metadata": meta(s.Name, s.Namespace, labels), "spec": map[string]any{
			"selector": map[string]any{lblApp: s.ID}, "ports": svcPorts,
		}}); err != nil {
			return fmt.Errorf("service: %w", err)
		}
	}
	if len(pubPorts) > 0 {
		if _, err := k.c.Apply(ctx, kube.Obj{"kind": "Service", "metadata": meta(s.Name+"-public", s.Namespace, labels), "spec": map[string]any{
			"type": "NodePort", "selector": map[string]any{lblApp: s.ID}, "ports": pubPorts, "externalTrafficPolicy": "Local",
		}}); err != nil {
			return fmt.Errorf("public service: %w", err)
		}
	} else {
		k.c.Delete(ctx, "Service", s.Namespace, s.Name+"-public")
	}

	return k.applyRouting(ctx, s, labels)
}

func portName(p store.Port) string {
	if p.Name != "" {
		return store.Slugify(p.Name)[:min(15, len(store.Slugify(p.Name)))]
	}
	return fmt.Sprintf("%s-%d", orDefault(p.Protocol, "tcp"), p.Port)
}

func (k *Kube) applyRouting(ctx context.Context, s AppSpec, labels map[string]string) error {
	var httpPort int
	for _, p := range s.Ports {
		if p.Protocol == "http" || p.Protocol == "" {
			httpPort = p.Port
			break
		}
	}
	if httpPort == 0 {
		k.c.Delete(ctx, "HTTPRoute", s.Namespace, s.Name)
		k.c.Delete(ctx, "Ingress", s.Namespace, s.Name)
		return nil
	}
	var generated, custom []any
	for _, d := range s.Domains {
		if d.Generated {
			generated = append(generated, d.Host)
		} else {
			custom = append(custom, d.Host)
		}
	}
	if s.AuthWall {
		mw := kube.Obj{"kind": "Middleware", "metadata": meta(s.Name+"-auth", s.Namespace, labels), "spec": map[string]any{
			"forwardAuth": map[string]any{
				"address":             k.cfg.ForwardAuthURL + "?app=" + s.ID,
				"trustForwardHeader":  true,
				"authResponseHeaders": []any{"X-Orchard-User", "X-Orchard-Email"},
			},
		}}
		if _, err := k.c.Apply(ctx, mw); err != nil {
			log.Printf("auth wall middleware (is Traefik installed?): %v", err)
		}
	} else {
		k.c.Delete(ctx, "Middleware", s.Namespace, s.Name+"-auth")
	}
	if len(generated) > 0 {
		rule := map[string]any{"backendRefs": []any{map[string]any{"name": s.Name, "port": httpPort}}}
		if s.AuthWall {
			rule["filters"] = []any{map[string]any{"type": "ExtensionRef", "extensionRef": map[string]any{"group": "traefik.io", "kind": "Middleware", "name": s.Name + "-auth"}}}
		}
		route := kube.Obj{"kind": "HTTPRoute", "metadata": meta(s.Name, s.Namespace, labels), "spec": map[string]any{
			"parentRefs": []any{map[string]any{"name": k.cfg.GatewayName, "namespace": k.cfg.GatewayNamespace}},
			"hostnames":  generated,
			"rules":      []any{rule},
		}}
		if _, err := k.c.Apply(ctx, route); err != nil {
			return fmt.Errorf("httproute: %w", err)
		}
	}
	if len(custom) > 0 {
		var rules, hosts []any
		for _, h := range custom {
			hosts = append(hosts, h)
			rules = append(rules, map[string]any{"host": h, "http": map[string]any{"paths": []any{map[string]any{
				"path": "/", "pathType": "Prefix",
				"backend": map[string]any{"service": map[string]any{"name": s.Name, "port": map[string]any{"number": httpPort}}},
			}}}})
		}
		m := meta(s.Name, s.Namespace, labels)
		ann := map[string]any{"cert-manager.io/cluster-issuer": k.cfg.ClusterIssuer}
		if s.AuthWall {
			ann["traefik.ingress.kubernetes.io/router.middlewares"] = s.Namespace + "-" + s.Name + "-auth@kubernetescrd"
		}
		m["annotations"] = ann
		ing := kube.Obj{"kind": "Ingress", "metadata": m, "spec": map[string]any{
			"ingressClassName": k.cfg.IngressClass,
			"tls":              []any{map[string]any{"hosts": hosts, "secretName": s.Name + "-tls"}},
			"rules":            rules,
		}}
		if _, err := k.c.Apply(ctx, ing); err != nil {
			return fmt.Errorf("ingress: %w", err)
		}
	} else {
		k.c.Delete(ctx, "Ingress", s.Namespace, s.Name)
	}
	return nil
}

func (k *Kube) DeleteApp(ctx context.Context, ns, name string) error {
	for _, kd := range []string{"Deployment", "Service", "HTTPRoute", "Ingress", "Middleware"} {
		if err := k.c.Delete(ctx, kd, ns, name); err != nil && !strings.Contains(err.Error(), "unknown kind") {
			log.Printf("delete %s %s/%s: %v", kd, ns, name, err)
		}
	}
	k.c.Delete(ctx, "Service", ns, name+"-public")
	k.c.Delete(ctx, "Middleware", ns, name+"-auth")
	k.c.Delete(ctx, "Secret", ns, name+"-env")
	return k.c.DeleteCollection(ctx, "PersistentVolumeClaim", ns, kube.ListOpts{LabelSelector: lblAppName + "=" + name})
}

func (k *Kube) appPods(ctx context.Context, ns, name string) ([]kube.Obj, error) {
	items, _, err := k.c.List(ctx, "Pod", ns, kube.ListOpts{LabelSelector: lblAppName + "=" + name + "," + lblManaged + "=true"})
	return items, err
}

func (k *Kube) Logs(ctx context.Context, ns, name, pod string, previous bool, since time.Duration, out func(pod, line string)) error {
	var pods []string
	if pod != "" {
		pods = []string{pod}
	} else {
		items, err := k.appPods(ctx, ns, name)
		if err != nil {
			return err
		}
		for _, p := range items {
			pods = append(pods, kube.Str(p, "metadata", "name"))
		}
	}
	if len(pods) == 0 {
		out("", "(no pods are running)")
		return nil
	}
	var wg sync.WaitGroup
	var omu sync.Mutex
	for _, p := range pods {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			err := k.c.Logs(ctx, ns, p, name, !previous, previous, int(since.Seconds()), 500, func(l string) {
				omu.Lock()
				out(p, l)
				omu.Unlock()
			})
			if err != nil && ctx.Err() == nil {
				omu.Lock()
				out(p, "(logs unavailable: "+err.Error()+")")
				omu.Unlock()
			}
		}(p)
	}
	wg.Wait()
	return nil
}

func (k *Kube) firstPod(ctx context.Context, ns, name string) (string, error) {
	items, err := k.appPods(ctx, ns, name)
	if err != nil {
		return "", err
	}
	for _, p := range items {
		if kube.Str(p, "status", "phase") == "Running" {
			return kube.Str(p, "metadata", "name"), nil
		}
	}
	return "", fmt.Errorf("no running pod for %s", name)
}

// lineShell adapts a non-TTY exec to the line-based web terminal: each
// line typed is run through the container's shell.
func (k *Kube) Exec(ctx context.Context, ns, name, pod string, cmd []string, io Stdio) error {
	if pod == "" {
		var err error
		if pod, err = k.firstPod(ctx, ns, name); err != nil {
			return err
		}
	}
	if len(cmd) == 0 {
		cmd = []string{"sh", "-c", "command -v bash >/dev/null && exec bash -i 2>&1 || exec sh -i 2>&1"}
	}
	return k.c.Exec(ctx, ns, pod, name, cmd, io.Stdin, io.Stdout, io.Stdout)
}

func (k *Kube) AppMetrics(ctx context.Context, ns, name string) (Metrics, error) {
	items, _, err := k.c.List(ctx, "PodMetrics", ns, kube.ListOpts{LabelSelector: lblAppName + "=" + name})
	if err != nil {
		return Metrics{}, err
	}
	var m Metrics
	for _, it := range items {
		for _, c := range kube.Items(it["containers"]) {
			m.CPUMillis += float64(kube.ParseCPU(kube.Str(c, "usage", "cpu")))
			m.MemoryMi += float64(kube.ParseMem(kube.Str(c, "usage", "memory")))
		}
	}
	return m, nil
}

// ---- builds ----

func (k *Kube) Build(ctx context.Context, s BuildSpec, logf func(string)) error {
	name := "build-" + strings.ToLower(s.DeployID)
	name = strings.ReplaceAll(name, "_", "-")
	labels := map[string]string{lblBuild: s.DeployID}
	auth := map[string]any{"auths": map[string]any{}}
	if v := s.PullSecrets["DOCKER_AUTH_CONFIG"]; v != "" {
		json.Unmarshal([]byte(v), &auth)
	}
	if u, t := s.PullSecrets["DHI_USERNAME"], s.PullSecrets["DHI_TOKEN"]; u != "" && t != "" {
		auths, _ := auth["auths"].(map[string]any)
		if auths == nil {
			auths = map[string]any{}
			auth["auths"] = auths
		}
		auths["dhi.io"] = map[string]any{"auth": base64.StdEncoding.EncodeToString([]byte(u + ":" + t))}
	}
	authJSON, _ := json.Marshal(auth)
	if _, err := k.c.Apply(ctx, kube.Obj{"kind": "Secret", "metadata": meta(name, s.Namespace, labels), "stringData": map[string]any{
		"config.json": string(authJSON),
		"clone-url":   s.CloneURL,
	}}); err != nil {
		return err
	}
	defer k.c.Delete(context.Background(), "Secret", s.Namespace, name)

	ctxDir := strings.Trim(orDefault(s.Context, "."), "/")
	dfPath := orDefault(s.Dockerfile, "Dockerfile")
	dfDir := "/workspace/src/" + ctxDir
	if i := strings.LastIndex(dfPath, "/"); i >= 0 {
		dfDir = "/workspace/src/" + dfPath[:i]
		dfPath = dfPath[i+1:]
	}
	args := []any{"build", "--frontend", "dockerfile.v0",
		"--local", "context=/workspace/src/" + ctxDir,
		"--local", "dockerfile=" + dfDir,
		"--opt", "filename=" + dfPath,
		"--output", fmt.Sprintf("type=image,name=%s,push=true,registry.insecure=%v", s.Image, k.cfg.RegistryInsecure),
		"--export-cache", fmt.Sprintf("type=registry,ref=%s/cache/%s:buildcache,mode=max,registry.insecure=%v", k.cfg.Registry, s.AppName, k.cfg.RegistryInsecure),
		"--import-cache", fmt.Sprintf("type=registry,ref=%s/cache/%s:buildcache,registry.insecure=%v", k.cfg.Registry, s.AppName, k.cfg.RegistryInsecure),
		"--progress", "plain",
	}
	if s.Target != "" {
		args = append(args, "--opt", "target="+s.Target)
	}
	for kk, v := range s.BuildArgs {
		args = append(args, "--opt", "build-arg:"+kk+"="+v)
	}
	clone := fmt.Sprintf(`set -e
git clone --filter=blob:none --branch %q "$(cat /secret/clone-url)" /workspace/src 2>&1
cd /workspace/src
%s
echo "HEAD is now at $(git rev-parse --short HEAD) $(git log -1 --pretty=%%s)"`, s.Branch, checkoutLine(s.Commit))
	podSpec := map[string]any{
		"restartPolicy": "Never",
		"initContainers": []any{map[string]any{
			"name": "clone", "image": k.cfg.GitImage,
			"command":         []any{"sh", "-c", clone},
			"volumeMounts":    []any{map[string]any{"name": "workspace", "mountPath": "/workspace"}, map[string]any{"name": "secret", "mountPath": "/secret"}},
			"securityContext": map[string]any{"runAsUser": 1000, "runAsGroup": 1000},
		}},
		"containers": []any{map[string]any{
			"name": "buildkit", "image": k.cfg.BuildkitImage,
			"command": []any{"buildctl-daemonless.sh"},
			"args":    args,
			"env": []any{
				map[string]any{"name": "BUILDKITD_FLAGS", "value": "--oci-worker-no-process-sandbox"},
				map[string]any{"name": "DOCKER_CONFIG", "value": "/secret"},
			},
			"securityContext": map[string]any{
				"runAsUser": 1000, "runAsGroup": 1000,
				"seccompProfile":  map[string]any{"type": "Unconfined"},
				"appArmorProfile": map[string]any{"type": "Unconfined"},
			},
			"volumeMounts": []any{
				map[string]any{"name": "workspace", "mountPath": "/workspace"},
				map[string]any{"name": "secret", "mountPath": "/secret"},
				map[string]any{"name": "buildkitd", "mountPath": "/home/user/.local/share/buildkit"},
			},
			"resources": map[string]any{"requests": map[string]any{"cpu": "500m", "memory": "1Gi"}, "limits": map[string]any{"memory": "4Gi"}},
		}},
		"volumes": []any{
			map[string]any{"name": "workspace", "emptyDir": map[string]any{}},
			map[string]any{"name": "buildkitd", "emptyDir": map[string]any{}},
			map[string]any{"name": "secret", "secret": map[string]any{"secretName": name}},
		},
	}
	if s.Sandboxed && k.cfg.BuilderRuntime != "" {
		podSpec["runtimeClassName"] = k.cfg.BuilderRuntime
	}
	job := kube.Obj{"kind": "Job", "metadata": meta(name, s.Namespace, labels), "spec": map[string]any{
		"backoffLimit":            0,
		"ttlSecondsAfterFinished": 3600,
		"activeDeadlineSeconds":   3600,
		"template":                map[string]any{"metadata": map[string]any{"labels": map[string]any{lblManaged: "true", lblBuild: s.DeployID}}, "spec": podSpec},
	}}
	if _, err := k.c.Apply(ctx, job); err != nil {
		return fmt.Errorf("create build job: %w", err)
	}
	logf("::step::schedule")
	pod, err := k.waitPod(ctx, s.Namespace, lblBuild+"="+s.DeployID, func(p kube.Obj) bool {
		return kube.Str(p, "status", "phase") != "Pending" || len(kube.Items(kube.Get(p, "status", "initContainerStatuses"))) > 0 && kube.Get(kube.Items(kube.Get(p, "status", "initContainerStatuses"))[0], "state", "running") != nil
	})
	if err != nil {
		return err
	}
	logf("::step::build")
	for _, c := range []string{"clone", "buildkit"} {
		if err := k.waitContainerStarted(ctx, s.Namespace, pod, c); err != nil {
			return err
		}
		k.c.Logs(ctx, s.Namespace, pod, c, true, false, 0, 0, func(l string) {
			if strings.Contains(l, "exporting to image") || strings.Contains(l, "pushing layers") {
				logf("::step::push")
			}
			logf(l)
		})
	}
	return k.waitJob(ctx, s.Namespace, name)
}

func checkoutLine(commit string) string {
	if commit == "" {
		return ""
	}
	return "git checkout -q " + commit
}

func (k *Kube) waitPod(ctx context.Context, ns, selector string, ok func(kube.Obj) bool) (string, error) {
	deadline := time.Now().Add(20 * time.Minute)
	for time.Now().Before(deadline) {
		items, _, err := k.c.List(ctx, "Pod", ns, kube.ListOpts{LabelSelector: selector})
		if err == nil && len(items) > 0 && ok(items[0]) {
			return kube.Str(items[0], "metadata", "name"), nil
		}
		if !sleep(ctx, time.Second) {
			return "", ctx.Err()
		}
	}
	return "", fmt.Errorf("timed out waiting for a node with capacity")
}

func (k *Kube) waitContainerStarted(ctx context.Context, ns, pod, container string) error {
	for {
		o, err := k.c.Get(ctx, "Pod", ns, pod)
		if err != nil {
			return err
		}
		statuses := append(kube.Items(kube.Get(o, "status", "initContainerStatuses")), kube.Items(kube.Get(o, "status", "containerStatuses"))...)
		for _, cs := range statuses {
			if cs["name"] == container && (kube.Get(cs, "state", "running") != nil || kube.Get(cs, "state", "terminated") != nil) {
				return nil
			}
			if cs["name"] == container {
				if r := kube.Str(cs, "state", "waiting", "reason"); r == "ErrImagePull" || r == "ImagePullBackOff" || r == "CreateContainerConfigError" {
					return fmt.Errorf("%s: %s", container, kube.Str(cs, "state", "waiting", "message"))
				}
			}
		}
		if kube.Str(o, "status", "phase") == "Failed" {
			return fmt.Errorf("pod failed before %s started", container)
		}
		if !sleep(ctx, 700*time.Millisecond) {
			return ctx.Err()
		}
	}
}

func (k *Kube) waitJob(ctx context.Context, ns, name string) error {
	for {
		o, err := k.c.Get(ctx, "Job", ns, name)
		if err != nil {
			return err
		}
		if kube.Num(o, "status", "succeeded") > 0 {
			return nil
		}
		if kube.Num(o, "status", "failed") > 0 {
			msg := "build failed"
			for _, c := range kube.Items(kube.Get(o, "status", "conditions")) {
				if c["type"] == "Failed" {
					if m, _ := c["message"].(string); m != "" {
						msg = m
					}
				}
			}
			return fmt.Errorf("%s", msg)
		}
		if !sleep(ctx, time.Second) {
			return ctx.Err()
		}
	}
}

// ---- databases ----

func pgName(name string) string { return "pg-" + name }

func (k *Kube) ApplyDatabase(ctx context.Context, s DatabaseSpec) error {
	labels := map[string]string{lblDB: s.ID}
	if _, err := k.c.Apply(ctx, kube.Obj{"kind": "Secret", "metadata": meta(pgName(s.Name)+"-app", s.Namespace, labels), "type": "kubernetes.io/basic-auth", "stringData": map[string]any{
		"username": s.User, "password": s.Password,
	}}); err != nil {
		return err
	}
	var post []any
	for _, e := range s.Extensions {
		post = append(post, fmt.Sprintf(`CREATE EXTENSION IF NOT EXISTS "%s"`, e))
	}
	m := meta(pgName(s.Name), s.Namespace, labels)
	hib := "off"
	if s.Stopped {
		hib = "on"
	}
	m["annotations"] = map[string]any{"cnpg.io/hibernation": hib}
	storage := map[string]any{"size": fmt.Sprintf("%dGi", max(s.StorageGi, 1))}
	if k.cfg.StorageClass != "" {
		storage["storageClass"] = k.cfg.StorageClass
	}
	cluster := kube.Obj{"kind": "Cluster", "metadata": m, "spec": map[string]any{
		"instances":             max(s.Instances, 1),
		"imageName":             fmt.Sprintf("%s:%d", k.cfg.PostgresImage, s.Version),
		"storage":               storage,
		"resources":             res(s.Resources),
		"enableSuperuserAccess": false,
		"bootstrap": map[string]any{"initdb": map[string]any{
			"database":               s.DBName,
			"owner":                  s.User,
			"secret":                 map[string]any{"name": pgName(s.Name) + "-app"},
			"postInitApplicationSQL": post,
		}},
		"inheritedMetadata": map[string]any{"labels": map[string]any{lblManaged: "true", lblDB: s.ID}},
	}}
	if _, err := k.c.Apply(ctx, cluster); err != nil {
		return fmt.Errorf("postgres cluster: %w", err)
	}
	if s.PublicPort > 0 {
		svc := kube.Obj{"kind": "Service", "metadata": meta(pgName(s.Name)+"-public", s.Namespace, labels), "spec": map[string]any{
			"type":     "NodePort",
			"selector": map[string]any{"cnpg.io/cluster": pgName(s.Name), "cnpg.io/instanceRole": "primary"},
			"ports":    []any{map[string]any{"name": "postgres", "port": 5432, "targetPort": 5432, "nodePort": s.PublicPort}},
		}}
		if _, err := k.c.Apply(ctx, svc); err != nil {
			return fmt.Errorf("public service: %w", err)
		}
	} else {
		k.c.Delete(ctx, "Service", s.Namespace, pgName(s.Name)+"-public")
	}
	return nil
}

func (k *Kube) DeleteDatabase(ctx context.Context, ns, name string) error {
	k.c.Delete(ctx, "Service", ns, pgName(name)+"-public")
	k.c.Delete(ctx, "Secret", ns, pgName(name)+"-app")
	return k.c.Delete(ctx, "Cluster", ns, pgName(name))
}

func (k *Kube) primary(ctx context.Context, s DatabaseSpec) (string, error) {
	o, err := k.c.Get(ctx, "Cluster", s.Namespace, pgName(s.Name))
	if err != nil {
		return "", err
	}
	p := kube.Str(o, "status", "currentPrimary")
	if p == "" {
		return "", fmt.Errorf("database has no primary yet")
	}
	return p, nil
}

func (k *Kube) Query(ctx context.Context, s DatabaseSpec, sql string) (*QueryResult, error) {
	pod, err := k.primary(ctx, s)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	var out, errb bytes.Buffer
	cmd := []string{"psql", "-X", "--csv", "-v", "ON_ERROR_STOP=1", "-d", s.DBName, "-c", sql}
	err = k.c.Exec(ctx, s.Namespace, pod, "postgres", cmd, nil, &out, &errb)
	if err != nil || strings.Contains(errb.String(), "ERROR:") {
		msg := strings.TrimSpace(errb.String())
		if msg == "" && err != nil {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s", strings.TrimPrefix(msg, "ERROR:  "))
	}
	r := &QueryResult{Duration: float64(time.Since(start).Microseconds()) / 1000}
	text := strings.TrimSpace(out.String())
	rows, cerr := csv.NewReader(strings.NewReader(text)).ReadAll()
	if cerr != nil || len(rows) == 0 || !strings.Contains(text, ",") && len(rows) == 1 && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(sql)), "select") {
		r.Command = text
		return r, nil
	}
	r.Columns = rows[0]
	r.Rows = rows[1:]
	r.Command = fmt.Sprintf("SELECT %d", len(r.Rows))
	return r, nil
}

func (k *Kube) Backup(ctx context.Context, s DatabaseSpec) (int64, error) {
	pod, err := k.primary(ctx, s)
	if err != nil {
		return 0, err
	}
	var out, errb bytes.Buffer
	stamp := time.Now().UTC().Format("20060102-150405")
	script := fmt.Sprintf("mkdir -p /var/lib/postgresql/data/orchard-backups && pg_dump -Fc -d %s -f /var/lib/postgresql/data/orchard-backups/%s.dump && stat -c %%s /var/lib/postgresql/data/orchard-backups/%s.dump", s.DBName, stamp, stamp)
	if err := k.c.Exec(ctx, s.Namespace, pod, "postgres", []string{"sh", "-c", script}, nil, &out, &errb); err != nil {
		return 0, fmt.Errorf("%v: %s", err, errb.String())
	}
	var n int64
	fmt.Sscan(strings.TrimSpace(out.String()), &n)
	return n, nil
}

func (k *Kube) DatabaseShell(ctx context.Context, s DatabaseSpec, io Stdio) error {
	pod, err := k.primary(ctx, s)
	if err != nil {
		return err
	}
	return k.c.Exec(ctx, s.Namespace, pod, "postgres", []string{"psql", "-X", "-d", s.DBName, "-e"}, io.Stdin, io.Stdout, io.Stdout)
}

// ---- jobs ----

// RunJob runs every step as an init container of one pod, so steps run
// strictly in order, each has its own logs and exit code, and a failing
// step stops the pipeline.
func (k *Kube) RunJob(ctx context.Context, s JobSpec, update func(StepUpdate)) error {
	name := strings.ReplaceAll("run-"+strings.ToLower(s.RunID), "_", "-")
	labels := map[string]string{lblRun: s.RunID}
	env := []any{}
	for kk, v := range s.Env {
		env = append(env, map[string]any{"name": kk, "value": v})
	}
	var steps []any
	for i, st := range s.Steps {
		c := map[string]any{
			"name":      fmt.Sprintf("step-%d", i),
			"image":     st.Image,
			"env":       env,
			"resources": map[string]any{"requests": map[string]any{"cpu": "100m", "memory": "128Mi"}, "limits": map[string]any{"memory": "1Gi"}},
		}
		if st.Script != "" {
			switch st.Lang {
			case "python":
				c["command"] = []any{"python3", "-c", st.Script}
			case "node":
				c["command"] = []any{"node", "-e", st.Script}
			default:
				c["command"] = []any{"bash", "-c", st.Script}
			}
		} else if len(st.Command) > 0 {
			var cmd []any
			for _, a := range st.Command {
				cmd = append(cmd, a)
			}
			c["command"] = cmd
		}
		steps = append(steps, c)
	}
	podSpec := map[string]any{
		"restartPolicy":                "Never",
		"automountServiceAccountToken": false,
		"initContainers":               steps,
		"containers":                   []any{map[string]any{"name": "done", "image": "busybox:1.37", "command": []any{"true"}}},
	}
	job := kube.Obj{"kind": "Job", "metadata": meta(name, s.Namespace, labels), "spec": map[string]any{
		"backoffLimit": 0, "ttlSecondsAfterFinished": 86400,
		"template": map[string]any{"metadata": map[string]any{"labels": map[string]any{lblManaged: "true", lblRun: s.RunID}}, "spec": podSpec},
	}}
	if _, err := k.c.Apply(ctx, job); err != nil {
		return err
	}
	pod, err := k.waitPod(ctx, s.Namespace, lblRun+"="+s.RunID, func(kube.Obj) bool { return true })
	if err != nil {
		return err
	}
	failed := false
	for i := range s.Steps {
		cname := fmt.Sprintf("step-%d", i)
		if failed {
			update(StepUpdate{Index: i, Status: "skipped"})
			continue
		}
		if err := k.waitContainerStarted(ctx, s.Namespace, pod, cname); err != nil {
			update(StepUpdate{Index: i, Status: "failed", Output: err.Error() + "\n"})
			failed = true
			continue
		}
		update(StepUpdate{Index: i, Status: "running"})
		k.c.Logs(ctx, s.Namespace, pod, cname, true, false, 0, 0, func(l string) { update(StepUpdate{Index: i, Output: l + "\n"}) })
		code := k.exitCode(ctx, s.Namespace, pod, cname)
		st := "succeeded"
		if code != 0 {
			st = "failed"
			failed = true
		}
		update(StepUpdate{Index: i, Status: st, ExitCode: &code})
	}
	if failed {
		return fmt.Errorf("a step failed")
	}
	return nil
}

func (k *Kube) exitCode(ctx context.Context, ns, pod, container string) int {
	for i := 0; i < 60; i++ {
		o, err := k.c.Get(ctx, "Pod", ns, pod)
		if err != nil {
			return 1
		}
		for _, cs := range kube.Items(kube.Get(o, "status", "initContainerStatuses")) {
			if cs["name"] == container {
				if t, ok := kube.Get(cs, "state", "terminated").(map[string]any); ok {
					return int(kube.Num(t, "exitCode"))
				}
			}
		}
		if !sleep(ctx, 500*time.Millisecond) {
			return 1
		}
	}
	return 1
}

// ---- sandboxes ----

func (k *Kube) ApplySandbox(ctx context.Context, s SandboxSpec) error {
	labels := map[string]string{lblSandbox: s.ID}
	k.obs.SandboxState(s.ID, "booting", 5, "Scheduling onto a Kata-capable node")
	pvc := kube.Obj{"kind": "PersistentVolumeClaim", "metadata": meta(s.Name+"-workspace", s.Namespace, labels), "spec": map[string]any{
		"accessModes": []any{"ReadWriteOnce"},
		"resources":   map[string]any{"requests": map[string]any{"storage": fmt.Sprintf("%dGi", max(s.StorageGi, 5))}},
	}}
	if _, err := k.c.Apply(ctx, pvc); err != nil {
		return err
	}
	// Internet egress only: sandboxes can reach out but not into the cluster.
	np := kube.Obj{"kind": "NetworkPolicy", "metadata": meta(s.Name+"-egress", s.Namespace, labels), "spec": map[string]any{
		"podSelector": map[string]any{"matchLabels": map[string]any{lblSandbox: s.ID}},
		"policyTypes": []any{"Ingress", "Egress"},
		"egress": []any{
			map[string]any{"to": []any{map[string]any{"ipBlock": map[string]any{"cidr": "0.0.0.0/0", "except": []any{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "100.64.0.0/10"}}}}},
			map[string]any{"to": []any{map[string]any{"namespaceSelector": map[string]any{"matchLabels": map[string]any{"kubernetes.io/metadata.name": "kube-system"}}}}, "ports": []any{map[string]any{"port": 53, "protocol": "UDP"}}},
		},
	}}
	if _, err := k.c.Apply(ctx, np); err != nil {
		return err
	}
	init := "mkdir -p /workspace && cd /workspace"
	if s.Repo != "" {
		init += fmt.Sprintf(" && ([ -d /workspace/%s ] || git clone https://github.com/%s.git /workspace/%s)", repoDir(s.Repo), s.Repo, repoDir(s.Repo))
	}
	podSpec := map[string]any{
		"containers": []any{map[string]any{
			"name": "sandbox", "image": s.Image,
			"command":      []any{"sh", "-c", init + "; exec sleep infinity"},
			"workingDir":   "/workspace",
			"resources":    res(s.Resources),
			"volumeMounts": []any{map[string]any{"name": "workspace", "mountPath": "/workspace"}},
		}},
		"volumes": []any{map[string]any{"name": "workspace", "persistentVolumeClaim": map[string]any{"claimName": s.Name + "-workspace"}}},
	}
	if s.RuntimeClass != "" {
		podSpec["runtimeClassName"] = s.RuntimeClass
	}
	dep := kube.Obj{"kind": "Deployment", "metadata": meta(s.Name, s.Namespace, labels), "spec": map[string]any{
		"replicas": 1, "strategy": map[string]any{"type": "Recreate"},
		"selector": map[string]any{"matchLabels": map[string]any{lblSandbox: s.ID}},
		"template": map[string]any{"metadata": map[string]any{"labels": map[string]any{lblManaged: "true", lblSandbox: s.ID}}, "spec": podSpec},
	}}
	if _, err := k.c.Apply(ctx, dep); err != nil {
		return err
	}
	go func() {
		stages := map[string]struct {
			p int
			s string
		}{"Pending": {20, "Starting micro-VM"}, "ContainerCreating": {60, "Pulling sandbox image"}, "Running": {100, "Ready"}}
		for i := 0; i < 600; i++ {
			items, _, err := k.c.List(context.Background(), "Pod", s.Namespace, kube.ListOpts{LabelSelector: lblSandbox + "=" + s.ID})
			if err == nil && len(items) > 0 {
				p := podFrom(items[0])
				st := stages[p.Phase]
				if p.Reason == "ContainerCreating" {
					st = stages["ContainerCreating"]
				}
				if p.Ready {
					k.obs.SandboxState(s.ID, "running", 100, "Ready")
					return
				}
				if st.p > 0 {
					k.obs.SandboxState(s.ID, "booting", st.p, st.s)
				}
			}
			time.Sleep(2 * time.Second)
		}
		k.obs.SandboxState(s.ID, "failed", 0, "Timed out waiting for the sandbox to boot")
	}()
	return nil
}

func repoDir(repo string) string {
	if i := strings.LastIndex(repo, "/"); i >= 0 {
		return repo[i+1:]
	}
	return repo
}

func (k *Kube) DeleteSandbox(ctx context.Context, ns, name string) error {
	k.c.Delete(ctx, "Deployment", ns, name)
	k.c.Delete(ctx, "NetworkPolicy", ns, name+"-egress")
	return k.c.Delete(ctx, "PersistentVolumeClaim", ns, name+"-workspace")
}

func (k *Kube) SandboxExec(ctx context.Context, ns, name string, cmd []string, sio Stdio) error {
	pods, _, err := k.c.List(ctx, "Pod", ns, kube.ListOpts{FieldSelector: "status.phase=Running"})
	if err != nil {
		return err
	}
	var pod string
	for _, p := range pods {
		if strings.HasPrefix(kube.Str(p, "metadata", "name"), name+"-") && kube.Labels(p)[lblSandbox] != "" {
			pod = kube.Str(p, "metadata", "name")
		}
	}
	if pod == "" {
		return fmt.Errorf("sandbox is not running")
	}
	var real []string
	var stdin io.Reader = sio.Stdin
	switch {
	case len(cmd) == 0:
		real = []string{"sh", "-c", "cd /workspace; command -v bash >/dev/null && exec bash -i 2>&1 || exec sh -i 2>&1"}
	case cmd[0] == "orchard-ls":
		real = []string{"sh", "-c", "cd /workspace && find . -type f -not -path './.git/*' -not -path '*/node_modules/*' | sed 's|^./||' | sort | head -3000"}
		stdin = nil
	case cmd[0] == "orchard-read":
		real = []string{"cat", "/workspace/" + strings.TrimPrefix(cmd[1], "/")}
		stdin = nil
	case cmd[0] == "orchard-write":
		p := "/workspace/" + strings.TrimPrefix(cmd[1], "/")
		real = []string{"sh", "-c", `mkdir -p "$(dirname "$1")" && cat > "$1"`, "sh", p}
	case cmd[0] == "orchard-rm":
		real = []string{"rm", "-f", "/workspace/" + strings.TrimPrefix(cmd[1], "/")}
		stdin = nil
	case cmd[0] == "git":
		real = append([]string{"git", "-C", "/workspace"}, cmd[1:]...)
		stdin = nil
	default:
		real = cmd
	}
	return k.c.Exec(ctx, ns, pod, "sandbox", real, stdin, sio.Stdout, sio.Stdout)
}

// ---- nodes ----

func (k *Kube) Nodes(ctx context.Context) ([]Node, error) {
	items, _, err := k.c.List(ctx, "Node", "", kube.ListOpts{})
	if err != nil {
		return nil, err
	}
	pods, _, _ := k.c.List(ctx, "Pod", "", kube.ListOpts{FieldSelector: "status.phase=Running"})
	usedCPU, usedMem, count := map[string]int{}, map[string]int{}, map[string]int{}
	for _, p := range pods {
		n := kube.Str(p, "spec", "nodeName")
		count[n]++
		for _, c := range kube.Items(kube.Get(p, "spec", "containers")) {
			usedCPU[n] += kube.ParseCPU(kube.Str(c, "resources", "requests", "cpu"))
			usedMem[n] += kube.ParseMem(kube.Str(c, "resources", "requests", "memory"))
		}
	}
	var out []Node
	for _, it := range items {
		name := kube.Str(it, "metadata", "name")
		n := Node{
			Name:      name,
			CPUMillis: kube.ParseCPU(kube.Str(it, "status", "capacity", "cpu")),
			MemoryMi:  kube.ParseMem(kube.Str(it, "status", "capacity", "memory")),
			AllocCPU:  kube.ParseCPU(kube.Str(it, "status", "allocatable", "cpu")),
			AllocMem:  kube.ParseMem(kube.Str(it, "status", "allocatable", "memory")),
			Arch:      kube.Str(it, "status", "nodeInfo", "architecture"),
			Kubelet:   kube.Str(it, "status", "nodeInfo", "kubeletVersion"),
			Labels:    kube.Labels(it),
			UsedCPU:   usedCPU[name],
			UsedMem:   usedMem[name],
			Pods:      count[name],
		}
		for l := range n.Labels {
			if strings.HasPrefix(l, "node-role.kubernetes.io/") {
				n.Roles = append(n.Roles, strings.TrimPrefix(l, "node-role.kubernetes.io/"))
			}
		}
		for _, c := range kube.Items(kube.Get(it, "status", "conditions")) {
			if c["type"] == "Ready" {
				n.Ready = c["status"] == "True"
			}
		}
		for _, t := range kube.Items(kube.Get(it, "spec", "taints")) {
			n.Taints = append(n.Taints, fmt.Sprintf("%v=%v:%v", t["key"], t["value"], t["effect"]))
		}
		out = append(out, n)
	}
	return out, nil
}

func (k *Kube) SetNodePool(ctx context.Context, node, pool string, taint bool) error {
	o, err := k.c.Get(ctx, "Node", "", node)
	if err != nil {
		return err
	}
	var taints []any
	for _, t := range kube.Items(kube.Get(o, "spec", "taints")) {
		if t["key"] != lblPool {
			taints = append(taints, t)
		}
	}
	if pool != "" && taint {
		taints = append(taints, map[string]any{"key": lblPool, "value": pool, "effect": "NoSchedule"})
	}
	var label any = pool
	if pool == "" {
		label = nil
	}
	if taints == nil {
		taints = []any{}
	}
	return k.c.Patch(ctx, "Node", "", node, map[string]any{
		"metadata": map[string]any{"labels": map[string]any{lblPool: label}},
		"spec":     map[string]any{"taints": taints},
	})
}
