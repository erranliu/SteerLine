# SteerlineTest debug task

In implement.execute run `powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./sandbox-check.ps1` in the normal sandbox with login=false. Preserve its actual exit code and output. Then create or update result.txt with `SteerlineTest debug result`, the fixed task ID and workflow_run_id, each on its own line. Stage and commit only that file locally. Do not push or create a PR during implementation. In implement.report summarize the supplied evidence without changes. In publish run `powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./publish.ps1`; return its exact verified URL in the required native final JSON result contract. If this fixed publication command fails due to sandbox process/credential-helper/pipe restrictions, request native automatic review for the same command and worktree; the script queries an existing matching PR before retrying publication. Do not change files or permission settings to work around the failure, and do not retry a denied review.

## Acceptance

The assigned candidate is committed, published as a PR against this environment's base, and delivered after that PR merges. Do not modify other tracked files.

The sandbox-check.ps1 probe must pass without escalation: native PowerShell, writable task-owned TEMP/TMP, disabled automatic Git maintenance, and a minimal fixture check. Publication must use the real Git credential helper and report any automatic review honestly. No native viewer or manual folder-trust action is required.
