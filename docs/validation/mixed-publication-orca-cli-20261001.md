# Mixed publication profiles and Orca CLI validation

Date: 2026-10-01. Rebased onto `origin/main` at `37b8b0c`.

## Behavior verified

- A project can explicitly select `publication_policy.publisher=execution_profile`; local profiles specify `host_service`, Orca profiles specify `worker`. Execution snapshots freeze the resolved responsibility with the existing v4 requirements/provider evidence.
- CLI manual start supplies `-execution-profile`, its sampled digest and the selected task. Read-only repository identity resolution canonicalizes Git before hashing, matching service preflight.
- An original live Orca terminal with no detected agent remains pending inside its existing preparation deadline. Once Codex is identified, the original launch is bound without another terminal or Dispatch.
- Actual `text(JSON.stringify(result))` command wrappers qualify readiness using the exact command, native exit code, marker, receipt and settlement. `text(result.output)` only proves a current business command attempt; it cannot supply readiness exit evidence or turn arbitrary stdout into a native initialization failure.

## Directed checks

```powershell
go run ./cmd/test-feedback -out <validation-dir> -- ./cmd/steerline ./internal/steerline/orcaworker ./internal/steerline -count=1 -timeout=3m -run '^(TestCLIExecutionProfile.*|TestActual.*WrapperProvidesProbeEvidence|TestProjectedCommandOutputOnlyProvesBusinessAttempt|TestBusinessToolFactsRequireCurrentTaskAndActualCommand|TestProbeInitializationFailureCannotBeOverwrittenByRetry|TestProfileSelectionDigestMatchesServicePreflight|TestProfilePublication.*|TestWorkerEnvironmentWaitsForOriginalAgentDetection|TestWorkerEnvironmentUnidentifiedAgentKeepsOriginalDeadline)$'
```

All 20 directed tests passed on the final PR baseline (36.811 seconds wall time); the CLI binary also built successfully. No full test suite or automatic PR merge is required.

## Real CLI experiment

A separate runtime home `C:/SteerlineRuntime/cli-orca-debug-20261001` used the real `C:/Projects/SteerlineTest` repository, native Orca and Codex 0.159.2 with explicitly selected `unelevated` workspace sandbox. The previous elevated session could not initialize its helper (Windows cancellation 1223); that configuration is not claimed as passing.

Task 103, workflow `06099bc926ab4349c4387fecbbefc8b1`, completed one verified readiness Dispatch and two successful business Dispatches in the same Orca worker terminal. The service observed native settlement and entered `waiting_pr_merge` without an execution error. The second round also verified feedback continuation and ordinary Git HTTPS publication using the fixed Git executable, OpenSSL and private in-memory authentication from the existing GitHub login; no token/header was logged or committed. This transport was a diagnostic worker instruction, not a new automatic transport fallback.

[SteerlineTest PR #29](https://github.com/erranliu/SteerlineTest/pull/29) targets `main` and changes only `evidence/native-103.md`. The final local HEAD, remote task branch and PR head all matched `d79159a9dc3245aa616bb6a72acac779f8e397a1`; the tree was `dcc4c0e4ea5f8e22928e583061810daa88191f01`. The first business round used GitHub API publication after ordinary Schannel/helper errors; the second round verified the real Git push and aligned the branch with the local commit using an exact lease. The PR was left open for human merge, so this sample proves publication and continuation, not merged delivery. An existing untracked `debug.log` remained outside the PR.

The earlier task 101 diagnostic was discarded in the debug authority, and its exact worker/coordinator terminals were closed before task 103. The original runtime database was not rewritten or migrated.
