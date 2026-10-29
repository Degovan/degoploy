// Package node installs Node.js releases for deploy scripts and picks the one
// a project asks for, the way nvm does but without a shell to source: it
// downloads the official build from nodejs.org, checks it against the
// release's SHASUMS256.txt, and unpacks it under <data_dir>/node/<version>.
// The runner puts that version's bin directory first on PATH.
package node

import (
	"archive/tar"
	"cmp"
	"compress/gzip"
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
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

// DefaultMirror is where releases are downloaded from unless the config
// names a mirror with the same layout.
const DefaultMirror = "https://nodejs.org/dist"

// indexTTL is how long the list of releases is reused.
const indexTTL = time.Hour

// Release is one entry of the mirror's index.json.
type Release struct {
	Version Version
	LTS     string // codename, such as "Jod"; empty for current releases
	Date    string
}

// Installed is a version unpacked on this server.
type Installed struct {
	Version     Version
	LTS         string
	Size        int64
	InstalledAt time.Time
}

// meta is kept next to each installed version.
type meta struct {
	LTS         string    `json:"lts"`
	Size        int64     `json:"size"`
	InstalledAt time.Time `json:"installed_at"`
}

const metaFile = ".degoploy.json"

type Manager struct {
	dir    string
	mirror string
	client *http.Client

	mu      sync.Mutex
	index   []Release
	indexAt time.Time
	locks   map[Version]*sync.Mutex
	active  map[Version]*Progress
	cancels map[Version]context.CancelCauseFunc
}

// ErrCanceled is returned by an install that Cancel stopped.
var ErrCanceled = errors.New("canceled")

// Progress is an install that is running, for showing how far it is.
type Progress struct {
	Version Version
	Phase   string // "downloading" or "unpacking"
	Done    int64  // bytes downloaded
	Total   int64  // bytes to download; 0 when the mirror does not say
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

func New(dir, mirror string) *Manager {
	return &Manager{
		dir:     dir,
		mirror:  strings.TrimRight(cmp.Or(mirror, DefaultMirror), "/"),
		client:  &http.Client{Timeout: 10 * time.Minute},
		locks:   map[Version]*sync.Mutex{},
		active:  map[Version]*Progress{},
		cancels: map[Version]context.CancelCauseFunc{},
	}
}

func (m *Manager) Dir() string    { return m.dir }
func (m *Manager) Mirror() string { return m.mirror }

// Platform is the build name for this machine, such as "linux-x64".
func Platform() string {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x64"
	case "386":
		arch = "x86"
	}
	return runtime.GOOS + "-" + arch
}

// BinDir holds node, npm, npx and the corepack shims of an installed version.
func (m *Manager) BinDir(v Version) string { return filepath.Join(m.dir, v.String(), "bin") }

// Installed lists the unpacked versions, newest first.
func (m *Manager) Installed() ([]Installed, error) {
	entries, err := os.ReadDir(m.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Installed
	for _, e := range entries {
		v, err := ParseVersion(e.Name())
		if err != nil || !e.IsDir() || e.Name() != v.String() {
			continue
		}
		if _, err := os.Stat(filepath.Join(m.BinDir(v), "node")); err != nil {
			continue
		}
		in := Installed{Version: v}
		var md meta
		if b, err := os.ReadFile(filepath.Join(m.dir, e.Name(), metaFile)); err == nil && json.Unmarshal(b, &md) == nil {
			in.LTS, in.Size, in.InstalledAt = md.LTS, md.Size, md.InstalledAt
		}
		out = append(out, in)
	}
	slices.SortFunc(out, func(a, b Installed) int { return b.Version.Compare(a.Version) })
	return out, nil
}

func (m *Manager) isInstalled(v Version) bool {
	_, err := os.Stat(filepath.Join(m.BinDir(v), "node"))
	return err == nil
}

// Releases returns the releases built for this platform, newest first. The
// list is fetched from the mirror at most once an hour.
func (m *Manager) Releases(ctx context.Context) ([]Release, error) {
	m.mu.Lock()
	if m.index != nil && time.Since(m.indexAt) < indexTTL {
		defer m.mu.Unlock()
		return m.index, nil
	}
	m.mu.Unlock()

	body, err := m.get(ctx, m.mirror+"/index.json")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Version string          `json:"version"`
		Date    string          `json:"date"`
		Files   []string        `json:"files"`
		LTS     json.RawMessage `json:"lts"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("read %s/index.json: %w", m.mirror, err)
	}
	platform := Platform()
	var out []Release
	for _, r := range raw {
		v, err := ParseVersion(r.Version)
		if err != nil || !slices.Contains(r.Files, platform) {
			continue
		}
		var lts string
		json.Unmarshal(r.LTS, &lts) // false for current releases
		out = append(out, Release{Version: v, LTS: lts, Date: r.Date})
	}
	slices.SortFunc(out, func(a, b Release) int { return b.Version.Compare(a.Version) })
	m.mu.Lock()
	m.index, m.indexAt = out, time.Now()
	m.mu.Unlock()
	return out, nil
}

// Resolve turns a spec into a version. An installed version that fits wins,
// so deployments work offline and keep their version until "update"; only
// when none fits is the newest fitting release looked up, preferring LTS
// releases. ok is false when that version still has to be installed.
func (m *Manager) Resolve(ctx context.Context, spec string) (v Version, installed bool, err error) {
	s, err := ParseSpec(spec)
	if err != nil {
		return v, false, err
	}
	have, err := m.Installed()
	if err != nil {
		return v, false, err
	}
	r := s.Range
	if s.Alias != "" {
		rel, err := m.Releases(ctx)
		if err != nil {
			// Offline: the newest installed version that fits the alias.
			for _, in := range have {
				if s.Alias == "latest" || (in.LTS != "" && (s.Codename == "" || strings.EqualFold(in.LTS, s.Codename))) {
					return in.Version, true, nil
				}
			}
			return v, false, err
		}
		i := slices.IndexFunc(rel, s.alias)
		if i < 0 {
			return v, false, fmt.Errorf("no Node.js release is %s", spec)
		}
		r = majorRange(rel[i].Version.Major())
	}
	for _, in := range have {
		if r.Match(in.Version) {
			return in.Version, true, nil
		}
	}
	rel, err := m.Releases(ctx)
	if err != nil {
		return v, false, err
	}
	if best, ok := newest(rel, r); ok {
		return best.Version, false, nil
	}
	return v, false, fmt.Errorf("no Node.js release for %s matches %q", Platform(), spec)
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
	return Version{}, fmt.Errorf("no Node.js release for %s matches %q", Platform(), spec)
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

func equalFold(a, b string) bool { return strings.EqualFold(a, b) }

func majorRange(major int) Range {
	return Range{{{">=", Version{major, 0, 0}}, {"<", Version{major + 1, 0, 0}}}}
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
	if m.isInstalled(v) {
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

	name := "node-" + v.String() + "-" + Platform()
	base := m.mirror + "/" + v.String()
	sums, err := m.get(ctx, base+"/SHASUMS256.txt")
	if err != nil {
		return err
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[1] == name+".tar.gz" {
			want = f[0]
		}
	}
	if want == "" {
		return fmt.Errorf("%s.tar.gz is not listed in %s/SHASUMS256.txt", name, base)
	}

	tmp, err := os.MkdirTemp(m.dir, ".install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	archive := filepath.Join(tmp, name+".tar.gz")
	logged := int64(0) // tenths of the download reported in w
	size, err := m.download(ctx, base+"/"+name+".tar.gz", archive, want, func(total int64) {
		fmt.Fprintf(w, "downloading %s.tar.gz (%s) from %s\n", name, humanSize(total), m.mirror)
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
	fmt.Fprintf(w, "checksum verified, unpacking %s\n", humanSize(size))

	root := filepath.Join(tmp, "root")
	if err := extract(archive, root); err != nil {
		return fmt.Errorf("unpack %s: %w", name, err)
	}
	os.Remove(archive)
	if err := context.Cause(ctx); err != nil {
		return err
	}

	// pnpm and yarn follow the packageManager field of package.json through
	// corepack. Node.js 25 and later no longer ship it, so it comes from npm.
	bin := filepath.Join(root, "bin")
	env := append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	run := func(name string, args ...string) error {
		cmd := exec.CommandContext(ctx, filepath.Join(bin, name), args...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if _, err := os.Stat(filepath.Join(bin, "corepack")); err != nil {
		fmt.Fprintf(w, "installing corepack from npm, for pnpm and yarn\n")
		if err := run("npm", "install", "--global", "--no-fund", "--no-audit", "corepack"); err != nil {
			fmt.Fprintf(w, "corepack could not be installed, so pnpm and yarn are not set up: %s\n", err)
		}
	}
	if _, err := os.Stat(filepath.Join(bin, "corepack")); err == nil {
		if err := run("corepack", "enable"); err != nil {
			fmt.Fprintf(w, "corepack enable failed, so pnpm and yarn are not set up: %s\n", err)
		}
	}

	md := meta{LTS: m.ltsOf(v), Size: dirSize(root), InstalledAt: time.Now().UTC()}
	b, _ := json.Marshal(md)
	if err := os.WriteFile(filepath.Join(root, metaFile), b, 0o644); err != nil {
		return err
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	return os.Rename(root, filepath.Join(m.dir, v.String()))
}

// ltsOf is the codename of v from the cached index, if it was fetched.
func (m *Manager) ltsOf(v Version) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.index {
		if r.Version == v {
			return r.LTS
		}
	}
	return ""
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

// Remove deletes an installed version.
func (m *Manager) Remove(v Version) error {
	dir := filepath.Join(m.dir, v.String())
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("Node.js %s is not installed", v)
	}
	// Rename first, so a deployment never sees half a version.
	trash, err := os.MkdirTemp(m.dir, ".remove-")
	if err != nil {
		return err
	}
	if err := os.Rename(dir, filepath.Join(trash, "v")); err != nil {
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

// Update installs the newest release of every installed major line, then
// removes the older versions of that line unless keep(version) says a project
// asks for exactly that one.
func (m *Manager) Update(ctx context.Context, keep func(Version) bool, w io.Writer) ([]Change, error) {
	have, err := m.Installed()
	if err != nil || len(have) == 0 {
		return nil, err
	}
	rel, err := m.Releases(ctx)
	if err != nil {
		return nil, err
	}
	var changes []Change
	seen := map[int]bool{}
	for _, in := range have {
		major := in.Version.Major()
		if seen[major] {
			continue
		}
		seen[major] = true
		latest, ok := newest(rel, majorRange(major))
		if !ok || latest.Version.Compare(in.Version) <= 0 {
			continue
		}
		if err := m.Install(ctx, latest.Version, w); err != nil {
			return changes, err
		}
		for _, old := range have {
			if old.Version.Major() != major {
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

// download saves url to path and checks its SHA256 against want.
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
	if hex.EncodeToString(h.Sum(nil)) != want {
		return n, fmt.Errorf("checksum mismatch for %s; the download is damaged or was altered", url)
	}
	return n, nil
}

// counter reports how many bytes went through it.
type counter struct {
	done, total int64
	f           func(done, total int64)
}

func (c *counter) Write(b []byte) (int, error) {
	c.done += int64(len(b))
	c.f(c.done, c.total)
	return len(b), nil
}

// ProgressLabel reads like "27.1 of 54.2 MB (50%)".
func ProgressLabel(done, total int64) string {
	mb := func(n int64) string { return fmt.Sprintf("%.1f", float64(n)/(1<<20)) }
	if total <= 0 {
		return mb(done) + " MB downloaded"
	}
	return fmt.Sprintf("%s of %s MB (%d%%)", mb(done), mb(total), done*100/total)
}

// extract unpacks a release tarball into dst, dropping its top directory.
// Entries that would land outside dst are refused.
func extract(archive, dst string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	inside := func(p string) bool {
		rel, err := filepath.Rel(dst, p)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
	}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		_, rel, ok := strings.Cut(filepath.ToSlash(h.Name), "/")
		if !ok || rel == "" {
			if h.Typeflag == tar.TypeDir {
				if err := os.MkdirAll(dst, 0o755); err != nil {
					return err
				}
			}
			continue
		}
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if !inside(target) {
			return fmt.Errorf("%s points outside the archive", h.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := fs.FileMode(h.Mode).Perm()
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode|0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode|0o600)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tr)
			if cerr := out.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		case tar.TypeSymlink:
			link := h.Linkname
			if filepath.IsAbs(link) || !inside(filepath.Join(filepath.Dir(target), link)) {
				return fmt.Errorf("%s links outside the archive", h.Name)
			}
			if err := os.Symlink(link, target); err != nil {
				return err
			}
		case tar.TypeLink:
			_, lrel, _ := strings.Cut(filepath.ToSlash(h.Linkname), "/")
			src := filepath.Join(dst, filepath.FromSlash(lrel))
			if !inside(src) {
				return fmt.Errorf("%s links outside the archive", h.Name)
			}
			if err := os.Link(src, target); err != nil {
				return err
			}
		}
	}
}

func dirSize(dir string) int64 {
	var n int64
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

func humanSize(n int64) string {
	switch {
	case n <= 0:
		return "size unknown"
	case n < 1<<20:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}

// versionDir matches the names Installed accepts, for callers that take a
// version from a URL.
var versionDir = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// ValidVersion reports whether s names an installed-version directory.
func ValidVersion(s string) bool { return versionDir.MatchString(s) }
