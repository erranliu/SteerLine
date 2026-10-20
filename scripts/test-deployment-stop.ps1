$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'deployment-stop.ps1')
. (Join-Path $PSScriptRoot 'deployment-runner.ps1')

$script:calls = [Collections.Generic.List[string]]::new()
$script:stopCode = 0
$script:verifyCode = 0
$script:invalidProof = $false
$script:verifyCommand = 'verify-stop'
$script:stagedHash = 'same'
function Get-FileHash([string]$LiteralPath, [string]$Algorithm) {
    if ($LiteralPath -eq 'staged-runner') { return @{Hash=$script:stagedHash} }
    Microsoft.PowerShell.Utility\Get-FileHash -LiteralPath $LiteralPath -Algorithm $Algorithm
}
function Invoke-StagedFixture {
    $script:calls.Add(($args -join '|'))
    if ($args[0] -eq 'stop') {
        $global:LASTEXITCODE = $script:stopCode
        return
    }
    if (($args -join '|') -ne ($script:verifyCommand+'|--runner-exe|staged-runner|--json')) { throw 'Verifier did not use selected staged Runner.' }
    $global:LASTEXITCODE = $script:verifyCode
    if ($script:verifyCode -eq 0) {
        if ($script:invalidProof) { '{}' }
        elseif ($script:verifyCommand -eq 'verify-restart') {
            $proof = @{restartable=$true;verifier_sha256=$script:stagedHash;original_host=@{instance_id='original'};verified_at='2026-10-19T00:00:00Z';projects=@();runner_build=@{sha256='same';resource_digest='fixed';state_dir='owned-shared-runner'};runner_service=@{instance_id='original-runner';request_id='original-stop';state_dir='owned-shared-runner';executable_sha256='same';resources_settled=$false;diagnostic='original terminal still active'}}
            if ($script:stagedHash -eq 'candidate') { $proof.candidate_build=@{sha256='candidate';state_dir='owned-shared-runner'}; $proof.takeover=@{contract='steerrunner-takeover/v1';compatible=$true;candidate_sha256='candidate';state_dir='owned-shared-runner'} }
            $proof | ConvertTo-Json -Depth 8 -Compress
        }
        else { '{"original_host":{"instance_id":"original"},"verified_at":"2026-10-16T00:00:00Z","projects":[],"runner":{"state_dir":"owned-shared-runner","resources_settled":true,"runs":[],"checks":[]}}' }
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
$script:calls.Clear()
$script:invalidProof = $false
$script:verifyCommand = 'verify-restart'
$proof = Stop-DeploymentApplication 'Invoke-StagedFixture' 'staged-runner' -AllowRestart
if (-not $proof.restartable -or $proof.runner_service.resources_settled -ne $false -or $script:calls.Count -ne 2) { throw 'Same-build restart confused retained native resources with settlement.' }
$script:stagedHash = 'candidate'
$proof = Stop-DeploymentApplication 'Invoke-StagedFixture' 'staged-runner' -AllowRestart
if ($proof.runner_build.sha256 -ne 'same' -or $proof.candidate_build.sha256 -ne 'candidate' -or $proof.runner_service.resources_settled -ne $false) { throw 'Compatible takeover changed original build or native exit facts.' }
$script:stagedHash = 'unproved'
$failed = $false
try { Stop-DeploymentApplication 'Invoke-StagedFixture' 'staged-runner' -AllowRestart } catch { $failed = $true }
if (-not $failed) { throw 'Changed build without takeover evidence was accepted.' }
$script:stagedHash = 'same'
$script:invalidProof = $true
$failed = $false
try { Stop-DeploymentApplication 'Invoke-StagedFixture' 'staged-runner' -AllowRestart } catch { $failed = $true }
if (-not $failed) { throw 'Incomplete restart proof did not block deployment.' }
$script:invalidProof = $false
$script:verifyCode = 1
$failed = $false
try { Stop-DeploymentApplication 'Invoke-StagedFixture' 'staged-runner' -AllowRestart } catch { $failed = $true }
if (-not $failed) { throw 'Unknown service shutdown did not block restart deployment.' }

$fixtureRoot = Join-Path ([IO.Path]::GetTempPath()) ('steerline-deploy-fixture-'+[Guid]::NewGuid().ToString('N'))
try {
    New-Item -ItemType Directory -Path (Join-Path $fixtureRoot 'bin') -Force | Out-Null
    $runner = Join-Path $fixtureRoot 'bin\steerrunner.exe'
    [IO.File]::WriteAllText($runner, 'original Runner bytes')
    $hash = (Get-FileHash -LiteralPath $runner -Algorithm SHA256).Hash
    $config = @{version=3;runner=@{executable=$runner;version='fixed';sha256=$hash};execution_profiles=@(@{id='fixture'})}
    $config | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $fixtureRoot 'execution.json') -Encoding UTF8
    $selected = Get-DeploymentRunnerSelection $fixtureRoot $false
    if (-not $selected.Reuse -or $selected.Executable -ne $runner -or $selected.SHA256 -ne $hash) { throw 'Ordinary deployment did not reuse the installed release.' }
    if ((Get-DeploymentRunnerSelection $fixtureRoot $true).Reuse) { throw 'Explicit Runner build was ignored.' }
    New-Item -ItemType Directory -Path (Join-Path $fixtureRoot 'application-api') | Out-Null
    @{sha256=$hash} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $fixtureRoot 'application-api\runner-build.json') -Encoding UTF8
    Assert-DeploymentRunnerBuild $fixtureRoot $hash
    $failed = $false
    try { Assert-DeploymentRunnerBuild $fixtureRoot ('a'*64) } catch { $failed = $true }
    if (-not $failed) { throw 'A new Runner build was allowed to replace an existing runtime.' }
    $script:takeoverDir = Join-Path $fixtureRoot 'original-runner-state'
    $script:takeoverFail = $false
    $script:takeoverWrongTarget = $false
    $script:takeoverCalls = 0
    function Invoke-TakeoverFixture {
        $script:takeoverCalls++
        $expected = 'verify-runner-takeover|--runner-exe|staged-runner|--runner-sha256|'+('a'*64)+'|--state-dir|'+$script:takeoverDir+'|--json'
        if (($args -join '|') -ne $expected) { throw 'Online preflight did not delegate to the staged Go verifier with original binding.' }
        $global:LASTEXITCODE = [int]$script:takeoverFail
        if ($script:takeoverFail) { return }
        $sha = if ($script:takeoverWrongTarget) { 'foreign' } else { 'a'*64 }
        @{candidate_sha256=$sha;state_dir=$script:takeoverDir;contract='steerrunner-takeover/v1';compatible=$true;items=@()} | ConvertTo-Json -Compress
    }
    @{sha256=$hash;state_dir=$script:takeoverDir} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $fixtureRoot 'application-api\runner-build.json') -Encoding UTF8
    $proof = Assert-DeploymentRunnerBuild $fixtureRoot ('a'*64) 'staged-runner' 'Invoke-TakeoverFixture'
    if ($proof.candidate_sha256 -ne ('a'*64) -or $script:takeoverCalls -ne 1) { throw 'Online preflight did not consume Go proof.' }
    foreach ($rejection in @('failure','foreign')) {
        $script:takeoverFail = $rejection -eq 'failure'
        $script:takeoverWrongTarget = $rejection -eq 'foreign'
        $failed = $false
        try { Assert-DeploymentRunnerBuild $fixtureRoot ('a'*64) 'staged-runner' 'Invoke-TakeoverFixture' } catch { $failed = $true }
        if (-not $failed) { throw "Online preflight accepted $rejection result." }
    }
    if ((Get-FileHash -LiteralPath $runner -Algorithm SHA256).Hash -ne $hash) { throw 'Preflight replaced the installed fixture.' }
    [IO.File]::WriteAllText($runner, 'different bytes')
    $failed = $false
    try { Get-DeploymentRunnerSelection $fixtureRoot $false } catch { $failed = $true }
    if (-not $failed) { throw 'A corrupted installed Runner pin was reused.' }
} finally {
    $resolved = [IO.Path]::GetFullPath($fixtureRoot)
    $expectedPrefix = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')+'\steerline-deploy-fixture-'
    if (-not $resolved.StartsWith($expectedPrefix,[StringComparison]::OrdinalIgnoreCase)) { throw 'Test cleanup escaped its temporary fixture root.' }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
Write-Output 'Passed: strict settlement, compatible takeover, pinned Runner reuse, and failed-verification gates.'
