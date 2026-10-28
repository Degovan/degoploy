package api_test

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/degovan/degoploy/internal/api"
	"github.com/degovan/degoploy/internal/store"
)

func TestAuditFilters(t *testing.T) {
	srv, st := newTestServer(t)
	createUser(t, st, "root", "root-password-1", store.RoleAdmin)
	admin := login(t, srv.URL, "root", "root-password-1")

	var p api.Project
	if code := admin.do("POST", "/api/v1/projects", projectJSON, &p); code != 201 {
		t.Fatalf("create project: %d", code)
	}
	admin.do("GET", fmt.Sprintf("/api/v1/projects/%d/secrets", p.ID), "", nil)
	admin.do("POST", fmt.Sprintf("/api/v1/projects/%d/regenerate-secret", p.ID), "", nil)
	// Entries so far: login, project.create, secrets_view, secret_regenerate.

	var page api.AuditPage
	if code := admin.do("GET", "/api/v1/audit?limit=2", "", &page); code != 200 {
		t.Fatalf("list: %d", code)
	}
	if page.Total != 4 || len(page.Entries) != 2 || page.NextBefore == nil {
		t.Fatalf("page = total %d, %d entries, next %v", page.Total, len(page.Entries), page.NextBefore)
	}
	if !page.Entries[0].Sensitive || page.Entries[0].Action != "project.secret_regenerate" {
		t.Fatalf("newest = %+v", page.Entries[0])
	}
	today := time.Now().UTC().Format(time.DateOnly)
	if page.DayCounts[today] != 4 {
		t.Fatalf("day counts = %v, want 4 on %s", page.DayCounts, today)
	}

	if admin.do("GET", "/api/v1/audit?sensitive=1", "", &page); page.Total != 2 {
		t.Fatalf("sensitive total = %d", page.Total)
	}
	// The search matches the slug stored in the detail, the action and the IP.
	if admin.do("GET", "/api/v1/audit?q=shop&action=project.", "", &page); page.Total != 3 {
		t.Fatalf("q=shop total = %d", page.Total)
	}
	if admin.do("GET", "/api/v1/audit?q=127.0.0.1", "", &page); page.Total != 4 {
		t.Fatalf("q=ip total = %d", page.Total)
	}
	if admin.do("GET", "/api/v1/audit?q=nothing-matches", "", &page); page.Total != 0 || len(page.Entries) != 0 {
		t.Fatalf("q=none = %+v", page)
	}

	var sum api.AuditSummary
	since := url.QueryEscape(time.Now().Add(-time.Hour).Format(time.RFC3339))
	if code := admin.do("GET", "/api/v1/audit/summary?since="+since, "", &sum); code != 200 || sum.Events != 4 || sum.Sensitive != 2 {
		t.Fatalf("summary: %d %+v", code, sum)
	}
	future := url.QueryEscape(time.Now().Add(time.Hour).Format(time.RFC3339))
	if admin.do("GET", "/api/v1/audit/summary?since="+future, "", &sum); sum.Events != 0 {
		t.Fatalf("future summary = %+v", sum)
	}
	if code := admin.do("GET", "/api/v1/audit/summary?since=yesterday", "", nil); code != 422 {
		t.Fatalf("bad since: %d", code)
	}
}
