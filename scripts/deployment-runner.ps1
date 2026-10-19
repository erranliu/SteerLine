function Get-DeploymentRunnerSelection([string]$InstallDirectory, [bool]$UpdateRunner) {
    $configPath = Join-Path $InstallDirectory 'execution.json'
    $installed = Join-Path $InstallDirectory 'bin\steerrunner.exe'
    if ($UpdateRunner -or -not (Test-Path -LiteralPath $configPath)) {
        return @{ Reuse = $false; Executable = $installed }
    }
    $config = Get-Content -LiteralPath $configPath -Raw -Encoding UTF8 | ConvertFrom-Json -ErrorAction Stop
    if ($config.version -ne 3 -or -not $config.runner.version -or -not $config.runner.sha256 -or -not $config.execution_profiles) {
        throw 'Deployment requires a complete version-3 execution.json.'
    }
    if (-not [IO.Path]::IsPathRooted($config.runner.executable) -or -not [IO.Path]::GetFullPath($config.runner.executable).Equals([IO.Path]::GetFullPath($installed), [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Installed Runner selection differs from the installation bin directory.'
    }
    if (-not (Test-Path -LiteralPath $installed -PathType Leaf) -or (Get-FileHash -LiteralPath $installed -Algorithm SHA256).Hash -ne $config.runner.sha256) {
        throw 'Installed Runner does not match its release pin; preserve the original runtime.'
    }
    return @{ Reuse = $true; Executable = $installed; Version = $config.runner.version; SHA256 = $config.runner.sha256 }
}

function Assert-DeploymentRunnerBuild([string]$InstallDirectory, [string]$SHA256) {
    $path = Join-Path $InstallDirectory 'application-api\runner-build.json'
    if (Test-Path -LiteralPath $path) {
        $pin = Get-Content -LiteralPath $path -Raw -Encoding UTF8 | ConvertFrom-Json -ErrorAction Stop
        if (-not $pin.sha256 -or $pin.sha256 -ne $SHA256) {
            throw 'Runner build change requires a fresh environment; ordinary deployment reuses the installed Runner and preserves its Runs. No installed service or file was changed.'
        }
    }
}
