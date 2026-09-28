# Test execution probes

Temporary Go overlays substitute test files only. Production Go files and runtime settings remain unchanged. The baseline uses commit 543f0d4; the default deterministic suite runs serially with -count=1.

- seed: initialize immutable Git worktree/bare seeds once per test process, then copy them into independent per-case directories and reset each origin URL. No hard links, shared writable database, or global Git configuration.
- helper: move only the existing git-proxy child entrypoint into a small executable with identical fetch redirection/exit-code/stdout/stderr behavior. Real Git still executes every command. Worker, Orca, and PR helper modes remain unchanged.
- layered: retain the first full cross-project CommandService validation in TestTaskBatchRejectsDuplicateAndCrossProjectDependencies. Evaluate the remaining duplicate-contract/edge, self-cycle, missing-node, and old-successor combinations through the existing validateTaskBatchAgainstGraph function with verifyFiles=false; keep real SQLite snapshot and contract digest checks. Separate tests retain committed Git blob verification and command target drift checks.
- combined: all three changes. TestDiagnosticFixtureBatch measures 10 fresh fixture instances; TestDiagnosticSeedIsolation checks independent worktrees and remotes.

No proposal is accepted merely because an overlay compiles; comparison results and preserved behavior determine the final scope.
