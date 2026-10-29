package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/degovan/degoploy/internal/admin"
	"github.com/degovan/degoploy/internal/toolchain"
)

// runtimeCmd is "degoploy node" or "degoploy php".
type runtimeCmd struct {
	name, title string
	example     string // a spec to show, such as "22"
	exact       string // an exact version, such as "20.11.1"
	line        string // what a line is called: "major version", "minor version"
	prefix      string // "v" for Node.js
}

var runtimeCmds = map[string]runtimeCmd{
	"node": {name: "node", title: "Node.js", example: "22", exact: "20.11.1", line: "major version", prefix: "v"},
	"php":  {name: "php", title: "PHP", example: "8.3", exact: "8.3.14", line: "minor version"},
}

func (rt runtimeCmd) usage() string {
	return "usage: degoploy " + rt.name + " list|install [VERSION]|remove [VERSION]|default [VERSION|none]|update|cancel [VERSION]"
}

// lineOf is a version's release line: "22" for Node.js, "8.3" for PHP.
func (rt runtimeCmd) lineOf(version string) string {
	parts := strings.SplitN(strings.TrimPrefix(version, "v"), ".", 3)
	if rt.name == "php" && len(parts) > 1 {
		return parts[0] + "." + parts[1]
	}
	return parts[0]
}

// runRuntime manages the versions of one runtime that deploy scripts run with.
func runRuntime(rt runtimeCmd, args []string) error {
	sub := "list"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet(rt.name+" "+sub, flag.ExitOnError)
	t := targetFlags(fs)
	fs.Parse(args)
	arg := strings.TrimSpace(fs.Arg(0))

	switch sub {
	case "list", "ls", "install", "remove", "rm", "uninstall", "default", "update", "cancel":
	default:
		return fmt.Errorf("unknown %s command %q\n%s", rt.name, sub, rt.usage())
	}
	ctx := context.Background()
	c, err := connect(ctx, t)
	if err != nil {
		return err
	}
	defer c.Close()

	switch sub {
	case "install":
		return nodeInstall(ctx, rt, c, arg)
	case "remove", "rm", "uninstall":
		return nodeRemove(ctx, rt, c, arg)
	case "default":
		return nodeDefault(ctx, rt, c, arg)
	case "update":
		return nodeUpdate(ctx, rt, c)
	case "cancel":
		return nodeCancel(ctx, rt, c, arg)
	}
	rep, err := c.RuntimeReport(ctx, rt.name)
	if err != nil {
		return err
	}
	printRuntimeReport(rt, rep)
	return nil
}

func printRuntimeReport(rt runtimeCmd, rep toolchain.Report) {
	if len(rep.Installed) == 0 {
		lipgloss.Println(muted.Render("No " + rt.title + " versions installed yet. Deployments install the version a project asks for,"))
		lipgloss.Println(muted.Render("or install one now with: degoploy " + rt.name + " install " + rt.example))
	} else {
		rows := make([][]string, 0, len(rep.Installed))
		for _, v := range rep.Installed {
			name := v.Version
			if v.Default {
				name += " " + green.Render("default")
			}
			if v.System {
				name += " " + muted.Render("system")
			}
			used := muted.Render("–")
			if len(v.UsedBy) > 0 {
				used = strings.Join(v.UsedBy, ", ")
			}
			if len(v.Pinned) > 0 {
				used += muted.Render("  (pinned)")
			}
			size := sizeLabel(v.Size)
			if v.System {
				size = muted.Render(v.Path)
			}
			if rt.name == "node" {
				rows = append(rows, []string{name, orDash(v.LTS), size, used})
			} else {
				rows = append(rows, []string{name, size, used})
			}
		}
		if rt.name == "node" {
			printTable([]string{"VERSION", "LTS", "SIZE", "USED BY"}, rows)
		} else {
			printTable([]string{"VERSION", "SIZE", "USED BY"}, rows)
		}
	}
	lipgloss.Println()
	source := ""
	if !rep.Verified {
		source = ", checksums for recent builds only"
	}
	if !rep.Enabled {
		lipgloss.Println(yellow.Render("!") + " " + rt.title + " is turned off: deployments do not use it. Turn it on on the Runtimes page.")
	}
	def := muted.Render("none; projects that name no version use the server's own")
	if rep.Default != "" {
		def = rep.Default
	}
	fields([][2]string{
		{"default", def},
		{"total", sizeLabel(rep.TotalSize)},
		{"source", rep.Mirror + muted.Render(" ("+rep.Platform+source+")")},
	})
}

func nodeInstall(ctx context.Context, rt runtimeCmd, c *admin.Client, spec string) error {
	if spec == "" {
		if !interactive() {
			return withHint("which version?", "For example: degoploy "+rt.name+" install "+rt.example+"   (or "+rt.exact+")")
		}
		spec = rt.example
		err := ask(huh.NewInput().Title(rt.title + " version to install").
			Description("A " + rt.line + " such as " + rt.example + " installs its newest release; " + rt.exact + " is exact.").
			Value(&spec).Validate(func(s string) error {
			_, err := toolchain.ParseSpec(s)
			return err
		}))
		if err != nil {
			return err
		}
	}
	var res admin.RuntimeInstallResult
	p := pollProgress(ctx, rt, c)
	defer p.stop()
	err := workingWith("Installing "+rt.title+" "+spec, p.status, func(ctx context.Context) error {
		var err error
		res, err = c.RuntimeInstall(ctx, rt.name, spec)
		return err
	})
	if err != nil {
		if isCanceled(err) {
			return errCanceled
		}
		return err
	}
	success("%s %s is installed", rt.title, bold.Render(res.Version))
	if res.Report.Default == "" {
		lipgloss.Println("  " + muted.Render("Make it the default with: degoploy "+rt.name+" default "+rt.lineOf(res.Version)))
	}
	return nil
}

func nodeRemove(ctx context.Context, rt runtimeCmd, c *admin.Client, version string) error {
	rep, err := c.RuntimeReport(ctx, rt.name)
	if err != nil {
		return err
	}
	if version == "" {
		if !interactive() {
			return withHint("which version?", "For example: degoploy "+rt.name+" remove "+rt.prefix+rt.exact)
		}
		if len(rep.Installed) == 0 {
			return errors.New("no " + rt.title + " versions are installed")
		}
		var opts []huh.Option[string]
		for _, v := range rep.Installed {
			label := v.Version
			if len(v.UsedBy) > 0 {
				label += muted.Render("  used by " + strings.Join(v.UsedBy, ", "))
			}
			opts = append(opts, huh.NewOption(label, v.Version))
		}
		if err := ask(huh.NewSelect[string]().Title("Remove which version?").Options(opts...).Value(&version)); err != nil {
			return err
		}
	}
	version = rt.prefix + strings.TrimPrefix(version, "v")
	for _, v := range rep.Installed {
		if v.Version == version && len(v.UsedBy) > 0 && interactive() {
			ok, err := confirm("Remove "+version+"?",
				strings.Join(v.UsedBy, ", ")+" deployed with it. Their next deployment installs it again if they still ask for it.", false)
			if err != nil {
				return err
			}
			if !ok {
				return errCanceled
			}
		}
	}
	if _, err := c.RuntimeRemove(ctx, rt.name, version); err != nil {
		return err
	}
	success("Removed %s %s", rt.title, bold.Render(version))
	return nil
}

func nodeDefault(ctx context.Context, rt runtimeCmd, c *admin.Client, spec string) error {
	if spec == "" {
		if !interactive() {
			return withHint("which version?", "For example: degoploy "+rt.name+" default "+rt.example+"   (none clears it)")
		}
		rep, err := c.RuntimeReport(ctx, rt.name)
		if err != nil {
			return err
		}
		spec = rep.Default
		opts := []huh.Option[string]{huh.NewOption("None "+muted.Render("use the server's node"), "")}
		seen := map[string]bool{}
		for _, v := range rep.Installed {
			major := rt.lineOf(v.Version)
			if !seen[major] {
				seen[major] = true
				opts = append(opts, huh.NewOption(rt.title+" "+major+muted.Render("  newest installed: "+v.Version), major))
			}
		}
		err = ask(huh.NewSelect[string]().Title("Default " + rt.title + " version").
			Description("For projects on Auto whose repository has no .nvmrc, .node-version or engines.node.").
			Options(opts...).Value(&spec))
		if err != nil {
			return err
		}
	}
	if strings.EqualFold(spec, "none") {
		spec = ""
	}
	rep, err := c.RuntimeDefault(ctx, rt.name, spec)
	if err != nil {
		return err
	}
	if rep.Default == "" {
		success("No default %s version; projects without one use the server's own", rt.title)
		return nil
	}
	success("The default %s version is %s", rt.title, bold.Render(rep.Default))
	return nil
}

func nodeUpdate(ctx context.Context, rt runtimeCmd, c *admin.Client) error {
	var res admin.RuntimeUpdateResult
	p := pollProgress(ctx, rt, c)
	defer p.stop()
	err := workingWith("Updating "+rt.title+" to the latest patch releases", p.status, func(ctx context.Context) error {
		var err error
		res, err = c.RuntimeUpdate(ctx, rt.name)
		return err
	})
	if err != nil {
		if isCanceled(err) {
			return errCanceled
		}
		return err
	}
	if len(res.Changes) == 0 {
		success("Every downloaded %s version is up to date", rt.title)
		return nil
	}
	for _, ch := range res.Changes {
		if ch.Removed {
			success("%s → %s", ch.From, bold.Render(ch.To))
		} else {
			success("Installed %s; kept %s, which a project pins", bold.Render(ch.To), ch.From)
		}
	}
	return nil
}

// progressPoller asks the service how far its downloads are, a few times a
// second. Ctrl+C meanwhile cancels the downloads it saw, rather than leaving
// them running on the server.
type progressPoller struct {
	line atomic.Value // string
	mu   sync.Mutex
	seen map[string]bool
	stop func()
}

func pollProgress(ctx context.Context, rt runtimeCmd, c *admin.Client) *progressPoller {
	p := &progressPoller{seen: map[string]bool{}}
	p.line.Store("")
	ctx, cancel := context.WithCancel(ctx)
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	p.stop = func() {
		signal.Stop(interrupt)
		cancel()
	}
	go func() {
		t := time.NewTicker(400 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-interrupt:
				p.line.Store("canceling")
				p.mu.Lock()
				for v := range p.seen {
					c.RuntimeCancel(context.Background(), rt.name, v)
				}
				p.mu.Unlock()
				continue
			case <-t.C:
			}
			rep, err := c.RuntimeReport(ctx, rt.name)
			if err != nil {
				continue
			}
			var parts []string
			p.mu.Lock()
			for _, in := range rep.Installing {
				p.seen[in.Version] = true
				s := in.Version + " " + toolchain.ProgressLabel(in.Done, in.Total)
				if in.Phase == "unpacking" {
					s = in.Version + " unpacking"
				}
				parts = append(parts, s)
			}
			p.mu.Unlock()
			if p.line.Load() != "canceling" {
				p.line.Store(strings.Join(parts, ", ") + muted.Render("   ctrl+c cancels"))
			}
		}
	}()
	return p
}

// isCanceled reports a refusal because the install was canceled.
func isCanceled(err error) bool {
	var e *admin.Error
	return errors.As(err, &e) && e.Code == "canceled"
}

func (p *progressPoller) status() string { return p.line.Load().(string) }

func nodeCancel(ctx context.Context, rt runtimeCmd, c *admin.Client, version string) error {
	if version == "" {
		rep, err := c.RuntimeReport(ctx, rt.name)
		if err != nil {
			return err
		}
		switch {
		case len(rep.Installing) == 0:
			return errors.New("no " + rt.title + " version is being installed")
		case len(rep.Installing) == 1:
			version = rep.Installing[0].Version
		case !interactive():
			return withHint("several versions are being installed; which one?", "For example: degoploy "+rt.name+" cancel "+rep.Installing[0].Version)
		default:
			var opts []huh.Option[string]
			for _, in := range rep.Installing {
				opts = append(opts, huh.NewOption(in.Version+muted.Render("  "+toolchain.ProgressLabel(in.Done, in.Total)), in.Version))
			}
			if err := ask(huh.NewSelect[string]().Title("Cancel which install?").Options(opts...).Value(&version)); err != nil {
				return err
			}
		}
	}
	version = rt.prefix + strings.TrimPrefix(version, "v")
	if err := c.RuntimeCancel(ctx, rt.name, version); err != nil {
		return err
	}
	success("Canceled the install of %s %s", rt.title, bold.Render(version))
	return nil
}

func sizeLabel(n int64) string {
	switch {
	case n <= 0:
		return "–"
	case n < 1<<20:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
}
