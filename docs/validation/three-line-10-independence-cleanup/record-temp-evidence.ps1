param(
    [Parameter(Mandatory = $true)][string]$RunId,
    [Parameter(Mandatory = $true)][string]$OutputDirectory
)

$ErrorActionPreference = 'Stop'
if ($RunId -notmatch '^[a-z0-9-]+$') {
    throw "RunId must contain only lowercase letters, digits and hyphens: $RunId"
}
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\..'))
$evidenceRoot = [IO.Path]::GetFullPath($PSScriptRoot)
$outPath = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
if (-not $outPath.StartsWith($evidenceRoot.TrimEnd('\') + '\', [StringComparison]::OrdinalIgnoreCase)) {
    throw "Output is outside this task evidence root: $outPath"
}
$invocationPath = Join-Path $outPath 'invocation.json'
$invocation = Get-Content -LiteralPath $invocationPath -Raw | ConvertFrom-Json
$resolvedScratch = [IO.Path]::GetFullPath($invocation.temp)
$resolvedRoot = [IO.Path]::GetFullPath((Split-Path -Parent $resolvedScratch))
$tempParent = [IO.Path]::GetFullPath((Split-Path -Parent $resolvedRoot))
if ((Split-Path -Leaf $resolvedRoot) -ne "sl82-$RunId" -or $resolvedScratch -ne (Join-Path $resolvedRoot 't') -or -not $resolvedRoot.StartsWith($tempParent.TrimEnd('\') + '\', [StringComparison]::OrdinalIgnoreCase)) {
    throw "Temporary path failed literal ownership validation: $resolvedRoot"
}
if (-not (Test-Path -LiteralPath $resolvedRoot)) {
    throw "Expected per-run temporary root is missing: $resolvedRoot"
}
$rootItem = Get-Item -LiteralPath $resolvedRoot -Force
if ($rootItem.Attributes.HasFlag([IO.FileAttributes]::ReparsePoint)) {
    throw "Temporary root is a reparse point; preserve it: $resolvedRoot"
}
$children = @(Get-ChildItem -LiteralPath $resolvedRoot -Force)
$expectedChildren = @('h', 't', 'stdout.txt', 'stderr.txt')
$unexpected = @($children | Where-Object { $_.Name -notin $expectedChildren })
if ($unexpected.Count -gt 0) {
    throw "Unexpected files appeared in the run-owned temp root; preserve it: $($unexpected.Name -join ', ')"
}
$homePath = Join-Path $resolvedRoot 'h'
$scratchPath = Join-Path $resolvedRoot 't'
$homeRemainders = @(Get-ChildItem -LiteralPath $homePath -Force -Recurse)
$scratchRemainders = @(Get-ChildItem -LiteralPath $scratchPath -Force -Recurse)
$summaryPath = Join-Path $outPath 'summary.json'
$summary = Get-Content -LiteralPath $summaryPath -Raw | ConvertFrom-Json
if ($summary.PSObject.Properties.Name -contains 'summary' -and $null -ne $summary.summary) {
    $runnerStatus = $summary.summary.status
    $complete = $summary.summary.complete
    $trueExit = $summary.summary.command_exit_code
    $parentTests = $summary.summary.test_count
    $passedTests = $summary.summary.passed_tests
    $failedTests = $summary.summary.failed_tests
    $skippedTests = $summary.summary.skipped_tests
} elseif ($summary.PSObject.Properties.Name -contains 'cleanup_complete') {
    $groupReports = @($summary.groups)
    $groupsComplete = $groupReports.Count -gt 0 -and @($groupReports | Where-Object { -not $_.summary.complete }).Count -eq 0
    $complete = [bool]$summary.cleanup_complete -and $groupsComplete
    $runnerStatus = if ($summary.passed -and $complete) { 'passed' } elseif ($complete) { 'failed' } else { 'incomplete' }
    $trueExit = $invocation.true_process_exit_code
    $parentTests = [int](($summary.groups | ForEach-Object { $_.summary.test_count } | Measure-Object -Sum).Sum)
    $passedTests = [int](($summary.groups | ForEach-Object { $_.summary.passed_tests } | Measure-Object -Sum).Sum)
    $failedTests = [int](($summary.groups | ForEach-Object { $_.summary.failed_tests } | Measure-Object -Sum).Sum)
    $skippedTests = [int](($summary.groups | ForEach-Object { $_.summary.skipped_tests } | Measure-Object -Sum).Sum)
} else {
    $complete = $summary.complete
    $runnerStatus = if ($summary.passed -and $complete) { 'passed' } else { 'failed_or_incomplete' }
    $trueExit = $invocation.true_process_exit_code
    $parentTests = [int](($summary.groups | ForEach-Object { $_.summary.test_count } | Measure-Object -Sum).Sum)
    $passedTests = [int](($summary.groups | ForEach-Object { $_.summary.passed_tests } | Measure-Object -Sum).Sum)
    $failedTests = [int](($summary.groups | ForEach-Object { $_.summary.failed_tests } | Measure-Object -Sum).Sum)
    $skippedTests = [int](($summary.groups | ForEach-Object { $_.summary.skipped_tests } | Measure-Object -Sum).Sum)
}
$cleanupAttemptMade = $RunId -eq 'task-before-20261006-01'
$cleanupResult = if ($cleanupAttemptMade) {
    'The exact-root recursive cleanup command was rejected by the shell tool before execution. No alternative or file-by-file removal was attempted.'
} else {
    'The first exact-root recursive cleanup command was rejected; this root is preserved under the no-bypass rule. No removal attempt was made for this run.'
}
$record = [ordered]@{
    run_id = $RunId
    source_commit = $summary.environment.source_commit
    runner_status = $runnerStatus
    runner_complete = $complete
    true_exit = $trueExit
    parent_tests = $parentTests
    passed_tests = $passedTests
    failed_tests = $failedTests
    skipped_tests = $skippedTests
    temporary_root = $resolvedRoot
    resolved_root_matches_literal = $true
    inside_temp_parent = $true
    home_contents_after_runner = @($homeRemainders | ForEach-Object { $_.FullName })
    temp_contents_after_runner = @($scratchRemainders | ForEach-Object { $_.FullName })
    home_and_temp_empty = ($homeRemainders.Count -eq 0 -and $scratchRemainders.Count -eq 0)
    recursive_cleanup_command = 'Remove-Item -LiteralPath <verified exact run root> -Recurse -Force'
    recursive_cleanup_attempted_on_this_root = $cleanupAttemptMade
    recursive_cleanup_result = $cleanupResult
    temporary_root_removed = $false
}
$recordJson = ConvertTo-Json -InputObject $record -Depth 8
[IO.File]::WriteAllText((Join-Path $outPath 'environment-cleanup.json'), $recordJson + "`n", [Text.UTF8Encoding]::new($false))

$normalizedInvocation = [ordered]@{
    schema_version = $invocation.schema_version
    run_id = $invocation.run_id
    source_commit = $invocation.source_commit
    source_tree = $invocation.source_tree
    working_tree_head_after_run = $invocation.working_tree_head_after_run
    started_at = $invocation.started_at
    finished_at = $invocation.finished_at
    true_process_exit_code = $invocation.true_process_exit_code
    invocation_error = $invocation.invocation_error
    command = $invocation.command
    go_executable = $invocation.go_executable
    go_version = $invocation.go_version
    goos = $invocation.goos
    goarch = $invocation.goarch
    goflags = $invocation.goflags
    steerline_home = $invocation.steerline_home
    temp = $invocation.temp
    tmp = $invocation.tmp
    temp_parent = $invocation.temp_parent
    isolated_steerline_environment_variable_count = $invocation.isolated_steerline_environment_variable_count
    native_opt_in_variables_disabled = @($invocation.native_opt_in_variables_disabled)
    environment_restored_before_return = $invocation.environment_restored_before_return
    output_directory = $invocation.output_directory
    temporary_root = $resolvedRoot
    temporary_root_cleanup_status = 'preserved after shell cleanup rejection; no workaround attempted'
}
[IO.File]::WriteAllText($invocationPath, ((ConvertTo-Json -InputObject $normalizedInvocation -Depth 8) + "`n"), [Text.UTF8Encoding]::new($false))
"run=$RunId status=$runnerStatus exit=$trueExit tests=$parentTests fail=$failedTests skip=$skippedTests scratch_empty=$($record.home_and_temp_empty) temp_root_preserved=true"
