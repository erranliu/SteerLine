param([string]$OutDir = '', [switch]$IndependentProcess)
$ErrorActionPreference = 'Stop'
if (-not $IndependentProcess) {
    . (Join-Path $PSScriptRoot 'independent-test.ps1')
    $testArguments = @()
    if ($OutDir) { $testArguments += @('-OutDir', $OutDir) }
    Invoke-IndependentTest -ScriptPath $PSCommandPath -TestArguments $testArguments -Timeout ([TimeSpan]::FromMinutes(15))
    return
}
Push-Location ([IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..')))
try {
    $testArguments = @('run', './cmd/test-feedback', '-level=L2')
    if ($OutDir) { $testArguments += ('-out=' + $OutDir) }
    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        & go @testArguments 2>&1 | ForEach-Object { $_.ToString() }
        $testExit = $LASTEXITCODE
    } finally { $ErrorActionPreference = $previousErrorAction }
    if ($testExit -ne 0) { throw 'Controlled L2 failed; preserve the test-feedback evidence.' }
} finally { Pop-Location }
