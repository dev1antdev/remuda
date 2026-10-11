package tasks

import (
	"encoding/json"
	"errors"
	"strconv"
	"testing"
)

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func TestFieldsRoundTrip(t *testing.T) {
	st := open(t)
	inbox, _ := st.Projects()
	id := inbox[0].ID
	fs, err := st.SetFields(id, []Field{
		{Title: " Area ", Kind: FieldSelect, Options: []string{"web", " api ", "", "web"}, OnCard: true},
		{Title: "Estimate", Kind: FieldNumber, InPrompt: true},
		{Title: "Tags", Kind: FieldMulti, Options: []string{"a", "b", "c"}},
		{Title: "Blocking", Kind: FieldCheck},
		{Title: "Spec", Kind: FieldLink},
		{Title: "Review by", Kind: FieldDate},
		{Title: "Notes", Kind: FieldText},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 7 || fs[0].Title != "Area" || len(fs[0].Options) != 2 || fs[0].Options[1] != "api" || fs[0].Position != 1 || fs[6].Position != 7 {
		t.Fatalf("fields %+v", fs)
	}
	p, _ := st.Project(id)
	if len(p.Fields) != 7 || !p.Fields[0].OnCard || !p.Fields[1].InPrompt {
		t.Fatalf("project fields %+v", p.Fields)
	}
	ids := map[string]string{}
	for _, f := range fs {
		ids[f.Title] = itoa(f.ID)
	}
	tk, err := st.Create(id, Patch{Title: str("x"), Fields: map[string]json.RawMessage{
		ids["Area"]:      raw(`"web"`),
		ids["Estimate"]:  raw(`"2.5"`),
		ids["Tags"]:      raw(`["b", "a", "b"]`),
		ids["Blocking"]:  raw(`true`),
		ids["Spec"]:      raw(`"https://x.test/spec"`),
		ids["Review by"]: raw(`"2026-10-12T15:00:00+03:00"`),
		ids["Notes"]:     raw(`"  "`),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if tk.Fields[ids["Area"]] != "web" || tk.Fields[ids["Estimate"]] != 2.5 || tk.Fields[ids["Blocking"]] != true || tk.Fields[ids["Spec"]] != "https://x.test/spec" || tk.Fields[ids["Review by"]] != "2026-10-12T12:00:00Z" {
		t.Fatalf("values %+v", tk.Fields)
	}
	if tags, ok := tk.Fields[ids["Tags"]].([]any); !ok || len(tags) != 2 || tags[0] != "b" || tags[1] != "a" {
		t.Fatalf("tags %+v", tk.Fields[ids["Tags"]])
	}
	if _, there := tk.Fields[ids["Notes"]]; there {
		t.Fatalf("an empty text was kept: %+v", tk.Fields)
	}
	// the list carries the values too
	list, _, _ := st.List(Query{View: "open"})
	if list[0].Fields[ids["Area"]] != "web" {
		t.Fatalf("list values %+v", list[0].Fields)
	}
	// refusals
	for what, patch := range map[string]map[string]json.RawMessage{
		"not an option":       {ids["Area"]: raw(`"mobile"`)},
		"a tag not an option": {ids["Tags"]: raw(`["a", "z"]`)},
		"not a number":        {ids["Estimate"]: raw(`"soon"`)},
		"not a link":          {ids["Spec"]: raw(`"ftp://x"`)},
		"not a date":          {ids["Review by"]: raw(`"tomorrow"`)},
		"not a bool":          {ids["Blocking"]: raw(`"yes"`)},
		"a foreign field":     {"999": raw(`"x"`)},
		"not an id":           {"area": raw(`"x"`)},
	} {
		if _, err := st.Update(tk.ID, Patch{Fields: patch}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: accepted (%v)", what, err)
		}
	}
	// a patch is partial; null and false clear
	tk, err = st.Update(tk.ID, Patch{Fields: map[string]json.RawMessage{ids["Area"]: raw(`null`), ids["Blocking"]: raw(`false`), ids["Notes"]: raw(`"hello"`)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, there := tk.Fields[ids["Area"]]; there {
		t.Fatalf("null did not clear: %+v", tk.Fields)
	}
	if _, there := tk.Fields[ids["Blocking"]]; there {
		t.Fatalf("false did not clear: %+v", tk.Fields)
	}
	if tk.Fields[ids["Notes"]] != "hello" || tk.Fields[ids["Estimate"]] != 2.5 {
		t.Fatalf("the rest did not stay: %+v", tk.Fields)
	}
}

func TestSetFieldsRules(t *testing.T) {
	st := open(t)
	inbox, _ := st.Projects()
	id := inbox[0].ID
	bad := func(what string, want []Field) {
		t.Helper()
		if _, err := st.SetFields(id, want); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: accepted (%v)", what, err)
		}
	}
	bad("nameless", []Field{{Title: " ", Kind: FieldText}})
	bad("strange kind", []Field{{Title: "A", Kind: "colour"}})
	bad("foreign id", []Field{{ID: 999, Title: "A", Kind: FieldText}})
	if _, err := st.SetFields(999, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing project: %v", err)
	}
	fs, _ := st.SetFields(id, []Field{{Title: "Area", Kind: FieldSelect, Options: []string{"web", "api"}}, {Title: "Tags", Kind: FieldMulti, Options: []string{"a", "b"}}, {Title: "Size", Kind: FieldNumber}})
	area, tags, size := itoa(fs[0].ID), itoa(fs[1].ID), itoa(fs[2].ID)
	tk, _ := st.Create(id, Patch{Title: str("x"), Fields: map[string]json.RawMessage{area: raw(`"api"`), tags: raw(`["a","b"]`), size: raw(`3`)}})
	// options shrink: a chosen one that went is dropped, the rest stay
	fs[0].Options = []string{"web"}
	fs[1].Options = []string{"b"}
	// a kind change empties the field
	fs[2].Kind = FieldText
	if _, err := st.SetFields(id, fs); err != nil {
		t.Fatal(err)
	}
	tk, _ = st.Get(tk.ID)
	if _, there := tk.Fields[area]; there {
		t.Fatalf("a value outside the options stayed: %+v", tk.Fields)
	}
	if got, _ := tk.Fields[tags].([]any); len(got) != 1 || got[0] != "b" {
		t.Fatalf("multi not pruned: %+v", tk.Fields[tags])
	}
	if _, there := tk.Fields[size]; there {
		t.Fatalf("a value survived its kind: %+v", tk.Fields)
	}
	// a field left out goes with its values
	if _, err := st.SetFields(id, fs[1:]); err != nil {
		t.Fatal(err)
	}
	p, _ := st.Project(id)
	if len(p.Fields) != 2 || p.Fields[0].Title != "Tags" {
		t.Fatalf("after the drop %+v", p.Fields)
	}
	var n int
	_ = st.db.QueryRow(`SELECT count(*) FROM task_fields WHERE field_id = ?`, fs[0].ID).Scan(&n)
	if n != 0 {
		t.Fatalf("values of a dropped field stayed: %d", n)
	}
}

func TestFieldsFollowATaskBetweenProjects(t *testing.T) {
	st := open(t)
	inbox, _ := st.Projects()
	id := inbox[0].ID
	fs, _ := st.SetFields(id, []Field{{Title: "Area", Kind: FieldSelect, Options: []string{"web", "api"}}, {Title: "Size", Kind: FieldNumber}, {Title: "Only here", Kind: FieldText}})
	tk, _ := st.Create(id, Patch{Title: str("x"), Fields: map[string]json.RawMessage{itoa(fs[0].ID): raw(`"api"`), itoa(fs[1].ID): raw(`5`), itoa(fs[2].ID): raw(`"gone"`)}})
	// a child copies the fields — its own ids
	child, _ := st.CreateProject("Child", id)
	if len(child.Fields) != 3 || child.Fields[0].ID == fs[0].ID || len(child.Fields[0].Options) != 2 {
		t.Fatalf("child fields %+v", child.Fields)
	}
	// another project with some of the same fields, one with narrower options
	other, _ := st.CreateProject("Other", 0)
	ofs, _ := st.SetFields(other.ID, []Field{{Title: "size", Kind: FieldNumber}, {Title: "Area", Kind: FieldSelect, Options: []string{"web"}}, {Title: "Only here", Kind: FieldNumber}})
	tk, err := st.Update(tk.ID, Patch{ProjectID: &other.ID})
	if err != nil {
		t.Fatal(err)
	}
	if tk.Fields[itoa(ofs[0].ID)] != 5.0 {
		t.Fatalf("size did not follow by name: %+v", tk.Fields)
	}
	if _, there := tk.Fields[itoa(ofs[1].ID)]; there {
		t.Fatalf("a value outside the options followed: %+v", tk.Fields)
	}
	if _, there := tk.Fields[itoa(ofs[2].ID)]; there {
		t.Fatalf("a value of another kind followed: %+v", tk.Fields)
	}
	if len(tk.Fields) != 1 {
		t.Fatalf("stray values: %+v", tk.Fields)
	}
}

func itoa(i int64) string { return strconv.FormatInt(i, 10) }
