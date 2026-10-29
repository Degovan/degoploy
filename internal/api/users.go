package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/degovan/degoploy/internal/audit"
	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/store"
)

func (a *API) registerUsers(mux *http.ServeMux) {
	need := func(action auth.Action, fn http.HandlerFunc) http.Handler {
		return auth.RequireAuth(a.authz.Require(action, fn))
	}
	mux.Handle("GET /api/v1/users", need(auth.ActionUserManage, a.listUsers))
	mux.Handle("POST /api/v1/users", need(auth.ActionUserManage, a.createUser))
	mux.Handle("PATCH /api/v1/users/{uid}", need(auth.ActionUserManage, a.updateUser))
	mux.Handle("PUT /api/v1/users/{uid}/password", need(auth.ActionUserManage, a.setUserPassword))
	mux.Handle("GET /api/v1/users/{uid}/sessions", need(auth.ActionUserManage, a.userSessions))
	mux.Handle("DELETE /api/v1/users/{uid}", need(auth.ActionUserManage, a.deleteUser))
	mux.Handle("POST /api/v1/me/password", auth.RequireAuth(http.HandlerFunc(a.changeOwnPassword)))
	mux.Handle("GET /api/v1/me/sessions", auth.RequireAuth(http.HandlerFunc(a.ownSessions)))
	mux.Handle("GET /api/v1/audit", need(auth.ActionAuditView, a.listAudit))
	mux.Handle("GET /api/v1/audit/summary", need(auth.ActionAuditView, a.auditSummary))
}

func validationError(w http.ResponseWriter, field, msg string) {
	httpx.JSON(w, http.StatusUnprocessableEntity, httpx.ErrorBody{Error: msg, Code: "validation", Field: field})
}

func (a *API) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.store.ListUsers(r.Context())
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	out := make([]User, 0, len(users))
	for _, u := range users {
		out = append(out, toUser(u))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	ident := auth.FromContext(r.Context())
	var req CreateUserRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	switch {
	case !auth.ValidUsername(req.Username):
		validationError(w, "username", auth.UsernameRule)
		return
	case req.Email != "" && !strings.Contains(req.Email, "@"):
		validationError(w, "email", "is not a valid email address")
		return
	case !store.Role(req.Role).Valid():
		validationError(w, "role", "must be admin, deployer or viewer")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		validationError(w, "password", err.Error())
		return
	}
	u := &store.User{
		Username: req.Username, Email: req.Email, DisplayName: firstNonEmpty(req.DisplayName, req.Username),
		AuthProvider: store.ProviderLocal, PasswordHash: hash, Role: store.Role(req.Role),
	}
	if err := a.store.CreateUser(r.Context(), u); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			validationError(w, "username", "is already taken")
			return
		}
		httpx.InternalError(w, r, err)
		return
	}
	a.recordUser(r, ident.User.ID, audit.UserCreate, u, map[string]any{"role": u.Role})
	httpx.JSON(w, http.StatusCreated, toUser(u))
}

func (a *API) updateUser(w http.ResponseWriter, r *http.Request) {
	ident := auth.FromContext(r.Context())
	u, ok := a.loadUser(w, r)
	if !ok {
		return
	}
	var req UpdateUserRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	next := *u
	if req.Email != nil {
		next.Email = strings.TrimSpace(*req.Email)
		if next.Email != "" && !strings.Contains(next.Email, "@") {
			validationError(w, "email", "is not a valid email address")
			return
		}
	}
	if req.DisplayName != nil {
		next.DisplayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.Role != nil {
		if !store.Role(*req.Role).Valid() {
			validationError(w, "role", "must be admin, deployer or viewer")
			return
		}
		next.Role = store.Role(*req.Role)
	}
	if req.Disabled != nil {
		next.Disabled = *req.Disabled
	}

	if u.ID == ident.User.ID && (next.Role != u.Role || next.Disabled) {
		httpx.Error(w, http.StatusConflict, "self", "you cannot change your own role or disable yourself")
		return
	}
	if !a.keepsAnAdmin(w, r, u, next.Role == store.RoleAdmin && !next.Disabled) {
		return
	}

	if err := a.store.UpdateUser(r.Context(), &next); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if next.Disabled && !u.Disabled {
		// Signs the user out everywhere, including open realtime connections.
		if err := a.store.DeleteUserSessions(r.Context(), u.ID); err != nil {
			httpx.InternalError(w, r, err)
			return
		}
	}
	changes := map[string][2]any{}
	if u.Email != next.Email {
		changes["email"] = [2]any{u.Email, next.Email}
	}
	if u.DisplayName != next.DisplayName {
		changes["display_name"] = [2]any{u.DisplayName, next.DisplayName}
	}
	if u.Role != next.Role {
		changes["role"] = [2]any{u.Role, next.Role}
	}
	if u.Disabled != next.Disabled {
		changes["disabled"] = [2]any{u.Disabled, next.Disabled}
	}
	a.recordUser(r, ident.User.ID, audit.UserUpdate, &next, map[string]any{"changes": changes})
	httpx.JSON(w, http.StatusOK, toUser(&next))
}

func (a *API) setUserPassword(w http.ResponseWriter, r *http.Request) {
	ident := auth.FromContext(r.Context())
	u, ok := a.loadUser(w, r)
	if !ok {
		return
	}
	if u.AuthProvider != store.ProviderLocal {
		httpx.Error(w, http.StatusConflict, "sso_user", "this user signs in with SSO and has no local password")
		return
	}
	var req SetPasswordRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		validationError(w, "password", err.Error())
		return
	}
	// Asking yourself to change again right away would only lock you out of this session.
	mustChange := req.RequireChange && u.ID != ident.User.ID
	if err := a.store.SetUserPassword(r.Context(), u.ID, hash, mustChange); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	// Existing sessions of that user end, except the admin's own when resetting themselves.
	if u.ID == ident.User.ID {
		err = a.store.DeleteUserSessionsExcept(r.Context(), u.ID, ident.Session.IDHash)
	} else {
		err = a.store.DeleteUserSessions(r.Context(), u.ID)
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	a.recordUser(r, ident.User.ID, audit.UserPasswordReset, u, map[string]any{"require_change": mustChange})
	w.WriteHeader(http.StatusNoContent)
}

// userSessions counts the sessions a password reset would end: all of the
// user's, or, when resetting yourself, all but the current one.
func (a *API) userSessions(w http.ResponseWriter, r *http.Request) {
	ident := auth.FromContext(r.Context())
	u, ok := a.loadUser(w, r)
	if !ok {
		return
	}
	except := ""
	if u.ID == ident.User.ID {
		except = ident.Session.IDHash
	}
	n, err := a.store.CountUserSessions(r.Context(), u.ID, except)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, UserSessions{Active: n})
}

func (a *API) deleteUser(w http.ResponseWriter, r *http.Request) {
	ident := auth.FromContext(r.Context())
	u, ok := a.loadUser(w, r)
	if !ok {
		return
	}
	if u.ID == ident.User.ID {
		httpx.Error(w, http.StatusConflict, "self", "you cannot delete your own account")
		return
	}
	if !a.keepsAnAdmin(w, r, u, false) {
		return
	}
	if err := a.store.DeleteUser(r.Context(), u.ID); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	a.recordUser(r, ident.User.ID, audit.UserDelete, u, map[string]any{"username": u.Username})
	w.WriteHeader(http.StatusNoContent)
}

// ownSessions counts your other sessions, which a password change signs out.
func (a *API) ownSessions(w http.ResponseWriter, r *http.Request) {
	ident := auth.FromContext(r.Context())
	n, err := a.store.CountUserSessions(r.Context(), ident.User.ID, ident.Session.IDHash)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, UserSessions{Active: n})
}

func (a *API) changeOwnPassword(w http.ResponseWriter, r *http.Request) {
	ident := auth.FromContext(r.Context())
	u := ident.User
	if u.AuthProvider != store.ProviderLocal {
		httpx.Error(w, http.StatusConflict, "sso_user", "you sign in with SSO; change your password there")
		return
	}
	ip := a.clientIP(r)
	if !a.loginLimiter.Allow(ip) {
		httpx.Error(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later")
		return
	}
	var req ChangePasswordRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if !auth.VerifyPassword(u.PasswordHash, req.CurrentPassword) {
		validationError(w, "current_password", "is incorrect")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		validationError(w, "new_password", err.Error())
		return
	}
	if err := a.store.SetUserPassword(r.Context(), u.ID, hash, false); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if err := a.store.DeleteUserSessionsExcept(r.Context(), u.ID, ident.Session.IDHash); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	a.audit.Record(r.Context(), audit.Entry{UserID: u.ID, Action: audit.PasswordChange, IP: ip,
		TargetType: "user", TargetID: strconv.FormatInt(u.ID, 10)})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) loadUser(w http.ResponseWriter, r *http.Request) (*store.User, bool) {
	id, err := strconv.ParseInt(r.PathValue("uid"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "user not found")
		return nil, false
	}
	u, err := a.store.UserByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found", "user not found")
		return nil, false
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return nil, false
	}
	return u, true
}

// keepsAnAdmin rejects a change that would leave no active admin. stillAdmin
// says whether u remains an active admin after the change.
func (a *API) keepsAnAdmin(w http.ResponseWriter, r *http.Request, u *store.User, stillAdmin bool) bool {
	if u.Role != store.RoleAdmin || u.Disabled || stillAdmin {
		return true
	}
	n, err := a.store.CountActiveAdmins(r.Context())
	if err != nil {
		httpx.InternalError(w, r, err)
		return false
	}
	if n <= 1 {
		httpx.Error(w, http.StatusConflict, "last_admin", "at least one active admin must remain")
		return false
	}
	return true
}

func (a *API) recordUser(r *http.Request, actorID int64, action string, u *store.User, detail map[string]any) {
	a.audit.Record(r.Context(), audit.Entry{
		UserID: actorID, Action: action, TargetType: "user", TargetID: strconv.FormatInt(u.ID, 10),
		Detail: mergeDetail(map[string]any{"username": u.Username}, detail), IP: a.clientIP(r),
	})
}
