package toolchain

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// counter reports how many bytes went through it.
type counter struct {
	done, total int64
	f           func(done, total int64)
}

func (c *counter) Write(b []byte) (int, error) {
	c.done += int64(len(b))
	c.f(c.done, c.total)
	return len(b), nil
}

// ProgressLabel reads like "27.1 of 54.2 MB (50%)".
func ProgressLabel(done, total int64) string {
	mb := func(n int64) string { return fmt.Sprintf("%.1f", float64(n)/(1<<20)) }
	if total <= 0 {
		return mb(done) + " MB downloaded"
	}
	return fmt.Sprintf("%s of %s MB (%d%%)", mb(done), mb(total), done*100/total)
}

// extract unpacks a release tarball into dst, dropping its top directory when
// strip is set. Entries that would land outside dst are refused.
func extract(archive, dst string, strip bool) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	inside := func(p string) bool {
		rel, err := filepath.Rel(dst, p)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
	}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		rel, ok := strings.TrimPrefix(filepath.ToSlash(h.Name), "./"), true
		if strip {
			_, rel, ok = strings.Cut(rel, "/")
		}
		if !ok || rel == "" {
			if h.Typeflag == tar.TypeDir {
				if err := os.MkdirAll(dst, 0o755); err != nil {
					return err
				}
			}
			continue
		}
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if !inside(target) {
			return fmt.Errorf("%s points outside the archive", h.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := fs.FileMode(h.Mode).Perm()
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode|0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode|0o600)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tr)
			if cerr := out.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		case tar.TypeSymlink:
			link := h.Linkname
			if filepath.IsAbs(link) || !inside(filepath.Join(filepath.Dir(target), link)) {
				return fmt.Errorf("%s links outside the archive", h.Name)
			}
			if err := os.Symlink(link, target); err != nil {
				return err
			}
		case tar.TypeLink:
			lrel := strings.TrimPrefix(filepath.ToSlash(h.Linkname), "./")
			if strip {
				_, lrel, _ = strings.Cut(lrel, "/")
			}
			src := filepath.Join(dst, filepath.FromSlash(lrel))
			if !inside(src) {
				return fmt.Errorf("%s links outside the archive", h.Name)
			}
			if err := os.Link(src, target); err != nil {
				return err
			}
		}
	}
}

func dirSize(dir string) int64 {
	var n int64
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

func humanSize(n int64) string {
	switch {
	case n <= 0:
		return "size unknown"
	case n < 1<<20:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}
