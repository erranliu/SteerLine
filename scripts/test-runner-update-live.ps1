param(
    [Parameter(Mandatory=$true)][string]$DebugPackage,
    [Parameter(Mandatory=$true)][string]$RunnerCandidate,
    [Parameter(Mandatory=$true)][string]$AgentExecutable,
    [switch]$ContinuePrepared
)
$ErrorActionPreference = 'Stop'
Import-Module Microsoft.PowerShell.Utility -ErrorAction Stop
$package = (Resolve-Path -LiteralPath $DebugPackage).Path.TrimEnd('\')
if (-not $package.StartsWith('C:\SteerlineDebug\', [StringComparison]::OrdinalIgnoreCase) -or
    (Get-Item -LiteralPath $package).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Select an unlinked generated debug package under C:\SteerlineDebug.' }
$manifest = Get-Content -LiteralPath (Join-Path $package 'manifest.json') -Raw | ConvertFrom-Json
$schemePath=Join-Path $package 'scheme.json'
$schemeExists=Test-Path -LiteralPath $schemePath
if ($manifest.status -ne 'ready' -or ($schemeExists -and -not $ContinuePrepared) -or ($ContinuePrepared -and -not $schemeExists)) { throw 'Select a new package, or explicitly continue its frozen preparation before any candidate exists.' }
if ($ContinuePrepared) {
    $scheme=Get-Content -LiteralPath $schemePath -Raw | ConvertFrom-Json
    if ($scheme.scheme -ne 'live' -or $scheme.ready -ne $true) { throw 'Only the same fully prepared live scheme can continue.' }
}
$homeDirectory = Join-Path $package 'home'
$repository = Join-Path $package 'projects\SteerlineTest'
$evidence = Join-Path $package 'evidence\runner-update-live'
if ($ContinuePrepared) { $evidence += '-continue-'+[DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss') }
New-Item -ItemType Directory -Path $evidence | Out-Null
Copy-Item -LiteralPath $PSCommandPath -Destination (Join-Path $evidence 'qualification-entry.ps1')
$nativeReader=Join-Path $evidence 'read-native-turn.py'
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'testdata\read-native-turn.py') -Destination $nativeReader
$product = Join-Path $homeDirectory 'bin\steerline.exe'
$helper = Join-Path $package 'bin\debug-helper.exe'
$runner = Join-Path $package 'bin\steerrunner.exe'
$candidate = Join-Path $evidence 'candidate\steerrunner.exe'
New-Item -ItemType Directory -Path (Split-Path -Parent $candidate) | Out-Null
Copy-Item -LiteralPath (Resolve-Path -LiteralPath $RunnerCandidate).Path -Destination $candidate
$candidateSHA = (Get-FileHash -LiteralPath $candidate -Algorithm SHA256).Hash.ToLowerInvariant()
$originalSHA = (Get-FileHash -LiteralPath $runner -Algorithm SHA256).Hash.ToLowerInvariant()
if ($candidateSHA -eq $originalSHA) { throw 'The candidate must be a different compatible build.' }
$script:sequence = 0
function Save([string]$Name, $Value) {
    [IO.File]::WriteAllText((Join-Path $evidence ($Name+'.json')), ($Value | ConvertTo-Json -Depth 90), [Text.UTF8Encoding]::new($false))
}
function Invoke-Audit([string]$Exe, [string[]]$Arguments, [switch]$AllowFailure) {
    $script:sequence++
    $saved = $ErrorActionPreference
    try { $ErrorActionPreference = 'Continue'; $lines = & $Exe @Arguments 2>&1; $code = $LASTEXITCODE } finally { $ErrorActionPreference = $saved }
    $output = ($lines | Out-String).Trim()
    Save ('command-'+$script:sequence.ToString('0000')) @{executable=$Exe;arguments=$Arguments;exit_code=$code;output=$output;at=[DateTime]::UtcNow.ToString('o')}
    if ($code -ne 0 -and -not $AllowFailure) { throw "Command failed ($code): $Exe $($Arguments -join ' '): $output" }
    return @{code=$code;output=$output}
}
function Snapshot {
    $result = Invoke-Audit $helper @('debug','snapshot') -AllowFailure
    if ($result.code -ne 0) { return $null }
    return ($result.output | ConvertFrom-Json)
}
function Wait-Snapshot([scriptblock]$Predicate, [int]$Seconds = 300) {
    $deadline = [DateTime]::UtcNow.AddSeconds($Seconds)
    do {
        $snap = Snapshot
        if ($null -eq $snap) {
            $startupFailure = Join-Path $package 'evidence\startup-failure.json'
            if (Test-Path -LiteralPath $startupFailure) { throw ('Product startup failed: '+[IO.File]::ReadAllText($startupFailure)) }
            Start-Sleep -Seconds 2; continue
        }
        if (& $Predicate $snap) { return $snap }
        $failed = @($snap.RunnerState.Candidates | Where-Object { $_.projection.status -eq 'blocked' })
        if ($failed.Count) { Save 'blocked' $snap; throw ('Original candidate blocked: '+$failed[0].projection.diagnostic) }
        Start-Sleep -Seconds 2
    } while ([DateTime]::UtcNow -lt $deadline)
    Save 'timeout' $snap
    throw 'Live product checkpoint timed out.'
}
function Start-Coordinator([string]$Name, [string[]]$Arguments) {
    $file = Join-Path $evidence ($Name+'.ps1')
    $quoted = ($Arguments | ForEach-Object { "'"+$_.Replace("'","''")+"'" }) -join ','
    $body = "`$env:STEERLINE_HOME='"+$homeDirectory.Replace("'","''")+"'`r`n& '"+$helper.Replace("'","''")+"' @("+$quoted+") *> '"+(Join-Path $evidence ($Name+'.log')).Replace("'","''")+"'`r`nexit `$LASTEXITCODE`r`n"
    [IO.File]::WriteAllText($file, $body, [Text.UTF8Encoding]::new($true))
    $startup = New-CimInstance -ClassName Win32_ProcessStartup -ClientOnly -Property @{ShowWindow=[uint16]0}
    $created = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{CommandLine=('powershell.exe -NoProfile -ExecutionPolicy Bypass -File "'+$file+'"');CurrentDirectory=$package;ProcessStartupInformation=$startup}
    if ($created.ReturnValue -ne 0) { throw 'Independent debug coordinator could not start.' }
    Save $Name @{pid=$created.ProcessId;script=$file;at=[DateTime]::UtcNow.ToString('o')}
    # snapshot also validates the frozen scheme under the preparation lock.
    # Do not race it against the launcher's initial preparation.
    $deadline=[DateTime]::UtcNow.AddMinutes(5)
    $logPath=Join-Path $evidence ($Name+'.log')
    do {
        $log=if(Test-Path -LiteralPath $logPath){[string](Get-Content -LiteralPath $logPath -Raw -ErrorAction SilentlyContinue)}else{''}
        if ($log -and $log.Contains('Unified debug host ready:')) { return }
        if (-not (Get-Process -Id $created.ProcessId -ErrorAction SilentlyContinue)) { throw ('Debug coordinator exited: '+$log) }
        Start-Sleep -Seconds 1
    } while([DateTime]::UtcNow -lt $deadline)
    throw ('Debug coordinator readiness timed out: '+$log)
}
$oldHome = $env:STEERLINE_HOME
$env:STEERLINE_HOME = $homeDirectory
$summary = @{status='running';package=$package;source=$manifest;runner_a=$originalSHA;runner_b=$candidateSHA;repository='erranliu/SteerlineTest';base=$manifest.base_branch}
Save 'summary' $summary
try {
    if ($ContinuePrepared) {
        $prepared=Snapshot
        if ($null -eq $prepared -or @($prepared.RunnerState.Candidates).Count -ne 0 -or
            @($prepared.ProjectView.Tasks.Tasks).Count -ne 2 -or
            @($prepared.ProjectView.Tasks.Tasks | Where-Object status -ne 'open').Count -ne 0 -or
            $prepared.RunnerState.Checkpoint.Service.Status -ne 'stopped') { throw 'Preparation continuation requires a stopped, seeded package with no business candidate.' }
        Save 'prepared-before-business' $prepared
        $summary.continuation_from=Join-Path $package 'evidence\runner-update-live'
    } else {
    # This fixture variation is committed before the first scheme/task freeze.
    $implementation = @'
$ErrorActionPreference = 'Stop'
[IO.File]::WriteAllText((Join-Path (Get-Location) '.recovery-started.json'), (@{pid=$PID;at=[DateTime]::UtcNow.ToString('o')} | ConvertTo-Json))
Start-Sleep -Seconds 90
$branch = (& git symbolic-ref --short HEAD).Trim()
if ($LASTEXITCODE -ne 0) { throw 'No original branch' }
$expected = "SteerlineTest debug result`n$branch`n"
[IO.File]::WriteAllText((Join-Path (Get-Location) 'result.txt'), $expected, [Text.UTF8Encoding]::new($false))
if ([IO.File]::ReadAllText((Join-Path (Get-Location) 'result.txt')) -cne $expected) { throw 'Result differs' }
git add -- result.txt
if ($LASTEXITCODE -ne 0) { throw 'Stage failed' }
git -c commit.gpgsign=false commit -m 'Verify original execution across Runner update'
if ($LASTEXITCODE -ne 0) { throw 'Commit failed' }
$commit = (& git rev-parse HEAD).Trim()
[IO.File]::WriteAllText((Join-Path (Get-Location) '.recovery-completed.json'), (@{pid=$PID;commit=$commit;at=[DateTime]::UtcNow.ToString('o')} | ConvertTo-Json))
Write-Output $commit
'@
    [IO.File]::WriteAllText((Join-Path $repository 'implement-recovery.ps1'), $implementation, [Text.UTF8Encoding]::new($true))
    [IO.File]::AppendAllText((Join-Path $repository '.gitignore'), "`n.recovery-*.json`n", [Text.UTF8Encoding]::new($false))
    $specPath = Join-Path $repository 'SteerlineFlow\spec\task.spec.md'
    $spec = [IO.File]::ReadAllText($specPath)
    $spec += "`nRecovery qualification: in implementation run powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./implement-recovery.ps1 exactly once. It waits 90 seconds to exercise the authorized management-service outage, then writes, verifies and commits result.txt. Use a shell timeout of at least 120000ms, yield at 1000ms and poll that same command to completion. Do not repeat it or implement the file separately. The ignored .recovery-*.json files are test observations; never stage them. Do not modify the script or any other tracked file. After success return the exact completion line: 实施及必要验证已完成。`n"
    [IO.File]::WriteAllText($specPath, $spec, [Text.UTF8Encoding]::new($false))
    $planPath = Join-Path $repository 'SteerlineFlow\plans\task.plan.md'
    [IO.File]::AppendAllText($planPath, "`nFor this recovery qualification, run implement-recovery.ps1 once instead of doing its operations separately. Allow its 90 second wait to finish; publication remains a separate step.`n", [Text.UTF8Encoding]::new($false))
    Invoke-Audit git @('-C',$repository,'add','.') | Out-Null
    Invoke-Audit git @('-C',$repository,'commit','-m','Freeze live recovery qualification materials') | Out-Null
    Invoke-Audit $helper @('debug','init') | Out-Null
    }
    Invoke-Audit $helper @('debug','prepare') | Out-Null
    Start-Coordinator 'launch-a' @('debug','run','--no-open')
    $ready = Wait-Snapshot { param($s) $s.RunnerState.Checkpoint.Service.Ready -and @($s.ProjectView.Tasks.Tasks).Count -eq 2 }
    Save 'ready-a' $ready
    Invoke-Audit $helper @('debug','control','-config',(Join-Path $homeDirectory 'etc\SteerlineTest.json'),'-action','resume_dispatch') | Out-Null
    Invoke-Audit $helper @('debug','control','-config',(Join-Path $homeDirectory 'etc\SteerlineTest.json'),'-action','start_task','-task-id','1','-profile','live') | Out-Null
    $active = Wait-Snapshot { param($s) $c=@($s.RunnerState.Candidates | Where-Object task_id -eq '1'); $c.Count -eq 1 -and $c[0].projection.execution.turn_started -and (Test-Path -LiteralPath (Join-Path $c[0].projection.execution.workspace '.recovery-started.json')) }
    Save 'active-a' $active
    $original = @($active.RunnerState.Candidates | Where-Object task_id -eq '1')[0]
    $runID = $original.run_id
    $stateDirectory = $original.state_dir
    $native = $original.projection.execution
    $summary.run_id=$runID; $summary.candidate_id=$original.id; $summary.session_id=$native.session_id; $summary.original_execution=$native
    Save 'service-a' ((Invoke-Audit $runner @('service','status','--state-dir',$stateDirectory,'--json')).output | ConvertFrom-Json)
    . (Join-Path $PSScriptRoot 'deployment-runner.ps1')
    . (Join-Path $PSScriptRoot 'deployment-stop.ps1')
    Save 'takeover-online' (Assert-DeploymentRunnerBuild $homeDirectory $candidateSHA $candidate $product)
    $restartProof = Stop-DeploymentApplication $product $candidate -AllowRestart
    Save 'restart-proof' $restartProof
    $stoppedAt=[DateTime]::UtcNow
    $summary.offline_started_at=$stoppedAt.ToString('o')
    $offline=((Invoke-Audit $runner @('service','status','--state-dir',$stateDirectory,'--json')).output | ConvertFrom-Json)
    Save 'service-offline' $offline
    if ($offline.data.status -ne 'not_running') { throw 'Original management service did not exit.' }
    Write-Host 'Management services stopped; waiting for the original native turn to finish offline.'
    $deadline=[DateTime]::UtcNow.AddMinutes(4)
    do {
        $probePath=Join-Path $evidence 'native-offline.json'
        $probe=Invoke-Audit python @($nativeReader,'--executable',$AgentExecutable,'--workspace',$native.workspace,'--session',$native.session_id,'--turn',$native.turn_id,'--output',$probePath) -AllowFailure
        if ($probe.code -eq 0) { $read=Get-Content -LiteralPath $probePath -Raw | ConvertFrom-Json; if ($read.original_completed) { break } }
        Start-Sleep -Seconds 5
    } while ([DateTime]::UtcNow -lt $deadline)
    if (-not $read.original_completed) { throw 'Original native completion was not verified while the management service was offline.' }
    $completion=Get-Content -LiteralPath (Join-Path $native.workspace '.recovery-completed.json') -Raw | ConvertFrom-Json
    if ([DateTime]$completion.at -le $stoppedAt) { throw 'Implementation completed before service outage.' }
    Save 'offline-implementation' $completion
    Copy-Item -LiteralPath $candidate -Destination $runner -Force
    $executionPath=Join-Path $homeDirectory 'execution.json'
    $configuration=Get-Content -LiteralPath $executionPath -Raw | ConvertFrom-Json
    $version=((Invoke-Audit $candidate @('version','--json')).output | ConvertFrom-Json).data.version
    $configuration.runner.version=$version; $configuration.runner.sha256=$candidateSHA
    [IO.File]::WriteAllText($executionPath, ($configuration | ConvertTo-Json -Depth 40), [Text.UTF8Encoding]::new($false))
    # The original helper still owns its launcher lease; use the product's startup entry.
    $startFile=Join-Path $evidence 'start-b.ps1'
    [IO.File]::WriteAllText($startFile, ("`$env:STEERLINE_HOME='"+$homeDirectory+"'`r`n& '"+$product+"' start --no-open *> '"+(Join-Path $evidence 'start-b.log')+"'`r`n"), [Text.UTF8Encoding]::new($true))
    $startup=New-CimInstance -ClassName Win32_ProcessStartup -ClientOnly -Property @{ShowWindow=[uint16]0}
    $created=Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{CommandLine=('powershell.exe -NoProfile -ExecutionPolicy Bypass -File "'+$startFile+'"');CurrentDirectory=$package;ProcessStartupInformation=$startup}
    if ($created.ReturnValue -ne 0) { throw 'Candidate product startup failed.' }
    $published=Wait-Snapshot { param($s) $c=@($s.RunnerState.Candidates | Where-Object task_id -eq '1'); $s.RunnerState.Checkpoint.Service.Ready -and $c.Count -eq 1 -and $c[0].projection.pr.state -eq 'open' } 480
    Save 'published-b' $published
    $accepted=@($published.RunnerState.Candidates | Where-Object task_id -eq '1')[0]
    if ($accepted.id -ne $original.id -or $accepted.run_id -ne $runID -or $accepted.input_digest -ne $original.input_digest -or
        $accepted.projection.execution.session_id -ne $native.session_id -or $accepted.projection.resume_required -or
        ($accepted.runner_build | ConvertTo-Json -Compress) -cne ($original.runner_build | ConvertTo-Json -Compress) -or
        ($accepted.submit | ConvertTo-Json -Depth 40 -Compress) -cne ($original.submit | ConvertTo-Json -Depth 40 -Compress) -or
        @($accepted.actions | Where-Object { $null -ne $_ }).Count) { throw 'Takeover replaced original facts or required another caller input.' }
    $serviceB=((Invoke-Audit $runner @('service','status','--state-dir',$stateDirectory,'--json')).output | ConvertFrom-Json).data
    if ($serviceB.executable_sha256 -ne $candidateSHA) { throw 'Original Runner was not updated.' }
    Save 'service-b' $serviceB
    $pr=$accepted.projection.pr
    if ($pr.repository -ne 'erranliu/SteerlineTest' -or $pr.base_branch -ne $manifest.base_branch -or $pr.head_branch -ne $native.branch -or $pr.url -notmatch '^https://github.com/erranliu/SteerlineTest/pull/[0-9]+$') { throw 'PR differs from this dedicated debug identity.' }
    $proof=((Invoke-Audit gh @('pr','view',$pr.url,'--repo','erranliu/SteerlineTest','--json','url,state,isDraft,headRefName,headRefOid,baseRefName,files')).output | ConvertFrom-Json)
    if ($proof.state -ne 'OPEN' -or $proof.headRefName -ne $native.branch -or $proof.baseRefName -ne $manifest.base_branch -or $proof.headRefOid -ne $completion.commit -or @($proof.files).Count -ne 1 -or $proof.files[0].path -ne 'result.txt') { throw 'Published Git artifact differs from the original offline completion.' }
    $expectedBlob="SteerlineTest debug result`n"+$native.branch+"`n"
    $bytes=[Text.Encoding]::UTF8.GetBytes($expectedBlob)
    $format=(Invoke-Audit git @('-C',$native.workspace,'rev-parse','--show-object-format')).output
    if ($format -ne 'sha1') { throw 'This debug fixture requires SHA-1 Git objects.' }
    $hash=[Security.Cryptography.SHA1]::Create()
    try { $expectedOID=([BitConverter]::ToString($hash.ComputeHash([byte[]]([Text.Encoding]::ASCII.GetBytes(('blob '+$bytes.Length+[char]0))+$bytes)))).Replace('-','').ToLowerInvariant() } finally { $hash.Dispose() }
    $actualOID=(Invoke-Audit git @('-C',$native.workspace,'rev-parse',($proof.headRefOid+':result.txt'))).output
    Save 'git-blob-proof' @{commit=$proof.headRefOid;path='result.txt';expected_oid=$expectedOID;actual_oid=$actualOID;expected_bytes=$bytes.Length}
    if ($actualOID -cne $expectedOID) { throw 'Committed PR result bytes differ from the frozen task.' }
    Save 'pr-before-merge' $proof
    if ($proof.isDraft) { Invoke-Audit gh @('pr','ready',$pr.url,'--repo','erranliu/SteerlineTest') | Out-Null }
    Invoke-Audit gh @('pr','merge',$pr.url,'--repo','erranliu/SteerlineTest','--merge','--match-head-commit',$proof.headRefOid) | Out-Null
    $done=Wait-Snapshot { param($s) $s.RunnerState.Graph.Deliveries.'1' -and @($s.ProjectView.Tasks.Tasks | Where-Object { $_.task_id -eq '2' -and $_.dependency_ready -and $_.status -eq 'open' -and @($_.candidates).Count -eq 0 }).Count -eq 1 } 180
    Save 'delivered' $done
    if (@($done.RunnerState.Candidates).Count -ne 1 -or @($done.RunnerState.Graph.Deliveries.PSObject.Properties).Count -ne 1) { throw 'Duplicate candidate or delivery.' }
    $merged=((Invoke-Audit gh @('pr','view',$pr.url,'--repo','erranliu/SteerlineTest','--json','url,state,headRefName,headRefOid,baseRefName,mergeCommit,mergedAt')).output | ConvertFrom-Json)
    Save 'pr-merged' $merged
    $delivery=$done.RunnerState.Graph.Deliveries.'1'
    if ($merged.state -ne 'MERGED' -or $merged.headRefOid -ne $completion.commit -or $merged.headRefName -ne $native.branch -or
        $merged.baseRefName -ne $manifest.base_branch -or -not $merged.mergeCommit.oid -or
        $delivery.selected_workflow_run_id -ne $original.id -or $delivery.pr_url -ne $pr.url -or
        $delivery.merge_commit -ne $merged.mergeCommit.oid) { throw 'Delivery differs from the original candidate or verified merge.' }
    $summary.status='passed'; $summary.pr=$pr.url; $summary.commit=$completion.commit; $summary.delivery_count=1; $summary.successor_ready=$true
    Save 'summary' $summary
    Write-Host ('Passed: original native execution, compatible update, offline final, original PR and one delivery. '+$pr.url)
} catch {
    $summary.status='failed'; $summary.error=$_.Exception.Message
    Save 'summary' $summary
    throw
} finally {
    # Stop only this package. Task/native cleanup remains the product's formal path.
    Invoke-Audit $helper @('debug','stop') -AllowFailure | Out-Null
    if ($ready) {
        $cleanup = Invoke-Audit $helper @('runner-cleanup','-config',(Join-Path $homeDirectory 'etc\SteerlineTest.json')) -AllowFailure
        Save 'cleanup' $cleanup
        $summary.cleanup_confirmed=($cleanup.code -eq 0)
        Save 'summary' $summary
        if ($cleanup.code -ne 0) {
            $summary.status='failed'; $summary.cleanup_error=$cleanup.output
            Save 'summary' $summary
            $cleanupFailed = -not $summary.error
        }
    }
    $env:STEERLINE_HOME=$oldHome
    if ($cleanupFailed) { throw 'Owned Runner resources did not settle; preserve cleanup evidence.' }
}
