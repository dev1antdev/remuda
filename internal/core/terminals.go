package core

import (
	"context"
	"encoding/json"
	"slices"
	"time"
)

// Terminal is a terminal as the core tells it (schema `Terminal`): the facts remuda reads.
type Terminal struct {
	Address string            `json:"address"`
	Name    string            `json:"name"`
	Labels  map[string]string `json:"labels"`
	Argv    []string          `json:"argv"`
	// Literal: it runs what a literal run started (a scene pane's own command), not a shell keep gave it since; nil
	// from a core whose record says nothing of it (one without literal_record, rolled back to, say).
	Literal    *bool  `json:"literal"`
	Cwd        string `json:"cwd"`
	Keep       bool   `json:"keep"`
	State      string `json:"state"`
	Pid        int    `json:"pid"`
	ExitCode   *int   `json:"exit_code"`
	Signal     *int   `json:"signal"`
	Reason     string `json:"reason"`
	CreatedMs  int64  `json:"created_ms"`
	FinishedMs int64  `json:"finished_ms"`
	Cols       uint16 `json:"cols"`
	Rows       uint16 `json:"rows"`
	// Status is what the program last reported through OSC 7501, nil when it never said or has exited.
	Status   *Status           `json:"status"`
	StatusMs int64             `json:"status_ms"`
	Records  map[string]Status `json:"records"`
	// Turn goes up each time the status leaves working.
	Turn       uint64      `json:"turn"`
	Reported   bool        `json:"reported"`
	Title      string      `json:"title"`
	Parent     string      `json:"parent"`
	Foreground *Foreground `json:"foreground"`

	// agentAt is when the mirror saw an agent take the terminal's foreground, statusAt when it last heard the
	// program's status (zero: before the mirror looked).
	agentAt, statusAt time.Time
	// restored: its shell was lost (a kept terminal) and the core gives it a new one; no agent took it since.
	restored bool
	// held counts, and heldAt notes, the moment its foreground last changed hands or the mirror took the record in.
	held   uint64
	heldAt time.Time
}

// Status is a program's own word: idle, working, done, blocked or error, with what it said beside it.
type Status struct {
	State    string `json:"state"`
	Kind     string `json:"kind"`
	Progress *int   `json:"progress"`
	App      string `json:"app"`
	Title    string `json:"title"`
	Msg      string `json:"msg"`
}

// Foreground is what runs in a terminal now and where.
type Foreground struct {
	Pid  int      `json:"pid"`
	Name string   `json:"name"`
	Argv []string `json:"argv"`
	Cwd  string   `json:"cwd"`
	// StartedMS is when that process was created, unix ms, as the OS records it: a core advertising
	// `foreground_started` tells it, 0 from one that does not.
	StartedMS int64 `json:"started_ms"`
}

// literalRuns are the hello features of a remuda-core that starts argv as it is (`literal`, Core 35) and says so on the
// terminal's record until keep gives it a shell (`literal_record`). An older core ignores `literal` and may start a
// template of the program's name, sending its first message; one without the record reads every terminal as no literal
// run, a scene pane's running `sh -c` a shell at its prompt. A test hides them.
var literalRuns = []string{"run_literal", "literal_record"}

// runsLiteral says whether the core takes a literal run and tells it on the record.
func runsLiteral(h Hello) bool {
	return !slices.ContainsFunc(literalRuns, func(feature string) bool { return !h.Has(feature) })
}

// RunParams starts a program in a new terminal: Argv, or Line through the machine's shell, or neither for the shell.
// Literal starts Argv as it is, with no template looked up by its first word.
type RunParams struct {
	Argv    []string          `json:"argv,omitempty"`
	Literal bool              `json:"literal,omitempty"`
	Line    string            `json:"line,omitempty"`
	Name    string            `json:"name,omitempty"`
	Cwd     string            `json:"cwd,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
	Size    *Size             `json:"size,omitempty"`
	Keep    bool              `json:"keep,omitempty"`
	// Rm removes the terminal once its program exits, as herdr closes a pane whose shell ended.
	Rm bool `json:"rm,omitempty"`
}

// Size is a grid in cells.
type Size struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// ReadResult is a terminal's text: Format "vt" keeps the rows' colours as SGR.
type ReadResult struct {
	Text     string `json:"text"`
	Complete bool   `json:"complete"`
	How      string `json:"how"`
}

// SendResult is how a terminal stood when the text went in.
type SendResult struct {
	Status  *string `json:"status"`
	Turn    uint64  `json:"turn"`
	Started *bool   `json:"started"`
}

// KeysResult is the status and turn when the keys went in.
type KeysResult struct {
	Status *string `json:"status"`
	Turn   uint64  `json:"turn"`
}

// InterruptResult is what an interrupt came to: stopped, exited, still_working or unknown.
type InterruptResult struct {
	Outcome string `json:"outcome"`
	Key     string `json:"key"`
}

// WaitResult is how a wait ended: status, exit, output or timeout.
type WaitResult struct {
	Ended    bool    `json:"ended"`
	Reason   string  `json:"reason"`
	Status   *string `json:"status"`
	Turn     uint64  `json:"turn"`
	ExitCode *int    `json:"exit_code"`
}

// Doc is a document of the core's and the version a write must name.
type Doc struct {
	Value   json.RawMessage `json:"value"`
	Version uint64          `json:"version"`
}

// ServerStatus is the core's own state.
type ServerStatus struct {
	Core     CoreInfo `json:"core"`
	Version  string   `json:"version"`
	Pid      int      `json:"pid"`
	UptimeMs int64    `json:"uptime_ms"`
	Live     int      `json:"live"`
	Finished int      `json:"finished"`
}

// Run starts a terminal. A literal run goes only on a connection whose core said it takes one.
func (c *Client) Run(ctx context.Context, params RunParams) (Terminal, error) {
	cn, err := c.connection(ctx)
	if err != nil {
		return Terminal{}, err
	}
	if params.Literal && !runsLiteral(cn.hello) {
		return Terminal{}, &Error{Kind: "needs_update", Message: "this remuda-core cannot start a program without reading its name as a template's"}
	}
	var t Terminal
	return t, cn.call(ctx, "terminal.run", params, &t, nil)
}

// List is the live terminals, the finished ones too with all, those carrying every label given.
func (c *Client) List(ctx context.Context, all bool, labels map[string]string) ([]Terminal, error) {
	var listed struct {
		Terminals []Terminal `json:"terminals"`
	}
	params := map[string]any{"all": all}
	if len(labels) > 0 {
		params["labels"] = labels
	}
	err := c.Call(ctx, "terminal.list", params, &listed)
	return listed.Terminals, err
}

func (c *Client) Get(ctx context.Context, terminal string) (Terminal, error) {
	var t Terminal
	return t, c.Call(ctx, "terminal.get", map[string]any{"terminal": terminal}, &t)
}

func (c *Client) Kill(ctx context.Context, terminal string) (Terminal, error) {
	var t Terminal
	return t, c.Call(ctx, "terminal.kill", map[string]any{"terminal": terminal}, &t)
}

// Keep sets whether a lost program is replaced by the machine's shell in place (terminal.keep).
func (c *Client) Keep(ctx context.Context, terminal string, keep bool) error {
	return c.Call(ctx, "terminal.keep", map[string]any{"terminal": terminal, "keep": keep}, nil)
}

// Label sets labels; an empty value removes one.
func (c *Client) Label(ctx context.Context, terminal string, labels map[string]string) (Terminal, error) {
	var t Terminal
	return t, c.Call(ctx, "terminal.label", map[string]any{"terminal": terminal, "labels": labels}, &t)
}

// Read is the screen and lines of history above it, format "plain" or "vt".
func (c *Client) Read(ctx context.Context, terminal string, lines int, format string) (ReadResult, error) {
	var read ReadResult
	params := map[string]any{"terminal": terminal, "lines": lines}
	if format != "" {
		params["format"] = format
	}
	return read, c.Call(ctx, "terminal.read", params, &read)
}

// Send pastes text and presses Enter once the person is quiet. It never types into a blocked program: the core
// refuses (remuda's 409), since a dialog is answered by its own keys, never by a message typed into it.
func (c *Client) Send(ctx context.Context, terminal, text string) (SendResult, error) {
	var sent SendResult
	return sent, c.Call(ctx, "terminal.send", map[string]any{"terminal": terminal, "text": text}, &sent)
}

// Keys presses named keys (enter, esc, ctrl+c, …); now skips the wait for the person's quiet.
func (c *Client) Keys(ctx context.Context, terminal string, keys []string, now bool) (KeysResult, error) {
	var pressed KeysResult
	return pressed, c.Call(ctx, "terminal.keys", map[string]any{"terminal": terminal, "keys": keys, "now": now}, &pressed)
}

func (c *Client) Interrupt(ctx context.Context, terminal string) (InterruptResult, error) {
	var result InterruptResult
	return result, c.Call(ctx, "terminal.interrupt", map[string]any{"terminal": terminal}, &result)
}

// Wait waits until the terminal stops working, exits or prints output (when given), for timeout seconds at most.
func (c *Client) Wait(ctx context.Context, terminal, output string, timeout float64) (WaitResult, error) {
	params := map[string]any{"terminal": terminal, "timeout": timeout}
	if output != "" {
		params["output"] = output
	}
	var waited WaitResult
	return waited, c.Call(ctx, "terminal.wait", params, &waited)
}

func (c *Client) DocGet(ctx context.Context, name string) (Doc, error) {
	var doc Doc
	return doc, c.Call(ctx, "doc.get", map[string]any{"name": name}, &doc)
}

// DocSet writes a document only while it is at version (0 for one never written), answering the new version; a
// stale version is a `conflict`.
func (c *Client) DocSet(ctx context.Context, name string, value any, version uint64) (uint64, error) {
	var written struct {
		Version uint64 `json:"version"`
	}
	err := c.Call(ctx, "doc.set", map[string]any{"name": name, "value": value, "version": version}, &written)
	return written.Version, err
}

func (c *Client) Status(ctx context.Context) (ServerStatus, error) {
	var status ServerStatus
	return status, c.Call(ctx, "server.status", nil, &status)
}
