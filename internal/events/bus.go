// Package events is the in-process event bus that fans state changes out to
// server-sent-event streams. Every event carries a monotonically increasing
// id and the bus keeps a ring of recent events, so a reconnecting client
// replays from the last id it saw rather than starting blind.
package events

import (
	"strings"
	"sync"
	"time"
)

type Event struct {
	ID    int64     `json:"id"`
	Topic string    `json:"topic"` // e.g. "project:prj_x", "app:app_x", "deploy:dep_x"
	Type  string    `json:"type"`  // e.g. "app.updated"
	Data  any       `json:"data,omitempty"`
	At    time.Time `json:"at"`
}

type Bus struct {
	mu     sync.Mutex
	seq    int64
	ring   []Event
	head   int
	filled bool
	subs   map[*Sub]struct{}
}

type Sub struct {
	C        chan Event
	prefixes []string
}

func (s *Sub) matches(topic string) bool {
	if len(s.prefixes) == 0 {
		return true
	}
	for _, p := range s.prefixes {
		if strings.HasPrefix(topic, p) {
			return true
		}
	}
	return false
}

func New(size int) *Bus {
	return &Bus{ring: make([]Event, size), subs: map[*Sub]struct{}{}}
}

// Publish records and fans out an event. Slow subscribers drop events
// rather than blocking publishers; clients reconcile on reconnect.
func (b *Bus) Publish(topic, typ string, data any) {
	b.mu.Lock()
	b.seq++
	ev := Event{ID: b.seq, Topic: topic, Type: typ, Data: data, At: time.Now()}
	b.ring[b.head] = ev
	b.head = (b.head + 1) % len(b.ring)
	if b.head == 0 {
		b.filled = true
	}
	subs := make([]*Sub, 0, len(b.subs))
	for s := range b.subs {
		if s.matches(topic) {
			subs = append(subs, s)
		}
	}
	b.mu.Unlock()
	for _, s := range subs {
		select {
		case s.C <- ev:
		default:
		}
	}
}

// Subscribe returns a subscription for topics beginning with any of the
// prefixes, plus any buffered events after lastID.
func (b *Bus) Subscribe(lastID int64, prefixes ...string) (*Sub, []Event) {
	s := &Sub{C: make(chan Event, 256), prefixes: prefixes}
	b.mu.Lock()
	defer b.mu.Unlock()
	var replay []Event
	if lastID > 0 {
		n := b.head
		if b.filled {
			n = len(b.ring)
		}
		start := 0
		if b.filled {
			start = b.head
		}
		for i := 0; i < n; i++ {
			ev := b.ring[(start+i)%len(b.ring)]
			if ev.ID > lastID && s.matches(ev.Topic) {
				replay = append(replay, ev)
			}
		}
	}
	b.subs[s] = struct{}{}
	return s, replay
}

func (b *Bus) Unsubscribe(s *Sub) {
	b.mu.Lock()
	delete(b.subs, s)
	b.mu.Unlock()
}

// LogHub fans out log lines per stream key (an app, deploy, run step...).
// It keeps a bounded tail per key so late joiners see recent output.
type LogHub struct {
	mu      sync.Mutex
	streams map[string]*logStream
}

type LogLine struct {
	Pod  string    `json:"pod,omitempty"`
	Text string    `json:"text"`
	At   time.Time `json:"at"`
	Err  bool      `json:"err,omitempty"`
}

type logStream struct {
	tail []LogLine
	subs map[chan LogLine]struct{}
}

func NewLogHub() *LogHub { return &LogHub{streams: map[string]*logStream{}} }

const tailSize = 2000

func (h *LogHub) stream(key string) *logStream {
	s := h.streams[key]
	if s == nil {
		s = &logStream{subs: map[chan LogLine]struct{}{}}
		h.streams[key] = s
	}
	return s
}

func (h *LogHub) Append(key string, line LogLine) {
	if line.At.IsZero() {
		line.At = time.Now()
	}
	h.mu.Lock()
	s := h.stream(key)
	s.tail = append(s.tail, line)
	if len(s.tail) > tailSize {
		s.tail = s.tail[len(s.tail)-tailSize:]
	}
	subs := make([]chan LogLine, 0, len(s.subs))
	for c := range s.subs {
		subs = append(subs, c)
	}
	h.mu.Unlock()
	for _, c := range subs {
		select {
		case c <- line:
		default:
		}
	}
}

func (h *LogHub) Tail(key string) []LogLine {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.streams[key]
	if s == nil {
		return nil
	}
	return append([]LogLine(nil), s.tail...)
}

func (h *LogHub) Text(key string) string {
	var b strings.Builder
	for _, l := range h.Tail(key) {
		b.WriteString(l.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

func (h *LogHub) Subscribe(key string) (chan LogLine, []LogLine) {
	c := make(chan LogLine, 512)
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.stream(key)
	s.subs[c] = struct{}{}
	return c, append([]LogLine(nil), s.tail...)
}

func (h *LogHub) Unsubscribe(key string, c chan LogLine) {
	h.mu.Lock()
	if s := h.streams[key]; s != nil {
		delete(s.subs, c)
	}
	h.mu.Unlock()
}

func (h *LogHub) Drop(key string) {
	h.mu.Lock()
	delete(h.streams, key)
	h.mu.Unlock()
}
