// Package release lays out a project that deploys in releases. Each
// deployment checks out into a folder of its own and the "current" link
// switches to it only when the deploy script succeeded, so the site never
// serves a half-built release and an older one can be made current again:
//
//	<work_dir>/repo/          bare repository: fetched objects, one worktree per release
//	<work_dir>/releases/<id>/ the checkout of deployment <id>
//	<work_dir>/shared/        files every release links to (.env, storage/, uploads)
//	<work_dir>/current        symlink to releases/<id>; point the web server here
//
// The Op actions change these folders and run as the deploy user, which owns
// them; degoploy itself only reads them.
package release

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	RepoDir     = "repo"
	ReleasesDir = "releases"
	SharedDir   = "shared"
	CurrentLink = "current"
)

// Names are the entries degoploy keeps in a project's working directory.
var Names = []string{RepoDir, ReleasesDir, SharedDir, CurrentLink}

func Repo(base string) string    { return filepath.Join(base, RepoDir) }
func Shared(base string) string  { return filepath.Join(base, SharedDir) }
func Current(base string) string { return filepath.Join(base, CurrentLink) }
func Dir(base string, id int64) string {
	return filepath.Join(base, ReleasesDir, strconv.FormatInt(id, 10))
}

// Live returns the release current points at, or 0.
func Live(base string) int64 {
	target, err := os.Readlink(Current(base))
	if err != nil {
		return 0
	}
	if filepath.IsAbs(target) {
		target, _ = filepath.Rel(base, target)
	}
	dir, name := path.Split(filepath.ToSlash(target))
	if dir != ReleasesDir+"/" {
		return 0
	}
	id, _ := strconv.ParseInt(name, 10, 64)
	return id
}

// List returns the releases on disk, newest first.
func List(base string) []int64 {
	entries, _ := os.ReadDir(filepath.Join(base, ReleasesDir))
	var ids []int64
	for _, e := range entries {
		if id, err := strconv.ParseInt(e.Name(), 10, 64); err == nil && id > 0 && e.IsDir() {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b int64) int { return cmp.Compare(b, a) })
	return ids
}

// Exists reports whether release id is on disk.
func Exists(base string, id int64) bool {
	st, err := os.Stat(Dir(base, id))
	return err == nil && st.IsDir()
}

// CleanShared normalizes a shared path: relative, slash-separated, with a
// trailing slash when it names a folder. It returns "" for a path that would
// leave the release or replace all of it.
func CleanShared(p string) string {
	p = strings.TrimSpace(filepath.ToSlash(p))
	dir := strings.HasSuffix(p, "/")
	p = path.Clean(p)
	if p == "." || p == ".." || path.IsAbs(p) || strings.HasPrefix(p, "../") {
		return ""
	}
	if first, _, _ := strings.Cut(p, "/"); first == ".git" {
		return ""
	}
	if dir {
		p += "/"
	}
	return p
}

// Op is a change to a project's releases, run by "degoploy run-step" as the
// deploy user.
type Op struct {
	Action string `json:"action"` // see the Op* constants
	Base   string `json:"base"`
	ID     int64  `json:"id,omitempty"`
	// Shared paths to link (OpLink), cleaned by CleanShared.
	Shared []string `json:"shared,omitempty"`
	// Keep is how many releases OpPrune leaves, counting the live one and ID.
	Keep int `json:"keep,omitempty"`
}

const (
	// OpPrepare makes the folders and removes a stale release ID.
	OpPrepare = "prepare"
	// OpLink links release ID's shared paths to shared/.
	OpLink = "link"
	// OpActivate points current at release ID.
	OpActivate = "activate"
	// OpRemove removes release ID unless it is live.
	OpRemove = "remove"
	// OpPrune removes all but the newest Keep releases, the live one and ID.
	OpPrune = "prune"
)

// Run performs the operation and prints what it did to w.
func (op Op) Run(w io.Writer) error {
	if !filepath.IsAbs(op.Base) {
		return fmt.Errorf("release folder %q is not absolute", op.Base)
	}
	needID := op.Action != OpPrune
	if needID && op.ID <= 0 {
		return errors.New("no release given")
	}
	switch op.Action {
	case OpPrepare:
		return op.prepare()
	case OpLink:
		return op.link(w)
	case OpActivate:
		return op.activate()
	case OpRemove:
		if Live(op.Base) == op.ID {
			return nil
		}
		return os.RemoveAll(Dir(op.Base, op.ID))
	case OpPrune:
		op.prune(w)
		return nil
	}
	return fmt.Errorf("unknown release action %q", op.Action)
}

func (op Op) prepare() error {
	for _, dir := range []string{filepath.Join(op.Base, ReleasesDir), Shared(op.Base)} {
		if err := os.MkdirAll(dir, 0o775); err != nil {
			return err
		}
	}
	if Live(op.Base) == op.ID {
		return fmt.Errorf("release %d is live; it cannot be built again", op.ID)
	}
	return os.RemoveAll(Dir(op.Base, op.ID))
}

// link replaces each shared path in the release with a link into shared/.
// The first release to have a path moves its copy there; a path no release
// had starts as an empty folder (trailing slash) or an empty file.
func (op Op) link(w io.Writer) error {
	rel := Dir(op.Base, op.ID)
	for _, p := range op.Shared {
		clean := CleanShared(p)
		if clean == "" {
			return fmt.Errorf("shared path %q is not inside the release", p)
		}
		isDir := strings.HasSuffix(clean, "/")
		name := strings.TrimSuffix(clean, "/")
		target := filepath.Join(Shared(op.Base), filepath.FromSlash(name))
		in := filepath.Join(rel, filepath.FromSlash(name))

		if _, err := os.Lstat(target); errors.Is(err, fs.ErrNotExist) {
			if err := os.MkdirAll(filepath.Dir(target), 0o775); err != nil {
				return err
			}
			st, err := os.Lstat(in)
			switch {
			case err == nil && st.Mode()&fs.ModeSymlink == 0:
				if err := os.Rename(in, target); err != nil {
					return err
				}
				fmt.Fprintf(w, "shared/%s: moved here from this release\n", clean)
			case isDir:
				if err := os.Mkdir(target, 0o775); err != nil {
					return err
				}
				fmt.Fprintf(w, "shared/%s: created empty\n", clean)
			default:
				f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o660)
				if err != nil {
					return err
				}
				f.Close()
				fmt.Fprintf(w, "shared/%s: created empty; fill it in on the server\n", clean)
			}
		} else if err != nil {
			return err
		}

		if err := os.RemoveAll(in); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(in), 0o775); err != nil {
			return err
		}
		if err := os.Symlink(target, in); err != nil {
			return err
		}
	}
	if len(op.Shared) > 0 {
		fmt.Fprintf(w, "linked %d shared %s\n", len(op.Shared), plural(len(op.Shared), "path"))
	}
	return nil
}

// activate switches current to the release in one rename, so requests see
// either the old release or the new one.
func (op Op) activate() error {
	if !Exists(op.Base, op.ID) {
		return fmt.Errorf("release %d is no longer on the server", op.ID)
	}
	cur := Current(op.Base)
	if st, err := os.Lstat(cur); err == nil && st.Mode()&fs.ModeSymlink == 0 {
		return fmt.Errorf("%s is a folder, not a link; move it away so degoploy can point it at the release", cur)
	}
	tmp := filepath.Join(op.Base, ".current-"+strconv.FormatInt(op.ID, 10))
	os.Remove(tmp)
	// Relative, so the link still works when the folder is mounted elsewhere.
	if err := os.Symlink(path.Join(ReleasesDir, strconv.FormatInt(op.ID, 10)), tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, cur); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// prune keeps the newest releases, Keep in all, counting the live one and ID.
func (op Op) prune(w io.Writer) {
	live := Live(op.Base)
	kept := 0
	if live != 0 {
		kept++
	}
	if op.ID != 0 && op.ID != live && Exists(op.Base, op.ID) {
		kept++
	}
	for _, id := range List(op.Base) {
		if id == live || id == op.ID {
			continue
		}
		if kept < max(op.Keep, 1) {
			kept++
			continue
		}
		if err := os.RemoveAll(Dir(op.Base, id)); err != nil {
			fmt.Fprintf(w, "could not remove release %d: %v\n", id, err)
			continue
		}
		fmt.Fprintf(w, "removed release %d\n", id)
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
