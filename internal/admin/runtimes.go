package admin

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/degovan/degoploy/internal/audit"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/store"
	"github.com/degovan/degoploy/internal/toolchain"
)

// RuntimeSpecRequest names a version: "22", "8.3", "lts", or "" for the default only.
type RuntimeSpecRequest struct {
	Spec string `json:"spec"`
}

// RuntimeInstallResult is the version an install fetched, with the new state.
type RuntimeInstallResult struct {
	Version string           `json:"version"`
	Report  toolchain.Report `json:"report"`
}

type RuntimeUpdateResult struct {
	Changes []RuntimeChange  `json:"changes"`
	Report  toolchain.Report `json:"report"`
}

type RuntimeChange struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Removed bool   `json:"removed"`
}

func (h *handler) registerRuntimes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/runtimes/{name}", h.runtimeReport)
	mux.HandleFunc("POST /v1/runtimes/{name}/install", h.runtimeInstall)
	mux.HandleFunc("DELETE /v1/runtimes/{name}/versions/{version}", h.runtimeRemove)
	mux.HandleFunc("PUT /v1/runtimes/{name}/default", h.runtimeDefault)
	mux.HandleFunc("POST /v1/runtimes/{name}/update", h.runtimeUpdate)
	mux.HandleFunc("POST /v1/runtimes/{name}/installing/{version}/cancel", h.runtimeCancel)
}

func (h *handler) runtime(w http.ResponseWriter, r *http.Request) (*toolchain.Manager, bool) {
	m := h.Runtimes.Get(r.PathValue("name"))
	if m == nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "no such runtime")
	}
	return m, m != nil
}

func (h *handler) runtimeReport(w http.ResponseWriter, r *http.Request) {
	if m, ok := h.runtime(w, r); ok {
		h.writeReport(w, r, m)
	}
}

func (h *handler) writeReport(w http.ResponseWriter, r *http.Request, m *toolchain.Manager) {
	rep, err := m.Status(r.Context(), h.Store)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, rep)
}

func (h *handler) runtimeInstall(w http.ResponseWriter, r *http.Request) {
	m, ok := h.runtime(w, r)
	if !ok {
		return
	}
	var req RuntimeSpecRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	req.Spec = strings.TrimSpace(req.Spec)
	if _, err := toolchain.ParseSpec(req.Spec); err != nil {
		invalid(w, "spec", err.Error())
		return
	}
	// The download goes on if the CLI that started it goes away; Ctrl+C
	// cancels it explicitly.
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
	h.recordRuntime(r, m, audit.RuntimeInstall, map[string]any{"version": m.Label(v), "spec": req.Spec})
	rep, err := m.Status(r.Context(), h.Store)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, RuntimeInstallResult{Version: m.Label(v), Report: rep})
}

func (h *handler) runtimeRemove(w http.ResponseWriter, r *http.Request) {
	m, ok := h.runtime(w, r)
	if !ok {
		return
	}
	v, err := toolchain.ParseVersion(r.PathValue("version"))
	if err == nil {
		err = m.Remove(v)
	}
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	h.recordRuntime(r, m, audit.RuntimeRemove, map[string]any{"version": m.Label(v)})
	h.writeReport(w, r, m)
}

func (h *handler) runtimeDefault(w http.ResponseWriter, r *http.Request) {
	m, ok := h.runtime(w, r)
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
			invalid(w, "spec", err.Error())
			return
		}
	}
	key := store.SettingRuntimeDefault(m.Name())
	before, err := h.Store.Setting(r.Context(), key)
	if err == nil {
		err = h.Store.SetSetting(r.Context(), key, req.Spec)
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if before != req.Spec {
		h.recordRuntime(r, m, audit.RuntimeDefault, map[string]any{"from": before, "to": req.Spec})
	}
	h.writeReport(w, r, m)
}

func (h *handler) runtimeUpdate(w http.ResponseWriter, r *http.Request) {
	m, ok := h.runtime(w, r)
	if !ok {
		return
	}
	uses, err := m.Uses(r.Context(), h.Store)
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
		h.recordRuntime(r, m, audit.RuntimeUpdate, map[string]any{"changes": out.Changes})
	}
	if errors.Is(err, toolchain.ErrCanceled) {
		httpx.Error(w, http.StatusConflict, "canceled", "the update was canceled")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "runtime_update", err.Error())
		return
	}
	if out.Report, err = m.Status(r.Context(), h.Store); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *handler) runtimeCancel(w http.ResponseWriter, r *http.Request) {
	m, ok := h.runtime(w, r)
	if !ok {
		return
	}
	v, err := toolchain.ParseVersion(r.PathValue("version"))
	if err != nil || !m.Cancel(v) {
		httpx.Error(w, http.StatusNotFound, "not_found", "that version is not being installed")
		return
	}
	h.recordRuntime(r, m, audit.RuntimeCancel, map[string]any{"version": m.Label(v)})
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) recordRuntime(r *http.Request, m *toolchain.Manager, action string, detail map[string]any) {
	detail["runtime"] = m.Name()
	detail["title"] = m.Title()
	detail["via"] = "cli"
	detail["os_user"] = PeerFrom(r.Context()).Name
	target, _ := detail["version"].(string)
	h.Audit.Record(r.Context(), audit.Entry{Action: action, TargetType: "runtime", TargetID: target, Detail: detail})
}

func (c *Client) RuntimeReport(ctx context.Context, name string) (toolchain.Report, error) {
	var r toolchain.Report
	return r, c.do(ctx, http.MethodGet, "/v1/runtimes/"+url.PathEscape(name), nil, &r)
}

func (c *Client) RuntimeInstall(ctx context.Context, name, spec string) (RuntimeInstallResult, error) {
	var r RuntimeInstallResult
	return r, c.do(ctx, http.MethodPost, "/v1/runtimes/"+url.PathEscape(name)+"/install", RuntimeSpecRequest{Spec: spec}, &r)
}

func (c *Client) RuntimeRemove(ctx context.Context, name, version string) (toolchain.Report, error) {
	var r toolchain.Report
	return r, c.do(ctx, http.MethodDelete, "/v1/runtimes/"+url.PathEscape(name)+"/versions/"+url.PathEscape(version), nil, &r)
}

func (c *Client) RuntimeDefault(ctx context.Context, name, spec string) (toolchain.Report, error) {
	var r toolchain.Report
	return r, c.do(ctx, http.MethodPut, "/v1/runtimes/"+url.PathEscape(name)+"/default", RuntimeSpecRequest{Spec: spec}, &r)
}

func (c *Client) RuntimeUpdate(ctx context.Context, name string) (RuntimeUpdateResult, error) {
	var r RuntimeUpdateResult
	return r, c.do(ctx, http.MethodPost, "/v1/runtimes/"+url.PathEscape(name)+"/update", nil, &r)
}

func (c *Client) RuntimeCancel(ctx context.Context, name, version string) error {
	return c.do(ctx, http.MethodPost, "/v1/runtimes/"+url.PathEscape(name)+"/installing/"+url.PathEscape(version)+"/cancel", nil, nil)
}
