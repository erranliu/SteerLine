# Orca debug plan

Write and verify result.txt, commit only that file, then report without writes. Publish once using publish.ps1 and return the exact verified PR URL. Follow each short worker input in the same original terminal.

## Test plan

The independent debug driver checks the original terminal, compact replies, staged workflow, candidate/PR identity and merged successor. Test the same contract locally and through the paired Orca environment; no callback or extra terminal is required.
