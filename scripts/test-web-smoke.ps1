param([string]$WebAddress='127.0.0.1:19743')
$ErrorActionPreference='Stop'
$taskRepo=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskRelease=& (Join-Path $PSScriptRoot 'new-debug-release.ps1') -WebAddress $WebAddress
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
    $taskOwner=Start-Process -FilePath $taskHelper -ArgumentList @('debug','run','--simulate') -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $taskPackage 'evidence\launch.log') -RedirectStandardError (Join-Path $taskPackage 'evidence\launch.stderr.log')
    Push-Location $taskRepo
    try {
        & npm --prefix web run test:e2e -- smoke.spec.ts 2>&1 | Tee-Object -FilePath (Join-Path $taskPackage 'evidence\smoke.log')
        $taskPassed=$LASTEXITCODE -eq 0
    } finally { Pop-Location }
} finally {
    $taskStopErrors=@()
    foreach ($entry in @(@($taskWeb,'stop'),@($taskBinary,'stop-app'),@($taskBinary,'stop-service','-config',$env:STEERLINE_WEB_CONFIG))) {
        $taskExe=$entry[0];$taskArgs=@($entry | Select-Object -Skip 1)
        & $taskExe @taskArgs 2>&1 | Out-File -LiteralPath (Join-Path $taskPackage 'evidence\stop.log') -Append
        if ($LASTEXITCODE -ne 0) { $taskStopErrors+=($taskExe+' '+($taskArgs -join ' ')) }
    }
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
