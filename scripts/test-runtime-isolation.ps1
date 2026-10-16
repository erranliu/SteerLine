param(
    [string]$SteerRunnerExe = $env:STEERRUNNER_EXE,
    [ValidateSet('All', 'L3', 'L4')][string]$Level = 'All',
    [string]$Suite = 'runtime',
    [string]$Scenario = '',
    [string]$OutDir = '',
    [switch]$IndependentProcess
)
$ErrorActionPreference = 'Stop'
if ($SteerRunnerExe -and -not [IO.Path]::IsPathRooted($SteerRunnerExe)) { throw 'SteerRunnerExe must be absolute when supplied' }
if (-not $IndependentProcess) {
    . (Join-Path $PSScriptRoot 'independent-test.ps1')
    $arguments = @('-Level', $Level, '-Suite', $Suite)
    if ($SteerRunnerExe) { $arguments += @('-SteerRunnerExe', $SteerRunnerExe) }
    if ($Scenario) { $arguments += @('-Scenario', $Scenario) }
    if ($OutDir) { $arguments += @('-OutDir', $OutDir) }
    $minutes = if ($Level -eq 'All') { 100 } else { 50 }
    Invoke-IndependentTest -ScriptPath $PSCommandPath -TestArguments $arguments -Timeout ([TimeSpan]::FromMinutes($minutes))
    return
}
$savedRunner = $env:STEERRUNNER_EXE
try {
    $env:STEERRUNNER_EXE = $SteerRunnerExe
    Push-Location ([IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..')))
    try {
        $levels = if ($Level -eq 'All') { @('L3', 'L4') } else { @($Level) }
        foreach ($selectedLevel in $levels) {
            $testArguments = @('run', './cmd/test-feedback', ('-level=' + $selectedLevel), ('-suite=' + $Suite), '-env=controlled')
            if ($Scenario) { $testArguments += ('-scenario=' + $Scenario) }
            if ($OutDir) { $testArguments += ('-out=' + (Join-Path $OutDir $selectedLevel)) }
            # Windows PowerShell wraps native stderr as ErrorRecord when the
            # parent captures streams, including successful test summaries.
            $savedErrorAction = $ErrorActionPreference
            try {
                $ErrorActionPreference = 'Continue'
                & go @testArguments 2>&1 | ForEach-Object { $_.ToString() }
                $testExit = $LASTEXITCODE
            } finally { $ErrorActionPreference = $savedErrorAction }
            if ($testExit -ne 0) { throw "$selectedLevel acceptance failed; preserve test-feedback evidence." }
        }
    } finally { Pop-Location }
} finally { $env:STEERRUNNER_EXE = $savedRunner }
