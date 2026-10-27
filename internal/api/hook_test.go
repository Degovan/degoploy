package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/degovan/degoploy/internal/api"
	"github.com/degovan/degoploy/internal/store"
	"github.com/degovan/degoploy/internal/webhook"
)

func pushBody(ref, msg string, files ...string) string {
	f, _ := json.Marshal(files)
	return fmt.Sprintf(`{"ref":%q,"before":"1111111111111111111111111111111111111111",
		"after":"2222222222222222222222222222222222222222",
		"commits":[{"id":"2222222222222222222222222222222222222222","message":%q,"added":[],"removed":[],"modified":%s,"author":{"name":"Ana"}}],
		"head_commit":{"id":"2222222222222222222222222222222222222222","message":%q,"author":{"name":"Ana"}}}`,
		ref, msg, f, msg)
}

func deliver(t *testing.T, url, provider, event, body, secret string) (int, api.HookResponse) {
	t.Helper()
	hdr := map[string]string{"Content-Type": "application/json"}
	if provider == "gitea" {
		hdr["X-Gitea-Event"] = event
		hdr["X-Gitea-Delivery"] = "gd-1"
		hdr["X-Gitea-Signature"] = strings.TrimPrefix(webhook.Sign([]byte(body), secret), "sha256=")
	} else {
		hdr["X-GitHub-Event"] = event
		hdr["X-GitHub-Delivery"] = "d-1"
		hdr["X-Hub-Signature-256"] = webhook.Sign([]byte(body), secret)
	}
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out api.HookResponse
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestWebhookFlow(t *testing.T) {
	srv, st := newTestServer(t)
	createUser(t, st, "admin", "admin-password-1", store.RoleAdmin)
	createUser(t, st, "view", "viewer-password1", store.RoleViewer)
	admin := login(t, srv.URL, "admin", "admin-password-1")
	view := login(t, srv.URL, "view", "viewer-password1")

	body := strings.Replace(projectJSON, `"enabled":true}`, `"enabled":true,"default_branch":"main"}`, 1)
	var p api.Project
	if code := admin.do("POST", "/api/v1/projects", body, &p); code != 201 {
		t.Fatalf("create: %d", code)
	}
	base := fmt.Sprintf("/api/v1/projects/%d", p.ID)
	var sec api.ProjectSecrets
	admin.do("GET", base+"/secrets", "", &sec)
	hook := srv.URL + sec.WebhookPath

	var rules []api.TriggerRule
	admin.do("GET", base+"/rules", "", &rules)
	if len(rules) != 1 || rules[0].RefPattern != "main" || rules[0].Event != "push" {
		t.Fatalf("default rule = %+v", rules)
	}

	// The project is Gitea; deliveries use Gitea headers.
	if code, res := deliver(t, hook, "gitea", "push", pushBody("refs/heads/main", "fix"), sec.WebhookSecret); code != 202 || res.Status != "queued" || res.DeploymentID == 0 {
		t.Fatalf("matching push: %d %+v", code, res)
	}
	if code, res := deliver(t, hook, "gitea", "push", pushBody("refs/heads/dev", "wip"), sec.WebhookSecret); code != 202 || res.Status != "skipped" || !strings.Contains(res.Reason, `"dev" does not match main`) {
		t.Fatalf("other branch: %d %+v", code, res)
	}
	if code, res := deliver(t, hook, "gitea", "push", pushBody("refs/heads/main", "x [skip deploy]"), sec.WebhookSecret); res.Status != "skipped" {
		t.Fatalf("skip keyword: %d %+v", code, res)
	}
	if code, res := deliver(t, hook, "gitea", "push", pushBody("refs/heads/main", "fix"), "wrong-secret"); code != 401 || res.Status != "rejected" {
		t.Fatalf("bad signature: %d %+v", code, res)
	}
	if code, res := deliver(t, hook, "gitea", "ping", `{}`, sec.WebhookSecret); code != 200 || res.Status != "pong" {
		t.Fatalf("ping: %d %+v", code, res)
	}
	if code, _ := deliver(t, srv.URL+"/hook/doesnotexist", "gitea", "push", `{}`, "x"); code != 404 {
		t.Fatalf("unknown hook: %d", code)
	}

	// Tag rule with env, path filter test via the simulate endpoint.
	var tagRule api.TriggerRule
	code := admin.do("POST", base+"/rules", `{"name":"Release","enabled":true,"event":"tag","ref_pattern":"v*",
		"paths_include":[],"paths_exclude":[],"skip_keyword":"","extra_env":{"APP_ENV":"prod"}}`, &tagRule)
	if code != 201 {
		t.Fatalf("create tag rule: %d", code)
	}
	if code, res := deliver(t, hook, "gitea", "push", pushBody("refs/tags/v1.0.0", "release"), sec.WebhookSecret); res.Status != "queued" || !strings.Contains(res.Reason, "Release") {
		t.Fatalf("tag push: %d %+v", code, res)
	}

	var masked []api.TriggerRule
	view.do("GET", base+"/rules", "", &masked)
	if masked[1].ExtraEnv["APP_ENV"] == "prod" {
		t.Fatal("viewer must not see extra_env values")
	}
	if code := view.do("POST", base+"/rules", `{}`, nil); code != 403 {
		t.Fatalf("viewer create rule: %d", code)
	}

	var sim api.RuleTestResponse
	admin.do("POST", base+"/rules/test", `{"event":"push","ref":"main","message":"m","files":["README.md"]}`, &sim)
	if sim.MatchedRuleID == nil || *sim.MatchedRuleID != rules[0].ID || len(sim.Results) != 2 {
		t.Fatalf("simulate = %+v", sim)
	}

	// Reorder: tag rule first.
	var reordered []api.TriggerRule
	if code := admin.do("PUT", base+"/rules/order", fmt.Sprintf(`{"ids":[%d,%d]}`, tagRule.ID, rules[0].ID), &reordered); code != 200 || reordered[0].ID != tagRule.ID {
		t.Fatalf("reorder: %d %+v", code, reordered)
	}
	if code := admin.do("PUT", base+"/rules/order", fmt.Sprintf(`{"ids":[%d]}`, tagRule.ID), nil); code != 422 {
		t.Fatalf("partial reorder: %d", code)
	}

	// Disabled projects record but ignore pushes.
	admin.do("PATCH", base, `{"enabled":false}`, nil)
	if _, res := deliver(t, hook, "gitea", "push", pushBody("refs/heads/main", "fix"), sec.WebhookSecret); res.Status != "skipped" || res.Reason != "project is disabled" {
		t.Fatalf("disabled: %+v", res)
	}

	var events []api.WebhookEvent
	view.do("GET", base+"/webhook-events", "", &events)
	if len(events) != 7 {
		t.Fatalf("events = %d, want 7", len(events))
	}
	queued := 0
	for _, e := range events {
		if e.Matched != (e.Status == "queued") {
			t.Fatalf("matched %v with status %q", e.Matched, e.Status)
		}
		if e.Matched {
			queued++
			if e.DeploymentID == nil {
				t.Fatalf("matched event without deployment: %+v", e)
			}
		}
	}
	if queued != 2 || events[len(events)-1].Ref != "refs/heads/main" || events[0].Reason != "project is disabled" || events[0].Status != "skipped" {
		t.Fatalf("events = %+v", events)
	}
}
