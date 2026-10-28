package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/degovan/degoploy/internal/audit"
	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/secrets"
	"github.com/degovan/degoploy/internal/store"
	"github.com/degovan/degoploy/internal/trigger"
)

// maxVariableSize bounds one value; enough for keys and certificates.
const maxVariableSize = 64 << 10

func (a *API) registerVariables(mux *http.ServeMux) {
	h := func(fn http.HandlerFunc) http.Handler { return auth.RequireAuth(fn) }

	mux.Handle("GET /api/v1/projects/{id}/variables", h(a.listVariables))
	mux.Handle("POST /api/v1/projects/{id}/variables", h(a.createVariable))
	mux.Handle("PUT /api/v1/projects/{id}/variables/{varID}", h(a.updateVariable))
	mux.Handle("DELETE /api/v1/projects/{id}/variables/{varID}", h(a.deleteVariable))
}

func (a *API) listVariables(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectView)
	if !ok {
		return
	}
	vars, err := a.store.ListVariables(r.Context(), p.ID)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	reveal := a.authz.Can(r.Context(), ident.User, auth.ActionProjectEdit, p.ID)
	out := make([]Variable, 0, len(vars))
	for _, v := range vars {
		out = append(out, toVariable(v, reveal))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (a *API) createVariable(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectSecrets)
	if !ok {
		return
	}
	var in VariableInput
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !validateVariable(w, in) {
		return
	}
	if in.Value == nil || (in.Secret && *in.Value == "") {
		validationError(w, "value", "is required")
		return
	}
	v := &store.Variable{ProjectID: p.ID, Name: in.Name, Secret: in.Secret, UpdatedBy: &ident.User.ID}
	if !a.setVariableValue(w, r, v, *in.Value) {
		return
	}
	if !a.saveVariable(w, r, v, a.store.CreateVariable) {
		return
	}
	a.recordProject(r, ident.User.ID, audit.VariableCreate, p, map[string]any{"name": v.Name, "secret": v.Secret})
	v.UpdatedByName = ident.User.Username
	httpx.JSON(w, http.StatusCreated, toVariable(v, true))
}

func (a *API) updateVariable(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectSecrets)
	if !ok {
		return
	}
	v, ok := a.loadVariable(w, r, p)
	if !ok {
		return
	}
	var in VariableInput
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !validateVariable(w, in) {
		return
	}
	if in.Secret && in.Value != nil && *in.Value == "" {
		validationError(w, "value", "is required")
		return
	}

	// Without a new value the stored one stays, converted if the kind changes.
	// A secret cannot be read back, so turning it into a plain variable needs one.
	switch {
	case in.Value != nil:
		v.Secret = in.Secret
		if !a.setVariableValue(w, r, v, *in.Value) {
			return
		}
	case v.Secret && !in.Secret:
		validationError(w, "value", "enter the value to store it as a plain variable")
		return
	case !v.Secret && in.Secret:
		v.Secret = true
		if !a.setVariableValue(w, r, v, v.Value) {
			return
		}
	}
	renamed := v.Name != in.Name
	from := v.Name
	v.Name, v.UpdatedBy = in.Name, &ident.User.ID
	if !a.saveVariable(w, r, v, a.store.UpdateVariable) {
		return
	}
	detail := map[string]any{"name": v.Name, "secret": v.Secret, "value_changed": in.Value != nil}
	if renamed {
		detail["renamed_from"] = from
	}
	a.recordProject(r, ident.User.ID, audit.VariableUpdate, p, detail)
	v.UpdatedByName = ident.User.Username
	httpx.JSON(w, http.StatusOK, toVariable(v, true))
}

func (a *API) deleteVariable(w http.ResponseWriter, r *http.Request) {
	p, ident, ok := a.loadProject(w, r, auth.ActionProjectSecrets)
	if !ok {
		return
	}
	v, ok := a.loadVariable(w, r, p)
	if !ok {
		return
	}
	if err := a.store.DeleteVariable(r.Context(), p.ID, v.ID); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	a.recordProject(r, ident.User.ID, audit.VariableDelete, p, map[string]any{"name": v.Name, "secret": v.Secret})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) loadVariable(w http.ResponseWriter, r *http.Request, p *store.Project) (*store.Variable, bool) {
	id, err := strconv.ParseInt(r.PathValue("varID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "variable not found")
		return nil, false
	}
	v, err := a.store.VariableByID(r.Context(), p.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found", "variable not found")
		return nil, false
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return nil, false
	}
	return v, true
}

// setVariableValue stores value in v, sealed when v is a secret.
func (a *API) setVariableValue(w http.ResponseWriter, r *http.Request, v *store.Variable, value string) bool {
	if !v.Secret {
		v.Value = value
		return true
	}
	if a.secrets == nil {
		httpx.InternalError(w, r, errors.New("no secret key configured"))
		return false
	}
	v.Value = a.secrets.Seal(value, secrets.ProjectScope(v.ProjectID))
	return true
}

// saveVariable runs save (create or update) and reports a taken name on the form.
func (a *API) saveVariable(w http.ResponseWriter, r *http.Request, v *store.Variable,
	save func(context.Context, *store.Variable) error) bool {
	err := save(r.Context(), v)
	if errors.Is(err, store.ErrDuplicate) {
		validationError(w, "name", v.Name+" already exists in this project")
		return false
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return false
	}
	return true
}

func validateVariable(w http.ResponseWriter, in VariableInput) bool {
	if msg := trigger.CheckEnvName(in.Name); msg != "" {
		validationError(w, "name", msg)
		return false
	}
	if in.Value != nil && len(*in.Value) > maxVariableSize {
		validationError(w, "value", "must be at most 64 KiB")
		return false
	}
	return true
}

func toVariable(v *store.Variable, reveal bool) Variable {
	value := v.Value
	switch {
	case v.Secret:
		value = ""
	case !reveal:
		value = "••••••"
	}
	return Variable{
		ID: v.ID, Name: v.Name, Value: value, Secret: v.Secret,
		UpdatedAt: msTime(v.UpdatedAt), UpdatedBy: v.UpdatedByName,
	}
}
