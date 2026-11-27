package server

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/rpath"
)

// The Changes lens: a folder's uncommitted work, the diff of one file, the baseline moved by a look.

func (s *Server) reviewRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/review", s.handleReview)
	api.HandleFunc("GET /api/review/diff", s.handleReviewDiff)
	api.HandleFunc("POST /api/review/seen", s.handleReviewSeen)
	api.HandleFunc("GET /api/review/secrets", s.handleReviewSecrets)
}

// handleReviewSecrets answers GET /api/review/secrets?machine=&dir=&push=1 with the secret check of the folder's work —
// what a commit of it would carry, and with push the commits a push would send; never a viewer's or an agent key's.
func (s *Server) handleReviewSecrets(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	mc, ok := s.reviewTarget(w, r, "", dir)
	if !ok {
		return
	}
	scan, err := review.ScanWork(mc, dir, r.URL.Query().Get("push") == "1")
	if err != nil {
		httpErr(w, 502, err) // the reason alone: the page says it is the secret check's
		return
	}
	s.Review.NoteScan(mc.ID(), dir, scan) // the review queue's risk reads it while the work stays as it was read
	writeJSON(w, scan)
}

// reviewTarget resolves the request's machine and insists on an absolute folder.
func (s *Server) reviewTarget(w http.ResponseWriter, r *http.Request, bodyMachine, dir string) (*machines.Machine, bool) {
	m, err := s.machineFor(r, bodyMachine)
	if err != nil {
		httpErr(w, 404, err)
		return nil, false
	}
	if !rpath.Absolute(dir) {
		httpErr(w, 400, errors.New("an absolute folder is required"))
		return nil, false
	}
	return m, true
}

// handleReview answers GET /api/review?machine=&dir= with the folder's Summary.
func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	mc, ok := s.reviewTarget(w, r, "", dir)
	if !ok {
		return
	}
	summary := s.Review.Summary(mc, dir)
	if s.Stage != nil && summary.Repo != "" {
		busy := s.Stage.Advisory
		if busy == nil {
			busy = s.Stage.Busy
		}
		if busy != nil {
			summary.Busy = busy(mc.ID(), dir, rpath.For(mc.Home()).Clean(summary.Repo))
		}
	}
	writeJSON(w, summary)
}

var hashOK = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

// seenBlobsOK checks what a page says it showed: a commit hash or none, and every file a blob hash, "gone" or a
// size: fingerprint — nothing else is written as the baseline.
func seenBlobsOK(head string, blobs map[string]string) bool {
	if head != "" && !hashOK.MatchString(head) {
		return false
	}
	for _, blob := range blobs {
		if blob == "gone" || strings.HasPrefix(blob, "size:") || hashOK.MatchString(blob) {
			continue
		}
		return false
	}
	return true
}

// handleReviewDiff answers GET /api/review/diff?machine=&dir=&path= with one file's unified diff: since the last look,
// on one side of the index (side=staged|unstaged), in one commit (commit=<sha>), or from a merge base (base=<sha>).
func (s *Server) handleReviewDiff(w http.ResponseWriter, r *http.Request) {
	dir, path := r.URL.Query().Get("dir"), r.URL.Query().Get("path")
	mc, ok := s.reviewTarget(w, r, "", dir)
	if !ok {
		return
	}
	// a path starting with "-" would reach git as an option
	if path == "" || rpath.Absolute(path) || strings.HasPrefix(path, "-") || strings.Contains(path, "..") {
		httpErr(w, 400, errors.New("a path relative to the folder is required"))
		return
	}
	query := r.URL.Query()
	var diff string
	var truncated bool
	var err error
	switch {
	case query.Get("commit") != "":
		diff, truncated, err = review.CommitDiff(mc, dir, query.Get("commit"), path)
	case query.Get("base") != "":
		diff, truncated, err = review.BaseDiff(mc, dir, query.Get("base"), path)
	default:
		diff, truncated, err = s.Review.DiffSide(mc, dir, path, query.Get("side"))
	}
	if err != nil {
		httpErr(w, 502, err)
		return
	}
	writeJSON(w, map[string]any{"diff": diff, "truncated": truncated})
}

// handleReviewSeen answers POST /api/review/seen {machine, dir, verdict?}: the baseline moves to now.
func (s *Server) handleReviewSeen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string            `json:"machine"`
		Dir     string            `json:"dir"`
		Verdict string            `json:"verdict"`
		Head    string            `json:"head"`
		Blobs   map[string]string `json:"blobs"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !review.VerdictOK(req.Verdict) {
		httpErr(w, 400, errors.New("the verdict is approved, changes, or none"))
		return
	}
	if req.Blobs != nil && !seenBlobsOK(req.Head, req.Blobs) {
		httpErr(w, 400, errors.New("the blobs a look showed are blob hashes, size: fingerprints or gone"))
		return
	}
	m, ok := s.reviewTarget(w, r, req.Machine, req.Dir)
	if !ok {
		return
	}
	var b review.Baseline
	var err error
	if req.Blobs != nil {
		b, err = s.Review.MarkSeenShown(m, req.Dir, req.Verdict, req.Head, req.Blobs)
	} else {
		b, err = s.Review.MarkSeen(m, req.Dir, req.Verdict)
	}
	if err != nil {
		httpErr(w, 502, err)
		return
	}
	writeJSON(w, b)
}
