package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/degovan/degoploy/internal/audit"
	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/store"
	"github.com/degovan/degoploy/internal/toolchain"
)

func (a *API) registerRuntimes(mux *http.ServeMux) {
	h := func(fn http.HandlerFunc) http.Handler {
		return auth.RequireAuth(a.authz.Require(auth.ActionUserManage, fn))
	}
	mux.Handle("GET /api/v1/runtimes", h(a.listRuntimes))
	mux.Handle("GET /api/v1/runtimes/{name}/releases", h(a.runtimeReleases))
	mux.Handle("POST /api/v1/runtimes/{name}/install", h(a.runtimeInstall))
	mux.Handle("DELETE /api/v1/runtimes/{name}/versions/{version}", h(a.runtimeRemove))
	mux.Handle("PUT /api/v1/runtimes/{name}/default", h(a.runtimeDefault))
	mux.Handle("PUT /api/v1/runtimes/{name}/enabled", h(a.runtimeEnabled))
	mux.Handle("POST /api/v1/runtimes/{name}/update", h(a.runtimeUpdate))
	mux.Handle("POST /api/v1/runtimes/{name}/installing/{version}/cancel", h(a.runtimeCancel))
}

// runtime resolves {name}.
func (a *API) runtime(w http.ResponseWriter, r *http.Request) (*toolchain.Manager, bool) {
	m := a.runtimes.Get(r.PathValue("name"))
	if m == nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "no such runtime")
	}
	return m, m != nil
}

func (a *API) listRuntimes(w http.ResponseWriter, r *http.Request) {
	out := []RuntimeStatus{}
	for _, m := range a.runtimes.All() {
		st, err := a.runtimeStatus(r.Context(), m)
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		out = append(out, st)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (a *API) writeRuntime(w http.ResponseWriter, r *http.Request, m *toolchain.Manager) {
	st, err := a.runtimeStatus(r.Context(), m)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (a *API) runtimeStatus(ctx context.Context, m *toolchain.Manager) (RuntimeStatus, error) {
	rep, err := m.Status(ctx, a.store)
	if err != nil {
		return RuntimeStatus{}, err
	}
	out := RuntimeStatus{
		Name: rep.Name, Title: rep.Title, Platform: rep.Platform, Mirror: rep.Mirror, Verified: rep.Verified,
		Enabled: rep.Enabled, Default: rep.Default, TotalSize: rep.TotalSize,
		Installed: []RuntimeVersion{}, Installing: []RuntimeProgress{},
	}
	for _, p := range rep.Installing {
		out.Installing = append(out.Installing, RuntimeProgress{Version: p.Version, Phase: p.Phase, Done: p.Done, Total: p.Total})
	}
	for _, v := range rep.Installed {
		at := ""
		if !v.InstalledAt.IsZero() {
			at = v.InstalledAt.UTC().Format(time.RFC3339)
		}
		out.Installed = append(out.Installed, RuntimeVersion{
			Version: v.Version, LTS: v.LTS, Size: v.Size, InstalledAt: at, System: v.System, Path: v.Path,
			Default: v.Default, UsedBy: v.UsedBy, Pinned: v.Pinned,
		})
	}
	return out, nil
}

// runtimeReleases lists the newest release of each line, newest first.
func (a *API) runtimeReleases(w http.ResponseWriter, r *http.Request) {
	m, ok := a.runtime(w, r)
	if !ok {
		return
	}
	lines, err := m.Lines(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "mirror_unreachable", err.Error())
		return
	}
	have, err := m.Installed()
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	out := []RuntimeRelease{}
	for _, x := range lines {
		out = append(out, RuntimeRelease{
			Version: m.Label(x.Version), LTS: x.LTS, Date: x.Date,
			Installed: slices.ContainsFunc(have, func(in toolchain.Installed) bool { return in.Version == x.Version }),
		})
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (a *API) runtimeInstall(w http.ResponseWriter, r *http.Request) {
	m, ok := a.runtime(w, r)
	if !ok {
		return
	}
	var req RuntimeSpecRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	req.Spec = strings.TrimSpace(req.Spec)
	if _, err := toolchain.ParseSpec(req.Spec); err != nil {
		httpx.JSON(w, http.StatusUnprocessableEntity, httpx.ErrorBody{Error: err.Error(), Code: "validation", Field: "spec"})
		return
	}
	// The download goes on if the page or CLI that started it goes away; the
	// Runtimes page shows its progress, and the download timeout bounds it.
	ctx := context.WithoutCancel(r.Context())
	v, err := m.Latest(ctx, req.Spec)
	if err == nil {
		err = m.Install(ctx, v, nil)
	}
	if errors.Is(err, toolchain.ErrCanceled) {
		httpx.Error(w, http.StatusConflict, "canceled", "the install was canceled")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "runtime_install", err.Error())
		return
	}
	a.recordRuntime(r, m, audit.RuntimeInstall, map[string]any{"version": m.Label(v), "spec": req.Spec})
	a.writeRuntime(w, r, m)
}

func (a *API) runtimeRemove(w http.ResponseWriter, r *http.Request) {
	m, ok := a.runtime(w, r)
	if !ok {
		return
	}
	v, err := toolchain.ParseVersion(r.PathValue("version"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "no such version")
		return
	}
	if in, ok := m.Lookup(v); ok && in.System {
		httpx.Error(w, http.StatusConflict, "system_version", m.Title()+" "+m.Label(v)+" comes with the system; remove it with the system's package manager")
		return
	}
	if err := m.Remove(v); err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	a.recordRuntime(r, m, audit.RuntimeRemove, map[string]any{"version": m.Label(v)})
	a.writeRuntime(w, r, m)
}

func (a *API) runtimeDefault(w http.ResponseWriter, r *http.Request) {
	m, ok := a.runtime(w, r)
	if !ok {
		return
	}
	var req RuntimeSpecRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	req.Spec = strings.TrimSpace(req.Spec)
	if req.Spec != "" {
		if _, err := toolchain.ParseSpec(req.Spec); err != nil {
			httpx.JSON(w, http.StatusUnprocessableEntity, httpx.ErrorBody{Error: err.Error(), Code: "validation", Field: "spec"})
			return
		}
	}
	key := store.SettingRuntimeDefault(m.Name())
	before, err := a.store.Setting(r.Context(), key)
	if err == nil {
		err = a.store.SetSetting(r.Context(), key, req.Spec)
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if before != req.Spec {
		a.recordRuntime(r, m, audit.RuntimeDefault, map[string]any{"from": before, "to": req.Spec})
	}
	a.writeRuntime(w, r, m)
}

func (a *API) runtimeEnabled(w http.ResponseWriter, r *http.Request) {
	m, ok := a.runtime(w, r)
	if !ok {
		return
	}
	var req RuntimeEnabledRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	before, err := toolchain.Enabled(r.Context(), a.store, m.Name())
	value := ""
	if !req.Enabled {
		value = "false"
	}
	if err == nil {
		err = a.store.SetSetting(r.Context(), store.SettingRuntimeEnabled(m.Name()), value)
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if before != req.Enabled {
		a.recordRuntime(r, m, audit.RuntimeEnable, map[string]any{"enabled": req.Enabled})
	}
	a.writeRuntime(w, r, m)
}

func (a *API) runtimeUpdate(w http.ResponseWriter, r *http.Request) {
	m, ok := a.runtime(w, r)
	if !ok {
		return
	}
	uses, err := m.Uses(r.Context(), a.store)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	changes, err := m.Update(context.WithoutCancel(r.Context()), toolchain.Keep(uses), nil)
	out := RuntimeUpdateResult{Changes: []RuntimeChange{}}
	for _, c := range changes {
		out.Changes = append(out.Changes, RuntimeChange{From: m.Label(c.From), To: m.Label(c.To), Removed: c.Removed})
	}
	if len(out.Changes) > 0 {
		a.recordRuntime(r, m, audit.RuntimeUpdate, map[string]any{"changes": out.Changes})
	}
	if errors.Is(err, toolchain.ErrCanceled) {
		httpx.Error(w, http.StatusConflict, "canceled", "the update was canceled")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "runtime_update", err.Error())
		return
	}
	if out.Status, err = a.runtimeStatus(r.Context(), m); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// runtimeCancel stops a running install, whoever started it. A deployment
// that was waiting for it fails.
func (a *API) runtimeCancel(w http.ResponseWriter, r *http.Request) {
	m, ok := a.runtime(w, r)
	if !ok {
		return
	}
	v, err := toolchain.ParseVersion(r.PathValue("version"))
	if err != nil || !m.Cancel(v) {
		httpx.Error(w, http.StatusNotFound, "not_found", "that version is not being installed")
		return
	}
	a.recordRuntime(r, m, audit.RuntimeCancel, map[string]any{"version": m.Label(v)})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) recordRuntime(r *http.Request, m *toolchain.Manager, action string, detail map[string]any) {
	ident := auth.FromContext(r.Context())
	detail["runtime"] = m.Name()
	detail["title"] = m.Title()
	target, _ := detail["version"].(string)
	a.audit.Record(r.Context(), audit.Entry{
		UserID: ident.User.ID, Action: action, TargetType: "runtime", TargetID: target,
		Detail: detail, IP: a.clientIP(r),
	})
}

// projectRuntimes lists a project's runtime settings in display order;
// runtimes without a setting are on "auto".
func (a *API) projectRuntimes(rt map[string]store.ProjectRuntime) []ProjectRuntime {
	out := make([]ProjectRuntime, 0, len(toolchain.Runtimes))
	for _, name := range toolchain.Runtimes {
		r := rt[name]
		if r.Spec == "" {
			r.Spec = toolchain.Auto
		}
		out = append(out, ProjectRuntime{Name: name, Spec: r.Spec, Used: r.Used})
	}
	return out
}
