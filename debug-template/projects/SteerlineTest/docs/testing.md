# Fixture testing

The catalogue is docs/testing/test-list.json. Plan test IDs are in the adjacent
task.tests.json. Keep definitions and arrangements consistent when editing tests.

For a registered marker check, run its exact command with
`-Out .steerrunner/artifacts/<unique-execution-id>` in the original Run workspace.
The committed .gitignore excludes this directory. The test reads the run-owned
input `.steerrunner/artifacts/fixture-input.txt`, compares its actual bytes with
`fixture-ok`, writes the original summary, then atomically publishes a managed
report and exits 0/1. A different input is a real negative assertion case.
The caller collects reports through Runner; an Agent must not manufacture results.
Only clean, unchanged commits qualify as current results. Keep historical failures.
