// Package audit records security-relevant actions.
package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"

	"github.com/degovan/degoploy/internal/store"
)

const (
	LoginSuccess = "auth.login"
	LoginFailed  = "auth.login_failed"
	Logout       = "auth.logout"

	ProjectCreate           = "project.create"
	ProjectUpdate           = "project.update"
	ProjectDelete           = "project.delete"
	ProjectSecretsView      = "project.secrets_view"
	ProjectKeyRegenerate    = "project.key_regenerate"
	ProjectSecretRegenerate = "project.secret_regenerate"
	ProjectScriptUpdate     = "project.script_update"

	RuleCreate  = "rule.create"
	RuleUpdate  = "rule.update"
	RuleDelete  = "rule.delete"
	RuleReorder = "rule.reorder"

	VariableCreate = "variable.create"
	VariableUpdate = "variable.update"
	VariableDelete = "variable.delete"

	DeploymentManual   = "deployment.manual"
	DeploymentCancel   = "deployment.cancel"
	DeploymentRedeploy = "deployment.redeploy"

	UserCreate        = "user.create"
	UserUpdate        = "user.update"
	UserDelete        = "user.delete"
	UserPasswordReset = "user.password_reset"
	PasswordChange    = "auth.password_change"

	InviteCreate = "invite.create"
	InviteRenew  = "invite.renew"
	InviteRevoke = "invite.revoke"
	InviteAccept = "invite.accept"

	SSOUpdate = "sso.update"

	EmailUpdate = "email.update"
	EmailTest   = "email.test"
)

// sensitive actions touch credentials or access; the audit log highlights them.
var sensitive = map[string]bool{
	LoginFailed:             true,
	ProjectDelete:           true,
	ProjectSecretsView:      true,
	ProjectKeyRegenerate:    true,
	ProjectSecretRegenerate: true,
	UserDelete:              true,
	UserPasswordReset:       true,
	SSOUpdate:               true,
	EmailUpdate:             true,
}

// IsSensitive reports whether an action is highlighted as sensitive.
func IsSensitive(action string) bool { return sensitive[action] }

// SensitiveActions lists the sensitive actions, for filtering.
func SensitiveActions() []string {
	out := make([]string, 0, len(sensitive))
	for a := range sensitive {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

type Logger struct {
	store *store.Store
}

func New(s *store.Store) *Logger { return &Logger{store: s} }

type Entry struct {
	UserID     int64 // 0 = anonymous
	Action     string
	TargetType string
	TargetID   string
	Detail     map[string]any
	IP         string
}

// Record writes an audit entry. Failures are logged, never returned: auditing must
// not break the action being audited.
func (l *Logger) Record(ctx context.Context, e Entry) {
	detail := "{}"
	if len(e.Detail) > 0 {
		if b, err := json.Marshal(e.Detail); err == nil {
			detail = string(b)
		}
	}
	row := &store.AuditEntry{
		Action:     e.Action,
		TargetType: e.TargetType,
		TargetID:   e.TargetID,
		Detail:     detail,
		IP:         e.IP,
	}
	if e.UserID != 0 {
		row.UserID = &e.UserID
	}
	if err := l.store.InsertAudit(context.WithoutCancel(ctx), row); err != nil {
		slog.Error("write audit log", "action", e.Action, "err", err)
	}
}
