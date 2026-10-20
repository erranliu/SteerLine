function Stop-DeploymentApplication([string]$CLI, [string]$RunnerExe, [switch]$AllowRestart) {
    # Use the staged orchestrator. Installed files are replaced only after this
    # function returns successfully; neither command starts execution.
    $savedPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $stopOutput = & $CLI stop 2>&1
        $stopCode = $LASTEXITCODE
    } finally { $ErrorActionPreference = $savedPreference }
    $stopOutput | ForEach-Object { Write-Host $_ }
    if ($stopCode -eq 0) { return $null }
    if ($AllowRestart) {
        Write-Host 'Checking restart/takeover eligibility; original native executions and their diagnostics are retained.'
        $raw = & $CLI verify-restart --runner-exe $RunnerExe --json
        if ($LASTEXITCODE -ne 0) { throw 'Original service closure or compatible takeover could not be verified; installed files were preserved.' }
        $proof = $raw | ConvertFrom-Json -ErrorAction Stop
        if ($proof.restartable -ne $true -or -not $proof.original_host.instance_id -or -not $proof.verified_at -or $null -eq $proof.projects -or -not $proof.runner_build.sha256 -or -not $proof.runner_build.resource_digest -or -not $proof.runner_build.state_dir -or -not $proof.runner_service.instance_id -or -not $proof.runner_service.request_id -or $proof.runner_service.state_dir -ne $proof.runner_build.state_dir -or $proof.runner_service.executable_sha256 -ne $proof.runner_build.sha256) {
            throw 'Restart verification did not return complete original evidence; installed files were preserved.'
        }
        $candidateHash = (Get-FileHash -LiteralPath $RunnerExe -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($proof.verifier_sha256 -ne $candidateHash) { throw 'Restart verifier differs from staged Runner; installed files were preserved.' }
        if ($candidateHash -ne $proof.runner_build.sha256 -and ($null -eq $proof.candidate_build -or $proof.candidate_build.sha256 -ne $candidateHash -or $proof.candidate_build.state_dir -ne $proof.runner_build.state_dir -or $null -eq $proof.takeover -or $proof.takeover.contract -ne 'steerrunner-takeover/v1' -or $proof.takeover.compatible -ne $true -or $proof.takeover.candidate_sha256 -ne $candidateHash -or $proof.takeover.state_dir -ne $proof.runner_build.state_dir)) { throw 'Changed Runner lacks compatible original-state takeover evidence; installed files were preserved.' }
        return $proof
    }
    Write-Host 'Original shutdown is unverified; checking original resource identities without restarting execution.'
    $raw = & $CLI verify-stop --runner-exe $RunnerExe --json
    if ($LASTEXITCODE -ne 0) { throw 'Original resource verification failed; installed files were preserved.' }
    $proof = $raw | ConvertFrom-Json -ErrorAction Stop
    if ($null -eq $proof -or -not $proof.original_host.instance_id -or -not $proof.verified_at -or $null -eq $proof.projects) {
        throw 'Original resource verification did not return complete evidence; installed files were preserved.'
    }
    if ($proof.runner.resources_settled -ne $true -or -not $proof.runner.state_dir -or $null -eq $proof.runner.runs -or $null -eq $proof.runner.checks) {
        throw 'Shared Runner has no complete settled resource evidence; installed files were preserved.'
    }
    return $proof
}
