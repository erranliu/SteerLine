param(
    [string]$OutputDirectory = (Join-Path $env:TEMP 'steerline-worker-flow-local-probe')
)

$ErrorActionPreference = 'Stop'
$repository = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..\..\..')).Path
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
$env:STEERLINE_REQUIRE_DETACHED_TESTS = '1'
Set-Location -LiteralPath $repository

$log = Join-Path $OutputDirectory 'go-test.txt'
& go test ./internal/steerline -run '^TestLocalWorkerTurnResumesExactSessionInSameWorkspace$' -count=1 -v *> $log
$result = $LASTEXITCODE
[ordered]@{
    tested_at = (Get-Date).ToUniversalTime().ToString('o')
    repository = $repository
    test = 'TestLocalWorkerTurnResumesExactSessionInSameWorkspace'
    exit_code = $result
    log = $log
} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $OutputDirectory 'summary.json')
exit $result
