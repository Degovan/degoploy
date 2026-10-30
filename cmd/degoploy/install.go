package main

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/degovan/degoploy/deploy"
	"github.com/degovan/degoploy/internal/admin"
	"github.com/degovan/degoploy/internal/config"
)

// Where setup installs degoploy.
const (
	installBin  = "/usr/local/bin/degoploy"
	installUnit = "/etc/systemd/system/degoploy.service"
	confDir     = "/etc/degoploy"
	envFile     = confDir + "/degoploy.env"
	installData = "/var/lib/degoploy"
	// installProjects holds projects that do not name a working directory.
	installProjects = "/srv/degoploy"
	serviceUser     = "degoploy"
)

// installOptions answer setup's questions ahead of time, for scripts.
type installOptions struct {
	listen, baseURL, admin, operator string
	noOperator, noStart, yes         bool
}

func installFlags(fs *flag.FlagSet) *installOptions {
	o := &installOptions{}
	fs.StringVar(&o.listen, "listen", "", "address to listen on, for a new install (default 127.0.0.1:8090)")
	fs.StringVar(&o.baseURL, "base-url", "", "public https:// URL, for a new install")
	fs.StringVar(&o.admin, "admin", "", "username of the first admin")
	fs.StringVar(&o.operator, "operator", "", "user who may manage degoploy without sudo (default: whoever ran sudo)")
	fs.BoolVar(&o.noOperator, "no-operator", false, "add nobody to the degoploy group")
	fs.BoolVar(&o.noStart, "no-start", false, "install without starting the service")
	fs.BoolVar(&o.yes, "yes", false, "take the defaults instead of asking")
	fs.BoolVar(&o.yes, "y", false, "short for --yes")
	return o
}

// installState is what setup finds on this server.
type installState struct {
	exe       string // this binary
	config    bool   // defaultConfig exists
	current   bool   // installBin is this binary
	installed string // version of installBin when it is another binary
	systemd   bool
}

func inspectInstall() installState {
	s := installState{config: installedHere()}
	s.exe, _ = os.Executable()
	s.current = sameBinary(s.exe, installBin)
	if !s.current {
		if out, err := exec.Command(installBin, "version").Output(); err == nil {
			s.installed = strings.TrimSpace(string(out))
		}
	}
	if fi, err := os.Stat("/run/systemd/system"); err == nil && fi.IsDir() {
		s.systemd = true
	}
	return s
}

// installedHere reports whether setup has run on this server. Users outside
// the degoploy group may not look inside /etc/degoploy, which still counts.
func installedHere() bool {
	_, err := os.Stat(defaultConfig)
	return !errors.Is(err, fs.ErrNotExist)
}

// sameBinary reports whether path holds the same program as exe.
func sameBinary(exe, path string) bool {
	a, errA := os.Stat(exe)
	b, errB := os.Stat(path)
	if errA != nil || errB != nil {
		return false
	}
	if os.SameFile(a, b) {
		return true
	}
	if a.Size() != b.Size() {
		return false
	}
	x, errA := os.ReadFile(exe)
	y, errB := os.ReadFile(path)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

// asRoot runs setup again through sudo, with the same flags.
func asRoot(exe string, args []string) error {
	line := shellArg(exe) + " setup"
	for _, a := range args {
		line += " " + shellArg(a)
	}
	sudo, err := exec.LookPath("sudo")
	if err != nil || !interactive() {
		return withHint("installing degoploy needs root", "Run: sudo "+line)
	}
	lipgloss.Println(muted.Render("Continuing as root: sudo " + line))
	return syscall.Exec(sudo, append([]string{"sudo", "--", exe, "setup"}, args...), os.Environ())
}

func shellArg(s string) string {
	if s == "" || strings.ContainsAny(s, " \t'\"$\\`;&|<>()*?") {
		return strconv.Quote(s)
	}
	return s
}

// install puts degoploy on this server, or brings an existing install up to
// this binary: the service user, directories, config, systemd unit and the
// operator's group. Every step checks first, so running it again repairs.
func install(ctx context.Context, s installState, o *installOptions) error {
	if runtime.GOOS != "linux" {
		return errors.New("degoploy setup installs on Linux only")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		return errors.New("bash is required: deploy scripts run with it")
	}
	operator := o.operator
	if operator == "" {
		if u := os.Getenv("SUDO_USER"); u != "root" {
			operator = u
		}
	}
	if o.noOperator {
		operator = ""
	}
	if operator != "" {
		if _, err := user.Lookup(operator); err != nil {
			return fmt.Errorf("--operator %s: no such user", operator)
		}
	}
	fresh := !s.config
	_, err := os.Stat(filepath.Join(installData, "degoploy.db"))
	freshData := errors.Is(err, fs.ErrNotExist)

	switch {
	case fresh:
		heading("degoploy setup", version+" · new install")
	case s.installed != "" && s.installed != version:
		heading("degoploy setup", "upgrading "+s.installed+" → "+version)
	default:
		heading("degoploy setup", version)
	}
	lipgloss.Println()

	// Questions first, so nothing changes until the answers are in.
	listen, baseURL := cmp.Or(o.listen, "127.0.0.1:8090"), o.baseURL
	joinGroup := operator != "" && !inGroup(operator, serviceUser)
	tools := missingTools()
	pm := findPkgManager()
	installTools := len(tools) > 0 && pm != nil
	if interactive() && !o.yes {
		var form []huh.Field
		if fresh && o.listen == "" {
			form = append(form, huh.NewInput().Title("Address to listen on").
				Description("Keep it on 127.0.0.1 and put a reverse proxy with HTTPS in front.").
				Value(&listen).Validate(validListen))
		}
		if fresh && o.baseURL == "" {
			form = append(form, huh.NewInput().Title("Public URL").
				Description("For example https://deploy.example.com. Leave it empty to set it later.").
				Value(&baseURL).Validate(validBaseURL))
		}
		if joinGroup && o.operator == "" {
			form = append(form, huh.NewConfirm().Title("Let "+operator+" manage degoploy without sudo?").
				Description("Adds them to the "+serviceUser+" group. Members can do everything an admin can.").
				Value(&joinGroup))
		}
		if installTools {
			form = append(form, huh.NewConfirm().Title("Install "+toolNames(tools)+" with "+pm.name+"?").
				Description("Deployments fetch code with them.").
				Value(&installTools))
		}
		if len(form) > 0 {
			if err := ask(form...); err != nil {
				return err
			}
			lipgloss.Println()
		}
	}
	if err := validListen(listen); err != nil {
		return fmt.Errorf("--listen: %w", err)
	}
	if err := validBaseURL(baseURL); err != nil {
		return fmt.Errorf("--base-url: %w", err)
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")

	if len(tools) > 0 {
		if installTools {
			if err := working("Installing "+toolNames(tools), func(context.Context) error { return pm.add(tools) }); err != nil {
				return err
			}
			success("Installed %s", toolNames(tools))
		} else {
			warning("%s not installed; deployments need it", toolNames(tools))
		}
	}

	if _, err := user.Lookup(serviceUser); err != nil {
		nologin := "/usr/sbin/nologin"
		if p, err := exec.LookPath("nologin"); err == nil {
			nologin = p
		}
		if err := run("useradd", "--system", "--user-group", "--home-dir", installData, "--shell", nologin, serviceUser); err != nil {
			return err
		}
		success("Created the %s system user", serviceUser)
	}
	uid, gid, err := lookupIDs(serviceUser)
	if err != nil {
		return err
	}
	if err := ensureDir(installData, 0o700, uid, gid); err != nil {
		return err
	}
	// The service owns its config, so the SSO and Email pages can save into it.
	if err := ensureDir(confDir, 0o750, uid, gid); err != nil {
		return err
	}

	switch {
	case joinGroup:
		if err := addToGroup(operator, serviceUser); err != nil {
			return err
		}
		success("Added %s to the %s group", operator, serviceUser)
	case operator != "" && inGroup(operator, serviceUser):
		success("%s is in the %s group", operator, serviceUser)
	}

	binChanged := !s.current
	if binChanged {
		if err := copyBinary(s.exe, installBin); err != nil {
			return err
		}
		success("Installed degoploy %s to %s", version, installBin)
	}

	configChanged := false
	if fresh {
		if err := writeConfig(listen, baseURL); err != nil {
			return err
		}
		success("Wrote %s", defaultConfig)
	} else if added, err := config.AddProjectsDir(defaultConfig, installProjects); err != nil {
		return err
	} else if added {
		configChanged = true
		success("Set projects_dir: %s in %s", installProjects, defaultConfig)
	}
	if err := errors.Join(os.Chown(defaultConfig, uid, gid), os.Chmod(defaultConfig, 0o640)); err != nil {
		return err
	}
	if _, err := os.Stat(envFile); errors.Is(err, fs.ErrNotExist) {
		env := "# Secrets for degoploy, loaded by systemd (EnvironmentFile). Example:\n# DEGOPLOY_OIDC_SECRET=change-me\n"
		if err := os.WriteFile(envFile, []byte(env), 0o600); err != nil {
			return err
		}
	}
	// systemd reads it as root before starting the service; nobody else needs it.
	if err := errors.Join(os.Chown(envFile, 0, 0), os.Chmod(envFile, 0o600)); err != nil {
		return err
	}

	// Check the config with the environment the service gets.
	if err := loadEnvFile(envFile); err != nil {
		return err
	}
	rep := admin.Check(ctx, defaultConfig)
	if rep.Error != "" {
		return withHint("the configuration does not load: "+rep.Error, "Fix "+defaultConfig+", then run: sudo degoploy setup")
	}
	cfg, err := config.Load(defaultConfig)
	if err != nil {
		return err
	}
	if cfg.ProjectsDir != "" {
		if err := ensureProjectsDir(cfg.ProjectsDir, uid, gid); err != nil {
			return err
		}
	}

	if !s.systemd {
		for _, w := range rep.Warnings {
			warning("%s", w)
		}
		lipgloss.Println()
		warning("systemd is not running here, so degoploy was not started. Start it with:")
		lipgloss.Println("    install -d -m 0750 -o " + serviceUser + " -g " + serviceUser + " /run/degoploy")
		lipgloss.Println("    RUNTIME_DIRECTORY=/run/degoploy runuser -u " + serviceUser + " -- " + installBin + " serve --config " + defaultConfig)
		lipgloss.Println("  " + muted.Render("then create the first admin with: degoploy setup"))
		return nil
	}

	unitChanged, err := installUnitFile()
	if err != nil {
		return err
	}
	if unitChanged {
		if err := run("systemctl", "daemon-reload"); err != nil {
			return err
		}
		success("Installed the systemd service")
	}
	if err := run("systemctl", "enable", "--quiet", "degoploy"); err != nil {
		return err
	}

	var c *admin.Client
	active := exec.Command("systemctl", "is-active", "--quiet", "degoploy").Run() == nil
	if active {
		// A binary dropped into place by hand leaves the old one running.
		if cl, err := admin.Dial(defaultSocket); err == nil {
			if st, err := cl.Status(ctx); err == nil && st.Version != version {
				binChanged = true
			}
			c = cl
		}
	}
	switch {
	case active && (binChanged || unitChanged || configChanged || c == nil):
		if c != nil {
			c.Close()
		}
		err = working("Restarting degoploy (running deployments get up to 60s to finish)", func(ctx context.Context) error {
			if err := run("systemctl", "restart", "degoploy"); err != nil {
				return err
			}
			c, err = waitForService(ctx)
			return err
		})
		if err != nil {
			return err
		}
		success("Restarted degoploy %s", version)
	case !active && o.noStart:
		lipgloss.Println(muted.Render("Not started (--no-start). Start it with: sudo systemctl start degoploy"))
		return nil
	case !active:
		err = working("Starting degoploy", func(ctx context.Context) error {
			if err := run("systemctl", "start", "degoploy"); err != nil {
				return err
			}
			c, err = waitForService(ctx)
			return err
		})
		if err != nil {
			return err
		}
		success("Started degoploy; it answers on %s", defaultSocket)
	default:
		success("degoploy is running")
	}
	defer c.Close()

	st, err := c.Status(ctx)
	if err != nil {
		return err
	}
	if err := firstAdmin(ctx, c, st, o.admin, freshData); err != nil {
		return err
	}
	if rep, err := c.CheckConfig(ctx); err == nil {
		for _, w := range rep.Warnings {
			warning("%s", w)
		}
	}

	lipgloss.Println()
	lipgloss.Println(green.Render("●") + " " + bold.Render("degoploy is ready"))
	fields([][2]string{
		{"web", webAddress(st)},
		{"config", defaultConfig + muted.Render("   secrets: "+envFile)},
		{"data", installData},
		{"projects", cmp.Or(cfg.ProjectsDir, muted.Render("(not set: each project names its folder)"))},
		{"logs", "journalctl -u degoploy -f"},
		{"manage", "degoploy " + muted.Render("  as root or a member of the "+serviceUser+" group, no sudo needed")},
	})
	if joinGroup {
		lipgloss.Println()
		lipgloss.Println(yellow.Render("!") + " " + operator + " joined the " + serviceUser + " group. It takes effect at their next login, or now with: newgrp " + serviceUser)
	}
	lipgloss.Println()
	lipgloss.Println(bold.Render("Next"))
	if st.BaseURL == "" {
		lipgloss.Println("  " + muted.Render("·") + " Put a reverse proxy with HTTPS in front, then set base_url in " + defaultConfig + ".")
	}
	if cfg.ProjectsDir != "" {
		lipgloss.Println("  " + muted.Render("·") + " Projects are cloned into " + cfg.ProjectsDir + "/<slug>. A folder elsewhere needs write access for " + serviceUser + " (see INSTALL.md).")
	} else {
		lipgloss.Println("  " + muted.Render("·") + " Give " + serviceUser + " write access to each project's directory (see INSTALL.md).")
	}
	lipgloss.Println("  " + muted.Render("·") + " Open " + blue.Render(webAddress(st)) + " and sign in.")
	return nil
}

// firstAdmin creates the first admin when there is none.
func firstAdmin(ctx context.Context, c *admin.Client, st admin.Status, username string, fresh bool) error {
	if st.Admins > 0 {
		success("%d active %s", st.Admins, pluralWord(st.Admins, "admin"))
		return nil
	}
	if username == "" && !interactive() {
		warning("There is no admin yet. Create one with: degoploy user create")
		return nil
	}
	lipgloss.Println()
	lipgloss.Println(bold.Render("Create the first admin"))
	lipgloss.Println(muted.Render("They sign in to the web interface and can invite everyone else."))
	if !fresh {
		lipgloss.Println(muted.Render("The existing data has no active admin."))
	}
	return createUser(ctx, c, admin.CreateUserRequest{Username: username, Role: "admin"})
}

// waitForService waits until the started service answers on its socket.
func waitForService(ctx context.Context) (*admin.Client, error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		if c, err := admin.Dial(defaultSocket); err == nil {
			return c, nil
		}
		out, _ := exec.Command("systemctl", "show", "degoploy", "--property=ActiveState,SubState", "--value").Output()
		state := strings.Fields(string(out))
		if slices.Contains(state, "failed") || slices.Contains(state, "auto-restart") || time.Now().After(deadline) {
			hint := []string{"Its last log lines (journalctl -u degoploy):"}
			if log, err := exec.Command("journalctl", "-u", "degoploy", "-n", "12", "--no-pager", "-o", "cat").Output(); err == nil {
				for _, l := range strings.Split(strings.TrimSpace(string(log)), "\n") {
					hint = append(hint, "  "+l)
				}
			}
			return nil, withHint("degoploy did not start", hint...)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func writeConfig(listen, baseURL string) error {
	tmpl, err := template.New("config").Funcs(template.FuncMap{"quote": strconv.Quote}).Parse(deploy.ConfigTemplate)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]string{
		"Listen":      listen,
		"BaseURL":     baseURL,
		"DataDir":     installData,
		"ProjectsDir": installProjects,
		"EnvFile":     envFile,
		"RedirectURL": cmp.Or(baseURL, "https://deploy.example.com") + "/auth/oidc/callback",
	})
	if err != nil {
		return err
	}
	return writeFileAtomic(defaultConfig, buf.Bytes(), 0o640)
}

// installUnitFile writes the systemd unit; a changed one is kept as .bak.
func installUnitFile() (bool, error) {
	old, err := os.ReadFile(installUnit)
	if err == nil && bytes.Equal(old, deploy.Unit) {
		return false, nil
	}
	if err == nil {
		if err := os.WriteFile(installUnit+".bak", old, 0o644); err != nil {
			return false, err
		}
		warning("Replaced %s; the previous one is in %s.bak (put local changes in drop-ins under %s.d/)", installUnit, installUnit, installUnit)
	}
	return true, writeFileAtomic(installUnit, deploy.Unit, 0o644)
}

// copyBinary replaces dst through a rename, so a running degoploy keeps its
// file until it restarts.
func copyBinary(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(dst), ".degoploy.new")
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, 0o755)
	}
	if err == nil {
		err = os.Rename(tmp, dst)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".new"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func ensureDir(path string, mode os.FileMode, uid, gid int) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	return errors.Join(os.Chown(path, uid, gid), os.Chmod(path, mode))
}

// webGroups are the groups web servers run as, by distribution.
var webGroups = []string{"www-data", "nginx", "apache", "http"}

// ensureProjectsDir makes the folder projects are cloned into: owned by the
// service user and setgid, so every project inherits its group. A new folder
// gets the web server's group, so the server can read the apps and write to
// their storage folders. A folder someone else owns is left as it is.
func ensureProjectsDir(dir string, uid, gid int) error {
	st, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		group := serviceUser
		for _, name := range webGroups {
			if g, err := user.LookupGroup(name); err == nil {
				gid, _ = strconv.Atoi(g.Gid)
				group = name
				break
			}
		}
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return err
		}
		if err := os.Mkdir(dir, 0o775); err != nil {
			return err
		}
		if err := errors.Join(os.Chown(dir, uid, gid), os.Chmod(dir, 0o775|fs.ModeSetgid)); err != nil {
			return err
		}
		success("Created %s for projects (group %s)", dir, group)
		return nil
	}
	if err != nil {
		return err
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); !ok || int(sys.Uid) != uid {
		warning("%s is not owned by %s; it needs write access there to clone projects", dir, serviceUser)
		return nil
	}
	return os.Chmod(dir, st.Mode().Perm()|0o070|fs.ModeSetgid)
}

func lookupIDs(name string) (int, int, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, 0, err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	return uid, gid, nil
}

func inGroup(name, group string) bool {
	u, err := user.Lookup(name)
	g, gerr := user.LookupGroup(group)
	if err != nil || gerr != nil {
		return false
	}
	ids, _ := u.GroupIds()
	return slices.Contains(ids, g.Gid)
}

func addToGroup(name, group string) error {
	if _, err := exec.LookPath("usermod"); err == nil {
		return run("usermod", "-aG", group, name)
	}
	return run("gpasswd", "-a", name, group)
}

// loadEnvFile sets the KEY=VALUE lines of a systemd EnvironmentFile.
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		os.Setenv(strings.TrimSpace(k), v)
	}
	return sc.Err()
}

// run runs a command, returning its output in the error when it fails.
func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		msg := name + " " + strings.Join(args, " ") + ": " + err.Error()
		if s := strings.TrimSpace(string(out)); s != "" {
			msg += "\n" + s
		}
		return errors.New(msg)
	}
	return nil
}

func validListen(s string) error {
	_, port, err := net.SplitHostPort(strings.TrimSpace(s))
	if err != nil {
		return errors.New("use host:port, for example 127.0.0.1:8090")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return errors.New("the port must be a number from 1 to 65535")
	}
	return nil
}

func validBaseURL(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errors.New("use a full URL starting with https://")
	}
	return nil
}

// tool is a program deployments need, with its package name where it differs.
type tool struct {
	cmd  string
	pkgs map[string]string // package manager → package
}

var neededTools = []tool{
	{cmd: "git"},
	{cmd: "ssh", pkgs: map[string]string{
		"apt-get": "openssh-client", "apk": "openssh-client",
		"dnf": "openssh-clients", "yum": "openssh-clients", "zypper": "openssh-clients",
		"pacman": "openssh",
	}},
}

func missingTools() []tool {
	var missing []tool
	for _, t := range neededTools {
		if _, err := exec.LookPath(t.cmd); err != nil {
			missing = append(missing, t)
		}
	}
	return missing
}

func toolNames(tools []tool) string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.cmd
	}
	return strings.Join(names, " and ")
}

type pkgManager struct {
	name    string
	refresh []string // run first, so a fresh server finds the packages
	install []string // the packages are appended
}

var pkgManagers = []pkgManager{
	{name: "apt-get", refresh: []string{"apt-get", "update", "-q"}, install: []string{"apt-get", "install", "-y", "-q"}},
	{name: "dnf", install: []string{"dnf", "install", "-y"}},
	{name: "yum", install: []string{"yum", "install", "-y"}},
	{name: "zypper", install: []string{"zypper", "--non-interactive", "install"}},
	{name: "pacman", install: []string{"pacman", "-S", "--noconfirm", "--needed"}},
	{name: "apk", install: []string{"apk", "add"}},
}

func findPkgManager() *pkgManager {
	for i, pm := range pkgManagers {
		if _, err := exec.LookPath(pm.name); err == nil {
			return &pkgManagers[i]
		}
	}
	return nil
}

func (pm *pkgManager) add(tools []tool) error {
	args := slices.Clone(pm.install[1:])
	for _, t := range tools {
		args = append(args, cmp.Or(t.pkgs[pm.name], t.cmd))
	}
	os.Setenv("DEBIAN_FRONTEND", "noninteractive")
	if pm.refresh != nil {
		if err := run(pm.refresh[0], pm.refresh[1:]...); err != nil {
			return err
		}
	}
	return run(pm.install[0], args...)
}
