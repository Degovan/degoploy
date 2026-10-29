package admin

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/degovan/degoploy/internal/store"
)

// Trailers of a backup response: the stream is sent before the outcome is known.
const (
	trailerFiles = "X-Backup-Files"
	trailerError = "X-Backup-Error"
)

func (h *handler) backup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Trailer", trailerFiles+", "+trailerError)
	w.Header().Set("Content-Type", "application/gzip")
	n, err := WriteBackup(r.Context(), w, h.Store, h.Config.DataDir, r.URL.Query().Get("logs") != "0")
	w.Header().Set(trailerFiles, strconv.Itoa(n))
	if err != nil {
		w.Header().Set(trailerError, err.Error())
	}
}

// WriteBackup writes a .tar.gz of the data directory to w: a consistent snapshot
// of the database plus deploy keys, managed scripts, known_hosts and,
// optionally, logs. Restore by stopping degoploy and extracting the archive into
// data_dir. It returns the number of files written.
func WriteBackup(ctx context.Context, w io.Writer, st *store.Store, dataDir string, logs bool) (int, error) {
	tmp, err := os.MkdirTemp("", "degoploy-backup-*")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(tmp)
	snapshot := filepath.Join(tmp, "degoploy.db")
	if err := st.Backup(ctx, snapshot); err != nil {
		return 0, fmt.Errorf("snapshot database: %w", err)
	}
	if err := os.Chmod(snapshot, 0o600); err != nil {
		return 0, err
	}

	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	if err := addFile(tw, snapshot, "degoploy.db"); err != nil {
		return 0, err
	}
	files := 1
	err = filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dataDir, path)
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
		case !logs && d.IsDir() && d.Name() == "logs":
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
		return files, err
	}
	if err := tw.Close(); err != nil {
		return files, err
	}
	return files, gz.Close()
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
