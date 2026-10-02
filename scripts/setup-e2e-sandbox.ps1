<#
Initialize the reusable native Windows sandbox for live E2E tests.
No model calls. Existing valid setup is reused without requesting elevation.
Requires the project's Go toolchain and pinned Codex CLI on PATH.
#>
[CmdletBinding()]
param(
    [string]$CodexHome = $(if ($env:CODEX_HOME) { $env:CODEX_HOME } else { Join-Path $env:USERPROFILE '.codex' }),
    [string]$CodexCommand = 'codex',
    [switch]$CheckOnly,
    [ValidateRange(30, 900)][int]$TimeoutSeconds = 300
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$goCommand = (Get-Command go -CommandType Application -ErrorAction Stop | Select-Object -First 1).Source
$setupArgs = @('run', './tests/e2e/cmd/sandbox-setup', '-codex-home', $CodexHome, '-codex', $CodexCommand, '-timeout', ($TimeoutSeconds.ToString() + 's'))
if ($CheckOnly) { $setupArgs += '-check-only' }

Push-Location -LiteralPath $projectRoot
try {
    & $goCommand @setupArgs
    if ($LASTEXITCODE -ne 0) {
        throw "Sandbox setup/check failed (exit $LASTEXITCODE). No model request was made."
    }
}
finally {
    Pop-Location
}
