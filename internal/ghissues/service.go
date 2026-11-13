package ghissues

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// Host is a machine gh runs on: its shell, and a file put there whole (a comment's text, which never goes on a command line).
type Host interface {
	shell.Host
	PutFile(path string, data []byte, timeout time.Duration) error
}

// MaxLists and MaxDetails bound what is kept; the answer read longest ago goes first.
const (
	MaxLists   = 64
	MaxDetails = 128
)

type listEntry struct {
	list List
	etag string
}

type detailEntry struct {
	detail Detail
	etag   string
}

type loginEntry struct {
	login string
	at    time.Time
}

type originsEntry struct {
	byDir map[string]*checkout
	dirs  []string
	at    time.Time
}

// Service reads and writes the issues; its answers are kept by machine, repository and search or number.
type Service struct {
	Hosts func(machineID string) (Host, error)
	now   func() time.Time

	mu      sync.Mutex
	lists   map[string]*listEntry
	details map[string]*detailEntry
	logins  map[string]loginEntry
	origins map[string]originsEntry
	locks   map[string]*sync.Mutex
}

func New(hosts func(machineID string) (Host, error)) *Service {
	return &Service{Hosts: hosts, now: time.Now, lists: map[string]*listEntry{}, details: map[string]*detailEntry{}, logins: map[string]loginEntry{}, origins: map[string]originsEntry{}, locks: map[string]*sync.Mutex{}}
}

// lock is one read of a key at a time: a second asker waits and is served what the first read.
func (s *Service) lock(key string) func() {
	s.mu.Lock()
	l := s.locks[key]
	if l == nil {
		l = &sync.Mutex{}
		s.locks[key] = l
	}
	s.mu.Unlock()
	l.Lock()
	return l.Unlock
}

func listKey(machineID, repo, query string) string {
	return machineID + "|" + strings.ToLower(repo) + "|" + query
}

func detailKey(machineID, repo string, number int) string {
	return machineID + "|" + strings.ToLower(repo) + "#" + strconv.Itoa(number)
}

func (s *Service) login(machineID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.logins[machineID]
	if !ok || s.now().Sub(l.at) >= LoginFor {
		return "", false
	}
	return l.login, true
}

// noteLogin keeps a login gh printed: one word, an account name.
func (s *Service) noteLogin(machineID, section string) string {
	name := strings.TrimSpace(section)
	if name == "" || strings.ContainsAny(name, " \t\n") {
		return ""
	}
	s.mu.Lock()
	s.logins[machineID] = loginEntry{login: name, at: s.now()}
	s.mu.Unlock()
	return name
}

// List is the repository's issues the search finds. Kept under Fresh, it is served with no request; to ProbeFor a 304
// keeps it; past that, or when fresh is asked for, it is read whole.
func (s *Service) List(machineID, repo, query string, fresh bool) (List, error) {
	if !RepoOK(repo) || !QueryOK(query) {
		return List{}, ErrInvalid
	}
	key := listKey(machineID, repo, query)
	defer s.lock(key)()
	s.mu.Lock()
	kept := s.lists[key]
	s.mu.Unlock()
	now := s.now()
	if kept != nil && !fresh && now.Sub(kept.list.At) < Fresh {
		answer := kept.list
		answer.Source = FromCache
		return answer, nil
	}
	host, err := s.Hosts(machineID)
	if err != nil {
		return List{}, err
	}
	etag := ""
	if kept != nil && !fresh && now.Sub(kept.list.At) < ProbeFor {
		etag = kept.etag
	}
	me, known := s.login(machineID)
	out, runErr := shell.Text(host, listScript(repo, query, etag, !known), Timeout)
	sections := script.Sections(out)
	if len(sections) < 2 {
		return List{}, troubleOf(host.Label(), repo, firstText(out, runErr))
	}
	if !known {
		me = s.noteLogin(machineID, sections[0])
	}
	probed := parseResponse(sections[1])
	if probed.status == 304 && kept != nil {
		s.mu.Lock()
		kept.list.At = now
		kept.list.Me = me
		answer := kept.list
		s.mu.Unlock()
		answer.Source = FromProbe
		return answer, nil
	}
	if len(sections) < 3 {
		return List{}, troubleOf(host.Label(), repo, firstText(probed.text, runErr))
	}
	items, err := parseList(sections[2], query)
	if err != nil {
		return List{}, troubleOf(host.Label(), repo, err.Error())
	}
	list := List{Items: items, More: len(items) >= Limit, Me: me, At: now, Source: FromGitHub}
	s.mu.Lock()
	s.lists[key] = &listEntry{list: list, etag: probed.etag}
	evictLists(s.lists)
	s.mu.Unlock()
	return list, nil
}

// Issue is one issue whole, kept the way List keeps a list: the issue's own ETag says whether it moved.
func (s *Service) Issue(machineID, repo string, number int, fresh bool) (Detail, error) {
	if !RepoOK(repo) || number <= 0 {
		return Detail{}, ErrInvalid
	}
	key := detailKey(machineID, repo, number)
	defer s.lock(key)()
	s.mu.Lock()
	kept := s.details[key]
	s.mu.Unlock()
	now := s.now()
	if kept != nil && !fresh && now.Sub(kept.detail.At) < Fresh {
		answer := kept.detail
		answer.Source = FromCache
		return answer, nil
	}
	host, err := s.Hosts(machineID)
	if err != nil {
		return Detail{}, err
	}
	etag := ""
	if kept != nil && !fresh && now.Sub(kept.detail.At) < ProbeFor {
		etag = kept.etag
	}
	me, known := s.login(machineID)
	out, runErr := shell.Text(host, detailScript(repo, number, etag, !known), Timeout)
	sections := script.Sections(out)
	if len(sections) < 2 {
		return Detail{}, troubleOf(host.Label(), repo, firstText(out, runErr))
	}
	if !known {
		me = s.noteLogin(machineID, sections[0])
	}
	issue := parseResponse(sections[1])
	if issue.status == 304 && kept != nil {
		s.mu.Lock()
		kept.detail.At = now
		kept.detail.Me = me
		answer := kept.detail
		s.mu.Unlock()
		answer.Source = FromProbe
		return answer, nil
	}
	if issue.status != 200 || len(sections) < 4 {
		return Detail{}, troubleOf(host.Label(), repo, firstText(issue.text, runErr))
	}
	detail, err := detailOf(issue.body, sections[2], sections[3])
	if err != nil {
		return Detail{}, troubleOf(host.Label(), repo, err.Error())
	}
	detail.Me, detail.At, detail.Source = me, now, FromGitHub
	s.mu.Lock()
	s.details[key] = &detailEntry{detail: detail, etag: issue.etag}
	evictDetails(s.details)
	s.mu.Unlock()
	return detail, nil
}

// Repos is the machine's GitHub repositories among the folders, and what the folder asked about is; kept OriginsFor.
func (s *Service) Repos(machineID string, dirs []string, asked string) (Repos, error) {
	all := append([]string(nil), dirs...)
	if asked != "" && !contains(all, asked) {
		all = append(all, asked)
	}
	sort.Strings(all)
	host, err := s.Hosts(machineID)
	if err != nil {
		return Repos{}, err
	}
	style := rpath.For(host.Home())
	s.mu.Lock()
	kept, ok := s.origins[machineID]
	s.mu.Unlock()
	if !ok || s.now().Sub(kept.at) >= OriginsFor || !sameDirs(kept.dirs, all) {
		if len(all) == 0 {
			return Repos{Repos: []Repo{}}, nil
		}
		out, err := shell.Text(host, originsScript(all), Timeout)
		if err != nil && strings.TrimSpace(out) == "" {
			return Repos{}, fmt.Errorf("the folders on %s could not be read: %w", host.Label(), err)
		}
		kept = originsEntry{byDir: parseOrigins(out), dirs: all, at: s.now()}
		s.mu.Lock()
		s.origins[machineID] = kept
		s.mu.Unlock()
	}
	return reposOf(kept.byDir, all, style, asked), nil
}

// Forget drops what was read on a machine that left the fleet.
func (s *Service) Forget(machineID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := machineID + "|"
	for key := range s.lists {
		if strings.HasPrefix(key, prefix) {
			delete(s.lists, key)
		}
	}
	for key := range s.details {
		if strings.HasPrefix(key, prefix) {
			delete(s.details, key)
		}
	}
	delete(s.logins, machineID)
	delete(s.origins, machineID)
}

// moved marks the repository's lists and the issue as needing a read: the person just changed it.
func (s *Service) moved(machineID, repo string, number int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := machineID + "|" + strings.ToLower(repo) + "|"
	for key := range s.lists {
		if strings.HasPrefix(key, prefix) {
			delete(s.lists, key)
		}
	}
	delete(s.details, detailKey(machineID, repo, number))
}

func firstText(out string, err error) string {
	if strings.TrimSpace(out) == "" && err != nil {
		return err.Error()
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func sameDirs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func evictLists(m map[string]*listEntry) {
	for len(m) > MaxLists {
		oldest, at := "", time.Time{}
		for key, e := range m {
			if oldest == "" || e.list.At.Before(at) {
				oldest, at = key, e.list.At
			}
		}
		delete(m, oldest)
	}
}

func evictDetails(m map[string]*detailEntry) {
	for len(m) > MaxDetails {
		oldest, at := "", time.Time{}
		for key, e := range m {
			if oldest == "" || e.detail.At.Before(at) {
				oldest, at = key, e.detail.At
			}
		}
		delete(m, oldest)
	}
}
