# SteerlineTest Orca single-terminal debug

Write result.txt in UTF-8 with exactly two lines: `SteerlineTest Orca debug result` and the current Git branch name, followed by a trailing LF. Replace an inherited result.txt so the artifact identifies this checkout. Read it back and verify its bytes. Do not change other tracked files.

## Acceptance

The file contains the marker and the exact current branch, encoded as UTF-8 with LF line endings.
