package node

import (
	"slices"
	"time"

	"github.com/degovan/degoploy/internal/store"
)

// Report describes the installed versions, for the Node.js page and the CLI.
type Report struct {
	Platform  string          `json:"platform"`
	Mirror    string          `json:"mirror"`
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
	// Default: the default spec picks this version.
	Default bool `json:"default"`
	// UsedBy lists the projects whose latest deployment ran with this version.
	UsedBy []string `json:"used_by"`
	// Pinned lists the projects that ask for exactly this version.
	Pinned []string `json:"pinned"`
}

func (m *Manager) Report(projects []*store.Project, def string) (Report, error) {
	have, err := m.Installed()
	if err != nil {
		return Report{}, err
	}
	rep := Report{Platform: Platform(), Mirror: m.mirror, Default: def, Installed: []ReportVersion{}, Installing: []ReportProgress{}}
	for _, p := range m.Installing() {
		rep.Installing = append(rep.Installing, ReportProgress{Version: p.Version.String(), Phase: p.Phase, Done: p.Done, Total: p.Total})
	}
	defVersion, defOK := m.resolveInstalled(have, def)
	for _, in := range have {
		rv := ReportVersion{
			Version: in.Version.String(), LTS: in.LTS, Size: in.Size, InstalledAt: in.InstalledAt,
			Default: defOK && defVersion == in.Version, UsedBy: []string{}, Pinned: []string{},
		}
		for _, p := range projects {
			if p.NodeUsed == rv.Version {
				rv.UsedBy = append(rv.UsedBy, p.Name)
			}
			if v, ok := pinnedVersion(p); ok && v == in.Version {
				rv.Pinned = append(rv.Pinned, p.Name)
			}
		}
		rep.TotalSize += in.Size
		rep.Installed = append(rep.Installed, rv)
	}
	return rep, nil
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
	for _, in := range have {
		switch {
		case s.Alias == "latest",
			s.Alias == "lts" && in.LTS != "" && (s.Codename == "" || equalFold(in.LTS, s.Codename)),
			s.Alias == "" && s.Range.Match(in.Version):
			return in.Version, true
		}
	}
	return Version{}, false
}

// Keep reports the versions that projects ask for exactly, which Update keeps.
func Keep(projects []*store.Project) func(Version) bool {
	var pinned []Version
	for _, p := range projects {
		if v, ok := pinnedVersion(p); ok {
			pinned = append(pinned, v)
		}
	}
	return func(v Version) bool { return slices.Contains(pinned, v) }
}

func pinnedVersion(p *store.Project) (Version, bool) {
	if p.NodeVersion == Auto || p.NodeVersion == None {
		return Version{}, false
	}
	s, err := ParseSpec(p.NodeVersion)
	if err != nil || s.Alias != "" {
		return Version{}, false
	}
	return s.Range.Exact()
}
