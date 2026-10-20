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
    $path = Join-Path $InstallDirectory 'application-api\runner-build.json'
    if (Test-Path -LiteralPath $path) {
        $pin = Get-Content -LiteralPath $path -Raw -Encoding UTF8 | ConvertFrom-Json -ErrorAction Stop
        if (-not $pin.sha256 -or $pin.sha256 -ne $SHA256) {
            if (-not $pin.sha256 -or -not $pin.state_dir -or -not [IO.Path]::IsPathRooted($pin.state_dir) -or -not $CandidateExecutable -or -not $VerifierCLI) { throw 'Runner build change requires original state binding and candidate takeover evidence; no installed service or file was changed.' }
            $raw = & $VerifierCLI verify-runner-takeover --runner-exe $CandidateExecutable --runner-sha256 $SHA256 --state-dir $pin.state_dir --json
            if ($LASTEXITCODE -ne 0) { throw 'Candidate cannot verify original Runner contracts; installed service and files were preserved.' }
            $proof = $raw | ConvertFrom-Json -ErrorAction Stop
            # Go validates the public report and every item. Only bind the
            # returned result to this staged invocation here.
            if ($null -eq $proof -or $proof.candidate_sha256 -ne $SHA256 -or $proof.state_dir -ne $pin.state_dir) { throw 'Candidate takeover result differs from the staged invocation; installed service and files were preserved.' }
            return $proof
        }
    }
}
