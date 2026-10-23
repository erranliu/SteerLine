param(
    [Parameter(Mandatory)][string]$SteerRunnerExe,
    [Parameter(Mandatory)][string]$ExecutionModel,
    [Parameter(Mandatory)][string]$AuditModel,
    [string]$OutDir = '',
    [switch]$IndependentProcess
)
$ErrorActionPreference = 'Stop'
if (-not [IO.Path]::IsPathRooted($SteerRunnerExe) -or -not (Test-Path -LiteralPath $SteerRunnerExe -PathType Leaf)) { throw 'Select an existing absolute Runner EXE' }
if (-not $ExecutionModel.Trim() -or -not $AuditModel.Trim() -or $ExecutionModel -ceq $AuditModel) { throw 'Select two distinct explicit models' }
if (-not $IndependentProcess) {
    . (Join-Path $PSScriptRoot 'independent-test.ps1')
    $arguments = @('-SteerRunnerExe', $SteerRunnerExe, '-ExecutionModel', $ExecutionModel, '-AuditModel', $AuditModel)
    if ($OutDir) { $arguments += @('-OutDir', $OutDir) }
    Invoke-IndependentTest -ScriptPath $PSCommandPath -TestArguments $arguments -Timeout ([TimeSpan]::FromMinutes(45))
    return
}
$savedRunner = $env:STEERRUNNER_EXE
$savedExecution = $env:STEERLINE_AUDIT_EXECUTION_MODEL
$savedAudit = $env:STEERLINE_AUDIT_MODEL
Push-Location ([IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..')))
try {
    $env:STEERRUNNER_EXE = $SteerRunnerExe
    $env:STEERLINE_AUDIT_EXECUTION_MODEL = $ExecutionModel
    $env:STEERLINE_AUDIT_MODEL = $AuditModel
    $testArguments = @('run', './cmd/test-feedback', '-level=L4', '-suite=workflow', '-scenario=workflow.local-codex-audited', '-env=live')
    if ($OutDir) { $testArguments += ('-out=' + $OutDir) }
    $savedErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        & go @testArguments 2>&1 | ForEach-Object { $_.ToString() }
        $testExit = $LASTEXITCODE
    } finally { $ErrorActionPreference = $savedErrorAction }
    if ($testExit -ne 0) { throw 'Audited workflow failed; preserve test-feedback evidence.' }
} finally {
    $env:STEERRUNNER_EXE = $savedRunner
    $env:STEERLINE_AUDIT_EXECUTION_MODEL = $savedExecution
    $env:STEERLINE_AUDIT_MODEL = $savedAudit
    Pop-Location
}
