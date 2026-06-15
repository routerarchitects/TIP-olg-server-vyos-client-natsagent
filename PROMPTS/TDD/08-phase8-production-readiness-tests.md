# Phase 8: Logging, Restart, Idempotency, Concurrency, and Lightweight Load Tests

## Purpose

Read these files before making changes:

1. `TDD_SPEC.md`
2. `PROMPTS/TDD/PHASE3_COVERAGE.md`
3. `PROMPTS/TDD/PHASE4_COVERAGE.md`
4. `PROMPTS/TDD/PHASE5_COVERAGE.md`
5. `PROMPTS/TDD/PHASE6_COVERAGE.md`
6. `PROMPTS/TDD/PHASE7_COVERAGE.md`
7. `.github/workflows/ci.yml`
8. `internal/configure/*`
9. `internal/actions/*`
10. `internal/state/*`
11. `internal/agent/*`
12. `internal/renderervyos/*`
13. `internal/applyvyos/*`
14. `internal/testutil/*`
15. Existing integration tests under `tests/integration/`

We are implementing ONLY Phase 8 from `TDD_SPEC.md`:

```text
Phase 8: Logging, Restart, Concurrency, and Load
```

This phase collects production-readiness hardening that does not require a real VyOS device:

```text
logging/security safety
restart and persistence behavior
retry/idempotency completeness
concurrency/race sanity
large payload / lightweight load sanity
```

The goal is to close remaining non-lab production-readiness gaps before Phase 9 real VyOS evidence.

## Phase 8 Mental Model

Earlier phases proved:

```text
Phase 1: test infrastructure
Phase 2: config/state basics
Phase 3: configure happy path
Phase 4: configure failure safety
Phase 5: action workflow
Phase 6: adapter boundaries
Phase 7: real NATS mocked integration
```

Phase 8 asks:

```text
Will this remain safe under production-like edge conditions?
```

Specifically:

```text
do logs hide sensitive payloads/commands?
does restart with persisted state avoid duplicate apply?
do duplicate/retry events stay idempotent?
do concurrent/burst events avoid race/corruption?
do large payloads/large rendered outputs avoid crashes?
```

## Does Phase 8 Require Real VyOS or Real NATS?

No real VyOS is required.

Most Phase 8 tests should be normal Go unit tests using:

```text
FakeRenderer
FakeApplyEngine
FakeStateStore or real temp file state store
StatusResultRecorder
LogCapture
EventRecorder
temporary directories
```

Real NATS is optional and should only be used if reusing Phase 7 integration helpers for burst/routing behavior is useful.

The preferred approach:

```text
unit tests for logging, restart/state, idempotency, and large payload behavior
race detector in CI via go test -race ./...
optional integration-tag tests only if necessary
```

Phase 8 should be fully runnable in CI/CD.

It does not need real VyOS and should not require external NATS infrastructure.

## Strict Scope

Implement only Phase 8 test-hardening work and minimal fixes needed to make those tests pass.

Do NOT implement:

- real VyOS lab tests
- real platform apply
- real trace/rtty execution
- new business features
- broad runtime redesign
- Phase 9 real-device evidence

Do not introduce real VyOS dependency.

Keep tests deterministic, CI-friendly, and time-bounded.

## TDD_SPEC Sections Covered By Phase 8

Phase 8 should cover these TDD_SPEC sections:

```text
16. Logging and Security Tests: LOG-001 through LOG-011
19. Restart and Persistence Tests: RST-001 through RST-006
20. Retry and Idempotency Tests: IDEMP-001 through IDEMP-008
21. Concurrency and Race Tests: CONC-001 through CONC-006
22. Large Payload and Lightweight Load Tests: PERF-001 through PERF-005
```

Some items may already be covered by earlier phases. Do not duplicate blindly. Instead:

```text
map existing coverage where valid
add missing tests
document deferred/not-applicable items clearly
```

## Recommended Test Files

Use focused files by concern:

```text
internal/configure/logging_security_test.go
internal/configure/restart_persistence_test.go
internal/configure/idempotency_retry_test.go
internal/configure/concurrency_race_test.go
internal/configure/large_payload_test.go
internal/actions/action_large_payload_test.go
internal/state/restart_persistence_test.go
PROMPTS/TDD/PHASE8_COVERAGE.md
```

Adapt paths to the actual package layout.

## Part A: Logging and Security Tests

TDD_SPEC section 16 requires LOG-001 through LOG-011.

Goal:

```text
logs must be useful but must not leak raw desired config, rendered commands, apply plan commands, passwords, tokens, keys, or large payloads by default
```

### LOG-001: Info logs do not log raw payload

Add or map:

```go
func TestLoggingInfoLevelDoesNotLogPayload(t *testing.T)
```

Expected:

```text
configure/action runs with payload containing sensitive-looking fields
normal info logs do not contain raw payload JSON
logs may contain safe metadata like target, uuid, rpc_id, stage
```

### LOG-002: Info logs do not log rendered commands

Add or map:

```go
func TestLoggingInfoLevelDoesNotLogRenderedCommands(t *testing.T)
```

Expected:

```text
rendered commands include sensitive-looking text
normal logs do not include command text
```

### LOG-003: Info logs do not log apply plan commands

Add or map:

```go
func TestLoggingInfoLevelDoesNotLogApplyPlan(t *testing.T)
```

Expected:

```text
apply plan has delete/set commands
normal logs include only counts/safe flags, not raw commands
```

If already covered by Phase 6 `TestApplyAdapterLogsPlanFieldsSafely`, map it in coverage.

### LOG-004: Debug with payload flag does not log raw payload

P1.

Add only if the current code has both debug level and explicit payload logging flag.

If no such behavior exists, document as deferred to debug logging implementation.

### LOG-005: Debug without payload flag does not log payload

Add if debug logging configuration exists.

Expected:

```text
debug level alone is not enough to emit raw payload
```

### LOG-006: Payload flag without debug does not log payload

Add if payload logging flag exists.

Expected:

```text
payload flag alone is not enough
```

### LOG-007: Partial debug config does not emit debug logs

Add if debug config exists.

Expected:

```text
debug payload logging requires complete/explicit debug config
partial debug configuration remains safe
```

### LOG-008: Redacts known secret fields

P1.

Add if redaction helpers exist.

Expected:

```text
password/token/key values absent or redacted
```

If no redaction layer exists because default behavior avoids raw payload entirely, document as deferred or covered-by-no-raw-payload depending on current design.

### LOG-009: Large payload logging does not crash

Add:

```go
func TestLoggingLargePayloadDoesNotCrash(t *testing.T)
```

Expected:

```text
large payload command flow completes/fails safely
logging does not panic
```

### LOG-010: Large payload does not convert unnecessarily to string

This is hard to prove directly unless code structure supports it.

Recommended:

```text
assert logs do not contain large payload contents
optionally add allocation benchmark/test only if reliable
document that no raw payload string logging is performed by default
```

Do not add brittle memory assertions.

### LOG-011: Failure logs contain safe error context

Add:

```go
func TestFailureLogsContainSafeErrorContext(t *testing.T)
```

Expected:

```text
failure logs include safe context like target, uuid, rpc_id, error_code/stage
failure logs do not include raw payload/rendered commands/secrets
```

## Part B: Restart and Persistence Tests

TDD_SPEC section 19 requires RST-001 through RST-006.

Goal:

```text
restart should not cause duplicate apply if persisted state says UUID is already applied
corrupt state should not be blindly trusted
production state path should not be unsafe by default
```

### RST-001: Restart with persisted state does not reapply same UUID

Add:

```go
func TestRestartWithPersistedStateDoesNotReapplySameUUID(t *testing.T)
```

Expected:

```text
first service instance applies/saves UUID
new service instance uses same persisted state path/store
same UUID configure is handled
render/apply skipped
already_in_sync result published
```

Use real temp file state store if available. If only fake state store is available, simulate restart by creating new service with same persisted file path.

### RST-002: Restart with missing state reapplies config

P1.

Expected behavior must match spec:

```text
missing state means no applied UUID known
same desired config may apply again or reconcile safely
```

Add if behavior is clear. Otherwise document policy.

### RST-003: Restart with corrupt state fails safely

Add or map from Phase 2 state tests.

Expected:

```text
corrupt state is not trusted
service does not blindly skip apply due to corrupt state
safe error/recovery path occurs
```

### RST-004: Restart reads state before configure decision

Add or map:

```go
func TestRestartReadsStateBeforeConfigureDecision(t *testing.T)
```

Expected:

```text
state load/check happens before deciding already_in_sync or apply
```

### RST-005: Restart does not lose configured state when path persistent

P1.

Expected:

```text
state file in persistent temp dir survives new service/store instance
applied UUID is still known
```

### RST-006: Tmp state path not used for production by default

P1.

Expected:

```text
production/default config does not silently use /tmp for state unless explicit
```

If state path is not configured in this repo or belongs to deployment config, document as deferred/not applicable.

## Part C: Retry and Idempotency Tests

TDD_SPEC section 20 requires IDEMP-001 through IDEMP-008.

Many are already covered in Phases 3/4. Map existing tests where valid.

### IDEMP-001: Same UUID after success skips apply

Covered by Phase 3 if existing test proves it.

Map to:

```text
TestConfigureWorkflowRepeatedSameUUIDIsIdempotent
TestConfigureWorkflowAlreadyInSyncSkipsApply
```

### IDEMP-002: Same UUID after renderer failure retries

Covered by Phase 4 if existing test proves renderer failure does not checkpoint and retry renders again. If only partially covered, add explicit test.

### IDEMP-003: Same UUID after apply failure retries

Covered by Phase 4 if existing.

### IDEMP-004: Same UUID after state save failure retries

Covered by Phase 4 if existing.

### IDEMP-005: Duplicate configure events do not double apply after success

Add if not already explicit:

```go
func TestDuplicateConfigureEventsDoNotDoubleApplyAfterSuccess(t *testing.T)
```

Expected:

```text
same UUID submitted/handled twice after first success
apply call count remains 1
only one successful checkpoint
second result is already_in_sync or equivalent
```

### IDEMP-006: New UUID after old success applies again

Covered by Phase 3 if existing.

### IDEMP-007: Older UUID after newer applied does not rollback unexpectedly

P1.

This requires explicit product policy.

Options:

```text
if service has no ordering/version policy, document as deferred pending config ordering policy
if UUID order is not meaningful, document not applicable
```

Do not invent lexicographic UUID rollback policy without design decision.

### IDEMP-008: Retry does not publish duplicate success for same attempt

P1.

Add if easy:

```text
single successful attempt publishes one final success result
failure retry sequence does not publish duplicate success for same attempt
```

Avoid brittle assertions if result semantics are intentionally one result per Handle call.

## Part D: Concurrency and Race Tests

TDD_SPEC section 21 requires CONC-001 through CONC-006.

Goal:

```text
detect race conditions and unsafe state access before production
```

Important:

```text
go test -race ./... already runs in CI
```

Add deterministic concurrency tests only where current service/state design supports it.

### CONC-001: Concurrent configure events same UUID apply at most once

P1.

Add if service has concurrency protection or state store atomicity.

Expected:

```text
N goroutines handle same UUID
apply count <= 1
state valid
no panic
```

If current configure service has no single-flight/locking and this would expose a product behavior gap, document as deferred with recommendation.

### CONC-002: Concurrent configure events different UUIDs preserve ordering

P1.

This requires explicit ordering policy.

If no ordering policy exists, document deferred pending queue/serialization design.

### CONC-003: Configure and action overlap safely

Add if easy at service level:

```text
run configure and action concurrently with independent fakes
both complete
no panic
race detector should pass
```

### CONC-004: Concurrent state access no race

This is primarily covered by:

```bash
go test -race ./...
```

Add a focused state-store concurrent read/write test if real state store should be concurrency-safe.

### CONC-005: Burst configure events no panic/no race

P1.

Add a bounded burst test if deterministic.

Expected:

```text
many sequential or limited concurrent configure calls
no panic
final state valid
race detector passes
```

### CONC-006: Burst action events no panic/no race

P2.

Add only if low-risk, or document as P2 deferred.

## Part E: Large Payload and Lightweight Load Tests

TDD_SPEC section 22 requires PERF-001 through PERF-005.

Goal:

```text
lightweight sanity, not benchmarking
```

### PERF-001: Large config one thousand commands does not crash

Add:

```go
func TestLargeConfigOneThousandCommandsDoesNotCrash(t *testing.T)
```

Use fake renderer/apply or apply adapter with large rendered text.

Expected:

```text
no panic
result correct
apply receives full output
```

### PERF-002: Large payload does not cause unsafe logging allocation

Map to LOG-009/LOG-010 where appropriate.

### PERF-003: Rapid configure events complete without race

Map/add limited burst test.

### PERF-004: Large rendered output handled safely

Add:

```go
func TestLargeRenderedOutputHandledSafely(t *testing.T)
```

Expected:

```text
large rendered command text reaches apply backend intact
no crash
no log leak
```

### PERF-005: Large action payload rejected or handled safely

P2.

Add if action payload limits exist. Otherwise document current behavior:

```text
placeholder executor validates JSON but may not enforce size limit
size limit policy deferred
```

Do not invent arbitrary size limits unless design already specifies them.

## Coverage Documentation

Add:

```text
PROMPTS/TDD/PHASE8_COVERAGE.md
```

Map every item:

```text
LOG-001 through LOG-011
RST-001 through RST-006
IDEMP-001 through IDEMP-008
CONC-001 through CONC-006
PERF-001 through PERF-005
```

For each row include:

```text
ID
Status: Covered / Partially Covered / Deferred / Not Applicable
Test file
Test name or existing phase coverage
Notes
```

Also document CI mapping:

```text
go test ./...
go test -race ./...
go build ./...
go test -tags=integration ./...
existing smoke scripts
```

## CI/CD Requirement

Phase 8 should remain CI-friendly.

Do not add long-running or flaky tests.

The existing CI already runs:

```text
go test ./...
go test -race ./...
go build ./...
go test -tags=integration ./...
NATS smoke scripts
```

Update CI only if you add new build tags or a separate command.

If all Phase 8 tests are normal unit tests, no CI change is needed beyond verifying existing jobs pick them up.

If you add specific integration-tag tests for Phase 8, ensure CI runs them.

## Production Code Rules

Prefer tests first.

Production code changes are allowed only if needed to:

```text
prevent unsafe logging
make state persistence testable
fix idempotency/race bugs
add minimal locking if required by tests and design
make payload/command logging policy explicit
```

Do not add broad refactors.

Do not implement real VyOS.

Do not introduce long-running background workers.

## Commands To Run

After implementation, run:

```bash
go test ./...
go test -race ./...
go build ./...
go test -tags=integration ./...
```

Run existing smoke scripts if they are part of CI or easy locally.

## Final Codex Response Required

After implementation, summarize:

1. Files added.
2. Files modified.
3. Logging/security tests added or mapped.
4. Restart/persistence tests added or mapped.
5. Idempotency/retry tests added or mapped.
6. Concurrency/race tests added or mapped.
7. Large payload/load tests added or mapped.
8. Deferred/not-applicable items and why.
9. CI/CD changes, if any.
10. Commands run and results.
11. What remains for Phase 9.

## Acceptance Criteria

Phase 8 is complete when:

```text
[ ] LOG-001 through LOG-011 covered/mapped/deferred with reason
[ ] RST-001 through RST-006 covered/mapped/deferred with reason
[ ] IDEMP-001 through IDEMP-008 covered/mapped/deferred with reason
[ ] CONC-001 through CONC-006 covered/mapped/deferred with reason
[ ] PERF-001 through PERF-005 covered/mapped/deferred with reason
[ ] no raw payload/rendered commands/apply plans are logged by default
[ ] restart with persisted state avoids duplicate apply
[ ] corrupt state behavior remains safe
[ ] duplicate/retry idempotency is documented and tested
[ ] race detector passes
[ ] lightweight large payload/load sanity is covered
[ ] no real VyOS dependency introduced
[ ] tests remain CI-friendly
[ ] PHASE8_COVERAGE.md maps all Phase 8 items
[ ] go test ./... passes
[ ] go test -race ./... passes
[ ] go build ./... passes
[ ] go test -tags=integration ./... passes
```
