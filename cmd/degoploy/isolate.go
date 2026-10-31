package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/degovan/degoploy/internal/config"
	"github.com/degovan/degoploy/internal/release"
	"github.com/degovan/degoploy/internal/runas"
	"github.com/degovan/degoploy/internal/toolchain"
)

// Deployments run as their own user, so deploy scripts cannot read degoploy's
// data or reach its admin socket.
const (
	deployUser      = "degoploy-deploy"
	deployHome      = "/var/lib/degoploy-deploy"
	installRuntimes = "/var/lib/degoploy-runtimes"
	sudoersFile     = "/etc/sudoers.d/degoploy"
	// sudoersCopy holds the service user's own sudo rules, copied for the
	// deploy user when setup first moved scripts to it.
	sudoersCopy = "/etc/sudoers.d/degoploy-deploy"
)

// newSettings are added to an existing config that does not have them.
var newSettings = [][3]string{
	{"projects_dir", installProjects, "Projects without a working directory of their own are cloned into <projects_dir>/<slug>."},
	{"runtimes_dir", installRuntimes, "Node.js and PHP versions for deploy scripts, readable by deploy_user."},
	{"deploy_user", deployUser, "git and deploy scripts run as this user (through sudo), so they cannot read degoploy's data."},
}

// ensureDeployUser creates the user deployments run as and lets the service
// user hand it files (the SSH agent socket) through its group. It reports
// whether the service user joined the group, which takes a restart.
func ensureDeployUser() (bool, error) {
	if _, err := user.Lookup(deployUser); err != nil {
		nologin := "/usr/sbin/nologin"
		if p, err := exec.LookPath("nologin"); err == nil {
			nologin = p
		}
		if err := run("useradd", "--system", "--user-group", "--home-dir", deployHome, "--shell", nologin, deployUser); err != nil {
			return false, err
		}
		success("Created the %s system user; deployments run as it", deployUser)
	}
	uid, gid, err := lookupIDs(deployUser)
	if err != nil {
		return false, err
	}
	// npm, Composer and friends keep their caches in HOME.
	if err := ensureDir(deployHome, 0o750, uid, gid); err != nil {
		return false, err
	}
	if inGroup(serviceUser, deployUser) {
		return false, nil
	}
	return true, addToGroup(serviceUser, deployUser)
}

// sudoRule lets the service user run "degoploy run-step" as the deploy user,
// and nothing else. No pty, so the step stays in sudo's process group.
func sudoRule(deploy string) string {
	return fmt.Sprintf(`# Written by degoploy setup, which replaces it on every run.
# degoploy runs git and deploy scripts as %[2]s through this rule.
Cmnd_Alias DEGOPLOY_STEP = %[3]s %[4]s
Defaults!DEGOPLOY_STEP !use_pty, !requiretty
%[1]s ALL=(%[2]s) NOPASSWD: DEGOPLOY_STEP
`, serviceUser, deploy, installBin, runas.Subcommand)
}

// installSudoers writes a sudoers drop-in after visudo accepts it; a broken
// file there would break sudo for everyone. It reports whether it changed.
func installSudoers(path, content string) (bool, error) {
	if old, err := os.ReadFile(path); err == nil && string(old) == content {
		return false, nil
	}
	if _, err := exec.LookPath("visudo"); err != nil {
		return false, errors.New("sudo is not installed; deployments run as " + deployUser + " through it")
	}
	// sudo skips files whose name has a dot, so the draft is never read.
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".new")
	if err := os.WriteFile(tmp, []byte(content), 0o440); err != nil {
		return false, err
	}
	defer os.Remove(tmp)
	if out, err := exec.Command("visudo", "-cqf", tmp).CombinedOutput(); err != nil {
		return false, fmt.Errorf("visudo rejected %s: %s", path, strings.TrimSpace(string(out)))
	}
	if err := errors.Join(os.Chown(tmp, 0, 0), os.Chmod(tmp, 0o440)); err != nil {
		return false, err
	}
	return true, os.Rename(tmp, path)
}

// copySudoRules copies the sudo rules written for the service user, which
// deploy scripts used while they ran as it, for the deploy user. It runs
// once: the copy is the admin's to change after that.
func copySudoRules(deploy string) error {
	if _, err := os.Stat(sudoersCopy); err == nil {
		return nil
	}
	files := []string{"/etc/sudoers"}
	if entries, err := os.ReadDir("/etc/sudoers.d"); err == nil {
		for _, e := range entries {
			path := filepath.Join("/etc/sudoers.d", e.Name())
			if !e.IsDir() && !strings.ContainsAny(e.Name(), ".~") && path != sudoersFile && path != sudoersCopy {
				files = append(files, path)
			}
		}
	}
	var rules []string
	var from []string
	for _, path := range files {
		found := false
		for _, line := range sudoLines(path) {
			if f := strings.Fields(line); len(f) > 1 && f[0] == serviceUser {
				rules = append(rules, deploy+strings.TrimPrefix(line, serviceUser))
				found = true
			}
		}
		if found {
			from = append(from, path)
		}
	}
	if len(rules) == 0 {
		return nil
	}
	content := "# Copied by degoploy setup from the rules for " + serviceUser + " in " + strings.Join(from, ", ") + ",\n" +
		"# when deploy scripts moved to " + deploy + ". setup leaves this file alone from now on.\n" +
		strings.Join(rules, "\n") + "\n"
	if _, err := installSudoers(sudoersCopy, content); err != nil {
		warning("Could not copy the sudo rules of %s for %s: %v", serviceUser, deploy, err)
		return nil
	}
	success("Copied %d sudo %s of %s for %s into %s", len(rules), pluralWord(len(rules), "rule"), serviceUser, deploy, sudoersCopy)
	warning("Deploy scripts now run as %s. The rules for %s in %s are no longer needed by them.", deploy, serviceUser, strings.Join(from, ", "))
	return nil
}

// sudoLines returns the lines of a sudoers file with continuations joined
// and comments dropped.
func sudoLines(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	var cur strings.Builder
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if cont, ok := strings.CutSuffix(line, `\`); ok {
			cur.WriteString(cont + " ")
			continue
		}
		cur.WriteString(line)
		if l := strings.TrimSpace(cur.String()); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
		cur.Reset()
	}
	return out
}

// moveRuntimes moves Node.js and PHP versions downloaded into the data
// directory, which the deploy user may not read, to runtimes_dir.
func moveRuntimes(cfg config.Config, uid, gid int) error {
	if cfg.RuntimesDir == cfg.DataDir {
		return nil
	}
	if err := ensureDir(cfg.RuntimesDir, 0o755, uid, gid); err != nil {
		return err
	}
	for _, name := range toolchain.Runtimes {
		from, to := filepath.Join(cfg.DataDir, name), filepath.Join(cfg.RuntimesDir, name)
		if _, err := os.Stat(from); err != nil {
			continue
		}
		if _, err := os.Stat(to); err == nil {
			continue
		}
		if err := os.Rename(from, to); err != nil {
			return err
		}
		success("Moved %s to %s", from, to)
	}
	return nil
}

// chownTree gives what uid from owns under root to uid to, keeping groups,
// and returns how many entries it changed. Symlinks are not followed.
func chownTree(root string, from, to int) (int, error) {
	n := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) == from {
			if err := os.Lchown(path, to, -1); err != nil {
				return err
			}
			// chown drops setuid/setgid on files; directories keep setgid.
			if !d.IsDir() && d.Type()&fs.ModeSymlink == 0 && info.Mode()&(fs.ModeSetuid|fs.ModeSetgid) != 0 {
				os.Chmod(path, info.Mode())
			}
			n++
		}
		return nil
	})
	return n, err
}

// migrateWorkDirs hands the files deployments made as the service user (uid)
// to the deploy user (name, duid), then checks that it may deploy there. skip
// is the projects folder, which ensureProjectsDir already handed over.
func migrateWorkDirs(ctx context.Context, dirs []string, skip string, uid int, name string, duid int) {
	r := &runas.Runner{User: name, Exe: installBin}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		if skip == "" || !strings.HasPrefix(dir+"/", skip+"/") {
			n, err := chownTree(dir, uid, duid)
			if err != nil {
				warning("%s: %v", dir, err)
				continue
			}
			if n > 0 {
				success("Gave %d %s in %s to %s", n, pluralWord(n, "file"), dir, name)
			}
		}
		// A project with releases has them in its folder by now.
		_, err := os.Stat(filepath.Join(dir, release.ReleasesDir))
		if msg, err := r.CheckWorkDir(ctx, dir, err == nil); err != nil {
			warning("%s: %v", dir, err)
		} else if msg != "" {
			warning("%s", msg)
		}
	}
}
