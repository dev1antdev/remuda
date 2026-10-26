package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/mcp"
	"github.com/dev1antdev/remuda/internal/tasksapi"
)

const tasksUsage = `usage: remuda tasks list [-view open|today|upcoming] [-project P] [-q words] [-done] [-page N] [-json]
       remuda tasks show <id> [-json]
       remuda tasks add -project P -title T [-md file|-] [-description text] [-column C] [-due YYYY-MM-DD] [-priority 0-5] [-label L]... [-json]
       remuda tasks move <id> [-column C] [-done] [-json]
       remuda tasks comment <id> <text>
The list answers from the remuda this machine's door names (REMUDA_AGENT_URL / REMUDA_AGENT_KEY, else the state dir's agent-url and agent-key).
`

// labels is a repeatable -label flag.
type labels []string

func (l *labels) String() string     { return strings.Join(*l, ",") }
func (l *labels) Set(s string) error { *l = append(*l, s); return nil }

// tasksCmd is `remuda tasks list|show|add|move|comment`: the task list from a shell, through the door `remuda mcp` uses.
func tasksCmd(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "help" {
		fmt.Fprint(os.Stderr, tasksUsage)
		os.Exit(2)
	}
	door, err := tasksapi.FindDoor()
	if err != nil {
		fmt.Fprintln(os.Stderr, "tasks:", err)
		os.Exit(1)
	}
	client := tasksapi.New(door)
	tools := &mcp.Tools{Client: client, Source: os.Getenv("HERDR_PANE_ID")}
	var text string
	var answer any
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("tasks list", flag.ExitOnError)
		a := mcp.ListArgs{}
		fs.StringVar(&a.View, "view", "open", "open | today | upcoming")
		fs.StringVar(&a.Project, "project", "", "a project's name or id")
		fs.StringVar(&a.Q, "q", "", "words to search titles and descriptions for")
		fs.BoolVar(&a.Done, "done", false, "the finished tasks too (with -project)")
		fs.IntVar(&a.Page, "page", 0, "the page, from 1")
		asJSON := fs.Bool("json", false, "the list's answer as JSON")
		fs.Parse(args[1:])
		if *asJSON {
			q, _, qerr := tools.Query(a)
			if qerr != nil {
				err = qerr
				break
			}
			var list []tasksapi.Task
			var pages int
			if list, pages, err = client.List(q); err == nil {
				answer = map[string]any{"tasks": list, "pages": pages}
			}
		} else {
			text, err = tools.List(a)
		}
	case "show":
		id, rest := idArg(args[1:], "show")
		fs := flag.NewFlagSet("tasks show", flag.ExitOnError)
		asJSON := fs.Bool("json", false, "the task as JSON")
		fs.Parse(rest)
		if *asJSON {
			answer, err = client.Get(id)
		} else {
			text, err = tools.Get(id)
		}
	case "add":
		fs := flag.NewFlagSet("tasks add", flag.ExitOnError)
		a := mcp.CreateArgs{}
		var priority int
		var labelList labels
		md := fs.String("md", "", "a Markdown file for the description, or - for stdin")
		fs.StringVar(&a.Project, "project", "", "the project's name or id")
		fs.StringVar(&a.Title, "title", "", "the task's title")
		fs.StringVar(&a.Description, "description", "", "the description, Markdown")
		fs.StringVar(&a.Column, "column", "", "the column to put it in")
		fs.StringVar(&a.Due, "due", "", "YYYY-MM-DD or RFC 3339")
		fs.IntVar(&priority, "priority", -1, "0 none … 5 do now")
		fs.Var(&labelList, "label", "a label by name (repeatable)")
		asJSON := fs.Bool("json", false, "the task as JSON")
		fs.Parse(args[1:])
		if *md != "" {
			a.Description = readText(*md)
		}
		if priority >= 0 {
			a.Priority = &priority
		}
		a.Labels = labelList
		task, notes, cerr := tools.Create(a)
		if cerr != nil {
			err = cerr
			break
		}
		if *asJSON {
			answer = task
		} else {
			text = mcp.Created(task, notes)
		}
	case "move":
		id, rest := idArg(args[1:], "move")
		fs := flag.NewFlagSet("tasks move", flag.ExitOnError)
		a := mcp.UpdateArgs{ID: id}
		fs.StringVar(&a.Column, "column", "", "the column to move it to")
		done := fs.Bool("done", false, "mark it done")
		asJSON := fs.Bool("json", false, "the task as JSON")
		fs.Parse(rest)
		if *done {
			a.Done = done
		}
		if a.Column == "" && !*done {
			err = fmt.Errorf("move needs -column or -done")
			break
		}
		task, uerr := tools.Update(a)
		if uerr != nil {
			err = uerr
			break
		}
		if *asJSON {
			answer = task
		} else {
			text, err = tools.Get(task.ID)
		}
	case "comment":
		id, rest := idArg(args[1:], "comment")
		text, err = tools.Comment(id, strings.Join(rest, " "))
	default:
		fmt.Fprint(os.Stderr, tasksUsage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tasks:", err)
		os.Exit(1)
	}
	if answer != nil {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(answer)
		return
	}
	fmt.Print(strings.TrimRight(text, "\n") + "\n")
}

// idArg is the task id a subcommand starts with, and the arguments after it.
func idArg(args []string, what string) (int64, []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "remuda tasks %s needs a task id\n", what)
		os.Exit(2)
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(args[0], "#"), 10, 64)
	if err != nil || id <= 0 {
		fmt.Fprintf(os.Stderr, "not a task id: %s\n", args[0])
		os.Exit(2)
	}
	return id, args[1:]
}

// readText is a file's text, or stdin's for "-".
func readText(path string) string {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tasks:", err)
		os.Exit(1)
	}
	return string(data)
}
