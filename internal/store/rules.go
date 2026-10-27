package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type TriggerRule struct {
	ID           int64
	ProjectID    int64
	Name         string
	Enabled      bool
	Position     int
	Event        string // push | tag
	RefPattern   string // comma-separated globs
	PathsInclude []string
	PathsExclude []string
	SkipKeyword  string
	ExtraEnv     map[string]string
}

const ruleCols = `id, project_id, name, enabled, position, event, ref_pattern,
	paths_include, paths_exclude, skip_keyword, extra_env`

func scanRule(row interface{ Scan(...any) error }) (*TriggerRule, error) {
	var r TriggerRule
	var inc, exc, env string
	err := row.Scan(&r.ID, &r.ProjectID, &r.Name, &r.Enabled, &r.Position, &r.Event, &r.RefPattern,
		&inc, &exc, &r.SkipKeyword, &env)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(inc), &r.PathsInclude); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(exc), &r.PathsExclude); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(env), &r.ExtraEnv); err != nil {
		return nil, err
	}
	return &r, nil
}

func ruleJSON(r *TriggerRule) (inc, exc, env string) {
	if r.PathsInclude == nil {
		r.PathsInclude = []string{}
	}
	if r.PathsExclude == nil {
		r.PathsExclude = []string{}
	}
	if r.ExtraEnv == nil {
		r.ExtraEnv = map[string]string{}
	}
	a, _ := json.Marshal(r.PathsInclude)
	b, _ := json.Marshal(r.PathsExclude)
	c, _ := json.Marshal(r.ExtraEnv)
	return string(a), string(b), string(c)
}

// CreateRule appends r after the project's existing rules.
func (s *Store) CreateRule(ctx context.Context, r *TriggerRule) error {
	inc, exc, env := ruleJSON(r)
	return s.tx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), -1) + 1 FROM trigger_rules WHERE project_id = ?`,
			r.ProjectID).Scan(&r.Position); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO trigger_rules
			(project_id, name, enabled, position, event, ref_pattern, paths_include, paths_exclude, skip_keyword, extra_env)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.ProjectID, r.Name, r.Enabled, r.Position, r.Event, r.RefPattern, inc, exc, r.SkipKeyword, env)
		if err != nil {
			return err
		}
		r.ID, err = res.LastInsertId()
		return err
	})
}

func (s *Store) UpdateRule(ctx context.Context, r *TriggerRule) error {
	inc, exc, env := ruleJSON(r)
	res, err := s.db.ExecContext(ctx, `UPDATE trigger_rules SET
		name = ?, enabled = ?, event = ?, ref_pattern = ?, paths_include = ?, paths_exclude = ?,
		skip_keyword = ?, extra_env = ?
		WHERE id = ? AND project_id = ?`,
		r.Name, r.Enabled, r.Event, r.RefPattern, inc, exc, r.SkipKeyword, env, r.ID, r.ProjectID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteRule(ctx context.Context, projectID, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM trigger_rules WHERE id = ? AND project_id = ?`, id, projectID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RuleByID(ctx context.Context, projectID, id int64) (*TriggerRule, error) {
	return scanRule(s.db.QueryRowContext(ctx, `SELECT `+ruleCols+` FROM trigger_rules WHERE id = ? AND project_id = ?`,
		id, projectID))
}

// ListRules returns a project's rules in evaluation order.
func (s *Store) ListRules(ctx context.Context, projectID int64) ([]*TriggerRule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+ruleCols+` FROM trigger_rules WHERE project_id = ?
		ORDER BY position, id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*TriggerRule{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReorderRules sets positions to follow ids. ids must be exactly the project's rule ids.
func (s *Store) ReorderRules(ctx context.Context, projectID int64, ids []int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM trigger_rules WHERE project_id = ?`, projectID).Scan(&n); err != nil {
			return err
		}
		if n != len(ids) {
			return ErrNotFound
		}
		for i, id := range ids {
			res, err := tx.ExecContext(ctx, `UPDATE trigger_rules SET position = ? WHERE id = ? AND project_id = ?`, i, id, projectID)
			if err != nil {
				return err
			}
			if k, _ := res.RowsAffected(); k == 0 {
				return ErrNotFound
			}
		}
		return nil
	})
}
