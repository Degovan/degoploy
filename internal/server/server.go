// Package server wires configuration, storage and HTTP handlers together.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/degovan/degoploy/internal/admin"
	"github.com/degovan/degoploy/internal/api"
	"github.com/degovan/degoploy/internal/audit"
	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/config"
	"github.com/degovan/degoploy/internal/deploy"
	"github.com/degovan/degoploy/internal/project"
	"github.com/degovan/degoploy/internal/realtime"
	"github.com/degovan/degoploy/internal/runas"
	"github.com/degovan/degoploy/internal/secrets"
	"github.com/degovan/degoploy/internal/store"
	"github.com/degovan/degoploy/internal/toolchain"
	"github.com/degovan/degoploy/internal/web"
)

type Server struct {
	cfg     *config.Config
	store   *store.Store
	api     *api.API
	hub     *realtime.Hub
	deploys *deploy.Service
	http    *http.Server
	admin   http.Handler
}

// deployGrace is how long running deployments may finish during shutdown.
const deployGrace = 60 * time.Second

func New(cfg *config.Config, st *store.Store, box *secrets.Box, version string) *Server {
	sessions := auth.NewSessions(st, cfg.SessionTTL, cfg.SecureCookies())
	hub := realtime.NewHub()
	projects := project.NewManager(st, cfg.DataDir, cfg.ProjectsDir)
	runner := &runas.Runner{User: cfg.DeployUser}
	projects.CheckAccess = func(ctx context.Context, dir string, releases bool) error {
		msg, err := runner.CheckWorkDir(ctx, dir, releases)
		if err != nil {
			return err
		}
		if msg != "" {
			return &project.ValidationError{Field: "work_dir", Message: msg}
		}
		return nil
	}
	deploys := deploy.NewService(st, projects, hub, box, cfg.MaxParallel)
	deploys.Runner = runner
	runtimes := toolchain.NewSet(cfg.RuntimesDir, toolchain.Options{
		NodeMirror: cfg.NodeMirror, PHPMirror: cfg.PHPMirror, ComposerURL: cfg.ComposerURL,
	})
	deploys.Runtimes = runtimes
	auditLog := audit.New(st)
	var sso *auth.OIDCProvider
	if cfg.OIDC.Enabled {
		sso = auth.NewOIDCProvider(cfg.OIDC)
	}
	a := api.New(api.Deps{
		Config:   cfg,
		Store:    st,
		Sessions: sessions,
		Authz:    &auth.Authorizer{},
		Audit:    auditLog,
		Projects: projects,
		Deploys:  deploys,
		Hub:      hub,
		Secrets:  box,
		OIDC:     sso,
		Runtimes: runtimes,
		Version:  version,
	})

	mux := http.NewServeMux()
	a.Register(mux)
	mux.Handle("/", web.Handler())

	var h http.Handler = mux
	h = sessions.Load(h)
	h = securityHeaders(h)
	h = requestLog(h)
	h = recoverer(h)

	return &Server{
		cfg:     cfg,
		store:   st,
		api:     a,
		hub:     hub,
		deploys: deploys,
		admin: admin.NewHandler(admin.Deps{
			Config: cfg, Store: st, Audit: auditLog, Runtimes: runtimes, Version: version, Started: time.Now(),
		}),
		http: &http.Server{
			Addr:              cfg.Listen,
			Handler:           h,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			IdleTimeout:       120 * time.Second,
			// No WriteTimeout: WebSocket and log streaming connections are long-lived.
		},
	}
}

// Run serves until ctx is cancelled, then shuts down gracefully.
func (s *Server) Run(ctx context.Context) error {
	if err := s.deploys.Start(ctx); err != nil {
		return fmt.Errorf("start deploy runner: %w", err)
	}
	go s.housekeeping(ctx)
	s.serveAdmin(ctx)

	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", s.cfg.Listen, "data_dir", s.cfg.DataDir)
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
		close(errc)
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := s.http.Shutdown(shutdownCtx)
	s.hub.CloseAll()
	s.deploys.Shutdown(deployGrace)
	return err
}

// serveAdmin opens the socket the CLI uses. Deployments do not depend on it, so
// a failure is logged instead of stopping the service.
func (s *Server) serveAdmin(ctx context.Context) {
	path := s.cfg.AdminSocket
	if path == "off" {
		return
	}
	l, err := admin.Listen(path)
	if err != nil {
		slog.Warn("admin socket unavailable; the CLI needs sudo -u degoploy", "err", err)
		return
	}
	slog.Info("admin socket ready", "path", path)
	go func() {
		if err := admin.Serve(ctx, l, s.admin); err != nil {
			slog.Error("admin socket", "err", err)
		}
	}()
}

func (s *Server) housekeeping(ctx context.Context) {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := s.store.PurgeExpiredSessions(ctx); err != nil {
				slog.Error("purge sessions", "err", err)
			} else if n > 0 {
				slog.Debug("purged expired sessions", "count", n)
			}
			s.api.Sweep()
		}
	}
}

// EnsureDataDir creates the data directory with owner-only permissions.
func EnsureDataDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	return os.Chmod(dir, 0o700)
}
