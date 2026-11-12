package placement

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/state"
	"github.com/dev1antdev/remuda/internal/teleport"
)

// RemotesEvery is how often a machine's folders' remotes are read again while it is connected.
var RemotesEvery = 10 * time.Minute

const remotesFile = "remotes.json"

// Remotes knows the repository each workspace folder of every machine holds — its remote reduced as teleport.RemoteKey
// reduces it — read in one script per machine, kept in remotes.json, so a project whose own machine is away still
// knows its repository and finds the checkouts of it elsewhere.
type Remotes struct {
	dir string
	now func() time.Time

	mu         sync.Mutex
	known      map[string]string    // machine|folder → repository key, "" for none
	readAt     map[string]time.Time // machine → when its folders were last read
	complained bool                 // a write that failed is logged once
}

// OpenRemotes reads what was known in dir.
func OpenRemotes(dir string) *Remotes {
	r := &Remotes{dir: dir, now: time.Now, known: map[string]string{}, readAt: map[string]time.Time{}}
	if err := state.LoadJSON(dir, remotesFile, &r.known); err != nil || r.known == nil {
		r.known = map[string]string{}
	}
	return r
}

// Of is the repository a machine's folder holds as last read, "" when it is not known or has no remote.
func (r *Remotes) Of(machine, dir string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.known[machine+"|"+dir]
}

// Refresh reads the remotes of a machine's folders when the last read is RemotesEvery old or a folder was never
// read; a machine that does not answer keeps what was known.
func (r *Remotes) Refresh(host shell.Host, dirs []string) {
	r.mu.Lock()
	fresh := r.now().Sub(r.readAt[host.ID()]) < RemotesEvery
	missing := false
	for _, dir := range dirs {
		if _, ok := r.known[host.ID()+"|"+dir]; !ok {
			missing = true
		}
	}
	r.mu.Unlock()
	if len(dirs) == 0 || fresh && !missing {
		return
	}
	var sweep strings.Builder
	for _, dir := range dirs {
		sweep.WriteString(`( cd ` + script.Quote(dir) + ` 2>/dev/null && printf '%s\037%s\036' ` + script.Quote(dir) + ` "$(` + teleport.RemoteScript + `)" ); `)
	}
	sweep.WriteString("exit 0") // a folder gone answers nothing, not a failed read
	out, err := shell.Text(host, sweep.String(), 30*time.Second)
	if err != nil {
		return
	}
	read := map[string]string{}
	for _, record := range strings.Split(out, script.RS) {
		if fields := script.Fields(strings.TrimLeft(record, "\n")); len(fields) == 2 {
			read[fields[0]] = teleport.RemoteKey(fields[1])
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.readAt[host.ID()] = r.now()
	moved := false
	for _, dir := range dirs {
		key := host.ID() + "|" + dir
		if have, ok := r.known[key]; !ok || have != read[dir] {
			r.known[key] = read[dir]
			moved = true
		}
	}
	if moved && r.dir != "" {
		if err := state.SaveJSON(r.dir, remotesFile, r.known); err != nil && !r.complained {
			r.complained = true
			log.Printf("%s: %v", remotesFile, err)
		}
	}
}

// Forget drops a removed machine's folders.
func (r *Remotes) Forget(machine string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	gone := false
	for key := range r.known {
		if strings.HasPrefix(key, machine+"|") {
			delete(r.known, key)
			gone = true
		}
	}
	delete(r.readAt, machine)
	if gone && r.dir != "" {
		if err := state.SaveJSON(r.dir, remotesFile, r.known); err != nil {
			log.Printf("%s: %v", remotesFile, err)
		}
	}
}
