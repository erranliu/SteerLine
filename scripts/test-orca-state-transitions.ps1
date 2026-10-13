param(
    [Parameter(Mandatory=$true)][string]$PackageRoot,
    [Parameter(Mandatory=$true)][string]$OrcaExecutable
)
$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath($PackageRoot).TrimEnd('\')
if (-not $root.StartsWith('C:\SteerlineDebug\', [StringComparison]::OrdinalIgnoreCase)) {
    throw 'The probe must use a generated package under C:\SteerlineDebug.'
}
if (-not (Test-Path -LiteralPath (Join-Path $root 'manifest.json'))) { throw 'Generated package manifest is missing.' }
$project = Join-Path $root 'projects\SteerlineTest'
if (-not (Test-Path -LiteralPath $project)) { throw 'Generated SteerlineTest repository is missing.' }
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$savedRoot = $env:STEERLINE_ORCA_STATE_PROBE_ROOT
$savedWorktree = $env:STEERLINE_ORCA_STATE_PROBE_WORKTREE
$savedRuntime = $env:STEERLINE_ORCA_STATE_PROBE_RUNTIME
$savedCLI = $env:STEERLINE_ORCA_STATE_PROBE_CLI
$savedEnvironment = $env:ORCA_ENVIRONMENT
$savedPairing = $env:ORCA_PAIRING_CODE
$savedRemotePairing = $env:ORCA_REMOTE_PAIRING
try {
    Remove-Item Env:ORCA_ENVIRONMENT,Env:ORCA_PAIRING_CODE,Env:ORCA_REMOTE_PAIRING -ErrorAction SilentlyContinue
    $registration = & $OrcaExecutable repo add --path $project --json
    if ($LASTEXITCODE -ne 0) { throw 'Orca could not register the probe repository.' }
    $registered = ($registration | Out-String | ConvertFrom-Json)
    if (-not $registered.ok -or -not $registered.result.repo.id -or -not $registered._meta.runtimeId) { throw 'Orca returned no probe repository/runtime identity.' }
    $env:STEERLINE_ORCA_STATE_PROBE_ROOT = $root
    $env:STEERLINE_ORCA_STATE_PROBE_WORKTREE = 'id:' + $registered.result.repo.id + '::' + $project.Replace('\', '/')
    $env:STEERLINE_ORCA_STATE_PROBE_RUNTIME = $registered._meta.runtimeId
    $env:STEERLINE_ORCA_STATE_PROBE_CLI = [IO.Path]::GetFullPath($OrcaExecutable)
    $env:ORCA_PAIRING_CODE = $savedPairing
    $env:ORCA_REMOTE_PAIRING = $savedRemotePairing
    Push-Location $repo
    try {
        & go run ./cmd/test-feedback -- ./internal/execution/adapters/orcatransport -run '^TestOrcaNativeStateLiveLifecycle$' -count=1 -v -timeout=6m
        $probeExit = $LASTEXITCODE
    } finally { Pop-Location }
    if ($probeExit -ne 0) {
        throw "Native state capability did not pass. See $root\evidence for input, signal and cleanup evidence."
    }
} finally {
    $env:STEERLINE_ORCA_STATE_PROBE_ROOT = $savedRoot
    $env:STEERLINE_ORCA_STATE_PROBE_WORKTREE = $savedWorktree
    $env:STEERLINE_ORCA_STATE_PROBE_RUNTIME = $savedRuntime
    $env:STEERLINE_ORCA_STATE_PROBE_CLI = $savedCLI
    $env:ORCA_ENVIRONMENT = $savedEnvironment
    $env:ORCA_PAIRING_CODE = $savedPairing
    $env:ORCA_REMOTE_PAIRING = $savedRemotePairing
}
