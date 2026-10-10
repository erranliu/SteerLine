param()
$ErrorActionPreference = 'Stop'
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
# The controlled gate and the real worker use separate newly generated packages.
& (Join-Path $PSScriptRoot 'test-debug-release.ps1')
if ($LASTEXITCODE -ne 0) { throw 'Independent controlled gate failed' }
$release = & (Join-Path $PSScriptRoot 'new-debug-release.ps1') | ConvertFrom-Json
$savedRoot = $env:STEERLINE_DEBUG_SANDBOX_ROOT
try {
    $env:STEERLINE_DEBUG_SANDBOX_ROOT = $release.root
    Push-Location $repo
    try {
        & go test ./tests/e2e/internal/driver -run '^TestDebugReleaseCodexSandbox$' -count=1 -timeout=30m -v *> (Join-Path $release.root 'evidence\sandbox-test.log')
        if ($LASTEXITCODE -ne 0) { throw "Real sandbox test failed: $($release.root)\evidence\sandbox-test.log" }
    } finally { Pop-Location }
    [pscustomobject]@{ passed = $true; live = $true; root = $release.root; evidence = (Join-Path $release.root 'evidence\sandbox-live.json') } | ConvertTo-Json
} finally { $env:STEERLINE_DEBUG_SANDBOX_ROOT = $savedRoot }
