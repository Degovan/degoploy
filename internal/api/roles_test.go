package api_test

import (
	"slices"
	"testing"

	"github.com/degovan/degoploy/internal/api"
	"github.com/degovan/degoploy/internal/store"
)

func TestRoles(t *testing.T) {
	srv, st := newTestServer(t)
	createUser(t, st, "root", "root-password-1", store.RoleAdmin)
	createUser(t, st, "dep", "deployer-password", store.RoleDeployer)
	createUser(t, st, "view", "viewer-password1", store.RoleViewer)
	admin := login(t, srv.URL, "root", "root-password-1")
	dep := login(t, srv.URL, "dep", "deployer-password")
	admin.do("PATCH", "/api/v1/users/3", `{"disabled":true}`, nil)

	if code := dep.do("GET", "/api/v1/roles", "", nil); code != 403 {
		t.Fatalf("deployer roles: %d", code)
	}
	var roles []api.RoleSummary
	if code := admin.do("GET", "/api/v1/roles", "", &roles); code != 200 || len(roles) != 3 {
		t.Fatalf("roles: %d %+v", code, roles)
	}
	admins, deployers, viewers := roles[0], roles[1], roles[2]
	if admins.Role != "admin" || len(admins.Permissions) != 7 || admins.Members[0].Username != "root" {
		t.Fatalf("admin = %+v", admins)
	}
	if !slices.Equal(deployers.Permissions, []string{"project.view", "project.deploy"}) || len(deployers.Members) != 1 {
		t.Fatalf("deployer = %+v", deployers)
	}
	if !slices.Equal(viewers.Permissions, []string{"project.view"}) || !viewers.Members[0].Disabled {
		t.Fatalf("viewer = %+v", viewers)
	}
}
