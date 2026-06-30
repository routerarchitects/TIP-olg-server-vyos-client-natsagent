Read TDD_SPEC.md carefully before making any change.

We are implementing ONLY Phase 1 from TDD_SPEC.md:

Phase 1: Test Infrastructure

Goal:
Create clean, reusable, well-documented test infrastructure for future TDD phases of the VyOS Agent. This phase should not add the full test suite yet. It should only add the test helpers, fakes, recorders, fixtures, and documentation needed so later phases can implement configuration, state, workflow, failure, adapter, logging, and integration tests in a clean way.

Important:
This repository already has working code, unit tests, smoke scripts, and real-mode integration work. Do not rewrite production logic unless a very small testability change is absolutely required. Prefer test-only helpers.

Scope:
Implement test infrastructure only.

Do NOT implement:
- Phase 2 config/state tests
- configure workflow tests
- failure injection tests
- adapter tests
- logging/security tests
- mocked NATS integration tests
- restart/concurrency/load tests

Those will be handled in later phases.

Directory / organization requirements:
Keep everything readable, discoverable, and easy to review.

Create a proper test helper structure. Prefer one of these approaches depending on the repo layout:

Option A, preferred if suitable:
internal/testutil/

Option B, if test-only package is better:
internal/testutil/... with files ending in _test.go where needed

Do not scatter fake structs randomly across unrelated test files.

Suggested structure:

internal/testutil/
  README.md
  renderer_fake.go
  apply_fake.go
  state_fake.go
  status_recorder.go
  log_capture.go
  event_recorder.go
  fixtures.go

If the repo already has a test helper directory, use the existing convention instead of creating a conflicting structure.

Documentation requirement:
Add a simple README.md under the test helper directory explaining:

1. What this test infrastructure is for
2. Why fake components are needed
3. Difference between placeholder, fake, mock, and real backend
4. How later tests should use these helpers
5. Which TDD_SPEC.md phase this supports

Keep the language simple and practical.

Test helper requirements:

1. FakeRenderer

Purpose:
A controllable renderer test double used to simulate renderer behavior without calling the real renderer library.

It must support:
- successful render
- configured render error
- call counting
- input capture
- configurable output
- optional validation hook if useful
- safe use in tests

Expected behavior:
- Every call increments call count
- Every call records input
- If Err is set, return that error
- Otherwise return configured output
- If no output is configured, return a sensible minimal output suitable for tests

Expose helper methods where useful:
- Calls()
- LastInput()
- Inputs()
- Reset()

2. FakeApplyBackend

Purpose:
A controllable apply backend test double used to simulate apply behavior without requiring VyOS.

It must support:
- successful apply
- configured apply error
- optional prepare behavior if the production interface supports Prepare
- configured prepare error
- apply call counting
- prepare call counting
- input capture for Prepare and Apply
- optional mutation simulation during Prepare, if useful for later adapter tests

Expected behavior:
- Prepare call increments prepare count and records input
- Apply call increments apply count and records input
- If PrepareErr is set, Prepare returns that error
- If ApplyErr is set, Apply returns that error
- If configured plan/output exists, return it
- Should allow later tests to verify:
  - Prepare called once
  - Apply called once
  - Apply not called when Prepare fails
  - Apply input is still correct after Prepare

Expose helper methods where useful:
- PrepareCalls()
- ApplyCalls()
- LastPrepareInput()
- LastApplyInput()
- Reset()

3. FakeStateStore

Purpose:
A controllable state store test double used to simulate local state behavior.

It must support:
- loading existing state
- missing state/default state
- load error
- save success
- save error
- save call counting
- saved state capture

Expected behavior:
- Load returns configured state or configured error
- Save records the state and increments call count
- If SaveErr is set, Save returns that error
- Save failure should still be observable by tests
- Do not silently swallow errors

Expose helper methods where useful:
- SaveCalls()
- LastSavedState()
- SavedStates()
- Reset()

4. StatusRecorder / ResultRecorder

Purpose:
Capture statuses/results that would normally be published, so tests can assert outcomes.

It must support:
- recording success statuses
- recording failure statuses
- recording ordered status transitions
- searching for status by type/name
- retrieving last status/result
- asserting or exposing order for later tests

Expected future use cases:
- configure success published
- configure failure published
- already_in_sync published
- action received → executing → completed
- action received → executing → failed

Keep it generic enough for current repo types.

5. LogCapture

Purpose:
Capture logs in tests so later phases can verify no sensitive data is logged.

It must support:
- info/debug/warn/error capture if the repo logger interface supports levels
- storing log messages in memory
- simple Contains(text string) helper
- simple DoesNotContain(text string) helper if useful
- reset support

Expected future use cases:
- payload is not logged at info level
- rendered commands are not logged by default
- debug logs appear only when level is debug and explicit flag is enabled
- secrets are not logged

6. EventRecorder / OrderRecorder

Purpose:
Allow workflow tests to verify ordering.

It must support recording events like:
- "render"
- "apply"
- "state_save"
- "publish_success"
- "publish_failure"

Expected future use cases:
- state saved after apply
- apply not called after renderer failure
- failure published after error
- action status sequence order

Keep it simple:
- Record(name string)
- Events() []string
- Reset()

7. Fixtures

Add simple reusable fixtures only if useful.

Examples:
- minimal valid desired config
- minimal UUID
- minimal target
- large payload helper
- invalid payload helper

Do not overbuild fixtures in Phase 1.

Production code changes:
Avoid production code changes.

Only change production code if absolutely required to make existing interfaces testable. If you must change production code:
- keep changes minimal
- explain why the change is required
- do not alter runtime behavior
- preserve backward compatibility

Validation:
After implementing Phase 1:

1. Run:
go test ./...

2. If the repo supports race tests and it is not too slow, also run:
go test -race ./...

3. Do not require real VyOS.
4. Do not require external lab setup.
5. Existing smoke scripts must not be broken.

Expected output from you:
After implementation, provide a concise summary with:

1. Files added
2. Files modified
3. Which helpers were implemented
4. How each helper maps to TDD_SPEC.md Phase 1
5. Whether any production code was changed
6. Test command results
7. Any follow-up notes for Phase 2

Acceptance criteria for Phase 1:
Phase 1 is complete only if:

- A clear test helper directory exists
- README.md explains the purpose and usage of helpers
- FakeRenderer exists and supports success/failure/call capture
- FakeApplyBackend exists and supports success/failure/prepare/apply/call capture
- FakeStateStore exists and supports load/save failures and saved state capture
- StatusRecorder or equivalent exists
- LogCapture or equivalent exists
- EventRecorder or equivalent exists
- Helpers are simple, readable, and reusable
- Existing tests pass
- No real VyOS dependency is introduced
- No later phase tests are implemented yet

Important review requirement:
Keep code and documentation simple. This phase is meant to improve readability and visibility for future test development. The reviewer should be able to quickly understand where test infrastructure lives and how it will be used in later phases.
