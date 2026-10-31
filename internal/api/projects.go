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
	"github.com/degovan/degoploy/internal/project"
	"github.com/degovan/degoploy/internal/release"
	"github.com/degovan/degoploy/internal/store"
)

func (a *API) registerProjects(mux *http.ServeMux) {
	h := func(fn http.HandlerFunc) http.Handler { return auth.RequireAuth(fn) }

	mux.Handle("GET /api/v1/projects", h(a.listProjects))
	mux.Handle("POST /api/v1/projects", h(a.createProject))
	mux.Handle("GET /api/v1/projects/{id}", h(a.getProject))
	mux.Handle("PATCH /api/v1/projects/{id}", h(a.updateProject))
	mux.Handle("DELETE /api/v1/projects/{id}", h(a.deleteProject))
	mux.Handle("GET /api/v1/projects/{id}/secrets", h(a.projectSecrets))
	mux.Handle("POST /api/v1/projects/{id}/regenerate-key", h(a.regenerateKey))
	mux.Handle("POST /api/v1/projects/{id}/regenerate-secret", h(a.regenerateSecret))
	mux.Handle("POST /api/v1/projects/{id}/test-connection", h(a.testConnection))
	mux.Handle("GET /api/v1/projects/{id}/script", h(a.getScript))
	mux.Handle("PUT /api/v1/projects/{id}/script", h(a.saveScript))
	mux.Handle("GET /api/v1/projects/{id}/script/revisions", h(a.listScriptRevisions))
}

// loadProject resolves {id} and checks action on it. Users who cannot even view
// the project get 404 so project existence is not leaked.
func (a *API) loadProject(w http.ResponseWriter, r *http.Request, action auth.Action) (*store.Project, *auth.Identity, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "project not found")
		return nil, nil, false
	}
	p, err := a.store.ProjectByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found", "project not found")
		return nil, nil, false
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return nil, nil, false
	}
	ident := auth.FromContext(r.Context())
	if !a.authz.Can(r.Context(), ident.User, auth.ActionProjectView, p.ID) {
		httpx.Error(w, http.StatusNotFound, "not_found", "project not found")
		return nil, nil, false
	}
	if !a.authz.Can(r.Context(), ident.User, action, p.ID) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "you do not have permission for this action")
		return nil, nil, false
	}
	return p, ident, true
}

func (a *API) projectError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *project.ValidationError
	if errors.As(err, &ve) {
		httpx.JSON(w, http.StatusUnprocessableEntity, httpx.ErrorBody{Error: ve.Message, Code: "validation", Field: ve.Field})
		return
	}
	httpx.InternalError(w, r, err)
}

func (a *API) recordProject(r *http.Request, userID int64, action string, p *store.Project, detail map[string]any) {
	a.audit.Record(r.Context(), audit.Entry{
		UserID: userID, Action: action, TargetType: "project", TargetID: strconv.FormatInt(p.ID, 10),
		Detail: mergeDetail(map[string]any{"slug": p.Slug}, detail), IP: a.clientIP(r),
	})
}

func (a *API) listProjects(w http.ResponseWriter, r *http.Request) {
	ident := auth.FromContext(r.Context())
	projects, err := a.store.ListProjects(r.Context())
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	latest, err := a.store.LatestDeployments(r.Context())
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	health, err := a.store.DeploymentHealth(r.Context(), 0, healthWindow)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	runtimes, err := a.store.AllProjectRuntimes(r.Context())
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	out := []Project{}
	for _, p := range projects {
		if a.authz.Can(r.Context(), ident.User, auth.ActionProjectView, p.ID) {
			dto := toProject(p)
			dto.Runtimes = a.projectRuntimes(runtimes[p.ID])
			if d, ok := latest[p.ID]; ok {
				dd := toDeployment(d)
				dto.LastDeployment = &dd
			}
			dto.Health = toHealth(health, p.ID)
			dto.Permissions = a.projectPermissions(r, p.ID)
			out = append(out, dto)
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (a *API) createProject(w http.ResponseWriter, r *http.Request) {
	ident := auth.FromContext(r.Context())
	if !a.authz.Can(r.Context(), ident.User, auth.ActionProjectCreate, 0) {
		httpx.Error(w, http.StatusForbidden, "forbidden", "you do not have permission for this action")
		return
	}
	var req CreateProjectRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	p, err := a.projects.Create(r.Context(), project.Input{
		Name: req.Name, Slug: req.Slug, Provider: req.Provider, RepoURL: req.RepoURL, WorkDir: req.WorkDir,
		ScriptSource: req.ScriptSource, ScriptPath: req.ScriptPath, AutoPull: req.AutoPull,
		TimeoutSec: req.TimeoutSec, Enabled: req.Enabled, Releases: req.Releases, DefaultBranch: req.DefaultBranch,
	}, ident.User.ID)
	if err != nil {
		a.projectError(w, r, err)
		return
	}
	a.recordProject(r, ident.User.ID, audit.ProjectCreate, p, nil)
	dto, err := a.projectDTO(r, p)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, dto)
}

func (a *API) getProject(w http.ResponseWriter, r *http.Request) {
	p, _, ok := a.loadProject(w, r, auth.ActionProjectView)
	if !ok {
		return
	}
	dto, err := a.projectDTO(r, p)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	health, err := a.store.DeploymentHealth(r.Context(), p.ID, healthWindow)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	dto.Health = toHealth(health, p.ID)
	httpx.JSON(w, http.StatusOK, dto)
}

// healthWindow is how many recent finished deployments a project's health covers.
const healthWindow = 10

func toHealth(m map[int64]store.Health, projectID int64) *ProjectHealth {
	h, ok := m[projectID]
	if !ok || h.Total == 0 {
		return nil
	}
	return &ProjectHealth{Passed: h.Passed, Total: h.Total}
}

func (a *API) updateProject(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectEdit)
	if !ok {
		return
	}
	var req UpdateProjectRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	in := project.InputFrom(p)
	setIf(&in.Name, req.Name)
	setIf(&in.Provider, req.Provider)
	setIf(&in.RepoURL, req.RepoURL)
	setIf(&in.WorkDir, req.WorkDir)
	setIf(&in.ScriptSource, req.ScriptSource)
	setIf(&in.ScriptPath, req.ScriptPath)
	setIf(&in.AutoPull, req.AutoPull)
	setIf(&in.TimeoutSec, req.TimeoutSec)
	setIf(&in.Enabled, req.Enabled)
	setIf(&in.Releases, req.Releases)
	setIf(&in.KeepReleases, req.KeepReleases)
	setIf(&in.SharedPaths, req.SharedPaths)
	setIf(&in.AfterActivate, req.AfterActivate)
	in.Runtimes = req.Runtimes

	before, err := a.projectDTO(r, p)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if err := a.projects.Update(r.Context(), p, in, ident.User.ID); err != nil {
		a.projectError(w, r, err)
		return
	}
	after, err := a.projectDTO(r, p)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	a.recordProject(r, ident.User.ID, audit.ProjectUpdate, p, map[string]any{"changes": diffProjects(before, after)})
	httpx.JSON(w, http.StatusOK, after)
}

func (a *API) deleteProject(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectEdit)
	if !ok {
		return
	}
	a.deploys.CancelProject(r.Context(), p.ID)
	if err := a.projects.Delete(r.Context(), p); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	a.recordProject(r, ident.User.ID, audit.ProjectDelete, p, map[string]any{"name": p.Name})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) projectSecrets(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectSecrets)
	if !ok {
		return
	}
	a.recordProject(r, ident.User.ID, audit.ProjectSecretsView, p, nil)
	a.writeSecrets(w, r, p)
}

func (a *API) writeSecrets(w http.ResponseWriter, r *http.Request, p *store.Project) {
	key, fp, err := a.projects.PublicKey(p)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	path := "/hook/" + p.PublicID
	out := ProjectSecrets{
		WebhookPath:    path,
		WebhookSecret:  p.WebhookSecret,
		PublicKey:      key,
		KeyFingerprint: fp,
	}
	if a.cfg.BaseURL != "" {
		out.WebhookURL = a.cfg.BaseURL + path
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (a *API) regenerateKey(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectSecrets)
	if !ok {
		return
	}
	if err := a.projects.RegenerateKey(p); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	a.recordProject(r, ident.User.ID, audit.ProjectKeyRegenerate, p, nil)
	a.writeSecrets(w, r, p)
}

func (a *API) regenerateSecret(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectSecrets)
	if !ok {
		return
	}
	if err := a.projects.RegenerateSecret(r.Context(), p); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	a.recordProject(r, ident.User.ID, audit.ProjectSecretRegenerate, p, nil)
	a.writeSecrets(w, r, p)
}

func (a *API) testConnection(w http.ResponseWriter, r *http.Request) {
	p, _, ok := a.loadProject(w, r, auth.ActionProjectEdit)
	if !ok {
		return
	}
	refs, err := a.projects.TestConnection(r.Context(), p)
	if err != nil {
		httpx.JSON(w, http.StatusOK, ConnectionTest{OK: false, Message: err.Error(), Branches: []string{}})
		return
	}
	out := ConnectionTest{OK: true, Branches: []string{}}
	for i, ref := range refs {
		if i == 100 {
			break
		}
		out.Branches = append(out.Branches, ref.Name)
	}
	out.Message = "Connected: " + strconv.Itoa(len(refs)) + " branches found"
	httpx.JSON(w, http.StatusOK, out)
}

// Deployers may read scripts (they trigger them); only editors may change them.
func (a *API) getScript(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectDeploy)
	if !ok {
		return
	}
	s, err := a.projects.ReadScript(p)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, Script{
		Source:   s.Source,
		Path:     s.Path,
		Content:  s.Content,
		Exists:   s.Exists,
		Editable: s.Source == store.ScriptManaged && a.authz.Can(r.Context(), ident.User, auth.ActionProjectEdit, p.ID),
	})
}

func (a *API) saveScript(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectEdit)
	if !ok {
		return
	}
	var req SaveScriptRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	rev, err := a.projects.SaveScript(r.Context(), p, req.Content, ident.User.ID)
	if err != nil {
		a.projectError(w, r, err)
		return
	}
	a.recordProject(r, ident.User.ID, audit.ProjectScriptUpdate, p, map[string]any{"revision": rev.ID})
	a.getScript(w, r)
}

func (a *API) listScriptRevisions(w http.ResponseWriter, r *http.Request) {
	p, _, ok := a.loadProject(w, r, auth.ActionProjectDeploy)
	if !ok {
		return
	}
	revs, err := a.store.ListScriptRevisions(r.Context(), p.ID, 30)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	out := []ScriptRevision{}
	for _, rev := range revs {
		out = append(out, ScriptRevision{
			ID: rev.ID, Content: rev.Content, CreatedBy: rev.Username,
			CreatedAt: rev.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	httpx.JSON(w, http.StatusOK, out)
}

func toProject(p *store.Project) Project {
	out := Project{
		ID: p.ID, Slug: p.Slug, Name: p.Name, Provider: p.Provider, RepoURL: p.RepoURL,
		WorkDir: p.WorkDir, ScriptSource: p.ScriptSource, ScriptPath: p.ScriptPath,
		AutoPull: p.AutoPull, TimeoutSec: p.TimeoutSec, Enabled: p.Enabled,
		Releases: p.Releases, KeepReleases: p.KeepReleases, SharedPaths: p.SharedPaths, AfterActivate: p.AfterActivate,
		CreatedAt:   p.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   p.UpdatedAt.UTC().Format(time.RFC3339),
		Permissions: []string{},
	}
	if p.Releases {
		if id := release.Live(p.WorkDir); id != 0 {
			out.LiveRelease = &id
		}
	}
	return out
}

// projectDTO is toProject with the project's runtime settings and what the
// signed-in user may do on it.
func (a *API) projectDTO(r *http.Request, p *store.Project) (Project, error) {
	rt, err := a.store.ProjectRuntimes(r.Context(), p.ID)
	if err != nil {
		return Project{}, err
	}
	dto := toProject(p)
	dto.Runtimes = a.projectRuntimes(rt)
	dto.Permissions = a.projectPermissions(r, p.ID)
	return dto, nil
}

func (a *API) projectPermissions(r *http.Request, projectID int64) []string {
	out := []string{}
	for _, act := range a.authz.ProjectPermissions(r.Context(), auth.FromContext(r.Context()).User, projectID) {
		out = append(out, string(act))
	}
	return out
}

// diffProjects lists changed fields as {field: [old, new]} for the audit log.
func diffProjects(before, after Project) map[string][2]any {
	out := map[string][2]any{}
	add := func(name string, a, b any) {
		if a != b {
			out[name] = [2]any{a, b}
		}
	}
	add("name", before.Name, after.Name)
	add("provider", before.Provider, after.Provider)
	add("repo_url", before.RepoURL, after.RepoURL)
	add("work_dir", before.WorkDir, after.WorkDir)
	add("script_source", before.ScriptSource, after.ScriptSource)
	add("script_path", before.ScriptPath, after.ScriptPath)
	add("auto_pull", before.AutoPull, after.AutoPull)
	add("timeout_sec", before.TimeoutSec, after.TimeoutSec)
	add("enabled", before.Enabled, after.Enabled)
	add("releases", before.Releases, after.Releases)
	add("keep_releases", before.KeepReleases, after.KeepReleases)
	add("shared_paths", strings.Join(before.SharedPaths, " "), strings.Join(after.SharedPaths, " "))
	add("after_activate", before.AfterActivate, after.AfterActivate)
	for i, r := range after.Runtimes {
		if i < len(before.Runtimes) && before.Runtimes[i].Name == r.Name {
			add("runtimes."+r.Name, before.Runtimes[i].Spec, r.Spec)
		}
	}
	return out
}

func setIf[T any](dst *T, v *T) {
	if v != nil {
		*dst = *v
	}
}

func mergeDetail(base, extra map[string]any) map[string]any {
	for k, v := range extra {
		base[k] = v
	}
	return base
}
