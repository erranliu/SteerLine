param(
    [Parameter(Mandatory = $true)][string]$RunId,
    [Parameter(Mandatory = $true)][string[]]$FeedbackArgs,
    [string]$TempParent
)

$ErrorActionPreference = 'Stop'

if ($RunId -notmatch '^[a-z0-9-]+$') {
    throw "RunId must contain only lowercase letters, digits and hyphens: $RunId"
}
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\..'))
$evidenceRoot = [IO.Path]::GetFullPath($PSScriptRoot)
$branch = (& git -C $repoRoot branch --show-current).Trim()
if ($branch -ne 'codex/task-82-independence-cleanup') {
    throw "Unexpected source branch: $branch"
}
$sourceCommit = (& git -C $repoRoot rev-parse HEAD).Trim()
$sourceTree = (& git -C $repoRoot rev-parse 'HEAD^{tree}').Trim()

$outValue = $null
for ($i = 0; $i -lt $FeedbackArgs.Count; $i++) {
    if ($FeedbackArgs[$i].StartsWith('-out=')) {
        $outValue = $FeedbackArgs[$i].Substring(5)
        break
    }
    if ($FeedbackArgs[$i] -eq '-out' -and $i + 1 -lt $FeedbackArgs.Count) {
        $outValue = $FeedbackArgs[$i + 1]
        break
    }
}
if ([string]::IsNullOrWhiteSpace($outValue)) {
    throw 'Every run must provide an explicit -out path.'
}
$outPath = [IO.Path]::GetFullPath((Join-Path $repoRoot $outValue))
$evidencePrefix = $evidenceRoot.TrimEnd('\') + '\'
if (-not $outPath.StartsWith($evidencePrefix, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Refusing an output path outside this task evidence directory: $outPath"
}
if (Test-Path -LiteralPath $outPath) {
    throw "Refusing to overwrite an existing run directory: $outPath"
}

$tempParentInput = if ([string]::IsNullOrWhiteSpace($TempParent)) { $env:TEMP } else { $TempParent }
$tempParent = [IO.Path]::GetFullPath($tempParentInput)
if (-not (Test-Path -LiteralPath $tempParent -PathType Container)) {
    throw "Temporary parent must already exist: $tempParent"
}
$tempPrefix = $tempParent.TrimEnd('\') + '\'
$runRoot = [IO.Path]::GetFullPath((Join-Path $tempParent "sl82-$RunId"))
if (-not $runRoot.StartsWith($tempPrefix, [StringComparison]::OrdinalIgnoreCase) -or $runRoot -eq $tempParent) {
    throw "Temporary root did not resolve inside the existing TEMP directory: $runRoot"
}
if (Test-Path -LiteralPath $runRoot) {
    throw "Refusing to reuse a temporary test root: $runRoot"
}
$homePath = Join-Path $runRoot 'h'
$scratchPath = Join-Path $runRoot 't'
New-Item -ItemType Directory -Path $runRoot | Out-Null
New-Item -ItemType Directory -Path $homePath | Out-Null
New-Item -ItemType Directory -Path $scratchPath | Out-Null

$savedSteerline = @{}
$isolatedSteerlineNameCount = 0
$nativeOptInNames = @()
foreach ($item in Get-ChildItem Env:) {
    if ($item.Name.StartsWith('STEERLINE_', [StringComparison]::OrdinalIgnoreCase)) {
        $savedSteerline[$item.Name] = $item.Value
        $isolatedSteerlineNameCount++
        if ($item.Name -match '^STEERLINE_(REAL_|NATIVE_|LOCAL_CAPABILITY_|LIVE_|RUN_WINDOW_|WINDOW_PROBE_|CODEX_HISTORY_NATIVE_)' -or $item.Name -eq 'STEERLINE_CC5_MANIFEST' -or $item.Name -eq 'STEERLINE_ORCA_HOST_WAIT_PROBE') {
            $nativeOptInNames += $item.Name
        }
        [Environment]::SetEnvironmentVariable($item.Name, $null, 'Process')
    }
}
$savedTemp = $env:TEMP
$savedTmp = $env:TMP
$env:STEERLINE_HOME = $homePath
$env:TEMP = $scratchPath
$env:TMP = $scratchPath

$stdoutPath = Join-Path $runRoot 'stdout.txt'
$stderrPath = Join-Path $runRoot 'stderr.txt'
$startedAt = [DateTime]::UtcNow.ToString('o')
$goPath = (Get-Command go).Source
$goVersion = (& go version).Trim()
$goOS = (& go env GOOS).Trim()
$goArch = (& go env GOARCH).Trim()
$goFlags = (& go env GOFLAGS).Trim()
$goArgs = @('run', './cmd/test-feedback') + $FeedbackArgs
$actualExit = 127
$executionError = $null
$environmentRestored = $false

try {
    & go @goArgs 1> $stdoutPath 2> $stderrPath
    $actualExit = $LASTEXITCODE
} catch {
    $executionError = $_.ToString()
    if (-not (Test-Path -LiteralPath $stdoutPath)) { [IO.File]::WriteAllText($stdoutPath, '') }
    if (-not (Test-Path -LiteralPath $stderrPath)) { [IO.File]::WriteAllText($stderrPath, $executionError) }
} finally {
    $env:TEMP = $savedTemp
    $env:TMP = $savedTmp
    [Environment]::SetEnvironmentVariable('STEERLINE_HOME', $null, 'Process')
    foreach ($name in $savedSteerline.Keys) {
        [Environment]::SetEnvironmentVariable($name, $savedSteerline[$name], 'Process')
    }
    $environmentRestored = $true
}

$finishedAt = [DateTime]::UtcNow.ToString('o')
if (-not (Test-Path -LiteralPath $outPath)) {
    New-Item -ItemType Directory -Path $outPath | Out-Null
}
Copy-Item -LiteralPath $stdoutPath -Destination (Join-Path $outPath 'invocation.stdout.txt')
Copy-Item -LiteralPath $stderrPath -Destination (Join-Path $outPath 'invocation.stderr.txt')
$invocation = [ordered]@{
    schema_version = 1
    run_id = $RunId
    source_commit = $sourceCommit
    source_tree = $sourceTree
    working_tree_head_after_run = (& git -C $repoRoot rev-parse HEAD).Trim()
    started_at = $startedAt
    finished_at = $finishedAt
    true_process_exit_code = $actualExit
    invocation_error = $executionError
    command = @('go') + $goArgs
    go_executable = $goPath
    go_version = $goVersion
    goos = $goOS
    goarch = $goArch
    goflags = $goFlags
    steerline_home = $homePath
    temp = $scratchPath
    tmp = $scratchPath
    temp_parent = $tempParent
    isolated_steerline_environment_variable_count = $isolatedSteerlineNameCount
    native_opt_in_variables_disabled = @($nativeOptInNames | Sort-Object)
    environment_restored_before_return = $environmentRestored
    output_directory = $outPath
    temporary_root = $runRoot
    temporary_root_cleanup_status = 'pending-explicit-path-check'
}
$invocationJson = ConvertTo-Json -InputObject $invocation -Depth 8
[IO.File]::WriteAllText((Join-Path $outPath 'invocation.json'), $invocationJson + "`n", [Text.UTF8Encoding]::new($false))

"run=$RunId exit=$actualExit source=$sourceCommit tree=$sourceTree out=$outPath temp=$runRoot"
if ($executionError) {
    [Console]::Error.WriteLine($executionError)
}
exit $actualExit
