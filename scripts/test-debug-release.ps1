param([switch]$Live, [string]$SteerRunnerExe = $env:STEERRUNNER_EXE)
$ErrorActionPreference = 'Stop'
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$generator = Join-Path $PSScriptRoot 'new-debug-release.ps1'
$testRoot = Join-Path 'C:\SteerlineDebug' (([string][char]0x4E2D + [char]0x6587) + ' test-' + [Guid]::NewGuid().ToString('N').Substring(0,12))
$oldHome = $env:STEERLINE_HOME
$oldConfig = $env:STEERLINE_CONFIG
$oldTestRoot = $env:STEERLINE_DEBUG_TEST_ROOT
$oldCodexHome = $env:CODEX_HOME
$oldSandboxRoot = $env:STEERLINE_DEBUG_SANDBOX_ROOT
try {
    # Deliberate inherited pollution must not select installed state.
    $env:STEERLINE_HOME = 'C:\SteerlineRuntime'
    $env:STEERLINE_CONFIG = 'C:\SteerlineRuntime\etc\SteerlineTest.json'
    $first = (& $generator -DebugRoot $testRoot -SteerRunnerExe $SteerRunnerExe | ConvertFrom-Json)
    $second = (& $generator -DebugRoot $testRoot -SteerRunnerExe $SteerRunnerExe | ConvertFrom-Json)
    if ($first.root -eq $second.root) { throw 'Generator reused a release' }
    foreach ($release in @($first, $second)) {
        $manifest = Get-Content -LiteralPath (Join-Path $release.root 'manifest.json') -Raw | ConvertFrom-Json
        if ($manifest.status -ne 'ready' -or $manifest.default_scheme -ne 'live') { throw 'Generated package is not live by default' }
        $databases = @(Get-ChildItem -LiteralPath (Join-Path $release.root 'home\state') -Filter steerline.db -Recurse)
        if ($databases.Count -ne 1) { throw 'Expected one independent database' }
    }
    $firstIdentity = Get-Content -LiteralPath (Join-Path $first.root 'evidence\initialization.json') -Raw | ConvertFrom-Json
    $secondIdentity = Get-Content -LiteralPath (Join-Path $second.root 'evidence\initialization.json') -Raw | ConvertFrom-Json
    if ($firstIdentity.validation.database_id -eq $secondIdentity.validation.database_id -or $firstIdentity.validation.project_id -eq $secondIdentity.validation.project_id) { throw 'Generated packages share database/project identity' }
    $env:STEERLINE_DEBUG_TEST_ROOT = $first.root
    Push-Location $repo
    try {
        & go test ./tests/e2e/internal/driver -run '^TestDebugReleaseRuntime$' -count=1 -timeout=12m -v *> (Join-Path $first.root 'evidence\runtime-test.log')
        if ($LASTEXITCODE -ne 0) { throw "Runtime tests failed: $($first.root)\evidence\runtime-test.log" }
    } finally { Pop-Location }
    $refused = $false
    try { & $generator -DebugRoot 'C:\SteerlineRuntime\debug' -SteerRunnerExe $SteerRunnerExe | Out-Null } catch { $refused = $true }
    if (-not $refused) { throw 'Installed output root was accepted' }
    $alias = Join-Path $testRoot 'linked-root'
    New-Item -ItemType Junction -Path $alias -Target $second.root | Out-Null
    $refused = $false
    try { & $generator -DebugRoot $alias -SteerRunnerExe $SteerRunnerExe | Out-Null } catch { $refused = $true }
    if (-not $refused) { throw 'Linked output root was accepted' }
    # Remove only the exact junction, never recursively follow its target.
    (Get-Item -LiteralPath $alias).Delete()
    $collisionRoot = Join-Path $testRoot 'collision'
    $now = [TimeZoneInfo]::ConvertTimeFromUtc([DateTime]::UtcNow, [TimeZoneInfo]::FindSystemTimeZoneById('China Standard Time'))
    # The checkout may advance between the earlier builds and this assertion.
    $collisionCommit = (& git -C $repo rev-parse HEAD).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Cannot resolve collision source commit' }
    $suffix = '-' + $collisionCommit.Substring(0, 8)
    if (-not [string]::IsNullOrWhiteSpace((& git -C $repo status --porcelain | Out-String))) { $suffix += '-dirty' }
    # Preoccupy the immediate timestamp window, so the real generator must refuse before building.
    foreach ($offset in 0..10) { New-Item -ItemType Directory -Path (Join-Path $collisionRoot ('dev-' + $now.AddSeconds($offset).ToString('yyyyMMdd-HHmmss') + $suffix)) -Force | Out-Null }
    $refused = $false
    try { & $generator -DebugRoot $collisionRoot -SteerRunnerExe $SteerRunnerExe | Out-Null } catch { $refused = $true }
    if (-not $refused) { throw 'Existing timestamp directory was accepted' }
    if ($Live) {
        # Exercise the actual Runner/agent workflow. Existing native login stays
        # external; this harness never exports tokens or publishes for a worker.
        $env:CODEX_HOME = $oldCodexHome
        $env:STEERLINE_DEBUG_SANDBOX_ROOT = $second.root
        Push-Location $repo
        try {
            & go test ./tests/e2e/internal/driver -run '^TestDebugReleaseCodexSandbox$' -count=1 -timeout=30m -v *> (Join-Path $second.root 'evidence\native-workflow-test.log')
            if ($LASTEXITCODE -ne 0) { throw "Native workflow failed; inspect $($second.root)\evidence\native-workflow-test.log" }
        } finally { Pop-Location }
    }
    $result = [pscustomobject]@{ passed = $true; controlled = $first.root; other = $second.root; live = [bool]$Live }
    [IO.File]::WriteAllText((Join-Path $first.root 'evidence\generation-test.json'), ($result | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
    $result | ConvertTo-Json
} finally {
    $env:STEERLINE_HOME = $oldHome
    $env:STEERLINE_CONFIG = $oldConfig
    $env:STEERLINE_DEBUG_TEST_ROOT = $oldTestRoot
    $env:CODEX_HOME = $oldCodexHome
    $env:STEERLINE_DEBUG_SANDBOX_ROOT = $oldSandboxRoot
}
