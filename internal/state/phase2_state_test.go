package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

/*
TC-STATE-001
Type: Positive
Title: State load valid file returns state
Summary:
Loads a valid JSON state file from disk.
The store should return the target, applied UUID, and timestamp exactly
as persisted.

Validates:
  - valid state file loads successfully
  - target, applied UUID, and applied_at are preserved
*/
func TestStateLoadValidFileReturnsState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	wantTime := time.Date(2026, 5, 18, 12, 30, 0, 0, time.UTC)
	writeStateFileForTest(t, path, `{
  "target": "vyos",
  "applied_uuid": "cfg-valid-1",
  "applied_at": "`+wantTime.Format(time.RFC3339)+`"
}`)

	got, err := NewFileStore(path).Load(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if got.Target != "vyos" {
		t.Fatalf("target got=%q want=vyos", got.Target)
	}
	if got.AppliedUUID != "cfg-valid-1" {
		t.Fatalf("applied uuid got=%q want=cfg-valid-1", got.AppliedUUID)
	}
	if !got.AppliedAt.Equal(wantTime) {
		t.Fatalf("applied_at got=%s want=%s", got.AppliedAt, wantTime)
	}
}

/*
TC-STATE-002
Type: Recovery
Title: State load missing file returns default state
Summary:
Loads state from a path that does not exist.
Missing state is expected on first run and should return the empty
checkpoint state.

Validates:
  - missing file returns no error
  - default empty state is returned
*/
func TestStateLoadMissingFileReturnsDefaultState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-state.json")

	got, err := NewFileStore(path).Load(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if got != (State{}) {
		t.Fatalf("state got=%+v want empty state", got)
	}
}

/*
TC-STATE-003
Type: Positive / Recovery
Title: State load corrupt JSON returns empty state gracefully
Summary:
Loads a corrupt JSON state file.
The store should recover gracefully from corrupt state by returning an empty checkpoint without error.

Validates:
  - corrupt JSON does not return an error
  - empty state is returned
*/
func TestStateLoadCorruptJSONFailsSafely(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	writeStateFileForTest(t, path, `{"target":"vyos","applied_uuid":`)

	got, err := NewFileStore(path).Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != (State{}) {
		t.Fatalf("expected empty state, got %+v", got)
	}
}

/*
TC-STATE-004
Type: Positive / Recovery
Title: State load invalid UUID returns empty state gracefully
Summary:
Loads state content where applied_uuid has an invalid JSON type.
The store should recover gracefully from invalid format by returning an empty state without error.

Validates:
  - invalid JSON formatting does not return an error
  - empty state is returned
*/
func TestStateLoadInvalidUUIDFailsSafely(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	writeStateFileForTest(t, path, `{
  "target": "vyos",
  "applied_uuid": 12345,
  "applied_at": "2026-05-18T12:30:00Z"
}`)

	got, err := NewFileStore(path).Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != (State{}) {
		t.Fatalf("expected empty state, got %+v", got)
	}
}

/*
TC-STATE-005
Type: Positive
Title: State write valid state persists UUID
Summary:
Saves a valid state checkpoint and reloads it.
The store should persist the applied UUID and metadata.

Validates:
  - state file is written
  - reloaded state contains the saved UUID
  - metadata is preserved
*/
func TestStateWriteValidStatePersistsUUID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	want := State{
		Target:      "vyos",
		AppliedUUID: "cfg-write-1",
		AppliedAt:   time.Date(2026, 5, 18, 13, 0, 0, 0, time.UTC),
	}

	store := NewFileStore(path)
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatalf("save state: %v", err)
	}

	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if got.Target != want.Target || got.AppliedUUID != want.AppliedUUID || !got.AppliedAt.Equal(want.AppliedAt) {
		t.Fatalf("loaded state got=%+v want=%+v", got, want)
	}
}

/*
TC-STATE-006
Type: Negative
Title: State save failure returns error
Summary:
Attempts to save under a parent path that is a regular file.
Directory creation should fail and the error must be returned to the
caller.

Validates:
  - save failure is visible
  - error is not swallowed
*/
func TestStateSaveFailureReturnsError(t *testing.T) {
	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("file"), 0o600); err != nil {
		t.Fatalf("write parent file: %v", err)
	}

	err := NewFileStore(filepath.Join(parentFile, "state.json")).Save(context.Background(), State{
		Target:      "vyos",
		AppliedUUID: "cfg-failure-1",
		AppliedAt:   time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "create state directory") {
		t.Fatalf("error %q does not contain create state directory", err.Error())
	}
}

/*
TC-STATE-009
Type: Safety
Title: State save failure does not create partial valid state
Summary:
Forces save to fail before the target state path can be created.
The failed save must not leave behind a valid state file containing the
new UUID checkpoint.

Validates:
  - failed save returns error
  - no partial valid checkpoint file is created
*/
func TestStateSaveDoesNotCreatePartialValidStateOnFailure(t *testing.T) {
	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("file"), 0o600); err != nil {
		t.Fatalf("write parent file: %v", err)
	}
	path := filepath.Join(parentFile, "state.json")

	err := NewFileStore(path).Save(context.Background(), State{
		Target:      "vyos",
		AppliedUUID: "cfg-partial-1",
		AppliedAt:   time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if info, statErr := os.Stat(path); statErr == nil {
		t.Fatalf("expected no partial state file, found mode=%s", info.Mode())
	}
}

/*
TC-STATE-010
Type: Safety
Title: State reflects last applied UUID only
Summary:
Saves two state checkpoints in sequence and reloads state.
The file should reflect the most recent successful save.

Validates:
  - second save replaces first checkpoint
  - loaded applied UUID is the latest saved UUID
*/
func TestStateReflectsLastAppliedUUIDOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := NewFileStore(path)

	first := State{
		Target:      "vyos",
		AppliedUUID: "cfg-first",
		AppliedAt:   time.Date(2026, 5, 18, 13, 0, 0, 0, time.UTC),
	}
	second := State{
		Target:      "vyos",
		AppliedUUID: "cfg-second",
		AppliedAt:   time.Date(2026, 5, 18, 14, 0, 0, 0, time.UTC),
	}
	if err := store.Save(context.Background(), first); err != nil {
		t.Fatalf("save first: %v", err)
	}
	if err := store.Save(context.Background(), second); err != nil {
		t.Fatalf("save second: %v", err)
	}

	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if got.AppliedUUID != "cfg-second" {
		t.Fatalf("applied uuid got=%q want=cfg-second", got.AppliedUUID)
	}
	if !got.AppliedAt.Equal(second.AppliedAt) {
		t.Fatalf("applied_at got=%s want=%s", got.AppliedAt, second.AppliedAt)
	}
}

func writeStateFileForTest(t *testing.T, path string, data string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write state file: %v", err)
	}
}
