package deploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/degovan/degoploy/internal/release"
	"github.com/degovan/degoploy/internal/store"
)

// healthInterval is the pause between health check requests.
var healthInterval = 2 * time.Second

// healthRequestTimeout bounds one health check request. A request started
// before the check's deadline gets all of it, so a slow last answer still counts.
const healthRequestTimeout = 10 * time.Second

// healthClient does not follow redirects: a 3xx already means the app answers.
var healthClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// checkHealth requests the project's health URL until it answers 2xx or 3xx,
// starting requests for HealthTimeout seconds.
func (s *Service) checkHealth(ctx context.Context, j *job) error {
	p, w := j.p, j.w
	step(w, "Checking health: GET %s", p.HealthURL)
	start := time.Now()
	deadline := start.Add(time.Duration(p.HealthTimeout) * time.Second)
	last := ""
	for {
		status, err := healthRequest(ctx, p.HealthURL)
		took := time.Since(start).Round(100 * time.Millisecond)
		if err == nil && status >= 200 && status < 400 {
			note(w, "%s after %s", http.StatusText(status), took)
			return nil
		}
		result := ""
		if err != nil {
			result = "no answer: " + err.Error()
		} else {
			result = fmt.Sprintf("%d %s", status, http.StatusText(status))
		}
		// Only changes are logged: a minute of identical 502s says nothing new.
		if result != last {
			note(w, "%s (after %s)", result, took)
			last = result
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().Add(healthInterval).After(deadline) {
			return fmt.Errorf("health check failed: %s still gave %s after %ds", p.HealthURL, result, p.HealthTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(healthInterval):
		}
	}
}

func healthRequest(ctx context.Context, rawURL string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, healthRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "degoploy-health-check")
	resp, err := healthClient.Do(req)
	if err != nil {
		// The URL is in the step's header; the cause is enough here.
		if ue := (*url.Error)(nil); errors.As(err, &ue) {
			err = ue.Err
		}
		if errors.Is(err, context.DeadlineExceeded) {
			err = errors.New("timed out")
		}
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

// restore switches current back to the release that was live before j.release
// failed its health check, runs the after-activate commands there and returns
// cause with the rollback noted. j still names the failed release afterwards,
// so the deployment removes it.
func (s *Service) restore(ctx context.Context, j *job, prev int64, cause error) error {
	p, w := j.p, j.w
	base := p.WorkDir
	failed, failedDir := j.release, j.dir
	defer func() { j.release, j.dir = failed, failedDir }()

	step(w, "Rolling back to release %d", prev)
	if err := s.releaseOp(ctx, j, release.Op{Action: release.OpActivate, Base: base, ID: prev}); err != nil {
		note(w, "could not switch back: %v", err)
		return fmt.Errorf("%w; switching back to release %d failed, release %d stays live", cause, prev, failed)
	}
	j.release, j.dir = prev, release.Dir(base, prev)
	note(w, "%s → %s", release.Current(base), j.dir)
	if p.AfterActivate != "" {
		var err error
		if j.bins, j.runtimeEnv, err = s.setupRuntimes(ctx, p, w, j.dir); err == nil {
			step(w, "Running the after-activate commands")
			err = s.bash(ctx, j, release.Current(base), "after-activate", p.AfterActivate)
		}
		if err != nil {
			note(w, "release %d is live again, but the after-activate commands failed: %v", prev, err)
		}
	}
	return fmt.Errorf("%w; rolled back to release %d", cause, prev)
}

// rollsBack reports whether a failed health check after switching to j.release
// switches current back to prev.
func rollsBack(j *job, prev int64) bool {
	return j.d.Trigger != store.TriggerRollback && prev != 0 && prev != j.release && release.Exists(j.p.WorkDir, prev)
}
