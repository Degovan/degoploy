// Command degoploy runs the webhook deploy server and its admin CLI.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"

	"golang.org/x/term"

	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/config"
	"github.com/degovan/degoploy/internal/server"
	"github.com/degovan/degoploy/internal/store"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

const usage = `degoploy — webhook deploy runner

Usage:
  degoploy [serve] [--config FILE]           run the server (default)
  degoploy user create --username NAME [--role admin|deployer|viewer] [--email E]
  degoploy user passwd --username NAME
  degoploy user list
  degoploy backup [--output FILE.tar.gz] [--no-logs]
  degoploy config check
  degoploy version

Global flag --config may also be set with DEGOPLOY_CONFIG.
`

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}

	var err error
	switch cmd {
	case "serve":
		err = runServe(args)
	case "user":
		err = runUser(args)
	case "backup":
		err = runBackup(args)
	case "config":
		err = runConfig(args)
	case "version":
		fmt.Println(version)
	case "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func configFlag(fs *flag.FlagSet) *string {
	return fs.String("config", os.Getenv("DEGOPLOY_CONFIG"), "path to degoploy.yaml")
}

func openStore(ctx context.Context, cfgPath string) (*config.Config, *store.Store, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, nil, err
	}
	if err := server.EnsureDataDir(cfg.DataDir); err != nil {
		return nil, nil, err
	}
	st, err := store.Open(ctx, cfg.DBPath())
	if err != nil {
		return nil, nil, err
	}
	return &cfg, st, nil
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	cfgPath := configFlag(fs)
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
		slog.Warn("no users yet; create an admin with: degoploy user create --username admin --role admin")
	}

	slog.Info("starting degoploy", "version", version)
	return server.New(cfg, st, version).Run(ctx)
}

func runUser(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: degoploy user create|passwd|list")
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("user "+sub, flag.ExitOnError)
	cfgPath := configFlag(fs)
	username := fs.String("username", "", "username")
	role := fs.String("role", string(store.RoleAdmin), "role: admin, deployer or viewer")
	email := fs.String("email", "", "email address")
	fs.Parse(args)

	ctx := context.Background()
	_, st, err := openStore(ctx, *cfgPath)
	if err != nil {
		return err
	}
	defer st.Close()

	switch sub {
	case "create":
		if *username == "" {
			return errors.New("--username is required")
		}
		r := store.Role(*role)
		if !r.Valid() {
			return fmt.Errorf("invalid role %q", *role)
		}
		pw, err := readNewPassword()
		if err != nil {
			return err
		}
		hash, err := auth.HashPassword(pw)
		if err != nil {
			return err
		}
		u := &store.User{
			Username:     *username,
			Email:        *email,
			DisplayName:  *username,
			AuthProvider: store.ProviderLocal,
			PasswordHash: hash,
			Role:         r,
		}
		if err := st.CreateUser(ctx, u); err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				return fmt.Errorf("user %q already exists", *username)
			}
			return err
		}
		fmt.Printf("created user %q (role %s)\n", u.Username, u.Role)

	case "passwd":
		u, err := st.UserByUsername(ctx, *username)
		if err != nil {
			return fmt.Errorf("user %q: %w", *username, err)
		}
		if u.AuthProvider != store.ProviderLocal {
			return fmt.Errorf("user %q signs in via %s and has no local password", u.Username, u.AuthProvider)
		}
		pw, err := readNewPassword()
		if err != nil {
			return err
		}
		hash, err := auth.HashPassword(pw)
		if err != nil {
			return err
		}
		if err := st.SetUserPassword(ctx, u.ID, hash); err != nil {
			return err
		}
		if err := st.DeleteUserSessions(ctx, u.ID); err != nil {
			return err
		}
		fmt.Printf("password updated for %q; existing sessions revoked\n", u.Username)

	case "list":
		users, err := st.ListUsers(ctx)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tUSERNAME\tROLE\tPROVIDER\tDISABLED")
		for _, u := range users {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%v\n", u.ID, u.Username, u.Role, u.AuthProvider, u.Disabled)
		}
		tw.Flush()

	default:
		return fmt.Errorf("unknown user subcommand %q", sub)
	}
	return nil
}

// readNewPassword prompts twice on a terminal, or reads one line from piped stdin.
func readNewPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("read password from stdin: %w", err)
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fmt.Fprint(os.Stderr, "Password: ")
	p1, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "Confirm:  ")
	p2, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if string(p1) != string(p2) {
		return "", errors.New("passwords do not match")
	}
	return string(p1), nil
}
