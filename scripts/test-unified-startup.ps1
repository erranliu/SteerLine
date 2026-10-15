param(
    [Parameter(Mandatory=$true)][string]$Package,
    [switch]$Simulate,
    [ValidateSet('unified-startup','query-cache')][string]$Scenario='unified-startup'
)
$ErrorActionPreference='Stop'
if (-not $Simulate) { throw 'Controlled verification requires explicit -Simulate; package defaults remain live.' }
$taskPackage=[IO.Path]::GetFullPath($Package).TrimEnd('\')
if (-not $taskPackage.StartsWith('C:\SteerlineDebug\',[StringComparison]::OrdinalIgnoreCase)) { throw 'Package must stay under C:\SteerlineDebug' }
$taskCursor=$taskPackage
while ($taskCursor) {
    $taskItem=Get-Item -LiteralPath $taskCursor -Force -ErrorAction Stop
    if (($taskItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw 'Linked package paths are not allowed' }
    $taskCursor=[IO.Path]::GetDirectoryName($taskCursor)
}
$taskManifest=Get-Content -LiteralPath (Join-Path $taskPackage 'manifest.json') -Raw | ConvertFrom-Json
if ($taskManifest.status -ne 'ready' -or $taskManifest.web_projects -ne 2 -or -not $taskManifest.web_binary_sha256 -or (Test-Path -LiteralPath (Join-Path $taskPackage 'scheme.json'))) { throw 'A fresh ready dual-project package is required' }
$taskRepo=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskFile=if ($Scenario -eq 'query-cache') {'query-cache.spec.ts'} else {'project-switch.spec.ts'}
$taskSavedPackage=$env:STEERLINE_WEB_PACKAGE
$taskSavedEvidence=$env:STEERLINE_WEB_EVIDENCE
try {
    $env:STEERLINE_WEB_PACKAGE=$taskPackage
    $env:STEERLINE_WEB_EVIDENCE=Join-Path $taskPackage ('evidence\browser-'+$Scenario)
    Push-Location $taskRepo
    try {
        & npm --prefix web run test:e2e -- $taskFile 2>&1 | Tee-Object -FilePath (Join-Path $taskPackage ('evidence\'+$Scenario+'.log'))
        if ($LASTEXITCODE -ne 0) { throw "Scenario or cleanup failed; inspect $taskPackage\evidence" }
    } finally { Pop-Location }
} finally {
    $env:STEERLINE_WEB_PACKAGE=$taskSavedPackage
    $env:STEERLINE_WEB_EVIDENCE=$taskSavedEvidence
}
