// Package runtime is the boundary between Orchard's control plane and the
// thing that actually runs workloads. The kube driver talks to a real
// Kubernetes API; the sim driver fakes a cluster in-process so the whole
// dashboard can be developed and demoed on a laptop.
package runtime

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

var ErrUnsupported = errors.New("not supported by this runtime")

// AppSpec is the desired state of one app, fully resolved: variables are
// merged, domains are final, image is pinned.
type AppSpec struct {
	Namespace    string
	Name         string
	ID           string
	Org          string
	Pool         string
	Image        string
	Replicas     int
	Command      string
	Ports        []store.Port
	Env          map[string]string
	Secrets      map[string]string
	Resources    store.Resources
	Volumes      []store.Volume
	Domains      []store.Domain
	Health       store.HealthCheck
	AuthWall     bool
	Sandboxed    bool
	RuntimeClass string
	RestartNonce string // bump to roll pods on the same image
}

type BuildSpec struct {
	Namespace   string
	DeployID    string
	AppName     string
	Repo        string // owner/name
	CloneURL    string
	Branch      string
	Commit      string
	Dockerfile  string
	Context     string
	Target      string
	Image       string // destination ref
	BuildArgs   map[string]string
	PullSecrets map[string]string // DHI_USERNAME, DHI_TOKEN, DOCKER_AUTH_CONFIG
	Sandboxed   bool
}

type DatabaseSpec struct {
	Namespace  string
	Name       string
	ID         string
	Version    int
	Instances  int
	StorageGi  int
	Resources  store.Resources
	DBName     string
	User       string
	Password   string
	Extensions []string
	BackupCron string
	PublicPort int
	Stopped    bool
}

type JobSpec struct {
	Namespace string
	Name      string
	RunID     string
	Steps     []ResolvedStep
	Env       map[string]string
}

type ResolvedStep struct {
	Name    string
	Image   string
	Command []string
	Script  string // for script steps
	Lang    string
}

type SandboxSpec struct {
	Namespace    string
	Name         string
	ID           string
	Image        string
	Repo         string
	Resources    store.Resources
	StorageGi    int
	RuntimeClass string
}

type Metrics struct {
	CPUMillis float64 `json:"cpuMillis"`
	MemoryMi  float64 `json:"memoryMi"`
}

type Node struct {
	Name      string            `json:"name"`
	Ready     bool              `json:"ready"`
	Roles     []string          `json:"roles"`
	CPUMillis int               `json:"cpuMillis"`
	MemoryMi  int               `json:"memoryMi"`
	AllocCPU  int               `json:"allocatableCpuMillis"`
	AllocMem  int               `json:"allocatableMemoryMi"`
	UsedCPU   int               `json:"requestedCpuMillis"`
	UsedMem   int               `json:"requestedMemoryMi"`
	Pods      int               `json:"pods"`
	Arch      string            `json:"arch"`
	Kubelet   string            `json:"kubelet"`
	Labels    map[string]string `json:"labels"`
	Taints    []string          `json:"taints"`
}

type QueryResult struct {
	Columns  []string   `json:"columns"`
	Rows     [][]string `json:"rows"`
	Command  string     `json:"command"`
	Duration float64    `json:"durationMs"`
}

// StepUpdate reports progress of one job step.
type StepUpdate struct {
	Index    int
	Status   string
	ExitCode *int
	Output   string // appended output
}

// Observer receives mirrored cluster state. The control plane implements
// it; drivers call it from their informers.
type Observer interface {
	AppPods(appID string, pods []store.Pod)
	DatabaseState(dbID string, status string, sizeBytes int64, connections int)
	Crash(ownerID string, c store.CrashReport)
	Warning(ownerID string, e store.KubeEvent)
	AppLog(appID string, pod string, line string)
	SandboxState(id, status string, progress int, stage string)
}

// Stdio for interactive exec sessions.
type Stdio struct {
	Stdin  io.Reader
	Stdout io.Writer
	Resize <-chan [2]uint16 // cols, rows
}

type Driver interface {
	Name() string
	Start(ctx context.Context, obs Observer) error

	Build(ctx context.Context, spec BuildSpec, log func(string)) error
	ApplyApp(ctx context.Context, spec AppSpec) error
	DeleteApp(ctx context.Context, namespace, name string) error
	Logs(ctx context.Context, namespace, name, pod string, previous bool, since time.Duration, out func(pod, line string)) error
	Exec(ctx context.Context, namespace, name, pod string, cmd []string, io Stdio) error
	AppMetrics(ctx context.Context, namespace, name string) (Metrics, error)

	ApplyDatabase(ctx context.Context, spec DatabaseSpec) error
	DeleteDatabase(ctx context.Context, namespace, name string) error
	Query(ctx context.Context, spec DatabaseSpec, sql string) (*QueryResult, error)
	Backup(ctx context.Context, spec DatabaseSpec) (int64, error)
	DatabaseShell(ctx context.Context, spec DatabaseSpec, io Stdio) error

	RunJob(ctx context.Context, spec JobSpec, update func(StepUpdate)) error

	ApplySandbox(ctx context.Context, spec SandboxSpec) error
	DeleteSandbox(ctx context.Context, namespace, name string) error
	SandboxExec(ctx context.Context, namespace, name string, cmd []string, io Stdio) error

	EnsureNamespace(ctx context.Context, namespace string, labels map[string]string) error
	DeleteNamespace(ctx context.Context, namespace string) error
	Nodes(ctx context.Context) ([]Node, error)
	SetNodePool(ctx context.Context, node, pool string, taint bool) error
}
