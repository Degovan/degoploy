package admin

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/degovan/degoploy/internal/audit"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/node"
	"github.com/degovan/degoploy/internal/store"
)

// NodeSpecRequest names a version: "22", "20.11.1", "lts", or "" for the default only.
type NodeSpecRequest struct {
	Spec string `json:"spec"`
}

// NodeInstallResult is the version an install fetched, with the new state.
type NodeInstallResult struct {
	Version string      `json:"version"`
	Report  node.Report `json:"report"`
}

type NodeUpdateResult struct {
	Changes []NodeChange `json:"changes"`
	Report  node.Report  `json:"report"`
}

type NodeChange struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Removed bool   `json:"removed"`
}

func (h *handler) registerNode(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/node", h.nodeReport)
	mux.HandleFunc("POST /v1/node/install", h.nodeInstall)
	mux.HandleFunc("DELETE /v1/node/{version}", h.nodeRemove)
	mux.HandleFunc("PUT /v1/node/default", h.nodeDefault)
	mux.HandleFunc("POST /v1/node/update", h.nodeUpdate)
	mux.HandleFunc("POST /v1/node/installing/{version}/cancel", h.nodeCancel)
}

func (h *handler) report(ctx context.Context) (node.Report, error) {
	projects, err := h.Store.ListProjects(ctx)
	if err != nil {
		return node.Report{}, err
	}
	def, err := h.Store.Setting(ctx, store.SettingNodeDefault)
	if err != nil {
		return node.Report{}, err
	}
	return h.Node.Report(projects, def)
}

func (h *handler) nodeReport(w http.ResponseWriter, r *http.Request) {
	rep, err := h.report(r.Context())
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, rep)
}

func (h *handler) nodeInstall(w http.ResponseWriter, r *http.Request) {
	var req NodeSpecRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	req.Spec = strings.TrimSpace(req.Spec)
	if _, err := node.ParseSpec(req.Spec); err != nil {
		invalid(w, "spec", err.Error())
		return
	}
	// The download goes on if the page or CLI that started it goes away; the
	// Node.js page shows its progress, and the mirror timeout bounds it.
	ctx := context.WithoutCancel(r.Context())
	v, err := h.Node.Latest(ctx, req.Spec)
	if err == nil {
		err = h.Node.Install(ctx, v, nil)
	}
	if errors.Is(err, node.ErrCanceled) {
		httpx.Error(w, http.StatusConflict, "canceled", "the install was canceled")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "node_install", err.Error())
		return
	}
	h.recordNode(r, audit.NodeInstall, map[string]any{"version": v.String(), "spec": req.Spec})
	rep, err := h.report(r.Context())
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, NodeInstallResult{Version: v.String(), Report: rep})
}

func (h *handler) nodeRemove(w http.ResponseWriter, r *http.Request) {
	v, err := node.ParseVersion(r.PathValue("version"))
	if err == nil {
		err = h.Node.Remove(v)
	}
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	h.recordNode(r, audit.NodeRemove, map[string]any{"version": v.String()})
	h.nodeReport(w, r)
}

func (h *handler) nodeDefault(w http.ResponseWriter, r *http.Request) {
	var req NodeSpecRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	req.Spec = strings.TrimSpace(req.Spec)
	if req.Spec != "" {
		if _, err := node.ParseSpec(req.Spec); err != nil {
			invalid(w, "spec", err.Error())
			return
		}
	}
	before, err := h.Store.Setting(r.Context(), store.SettingNodeDefault)
	if err == nil {
		err = h.Store.SetSetting(r.Context(), store.SettingNodeDefault, req.Spec)
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if before != req.Spec {
		h.recordNode(r, audit.NodeDefault, map[string]any{"from": before, "to": req.Spec})
	}
	h.nodeReport(w, r)
}

func (h *handler) nodeUpdate(w http.ResponseWriter, r *http.Request) {
	projects, err := h.Store.ListProjects(r.Context())
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	changes, err := h.Node.Update(context.WithoutCancel(r.Context()), node.Keep(projects), nil)
	out := NodeUpdateResult{Changes: []NodeChange{}}
	for _, c := range changes {
		out.Changes = append(out.Changes, NodeChange{From: c.From.String(), To: c.To.String(), Removed: c.Removed})
	}
	if len(out.Changes) > 0 {
		h.recordNode(r, audit.NodeUpdate, map[string]any{"changes": out.Changes})
	}
	if errors.Is(err, node.ErrCanceled) {
		httpx.Error(w, http.StatusConflict, "canceled", "the update was canceled")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "node_update", err.Error())
		return
	}
	if out.Report, err = h.report(r.Context()); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *handler) nodeCancel(w http.ResponseWriter, r *http.Request) {
	v, err := node.ParseVersion(r.PathValue("version"))
	if err != nil || !h.Node.Cancel(v) {
		httpx.Error(w, http.StatusNotFound, "not_found", "that version is not being installed")
		return
	}
	h.recordNode(r, audit.NodeCancel, map[string]any{"version": v.String()})
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) recordNode(r *http.Request, action string, detail map[string]any) {
	detail["via"] = "cli"
	detail["os_user"] = PeerFrom(r.Context()).Name
	target, _ := detail["version"].(string)
	h.Audit.Record(r.Context(), audit.Entry{Action: action, TargetType: "node", TargetID: target, Detail: detail})
}

func (c *Client) NodeReport(ctx context.Context) (node.Report, error) {
	var r node.Report
	return r, c.do(ctx, http.MethodGet, "/v1/node", nil, &r)
}

func (c *Client) NodeInstall(ctx context.Context, spec string) (NodeInstallResult, error) {
	var r NodeInstallResult
	return r, c.do(ctx, http.MethodPost, "/v1/node/install", NodeSpecRequest{Spec: spec}, &r)
}

func (c *Client) NodeRemove(ctx context.Context, version string) (node.Report, error) {
	var r node.Report
	return r, c.do(ctx, http.MethodDelete, "/v1/node/"+url.PathEscape(version), nil, &r)
}

func (c *Client) NodeDefault(ctx context.Context, spec string) (node.Report, error) {
	var r node.Report
	return r, c.do(ctx, http.MethodPut, "/v1/node/default", NodeSpecRequest{Spec: spec}, &r)
}

func (c *Client) NodeCancel(ctx context.Context, version string) error {
	return c.do(ctx, http.MethodPost, "/v1/node/installing/"+url.PathEscape(version)+"/cancel", nil, nil)
}

func (c *Client) NodeUpdate(ctx context.Context) (NodeUpdateResult, error) {
	var r NodeUpdateResult
	return r, c.do(ctx, http.MethodPost, "/v1/node/update", nil, &r)
}
