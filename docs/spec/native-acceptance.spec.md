# SteerLine desktop64g native canary Spec

This is an isolated acceptance Task for the `erranliu/SteerLine` repository. Its PR targets only `codex/spec03-desktop64g-native-canary-base`, not `main`. Create exactly one file named `steerline-spec03-native-stage.txt` containing the UTF-8 bytes `spec03-supervised-stage-v1` followed by one LF byte. The implement step must commit only that file. Verify that `git rev-parse HEAD:steerline-spec03-native-stage.txt` equals Git blob `a7890c716cc3ad0cca79f8d7b68cffe125e6b106`. Review the single-file change, then publish one draft PR to the fixed canary base. Do not modify production source or create duplicate PRs.

Before editing during implement, ask the coordinator exactly one blocking business question through the active Orca Dispatch using the injected preamble's `ask --json` command and `--options yes,no`: "Should I create the exact native acceptance artifact?" Wait for the answer to that original question. Only `yes` authorizes the file and commit; for `no`, leave the worktree untouched and report the decision. After acting on `yes`, send the injected protocol's separate answer-application status proof bound to the original question, answer, Attempt and Execution before sending the implement result. Do not ask a replacement question after a timeout; resume the original ID.

The center verifies the exact candidate and PR head before merging, then closes the original native Task. This branch exists solely to measure the remote workflow in the SteerLine project identity; its canary artifact must not be merged to `main`.

## Acceptance

The original Task closes only after the five fixed steps succeed, the business answer has a bound application proof, the PR with the verified exact head is merged into the canary base, and every Dispatch has been released. A failed or unknown Attempt remains visible and blocks the next step until an eligible recovery is explicitly confirmed.
