$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'deployment-stop.ps1')

$script:calls = [Collections.Generic.List[string]]::new()
$script:stopCode = 0
$script:verifyCode = 0
$script:invalidProof = $false
function Invoke-StagedFixture {
    $script:calls.Add(($args -join '|'))
    if ($args[0] -eq 'stop') {
        $global:LASTEXITCODE = $script:stopCode
        return
    }
    if (($args -join '|') -ne 'verify-stop|--runner-exe|staged-runner|--json') { throw 'Verifier did not use selected staged Runner.' }
    $global:LASTEXITCODE = $script:verifyCode
    if ($script:verifyCode -eq 0) {
        if ($script:invalidProof) { '{}' } else { '{"original_host":{"instance_id":"original"},"verified_at":"2026-10-16T00:00:00Z","projects":[],"runner":{"state_dir":"owned-shared-runner","resources_settled":true,"runs":[],"checks":[]}}' }
    }
}

$proof = Stop-DeploymentApplication 'Invoke-StagedFixture' 'staged-runner'
if ($null -ne $proof -or $script:calls.Count -ne 1 -or $script:calls[0] -ne 'stop') { throw 'Clean stop should finish directly.' }
$script:calls.Clear()
$script:stopCode = 1
$proof = Stop-DeploymentApplication 'Invoke-StagedFixture' 'staged-runner'
if ($script:calls.Count -ne 2 -or $proof.original_host.instance_id -ne 'original') { throw 'Failed receipt was not independently verified.' }
$script:calls.Clear()
$script:verifyCode = 1
$failed = $false
try { Stop-DeploymentApplication 'Invoke-StagedFixture' 'staged-runner' } catch { $failed = $true }
if (-not $failed -or $script:calls.Count -ne 2) { throw 'Failed verification did not block deployment.' }
$script:verifyCode = 0
$script:invalidProof = $true
$failed = $false
try { Stop-DeploymentApplication 'Invoke-StagedFixture' 'staged-runner' } catch { $failed = $true }
if (-not $failed) { throw 'Incomplete verification evidence did not block deployment.' }
Write-Output 'Passed: staged stop, independent verification, and failed-verification deployment gate.'
