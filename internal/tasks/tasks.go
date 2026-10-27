// Package tasks is remuda's own task list — a project tree, each project
// with its workflow columns and fields, tasks with labels, comments and
// agent runs — in one SQLite file (tasks.db, WAL) in the state dir.
package tasks

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // the driver, pure Go: cross-compiles with CGO_ENABLED=0
)

const file = "tasks.db"

// PageSize is how many tasks a page of the list holds.
const PageSize = 50

// BoardDone is how many done tasks a board column shows, the latest ones.
const BoardDone = 50

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid")
)

func invalid(msg string) error { return fmt.Errorf("%w: %s", ErrInvalid, msg) }

type Project struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	HexColor    string   `json:"hex_color"`
	ParentID    int64    `json:"parent_project_id"`
	Archived    bool     `json:"is_archived"`
	Favorite    bool     `json:"is_favorite"`
	Position    float64  `json:"position"`
	Created     string   `json:"created"`
	Updated     string   `json:"updated"`
	Statuses    []Status `json:"statuses"`
	Fields      []Field  `json:"fields"`
	Proposed    int      `json:"proposed"` // the open tasks agents put in its on_agent column, for the person to triage
}

// Status is one column of a project's board; a task is always in exactly one, and done follows the column's kind.
// The flags say where the workflow's own moves land (new tasks, a started agent, a finish, an agent's proposal), at most one column per flag.
type Status struct {
	ID        int64   `json:"id"`
	ProjectID int64   `json:"project_id"`
	Title     string  `json:"title"`
	HexColor  string  `json:"hex_color"`
	Kind      string  `json:"kind"`
	Position  float64 `json:"position"`
	OnNew     bool    `json:"on_new"`
	OnStart   bool    `json:"on_start"`
	OnFinish  bool    `json:"on_finish"`
	OnAgent   bool    `json:"on_agent"` // where a task an agent makes lands, for the person to accept or decline
}

const (
	KindOpen = "open"
	KindDone = "done"
)

type Label struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	HexColor string `json:"hex_color"`
}

type Task struct {
	ID          int64          `json:"id"`
	ProjectID   int64          `json:"project_id"`
	StatusID    int64          `json:"status_id"`
	Title       string         `json:"title"`
	Description string         `json:"description"` // HTML: paragraphs from the editor
	Done        bool           `json:"done"`
	DoneAt      string         `json:"done_at"`  // RFC 3339 UTC, "" when open
	DueDate     string         `json:"due_date"` // RFC 3339 UTC, "" when none
	Priority    int            `json:"priority"`
	Position    float64        `json:"position"`
	Labels      []Label        `json:"labels"`
	Fields      map[string]any `json:"fields"`     // the project's fields' values, by field id
	Identifier  string         `json:"identifier"` // "#12"
	Source      string         `json:"source"`     // "" for a person; an agent's: the key's name and its pane, "GOG · w3:p1"
	Created     string         `json:"created"`
	Updated     string         `json:"updated"`
	Run         *Run           `json:"run,omitempty"`
	Comments    []Comment      `json:"comments,omitempty"`
}

// Run is an agent started on a task: a worktree made for it, the agent in it, the prompt sent once the agent is up.
type Run struct {
	ID         int64  `json:"id"`
	TaskID     int64  `json:"task_id"`
	Machine    string `json:"machine"`
	Workspace  string `json:"workspace"`
	Pane       string `json:"pane"`
	Dir        string `json:"dir"`
	Branch     string `json:"branch"`
	Agent      string `json:"agent"`
	State      string `json:"state"`
	Started    string `json:"started"`
	PromptSent string `json:"prompt_sent,omitempty"`
	Finished   string `json:"finished,omitempty"`
	Reported   string `json:"-"` // the transcript record the reported finish ended with
	Error      string `json:"error,omitempty"`
	PRURL      string `json:"pr_url,omitempty"`   // the pull request the agent opened, once it named it
	PRLabel    string `json:"pr_label,omitempty"` // "PR #15" / "MR !9"
}

const (
	RunStarting = "starting"
	RunRunning  = "running"
	RunFinished = "finished"
	RunGone     = "gone"
	RunFailed   = "failed"
)

// legacyInProgress is the label older versions put on a task while an agent was on it.
// seedWorkflows reads it once to place such tasks in the on_start column; it is never written again.
const legacyInProgress = "in progress"

type Comment struct {
	ID      int64  `json:"id"`
	TaskID  int64  `json:"task_id"`
	Author  string `json:"author"` // agent | you | remuda
	Body    string `json:"body"`   // markdown
	Created string `json:"created"`
}

// Patch is a partial update: a nil field stays as it is, a due date of "" clears it, a Fields key is a field id with null clearing the value.
// After places the task in its column (0 is the top); a changed column without it goes to the bottom.
type Patch struct {
	Title       *string                    `json:"title"`
	Description *string                    `json:"description"`
	Done        *bool                      `json:"done"`
	DueDate     *string                    `json:"due_date"`
	Priority    *int                       `json:"priority"`
	ProjectID   *int64                     `json:"project_id"`
	StatusID    *int64                     `json:"status_id"`
	After       *int64                     `json:"after"`
	Fields      map[string]json.RawMessage `json:"fields"`
}

type ProjectPatch struct {
	Title       *string  `json:"title"`
	Description *string  `json:"description"`
	HexColor    *string  `json:"hex_color"`
	ParentID    *int64   `json:"parent_project_id"`
	Archived    *bool    `json:"is_archived"`
	Favorite    *bool    `json:"is_favorite"`
	Position    *float64 `json:"position"`
}

// Query is one page of the list: a smart view or a project, a search, in the viewer's time zone.
type Query struct {
	View    string // today | upcoming | open | proposed | project | board
	Project int64
	Done    bool
	Search  string
	Page    int
	Now     time.Time
	Loc     *time.Location
}

type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open opens or creates the database in dir.
func Open(dir string) (*Store, error) {
	dsn := "file:" + filepath.Join(dir, file) + "?" + url.Values{
		"_pragma": {"journal_mode(WAL)", "busy_timeout(5000)", "foreign_keys(1)"},
	}.Encode()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one connection: SQLite has one writer at a time, and reads queue behind it
	st := &Store{db: db, now: time.Now}
	if err := st.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return st, nil
}

func (st *Store) Close() error { return st.db.Close() }

func (st *Store) migrate() error {
	_, err := st.db.Exec(`
CREATE TABLE IF NOT EXISTS projects (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  hex_color TEXT NOT NULL DEFAULT '',
  parent_id INTEGER NOT NULL DEFAULT 0,
  archived INTEGER NOT NULL DEFAULT 0,
  favorite INTEGER NOT NULL DEFAULT 0,
  position REAL NOT NULL DEFAULT 0,
  created TEXT NOT NULL,
  updated TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS tasks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  done INTEGER NOT NULL DEFAULT 0,
  done_at TEXT NOT NULL DEFAULT '',
  due_date TEXT NOT NULL DEFAULT '',
  priority INTEGER NOT NULL DEFAULT 0,
  position REAL NOT NULL DEFAULT 0,
  created TEXT NOT NULL,
  updated TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS tasks_project ON tasks(project_id, done);
CREATE INDEX IF NOT EXISTS tasks_due ON tasks(done, due_date);
CREATE TABLE IF NOT EXISTS labels (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  hex_color TEXT NOT NULL DEFAULT '',
  created TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS task_labels (
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  label_id INTEGER NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
  PRIMARY KEY (task_id, label_id)
);
CREATE TABLE IF NOT EXISTS task_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  machine TEXT NOT NULL,
  workspace TEXT NOT NULL,
  pane TEXT NOT NULL,
  dir TEXT NOT NULL DEFAULT '',
  branch TEXT NOT NULL DEFAULT '',
  agent TEXT NOT NULL,
  state TEXT NOT NULL,
  started TEXT NOT NULL,
  prompt_sent TEXT NOT NULL DEFAULT '',
  finished TEXT NOT NULL DEFAULT '',
  reported TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS task_runs_task ON task_runs(task_id, id);
CREATE TABLE IF NOT EXISTS comments (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  author TEXT NOT NULL,
  body TEXT NOT NULL,
  created TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS comments_task ON comments(task_id, id);
CREATE TABLE IF NOT EXISTS statuses (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  hex_color TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL DEFAULT 'open',
  position REAL NOT NULL DEFAULT 0,
  on_new INTEGER NOT NULL DEFAULT 0,
  on_start INTEGER NOT NULL DEFAULT 0,
  on_finish INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS statuses_project ON statuses(project_id, position);
CREATE TABLE IF NOT EXISTS fields (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  kind TEXT NOT NULL,
  options TEXT NOT NULL DEFAULT '[]',
  position REAL NOT NULL DEFAULT 0,
  on_card INTEGER NOT NULL DEFAULT 0,
  in_prompt INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS fields_project ON fields(project_id, position);
CREATE TABLE IF NOT EXISTS task_fields (
  task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  field_id INTEGER NOT NULL REFERENCES fields(id) ON DELETE CASCADE,
  value TEXT NOT NULL,
  PRIMARY KEY (task_id, field_id)
);`)
	if err != nil {
		return err
	}
	// the column came with the workflow; a store from before gets it here
	if err := st.addColumn("tasks", "status_id", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := st.addColumn("tasks", "source", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := st.addColumn("statuses", "on_agent", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := st.addColumn("projects", "proposed_seeded", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	for _, col := range []string{"pr_url", "pr_label"} {
		if err := st.addColumn("task_runs", col, "TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	if _, err := st.db.Exec(`CREATE INDEX IF NOT EXISTS tasks_status ON tasks(project_id, status_id, position)`); err != nil {
		return err
	}
	var n int
	if err := st.db.QueryRow(`SELECT count(*) FROM projects`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		if _, err := st.CreateProject("Inbox", 0); err != nil {
			return err
		}
	}
	if err := st.seedWorkflows(); err != nil {
		return err
	}
	return st.seedProposed()
}

// seedProposed gives every project from before the agents' column a "Proposed" column right before where new tasks land,
// once: a project the person later strips of it never gets it back (proposed_seeded notes the visit).
func (st *Store) seedProposed() error {
	rows, err := st.db.Query(`SELECT id FROM projects WHERE proposed_seeded = 0`)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		ss, err := st.statusesOf(id)
		if err != nil {
			return err
		}
		if proposals(ss) == nil {
			if at := landing(ss); at != nil {
				if _, err := st.db.Exec(`INSERT INTO statuses (project_id, title, hex_color, kind, position, on_agent) VALUES (?, ?, ?, ?, ?, 1)`,
					id, proposedColumn.Title, proposedColumn.HexColor, KindOpen, at.Position-0.5); err != nil {
					return err
				}
			}
		}
		if _, err := st.db.Exec(`UPDATE projects SET proposed_seeded = 1 WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

// addColumn adds a column to a table that does not have it yet.
func (st *Store) addColumn(table, col, def string) error {
	rows, err := st.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == col {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	_, err = st.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + col + ` ` + def)
	return err
}

// seedWorkflows gives every project without columns the default workflow and every task without a column its place.
// It runs at every open: done tasks go to the done column, those with the old "in progress" label to on_start, the rest where new tasks land.
func (st *Store) seedWorkflows() error {
	ps, err := st.Projects()
	if err != nil {
		return err
	}
	byProject := map[int64][]Status{}
	for _, p := range ps {
		ss := p.Statuses
		if len(ss) == 0 {
			if ss, err = st.SetStatuses(p.ID, DefaultStatuses(), nil); err != nil {
				return err
			}
		}
		byProject[p.ID] = ss
	}
	rows, err := st.db.Query(`SELECT t.id, t.project_id, t.done,
  EXISTS (SELECT 1 FROM task_labels tl JOIN labels l ON l.id = tl.label_id WHERE tl.task_id = t.id AND l.title = ? COLLATE NOCASE)
  FROM tasks t WHERE NOT EXISTS (SELECT 1 FROM statuses s WHERE s.id = t.status_id AND s.project_id = t.project_id)`, legacyInProgress)
	if err != nil {
		return err
	}
	type placing struct {
		id, project int64
		done, busy  bool
	}
	var todo []placing
	for rows.Next() {
		var p placing
		if err := rows.Scan(&p.id, &p.project, &p.done, &p.busy); err != nil {
			rows.Close()
			return err
		}
		todo = append(todo, p)
	}
	rows.Close()
	for _, p := range todo {
		ss := byProject[p.project]
		var s *Status
		switch {
		case p.done:
			s = doneStatus(ss)
		case p.busy:
			s = flagged(ss, func(x Status) bool { return x.OnStart })
		}
		if s == nil {
			s = landing(ss)
		}
		if s == nil {
			continue
		}
		if _, err := st.db.Exec(`UPDATE tasks SET status_id = ?, done = ?, done_at = CASE WHEN ? THEN done_at ELSE '' END WHERE id = ?`, s.ID, s.Kind == KindDone, s.Kind == KindDone, p.id); err != nil {
			return err
		}
	}
	return nil
}

func (st *Store) stamp() string { return st.now().UTC().Format(time.RFC3339) }

// ---- projects ----

const projectCols = `id, title, description, hex_color, parent_id, archived, favorite, position, created, updated`

func scanProject(sc interface{ Scan(...any) error }) (Project, error) {
	var p Project
	err := sc.Scan(&p.ID, &p.Title, &p.Description, &p.HexColor, &p.ParentID, &p.Archived, &p.Favorite, &p.Position, &p.Created, &p.Updated)
	p.Statuses = []Status{}
	p.Fields = []Field{}
	return p, err
}

func (st *Store) Projects() ([]Project, error) {
	rows, err := st.db.Query(`SELECT ` + projectCols + ` FROM projects ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	out := []Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	all, err := st.statusesOf(0)
	if err != nil {
		return nil, err
	}
	byProject := map[int64][]Status{}
	for _, s := range all {
		byProject[s.ProjectID] = append(byProject[s.ProjectID], s)
	}
	allFields, err := st.fieldsOf(0)
	if err != nil {
		return nil, err
	}
	fieldsByProject := map[int64][]Field{}
	for _, f := range allFields {
		fieldsByProject[f.ProjectID] = append(fieldsByProject[f.ProjectID], f)
	}
	proposed, err := st.proposedCounts(0)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if ss := byProject[out[i].ID]; ss != nil {
			out[i].Statuses = ss
		}
		if fs := fieldsByProject[out[i].ID]; fs != nil {
			out[i].Fields = fs
		}
		out[i].Proposed = proposed[out[i].ID]
	}
	return out, nil
}

// proposedCounts is how many open tasks sit in each project's on_agent column; project 0 counts every project's.
func (st *Store) proposedCounts(projectID int64) (map[int64]int, error) {
	q := `SELECT t.project_id, count(*) FROM tasks t JOIN statuses s ON s.id = t.status_id WHERE s.on_agent = 1 AND t.done = 0`
	var args []any
	if projectID != 0 {
		q += ` AND t.project_id = ?`
		args = append(args, projectID)
	}
	rows, err := st.db.Query(q+` GROUP BY t.project_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

func (st *Store) Project(id int64) (Project, error) {
	p, err := scanProject(st.db.QueryRow(`SELECT `+projectCols+` FROM projects WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	if p.Statuses, err = st.statusesOf(id); err != nil {
		return p, err
	}
	if p.Fields, err = st.fieldsOf(id); err != nil {
		return p, err
	}
	proposed, err := st.proposedCounts(id)
	if err != nil {
		return p, err
	}
	p.Proposed = proposed[id]
	return p, nil
}

// CreateProject makes a project with a copy of its parent's workflow and fields, or the default workflow at the top level.
func (st *Store) CreateProject(title string, parent int64) (Project, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Project{}, invalid("a project needs a name")
	}
	tpl := DefaultStatuses()
	var fields []Field
	if parent != 0 {
		pp, err := st.Project(parent)
		if err != nil {
			return Project{}, fmt.Errorf("parent project: %w", err)
		}
		if len(pp.Statuses) > 0 {
			tpl = pp.Statuses
		}
		fields = pp.Fields
	}
	now := st.stamp()
	var pos float64
	_ = st.db.QueryRow(`SELECT coalesce(max(position), 0) + 1 FROM projects`).Scan(&pos)
	res, err := st.db.Exec(`INSERT INTO projects (title, parent_id, position, created, updated, proposed_seeded) VALUES (?, ?, ?, ?, ?, 1)`, title, parent, pos, now, now)
	if err != nil {
		return Project{}, err
	}
	id, _ := res.LastInsertId()
	copies := make([]Status, len(tpl))
	for i, s := range tpl {
		s.ID, s.ProjectID = 0, 0
		copies[i] = s
	}
	if _, err := st.SetStatuses(id, copies, nil); err != nil {
		return Project{}, err
	}
	if len(fields) > 0 {
		fcopies := make([]Field, len(fields))
		for i, f := range fields {
			f.ID, f.ProjectID = 0, 0
			fcopies[i] = f
		}
		if _, err := st.SetFields(id, fcopies); err != nil {
			return Project{}, err
		}
	}
	return st.Project(id)
}

func (st *Store) UpdateProject(id int64, p ProjectPatch) (Project, error) {
	cur, err := st.Project(id)
	if err != nil {
		return cur, err
	}
	if p.Title != nil {
		if cur.Title = strings.TrimSpace(*p.Title); cur.Title == "" {
			return cur, invalid("a project needs a name")
		}
	}
	if p.Description != nil {
		cur.Description = *p.Description
	}
	if p.HexColor != nil {
		if cur.HexColor, err = normColor(*p.HexColor); err != nil {
			return cur, err
		}
	}
	if p.ParentID != nil {
		for pid := *p.ParentID; pid != 0; {
			if pid == id {
				return cur, invalid("a project cannot be inside itself")
			}
			parent, err := st.Project(pid)
			if err != nil {
				return cur, fmt.Errorf("parent project: %w", err)
			}
			pid = parent.ParentID
		}
		cur.ParentID = *p.ParentID
	}
	if p.Archived != nil {
		cur.Archived = *p.Archived
	}
	if p.Favorite != nil {
		cur.Favorite = *p.Favorite
	}
	if p.Position != nil {
		cur.Position = *p.Position
	}
	_, err = st.db.Exec(`UPDATE projects SET title=?, description=?, hex_color=?, parent_id=?, archived=?, favorite=?, position=?, updated=? WHERE id=?`,
		cur.Title, cur.Description, cur.HexColor, cur.ParentID, cur.Archived, cur.Favorite, cur.Position, st.stamp(), id)
	if err != nil {
		return cur, err
	}
	return st.Project(id)
}

// DeleteProject removes a project with its tasks and columns; projects inside it move to the top level.
func (st *Store) DeleteProject(id int64) error {
	if _, err := st.Project(id); err != nil {
		return err
	}
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE projects SET parent_id = 0 WHERE parent_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM projects WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- the workflow: a project's columns ----

// DefaultStatuses is the workflow a project starts with when it has no parent to copy.
// proposedColumn is the column agents' tasks land in, as the default workflow and the seeding of an older project make it.
var proposedColumn = Status{Title: "Proposed", HexColor: "9b59b6", Kind: KindOpen, OnAgent: true}

// DefaultStatuses is the workflow a new top-level project gets: agents' proposals, then the person's own columns.
func DefaultStatuses() []Status {
	return []Status{
		proposedColumn,
		{Title: "New", HexColor: "95a5a6", Kind: KindOpen, OnNew: true},
		{Title: "In progress", HexColor: "3498db", Kind: KindOpen, OnStart: true},
		{Title: "Testing", HexColor: "e67e22", Kind: KindOpen, OnFinish: true},
		{Title: "Done", HexColor: "2ecc71", Kind: KindDone},
	}
}

const statusCols = `id, project_id, title, hex_color, kind, position, on_new, on_start, on_finish, on_agent`

// statusesOf lists a project's columns in order; project 0 lists every project's.
func (st *Store) statusesOf(projectID int64) ([]Status, error) {
	q := `SELECT ` + statusCols + ` FROM statuses`
	var args []any
	if projectID != 0 {
		q += ` WHERE project_id = ?`
		args = append(args, projectID)
	}
	rows, err := st.db.Query(q+` ORDER BY project_id, position, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Status{}
	for rows.Next() {
		var s Status
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.Title, &s.HexColor, &s.Kind, &s.Position, &s.OnNew, &s.OnStart, &s.OnFinish, &s.OnAgent); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func statusByID(ss []Status, id int64) *Status {
	for i := range ss {
		if ss[i].ID == id {
			return &ss[i]
		}
	}
	return nil
}

func statusByTitle(ss []Status, title string) *Status {
	for i := range ss {
		if strings.EqualFold(ss[i].Title, title) {
			return &ss[i]
		}
	}
	return nil
}

func flagged(ss []Status, flag func(Status) bool) *Status {
	for i := range ss {
		if flag(ss[i]) {
			return &ss[i]
		}
	}
	return nil
}

// landing is where a new task goes: the on_new column, else the first open one, else the first.
func landing(ss []Status) *Status {
	if s := flagged(ss, func(x Status) bool { return x.OnNew }); s != nil {
		return s
	}
	if s := flagged(ss, func(x Status) bool { return x.Kind != KindDone }); s != nil {
		return s
	}
	if len(ss) > 0 {
		return &ss[0]
	}
	return nil
}

// proposals is the column an agent's task lands in for the person to triage, nil when the project keeps none.
func proposals(ss []Status) *Status {
	return flagged(ss, func(x Status) bool { return x.OnAgent })
}

// doneStatus is the first done column, else nil.
func doneStatus(ss []Status) *Status {
	return flagged(ss, func(x Status) bool { return x.Kind == KindDone })
}

// SetStatuses replaces a project's columns with want, in that order: id 0 is a new column, a known id keeps its tasks, one left out is removed.
// A removed column's tasks go to moves[old id], required while it has any; at least one open and one done column, each flag on at most one.
func (st *Store) SetStatuses(projectID int64, want []Status, moves map[int64]int64) ([]Status, error) {
	var n int
	if err := st.db.QueryRow(`SELECT count(*) FROM projects WHERE id = ?`, projectID).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, fmt.Errorf("project: %w", ErrNotFound)
	}
	if len(want) == 0 {
		return nil, invalid("a project needs at least one column")
	}
	cur, err := st.statusesOf(projectID)
	if err != nil {
		return nil, err
	}
	open, done := 0, 0
	flags := map[string]int{}
	keep := map[int64]bool{}
	for i := range want {
		w := &want[i]
		w.Title = strings.TrimSpace(w.Title)
		if w.Title == "" {
			return nil, invalid("a column needs a name")
		}
		var err error
		if w.HexColor, err = normColor(w.HexColor); err != nil {
			return nil, err
		}
		switch w.Kind {
		case "", KindOpen:
			w.Kind = KindOpen
			open++
		case KindDone:
			done++
		default:
			return nil, invalid(fmt.Sprintf("column %q: a kind is open or done, not %q", w.Title, w.Kind))
		}
		if w.OnNew {
			flags["take new tasks"]++
		}
		if w.OnStart {
			flags["take a task its agent was started on"]++
		}
		if w.OnFinish {
			flags["take a task its agent finished"]++
		}
		if w.OnAgent {
			flags["take the tasks agents propose"]++
		}
		if w.ID != 0 {
			if statusByID(cur, w.ID) == nil {
				return nil, invalid(fmt.Sprintf("column %d is not this project's", w.ID))
			}
			if keep[w.ID] {
				return nil, invalid(fmt.Sprintf("column %q is listed twice", w.Title))
			}
			keep[w.ID] = true
		}
	}
	if open == 0 || done == 0 {
		return nil, invalid("a project needs an open column and a done column")
	}
	for what, c := range flags {
		if c > 1 {
			return nil, invalid("only one column can " + what)
		}
	}
	counts := map[int64]int{}
	rows, err := st.db.Query(`SELECT status_id, count(*) FROM tasks WHERE project_id = ? GROUP BY status_id`, projectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var c int
		if err := rows.Scan(&id, &c); err != nil {
			rows.Close()
			return nil, err
		}
		counts[id] = c
	}
	rows.Close()
	type removal struct{ from, to int64 }
	var gone []removal
	for _, c := range cur {
		if keep[c.ID] {
			continue
		}
		to := moves[c.ID]
		if counts[c.ID] > 0 && !keep[to] {
			return nil, invalid(fmt.Sprintf("column %q has %d tasks: say which column they move to", c.Title, counts[c.ID]))
		}
		gone = append(gone, removal{c.ID, to})
	}

	tx, err := st.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for i := range want {
		w := &want[i]
		w.ProjectID = projectID
		w.Position = float64(i + 1)
		if w.ID == 0 {
			res, err := tx.Exec(`INSERT INTO statuses (project_id, title, hex_color, kind, position, on_new, on_start, on_finish, on_agent) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				projectID, w.Title, w.HexColor, w.Kind, w.Position, w.OnNew, w.OnStart, w.OnFinish, w.OnAgent)
			if err != nil {
				return nil, err
			}
			w.ID, _ = res.LastInsertId()
		} else if _, err := tx.Exec(`UPDATE statuses SET title=?, hex_color=?, kind=?, position=?, on_new=?, on_start=?, on_finish=?, on_agent=? WHERE id=?`,
			w.Title, w.HexColor, w.Kind, w.Position, w.OnNew, w.OnStart, w.OnFinish, w.OnAgent, w.ID); err != nil {
			return nil, err
		}
	}
	for _, g := range gone {
		if g.to != 0 {
			if _, err := tx.Exec(`UPDATE tasks SET status_id = ? WHERE status_id = ?`, g.to, g.from); err != nil {
				return nil, err
			}
		}
		if _, err := tx.Exec(`DELETE FROM statuses WHERE id = ?`, g.from); err != nil {
			return nil, err
		}
	}
	now := st.stamp()
	for _, w := range want {
		if w.Kind == KindDone {
			_, err = tx.Exec(`UPDATE tasks SET done = 1, done_at = ?, updated = ? WHERE status_id = ? AND done = 0`, now, now, w.ID)
		} else {
			_, err = tx.Exec(`UPDATE tasks SET done = 0, done_at = '', updated = ? WHERE status_id = ? AND done = 1`, now, w.ID)
		}
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return st.statusesOf(projectID)
}

// placeAfter is the position for a task that goes right after `after` in its column (0: the top).
// It is the midpoint between the new neighbours; the column is renumbered first when two of them have no room between them.
func placeAfter(tx *sql.Tx, projectID, statusID, self, after int64) (float64, error) {
	rows, err := tx.Query(`SELECT id, position FROM tasks WHERE project_id = ? AND status_id = ? AND id <> ? ORDER BY position, id DESC`, projectID, statusID, self)
	if err != nil {
		return 0, err
	}
	var ids []int64
	var poss []float64
	for rows.Next() {
		var id int64
		var pos float64
		if err := rows.Scan(&id, &pos); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
		poss = append(poss, pos)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	idx := -1
	if after != 0 {
		for i, id := range ids {
			if id == after {
				idx = i
			}
		}
		if idx < 0 {
			return 0, invalid("the task to follow is not in that column")
		}
	}
	neighbours := func() (prev, next float64) {
		switch {
		case len(ids) == 0:
			return 0, 2
		case idx < 0:
			return poss[0] - 2, poss[0]
		case idx+1 < len(ids):
			return poss[idx], poss[idx+1]
		default:
			return poss[idx], poss[idx] + 2
		}
	}
	prev, next := neighbours()
	if next-prev < 1e-6 {
		for i, id := range ids {
			poss[i] = float64(i + 1)
			if _, err := tx.Exec(`UPDATE tasks SET position = ? WHERE id = ?`, poss[i], id); err != nil {
				return 0, err
			}
		}
		prev, next = neighbours()
	}
	return (prev + next) / 2, nil
}

// bottom is the position after the last task of a column.
func bottom(q interface {
	QueryRow(string, ...any) *sql.Row
}, projectID, statusID int64) float64 {
	var pos float64
	_ = q.QueryRow(`SELECT coalesce(max(position), 0) + 1 FROM tasks WHERE project_id = ? AND status_id = ?`, projectID, statusID).Scan(&pos)
	return pos
}

// ---- labels ----

func (st *Store) Labels() ([]Label, error) {
	rows, err := st.db.Query(`SELECT id, title, hex_color FROM labels ORDER BY title COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Label{}
	for rows.Next() {
		var l Label
		if err := rows.Scan(&l.ID, &l.Title, &l.HexColor); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// CreateLabel makes a label, or returns the one already called that.
func (st *Store) CreateLabel(title, hexColor string) (Label, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Label{}, invalid("a label needs a name")
	}
	all, err := st.Labels()
	if err != nil {
		return Label{}, err
	}
	for _, l := range all {
		if strings.EqualFold(l.Title, title) {
			return l, nil
		}
	}
	color, err := normColor(hexColor)
	if err != nil {
		return Label{}, err
	}
	res, err := st.db.Exec(`INSERT INTO labels (title, hex_color, created) VALUES (?, ?, ?)`, title, color, st.stamp())
	if err != nil {
		return Label{}, err
	}
	id, _ := res.LastInsertId()
	return Label{ID: id, Title: title, HexColor: color}, nil
}

func (st *Store) DeleteLabel(id int64) error {
	res, err := st.db.Exec(`DELETE FROM labels WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// exists is Get as a yes/no: no labels, runs or comments loaded.
func (st *Store) exists(id int64) error {
	var n int
	if err := st.db.QueryRow(`SELECT count(*) FROM tasks WHERE id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (st *Store) AddLabel(taskID, labelID int64) error {
	if err := st.exists(taskID); err != nil {
		return err
	}
	var n int
	if err := st.db.QueryRow(`SELECT count(*) FROM labels WHERE id = ?`, labelID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("label: %w", ErrNotFound)
	}
	_, err := st.db.Exec(`INSERT OR IGNORE INTO task_labels (task_id, label_id) VALUES (?, ?)`, taskID, labelID)
	return err
}

func (st *Store) RemoveLabel(taskID, labelID int64) error {
	_, err := st.db.Exec(`DELETE FROM task_labels WHERE task_id = ? AND label_id = ?`, taskID, labelID)
	return err
}

// ---- tasks ----

const taskCols = `id, project_id, status_id, title, description, done, done_at, due_date, priority, position, created, updated, source`

func scanTask(sc interface{ Scan(...any) error }) (Task, error) {
	var t Task
	err := sc.Scan(&t.ID, &t.ProjectID, &t.StatusID, &t.Title, &t.Description, &t.Done, &t.DoneAt, &t.DueDate, &t.Priority, &t.Position, &t.Created, &t.Updated, &t.Source)
	t.Identifier = fmt.Sprintf("#%d", t.ID)
	t.Labels = []Label{}
	t.Fields = map[string]any{}
	return t, err
}

func (st *Store) Get(id int64) (Task, error) {
	t, err := scanTask(st.db.QueryRow(`SELECT `+taskCols+` FROM tasks WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, err
	}
	if err := st.fillLabels([]*Task{&t}); err != nil {
		return t, err
	}
	if err := st.fillFields([]*Task{&t}); err != nil {
		return t, err
	}
	if err := st.fillRuns([]*Task{&t}); err != nil {
		return t, err
	}
	if t.Comments, err = st.Comments(id); err != nil {
		return t, err
	}
	return t, nil
}

// fillRuns attaches each task's latest run.
func (st *Store) fillRuns(tasks []*Task) error {
	if len(tasks) == 0 {
		return nil
	}
	byID, ids := index(tasks)
	rows, err := st.db.Query(`SELECT ` + runCols + ` FROM task_runs WHERE id IN (SELECT max(id) FROM task_runs WHERE task_id IN (` + ids + `) GROUP BY task_id)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return err
		}
		if t := byID[r.TaskID]; t != nil {
			rr := r
			t.Run = &rr
		}
	}
	return rows.Err()
}

// index maps the tasks by id and lists the ids for an IN (…) clause.
func index(tasks []*Task) (map[int64]*Task, string) {
	byID := map[int64]*Task{}
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t
		ids = append(ids, strconv.FormatInt(t.ID, 10))
	}
	return byID, strings.Join(ids, ",")
}

func (st *Store) fillLabels(tasks []*Task) error {
	if len(tasks) == 0 {
		return nil
	}
	byID, ids := index(tasks)
	rows, err := st.db.Query(`SELECT tl.task_id, l.id, l.title, l.hex_color FROM task_labels tl JOIN labels l ON l.id = tl.label_id WHERE tl.task_id IN (` + ids + `) ORDER BY l.title COLLATE NOCASE`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var tid int64
		var l Label
		if err := rows.Scan(&tid, &l.ID, &l.Title, &l.HexColor); err != nil {
			return err
		}
		byID[tid].Labels = append(byID[tid].Labels, l)
	}
	return rows.Err()
}

// Create makes a task in a project: in the column asked for, else the done column for a task born done, else where new tasks land.
// Create makes a task a person wrote.
func (st *Store) Create(projectID int64, p Patch) (Task, error) {
	return st.CreateFrom(projectID, p, "")
}

// CreateFrom makes a task and notes its source, an agent's name (see Source), which no patch changes later;
// an agent's task that names no column goes to the project's on_agent column, for the person to accept or decline.
func (st *Store) CreateFrom(projectID int64, p Patch, source string) (Task, error) {
	project, err := st.Project(projectID)
	if err != nil {
		return Task{}, fmt.Errorf("project: %w", err)
	}
	title := ""
	if p.Title != nil {
		title = strings.TrimSpace(*p.Title)
	}
	if title == "" {
		return Task{}, invalid("a task needs a title")
	}
	t := Task{ProjectID: projectID, Title: title, Source: source}
	if p.Description != nil {
		t.Description = *p.Description
	}
	if p.DueDate != nil {
		d, err := normDate(*p.DueDate)
		if err != nil {
			return Task{}, err
		}
		t.DueDate = d
	}
	if p.Priority != nil {
		t.Priority = clampPriority(*p.Priority)
	}
	var s *Status
	switch {
	case p.StatusID != nil && *p.StatusID != 0:
		if s = statusByID(project.Statuses, *p.StatusID); s == nil {
			return Task{}, invalid("no such column in this project")
		}
	case p.Done != nil && *p.Done:
		s = doneStatus(project.Statuses)
	case source != "":
		s = proposals(project.Statuses)
	}
	if s == nil {
		s = landing(project.Statuses)
	}
	if s == nil {
		return Task{}, invalid("the project has no columns")
	}
	now := st.stamp()
	t.StatusID = s.ID
	if s.Kind == KindDone {
		t.Done, t.DoneAt = true, now
	}
	tx, err := st.db.Begin()
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	t.Position = bottom(tx, projectID, s.ID)
	res, err := tx.Exec(`INSERT INTO tasks (project_id, status_id, title, description, done, done_at, due_date, priority, position, created, updated, source) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ProjectID, t.StatusID, t.Title, t.Description, t.Done, t.DoneAt, t.DueDate, t.Priority, t.Position, now, now, t.Source)
	if err != nil {
		return Task{}, err
	}
	id, _ := res.LastInsertId()
	if err := applyFields(tx, id, project.Fields, p.Fields); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, err
	}
	return st.Get(id)
}

func (st *Store) Update(id int64, p Patch) (Task, error) {
	cur, err := st.Get(id)
	if err != nil {
		return cur, err
	}
	if p.Title != nil {
		if cur.Title = strings.TrimSpace(*p.Title); cur.Title == "" {
			return cur, invalid("a task needs a title")
		}
	}
	if p.Description != nil {
		cur.Description = *p.Description
	}
	if p.DueDate != nil {
		if cur.DueDate, err = normDate(*p.DueDate); err != nil {
			return cur, err
		}
	}
	if p.Priority != nil {
		cur.Priority = clampPriority(*p.Priority)
	}
	ss, err := st.statusesOf(cur.ProjectID)
	if err != nil {
		return cur, err
	}
	fields, err := st.fieldsOf(cur.ProjectID)
	if err != nil {
		return cur, err
	}
	moved := false
	var carryTo []Field
	if p.ProjectID != nil && *p.ProjectID != cur.ProjectID {
		target, err := st.Project(*p.ProjectID)
		if err != nil {
			return cur, fmt.Errorf("project: %w", err)
		}
		carryTo = target.Fields
		var next *Status
		if old := statusByID(ss, cur.StatusID); old != nil {
			next = statusByTitle(target.Statuses, old.Title)
		}
		if next == nil && cur.Done {
			next = doneStatus(target.Statuses)
		}
		if next == nil {
			next = landing(target.Statuses)
		}
		if next == nil {
			return cur, invalid("the project has no columns")
		}
		cur.ProjectID, cur.StatusID, ss, moved = target.ID, next.ID, target.Statuses, true
	}
	switch {
	case p.StatusID != nil:
		s := statusByID(ss, *p.StatusID)
		if s == nil {
			return cur, invalid("no such column in this project")
		}
		if s.ID != cur.StatusID {
			cur.StatusID, moved = s.ID, true
		}
	case p.Done != nil && *p.Done != cur.Done:
		var s *Status
		if *p.Done {
			s = doneStatus(ss)
		} else {
			s = landing(ss)
		}
		if s == nil {
			return cur, invalid("the project has no column for that")
		}
		if s.ID != cur.StatusID {
			cur.StatusID, moved = s.ID, true
		}
	}
	now := st.stamp()
	if s := statusByID(ss, cur.StatusID); s != nil {
		if done := s.Kind == KindDone; done != cur.Done {
			cur.Done = done
			if done {
				cur.DoneAt = now
			} else {
				cur.DoneAt = ""
			}
		}
	}
	tx, err := st.db.Begin()
	if err != nil {
		return cur, err
	}
	defer tx.Rollback()
	switch {
	case p.After != nil:
		if cur.Position, err = placeAfter(tx, cur.ProjectID, cur.StatusID, id, *p.After); err != nil {
			return cur, err
		}
	case moved:
		cur.Position = bottom(tx, cur.ProjectID, cur.StatusID)
	}
	if _, err := tx.Exec(`UPDATE tasks SET project_id=?, status_id=?, title=?, description=?, done=?, done_at=?, due_date=?, priority=?, position=?, updated=? WHERE id=?`,
		cur.ProjectID, cur.StatusID, cur.Title, cur.Description, cur.Done, cur.DoneAt, cur.DueDate, cur.Priority, cur.Position, now, id); err != nil {
		return cur, err
	}
	if carryTo != nil {
		if err := carryFields(tx, id, fields, carryTo); err != nil {
			return cur, err
		}
		fields = carryTo
	}
	if err := applyFields(tx, id, fields, p.Fields); err != nil {
		return cur, err
	}
	if err := tx.Commit(); err != nil {
		return cur, err
	}
	return st.Get(id)
}

func (st *Store) Delete(id int64) error {
	res, err := st.db.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// List is one page of the query, with how many pages there are: open before done, the nearest due date first, undated last, newest first among equals.
// The search is case-folded in Go: SQLite's own lower() folds ASCII only, so Cyrillic would not match either way.
func (st *Store) List(q Query) ([]Task, int, error) {
	now := q.Now
	if now.IsZero() {
		now = st.now()
	}
	loc := q.Loc
	if loc == nil {
		loc = time.Local
	}
	where := []string{"1=1"}
	var args []any
	from, order := `tasks`, ` ORDER BY done ASC, CASE WHEN due_date = '' THEN 1 ELSE 0 END, due_date ASC, id DESC`
	switch q.View {
	case "today":
		y, m, d := now.In(loc).Date()
		tomorrow := time.Date(y, m, d+1, 0, 0, 0, 0, loc)
		where = append(where, "done = 0", "due_date != ''", "due_date < ?")
		args = append(args, tomorrow.UTC().Format(time.RFC3339))
	case "upcoming":
		where = append(where, "done = 0", "due_date != ''", "due_date < ?")
		args = append(args, now.Add(7*24*time.Hour).UTC().Format(time.RFC3339))
	case "open":
		where = append(where, "done = 0")
	case "proposed":
		where = append(where, "done = 0", "status_id IN (SELECT id FROM statuses WHERE on_agent = 1)")
		if q.Project != 0 {
			where = append(where, "project_id = ?")
			args = append(args, q.Project)
		}
		order = ` ORDER BY id DESC`
	case "project":
		where = append(where, "project_id = ?")
		args = append(args, q.Project)
		if !q.Done {
			where = append(where, "done = 0")
		}
	case "board":
		where = append(where, "tasks.project_id = ?")
		args = append(args, q.Project)
		from = `tasks LEFT JOIN statuses s ON s.id = tasks.status_id`
		order = ` ORDER BY s.position, CASE WHEN s.kind = 'done' THEN tasks.done_at ELSE '' END DESC, tasks.position, tasks.id DESC`
	default:
		return nil, 0, invalid(fmt.Sprintf("unknown view %q", q.View))
	}
	cols := taskCols
	if q.View == "board" {
		cols = "tasks." + strings.ReplaceAll(taskCols, ", ", ", tasks.")
	}
	rows, err := st.db.Query(`SELECT `+cols+` FROM `+from+` WHERE `+strings.Join(where, " AND ")+order, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var all []Task
	needle := strings.ToLower(strings.TrimSpace(q.Search))
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, 0, err
		}
		if needle != "" && !strings.Contains(strings.ToLower(t.Title), needle) && !strings.Contains(strings.ToLower(stripTags(t.Description)), needle) {
			continue
		}
		all = append(all, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()
	var out []Task
	pages := 1
	if q.View == "board" {
		seen := map[int64]int{}
		for _, t := range all {
			if t.Done {
				seen[t.StatusID]++
				if seen[t.StatusID] > BoardDone {
					continue
				}
			}
			out = append(out, t)
		}
	} else {
		pages = max(1, (len(all)+PageSize-1)/PageSize)
		page := min(max(q.Page, 1), pages)
		from := (page - 1) * PageSize
		to := min(from+PageSize, len(all))
		out = all[from:to]
	}
	ptrs := make([]*Task, len(out))
	for i := range out {
		ptrs[i] = &out[i]
	}
	if err := st.fillLabels(ptrs); err != nil {
		return nil, 0, err
	}
	if err := st.fillFields(ptrs); err != nil {
		return nil, 0, err
	}
	if err := st.fillRuns(ptrs); err != nil {
		return nil, 0, err
	}
	if out == nil {
		out = []Task{}
	}
	return out, pages, nil
}

// ---- runs and comments ----

const runCols = `id, task_id, machine, workspace, pane, dir, branch, agent, state, started, prompt_sent, finished, reported, error, pr_url, pr_label`

func scanRun(sc interface{ Scan(...any) error }) (Run, error) {
	var r Run
	err := sc.Scan(&r.ID, &r.TaskID, &r.Machine, &r.Workspace, &r.Pane, &r.Dir, &r.Branch, &r.Agent, &r.State, &r.Started, &r.PromptSent, &r.Finished, &r.Reported, &r.Error, &r.PRURL, &r.PRLabel)
	return r, err
}

// StartRun records an agent started on a task and moves the task to its project's on_start column, in one transaction.
func (st *Store) StartRun(r Run) (Run, error) {
	t, err := st.Get(r.TaskID)
	if err != nil {
		return r, err
	}
	if r.State == "" {
		r.State = RunStarting
	}
	r.Started = st.stamp()
	ss, err := st.statusesOf(t.ProjectID)
	if err != nil {
		return r, err
	}
	tx, err := st.db.Begin()
	if err != nil {
		return r, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO task_runs (task_id, machine, workspace, pane, dir, branch, agent, state, started) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.TaskID, r.Machine, r.Workspace, r.Pane, r.Dir, r.Branch, r.Agent, r.State, r.Started)
	if err != nil {
		return r, err
	}
	r.ID, _ = res.LastInsertId()
	if s := flagged(ss, func(x Status) bool { return x.OnStart }); s != nil && s.ID != t.StatusID {
		if err := moveTo(tx, t, *s, r.Started); err != nil {
			return r, err
		}
	}
	return r, tx.Commit()
}

// OnFinish moves a task whose agent finished to its project's on_finish column and returns it, or nil when nothing moved.
// Only a task not done and still where the start put it moves: a card the person moved by hand stays.
func (st *Store) OnFinish(taskID int64) (*Status, error) {
	t, err := st.Get(taskID)
	if err != nil {
		return nil, err
	}
	ss, err := st.statusesOf(t.ProjectID)
	if err != nil {
		return nil, err
	}
	to := flagged(ss, func(x Status) bool { return x.OnFinish })
	if to == nil || to.ID == t.StatusID || t.Done {
		return nil, nil
	}
	if from := flagged(ss, func(x Status) bool { return x.OnStart }); from != nil && from.ID != t.StatusID {
		return nil, nil
	}
	tx, err := st.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := moveTo(tx, t, *to, st.stamp()); err != nil {
		return nil, err
	}
	return to, tx.Commit()
}

// moveTo puts a task at the bottom of a column, done following its kind.
func moveTo(tx *sql.Tx, t Task, s Status, now string) error {
	doneAt := ""
	if s.Kind == KindDone {
		doneAt = t.DoneAt
		if doneAt == "" {
			doneAt = now
		}
	}
	_, err := tx.Exec(`UPDATE tasks SET status_id = ?, done = ?, done_at = ?, position = ?, updated = ? WHERE id = ?`,
		s.ID, s.Kind == KindDone, doneAt, bottom(tx, t.ProjectID, s.ID), now, t.ID)
	return err
}

func (st *Store) Run(id int64) (Run, error) {
	r, err := scanRun(st.db.QueryRow(`SELECT `+runCols+` FROM task_runs WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// OpenRuns are the runs still to watch: starting, running, or finished with the pane still there.
func (st *Store) OpenRuns() ([]Run, error) {
	rows, err := st.db.Query(`SELECT `+runCols+` FROM task_runs WHERE state IN (?, ?, ?) ORDER BY id`, RunStarting, RunRunning, RunFinished)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetRun moves a run on to a state with its detail: the reported record for finished, else an error message.
func (st *Store) SetRun(id int64, state string, detail string) error {
	now := st.stamp()
	var res sql.Result
	var err error
	switch state {
	case RunRunning:
		res, err = st.db.Exec(`UPDATE task_runs SET state = ?, prompt_sent = ?, error = '' WHERE id = ?`, state, now, id)
	case RunFinished:
		res, err = st.db.Exec(`UPDATE task_runs SET state = ?, finished = ?, reported = ? WHERE id = ?`, state, now, detail, id)
	default:
		res, err = st.db.Exec(`UPDATE task_runs SET state = ?, error = ? WHERE id = ?`, state, detail, id)
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (st *Store) AddComment(taskID int64, author, body string) (Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return Comment{}, invalid("an empty comment")
	}
	if err := st.exists(taskID); err != nil {
		return Comment{}, err
	}
	c := Comment{TaskID: taskID, Author: author, Body: body, Created: st.stamp()}
	res, err := st.db.Exec(`INSERT INTO comments (task_id, author, body, created) VALUES (?, ?, ?, ?)`, taskID, author, body, c.Created)
	if err != nil {
		return c, err
	}
	c.ID, _ = res.LastInsertId()
	return c, nil
}

func (st *Store) Comments(taskID int64) ([]Comment, error) {
	rows, err := st.db.Query(`SELECT id, task_id, author, body, created FROM comments WHERE task_id = ? ORDER BY id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.TaskID, &c.Author, &c.Body, &c.Created); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (st *Store) DeleteComment(id int64) error {
	res, err := st.db.Exec(`DELETE FROM comments WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- helpers ----

// normDate accepts RFC 3339 (any zone) and keeps UTC; "" clears.
func normDate(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "0001-") {
		return "", nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return "", invalid(fmt.Sprintf("bad date %q: want RFC 3339", s))
	}
	return t.UTC().Format(time.RFC3339), nil
}

func clampPriority(p int) int { return min(max(p, 0), 5) }

var colorRe = regexp.MustCompile(`^(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// normColor is a colour as the UI keeps it: six (or three) hex digits without the "#", or nothing.
// The value lands in a style attribute, so nothing else passes.
func normColor(s string) (string, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if s != "" && !colorRe.MatchString(s) {
		return "", invalid(fmt.Sprintf("colour %q: six hex digits, please", s))
	}
	return strings.ToLower(s), nil
}

// stripTags is enough of an HTML-to-text for searching a description.
func stripTags(s string) string {
	if !strings.Contains(s, "<") {
		return s
	}
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
			b.WriteByte(' ')
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}
