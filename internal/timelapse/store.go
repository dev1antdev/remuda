package timelapse

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dev1antdev/remuda/internal/state"
)

// A recording on the disk is <timelapse>/<machine>/<id>.json.gz: the Recording as JSON, gzipped, its Meta also in the
// gzip header's extra field (RFC 1952, subfield "RM"), so a listing reads a header and no frame.

// ErrNotFound is a recording the folder does not hold (or one still being recorded).
var ErrNotFound = errors.New("no such recording")

const suffix = ".json.gz"

var (
	validID     = regexp.MustCompile(`^\d{8}T\d{6}Z-[0-9a-f]{6}$`)
	safeMachine = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	halfWritten = regexp.MustCompile(`\.json\.gz\.\d+$`) // the atomic writer's temp file a crash left
	metaField   = [2]byte{'R', 'M'}
)

// entry is a recording on the disk.
type entry struct {
	meta Meta
	path string
}

// machineFolder is the folder a machine's recordings go in: its id when that is a plain name, else a hash of it.
func machineFolder(id string) string {
	if safeMachine.MatchString(id) {
		return id
	}
	sum := sha256.Sum256([]byte(id))
	return "m-" + hex.EncodeToString(sum[:8])
}

// load lists the recordings already on the disk by their headers; a temp file a crash left behind is removed
func (r *Recorder) load() {
	folders, err := os.ReadDir(r.dir)
	if err != nil {
		return
	}
	for _, folder := range folders {
		if !folder.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(r.dir, folder.Name()))
		if err != nil {
			continue
		}
		for _, file := range files {
			path := filepath.Join(r.dir, folder.Name(), file.Name())
			if halfWritten.MatchString(file.Name()) {
				os.Remove(path)
				continue
			}
			if !strings.HasSuffix(file.Name(), suffix) || !validID.MatchString(strings.TrimSuffix(file.Name(), suffix)) {
				continue
			}
			meta, err := readMeta(path)
			if err != nil {
				log.Printf("timelapse: %s: %v; left as it is", path, err)
				continue
			}
			r.index[meta.ID] = &entry{meta: meta, path: path}
		}
	}
}

// readMeta is a recording's Meta, from its file's header, and the file's size
func readMeta(path string) (Meta, error) {
	f, err := os.Open(path)
	if err != nil {
		return Meta{}, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return Meta{}, err
	}
	defer zr.Close()
	extra := zr.Header.Extra
	for len(extra) >= 4 {
		size := int(binary.LittleEndian.Uint16(extra[2:4]))
		if 4+size > len(extra) {
			break
		}
		if extra[0] == metaField[0] && extra[1] == metaField[1] {
			var meta Meta
			if err := json.Unmarshal(extra[4:4+size], &meta); err != nil {
				return Meta{}, err
			}
			if !validID.MatchString(meta.ID) || filepath.Base(path) != meta.ID+suffix {
				return Meta{}, errors.New("the header names another recording")
			}
			info, err := f.Stat()
			if err != nil {
				return Meta{}, err
			}
			meta.Size = info.Size()
			return meta, nil
		}
		extra = extra[4+size:]
	}
	return Meta{}, errors.New("no recording header")
}

// pack is the file's bytes: the Meta in the header's extra field, the Recording as the body
func pack(rec Recording) ([]byte, error) {
	rec.Size = 0
	head, err := json.Marshal(rec.Meta)
	if err != nil {
		return nil, err
	}
	if len(head) > 0xFFFF-4 {
		return nil, fmt.Errorf("a header of %d bytes", len(head))
	}
	extra := append([]byte{metaField[0], metaField[1], 0, 0}, head...)
	binary.LittleEndian.PutUint16(extra[2:4], uint16(len(head)))
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Header.Extra = extra
	zw.Header.ModTime = rec.End
	if err := json.NewEncoder(zw).Encode(rec); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// write puts a recording on the disk, 0600 through the atomic writer, then keeps the folder within its bounds
func (r *Recorder) write(rec Recording) error {
	folder := filepath.Join(r.dir, machineFolder(rec.Machine))
	if err := mkdir(folder); err != nil {
		return err
	}
	data, err := pack(rec)
	if err != nil {
		return err
	}
	path := filepath.Join(folder, rec.ID+suffix)
	if err := state.WriteFile(path, data); err != nil {
		return err
	}
	meta := rec.Meta
	meta.Size = int64(len(data))
	r.mu.Lock()
	r.index[meta.ID] = &entry{meta: meta, path: path}
	victims := r.pruneLocked()
	r.mu.Unlock()
	for _, victim := range victims {
		os.Remove(victim)
	}
	return nil
}

// pruneLocked drops what is past the bounds — a pane's recordings past KeepPerPane, then the oldest until the folder
// holds MaxTotal — from the index and answers their files; a turn still going is never one of them. r.mu is held.
func (r *Recorder) pruneLocked() []string {
	var kept []*entry
	var total int64
	for id, e := range r.index {
		total += e.meta.Size
		if !r.isLive(id) {
			kept = append(kept, e)
		}
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].meta.Start.After(kept[j].meta.Start) })
	var victims []string
	drop := func(e *entry) {
		delete(r.index, e.meta.ID)
		total -= e.meta.Size
		victims = append(victims, e.path)
	}
	perPane := map[string]int{}
	var rest []*entry
	for _, e := range kept {
		k := key(e.meta.Machine, e.meta.Pane)
		perPane[k]++
		if perPane[k] > r.keepPerPane {
			drop(e)
			continue
		}
		rest = append(rest, e)
	}
	for i := len(rest) - 1; i >= 0 && total > r.maxTotal; i-- {
		drop(rest[i])
	}
	return victims
}

// remove takes a recording off the disk and out of the index; a missing one is nothing to do
func (r *Recorder) remove(id string) {
	r.mu.Lock()
	e := r.index[id]
	delete(r.index, id)
	r.mu.Unlock()
	if e != nil {
		os.Remove(e.path)
	}
}

// List is a machine's recordings of a pane, or of a folder — every pane that worked there — the newest first.
func (r *Recorder) List(machineID, paneID, dir string) []Meta {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []Meta{}
	for id, e := range r.index {
		m := e.meta
		if m.Machine != machineID || r.isLive(id) {
			continue
		}
		if (paneID != "" && m.Pane == paneID) || (dir != "" && m.Dir == dir) {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.After(out[j].Start) })
	return out
}

// File is where a finished recording is, gzipped as it is served.
func (r *Recorder) File(id string) (Meta, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.index[id]
	if e == nil || r.isLive(id) {
		return Meta{}, "", ErrNotFound
	}
	return e.meta, e.path, nil
}

// Read is a finished recording whole.
func (r *Recorder) Read(id string) (Recording, error) {
	_, path, err := r.File(id)
	if err != nil {
		return Recording{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return Recording{}, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return Recording{}, err
	}
	var rec Recording
	err = json.NewDecoder(io.LimitReader(zr, 1<<30)).Decode(&rec)
	return rec, err
}

// Delete removes one finished recording.
func (r *Recorder) Delete(id string) error {
	if _, _, err := r.File(id); err != nil {
		return err
	}
	r.remove(id)
	return nil
}

// DeleteAll removes every recording, the turns being recorded too (a turn still going starts afresh), and answers how
// many finished ones went.
func (r *Recorder) DeleteAll() (int, error) {
	r.mu.Lock()
	var dropped []*take
	for k, tk := range r.live {
		tk.written = true
		dropped = append(dropped, tk)
		delete(r.live, k)
	}
	r.mu.Unlock()
	for _, tk := range dropped {
		tk.writing.Lock() // a checkpoint being written lands first, then goes with the rest
		defer tk.writing.Unlock()
	}
	r.mu.Lock()
	count := len(r.index)
	for _, tk := range dropped {
		if r.index[tk.meta.ID] != nil {
			count-- // a checkpoint of a turn still going was never a recording of the person's
		}
	}
	r.index = map[string]*entry{}
	r.mu.Unlock()
	if err := os.RemoveAll(r.dir); err != nil {
		return 0, err
	}
	return count, nil
}
