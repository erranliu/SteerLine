param([string]$WebAddress='127.0.0.1:19743', [string]$SteerRunnerExe=$env:STEERRUNNER_EXE, [ValidateSet('smoke.spec.ts','native-session.spec.ts')][string]$BrowserScenario='smoke.spec.ts', [switch]$IndependentProcess)
$ErrorActionPreference='Stop'
if(-not $IndependentProcess){
 . (Join-Path $PSScriptRoot 'independent-test.ps1')
 $arguments=@('-WebAddress',$WebAddress,'-BrowserScenario',$BrowserScenario)
 if($SteerRunnerExe){$arguments+=@('-SteerRunnerExe',$SteerRunnerExe)}
 Invoke-IndependentTest -ScriptPath $PSCommandPath -TestArguments $arguments
 return
}
$taskRepo=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskRelease=& (Join-Path $PSScriptRoot 'new-debug-release.ps1') -WebAddress $WebAddress -SteerRunnerExe $SteerRunnerExe
if ($LASTEXITCODE -ne 0) { throw 'Fresh Web smoke package failed' }
$taskPackage=($taskRelease | ConvertFrom-Json).root
if (-not $taskPackage.StartsWith('C:\SteerlineDebug\',[StringComparison]::OrdinalIgnoreCase)) { throw 'Generated package escaped debug root' }
$taskPreviousHome=$env:STEERLINE_HOME
$taskPreviousConfig=$env:STEERLINE_WEB_CONFIG
$taskPreviousEvidence=$env:STEERLINE_WEB_EVIDENCE
$taskPreviousState=$env:STEERLINE_E2E_CONTROLLED_STATE
$env:STEERLINE_HOME=Join-Path $taskPackage 'home'
$env:STEERLINE_WEB_CONFIG=Join-Path $taskPackage 'home\etc\SteerlineTest.json'
$env:STEERLINE_WEB_EVIDENCE=Join-Path $taskPackage 'evidence\browser'
$env:STEERLINE_E2E_CONTROLLED_STATE=Join-Path $taskPackage 'provider\state.json'
$taskHelper=Join-Path $taskPackage 'bin\debug-helper.exe'
$taskBinary=Join-Path $taskPackage 'bin\steerline.exe'
$taskWeb=Join-Path $taskPackage 'bin\steerline-web.exe'
$taskOwner=$null
$taskPassed=$false
$taskCleanup=$false
try {
    $taskOwner=Start-Process -FilePath $taskHelper -ArgumentList @('debug','run','--simulate','--no-open') -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $taskPackage 'evidence\launch.log') -RedirectStandardError (Join-Path $taskPackage 'evidence\launch.stderr.log')
    Push-Location $taskRepo
    try {
        & npm --prefix web run test:e2e -- $BrowserScenario 2>&1 | Tee-Object -FilePath (Join-Path $taskPackage 'evidence\smoke.log')
        $taskPassed=$LASTEXITCODE -eq 0
    } finally { Pop-Location }
} finally {
    $taskStopErrors=@()
    $taskSavedAction=$ErrorActionPreference
    try {
        $ErrorActionPreference='Continue'
        & $taskHelper debug stop --simulate 2>&1 | Out-File -LiteralPath (Join-Path $taskPackage 'evidence\stop.log') -Append
        $taskStopExit=$LASTEXITCODE
    } finally { $ErrorActionPreference=$taskSavedAction }
    if ($taskStopExit -ne 0) { $taskStopErrors+=('debug stop exit '+$taskStopExit) }
    if ($taskOwner) { $taskOwner.WaitForExit(15000) | Out-Null }
    $taskProcesses=@(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -like ($taskPackage+'\*') })
    $taskListeners=@(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | Where-Object { $_.OwningProcess -in $taskProcesses.ProcessId -or $_.LocalPort -eq [int]($WebAddress.Split(':')[1]) })
    $taskCleanup=$taskProcesses.Count -eq 0 -and $taskListeners.Count -eq 0 -and $taskStopErrors.Count -eq 0
    @{passed=($taskPassed -and $taskCleanup);browser_passed=$taskPassed;processes=$taskProcesses.Count;listeners=$taskListeners.Count;stop_errors=$taskStopErrors;package=$taskPackage} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $taskPackage 'evidence\smoke-cleanup.json')
    $env:STEERLINE_HOME=$taskPreviousHome
    $env:STEERLINE_WEB_CONFIG=$taskPreviousConfig
    $env:STEERLINE_WEB_EVIDENCE=$taskPreviousEvidence
    $env:STEERLINE_E2E_CONTROLLED_STATE=$taskPreviousState
}
if (-not $taskPassed -or -not $taskCleanup) { throw "Web smoke or cleanup failed; inspect $taskPackage\evidence" }
Write-Output "Web smoke and cleanup passed: $taskPackage"
