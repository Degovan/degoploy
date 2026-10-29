package store

import "context"

// ProjectRuntime is a project's setting for one runtime.
type ProjectRuntime struct {
	Spec string // "auto", "off" or a version spec
	Used string // version the latest deployment ran with
}

// ProjectRuntimes returns a project's runtime settings by runtime name.
// Runtimes without a row are on "auto".
func (s *Store) ProjectRuntimes(ctx context.Context, projectID int64) (map[string]ProjectRuntime, error) {
	all, err := s.projectRuntimes(ctx, `WHERE project_id = ?`, projectID)
	return all[projectID], err
}

// AllProjectRuntimes returns every project's runtime settings, by project id.
func (s *Store) AllProjectRuntimes(ctx context.Context) (map[int64]map[string]ProjectRuntime, error) {
	return s.projectRuntimes(ctx, "")
}

func (s *Store) projectRuntimes(ctx context.Context, where string, args ...any) (map[int64]map[string]ProjectRuntime, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT project_id, runtime, spec, used FROM project_runtimes `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]map[string]ProjectRuntime{}
	for rows.Next() {
		var id int64
		var name string
		var r ProjectRuntime
		if err := rows.Scan(&id, &name, &r.Spec, &r.Used); err != nil {
			return nil, err
		}
		if out[id] == nil {
			out[id] = map[string]ProjectRuntime{}
		}
		out[id][name] = r
	}
	return out, rows.Err()
}

// SetProjectRuntime stores a project's setting for a runtime, keeping the
// version it last used.
func (s *Store) SetProjectRuntime(ctx context.Context, projectID int64, runtime, spec string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO project_runtimes (project_id, runtime, spec) VALUES (?, ?, ?)
		ON CONFLICT (project_id, runtime) DO UPDATE SET spec = excluded.spec`, projectID, runtime, spec)
	return err
}

// SetProjectRuntimeUsed records the version a deployment ran with.
func (s *Store) SetProjectRuntimeUsed(ctx context.Context, projectID int64, runtime, used string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO project_runtimes (project_id, runtime, spec, used) VALUES (?, ?, 'auto', ?)
		ON CONFLICT (project_id, runtime) DO UPDATE SET used = excluded.used`, projectID, runtime, used)
	return err
}
