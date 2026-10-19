function Stop-DeploymentApplication([string]$CLI, [string]$RunnerExe) {
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
