package watch

import "time"

// The stream's say over the poll: an event asks for a snapshot after a moment for the rest of its burst, never closer
// to the last one than the gap; a title moving asks for one within the lazy gap. Vars so tests can shrink them.
var (
	eventDebounce = 150 * time.Millisecond
	localGap      = 500 * time.Millisecond
	remoteGap     = time.Second
	lazyGap       = 5 * time.Second
)

// every is the poll's pace: today's without herdr's event stream, the safety net's behind it.
func every(local, streaming bool) time.Duration {
	switch {
	case local && streaming:
		return localStreamingEvery
	case local:
		return localEvery
	case streaming:
		return remoteStreamingEvery
	}
	return remoteEvery
}

// wait holds the loop until the next poll is due: the pace's timer, an event of the stream, or a lazy one's gap.
func (w *Watcher) wait(st *machineState, local bool, done <-chan struct{}) {
	timer := time.NewTimer(every(local, st.streaming.Load()))
	defer timer.Stop()
	var lazy <-chan time.Time
	for {
		select {
		case <-done:
			return
		case <-timer.C:
			return
		case <-st.kick:
			gap := remoteGap
			if local {
				gap = localGap
			}
			time.Sleep(max(eventDebounce, gap-time.Since(st.polled)))
			drain(st.kick)
			drain(st.nudge)
			return
		case <-st.nudge:
			if lazy == nil {
				lazyTimer := time.NewTimer(max(0, lazyGap-time.Since(st.polled)))
				defer lazyTimer.Stop()
				lazy = lazyTimer.C
			}
		case <-lazy:
			drain(st.nudge)
			return
		}
	}
}

// signal sets a cap-1 channel without waiting: one pending word is as good as many.
func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func drain(ch chan struct{}) {
	select {
	case <-ch:
	default:
	}
}
