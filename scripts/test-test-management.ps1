param([Parameter(Mandatory=$true)][string]$SteerRunnerExe, [string]$RunnerCandidate = '', [string]$Out = '', [switch]$IndependentProcess)
$ErrorActionPreference = 'Stop'
if ($RunnerCandidate) {
    if (-not [IO.Path]::IsPathRooted($RunnerCandidate) -or -not (Test-Path -LiteralPath $RunnerCandidate -PathType Leaf)) { throw 'RunnerCandidate must name an existing absolute executable' }
    if ((Get-FileHash -LiteralPath $RunnerCandidate).Hash -eq (Get-FileHash -LiteralPath $SteerRunnerExe).Hash) { throw 'RunnerCandidate must be a different build' }
}
if (-not $IndependentProcess) {
    . (Join-Path $PSScriptRoot 'independent-test.ps1')
    $testArguments = @('-SteerRunnerExe', $SteerRunnerExe)
    if ($RunnerCandidate) { $testArguments += @('-RunnerCandidate', $RunnerCandidate) }
    if ($Out) { $testArguments += @('-Out', $Out) }
    Invoke-IndependentTest -ScriptPath $PSCommandPath -TestArguments $testArguments -Timeout ([TimeSpan]::FromMinutes(18))
    return
}
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$release = & (Join-Path $PSScriptRoot 'new-debug-release.ps1') -SteerRunnerExe $SteerRunnerExe | ConvertFrom-Json
if (-not $release.root) { throw 'Debug package generation failed' }
if ($Out) {
    New-Item -ItemType Directory -Path $Out -Force | Out-Null
    [IO.File]::WriteAllText((Join-Path $Out 'debug-package.txt'), $release.root)
}
$env:STEERLINE_TM_SOURCE = $repo
$env:STEERLINE_TM_RUNNER_CANDIDATE = $RunnerCandidate
$ErrorActionPreference = 'Continue'
& (Join-Path $release.root 'bin/debug-helper.exe') debug testing
$testExit = $LASTEXITCODE
$ErrorActionPreference = 'Stop'
if ($Out -and (Test-Path -LiteralPath (Join-Path $release.root 'evidence/test-management/proof.json'))) {
    Copy-Item -LiteralPath (Join-Path $release.root 'evidence/test-management/proof.json') -Destination (Join-Path $Out 'proof.json')
}
if ($testExit -ne 0) { throw "Test management live failed; inspect $($release.root)/evidence/test-management" }
if ($RunnerCandidate) {
    $takeover = Get-Content -LiteralPath (Join-Path $release.root 'evidence/test-management/generation-takeover.json') -Raw | ConvertFrom-Json
    if ($takeover.passed -ne $true -or $takeover.original_sha256 -eq $takeover.candidate_sha256) { throw 'Generation takeover evidence is incomplete' }
}
