package toolchain

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// indexTTL is how long a list of releases or system versions is reused.
const (
	indexTTL  = time.Hour
	systemTTL = time.Minute
)

// Installed is a version this server has.
type Installed struct {
	Version     Version
	LTS         string
	Size        int64 // 0 for system versions
	InstalledAt time.Time
	// System versions come from the operating system; degoploy uses them but
	// never removes or updates them.
	System bool
	Path   string // system versions: the executable
}

// meta is kept next to each downloaded version.
type meta struct {
	LTS         string    `json:"lts"`
	Size        int64     `json:"size"`
	InstalledAt time.Time `json:"installed_at"`
}

const metaFile = ".degoploy.json"

// Progress is an install that is running, for showing how far it is.
type Progress struct {
	Version Version
	Phase   string // "downloading" or "unpacking"
	Done    int64  // bytes downloaded
	Total   int64  // bytes to download; 0 when the source does not say
}

// Manager installs and picks the versions of one runtime.
type Manager struct {
	kind   Kind
	dir    string
	mirror string
	client *http.Client

	mu       sync.Mutex
	index    []Release
	indexAt  time.Time
	system   []SystemVersion
	systemAt time.Time
	locks    map[Version]*sync.Mutex
	active   map[Version]*Progress
	cancels  map[Version]context.CancelCauseFunc
}

func New(kind Kind, dir, mirror string) *Manager {
	return &Manager{
		kind:    kind,
		dir:     dir,
		mirror:  strings.TrimRight(cmp.Or(mirror, kind.DefaultMirror()), "/"),
		client:  &http.Client{Timeout: 10 * time.Minute},
		locks:   map[Version]*sync.Mutex{},
		active:  map[Version]*Progress{},
		cancels: map[Version]context.CancelCauseFunc{},
	}
}

func (m *Manager) Kind() Kind       { return m.kind }
func (m *Manager) Name() string     { return m.kind.Name() }
func (m *Manager) Title() string    { return m.kind.Title() }
func (m *Manager) Dir() string      { return m.dir }
func (m *Manager) Mirror() string   { return m.mirror }
func (m *Manager) Env() []string    { return m.kind.Env(m) }
func (m *Manager) Platform() string { return platform(m.kind) }

// Label is how a version is written for this runtime: "v22.12.0" for
// Node.js, "8.4.23" for PHP.
func (m *Manager) Label(v Version) string {
	if p, ok := m.kind.(interface{ Prefix() string }); ok {
		return p.Prefix() + strings.TrimPrefix(v.String(), "v")
	}
	return strings.TrimPrefix(v.String(), "v")
}

func platform(k Kind) string {
	if p, ok := k.(interface{ Platform() string }); ok {
		return p.Platform()
	}
	return "linux-" + goarch()
}

// BinDir holds the executables of an installed version.
func (m *Manager) BinDir(in Installed) string {
	if in.System {
		return filepath.Join(m.dir, "system", m.Label(in.Version), "bin")
	}
	return filepath.Join(m.dir, m.Label(in.Version), "bin")
}

// Installed lists the versions this server has, newest first. Where the
// system and degoploy have the same version, the system one is listed.
func (m *Manager) Installed() ([]Installed, error) {
	var out []Installed
	for _, s := range m.systemVersions() {
		out = append(out, Installed{Version: s.Version, System: true, Path: s.Path})
	}
	entries, err := os.ReadDir(m.dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		v, err := ParseVersion(e.Name())
		if err != nil || !e.IsDir() || e.Name() != m.Label(v) {
			continue
		}
		if slices.ContainsFunc(out, func(in Installed) bool { return in.Version == v }) {
			continue
		}
		in := Installed{Version: v}
		if _, err := os.Stat(filepath.Join(m.BinDir(in), m.kind.Name())); err != nil {
			continue
		}
		var md meta
		if b, err := os.ReadFile(filepath.Join(m.dir, e.Name(), metaFile)); err == nil && json.Unmarshal(b, &md) == nil {
			in.LTS, in.Size, in.InstalledAt = md.LTS, md.Size, md.InstalledAt
		}
		out = append(out, in)
	}
	slices.SortStableFunc(out, func(a, b Installed) int { return b.Version.Compare(a.Version) })
	return out, nil
}

func (m *Manager) systemVersions() []SystemVersion {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.system == nil || time.Since(m.systemAt) > systemTTL {
		m.system, m.systemAt = m.kind.System(), time.Now()
		if m.system == nil {
			m.system = []SystemVersion{}
		}
	}
	return m.system
}

// Lookup returns an installed version.
func (m *Manager) Lookup(v Version) (Installed, bool) { return m.installed(v) }

func (m *Manager) installed(v Version) (Installed, bool) {
	have, _ := m.Installed()
	i := slices.IndexFunc(have, func(in Installed) bool { return in.Version == v })
	if i < 0 {
		return Installed{}, false
	}
	return have[i], true
}

// Releases returns what can be downloaded, newest first, fetched at most
// once an hour.
func (m *Manager) Releases(ctx context.Context) ([]Release, error) {
	m.mu.Lock()
	if m.index != nil && time.Since(m.indexAt) < indexTTL {
		defer m.mu.Unlock()
		return m.index, nil
	}
	m.mu.Unlock()
	rel, err := m.kind.Releases(ctx, m)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(rel, func(a, b Release) int { return b.Version.Compare(a.Version) })
	m.mu.Lock()
	m.index, m.indexAt = rel, time.Now()
	m.mu.Unlock()
	return rel, nil
}

// Find picks the installed version a spec means, system versions first, and
// reports false when none fits. It never downloads: versions are installed on
// the Runtimes page or with the CLI, so deployments never wait for, or race
// each other over, a download.
func (m *Manager) Find(spec string) (Version, bool, error) {
	if _, err := ParseSpec(spec); err != nil {
		return Version{}, false, err
	}
	have, err := m.Installed()
	if err != nil {
		return Version{}, false, err
	}
	v, ok := m.resolveInstalled(have, spec)
	return v, ok, nil
}

// Latest is the newest release a spec matches, whatever is installed: what
// installing a spec fetches.
func (m *Manager) Latest(ctx context.Context, spec string) (Version, error) {
	s, err := ParseSpec(spec)
	if err != nil {
		return Version{}, err
	}
	rel, err := m.Releases(ctx)
	if err != nil {
		return Version{}, err
	}
	if s.Alias != "" {
		if i := slices.IndexFunc(rel, s.alias); i >= 0 {
			return rel[i].Version, nil
		}
	} else if best, ok := newest(rel, s.Range); ok {
		return best.Version, nil
	}
	return Version{}, fmt.Errorf("no %s release for %s matches %q", m.Title(), m.Platform(), spec)
}

// newest picks the newest release in r, preferring LTS ones.
func newest(rel []Release, r Range) (Release, bool) {
	var any *Release
	for i := range rel {
		if !r.Match(rel[i].Version) {
			continue
		}
		if rel[i].LTS != "" {
			return rel[i], true
		}
		if any == nil {
			any = &rel[i]
		}
	}
	if any != nil {
		return *any, true
	}
	return Release{}, false
}

// line is the release line of v: its major version, or what the kind says.
func (m *Manager) line(v Version) Range {
	if l, ok := m.kind.(interface{ Line(Version) Range }); ok {
		return l.Line(v)
	}
	return majorRange(v.Major())
}

func majorRange(major int) Range {
	return Range{{{">=", Version{major, 0, 0}}, {"<", Version{major + 1, 0, 0}}}}
}

// ErrNotInstalled is returned by Use when no installed version fits the spec.
var ErrNotInstalled = errors.New("not installed")

// Use picks the installed version spec means and readies it for a deployment.
// It returns the version and the directory to put on PATH, or ErrNotInstalled.
func (m *Manager) Use(ctx context.Context, spec string, w io.Writer) (Version, string, error) {
	v, ok, err := m.Find(spec)
	if err != nil {
		return v, "", err
	}
	if !ok {
		return v, "", ErrNotInstalled
	}
	in, found := m.installed(v)
	if !found {
		return v, "", fmt.Errorf("%s %s disappeared", m.Title(), m.Label(v))
	}
	bin := m.BinDir(in)
	if in.System {
		// A directory of its own on PATH, with the system executable linked in.
		if err := os.MkdirAll(bin, 0o755); err != nil {
			return v, "", err
		}
		link := filepath.Join(bin, m.Name())
		if cur, err := os.Readlink(link); err != nil || cur != in.Path {
			os.Remove(link)
			if err := os.Symlink(in.Path, link); err != nil {
				return v, "", err
			}
		}
	}
	if w == nil {
		w = io.Discard
	}
	return v, bin, m.kind.Prepare(ctx, m, bin, v, w)
}

// Installing lists the installs that are running, whoever started them.
func (m *Manager) Installing() []Progress {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Progress, 0, len(m.active))
	for _, p := range m.active {
		out = append(out, *p)
	}
	slices.SortFunc(out, func(a, b Progress) int { return b.Version.Compare(a.Version) })
	return out
}

func (m *Manager) setProgress(v Version, f func(*Progress)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.active[v]
	if p == nil {
		p = &Progress{Version: v}
		m.active[v] = p
	}
	f(p)
}

// Install downloads and unpacks v unless it is there already. Progress goes to
// w, one line per step and every 10% of the download, and to Installing.
// Concurrent installs of one version wait for each other.
func (m *Manager) Install(ctx context.Context, v Version, w io.Writer) (err error) {
	m.mu.Lock()
	lock := m.locks[v]
	if lock == nil {
		lock = &sync.Mutex{}
		m.locks[v] = lock
	}
	m.mu.Unlock()
	lock.Lock()
	defer lock.Unlock()
	if _, ok := m.installed(v); ok {
		return nil
	}
	if w == nil {
		w = io.Discard
	}
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return err
	}
	m.setProgress(v, func(p *Progress) { p.Phase = "downloading" })
	ctx, cancel := context.WithCancelCause(ctx)
	m.mu.Lock()
	m.cancels[v] = cancel
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.active, v)
		delete(m.cancels, v)
		m.mu.Unlock()
		if err != nil && errors.Is(context.Cause(ctx), ErrCanceled) {
			err = ErrCanceled
		}
		cancel(nil)
	}()

	url, sum, err := m.kind.Archive(ctx, m, v)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(m.dir, ".install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	archive := filepath.Join(tmp, filepath.Base(url))
	name := filepath.Base(url)
	logged := int64(0) // tenths of the download reported in w
	size, err := m.download(ctx, url, archive, sum, func(total int64) {
		fmt.Fprintf(w, "downloading %s (%s) from %s\n", name, humanSize(total), m.mirror)
	}, func(done, total int64) {
		m.setProgress(v, func(p *Progress) { p.Done, p.Total = done, total })
		switch {
		case total > 0 && done*10/total > logged:
			logged = done * 10 / total
			fmt.Fprintf(w, "%s\n", ProgressLabel(done, total))
		case total <= 0 && done/(10<<20) > logged:
			logged = done / (10 << 20)
			fmt.Fprintf(w, "%s\n", ProgressLabel(done, total))
		}
	})
	if err != nil {
		return err
	}
	m.setProgress(v, func(p *Progress) { p.Phase = "unpacking" })
	if sum != "" {
		fmt.Fprintf(w, "checksum verified, unpacking %s\n", humanSize(size))
	} else {
		fmt.Fprintf(w, "unpacking %s (the source publishes no checksum for this build)\n", humanSize(size))
	}

	root := filepath.Join(tmp, "root")
	if err := extract(archive, root, m.kind.StripTop()); err != nil {
		return fmt.Errorf("unpack %s: %w", name, err)
	}
	os.Remove(archive)
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if err := m.kind.Finish(ctx, m, root, v, w); err != nil {
		return err
	}
	// The LTS name is kept with the version: Find matches "lts" specs offline.
	lts := ""
	if rel, err := m.Releases(ctx); err == nil {
		if i := slices.IndexFunc(rel, func(r Release) bool { return r.Version == v }); i >= 0 {
			lts = rel[i].LTS
		}
	}
	b, _ := json.Marshal(meta{LTS: lts, Size: dirSize(root), InstalledAt: time.Now().UTC()})
	if err := os.WriteFile(filepath.Join(root, metaFile), b, 0o644); err != nil {
		return err
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	return os.Rename(root, filepath.Join(m.dir, m.Label(v)))
}

// Cancel stops a running install of v, which then returns ErrCanceled and
// leaves nothing behind. It reports whether one was running.
func (m *Manager) Cancel(v Version) bool {
	m.mu.Lock()
	cancel := m.cancels[v]
	m.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel(ErrCanceled)
	return true
}

// Remove deletes a downloaded version. System versions stay.
func (m *Manager) Remove(v Version) error {
	in, ok := m.installed(v)
	switch {
	case !ok:
		return fmt.Errorf("%s %s is not installed", m.Title(), m.Label(v))
	case in.System:
		return fmt.Errorf("%s %s comes with the system (%s); remove it with the system's package manager", m.Title(), m.Label(v), in.Path)
	}
	// Rename first, so a deployment never sees half a version.
	trash, err := os.MkdirTemp(m.dir, ".remove-")
	if err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(m.dir, m.Label(v)), filepath.Join(trash, "v")); err != nil {
		os.RemoveAll(trash)
		return err
	}
	return os.RemoveAll(trash)
}

// Change is one version replaced by Update.
type Change struct {
	From, To Version
	Removed  bool // From was removed after To was installed
}

// Update installs the newest release of every downloaded major line, then
// removes the older versions of that line unless keep(version) says a project
// asks for exactly that one. System versions are left to the system.
func (m *Manager) Update(ctx context.Context, keep func(Version) bool, w io.Writer) ([]Change, error) {
	all, err := m.Installed()
	if err != nil {
		return nil, err
	}
	var have []Installed
	for _, in := range all {
		if !in.System {
			have = append(have, in)
		}
	}
	if len(have) == 0 {
		return nil, nil
	}
	rel, err := m.Releases(ctx)
	if err != nil {
		return nil, err
	}
	var changes []Change
	seen := map[Version]bool{}
	for _, in := range have {
		line := m.line(in.Version)[0][0].v
		if seen[line] {
			continue
		}
		seen[line] = true
		r := m.line(in.Version)
		latest, ok := newest(rel, r)
		if !ok || latest.Version.Compare(in.Version) <= 0 {
			continue
		}
		if err := m.Install(ctx, latest.Version, w); err != nil {
			return changes, err
		}
		for _, old := range have {
			if !r.Match(old.Version) || old.Version == latest.Version {
				continue
			}
			c := Change{From: old.Version, To: latest.Version}
			if keep == nil || !keep(old.Version) {
				if err := m.Remove(old.Version); err != nil {
					return changes, err
				}
				c.Removed = true
			}
			changes = append(changes, c)
		}
	}
	return changes, nil
}

func (m *Manager) get(ctx context.Context, url string) ([]byte, error) {
	resp, err := m.open(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

func (m *Manager) open(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	return resp, nil
}

// download saves url to path and, when want is set, checks its SHA256.
func (m *Manager) download(ctx context.Context, url, path, want string, started func(total int64), progress func(done, total int64)) (int64, error) {
	resp, err := m.open(ctx, url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	started(resp.ContentLength)
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h, &counter{total: resp.ContentLength, f: progress}), resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return n, fmt.Errorf("download %s: %w", url, err)
	}
	if want != "" && hex.EncodeToString(h.Sum(nil)) != want {
		return n, fmt.Errorf("checksum mismatch for %s; the download is damaged or was altered", url)
	}
	return n, nil
}
