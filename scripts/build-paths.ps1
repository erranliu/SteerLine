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

function Assert-SourceBuildPath([string]$Path, [string]$Description, [string]$Root = $sourceRoot) {
    $sourceRoot = $Root
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
