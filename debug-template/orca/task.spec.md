# SteerlineTest Orca single-terminal debug

Implement this task: write result.txt with exactly `SteerlineTest Orca debug result` and one trailing LF in UTF-8. Read it back, then stage and commit only result.txt locally. Use Git user.name=SteerLine Debug and user.email=debug@example.invalid for this commit if the host has no Git identity. Do not push or publish during implementation. Do not change other tracked files.

Do not publish as part of implementing this Spec. Only when a later input explicitly asks to push and create/reuse a PR, run `powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./publish.ps1`. Use the execution host's existing launcher, permissions and GitHub authentication; do not modify global settings, credentials or permissions.

## Acceptance

The original agent terminal completes implementation, report and publication on its assigned branch. The PR targets this debug environment's unique base; its head equals the committed candidate. A merge releases the successor. Keep summaries brief so the requested compact terminal result is complete.
