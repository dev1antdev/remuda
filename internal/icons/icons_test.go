package icons

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// a 1×1 PNG, a GIF header and an ICO header: enough for the sniffing
var (
	pngBytes, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	gifBytes    = []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;")
	icoBytes    = []byte{0, 0, 1, 0, 1, 0, 16, 16, 0, 0, 1, 0, 32, 0, 0, 0, 0, 0}
	svgText     = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><circle cx="8" cy="8" r="8"/></svg>`
)

func TestEmojiAreEmojiAlone(t *testing.T) {
	for _, good := range []string{"🦊", "🇺🇦", "👨‍👩‍👧‍👦", "1️⃣", "#️⃣", "❤️", "🏴󠁧󠁢󠁥󠁮󠁧󠁿", "🧑🏽‍💻"} {
		if err := CheckEmoji(good); err != nil {
			t.Errorf("%q refused: %v", good, err)
		}
	}
	for _, bad := range []string{"", "a", "AB", "1", "fox🦊", "я🦊", " 🦊", "🦊\n", strings.Repeat("🦊", 9), "\xff"} {
		if err := CheckEmoji(bad); err == nil {
			t.Errorf("%q passed", bad)
		}
	}
}

func TestPicturesAreWhatTheySay(t *testing.T) {
	good := map[string][]byte{"png": pngBytes, "gif": gifBytes, "x-icon": icoBytes, "vnd.microsoft.icon": icoBytes, "svg+xml": []byte(svgText)}
	for kind, data := range good {
		contentType, decoded, err := ParseImage("data:image/" + kind + ";base64," + base64.StdEncoding.EncodeToString(data))
		if err != nil || !bytes.Equal(decoded, data) || !strings.HasPrefix(contentType, "image/") {
			t.Errorf("%s: %q %v", kind, contentType, err)
		}
	}
	if contentType, _, _ := ParseImage("data:image/vnd.microsoft.icon;base64," + base64.StdEncoding.EncodeToString(icoBytes)); contentType != "image/x-icon" {
		t.Errorf("an ICO is served as %q", contentType)
	}
	refused := map[string]string{
		"not a data URL":    "https://example.com/a.png",
		"not base64":        "data:image/png,abc",
		"a PNG that isn't":  "data:image/png;base64," + base64.StdEncoding.EncodeToString(gifBytes),
		"an SVG that isn't": "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte("hello")),
		"html":              "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte("<b>x</b>")),
		"a kind not taken":  "data:image/tiff;base64," + base64.StdEncoding.EncodeToString(pngBytes),
		"too large":         "data:image/png;base64," + base64.StdEncoding.EncodeToString(append(append([]byte{}, pngBytes...), make([]byte, MaxImage)...)),
		"broken base64":     "data:image/png;base64,@@@@",
	}
	for name, dataURL := range refused {
		if _, _, err := ParseImage(dataURL); err == nil {
			t.Errorf("%s passed", name)
		}
	}
}

func TestStoreKeepsIconsByFolderAndDropsPicturesNoOneUses(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)
	if err := store.SetEmoji("gog", "/p/user-profiles/", "🦊"); err != nil {
		t.Fatal(err)
	}
	if icon, ok := store.Of("gog", "/p/user-profiles"); !ok || icon.Emoji != "🦊" {
		t.Fatalf("emoji: %+v %v", icon, ok)
	}
	if _, ok := store.Of("local", "/p/user-profiles"); ok {
		t.Fatal("another machine's folder has the icon")
	}
	hash, err := store.SetImage("gog", "/p/games-db", DataURL("image/png", pngBytes))
	if err != nil || len(hash) != 64 {
		t.Fatalf("image: %q %v", hash, err)
	}
	if contentType, data, ok := store.Image(hash); !ok || contentType != "image/png" || !bytes.Equal(data, pngBytes) {
		t.Fatalf("stored picture: %q %v", contentType, ok)
	}

	again := New(dir)
	if icon, _ := again.Of("gog", "/p/games-db"); icon.Image != hash {
		t.Fatalf("not kept on disk: %+v", icon)
	}
	if info, err := os.Stat(filepath.Join(dir, iconsFile)); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the file: %v %v", info, err)
	}

	if err := again.SetEmoji("gog", "/p/games-db", "🎮"); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := again.Image(hash); ok {
		t.Fatal("a picture no icon names any more is still kept")
	}
	if err := again.Clear("gog", "/p/games-db"); err != nil {
		t.Fatal(err)
	}
	if _, ok := again.Of("gog", "/p/games-db"); ok {
		t.Fatal("cleared icon still there")
	}
	if err := again.Clear("gog", "/nowhere"); err != nil {
		t.Fatalf("clearing a folder without an icon: %v", err)
	}
	if err := again.SetEmoji("gog", "relative/path", "🦊"); err == nil {
		t.Fatal("a relative folder was taken")
	}
	if err := again.SetEmoji("", "/p", "🦊"); err == nil {
		t.Fatal("no machine was taken")
	}
}

func TestADamagedFileReadsAsNoIcons(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, iconsFile), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := New(dir)
	if _, ok := store.Of("gog", "/p"); ok {
		t.Fatal("an icon out of a damaged file")
	}
	if err := store.SetEmoji("gog", "/p", "🦊"); err != nil {
		t.Fatalf("a store from a damaged file takes no icon: %v", err)
	}
}
