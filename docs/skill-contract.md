# Native project step contract

Native projects use `workflow_definition: minimal-v1` and committed `project_steps`. Every step receives a JSON object whose root schema is `native-task/v1`; its `step_input` contains the current step inputs and prerequisite results. The top level binds the fixed project, task contract and digest, workflow run, execution, attempt, repository, work branch and base commit.

The workflow runs `implement`, deterministic `verify`, optional `review`, and `publish_pr`. Each entry must exist as a regular file at the pinned base commit. Preflight scans committed entries and rejects Beads environment use, `bd`/`bv`, legacy start/finish markers, and old completion callbacks. Native step environment contracts cannot inject Beads variables.

Each worker performs only its assigned step and returns one `steerline/minimal-step-result/v1` JSON receipt bound to the project, task, workflow run, execution, attempt and step. It does not claim, retry, close or unlock tasks, write the center database, dispatch another step, merge a PR, or invoke a completion callback. Required verification checks must report the candidate commit they actually checked. Review returns a verdict for that same commit. `publish_pr` returns the precise PR URL, published head and target branch; the center then verifies the platform identity and merge fact.

New native projects start in `manual` mode and paused. The user explicitly resumes dispatch through the service control protocol; the center then uses the same transactional claim path for automatic and manual execution. Human PR merge remains the delivery gate.

Local and Orca native project steps use the same fixed task and step context on their target workspaces. The legacy start/finish and Beads interfaces are not native project steps.

## Supervised remote step boundary

An Orca supervised worker executes only the center's current `minimal-v1` step. It returns the local project/task/workflow/execution/attempt identity from Step Input; Orca taskId and dispatchId belong only to the outer message. A business question uses `orca orchestration ask --json` so the original question ID is retained, and application of an answer is reported separately from reply acceptance. The worker's `worker_done` settles only its Dispatch. Verify, review, PR merge and task closure remain center gates. A structured status sent through `--payload` cannot also use separate task/dispatch payload flags. See [Spec 03](spec/03-remote-execution-and-viewer.md) for the tested PowerShell 7 path and remaining capability gates.
