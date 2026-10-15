param([switch]$Live, [string]$SteerRunnerExe = $env:STEERRUNNER_EXE, [switch]$IndependentProcess)
$ErrorActionPreference = 'Stop'
if (-not $IndependentProcess) {
    . (Join-Path $PSScriptRoot 'independent-test.ps1')
    $testArguments = @()
    if ($Live) { $testArguments += '-Live' }
    if ($SteerRunnerExe) { $testArguments += @('-SteerRunnerExe', $SteerRunnerExe) }
    $testBudget = if ($Live) { [TimeSpan]::FromMinutes(80) } else { [TimeSpan]::FromMinutes(20) }
    Invoke-IndependentTest -ScriptPath $PSCommandPath -TestArguments $testArguments -Timeout $testBudget
    return
}
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$generator = Join-Path $PSScriptRoot 'new-debug-release.ps1'
$testRoot = Join-Path 'C:\SteerlineDebug' (([string][char]0x4E2D + [char]0x6587) + ' test-' + [Guid]::NewGuid().ToString('N').Substring(0,12))
$oldHome = $env:STEERLINE_HOME
$oldConfig = $env:STEERLINE_CONFIG
$oldTestRoot = $env:STEERLINE_DEBUG_TEST_ROOT
$oldCodexHome = $env:CODEX_HOME
$oldRunnerExe = $env:STEERRUNNER_EXE
$oldSecondRoot = $env:STEERLINE_DEBUG_SECOND_TEST_ROOT
try {
    # Deliberate inherited pollution must not select installed state.
    $env:STEERLINE_HOME = 'C:\SteerlineRuntime'
    $env:STEERLINE_CONFIG = 'C:\SteerlineRuntime\etc\SteerlineTest.json'
    $defaultRelease = (& $generator -DebugRoot $testRoot -SteerRunnerExe $SteerRunnerExe | ConvertFrom-Json)
    $first = (& $generator -DebugRoot $testRoot -SteerRunnerExe $SteerRunnerExe -WebAddress 127.0.0.1:19747 | ConvertFrom-Json)
    $second = (& $generator -DebugRoot $testRoot -SteerRunnerExe $SteerRunnerExe -WebAddress 127.0.0.1:19744 -WebProjects 2 | ConvertFrom-Json)
    if ($first.root -eq $second.root) { throw 'Generator reused a release' }
    foreach ($release in @($defaultRelease, $first, $second)) {
        $manifest = Get-Content -LiteralPath (Join-Path $release.root 'manifest.json') -Raw | ConvertFrom-Json
        if ($manifest.status -ne 'ready' -or $manifest.default_scheme -ne 'live') { throw 'Generated package is not live by default' }
        $databases = @(Get-ChildItem -LiteralPath (Join-Path $release.root 'home\state') -Filter steerline.db -Recurse)
        foreach ($name in @('bin\steerline.exe','bin\steerline-web.exe','bin\debug-helper.exe','home\start.bat','home\stop.bat')) { if (-not (Test-Path -LiteralPath (Join-Path $release.root $name))) { throw "Missing package artifact $name" } }
        if (Test-Path -LiteralPath (Join-Path $release.root 'console.bat')) { throw 'Retired TUI launcher present' }
        if (-not $manifest.web_binary_sha256) { throw 'Missing Web build hash' }
        if ($databases.Count -ne 0) { throw 'Fresh generation must not initialize project databases' }
    }
    $firstIdentity = Get-Content -LiteralPath (Join-Path $first.root 'seed\SteerlineTest.json.bound-tasks.json') -Raw | ConvertFrom-Json
    $secondIdentity = Get-Content -LiteralPath (Join-Path $second.root 'seed\SteerlineTest.json.bound-tasks.json') -Raw | ConvertFrom-Json
    if ($firstIdentity.project_id -eq $secondIdentity.project_id) { throw 'Generated packages share database/project identity' }
    $env:STEERLINE_DEBUG_TEST_ROOT = $first.root
    $env:STEERLINE_DEBUG_SECOND_TEST_ROOT = $second.root
    Push-Location $repo
    try {
        & go test ./tests/e2e/internal/driver -run '^TestDebugRelease(Runtime|WebProjects)$' -count=1 -timeout=20m -v *> (Join-Path $first.root 'evidence\runtime-test.log')
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
        # The single real product acceptance chain lives in L2.
        $env:STEERLINE_HOME = $null
        $env:STEERLINE_CONFIG = $null
        $env:CODEX_HOME = $oldCodexHome
        $env:STEERRUNNER_EXE = $SteerRunnerExe
        Push-Location $repo
        try {
            & (Join-Path $PSScriptRoot 'test-l2.ps1') -SteerRunnerExe $SteerRunnerExe -IndependentProcess
        } finally { Pop-Location }
    }
    $result = [pscustomobject]@{ passed = $true; default_artifacts = $defaultRelease.root; controlled = $first.root; other = $second.root; live = [bool]$Live }
    [IO.File]::WriteAllText((Join-Path $first.root 'evidence\generation-test.json'), ($result | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
    $result | ConvertTo-Json
} finally {
    $env:STEERLINE_HOME = $oldHome
    $env:STEERLINE_CONFIG = $oldConfig
    $env:STEERLINE_DEBUG_TEST_ROOT = $oldTestRoot
    $env:CODEX_HOME = $oldCodexHome
    $env:STEERRUNNER_EXE = $oldRunnerExe
    $env:STEERLINE_DEBUG_SECOND_TEST_ROOT = $oldSecondRoot
}
