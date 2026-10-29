package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/degovan/degoploy/internal/admin"
)

// runStatus shows whether the service runs and what it serves.
func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	t := targetFlags(fs)
	fs.Parse(args)

	ctx := context.Background()
	c, err := connect(ctx, t)
	if err != nil {
		return err
	}
	defer c.Close()
	st, err := c.Status(ctx)
	if err != nil {
		return err
	}

	if st.Local {
		heading(yellow.Render("●")+" No running degoploy answered", "showing its data directly")
	} else {
		heading(green.Render("●")+" degoploy is running", "since "+ago(st.StartedAt)+", pid "+strconv.Itoa(st.PID))
	}
	ver := st.Version
	if st.Version != version {
		ver += muted.Render("  (this CLI is " + version + "; degoploy setup restarts the service on it)")
	}
	rows := [][2]string{
		{"version", ver},
		{"web", webAddress(st)},
		{"config", orDash(st.Config)},
		{"data", st.DataDir},
		{"users", fmt.Sprintf("%d (%d active %s)", st.Users, st.Admins, pluralWord(st.Admins, "admin"))},
		{"projects", strconv.Itoa(st.Projects)},
	}
	if c.Socket != "" {
		rows = append(rows, [2]string{"socket", c.Socket}, [2]string{"you", st.You})
	}
	fields(rows)
	if st.Admins == 0 {
		lipgloss.Println()
		warning("There is no active admin. Create one with: degoploy user create")
	}
	return nil
}

// runSetup installs degoploy on a fresh server, upgrades an install to this
// binary, or checks an install and creates the first admin. Installing needs
// root, so for others it continues through sudo.
func runSetup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	t := targetFlags(fs)
	o := installFlags(fs)
	fs.Parse(args)

	ctx := context.Background()
	s := inspectInstall()
	elsewhere := t.config != "" || t.socket != ""
	switch {
	case os.Geteuid() == 0 && !elsewhere:
		return install(ctx, s, o)
	case !s.config && !elsewhere:
		heading("degoploy "+version, "not installed on this server yet")
		if interactive() && !o.yes {
			lipgloss.Println()
			ok, err := confirm("Install degoploy now?",
				"It runs as a service under its own user. Installing needs root; sudo asks for your password.", true)
			if err != nil {
				return err
			}
			if !ok {
				return errCanceled
			}
		}
		return asRoot(s.exe, args)
	case !s.current && s.installed != version && !elsewhere && interactive() && !o.yes:
		from := ""
		if s.installed != "" {
			from = " " + s.installed
		}
		ok, err := confirm("Upgrade degoploy"+from+" to "+version+"?",
			"This binary replaces "+installBin+" and restarts the service. sudo asks for your password.", true)
		if err != nil {
			return err
		}
		if ok {
			return asRoot(s.exe, args)
		}
	}

	c, err := connect(ctx, t)
	if err != nil {
		return err
	}
	defer c.Close()
	st, err := c.Status(ctx)
	if err != nil {
		return err
	}

	// A new binary copied over the installed one runs only after a restart.
	if !st.Local && st.Version != version && s.current && !elsewhere && interactive() && !o.yes {
		ok, err := confirm("Restart degoploy on "+version+"?",
			st.Version+" is still running from before "+installBin+" was replaced. sudo asks for your password.", true)
		if err != nil {
			return err
		}
		if ok {
			c.Close()
			return asRoot(s.exe, args)
		}
	}

	heading("degoploy setup", st.Version)
	lipgloss.Println()
	if st.Local {
		warning("No running degoploy answered on its admin socket; start it with: sudo systemctl start degoploy")
	} else {
		success("The service is running and answers on %s", c.Socket)
		success("%s can manage degoploy without sudo", bold.Render(st.You))
	}
	if err := firstAdmin(ctx, c, st, o.admin, true); err != nil {
		return err
	}

	rep, err := c.CheckConfig(ctx)
	if err != nil {
		return err
	}
	if rep.Error != "" {
		return withHint("the configuration does not load: "+rep.Error, "Fix "+rep.Config+", then run: sudo systemctl restart degoploy")
	}
	for _, w := range rep.Warnings {
		warning("%s", w)
	}
	if st.BaseURL == "" {
		lipgloss.Println("  " + muted.Render("Set base_url in "+orDash(st.Config)+" to the public https:// address, then restart degoploy."))
	}

	lipgloss.Println()
	lipgloss.Println("Open " + blue.Render(webAddress(st)) + " and sign in.")
	return nil
}

// runBackup writes a .tar.gz of the data directory: a consistent snapshot of the
// database plus deploy keys, managed scripts, known_hosts and (optionally) logs.
// Restore by stopping degoploy and extracting the archive into data_dir.
func runBackup(args []string) error {
	flags := flag.NewFlagSet("backup", flag.ExitOnError)
	t := targetFlags(flags)
	output := flags.String("output", "", "archive to write (default degoploy-backup-<time>.tar.gz)")
	noLogs := flags.Bool("no-logs", false, "skip deployment logs")
	flags.Parse(args)

	ctx := context.Background()
	c, err := connect(ctx, t)
	if err != nil {
		return err
	}
	defer c.Close()

	if *output == "" {
		*output = "degoploy-backup-" + time.Now().Format("20060102-150405") + ".tar.gz"
	}
	// The archive holds deploy keys and the secret store: only its owner may read it.
	f, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s already exists", *output)
	}
	if err != nil {
		return err
	}
	n, err := c.Backup(ctx, f, !*noLogs)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(*output)
		return err
	}
	success("Wrote %s (%s)", bold.Render(*output), plural(n, "file"))
	lipgloss.Println("  " + muted.Render("It contains deploy keys and secrets; keep it somewhere safe."))
	return nil
}

// runConfig validates the configuration and, with SSO enabled, the provider's
// discovery. Without --config it checks the running service's own file, with
// the environment the service sees.
func runConfig(args []string) error {
	if len(args) == 0 || args[0] != "check" {
		return errors.New("usage: degoploy config check [--config FILE]")
	}
	flags := flag.NewFlagSet("config check", flag.ExitOnError)
	t := targetFlags(flags)
	flags.Parse(args[1:])

	ctx := context.Background()
	var rep admin.Report
	c, err := dialService(t)
	switch {
	case err != nil && t.config == "":
		return err
	case c != nil && t.config == "":
		defer c.Close()
		if rep, err = c.CheckConfig(ctx); err != nil {
			return err
		}
	default:
		if c != nil {
			c.Close()
		}
		rep = admin.Check(ctx, t.configPath())
	}

	if rep.Error != "" {
		return withHint("invalid configuration in "+rep.Config, strings.Split(rep.Error, "\n")...)
	}
	heading("Configuration", rep.Config)
	fields(rep.Values)
	for _, w := range rep.Warnings {
		warning("%s", w)
	}
	success("Configuration is valid")
	return nil
}

// webAddress is where people open degoploy: base_url, or the listen address.
func webAddress(st admin.Status) string {
	if st.BaseURL != "" {
		return st.BaseURL
	}
	host, port, err := net.SplitHostPort(st.Listen)
	if err != nil {
		return st.Listen
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func orDash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}

func pluralWord(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
