package toolchain

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/degovan/degoploy/internal/toolchain/toolchaintest"
)

func TestRange(t *testing.T) {
	for _, c := range []struct {
		r     string
		in    []string
		notIn []string
	}{
		{"22", []string{"22.0.0", "22.11.0"}, []string{"21.9.9", "23.0.0"}},
		{"v20.11.1", []string{"20.11.1"}, []string{"20.11.0", "20.11.2"}},
		{"20.x", []string{"20.0.0", "20.19.5"}, []string{"21.0.0"}},
		{"18.17", []string{"18.17.0", "18.17.9"}, []string{"18.18.0"}},
		{"^18.17", []string{"18.17.0", "18.20.4"}, []string{"18.16.9", "19.0.0"}},
		{"~20.11.1", []string{"20.11.1", "20.11.9"}, []string{"20.12.0"}},
		{">=18 <21", []string{"18.0.0", "20.19.0"}, []string{"17.9.0", "21.0.0"}},
		{">= 18", []string{"18.0.0", "24.1.0"}, []string{"16.20.2"}},
		{">18", []string{"19.0.0"}, []string{"18.20.0"}},
		{"<=20", []string{"20.19.0"}, []string{"21.0.0"}},
		{"18 - 20", []string{"18.0.0", "20.19.0"}, []string{"21.0.0"}},
		{"20 || 22", []string{"20.1.0", "22.1.0"}, []string{"21.1.0"}},
		{"*", []string{"0.10.0", "24.0.0"}, nil},
	} {
		r, err := ParseRange(c.r)
		if err != nil {
			t.Fatalf("ParseRange(%q): %v", c.r, err)
		}
		for _, s := range c.in {
			v, _ := ParseVersion(s)
			if !r.Match(v) {
				t.Errorf("%q should match %s", c.r, s)
			}
		}
		for _, s := range c.notIn {
			v, _ := ParseVersion(s)
			if r.Match(v) {
				t.Errorf("%q should not match %s", c.r, s)
			}
		}
	}
	for _, bad := range []string{"twenty", "1.2.3.4", "abc || 2"} {
		if _, err := ParseRange(bad); err == nil {
			t.Errorf("ParseRange(%q) accepted", bad)
		}
	}
}

func TestParseSpec(t *testing.T) {
	for in, want := range map[string]Spec{
		"lts/*":    {Alias: "lts"},
		"LTS":      {Alias: "lts"},
		"lts/Iron": {Alias: "lts", Codename: "iron"},
		"node":     {Alias: "latest"},
	} {
		got, err := ParseSpec(in)
		if err != nil || got.Alias != want.Alias || got.Codename != want.Codename {
			t.Errorf("ParseSpec(%q) = %+v, %v", in, got, err)
		}
	}
	if _, err := ParseSpec("not a version"); err == nil {
		t.Error("garbage accepted")
	}
}

func TestDetectNode(t *testing.T) {
	dir := t.TempDir()
	var n NodeJS
	if s, _, uses := n.Detect(dir); s != "" || uses {
		t.Fatalf("empty dir gave %q %v", s, uses)
	}
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"app"}`), 0o644)
	if s, _, uses := n.Detect(dir); s != "" || !uses {
		t.Fatalf("package.json without a version: %q %v", s, uses)
	}
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"engines":{"node":">=20"}}`), 0o644)
	if s, src, _ := n.Detect(dir); s != ">=20" || src != "package.json engines.node" {
		t.Fatalf("engines: %q %q", s, src)
	}
	os.WriteFile(filepath.Join(dir, ".node-version"), []byte("v20.11.1\n"), 0o644)
	if s, src, _ := n.Detect(dir); s != "v20.11.1" || src != ".node-version" {
		t.Fatalf(".node-version: %q %q", s, src)
	}
	os.WriteFile(filepath.Join(dir, ".nvmrc"), []byte("# pinned\n\nlts/iron # for now\n"), 0o644)
	if s, src, _ := n.Detect(dir); s != "lts/iron" || src != ".nvmrc" {
		t.Fatalf(".nvmrc: %q %q", s, src)
	}
}

func v(s string) Version {
	x, err := ParseVersion(s)
	if err != nil {
		panic(err)
	}
	return x
}

func TestNodeInstallResolveUpdate(t *testing.T) {
	ctx := context.Background()
	mirror := toolchaintest.NewNodeMirror(t,
		toolchaintest.Release{Version: "v24.1.0", Date: "2026-05-01"},
		toolchaintest.Release{Version: "v22.12.0", LTS: "Jod", Date: "2026-04-01"},
		toolchaintest.Release{Version: "v22.11.0", LTS: "Jod", Date: "2026-01-01"},
		toolchaintest.Release{Version: "v20.19.0", LTS: "Iron", Date: "2026-03-01"},
	)
	m := New(NodeJS{}, filepath.Join(t.TempDir(), "node"), mirror.URL)

	// Nothing installed: "22" means the newest 22.x release.
	got, installed, err := m.Resolve(ctx, "22")
	if err != nil || got != v("22.12.0") || installed {
		t.Fatalf("Resolve(22) = %v %v %v", got, installed, err)
	}
	// Ranges prefer LTS releases over a newer current one.
	if got, _, _ := m.Resolve(ctx, ">=20"); got != v("22.12.0") {
		t.Fatalf("Resolve(>=20) = %v", got)
	}
	if got, _, _ := m.Resolve(ctx, "lts/iron"); got != v("20.19.0") {
		t.Fatalf("Resolve(lts/iron) = %v", got)
	}
	if got, _, _ := m.Resolve(ctx, "node"); got != v("24.1.0") {
		t.Fatalf("Resolve(node) = %v", got)
	}
	if _, _, err := m.Resolve(ctx, "19"); err == nil {
		t.Fatal("Resolve(19) found a release")
	}

	// An installed version that fits wins over a newer release.
	var log strings.Builder
	if err := m.Install(ctx, v("22.11.0"), &log); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.String(), "checksum verified") || !strings.Contains(log.String(), "(100%)") || len(m.Installing()) != 0 {
		t.Fatalf("log = %q", log.String())
	}
	if got, installed, _ := m.Resolve(ctx, "22"); got != v("22.11.0") || !installed {
		t.Fatalf("Resolve(22) after install = %v %v", got, installed)
	}
	if got, _, _ := m.Resolve(ctx, "lts"); got != v("22.11.0") {
		t.Fatalf("Resolve(lts) = %v", got)
	}
	out, err := os.ReadFile(filepath.Join(m.BinDir(Installed{Version: v("22.11.0")}), "npm"))
	if err != nil || !strings.Contains(string(out), "npm") {
		t.Fatalf("npm symlink: %q %v", out, err)
	}

	// A second install of the same version downloads nothing.
	n := mirror.Downloads.Load()
	if err := m.Install(ctx, v("22.11.0"), nil); err != nil || mirror.Downloads.Load() != n {
		t.Fatalf("reinstall: %v, downloads %d → %d", err, n, mirror.Downloads.Load())
	}

	list, err := m.Installed()
	if err != nil || len(list) != 1 || list[0].LTS != "Jod" || list[0].Size == 0 {
		t.Fatalf("Installed = %+v %v", list, err)
	}

	// Update moves 22.11.0 to 22.12.0 and removes the old one.
	changes, err := m.Update(ctx, nil, nil)
	if err != nil || len(changes) != 1 || changes[0].To != v("22.12.0") || !changes[0].Removed {
		t.Fatalf("Update = %+v %v", changes, err)
	}
	if list, _ := m.Installed(); len(list) != 1 || list[0].Version != v("22.12.0") {
		t.Fatalf("after update: %+v", list)
	}

	// A version a project pins exactly is kept by update.
	mirror2 := toolchaintest.NewNodeMirror(t,
		toolchaintest.Release{Version: "v20.19.1", LTS: "Iron"},
		toolchaintest.Release{Version: "v20.19.0", LTS: "Iron"},
	)
	m2 := New(NodeJS{}, filepath.Join(t.TempDir(), "node"), mirror2.URL)
	m2.Install(ctx, v("20.19.0"), nil)
	changes, err = m2.Update(ctx, func(x Version) bool { return x == v("20.19.0") }, nil)
	if err != nil || len(changes) != 1 || changes[0].Removed {
		t.Fatalf("Update with a pinned version = %+v %v", changes, err)
	}
	if list, _ := m2.Installed(); len(list) != 2 {
		t.Fatalf("pinned version removed: %+v", list)
	}

	m.Install(ctx, v("20.19.0"), nil)
	if err := m.Remove(v("20.19.0")); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(v("20.19.0")); err == nil {
		t.Fatal("removing a missing version succeeded")
	}
}

func TestInstallRejectsBadChecksum(t *testing.T) {
	mirror := toolchaintest.NewNodeMirror(t, toolchaintest.Release{Version: "v22.11.0", LTS: "Jod"})
	mirror.Corrupt = "v22.11.0"
	m := New(NodeJS{}, filepath.Join(t.TempDir(), "node"), mirror.URL)
	err := m.Install(context.Background(), v("22.11.0"), nil)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Install = %v", err)
	}
	if list, _ := m.Installed(); len(list) != 0 {
		t.Fatalf("a damaged download was installed: %+v", list)
	}
	entries, _ := os.ReadDir(m.Dir())
	if len(entries) != 0 {
		t.Fatalf("leftovers in %s: %v", m.Dir(), entries)
	}
}

func TestExtractRefusesEscapes(t *testing.T) {
	for _, h := range []*tar.Header{
		{Name: "node/../../evil", Typeflag: tar.TypeReg, Mode: 0o644},
		{Name: "node/bin/evil", Typeflag: tar.TypeSymlink, Linkname: "../../../etc/passwd"},
		{Name: "node/bin/abs", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
	} {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		tw.WriteHeader(h)
		tw.Close()
		gz.Close()
		archive := filepath.Join(t.TempDir(), "a.tar.gz")
		os.WriteFile(archive, buf.Bytes(), 0o644)
		if err := extract(archive, filepath.Join(t.TempDir(), "root"), true); err == nil {
			t.Errorf("%s (%s) was extracted", h.Name, h.Linkname)
		}
	}
}

func TestCancelInstall(t *testing.T) {
	name := "node-v22.11.0-" + toolchaintest.Platform() + ".tar.gz"
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v22.11.0/SHASUMS256.txt":
			fmt.Fprintf(w, "%s  %s\n", strings.Repeat("a", 64), name)
		case "/v22.11.0/" + name:
			// A download that never finishes on its own.
			w.Header().Set("Content-Length", "100000000")
			w.Write(make([]byte, 64<<10))
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	m := New(NodeJS{}, filepath.Join(t.TempDir(), "node"), srv.URL)

	if m.Cancel(v("22.11.0")) {
		t.Fatal("Cancel found an install before one started")
	}
	done := make(chan error, 1)
	go func() { done <- m.Install(context.Background(), v("22.11.0"), nil) }()
	<-started
	waitFor(t, func() bool { p := m.Installing(); return len(p) == 1 && p[0].Done > 0 && p[0].Total == 100000000 })
	if !m.Cancel(v("22.11.0")) {
		t.Fatal("Cancel did not find the running install")
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrCanceled) {
			t.Fatalf("Install = %v, want ErrCanceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Install kept running after Cancel")
	}
	if len(m.Installing()) != 0 {
		t.Fatal("canceled install still listed")
	}
	if entries, _ := os.ReadDir(m.Dir()); len(entries) != 0 {
		t.Fatalf("leftovers after cancel: %v", entries)
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for range 200 {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}
