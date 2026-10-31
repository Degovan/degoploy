package auth

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/store"
)

type Action string

const (
	ActionProjectView    Action = "project.view"
	ActionProjectDeploy  Action = "project.deploy"
	ActionProjectEdit    Action = "project.edit"
	ActionProjectSecrets Action = "project.secrets"
	ActionProjectCreate  Action = "project.create"
	ActionUserManage     Action = "user.manage"
	ActionAuditView      Action = "audit.view"
)

// AllActions is the canonical order used when listing permissions.
var AllActions = []Action{
	ActionProjectView, ActionProjectDeploy, ActionProjectEdit, ActionProjectSecrets,
	ActionProjectCreate, ActionUserManage, ActionAuditView,
}

var rolePerms = map[store.Role]map[Action]bool{
	store.RoleAdmin: {
		ActionProjectView: true, ActionProjectDeploy: true, ActionProjectEdit: true,
		ActionProjectSecrets: true, ActionProjectCreate: true, ActionUserManage: true, ActionAuditView: true,
	},
	store.RoleDeployer: {
		ActionProjectView: true, ActionProjectDeploy: true,
	},
	store.RoleViewer: {
		ActionProjectView: true,
	},
}

// RolePermissions lists what a role may do, in AllActions order.
func RolePermissions(role store.Role) []Action {
	out := []Action{}
	for _, act := range AllActions {
		if rolePerms[role][act] {
			out = append(out, act)
		}
	}
	return out
}

// globalOnly actions are never affected by per-project membership.
var globalOnly = map[Action]bool{
	ActionProjectCreate: true, ActionUserManage: true, ActionAuditView: true,
}

// ProjectRoleLookup returns a user's role on one project, if they are a member.
type ProjectRoleLookup func(ctx context.Context, projectID, userID int64) (store.Role, bool, error)

// Authorizer answers "may this user do X (on project P)?".
//
// Admins, and users without SelectedProjects, act with their global role on
// every project. A user with SelectedProjects acts on a project only with the
// role their membership in project_members gives them, through ProjectRoles.
type Authorizer struct {
	ProjectRoles ProjectRoleLookup
}

// Can checks action for u. projectID 0 means the action is not project-scoped.
func (a *Authorizer) Can(ctx context.Context, u *store.User, action Action, projectID int64) bool {
	if u == nil || u.Disabled {
		return false
	}
	role := u.Role
	if projectID != 0 && !globalOnly[action] && Selected(u) {
		if a.ProjectRoles == nil {
			return false
		}
		r, ok, err := a.ProjectRoles(ctx, projectID, u.ID)
		if err != nil {
			slog.Error("project role lookup", "project", projectID, "user", u.ID, "err", err)
			return false
		}
		if !ok {
			return false
		}
		role = r
	}
	return rolePerms[role][action]
}

// Selected reports whether u sees only the projects they are a member of.
func Selected(u *store.User) bool {
	return u.SelectedProjects && u.Role != store.RoleAdmin
}

// ProjectPermissions lists the project-scoped actions u may perform on a project.
func (a *Authorizer) ProjectPermissions(ctx context.Context, u *store.User, projectID int64) []Action {
	out := []Action{}
	for _, act := range AllActions {
		if !globalOnly[act] && a.Can(ctx, u, act, projectID) {
			out = append(out, act)
		}
	}
	return out
}

// Permissions lists the global actions u may perform.
func (a *Authorizer) Permissions(ctx context.Context, u *store.User) []Action {
	out := []Action{}
	for _, act := range AllActions {
		if a.Can(ctx, u, act, 0) {
			out = append(out, act)
		}
	}
	return out
}

// Require wraps a handler with a global (non project-scoped) permission check.
// It must run after RequireAuth.
func (a *Authorizer) Require(action Action, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := FromContext(r.Context())
		if id == nil || !a.Can(r.Context(), id.User, action, 0) {
			httpx.Error(w, http.StatusForbidden, "forbidden", "you do not have permission for this action")
			return
		}
		next.ServeHTTP(w, r)
	})
}
