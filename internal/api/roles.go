package api

import (
	"net/http"
	"sort"

	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/store"
)

// listRoles returns the built-in roles with their permissions, taken from the
// RBAC table so the Roles page can never drift from what the API enforces.
func (a *API) listRoles(w http.ResponseWriter, r *http.Request) {
	users, err := a.store.ListUsers(r.Context())
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	// Active members first, each group in username order (as ListUsers returns).
	sort.SliceStable(users, func(i, j int) bool { return !users[i].Disabled && users[j].Disabled })

	out := []RoleSummary{}
	for _, role := range []store.Role{store.RoleAdmin, store.RoleDeployer, store.RoleViewer} {
		s := RoleSummary{Role: string(role), Permissions: []string{}, Members: []RoleMember{}}
		for _, act := range auth.RolePermissions(role) {
			s.Permissions = append(s.Permissions, string(act))
		}
		for _, u := range users {
			if u.Role == role {
				s.Members = append(s.Members, RoleMember{
					ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Disabled: u.Disabled,
				})
			}
		}
		out = append(out, s)
	}
	httpx.JSON(w, http.StatusOK, out)
}
