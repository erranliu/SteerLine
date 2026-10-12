# Independent Orca debug repository

Only the current worker input and the task's Spec/Plan authorize work. The first input is implementation only: change result.txt, verify and commit it, then stop and report. Do not run publish.ps1, push or create a PR until a separate later input explicitly asks to push and publish. The publication_policy fact identifies the publisher; it is not an instruction to publish during implementation.

Use the assigned branch and publish only to the dedicated debug base through publish.ps1. Never push main or create another agent/terminal. Permissions and GitHub credentials belong to the execution host; do not modify global settings.
