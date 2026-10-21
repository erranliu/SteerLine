param([Parameter(Mandatory=$true)][string]$SteerRunnerExe, [string]$Out = '', [switch]$IndependentProcess)
$ErrorActionPreference = 'Stop'
if (-not $IndependentProcess) {
    . (Join-Path $PSScriptRoot 'independent-test.ps1')
    $testArguments = @('-SteerRunnerExe', $SteerRunnerExe)
    if ($Out) { $testArguments += @('-Out', $Out) }
    Invoke-IndependentTest -ScriptPath $PSCommandPath -TestArguments $testArguments -Timeout ([TimeSpan]::FromMinutes(12))
    return
}
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$release = & (Join-Path $PSScriptRoot 'new-debug-release.ps1') -SteerRunnerExe $SteerRunnerExe -ProjectMaintenance -WebProjects 2 | ConvertFrom-Json
if (-not $release.root) { throw 'Debug package generation failed' }
if ($Out) {
    New-Item -ItemType Directory -Path $Out -Force | Out-Null
    [IO.File]::WriteAllText((Join-Path $Out 'debug-package.txt'), $release.root)
}
$env:STEERLINE_PM_SOURCE = $repo
$ErrorActionPreference = 'Continue'
& (Join-Path $release.root 'bin/debug-helper.exe') debug maintenance --simulate
$testExit = $LASTEXITCODE
$ErrorActionPreference = 'Stop'
if ($Out -and (Test-Path -LiteralPath (Join-Path $release.root 'evidence/project-maintenance/proof.json'))) {
    Copy-Item -LiteralPath (Join-Path $release.root 'evidence/project-maintenance/proof.json') -Destination (Join-Path $Out 'proof.json')
}
if ($testExit -ne 0) { throw "Project maintenance failed; inspect $($release.root)/evidence/project-maintenance" }
