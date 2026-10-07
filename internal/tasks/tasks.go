// Package tasks is herdr-ui's own task list: projects, tasks, labels, in
// one SQLite file in the state dir (tasks.db, WAL). No server but this one,
// no account, nothing leaves the machine. The shapes are what the Tasks
// mode shows: a project tree, tasks with a due date, a priority, labels
// and an HTML description, three smart lists over them.
package tasks

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // the driver, pure Go: cross-compiles with CGO_ENABLED=0
)

const file = "tasks.db"

// PageSize is how many tasks a page of the list holds.
const PageSize = 50

var ErrNotFound = errors.New("not found")

type Project struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	HexColor    string  `json:"hex_color"`
	ParentID    int64   `json:"parent_project_id"`
	Archived    bool    `json:"is_archived"`
	Favorite    bool    `json:"is_favorite"`
	Position    float64 `json:"position"`
	Created     string  `json:"created"`
	Updated     string  `json:"updated"`
}

type Label struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	HexColor string `json:"hex_color"`
}

type Task struct {
	ID          int64   `json:"id"`
	ProjectID   int64   `json:"project_id"`
	Title       string  `json:"title"`
	Description string  `json:"description"` // HTML: paragraphs from the editor
	Done        bool    `json:"done"`
	DoneAt      string  `json:"done_at"`  // RFC 3339 UTC, "" when open
	DueDate     string  `json:"due_date"` // RFC 3339 UTC, "" when none
	Priority    int     `json:"priority"` // 0 none … 5 do now
	Labels      []Label `json:"labels"`
	Identifier  string  `json:"identifier"` // "#12"
	Created     string  `json:"created"`
	Updated     string  `json:"updated"`
}

// Patch is a partial update: a nil field stays as it is. A due date of ""
// clears it.
type Patch struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Done        *bool   `json:"done"`
	DueDate     *string `json:"due_date"`
	Priority    *int    `json:"priority"`
	ProjectID   *int64  `json:"project_id"`
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

// Query is one page of the list: a smart view or a project, open tasks or
// all, a search, in the viewer's time zone (the smart views cut by local
// days).
type Query struct {
	View    string // today | upcoming | open | project
	Project int64
	Done    bool // include done tasks (project view)
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
	db.SetMaxOpenConns(1) // one writer; reads queue behind it, which is fine for a personal list
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
);`)
	if err != nil {
		return err
	}
	// a first list has somewhere to add to
	var n int
	if err := st.db.QueryRow(`SELECT count(*) FROM projects`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		_, err = st.CreateProject("Inbox", 0)
	}
	return err
}

func (st *Store) stamp() string { return st.now().UTC().Format(time.RFC3339) }

// ---- projects ----

func (st *Store) Projects() ([]Project, error) {
	rows, err := st.db.Query(`SELECT id, title, description, hex_color, parent_id, archived, favorite, position, created, updated FROM projects ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Title, &p.Description, &p.HexColor, &p.ParentID, &p.Archived, &p.Favorite, &p.Position, &p.Created, &p.Updated); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (st *Store) Project(id int64) (Project, error) {
	var p Project
	err := st.db.QueryRow(`SELECT id, title, description, hex_color, parent_id, archived, favorite, position, created, updated FROM projects WHERE id = ?`, id).
		Scan(&p.ID, &p.Title, &p.Description, &p.HexColor, &p.ParentID, &p.Archived, &p.Favorite, &p.Position, &p.Created, &p.Updated)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func (st *Store) CreateProject(title string, parent int64) (Project, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Project{}, errors.New("a project needs a name")
	}
	if parent != 0 {
		if _, err := st.Project(parent); err != nil {
			return Project{}, fmt.Errorf("parent project: %w", err)
		}
	}
	now := st.stamp()
	var pos float64
	_ = st.db.QueryRow(`SELECT coalesce(max(position), 0) + 1 FROM projects`).Scan(&pos)
	res, err := st.db.Exec(`INSERT INTO projects (title, parent_id, position, created, updated) VALUES (?, ?, ?, ?, ?)`, title, parent, pos, now, now)
	if err != nil {
		return Project{}, err
	}
	id, _ := res.LastInsertId()
	return st.Project(id)
}

func (st *Store) UpdateProject(id int64, p ProjectPatch) (Project, error) {
	cur, err := st.Project(id)
	if err != nil {
		return cur, err
	}
	if p.Title != nil {
		if cur.Title = strings.TrimSpace(*p.Title); cur.Title == "" {
			return cur, errors.New("a project needs a name")
		}
	}
	if p.Description != nil {
		cur.Description = *p.Description
	}
	if p.HexColor != nil {
		cur.HexColor = strings.TrimPrefix(strings.TrimSpace(*p.HexColor), "#")
	}
	if p.ParentID != nil {
		if *p.ParentID == id {
			return cur, errors.New("a project cannot be inside itself")
		}
		if *p.ParentID != 0 {
			if _, err := st.Project(*p.ParentID); err != nil {
				return cur, fmt.Errorf("parent project: %w", err)
			}
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

// DeleteProject removes a project with its tasks; projects inside it move
// to the top level.
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
		return Label{}, errors.New("a label needs a name")
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
	res, err := st.db.Exec(`INSERT INTO labels (title, hex_color, created) VALUES (?, ?, ?)`, title, strings.TrimPrefix(hexColor, "#"), st.stamp())
	if err != nil {
		return Label{}, err
	}
	id, _ := res.LastInsertId()
	return Label{ID: id, Title: title, HexColor: strings.TrimPrefix(hexColor, "#")}, nil
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

func (st *Store) AddLabel(taskID, labelID int64) error {
	if _, err := st.Get(taskID); err != nil {
		return err
	}
	var n int
	if err := st.db.QueryRow(`SELECT count(*) FROM labels WHERE id = ?`, labelID).Scan(&n); err != nil || n == 0 {
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

const taskCols = `id, project_id, title, description, done, done_at, due_date, priority, created, updated`

func scanTask(sc interface{ Scan(...any) error }) (Task, error) {
	var t Task
	err := sc.Scan(&t.ID, &t.ProjectID, &t.Title, &t.Description, &t.Done, &t.DoneAt, &t.DueDate, &t.Priority, &t.Created, &t.Updated)
	t.Identifier = fmt.Sprintf("#%d", t.ID)
	t.Labels = []Label{}
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
	return t, nil
}

func (st *Store) fillLabels(tasks []*Task) error {
	if len(tasks) == 0 {
		return nil
	}
	byID := map[int64]*Task{}
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t
		ids = append(ids, fmt.Sprint(t.ID))
	}
	rows, err := st.db.Query(`SELECT tl.task_id, l.id, l.title, l.hex_color FROM task_labels tl JOIN labels l ON l.id = tl.label_id WHERE tl.task_id IN (` + strings.Join(ids, ",") + `) ORDER BY l.title COLLATE NOCASE`)
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

func (st *Store) Create(projectID int64, p Patch) (Task, error) {
	if _, err := st.Project(projectID); err != nil {
		return Task{}, fmt.Errorf("project: %w", err)
	}
	title := ""
	if p.Title != nil {
		title = strings.TrimSpace(*p.Title)
	}
	if title == "" {
		return Task{}, errors.New("a task needs a title")
	}
	t := Task{ProjectID: projectID, Title: title}
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
	now := st.stamp()
	if p.Done != nil && *p.Done {
		t.Done, t.DoneAt = true, now
	}
	res, err := st.db.Exec(`INSERT INTO tasks (project_id, title, description, done, done_at, due_date, priority, created, updated) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ProjectID, t.Title, t.Description, t.Done, t.DoneAt, t.DueDate, t.Priority, now, now)
	if err != nil {
		return Task{}, err
	}
	id, _ := res.LastInsertId()
	return st.Get(id)
}

func (st *Store) Update(id int64, p Patch) (Task, error) {
	cur, err := st.Get(id)
	if err != nil {
		return cur, err
	}
	if p.Title != nil {
		if cur.Title = strings.TrimSpace(*p.Title); cur.Title == "" {
			return cur, errors.New("a task needs a title")
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
	if p.ProjectID != nil && *p.ProjectID != cur.ProjectID {
		if _, err := st.Project(*p.ProjectID); err != nil {
			return cur, fmt.Errorf("project: %w", err)
		}
		cur.ProjectID = *p.ProjectID
	}
	now := st.stamp()
	if p.Done != nil && *p.Done != cur.Done {
		cur.Done = *p.Done
		if cur.Done {
			cur.DoneAt = now
		} else {
			cur.DoneAt = ""
		}
	}
	_, err = st.db.Exec(`UPDATE tasks SET project_id=?, title=?, description=?, done=?, done_at=?, due_date=?, priority=?, updated=? WHERE id=?`,
		cur.ProjectID, cur.Title, cur.Description, cur.Done, cur.DoneAt, cur.DueDate, cur.Priority, now, id)
	if err != nil {
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

// List is one page of the query, with how many pages there are. Order:
// open before done, the nearest due date first, undated last, newest
// first among equals. The search is case-folded on the Go side (SQLite's
// own folding is ASCII only), so it finds Cyrillic either way.
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
	case "project":
		where = append(where, "project_id = ?")
		args = append(args, q.Project)
		if !q.Done {
			where = append(where, "done = 0")
		}
	default:
		return nil, 0, fmt.Errorf("unknown view %q", q.View)
	}
	rows, err := st.db.Query(`SELECT `+taskCols+` FROM tasks WHERE `+strings.Join(where, " AND ")+
		` ORDER BY done ASC, CASE WHEN due_date = '' THEN 1 ELSE 0 END, due_date ASC, id DESC`, args...)
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
	pages := max(1, (len(all)+PageSize-1)/PageSize)
	page := min(max(q.Page, 1), pages)
	from := (page - 1) * PageSize
	to := min(from+PageSize, len(all))
	out := all[from:to]
	ptrs := make([]*Task, len(out))
	for i := range out {
		ptrs[i] = &out[i]
	}
	if err := st.fillLabels(ptrs); err != nil {
		return nil, 0, err
	}
	if out == nil {
		out = []Task{}
	}
	return out, pages, nil
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
		return "", fmt.Errorf("bad date %q: want RFC 3339", s)
	}
	return t.UTC().Format(time.RFC3339), nil
}

func clampPriority(p int) int { return min(max(p, 0), 5) }

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

// Sorted returns projects as a tree would list them: by position, parents
// first. (Exported for tests and the import.)
func Sorted(ps []Project) []Project {
	out := append([]Project(nil), ps...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out
}
