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
    Invoke-IndependentTest -ScriptPath $PSCommandPath -TestArguments $arguments
    return
}
$previousRunner = $env:STEERRUNNER_EXE
Push-Location ([IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..')))
try {
    $env:STEERRUNNER_EXE = $SteerRunnerExe
    $testArguments = @('run', './cmd/test-feedback', '-level=L2')
    if ($OutDir) { $testArguments += ('-out=' + $OutDir) }
    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        & go @testArguments 2>&1 | ForEach-Object { $_.ToString() }
        $testExit = $LASTEXITCODE
    } finally { $ErrorActionPreference = $previousErrorAction }
    if ($testExit -ne 0) { throw 'Full L2 failed; preserve the test-feedback evidence.' }
} finally {
    $env:STEERRUNNER_EXE = $previousRunner
    Pop-Location
}
