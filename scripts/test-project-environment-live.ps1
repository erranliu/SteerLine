param(
    [Parameter(Mandatory)][string]$ProjectConfig,
    [Parameter(Mandatory)][string]$ProjectCommit,
    [Parameter(Mandatory)][string]$SteerRunnerExe,
    [Parameter(Mandatory)][string]$OutRoot,
    [string]$RemoteProfile=$env:STEERLINE_L4_REMOTE_PROFILE,
    [ValidateRange(1,2147483647)][int]$RoundTimeoutSeconds=1200,
    [switch]$IndependentProcess
)
$ErrorActionPreference='Stop'
if (!$IndependentProcess) {
    . (Join-Path $PSScriptRoot 'independent-test.ps1')
    Invoke-IndependentTest -ScriptPath $PSCommandPath -TestArguments @('-ProjectConfig',$ProjectConfig,'-ProjectCommit',$ProjectCommit,'-SteerRunnerExe',$SteerRunnerExe,'-OutRoot',$OutRoot,'-RemoteProfile',$RemoteProfile,'-RoundTimeoutSeconds',"$RoundTimeoutSeconds") -Timeout ([TimeSpan]::FromSeconds(3*[double]$RoundTimeoutSeconds+2400))
    return
}
$env:STEERLINE_L4_REMOTE_PROFILE=$RemoteProfile
$env:STEERRUNNER_EXE=$SteerRunnerExe
$env:STEERLINE_L4_PROJECT_CONFIG=$ProjectConfig
$env:STEERLINE_L4_PROJECT_COMMIT=$ProjectCommit
$env:STEERLINE_L4_ROUND_TIMEOUT_SECONDS="$RoundTimeoutSeconds"
Push-Location ([IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..')))
try {
    $ErrorActionPreference='Continue'
    & go @('run','./cmd/test-feedback','-level=L4','-suite=testing','-env=live','-scenario=testing.project-environment-orca',('-out='+$OutRoot)) 2>&1 | ForEach-Object { $_.ToString() }
    $testExit=$LASTEXITCODE
    $ErrorActionPreference='Stop'
    if($testExit){throw 'Project environment L4 failed; inspect the original report'}
} finally { Pop-Location }
