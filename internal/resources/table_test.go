package resources

import (
	"os"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

// a Mac's answer: ps names the whole path, one with a space in it
const macAnswer = "kind\x1fps\ncores\x1f8\nmem\x1f16777216\n\x1e\n" +
	"    1     0   0.0  12096\x1f/sbin/launchd\n" +
	"1\x1f0\x1f0.0\x1f12096\x1f/sbin/launchd\n" +
	"400\x1f1\x1f1.5\x1f40960\x1f/Users/me/.local/bin/herdr\n" +
	"410\x1f400\x1f0.0\x1f4096\x1f/bin/zsh\n" +
	"411\x1f410\x1f24.0\x1f524288\x1f/opt/homebrew/Cellar/node/24.1.0/bin/node\n" +
	"412\x1f411\x1f3.0\x1f102400\x1f/opt/homebrew/Cellar/node/24.1.0/bin/node\n" +
	"420\x1f400\x1f0.0\x1f4096\x1f/bin/zsh\n" +
	"421\x1f420\x1f60.0\x1f2097152\x1f/Users/me/.local/bin/claude\n" +
	"430\x1f400\x1f0.0\x1f4096\x1f/bin/zsh\n" +
	"500\x1f1\x1f9.0\x1f819200\x1f/Applications/Google Chrome.app/Contents/MacOS/Google Chrome\n"

// a Linux answer: cumulative ticks, the name alone
const linuxAnswer = "kind\x1flinux\ncores\x1f4\nticks\x1f100\nmem\x1f8000000\n\x1e\n" +
	"1\x1f0\x1f1200\x1f9000\x1fsystemd\n" +
	"700\x1f1\x1f5000\x1f30000\x1fherdr\n" +
	"710\x1f700\x1f10\x1f5000\x1fbash\n" +
	"711\x1f710\x1f90000\x1f700000\x1fnode\n" +
	"720\x1f700\x1f10\x1f5000\x1fbash\n" +
	"broken line\n"

func TestParseReadsAMacsAndALinuxTable(t *testing.T) {
	mac, err := Parse(macAnswer)
	if err != nil {
		t.Fatal(err)
	}
	if mac.Linux || mac.Cores != 8 || mac.MemKB != 16777216 || len(mac.Procs) != 9 {
		t.Fatalf("mac %+v", mac)
	}
	if mac.Procs[411].Name != "node" || mac.Procs[500].Name != "Google Chrome" || mac.Procs[411].CPU != 24 || mac.Procs[421].RSSKB != 2097152 {
		t.Fatalf("mac processes %+v %+v", mac.Procs[411], mac.Procs[500])
	}
	if got := mac.Kids[400]; len(got) != 3 {
		t.Fatalf("herdr's children %v", got)
	}
	linux, err := Parse(linuxAnswer)
	if err != nil {
		t.Fatal(err)
	}
	if !linux.Linux || linux.Cores != 4 || linux.Ticks != 100 || len(linux.Procs) != 5 || linux.Procs[711].CPU != 90000 {
		t.Fatalf("linux %+v", linux)
	}
	if _, err := Parse("kind\x1fps\n\x1e\n"); err == nil {
		t.Fatal("an empty table is no reading")
	}
	if _, err := Parse("ps: command not found\n"); err == nil {
		t.Fatal("an answer without its sections is no reading")
	}
}

func TestTheScriptReadsThisMachine(t *testing.T) {
	out, err := shell.Text(shell.Local{Name: "local"}, Script(), ReadTimeout)
	if err != nil && strings.TrimSpace(out) == "" {
		t.Fatal(err)
	}
	table, err := Parse(out)
	if err != nil {
		t.Fatalf("%v in\n%s", err, out[:min(len(out), 400)])
	}
	if table.Cores < 1 || table.MemKB < 1024 {
		t.Fatalf("cores %d, memory %d KB", table.Cores, table.MemKB)
	}
	me, ok := table.Procs[os.Getpid()]
	if !ok {
		t.Fatalf("this test's own process %d is not in the table of %d", os.Getpid(), len(table.Procs))
	}
	if me.PPID != os.Getppid() || me.RSSKB < 1 || me.Name == "" {
		t.Fatalf("this process read as %+v, its parent is %d", me, os.Getppid())
	}
}
