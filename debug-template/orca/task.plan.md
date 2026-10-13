# Orca debug plan

Read the current branch, replace result.txt with the two required lines, and verify its bytes. Stage and commit only result.txt locally as the task artifact. Do not publish until the task input requests publication.

## Test plan

Read result.txt back and compare its UTF-8 bytes with the expected marker, branch name and LF line endings.
