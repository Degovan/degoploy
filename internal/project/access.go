package project

import (
	"errors"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// CheckWorkDirAccess makes sure a deployment can clone into dir as the user
// this process runs as: dir is a git checkout or an empty folder it may write
// to, or a folder it may create. The error says how to fix it, so the project
// form shows the problem before the first deployment runs into it.
func CheckWorkDirAccess(dir string) error {
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
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return nil
	}
	if entries, err := os.ReadDir(dir); err != nil {
		return invalid("work_dir", "%s", err)
	} else if len(entries) > 0 {
		return invalid("work_dir", "%s has files but is not a git repository; choose an empty folder or a git checkout", dir)
	}
	return nil
}
