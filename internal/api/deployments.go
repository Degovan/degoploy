package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/degovan/degoploy/internal/audit"
	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/deploy"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/realtime"
	"github.com/degovan/degoploy/internal/release"
	"github.com/degovan/degoploy/internal/store"
	"github.com/degovan/degoploy/internal/trigger"
)

func (a *API) registerDeployments(mux *http.ServeMux) {
	h := func(fn http.HandlerFunc) http.Handler { return auth.RequireAuth(fn) }

	mux.Handle("GET /api/v1/deployments", h(a.recentDeployments))
	mux.Handle("GET /api/v1/projects/{id}/deployments", h(a.listDeployments))
	mux.Handle("POST /api/v1/projects/{id}/deployments", h(a.manualDeploy))
	mux.Handle("GET /api/v1/deployments/{did}", h(a.getDeployment))
	mux.Handle("GET /api/v1/deployments/{did}/log", h(a.deploymentLog))
	mux.Handle("GET /api/v1/deployments/{did}/log/times", h(a.deploymentLogTimes))
	mux.Handle("GET /api/v1/deployments/{did}/neighbors", h(a.deploymentNeighbors))
	mux.Handle("POST /api/v1/deployments/{did}/cancel", h(a.cancelDeployment))
	mux.Handle("POST /api/v1/deployments/{did}/redeploy", h(a.redeploy))
	mux.Handle("GET /api/v1/projects/{id}/releases", h(a.listReleases))
	mux.Handle("POST /api/v1/projects/{id}/releases/{rid}/activate", h(a.activateRelease))
}

// publishDeployment pushes a deployment change to realtime subscribers.
func (a *API) publishDeployment(v *store.DeploymentView) {
	a.hub.Publish(realtime.Message{
		Topic: realtime.TopicEvents, Type: realtime.TypeDeploymentUpdated, ProjectID: v.ProjectID, Data: toDeployment(v),
	})
}

func (a *API) listDeployments(w http.ResponseWriter, r *http.Request) {
	p, _, ok := a.loadProject(w, r, auth.ActionProjectView)
	if !ok {
		return
	}
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	deps, err := a.store.ListDeployments(r.Context(), p.ID, before, limit)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	out := make([]Deployment, 0, len(deps))
	for _, d := range deps {
		out = append(out, toDeployment(d))
	}
	httpx.JSON(w, http.StatusOK, out)
}

// recentDeployments lists the newest deployments of all projects the user can view.
func (a *API) recentDeployments(w http.ResponseWriter, r *http.Request) {
	ident := auth.FromContext(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	// ?status=active lists only queued and running deployments.
	deps, err := a.store.RecentDeployments(r.Context(), limit, r.URL.Query().Get("status") == "active")
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	out := make([]Deployment, 0, len(deps))
	for _, d := range deps {
		if a.authz.Can(r.Context(), ident.User, auth.ActionProjectView, d.ProjectID) {
			out = append(out, toDeployment(d))
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

var refNameRe = regexp.MustCompile(`^refs/(heads|tags)/[^\s~^:?*\[\\]+$`)

func (a *API) manualDeploy(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectDeploy)
	if !ok {
		return
	}
	var req DeployRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	ref := strings.TrimSpace(req.Ref)
	if ref != "" && !strings.HasPrefix(ref, "refs/") {
		ref = "refs/heads/" + ref
	}
	if !refNameRe.MatchString(ref) || strings.Contains(ref, "..") || strings.HasSuffix(ref, "/") || strings.HasSuffix(ref, ".lock") {
		httpx.JSON(w, http.StatusUnprocessableEntity, httpx.ErrorBody{
			Error: "use a branch name or refs/heads/…, refs/tags/…", Code: "validation", Field: "ref"})
		return
	}

	// Apply the environment of the rule that would match this ref, if any.
	rules, err := a.store.ListRules(r.Context(), p.ID)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	kind, name := trigger.KindPush, strings.TrimPrefix(ref, "refs/heads/")
	if strings.HasPrefix(ref, "refs/tags/") {
		kind, name = trigger.KindTag, strings.TrimPrefix(ref, "refs/tags/")
	}
	rule, _ := trigger.Evaluate(rules, trigger.Push{Kind: kind, RefName: name})

	d, err := a.deploys.Enqueue(r.Context(), deploy.Request{
		Project: p, Rule: rule, Trigger: store.TriggerManual, TriggeredBy: ident.User.ID, Ref: ref,
	})
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	a.recordProject(r, ident.User.ID, audit.DeploymentManual, p, map[string]any{"deployment": d.ID, "ref": ref})
	a.writeDeployment(w, r, d.ID, http.StatusCreated)
}

// loadDeployment resolves {did} and checks action on its project.
func (a *API) loadDeployment(w http.ResponseWriter, r *http.Request, action auth.Action) (*store.DeploymentView, *auth.Identity, bool) {
	id, err := strconv.ParseInt(r.PathValue("did"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "deployment not found")
		return nil, nil, false
	}
	d, err := a.store.DeploymentViewByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found", "deployment not found")
		return nil, nil, false
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return nil, nil, false
	}
	ident := auth.FromContext(r.Context())
	if !a.authz.Can(r.Context(), ident.User, auth.ActionProjectView, d.ProjectID) {
		httpx.Error(w, http.StatusNotFound, "not_found", "deployment not found")
		return nil, nil, false
	}
	if !a.authz.Can(r.Context(), ident.User, action, d.ProjectID) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "you do not have permission for this action")
		return nil, nil, false
	}
	return d, ident, true
}

func (a *API) getDeployment(w http.ResponseWriter, r *http.Request) {
	d, _, ok := a.loadDeployment(w, r, auth.ActionProjectView)
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, toDeployment(d))
}

func (a *API) deploymentLog(w http.ResponseWriter, r *http.Request) {
	d, _, ok := a.loadDeployment(w, r, auth.ActionProjectView)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.URL.Query().Has("download") {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="deployment-%d.log"`, d.ID))
	}
	if d.LogPath == "" {
		return
	}
	f, err := os.Open(d.LogPath)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	defer f.Close()
	http.ServeContent(w, r, "", time.Time{}, f)
}

func (a *API) deploymentLogTimes(w http.ResponseWriter, r *http.Request) {
	d, _, ok := a.loadDeployment(w, r, auth.ActionProjectView)
	if !ok {
		return
	}
	out := LogTimes{Entries: [][2]int64{}}
	if d.LogPath != "" {
		entries, err := deploy.ReadLogTimes(d.LogPath)
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		if entries != nil {
			out.Entries = entries
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (a *API) deploymentNeighbors(w http.ResponseWriter, r *http.Request) {
	d, _, ok := a.loadDeployment(w, r, auth.ActionProjectView)
	if !ok {
		return
	}
	prev, next, err := a.store.DeploymentNeighbors(r.Context(), d.ProjectID, d.ID)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, DeploymentNeighbors{Prev: prev, Next: next})
}

func (a *API) cancelDeployment(w http.ResponseWriter, r *http.Request) {
	d, ident, ok := a.loadDeployment(w, r, auth.ActionProjectDeploy)
	if !ok {
		return
	}
	canceled, err := a.deploys.Cancel(r.Context(), d.ID)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if !canceled {
		httpx.Error(w, http.StatusConflict, "finished", "deployment has already finished")
		return
	}
	a.audit.Record(r.Context(), audit.Entry{
		UserID: ident.User.ID, Action: audit.DeploymentCancel, TargetType: "deployment",
		TargetID: strconv.FormatInt(d.ID, 10), IP: a.clientIP(r),
	})
	a.writeDeployment(w, r, d.ID, http.StatusOK)
}

func (a *API) redeploy(w http.ResponseWriter, r *http.Request) {
	d, ident, ok := a.loadDeployment(w, r, auth.ActionProjectDeploy)
	if !ok {
		return
	}
	p, err := a.store.ProjectByID(r.Context(), d.ProjectID)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	// Running a rollback again switches to the same release again.
	if d.Trigger == store.TriggerRollback && d.ReleaseID != nil {
		a.rollback(w, r, ident, p, *d.ReleaseID)
		return
	}
	if d.Ref == "" {
		httpx.Error(w, http.StatusConflict, "no_ref", "this deployment has no ref to redeploy")
		return
	}
	req := deploy.Request{
		Project: p, Trigger: store.TriggerRedeploy, TriggeredBy: ident.User.ID,
		Ref: d.Ref, CommitSHA: d.CommitSHA, Message: d.CommitMessage, Author: d.Author,
	}
	if d.RuleID != nil {
		if rule, err := a.store.RuleByID(r.Context(), p.ID, *d.RuleID); err == nil {
			req.Rule = rule
		}
	}
	nd, err := a.deploys.Enqueue(r.Context(), req)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	a.recordProject(r, ident.User.ID, audit.DeploymentRedeploy, p, map[string]any{"deployment": nd.ID, "from": d.ID})
	a.writeDeployment(w, r, nd.ID, http.StatusCreated)
}

func (a *API) writeDeployment(w http.ResponseWriter, r *http.Request, id int64, status int) {
	v, err := a.store.DeploymentViewByID(r.Context(), id)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, status, toDeployment(v))
}

func toDeployment(v *store.DeploymentView) Deployment {
	return Deployment{
		ID: v.ID, ProjectID: v.ProjectID, RuleID: v.RuleID, RuleName: v.RuleName, Trigger: v.Trigger,
		TriggeredBy: v.TriggeredByName, Ref: v.Ref, CommitSHA: v.CommitSHA, CommitMessage: v.CommitMessage,
		Author: v.Author, Status: v.Status, ExitCode: v.ExitCode, Error: v.Error, AutoPull: v.AutoPull,
		ScriptRevisionID: v.ScriptRevisionID, ReleaseID: v.ReleaseID,
		QueuedAt: msTime(v.QueuedAt), StartedAt: msTimePtr(v.StartedAt),
		ScriptStartedAt: msTimePtr(v.ScriptStartedAt), FinishedAt: msTimePtr(v.FinishedAt),
	}
}

// msTime formats with milliseconds: stage durations are often well under a second.
func msTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z07:00") }

func msTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := msTime(*t)
	return &s
}

// listReleases lists the releases on the server, newest first.
func (a *API) listReleases(w http.ResponseWriter, r *http.Request) {
	p, _, ok := a.loadProject(w, r, auth.ActionProjectView)
	if !ok {
		return
	}
	out := []Release{}
	if p.Releases {
		live := release.Live(p.WorkDir)
		for _, id := range release.List(p.WorkDir) {
			rel := Release{ID: id, Live: id == live}
			if v, err := a.store.DeploymentViewByID(r.Context(), id); err == nil && v.ProjectID == p.ID {
				d := toDeployment(v)
				rel.Deployment = &d
			}
			out = append(out, rel)
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

// activateRelease queues a deployment that makes a release current again.
func (a *API) activateRelease(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectDeploy)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("rid"), 10, 64)
	if err != nil || !p.Releases || !release.Exists(p.WorkDir, id) {
		httpx.Error(w, http.StatusNotFound, "not_found", "release not found")
		return
	}
	a.rollback(w, r, ident, p, id)
}

func (a *API) rollback(w http.ResponseWriter, r *http.Request, ident *auth.Identity, p *store.Project, id int64) {
	if !p.Releases {
		httpx.Error(w, http.StatusConflict, "no_releases", "this project deploys in place; turn releases on to switch between them")
		return
	}
	if !release.Exists(p.WorkDir, id) {
		httpx.Error(w, http.StatusConflict, "release_gone", fmt.Sprintf("release %d is no longer on the server", id))
		return
	}
	req := deploy.Request{Project: p, Trigger: store.TriggerRollback, TriggeredBy: ident.User.ID, ReleaseID: id}
	// Show what the release runs: the commit its own deployment built.
	if v, err := a.store.DeploymentViewByID(r.Context(), id); err == nil && v.ProjectID == p.ID {
		req.Ref, req.CommitSHA, req.Message, req.Author = v.Ref, v.CommitSHA, v.CommitMessage, v.Author
	}
	d, err := a.deploys.Enqueue(r.Context(), req)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	a.recordProject(r, ident.User.ID, audit.DeploymentRollback, p, map[string]any{"deployment": d.ID, "release": id})
	a.writeDeployment(w, r, d.ID, http.StatusCreated)
}
