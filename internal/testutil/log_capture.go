package testutil

import (
	"fmt"
	"strings"
	"sync"
)

type LogEntry struct {
	Level     string
	Message   string
	KeyValues []any
}

func (e LogEntry) String() string {
	var b strings.Builder
	b.WriteString(e.Level)
	b.WriteString(" ")
	b.WriteString(e.Message)
	for _, kv := range e.KeyValues {
		b.WriteByte(' ')
		fmt.Fprint(&b, kv)
	}
	return b.String()
}

// LogCapture is an in-memory agentcore.Logger for log assertions.
type LogCapture struct {
	mu      sync.Mutex
	entries []LogEntry
}

func (l *LogCapture) Debug(msg string, kv ...any) { l.record("debug", msg, kv...) }
func (l *LogCapture) Info(msg string, kv ...any)  { l.record("info", msg, kv...) }
func (l *LogCapture) Warn(msg string, kv ...any)  { l.record("warn", msg, kv...) }
func (l *LogCapture) Error(msg string, kv ...any) { l.record("error", msg, kv...) }

func (l *LogCapture) record(level, msg string, kv ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries = append(l.entries, LogEntry{
		Level:     level,
		Message:   msg,
		KeyValues: append([]any(nil), kv...),
	})
}

func (l *LogCapture) Entries() []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]LogEntry, len(l.entries))
	for i, entry := range l.entries {
		out[i] = LogEntry{
			Level:     entry.Level,
			Message:   entry.Message,
			KeyValues: append([]any(nil), entry.KeyValues...),
		}
	}
	return out
}

func (l *LogCapture) EntriesByLevel(level string) []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	var out []LogEntry
	for _, entry := range l.entries {
		if entry.Level == level {
			out = append(out, LogEntry{
				Level:     entry.Level,
				Message:   entry.Message,
				KeyValues: append([]any(nil), entry.KeyValues...),
			})
		}
	}
	return out
}

func (l *LogCapture) Contains(text string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, entry := range l.entries {
		if strings.Contains(entry.String(), text) {
			return true
		}
	}
	return false
}

func (l *LogCapture) DoesNotContain(text string) bool {
	return !l.Contains(text)
}

func (l *LogCapture) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = nil
}
