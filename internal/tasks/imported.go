package tasks

import (
	"fmt"
	"html"
	"strings"
)

// Imported is an issue of another tracker made into a task: its title, its address, what the first line says of it
// beside the address, its body as Markdown and its labels.
type Imported struct {
	Title    string
	URL      string
	Lead     string // "CRMOPS-6279 · Bug · by Ann", after the address
	Markdown string
	Labels   []string
}

// ImportFrom makes a task of an issue in a project: the title as it comes, a first paragraph naming the issue by its
// address (the agent reads it as text), then the body; labels made by name when missing. An issue already in the
// project is refused, found by its address as IssueHTML and ImportedHTML write it.
func ImportFrom(st *Store, projectID int64, in Imported) (Task, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.URL == "" || in.Title == "" {
		return Task{}, invalid("not an issue")
	}
	if id, ok, err := st.taskWithIssue(projectID, in.URL); err != nil {
		return Task{}, err
	} else if ok {
		return Task{}, invalid(fmt.Sprintf("this issue is already task #%d in the project", id))
	}
	description := ImportedHTML(in)
	task, err := st.Create(projectID, Patch{Title: &in.Title, Description: &description})
	if err != nil {
		return Task{}, err
	}
	for _, name := range in.Labels {
		if strings.TrimSpace(name) == "" {
			continue
		}
		label, err := st.CreateLabel(name, neutralLabelColor)
		if err != nil {
			return Task{}, err
		}
		if err := st.AddLabel(task.ID, label.ID); err != nil {
			return Task{}, err
		}
	}
	return st.Get(task.ID)
}

// ImportedHTML is an imported issue as a description: the line naming it, then its body drawn from Markdown.
func ImportedHTML(in Imported) string {
	address := html.EscapeString(in.URL)
	lead := `<p>Issue: <a href="` + address + `">` + address + `</a>`
	if in.Lead != "" {
		lead += " · " + html.EscapeString(in.Lead)
	}
	lead += "</p>"
	return lead + MarkdownHTML(in.Markdown)
}
