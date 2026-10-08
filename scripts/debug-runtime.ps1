param(
    [Parameter(Mandatory = $true)]
    [string]$DebugHome,

    [string]$InstalledHome = 'C:\SteerlineRuntime',

    [string]$CodexAuthHome,

    [Parameter(Mandatory = $true)]
    [string]$Config,

    [Parameter(Mandatory = $true)]
    [ValidateSet('build', 'install', 'launchers', 'console', 'service', 'cli')]
    [string]$Operation,

    [string[]]$Arguments = @()
)

$ErrorActionPreference = 'Stop'

function Resolve-CanonicalPath([string]$Path) {
    if (-not [IO.Path]::IsPathRooted($Path)) {
        throw "Path must be absolute: $Path"
    }
    return Resolve-ReparsePath $Path 0
}

function Resolve-ReparsePath([string]$Path, [int]$Depth) {
    if ($Depth -gt 32) {
        throw "Too many directory aliases while resolving '$Path'"
    }
    $full = [IO.Path]::GetFullPath($Path)
    $root = [IO.Path]::GetPathRoot($full)
    $remainder = $full.Substring($root.Length)
    $parts = @($remainder -split '[\\/]' | Where-Object { $_ -ne '' })
    $current = $root
    foreach ($part in $parts) {
        $candidate = [IO.Path]::GetFullPath((Join-Path $current $part))
        if (Test-Path -LiteralPath $candidate) {
            $item = Get-Item -LiteralPath $candidate -Force
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

function Test-PathWithin([string]$Root, [string]$Path) {
    $rootFull = [IO.Path]::GetFullPath($Root).TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
    $pathFull = [IO.Path]::GetFullPath($Path).TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
    if ([string]::Equals($rootFull, $pathFull, [StringComparison]::OrdinalIgnoreCase)) {
        return $true
    }
    return $pathFull.StartsWith($rootFull + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase) -or
        $pathFull.StartsWith($rootFull + [IO.Path]::AltDirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)
}

function Assert-Disjoint([string]$First, [string]$Second, [string]$Description) {
    if ((Test-PathWithin $First $Second) -or (Test-PathWithin $Second $First)) {
        throw "$Description paths overlap after resolving aliases: '$First' and '$Second'"
    }
}

function Invoke-Product([string]$Executable, [string[]]$ProductArguments) {
    & $Executable @ProductArguments
    if ($LASTEXITCODE -ne 0) {
        throw "SteerLine exited with code $LASTEXITCODE"
    }
}

function Get-FileSHA256([string]$Path) {
    $algorithm = [Security.Cryptography.SHA256]::Create()
    $stream = [IO.File]::OpenRead($Path)
    try {
        return [BitConverter]::ToString($algorithm.ComputeHash($stream)).Replace('-', '')
    } finally {
        $stream.Dispose()
        $algorithm.Dispose()
    }
}

function Assert-CurrentDebugExecutable([string]$BuildExe, [string]$DebugExe) {
    if (-not (Test-Path -LiteralPath $BuildExe -PathType Leaf)) {
        throw "Missing source build executable '$BuildExe'; run this script with -Operation build first."
    }
    if (-not (Test-Path -LiteralPath $DebugExe -PathType Leaf)) {
        throw "Missing debug executable '$DebugExe'; run this script with -Operation install first."
    }
    $buildHash = Get-FileSHA256 $BuildExe
    $debugHash = Get-FileSHA256 $DebugExe
    if ($buildHash -ne $debugHash) {
        throw "Debug executable is stale: '$DebugExe' does not match '$BuildExe'; run -Operation install."
    }
}

function Assert-DebugExecutableStopped([string]$DebugExe) {
    if (-not (Test-Path -LiteralPath $DebugExe -PathType Leaf)) {
        return
    }
    $target = Resolve-CanonicalPath $DebugExe
    $running = @(Get-Process -Name steerline -ErrorAction SilentlyContinue | Where-Object {
        $_.Path -and [string]::Equals([IO.Path]::GetFullPath($_.Path), $target, [StringComparison]::OrdinalIgnoreCase)
    })
    if ($running.Count -gt 0) {
        $pids = ($running | ForEach-Object { $_.Id }) -join ', '
        throw "Debug executable is still running at '$target' (PID: $pids); stop it before rebuilding or replacing it."
    }
}

function Set-DebugCodexHome {
    $codexHome = Join-Path $debugHomePath 'codex'
    New-Item -ItemType Directory -Path $codexHome -Force | Out-Null
    $authHome = $CodexAuthHome
    if ([string]::IsNullOrWhiteSpace($authHome)) {
        $authHome = $env:CODEX_HOME
    }
    if ([string]::IsNullOrWhiteSpace($authHome)) {
        $authHome = Join-Path $env:USERPROFILE '.codex'
    }
    if ([IO.Path]::IsPathRooted($authHome)) {
        $sourceAuth = Join-Path $authHome 'auth.json'
        if (Test-Path -LiteralPath $sourceAuth -PathType Leaf) {
            $destinationAuth = Join-Path $codexHome 'auth.json'
            if (-not (Test-Path -LiteralPath $destinationAuth -PathType Leaf)) {
                Copy-Item -LiteralPath $sourceAuth -Destination $destinationAuth
            }
        }
    }
    $env:CODEX_HOME = Resolve-CanonicalPath $codexHome
}

function Add-DebugCodexHomeToLauncher([string]$Path) {
    $encoding = New-Object System.Text.UTF8Encoding -ArgumentList $false
    $content = [IO.File]::ReadAllText($Path, $encoding)
    $codexAssignment = 'set "CODEX_HOME=%SHOME%\codex"'
    if ($content.Contains($codexAssignment)) {
        return
    }
    $homeAssignment = 'set "STEERLINE_HOME=%SHOME%"'
    if (-not $content.Contains($homeAssignment)) {
        throw "Launcher does not set its own home: '$Path'"
    }
    $content = $content.Replace($homeAssignment, $homeAssignment + "`r`n" + $codexAssignment)
    [IO.File]::WriteAllText($Path, $content, $encoding)
}

try {
    if (-not [IO.Path]::IsPathRooted($DebugHome) -or -not [IO.Path]::IsPathRooted($Config)) {
        throw 'DebugHome and Config must be absolute paths.'
    }
    $repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
    $debugHomePath = Resolve-CanonicalPath $DebugHome
    $installedHomePath = Resolve-CanonicalPath $InstalledHome
    $repoPath = Resolve-CanonicalPath $repoRoot
    $configPath = Resolve-CanonicalPath $Config

    if (-not (Test-Path -LiteralPath $Config -PathType Leaf)) {
        throw "Project config is missing: '$Config'"
    }
    Assert-Disjoint $debugHomePath $installedHomePath 'Debug and installed home'
    Assert-Disjoint $debugHomePath $repoPath 'Debug home and source worktree'
    Assert-Disjoint $installedHomePath $repoPath 'Installed home and source worktree'
    if (-not (Test-PathWithin $debugHomePath $configPath)) {
        throw "Project config must resolve inside the debug home: '$configPath'"
    }

    $configText = [IO.File]::ReadAllText($configPath, [Text.Encoding]::UTF8)
    $project = ConvertFrom-Json -InputObject $configText
    $configDirectory = Split-Path -Parent $configPath
    $repositoryValue = [string]$project.repository
    if ([string]::IsNullOrWhiteSpace($repositoryValue)) {
        throw 'Project config must name a repository.'
    }
    $repositoryPath = $repositoryValue
    if (-not [IO.Path]::IsPathRooted($repositoryPath)) {
        $repositoryPath = Join-Path $configDirectory $repositoryPath
    }
    $repositoryPath = Resolve-CanonicalPath $repositoryPath
    Assert-Disjoint $debugHomePath $repositoryPath 'Debug home and project repository'
    Assert-Disjoint $installedHomePath $repositoryPath 'Installed home and project repository'

    foreach ($entry in @(@('run_dir', $project.run_dir), @('worktree_root', $project.worktree_root))) {
        $name = [string]$entry[0]
        $value = [string]$entry[1]
        if ([string]::IsNullOrWhiteSpace($value)) {
            continue
        }
        $path = $value
        if (-not [IO.Path]::IsPathRooted($path)) {
            $path = Join-Path $configDirectory $path
        }
        $path = Resolve-CanonicalPath $path
        if (-not (Test-PathWithin $debugHomePath $path)) {
            throw "$name must resolve inside the debug home: '$path'"
        }
    }

    $buildExe = Join-Path $repoPath 'bin\steerline.exe'
    $debugExe = Join-Path $debugHomePath 'bin\steerline.exe'
    $launcher = Join-Path $debugHomePath 'service-window.bat'
    $oldHome = $env:STEERLINE_HOME
    $oldCodexHome = $env:CODEX_HOME
    $env:STEERLINE_HOME = $debugHomePath
    try {
        switch ($Operation) {
            'build' {
                Assert-DebugExecutableStopped $debugExe
                Push-Location $repoPath
                try {
                    & (Join-Path $repoPath 'build.ps1')
                    if ($LASTEXITCODE -ne 0) {
                        throw "build.ps1 exited with code $LASTEXITCODE"
                    }
                } finally {
                    Pop-Location
                }
                if (-not (Test-Path -LiteralPath $buildExe -PathType Leaf)) {
                    throw "Build did not create '$buildExe'"
                }
                Write-Output "Built debug source executable $buildExe"
            }
            'install' {
                Assert-DebugExecutableStopped $debugExe
                if (-not (Test-Path -LiteralPath $buildExe -PathType Leaf)) {
                    throw "Missing source build executable '$buildExe'; run -Operation build first."
                }
                Invoke-Product $buildExe @('install')
                $installedExe = Join-Path $debugHomePath 'bin\steerline.exe'
                if (-not (Test-Path -LiteralPath $installedExe -PathType Leaf)) {
                    throw "Debug install did not create '$installedExe'"
                }
                Write-Output "Installed debug executable $installedExe"
            }
            'launchers' {
                if (-not (Test-Path -LiteralPath $buildExe -PathType Leaf)) {
                    throw "Missing source build executable '$buildExe'; run -Operation build first."
                }
                Invoke-Product $buildExe @('launchers')
                Add-DebugCodexHomeToLauncher (Join-Path $debugHomePath 'console.bat')
                Add-DebugCodexHomeToLauncher $launcher
                Set-DebugCodexHome
            }
            'console' {
                Assert-CurrentDebugExecutable $buildExe $debugExe
                if (-not (Test-Path -LiteralPath (Join-Path $debugHomePath 'console.bat') -PathType Leaf) -or
                    -not (Test-Path -LiteralPath $launcher -PathType Leaf)) {
                    throw "Debug launchers are missing under '$debugHomePath'; run -Operation launchers first."
                }
                Set-DebugCodexHome
                Invoke-Product $debugExe @('console', '-config', $configPath)
            }
            'service' {
                Assert-CurrentDebugExecutable $buildExe $debugExe
                if (-not (Test-Path -LiteralPath $launcher -PathType Leaf)) {
                    throw "Debug service launcher is missing: '$launcher'; run -Operation launchers first."
                }
                Set-DebugCodexHome
                & $launcher $configPath
                if ($LASTEXITCODE -ne 0) {
                    throw "Debug service window exited with code $LASTEXITCODE"
                }
            }
            'cli' {
                Assert-CurrentDebugExecutable $buildExe $debugExe
                if ($Arguments.Count -eq 0) {
                    throw 'CLI requires a SteerLine command in -Arguments.'
                }
                foreach ($argument in $Arguments) {
                    if ($argument -eq '-config' -or $argument.StartsWith('-config=')) {
                        throw 'Pass project configuration with -Config; CLI arguments cannot override it.'
                    }
                }
                Set-DebugCodexHome
                Invoke-Product $debugExe (@($Arguments) + @('-config', $configPath))
            }
        }
    } finally {
        $env:STEERLINE_HOME = $oldHome
        $env:CODEX_HOME = $oldCodexHome
    }
} catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
