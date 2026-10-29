package toolchain

import (
	"path/filepath"
	"slices"
	"time"
)

// Report describes one runtime, for the Runtimes page and the CLI.
type Report struct {
	Name     string `json:"name"`
	Title    string `json:"title"`
	Platform string `json:"platform"`
	Mirror   string `json:"mirror"`
	// Verified: every download is checked against a checksum the source
	// publishes. PHP builds have one only when recent; all are run to check them.
	Verified bool `json:"verified"`
	// Enabled: off, no project uses the runtime.
	Enabled   bool            `json:"enabled"`
	Default   string          `json:"default"` // spec, "" when none is set
	Installed []ReportVersion `json:"installed"`
	TotalSize int64           `json:"total_size"`
	// Installing lists downloads in progress, from the page, the CLI or a deployment.
	Installing []ReportProgress `json:"installing"`
}

type ReportProgress struct {
	Version string `json:"version"`
	Phase   string `json:"phase"` // "downloading" or "unpacking"
	Done    int64  `json:"done"`
	Total   int64  `json:"total"` // 0 when unknown
}

type ReportVersion struct {
	Version     string    `json:"version"`
	LTS         string    `json:"lts"`
	Size        int64     `json:"size"`
	InstalledAt time.Time `json:"installed_at"`
	// System versions come with the operating system, at Path.
	System bool   `json:"system"`
	Path   string `json:"path"`
	// Default: the default spec picks this version.
	Default bool `json:"default"`
	// UsedBy lists the projects whose latest deployment ran with this version.
	UsedBy []string `json:"used_by"`
	// Pinned lists the projects that ask for exactly this version.
	Pinned []string `json:"pinned"`
}

// ProjectUse is one project's setting for this runtime.
type ProjectUse struct {
	Project string
	Spec    string // Auto, Off or a version spec
	Used    string // the version its latest deployment ran with
}

func (m *Manager) Report(uses []ProjectUse, def string, enabled bool) (Report, error) {
	have, err := m.Installed()
	if err != nil {
		return Report{}, err
	}
	rep := Report{
		Name: m.Name(), Title: m.Title(), Platform: m.Platform(), Mirror: m.mirror, Verified: verified(m.kind),
		Enabled: enabled, Default: def, Installed: []ReportVersion{}, Installing: []ReportProgress{},
	}
	for _, p := range m.Installing() {
		rep.Installing = append(rep.Installing, ReportProgress{Version: m.Label(p.Version), Phase: p.Phase, Done: p.Done, Total: p.Total})
	}
	defVersion, defOK := m.resolveInstalled(have, def)
	for _, in := range have {
		label := m.Label(in.Version)
		rv := ReportVersion{
			Version: label, LTS: in.LTS, Size: in.Size, InstalledAt: in.InstalledAt, System: in.System, Path: in.Path,
			Default: defOK && defVersion == in.Version, UsedBy: []string{}, Pinned: []string{},
		}
		for _, u := range uses {
			if u.Used == label {
				rv.UsedBy = append(rv.UsedBy, u.Project)
			}
			if v, ok := pinned(u.Spec); ok && v == in.Version {
				rv.Pinned = append(rv.Pinned, u.Project)
			}
		}
		rep.TotalSize += in.Size
		rep.Installed = append(rep.Installed, rv)
	}
	return rep, nil
}

func verified(k Kind) bool {
	_, unverified := k.(*PHP)
	return !unverified
}

// resolveInstalled picks the installed version a spec means, without the network.
func (m *Manager) resolveInstalled(have []Installed, spec string) (Version, bool) {
	if spec == "" {
		return Version{}, false
	}
	s, err := ParseSpec(spec)
	if err != nil {
		return Version{}, false
	}
	for _, sys := range []bool{true, false} {
		for _, in := range have {
			if in.System != sys {
				continue
			}
			switch {
			case s.Alias == "latest",
				s.Alias == "lts" && in.LTS != "" && (s.Codename == "" || equalFold(in.LTS, s.Codename)),
				s.Alias == "" && s.Range.Match(in.Version):
				return in.Version, true
			}
		}
	}
	return Version{}, false
}

// Keep reports the versions that projects ask for exactly, which Update keeps.
func Keep(uses []ProjectUse) func(Version) bool {
	var keep []Version
	for _, u := range uses {
		if v, ok := pinned(u.Spec); ok {
			keep = append(keep, v)
		}
	}
	return func(v Version) bool { return slices.Contains(keep, v) }
}

func pinned(spec string) (Version, bool) {
	if spec == Auto || spec == Off || spec == "" {
		return Version{}, false
	}
	s, err := ParseSpec(spec)
	if err != nil || s.Alias != "" {
		return Version{}, false
	}
	return s.Range.Exact()
}

// Runtimes names the runtimes NewSet sets up, in display order.
var Runtimes = []string{"node", "php"}

// Set is the runtimes degoploy offers, in display order.
type Set struct{ list []*Manager }

// Options configures NewSet; empty fields take the defaults.
type Options struct {
	NodeMirror  string
	PHPMirror   string
	ComposerURL string
	// NoSystem skips the system's own versions, for tests.
	NoSystem bool
}

// NewSet sets up Node.js and PHP under dataDir.
func NewSet(dataDir string, o Options) *Set {
	node := NodeJS{SystemPaths: []string{"/usr/bin/node"}}
	php := &PHP{SystemGlobs: []string{"/usr/bin/php*", "/usr/local/bin/php*"}, ComposerURL: o.ComposerURL}
	if o.NoSystem {
		node.SystemPaths, php.SystemGlobs = nil, nil
	}
	return &Set{list: []*Manager{
		New(node, filepath.Join(dataDir, "node"), o.NodeMirror),
		New(php, filepath.Join(dataDir, "php"), o.PHPMirror),
	}}
}

// NewSetOf builds a Set from managers, for tests.
func NewSetOf(ms ...*Manager) *Set { return &Set{list: ms} }

func (s *Set) All() []*Manager { return s.list }

// Get returns the runtime called name, or nil.
func (s *Set) Get(name string) *Manager {
	for _, m := range s.list {
		if m.Name() == name {
			return m
		}
	}
	return nil
}

// Names lists the runtimes' names, in display order.
func (s *Set) Names() []string {
	out := make([]string, len(s.list))
	for i, m := range s.list {
		out[i] = m.Name()
	}
	return out
}
