// Package deploy queues and runs deployments.
//
// Each project has at most one deployment running. While it runs, newer requests
// for the same project coalesce: only the most recent one stays queued, older
// waiting ones are marked skipped. A global semaphore bounds how many projects
// deploy at once.
package deploy

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/degovan/degoploy/internal/project"
	"github.com/degovan/degoploy/internal/realtime"
	"github.com/degovan/degoploy/internal/store"
)

// Request describes one deployment to queue.
type Request struct {
	Project     *store.Project
	Rule        *store.TriggerRule // nil when no rule applies
	Trigger     string             // store.TriggerWebhook, TriggerManual, TriggerRedeploy
	TriggeredBy int64              // user id; 0 for webhooks
	Ref         string             // full ref: refs/heads/... or refs/tags/...
	CommitSHA   string             // empty = whatever Ref points to at run time
	Message     string
	Author      string
}

var (
	errCanceledByUser = errors.New("canceled by user")
	errShutdown       = errors.New("degoploy is shutting down")
	errTimeout        = errors.New("timed out")
)

// keepDeployments is how many finished deployments (and logs) are kept per project.
const keepDeployments = 100

type Service struct {
	store    *store.Store
	projects *project.Manager
	hub      *realtime.Hub
	sem      chan struct{}

	// OnUpdate is called after every deployment state change (for realtime events).
	OnUpdate func(*store.DeploymentView)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu      sync.Mutex
	closing bool
	active  map[int64]bool                    // project id → loop running
	pending map[int64]int64                   // project id → waiting deployment id
	jobs    map[int64]context.CancelCauseFunc // deployment id → cancel
}

func NewService(st *store.Store, projects *project.Manager, hub *realtime.Hub, maxParallel int) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		store: st, projects: projects, hub: hub, sem: make(chan struct{}, maxParallel),
		ctx: ctx, cancel: cancel,
		active: map[int64]bool{}, pending: map[int64]int64{}, jobs: map[int64]context.CancelCauseFunc{},
	}
}

// Start recovers state left by a previous process and resumes queued deployments.
func (s *Service) Start(ctx context.Context) error {
	failed, queued, err := s.store.RecoverDeployments(ctx)
	if err != nil {
		return err
	}
	if failed > 0 {
		slog.Warn("marked interrupted deployments as failed", "count", failed)
	}
	for _, d := range queued {
		s.submit(d)
	}
	return nil
}

// Enqueue records a queued deployment and schedules it.
func (s *Service) Enqueue(ctx context.Context, req Request) (*store.Deployment, error) {
	d := &store.Deployment{
		ProjectID:     req.Project.ID,
		Trigger:       req.Trigger,
		Ref:           req.Ref,
		CommitSHA:     req.CommitSHA,
		CommitMessage: truncate(strings.TrimSpace(req.Message), 2000),
		Author:        truncate(req.Author, 200),
		Status:        store.DeployQueued,
	}
	if req.Rule != nil {
		d.RuleID = &req.Rule.ID
	}
	if req.TriggeredBy != 0 {
		d.TriggeredBy = &req.TriggeredBy
	}
	if err := s.store.CreateDeployment(ctx, d); err != nil {
		return nil, err
	}
	s.notify(d.ID)
	s.submit(d)
	return d, nil
}

// Cancel stops a running deployment or drops a queued one. It reports false if
// the deployment had already finished.
func (s *Service) Cancel(ctx context.Context, id int64) (bool, error) {
	s.mu.Lock()
	if cancel, ok := s.jobs[id]; ok {
		s.mu.Unlock()
		cancel(errCanceledByUser)
		return true, nil
	}
	for pid, pend := range s.pending {
		if pend == id {
			delete(s.pending, pid)
		}
	}
	s.mu.Unlock()

	ok, err := s.store.CloseQueuedDeployment(ctx, id, store.DeployCanceled)
	if ok {
		s.notify(id)
	}
	return ok, err
}

// CancelProject cancels everything queued or running for a project (before deletion).
func (s *Service) CancelProject(ctx context.Context, projectID int64) {
	deps, err := s.store.ListDeployments(ctx, projectID, 0, 50)
	if err != nil {
		return
	}
	for _, d := range deps {
		if d.Status == store.DeployQueued || d.Status == store.DeployRunning {
			s.Cancel(ctx, d.ID)
		}
	}
}

// Shutdown lets running deployments finish for up to grace, then cancels them.
// Queued deployments stay queued and resume on the next start.
func (s *Service) Shutdown(grace time.Duration) {
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()

	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
		return
	case <-time.After(grace):
	}
	slog.Warn("canceling deployments still running at shutdown")
	s.mu.Lock()
	for _, cancel := range s.jobs {
		cancel(errShutdown)
	}
	s.mu.Unlock()
	s.cancel()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		slog.Error("deployments did not stop in time")
	}
}

func (s *Service) submit(d *store.Deployment) {
	pid := d.ProjectID
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return
	}
	if s.active[pid] {
		replaced := s.pending[pid]
		s.pending[pid] = d.ID
		s.mu.Unlock()
		if replaced != 0 {
			if ok, err := s.store.CloseQueuedDeployment(s.ctx, replaced, store.DeploySkipped); err != nil {
				slog.Error("skip superseded deployment", "id", replaced, "err", err)
			} else if ok {
				s.notify(replaced)
			}
		}
		return
	}
	s.active[pid] = true
	s.wg.Add(1)
	s.mu.Unlock()
	go s.loop(pid, d.ID)
}

// loop runs a project's deployments one after another.
func (s *Service) loop(pid, id int64) {
	defer s.wg.Done()
	for id != 0 {
		s.execute(id)
		s.mu.Lock()
		id = 0
		if !s.closing {
			id = s.pending[pid]
		}
		delete(s.pending, pid)
		if id == 0 {
			delete(s.active, pid)
		}
		s.mu.Unlock()
	}
}

func (s *Service) execute(id int64) {
	ctx, cancel := context.WithCancelCause(s.ctx)
	defer cancel(nil)
	s.mu.Lock()
	s.jobs[id] = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.jobs, id)
		s.mu.Unlock()
	}()

	// Wait for a global slot; the deployment stays "queued" meanwhile.
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		if context.Cause(ctx) == errCanceledByUser {
			if ok, _ := s.store.CloseQueuedDeployment(context.Background(), id, store.DeployCanceled); ok {
				s.notify(id)
			}
		}
		return
	}

	d, err := s.store.DeploymentByID(ctx, id)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			slog.Error("load deployment", "id", id, "err", err)
		}
		return
	}
	p, err := s.store.ProjectByID(ctx, d.ProjectID)
	if err != nil {
		slog.Error("load project for deployment", "id", id, "err", err)
		return
	}

	sink, err := newLogSink(s.projects.LogDir(p), id, p.ID, s.hub)
	if err != nil {
		slog.Error("open deployment log", "id", id, "err", err)
		s.store.StartDeployment(context.Background(), id, "", p.AutoPull)
		s.finish(id, store.DeployFailed, nil, "could not open the deployment log")
		return
	}
	started, err := s.store.StartDeployment(ctx, id, sink.path, p.AutoPull)
	if err != nil || !started {
		sink.close()
		os.Remove(sink.path)
		return
	}
	s.notify(id)

	status, exitCode, errLine := s.run(ctx, d, p, sink)
	sink.close()
	s.finish(id, status, exitCode, errLine)

	if paths, err := s.store.PruneDeployments(context.Background(), p.ID, keepDeployments); err == nil {
		for _, path := range paths {
			os.Remove(path)
		}
	}
}

func (s *Service) finish(id int64, status string, exitCode *int, errLine string) {
	if err := s.store.FinishDeployment(context.Background(), id, status, exitCode, errLine); err != nil {
		slog.Error("finish deployment", "id", id, "err", err)
	}
	s.notify(id)
}

func (s *Service) notify(id int64) {
	if s.OnUpdate == nil {
		return
	}
	v, err := s.store.DeploymentViewByID(context.Background(), id)
	if err != nil {
		return
	}
	s.OnUpdate(v)
}

// truncate cuts s to at most n bytes without splitting a UTF-8 sequence.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
