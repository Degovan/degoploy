// Package nodetest serves fake Node.js releases for tests.
package nodetest

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

func NewMirror(t *testing.T, releases ...Release) *Mirror {
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

// Platform matches node.Platform without importing it.
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
