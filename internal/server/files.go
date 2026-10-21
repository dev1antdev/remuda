package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/machines"
)

// Files the agent wrote: resolved, served, diffed, uploaded.

func (s *Server) fileRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/fs/stat", s.handleFsStat)
	api.HandleFunc("GET /api/fs/dirs", s.handleFsDirs)
	api.HandleFunc("GET /api/fs/file", s.handleFsFile)
	api.HandleFunc("GET /api/fs/diff", s.handleFsDiff)
	api.HandleFunc("POST /api/fs/upload", s.handleFsUpload)
}

// handleFsStat resolves a path the agent printed (absolute, ~/, or relative to the pane's folder — to dir when there is no pane) and says what kind of file it is.
func (s *Server) handleFsStat(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	cwd := ""
	if paneID := r.URL.Query().Get("pane"); paneID != "" {
		if pane, err := s.findPane(mc, paneID); err == nil {
			cwd = pane.Dir()
		}
	}
	if cwd == "" {
		cwd = r.URL.Query().Get("dir")
	}
	info, err := mc.ResolvePath(cwd, r.URL.Query().Get("path"))
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	writeJSON(w, info)
}

// handleFsDirs is GET /api/fs/dirs?machine&path&hidden: a folder's subfolders on the machine, for the folder picker.
func (s *Server) handleFsDirs(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	listing, err := mc.ListDirs(r.URL.Query().Get("path"), r.URL.Query().Get("hidden") == "1")
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	writeJSON(w, listing)
}

// handleFsFile serves the bytes: text always as text/plain whatever the extension claims, sandboxed and never cached.
func (s *Server) handleFsFile(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	path := r.URL.Query().Get("path")
	if !strings.HasPrefix(path, "/") {
		httpErr(w, 400, fmt.Errorf("absolute path required (resolve it with /api/fs/stat first)"))
		return
	}
	info, err := mc.ResolvePath("", path)
	if err != nil || info.Kind == "missing" || info.Kind == "dir" {
		httpErr(w, 404, fmt.Errorf("not a readable file: %s", path))
		return
	}
	h := w.Header()
	h.Set("Content-Type", info.MIME)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, no-store")
	h.Set("X-Remuda-Kind", info.Kind)
	h.Set("X-Remuda-Size", strconv.FormatInt(info.Size, 10))
	h.Set("X-Remuda-Max", strconv.FormatInt(info.MaxBytes(), 10))
	if info.Kind != "pdf" { // the browser's PDF viewer refuses a sandboxed document
		h.Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src data:; media-src 'self'; style-src 'unsafe-inline'")
	}
	max := info.MaxBytes()
	if mc.Local() {
		f, err := mc.Open(info.Path)
		if err != nil {
			httpErr(w, 500, err)
			return
		}
		defer f.Close()
		fi, _ := f.Stat()
		var mod time.Time
		if fi != nil {
			mod = fi.ModTime()
		}
		h.Set("X-Remuda-Truncated", strconv.FormatBool(info.Size > max))
		http.ServeContent(w, r, filepath.Base(info.Path), mod, &cappedFile{f, min(info.Size, max)})
		return
	}
	data, truncated, err := mc.ReadRemote(info.Path, max)
	if err != nil {
		httpErr(w, 500, err)
		return
	}
	h.Set("X-Remuda-Truncated", strconv.FormatBool(truncated))
	w.Write(data)
}

// cappedFile lets ServeContent stream at most size bytes of a file.
type cappedFile struct {
	f    *os.File
	size int64
}

func (c *cappedFile) Read(b []byte) (int, error) {
	pos, _ := c.f.Seek(0, 1)
	if pos >= c.size {
		return 0, io.EOF
	}
	if rem := c.size - pos; int64(len(b)) > rem {
		b = b[:rem]
	}
	return c.f.Read(b)
}

func (c *cappedFile) Seek(off int64, whence int) (int64, error) {
	if whence == 2 {
		return c.f.Seek(c.size+off, 0)
	}
	return c.f.Seek(off, whence)
}

func (s *Server) handleFsDiff(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	path := r.URL.Query().Get("path")
	if !strings.HasPrefix(path, "/") {
		httpErr(w, 400, fmt.Errorf("absolute path required"))
		return
	}
	diff, err := mc.GitDiff(path)
	if err != nil {
		httpErr(w, 500, err)
		return
	}
	writeJSON(w, diff)
}

const (
	maxUploadFiles = 4
	maxUploadBytes = 25 << 20
)

// handleFsUpload stores up to four files (multipart field "file", repeated) in the pane's folder and returns their paths for the prompt.
func (s *Server) handleFsUpload(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	pane, err := s.findPane(mc, r.URL.Query().Get("pane"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadFiles*maxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		httpErr(w, 400, fmt.Errorf("upload too large or malformed: %v", err))
		return
	}
	defer r.MultipartForm.RemoveAll()
	parts := r.MultipartForm.File["file"]
	if len(parts) == 0 || len(parts) > maxUploadFiles {
		httpErr(w, 400, fmt.Errorf("send 1–%d files as \"file\"", maxUploadFiles))
		return
	}
	var out []machines.Upload
	for _, part := range parts {
		if part.Size > maxUploadBytes {
			httpErr(w, 413, fmt.Errorf("%s is over %d MB", part.Filename, maxUploadBytes>>20))
			return
		}
		f, err := part.Open()
		if err != nil {
			httpErr(w, 400, err)
			return
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			httpErr(w, 400, err)
			return
		}
		upload, err := mc.SaveUpload(pane.Dir(), part.Filename, data)
		if err != nil {
			httpErr(w, 500, err)
			return
		}
		out = append(out, upload)
	}
	writeJSON(w, map[string]any{"files": out})
}
