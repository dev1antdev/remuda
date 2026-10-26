package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readDoor(t *testing.T, dir string) (key, url string) {
	t.Helper()
	k, err := os.ReadFile(filepath.Join(dir, DoorKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	u, err := os.ReadFile(filepath.Join(dir, DoorURLFile))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(k)), string(u)
}

func TestEnsureDoorMintsOnceAndRotatesWhenRevoked(t *testing.T) {
	dir := t.TempDir()
	keys := openKeys(t, dir)
	if err := EnsureDoor(dir, keys, "This Mac", "http://127.0.0.1:7317"); err != nil {
		t.Fatal(err)
	}
	first, url := readDoor(t, dir)
	if url != "http://127.0.0.1:7317\n" || !IsKey(first) {
		t.Fatalf("door: %q %q", first, url)
	}
	if st, _ := os.Stat(filepath.Join(dir, DoorKeyFile)); st.Mode().Perm() != 0o600 {
		t.Fatalf("the key file is %v, want 0600", st.Mode().Perm())
	}
	if rec, ok := keys.Check(first); !ok || rec.Name != "This Mac" {
		t.Fatalf("the file's key is not the store's: %v %+v", ok, rec)
	}

	if err := EnsureDoor(dir, keys, "This Mac", "http://127.0.0.1:7317"); err != nil {
		t.Fatal(err)
	}
	if again, _ := readDoor(t, dir); again != first {
		t.Fatal("a second Ensure minted anew though the key stood")
	}

	rec, _ := keys.ByName("This Mac")
	keys.Revoke(rec.ID)
	if err := EnsureDoor(dir, keys, "This Mac", "http://127.0.0.1:7317"); err != nil {
		t.Fatal(err)
	}
	second, _ := readDoor(t, dir)
	if second == first {
		t.Fatal("a revoked key was kept in the file")
	}
	if _, ok := keys.Check(second); !ok {
		t.Fatal("the new key is not the store's")
	}

	if err := EnsureDoor(dir, keys, "Studio", "http://127.0.0.1:7399"); err != nil {
		t.Fatal(err)
	}
	third, url := readDoor(t, dir)
	if third == second || url != "http://127.0.0.1:7399\n" {
		t.Fatalf("a new name and address: %q %q", third, url)
	}
	if list := keys.List(); len(list) != 1 || list[0].Name != "Studio" {
		t.Fatalf("the old name's key should be gone: %+v", list)
	}
}

func TestLoopbackURLAndPort(t *testing.T) {
	for addr, want := range map[string]string{
		"127.0.0.1:7317": "http://127.0.0.1:7317",
		"0.0.0.0:7317":   "http://127.0.0.1:7317",
		":7317":          "http://127.0.0.1:7317",
		"[::]:7317":      "http://127.0.0.1:7317",
		"nonsense":       "http://nonsense",
	} {
		if got := LoopbackURL(addr); got != want {
			t.Errorf("LoopbackURL(%q) = %q, want %q", addr, got, want)
		}
	}
	if PortOf(":7317") != 7317 || PortOf("127.0.0.1:7441") != 7441 || PortOf("nonsense") != 0 || PortOf("host:x") != 0 {
		t.Error("PortOf")
	}
}
