param([Parameter(Mandatory)][ValidateSet('tty','prepare','publish')][string]$Phase)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'environment-tty.ps1')
if ($Phase -eq 'tty') {
    $tty = Get-EnvironmentTTYCheck
    $tty | ConvertTo-Json -Compress
    if ($tty.status -cne 'passed') { exit 1 }
    exit 0
}
$root=Split-Path $PSScriptRoot -Parent
Set-Location -LiteralPath $root
foreach($path in @($env:STEERLINE_PROGRESS_FILE,$env:STEERLINE_RESULT_FILE)) {
    if (!$path -or ![IO.Path]::IsPathRooted($path) -or !(Test-Path -LiteralPath $path -PathType Leaf)) { throw 'Runner must inject both existing channel files' }
}
$report=Get-Content -Raw -LiteralPath $env:STEERLINE_RESULT_FILE|ConvertFrom-Json
$definitions=@((Get-Content -Raw SteerlineFlow/testing/test-list.json|ConvertFrom-Json).tests|Where-Object level -CEQ L2)
$manifest=[regex]::Match((Get-Content -Raw SteerlineFlow/environment.md),'(?s)```steerline-environment\r?\n(.*?)\r?\n```').Groups[1].Value|ConvertFrom-Json
function Write-Atomic($Value,[string]$Path) {
    $bytes=[Text.UTF8Encoding]::new($false).GetBytes(($Value|ConvertTo-Json -Depth 20 -Compress))
    if($bytes.Length -gt 65536){throw 'Channel report exceeds 64 KiB'}
    $temporary=$Path+'.'+[guid]::NewGuid().ToString('N')+'.tmp'
    try {$stream=[IO.File]::Open($temporary,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None);try{$stream.Write($bytes);$stream.Flush($true)}finally{$stream.Dispose()};[IO.File]::Move($temporary,$Path,$true)}finally{if(Test-Path -LiteralPath $temporary){Remove-Item -LiteralPath $temporary}}
}
function Save-Result {Write-Atomic $report $env:STEERLINE_RESULT_FILE}
function Set-Progress {
    $progress=Get-Content -Raw -LiteralPath $env:STEERLINE_PROGRESS_FILE|ConvertFrom-Json -AsHashtable
    foreach($item in $report.l2){$key='test.'+$item.test_id;if(!$progress.ContainsKey($key)){throw 'Undeclared L2 progress field'};$progress[$key]=$item.status -ceq 'passed'}
    $progress.pr=[bool]($report.pr.url -and $report.pr.head_revision -and $report.pr.cleanup_completed -and !$report.pr.outcome_unknown)
    Write-Atomic $progress $env:STEERLINE_PROGRESS_FILE
}
function Invoke-GitValue {$Arguments=$args;$value=@(& git @Arguments);if($LASTEXITCODE){throw ('Git failed: '+$Arguments[0])};return ($value -join "`n").Trim()}
function Check([string]$Id,[scriptblock]$Probe) {try {$message=[string](& $Probe);$report.checks+=@{id=$Id;status='passed';message=$message}}catch {$report.checks+=@{id=$Id;status='failed';message=$_.Exception.Message}}}
function Invoke-GhValue {$Arguments=$args;$value=@(& gh @Arguments);if($LASTEXITCODE){throw ('GitHub command failed: '+$Arguments[0])};return ($value -join "`n")}
function Verify-L2 {
    $output = Join-Path $root '.steerrunner/artifacts/environment-l2'
    $summary = Get-Content -Raw (Join-Path $output 'summary.json') | ConvertFrom-Json
    if (!$definitions.Count -or !$summary.passed -or !$summary.cleanup_complete -or $summary.level -cne 'L2' -or $summary.scope -cne 'e2e' -or $summary.coverage -cne 'full-L2' -or @($summary.blocked_scenarios | Where-Object { $null -ne $_ }).Count -or @($summary.selected_scenarios).Count -ne $definitions.Count) { throw 'Original complete controlled L2 or cleanup did not pass' }
    $report.l2 = @()
    foreach ($definition in $definitions) {
        $managed = Get-Content -Raw (Join-Path $output ('managed-' + $definition.id + '.json')) | ConvertFrom-Json
        $items = @($managed.results | Where-Object test_id -CEQ $definition.id)
        $evidence = $managed.evidence
        $raw = [IO.File]::ReadAllBytes((Join-Path $output 'summary.json'))
        $digest = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($raw)).ToLowerInvariant()
        if ($items.Count -ne 1 -or $items[0].status -cne 'passed' -or @($summary.selected_scenarios | Where-Object id -CEQ $definition.id).Count -ne 1 -or @($managed.selection).Count -ne 1 -or $managed.selection[0] -cne $definition.id -or $managed.before_commit -cne $report.source_commit -or $managed.after_commit -cne $report.source_commit -or !$managed.before_clean -or !$managed.after_clean -or $evidence.path -cne 'summary.json' -or $evidence.bytes -ne $raw.Length -or $evidence.sha256 -cne $digest) { throw 'Original managed L2 source/summary/status binding failed' }
        $report.l2 += @{test_id=$definition.id;status='passed';message='Original complete L2 managed report and cleanup verified'}
    }
}

try {
    if($report.version -ne 2 -or $report.activity_id -notmatch '^[a-f0-9]{32}$' -or $report.round -notin 1,2,3 -or !$definitions.Count){throw 'Invalid original environment input'}
    $head=Invoke-GitValue @('branch','--show-current');$base=[string]$manifest.validation_base
    if(!$head -or $base -in @('main','master') -or $head -in @('main','master',$base)){throw 'Validation requires an isolated base and original Runner head'}
    $report.pr.base=$base;$report.pr.head=$head
    if($Phase -eq 'prepare') {
        $report.completed=$false;$report.l2=@($definitions|ForEach-Object {@{test_id=$_.id;status='incomplete';message='Not executed'}})
        $report.pr.cleanup_completed=$true;$report.checks=@();Set-Progress;Save-Result
        Check 'terminal.tty' { Assert-EnvironmentTTY }
        Check 'powershell' {if($PSVersionTable.PSVersion.Major -lt 7){throw 'PowerShell 7+ required'};'PowerShell '+$PSVersionTable.PSVersion}
        Check 'git' {Invoke-GitValue @('--version')}
        Check 'go' {$v=& go version;if($LASTEXITCODE -or $v -notmatch 'go(\d+\.\d+(?:\.\d+)?)' -or [version]$Matches[1] -lt [version]'1.26.0'){throw 'PATH go >=1.26.0 required'};[string]$v}
        Check 'github.auth' {$login=Invoke-GhValue @('api','user','--jq','.login');'GitHub authenticated as '+$login.Trim()}
        Check 'source.clean' {if((Invoke-GitValue @('rev-parse','HEAD')) -cne $report.source_commit -or (Invoke-GitValue @('status','--porcelain'))){throw 'Fixed source must be clean; no automatic restore is performed'};'HEAD matches fixed source; worktree clean'}
        if(@($report.checks|Where-Object status -CNE 'passed').Count){throw ('Environment requirements not met: '+(@($report.checks|Where-Object status -CNE 'passed'|ForEach-Object {$_.id+': '+$_.message}) -join '; '))}
        & go run ./cmd/test-feedback -level=L1 -out .steerrunner/artifacts/environment-l1
        if($LASTEXITCODE){throw 'Original L1 failed; see environment-l1/summary.json'}
        & go run ./cmd/test-feedback -level=L2 -out .steerrunner/artifacts/environment-l2
        if($LASTEXITCODE){throw 'Original L2 failed; see environment-l2/summary.json'}
        Verify-L2
        $report.completed=$true;$report.message='Original complete L1/L2 passed; publication pending';Set-Progress;Save-Result
        exit 0
    }
    if(!@($report.checks).Count -or @($report.checks|Where-Object status -CNE 'passed').Count){throw 'Environment requirements have not all passed; publication prohibited'}
    $tty = Get-EnvironmentTTYCheck
    $report.checks = @($report.checks | Where-Object id -CNE 'terminal.tty') + @($tty)
    Save-Result
    if ($tty.status -cne 'passed') { throw $tty.message }
    Verify-L2
    if($report.pr.outcome_unknown){throw 'Original publication outcome unknown; reconcile the original PR, do not repeat writes'}
    $probe='SteerlineFlow/testing/validation/'+$report.activity_id+'-'+$report.round+'.md'
    $revision=Invoke-GitValue @('rev-parse','HEAD')
    if($revision -ceq $report.source_commit){
        if(Invoke-GitValue @('status','--porcelain')){throw 'Unexpected tracked or untracked changes before publication'}
        [IO.Directory]::CreateDirectory((Split-Path $probe -Parent))|Out-Null
        [IO.File]::WriteAllText((Join-Path $root $probe),"Environment validation $($report.activity_id), round $($report.round), source $($report.source_commit).`n",[Text.UTF8Encoding]::new($false))
        Invoke-GitValue @('add','--',$probe)|Out-Null
        Invoke-GitValue @('commit','-m','Validate remote project environment')|Out-Null
        $revision=Invoke-GitValue @('rev-parse','HEAD')
    }
    $changed=@((Invoke-GitValue @('diff','--name-only',$report.source_commit,'HEAD')) -split "`n")
    if($changed.Count -ne 1 -or $changed[0] -cne $probe -or (Invoke-GitValue @('status','--porcelain'))){throw 'Validation commit changed anything besides its own probe'}
    $report.pr.outcome_unknown=$true;$report.pr.cleanup_completed=$false;Save-Result
    Invoke-GitValue @('push','origin',('HEAD:refs/heads/'+$head))|Out-Null
    $prs=@(Invoke-GhValue @('pr','list','--repo','erranliu/SteerLine','--base',$base,'--head',$head,'--state','all','--json','number,url')|ConvertFrom-Json)
    if($prs.Count -gt 1){throw 'Multiple PRs for the original validation head'}
    if(!$prs.Count){$report.pr.url=(Invoke-GhValue @('pr','create','--repo','erranliu/SteerLine','--base',$base,'--head',$head,'--title',('Environment validation '+$report.activity_id),'--body','Isolated project environment validation. Complete original L1/L2 ran in the assigned remote worktree.')).Trim();Save-Result}
    else {$report.pr.url=$prs[0].url;Save-Result}
    $pr=Invoke-GhValue @('pr','view',$report.pr.url,'--repo','erranliu/SteerLine','--json','url,state,baseRefName,headRefName,headRefOid,mergeCommit')|ConvertFrom-Json
    if($pr.state -cnotin @('OPEN','MERGED') -or $pr.baseRefName -cne $base -or $pr.headRefName -cne $head -or $pr.headRefOid -cne $revision){throw 'Original PR identity differs'}
    $report.pr.head_revision=$revision;$report.pr.cleanup_completed=$true;$report.pr.outcome_unknown=$false
    $report.completed=$true;$report.message='Original complete L2 and original PR publication verified; no script-owned live resources';Set-Progress;Save-Result
    Write-Output $report.pr.url
} catch {
    $report.completed=$true;$report.message=$_.Exception.Message
    foreach($item in $report.l2){if($item.status -ceq 'incomplete'){$item.message=$report.message}}
    if(!@($report.l2).Count){$report.l2=@($definitions|ForEach-Object {@{test_id=$_.id;status='incomplete';message='Preparation did not complete'}})}
    Set-Progress;Save-Result
    [Console]::Error.WriteLine($report.message)
    exit 1
}
