package core_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/timelapse"
)

// screenScript prints one of each style; testdata/screen-vt.txt and screen-herdr.txt are its screen as remuda-core 0.2.0
// (`terminal.read {format: "vt"}`, 60×8) and herdr 0.9.5 (`pane.read` format ansi) read it.
const screenScript = `printf '\033[1;31mred\033[0m plain \033[38;5;208morange\033[0m \033[38;2;1;2;3mrgb\033[0m\n` +
	`\033[2mdim\033[0m \033[3mit\033[0m \033[4mul\033[0m \033[7minv\033[0m \033[9mstrike\033[0m \033[8minvis\033[0m ` +
	`\033[93mbright\033[0m \033[44m bg \033[0m\n\n  \033[32mafter a gap\033[0m\n'; sleep 30`

func TestBlankRowsUnderTheTextAreLeftOutAsHerdrDoes(t *testing.T) {
	for _, c := range []struct{ name, vt, want string }{
		{"blank rows under the text", "\x1b[0mtop\n\x1b[0m\n\x1b[0m", "\x1b[0mtop"},
		{"a blank row between rows stays", "\x1b[0ma\n\x1b[0m\n\x1b[0mb\n\x1b[0m", "\x1b[0ma\n\x1b[0m\n\x1b[0mb"},
		{"coloured spaces show their colour", "\x1b[0mtop\n\x1b[0m\x1b[0;48;5;1m   \x1b[0m\n\x1b[0m", "\x1b[0mtop\n\x1b[0m\x1b[0;48;5;1m   \x1b[0m"},
		{"a blank screen", "\x1b[0m\n\x1b[0m\n\x1b[0m", ""},
		{"a finished terminal's plain text", "done\n\n", "done"},
	} {
		if got := core.ScreenText(c.vt); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestACoreScreenShowsTheRowsHerdrsShows(t *testing.T) {
	vt, err := os.ReadFile("testdata/screen-vt.txt")
	if err != nil {
		t.Fatal(err)
	}
	ansi, err := os.ReadFile("testdata/screen-herdr.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, want := shown(core.ScreenText(string(vt))), shown(string(ansi))
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("core shows\n%q\nherdr shows\n%q", got, want)
	}
}

func TestALiveCoreScreenEndsAtItsLastText(t *testing.T) {
	client := coretest.Server(t).Client(t)
	ctx := within(t, 20*time.Second)
	term, err := client.Run(ctx, core.RunParams{Argv: []string{"sh", "-c", screenScript}, Size: &core.Size{Cols: 60, Rows: 8}})
	if err != nil {
		t.Fatal(err)
	}
	var read core.ReadResult
	for !strings.Contains(read.Text, "after a gap") {
		time.Sleep(50 * time.Millisecond)
		if read, err = client.Read(ctx, term.Address, 0, "vt"); err != nil {
			t.Fatal(err)
		}
	}
	rows := strings.Split(core.ScreenText(read.Text), "\n")
	if len(rows) != 4 || !strings.Contains(rows[0], "\x1b[0;1;38;5;1mred") || !strings.HasSuffix(rows[3], "after a gap\x1b[0m") {
		t.Fatalf("rows %q", rows)
	}
}

// shown is each row as the timelapse player reads it (timelapse.Plain drops herdr's \r too).
func shown(screen string) []string {
	rows := strings.Split(screen, "\n")
	for i, row := range rows {
		rows[i] = timelapse.Plain(row)
	}
	return rows
}
