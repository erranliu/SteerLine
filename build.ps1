param([switch]$Web)

$ErrorActionPreference = 'Stop'

if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
    throw 'build.ps1 builds the Windows executable; use go build ./cmd/steerline on other platforms.'
}

function Resolve-ReparsePath([string]$Path, [int]$Depth) {
    if ($Depth -gt 32) {
        throw "Too many directory aliases while resolving '$Path'"
    }
    $full = [IO.Path]::GetFullPath($Path)
    $root = [IO.Path]::GetPathRoot($full)
    $parts = @($full.Substring($root.Length) -split '[\\/]' | Where-Object { $_ -ne '' })
    $current = $root
    foreach ($part in $parts) {
        $candidate = [IO.Path]::GetFullPath((Join-Path $current $part))
        $item = Get-Item -LiteralPath $candidate -Force -ErrorAction SilentlyContinue
        if ($null -ne $item) {
            if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                $target = [string]$item.Target
                if ([string]::IsNullOrWhiteSpace($target)) {
                    throw "Cannot resolve directory alias '$candidate'"
                }
                if (-not [IO.Path]::IsPathRooted($target)) {
                    $target = Join-Path ([IO.Path]::GetDirectoryName($candidate)) $target
                }
                $current = Resolve-ReparsePath $target ($Depth + 1)
            } else {
                $current = $candidate
            }
        } else {
            $current = $candidate
        }
    }
    return [IO.Path]::GetFullPath($current)
}

function Resolve-CanonicalPath([string]$Path) {
    if (-not [IO.Path]::IsPathRooted($Path)) {
        throw "Path must be absolute: $Path"
    }
    return Resolve-ReparsePath $Path 0
}

function Test-PathWithin([string]$Root, [string]$Path) {
    $rootFull = [IO.Path]::GetFullPath($Root).TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
    $pathFull = [IO.Path]::GetFullPath($Path).TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
    if ([string]::Equals($rootFull, $pathFull, [StringComparison]::OrdinalIgnoreCase)) {
        return $true
    }
    return $pathFull.StartsWith($rootFull + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase) -or
        $pathFull.StartsWith($rootFull + [IO.Path]::AltDirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)
}

function Assert-SourceBuildPath([string]$Path, [string]$Description) {
    $resolved = Resolve-CanonicalPath $Path
    if (-not (Test-PathWithin $sourceRoot $resolved)) {
        throw "Build $Description must resolve inside source tree: '$Path' resolves to '$resolved' outside '$sourceRoot'"
    }

    # Reject in-tree reparse points too. The build always owns bin/steerline.exe
    # and its temporary/backup files at their ordinary paths.
    $cursor = [IO.Path]::GetFullPath($Path)
    while ($true) {
        $item = Get-Item -LiteralPath $cursor -Force -ErrorAction SilentlyContinue
        if ($null -ne $item -and ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "Build $Description must not pass through a reparse point: '$cursor'"
        }
        if ([string]::Equals($cursor, $sourceRoot, [StringComparison]::OrdinalIgnoreCase)) {
            break
        }
        $parent = [IO.Path]::GetDirectoryName($cursor)
        if ([string]::IsNullOrWhiteSpace($parent) -or [string]::Equals($parent, $cursor, [StringComparison]::OrdinalIgnoreCase)) {
            break
        }
        $cursor = $parent
    }

    $item = Get-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
    if ($null -ne $item -and $item.LinkType -eq 'HardLink') {
        throw "Build $Description must not be a hard link: '$Path'"
    }
}

function Assert-OutputNotRunning {
    $target = Resolve-CanonicalPath $output
    $running = Get-Process -Name ([IO.Path]::GetFileNameWithoutExtension($output)) -ErrorAction SilentlyContinue |
        Where-Object { $_.Path -and [string]::Equals((Resolve-CanonicalPath $_.Path), $target, [StringComparison]::OrdinalIgnoreCase) }
    if ($running) {
        throw "Cannot replace running executable $target (PID: $($running.Id -join ', ')). Stop it first."
    }
}

$sourceRoot = Resolve-CanonicalPath $PSScriptRoot
$binDir = Join-Path $sourceRoot 'bin'
$output = Join-Path $binDir 'steerline.exe'
$temp = Join-Path $binDir ('.steerline-build-' + [guid]::NewGuid().ToString('N') + '.exe')

# Preflight the output directory, final binary, and temporary build path before
# creating bin or starting Go. A lexical path under this checkout may be a
# junction into an installed runtime.
Assert-SourceBuildPath $binDir 'output directory'
Assert-SourceBuildPath $output 'output file'
Assert-SourceBuildPath $temp 'temporary file'
Assert-OutputNotRunning

$sourceCommit = 'unknown'
$sourceDirty = 'unknown'
try {
    $git = (Get-Command git -CommandType Application -ErrorAction Stop | Select-Object -First 1).Source
    $commitOutput = & $git -C $sourceRoot rev-parse HEAD
    if ($LASTEXITCODE -eq 0 -and $commitOutput -match '^(?:[0-9a-f]{40}|[0-9a-f]{64})$') {
        $sourceCommit = $commitOutput.Trim()
        $statusOutput = & $git -C $sourceRoot status --porcelain
        if ($LASTEXITCODE -eq 0) {
            $sourceDirty = if ([string]::IsNullOrWhiteSpace(($statusOutput -join "`n"))) { 'false' } else { 'true' }
        }
    }
} catch {
    $sourceCommit = 'unknown'
    $sourceDirty = 'unknown'
}

Assert-SourceBuildPath $binDir 'output directory'
Assert-SourceBuildPath $output 'output file'
Assert-SourceBuildPath $temp 'temporary file'
Assert-OutputNotRunning
New-Item -ItemType Directory -Path $binDir -Force | Out-Null

$backup = $null
try {
    Assert-SourceBuildPath $binDir 'output directory'
    Assert-SourceBuildPath $output 'output file'
    Assert-SourceBuildPath $temp 'temporary file'
    Push-Location $sourceRoot
    try {
        $ldflags = "-X github.com/erranliu/SteerLine/internal/app.buildSourceCommit=$sourceCommit -X github.com/erranliu/SteerLine/internal/app.buildSourceDirty=$sourceDirty"
        & go build -trimpath -ldflags $ldflags -o $temp ./cmd/steerline
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed with exit code $LASTEXITCODE"
        }
    } finally {
        Pop-Location
    }

    Assert-SourceBuildPath $binDir 'output directory'
    Assert-SourceBuildPath $output 'output file'
    Assert-SourceBuildPath $temp 'temporary file'
    Assert-OutputNotRunning
    if (Test-Path -LiteralPath $output) {
        $backup = Join-Path $binDir ('.steerline-previous-' + [guid]::NewGuid().ToString('N') + '.exe')
        Assert-SourceBuildPath $backup 'backup file'
        try {
            [IO.File]::Replace($temp, $output, $backup)
        } finally {
            if (Test-Path -LiteralPath $backup) {
                Assert-SourceBuildPath $backup 'backup file'
                Remove-Item -LiteralPath $backup -Force
            }
        }
    } else {
        [IO.File]::Move($temp, $output)
    }
    Write-Output "Built $output"
} finally {
    if (Test-Path -LiteralPath $temp) {
        Assert-SourceBuildPath $temp 'temporary file'
        Remove-Item -LiteralPath $temp -Force
    }
}

if ($Web) {
    foreach ($webPath in @('web','web\server','web\server\dist','web\node_modules')) {
        Assert-SourceBuildPath (Join-Path $sourceRoot $webPath) 'Web build path'
    }
    Push-Location (Join-Path $sourceRoot 'web')
    try {
        & npm ci
        if ($LASTEXITCODE -ne 0) { throw 'Web npm install failed; business executable was built' }
        & npm run build
        if ($LASTEXITCODE -ne 0) { throw 'Web assets failed; business executable was built' }
    } finally { Pop-Location }
    $output = Join-Path $binDir 'steerline-web.exe'
    $temp = Join-Path $binDir ('.steerline-web-build-' + [guid]::NewGuid().ToString('N') + '.exe')
    Assert-SourceBuildPath $output 'Web output file'
    Assert-SourceBuildPath $temp 'Web temporary file'
    Assert-OutputNotRunning
    try {
        Push-Location $sourceRoot
        try {
            & go build -trimpath -tags webui -o $temp ./cmd/steerline-web
            if ($LASTEXITCODE -ne 0) { throw 'Web executable build failed; business executable was built' }
        } finally { Pop-Location }
        Assert-SourceBuildPath $output 'Web output file'
        Assert-SourceBuildPath $temp 'Web temporary file'
        Assert-OutputNotRunning
        if (Test-Path -LiteralPath $output) {
            $webBackup = Join-Path $binDir ('.steerline-web-previous-' + [guid]::NewGuid().ToString('N') + '.exe')
            Assert-SourceBuildPath $webBackup 'Web backup file'
            try {
                [IO.File]::Replace($temp, $output, $webBackup)
            } finally {
                if (Test-Path -LiteralPath $webBackup) {
                    Assert-SourceBuildPath $webBackup 'Web backup file'
                    Remove-Item -LiteralPath $webBackup -Force
                }
            }
        } else { [IO.File]::Move($temp, $output) }
        Write-Output "Built $output"
    } finally {
        if (Test-Path -LiteralPath $temp) {
            Assert-SourceBuildPath $temp 'Web temporary file'
            Remove-Item -LiteralPath $temp -Force
        }
    }
}
