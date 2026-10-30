// Package admin serves the local admin socket. The degoploy CLI talks to the
// running service through it, so operators in the service's group manage users,
// check the configuration and take backups without sudo. Anyone who can connect
// to the socket is trusted like an admin; its file permissions are the gate.
package admin

import (
	"errors"
	"maps"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/degovan/degoploy/internal/audit"
	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/config"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/store"
	"github.com/degovan/degoploy/internal/toolchain"
)

// Status describes the service the CLI reached.
type Status struct {
	Version   string    `json:"version"`
	StartedAt time.Time `json:"started_at"`
	PID       int       `json:"pid"`
	Listen    string    `json:"listen"`
	BaseURL   string    `json:"base_url"`
	DataDir   string    `json:"data_dir"`
	Config    string    `json:"config"`
	Users     int       `json:"users"`
	Admins    int       `json:"admins"`
	Projects  int       `json:"projects"`
	// DeployUser runs git and deploy scripts; empty means the service's user.
	DeployUser string `json:"deploy_user"`
	// WorkDirs are the projects' working directories, for setup.
	WorkDirs []string `json:"work_dirs"`
	// You is the Linux user the service sees on the other end of the socket.
	You string `json:"you"`
	// Local is true when the CLI opened the data directly because the service
	// was not running.
	Local bool `json:"local"`
}

type User struct {
	ID                 int64      `json:"id"`
	Username           string     `json:"username"`
	Email              string     `json:"email"`
	DisplayName        string     `json:"display_name"`
	Provider           string     `json:"provider"`
	Role               string     `json:"role"`
	Disabled           bool       `json:"disabled"`
	MustChangePassword bool       `json:"must_change_password"`
	LastLoginAt        *time.Time `json:"last_login_at"`
	CreatedAt          time.Time  `json:"created_at"`
}

type CreateUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

type UpdateUserRequest struct {
	Role     *string `json:"role,omitempty"`
	Disabled *bool   `json:"disabled,omitempty"`
}

type SetPasswordRequest struct {
	Password      string `json:"password"`
	RequireChange bool   `json:"require_change"`
}

type Deps struct {
	Config   *config.Config
	Store    *store.Store
	Audit    *audit.Logger
	Runtimes *toolchain.Set
	Version  string
	Started  time.Time
	Local    bool
}

type handler struct{ Deps }

// NewHandler serves the admin API that Client calls.
func NewHandler(d Deps) http.Handler {
	if d.Runtimes == nil {
		d.Runtimes = toolchain.NewSet(d.Config.RuntimesDir, toolchain.Options{
			NodeMirror: d.Config.NodeMirror, PHPMirror: d.Config.PHPMirror, ComposerURL: d.Config.ComposerURL,
		})
	}
	h := &handler{d}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", h.status)
	mux.HandleFunc("GET /v1/users", h.listUsers)
	mux.HandleFunc("POST /v1/users", h.createUser)
	mux.HandleFunc("PATCH /v1/users/{name}", h.updateUser)
	mux.HandleFunc("PUT /v1/users/{name}/password", h.setPassword)
	mux.HandleFunc("GET /v1/config/check", h.checkConfig)
	mux.HandleFunc("GET /v1/backup", h.backup)
	h.registerRuntimes(mux)
	return mux
}

func (h *handler) status(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	users, err := h.Store.ListUsers(ctx)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	admins, err := h.Store.CountActiveAdmins(ctx)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	projects, err := h.Store.ListProjects(ctx)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, Status{
		Version:    h.Version,
		StartedAt:  h.Started,
		PID:        os.Getpid(),
		Listen:     h.Config.Listen,
		BaseURL:    h.Config.BaseURL,
		DataDir:    h.Config.DataDir,
		Config:     h.Config.Path,
		Users:      len(users),
		Admins:     admins,
		Projects:   len(projects),
		DeployUser: h.Config.DeployUser,
		WorkDirs:   workDirs(projects),
		You:        PeerFrom(ctx).Name,
		Local:      h.Local,
	})
}

func (h *handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.Store.ListUsers(r.Context())
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

func (h *handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
	switch {
	case !auth.ValidUsername(req.Username):
		invalid(w, "username", "username: "+auth.UsernameRule)
		return
	case req.Email != "" && !strings.Contains(req.Email, "@"):
		invalid(w, "email", "not a valid email address")
		return
	case !store.Role(req.Role).Valid():
		invalid(w, "role", "role must be admin, deployer or viewer")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		invalid(w, "password", err.Error())
		return
	}
	u := &store.User{
		Username: req.Username, Email: req.Email, DisplayName: req.Username,
		AuthProvider: store.ProviderLocal, PasswordHash: hash, Role: store.Role(req.Role),
	}
	if err := h.Store.CreateUser(r.Context(), u); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			invalid(w, "username", "a user named "+strconv.Quote(req.Username)+" already exists")
			return
		}
		httpx.InternalError(w, r, err)
		return
	}
	h.record(r, audit.UserCreate, u, map[string]any{"role": u.Role})
	httpx.JSON(w, http.StatusCreated, toUser(u))
}

func (h *handler) updateUser(w http.ResponseWriter, r *http.Request) {
	u, ok := h.loadUser(w, r)
	if !ok {
		return
	}
	var req UpdateUserRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	next := *u
	if req.Role != nil {
		if !store.Role(*req.Role).Valid() {
			invalid(w, "role", "role must be admin, deployer or viewer")
			return
		}
		next.Role = store.Role(*req.Role)
	}
	if req.Disabled != nil {
		next.Disabled = *req.Disabled
	}
	if u.Role == store.RoleAdmin && !u.Disabled && (next.Role != store.RoleAdmin || next.Disabled) {
		n, err := h.Store.CountActiveAdmins(r.Context())
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		if n <= 1 {
			httpx.Error(w, http.StatusConflict, "last_admin", "at least one active admin must remain")
			return
		}
	}
	if err := h.Store.UpdateUser(r.Context(), &next); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if next.Disabled && !u.Disabled {
		if err := h.Store.DeleteUserSessions(r.Context(), u.ID); err != nil {
			httpx.InternalError(w, r, err)
			return
		}
	}
	changes := map[string][2]any{}
	if u.Role != next.Role {
		changes["role"] = [2]any{u.Role, next.Role}
	}
	if u.Disabled != next.Disabled {
		changes["disabled"] = [2]any{u.Disabled, next.Disabled}
	}
	if len(changes) > 0 {
		h.record(r, audit.UserUpdate, &next, map[string]any{"changes": changes})
	}
	httpx.JSON(w, http.StatusOK, toUser(&next))
}

func (h *handler) setPassword(w http.ResponseWriter, r *http.Request) {
	u, ok := h.loadUser(w, r)
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
		invalid(w, "password", err.Error())
		return
	}
	if err := h.Store.SetUserPassword(r.Context(), u.ID, hash, req.RequireChange); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if err := h.Store.DeleteUserSessions(r.Context(), u.ID); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	h.record(r, audit.UserPasswordReset, u, map[string]any{"require_change": req.RequireChange})
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) checkConfig(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, Check(r.Context(), h.Config.Path))
}

func (h *handler) loadUser(w http.ResponseWriter, r *http.Request) (*store.User, bool) {
	u, err := h.Store.UserByUsername(r.Context(), r.PathValue("name"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found", "no user named "+strconv.Quote(r.PathValue("name")))
		return nil, false
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return nil, false
	}
	return u, true
}

// record audits a change made from the CLI. There is no degoploy user behind
// it, so the Linux account that ran the command is kept in the detail.
func (h *handler) record(r *http.Request, action string, u *store.User, detail map[string]any) {
	d := map[string]any{"username": u.Username, "via": "cli", "os_user": PeerFrom(r.Context()).Name}
	maps.Copy(d, detail)
	h.Audit.Record(r.Context(), audit.Entry{
		Action: action, TargetType: "user", TargetID: strconv.FormatInt(u.ID, 10), Detail: d,
	})
}

func invalid(w http.ResponseWriter, field, msg string) {
	httpx.JSON(w, http.StatusUnprocessableEntity, httpx.ErrorBody{Error: msg, Code: "validation", Field: field})
}

func toUser(u *store.User) User {
	return User{
		ID: u.ID, Username: u.Username, Email: u.Email, DisplayName: u.DisplayName,
		Provider: u.AuthProvider, Role: string(u.Role), Disabled: u.Disabled,
		MustChangePassword: u.MustChangePassword, LastLoginAt: u.LastLoginAt, CreatedAt: u.CreatedAt,
	}
}

func workDirs(projects []*store.Project) []string {
	dirs := make([]string, len(projects))
	for i, p := range projects {
		dirs[i] = p.WorkDir
	}
	return dirs
}
