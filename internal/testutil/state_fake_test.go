package testutil

import (
	"context"
	"errors"
	"testing"

	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/state"
)

/*
TC-TESTUTIL-STATE-001
Type: Negative
Title: Failed save does not record saved state
Summary:
Configures the fake state store to fail persistence.
The fake should expose the attempted save through call counts without
making SavedStates or LastSavedState look like persistence succeeded.

Validates:
  - SaveErr is returned
  - SaveCalls still increments
  - SavedStates remains empty
  - LastSavedState reports no saved state
  - Current state is preserved
*/
func TestFakeStateStoreSaveErrDoesNotRecordSavedState(t *testing.T) {
	initial := state.State{Target: MinimalTarget, AppliedUUID: "old"}
	store := &FakeStateStore{
		Current: initial,
		SaveErr: errors.New("save failed"),
	}

	err := store.Save(context.Background(), state.State{Target: MinimalTarget, AppliedUUID: "new"})
	if err == nil {
		t.Fatal("expected save error, got nil")
	}
	if store.SaveCalls() != 1 {
		t.Fatalf("save calls got=%d want=1", store.SaveCalls())
	}
	if got := store.SavedStates(); len(got) != 0 {
		t.Fatalf("saved states got=%d want=0", len(got))
	}
	if _, ok := store.LastSavedState(); ok {
		t.Fatal("last saved state reported true after failed save")
	}
	if store.Current != initial {
		t.Fatalf("current state mutated got=%+v want=%+v", store.Current, initial)
	}
}

/*
TC-TESTUTIL-STATE-002
Type: Positive
Title: Successful save records saved state
Summary:
Saves a new state through the fake state store with no configured
error. Successful persistence should update both the saved-state
history and the current loaded state.

Validates:
  - Save succeeds
  - SaveCalls increments
  - LastSavedState returns the saved state
  - Current state is updated
*/
func TestFakeStateStoreSuccessfulSaveRecordsSavedState(t *testing.T) {
	store := &FakeStateStore{}
	next := state.State{Target: MinimalTarget, AppliedUUID: "new"}

	if err := store.Save(context.Background(), next); err != nil {
		t.Fatalf("save: %v", err)
	}
	if store.SaveCalls() != 1 {
		t.Fatalf("save calls got=%d want=1", store.SaveCalls())
	}
	got, ok := store.LastSavedState()
	if !ok {
		t.Fatal("last saved state missing")
	}
	if got != next {
		t.Fatalf("last saved state got=%+v want=%+v", got, next)
	}
	if store.Current != next {
		t.Fatalf("current state got=%+v want=%+v", store.Current, next)
	}
}
