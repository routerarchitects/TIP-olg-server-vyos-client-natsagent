# Phase 5 Action Workflow Coverage

This table maps `TDD_SPEC.md` action workflow cases to the current tests.

| ID | Status | Test file | Test name | Notes |
|---|---|---|---|---|
| ACT-001 | Covered | `internal/actions/action_workflow_test.go` | `TestActionTraceHappyPathPublishesReceivedExecutingCompleted` | Verifies received, executing, completed status order for trace success. |
| ACT-002 | Covered | `internal/actions/action_workflow_test.go` | `TestActionTraceHappyPathPublishesFinalResult` | Verifies success result fields and fake executor output propagation. |
| ACT-003 | Covered | `internal/actions/action_workflow_test.go` | `TestActionUnsupportedActionFails` | Enabled action without executor publishes `unsupported_action` and no success output. |
| ACT-004 | Covered | `internal/actions/action_workflow_test.go` | `TestActionDisabledActionFails` | Executor exists but action is not enabled; executor is not called. |
| ACT-005 | Covered | `internal/actions/action_workflow_test.go` | `TestActionExecutionFailurePublishesFailed` | Executor failure publishes received, executing, failed and `action_execute_failed`. |
| ACT-006 | Covered | `internal/actions/action_workflow_test.go` | `TestActionMissingRequiredPayloadFails` | Placeholder trace executor rejects empty and invalid JSON payloads with `invalid_action_payload`. |
| ACT-007 | Deferred | n/a | n/a | Action service does not own target authorization today. Runtime registers action handlers by configured target, so wrong-target behavior belongs to NATS subject routing/handler integration coverage. |
| ACT-008 | Covered | `internal/actions/action_workflow_test.go` | `TestActionStatusSequenceOrderIsStable` | Verifies deterministic success and executor-failure status order. |
| ACT-009 | Covered | `internal/actions/action_workflow_test.go` | `TestActionFailureDoesNotPublishCompleted` | Unsupported, disabled, executor failure, and invalid payload paths publish no completed/success output. |
| ACT-010 | Covered | `internal/actions/action_workflow_test.go` | `TestActionPreservesCorrelationData` | Success and failure results preserve target, action, RPC ID, and command type. |
| ACT-011 | Deferred | n/a | n/a | Cancellation is currently dependency/context driven. Full cancellation semantics belong to lifecycle/dependency behavior rather than Phase 5 service workflow. Existing lower-level tests cover canceled context behavior. |
| ACT-012 | Deferred | n/a | n/a | The action service has no owned timeout policy today. Timeout behavior belongs to lifecycle/integration configuration or a future timeout phase. |
