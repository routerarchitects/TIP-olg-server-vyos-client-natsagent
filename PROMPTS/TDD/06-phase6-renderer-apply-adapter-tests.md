# Phase 6: Renderer and Apply Adapter Tests

## Purpose

Read these files before making changes:

1. `TDD_SPEC.md`
2. `PROMPTS/TDD/PHASE3_COVERAGE.md`
3. `PROMPTS/TDD/PHASE4_COVERAGE.md`
4. `PROMPTS/TDD/PHASE5_COVERAGE.md`
5. `internal/configure/service.go`
6. `internal/renderer/*`
7. `internal/apply/*`
8. `internal/testutil/README.md`
9. `internal/testutil/*`
10. Any existing renderer/apply adapter code in this repo.

We are implementing ONLY Phase 6 from `TDD_SPEC.md`:

```text
Phase 6: Renderer and Apply Adapter Tests
```

Phase 3 tested configure workflow correctness using fakes.
Phase 4 tested configure failure safety using fakes.
Phase 5 tested action workflow using placeholder/fake executors.

Phase 6 now tests the adapter boundaries:

```text
desired config -> renderer adapter -> renderer library/input/output
rendered config -> apply adapter -> VyOS apply backend/input
```

This phase should validate mapping, metadata preservation, prepare/apply behavior, error propagation, and mutation safety.

## Phase 6 Mental Model

Adapters are glue code.

They are risky because they translate between your agent's internal types and external/library/backend types.

The main job of Phase 6 is to prove:

```text
the agent passes the right fields to renderer/apply libraries
and preserves target, UUID, payload, rendered commands, and correlation metadata
and propagates adapter/backend errors safely
```

## Strict Scope

Implement only renderer/apply adapter tests and minimal adapter/test seams required to make those tests pass.

Do NOT implement:

- real VyOS lab tests
- real NATS integration tests
- action workflow tests
- configure workflow service tests already covered in Phases 3/4
- logging/security tests except the apply-adapter safety case listed here
- restart/concurrency/load tests
- Phase 7 or later work

Do not require real VyOS.
Do not require real NATS.

If a real renderer/apply backend package is imported, use fake/mock backend implementations or existing adapter seams.

## Required TDD_SPEC Coverage

`TDD_SPEC.md` section 15 lists:

Renderer Adapter:
- RAD-001 through RAD-008

Apply Adapter:
- AAD-001 through AAD-011

Phase 6 is complete only when all P0 items are covered, and P1 items are either covered or explicitly documented as deferred with reason.

## Recommended Test Files

Use the existing package layout.

Likely files:

```text
internal/renderer/adapter_test.go
internal/apply/adapter_test.go
```

or, if current package names differ:

```text
internal/renderer/*_test.go
internal/apply/*_test.go
```

Do not place adapter tests inside `internal/configure` unless the adapter code itself lives there.

## Test Utility Guidance

Use existing `internal/testutil` fakes/recorders where possible.

Add small reusable fakes only if needed, for example:

```go
FakeRendererLibrary
FakeApplyBackend
FakePrepareApplyBackend
```

Useful fake behavior:

```text
call counts
input capture
configured output
configured error
prepare call count
apply call count
input mutation checks
event recording
safe log capture
```

Keep helpers simple and deterministic.

## Renderer Adapter Test Cases

---

# RAD-001: Minimal payload maps correctly

Add:

```go
func TestRendererAdapterMinimalPayloadMapsCorrectly(t *testing.T)
```

Expected:

```text
given a minimal DesiredConfig record
when renderer adapter renders it
then underlying renderer/library receives expected fields
and adapter returns expected rendered output
```

Assert:

```text
target preserved
uuid preserved
payload preserved
payload bytes equal original
renderer called once
output text/commands returned
```

---

# RAD-002: Large payload maps correctly

Add:

```go
func TestRendererAdapterLargePayloadMapsCorrectly(t *testing.T)
```

Expected:

```text
large desired payload does not crash
payload is passed through intact
renderer called once
output is returned
```

Use a deterministic large JSON payload.

Do not make test slow.

If large-payload behavior is better handled in load/security phase, cover basic large payload pass-through here and document limits.

---

# RAD-003: Invalid payload returns error

Add:

```go
func TestRendererAdapterInvalidPayloadReturnsError(t *testing.T)
```

Expected:

```text
invalid payload is rejected or renderer error is propagated
adapter returns error
no successful rendered output is returned
```

If current renderer adapter delegates validation to renderer library, configure fake renderer to return error and assert propagation.

---

# RAD-004: Renderer adapter preserves UUID

Add:

```go
func TestRendererAdapterPreservesUUID(t *testing.T)
```

Expected:

```text
input UUID == renderer/library input UUID
output UUID, if present, matches input UUID
```

---

# RAD-005: Renderer adapter preserves target

Add:

```go
func TestRendererAdapterPreservesTarget(t *testing.T)
```

Expected:

```text
input target == renderer/library input target
output target, if present, matches input target
```

---

# RAD-006: Renderer adapter preserves correlation ID

P1.

Add if renderer adapter/library supports RPC ID/correlation ID.

```go
func TestRendererAdapterPreservesCorrelationID(t *testing.T)
```

If renderer interface does not include RPC ID today, document as deferred or not applicable in `PHASE6_COVERAGE.md`.

Do not add RPC ID to renderer adapter only for the test unless the design requires it.

---

# RAD-007: Renderer adapter propagates renderer error

Add:

```go
func TestRendererAdapterPropagatesRendererError(t *testing.T)
```

Expected:

```text
underlying renderer/library returns error
adapter returns error
error is not swallowed
no successful output is returned
```

---

# RAD-008: Renderer adapter does not mutate payload

P1 but useful.

Add:

```go
func TestRendererAdapterDoesNotMutatePayload(t *testing.T)
```

Expected:

```text
original desired payload bytes remain unchanged after render
renderer adapter does not mutate input record
```

Use byte copy comparison before/after.

## Apply Adapter Test Cases

---

# AAD-001: Rendered output maps to apply input

Add:

```go
func TestApplyAdapterRenderedOutputMapsToApplyInput(t *testing.T)
```

Expected:

```text
given renderer.Output with target, uuid, text/commands
when apply adapter applies it
then underlying backend receives expected apply input
```

Assert:

```text
target preserved
uuid preserved
rendered text/commands preserved
backend called once
```

---

# AAD-002: Calls Prepare when supported

Add:

```go
func TestApplyAdapterCallsPrepareWhenSupported(t *testing.T)
```

Expected:

```text
if backend supports prepare + apply
adapter calls Prepare exactly once before Apply
```

Assert order:

```text
prepare -> apply
```

Use event recorder/order recorder if available.

---

# AAD-003: Logs plan fields safely

P1.

Add if apply adapter emits plan logs.

```go
func TestApplyAdapterLogsPlanFieldsSafely(t *testing.T)
```

Expected:

```text
logs contain safe summary fields only
logs do not include raw rendered commands unless explicit debug flag exists
```

If logging is not in apply adapter or belongs to Phase 8 logging/security, document as deferred to logging/security phase.

---

# AAD-004: Calls Apply exactly once

Add:

```go
func TestApplyAdapterCallsApplyExactlyOnce(t *testing.T)
```

Expected:

```text
successful apply calls backend Apply exactly once
does not duplicate apply
```

---

# AAD-005: Propagates Prepare error

Add:

```go
func TestApplyAdapterPropagatesPrepareError(t *testing.T)
```

Expected:

```text
Prepare returns error
adapter returns error
Apply is not called
```

---

# AAD-006: Propagates Apply error

Add:

```go
func TestApplyAdapterPropagatesApplyError(t *testing.T)
```

Expected:

```text
Prepare succeeds if present
Apply returns error
adapter returns error
error is not swallowed
```

---

# AAD-007: Prepare does not mutate input for Apply

Add:

```go
func TestApplyAdapterPrepareDoesNotMutateInputForApply(t *testing.T)
```

Expected:

```text
Prepare receives intended input
Apply receives correct intended input
Prepare cannot accidentally corrupt Apply input
```

If current backend prepare returns a plan rather than mutating input, assert that original rendered output and apply input remain unchanged.

This is a safety test.

---

# AAD-008: Backend with both Prepare and Apply uses correct input

Add:

```go
func TestApplyAdapterBackendWithBothPrepareAndApplyUsesCorrectInput(t *testing.T)
```

Expected:

```text
backend supports both Prepare and Apply
Prepare called once with correct input
Apply called once with correct input
Apply input is not stale, empty, or mutated
```

This is a critical reviewer case.

---

# AAD-009: Does not apply when Prepare fails

Add:

```go
func TestApplyAdapterDoesNotApplyWhenPrepareFails(t *testing.T)
```

Expected:

```text
Prepare fails
Apply call count == 0
adapter returns prepare error
```

This may overlap AAD-005, but keep explicit if it improves TDD_SPEC traceability.

---

# AAD-010: Handles nil or empty plan safely

P1.

Add if adapter has a plan abstraction.

```go
func TestApplyAdapterHandlesNilOrEmptyPlanSafely(t *testing.T)
```

Expected:

```text
nil/empty plan does not panic
behavior follows current spec
```

If there is no plan abstraction, document as not applicable/deferred.

---

# AAD-011: Does not log rendered commands by default

Add:

```go
func TestApplyAdapterDoesNotLogRenderedCommandsByDefault(t *testing.T)
```

Expected:

```text
default info logs do not contain rendered command text
```

If apply adapter does not log at all, this can be documented as covered by absence/no logging or deferred to Phase 8.

Do not add logging just for the test.

## Coverage Documentation

Add:

```text
PROMPTS/TDD/PHASE6_COVERAGE.md
```

Map every item:

```text
RAD-001
RAD-002
RAD-003
RAD-004
RAD-005
RAD-006
RAD-007
RAD-008
AAD-001
AAD-002
AAD-003
AAD-004
AAD-005
AAD-006
AAD-007
AAD-008
AAD-009
AAD-010
AAD-011
```

For each row include:

```text
ID
Status: Covered / Deferred / Not Applicable
Test file
Test name
Notes
```

If deferred, explain:

```text
why it is deferred
which future phase owns it
what behavior decision is needed
```

Likely deferrals may include:

```text
RAD-006 if renderer adapter has no rpc_id field
AAD-003 if plan logging belongs to Phase 8
AAD-010 if there is no plan abstraction
AAD-011 if logging/security phase owns this
```

But do not defer P0 items unless there is a clear architecture reason.

## Service Integration Check

Do not duplicate Phase 3 configure service tests.

But it is useful to ensure current configure service still compiles against adapter interfaces and test fakes.

If needed, add tiny compile-time assertions such as:

```go
var _ configure.Renderer = (*Adapter)(nil)
var _ configure.ApplyEngine = (*Adapter)(nil)
```

Only if this matches current package design.

## Rules for Production Code Changes

Prefer tests first.

Production changes are allowed only if needed to:

```text
expose dependency injection seams
make adapter behavior explicit
fix discovered adapter mapping bugs
prevent unsafe mutation
propagate errors correctly
```

Do not refactor broadly.

Do not implement real VyOS behavior.

Do not change placeholder behavior unless tests reveal a real adapter contract bug.

## Commands To Run

After implementation, run:

```bash
go test ./...
go test -race ./...
go build ./...
```

If CI smoke tests are quick and available, ensure they still pass.

## Final Codex Response Required

After implementation, summarize:

1. Files added.
2. Files modified.
3. Renderer adapter tests added.
4. Apply adapter tests added.
5. RAD-* IDs covered.
6. AAD-* IDs covered.
7. Any IDs deferred/not applicable and why.
8. Production code changes, if any.
9. Test helper changes, if any.
10. Commands run and results.
11. What remains for Phase 7.

## Acceptance Criteria

Phase 6 is complete when:

```text
[ ] RAD-001 minimal payload mapping covered
[ ] RAD-002 large payload mapping covered
[ ] RAD-003 invalid payload/error covered
[ ] RAD-004 UUID preservation covered
[ ] RAD-005 target preservation covered
[ ] RAD-006 correlation ID covered or documented N/A/deferred
[ ] RAD-007 renderer error propagation covered
[ ] RAD-008 payload mutation safety covered
[ ] AAD-001 rendered output -> apply input mapping covered
[ ] AAD-002 Prepare called when supported
[ ] AAD-003 safe plan logging covered or deferred to logging/security
[ ] AAD-004 Apply called exactly once
[ ] AAD-005 Prepare error propagated
[ ] AAD-006 Apply error propagated
[ ] AAD-007 Prepare does not mutate apply input
[ ] AAD-008 backend with prepare/apply uses correct input
[ ] AAD-009 no Apply when Prepare fails
[ ] AAD-010 nil/empty plan handled or documented N/A/deferred
[ ] AAD-011 rendered commands not logged by default or deferred to logging/security
[ ] no real VyOS dependency introduced
[ ] no real NATS dependency introduced
[ ] PHASE6_COVERAGE.md maps all RAD/AAD items
[ ] go test ./... passes
[ ] go test -race ./... passes
[ ] go build ./... passes
```
