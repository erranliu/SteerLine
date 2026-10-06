# F1 configuration owner comparison

## Result

F1 changes which package owns the loaded configuration and how runtime dependencies reach execution. It does not change a frozen profile, claim identity, workspace/resource binding or the manual recovery gate. The fixed profile version remains `6`; `FixedExecutionProfile` keeps its existing fields and JSON shape. The app profile port resolves the same profile ID and accepted project/global config digest before a claim.

## Field and decision comparison

| Existing frozen fact | Current owner and projection | Equality / recovery evidence |
| --- | --- | --- |
| Profile ID, name, host, agent, implementation key/version, canonical implementation envelopes/display, policy and policy digest, publication settings, requirement snapshot, accepted config version and fixed digest | `app.Config` resolves the selected catalog entry and freezes it through the same `ExecutionDirectory`; execution receives only the selected value from `ProfilePort` | `TestAppConfigProjectsOwnerInputsAndSelectedProfile` serializes the fixed profile and verifies its digest, accepted config version, selected route and implementation payload. `FixedExecutionProfile` version and type fields are unchanged. |
| Project identity, workflow run, execution, attempt, route, target, branch/base revision and selected profile digest | The app resolves repository identity and builds `execution.Config`; execution claim and recovery code consume that value plus the fixed profile returned by the port | `TestTM04ExecutionWiring/claim_transaction_failure_and_original_receipt` verifies rollback and exact replay leave the same receipt and database snapshot. `intent_binding_and_two_original_turns` verifies a second turn preserves the original session/process. |
| Native process identity, session generation, question/dispatch identity, worktree path, original request acknowledgment and accepted answer | Reconstructed from committed task/execution records and the existing lifecycle adapter; host timers and app file verification travel separately as runtime dependencies | `TestTM04ExecutionWiring/original_request_ack_and_manual_recovery` restarts the service, confirms the original resource/session/dispatch/worktree, checks that restart does not cross the input gate, then settles the saved answer on that original binding without repeating a dispatch, reply or send. |
| Current-config change check and service claimant/heartbeat/shutdown grace | App `ConfigVerifier` and `RuntimeSettings`, outside fixed workflow state | `TestAppConfigProjectsOwnerInputsAndSelectedProfile` verifies host/task projections; `TestTM04ExecutionWiring` exercises claim and restart through app-built service wiring. The verifier checks the accepted project/global digest before claim evaluation. |

The projection-only normalizer used on query paths preserves canonical implementation envelopes for display and diagnostics even when a profile is unavailable. Admission continues to use strict normalization, including profile-selection and permission-evidence checks. This keeps the unsupported remote-profile decision unchanged and does not permit that profile to claim work.

## L3 decision

No frozen field or recovery authorization changed, so `recovery.waiting-merge-restart` was not selected. The targeted app profile projection and TM04 claim/receipt/turn/manual-recovery reports are linked in the current Plan and indexed in [test-mapping.json](../test-mapping.json). The fixed L1 report is recorded after the final rerun.
