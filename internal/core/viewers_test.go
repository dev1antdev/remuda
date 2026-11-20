package core_test

import (
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/herdr"
)

// seen collects what a viewer is handed until it holds want.
func seen(t *testing.T, viewer herdr.Viewer, want string) {
	t.Helper()
	var got strings.Builder
	deadline := time.After(10 * time.Second)
	for !strings.Contains(got.String(), want) {
		select {
		case chunk, ok := <-viewer.Bytes():
			if !ok {
				t.Fatalf("the viewer ended before %q; it had %q", want, got.String())
			}
			got.Write(chunk)
		case <-deadline:
			t.Fatalf("no %q within 10 s; the viewer had %q", want, got.String())
		}
	}
}

func TestTwoViewersShareATerminalWithoutTakingItsSize(t *testing.T) {
	ctx := within(t, 30*time.Second)
	client := coretest.Server(t).Client(t)
	cat, err := client.Run(ctx, core.RunParams{Argv: []string{"cat"}, Size: &core.Size{Cols: 100, Rows: 30}})
	if err != nil {
		t.Fatal(err)
	}
	terminals := core.NewTerminals(client)
	desk, err := terminals.Open(cat.Address, herdr.View{Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	defer desk.Close()
	phone, err := terminals.Open(cat.Address, herdr.View{Cols: 40, Rows: 10, Keep: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := terminals.Viewers(cat.Address); n != 2 {
		t.Fatalf("viewers = %d, want 2", n)
	}
	seen(t, phone, "\x1b[8;30;100t") // a viewer that keeps the size is told the terminal's grid before its paint
	if err := phone.Write([]byte("from the phone\r")); err != nil {
		t.Fatal(err)
	}
	seen(t, desk, "from the phone")
	seen(t, phone, "from the phone")
	if err := phone.Resize(40, 10); err != nil {
		t.Fatal(err)
	}
	if got, _ := client.Get(ctx, cat.Address); got.Cols != 100 || got.Rows != 30 {
		t.Fatalf("the phone that keeps the size made it %dx%d", got.Cols, got.Rows)
	}
	if err := desk.Resize(90, 28); err != nil {
		t.Fatal(err)
	}
	for start := time.Now(); ; time.Sleep(50 * time.Millisecond) {
		if got, _ := client.Get(ctx, cat.Address); got.Cols == 90 && got.Rows == 28 {
			break
		}
		if time.Since(start) > 5*time.Second {
			t.Fatal("the desk's resize never reached the terminal")
		}
	}
	seen(t, phone, "\x1b[8;28;90t")
	phone.Close()
	if n := terminals.Viewers(cat.Address); n != 1 {
		t.Fatalf("viewers after the phone left = %d, want 1", n)
	}
}

func TestAViewerThatOnlyWatchesCannotType(t *testing.T) {
	ctx := within(t, 30*time.Second)
	client := coretest.Server(t).Client(t)
	cat, err := client.Run(ctx, core.RunParams{Argv: []string{"cat"}})
	if err != nil {
		t.Fatal(err)
	}
	terminals := core.NewTerminals(client)
	link, err := terminals.Open(cat.Address, herdr.View{Cols: 80, Rows: 24, Observe: true})
	if err != nil {
		t.Fatal(err)
	}
	defer link.Close()
	_ = link.Write([]byte("typed by a link\r"))
	desk, err := terminals.Open(cat.Address, herdr.View{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer desk.Close()
	if n := terminals.Viewers(cat.Address); n != 1 {
		t.Fatalf("viewers = %d: a link that only watches is not the person looking", n)
	}
	if err := desk.Write([]byte("typed at the desk\r")); err != nil {
		t.Fatal(err)
	}
	seen(t, link, "typed at the desk")
	if got, _ := client.Read(ctx, cat.Address, 0, ""); strings.Contains(got.Text, "typed by a link") {
		t.Fatalf("a viewer that only watches typed: %q", got.Text)
	}
}

func TestAViewerEndsWithItsProgram(t *testing.T) {
	ctx := within(t, 30*time.Second)
	client := coretest.Server(t).Client(t)
	brief, err := client.Run(ctx, core.RunParams{Argv: []string{"sh", "-c", `read line; echo "bye:$line"`}})
	if err != nil {
		t.Fatal(err)
	}
	terminals := core.NewTerminals(client)
	viewer, err := terminals.Open(brief.Address, herdr.View{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	if err := viewer.Write([]byte("now\r")); err != nil {
		t.Fatal(err)
	}
	seen(t, viewer, "bye:now")
	for deadline := time.After(10 * time.Second); ; {
		select {
		case _, ok := <-viewer.Bytes():
			if !ok {
				if n := terminals.Viewers(brief.Address); n != 0 {
					t.Fatalf("viewers after the program ended = %d", n)
				}
				return
			}
		case <-deadline:
			t.Fatal("the viewer's bytes never closed after the program ended")
		}
	}
}
