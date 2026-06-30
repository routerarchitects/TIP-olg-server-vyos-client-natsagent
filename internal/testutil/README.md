# Test Helpers

This package contains reusable test infrastructure for the VyOS NATS agent TDD hardening work.

It supports Phase 1 from `TDD_SPEC.md`: build fakes, recorders, fixtures, and log capture before adding the later behavior tests.

## Why These Helpers Exist

The agent has several paths that should be tested without a real VyOS device:

- rendering desired config,
- applying rendered commands,
- loading and saving local state,
- publishing status and result messages,
- checking log safety,
- verifying operation order.

The helpers in this package make those behaviors controllable and observable. Later test phases can inject errors, count calls, inspect inputs, and assert ordering without depending on NATS, JetStream, or VyOS.

## Placeholder, Fake, Mock, And Real

- Placeholder implementations are the safe runtime defaults used by local development and CI. They mostly return success and are intentionally simple.
- Fakes are controllable in-memory test implementations. They can return success, return configured errors, capture inputs, and count calls.
- Mocks or spies are tests that assert specific calls or behavior. These helpers can be used as spies by checking their recorded calls and inputs.
- Real backends call the actual renderer/apply libraries and, for apply, may require a VyOS-capable environment.

Phase 1 does not replace or change placeholder or real production behavior. It only adds helper code for tests.

## How To Use

Use `FakeRenderer` when testing configure-service behavior without calling the real renderer.

Use `FakeApplyBackend` when testing `applyvyos.Adapter` or mocked real-mode wiring. It implements the VyOS apply backend shape, including optional `Prepare`.

Use `FakeStateStore` to simulate missing state, existing applied UUIDs, load errors, and save failures.

Use `StatusRecorder`, `ResultRecorder`, or `StatusResultRecorder` to capture published envelopes and assert ordered transitions.

Use `LogCapture` as an `agentcore.Logger` when later tests need to check that payloads, rendered commands, apply plans, or secrets are not logged.

Use `EventRecorder` or `OrderRecorder` to record high-level workflow events such as `render`, `apply`, `state_save`, `publish_success`, and `publish_failure`.

Use fixtures for minimal target, UUID, placeholder-only desired config, minimally renderable desired config, rendered output, invalid payloads, and large payloads. Keep test-specific payload details in the tests that need them.

## Scope

These helpers are intentionally small. Phase 1 should not add config/state/workflow/failure/adapter/security/concurrency test cases. Those belong to later phases in `TDD_SPEC.md`.
