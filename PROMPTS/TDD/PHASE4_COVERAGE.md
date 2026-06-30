# Phase 4 Configure Failure Coverage

This table maps `TDD_SPEC.md` configure failure handling cases to the current tests.

| ID | Status | Test file | Test name | Notes |
|---|---|---|---|---|
| FAIL-001 | Covered | `internal/configure/configure_failure_test.go` | `TestConfigureRendererFailureStopsBeforeApply` | Renderer failure stops before apply/state save and publishes failure. |
| FAIL-002 | Covered | `internal/configure/configure_failure_test.go` | `TestConfigureRendererFailurePreservesPreviousState` | Previous applied UUID remains current and new UUID is not saved. |
| FAIL-003 | Covered | `internal/configure/configure_failure_test.go` | `TestConfigureRendererFailurePublishesClearFailure` | Failure result uses `render_failed`, preserves correlation data, and keeps message safe. |
| FAIL-004 | Covered | `internal/configure/configure_failure_test.go` | `TestConfigureApplyFailureDoesNotSaveState` | Apply failure prevents state save and publishes `apply_failed`. |
| FAIL-005 | Covered | `internal/configure/configure_failure_test.go` | `TestConfigureApplyFailureAllowsRetrySameUUID` | Failed apply does not checkpoint; retry with same UUID applies and saves successfully. |
| FAIL-006 | Covered | `internal/configure/configure_failure_test.go` | `TestConfigureApplyFailurePreservesPreviousUUID` | Previous UUID remains current when apply fails for a new UUID. |
| FAIL-007 | Covered | `internal/configure/configure_failure_test.go` | `TestConfigureStateSaveFailureMarksConfigureFailed` | Apply succeeds, save fails, and configure is reported failed without success output. |
| FAIL-008 | Covered | `internal/configure/configure_failure_test.go` | `TestConfigureStateSaveFailureDoesNotPersistUUID` | Failed save is observable but does not update loadable fake state. |
| FAIL-009 | Covered | `internal/configure/configure_failure_test.go` | `TestConfigureStateSaveFailureRetriesSafely` | Retry after save failure renders/applies/saves again and then succeeds. |
| FAIL-010 | Covered | `internal/configure/configure_failure_test.go` | `TestConfigureFailureDoesNotPublishSuccess` | Renderer, apply, and state-save failures publish no success result/status. |
| FAIL-011 | Covered | `internal/configure/configure_failure_test.go` | `TestConfigureFailureIncludesCorrelationData` | Renderer, apply, and state-save failure results preserve target, UUID, and RPC ID. |
| FAIL-012 | Deferred | n/a | n/a | Panic recovery is not currently a configure service contract. Adding recovery changes production failure semantics and should be decided in a dedicated resilience phase. |
| FAIL-013 | Deferred | n/a | n/a | Context cancellation is currently dependency-driven via passed `context.Context`; deeper cancellation behavior belongs with lifecycle/dependency timeout work. |
| FAIL-014 | Deferred | n/a | n/a | The configure service has no owned timeout policy today; timeout behavior belongs to lifecycle/integration configuration or a future timeout phase. |

Cross-coverage:

| ID | Status | Test file | Test name | Notes |
|---|---|---|---|---|
| STATE-006 | Covered | `internal/configure/configure_failure_test.go` | `TestConfigureRendererFailureStopsBeforeApply` | State save is not called when renderer fails. |
| STATE-007 | Covered | `internal/configure/configure_failure_test.go` | `TestConfigureApplyFailureDoesNotSaveState` | State save is not called when apply fails. |
