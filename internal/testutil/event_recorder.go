package testutil

import "sync"

// EventRecorder records named workflow events in order.
type EventRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *EventRecorder) Record(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, name)
}

func (r *EventRecorder) Events() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

func (r *EventRecorder) Contains(name string) bool {
	return r.Index(name) >= 0
}

func (r *EventRecorder) Index(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, event := range r.events {
		if event == name {
			return i
		}
	}
	return -1
}

func (r *EventRecorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = nil
}

// OrderRecorder is an alias for tests that use ordering language explicitly.
type OrderRecorder = EventRecorder
