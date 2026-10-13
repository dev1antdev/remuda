package herdr

import (
	"log"
	"sync"
)

// Keeper starts the local herdr server again whenever the watcher finds it gone, logging a change of fortune once.
type Keeper struct {
	Bin    string
	Socket string

	mu   sync.Mutex
	last string
}

// Revive is the answer to the local herdr not responding: EnsureServer, at most every two minutes.
func (k *Keeper) Revive(err error) {
	started := EnsureServer(k.Bin, k.Socket)
	msg := ""
	if started != nil {
		msg = started.Error()
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if msg == k.last {
		return
	}
	k.last = msg
	if started != nil {
		log.Printf("herdr server: %v", started)
	} else {
		log.Printf("herdr server: answering again")
	}
}
