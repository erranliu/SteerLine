param(
    [Parameter(Mandatory)][string]$SteerRunnerExe,
    [string]$RemoteProfile = '',
    [string]$OutRoot = '',
    [switch]$LocalOrca,
    [switch]$IndependentProcess
)
$ErrorActionPreference = 'Stop'
if (!$LocalOrca) {
    if (!$RemoteProfile) { $RemoteProfile=$env:STEERLINE_L4_REMOTE_PROFILE }
    if (!$RemoteProfile -or !$OutRoot) { throw 'Remote directory L4 requires -RemoteProfile (or STEERLINE_L4_REMOTE_PROFILE) and a new absolute -OutRoot' }
    if (!$IndependentProcess) {
        . (Join-Path $PSScriptRoot 'independent-test.ps1')
        Invoke-IndependentTest -ScriptPath $PSCommandPath -TestArguments @('-SteerRunnerExe',$SteerRunnerExe,'-RemoteProfile',$RemoteProfile,'-OutRoot',$OutRoot) -Timeout ([TimeSpan]::FromMinutes(25))
        return
    }
    $env:STEERLINE_L4_REMOTE_PROFILE=$RemoteProfile
    $env:STEERRUNNER_EXE=$SteerRunnerExe
    Push-Location ([IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..')))
    try {
        $ErrorActionPreference='Continue'
        & go @('run','./cmd/test-feedback','-level=L4','-suite=configuration','-env=live','-scenario=configuration.execution-profiles-orca',('-out='+$OutRoot)) 2>&1 | ForEach-Object { $_.ToString() }
        $testExit=$LASTEXITCODE
        $ErrorActionPreference='Stop'
        if($testExit){throw 'Remote directory L4 failed'}
    } finally { Pop-Location }
    return
}
$PSDefaultParameterValues=@{'Out-File:Encoding'='utf8'}
if (-not $IndependentProcess) {
    . (Join-Path $PSScriptRoot 'independent-test.ps1')
    $driverArgs = @('-SteerRunnerExe', $SteerRunnerExe)
    $driverArgs += '-LocalOrca'
    Invoke-IndependentTest -ScriptPath $PSCommandPath -TestArguments $driverArgs -Timeout ([TimeSpan]::FromMinutes(15))
    return
}
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$generated = & (Join-Path $PSScriptRoot 'new-debug-release.ps1') -SteerRunnerExe $SteerRunnerExe
$package = ($generated | Select-Object -Last 1) | ConvertFrom-Json
if (-not $package.root.StartsWith('C:\SteerlineDebug\', [StringComparison]::OrdinalIgnoreCase)) { throw 'Expected a new independent debug package' }
$oldHome = $env:STEERLINE_HOME
$oldWeb = $env:STEERLINE_CONFIG_WEB
$oldTarget = $env:STEERLINE_CONFIG_REMOTE
$oldLocal = $env:STEERLINE_CONFIG_LOCAL_ORCA
$oldEvidence = $env:STEERLINE_WEB_EVIDENCE
$evidence = Join-Path $package.root 'evidence\native-discovery'
New-Item -ItemType Directory -Path $evidence | Out-Null
$env:STEERLINE_HOME = Join-Path $package.root 'home'
$env:STEERLINE_CONFIG_WEB = Join-Path $package.root 'bin\steerline-web.exe'
$env:STEERLINE_CONFIG_REMOTE = ''
$env:STEERLINE_CONFIG_LOCAL_ORCA = if($LocalOrca) { '1' } else { '' }
$env:STEERLINE_WEB_EVIDENCE = Join-Path $evidence 'browser'
$cli = Join-Path $package.root 'bin\steerline.exe'
$failed = $null
Push-Location $repo
try {
    & $cli start --no-open *> (Join-Path $evidence 'start.log')
    if ($LASTEXITCODE -ne 0) { throw 'Independent package startup failed' }
    $savedErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        & npm.cmd --prefix web run test:e2e -- execution-configs-live.spec.ts 2>&1 | ForEach-Object { $_.ToString() } | Tee-Object -FilePath (Join-Path $evidence 'browser.log')
        $testExit = $LASTEXITCODE
    } finally { $ErrorActionPreference = $savedErrorAction }
    if ($testExit -ne 0) { throw 'Native catalog browser verification failed' }
} catch { $failed = $_ } finally {
    $ErrorActionPreference = 'Continue'
    & $cli stop *> (Join-Path $evidence 'stop.log')
    $stopExit = $LASTEXITCODE
    $cleanup = & $cli verify-stop --runner-exe (Join-Path $package.root 'bin\steerrunner.exe') --json
    $cleanupExit = $LASTEXITCODE
    $cleanup | Set-Content -Encoding utf8 -LiteralPath (Join-Path $evidence 'cleanup.json')
    @{package=$package.root; evidence=$evidence; stop_exit=$stopExit; cleanup_exit=$cleanupExit; passed=($null -eq $failed -and $stopExit -eq 0 -and $cleanupExit -eq 0)} | ConvertTo-Json | Set-Content -Encoding utf8 -LiteralPath (Join-Path $evidence 'result.json')
    $env:STEERLINE_HOME = $oldHome
    $env:STEERLINE_CONFIG_WEB = $oldWeb
    $env:STEERLINE_CONFIG_REMOTE = $oldTarget
    $env:STEERLINE_CONFIG_LOCAL_ORCA = $oldLocal
    $env:STEERLINE_WEB_EVIDENCE = $oldEvidence
    Pop-Location
}
Write-Output "Native discovery evidence: $evidence"
if ($failed) { throw $failed }
if ($stopExit -ne 0 -or $cleanupExit -ne 0) { throw 'Owned resource settlement is unverified' }
