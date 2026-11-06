param(
    [Parameter(Mandatory)][string]$SteerRunnerExe,
    [string]$RemoteProfile = '',
    [string]$OutRoot = '',
    [string]$DebugRoot = '',
    [string]$CatalogEnvironment = '',
    [switch]$ZeroProjects,
    [switch]$ConfigurationEdit,
    [switch]$LocalOrca,
    [switch]$IndependentProcess
)
$ErrorActionPreference = 'Stop'
if ($ConfigurationEdit -and !$ZeroProjects) {throw 'Configuration editing requires explicit -ZeroProjects'}
if (!$LocalOrca -and !$ConfigurationEdit) {
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
    if ($LocalOrca) {$driverArgs += '-LocalOrca'}
    if ($ConfigurationEdit) {$driverArgs += '-ConfigurationEdit'}
    if ($OutRoot) { $driverArgs += @('-OutRoot', $OutRoot) }
    if ($DebugRoot) { $driverArgs += @('-DebugRoot', $DebugRoot) }
    if ($CatalogEnvironment) { $driverArgs += @('-CatalogEnvironment', $CatalogEnvironment) }
    if ($ZeroProjects) { $driverArgs += '-ZeroProjects' }
    Invoke-IndependentTest -ScriptPath $PSCommandPath -TestArguments $driverArgs -Timeout ([TimeSpan]::FromMinutes(15))
    return
}
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$generatorArgs = @{SteerRunnerExe=$SteerRunnerExe}
if ($DebugRoot) { $generatorArgs['DebugRoot'] = $DebugRoot }
$generated = & (Join-Path $PSScriptRoot 'new-debug-release.ps1') @generatorArgs
$package = ($generated | Select-Object -Last 1) | ConvertFrom-Json
if (-not $package.root.StartsWith('C:\SteerlineDebug\', [StringComparison]::OrdinalIgnoreCase)) { throw 'Expected a new independent debug package' }
$oldHome = $env:STEERLINE_HOME
$oldWeb = $env:STEERLINE_CONFIG_WEB
$oldTarget = $env:STEERLINE_CONFIG_REMOTE
$oldLocal = $env:STEERLINE_CONFIG_LOCAL_ORCA
$oldEvidence = $env:STEERLINE_WEB_EVIDENCE
$oldZeroProjects = $env:STEERLINE_CONFIG_ZERO_PROJECTS
$oldEdit = $env:STEERLINE_CONFIG_EDIT
$evidence = if ($OutRoot) { [IO.Path]::GetFullPath($OutRoot) } else { Join-Path $package.root 'evidence\native-discovery' }
New-Item -ItemType Directory -Path $evidence | Out-Null
$env:STEERLINE_HOME = Join-Path $package.root 'home'
if ($ConfigurationEdit) {
    # A separate fresh home, with no package manifest, uses the same pinned
    # deployment assets. The generated package itself remains frozen.
    $env:STEERLINE_HOME=Join-Path $evidence 'configuration-home'
    New-Item -ItemType Directory -Path $env:STEERLINE_HOME | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $env:STEERLINE_HOME 'etc') | Out-Null
    foreach ($name in @('execution.json','web.json')) {Copy-Item -LiteralPath (Join-Path $package.root ('home\'+$name)) -Destination (Join-Path $env:STEERLINE_HOME $name)}
}
$env:STEERLINE_CONFIG_EDIT=if($ConfigurationEdit){'1'}else{''}

$env:STEERLINE_CONFIG_WEB = Join-Path $package.root 'bin\steerline-web.exe'
$env:STEERLINE_CONFIG_REMOTE = $CatalogEnvironment
$env:STEERLINE_CONFIG_LOCAL_ORCA = if($LocalOrca) { '1' } else { '' }
$env:STEERLINE_WEB_EVIDENCE = Join-Path $evidence 'browser'
$env:STEERLINE_CONFIG_ZERO_PROJECTS = if ($ZeroProjects) { '1' } else { '' }
$cli = Join-Path $package.root 'bin\steerline.exe'
$failed = $null
Push-Location $repo
try {
    if ($ConfigurationEdit) {
        & $cli install *> (Join-Path $evidence 'edit-home-install.log')
        if ($LASTEXITCODE -ne 0) {throw 'Independent editable home setup failed'}
        & $env:STEERLINE_CONFIG_WEB install *> (Join-Path $evidence 'edit-home-web-install.log')
        if ($LASTEXITCODE -ne 0) {throw 'Independent editable Web setup failed'}
    }
    if ($ZeroProjects) {
        $fixtureEtc = Join-Path $env:STEERLINE_HOME 'etc'
        $fixtureRegistrations = @(Get-ChildItem -LiteralPath $fixtureEtc -Filter '*.json' -File)
        $fixtureRemoved = @()
        foreach ($registration in $fixtureRegistrations) {
            $fixtureRemoved += @{name=$registration.Name;sha256=(Get-FileHash -LiteralPath $registration.FullName -Algorithm SHA256).Hash}
            Remove-Item -LiteralPath $registration.FullName
        }
        @{scope='new debug package only; zero registered projects before startup';removed=$fixtureRemoved} | ConvertTo-Json -Depth 5 | Set-Content -Encoding utf8 -LiteralPath (Join-Path $evidence 'zero-project-fixture.json')
    }
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
    $env:STEERLINE_CONFIG_ZERO_PROJECTS = $oldZeroProjects
    $env:STEERLINE_CONFIG_EDIT = $oldEdit
    Pop-Location
}
Write-Output "Native discovery evidence: $evidence"
if ($failed) { throw $failed }
if ($stopExit -ne 0 -or $cleanupExit -ne 0) { throw 'Owned resource settlement is unverified' }
