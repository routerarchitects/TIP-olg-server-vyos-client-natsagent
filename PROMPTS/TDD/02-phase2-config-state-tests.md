# Phase 2 Codex Prompt: Configuration and State Unit Tests

Read these files carefully before making any change:

1. `TDD_SPEC.md`
2. `PROMPTS/TDD/01-phase1-testinfrastructure.md`
3. Phase 1 test helper code that already exists in the repo
4. Existing config/state packages and their current tests
5. `README.md` and any config examples

We are implementing ONLY Phase 2 from `TDD_SPEC.md`:

## Phase 2: Configuration and State Unit Tests

## Goal

Add focused unit tests for:

1. Configuration loading
2. Configuration validation
3. Default overlay behavior
4. YAML-to-runtime config conversion
5. State loading
6. State saving
7. State corruption/error handling

This phase proves that the agent starts from a correct, validated configuration and handles local state safely.

This phase should NOT implement configure workflow tests yet.

---

## Strict Scope

Implement only Phase 2.

Do NOT implement:

- Configure workflow tests
- Renderer/apply workflow tests
- Failure injection for configure service
- Adapter-level tests
- Logging/security tests
- Mocked NATS integration tests
- Restart/reconciliation tests
- Concurrency/load tests
- Real VyOS tests

Those belong to later phases.

---

## Important Production-Code Rule

Do not modify production behavior unless absolutely required for testability.

Do NOT change:

- real-mode renderer/apply behavior
- `renderervyos.Adapter` production behavior
- `applyvyos.Adapter` production behavior
- real mode wiring
- existing placeholder runtime behavior
- production config semantics

If a tiny production-code change is unavoidable:

1. Keep it minimal.
2. Preserve runtime behavior.
3. Explain why it was needed.
4. Show existing tests still pass.

---

## Directory and Organization Requirements

Keep everything readable and discoverable.

Use existing package conventions if they already exist.

Prefer test files near the code they validate, for example:

```text
internal/config/
  config_test.go
  validation_test.go
  defaults_test.go
  conversion_test.go

internal/state/
  store_test.go
  corruption_test.go
  write_test.go
```

If the repo uses a different structure, follow the existing convention.

Use Phase 1 helpers from the test helper directory where useful.

Do not duplicate helper logic inside individual test files.

---

# Part A: Configuration Tests

Target area:

```text
internal/config/*
```

or the equivalent config package in the repo.

## A1. Config Load Tests

Add tests for:

### `TestConfigLoadValidYAMLReturnsSuccess`

Purpose:
Validate that a valid YAML config loads successfully.

Expected:
- no error
- required fields are populated
- defaults are applied where needed

---

### `TestConfigLoadMissingFileReturnsError`

Purpose:
Validate missing config file behavior.

Expected:
- error returned
- error message is clear enough for troubleshooting
- no panic

---

### `TestConfigLoadInvalidYAMLReturnsError`

Purpose:
Invalid YAML must fail during parsing.

Expected:
- parse/load error returned
- no runtime fallback to unsafe defaults
- no panic

---

### `TestConfigLoadPartialYAMLAppliesDefaults`

Purpose:
Partial config should still produce a usable config with defaults.

Expected:
- missing optional fields are defaulted
- explicitly provided fields remain unchanged

---

## A2. Config Validation Tests

Add tests for:

### `TestConfigInvalidNATSConfigFailsValidation`

Purpose:
Invalid NATS settings should fail validation.

Examples:
- empty URL if URL is required
- malformed URL
- missing bucket/stream if required by config model

Expected:
- validation error
- no agent startup attempt

---

### `TestConfigInvalidSubjectPatternFailsValidation`

Purpose:
Malformed subject patterns should fail early.

Expected:
- validation error
- clear failure reason

---

### `TestConfigUnsupportedActionFailsValidation`

Purpose:
Unsupported actions must not be accepted silently.

Expected:
- validation error
- unsupported action name is identifiable in error if possible

---

### `TestConfigInvalidConfigureModeFailsAtParseLevel`

Purpose:
Reviewer explicitly requested this.

Invalid `agent.configure.mode` must fail at config parse/validation level, not later during runtime wiring.

Example invalid values:
- `invalid`
- `real-mode`
- empty string if empty is not valid after defaults
- random unsupported value

Expected:
- config load/validation fails
- runtime engine creation is not reached

---

## A3. Default Overlay Tests

Add tests for:

### `TestConfigDefaultConfigureModeIsPlaceholder`

Purpose:
Prevent implicit behavior drift.

If configure mode is omitted, final loaded config should explicitly resolve to `placeholder`.

Expected:
- final config contains placeholder mode
- no hidden/implicit mode selection

---

### `TestConfigYAMLOverridesDefaultsCorrectly`

Purpose:
YAML values must override defaults.

Expected:
- values provided in YAML are preserved
- defaults only fill missing fields

---

### `TestConfigDefaultsAreNotReappliedAfterOverlay`

Purpose:
Prevent default overlay bug.

If a YAML value overrides a default, the default must not be applied again later and overwrite the YAML value.

Expected:
- explicit YAML value wins
- final config is stable

---

## A4. Conversion Tests

Add tests for:

### `TestConfigConvertsToAgentCoreConfigCorrectly`

Purpose:
Validate mapping from loaded YAML config to `agentcore.Config` or equivalent runtime/shared-library config.

Expected:
- NATS fields mapped correctly
- subject fields mapped correctly
- target/agent identity mapped correctly
- action settings mapped correctly
- no important config value is dropped

---

### `TestConfigDebugFlagsDoNotChangeEngineSelection`

Purpose:
Reviewer explicitly requested this.

Debug flags should affect logging/debug visibility only, not renderer/apply mode selection.

Expected:
- placeholder mode stays placeholder when debug flags are enabled
- real mode stays real when debug flags are enabled
- debug options do not change engine selection

---

## A5. Optional Wiring Assertion Tests

Only add these if they naturally fit the config package or existing tests.

Do not force production refactors.

### `TestConfigPlaceholderModeSelectsPlaceholderAdapters`

Expected:
- placeholder mode wires placeholder renderer/apply

### `TestConfigRealModeSelectsRealAdapters`

Expected:
- real mode wires real renderer/apply adapters

If these tests belong more naturally in a later wiring phase, document them as deferred instead of forcing them into Phase 2.

---

# Part B: State Management Tests

Target area:

```text
internal/state/*
```

or the equivalent state package in the repo.

Use temporary directories/files. Do not write to real production paths.

## B1. State Load Tests

Add tests for:

### `TestStateLoadValidFileReturnsState`

Purpose:
Valid state file should load correctly.

Expected:
- no error
- UUID/target/metadata match file content

---

### `TestStateLoadMissingFileReturnsDefaultState`

Purpose:
First run behavior.

Expected:
- missing state file does not panic
- default/empty state returned if that is the intended behavior
- behavior matches README/spec

---

### `TestStateLoadCorruptJSONFailsSafely`

Purpose:
Reviewer explicitly identified this gap.

Corrupt state must not be blindly trusted.

Expected:
- error returned OR safe recovery behavior if explicitly designed
- no silent success with invalid state
- no unsafe apply decision based on corrupt state

---

### `TestStateLoadInvalidUUIDFailsSafely`

Purpose:
Invalid state content should not be treated as valid checkpoint.

Expected:
- error or safe invalid-state behavior
- invalid UUID is not accepted as applied state

---

## B2. State Write Tests

Add tests for:

### `TestStateWriteValidStatePersistsUUID`

Purpose:
Successful state save should persist applied UUID.

Expected:
- file is written
- reloading file returns same UUID
- required metadata is preserved

---

### `TestStateSaveFailureReturnsError`

Purpose:
Write failures must be visible.

How to simulate:
- invalid path
- permission-restricted directory if reliable
- fake state store from Phase 1 if production store is hard to force

Expected:
- error returned
- error not swallowed

---

### `TestStateSaveDoesNotCreatePartialValidStateOnFailure`

Purpose:
Avoid false checkpoint.

Expected:
- failed save does not leave a valid new UUID checkpoint
- old state remains intact if applicable

Add this only if it is practical with current implementation. If not practical, document as deferred for a later atomic-write improvement.

---

## B3. State Consistency Tests

Add tests for:

### `TestStateReflectsLastAppliedUUIDOnly`

Purpose:
State should represent the last successfully applied config UUID.

Expected:
- save UUID1
- save UUID2
- load returns UUID2

---

### `TestStateNoWriteWhenSaveInputInvalid`

Purpose:
Invalid state input should not create a valid state file.

Expected:
- error returned
- no valid checkpoint written

Add this only if the state model has input validation.

---

## Documentation Requirement

Update or add simple test documentation only if useful.

Possible locations:

```text
internal/config/README.md
internal/state/README.md
internal/testutil/README.md
```

Do not over-document. Keep it simple.

The documentation should explain:

- these are Phase 2 tests from `TDD_SPEC.md`
- config tests validate safe startup configuration
- state tests validate safe checkpoint behavior
- later phases will test workflow/failure paths using these foundations

---

## Test Style Requirements

- Prefer table-driven tests where helpful.
- Use clear test names.
- Keep each test focused on one behavior.
- Use temporary files/directories for state tests.
- Avoid sleeps/time-based tests unless absolutely necessary.
- Do not require real NATS.
- Do not require real VyOS.
- Do not require external lab setup.
- Do not duplicate Phase 1 helpers.

---

## Commands to Run

After implementation, run:

```bash
go test ./...
```

If practical, also run:

```bash
go test -race ./...
```

Do not require smoke scripts for this phase unless they already run quickly and reliably.

---

## Expected Codex Summary After Implementation

After making changes, summarize:

1. Files added
2. Files modified
3. Config test cases implemented
4. State test cases implemented
5. TDD_SPEC.md test IDs covered, if available
6. Any test cases deferred and why
7. Whether any production code changed
8. `go test ./...` result
9. `go test -race ./...` result, if run
10. What is ready for Phase 3

---

## Acceptance Criteria

Phase 2 is complete only if:

- Valid YAML config load is tested.
- Missing config file is tested.
- Invalid YAML is tested.
- Partial YAML default behavior is tested.
- Invalid NATS config validation is tested where applicable.
- Invalid subject validation is tested where applicable.
- Unsupported action validation is tested where applicable.
- Invalid `agent.configure.mode` fails early.
- Default configure mode explicitly resolves to placeholder.
- YAML overrides defaults correctly.
- Defaults are not incorrectly re-applied after YAML overlay.
- YAML/config conversion to runtime config is tested.
- Debug flags do not change engine selection.
- Valid state file load is tested.
- Missing state file behavior is tested.
- Corrupt JSON state behavior is tested.
- Invalid UUID state behavior is tested where applicable.
- State save success is tested.
- State save failure is tested.
- State consistency is tested.
- Existing tests still pass.
- No real VyOS dependency is introduced.
- No later-phase workflow tests are implemented yet.
