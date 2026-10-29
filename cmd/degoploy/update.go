package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// releasesURL is where releases are published; DEGOPLOY_DOWNLOAD_URL points
// at a mirror instead. install.sh uses the same layout.
func releasesURL() string {
	return strings.TrimRight(cmp.Or(os.Getenv("DEGOPLOY_DOWNLOAD_URL"), "https://github.com/irsyadulibad/degoploy/releases"), "/")
}

// runUpdate downloads a release, checks it, and hands over to its setup, which
// replaces the installed binary and restarts the service.
func runUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	want := fs.String("version", "", "install this release, such as v1.2.3, instead of the latest")
	check := fs.Bool("check", false, "only report whether a newer release exists")
	yes := fs.Bool("yes", false, "update without asking")
	fs.BoolVar(yes, "y", false, "short for --yes")
	fs.Parse(args)

	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return fmt.Errorf("releases are built for linux/amd64 and linux/arm64, not %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	ctx := context.Background()
	tag := *want
	if tag == "" {
		err := working("Looking for the latest release", func(ctx context.Context) error {
			var err error
			tag, err = latestRelease(ctx)
			return err
		})
		if err != nil {
			return err
		}
		switch {
		case tag == version:
			success("degoploy %s is the latest release", version)
			return nil
		case olderVersion(tag, version):
			success("degoploy %s is newer than the latest release (%s)", version, tag)
			return nil
		}
	}
	if *check {
		lipgloss.Println(blue.Render("●") + " degoploy " + bold.Render(tag) + " is available; you have " + version + ". Update with: degoploy update")
		return nil
	}
	if interactive() && !*yes {
		ok, err := confirm("Update degoploy "+version+" to "+tag+"?",
			"Downloads it from "+releasesURL()+", checks its SHA256 sum and restarts the service. Running deployments get up to 60s to finish.", true)
		if err != nil {
			return err
		}
		if !ok {
			return errCanceled
		}
	}

	tmp, err := os.MkdirTemp("", "degoploy-update")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	bin := filepath.Join(tmp, "degoploy")
	err = working("Downloading degoploy "+tag, func(ctx context.Context) error {
		return downloadRelease(ctx, tag, bin)
	})
	if err != nil {
		return err
	}
	success("Downloaded and verified degoploy %s", tag)
	lipgloss.Println()

	// The new binary's own setup does the rest, as root.
	argv := []string{bin, "setup", "--yes", "--no-operator"}
	if os.Geteuid() != 0 {
		sudo, err := exec.LookPath("sudo")
		if err != nil {
			return withHint("updating needs root", "Run it as root: degoploy update")
		}
		argv = append([]string{sudo, "--"}, argv...)
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		return err
	}
	return nil
}

// latestRelease reads the tag that releases/latest redirects to, which needs
// no API token and has no rate limit.
func latestRelease(ctx context.Context) (string, error) {
	client := &http.Client{
		Timeout:       20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, releasesURL()+"/latest", nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("look up the latest release: %w", err)
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if _, tag, ok := strings.Cut(loc, "/tag/"); ok && tag != "" {
		return path.Base(tag), nil
	}
	return "", fmt.Errorf("no release found at %s/latest", releasesURL())
}

// downloadRelease fetches the tarball for this machine, checks it against
// SHA256SUMS, and extracts the binary to dst.
func downloadRelease(ctx context.Context, tag, dst string) error {
	file := "degoploy-linux-" + runtime.GOARCH + ".tar.gz"
	base := releasesURL() + "/download/" + tag
	sums, err := fetch(ctx, base+"/SHA256SUMS")
	if err != nil {
		return err
	}
	var want string
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		if f := strings.Fields(sc.Text()); len(f) == 2 && strings.TrimPrefix(f[1], "*") == file {
			want = f[0]
		}
	}
	if want == "" {
		return fmt.Errorf("%s is not listed in %s/SHA256SUMS", file, base)
	}
	tarball, err := fetch(ctx, base+"/"+file)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(tarball)
	if hex.EncodeToString(sum[:]) != want {
		return fmt.Errorf("checksum mismatch for %s; the download is damaged or was altered", file)
	}

	gz, err := gzip.NewReader(bytes.NewReader(tarball))
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("%s has no degoploy binary", file)
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg || path.Base(h.Name) != "degoploy" {
			continue
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, tr)
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		return err
	}
}

func fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// olderVersion reports whether a sorts before b, comparing the numbers of
// vMAJOR.MINOR.PATCH. A release (v1.2.0) is newer than its pre-releases
// (v1.2.0-rc.1). Versions that do not parse, like "dev", compare as older.
func olderVersion(a, b string) bool {
	pa, oka := parseVersion(a)
	pb, okb := parseVersion(b)
	if !oka || !okb {
		return !oka && okb
	}
	for i := range 3 {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	preA, preB := strings.Contains(a, "-"), strings.Contains(b, "-")
	return preA && !preB
}

func parseVersion(v string) ([3]int, bool) {
	var n [3]int
	core, _, _ := strings.Cut(strings.TrimPrefix(v, "v"), "-")
	core, _, _ = strings.Cut(core, "+")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return n, false
	}
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil {
			return n, false
		}
		n[i] = x
	}
	return n, true
}
