package store

import "time"

// Roles within an organization, most to least privileged.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleViewer = "viewer"
)

// RoleRank orders roles so permission checks can compare them.
func RoleRank(role string) int {
	switch role {
	case RoleOwner:
		return 4
	case RoleAdmin:
		return 3
	case RoleMember:
		return 2
	case RoleViewer:
		return 1
	}
	return 0
}

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"passwordHash,omitempty"`
	Superadmin   bool      `json:"superadmin"`
	AvatarSeed   string    `json:"avatarSeed"`
	GitHubLogin  string    `json:"githubLogin,omitempty"`
	GitHubToken  string    `json:"githubToken,omitempty"`
	Passkeys     []Passkey `json:"passkeys,omitempty"`
	SSHKeys      []SSHKey  `json:"sshKeys,omitempty"`
	Disabled     bool      `json:"disabled,omitempty"`
	ExternalID   string    `json:"externalId,omitempty"` // SCIM
	CreatedAt    time.Time `json:"createdAt"`
}

type Passkey struct {
	ID        string    `json:"id"` // base64url credential id
	Name      string    `json:"name"`
	PublicKey []byte    `json:"publicKey"` // uncompressed P-256 point
	SignCount uint32    `json:"signCount"`
	CreatedAt time.Time `json:"createdAt"`
}

type SSHKey struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	PublicKey   string    `json:"publicKey"`
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type APIToken struct {
	ID        string     `json:"id"`
	UserID    string     `json:"userId"`
	Name      string     `json:"name"`
	Hash      string     `json:"hash"`
	Prefix    string     `json:"prefix"`
	LastUsed  *time.Time `json:"lastUsed,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

type Quota struct {
	CPUMillis int `json:"cpuMillis"`
	MemoryMi  int `json:"memoryMi"`
	StorageGi int `json:"storageGi"`
	Apps      int `json:"apps"`
	Databases int `json:"databases"`
	Sandboxes int `json:"sandboxes"`
}

type Org struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Quota       Quota     `json:"quota"`
	MemberQuota Quota     `json:"memberQuota"` // per-member allowance inside the org cap
	Defaults    Resources `json:"defaults"`
	PublicIP    string    `json:"publicIp,omitempty"`
	Pool        string    `json:"pool,omitempty"`
	SSO         SSOConfig `json:"sso"`
	SCIMToken   string    `json:"scimTokenHash,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

type SSOConfig struct {
	Enabled      bool   `json:"enabled"`
	Issuer       string `json:"issuer,omitempty"`
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	Domain       string `json:"domain,omitempty"` // email domain auto-joined
	DefaultRole  string `json:"defaultRole,omitempty"`
}

type Membership struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"orgId"`
	UserID    string    `json:"userId"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

type Invite struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"orgId"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Token     string    `json:"token"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
}

type Environment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Project struct {
	ID           string        `json:"id"`
	OrgID        string        `json:"orgId"`
	Name         string        `json:"name"`
	Slug         string        `json:"slug"`
	Icon         string        `json:"icon"`       // emoji or icon key
	Background   string        `json:"background"` // preset name
	Environments []Environment `json:"environments"`
	Members      []string      `json:"members"` // user ids
	CreatedBy    string        `json:"createdBy"`
	CreatedAt    time.Time     `json:"createdAt"`
	UpdatedAt    time.Time     `json:"updatedAt"`
}

// Variable is a project- or app-scoped environment variable, optionally
// restricted to one environment.
type Variable struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	EnvID     string    `json:"envId,omitempty"` // empty: every environment
	AppID     string    `json:"appId,omitempty"` // empty: shared across the project
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Secret    bool      `json:"secret"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Resources struct {
	CPUMillis int `json:"cpuMillis"`
	MemoryMi  int `json:"memoryMi"`
}

type Port struct {
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"` // http, tcp, udp
	Public   bool   `json:"public"`   // raw TCP/UDP published on the org IP
	NodePort int    `json:"nodePort,omitempty"`
}

type Volume struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	SizeGi    int    `json:"sizeGi"`
}

type Domain struct {
	Host      string    `json:"host"`
	Generated bool      `json:"generated"`
	CertState string    `json:"certState"` // pending, issued, failed
	AddedAt   time.Time `json:"addedAt"`
}

type HealthCheck struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
	Port    int    `json:"port"`
	Initial int    `json:"initialDelaySeconds"`
}

type Source struct {
	Type       string `json:"type"` // image | github
	Image      string `json:"image,omitempty"`
	Repo       string `json:"repo,omitempty"` // owner/name
	Branch     string `json:"branch,omitempty"`
	Dockerfile string `json:"dockerfile,omitempty"`
	Context    string `json:"context,omitempty"`
	Target     string `json:"target,omitempty"` // build stage
	AutoDeploy bool   `json:"autoDeploy"`
}

type App struct {
	ID            string            `json:"id"`
	ProjectID     string            `json:"projectId"`
	EnvID         string            `json:"envId"`
	Name          string            `json:"name"`
	Source        Source            `json:"source"`
	Image         string            `json:"image"` // image currently deployed
	Replicas      int               `json:"replicas"`
	Ports         []Port            `json:"ports"`
	Resources     Resources         `json:"resources"`
	Volumes       []Volume          `json:"volumes"`
	Domains       []Domain          `json:"domains"`
	Health        HealthCheck       `json:"health"`
	AuthWall      bool              `json:"authWall"`
	Command       string            `json:"command,omitempty"`
	Sandboxed     bool              `json:"sandboxed"`
	Status        string            `json:"status"` // pending, building, deploying, running, degraded, failed, stopped
	StatusMsg     string            `json:"statusMessage,omitempty"`
	Pods          []Pod             `json:"pods"`
	TemplateID    string            `json:"templateInstanceId,omitempty"`
	ComposeID     string            `json:"composeId,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	CreatedBy     string            `json:"createdBy"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
	DeployedAt    *time.Time        `json:"deployedAt,omitempty"`
	CurrentDeploy string            `json:"currentDeploy,omitempty"`
}

type Pod struct {
	Name      string    `json:"name"`
	Phase     string    `json:"phase"` // Pending, Running, Succeeded, Failed
	Ready     bool      `json:"ready"`
	Restarts  int       `json:"restarts"`
	Node      string    `json:"node"`
	Reason    string    `json:"reason,omitempty"`
	StartedAt time.Time `json:"startedAt"`
}

type Step struct {
	Name       string     `json:"name"`
	Status     string     `json:"status"` // pending, running, succeeded, failed, skipped
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// Deploy is one build-and-rollout (or rollback) of an app. Every deploy
// keeps its image so rollbacks are one click.
type Deploy struct {
	ID         string     `json:"id"`
	AppID      string     `json:"appId"`
	Number     int        `json:"number"`
	Kind       string     `json:"kind"` // build, image, rollback, restart
	Image      string     `json:"image"`
	Commit     string     `json:"commit,omitempty"`
	Message    string     `json:"message,omitempty"`
	Trigger    string     `json:"trigger"` // manual, push, api, mcp
	Status     string     `json:"status"`  // queued, running, succeeded, failed, superseded
	Steps      []Step     `json:"steps"`
	Error      string     `json:"error,omitempty"`
	CreatedBy  string     `json:"createdBy"`
	CreatedAt  time.Time  `json:"createdAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

type Backup struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	SizeBytes int64     `json:"sizeBytes"`
	Method    string    `json:"method"` // scheduled, manual
	CreatedAt time.Time `json:"createdAt"`
}

type Database struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"projectId"`
	EnvID       string    `json:"envId"`
	Name        string    `json:"name"`
	Version     int       `json:"version"`
	StorageGi   int       `json:"storageGi"`
	Resources   Resources `json:"resources"`
	Mode        string    `json:"mode"` // shared, dedicated
	DBName      string    `json:"dbName"`
	User        string    `json:"user"`
	Password    string    `json:"password"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	Status      string    `json:"status"`    // provisioning, ready, stopped, failed
	Instances   int       `json:"instances"` // 1 primary + read replicas
	Extensions  []string  `json:"extensions"`
	Backups     []Backup  `json:"backups"`
	BackupCron  string    `json:"backupSchedule"`
	PublicPort  int       `json:"publicPort,omitempty"`
	SizeBytes   int64     `json:"sizeBytes"`
	Connections int       `json:"connections"`
	TemplateID  string    `json:"templateInstanceId,omitempty"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type JobStep struct {
	Name    string `json:"name"`
	Type    string `json:"type"`           // script, app, image
	Lang    string `json:"lang,omitempty"` // bash, python, node
	Source  string `json:"source,omitempty"`
	AppID   string `json:"appId,omitempty"`
	Image   string `json:"image,omitempty"`
	Command string `json:"command,omitempty"`
}

type Job struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"projectId"`
	EnvID       string     `json:"envId"`
	Name        string     `json:"name"`
	Schedule    string     `json:"schedule,omitempty"`
	Concurrency string     `json:"concurrency"` // skip, queue, allow
	Steps       []JobStep  `json:"steps"`
	Paused      bool       `json:"paused"`
	LastRunAt   *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt   *time.Time `json:"nextRunAt,omitempty"`
	CreatedBy   string     `json:"createdBy"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type RunStep struct {
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	Image      string     `json:"image"`
	Status     string     `json:"status"`
	ExitCode   *int       `json:"exitCode,omitempty"`
	Output     string     `json:"output"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

type JobRun struct {
	ID         string     `json:"id"`
	JobID      string     `json:"jobId"`
	Number     int        `json:"number"`
	Trigger    string     `json:"trigger"` // manual, schedule, cli, mcp
	Status     string     `json:"status"`  // queued, running, succeeded, failed, cancelled
	Steps      []RunStep  `json:"steps"`
	CreatedBy  string     `json:"createdBy"`
	CreatedAt  time.Time  `json:"createdAt"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

type TemplateInstance struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"projectId"`
	EnvID      string    `json:"envId"`
	TemplateID string    `json:"templateId"`
	Name       string    `json:"name"`
	Label      string    `json:"label"`
	Status     string    `json:"status"`
	CreatedBy  string    `json:"createdBy"`
	CreatedAt  time.Time `json:"createdAt"`
}

type ComposeStack struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	EnvID     string    `json:"envId"`
	Source    string    `json:"source"`
	AppIDs    []string  `json:"appIds"`
	UpdatedBy string    `json:"updatedBy"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Sandbox struct {
	ID            string         `json:"id"`
	OrgID         string         `json:"orgId"`
	OwnerID       string         `json:"ownerId"`
	Name          string         `json:"name"`
	Image         string         `json:"image"`
	Repo          string         `json:"repo,omitempty"`
	Status        string         `json:"status"` // booting, running, stopped, failed
	BootProgress  int            `json:"bootProgress"`
	BootStage     string         `json:"bootStage"`
	Resources     Resources      `json:"resources"`
	StorageGi     int            `json:"storageGi"`
	Conversations []Conversation `json:"conversations"`
	CreatedAt     time.Time      `json:"createdAt"`
}

type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Cwd       string    `json:"cwd"`
	Messages  []Message `json:"messages"`
	CreatedAt time.Time `json:"createdAt"`
}

type Message struct {
	Role      string    `json:"role"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

type AuditEntry struct {
	ID        string            `json:"id"`
	OrgID     string            `json:"orgId"`
	ActorID   string            `json:"actorId"`
	Actor     string            `json:"actor"`
	Action    string            `json:"action"`
	Target    string            `json:"target"`
	Meta      map[string]string `json:"meta,omitempty"`
	IP        string            `json:"ip,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
}

// CrashReport keeps the last output of a container that died, which
// Kubernetes would otherwise throw away on restart.
type CrashReport struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"ownerId"` // app or database id
	Pod       string    `json:"pod"`
	Container string    `json:"container"`
	Reason    string    `json:"reason"`
	ExitCode  int       `json:"exitCode"`
	Logs      string    `json:"logs"`
	CreatedAt time.Time `json:"createdAt"`
}

// KubeEvent is a translated Kubernetes warning event.
type KubeEvent struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"ownerId"`
	Object    string    `json:"object"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Plain     string    `json:"plain"`
	Count     int       `json:"count"`
	FirstSeen time.Time `json:"firstSeen"`
	LastSeen  time.Time `json:"lastSeen"`
}

type Pool struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Nodes       []string `json:"nodes"`
	Taint       bool     `json:"taint"`
	OrgIDs      []string `json:"orgIds"`
}

type Settings struct {
	InstanceName   string          `json:"instanceName"`
	Domain         string          `json:"domain"`
	AppDomain      string          `json:"appDomain"`
	MCPDomain      string          `json:"mcpDomain"`
	MCPEnabled     bool            `json:"mcpEnabled"`
	IngressMode    string          `json:"ingressMode"` // public, tunnel, lan
	IngressCNAME   string          `json:"ingressCname"`
	HTTPPort       int             `json:"httpPort"`
	HTTPSPort      int             `json:"httpsPort"`
	PublicIP       string          `json:"publicIp"`
	PublicDBDomain string          `json:"publicDbDomain"`
	SignupMode     string          `json:"signupMode"` // open, invite, closed
	Hostnames      []string        `json:"hostnames"`
	BuildSlots     int             `json:"buildSlots"`
	GitHub         GitHubAppConfig `json:"github"`
	TenantSandbox  bool            `json:"tenantSandbox"`
	BuilderSandbox bool            `json:"builderSandbox"`
	SetupTokenHash string          `json:"setupTokenHash,omitempty"`
	SetupExpires   *time.Time      `json:"setupExpires,omitempty"`
	AnthropicKey   string          `json:"anthropicKey,omitempty"`
	Secret         string          `json:"secret,omitempty"` // HMAC key for auth-wall cookies
}

type GitHubAppConfig struct {
	AppID         string `json:"appId,omitempty"`
	Slug          string `json:"slug,omitempty"`
	ClientID      string `json:"clientId,omitempty"`
	ClientSecret  string `json:"clientSecret,omitempty"`
	PrivateKey    string `json:"privateKey,omitempty"`
	WebhookSecret string `json:"webhookSecret,omitempty"`
}
