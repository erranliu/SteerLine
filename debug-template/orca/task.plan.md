# Orca debug plan

Write and verify result.txt, commit only that file, then stop and report. Do not publish until a later input explicitly asks for publication; that input may use publish.ps1. Follow each short worker input in the same original terminal.

## Test plan

The independent debug driver checks the original terminal, compact replies, staged workflow, candidate/PR identity and merged successor. Test the same contract locally and through the paired Orca environment; no callback or extra terminal is required.
