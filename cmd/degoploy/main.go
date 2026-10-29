// Command degoploy runs the webhook deploy server and its admin CLI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"charm.land/lipgloss/v2"

	"github.com/degovan/degoploy/internal/config"
	"github.com/degovan/degoploy/internal/secrets"
	"github.com/degovan/degoploy/internal/server"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func usage() string {
	cmd := func(c, desc string) string { return "  " + blue.Render(fmt.Sprintf("%-40s", c)) + desc + "\n" }
	return bold.Render("degoploy") + " — webhook deploy runner " + muted.Render(version) + "\n\n" +
		bold.Render("Install") + "\n" +
		cmd("setup [--yes] [--base-url URL]", "install or upgrade degoploy on this server, then create") +
		cmd("", "the first admin; asks for sudo when needed") +
		cmd("update [--version V] [--check]", "download the latest release and restart on it") +
		"\n" + bold.Render("Manage") + muted.Render("  as root or a member of the degoploy group, no sudo needed") + "\n" +
		cmd("status", "is the service running, and where") +
		cmd("user", "pick a user action from a menu") +
		cmd("user list", "list users") +
		cmd("user create [--username N] [--role R]", "add a user with a password") +
		cmd("user passwd [--username N]", "set a user's password") +
		cmd("user role [--username N] [--role R]", "change a user's role") +
		cmd("user disable|enable [--username N]", "block or allow sign-in") +
		cmd("config check [--config FILE]", "validate the configuration") +
		cmd("backup [--output FILE] [--no-logs]", "archive the data directory") +
		"\n" + bold.Render("Run") + "\n" +
		cmd("serve [--config FILE] [--debug]", "run the server (what systemd runs)") +
		cmd("version", "print the version") +
		"\n" + muted.Render("Missing flags are asked for in a terminal. Scripts pass flags and pipe passwords on stdin.") + "\n" +
		muted.Render("Run without a command in a terminal, degoploy installs itself or shows its status.") + "\n" +
		muted.Render("--config (DEGOPLOY_CONFIG) defaults to "+defaultConfig+" when present,") + "\n" +
		muted.Render("--socket (DEGOPLOY_SOCKET) to "+defaultSocket+".") + "\n"
}

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	// Typed in a terminal, plain "degoploy" installs itself the first time and
	// shows the status after that. systemd and scripts run "serve" explicitly.
	bare := len(os.Args) == 1 && interactive() && os.Getenv("DEGOPLOY_CONFIG") == ""
	if bare {
		cmd = "status"
		if !installedHere() {
			cmd = "setup"
		}
	}
	if cmd == "serve" && len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		cmd = "help"
	}

	var err error
	switch cmd {
	case "serve":
		err = runServe(args)
	case "setup":
		err = runSetup(args)
	case "update":
		err = runUpdate(args)
	case "status":
		err = runStatus(args)
		if bare && err == nil {
			lipgloss.Println("\n" + muted.Render("degoploy help lists every command."))
		}
	case "user", "users":
		err = runUser(args)
	case "backup":
		err = runBackup(args)
	case "config":
		err = runConfig(args)
	case "version":
		fmt.Println(version)
	case "help":
		lipgloss.Print(usage())
	default:
		printError(withHint(fmt.Sprintf("unknown command %q", cmd), "Run degoploy help for the list."))
		os.Exit(2)
	}
	switch {
	case errors.Is(err, errCanceled):
		lipgloss.Fprintln(os.Stderr, muted.Render("Canceled."))
		os.Exit(130)
	case err != nil:
		printError(err)
		os.Exit(1)
	}
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	cfgPath := fs.String("config", os.Getenv("DEGOPLOY_CONFIG"), "path to degoploy.yaml")
	debug := fs.Bool("debug", os.Getenv("DEGOPLOY_DEBUG") != "", "verbose logging")
	fs.Parse(args)

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, st, err := openStore(ctx, *cfgPath)
	if err != nil {
		return err
	}
	defer st.Close()

	if n, err := st.CountUsers(ctx); err == nil && n == 0 {
		slog.Warn("no users yet; create the first admin with: degoploy setup")
	}

	key, err := secrets.LoadKey(cfg.SecretKey, cfg.SecretKeyPath())
	if err != nil {
		return fmt.Errorf("secret key: %w", err)
	}
	box, err := secrets.New(key)
	if err != nil {
		return fmt.Errorf("secret key: %w", err)
	}
	// The SSO and Email pages write secrets encrypted with this key.
	if sealed, ok := strings.CutPrefix(cfg.OIDC.ClientSecret, config.SealedPrefix); ok {
		if cfg.OIDC.ClientSecret, err = box.Open(sealed, config.OIDCSecretScope); err != nil {
			return fmt.Errorf("oidc client_secret: %w", err)
		}
	}
	if sealed, ok := strings.CutPrefix(cfg.SMTP.Password, config.SealedPrefix); ok {
		if cfg.SMTP.Password, err = box.Open(sealed, config.SMTPPasswordScope); err != nil {
			return fmt.Errorf("smtp password: %w", err)
		}
	}

	slog.Info("starting degoploy", "version", version)
	return server.New(cfg, st, box, version).Run(ctx)
}
