param([switch]$Live)
$ErrorActionPreference = 'Stop'
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$generator = Join-Path $PSScriptRoot 'new-debug-release.ps1'
$testRoot = Join-Path 'C:\SteerlineDebug' (([string][char]0x4E2D + [char]0x6587) + ' verification-' + [Guid]::NewGuid().ToString('N'))
$oldHome = $env:STEERLINE_HOME
$oldConfig = $env:STEERLINE_CONFIG
$oldTestRoot = $env:STEERLINE_DEBUG_TEST_ROOT
$oldCodexHome = $env:CODEX_HOME
try {
    # Deliberate inherited pollution must not select installed state.
    $env:STEERLINE_HOME = 'C:\SteerlineRuntime'
    $env:STEERLINE_CONFIG = 'C:\SteerlineRuntime\etc\SteerlineTest.json'
    $first = (& $generator -DebugRoot $testRoot | ConvertFrom-Json)
    $second = (& $generator -DebugRoot $testRoot | ConvertFrom-Json)
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
        & go test ./tests/e2e/internal/driver -run DebugRelease -count=1 -v *> (Join-Path $first.root 'evidence\runtime-test.log')
        if ($LASTEXITCODE -ne 0) { throw "Runtime tests failed: $($first.root)\evidence\runtime-test.log" }
    } finally { Pop-Location }
    $refused = $false
    try { & $generator -DebugRoot 'C:\SteerlineRuntime\debug' | Out-Null } catch { $refused = $true }
    if (-not $refused) { throw 'Installed output root was accepted' }
    $alias = Join-Path $testRoot 'linked-root'
    New-Item -ItemType Junction -Path $alias -Target $second.root | Out-Null
    $refused = $false
    try { & $generator -DebugRoot $alias | Out-Null } catch { $refused = $true }
    if (-not $refused) { throw 'Linked output root was accepted' }
    # Remove only the exact junction, never recursively follow its target.
    (Get-Item -LiteralPath $alias).Delete()
    $collisionRoot = Join-Path $testRoot 'collision'
    $now = [TimeZoneInfo]::ConvertTimeFromUtc([DateTime]::UtcNow, [TimeZoneInfo]::FindSystemTimeZoneById('China Standard Time'))
    $suffix = $first.release -replace '^dev-\d{8}-\d{6}', ''
    # Preoccupy the immediate timestamp window, so the real generator must refuse before building.
    foreach ($offset in 0..10) { New-Item -ItemType Directory -Path (Join-Path $collisionRoot ('dev-' + $now.AddSeconds($offset).ToString('yyyyMMdd-HHmmss') + $suffix)) -Force | Out-Null }
    $refused = $false
    try { & $generator -DebugRoot $collisionRoot | Out-Null } catch { $refused = $true }
    if (-not $refused) { throw 'Existing timestamp directory was accepted' }
    if ($Live) {
        $env:CODEX_HOME = $oldCodexHome
        $helper = Join-Path $second.root 'bin\debug-helper.exe'
        & $helper debug prepare
        if ($LASTEXITCODE -ne 0) { throw 'Live preparation failed' }
        $manifest = Get-Content -LiteralPath (Join-Path $second.root 'manifest.json') -Raw | ConvertFrom-Json
        $token = (Get-Content -LiteralPath (Join-Path $second.root 'home\github-token') -Raw).Trim()
        $savedGhToken = $env:GH_TOKEN
        $env:GH_TOKEN = $token
        $project = Join-Path $second.root 'projects\SteerlineTest'
        $base = $manifest.base_branch
        $head = $base.Replace('/base', '/verification')
        $pr = $null
        try {
            # Token stays in the environment and never appears in argv/logs.
            $env:GIT_CONFIG_COUNT = '1'
            $env:GIT_CONFIG_KEY_0 = 'http.https://github.com/erranliu/SteerlineTest.git.extraheader'
            $env:GIT_CONFIG_VALUE_0 = 'AUTHORIZATION: basic ' + [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes('x-access-token:' + $token))
            & git -C $project -c credential.helper= push origin "${base}:refs/heads/$base"
            if ($LASTEXITCODE -ne 0) { throw 'Base push failed' }
            & git -C $project checkout -b $head
            if ($LASTEXITCODE -ne 0) { throw 'Head checkout failed' }
            [IO.File]::WriteAllText((Join-Path $project 'verification.txt'), 'debug release live PR verification', [Text.UTF8Encoding]::new($false))
            & git -C $project add verification.txt
            & git -C $project commit -m 'Verify independent debug release'
            if ($LASTEXITCODE -ne 0) { throw 'Verification commit failed' }
            Push-Location $project
            $savedErrorPreference = $ErrorActionPreference
            try {
                $ErrorActionPreference = 'Continue'
                $publication = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File (Join-Path $project 'publish.ps1') 2> (Join-Path $second.root 'evidence\publish-stderr.log')
                $publicationExit = $LASTEXITCODE
            } finally { $ErrorActionPreference = $savedErrorPreference; Pop-Location }
            $prLine = $publication | Where-Object { $_ -match '^https://github.com/erranliu/SteerlineTest/pull/\d+$' } | Select-Object -Last 1
            if ($prLine) { $pr = $prLine.Trim() }
            if ($publicationExit -ne 0 -or -not $pr) { throw 'Live fixture publish.ps1 failed; see evidence/publish-stderr.log' }
            $proof = & gh pr view $pr --repo erranliu/SteerlineTest --json url,state,baseRefName,headRefName,headRefOid
            if ($LASTEXITCODE -ne 0) { throw 'Live PR query failed' }
            $facts = $proof | ConvertFrom-Json

            if ($facts.baseRefName -ne $base -or $facts.headRefName -ne $head -or $facts.state -ne 'OPEN') { throw 'Live PR identity mismatch' }
            [IO.File]::WriteAllText((Join-Path $second.root 'evidence\live-pr.json'), ($proof | Out-String), [Text.UTF8Encoding]::new($false))
        } finally {
            if ($pr) { & gh pr close $pr --repo erranliu/SteerlineTest; if ($LASTEXITCODE -ne 0) { throw 'PR cleanup failed' } }
            & git -C $project -c credential.helper= push origin --delete $head $base
            if ($LASTEXITCODE -ne 0) { throw 'Branch cleanup failed' }
            $env:GH_TOKEN = $savedGhToken
            Remove-Item Env:GIT_CONFIG_COUNT,Env:GIT_CONFIG_KEY_0,Env:GIT_CONFIG_VALUE_0 -ErrorAction SilentlyContinue
        }
    }
    $result = [pscustomobject]@{ passed = $true; controlled = $first.root; other = $second.root; live = [bool]$Live }
    [IO.File]::WriteAllText((Join-Path $first.root 'evidence\generation-test.json'), ($result | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
    $result | ConvertTo-Json
} finally {
    $env:STEERLINE_HOME = $oldHome
    $env:STEERLINE_CONFIG = $oldConfig
    $env:STEERLINE_DEBUG_TEST_ROOT = $oldTestRoot
    $env:CODEX_HOME = $oldCodexHome
}
