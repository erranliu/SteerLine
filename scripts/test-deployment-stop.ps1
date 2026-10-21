$ErrorActionPreference = 'Stop'
Import-Module Microsoft.PowerShell.Utility -ErrorAction Stop
. (Join-Path $PSScriptRoot 'deployment-stop.ps1')
. (Join-Path $PSScriptRoot 'deployment-runner.ps1')

$script:calls = [Collections.Generic.List[string]]::new()
$script:stagedHash = 'same'
$script:packageDigest = 'a'*64
$script:realPackageDigest = ${function:Get-DeploymentPackageDigest}
function Get-DeploymentPackageDigest([string]$CLI, [string]$RunnerExe) {
    if ($CLI -eq 'Invoke-StagedFixture') { return $script:packageDigest }
    & $script:realPackageDigest $CLI $RunnerExe
}
$script:mode = 'valid'
$script:expectedGoal = 'replace'
$savedHome = $env:STEERLINE_HOME
$env:STEERLINE_HOME = Join-Path ([IO.Path]::GetTempPath()) 'steerline-script-test-home'
function Get-FileHash([string]$LiteralPath, [string]$Algorithm) {
    if ($LiteralPath -eq 'staged-runner') { return @{Hash=$script:stagedHash} }
    Microsoft.PowerShell.Utility\Get-FileHash -LiteralPath $LiteralPath -Algorithm $Algorithm
}
function Invoke-StagedFixture {
    $script:calls.Add(($args -join '|'))
    if ($args.Count -ne 10 -or $args[6] -notmatch '^[0-9a-f]{32}$') { throw 'Missing fresh deployment invocation.' }
    $expected = 'prepare-deployment|--runner-exe|staged-runner|--goal|'+$script:expectedGoal+'|--invocation-id|'+$args[6]+'|--package-sha256|'+$script:packageDigest+'|--json'
    if (($args -join '|') -ne $expected) { throw 'Deployment did not delegate the exact selected target to Go.' }
    $global:LASTEXITCODE = if ($script:mode -eq 'failure') { 1 } else { 0 }
    if ($global:LASTEXITCODE) { return }
    if ($script:mode -eq 'malformed') { 'not JSON'; return }
    $proof = @{contract='steerline-deployment-preparation/v1';home=$env:STEERLINE_HOME;goal=$script:expectedGoal;invocation_id=$args[6];package_sha256=$script:packageDigest;outcome='prepared';verifier_sha256=$script:stagedHash;verified_at='2026-10-21T00:00:00Z';facts=@{native_resources='unverified'}}
    switch ($script:mode) {
        'wrong-contract' { $proof.contract='foreign' }
        'wrong-home' { $proof.home=Join-Path $env:STEERLINE_HOME 'foreign' }
        'wrong-goal' { $proof.goal='reset' }
        'wrong-candidate' { $proof.verifier_sha256='foreign' }
        'wrong-invocation' { $proof.invocation_id='stale' }
        'wrong-package' { $proof.package_sha256='foreign' }
        'changing-package' { $script:packageDigest='b'*64 }
        'blocked' { $proof.outcome='blocked' }
        'missing-time' { $proof.Remove('verified_at') }
        'changing-runner' { $script:stagedHash='changed-during-call' }
    }
    $proof | ConvertTo-Json -Depth 8 -Compress
}
try {
    foreach ($script:expectedGoal in @('replace','update')) {
        $script:calls.Clear()
        $script:mode='valid'
        $script:stagedHash='candidate'
        $proof = Stop-DeploymentApplication 'Invoke-StagedFixture' 'staged-runner' -AllowRestart:($script:expectedGoal -eq 'update')
        if ($script:calls.Count -ne 1 -or $proof.goal -ne $script:expectedGoal -or $proof.facts.native_resources -ne 'unverified') { throw 'Go result was reinterpreted or lifecycle orchestration was duplicated.' }
        foreach ($script:mode in @('failure','malformed','wrong-contract','wrong-home','wrong-goal','wrong-candidate','wrong-invocation','wrong-package','changing-package','blocked','missing-time','changing-runner')) {
            $script:calls.Clear()
            $script:stagedHash='same'
            $script:packageDigest='a'*64
            $failed=$false
            try { Stop-DeploymentApplication 'Invoke-StagedFixture' 'staged-runner' -AllowRestart:($script:expectedGoal -eq 'update') } catch { $failed=$true }
            if (-not $failed -or $script:calls.Count -ne 1) { throw "Accepted $script:mode or retried deployment outside Go." }
        }
    }
    $script:calls.Clear()
    $env:STEERLINE_HOME=''
    $failed=$false
    try { Stop-DeploymentApplication 'Invoke-StagedFixture' 'staged-runner' } catch { $failed=$true }
    if (-not $failed -or $script:calls.Count) { throw 'Missing explicit home reached a mutating command.' }
} finally {
    $env:STEERLINE_HOME=$savedHome
}
$script:stagedHash='same'
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
Write-Output 'Passed: one Go preparation call, invocation/package binding, pinned Runner reuse, and takeover preflight gates.'
