package toolchain

import (
	"context"

	"github.com/degovan/degoploy/internal/store"
)

// Enabled reports whether a runtime is switched on for the server.
func Enabled(ctx context.Context, st *store.Store, name string) (bool, error) {
	v, err := st.Setting(ctx, store.SettingRuntimeEnabled(name))
	return v != "false", err
}

// Uses lists every project's setting for this runtime.
func (m *Manager) Uses(ctx context.Context, st *store.Store) ([]ProjectUse, error) {
	projects, err := st.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	all, err := st.AllProjectRuntimes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectUse, 0, len(projects))
	for _, p := range projects {
		r := all[p.ID][m.Name()]
		if r.Spec == "" {
			r.Spec = Auto
		}
		out = append(out, ProjectUse{Project: p.Name, Spec: r.Spec, Used: r.Used})
	}
	return out, nil
}

// Status is the Report with the projects' and the server's settings.
func (m *Manager) Status(ctx context.Context, st *store.Store) (Report, error) {
	uses, err := m.Uses(ctx, st)
	if err != nil {
		return Report{}, err
	}
	def, err := st.Setting(ctx, store.SettingRuntimeDefault(m.Name()))
	if err != nil {
		return Report{}, err
	}
	on, err := Enabled(ctx, st, m.Name())
	if err != nil {
		return Report{}, err
	}
	return m.Report(uses, def, on)
}

// Lines lists the newest release of each release line (Node.js majors, PHP
// minors), newest first.
func (m *Manager) Lines(ctx context.Context) ([]Release, error) {
	rel, err := m.Releases(ctx)
	if err != nil {
		return nil, err
	}
	var out []Release
	seen := map[Version]bool{}
	for _, r := range rel {
		start := m.line(r.Version)[0][0].v
		if !seen[start] {
			seen[start] = true
			out = append(out, r)
		}
	}
	return out, nil
}
