// Package toolchaintest serves fake Node.js and PHP releases for tests.
package toolchaintest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// Release is one fake release; Version looks like "v22.11.0".
type Release struct {
	Version string
	LTS     string
	Date    string
}

// Mirror serves index.json, SHASUMS256.txt and tarballs in the nodejs.org
// layout. Each tarball's bin/node is a shell script that prints the version,
// and bin/npm links to a script that prints "npm".
type Mirror struct {
	*httptest.Server
	Downloads atomic.Int32
	// Corrupt names a version whose tarball no longer matches its checksum.
	Corrupt string
}

// NewNodeMirror serves Node.js releases.
func NewNodeMirror(t *testing.T, releases ...Release) *Mirror {
	t.Helper()
	m := &Mirror{}
	platform := Platform()
	files := map[string][]byte{}
	var index []map[string]any
	for _, r := range releases {
		name := "node-" + r.Version + "-" + platform
		tarball := Tarball(t, name, r.Version)
		sum := sha256.Sum256(tarball)
		files["/"+r.Version+"/"+name+".tar.gz"] = tarball
		files["/"+r.Version+"/SHASUMS256.txt"] = fmt.Appendf(nil, "%s  %s.tar.gz\n%s  %s.tar.xz\n",
			hex.EncodeToString(sum[:]), name, strings.Repeat("0", 64), name)
		var lts any = false
		if r.LTS != "" {
			lts = r.LTS
		}
		index = append(index, map[string]any{"version": r.Version, "date": r.Date, "files": []string{platform, "win-x64-zip"}, "lts": lts})
	}
	files["/index.json"], _ = json.Marshal(index)
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".tar.gz") {
			m.Downloads.Add(1)
			if m.Corrupt != "" && strings.Contains(r.URL.Path, m.Corrupt) {
				body = append(bytes.Clone(body), 'x')
			}
		}
		w.Write(body)
	}))
	t.Cleanup(m.Close)
	return m
}

// Platform matches toolchain.NodeJS.Platform without importing it.
func Platform() string {
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}
	return runtime.GOOS + "-" + arch
}

// Tarball builds a release archive with top as its top directory.
func Tarball(t *testing.T, top, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	add := func(h *tar.Header, body string) {
		h.Size = int64(len(body))
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	add(&tar.Header{Name: top + "/", Typeflag: tar.TypeDir, Mode: 0o755}, "")
	add(&tar.Header{Name: top + "/bin/", Typeflag: tar.TypeDir, Mode: 0o755}, "")
	add(&tar.Header{Name: top + "/bin/node", Typeflag: tar.TypeReg, Mode: 0o755}, "#!/bin/sh\necho "+version+"\n")
	add(&tar.Header{Name: top + "/lib/node_modules/npm/bin/npm-cli.js", Typeflag: tar.TypeReg, Mode: 0o755}, "#!/bin/sh\necho npm\n")
	add(&tar.Header{Name: top + "/bin/npm", Typeflag: tar.TypeSymlink, Linkname: "../lib/node_modules/npm/bin/npm-cli.js"}, "")
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// NewPHPMirror serves static-php.dev's layout: a JSON file list and one
// tarball per release holding a php script that prints its version, with a
// .sha256 file unless the version ends in "!" (older builds have none). It
// also serves composer.phar and its .sha256sum under /composer.phar.
func NewPHPMirror(t *testing.T, versions ...string) *Mirror {
	t.Helper()
	m := &Mirror{}
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[runtime.GOARCH]
	files := map[string][]byte{}
	var list []map[string]any
	for _, v := range versions {
		v, noSum := strings.CutSuffix(v, "!")
		name := "php-" + v + "-cli-linux-" + arch + ".tar.gz"
		files["/"+name] = PHPTarball(t, v)
		if !noSum {
			sum := sha256.Sum256(files["/"+name])
			files["/"+name+".sha256"] = []byte(hex.EncodeToString(sum[:]) + "  " + name + "\n")
		}
		list = append(list, map[string]any{"name": name, "is_dir": false, "last_modified": "2026-07-07 05:34:44"})
	}
	list = append(list, map[string]any{"name": "README.txt", "is_dir": false, "last_modified": "2026-07-07 05:34:44"})
	files["/"], _ = json.Marshal(list)
	phar := []byte("<?php echo 'Composer';")
	sum := sha256.Sum256(phar)
	files["/composer.phar"] = phar
	files["/composer.phar.sha256sum"] = []byte(hex.EncodeToString(sum[:]) + "  composer.phar\n")
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".tar.gz") {
			m.Downloads.Add(1)
		}
		w.Write(body)
	}))
	t.Cleanup(m.Close)
	return m
}

// PHPTarball is a static-php.dev style archive: one php file at the top.
// The php is a script that prints the version, as php -r 'echo PHP_VERSION;'
// would, and runs any other arguments as a shell would not: it echoes them.
func PHPTarball(t *testing.T, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	script := "#!/bin/sh\nif [ \"$3\" = 'echo PHP_VERSION;' ]; then printf '%s' '" + version + "'; else echo php " + version + " \"$@\"; fi\n"
	tw.WriteHeader(&tar.Header{Name: "php", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(script))})
	tw.Write([]byte(script))
	tw.Close()
	gz.Close()
	return buf.Bytes()
}
