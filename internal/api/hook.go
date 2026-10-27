package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/degovan/degoploy/internal/deploy"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/realtime"
	"github.com/degovan/degoploy/internal/store"
	"github.com/degovan/degoploy/internal/trigger"
	"github.com/degovan/degoploy/internal/webhook"
)

// maxHookBody bounds webhook payloads. GitHub caps deliveries at 25 MB, but push
// payloads that large are rare; 10 MB keeps memory use predictable.
const maxHookBody = 10 << 20

// hook receives GitHub/Gitea deliveries at /hook/{publicID}. It is unauthenticated:
// the HMAC signature with the project's secret is the authentication.
func (a *API) hook(w http.ResponseWriter, r *http.Request) {
	if !a.hookLimiter.Allow(a.clientIP(r)) {
		httpx.Error(w, http.StatusTooManyRequests, "rate_limited", "too many webhook deliveries")
		return
	}
	p, err := a.store.ProjectByPublicID(r.Context(), r.PathValue("publicID"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found", "unknown webhook")
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxHookBody))
	if err != nil {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "too_large", "payload too large")
		return
	}

	ev := &store.WebhookEvent{
		ProjectID:  p.ID,
		Provider:   p.Provider,
		Event:      firstNonEmpty(r.Header.Get("X-Gitea-Event"), r.Header.Get("X-GitHub-Event"), "unknown"),
		DeliveryID: firstNonEmpty(r.Header.Get("X-Gitea-Delivery"), r.Header.Get("X-GitHub-Delivery")),
	}
	reply := func(status int, res HookResponse) {
		if len(res.Reason) > 1000 {
			res.Reason = res.Reason[:997] + "..."
		}
		ev.Status, ev.Reason = res.Status, res.Reason
		if err := a.store.InsertWebhookEvent(r.Context(), ev); err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		a.hub.Publish(realtime.Message{Topic: realtime.TopicEvents, Type: realtime.TypeWebhookReceived, ProjectID: p.ID})
		httpx.JSON(w, status, res)
	}

	if err := webhook.Verify(p.Provider, r.Header, body, p.WebhookSecret); err != nil {
		reply(http.StatusUnauthorized, HookResponse{Status: "rejected", Reason: err.Error()})
		return
	}
	d, err := webhook.Parse(p.Provider, r.Header, body)
	if err != nil {
		reply(http.StatusBadRequest, HookResponse{Status: "rejected", Reason: "invalid payload: " + err.Error()})
		return
	}
	ev.Event = d.Event

	if d.Event == "ping" {
		reply(http.StatusOK, HookResponse{Status: "pong", Reason: "webhook is configured correctly"})
		return
	}
	if d.Push == nil {
		reply(http.StatusAccepted, HookResponse{Status: "ignored", Reason: fmt.Sprintf("%s events are not used", d.Event)})
		return
	}
	push := d.Push
	ev.Ref, ev.CommitSHA = push.Ref, push.After

	switch {
	case !p.Enabled:
		reply(http.StatusAccepted, HookResponse{Status: "skipped", Reason: "project is disabled"})
		return
	case push.Deleted:
		reply(http.StatusAccepted, HookResponse{Status: "skipped", Reason: "ref was deleted"})
		return
	case push.Trigger.Kind == "":
		reply(http.StatusAccepted, HookResponse{Status: "skipped", Reason: "ref is neither a branch nor a tag"})
		return
	}

	rules, err := a.store.ListRules(r.Context(), p.ID)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	rule, results := trigger.Evaluate(rules, push.Trigger)
	if rule == nil {
		reply(http.StatusAccepted, HookResponse{Status: "skipped", Reason: noMatchReason(results)})
		return
	}

	dep, err := a.deploys.Enqueue(r.Context(), deploy.Request{
		Project:   p,
		Rule:      rule,
		Trigger:   store.TriggerWebhook,
		Ref:       push.Ref,
		CommitSHA: push.After,
		Message:   push.Trigger.Message,
		Author:    push.Author,
	})
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	ev.Matched, ev.DeploymentID = true, &dep.ID
	reply(http.StatusAccepted, HookResponse{
		Status:       "queued",
		Reason:       fmt.Sprintf("matched rule %q", rule.Name),
		DeploymentID: dep.ID,
	})
}

func noMatchReason(results []trigger.Result) string {
	if len(results) == 0 {
		return "no trigger rules configured"
	}
	parts := make([]string, 0, len(results))
	for _, r := range results {
		parts = append(parts, fmt.Sprintf("%s: %s", r.Name, r.Reason))
	}
	return "no rule matched (" + strings.Join(parts, "; ") + ")"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
