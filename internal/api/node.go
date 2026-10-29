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
	"github.com/degovan/degoploy/internal/node"
	"github.com/degovan/degoploy/internal/store"
)

func (a *API) registerNode(mux *http.ServeMux) {
	h := func(fn http.HandlerFunc) http.Handler {
		return auth.RequireAuth(a.authz.Require(auth.ActionUserManage, fn))
	}
	mux.Handle("GET /api/v1/node", h(a.nodeStatus))
	mux.Handle("GET /api/v1/node/releases", h(a.nodeReleases))
	mux.Handle("POST /api/v1/node/install", h(a.nodeInstall))
	mux.Handle("DELETE /api/v1/node/versions/{version}", h(a.nodeRemove))
	mux.Handle("PUT /api/v1/node/default", h(a.nodeDefault))
	mux.Handle("POST /api/v1/node/update", h(a.nodeUpdate))
	mux.Handle("POST /api/v1/node/installing/{version}/cancel", h(a.nodeCancel))
}

func (a *API) nodeStatus(w http.ResponseWriter, r *http.Request) {
	st, err := a.buildNodeStatus(r)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (a *API) buildNodeStatus(r *http.Request) (NodeStatus, error) {
	projects, err := a.store.ListProjects(r.Context())
	if err != nil {
		return NodeStatus{}, err
	}
	def, err := a.store.Setting(r.Context(), store.SettingNodeDefault)
	if err != nil {
		return NodeStatus{}, err
	}
	rep, err := a.node.Report(projects, def)
	if err != nil {
		return NodeStatus{}, err
	}
	out := NodeStatus{Platform: rep.Platform, Mirror: rep.Mirror, Default: rep.Default, TotalSize: rep.TotalSize, Installed: []NodeVersion{}, Installing: []NodeProgress{}}
	for _, p := range rep.Installing {
		out.Installing = append(out.Installing, NodeProgress{Version: p.Version, Phase: p.Phase, Done: p.Done, Total: p.Total})
	}
	for _, v := range rep.Installed {
		at := ""
		if !v.InstalledAt.IsZero() {
			at = v.InstalledAt.UTC().Format(time.RFC3339)
		}
		out.Installed = append(out.Installed, NodeVersion{
			Version: v.Version, LTS: v.LTS, Size: v.Size, InstalledAt: at,
			Default: v.Default, UsedBy: v.UsedBy, Pinned: v.Pinned,
		})
	}
	return out, nil
}

// nodeReleases lists the newest release of each major line, newest first.
func (a *API) nodeReleases(w http.ResponseWriter, r *http.Request) {
	rel, err := a.node.Releases(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "mirror_unreachable", err.Error())
		return
	}
	have, err := a.node.Installed()
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	out := []NodeRelease{}
	seen := map[int]bool{}
	for _, x := range rel {
		if seen[x.Version.Major()] {
			continue
		}
		seen[x.Version.Major()] = true
		out = append(out, NodeRelease{
			Version: x.Version.String(), LTS: x.LTS, Date: x.Date,
			Installed: slices.ContainsFunc(have, func(in node.Installed) bool { return in.Version == x.Version }),
		})
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (a *API) nodeInstall(w http.ResponseWriter, r *http.Request) {
	var req NodeSpecRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	req.Spec = strings.TrimSpace(req.Spec)
	if _, err := node.ParseSpec(req.Spec); err != nil {
		httpx.JSON(w, http.StatusUnprocessableEntity, httpx.ErrorBody{Error: err.Error(), Code: "validation", Field: "spec"})
		return
	}
	// Ask the mirror for the newest release, as "install" means.
	// The download goes on if the page or CLI that started it goes away; the
	// Node.js page shows its progress, and the mirror timeout bounds it.
	ctx := context.WithoutCancel(r.Context())
	v, err := a.node.Latest(ctx, req.Spec)
	if err == nil {
		err = a.node.Install(ctx, v, nil)
	}
	if errors.Is(err, node.ErrCanceled) {
		httpx.Error(w, http.StatusConflict, "canceled", "the install was canceled")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "node_install", err.Error())
		return
	}
	a.recordNode(r, audit.NodeInstall, map[string]any{"version": v.String(), "spec": req.Spec})
	a.nodeStatus(w, r)
}

func (a *API) nodeRemove(w http.ResponseWriter, r *http.Request) {
	v, err := node.ParseVersion(r.PathValue("version"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "no such Node.js version")
		return
	}
	if err := a.node.Remove(v); err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	a.recordNode(r, audit.NodeRemove, map[string]any{"version": v.String()})
	a.nodeStatus(w, r)
}

func (a *API) nodeDefault(w http.ResponseWriter, r *http.Request) {
	var req NodeSpecRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	req.Spec = strings.TrimSpace(req.Spec)
	if req.Spec != "" {
		if _, err := node.ParseSpec(req.Spec); err != nil {
			httpx.JSON(w, http.StatusUnprocessableEntity, httpx.ErrorBody{Error: err.Error(), Code: "validation", Field: "spec"})
			return
		}
	}
	before, err := a.store.Setting(r.Context(), store.SettingNodeDefault)
	if err == nil {
		err = a.store.SetSetting(r.Context(), store.SettingNodeDefault, req.Spec)
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if before != req.Spec {
		a.recordNode(r, audit.NodeDefault, map[string]any{"from": before, "to": req.Spec})
	}
	a.nodeStatus(w, r)
}

func (a *API) nodeUpdate(w http.ResponseWriter, r *http.Request) {
	projects, err := a.store.ListProjects(r.Context())
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	changes, err := a.node.Update(context.WithoutCancel(r.Context()), node.Keep(projects), nil)
	out := NodeUpdateResult{Changes: []NodeChange{}}
	for _, c := range changes {
		out.Changes = append(out.Changes, NodeChange{From: c.From.String(), To: c.To.String(), Removed: c.Removed})
	}
	if len(out.Changes) > 0 {
		a.recordNode(r, audit.NodeUpdate, map[string]any{"changes": out.Changes})
	}
	if errors.Is(err, node.ErrCanceled) {
		httpx.Error(w, http.StatusConflict, "canceled", "the update was canceled")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "node_update", err.Error())
		return
	}
	if out.Status, err = a.buildNodeStatus(r); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// nodeCancel stops a running install, whoever started it. A deployment that
// was waiting for it fails.
func (a *API) nodeCancel(w http.ResponseWriter, r *http.Request) {
	v, err := node.ParseVersion(r.PathValue("version"))
	if err != nil || !a.node.Cancel(v) {
		httpx.Error(w, http.StatusNotFound, "not_found", "that version is not being installed")
		return
	}
	a.recordNode(r, audit.NodeCancel, map[string]any{"version": v.String()})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) recordNode(r *http.Request, action string, detail map[string]any) {
	ident := auth.FromContext(r.Context())
	a.audit.Record(r.Context(), audit.Entry{
		UserID: ident.User.ID, Action: action, TargetType: "node", TargetID: str(detail["version"]),
		Detail: detail, IP: a.clientIP(r),
	})
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
