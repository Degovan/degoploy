package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/config"
)

// runBackup writes a .tar.gz of the data directory: a consistent snapshot of the
// database plus deploy keys, managed scripts, known_hosts and (optionally) logs.
// Restore by stopping degoploy and extracting the archive into data_dir.
func runBackup(args []string) error {
	flags := flag.NewFlagSet("backup", flag.ExitOnError)
	cfgPath := configFlag(flags)
	output := flags.String("output", "", "archive to write (default degoploy-backup-<time>.tar.gz)")
	noLogs := flags.Bool("no-logs", false, "skip deployment logs")
	flags.Parse(args)

	ctx := context.Background()
	cfg, st, err := openStore(ctx, *cfgPath)
	if err != nil {
		return err
	}
	defer st.Close()

	if *output == "" {
		*output = "degoploy-backup-" + time.Now().Format("20060102-150405") + ".tar.gz"
	}
	if _, err := os.Stat(*output); err == nil {
		return fmt.Errorf("%s already exists", *output)
	}

	tmp, err := os.MkdirTemp("", "degoploy-backup-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	snapshot := filepath.Join(tmp, "degoploy.db")
	if err := st.Backup(ctx, snapshot); err != nil {
		return fmt.Errorf("snapshot database: %w", err)
	}
	if err := os.Chmod(snapshot, 0o600); err != nil {
		return err
	}

	f, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(*output)
		}
	}()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	if err := addFile(tw, snapshot, "degoploy.db"); err != nil {
		return err
	}
	files := 1
	err = filepath.WalkDir(cfg.DataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(cfg.DataDir, path)
		top := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
		switch {
		case rel == ".":
			return nil
		case top != "projects" && top != "known_hosts":
			// Only degoploy's own state; the live database (and its WAL) is replaced by
			// the snapshot, and earlier backups stored here are not nested into new ones.
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		case *noLogs && d.IsDir() && d.Name() == "logs":
			return filepath.SkipDir
		case d.IsDir():
			return addDir(tw, path, rel)
		case !d.Type().IsRegular():
			return nil
		}
		files++
		return addFile(tw, path, rel)
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	ok = true
	fmt.Printf("wrote %s (%d files from %s)\n", *output, files, cfg.DataDir)
	return nil
}

func addDir(tw *tar.Writer, path, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	hdr.Name = filepath.ToSlash(name) + "/"
	return tw.WriteHeader(hdr)
}

func addFile(tw *tar.Writer, path, name string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	hdr.Name = filepath.ToSlash(name)
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

// runConfig validates the configuration and, with SSO enabled, the provider's discovery.
func runConfig(args []string) error {
	if len(args) == 0 || args[0] != "check" {
		return errors.New("usage: degoploy config check [--config FILE]")
	}
	flags := flag.NewFlagSet("config check", flag.ExitOnError)
	cfgPath := configFlag(flags)
	flags.Parse(args[1:])

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}
	src := *cfgPath
	if src == "" {
		src = "(defaults + environment)"
	}
	fmt.Printf("config:       %s\n", src)
	fmt.Printf("listen:       %s\n", cfg.Listen)
	fmt.Printf("data_dir:     %s\n", cfg.DataDir)
	fmt.Printf("base_url:     %s\n", orNone(cfg.BaseURL))
	fmt.Printf("local_login:  %v\n", cfg.LocalLogin)
	fmt.Printf("max_parallel: %d\n", cfg.MaxParallel)

	var warnings []string
	if cfg.BaseURL == "" {
		warnings = append(warnings, "base_url is empty: webhook URLs are derived from the browser address and cookies are not marked Secure")
	} else if !cfg.SecureCookies() {
		warnings = append(warnings, "base_url is not https: session cookies are not marked Secure")
	}
	if st, err := os.Stat(cfg.DataDir); err == nil && st.Mode().Perm()&0o077 != 0 {
		warnings = append(warnings, fmt.Sprintf("data_dir permissions are %v; deploy keys live there, use 0700", st.Mode().Perm()))
	}

	if cfg.OIDC.Enabled {
		fmt.Printf("oidc:         %s (%s)\n", cfg.OIDC.ProviderName, cfg.OIDC.Issuer)
		if cfg.OIDC.ClientSecret == "" {
			warnings = append(warnings, "oidc.client_secret is empty (is the environment variable set?)")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		// AuthURL performs discovery; the state values are throwaway.
		if _, err := auth.NewOIDCProvider(cfg.OIDC).AuthURL(ctx, "check", "check", "check-verifier-check-verifier-check-verifier"); err != nil {
			warnings = append(warnings, "oidc discovery failed: "+err.Error())
		} else {
			fmt.Println("oidc discovery: ok")
		}
	} else {
		fmt.Println("oidc:         disabled")
	}

	for _, w := range warnings {
		fmt.Println("warning:", w)
	}
	fmt.Println("configuration is valid")
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "(not set)"
	}
	return s
}
