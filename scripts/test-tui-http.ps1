param([string]$DebugRoot = 'C:\SteerlineDebug', [Parameter(Mandatory=$true)][string]$OutDir)
$ErrorActionPreference = 'Stop'
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$evidenceRoot = [IO.Path]::GetFullPath($OutDir)
if (Test-Path -LiteralPath $evidenceRoot) { throw 'Evidence directory must be new' }
if ($evidenceRoot.StartsWith('C:\SteerlineRuntime', [StringComparison]::OrdinalIgnoreCase)) { throw 'Installed runtime is not a test target' }
New-Item -ItemType Directory -Path $evidenceRoot | Out-Null
$previousRoot = $env:STEERLINE_TUI_HTTP_TEST_ROOT
$previousEvidence = $env:STEERLINE_TUI_HTTP_EVIDENCE
try {
    $release = (& (Join-Path $PSScriptRoot 'new-debug-release.ps1') -DebugRoot $DebugRoot | ConvertFrom-Json)
    if (-not $release.root) { throw 'Debug generator returned no package' }
    $env:STEERLINE_TUI_HTTP_TEST_ROOT = $release.root
    $env:STEERLINE_TUI_HTTP_EVIDENCE = $evidenceRoot
    Push-Location $repoRoot
    try {
        & go test ./tests/e2e/internal/driver -run '^TestTUIHTTPRuntime$' -count=1 -v *> (Join-Path $evidenceRoot 'runtime.log')
        if ($LASTEXITCODE -ne 0) { throw ('TUI HTTP runtime failed; see ' + (Join-Path $evidenceRoot 'runtime.log')) }
    } finally { Pop-Location }
    $summaryPath = Join-Path $evidenceRoot 'summary.json'
    if (-not (Test-Path -LiteralPath $summaryPath)) { throw 'Runtime test produced no summary' }
    $summary = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json
    if (-not $summary.passed -or -not $summary.complete -or -not $summary.cleaned) { throw 'Incomplete runtime evidence' }
    Write-Output $summaryPath
} finally {
    $env:STEERLINE_TUI_HTTP_TEST_ROOT = $previousRoot
    $env:STEERLINE_TUI_HTTP_EVIDENCE = $previousEvidence
}
