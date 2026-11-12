package collisions

import (
	"log"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/state"
)

// Every is how often the radar reads the works again: a minute, one script per machine, and only the folders of the
// repositories two or more active workspaces share.
const Every = time.Minute

// scriptTimeout bounds one machine's reading; a machine that does not answer in time keeps its last collisions out.
const scriptTimeout = 60 * time.Second

const episodesFile = "collisions.json"

// Inputs are what the radar reads and tells: the workspaces of every connected machine, a machine's shell, the roster
// to wake, and the push service with its mutes.
type Inputs struct {
	Workspaces func() []Workspace
	Host       func(machineID string) (shell.Host, bool)
	Changed    func()
	Notify     func(push.Payload)
	Muted      func(machineID, paneID string) bool
}

// Episode is two works whose lines touch, by their machine|top, kept so a restart pushes no overlap twice.
type Episode struct {
	A string `json:"a"`
	B string `json:"b"`
}

func (e Episode) key() string { return e.A + " ~ " + e.B }

// Service is the collision radar: the last reading's collisions by folder, and the overlaps of the same lines pushed.
type Service struct {
	in  Inputs
	dir string

	mu       sync.Mutex
	byFolder map[string][]Collision // machine|dir
	episodes map[string]Episode
	busy     bool
}

// New opens the radar, the episodes already told read back from dir.
func New(dir string, in Inputs) *Service {
	s := &Service{in: in, dir: dir, byFolder: map[string][]Collision{}, episodes: map[string]Episode{}}
	var kept []Episode
	if err := state.LoadJSON(dir, episodesFile, &kept); err != nil {
		log.Printf("%s is unreadable (%v): every overlap counts as new", episodesFile, err)
	}
	for _, e := range kept {
		s.episodes[e.key()] = e
	}
	return s
}

// For is what the folder's work collides with, the overlaps of the same lines first.
func (s *Service) For(machineID, dir string) []Collision {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.byFolder[machineID+"|"+dir]
}

// Loop reads the works every Every until done closes.
func (s *Service) Loop(done <-chan struct{}) {
	ticker := time.NewTicker(Every)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			s.Sweep()
		}
	}
}

// member is one work of a repository and the workspaces standing on it.
type member struct {
	work       Work
	workspaces []Workspace
}

// Sweep reads every work worth reading, one script per machine side by side, compares the works of each repository,
// keeps the collisions and pushes each new overlap of the same lines; never two at once.
func (s *Service) Sweep() {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return
	}
	s.busy = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
	}()
	groups := Groups(s.in.Workspaces())
	works, read := s.read(groups)
	byFolder := map[string][]Collision{}
	touching := map[string]Episode{}
	notices := map[string]push.Payload{}
	for _, group := range groups {
		members := membersOf(group, works)
		for i := range members {
			for j := i + 1; j < len(members); j++ {
				a, b := members[i], members[j]
				files := Compare(a.work, b.work)
				if len(files) == 0 {
					continue
				}
				sideA, sideB := sides(a.work, b.work, peerOf(a.workspaces[0]), peerOf(b.workspaces[0]), standing(a), standing(b), files)
				for _, ws := range a.workspaces {
					byFolder[ws.Machine+"|"+ws.Dir] = append(byFolder[ws.Machine+"|"+ws.Dir], sideA)
				}
				for _, ws := range b.workspaces {
					byFolder[ws.Machine+"|"+ws.Dir] = append(byFolder[ws.Machine+"|"+ws.Dir], sideB)
				}
				if sideA.Lines {
					episode := episodeOf(a.work, b.work)
					touching[episode.key()] = episode
					notices[episode.key()] = s.notice(a, b, sideA)
				}
			}
		}
	}
	for _, list := range byFolder {
		sort.SliceStable(list, func(i, j int) bool { return list[i].Lines && !list[j].Lines })
	}
	s.settle(byFolder, touching, notices, read)
}

// read runs Script on every machine that has folders in the groups, side by side, and answers the works by
// machine|dir and which machines answered.
func (s *Service) read(groups [][]Workspace) (map[string]Work, map[string]bool) {
	dirs := map[string][]string{}
	seen := map[string]bool{}
	for _, group := range groups {
		for _, ws := range group {
			if folder := ws.Machine + "|" + ws.Dir; !seen[folder] {
				seen[folder] = true
				dirs[ws.Machine] = append(dirs[ws.Machine], ws.Dir)
			}
		}
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	works, read := map[string]Work{}, map[string]bool{}
	for machineID, list := range dirs {
		host, ok := s.in.Host(machineID)
		if !ok {
			continue
		}
		wg.Add(1)
		go func(machineID string, list []string) {
			defer wg.Done()
			output, err := shell.Text(host, Script(list), scriptTimeout)
			if err != nil {
				log.Printf("collisions: %s could not be read: %v", machineID, err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			read[machineID] = true
			for _, work := range Parse(machineID, output) {
				works[machineID+"|"+work.Dir] = work
			}
		}(machineID, list)
	}
	wg.Wait()
	return works, read
}

// membersOf is a repository's works, one per checkout — two folders inside one checkout are one work — each with
// the workspaces standing on it.
func membersOf(group []Workspace, works map[string]Work) []member {
	var members []member
	index := map[string]int{}
	for _, ws := range group {
		work, ok := works[ws.Machine+"|"+ws.Dir]
		if !ok {
			continue
		}
		checkout := ws.Machine + "|" + work.Top
		if at, ok := index[checkout]; ok {
			members[at].workspaces = append(members[at].workspaces, ws)
			continue
		}
		index[checkout] = len(members)
		members = append(members, member{work: work, workspaces: []Workspace{ws}})
	}
	return members
}

func peerOf(ws Workspace) Peer {
	return Peer{Machine: ws.Machine, Workspace: ws.ID, Name: ws.Label, Dir: ws.Dir, Branch: ws.Branch}
}

// standing is what orders a work: any of its workspaces with a pull request open, any agent at work, its size, the
// first workspace's name.
func standing(m member) Standing {
	out := Standing{Lines: m.work.Lines, Name: m.workspaces[0].Label}
	for _, ws := range m.workspaces {
		out.PR = out.PR || ws.PR
		out.Working = out.Working || ws.Working
	}
	return out
}

func episodeOf(a, b Work) Episode {
	first, second := a.Machine+"|"+a.Top, b.Machine+"|"+b.Top
	if second < first {
		first, second = second, first
	}
	return Episode{A: first, B: second}
}

// notice is the push for a new overlap of the same lines, told to the workspace that lands second — the one that will
// rebase — unless either folder is muted.
func (s *Service) notice(a, b member, sideA Collision) push.Payload {
	first, second := a, b
	if !sideA.First {
		first, second = b, a
	}
	if s.in.Muted != nil && (s.in.Muted(a.workspaces[0].Machine, a.workspaces[0].Pane) || s.in.Muted(b.workspaces[0].Machine, b.workspaces[0].Pane)) {
		return push.Payload{}
	}
	title, body := Words(first.workspaces[0].Label, second.workspaces[0].Label, sideA)
	return push.Payload{
		Machine: second.workspaces[0].Machine, Pane: second.workspaces[0].Pane, Kind: "collision",
		Tag:   "collision|" + episodeOf(a.work, b.work).key(),
		Title: title, Body: body,
	}
}

// settle keeps a sweep's collisions, pushes the overlaps not told yet and ends the episodes whose two works were both
// read and no longer touch — one on a machine that did not answer stays, so its return is no news.
func (s *Service) settle(byFolder map[string][]Collision, touching map[string]Episode, notices map[string]push.Payload, read map[string]bool) {
	s.mu.Lock()
	moved := !reflect.DeepEqual(s.byFolder, byFolder)
	s.byFolder = byFolder
	var tell []push.Payload
	kept := len(s.episodes)
	for key, episode := range touching {
		if _, told := s.episodes[key]; !told {
			s.episodes[key] = episode
			if notice := notices[key]; notice.Title != "" {
				tell = append(tell, notice)
			}
		}
	}
	for key, episode := range s.episodes {
		if _, still := touching[key]; !still && read[machineOf(episode.A)] && read[machineOf(episode.B)] {
			delete(s.episodes, key)
		}
	}
	changed := kept != len(s.episodes) || len(tell) > 0
	var list []Episode
	if changed {
		for _, episode := range s.episodes {
			list = append(list, episode)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].key() < list[j].key() })
	}
	s.mu.Unlock()
	if changed {
		if err := state.SaveJSON(s.dir, episodesFile, list); err != nil {
			log.Printf("collisions: %s could not be kept: %v", episodesFile, err)
		}
	}
	for _, notice := range tell {
		if s.in.Notify != nil {
			s.in.Notify(notice)
		}
	}
	if moved && s.in.Changed != nil {
		s.in.Changed()
	}
}

func machineOf(checkout string) string {
	machineID, _, _ := strings.Cut(checkout, "|")
	return machineID
}

// Forget drops a removed machine's collisions; the next sweep reads the rest again.
func (s *Service) Forget(machineID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for folder := range s.byFolder {
		if machineOf(folder) == machineID {
			delete(s.byFolder, folder)
		}
	}
}
