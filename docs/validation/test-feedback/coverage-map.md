# Fixture and test coverage map

No product test was deleted, merged, skipped by default, or reduced to a weaker
assertion in this change. The following existing protections still run under
the same test names; only the shared fixture preparation changed.

| Original use | Protected behavior | Current test or setup |
| --- | --- | --- |
| `TestControlCrashBoundariesNeverReplayClaimWorktreeOrWorker` and all eight crash-point subtests | Restart recovery does not replay a claim, worktree creation, worker launch, or terminal receipt | Same test and eight subtests; each receives a separate repository, database, runtime directory, and fake platform state |
| `TestBadHandoffsNeverClose` and its seven result variants | Missing, malformed, mismatched, failed, or off-worktree worker evidence cannot close a task | Same test and all seven variants; each gets an independent Git repository and remote |
| `TestInvalidCompletionReceiptsNeverComplete` and its five receipt variants | Invalid completion evidence cannot mark a task complete | Same test and all five variants; each gets a separate repository, runtime state, and fake completion backend |
| `TestCLIRejectsBeadsExecutionSource` | The CLI rejects the inactive execution source before writing runtime state | Same test, now using the shared immutable CLI executable built from the current test source |
| `TestDetachedServiceLaunchReportsJobRestriction`, `TestDetachedServiceSurvivesStarterExitAndStopsWorker`, `TestStartServiceSerializesSameConfigAndRepositoryLockRejectsOtherRunDir`, `TestStartServiceReportsPreflightFailureAndPendingHandshake`, and `TestRunCommandRecordsInactiveBeadsSourceFailureAndLog` | Child process identity, startup handshakes, service lifecycle, and CLI rejection behavior | Same tests; they reuse one read-only executable while their configs, logs, services, processes, and runtime directories remain test-specific |
| `TestWindowsServiceWindowRejectsMissingConsole`, `TestWindowsForegroundServiceWindowLifecycle`, visible-window acceptance tests, and `TestWindowsConcurrentVisibleServiceWindows` | Windows console visibility, service-window startup, stop/restart, and concurrent-window behavior | Same platform tests and existing acceptance gates; the shared executable is copied into each test's own installed launcher home where required |

`newFixture` now creates one immutable seed per `internal/steerline` test
process, copies the bare remote and worktree files into each test's temporary
directory, and changes only that copy's origin and web remotes. The read-only
seed is removed when the test process exits. Each repository and bare remote
has separate object files and refs; databases, runtime directories, HTTP
servers, workspaces, and fake backends remain per test. The added
`TestFixtureRepositoriesReuseSeedWithIndependentWritableState` verifies common
starting revisions and separate writable paths.

The original `newFixture` path made eight Git subprocess calls per test
fixture: initialize the worktree and bare remote, configure hooks and remotes,
then add, commit, and push the two seed files. The current path makes two Git
configuration calls per fixture and copies the immutable seed files. Seed
creation itself runs once per test process. The measured group uses 20
fixtures, so the per-fixture Git preparation falls from 160 calls to 40; seed
creation adds eight calls once per test process.

The CLI helper also previously ran `go build` for every caller. It now builds
once per test process and shares that immutable binary. Each test still starts
its own process when process behavior is part of the assertion. There are no
shared live services.
