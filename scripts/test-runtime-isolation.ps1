param([string]$SteerRunnerExe = $env:STEERRUNNER_EXE, [switch]$IndependentProcess)
$ErrorActionPreference = 'Stop'
if (-not $SteerRunnerExe -or -not [IO.Path]::IsPathRooted($SteerRunnerExe)) { throw 'Select an absolute standalone SteerRunnerExe' }
if (-not $IndependentProcess) {
    . (Join-Path $PSScriptRoot 'independent-test.ps1')
    Invoke-IndependentTest -ScriptPath $PSCommandPath -TestArguments @('-SteerRunnerExe', $SteerRunnerExe)
    return
}
$savedRunner = $env:STEERRUNNER_EXE
try {
    $env:STEERRUNNER_EXE = $SteerRunnerExe
    Push-Location ([IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..')))
    try {
        # Windows PowerShell wraps native stderr as ErrorRecord when the parent
        # captures streams. test-feedback also writes its successful summary there.
        $savedErrorAction = $ErrorActionPreference
        try {
            $ErrorActionPreference = 'Continue'
            & go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled 2>&1 | ForEach-Object { $_.ToString() }
            $testExit = $LASTEXITCODE
        } finally { $ErrorActionPreference = $savedErrorAction }
        if ($testExit -ne 0) { throw 'Runtime isolation acceptance failed' }
    } finally { Pop-Location }
} finally { $env:STEERRUNNER_EXE = $savedRunner }
