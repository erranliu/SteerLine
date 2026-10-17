# Independent Orca debug repository

Only the current task input authorizes Git and publication actions. Follow the Spec/Plan for the requested artifact. Do not push or create a PR until an input explicitly asks for publication. Commit with `git -c user.name="SteerLine Debug" -c user.email=debug@example.invalid commit ...`; these command-scoped settings work without changing the shared repository config. Do not write Git identity with `git config --local` or `git config --global`. Keep replies brief.

Use the assigned branch and publish only to the dedicated debug base named in the task input, using ordinary Git and gh commands with the prepared authentication. Do not invoke publish.ps1 or change PowerShell execution policy. Never push main or create another agent/terminal. Permissions and GitHub credentials belong to the execution host; do not modify global settings.
