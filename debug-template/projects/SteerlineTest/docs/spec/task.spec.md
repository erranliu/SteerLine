# SteerlineTest debug task

The implementation input authorizes only the assigned task. Read the current branch with `git branch --show-current`. Create or replace result.txt with exactly two lines: `SteerlineTest debug result` and that exact branch name, followed by a newline. Verify, stage and commit only result.txt locally, then stop and report the actual result. Replace an inherited result.txt; it belongs to a preceding task. Do not guess task or run IDs.

Do not invoke publish.ps1, push or create a PR during implementation. A separate report input asks only for a JSON summary of the preceding implementation result; use no tools and make no changes.

When a later publication input explicitly asks to push and create or reuse a PR, run `powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./publish.ps1`. It targets only erranliu/SteerlineTest against this environment's dedicated base `__BASE__`. Return its verified PR URL in the required final result contract. Preserve real failures as blockers; do not copy credentials or change login/permission settings.

## Acceptance

The committed result.txt contains exactly the marker and current branch. The real published PR targets this environment's base and changes only result.txt. After that PR merges, SteerLine records one delivery and releases the successor without starting it automatically. Native sandbox/permissions, sessions, usage and recovery are tested by SteerRunner.
