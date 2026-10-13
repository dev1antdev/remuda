package events

import "testing"

func TestBusDeliversByTypeInOrder(t *testing.T) {
	bus := NewBus()
	var got []string
	Subscribe(bus, func(e PaneStatus) { got = append(got, "first:"+e.To) })
	Subscribe(bus, func(e PaneStatus) { got = append(got, "second:"+e.To) })
	Subscribe(bus, func(e JobDone) { got = append(got, "job:"+e.Title) })
	Publish(bus, PaneStatus{To: "done"})
	Publish(bus, JobDone{Title: "update"})
	Publish(bus, MachineRemoved{Machine: "x"})
	if len(got) != 3 || got[0] != "first:done" || got[1] != "second:done" || got[2] != "job:update" {
		t.Fatalf("got %v", got)
	}
	Publish[PaneStatus](nil, PaneStatus{})
	if (PaneRef{"m", "p"}).String() != "m|p" {
		t.Fatal("PaneRef")
	}
}
