param(
    [string]$OutputDirectory = (Join-Path $env:TEMP 'steerline-worker-flow-local-probe'),
    [string]$GoExecutable = 'C:\Users\Administrator\.local\bin\go.exe',
    [string]$GitDirectory = 'C:\Program Files\Git\cmd',
    [string]$TestPattern = '^TestLocalWorkerTurnResumesExactSessionInSameWorkspace$'
)

$ErrorActionPreference = 'Stop'
$repository = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..\..\..')).Path
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
$env:STEERLINE_REQUIRE_DETACHED_TESTS = '1'
Set-Location -LiteralPath $repository

$log = Join-Path $OutputDirectory 'go-test.txt'
$result = 1
try {
    if (-not (Test-Path -LiteralPath $GoExecutable) -or -not (Test-Path -LiteralPath (Join-Path $GitDirectory 'git.exe'))) {
        throw 'The isolated host does not have the pinned Go or Git executable.'
    }
    $env:PATH = "$GitDirectory;$env:PATH"
    & $GoExecutable test ./internal/steerline -run $TestPattern -count=1 -v *> $log
    $result = $LASTEXITCODE
} catch {
    $_ | Out-String | Set-Content -LiteralPath $log
}
[ordered]@{
    tested_at = (Get-Date).ToUniversalTime().ToString('o')
    repository = $repository
    test = $TestPattern
    exit_code = $result
    log = $log
} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $OutputDirectory 'summary.json')
exit $result
