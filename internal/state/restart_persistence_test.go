package state

import (
	"context"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

/*
TC-RESTART-005
Type: Recovery
Title: Restart does not lose configured state when path persistent
Summary:
Saves state through one file store instance, then constructs a new store
instance with the same path to simulate process restart.

Validates:
  - persistent state path survives new store instance
  - applied UUID and target are preserved
*/
func TestRestartDoesNotLoseConfiguredStateWhenPathPersistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "state.json")
	first := NewFileStore(path)
	want := State{
		Target:      "vyos",
		AppliedUUID: "cfg-persistent",
		AppliedAt:   time.Date(2026, 6, 8, 16, 0, 0, 0, time.UTC),
	}

	if err := first.Save(context.Background(), want); err != nil {
		t.Fatalf("save: %v", err)
	}

	second := NewFileStore(path)
	got, err := second.Load(context.Background())
	if err != nil {
		t.Fatalf("load after restart: %v", err)
	}
	if got.Target != want.Target || got.AppliedUUID != want.AppliedUUID || !got.AppliedAt.Equal(want.AppliedAt) {
		t.Fatalf("loaded state got=%+v want=%+v", got, want)
	}
}

/*
TC-CONCURRENCY-004
Type: Concurrency
Title: Concurrent state access no race
Summary:
Runs bounded concurrent saves and loads against the file store. The test is
intended to run under go test -race and verifies operations do not panic or
produce unreadable final state.

Validates:
  - concurrent file-store access returns no errors
  - final state can be loaded
  - race detector remains green
*/
func TestConcurrentStateAccessNoRace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := NewFileStore(path)
	const workers = 8

	var wg sync.WaitGroup
	errs := make(chan error, workers*2)
	for i := 0; i < workers; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			errs <- store.Save(context.Background(), State{
				Target:      "vyos",
				AppliedUUID: "cfg-concurrent-" + strconv.Itoa(i),
				AppliedAt:   time.Date(2026, 6, 8, 16, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Second),
			})
		}(i)
		go func() {
			defer wg.Done()
			_, err := store.Load(context.Background())
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent state access error: %v", err)
		}
	}
	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("final load: %v", err)
	}
	if got.AppliedUUID == "" {
		t.Fatalf("final state missing applied UUID: %+v", got)
	}
}
