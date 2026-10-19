param(
    [string]$InstallHome = 'C:\SteerlineInstall',
    [string]$RunnerRepo = '',
    [switch]$UpdateRunner,
    [switch]$NoStart,
    [switch]$NoPause
)
$ErrorActionPreference = 'Stop'
if (-not [IO.Path]::IsPathRooted($InstallHome)) { throw 'InstallHome must be absolute.' }
$sourceRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if (-not $RunnerRepo) { $RunnerRepo = Join-Path (Split-Path -Parent $sourceRoot) 'SteerRunner' }
$RunnerRepo = [IO.Path]::GetFullPath($RunnerRepo)
$InstallHome = [IO.Path]::GetFullPath($InstallHome)
. (Join-Path $PSScriptRoot 'deployment-stop.ps1')
. (Join-Path $PSScriptRoot 'deployment-runner.ps1')

function Within([string]$Root, [string]$Path) {
    $rootPath = [IO.Path]::GetFullPath($Root).TrimEnd('\','/')
    $valuePath = [IO.Path]::GetFullPath($Path).TrimEnd('\','/')
    return $valuePath.Equals($rootPath, [StringComparison]::OrdinalIgnoreCase) -or $valuePath.StartsWith($rootPath + '\', [StringComparison]::OrdinalIgnoreCase)
}
function Assert-Plain([string]$Path) {
    $cursor = [IO.Path]::GetFullPath($Path)
    while ($cursor) {
        $item = Get-Item -LiteralPath $cursor -Force -ErrorAction SilentlyContinue
        if ($item -and (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $item.LinkType -eq 'HardLink')) { throw "Linked deployment path: $cursor" }
        $parent = [IO.Path]::GetDirectoryName($cursor)
        if ($parent -eq $cursor) { break }
        $cursor = $parent
    }
}
function Invoke-Checked([string]$Exe, [string[]]$Arguments) {
    & $Exe @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$Exe failed (exit $LASTEXITCODE)" }
}
function Install-Executable([string]$Exe, [string]$Target) {
    for ($attempt = 1; $attempt -le 5; $attempt++) {
        # Windows can briefly retain a stopped image or scan the replacement.
        # Retry only this idempotent file install, never stop/start or task work.
        $savedPreference = $ErrorActionPreference
        try {
            $ErrorActionPreference = 'Continue'
            $output = & $Exe install 2>&1
            $code = $LASTEXITCODE
        } finally { $ErrorActionPreference = $savedPreference }
        if ($code -eq 0) { $output | Write-Output; return }
        $message = $output | Out-String
        if ($attempt -eq 5 -or $message -notmatch '(?is)rename .*Access is denied|sharing violation|being used by another process') { throw "Installation failed: $message" }
        Assert-Plain $Target
        $running = @(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -and $_.ExecutablePath.Equals($Target, [StringComparison]::OrdinalIgnoreCase) })
        if ($running.Count) { throw "Installation target restarted: $Target" }
        Write-Output "Windows has not released the installation file; retry $attempt/4."
        Start-Sleep -Seconds 1
    }
}
function Write-JSON([string]$Path, $Value) {
    Assert-Plain $Path
    $temporary = $Path + '.' + [Guid]::NewGuid().ToString('N') + '.tmp'
    [IO.File]::WriteAllText($temporary, ($Value | ConvertTo-Json -Depth 50), [Text.UTF8Encoding]::new($false))
    Move-Item -LiteralPath $temporary -Destination $Path -Force
}
function Wait-Ready([string]$Exe) {
    $deadline = [DateTime]::UtcNow.AddSeconds(180)
    do {
        $raw = & $Exe status --json
        if ($LASTEXITCODE -ne 0) { throw 'Installed status could not be verified.' }
        $status = $raw | ConvertFrom-Json
        $failed = @($status.projects | Where-Object runtime_state -eq 'failed')
        if ($failed.Count) { throw ('Project startup failed: ' + (($failed | ForEach-Object { $_.config_id + ': ' + $_.diagnostic }) -join '; ')) }
        $pending = @($status.projects | Where-Object { $_.runtime_state -ne 'ready' -or -not $_.online })
        if ($status.backend.state -eq 'ready' -and $status.web.state -eq 'ready' -and -not $status.degraded -and $pending.Count -eq 0) {
            Write-Output "Ready: backend, Web, $(@($status.projects).Count) projects."
            return
        }
        Start-Sleep -Seconds 1
    } while ([DateTime]::UtcNow -lt $deadline)
    throw 'Installed projects did not become ready within 180 seconds; inspect installation logs.'
}

if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'Deployment requires Windows.' }
if (-not [IO.Path]::IsPathRooted($InstallHome) -or $InstallHome.TrimEnd('\') -eq [IO.Path]::GetPathRoot($InstallHome).TrimEnd('\')) { throw 'Select a dedicated absolute installation directory.' }
foreach ($protected in @($sourceRoot, $RunnerRepo, 'C:\SteerlineRuntime', 'C:\SteerlineDebug')) {
    if ((Within $protected $InstallHome) -or (Within $InstallHome $protected)) { throw "Installation must be disjoint from $protected" }
}
foreach ($path in @($InstallHome, $sourceRoot, $RunnerRepo)) { Assert-Plain $path }
foreach ($path in @((Join-Path $RunnerRepo 'bin\steerrunner.exe'), (Join-Path $sourceRoot 'bin\steerline.exe'), (Join-Path $sourceRoot 'bin\steerline-web.exe'))) { Assert-Plain $path }
New-Item -ItemType Directory -Path $InstallHome -Force | Out-Null
$lockPath = Join-Path $InstallHome '.deploy.lock'
Assert-Plain $lockPath
$deploymentLock = [IO.File]::Open($lockPath, [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
try {

# Complete all builds before stopping or replacing an installed component.
Assert-Plain (Join-Path $InstallHome 'execution.json')
Assert-Plain (Join-Path $InstallHome 'bin\steerrunner.exe')
$runnerSelection = Get-DeploymentRunnerSelection $InstallHome ([bool]$UpdateRunner)
if (-not $runnerSelection.Reuse) {
    if (-not (Test-Path -LiteralPath (Join-Path $RunnerRepo 'scripts\build.ps1'))) { throw 'RunnerRepo must contain the SteerRunner checkout.' }
    Push-Location $RunnerRepo
    try { & (Join-Path $RunnerRepo 'scripts\build.ps1'); if (-not $?) { throw 'Runner build failed.' } } finally { Pop-Location }
    $runnerSource = Join-Path $RunnerRepo 'bin\steerrunner.exe'
} else {
    $runnerSource = $runnerSelection.Executable
    Write-Output 'Reusing the installed, pinned Runner; building SteerLine and Web only.'
}
Push-Location $sourceRoot
try { & (Join-Path $sourceRoot 'build.ps1') -Web; if (-not $?) { throw 'SteerLine/Web build failed.' } } finally { Pop-Location }

$stamp = [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss') + '-' + [Guid]::NewGuid().ToString('N').Substring(0,8)
$package = Join-Path $InstallHome ('deployments\' + $stamp)
New-Item -ItemType Directory -Path $package -Force | Out-Null
$bin = Join-Path $InstallHome 'bin'
foreach ($name in @('steerline.exe','steerline-web.exe','steerrunner.exe')) { Assert-Plain (Join-Path $bin $name) }
foreach ($pair in @(@('steerline.exe',$sourceRoot), @('steerline-web.exe',$sourceRoot))) {
    $source = Join-Path $pair[1] ('bin\' + $pair[0])
    Assert-Plain $source
    Copy-Item -LiteralPath $source -Destination (Join-Path $package $pair[0])
}
Copy-Item -LiteralPath $runnerSource -Destination (Join-Path $package 'steerrunner.exe')
$runnerStage = Join-Path $package 'steerrunner.exe'
$versionRaw = & $runnerStage version --json
if ($LASTEXITCODE -ne 0) { throw 'Staged Runner cannot report its version.' }
$version = $versionRaw | ConvertFrom-Json
if (-not $version.ok -or $version.protocol -ne 'steerrunner-cli/v1') { throw 'Staged Runner protocol is unsupported.' }
$runnerHash = (Get-FileHash -LiteralPath $runnerStage -Algorithm SHA256).Hash.ToLowerInvariant()
if ($runnerSelection.Reuse -and ($runnerHash -ne $runnerSelection.SHA256 -or $version.data.version -ne $runnerSelection.Version)) { throw 'Staged Runner differs from its installed release pin.' }
Assert-Plain (Join-Path $InstallHome 'application-api\runner-build.json')
Assert-DeploymentRunnerBuild $InstallHome $runnerHash
$configPath = Join-Path $InstallHome 'execution.json'
Assert-Plain $configPath
if (Test-Path -LiteralPath $configPath) {
    $execution = Get-Content -LiteralPath $configPath -Raw -Encoding UTF8 | ConvertFrom-Json
    if ($execution.version -ne 3 -or -not $execution.runner -or -not $execution.execution_profiles) { throw 'Deployment requires current version-3 execution.json; it does not migrate legacy data.' }
} else {
    $execution = Get-Content -LiteralPath (Join-Path $sourceRoot 'examples\execution.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    $execution.execution_profiles = @($execution.execution_profiles | Where-Object id -eq 'local-luna-medium')
    $execution.default_execution_profile = 'local-luna-medium'
}
$allowRestart = $execution.runner.sha256 -eq $runnerHash
$execution.runner.executable = Join-Path $bin 'steerrunner.exe'
$execution.runner.version = $version.data.version
$execution.runner.sha256 = $runnerHash

$savedHome = $env:STEERLINE_HOME
try {
    $env:STEERLINE_HOME = $InstallHome
    $installed = Join-Path $bin 'steerline.exe'
    if (Test-Path -LiteralPath $installed) {
        $verification = Stop-DeploymentApplication (Join-Path $package 'steerline.exe') $runnerStage -AllowRestart:$allowRestart
        if ($null -ne $verification) {
            Write-JSON (Join-Path $package 'shutdown-verification.json') $verification
        }
    }
    $owned = @(Get-CimInstance Win32_Process | Where-Object { $_.Name -in @('steerline.exe','steerline-web.exe','steerrunner.exe') -and $_.ExecutablePath -and (Within $bin $_.ExecutablePath) })
    if ($owned.Count) { throw 'Installed processes remain; preserve the original instance and inspect its shutdown result.' }
    Assert-Plain $bin
    Install-Executable (Join-Path $package 'steerline.exe') (Join-Path $bin 'steerline.exe')
    Install-Executable (Join-Path $package 'steerline-web.exe') (Join-Path $bin 'steerline-web.exe')
    Assert-Plain (Join-Path $bin 'steerrunner.exe')
    Copy-Item -LiteralPath $runnerStage -Destination (Join-Path $bin 'steerrunner.exe') -Force
    foreach ($name in @('steerline.exe','steerline-web.exe','steerrunner.exe')) {
        if ((Get-FileHash -LiteralPath (Join-Path $bin $name) -Algorithm SHA256).Hash -ne (Get-FileHash -LiteralPath (Join-Path $package $name) -Algorithm SHA256).Hash) { throw "Installed executable differs from staged package: $name" }
    }
    New-Item -ItemType Directory -Path (Join-Path $InstallHome 'etc') -Force | Out-Null
    Write-JSON $configPath $execution
    $record = [ordered]@{ created_at=[DateTime]::UtcNow.ToString('o'); home=$InstallHome; steerline_source=(& git -C $sourceRoot rev-parse HEAD); steerline_source_dirty=(-not [string]::IsNullOrWhiteSpace((& git -C $sourceRoot status --porcelain | Out-String))); runner_source=$version.data.source_commit; runner_version=$version.data.version; files=@{} }
    foreach ($name in @('steerline.exe','steerline-web.exe','steerrunner.exe')) { $record.files[$name] = (Get-FileHash -LiteralPath (Join-Path $bin $name) -Algorithm SHA256).Hash.ToLowerInvariant() }
    Write-JSON (Join-Path $package 'deployment.json') $record
    Write-JSON (Join-Path $InstallHome 'deployment.json') $record
    if (-not $NoStart) {
        Invoke-Checked $installed @('start')
        Wait-Ready $installed
    }
    Write-Output "Installed: $InstallHome"
    Write-Output "Build record: $package\deployment.json"
} finally { $env:STEERLINE_HOME = $savedHome }
} finally { $deploymentLock.Dispose() }
