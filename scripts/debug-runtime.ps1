# Source-isolation regression fixture only. Ordinary and AI debugging must use
# scripts/new-debug-release.ps1 and its complete generated SteerlineTest package.
param(
    [Parameter(Mandatory = $true)]
    [string]$DebugHome,

    [string]$InstalledHome = 'C:\SteerlineRuntime',

    [string]$CodexAuthHome,

    [Parameter(Mandatory = $true)]
    [string]$Config,

    [Parameter(Mandatory = $true)]
    [ValidateSet('build', 'launchers', 'console', 'service', 'cli')]
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
        # Get-Item also exposes dangling links, whose missing target must still
        # be resolved before a later write creates it.
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

function Assert-WritablePathWithin([string]$Root, [string]$Path) {
    $resolved = Resolve-CanonicalPath $Path
    if (-not (Test-PathWithin $Root $resolved)) {
        throw "Writable path must resolve inside its home: '$Path' resolves to '$resolved' outside '$Root'"
    }
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
    if ($null -ne $item -and $item.LinkType -eq 'HardLink') {
        throw "Writable file must not be a hard link: '$Path'"
    }
}

function Invoke-Product([string]$Executable, [string[]]$ProductArguments) {
    & $Executable @ProductArguments
    if ($LASTEXITCODE -ne 0) {
        throw "SteerLine exited with code $LASTEXITCODE"
    }
}

function Assert-SourceExecutable([string]$BuildExe) {
    if (-not (Test-Path -LiteralPath $BuildExe -PathType Leaf)) {
        throw "Missing source build executable '$BuildExe'; run this script with -Operation build first."
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

function Set-DebugRuntimeOnLauncher([string]$Path, [string]$Executable) {
    if ($Executable.Contains('%') -or $Executable.Contains('"') -or $Executable.Contains("`r") -or $Executable.Contains("`n")) {
        throw "The source executable path cannot be represented safely in a Windows launcher: '$Executable'"
    }
    $encoding = New-Object System.Text.UTF8Encoding -ArgumentList $false
    $content = [IO.File]::ReadAllText($Path, $encoding)
    $codexAssignment = 'set "CODEX_HOME=%SHOME%\codex"'
    $binTemplate = 'set "BIN=%SHOME%\bin\steerline.exe"'
    $executableAssignment = 'set "BIN=' + $Executable + '"'
    if ($content.Contains($codexAssignment) -and $content.Contains($executableAssignment)) {
        return
    }
    $homeAssignment = 'set "STEERLINE_HOME=%SHOME%"'
    if (-not $content.Contains($homeAssignment) -or -not $content.Contains($binTemplate)) {
        throw "Launcher does not set its own home: '$Path'"
    }
    $content = [regex]::Replace($content, '(?m)^set "CODEX_HOME=.*"\r?\n?', '')
    $content = [regex]::Replace($content, '(?m)^set "BIN=.*"\r?\n?', '')
    $assignments = $homeAssignment + "`r`n" + $executableAssignment + "`r`n" + $codexAssignment
    $content = $content.Replace($homeAssignment, $assignments)
    [IO.File]::WriteAllText($Path, $content, $encoding)
}

function Assert-DebugLauncherExecutable([string]$Path, [string]$Executable) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "Debug launcher is missing: '$Path'; run -Operation launchers first."
    }
    $content = [IO.File]::ReadAllText($Path, [Text.Encoding]::UTF8)
    $expected = 'set "BIN=' + $Executable + '"'
    if (-not $content.Contains($expected)) {
        throw "Debug launcher is not bound to the current source executable '$Executable'; run -Operation launchers."
    }
}

function Write-DebugReadme([string]$Path, [string]$Executable, [string]$ProjectConfig) {
    $encoding = New-Object System.Text.UTF8Encoding -ArgumentList $false
    $content = @"
SteerLine source debug home
==========================

STEERLINE_HOME: $debugHomePath
CODEX_HOME: $debugHomePath\codex
Project config: $ProjectConfig
Source executable: $Executable

console.bat and service-window.bat run the source executable above. This home
owns its configuration, SQLite state, logs, service files, worktrees, Codex
sessions, and copied authentication file. No executable copy is installed in
this home's bin directory.

Stop the debug service before rebuilding the source executable. Run
scripts\debug-runtime.ps1 with -Operation build and -Operation launchers to
rebuild and bind the launchers to the current source executable.
"@
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
    $launcher = Join-Path $debugHomePath 'service-window.bat'
    # Preflight every script/product output before the first side effect. A
    # disjoint home root does not imply its existing output targets are disjoint.
    foreach ($relative in @('bin', 'bin\steerline.exe', 'etc', 'execution.json', 'state', 'ui', 'ui\views', 'codex', 'codex\auth.json', 'console.bat', 'service-window.bat', 'README.txt')) {
        Assert-WritablePathWithin $debugHomePath (Join-Path $debugHomePath $relative)
    }
    Assert-WritablePathWithin $repoPath (Join-Path $repoPath 'bin')
    Assert-WritablePathWithin $repoPath $buildExe
    $homeExecutable = Get-Item -LiteralPath (Join-Path $debugHomePath 'bin\steerline.exe') -Force -ErrorAction SilentlyContinue
    if ($null -ne $homeExecutable) {
        throw "Debug home contains an executable copy at '$($homeExecutable.FullName)'; use a clean debug home. Debug launchers use the source executable '$buildExe'."
    }
    $oldHome = $env:STEERLINE_HOME
    $oldCodexHome = $env:CODEX_HOME
    $env:STEERLINE_HOME = $debugHomePath
    try {
        switch ($Operation) {
            'build' {
                Assert-DebugExecutableStopped $buildExe
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
            'launchers' {
                Assert-SourceExecutable $buildExe
                Invoke-Product $buildExe @('launchers')
                Set-DebugRuntimeOnLauncher (Join-Path $debugHomePath 'console.bat') $buildExe
                Set-DebugRuntimeOnLauncher $launcher $buildExe
                Set-DebugCodexHome
                Write-DebugReadme (Join-Path $debugHomePath 'README.txt') $buildExe $configPath
            }
            'console' {
                Assert-SourceExecutable $buildExe
                if (-not (Test-Path -LiteralPath (Join-Path $debugHomePath 'console.bat') -PathType Leaf) -or
                    -not (Test-Path -LiteralPath $launcher -PathType Leaf)) {
                    throw "Debug launchers are missing under '$debugHomePath'; run -Operation launchers first."
                }
                Assert-DebugLauncherExecutable (Join-Path $debugHomePath 'console.bat') $buildExe
                Assert-DebugLauncherExecutable $launcher $buildExe
                Set-DebugCodexHome
                Invoke-Product $buildExe @('console', '-config', $configPath)
            }
            'service' {
                Assert-SourceExecutable $buildExe
                Assert-DebugLauncherExecutable $launcher $buildExe
                Set-DebugCodexHome
                & $launcher $configPath
                if ($LASTEXITCODE -ne 0) {
                    throw "Debug service window exited with code $LASTEXITCODE"
                }
            }
            'cli' {
                Assert-SourceExecutable $buildExe
                if ($Arguments.Count -eq 0) {
                    throw 'CLI requires a SteerLine command in -Arguments.'
                }
                if ($Arguments[0] -in @('install', 'launchers', 'console')) {
                    throw "Use the dedicated debug runtime operation for '$($Arguments[0])'."
                }
                foreach ($argument in $Arguments) {
                    if ($argument -eq '-config' -or $argument.StartsWith('-config=')) {
                        throw 'Pass project configuration with -Config; CLI arguments cannot override it.'
                    }
                }
                Set-DebugCodexHome
                Invoke-Product $buildExe (@($Arguments) + @('-config', $configPath))
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
