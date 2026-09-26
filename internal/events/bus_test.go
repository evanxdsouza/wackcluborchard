package events

import "testing"

func TestReplayAndFilter(t *testing.T) {
	b := New(4)
	b.Publish("app:a", "x", 1)
	b.Publish("app:b", "x", 2)
	b.Publish("app:a", "x", 3)
	_, replay := b.Subscribe(1, "app:a")
	if len(replay) != 1 || replay[0].Data != 3 {
		t.Fatalf("replay after id 1: %+v", replay)
	}
	sub, _ := b.Subscribe(0, "app:b")
	b.Publish("app:a", "x", 4)
	b.Publish("app:b", "x", 5)
	ev := <-sub.C
	if ev.Data != 5 {
		t.Fatalf("filter: got %+v", ev)
	}
	// ring overflow drops the oldest
	for i := 0; i < 10; i++ {
		b.Publish("app:c", "x", i)
	}
	_, replay = b.Subscribe(1, "app:")
	if len(replay) != 4 {
		t.Fatalf("ring holds 4, replayed %d", len(replay))
	}
}
