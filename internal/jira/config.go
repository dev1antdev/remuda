// Package jira follows the Jira issues the agents' work names: the person's site and token, kept in the state dir and
// never shown back; the issue keys in a workspace's branch and its agents' words, read from the site only while a page
// looks; and an issue made into a task. Nothing is contacted until the person configured a site.
package jira

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"

	"github.com/dev1antdev/remuda/internal/state"
)

const configFile = "jira.json"

// Config is how remuda reaches the person's Jira: the site's address, the account (the e-mail on Jira Cloud, the user
// name on a server, none with a personal access token) and the API token or personal access token.
type Config struct {
	Site    string `json:"site"`
	Account string `json:"account,omitempty"`
	Token   string `json:"token"`
}

// View is the configuration as the page sees it: the token only said to be there.
type View struct {
	Site     string `json:"site"`
	Account  string `json:"account"`
	HasToken bool   `json:"has_token"`
	Cloud    bool   `json:"cloud"`
}

// Store keeps the configuration in the state dir, 0600.
type Store struct {
	dir string
	mu  sync.Mutex
	cfg Config
}

// Load reads the configuration kept in dir; a damaged file reads as none, said once.
func Load(dir string) *Store {
	s := &Store{dir: dir}
	if err := state.LoadJSON(dir, configFile, &s.cfg); err != nil {
		log.Printf("%s is unreadable (%v): Jira stays off until it is set again", configFile, err)
		s.cfg = Config{}
	}
	return s
}

// Config is the configuration as kept.
func (s *Store) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Configured says whether a site and a token are there.
func (s *Store) Configured() bool {
	cfg := s.Config()
	return cfg.Site != "" && cfg.Token != ""
}

// View is the configuration with its token masked.
func (s *Store) View() View {
	cfg := s.Config()
	return View{Site: cfg.Site, Account: cfg.Account, HasToken: cfg.Token != "", Cloud: cfg.Site != "" && cloud(cfg.Site)}
}

// Replace sets the configuration: a token left empty keeps the one kept, so the page never holds it — only for the
// site it was given for, since another host, or plain http where it was https, must never be sent it.
func (s *Store) Replace(site, account, token string) error {
	site, err := cleanSite(site)
	if err != nil {
		return err
	}
	account = strings.TrimSpace(account)
	token = strings.TrimSpace(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	if token == "" && s.cfg.Token != "" {
		if !sameOrigin(s.cfg.Site, site) {
			return errors.New("type the token again for a new site: the one kept was given for " + hostOf(s.cfg.Site))
		}
		token = s.cfg.Token
	}
	if token == "" {
		return errors.New("an API token (or a personal access token) is needed")
	}
	if cloud(site) && account == "" {
		return errors.New("Jira Cloud signs in with the account's e-mail beside the API token")
	}
	next := Config{Site: site, Account: account, Token: token}
	if err := state.SaveJSON(s.dir, configFile, next); err != nil {
		return err
	}
	s.cfg = next
	return nil
}

// Clear forgets the configuration.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := state.SaveJSON(s.dir, configFile, Config{}); err != nil {
		return err
	}
	s.cfg = Config{}
	return nil
}

// cleanSite is the site's address as kept: http or https, a host, no query, no trailing slash; a path is kept for a
// server under one.
func cleanSite(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw != "" && !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("the site's address looks like https://yourcompany.atlassian.net")
	}
	if cloud(u.String()) && u.Scheme != "https" {
		return "", errors.New("Jira Cloud is reached over https")
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host+u.Path, "/"), nil
}

// sameOrigin says whether two sites are one scheme and one host, the place a token kept for the one may go.
func sameOrigin(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	return errA == nil && errB == nil && ua.Scheme == ub.Scheme && strings.EqualFold(ua.Host, ub.Host)
}

// hostOf is a site's host alone, what an error may name.
func hostOf(site string) string {
	if u, err := url.Parse(site); err == nil && u.Host != "" {
		return u.Host
	}
	return "the site before"
}

// cloud says whether the site is Atlassian's own, which speaks REST v3 and signs in with an e-mail and an API token.
func cloud(site string) bool {
	u, err := url.Parse(site)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return strings.HasSuffix(host, ".atlassian.net") || strings.HasSuffix(host, ".jira.com")
}
