package project

import (
	"errors"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/degovan/degoploy/internal/release"
)

// CheckWorkDirAccess makes sure a deployment can clone into dir as the user
// this process runs as: dir is a git checkout (with releases, a folder laid
// out for them) or an empty folder it may write to, or a folder it may
// create. The error says how to fix it, so the project form shows the problem
// before the first deployment runs into it.
func CheckWorkDirAccess(dir string, releases bool) error {
	who := "degoploy"
	if u, err := user.Current(); err == nil {
		who = u.Username
	}

	// The deepest folder that exists, and the one below it on the way to dir.
	have, next := dir, ""
	for {
		st, err := os.Stat(have)
		if err == nil {
			if !st.IsDir() {
				return invalid("work_dir", "%s is a file, not a folder", have)
			}
			break
		}
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, fs.ErrPermission) && !errors.Is(err, unix.ENOTDIR) {
			return invalid("work_dir", "%s", err)
		}
		parent := filepath.Dir(have)
		if parent == have {
			return invalid("work_dir", "%s", err)
		}
		have, next = parent, have
	}

	switch {
	case unix.Access(have, unix.X_OK) != nil:
		return invalid("work_dir", "%s cannot open %s. Let it in with: sudo setfacl -m u:%s:x %s", who, have, who, have)
	case next != "" && unix.Access(have, unix.W_OK) != nil:
		return invalid("work_dir", "%s cannot create folders in %s. Create this one for it with: sudo install -d -o %s -m 2775 %s", who, have, who, dir)
	case next != "":
		return nil
	case unix.Access(dir, unix.W_OK) != nil:
		return invalid("work_dir", "%s cannot write to %s. Give it access with: sudo setfacl -R -m u:%s:rwX -m d:u:%s:rwX %s", who, dir, who, who, dir)
	}
	_, gitErr := os.Stat(filepath.Join(dir, ".git"))
	if !releases && gitErr == nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return invalid("work_dir", "%s", err)
	}
	switch {
	case releases && gitErr == nil:
		return invalid("work_dir", "%s holds a checkout deployed in place; releases need a folder of their own. Choose an empty one, then point the web server at its current folder", dir)
	case releases:
		for _, e := range entries {
			if !slices.Contains(release.Names, e.Name()) && !strings.HasPrefix(e.Name(), ".current-") {
				return invalid("work_dir", "%s has files that are not releases (%s); choose an empty folder", dir, e.Name())
			}
		}
	case len(entries) > 0:
		if _, err := os.Stat(release.Current(dir)); err == nil {
			return invalid("work_dir", "%s holds releases; turn releases back on, or choose an empty folder to deploy in place", dir)
		}
		return invalid("work_dir", "%s has files but is not a git repository; choose an empty folder or a git checkout", dir)
	}
	return nil
}
