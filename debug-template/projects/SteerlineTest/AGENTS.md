# SteerlineTest debug fixture

Only implement the assigned task in its assigned worktree. For Runner readiness, execute only its exact command and preserve the actual exit code and output. Use login=false for shell commands.

Read the original bound Spec and Plan files without reconstructing their bytes through pipelines. Implementation only changes, verifies and commits result.txt locally. Do not push or run publish.ps1 until a separate publication input. The report input summarizes supplied facts without tools or changes.

Publication uses publish.ps1 and returns its verified PR URL. Never push to main or another environment's base, copy credentials or change native permissions. Preserve failures as blockers. Read the actual branch instead of inferring identity from another task's result.
