package jira

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Timeout is how long one request to the site may take.
var Timeout = 10 * time.Second

// maxBody is the most of an answer read.
const maxBody = 4 << 20

var (
	// ErrNotFound: the site knows no such issue, or the account may not see it.
	ErrNotFound = errors.New("no such issue, or the account may not see it")
	// ErrRefused: the site refused the account or the token.
	ErrRefused = errors.New("Jira refused the account or the token")
)

// Issue is what a chip says of an issue: its key, summary, status and the status's category (new, indeterminate, done).
type Issue struct {
	Key      string `json:"key"`
	Summary  string `json:"summary"`
	Status   string `json:"status"`
	Category string `json:"category"`
	Assignee string `json:"assignee,omitempty"`
	URL      string `json:"url"`
}

// Full is an issue as a task is made of it: the chip's words, and its type, reporter, labels and description as Markdown.
type Full struct {
	Issue
	Type        string
	Reporter    string
	Labels      []string
	Description string // Markdown, from Cloud's document format or a server's wiki markup
}

// Client asks one site, as the person configured it.
type Client struct {
	cfg  Config
	http *http.Client
}

// NewClient is a client of the site; a nil http client gets one with Timeout. Either way it follows no redirect off the
// site's own host and port, nor from https down to http: the token rides on every request it makes.
func NewClient(cfg Config, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: Timeout}
	}
	onSite := *hc
	onSite.CheckRedirect = stayOnSite(cfg.Site)
	return &Client{cfg: cfg, http: &onSite}
}

// stayOnSite is the redirect rule: the site's own scheme, host and port, or an upgrade to https on its host.
func stayOnSite(site string) func(*http.Request, []*http.Request) error {
	base, err := url.Parse(site)
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if err != nil {
			return errors.New("the site's address is not usable")
		}
		dest := req.URL
		sameHost := strings.EqualFold(dest.Hostname(), base.Hostname())
		same := dest.Scheme == base.Scheme && sameHost && portOf(dest) == portOf(base)
		upgrade := base.Scheme == "http" && dest.Scheme == "https" && sameHost
		if !same && !upgrade {
			return errors.New("it sent the request off the site, where the token does not go")
		}
		return nil
	}
}

// portOf is the port an address reaches, its scheme's own when it names none.
func portOf(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

// api is the REST path of the site's version: 3 on Cloud, 2 on a server.
func (c *Client) api() string {
	if cloud(c.cfg.Site) {
		return "/rest/api/3"
	}
	return "/rest/api/2"
}

// IssueURL is where the person reads an issue.
func (c *Client) IssueURL(key string) string { return c.cfg.Site + "/browse/" + key }

// get reads one answer as JSON: the token goes as Basic with the account, else as Bearer (a personal access token); an
// error never names the token or the full address.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	address := c.cfg.Site + c.api() + path
	if len(query) > 0 {
		address += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return fmt.Errorf("the site's address is not usable")
	}
	if c.cfg.Account != "" {
		req.SetBasicAuth(c.cfg.Account, c.cfg.Token)
	} else {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Jira at %s could not be reached: %v", c.host(), plainError(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("Jira at %s: the answer broke off", c.host())
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w (%d)", ErrRefused, resp.StatusCode)
	case resp.StatusCode/100 != 2:
		return fmt.Errorf("Jira at %s answered %d", c.host(), resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("Jira at %s answered something that is not its JSON", c.host())
	}
	return nil
}

func (c *Client) host() string {
	u, err := url.Parse(c.cfg.Site)
	if err != nil {
		return "the site"
	}
	return u.Host
}

// plainError is a transport error without the address it names.
func plainError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

// Myself is the name the site knows the account by: the Test button's proof.
func (c *Client) Myself(ctx context.Context) (string, error) {
	var me struct {
		DisplayName  string `json:"displayName"`
		Name         string `json:"name"`
		EmailAddress string `json:"emailAddress"`
	}
	if err := c.get(ctx, "/myself", nil, &me); err != nil {
		return "", err
	}
	for _, name := range []string{me.DisplayName, me.Name, me.EmailAddress} {
		if name != "" {
			return name, nil
		}
	}
	return "the account", nil
}

// Projects is the keys of every project the account sees: Cloud's search a page at a time, a server's whole list.
func (c *Client) Projects(ctx context.Context) ([]string, error) {
	type project struct {
		Key string `json:"key"`
	}
	if !cloud(c.cfg.Site) {
		var list []project
		if err := c.get(ctx, "/project", nil, &list); err != nil {
			return nil, err
		}
		keys := make([]string, 0, len(list))
		for _, p := range list {
			keys = append(keys, p.Key)
		}
		return keys, nil
	}
	var keys []string
	for start := 0; start < 2000; {
		var page struct {
			Values []project `json:"values"`
			IsLast bool      `json:"isLast"`
		}
		query := url.Values{"startAt": {strconv.Itoa(start)}, "maxResults": {"100"}}
		if err := c.get(ctx, "/project/search", query, &page); err != nil {
			return nil, err
		}
		for _, p := range page.Values {
			keys = append(keys, p.Key)
		}
		if page.IsLast || len(page.Values) == 0 {
			break
		}
		start += len(page.Values)
	}
	return keys, nil
}

// issueFields is the part of an issue's answer remuda reads.
type issueFields struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
		Status  struct {
			Name     string `json:"name"`
			Category struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"status"`
		Assignee  *person               `json:"assignee"`
		Reporter  *person               `json:"reporter"`
		IssueType struct{ Name string } `json:"issuetype"`
		Labels    []string              `json:"labels"`
		Desc      json.RawMessage       `json:"description"`
	} `json:"fields"`
}

type person struct {
	DisplayName string `json:"displayName"`
}

func (p *person) name() string {
	if p == nil {
		return ""
	}
	return p.DisplayName
}

// Issue reads an issue's chip: its summary, status and assignee.
func (c *Client) Issue(ctx context.Context, key string) (Issue, error) {
	var raw issueFields
	if err := c.get(ctx, "/issue/"+url.PathEscape(key), url.Values{"fields": {"summary,status,assignee"}}, &raw); err != nil {
		return Issue{}, err
	}
	return c.issueOf(key, raw), nil
}

// Full reads an issue whole, its description made Markdown, for a task.
func (c *Client) Full(ctx context.Context, key string) (Full, error) {
	var raw issueFields
	query := url.Values{"fields": {"summary,status,assignee,reporter,issuetype,labels,description"}}
	if err := c.get(ctx, "/issue/"+url.PathEscape(key), query, &raw); err != nil {
		return Full{}, err
	}
	full := Full{Issue: c.issueOf(key, raw), Type: raw.Fields.IssueType.Name, Reporter: raw.Fields.Reporter.name(), Labels: raw.Fields.Labels}
	full.Description = descriptionMarkdown(raw.Fields.Desc)
	return full, nil
}

func (c *Client) issueOf(key string, raw issueFields) Issue {
	if raw.Key != "" {
		key = raw.Key
	}
	category := raw.Fields.Status.Category.Key
	if category == "" {
		category = "new"
	}
	return Issue{
		Key: key, Summary: strings.TrimSpace(raw.Fields.Summary), Status: raw.Fields.Status.Name, Category: category,
		Assignee: raw.Fields.Assignee.name(), URL: c.IssueURL(key),
	}
}
