function Get-DeploymentPackageDigest([string]$CLI, [string]$RunnerExe) {
    $directory = Split-Path -Parent $CLI
    if (-not [IO.Path]::GetFullPath((Join-Path $directory 'steerrunner.exe')).Equals([IO.Path]::GetFullPath($RunnerExe), [StringComparison]::OrdinalIgnoreCase)) { throw 'Runner must belong to the selected staged package.' }
    $lines = foreach ($name in @('steerline.exe','steerline-web.exe','steerrunner.exe')) {
        $hash = (Get-FileHash -LiteralPath (Join-Path $directory $name) -Algorithm SHA256).Hash.ToLowerInvariant()
        $name + ':' + $hash + [char]10
    }
    $digest = [Security.Cryptography.SHA256]::Create()
    try { return [BitConverter]::ToString($digest.ComputeHash([Text.Encoding]::UTF8.GetBytes(($lines -join '')))).Replace('-','').ToLowerInvariant() } finally { $digest.Dispose() }
}

function Stop-DeploymentApplication([string]$CLI, [string]$RunnerExe, [switch]$AllowRestart) {
    $goal = if ($AllowRestart) { 'update' } else { 'replace' }
    if (-not $env:STEERLINE_HOME -or -not [IO.Path]::IsPathRooted($env:STEERLINE_HOME)) {
        throw 'Deployment requires an explicit absolute STEERLINE_HOME.'
    }
    $expectedHome = [IO.Path]::GetFullPath($env:STEERLINE_HOME).TrimEnd('\','/')
    $candidateHash = (Get-FileHash -LiteralPath $RunnerExe -Algorithm SHA256).Hash.ToLowerInvariant()
    $packageSHA = Get-DeploymentPackageDigest $CLI $RunnerExe
    $invocation = [Guid]::NewGuid().ToString('N')
    # Go owns stop, goal-specific verification and the single guarded recovery.
    # This result is evidence, not a lease held across the later replacement.
    $raw = & $CLI prepare-deployment --runner-exe $RunnerExe --goal $goal --invocation-id $invocation --package-sha256 $packageSHA --json
    if ($LASTEXITCODE -ne 0) {
        throw 'Deployment preparation failed; installed files were preserved. Inspect the Go diagnostic for the blocked boundary.'
    }
    $proof = $raw | ConvertFrom-Json -ErrorAction Stop
    if ($null -eq $proof -or $proof.contract -ne 'steerline-deployment-preparation/v1' -or $proof.outcome -ne 'prepared' -or $proof.goal -ne $goal -or $proof.invocation_id -ne $invocation -or $proof.package_sha256 -ne $packageSHA -or -not $proof.verified_at -or -not $proof.home -or -not [IO.Path]::IsPathRooted($proof.home) -or -not [IO.Path]::GetFullPath($proof.home).TrimEnd('\','/').Equals($expectedHome, [StringComparison]::OrdinalIgnoreCase) -or $proof.verifier_sha256 -ne $candidateHash) {
        throw 'Deployment result does not match this home, goal or staged Runner; installed files were preserved.'
    }
    if ((Get-FileHash -LiteralPath $RunnerExe -Algorithm SHA256).Hash.ToLowerInvariant() -ne $candidateHash) {
        throw 'Staged Runner changed during deployment preparation; installed files were preserved.'
    }
    if ((Get-DeploymentPackageDigest $CLI $RunnerExe) -ne $packageSHA) { throw 'Staged package changed during preparation; installed files were preserved.' }
    return $proof
}
