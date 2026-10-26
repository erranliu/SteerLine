param(
    [switch]$Web,
    [switch]$ForceWebInstall,
    [switch]$SerialBuild,
    [string]$RunnerRepo = '',
    [string]$StageDirectory = '',
    [string]$TimingPath = ''
)
$ErrorActionPreference = 'Stop'
if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
    throw 'build.ps1 builds the Windows executable; use go build ./cmd/steerline on other platforms.'
}
. (Join-Path $PSScriptRoot 'scripts\build-paths.ps1')
. (Join-Path $PSScriptRoot 'scripts\build-timing.ps1')
$sourceRoot = Resolve-CanonicalPath $PSScriptRoot
$binDir = Join-Path $sourceRoot 'bin'
$output = Join-Path $binDir 'steerline.exe'
# Refuse before creating bin, locks or reports, including aliased/hard-linked output.
Assert-SourceBuildPath $binDir 'output directory'
Assert-SourceBuildPath $output 'output file'
Assert-OutputNotRunning
if ($Web) {
    $output = Join-Path $binDir 'steerline-web.exe'
    Assert-SourceBuildPath $output 'Web output file'
    Assert-OutputNotRunning
    foreach ($relative in @('web', 'web\server', 'web\server\dist', 'web\node_modules')) {
        Assert-SourceBuildPath (Join-Path $sourceRoot $relative) 'Web build path'
    }
}
if ($RunnerRepo) {
    $RunnerRepo = Resolve-CanonicalPath ([IO.Path]::GetFullPath($RunnerRepo))
    if ((Test-PathWithin $sourceRoot $RunnerRepo) -or (Test-PathWithin $RunnerRepo $sourceRoot)) { throw 'Runner checkout must be disjoint from SteerLine.' }
    if (-not (Test-Path -LiteralPath (Join-Path $RunnerRepo 'scripts\build.ps1') -PathType Leaf)) { throw 'RunnerRepo must contain the SteerRunner checkout.' }
    Assert-SourceBuildPath (Join-Path $RunnerRepo 'bin') 'Runner output directory' $RunnerRepo
    Assert-SourceBuildPath (Join-Path $RunnerRepo 'bin\steerrunner.exe') 'Runner output file' $RunnerRepo
}
if ($StageDirectory) {
    $StageDirectory = [IO.Path]::GetFullPath($StageDirectory)
    Assert-SourceBuildPath $StageDirectory 'staged output directory'
    if (-not (Test-PathWithin $binDir $StageDirectory) -or $StageDirectory -eq $binDir -or (Test-Path -LiteralPath $StageDirectory)) {
        throw 'StageDirectory must be a new directory below source bin/.'
    }
}
New-Item -ItemType Directory -Path $binDir -Force | Out-Null
$lockPath = Join-Path $binDir '.build.lock'
Assert-SourceBuildPath $lockPath 'build lock'
# Different installation targets must not race on the same source outputs.
$buildLock = [IO.File]::Open($lockPath, [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
$runID = [Guid]::NewGuid().ToString('N')
$webTimingPath = Join-Path $binDir ('.web-timings-' + $runID + '.json')
if (-not $TimingPath) { $TimingPath = Join-Path $binDir ('build-timings\' + $runID + '.json') }
$TimingPath = [IO.Path]::GetFullPath($TimingPath)
Initialize-BuildTiming 'build'
$succeeded = $false
$webJob = $null
$runnerLock = $null

function Build-GoExecutable([string]$Name, [string[]]$GoArguments) {
    $output = Join-Path $binDir $Name
    $temp = Join-Path $binDir ('.steerline-build-' + [Guid]::NewGuid().ToString('N') + '.exe')
    foreach ($path in @($binDir, $output, $temp)) { Assert-SourceBuildPath $path 'output file' }
    Assert-OutputNotRunning
    try {
        Push-Location $sourceRoot
        try {
            $options = $GoArguments[0..($GoArguments.Length - 2)]
            & go build @options -o $temp $GoArguments[-1] | Out-Host
            if ($LASTEXITCODE -ne 0) { throw "Go build failed for $Name (exit $LASTEXITCODE)." }
        } finally { Pop-Location }
        foreach ($path in @($output, $temp)) { Assert-SourceBuildPath $path 'output file' }
        Assert-OutputNotRunning
        if (Test-Path -LiteralPath $output) {
            $backup = Join-Path $binDir ('.steerline-previous-' + [Guid]::NewGuid().ToString('N') + '.exe')
            Assert-SourceBuildPath $backup 'backup file'
            try { [IO.File]::Replace($temp, $output, $backup) }
            finally {
                if (Test-Path -LiteralPath $backup) {
                    Assert-SourceBuildPath $backup 'backup file'
                    Remove-Item -LiteralPath $backup -Force
                }
            }
        } else { [IO.File]::Move($temp, $output) }
        Write-Host "Built $output"
    } finally {
        if (Test-Path -LiteralPath $temp) {
            Assert-SourceBuildPath $temp 'temporary file'
            Remove-Item -LiteralPath $temp -Force
        }
    }
}

function Complete-WebBuild {
    if ($null -eq $script:webJob) { return }
    $job = $script:webJob
    $script:webJob = $null
    try {
        # Drain even on a Go failure. Keep the checkout lock until frontend writes end.
        Receive-Job -Job $job -Wait -ErrorAction Continue | Out-Host
        if ($job.State -ne 'Completed') { throw "Frontend build failed: $($job.ChildJobs[0].JobStateInfo.Reason)" }
    } finally {
        Import-BuildTiming $webTimingPath
        Remove-Job -Job $job -Force
    }
}

try {
    Assert-SourceBuildPath $webTimingPath 'Web timing file'
    if ($RunnerRepo) {
        New-Item -ItemType Directory -Path (Join-Path $RunnerRepo 'bin') -Force | Out-Null
        $runnerLockPath = Join-Path $RunnerRepo 'bin\.steerline-build.lock'
        Assert-SourceBuildPath $runnerLockPath 'Runner build lock' $RunnerRepo
        $runnerLock = [IO.File]::Open($runnerLockPath, [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
    }
    if ($Web -and -not $SerialBuild) {
        Write-Host 'Building frontend and Go pipelines in parallel (Go builds remain serial).'
        $webJob = Start-Job -ScriptBlock {
            param($Script, $ForceInstall, $Report)
            $ErrorActionPreference = 'Stop'
            # The hidden Windows PowerShell worker does not inherit the console's
            # UTF-8 decoder. Preserve npm/Vite output across job serialization.
            [Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
            & $Script -ForceWebInstall:$ForceInstall -TimingPath $Report
        } -ArgumentList (Join-Path $sourceRoot 'scripts\build-web-assets.ps1'), ([bool]$ForceWebInstall), $webTimingPath
    }
    if ($RunnerRepo) {
        Invoke-BuildStage 'go.runner' {
            Push-Location $RunnerRepo
            try {
                & (Join-Path $RunnerRepo 'scripts\build.ps1')
                if (-not $?) { throw 'Runner build failed.' }
            } finally { Pop-Location }
        }
    } else { Add-SkippedBuildStage 'go.runner' 'Runner source build not requested' }
    $sourceCommit = 'unknown'
    $sourceDirty = 'unknown'
    try {
        $commitOutput = & git -C $sourceRoot rev-parse HEAD
        if ($LASTEXITCODE -eq 0 -and $commitOutput -match '^(?:[0-9a-f]{40}|[0-9a-f]{64})$') {
            $sourceCommit = $commitOutput.Trim()
            $statusOutput = & git -C $sourceRoot status --porcelain
            if ($LASTEXITCODE -eq 0) { $sourceDirty = if ([string]::IsNullOrWhiteSpace(($statusOutput -join [Environment]::NewLine))) { 'false' } else { 'true' } }
        }
    } catch { $sourceCommit = 'unknown'; $sourceDirty = 'unknown' }
    $ldflags = "-X github.com/erranliu/SteerLine/internal/app.buildSourceCommit=$sourceCommit -X github.com/erranliu/SteerLine/internal/app.buildSourceDirty=$sourceDirty"
    Invoke-BuildStage 'go.steerline' {
        Build-GoExecutable 'steerline.exe' @('-trimpath', '-ldflags', $ldflags, './cmd/steerline')
    }
    if ($Web) {
        if ($SerialBuild) {
            try { & (Join-Path $sourceRoot 'scripts\build-web-assets.ps1') -ForceWebInstall:$ForceWebInstall -TimingPath $webTimingPath }
            finally { Import-BuildTiming $webTimingPath }
        } else { Complete-WebBuild }
        Invoke-BuildStage 'go.web' { Build-GoExecutable 'steerline-web.exe' @('-trimpath', '-tags', 'webui', './cmd/steerline-web') }
    } else { Add-SkippedBuildStage 'web' 'Web build not requested' }
    if ($StageDirectory) {
        Invoke-BuildStage 'build.snapshot' {
            Assert-SourceBuildPath $StageDirectory 'staged output directory'
            New-Item -ItemType Directory -Path $StageDirectory | Out-Null
            $names = @('steerline.exe')
            if ($Web) { $names += 'steerline-web.exe' }
            if ($RunnerRepo) { $names += 'steerrunner.exe' }
            foreach ($name in $names) {
                $source = Join-Path $binDir $name
                if ($name -eq 'steerrunner.exe') { $source = Join-Path $RunnerRepo 'bin\steerrunner.exe' }
                $target = Join-Path $StageDirectory $name
                Assert-SourceBuildPath $target 'staged output file'
                Copy-Item -LiteralPath $source -Destination $target
            }
        }
    }
    $succeeded = $true
} finally {
    try { Complete-WebBuild }
    finally {
        try {
            if (Test-Path -LiteralPath $webTimingPath) {
                Assert-SourceBuildPath $webTimingPath 'Web timing file'
                Remove-Item -LiteralPath $webTimingPath -Force
            }
            Write-BuildTiming $TimingPath $succeeded
        } finally {
            if ($runnerLock) { $runnerLock.Dispose() }
            $buildLock.Dispose()
        }
    }
}
