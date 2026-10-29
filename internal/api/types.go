package api

// Wire types for /api/v1. TypeScript definitions are generated from this file by tygo
// (see tygo.yaml), so keep JSON tags authoritative and avoid unexported shapes.

type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	DisplayName  string `json:"display_name"`
	AuthProvider string `json:"auth_provider"`
	Role         string `json:"role" tstype:"Role"`
	Disabled     bool   `json:"disabled"`
	// MustChangePassword: the API refuses everything but a password change.
	MustChangePassword bool    `json:"must_change_password"`
	LastLoginAt        *string `json:"last_login_at" tstype:"string | null,required"`
	CreatedAt          string  `json:"created_at"`
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
	// NodeVersion is "auto", "none" or a version spec such as "22" or "lts".
	NodeVersion string `json:"node_version"`
	// NodeUsed is the Node.js version the latest deployment ran with ("" if none).
	NodeUsed  string `json:"node_used"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
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

// Variable is a project environment variable. Secret values are never sent:
// Value is empty for secrets, and masked for users who may not manage secrets.
type Variable struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Value     string `json:"value"`
	Secret    bool   `json:"secret"`
	UpdatedAt string `json:"updated_at"`
	UpdatedBy string `json:"updated_by"` // username; empty if the user was deleted
}

// VariableInput creates or updates a variable. On update, a nil Value keeps a
// secret's stored value; plain variables always need one.
type VariableInput struct {
	Name   string  `json:"name"`
	Value  *string `json:"value" tstype:"string,omitempty"`
	Secret bool    `json:"secret"`
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

// RoleSummary describes a built-in role: what it may do and who has it.
type RoleSummary struct {
	Role        string       `json:"role" tstype:"Role"`
	Permissions []string     `json:"permissions" tstype:"Action[]"`
	Members     []RoleMember `json:"members"`
}

type RoleMember struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Disabled    bool   `json:"disabled"`
}

// EmailStatus describes the SMTP settings (never the password) and whether
// invites can be sent.
type EmailStatus struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Encryption  string `json:"encryption" tstype:"'starttls' | 'tls' | 'none'"`
	Username    string `json:"username"`
	PasswordSet bool   `json:"password_set"`
	FromName    string `json:"from_name"`
	FromAddress string `json:"from_address"`
	// Locked maps settings that come from the environment to their source.
	Locked     map[string]string `json:"locked"`
	ConfigPath string            `json:"config_path"`
	// Configured: host, port and from address are set.
	Configured bool `json:"configured"`
	// Ready: a test email was delivered with the current settings, so invites go out.
	Ready    bool       `json:"ready"`
	LastTest *EmailTest `json:"last_test" tstype:"EmailTest | null,required"`
}

// EmailTest is the latest test email. Current is false when the settings
// changed since it was sent.
type EmailTest struct {
	To      string `json:"to"`
	OK      bool   `json:"ok"`
	Detail  string `json:"detail"`
	At      string `json:"at"`
	Current bool   `json:"current"`
}

// UpdateEmailRequest is a partial update of the SMTP settings. Password is write-only.
type UpdateEmailRequest struct {
	Host        *string `json:"host,omitempty"`
	Port        *int    `json:"port,omitempty"`
	Encryption  *string `json:"encryption,omitempty" tstype:"'starttls' | 'tls' | 'none'"`
	Username    *string `json:"username,omitempty"`
	Password    *string `json:"password,omitempty"`
	FromName    *string `json:"from_name,omitempty"`
	FromAddress *string `json:"from_address,omitempty"`
}

type TestEmailRequest struct {
	To string `json:"to"`
}

// Invite is a pending invitation to create a local account.
type Invite struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role" tstype:"Role"`
	InvitedBy string `json:"invited_by"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	Expired   bool   `json:"expired"`
}

// CreateInvitesRequest invites every address with the same role.
type CreateInvitesRequest struct {
	Emails []string `json:"emails"`
	Role   string   `json:"role" tstype:"Role"`
}

// InviteLink carries the link token, which is returned only when an invite is
// created or renewed. The accept page lives at /invite/{token}.
type InviteLink struct {
	Invite Invite `json:"invite"`
	Token  string `json:"token"`
	// EmailError is why the invitation email could not be sent; empty when it was.
	EmailError string `json:"email_error"`
}

// InvitePreview is what the accept page shows before the account exists.
type InvitePreview struct {
	Email     string `json:"email"`
	Role      string `json:"role" tstype:"Role"`
	InvitedBy string `json:"invited_by"`
	ExpiresAt string `json:"expires_at"`
}

type AcceptInviteRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

type SetPasswordRequest struct {
	Password string `json:"password"`
	// RequireChange makes the user pick their own password at next sign-in.
	RequireChange bool `json:"require_change"`
}

// UserSessions counts the sessions a password reset would end.
type UserSessions struct {
	Active int `json:"active"`
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
	// Sensitive marks actions that touch credentials or access.
	Sensitive bool `json:"sensitive"`
}

type AuditPage struct {
	Entries []AuditEntry `json:"entries"`
	// NextBefore is the cursor for the next (older) page, or null at the end.
	NextBefore *int64 `json:"next_before" tstype:"number | null,required"`
	// Total counts every entry matching the filters, across all pages.
	Total int `json:"total"`
	// DayCounts counts matching entries per local day ("2006-01-02", in the
	// zone given by the tz parameter) for the days present in this page.
	DayCounts map[string]int `json:"day_counts"`
}

// AuditSummary counts entries since a point in time (the page uses local midnight).
type AuditSummary struct {
	Events    int `json:"events"`
	Sensitive int `json:"sensitive"`
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
	NodeVersion  *string `json:"node_version,omitempty"`
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

// SSOStatus describes the sign-in settings (never the client secret), who uses
// each method, and recent SSO sign-ins.
type SSOStatus struct {
	Enabled         bool              `json:"enabled"`
	LocalLogin      bool              `json:"local_login"`
	ProviderName    string            `json:"provider_name"`
	Issuer          string            `json:"issuer"`
	ClientID        string            `json:"client_id"`
	ClientSecretSet bool              `json:"client_secret_set"`
	RedirectURL     string            `json:"redirect_url"`
	Scopes          []string          `json:"scopes"`
	GroupsClaim     string            `json:"groups_claim"`
	RoleMapping     map[string]string `json:"role_mapping"`
	SyncRoleOnLogin bool              `json:"sync_role_on_login"`
	// Locked maps settings that come from the environment ("oidc.issuer") to
	// their source ("DEGOPLOY_OIDC_ISSUER" or "${VAR}"); they cannot be edited here.
	Locked map[string]string `json:"locked"`
	// ConfigPath is the YAML file that saving writes; empty when there is none.
	ConfigPath     string     `json:"config_path"`
	LocalUsers     int        `json:"local_users"`
	LastLocalLogin *string    `json:"last_local_login" tstype:"string | null,required"`
	SSOUsers       int        `json:"sso_users"`
	LastSSOLogin   *string    `json:"last_sso_login" tstype:"string | null,required"`
	RecentLogins   []SSOLogin `json:"recent_logins"`
}

// UpdateSSORequest is a partial update of the sign-in settings; omitted fields
// keep their value. ClientSecret is write-only.
type UpdateSSORequest struct {
	LocalLogin      *bool             `json:"local_login,omitempty"`
	Enabled         *bool             `json:"enabled,omitempty"`
	ProviderName    *string           `json:"provider_name,omitempty"`
	Issuer          *string           `json:"issuer,omitempty"`
	ClientID        *string           `json:"client_id,omitempty"`
	ClientSecret    *string           `json:"client_secret,omitempty"`
	RedirectURL     *string           `json:"redirect_url,omitempty"`
	Scopes          []string          `json:"scopes,omitempty"`
	GroupsClaim     *string           `json:"groups_claim,omitempty"`
	RoleMapping     map[string]string `json:"role_mapping,omitempty"`
	SyncRoleOnLogin *bool             `json:"sync_role_on_login,omitempty"`
}

type DiscoverRequest struct {
	Issuer string `json:"issuer"`
}

// OIDCDiscovery is the part of a provider's discovery document the SSO page shows.
type OIDCDiscovery struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	UserinfoEndpoint      string   `json:"userinfo_endpoint"`
	JWKSURI               string   `json:"jwks_uri"`
	ScopesSupported       []string `json:"scopes_supported"`
	ClaimsSupported       []string `json:"claims_supported"`
	CheckedAt             string   `json:"checked_at"`
}

// NodeStatus is the Node.js page: the versions installed for deploy scripts.
type NodeStatus struct {
	// Platform is the build degoploy downloads, such as "linux-x64".
	Platform string `json:"platform"`
	Mirror   string `json:"mirror"`
	// Default is the spec used by projects on "auto" whose repository names
	// no version; "" when none is set.
	Default   string        `json:"default"`
	Installed []NodeVersion `json:"installed"`
	TotalSize int64         `json:"total_size"`
	// Installing lists downloads in progress, started here, from the CLI or by a deployment.
	Installing []NodeProgress `json:"installing"`
}

type NodeProgress struct {
	Version string `json:"version"`
	Phase   string `json:"phase" tstype:"'downloading' | 'unpacking'"`
	Done    int64  `json:"done"`
	// Total is 0 when the mirror does not say how big the download is.
	Total int64 `json:"total"`
}

type NodeVersion struct {
	Version     string `json:"version"`
	LTS         string `json:"lts"`
	Size        int64  `json:"size"`
	InstalledAt string `json:"installed_at"`
	// Default: the default spec picks this version.
	Default bool `json:"default"`
	// UsedBy names the projects whose latest deployment ran with this version.
	UsedBy []string `json:"used_by"`
	// Pinned names the projects that ask for exactly this version; update keeps it.
	Pinned []string `json:"pinned"`
}

// NodeRelease is the newest release of one major line.
type NodeRelease struct {
	Version   string `json:"version"`
	LTS       string `json:"lts"`
	Date      string `json:"date"`
	Installed bool   `json:"installed"`
}

// NodeSpecRequest names a version: "22", "20.11.1", "lts" or "" (default only).
type NodeSpecRequest struct {
	Spec string `json:"spec"`
}

type NodeUpdateResult struct {
	Changes []NodeChange `json:"changes"`
	Status  NodeStatus   `json:"status"`
}

type NodeChange struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Removed: From was removed; false when a project pins it.
	Removed bool `json:"removed"`
}
