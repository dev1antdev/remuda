package tasks

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Field is one of a project's own fields, beyond the title, description, due date, priority and labels every task has.
// A task's value for it is kept as JSON, and only when it means something: an empty text, an unticked box, nothing chosen is no row at all.
type Field struct {
	ID        int64    `json:"id"`
	ProjectID int64    `json:"project_id"`
	Title     string   `json:"title"`
	Kind      string   `json:"kind"`
	Options   []string `json:"options"`
	Position  float64  `json:"position"`
	OnCard    bool     `json:"on_card"`
	InPrompt  bool     `json:"in_prompt"`
}

const (
	FieldText   = "text"
	FieldNumber = "number"
	FieldSelect = "select"
	FieldMulti  = "multi"
	FieldDate   = "date"
	FieldCheck  = "check"
	FieldLink   = "link"
)

var fieldKinds = map[string]bool{FieldText: true, FieldNumber: true, FieldSelect: true, FieldMulti: true, FieldDate: true, FieldCheck: true, FieldLink: true}

const fieldCols = `id, project_id, title, kind, options, position, on_card, in_prompt`

func scanField(sc interface{ Scan(...any) error }) (Field, error) {
	var f Field
	var opts string
	if err := sc.Scan(&f.ID, &f.ProjectID, &f.Title, &f.Kind, &opts, &f.Position, &f.OnCard, &f.InPrompt); err != nil {
		return f, err
	}
	f.Options = []string{}
	if opts != "" {
		_ = json.Unmarshal([]byte(opts), &f.Options)
	}
	return f, nil
}

// fieldsOf lists a project's fields in order; project 0 lists every project's.
func (st *Store) fieldsOf(projectID int64) ([]Field, error) {
	q := `SELECT ` + fieldCols + ` FROM fields`
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
	out := []Field{}
	for rows.Next() {
		f, err := scanField(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func fieldByID(fs []Field, id int64) *Field {
	for i := range fs {
		if fs[i].ID == id {
			return &fs[i]
		}
	}
	return nil
}

// SetFields replaces a project's fields with want, in that order: id 0 is a new field, a known id keeps its values, one left out goes with its values.
// A field whose kind changed loses its values; one whose options shrank loses the values no longer among them.
func (st *Store) SetFields(projectID int64, want []Field) ([]Field, error) {
	var n int
	if err := st.db.QueryRow(`SELECT count(*) FROM projects WHERE id = ?`, projectID).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, fmt.Errorf("project: %w", ErrNotFound)
	}
	cur, err := st.fieldsOf(projectID)
	if err != nil {
		return nil, err
	}
	keep := map[int64]bool{}
	for i := range want {
		w := &want[i]
		w.Title = strings.TrimSpace(w.Title)
		if w.Title == "" {
			return nil, invalid("a field needs a name")
		}
		if !fieldKinds[w.Kind] {
			return nil, invalid(fmt.Sprintf("field %q: no such kind %q", w.Title, w.Kind))
		}
		opts := []string{}
		if w.Kind == FieldSelect || w.Kind == FieldMulti {
			seen := map[string]bool{}
			for _, o := range w.Options {
				o = strings.TrimSpace(o)
				if o == "" || seen[o] {
					continue
				}
				seen[o] = true
				opts = append(opts, o)
			}
		}
		w.Options = opts
		if w.ID != 0 {
			if fieldByID(cur, w.ID) == nil {
				return nil, invalid(fmt.Sprintf("field %d is not this project's", w.ID))
			}
			if keep[w.ID] {
				return nil, invalid(fmt.Sprintf("field %q is listed twice", w.Title))
			}
			keep[w.ID] = true
		}
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
		opts, _ := json.Marshal(w.Options)
		if w.ID == 0 {
			res, err := tx.Exec(`INSERT INTO fields (project_id, title, kind, options, position, on_card, in_prompt) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				projectID, w.Title, w.Kind, string(opts), w.Position, w.OnCard, w.InPrompt)
			if err != nil {
				return nil, err
			}
			w.ID, _ = res.LastInsertId()
			continue
		}
		was := fieldByID(cur, w.ID)
		if _, err := tx.Exec(`UPDATE fields SET title=?, kind=?, options=?, position=?, on_card=?, in_prompt=? WHERE id=?`,
			w.Title, w.Kind, string(opts), w.Position, w.OnCard, w.InPrompt, w.ID); err != nil {
			return nil, err
		}
		switch {
		case was.Kind != w.Kind:
			if _, err := tx.Exec(`DELETE FROM task_fields WHERE field_id = ?`, w.ID); err != nil {
				return nil, err
			}
		case w.Kind == FieldSelect || w.Kind == FieldMulti:
			if err := pruneOptions(tx, *w); err != nil {
				return nil, err
			}
		}
	}
	for _, c := range cur {
		if !keep[c.ID] {
			if _, err := tx.Exec(`DELETE FROM fields WHERE id = ?`, c.ID); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return st.fieldsOf(projectID)
}

// pruneOptions drops the values of a select or multi field that its options no longer hold.
func pruneOptions(tx *sql.Tx, f Field) error {
	rows, err := tx.Query(`SELECT task_id, value FROM task_fields WHERE field_id = ?`, f.ID)
	if err != nil {
		return err
	}
	type fix struct {
		task  int64
		value string
	}
	var fixes []fix
	for rows.Next() {
		var task int64
		var raw string
		if err := rows.Scan(&task, &raw); err != nil {
			rows.Close()
			return err
		}
		stored, ok := fit(f, raw)
		if !ok {
			fixes = append(fixes, fix{task, ""})
		} else if stored != raw {
			fixes = append(fixes, fix{task, stored})
		}
	}
	rows.Close()
	for _, x := range fixes {
		if x.value == "" {
			_, err = tx.Exec(`DELETE FROM task_fields WHERE task_id = ? AND field_id = ?`, x.task, f.ID)
		} else {
			_, err = tx.Exec(`UPDATE task_fields SET value = ? WHERE task_id = ? AND field_id = ?`, x.value, x.task, f.ID)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// fit is a stored value read again against its field as it is now: the same, cut down to the options left, or nothing.
func fit(f Field, raw string) (string, bool) {
	switch f.Kind {
	case FieldSelect:
		var s string
		if json.Unmarshal([]byte(raw), &s) != nil || !has(f.Options, s) {
			return "", false
		}
		return raw, true
	case FieldMulti:
		var list []string
		_ = json.Unmarshal([]byte(raw), &list)
		var keep []string
		for _, s := range list {
			if has(f.Options, s) {
				keep = append(keep, s)
			}
		}
		if len(keep) == 0 {
			return "", false
		}
		b, _ := json.Marshal(keep)
		return string(b), true
	}
	stored, empty, err := normFieldValue(f, json.RawMessage(raw))
	if err != nil || empty {
		return "", false
	}
	return stored, true
}

// normFieldValue checks a value against its field's kind and returns it as stored (JSON), or empty when there is nothing to keep.
func normFieldValue(f Field, raw json.RawMessage) (stored string, empty bool, err error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", true, nil
	}
	bad := func(want string) error { return invalid(fmt.Sprintf("field %q wants %s", f.Title, want)) }
	keep := func(v any) (string, bool, error) {
		b, err := json.Marshal(v)
		return string(b), false, err
	}
	switch f.Kind {
	case FieldText, FieldLink, FieldDate, FieldSelect:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", false, bad("text")
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return "", true, nil
		}
		switch f.Kind {
		case FieldLink:
			if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
				return "", false, bad("an http(s) link")
			}
		case FieldDate:
			if s, err = normDate(s); err != nil {
				return "", false, err
			}
		case FieldSelect:
			if !has(f.Options, s) {
				return "", false, bad("one of its options")
			}
		}
		return keep(s)
	case FieldNumber:
		var n float64
		if err := json.Unmarshal(raw, &n); err != nil {
			var s string
			if json.Unmarshal(raw, &s) != nil {
				return "", false, bad("a number")
			}
			if s = strings.TrimSpace(s); s == "" {
				return "", true, nil
			}
			if n, err = strconv.ParseFloat(s, 64); err != nil {
				return "", false, bad("a number")
			}
		}
		return keep(n)
	case FieldMulti:
		var list []string
		if err := json.Unmarshal(raw, &list); err != nil {
			var s string
			if json.Unmarshal(raw, &s) != nil {
				return "", false, bad("a list of its options")
			}
			list = []string{s}
		}
		var out []string
		for _, s := range list {
			s = strings.TrimSpace(s)
			if s == "" || has(out, s) {
				continue
			}
			if !has(f.Options, s) {
				return "", false, bad("its options only")
			}
			out = append(out, s)
		}
		if len(out) == 0 {
			return "", true, nil
		}
		return keep(out)
	case FieldCheck:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return "", false, bad("true or false")
		}
		if !b {
			return "", true, nil
		}
		return keep(true)
	}
	return "", false, invalid(fmt.Sprintf("field %q has no kind", f.Title))
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// applyFields writes a task's values from a patch, each key a field id of the task's project; null or nothing worth keeping removes the value.
func applyFields(tx *sql.Tx, taskID int64, fields []Field, patch map[string]json.RawMessage) error {
	for key, raw := range patch {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			return invalid(fmt.Sprintf("field %q: not an id", key))
		}
		f := fieldByID(fields, id)
		if f == nil {
			return invalid(fmt.Sprintf("field %d is not this project's", id))
		}
		stored, empty, err := normFieldValue(*f, raw)
		if err != nil {
			return err
		}
		if empty {
			_, err = tx.Exec(`DELETE FROM task_fields WHERE task_id = ? AND field_id = ?`, taskID, id)
		} else {
			_, err = tx.Exec(`INSERT OR REPLACE INTO task_fields (task_id, field_id, value) VALUES (?, ?, ?)`, taskID, id, stored)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// carryFields moves a task's values to another project's fields of the same name and kind, when they fit the options there; the rest are dropped.
func carryFields(tx *sql.Tx, taskID int64, from, to []Field) error {
	rows, err := tx.Query(`SELECT field_id, value FROM task_fields WHERE task_id = ?`, taskID)
	if err != nil {
		return err
	}
	type kept struct {
		field int64
		value string
	}
	var carry []kept
	for rows.Next() {
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		src := fieldByID(from, id)
		if src == nil {
			continue
		}
		for i := range to {
			dst := &to[i]
			if dst.Kind != src.Kind || !strings.EqualFold(dst.Title, src.Title) {
				continue
			}
			if stored, ok := fit(*dst, raw); ok {
				carry = append(carry, kept{dst.ID, stored})
			}
			break
		}
	}
	rows.Close()
	if _, err := tx.Exec(`DELETE FROM task_fields WHERE task_id = ?`, taskID); err != nil {
		return err
	}
	for _, k := range carry {
		if _, err := tx.Exec(`INSERT INTO task_fields (task_id, field_id, value) VALUES (?, ?, ?)`, taskID, k.field, k.value); err != nil {
			return err
		}
	}
	return nil
}

// fillFields attaches each task's values, keyed by field id.
func (st *Store) fillFields(tasks []*Task) error {
	if len(tasks) == 0 {
		return nil
	}
	byID, ids := index(tasks)
	rows, err := st.db.Query(`SELECT task_id, field_id, value FROM task_fields WHERE task_id IN (` + ids + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var tid, fid int64
		var raw string
		if err := rows.Scan(&tid, &fid, &raw); err != nil {
			return err
		}
		var v any
		if json.Unmarshal([]byte(raw), &v) != nil {
			continue
		}
		byID[tid].Fields[strconv.FormatInt(fid, 10)] = v
	}
	return rows.Err()
}

// ValueText is a field's value as words: for the prompt, the card, a chip.
func ValueText(f Field, v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		if f.Kind == FieldDate && len(x) >= 10 {
			return x[:10]
		}
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "yes"
		}
		return ""
	case []any:
		var parts []string
		for _, e := range x {
			if s, ok := e.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	case []string:
		return strings.Join(x, ", ")
	}
	return fmt.Sprint(v)
}
