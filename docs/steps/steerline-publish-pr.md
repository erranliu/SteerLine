# SteerLine publish_pr step

Read the supplied `native-task/v1` context and perform only its `publish_pr` step.
Use the fixed task contract, workflow, execution, attempt, branch, workspace and
candidate revision from that context. Confirm that the worktree is clean and
`HEAD` equals the verified candidate revision before publishing.

Push the fixed worktree branch to `origin`. Use `gh` to find an existing PR for
that exact head branch against the configured base branch. Reuse it only when
its head revision still equals the verified candidate. Otherwise create one PR
for this task and query it again. Leave the PR open for human review and merge.

Return one JSON object matching the supplied step result schema. Include the
original project, task, workflow, execution and attempt IDs; a new result ID;
`step_id: "publish_pr"`, `succeeded: true`, the fixed input and candidate
revisions, the exact PR URL and `published_head`, prerequisite result IDs and
verified clean workspace evidence. The workspace evidence uses the supplied
project ID as `repository`, the actual worktree path and branch, the supplied
base commit, the candidate HEAD and a current UTC verification timestamp.
If any identity or publication fact is uncertain, report failure instead of
inventing a PR or merge result. Never merge the PR or mark the task complete.
