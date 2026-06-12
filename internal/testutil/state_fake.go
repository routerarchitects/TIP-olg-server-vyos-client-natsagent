package testutil

import (
	"context"
	"sync"

	"github.com/routerarchitects/olg-server-vyos-client-natagent/internal/state"
)

// FakeStateStore is a controllable in-memory state store test double.
type FakeStateStore struct {
	Current state.State
	LoadErr error
	SaveErr error
	Events  *EventRecorder

	mu          sync.Mutex
	loadCalls   int
	saveCalls   int
	savedStates []state.State
}

func (f *FakeStateStore) Load(ctx context.Context) (state.State, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.loadCalls++
	if f.LoadErr != nil {
		return state.State{}, f.LoadErr
	}
	return f.Current, nil
}

func (f *FakeStateStore) Save(ctx context.Context, st state.State) error {
	if f.Events != nil {
		f.Events.Record("state_save")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.saveCalls++
	if f.SaveErr != nil {
		return f.SaveErr
	}
	f.savedStates = append(f.savedStates, st)
	f.Current = st
	return nil
}

func (f *FakeStateStore) LoadCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.loadCalls
}

func (f *FakeStateStore) SaveCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.saveCalls
}

func (f *FakeStateStore) SavedStates() []state.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]state.State(nil), f.savedStates...)
}

func (f *FakeStateStore) LastSavedState() (state.State, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.savedStates) == 0 {
		return state.State{}, false
	}
	return f.savedStates[len(f.savedStates)-1], true
}

func (f *FakeStateStore) CurrentState() state.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Current
}

func (f *FakeStateStore) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.loadCalls = 0
	f.saveCalls = 0
	f.savedStates = nil
}
