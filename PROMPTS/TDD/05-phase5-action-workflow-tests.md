# Phase 5: Action Workflow Tests

## Purpose

Read these files before making changes:

1. `TDD_SPEC.md`
2. `PROMPTS/TDD/PHASE3_COVERAGE.md`
3. `PROMPTS/TDD/PHASE4_COVERAGE.md`
4. `internal/actions/service.go`
5. `internal/actions/types.go`
6. `internal/actions/placeholder_trace.go`
7. `internal/agent/handlers.go`
8. `internal/testutil/README.md`
9. `internal/testutil/*`

We are implementing ONLY Phase 5 from `TDD_SPEC.md`:

```text
Phase 5: Action Workflow Tests
```

Phase 5 should test the action command workflow using placeholder/fake executors.

This phase does NOT require real trace, real rtty, real VyOS, or real NATS.

The goal is to prove the agent handles action commands safely and predictably at service level.

## Mental Model

The action workflow is:

```text
action command received
-> validate action is enabled
-> validate action is supported / executor exists
-> publish received status
-> publish executing status
-> execute action using executor
-> publish completed status and success result
```

Failure workflow:

```text
action command received
-> validation or execution fails
-> publish failed status
-> publish failure result
-> do not publish completed/success result
```

## Strict Scope

Implement only Phase 5 action workflow unit tests and minimal reusable test helpers needed for those tests.

Do NOT implement:

- real VyOS trace
- real VyOS rtty
- real platform action execution
- renderer adapter tests
- apply adapter tests
- real NATS integration tests
- logging/security tests
- restart/concurrency/load tests
- Phase 6 or later work

Do not introduce real VyOS dependency.
Do not introduce real NATS dependency.

## Existing Code Expectations

The current action service should already support:

```text
NewService(...)
Handle(ctx, agentcore.ActionCommand)
enabled action validation
executor lookup
PublishStatus(...)
PublishResult(...)
Executor interface
PlaceholderTraceExecutor
```

Use existing public/internal interfaces. Do not redesign action service unless tests expose a small missing seam.

## Recommended Test File

Add a focused file:

```text
internal/actions/action_workflow_test.go
```

or follow existing repository convention.

If existing tests already cover some cases, keep them and add the missing ones. Avoid unnecessary duplication, but make TDD_SPEC coverage clear.

## Recommended Test Utilities

Use or add small test-only helpers.

Prefer placing reusable helpers under:

```text
internal/testutil/
```

Possible helper:

```go
type FakeActionExecutor struct {
    Output actions.Output
    Err error
    Calls int
    Inputs []agentcore.ActionCommand
    Events *EventRecorder
}
```

Useful behavior:

```text
success output
configured error
call counting
input capture
event recording
optional validation hook
reset
```

Also use existing:

```text
StatusRecorder
ResultRecorder
StatusResultRecorder
EventRecorder
LogCapture
fixtures
```

If a helper is only local to action tests, it can live in the test file. If it is reusable for later phases, put it in `internal/testutil`.

## Required Phase 5 Test Cases

The `TDD_SPEC.md` action workflow section lists ACT-001 through ACT-012.

Implement all P0 items. P1 cancellation/timeout items may be covered if service behavior already exists, or explicitly documented as deferred.

---

# ACT-001: Trace happy path publishes received/executing/completed

Add:

```go
func TestActionTraceHappyPathPublishesReceivedExecutingCompleted(t *testing.T)
```

Expected:

```text
given trace action is enabled
and trace executor succeeds
when action service handles command
then statuses are published in order:
  running / received
  running / executing
  success / completed
```

Assertions:

```text
status stages == received, executing, completed
status order is deterministic
no failed status
```

Use `EventRecorder` or recorder status order.

---

# ACT-002: Trace happy path publishes final result

Add:

```go
func TestActionTraceHappyPathPublishesFinalResult(t *testing.T)
```

Expected:

```text
success result is published
command_type = action
action = trace
result = success
target preserved
rpc_id preserved
payload preserved or expected placeholder payload returned
message is expected
```

If using `PlaceholderTraceExecutor`, assert the placeholder output contract.

If using fake executor, assert output from fake executor is passed to result.

---

# ACT-003: Unsupported action fails

Add:

```go
func TestActionUnsupportedActionFails(t *testing.T)
```

Expected:

```text
action is enabled or requested, but no executor exists
service.Handle returns error
failure status is published
failure result is published
error_code = unsupported_action
executor is not called
completed/success is not published
```

Important:
If current service checks disabled before unsupported, structure the test so the action is enabled but executor is missing. That forces unsupported-action path.

---

# ACT-004: Disabled action fails

Add:

```go
func TestActionDisabledActionFails(t *testing.T)
```

Expected:

```text
action has an executor but is not in enabled list
service.Handle returns error
failure status is published
failure result is published
error_code = disabled_action
executor is not called
completed/success is not published
```

---

# ACT-005: Execution failure publishes failed

Add:

```go
func TestActionExecutionFailurePublishesFailed(t *testing.T)
```

Expected:

```text
action is enabled
executor is present
executor returns error
service.Handle returns error
received status is published
executing status is published
failed status is published
failure result is published
error_code = action_execute_failed
completed/success result is not published
```

Assertions:

```text
executor calls == 1
failure result exists
success result absent
completed status absent
```

---

# ACT-006: Missing or invalid required payload fails

Add:

```go
func TestActionMissingRequiredPayloadFails(t *testing.T)
```

Expected:

```text
trace action with empty/invalid payload fails
service.Handle returns error
failure status/result is published
error_code = invalid_action_payload
executor behavior depends on test shape:
  if using PlaceholderTraceExecutor, executor is called and rejects payload
  if validation is service-level, executor is not called
```

Prefer using `PlaceholderTraceExecutor` because current placeholder validates payload.

Cover at least:

```text
empty payload
invalid JSON payload
```

Table-driven is fine.

---

# ACT-007: Wrong target does not execute

Add:

```go
func TestActionWrongTargetDoesNotExecute(t *testing.T)
```

Expected target behavior must be explicit.

Current action service may not own target filtering because routing/registration may already route only target-owned subjects. If so, do not invent a broad target model without design decision.

Recommended options:

Option A, if action service has configured target or can safely be given one:

```text
wrong target -> no executor call -> failure or ignored result per design
```

Option B, if target ownership is enforced by NATS subject routing / handler registration:

```text
document ACT-007 as deferred or covered at integration/handler-routing phase
```

Do not add misleading service-level target filtering unless the design says action service owns target authorization.

---

# ACT-008: Status sequence order is stable

Add:

```go
func TestActionStatusSequenceOrderIsStable(t *testing.T)
```

Expected success order:

```text
received
executing
completed
```

Expected failure order for executor failure:

```text
received
executing
failed
```

This can be table-driven.

If ACT-001 already clearly asserts success order, ACT-008 can add failure order.

---

# ACT-009: Failure does not publish completed

Add:

```go
func TestActionFailureDoesNotPublishCompleted(t *testing.T)
```

Cover at least:

```text
unsupported action
disabled action
executor failure
invalid payload
```

Expected:

```text
failure result/status exists
completed status absent
success result absent
```

---

# ACT-010: Action preserves correlation data

Add:

```go
func TestActionPreservesCorrelationData(t *testing.T)
```

Prefer table-driven success and failure cases.

Expected status/result preserve:

```text
target
action
rpc_id
command_type = action
result
error_code when failure
```

Note:
`StatusEnvelope` may or may not have an `Action` field depending on `agentcore`. Assert what exists. At minimum result must preserve action and RPC ID.

---

# ACT-011: Context cancellation stops execution

P1.

Add only if the current action service/executor behavior supports it clearly.

Possible test:

```go
func TestActionContextCancellationStopsExecution(t *testing.T)
```

Expected:

```text
context is canceled before or during executor execution
service.Handle returns error
failure status/result is published
success/completed is not published
```

If current behavior is dependency-driven through executor context only, document ACT-011 as deferred or partially covered.

Do not introduce a new cancellation framework unless already present.

---

# ACT-012: Timeout publishes failure

P1.

Add only if current action service has timeout behavior or configuration.

Possible test:

```go
func TestActionTimeoutPublishesFailure(t *testing.T)
```

Expected:

```text
timeout cancels action
failure status/result is published
completed/success not published
```

If timeout is owned by lifecycle/integration layer and not `actions.Service`, document ACT-012 as deferred.

Do not add timeout policy in Phase 5 unless already designed.

---

## Additional Useful Tests

Add these if they are natural and low-risk:

### Action service constructor validation

```go
func TestActionServiceRequiresClient(t *testing.T)
func TestActionServiceRequiresExecutor(t *testing.T)
func TestActionServiceRejectsNilExecutor(t *testing.T)
```

These are not core ACT requirements but make service behavior clearer.

### Placeholder trace executor contract

If not already tested, add:

```go
func TestPlaceholderTraceExecutorValidPayloadSucceeds(t *testing.T)
func TestPlaceholderTraceExecutorInvalidPayloadFails(t *testing.T)
func TestPlaceholderTraceExecutorWrongActionFails(t *testing.T)
```

Keep this small. Do not overbuild real trace behavior.

## Expected Failure Codes

Use the current service contract where available:

```text
disabled_action
unsupported_action
action_execute_failed
invalid_action_payload
status_publish_failed
result_publish_failed
```

Do not rename error codes unless required.

## Expected Status Stages

Success path should be:

```text
received
executing
completed
```

Failure path should be:

```text
received
executing
failed
```

For disabled/unsupported action, expected path may be:

```text
received
failed
```

because executor is never reached.

Document exact behavior in tests.

## Coverage Documentation

Add:

```text
PROMPTS/TDD/PHASE5_COVERAGE.md
```

Map every action workflow requirement:

```text
ACT-001
ACT-002
ACT-003
ACT-004
ACT-005
ACT-006
ACT-007
ACT-008
ACT-009
ACT-010
ACT-011
ACT-012
```

For each row:

```text
ID
Status: Covered / Deferred / Partially Covered
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

This is especially important for:

```text
ACT-007 wrong target
ACT-011 context cancellation
ACT-012 timeout
```

## Test Helper Rules

Keep helpers simple.

Do not create a fake NATS client if the action service only needs the existing status/result recorder shape.

Do not add real NATS.

Do not add real VyOS.

Do not add actual trace/rtty platform code.

If adding `FakeActionExecutor`, make it generic and reusable.

Suggested shape:

```go
type FakeActionExecutor struct {
    Output actions.Output
    Err error
    Calls int
    Inputs []agentcore.ActionCommand
    Events *EventRecorder
    Validate func(agentcore.ActionCommand) error
}
```

Useful methods:

```go
Calls() int
Inputs() []agentcore.ActionCommand
LastInput() (agentcore.ActionCommand, bool)
Reset()
```

## Commands To Run

After implementation, run:

```bash
go test ./...
go test -race ./...
go build ./...
```

If CI smoke tests are quick and available, make sure they are not broken.

## Final Codex Response Required

After implementation, summarize:

1. Files added.
2. Files modified.
3. Phase 5 tests added.
4. `ACT-*` IDs covered.
5. Any `ACT-*` IDs deferred and why.
6. Any service behavior changes.
7. Any test helper changes.
8. Commands run and results.
9. What remains for Phase 6.

## Acceptance Criteria

Phase 5 is complete when:

```text
[ ] ACT-001 trace happy path publishes received/executing/completed
[ ] ACT-002 trace happy path publishes final result
[ ] ACT-003 unsupported action fails safely
[ ] ACT-004 disabled action fails safely
[ ] ACT-005 action execution failure publishes failed
[ ] ACT-006 missing/invalid payload fails safely
[ ] ACT-007 wrong target behavior covered or explicitly deferred
[ ] ACT-008 status sequence order is stable
[ ] ACT-009 failure does not publish completed
[ ] ACT-010 action preserves correlation data
[ ] ACT-011 context cancellation covered or explicitly deferred
[ ] ACT-012 timeout behavior covered or explicitly deferred
[ ] no real VyOS dependency introduced
[ ] no real NATS dependency introduced
[ ] PHASE5_COVERAGE.md maps all ACT items
[ ] go test ./... passes
[ ] go test -race ./... passes
[ ] go build ./... passes
```
