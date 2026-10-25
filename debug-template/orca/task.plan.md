# Orca debug plan

Read the current branch, replace result.txt with the two required lines, and verify its bytes. Stage and commit only result.txt locally as the task artifact, using `git -c user.name="SteerLine Debug" -c user.email=debug@example.invalid commit ...` without writing shared Git configuration. Do not publish until the task input requests publication.

When a later input requests publication, use ordinary Git and gh commands to push the assigned head and create or reuse its PR against the dedicated debug base. Do not run publish.ps1 or change execution policy.

## Test plan

Read result.txt back and compare its UTF-8 bytes with the expected marker, branch name and LF line endings.

No automated tests from the project catalogue are required for this isolated fixture. Verify the file bytes and local commit as described above; the caller's live acceptance separately checks waiting, recovery and resource settlement.
