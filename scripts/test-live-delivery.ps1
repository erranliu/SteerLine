param(
    [string]$SteerRunnerExe = $env:STEERRUNNER_EXE,
    [string]$OutDir = '',
    [switch]$IndependentProcess
)
$ErrorActionPreference = 'Stop'
if (-not $IndependentProcess) {
    . (Join-Path $PSScriptRoot 'independent-test.ps1')
    $arguments = @()
    if ($SteerRunnerExe) { $arguments += @('-SteerRunnerExe', $SteerRunnerExe) }
    if ($OutDir) { $arguments += @('-OutDir', $OutDir) }
    # Cover live deadlines, builds and independent cleanup.
    Invoke-IndependentTest -ScriptPath $PSCommandPath -TestArguments $arguments -Timeout ([TimeSpan]::FromMinutes(60))
    return
}
$previousRunner = $env:STEERRUNNER_EXE
Push-Location ([IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..')))
try {
    $env:STEERRUNNER_EXE = $SteerRunnerExe
    $testArguments = @('run', './cmd/test-feedback', '-level=L4', '-suite=smoke', '-env=live', '-scenario=smoke.local-codex')
    if ($OutDir) { $testArguments += ('-out=' + $OutDir) }
    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        & go @testArguments 2>&1 | ForEach-Object { $_.ToString() }
        $testExit = $LASTEXITCODE
    } finally { $ErrorActionPreference = $previousErrorAction }
    if ($testExit -ne 0) { throw 'Live delivery failed; preserve the test-feedback evidence.' }
} finally {
    $env:STEERRUNNER_EXE = $previousRunner
    Pop-Location
}
