# Phase 4: Configure Failure Handling Tests

## Purpose

Read these files before making changes:

1. `TDD_SPEC.md`
2. `PROMPTS/TDD/03-phase3-configure-workflow-correctness.md`
3. `PROMPTS/TDD/PHASE3_COVERAGE.md`
4. `internal/configure/service.go`
5. `internal/configure/configure_workflow_test.go`
6. `internal/testutil/README.md`
7. `internal/testutil/*`

We are implementing ONLY Phase 4 from `TDD_SPEC.md`:

```text
Phase 4: Configure Failure Handling Tests
```

Phase 3 proved the successful configure workflow and safe validation/idempotency paths.

Phase 4 must now prove that configure remains safe when renderer, apply, state-save, context, timeout, or dependency failure happens.

This phase is production-readiness hardening.

## Phase 4 Goal

Prove the configure workflow is safe when:

```text
renderer fails
apply fails
state save fails after apply
unexpected dependency panic happens, if recovery is supported
context cancellation happens
timeout happens, if timeout behavior is supported
```

The most important rule:

```text
A failed configure must never falsely checkpoint the requested UUID as applied.
```

## Strict Scope

Implement only configure failure-handling tests and minimal service fixes needed to make those tests pass.

Do NOT implement:

- action workflow tests
- renderer adapter mapping tests
- apply adapter mapping tests
- real NATS integration tests
- real VyOS tests
- logging/security tests
- restart/reconciliation tests
- concurrency/load tests
- Phase 5 or later tests

Do not introduce real VyOS dependency.
Do not introduce real NATS dependency.

## Existing Phase 3 Baseline

Phase 3 already covers:

- configure success path
- renderer/apply/state-save call counts on success
- state save after apply
- success status/result after state save
- already-in-sync behavior
- repeated same UUID idempotency
- new UUID after old state
- missing desired config
- wrong target
- UUID mismatch
- empty target
- empty UUID
- invalid payload
- correlation preservation

Do not duplicate those tests unless needed for failure assertions.

## Required Phase 4 Test Cases

Implement these tests in a focused file, preferably:

```text
internal/configure/configure_failure_test.go
```

or use the existing package convention.

Use Phase 1 test helpers from `internal/testutil`.

---

# FAIL-001: Renderer failure stops before apply

Add:

```go
func TestConfigureRendererFailureStopsBeforeApply(t *testing.T)
```

Expected behavior:

```text
given valid desired config and new UUID
and renderer returns an error
when configure is handled
then service.Handle returns error
and apply is not called
and state save is not called
and failure status is published
and failure result is published
and no success result is published
```

Assertions:

```text
renderer calls == 1
apply calls == 0
state save calls == 0
failure result exists
failure error_code == render_failed
success result does not exist
```

---

# FAIL-002: Renderer failure preserves previous state

Add:

```go
func TestConfigureRendererFailurePreservesPreviousState(t *testing.T)
```

Expected behavior:

```text
given local state has AppliedUUID = cfg-previous
and desired config has UUID = cfg-new
and renderer fails
then state remains cfg-previous
and cfg-new is not saved
```

Assertions:

```text
state save calls == 0
state current/applied UUID remains cfg-previous
no saved state has AppliedUUID cfg-new
```

---

# FAIL-003: Renderer failure publishes clear failure

Add:

```go
func TestConfigureRendererFailurePublishesClearFailure(t *testing.T)
```

Expected behavior:

```text
failure result is observable
failure result has:
  result = failure
  command_type = configure
  error_code = render_failed
  target preserved
  uuid preserved
  rpc_id preserved
```

Do not leak raw payload/rendered commands in the result message.

---

# FAIL-004: Apply failure does not save state

Add:

```go
func TestConfigureApplyFailureDoesNotSaveState(t *testing.T)
```

Expected behavior:

```text
given renderer succeeds
and apply returns an error
then service.Handle returns error
and renderer is called once
and apply is called once
and state save is not called
and failure result is published
and no success result is published
```

Assertions:

```text
renderer calls == 1
apply calls == 1
state save calls == 0
failure error_code == apply_failed
```

---

# FAIL-005: Apply failure allows retry with same UUID

Add:

```go
func TestConfigureApplyFailureAllowsRetrySameUUID(t *testing.T)
```

Expected behavior:

```text
first configure attempt:
  renderer succeeds
  apply fails
  state is not saved
  failure published

second configure attempt with same UUID:
  apply is allowed to run again
  if apply succeeds on retry, state is saved
  success result is published
```

Implementation hint:

Use a fake apply engine that can fail once and then succeed, or update `FakeApplyEngine` with reusable fail-on-call behavior if needed.

Assertions:

```text
after first attempt:
  renderer calls == 1
  apply calls == 1
  state save calls == 0
  failure result exists

after second attempt:
  renderer calls == 2
  apply calls == 2
  state save calls == 1
  saved AppliedUUID == requested UUID
  success result exists
```

This proves the failed UUID was not falsely checkpointed.

---

# FAIL-006: Apply failure preserves previous UUID

Add:

```go
func TestConfigureApplyFailurePreservesPreviousUUID(t *testing.T)
```

Expected behavior:

```text
given local state AppliedUUID = cfg-previous
and desired UUID = cfg-new
and apply fails
then state remains cfg-previous
and cfg-new is not saved
```

Assertions:

```text
state save calls == 0
current state AppliedUUID remains cfg-previous
no saved state has cfg-new
failure error_code == apply_failed
```

---

# FAIL-007: State save failure marks configure failed

Add:

```go
func TestConfigureStateSaveFailureMarksConfigureFailed(t *testing.T)
```

Expected behavior:

```text
given render succeeds
and apply succeeds
and state save fails
then service.Handle returns error
and configure is reported as failure
and no success result is published
```

Assertions:

```text
renderer calls == 1
apply calls == 1
state save calls == 1
failure result exists
failure error_code == state_save_failed
success result does not exist
```

Important:
Because Phase 3 changed final success status/result to publish after state save, this path must not publish final success.

---

# FAIL-008: State save failure does not persist UUID

Add:

```go
func TestConfigureStateSaveFailureDoesNotPersistUUID(t *testing.T)
```

Expected behavior:

```text
given previous UUID = cfg-previous
and new desired UUID = cfg-new
and apply succeeds
and state save fails
then cfg-new is not treated as applied
```

Assertions:

```text
state save attempted once
save returned error
fake state current AppliedUUID remains cfg-previous
saved attempt may be recorded as attempted save, but must not be considered successfully persisted
already-in-sync must not happen on retry unless save later succeeds
```

Important note:
If `FakeStateStore.SavedStates()` records attempted saves including failed attempts, do not treat that as successful persistence. Assert against current/loadable state, not only attempted saves.

---

# FAIL-009: State save failure retries safely

Add:

```go
func TestConfigureStateSaveFailureRetriesSafely(t *testing.T)
```

Expected behavior:

```text
first configure attempt:
  render succeeds
  apply succeeds
  state save fails
  failure published
  UUID not checkpointed

second configure attempt with same UUID:
  workflow does not think already-in-sync
  render/apply/save are attempted again
  if save succeeds on retry, success result is published
```

Implementation hint:

Use a fake state store that can fail first save then succeed on second save, or add a small reusable fail-on-save-call option.

Assertions:

```text
after first attempt:
  renderer calls == 1
  apply calls == 1
  save calls == 1
  failure error_code == state_save_failed

after second attempt:
  renderer calls == 2
  apply calls == 2
  save calls == 2
  success result exists
  saved/loadable AppliedUUID == requested UUID
```

This proves no false idempotency after checkpoint failure.

---

# FAIL-010: Configure failure does not publish success

Add one table-driven test if possible:

```go
func TestConfigureFailureDoesNotPublishSuccess(t *testing.T)
```

Cover at least:

```text
renderer failure
apply failure
state-save failure
```

Expected for each case:

```text
failure result exists
failure status exists
success result does not exist
final success status does not exist
```

If this duplicates earlier assertions, it is acceptable because `TDD_SPEC.md` has a separate FAIL-010 requirement. Keep it readable.

---

# FAIL-011: Configure failure includes correlation data

Add:

```go
func TestConfigureFailureIncludesCorrelationData(t *testing.T)
```

Prefer table-driven cases for:

```text
renderer failure
apply failure
state-save failure
```

Expected failure result preserves:

```text
target
uuid
rpc_id
command_type = configure
result = failure
error_code
```

This may overlap with FAIL-003, but this test makes correlation coverage explicit.

---

# FAIL-012: Unexpected panic recovered as failure

Add only if current service supports or should support panic recovery.

Suggested test:

```go
func TestConfigureUnexpectedPanicRecoveredAsFailure(t *testing.T)
```

Expected behavior if supported:

```text
dependency panic does not crash test
service.Handle returns error
failure status/result is published
state save is not called unless panic occurs after save
failure preserves correlation data
```

If the current service does not support panic recovery and adding it would be a meaningful production behavior change, do one of these:

Option A, preferred if small and safe:
- add a minimal recover guard around configure processing
- publish failure result with error_code = unexpected_panic
- add test

Option B:
- document FAIL-012 as deferred in Phase 4 coverage with reason:
  "Panic recovery is P1 and requires explicit production behavior decision."

Do not overbuild.

---

# FAIL-013: Context cancellation stops workflow

Add if current fake components can support context cancellation cleanly.

Suggested tests:

```go
func TestConfigureContextCancellationStopsWorkflowBeforeRender(t *testing.T)
func TestConfigureContextCancellationStopsWorkflowBeforeApply(t *testing.T)
```

Expected behavior:

```text
context cancellation is respected
workflow returns error
no unsafe state save occurs
failure result/status is published if service owns that behavior
```

At minimum, test cancellation before render/apply if current service/fakes support it.

If current service or interfaces do not check context except through dependencies, document what is covered and what is deferred.

---

# FAIL-014: Timeout publishes failure

Add only if service currently has timeout behavior or configuration.

Suggested test:

```go
func TestConfigureTimeoutPublishesFailure(t *testing.T)
```

Expected behavior if supported:

```text
timeout cancels workflow
state is not saved unsafely
failure result/status is published
correlation is preserved
```

If timeout behavior belongs to a higher agent/handler layer rather than `configure.Service`, document FAIL-014 as deferred to lifecycle/integration phase.

Do not introduce a new timeout framework in Phase 4 unless already present.

---

# State-related overlap

`TDD_SPEC.md` section 14 includes:

```text
STATE-006 TestStateNotWrittenWhenRendererFails
STATE-007 TestStateNotWrittenWhenApplyFails
```

These are effectively covered by Phase 4 configure failure tests.

Document them in Phase 4 coverage as covered by:

```text
TestConfigureRendererFailureStopsBeforeApply
TestConfigureApplyFailureDoesNotSaveState
```

Do not duplicate state-package tests unless useful.

## Service Behavior Rules

The service must follow these safety rules:

### Renderer failure

```text
load desired
load state
render fails
-> apply not called
-> state save not called
-> failure status/result published
-> success not published
```

### Apply failure

```text
load desired
load state
render succeeds
apply fails
-> state save not called
-> failure status/result published
-> success not published
```

### State-save failure

```text
load desired
load state
render succeeds
apply succeeds
state save fails
-> failure status/result published
-> success not published
-> UUID not considered applied
-> retry with same UUID is allowed
```

## Test Helper Rules

Use existing fakes and recorders.

Only add small reusable helper improvements if needed, for example:

```text
FakeApplyEngine.FailOnCall
FakeApplyEngine.ErrSequence
FakeStateStore.SaveErrSequence
FakeStateStore.Current update only on successful Save
ContainsFailureResult
ContainsSuccessStatus
AssertNoSuccessResult
AssertNoSuccessStatus
```

If adding helper behavior, document the semantics in comments.

Important `FakeStateStore` rule:

```text
SavedStates may include attempted saves.
Current/loadable state should change only after successful Save.
```

This matters for state-save failure tests.

## Coverage Documentation

Add a Phase 4 coverage document, preferably:

```text
PROMPTS/TDD/PHASE4_COVERAGE.md
```

Map every `FAIL-*` requirement to:

```text
ID
Status: Covered / Deferred
Test file
Test name
Notes
```

Include all:

```text
FAIL-001
FAIL-002
FAIL-003
FAIL-004
FAIL-005
FAIL-006
FAIL-007
FAIL-008
FAIL-009
FAIL-010
FAIL-011
FAIL-012
FAIL-013
FAIL-014
```

If any P1 item is deferred, explain:

```text
why it is deferred
which future phase owns it
what behavior decision is needed
```

Also include cross-coverage for:

```text
STATE-006 State not written when renderer fails
STATE-007 State not written when apply fails
```

## Test Names Expected

Prefer these exact names where applicable:

```go
TestConfigureRendererFailureStopsBeforeApply
TestConfigureRendererFailurePreservesPreviousState
TestConfigureRendererFailurePublishesClearFailure
TestConfigureApplyFailureDoesNotSaveState
TestConfigureApplyFailureAllowsRetrySameUUID
TestConfigureApplyFailurePreservesPreviousUUID
TestConfigureStateSaveFailureMarksConfigureFailed
TestConfigureStateSaveFailureDoesNotPersistUUID
TestConfigureStateSaveFailureRetriesSafely
TestConfigureFailureDoesNotPublishSuccess
TestConfigureFailureIncludesCorrelationData
TestConfigureUnexpectedPanicRecoveredAsFailure
TestConfigureContextCancellationStopsWorkflow
TestConfigureTimeoutPublishesFailure
```

## Commands To Run

After implementation, run:

```bash
go test ./...
go test -race ./...
go build ./...
```

If smoke tests are part of CI and still quick, ensure they are not broken.

## Final Codex Response Required

After implementation, summarize:

1. Files added.
2. Files modified.
3. Phase 4 tests added.
4. `FAIL-*` IDs covered.
5. Any `FAIL-*` IDs deferred and why.
6. Any service behavior changes.
7. Any test helper changes.
8. Commands run and results.
9. What remains for Phase 5.

## Acceptance Criteria

Phase 4 is complete when:

```text
[ ] FAIL-001 renderer failure stops before apply
[ ] FAIL-002 renderer failure preserves previous state
[ ] FAIL-003 renderer failure publishes clear failure
[ ] FAIL-004 apply failure does not save state
[ ] FAIL-005 apply failure allows retry same UUID
[ ] FAIL-006 apply failure preserves previous UUID
[ ] FAIL-007 state save failure marks configure failed
[ ] FAIL-008 state save failure does not persist UUID
[ ] FAIL-009 state save failure retries safely
[ ] FAIL-010 failure paths do not publish success
[ ] FAIL-011 failure result includes correlation data
[ ] FAIL-012 panic recovery covered or explicitly deferred
[ ] FAIL-013 context cancellation covered or explicitly deferred
[ ] FAIL-014 timeout behavior covered or explicitly deferred
[ ] STATE-006 cross-coverage documented
[ ] STATE-007 cross-coverage documented
[ ] no real VyOS dependency introduced
[ ] no real NATS dependency introduced
[ ] go test ./... passes
[ ] go test -race ./... passes
[ ] go build ./... passes
[ ] PROMPTS/TDD/PHASE4_COVERAGE.md documents all FAIL items
```
