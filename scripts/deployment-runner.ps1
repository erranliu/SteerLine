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

function Assert-DeploymentRunnerBuild([string]$InstallDirectory, [string]$SHA256, [string]$CandidateExecutable, [string]$VerifierCLI) {
    $raw = & $VerifierCLI verify-runner-upgrade --home $InstallDirectory --runner-exe $CandidateExecutable --runner-sha256 $SHA256 --json
    if ($LASTEXITCODE -ne 0) { throw 'Runner upgrade preflight failed; installed service and files were preserved. Drain the original installation first; v8 requires a fresh InstallHome for old state.' }
    $proof = $raw | ConvertFrom-Json -ErrorAction Stop
    if ($null -eq $proof -or $proof.contract -ne 'steerline-runner-upgrade/v1' -or $proof.home -ne $InstallDirectory -or $proof.candidate_sha256 -ne $SHA256 -or $proof.state_format -ne 'steerrunner-state/v8' -or -not [IO.Path]::IsPathRooted($proof.state_dir)) { throw 'Runner upgrade result differs from the staged invocation; installed service and files were preserved.' }
    return $proof
}
