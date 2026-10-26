// Package tasksapi is remuda's task list as a program on a machine sees it: the tasks routes over HTTP with an agent
// key, and the door — the address and the key — that the remuda holding the list leaves in the state dir of every
// machine its agents run on. `remuda mcp` and `remuda tasks` speak through it.
package tasksapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/state"
	"github.com/dev1antdev/remuda/internal/tasks"
)

const (
	KeyFile = "agent-key" // the key, in the state dir of the machine the agent runs on
	URLFile = "agent-url" // where the list answers from that machine: the cockpit itself, or its door forwarded over ssh
	EnvKey  = "REMUDA_AGENT_KEY"
	EnvURL  = "REMUDA_AGENT_URL"
	// DefaultURL is the cockpit's own address, what a machine without an agent-url file reaches.
	DefaultURL = "http://127.0.0.1:7317"
)

// Door is where a command on this machine reaches the task list.
type Door struct {
	URL string
	Key string
}

// FindDoor reads the door from the environment, else from the state dir's files; without a key there is no door.
func FindDoor() (Door, error) {
	d := Door{URL: strings.TrimSpace(os.Getenv(EnvURL)), Key: strings.TrimSpace(os.Getenv(EnvKey))}
	if d.URL != "" && d.Key != "" {
		return d.clean(), nil
	}
	dir, err := state.Dir()
	if err != nil {
		return Door{}, err
	}
	if d.Key == "" {
		d.Key = fileLine(filepath.Join(dir, KeyFile))
	}
	if d.URL == "" {
		d.URL = fileLine(filepath.Join(dir, URLFile))
	}
	if d.Key == "" {
		return Door{}, fmt.Errorf("no door to remuda's tasks here (no %s): is remuda running on this machine, and is this machine known to the remuda that holds the list?", filepath.Join(dir, KeyFile))
	}
	return d.clean(), nil
}

func (d Door) clean() Door {
	if d.URL == "" {
		d.URL = DefaultURL
	}
	d.URL = strings.TrimRight(d.URL, "/")
	return d
}

func fileLine(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(b), "\n")
	return strings.TrimSpace(line)
}

// Error is the list's refusal: the status and what it said.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

// IsNotFound says whether the list had no such task, project or label.
func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

// Task is a task as the list answers it, with the source an older list does not carry yet.
type Task struct {
	tasks.Task
	Source string `json:"source,omitempty"`
}

// Write is what a task is made or changed with: a nil field stays, a Markdown description is rendered by the list
// (an older list keeps the plain description sent beside it), the source names the agent that wrote.
type Write struct {
	Title       *string                    `json:"title,omitempty"`
	Description *string                    `json:"description,omitempty"`
	Markdown    string                     `json:"markdown,omitempty"`
	Source      string                     `json:"source,omitempty"`
	Done        *bool                      `json:"done,omitempty"`
	DueDate     *string                    `json:"due_date,omitempty"`
	Priority    *int                       `json:"priority,omitempty"`
	StatusID    *int64                     `json:"status_id,omitempty"`
	Fields      map[string]json.RawMessage `json:"fields,omitempty"`
}

// Query is one page of the list.
type Query struct {
	View    string // today | upcoming | open | project
	Project int64
	Done    bool
	Search  string
	Page    int
	TZ      string
}

type Client struct {
	door Door
	http *http.Client
}

func New(door Door) *Client {
	return &Client{door: door.clean(), http: &http.Client{Timeout: 30 * time.Second}}
}

// URL is the list's address.
func (c *Client) URL() string { return c.door.URL }

func (c *Client) Projects() ([]tasks.Project, error) {
	var out []tasks.Project
	return out, c.do(http.MethodGet, "/api/tasks/projects", nil, &out)
}

func (c *Client) Labels() ([]tasks.Label, error) {
	var out []tasks.Label
	return out, c.do(http.MethodGet, "/api/tasks/labels", nil, &out)
}

func (c *Client) CreateLabel(title, hexColor string) (tasks.Label, error) {
	var out tasks.Label
	return out, c.do(http.MethodPost, "/api/tasks/labels", map[string]string{"title": title, "hex_color": hexColor}, &out)
}

// List answers one page of tasks and how many pages there are.
func (c *Client) List(q Query) ([]Task, int, error) {
	v := url.Values{}
	if q.View != "" {
		v.Set("view", q.View)
	}
	if q.Project > 0 {
		v.Set("project", strconv.FormatInt(q.Project, 10))
	}
	if q.Done {
		v.Set("done", "1")
	}
	if q.Search != "" {
		v.Set("q", q.Search)
	}
	if q.Page > 0 {
		v.Set("page", strconv.Itoa(q.Page))
	}
	if q.TZ != "" {
		v.Set("tz", q.TZ)
	}
	var out struct {
		Tasks []Task `json:"tasks"`
		Pages int    `json:"pages"`
	}
	err := c.do(http.MethodGet, "/api/tasks?"+v.Encode(), nil, &out)
	return out.Tasks, out.Pages, err
}

func (c *Client) Get(id int64) (Task, error) {
	var out Task
	return out, c.do(http.MethodGet, "/api/tasks/"+strconv.FormatInt(id, 10), nil, &out)
}

func (c *Client) Create(projectID int64, w Write) (Task, error) {
	body := struct {
		ProjectID int64 `json:"project_id"`
		Write
	}{projectID, w}
	var out Task
	return out, c.do(http.MethodPost, "/api/tasks", body, &out)
}

func (c *Client) Patch(id int64, w Write) (Task, error) {
	var out Task
	return out, c.do(http.MethodPatch, "/api/tasks/"+strconv.FormatInt(id, 10), w, &out)
}

func (c *Client) Comment(id int64, body string) (tasks.Comment, error) {
	var out tasks.Comment
	return out, c.do(http.MethodPost, "/api/tasks/"+strconv.FormatInt(id, 10)+"/comments", map[string]string{"body": body}, &out)
}

func (c *Client) AddLabel(taskID, labelID int64) (Task, error) {
	var out Task
	return out, c.do(http.MethodPut, "/api/tasks/"+strconv.FormatInt(taskID, 10)+"/labels", map[string]int64{"label_id": labelID}, &out)
}

// do is one request with the key; the list's JSON refusal becomes an *Error.
func (c *Client) do(method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.door.URL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.door.Key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("remuda at %s: %w", c.door.URL, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return &Error{Status: res.StatusCode, Message: refusal(res.StatusCode, data)}
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func refusal(status int, data []byte) string {
	var body struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &body) == nil && body.Error != "" {
		return body.Error
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		text = http.StatusText(status)
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return "remuda refused the key (" + text + "): the door files may be stale"
	}
	return text
}
