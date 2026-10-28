package api

// Wire types for /api/v1. TypeScript definitions are generated from this file by tygo
// (see tygo.yaml), so keep JSON tags authoritative and avoid unexported shapes.

type User struct {
	ID           int64   `json:"id"`
	Username     string  `json:"username"`
	Email        string  `json:"email"`
	DisplayName  string  `json:"display_name"`
	AuthProvider string  `json:"auth_provider"`
	Role         string  `json:"role" tstype:"Role"`
	Disabled     bool    `json:"disabled"`
	LastLoginAt  *string `json:"last_login_at" tstype:"string | null,required"`
	CreatedAt    string  `json:"created_at"`
}

type OIDCProvider struct {
	Enabled bool   `json:"enabled"`
	Name    string `json:"name"`
}

type AuthProviders struct {
	Local bool         `json:"local"`
	OIDC  OIDCProvider `json:"oidc"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Me is returned by GET /me and POST /auth/login.
type Me struct {
	User        User     `json:"user"`
	CSRFToken   string   `json:"csrf_token"`
	Permissions []string `json:"permissions" tstype:"Action[]"`
}

type Health struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type Project struct {
	ID           int64  `json:"id"`
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	Provider     string `json:"provider" tstype:"Provider"`
	RepoURL      string `json:"repo_url"`
	WorkDir      string `json:"work_dir"`
	ScriptSource string `json:"script_source" tstype:"ScriptSource"`
	ScriptPath   string `json:"script_path"`
	AutoPull     bool   `json:"auto_pull"`
	TimeoutSec   int    `json:"timeout_sec"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	// LastDeployment is the newest deployment, if any.
	LastDeployment *Deployment `json:"last_deployment" tstype:"Deployment | null,required"`
	// Health covers the last finished deployments; null when there are none.
	Health *ProjectHealth `json:"health" tstype:"ProjectHealth | null,required"`
}

// ProjectHealth: Passed of the last Total successful or failed deployments succeeded.
type ProjectHealth struct {
	Passed int `json:"passed"`
	Total  int `json:"total"`
}

type CreateProjectRequest struct {
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Provider     string `json:"provider" tstype:"Provider"`
	RepoURL      string `json:"repo_url"`
	WorkDir      string `json:"work_dir"`
	ScriptSource string `json:"script_source" tstype:"ScriptSource"`
	ScriptPath   string `json:"script_path"`
	AutoPull     bool   `json:"auto_pull"`
	TimeoutSec   int    `json:"timeout_sec"`
	Enabled      bool   `json:"enabled"`
	// DefaultBranch, when set, creates a first trigger rule deploying pushes to it.
	DefaultBranch string `json:"default_branch"`
}

// ProjectSecrets is admin-only: what to paste into GitHub/Gitea.
type ProjectSecrets struct {
	WebhookPath    string `json:"webhook_path"`
	WebhookURL     string `json:"webhook_url"` // empty when base_url is not configured
	WebhookSecret  string `json:"webhook_secret"`
	PublicKey      string `json:"public_key"`
	KeyFingerprint string `json:"key_fingerprint"`
}

type ConnectionTest struct {
	OK       bool     `json:"ok"`
	Message  string   `json:"message"`
	Branches []string `json:"branches"`
}

type Script struct {
	Source   string `json:"source" tstype:"ScriptSource"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Exists   bool   `json:"exists"`
	Editable bool   `json:"editable"`
}

type SaveScriptRequest struct {
	Content string `json:"content"`
}

type ScriptRevision struct {
	ID        int64  `json:"id"`
	Content   string `json:"content"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
}

// HookResponse is returned to GitHub/Gitea and shown in their delivery logs.
type HookResponse struct {
	Status       string `json:"status"` // pong | queued | skipped | ignored | rejected
	Reason       string `json:"reason"`
	DeploymentID int64  `json:"deployment_id,omitempty"`
}

type TriggerRule struct {
	ID           int64             `json:"id"`
	Name         string            `json:"name"`
	Enabled      bool              `json:"enabled"`
	Position     int               `json:"position"`
	Event        string            `json:"event" tstype:"TriggerEvent"`
	RefPattern   string            `json:"ref_pattern"`
	PathsInclude []string          `json:"paths_include"`
	PathsExclude []string          `json:"paths_exclude"`
	SkipKeyword  string            `json:"skip_keyword"`
	ExtraEnv     map[string]string `json:"extra_env"` // values are masked for non-editors
	// LastMatch is the newest deployment a webhook push started through this rule.
	LastMatch *RuleMatch `json:"last_match" tstype:"RuleMatch | null,required"`
}

type RuleMatch struct {
	DeploymentID int64  `json:"deployment_id"`
	At           string `json:"at"`
}

type RuleInput struct {
	Name         string            `json:"name"`
	Enabled      bool              `json:"enabled"`
	Event        string            `json:"event" tstype:"TriggerEvent"`
	RefPattern   string            `json:"ref_pattern"`
	PathsInclude []string          `json:"paths_include"`
	PathsExclude []string          `json:"paths_exclude"`
	SkipKeyword  string            `json:"skip_keyword"`
	ExtraEnv     map[string]string `json:"extra_env"`
}

type RuleOrderRequest struct {
	IDs []int64 `json:"ids"`
}

// RuleTestRequest simulates a push against the project's rules.
type RuleTestRequest struct {
	Event   string   `json:"event" tstype:"TriggerEvent"`
	Ref     string   `json:"ref"` // branch or tag name
	Message string   `json:"message"`
	Files   []string `json:"files"` // empty = unknown
}

type RuleTestResult struct {
	RuleID  int64  `json:"rule_id"`
	Name    string `json:"name"`
	Matched bool   `json:"matched"`
	Reason  string `json:"reason"`
}

type RuleTestResponse struct {
	MatchedRuleID *int64           `json:"matched_rule_id" tstype:"number | null,required"`
	Results       []RuleTestResult `json:"results"`
}

type WebhookEvent struct {
	ID           int64  `json:"id"`
	Provider     string `json:"provider" tstype:"Provider"`
	Event        string `json:"event"`
	DeliveryID   string `json:"delivery_id"`
	Ref          string `json:"ref"`
	CommitSHA    string `json:"commit_sha"`
	Matched      bool   `json:"matched"`
	Status       string `json:"status" tstype:"DeliveryStatus"` // HookResponse.Status that was sent back
	Reason       string `json:"reason"`
	DeploymentID *int64 `json:"deployment_id" tstype:"number | null,required"`
	ReceivedAt   string `json:"received_at"`
}

type Deployment struct {
	ID            int64  `json:"id"`
	ProjectID     int64  `json:"project_id"`
	RuleID        *int64 `json:"rule_id" tstype:"number | null,required"`
	RuleName      string `json:"rule_name"`
	Trigger       string `json:"trigger" tstype:"DeployTrigger"`
	TriggeredBy   string `json:"triggered_by"` // username for manual deploys
	Ref           string `json:"ref"`
	CommitSHA     string `json:"commit_sha"`
	CommitMessage string `json:"commit_message"`
	Author        string `json:"author"`
	Status        string `json:"status" tstype:"DeployStatus"`
	ExitCode      *int   `json:"exit_code" tstype:"number | null,required"`
	// Timestamps have millisecond precision. Stages: queued_at → started_at (trigger),
	// started_at → script_started_at (repository update), script_started_at → finished_at (script).
	QueuedAt        string  `json:"queued_at"`
	StartedAt       *string `json:"started_at" tstype:"string | null,required"`
	ScriptStartedAt *string `json:"script_started_at" tstype:"string | null,required"`
	FinishedAt      *string `json:"finished_at" tstype:"string | null,required"`
	// Error is a one-line reason for a failed, timed-out or canceled deployment.
	Error string `json:"error"`
	// AutoPull: whether the run updated the repository before the script; null until
	// it starts, and for deployments recorded before degoploy kept it.
	AutoPull *bool `json:"auto_pull" tstype:"boolean | null,required"`
	// ScriptRevisionID: the managed script revision the run executed; null for
	// repository scripts, runs that never reached the script and older deployments.
	ScriptRevisionID *int64 `json:"script_revision_id" tstype:"number | null,required"`
}

// DeploymentNeighbors are the ids of the same project's previous and next deployments.
type DeploymentNeighbors struct {
	Prev *int64 `json:"prev" tstype:"number | null,required"`
	Next *int64 `json:"next" tstype:"number | null,required"`
}

// LogTimes says when deployment output arrived: each entry is [byte offset, Unix ms],
// and a log line was written at the time of the last entry at or before its start.
// Empty for deployments recorded before degoploy kept write times.
type LogTimes struct {
	Entries [][2]int64 `json:"entries" tstype:"number[][]"`
}

// DeployRequest starts a manual deployment. Ref is a branch name or a full ref
// (refs/heads/main, refs/tags/v1.0.0).
type DeployRequest struct {
	Ref string `json:"ref"`
}

type CreateUserRequest struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role" tstype:"Role"`
	Password    string `json:"password"`
}

type SetPasswordRequest struct {
	Password string `json:"password"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type AuditEntry struct {
	ID         int64          `json:"id"`
	UserID     *int64         `json:"user_id" tstype:"number | null,required"`
	Username   string         `json:"username"`
	Action     string         `json:"action"`
	TargetType string         `json:"target_type"`
	TargetID   string         `json:"target_id"`
	Detail     map[string]any `json:"detail" tstype:"AuditDetail"`
	IP         string         `json:"ip"`
	CreatedAt  string         `json:"created_at"`
}

type AuditPage struct {
	Entries []AuditEntry `json:"entries"`
	// NextBefore is the cursor for the next (older) page, or null at the end.
	NextBefore *int64 `json:"next_before" tstype:"number | null,required"`
}

// UpdateProjectRequest is a partial update; omitted fields keep their value.
type UpdateProjectRequest struct {
	Name         *string `json:"name,omitempty"`
	Provider     *string `json:"provider,omitempty" tstype:"Provider"`
	RepoURL      *string `json:"repo_url,omitempty"`
	WorkDir      *string `json:"work_dir,omitempty"`
	ScriptSource *string `json:"script_source,omitempty" tstype:"ScriptSource"`
	ScriptPath   *string `json:"script_path,omitempty"`
	AutoPull     *bool   `json:"auto_pull,omitempty"`
	TimeoutSec   *int    `json:"timeout_sec,omitempty"`
	Enabled      *bool   `json:"enabled,omitempty"`
}

// UpdateUserRequest is a partial update; omitted fields keep their value.
type UpdateUserRequest struct {
	Email       *string `json:"email,omitempty"`
	DisplayName *string `json:"display_name,omitempty"`
	Role        *string `json:"role,omitempty" tstype:"Role"`
	Disabled    *bool   `json:"disabled,omitempty"`
}

// SSOLogin is one recent SSO sign-in attempt, kept in memory for the admin debug page.
type SSOLogin struct {
	At       string         `json:"at"`
	Subject  string         `json:"subject"`
	Username string         `json:"username"` // degoploy account, if one was used/created
	Email    string         `json:"email"`
	Groups   []string       `json:"groups"`
	Role     string         `json:"role"` // mapped role, empty when not allowed
	Outcome  string         `json:"outcome"`
	Claims   map[string]any `json:"claims" tstype:"AuditDetail"`
}

// SSOStatus describes the OIDC setup (no secrets) and recent sign-ins.
type SSOStatus struct {
	Enabled         bool              `json:"enabled"`
	LocalLogin      bool              `json:"local_login"`
	ProviderName    string            `json:"provider_name"`
	Issuer          string            `json:"issuer"`
	ClientID        string            `json:"client_id"`
	RedirectURL     string            `json:"redirect_url"`
	Scopes          []string          `json:"scopes"`
	GroupsClaim     string            `json:"groups_claim"`
	RoleMapping     map[string]string `json:"role_mapping"`
	SyncRoleOnLogin bool              `json:"sync_role_on_login"`
	RecentLogins    []SSOLogin        `json:"recent_logins"`
}
