package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/degovan/degoploy/internal/audit"
	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/store"
	"github.com/degovan/degoploy/internal/trigger"
)

func (a *API) registerRules(mux *http.ServeMux) {
	h := func(fn http.HandlerFunc) http.Handler { return auth.RequireAuth(fn) }

	mux.Handle("GET /api/v1/projects/{id}/rules", h(a.listRules))
	mux.Handle("POST /api/v1/projects/{id}/rules", h(a.createRule))
	mux.Handle("PUT /api/v1/projects/{id}/rules/order", h(a.reorderRules))
	mux.Handle("POST /api/v1/projects/{id}/rules/test", h(a.testRules))
	mux.Handle("PUT /api/v1/projects/{id}/rules/{ruleID}", h(a.updateRule))
	mux.Handle("DELETE /api/v1/projects/{id}/rules/{ruleID}", h(a.deleteRule))
	mux.Handle("GET /api/v1/projects/{id}/webhook-events", h(a.listWebhookEvents))
}

func (a *API) listRules(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectView)
	if !ok {
		return
	}
	rules, err := a.store.ListRules(r.Context(), p.ID)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	reveal := a.authz.Can(r.Context(), ident.User, auth.ActionProjectEdit, p.ID)
	out := make([]TriggerRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, toRule(rule, reveal))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (a *API) createRule(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectEdit)
	if !ok {
		return
	}
	var in RuleInput
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	rule := fromRuleInput(in)
	rule.ProjectID = p.ID
	if !validateRule(w, rule) {
		return
	}
	if err := a.store.CreateRule(r.Context(), rule); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	a.recordProject(r, ident.User.ID, audit.RuleCreate, p, map[string]any{"rule": rule.Name})
	httpx.JSON(w, http.StatusCreated, toRule(rule, true))
}

func (a *API) updateRule(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectEdit)
	if !ok {
		return
	}
	existing, ok := a.loadRule(w, r, p)
	if !ok {
		return
	}
	var in RuleInput
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	rule := fromRuleInput(in)
	rule.ID, rule.ProjectID, rule.Position = existing.ID, p.ID, existing.Position
	if !validateRule(w, rule) {
		return
	}
	if err := a.store.UpdateRule(r.Context(), rule); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	a.recordProject(r, ident.User.ID, audit.RuleUpdate, p, map[string]any{"rule": rule.Name})
	httpx.JSON(w, http.StatusOK, toRule(rule, true))
}

func (a *API) deleteRule(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectEdit)
	if !ok {
		return
	}
	rule, ok := a.loadRule(w, r, p)
	if !ok {
		return
	}
	if err := a.store.DeleteRule(r.Context(), p.ID, rule.ID); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	a.recordProject(r, ident.User.ID, audit.RuleDelete, p, map[string]any{"rule": rule.Name})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) reorderRules(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectEdit)
	if !ok {
		return
	}
	var req RuleOrderRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	err := a.store.ReorderRules(r.Context(), p.ID, req.IDs)
	if errors.Is(err, store.ErrNotFound) {
		httpx.JSON(w, http.StatusUnprocessableEntity, httpx.ErrorBody{
			Error: "ids must list every rule of the project exactly once", Code: "validation", Field: "ids"})
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	a.recordProject(r, ident.User.ID, audit.RuleReorder, p, nil)
	a.listRules(w, r)
}

func (a *API) testRules(w http.ResponseWriter, r *http.Request) {
	p, _, ok := a.loadProject(w, r, auth.ActionProjectView)
	if !ok {
		return
	}
	var req RuleTestRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Event != trigger.KindPush && req.Event != trigger.KindTag {
		httpx.JSON(w, http.StatusUnprocessableEntity, httpx.ErrorBody{Error: "must be push or tag", Code: "validation", Field: "event"})
		return
	}
	rules, err := a.store.ListRules(r.Context(), p.ID)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	files := []string{}
	for _, f := range req.Files {
		if f = strings.TrimSpace(f); f != "" {
			files = append(files, f)
		}
	}
	ref := strings.TrimSpace(req.Ref)
	ref = strings.TrimPrefix(strings.TrimPrefix(ref, "refs/heads/"), "refs/tags/")
	winner, results := trigger.Evaluate(rules, trigger.Push{
		Kind: req.Event, RefName: ref, Message: req.Message, Files: files, FilesKnown: len(files) > 0,
	})
	out := RuleTestResponse{Results: make([]RuleTestResult, 0, len(results))}
	if winner != nil {
		out.MatchedRuleID = &winner.ID
	}
	for _, res := range results {
		out.Results = append(out.Results, RuleTestResult(res))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (a *API) listWebhookEvents(w http.ResponseWriter, r *http.Request) {
	p, _, ok := a.loadProject(w, r, auth.ActionProjectView)
	if !ok {
		return
	}
	events, err := a.store.ListWebhookEvents(r.Context(), p.ID, 50)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	out := make([]WebhookEvent, 0, len(events))
	for _, e := range events {
		out = append(out, WebhookEvent{
			ID: e.ID, Provider: e.Provider, Event: e.Event, DeliveryID: e.DeliveryID, Ref: e.Ref,
			CommitSHA: e.CommitSHA, Matched: e.Matched, Reason: e.Reason, DeploymentID: e.DeploymentID,
			ReceivedAt: e.ReceivedAt.UTC().Format(time.RFC3339),
		})
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (a *API) loadRule(w http.ResponseWriter, r *http.Request, p *store.Project) (*store.TriggerRule, bool) {
	id, err := strconv.ParseInt(r.PathValue("ruleID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "rule not found")
		return nil, false
	}
	rule, err := a.store.RuleByID(r.Context(), p.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found", "rule not found")
		return nil, false
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return nil, false
	}
	return rule, true
}

func validateRule(w http.ResponseWriter, rule *store.TriggerRule) bool {
	if field, msg := trigger.ValidateRule(rule); field != "" {
		httpx.JSON(w, http.StatusUnprocessableEntity, httpx.ErrorBody{Error: msg, Code: "validation", Field: field})
		return false
	}
	return true
}

func fromRuleInput(in RuleInput) *store.TriggerRule {
	return &store.TriggerRule{
		Name: in.Name, Enabled: in.Enabled, Event: in.Event, RefPattern: in.RefPattern,
		PathsInclude: in.PathsInclude, PathsExclude: in.PathsExclude, SkipKeyword: in.SkipKeyword,
		ExtraEnv: in.ExtraEnv,
	}
}

func toRule(r *store.TriggerRule, revealEnv bool) TriggerRule {
	env := make(map[string]string, len(r.ExtraEnv))
	for k, v := range r.ExtraEnv {
		if revealEnv {
			env[k] = v
		} else {
			env[k] = "••••••"
		}
	}
	return TriggerRule{
		ID: r.ID, Name: r.Name, Enabled: r.Enabled, Position: r.Position, Event: r.Event,
		RefPattern: r.RefPattern, PathsInclude: nonNil(r.PathsInclude), PathsExclude: nonNil(r.PathsExclude),
		SkipKeyword: r.SkipKeyword, ExtraEnv: env,
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
