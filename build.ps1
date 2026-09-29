param()

$ErrorActionPreference = 'Stop'

if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
    throw 'build.ps1 builds the Windows executable; use go build ./cmd/steerline on other platforms.'
}

$root = $PSScriptRoot
$output = [IO.Path]::GetFullPath((Join-Path $root 'bin\steerline.exe'))
$binDir = Split-Path -Parent $output
$temp = Join-Path $binDir ('.steerline-build-' + [guid]::NewGuid().ToString('N') + '.exe')

function Assert-OutputNotRunning {
    $running = Get-Process -Name steerline -ErrorAction SilentlyContinue |
        Where-Object { $_.Path -and [IO.Path]::GetFullPath($_.Path) -eq $output }
    if ($running) {
        throw "Cannot replace running executable $output (PID: $($running.Id -join ', ')). Stop it first."
    }
}

Assert-OutputNotRunning
New-Item -ItemType Directory -Path $binDir -Force | Out-Null

try {
    Push-Location $root
    try {
        & go build -trimpath -o $temp ./cmd/steerline
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed with exit code $LASTEXITCODE"
        }
    } finally {
        Pop-Location
    }

    Assert-OutputNotRunning
    if (Test-Path -LiteralPath $output) {
        $backup = Join-Path $binDir ('.steerline-previous-' + [guid]::NewGuid().ToString('N') + '.exe')
        try {
            [IO.File]::Replace($temp, $output, $backup)
        } finally {
            if (Test-Path -LiteralPath $backup) {
                Remove-Item -LiteralPath $backup -Force
            }
        }
    } else {
        [IO.File]::Move($temp, $output)
    }
    Write-Output "Built $output"
} finally {
    if (Test-Path -LiteralPath $temp) {
        Remove-Item -LiteralPath $temp -Force
    }
}
