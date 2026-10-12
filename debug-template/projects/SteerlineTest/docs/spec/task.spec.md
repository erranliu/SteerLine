# SteerlineTest debug task

The first implementation input authorizes only these actions: run `powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./sandbox-check.ps1` in the normal sandbox with login=false and preserve its actual exit code and output. Read the current branch with `git branch --show-current`. Create or replace result.txt with exactly two lines: `SteerlineTest debug result` and that exact branch name, followed by a newline. Verify, stage and commit only result.txt locally, then stop and report the probe and commit result. Always replace an inherited result.txt; it belongs to the preceding task. Task IDs and workflow run IDs are not supplied by the implementation input and must not be guessed.

Do not invoke publish.ps1, push or create a PR during implementation. Do not advance yourself to another stage. A separate report input asks only for a JSON summary of the preceding implementation result; use no tools and make no changes.

Only after a separate later input explicitly asks to push and create or reuse a PR, run `powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./publish.ps1`. It publishes only the current checked-out candidate to erranliu/SteerlineTest against this environment's dedicated base, `__BASE__`; return its exact verified URL in the required native final JSON result contract. If this fixed publication command fails due to sandbox process/credential-helper/pipe restrictions, request native automatic review for the same command and worktree; the script queries an existing matching PR before retrying publication. Do not change files or permission settings to work around the failure, and do not retry a denied review.

## Acceptance

The assigned candidate contains exactly the marker and its actual branch, is committed, published as a PR against this environment's base, and delivered after that PR merges. Do not modify other tracked files. Each task must use its own branch, worktree and native session.

The sandbox-check.ps1 probe must pass without escalation: native PowerShell, writable task-owned TEMP/TMP, disabled automatic Git maintenance, and a minimal fixture check. Publication must use the real Git credential helper and report any automatic review honestly. No native viewer or manual folder-trust action is required.
