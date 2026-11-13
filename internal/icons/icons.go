// Package icons keeps the icon the person gave each folder the sidebar groups by: an emoji, or a small picture —
// a site's favicon or one from their computer — stored by its sha256 so the roster names it and the page loads it
// once, cached for good.
package icons

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/state"
)

const iconsFile = "icons.json"

// MaxImage is the largest picture an icon may be, decoded.
const MaxImage = 256 << 10

// MaxEmoji is the longest emoji an icon may be, in bytes: a flag, a keycap or a joined family fit.
const MaxEmoji = 32

// Icon is what the roster carries for a folder: an emoji, or the hash of a picture served at /api/icons/{hash}.
type Icon struct {
	Emoji string `json:"emoji,omitempty"`
	Image string `json:"image,omitempty"`
}

type image struct {
	Type string `json:"type"`
	Data []byte `json:"data"`
}

type stored struct {
	Icons  map[string]Icon  `json:"icons"`
	Images map[string]image `json:"images"`
}

// Store is every folder's icon, kept in the state dir.
type Store struct {
	mu   sync.Mutex
	dir  string
	data stored
}

// New loads the icons kept in dir; a damaged file is logged once and read as none.
func New(dir string) *Store {
	store := &Store{dir: dir}
	if err := state.LoadJSON(dir, iconsFile, &store.data); err != nil {
		log.Printf("icons: %s is unreadable, every folder starts without an icon: %v", iconsFile, err)
		store.data = stored{}
	}
	if store.data.Icons == nil {
		store.data.Icons = map[string]Icon{}
	}
	if store.data.Images == nil {
		store.data.Images = map[string]image{}
	}
	return store
}

func key(machine, folder string) string { return machine + "|" + filepath.Clean(folder) }

// Of is the folder's icon, false when it has none.
func (store *Store) Of(machine, folder string) (Icon, bool) {
	if machine == "" || folder == "" {
		return Icon{}, false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	icon, ok := store.data.Icons[key(machine, folder)]
	return icon, ok
}

// SetEmoji gives the folder an emoji for its icon.
func (store *Store) SetEmoji(machine, folder, emoji string) error {
	if err := checkPlace(machine, folder); err != nil {
		return err
	}
	if err := CheckEmoji(emoji); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.data.Icons[key(machine, folder)] = Icon{Emoji: emoji}
	return store.saveLocked()
}

// SetImage gives the folder a picture for its icon, from a data URL, and answers the picture's hash.
func (store *Store) SetImage(machine, folder, dataURL string) (string, error) {
	if err := checkPlace(machine, folder); err != nil {
		return "", err
	}
	contentType, data, err := ParseImage(dataURL)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	store.mu.Lock()
	defer store.mu.Unlock()
	store.data.Images[hash] = image{Type: contentType, Data: data}
	store.data.Icons[key(machine, folder)] = Icon{Image: hash}
	return hash, store.saveLocked()
}

// Clear takes the folder's icon away; a folder without one is no error.
func (store *Store) Clear(machine, folder string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.data.Icons[key(machine, folder)]; !ok {
		return nil
	}
	delete(store.data.Icons, key(machine, folder))
	return store.saveLocked()
}

// Image is a stored picture by its hash.
func (store *Store) Image(hash string) (string, []byte, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	picture, ok := store.data.Images[hash]
	return picture.Type, picture.Data, ok
}

// saveLocked drops the pictures no icon names any more and writes the file.
func (store *Store) saveLocked() error {
	used := map[string]bool{}
	for _, icon := range store.data.Icons {
		if icon.Image != "" {
			used[icon.Image] = true
		}
	}
	for hash := range store.data.Images {
		if !used[hash] {
			delete(store.data.Images, hash)
		}
	}
	if store.dir == "" {
		return nil
	}
	return state.SaveJSON(store.dir, iconsFile, store.data)
}

func checkPlace(machine, folder string) error {
	if machine == "" || folder == "" {
		return errors.New("a machine and a folder are required")
	}
	if !rpath.Absolute(folder) {
		return fmt.Errorf("the folder must be an absolute path: %q", folder)
	}
	return nil
}

// CheckEmoji refuses anything but an emoji: no letters or digits (a keycap's own aside), no control characters,
// and at least one character from past the Latin ranges.
func CheckEmoji(emoji string) error {
	if emoji == "" || len(emoji) > MaxEmoji || !utf8.ValidString(emoji) {
		return fmt.Errorf("an icon's emoji is one emoji, at most %d bytes", MaxEmoji)
	}
	runes := []rune(emoji)
	pictographic := false
	for i, r := range runes {
		switch {
		case unicode.IsControl(r):
			return errors.New("an emoji holds no control characters")
		case r < 0x80 && keycapBase(r) && keycapAt(runes, i):
		case r < 0x80 || unicode.IsLetter(r) || unicode.IsDigit(r):
			return errors.New("an icon's emoji holds no letters, digits or punctuation")
		case r >= 0x2000:
			pictographic = true
		}
	}
	if !pictographic {
		return errors.New("that is not an emoji")
	}
	return nil
}

func keycapBase(r rune) bool { return r == '#' || r == '*' || (r >= '0' && r <= '9') }

// keycapAt says whether the character at i starts a keycap: itself, perhaps the emoji selector, then the enclosing keycap.
func keycapAt(runes []rune, i int) bool {
	next := i + 1
	if next < len(runes) && runes[next] == 0xFE0F {
		next++
	}
	return next < len(runes) && runes[next] == 0x20E3
}

// imageKinds are the pictures an icon may be, by the subtype a data URL names, with what each sniffs as.
var imageKinds = map[string]string{
	"png":     "image/png",
	"jpeg":    "image/jpeg",
	"gif":     "image/gif",
	"webp":    "image/webp",
	"x-icon":  "image/x-icon",
	"svg+xml": "image/svg+xml",
}

// ParseImage reads a data URL of a picture, answering its content type and bytes when they are what it says.
func ParseImage(dataURL string) (string, []byte, error) {
	const prefix = "data:image/"
	if !strings.HasPrefix(dataURL, prefix) {
		return "", nil, errors.New("an icon's picture is a data:image/… URL")
	}
	kind, encoded, ok := strings.Cut(dataURL[len(prefix):], ";base64,")
	if !ok {
		return "", nil, errors.New("an icon's picture must be base64")
	}
	kind = strings.ToLower(kind)
	if kind == "vnd.microsoft.icon" {
		kind = "x-icon"
	}
	contentType, known := imageKinds[kind]
	if !known {
		return "", nil, fmt.Errorf("an icon may be PNG, JPEG, GIF, WebP, SVG or ICO, not %q", kind)
	}
	if base64.StdEncoding.DecodedLen(len(encoded)) > MaxImage+3 {
		return "", nil, fmt.Errorf("an icon's picture is at most %d KB", MaxImage>>10)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", nil, errors.New("the picture's base64 does not read")
	}
	if len(data) == 0 || len(data) > MaxImage {
		return "", nil, fmt.Errorf("an icon's picture is at most %d KB", MaxImage>>10)
	}
	if !sniffsAs(data, contentType) {
		return "", nil, fmt.Errorf("the picture is not the %s it says it is", kind)
	}
	return contentType, data, nil
}

// sniffsAs says whether the bytes are the picture the content type names; an SVG is text holding an <svg element.
func sniffsAs(data []byte, contentType string) bool {
	sniffed := http.DetectContentType(data)
	if contentType == "image/svg+xml" {
		text := strings.HasPrefix(sniffed, "text/xml") || strings.HasPrefix(sniffed, "text/plain")
		return text && strings.Contains(strings.ToLower(string(data)), "<svg")
	}
	return strings.HasPrefix(sniffed, contentType)
}

// DataURL is the bytes as a data URL of their own content type.
func DataURL(contentType string, data []byte) string {
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
}
