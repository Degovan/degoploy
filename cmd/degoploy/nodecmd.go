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
	"github.com/degovan/degoploy/internal/node"
)

const nodeUsage = "usage: degoploy node list|install [VERSION]|remove [VERSION]|default [VERSION|none]|update|cancel [VERSION]"

// runNode manages the Node.js versions deploy scripts run with.
func runNode(args []string) error {
	sub := "list"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("node "+sub, flag.ExitOnError)
	t := targetFlags(fs)
	fs.Parse(args)
	arg := strings.TrimSpace(fs.Arg(0))

	switch sub {
	case "list", "ls", "install", "remove", "rm", "uninstall", "default", "update", "cancel":
	default:
		return fmt.Errorf("unknown node command %q\n%s", sub, nodeUsage)
	}
	ctx := context.Background()
	c, err := connect(ctx, t)
	if err != nil {
		return err
	}
	defer c.Close()

	switch sub {
	case "install":
		return nodeInstall(ctx, c, arg)
	case "remove", "rm", "uninstall":
		return nodeRemove(ctx, c, arg)
	case "default":
		return nodeDefault(ctx, c, arg)
	case "update":
		return nodeUpdate(ctx, c)
	case "cancel":
		return nodeCancel(ctx, c, arg)
	}
	rep, err := c.NodeReport(ctx)
	if err != nil {
		return err
	}
	printNodeReport(rep)
	return nil
}

func printNodeReport(rep node.Report) {
	if len(rep.Installed) == 0 {
		lipgloss.Println(muted.Render("No Node.js versions installed yet. Deployments install the version a project asks for,"))
		lipgloss.Println(muted.Render("or install one now with: degoploy node install lts"))
	} else {
		rows := make([][]string, 0, len(rep.Installed))
		for _, v := range rep.Installed {
			name := v.Version
			if v.Default {
				name += " " + green.Render("default")
			}
			used := muted.Render("–")
			if len(v.UsedBy) > 0 {
				used = strings.Join(v.UsedBy, ", ")
			}
			if len(v.Pinned) > 0 {
				used += muted.Render("  (pinned)")
			}
			rows = append(rows, []string{name, orDash(v.LTS), sizeLabel(v.Size), used})
		}
		printTable([]string{"VERSION", "LTS", "SIZE", "USED BY"}, rows)
	}
	lipgloss.Println()
	def := muted.Render("none; projects without a version in their repository use the server's node")
	if rep.Default != "" {
		def = rep.Default
	}
	fields([][2]string{
		{"default", def},
		{"total", sizeLabel(rep.TotalSize)},
		{"source", rep.Mirror + muted.Render(" ("+rep.Platform+")")},
	})
}

func nodeInstall(ctx context.Context, c *admin.Client, spec string) error {
	if spec == "" {
		if !interactive() {
			return withHint("which version?", "For example: degoploy node install 22   (or lts, 20.11.1)")
		}
		spec = "lts"
		err := ask(huh.NewInput().Title("Node.js version to install").
			Description("A major version such as 22, an exact one such as 20.11.1, or lts for the newest LTS.").
			Value(&spec).Validate(func(s string) error {
			_, err := node.ParseSpec(s)
			return err
		}))
		if err != nil {
			return err
		}
	}
	var res admin.NodeInstallResult
	p := pollProgress(ctx, c)
	defer p.stop()
	err := workingWith("Installing Node.js "+spec, p.status, func(ctx context.Context) error {
		var err error
		res, err = c.NodeInstall(ctx, spec)
		return err
	})
	if err != nil {
		if isCanceled(err) {
			return errCanceled
		}
		return err
	}
	success("Node.js %s is installed", bold.Render(res.Version))
	if res.Report.Default == "" {
		major := strings.SplitN(strings.TrimPrefix(res.Version, "v"), ".", 2)[0]
		lipgloss.Println("  " + muted.Render("Make it the default with: degoploy node default "+major))
	}
	return nil
}

func nodeRemove(ctx context.Context, c *admin.Client, version string) error {
	rep, err := c.NodeReport(ctx)
	if err != nil {
		return err
	}
	if version == "" {
		if !interactive() {
			return withHint("which version?", "For example: degoploy node remove v20.19.0")
		}
		if len(rep.Installed) == 0 {
			return errors.New("no Node.js versions are installed")
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
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
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
	if _, err := c.NodeRemove(ctx, version); err != nil {
		return err
	}
	success("Removed Node.js %s", bold.Render(version))
	return nil
}

func nodeDefault(ctx context.Context, c *admin.Client, spec string) error {
	if spec == "" {
		if !interactive() {
			return withHint("which version?", "For example: degoploy node default 22   (none clears it)")
		}
		rep, err := c.NodeReport(ctx)
		if err != nil {
			return err
		}
		spec = rep.Default
		opts := []huh.Option[string]{huh.NewOption("None "+muted.Render("use the server's node"), "")}
		seen := map[string]bool{}
		for _, v := range rep.Installed {
			major := strings.SplitN(strings.TrimPrefix(v.Version, "v"), ".", 2)[0]
			if !seen[major] {
				seen[major] = true
				opts = append(opts, huh.NewOption("Node.js "+major+muted.Render("  newest installed: "+v.Version), major))
			}
		}
		err = ask(huh.NewSelect[string]().Title("Default Node.js version").
			Description("For projects on Auto whose repository has no .nvmrc, .node-version or engines.node.").
			Options(opts...).Value(&spec))
		if err != nil {
			return err
		}
	}
	if strings.EqualFold(spec, "none") {
		spec = ""
	}
	rep, err := c.NodeDefault(ctx, spec)
	if err != nil {
		return err
	}
	if rep.Default == "" {
		success("No default Node.js version; projects without one use the server's node")
		return nil
	}
	success("The default Node.js version is %s", bold.Render(rep.Default))
	return nil
}

func nodeUpdate(ctx context.Context, c *admin.Client) error {
	var res admin.NodeUpdateResult
	p := pollProgress(ctx, c)
	defer p.stop()
	err := workingWith("Updating Node.js to the latest patch releases", p.status, func(ctx context.Context) error {
		var err error
		res, err = c.NodeUpdate(ctx)
		return err
	})
	if err != nil {
		if isCanceled(err) {
			return errCanceled
		}
		return err
	}
	if len(res.Changes) == 0 {
		success("Every installed Node.js version is up to date")
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

func pollProgress(ctx context.Context, c *admin.Client) *progressPoller {
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
					c.NodeCancel(context.Background(), v)
				}
				p.mu.Unlock()
				continue
			case <-t.C:
			}
			rep, err := c.NodeReport(ctx)
			if err != nil {
				continue
			}
			var parts []string
			p.mu.Lock()
			for _, in := range rep.Installing {
				p.seen[in.Version] = true
				s := in.Version + " " + node.ProgressLabel(in.Done, in.Total)
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

func nodeCancel(ctx context.Context, c *admin.Client, version string) error {
	if version == "" {
		rep, err := c.NodeReport(ctx)
		if err != nil {
			return err
		}
		switch {
		case len(rep.Installing) == 0:
			return errors.New("no Node.js version is being installed")
		case len(rep.Installing) == 1:
			version = rep.Installing[0].Version
		case !interactive():
			return withHint("several versions are being installed; which one?", "For example: degoploy node cancel "+rep.Installing[0].Version)
		default:
			var opts []huh.Option[string]
			for _, in := range rep.Installing {
				opts = append(opts, huh.NewOption(in.Version+muted.Render("  "+node.ProgressLabel(in.Done, in.Total)), in.Version))
			}
			if err := ask(huh.NewSelect[string]().Title("Cancel which install?").Options(opts...).Value(&version)); err != nil {
				return err
			}
		}
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	if err := c.NodeCancel(ctx, version); err != nil {
		return err
	}
	success("Canceled the install of Node.js %s", bold.Render(version))
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
