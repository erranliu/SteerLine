param([switch]$ExecutionActiveBranchOnly)

$ErrorActionPreference = 'Stop'

$sourceCommitExpected = 'd87a714baeea5aa8a8d0173deb5a232cf0ba1d47'
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\..'))
$sourceCommit = (& git -C $repoRoot rev-parse HEAD).Trim()
$sourceTree = (& git -C $repoRoot rev-parse 'HEAD^{tree}').Trim()
if ($sourceCommit -ne $sourceCommitExpected) {
    throw "Probe source commit mismatch: got $sourceCommit, expected $sourceCommitExpected"
}

function Write-ProbeEvidence {
    param(
        [string]$name,
        [string]$sourceRelative,
        [string]$testPackage,
        [string]$testSelection,
        [string]$mutationDescription,
        [scriptblock]$mutator
    )
    $probeDir = Join-Path $PSScriptRoot "probes\$name"
    $sourcePath = [IO.Path]::GetFullPath((Join-Path $repoRoot $sourceRelative))
    $beforePath = Join-Path $probeDir 'source-before.go.txt'
    $mutatedPath = Join-Path $probeDir 'source-mutated.go.txt'
    $patchPath = Join-Path $probeDir 'patch.diff'
    $overlayPath = Join-Path $probeDir 'overlay.json'
    $manifestPath = Join-Path $probeDir 'manifest.json'
    foreach ($path in @($beforePath, $mutatedPath, $patchPath, $overlayPath, $manifestPath)) {
        if (Test-Path -LiteralPath $path) {
            throw "Refusing to replace existing probe evidence: $path"
        }
    }

    $beforeText = [IO.File]::ReadAllText($sourcePath)
    $mutatedText = & $mutator $beforeText
    if ([string]::IsNullOrEmpty($mutatedText) -or $mutatedText -ceq $beforeText) {
        throw "Mutation did not produce a distinct source file for $name"
    }
    Copy-Item -LiteralPath $sourcePath -Destination $beforePath
    [IO.File]::WriteAllText($mutatedPath, $mutatedText, [Text.UTF8Encoding]::new($false))

    $patchLines = & git -C $repoRoot --no-pager diff --no-index --no-prefix -- $beforePath $mutatedPath
    $patchExit = $LASTEXITCODE
    if ($patchExit -ne 1) {
        throw "Expected one source patch for $name, got git diff exit $patchExit"
    }
    [IO.File]::WriteAllText($patchPath, (($patchLines -join "`n") + "`n"), [Text.UTF8Encoding]::new($false))

    $replace = @{}
    $replace[$sourcePath] = [IO.Path]::GetFullPath($mutatedPath)
    $overlay = @{ Replace = $replace }
    $overlayText = ConvertTo-Json -InputObject $overlay -Depth 6
    [IO.File]::WriteAllText($overlayPath, $overlayText + "`n", [Text.UTF8Encoding]::new($false))

    $out = "docs/validation/three-line-10-independence-cleanup/independence/$name-mutated-20261006-01"
    $command = "go run ./cmd/test-feedback -out $out -- -overlay=docs/validation/three-line-10-independence-cleanup/probes/$name/overlay.json $testPackage -run '$testSelection' -count=1"
    $manifest = [ordered]@{
        schema_version = 1
        owner = $name
        source_commit = $sourceCommit
        source_tree_before = $sourceTree
        source_tree_after_generation = (& git -C $repoRoot rev-parse 'HEAD^{tree}').Trim()
        source_path = $sourceRelative
        source_absolute_path = $sourcePath
        source_sha256_before = (Get-FileHash -LiteralPath $beforePath -Algorithm SHA256).Hash.ToLowerInvariant()
        source_sha256_mutated = (Get-FileHash -LiteralPath $mutatedPath -Algorithm SHA256).Hash.ToLowerInvariant()
        source_sha256_in_worktree_after_generation = (Get-FileHash -LiteralPath $sourcePath -Algorithm SHA256).Hash.ToLowerInvariant()
        mutation = $mutationDescription
        selected_test = $testSelection
        overlay_path = "probes/$name/overlay.json"
        patch_path = "probes/$name/patch.diff"
        patch_sha256 = (Get-FileHash -LiteralPath $patchPath -Algorithm SHA256).Hash.ToLowerInvariant()
        expected_mutation_exit = 1
        exact_test_feedback_command = $command
    }
    $manifestText = ConvertTo-Json -InputObject $manifest -Depth 8
    [IO.File]::WriteAllText($manifestPath, $manifestText + "`n", [Text.UTF8Encoding]::new($false))
    "[$name] before=$($manifest.source_sha256_before) mutated=$($manifest.source_sha256_mutated) patch=$($manifest.patch_sha256)"
}

$taskMutator = {
    param($text)
    $old = 'return x.Priority < y.Priority'
    if ([regex]::Matches($text, [regex]::Escape($old)).Count -ne 1) {
        throw 'Expected exactly one task priority comparison'
    }
    $text.Replace($old, 'return x.Priority > y.Priority')
}

$executionMutator = {
    param($text)
    $pattern = '(?m)(?<indent>\t*)if err := s\.resumeRetiredCandidate\(ctx, run\); err != nil \{\r?\n(?<body>\t*)resumeErr = errors\.Join\(resumeErr, err\)\r?\n(?<close>\t*)\}'
    if ([regex]::Matches($text, $pattern).Count -ne 1) {
        throw 'Expected exactly one retired-candidate error aggregation block'
    }
    $evaluator = [System.Text.RegularExpressions.MatchEvaluator]{
        param($match)
        return $match.Groups['indent'].Value + 'if err := s.resumeRetiredCandidate(ctx, run); err != nil {' + "`r`n" + $match.Groups['body'].Value + 'return err' + "`r`n" + $match.Groups['close'].Value + '}'
    }
    [regex]::Replace($text, $pattern, $evaluator, 1)
}

$consoleMutator = {
    param($text)
    $pattern = '(?m)\t*if run\.EnvironmentBlocker != nil \{\r?\n\t*dialog\.error = "环境阻断：" \+ run\.EnvironmentBlocker\.Fact\.Reason \+ "；未发送，解除后请重新提交"\r?\n\t*return m, nil\r?\n\t*\}\r?\n'
    if ([regex]::Matches($text, $pattern).Count -ne 1) {
        throw 'Expected exactly one decision-answer environmental blocker barrier'
    }
    [regex]::Replace($text, $pattern, '', 1)
}

if ($ExecutionActiveBranchOnly) {
    $executionActiveMutator = {
        param($text)
        $pattern = '(?m)(?<lineindent>\t*)_, err = s\.resumeRegisteredWorkerFlow\(ctx, task, run, adapter\)\r?\n(?<ifindent>\t*)if err != nil \{\r?\n(?<body>\t*)resumeErr = errors\.Join\(resumeErr, err\)\r?\n(?<close>\t*)\}'
        if ([regex]::Matches($text, $pattern).Count -ne 1) {
            throw 'Expected exactly one registered-worker resume error aggregation block'
        }
        $evaluator = [System.Text.RegularExpressions.MatchEvaluator]{
            param($match)
            return $match.Groups['lineindent'].Value + '_, err = s.resumeRegisteredWorkerFlow(ctx, task, run, adapter)' + "`r`n" + $match.Groups['ifindent'].Value + 'if err != nil {' + "`r`n" + $match.Groups['body'].Value + 'return err' + "`r`n" + $match.Groups['close'].Value + '}'
        }
        [regex]::Replace($text, $pattern, $evaluator, 1)
    }
    Write-ProbeEvidence 'execution-active-branch' 'internal/execution/worker_recovery.go' './internal/execution' '^(TestMC9|TestCandidateResumeErrorDoesNotStarveSibling)$' 'Return only the active registered-worker resume error before the service can visit the next sibling.' $executionActiveMutator
    return
}

Write-ProbeEvidence 'task' 'internal/task/model/taskgraph.go' './internal/task/model' 'TestStableTaskOrderUsesPriorityTimeAndNumericID' 'Invert only the task-priority comparison from ascending to descending.' $taskMutator
Write-ProbeEvidence 'execution' 'internal/execution/worker_recovery.go' './internal/execution' '^(TestMC9|TestCandidateResumeErrorDoesNotStarveSibling)$' 'Return the first retired-candidate resume error before the service visits sibling runs.' $executionMutator
Write-ProbeEvidence 'console' 'internal/console/model.go' './internal/console ./tests/integration' '^(TestUX4RejectPreservesDraftAndBlockerStaysIndependent|TestCandidateConsoleWiring)$' 'Remove only the environmental blocker barrier before decision-answer submission.' $consoleMutator
