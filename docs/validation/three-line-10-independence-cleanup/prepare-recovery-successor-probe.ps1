$ErrorActionPreference = 'Stop'

$expectedHead = '8da65c7fb81dddc2300350b202efdf9d47e9f03e'
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\..'))
$head = (& git -C $repoRoot rev-parse HEAD).Trim()
$tree = (& git -C $repoRoot rev-parse 'HEAD^{tree}').Trim()
if ($head -ne $expectedHead) {
    throw "Recovery probe source mismatch: got $head, expected $expectedHead"
}
$probeDir = Join-Path $PSScriptRoot 'probes\recovery-successor-base-v2'
$relative = 'tests/e2e/suites/recovery/waiting_merge_restart_test.go'
$source = Join-Path $repoRoot $relative
$before = Join-Path $probeDir 'source-before.go.txt'
$mutated = Join-Path $probeDir 'source-mutated.go.txt'
$patchPath = Join-Path $probeDir 'patch.diff'
$manifestPath = Join-Path $probeDir 'manifest.json'
$overlayPath = Join-Path $probeDir 'overlay.json'
foreach ($path in @($before, $mutated, $patchPath, $manifestPath, $overlayPath)) {
    if (Test-Path -LiteralPath $path) {
        throw "Refusing to overwrite existing probe evidence: $path"
    }
}

$sourceText = [IO.File]::ReadAllText($source)
$pattern = '(?m)\tif len\(delivery\) == 0 \{\r?\n\t\tt\.Fatal\("missing delivery"\)\r?\n\t\}\r?\n\t+again, err := p\.Snapshot\(\)'
if ([regex]::Matches($sourceText, $pattern).Count -ne 1) {
    throw 'Expected the existing delivery/reconciliation tail exactly once'
}
$insert = @'
	if len(closed.Domain.Deliveries) != 1 || closed.Domain.Deliveries[0].MergeCommit == "" {
		t.Fatal("original delivery has no unique merge commit")
	}
	deliveryMerge := closed.Domain.Deliveries[0].MergeCommit
	startErr := p.Driver.Control(env.Context, env.Run.Helper, "start_task", p.Tasks["b"].ID)
	if startErr != nil {
		diagnostic, readErr := p.Snapshot()
		if readErr == nil {
			p.Record("successor-start-diagnostic", diagnostic)
			for _, workflow := range diagnostic.Domain.WorkflowRuns {
				if workflow.TaskID != p.Tasks["b"].ID {
					continue
				}
				for _, execution := range diagnostic.Domain.Executions {
					if execution.WorkflowRunID == workflow.WorkflowRunID && execution.Workspace.Base != "" {
						t.Logf("successor base after rejected original receipt: %s; delivery merge: %s", execution.Workspace.Base, deliveryMerge)
					}
				}
			}
		}
		t.Fatalf("one original successor start request was rejected; no retry: %v", startErr)
	}
	var successor testenv.Snapshot
	env.Wait("successor base follows delivered merge after restart", func() (bool, error) {
		var readErr error
		successor, readErr = p.Snapshot()
		if readErr != nil {
			return false, readErr
		}
		for _, workflow := range successor.Domain.WorkflowRuns {
			if workflow.TaskID != p.Tasks["b"].ID {
				continue
			}
			for _, execution := range successor.Domain.Executions {
				if execution.WorkflowRunID == workflow.WorkflowRunID && execution.Workspace.Base != "" {
					return true, nil
				}
			}
		}
		return false, nil
	})
	for _, execution := range successor.Domain.Executions {
		if execution.Workspace.Base != "" && execution.WorkflowRunID != "" {
			for _, workflow := range successor.Domain.WorkflowRuns {
				if workflow.WorkflowRunID == execution.WorkflowRunID && workflow.TaskID == p.Tasks["b"].ID && execution.Workspace.Base != deliveryMerge {
					t.Fatalf("successor base %s differs from delivery merge %s", execution.Workspace.Base, deliveryMerge)
				}
			}
		}
	}
	if len(successor.Domain.Deliveries) != 1 || successor.Domain.Deliveries[0].MergeCommit != deliveryMerge {
		t.Fatalf("successor start changed original Delivery: %+v", successor.Domain.Deliveries)
	}
	p.Record("successor-base", successor)
	again, err := p.Snapshot()
'@
$insert = $insert -replace "`n", "`r`n"
$mutatedText = [regex]::Replace($sourceText, $pattern, { param($m) $insert }, 1)
if ($mutatedText -ceq $sourceText) {
    throw 'Recovery probe patch did not change the source'
}
Copy-Item -LiteralPath $source -Destination $before
[IO.File]::WriteAllText($mutated, $mutatedText, [Text.UTF8Encoding]::new($false))
$patchLines = & git -C $repoRoot --no-pager diff --no-index --no-prefix -- $before $mutated
if ($LASTEXITCODE -ne 1) {
    throw "Expected one recovery probe diff, got exit $LASTEXITCODE"
}
[IO.File]::WriteAllText($patchPath, (($patchLines -join "`n") + "`n"), [Text.UTF8Encoding]::new($false))
$replace = @{}
$replace[[IO.Path]::GetFullPath($source)] = [IO.Path]::GetFullPath($mutated)
[IO.File]::WriteAllText($overlayPath, (ConvertTo-Json @{Replace=$replace} -Depth 6) + "`n", [Text.UTF8Encoding]::new($false))
$manifest = [ordered]@{
    schema_version = 1
    source_commit = $head
    source_tree = $tree
    source_path = $relative
    source_sha256_before = (Get-FileHash $before -Algorithm SHA256).Hash.ToLowerInvariant()
    source_sha256_mutated = (Get-FileHash $mutated -Algorithm SHA256).Hash.ToLowerInvariant()
    patch_sha256 = (Get-FileHash $patchPath -Algorithm SHA256).Hash.ToLowerInvariant()
    selected_scenario = 'recovery.waiting-merge-restart'
    selected_parent = 'TestWaitingMergeRestart'
    test_mutation = 'After the original restart/reconciliation and unique delivery, submit one new start_task for the ready successor, read its actual execution base, and do not retry if the receipt is rejected.'
    expected_behavior = 'Pass only if the one original product request is accepted and successor Workspace.Base equals the original Delivery.MergeCommit; preserve any pending-operation failure as a diagnostic.'
    exact_runner_command = "go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled '-scenario=recovery.waiting-merge-restart' -out docs/validation/three-line-10-independence-cleanup/independence/recovery-successor-probe-20261006-02"
    post_probe_withdrawal = 'Restore the source file to source-before.go.txt and verify its SHA-256 and HEAD tree are unchanged.'
}
[IO.File]::WriteAllText($manifestPath, (ConvertTo-Json $manifest -Depth 8) + "`n", [Text.UTF8Encoding]::new($false))
Copy-Item -LiteralPath $mutated -Destination $source -Force
"probe=$relative before=$($manifest.source_sha256_before) mutated=$($manifest.source_sha256_mutated) patch=$($manifest.patch_sha256)"
