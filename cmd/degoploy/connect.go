package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/degovan/degoploy/internal/admin"
	"github.com/degovan/degoploy/internal/audit"
	"github.com/degovan/degoploy/internal/config"
	"github.com/degovan/degoploy/internal/server"
	"github.com/degovan/degoploy/internal/store"
)

// Where install.sh puts things.
const (
	defaultConfig = "/etc/degoploy/degoploy.yaml"
	defaultSocket = "/run/degoploy/admin.sock"
)

// target says where the CLI finds degoploy.
type target struct {
	config string
	socket string
}

func targetFlags(fs *flag.FlagSet) *target {
	t := &target{}
	fs.StringVar(&t.config, "config", os.Getenv("DEGOPLOY_CONFIG"), "path to degoploy.yaml (default "+defaultConfig+" when present)")
	fs.StringVar(&t.socket, "socket", os.Getenv("DEGOPLOY_SOCKET"), "admin socket of the running service (default "+defaultSocket+")")
	return t
}

func (t *target) configPath() string {
	if t.config != "" {
		return t.config
	}
	if _, err := os.Stat(defaultConfig); err == nil {
		return defaultConfig
	}
	return ""
}

// connect reaches the running service through its admin socket. With no
// service running it opens the data directly, which needs the service's own
// file access (root or the degoploy user).
func connect(ctx context.Context, t *target) (*admin.Client, error) {
	c, err := dialService(t)
	if c != nil || err != nil {
		return c, err
	}
	if t.socket != "" {
		return nil, withHint("cannot reach "+t.socket, "Is degoploy running? Check with: systemctl status degoploy")
	}
	return openLocal(ctx, t.configPath())
}

// dialService connects to the running service's admin socket; nil without error
// means no service is running.
func dialService(t *target) (*admin.Client, error) {
	type candidate struct {
		path string
		// known: a denied connection means the user is missing the group, not
		// just that the path sits in a directory they cannot read.
		known bool
	}
	var cands []candidate
	if t.socket != "" {
		cands = append(cands, candidate{t.socket, true})
	} else {
		cands = append(cands, candidate{defaultSocket, true})
		if cfg, err := config.Load(t.configPath()); err == nil && cfg.AdminSocket != "off" && cfg.AdminSocket != defaultSocket {
			cands = append(cands, candidate{cfg.AdminSocket, false})
		}
	}
	for _, c := range cands {
		cl, err := admin.Dial(c.path)
		if err == nil {
			return cl, nil
		}
		if c.known && errors.Is(err, fs.ErrPermission) {
			return nil, accessError(c.path)
		}
	}
	return nil, nil
}

// openLocal serves the admin API in-process on the data directory.
func openLocal(ctx context.Context, cfgPath string) (*admin.Client, error) {
	cfg, st, err := openStore(ctx, cfgPath)
	if errors.Is(err, fs.ErrPermission) {
		me := admin.CurrentPeer().Name
		return nil, withHint(
			"degoploy is not running, and "+me+" cannot open its data directly",
			"Start the service:     sudo systemctl start degoploy",
			"or run as its user:    sudo -u degoploy "+commandLine(),
		)
	}
	if err != nil {
		return nil, err
	}
	h := admin.NewHandler(admin.Deps{
		Config: cfg, Store: st, Audit: audit.New(st), Version: version, Started: time.Now(), Local: true,
	})
	return admin.InProcess(h, func() { st.Close() }), nil
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

// accessError explains a socket the user may not open: usually a missing
// group, sometimes a group added after this login session started.
func accessError(sock string) error {
	group := "degoploy"
	var gid uint32
	if st, err := os.Stat(filepath.Dir(sock)); err == nil {
		if sys, ok := st.Sys().(*syscall.Stat_t); ok {
			gid = sys.Gid
			if g, err := user.LookupGroupId(strconv.Itoa(int(gid))); err == nil {
				group = g.Name
			}
		}
	}
	me, _ := user.Current()
	name := admin.CurrentPeer().Name
	if me != nil && gid != 0 {
		ids, _ := me.GroupIds()
		session, _ := os.Getgroups()
		if slices.Contains(ids, strconv.Itoa(int(gid))) && !slices.Contains(session, int(gid)) {
			return withHint(
				name+" is in the "+group+" group, but this login session started before that",
				"Log out and back in, or start a shell with the group:  newgrp "+group,
			)
		}
	}
	return withHint(
		name+" may not manage degoploy (no access to "+sock+")",
		"Add yourself to the "+group+" group, then log out and back in:",
		"  sudo usermod -aG "+group+" "+name,
	)
}

func commandLine() string {
	s := "degoploy"
	for _, a := range os.Args[1:] {
		if a == "" || strings.ContainsAny(a, " \t'\"$\\") {
			a = strconv.Quote(a)
		}
		s += " " + a
	}
	return s
}
