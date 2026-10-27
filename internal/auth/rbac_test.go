package auth

import (
	"context"
	"testing"

	"github.com/degovan/degoploy/internal/store"
)

func TestCanGlobalRoles(t *testing.T) {
	ctx := context.Background()
	a := &Authorizer{}
	cases := []struct {
		role   store.Role
		action Action
		want   bool
	}{
		{store.RoleViewer, ActionProjectView, true},
		{store.RoleViewer, ActionProjectDeploy, false},
		{store.RoleDeployer, ActionProjectDeploy, true},
		{store.RoleDeployer, ActionProjectEdit, false},
		{store.RoleDeployer, ActionProjectSecrets, false},
		{store.RoleAdmin, ActionUserManage, true},
	}
	for _, c := range cases {
		u := &store.User{ID: 1, Role: c.role}
		if got := a.Can(ctx, u, c.action, 7); got != c.want {
			t.Errorf("%s %s = %v, want %v", c.role, c.action, got, c.want)
		}
	}
	if a.Can(ctx, &store.User{Role: store.RoleAdmin, Disabled: true}, ActionProjectView, 0) {
		t.Error("disabled user must not be allowed")
	}
}

func TestCanProjectOverride(t *testing.T) {
	ctx := context.Background()
	a := &Authorizer{ProjectRoles: func(_ context.Context, projectID, _ int64) (store.Role, bool, error) {
		if projectID == 7 {
			return store.RoleDeployer, true, nil
		}
		return "", false, nil
	}}
	u := &store.User{ID: 1, Role: store.RoleViewer}
	if !a.Can(ctx, u, ActionProjectDeploy, 7) {
		t.Error("project override should grant deploy on project 7")
	}
	if a.Can(ctx, u, ActionProjectDeploy, 8) {
		t.Error("no override on project 8; viewer must not deploy")
	}
	admin := &store.User{ID: 2, Role: store.RoleViewer}
	a.ProjectRoles = func(context.Context, int64, int64) (store.Role, bool, error) { return store.RoleAdmin, true, nil }
	if a.Can(ctx, admin, ActionUserManage, 7) {
		t.Error("project membership must not grant global-only actions")
	}
}
