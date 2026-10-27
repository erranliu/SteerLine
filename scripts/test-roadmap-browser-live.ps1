param(
    [Parameter(Mandatory=$true)][string]$SteerRunnerExe,
    [Parameter(Mandatory=$true)][string]$OutRoot,
    [switch]$IndependentProcess
)
$ErrorActionPreference='Stop'
if (!$IndependentProcess) {
    . (Join-Path $PSScriptRoot 'independent-test.ps1')
    Invoke-IndependentTest -ScriptPath $PSCommandPath -TestArguments @('-SteerRunnerExe',$SteerRunnerExe,'-OutRoot',$OutRoot) -Timeout ([TimeSpan]::FromMinutes(30))
    return
}
$taskSource=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskRoot=[IO.Path]::GetFullPath($OutRoot).TrimEnd('\')
if (!$taskRoot.StartsWith('C:\SteerlineDebug\',[StringComparison]::OrdinalIgnoreCase) -or (Test-Path -LiteralPath $taskRoot)) { throw 'OutRoot must be a new directory under C:\SteerlineDebug.' }
$taskCursor=[IO.Path]::GetDirectoryName($taskRoot)
while ($taskCursor) {
    if (Test-Path -LiteralPath $taskCursor) {
        if ((Get-Item -LiteralPath $taskCursor -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Linked output roots are not allowed.' }
    }
    $taskCursor=[IO.Path]::GetDirectoryName($taskCursor)
}
[IO.Directory]::CreateDirectory($taskRoot)|Out-Null
$taskAcl=Get-Acl -LiteralPath $taskRoot
$taskAcl.SetAccessRuleProtection($true,$false)
$taskSID=[Security.Principal.WindowsIdentity]::GetCurrent().User
$taskAcl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($taskSID,'FullControl','ContainerInherit,ObjectInherit','None','Allow'))
$taskAcl.SetOwner($taskSID)
Set-Acl -LiteralPath $taskRoot -AclObject $taskAcl
foreach($name in @('bin','home','home\etc','project','evidence')) { [IO.Directory]::CreateDirectory((Join-Path $taskRoot $name))|Out-Null }
$taskUTF8=[Text.UTF8Encoding]::new($false)
function Write-TaskJSON([string]$Name,$Value) { [IO.File]::WriteAllText((Join-Path $taskRoot $Name),($Value|ConvertTo-Json -Depth 50),$taskUTF8) }
function Invoke-TaskNative([string]$Executable,[string[]]$Arguments) {
    $saved=$ErrorActionPreference
    try { $ErrorActionPreference='Continue';$output=& $Executable @Arguments 2>&1;$code=$LASTEXITCODE }
    finally { $ErrorActionPreference=$saved }
    $body=($output|Out-String).Trim()
    if($code -ne 0) { throw "Native command failed ($code): $Executable $($Arguments -join ' ')`n$body" }
    return $body
}
$taskProject=Join-Path $taskRoot 'project'
$taskHome=Join-Path $taskRoot 'home'
$taskBinary=Join-Path $taskRoot 'bin\steerline.exe'
$taskWeb=Join-Path $taskRoot 'bin\steerline-web.exe'
$taskRunner=Join-Path $taskRoot 'bin\steerrunner.exe'
$taskRemote='erranliu/SteerlineTest'
$taskNonce=(Get-Date -Format 'yyyyMMdd-HHmmss')+'-'+[Guid]::NewGuid().ToString('N').Substring(0,12)
$taskBase='codex/roadmap-e2e/'+$taskNonce+'/base'
$taskStarted=$false;$taskBrowser=$false;$taskCleanup=$false;$taskPR=$null;$taskHead='';$taskFailure='';$taskCleanupErrors=@()
$taskPreviousHome=$env:STEERLINE_HOME
$taskPreviousRoot=$env:STEERLINE_ROADMAP_BROWSER_ROOT
$taskPreviousEvidence=$env:STEERLINE_WEB_EVIDENCE
try {
    $repoInfo=Invoke-TaskNative 'gh' @('api',('repos/'+$taskRemote),'--jq','{full_name,default_branch,permissions}')
    $taskPermission=$repoInfo|ConvertFrom-Json
    if($taskPermission.full_name -ne $taskRemote -or !$taskPermission.permissions.push) { throw 'SteerlineTest push permission required.' }
    $taskMain=$taskPermission.default_branch
    $taskMainBefore=(Invoke-TaskNative 'gh' @('api',('repos/'+$taskRemote+'/git/ref/heads/'+$taskMain),'--jq','.object.sha'))
    Write-TaskJSON 'evidence\remote-before.json' @{repository=$taskRemote;default_branch=$taskMain;commit=$taskMainBefore;base_branch=$taskBase}
    Push-Location $taskSource
    try {
        $taskSHA=(Invoke-TaskNative 'git' @('rev-parse','HEAD'))
        $taskDirty=if(Invoke-TaskNative 'git' @('status','--porcelain')){'true'}else{'false'}
        $ErrorActionPreference='Continue'
        & go build -trimpath -ldflags "-X github.com/erranliu/SteerLine/internal/app.buildSourceCommit=$taskSHA -X github.com/erranliu/SteerLine/internal/app.buildSourceDirty=$taskDirty" -o $taskBinary ./cmd/steerline *> (Join-Path $taskRoot 'evidence\build-backend.log')
        $taskExit=$LASTEXITCODE;$ErrorActionPreference='Stop'
        if($taskExit){throw 'Backend build failed.'}
        $ErrorActionPreference='Continue'
        & npm.cmd --prefix web run build *> (Join-Path $taskRoot 'evidence\build-assets.log')
        $taskExit=$LASTEXITCODE;$ErrorActionPreference='Stop'
        if($taskExit){throw 'Web assets build failed.'}
        $ErrorActionPreference='Continue'
        & go build -trimpath -tags=webui -o $taskWeb ./cmd/steerline-web *> (Join-Path $taskRoot 'evidence\build-web.log')
        $taskExit=$LASTEXITCODE;$ErrorActionPreference='Stop'
        if($taskExit){throw 'Web executable build failed.'}
    } finally {Pop-Location}
    Copy-Item -LiteralPath $SteerRunnerExe -Destination $taskRunner
    $taskVersion=(Invoke-TaskNative $taskRunner @('version','--json')|ConvertFrom-Json).data
    $taskRunnerSHA=(Get-FileHash -LiteralPath $taskRunner -Algorithm SHA256).Hash.ToLowerInvariant()
    $taskCodex=(Get-Command codex.exe -ErrorAction Stop).Source
    Write-TaskJSON 'evidence\build.json' @{source_commit=$taskSHA;source_dirty=$taskDirty;runner=$taskVersion;runner_sha256=$taskRunnerSHA;backend_sha256=(Get-FileHash $taskBinary).Hash;web_sha256=(Get-FileHash $taskWeb).Hash;codex=(Invoke-TaskNative $taskCodex @('--version'))}
    Invoke-TaskNative 'git' @('-C',$taskProject,'init','--initial-branch',$taskBase)|Out-Null
    Invoke-TaskNative 'git' @('-C',$taskProject,'config','user.name','SteerLine test')|Out-Null
    Invoke-TaskNative 'git' @('-C',$taskProject,'config','user.email','roadmap-test@example.invalid')|Out-Null
    Invoke-TaskNative 'git' @('-C',$taskProject,'config','commit.gpgsign','false')|Out-Null
    Invoke-TaskNative 'git' @('-C',$taskProject,'config','credential.helper','!gh auth git-credential')|Out-Null
    Invoke-TaskNative 'git' @('-C',$taskProject,'remote','add','origin',('https://github.com/'+$taskRemote+'.git'))|Out-Null
    $files=@{
        'go.mod'="module example.invalid/roadmap`n`ngo 1.24.2`n"
        'logic.go'="package logic`n// Pure integer arithmetic; new functions are planned, not implemented.`n"
        'AGENTS.md'="# 项目规则`n保持纯算术范围，沿用已有 logic 包和 go.mod。规范仅为下列三份完整文件，无额外隐含规范。规划阶段不实现或执行测试。`n"
        'SteerlineFlow/spec/SpecGuide.md'="# 生成规范`n实际调查固定版本 logic.go 与 go.mod，消费已确认 T1/T2 和依赖。每个任务生成完整中文 Spec、Plan、专属验收文档。使用内部代号与给定稳定路径，不分配正式编号。整体设计说明复用、所有者、依赖交接；不增加发布框架、兼容层或新任务。必需规范仅本文件、testing.md、spec-plan-guidelines.md，均已提供且不引用其他规范。`n"
        'SteerlineFlow/testing.md'="# 测试规范`n本项目已有 go.mod；实现任务以后运行 go test ./... 与 go vet ./...。预期断言 Sum(2,3)=5、Difference(5,2)=3。测试目录条目为 unit.arithmetic；Plan 写命令、环境、通过条件和证据位置，当前结果为未验。不使用 SteerLine 本仓 L1/L2 执行器，不需要新增测试基础设施。`n"
        'SteerlineProjectGuideDoc/spec-plan-guidelines.md'="# 编写规范`nSpec 写目标、范围、接口行为、非目标和验收条件；Plan 写具体文件、复用、T1 到 T2 交接、单元测试与证据。每个任务独立验收文件，自动验收可执行，纯函数无需人工 UI 体验。`n"
        'SteerlineFlow/testing/test-list.json'='{"tests":[{"id":"unit.arithmetic","name":"算术单元","level":"L1","environment":"controlled","command":"go test ./...","check":"Sum(2,3)=5、Difference(5,2)=3；当前未执行"}]}'
        'SteerlineFlow/Roadmap/execution-map.md'="# 算术任务执行清单`n`n当前无正式任务。编号仅由 SteerLine 入图分配。`n"
        'SteerlineFlow/Roadmap/drafts/arithmetic-roadmap.md'="# 算术 Roadmap`nT1：在已有 logic 包新增纯函数 Sum(a,b int) int 和单元测试，Sum(2,3)=5；不改变其他范围。`nT2：新增纯函数 Difference(a,b int) int，使用 Sum(a,-b) 实现并补单元测试，Difference(5,2)=3；依赖本 Roadmap 的 T1。`n本次只生成规划文档，不实现函数，不执行测试。`n"
    }
    foreach($item in $files.GetEnumerator()) {
        $path=Join-Path $taskProject $item.Key
        [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($path))|Out-Null
        [IO.File]::WriteAllText($path,$item.Value,$taskUTF8)
    }
    Invoke-TaskNative 'git' @('-C',$taskProject,'add','.')|Out-Null
    Invoke-TaskNative 'git' @('-C',$taskProject,'commit','-qm','Isolated roadmap browser fixture')|Out-Null
    $taskBaseCommit=Invoke-TaskNative 'git' @('-C',$taskProject,'rev-parse','HEAD')
    Write-TaskJSON 'push-intent.json' @{repository=$taskRemote;branch=$taskBase;commit=$taskBaseCommit}
    Invoke-TaskNative 'git' @('-C',$taskProject,'push','origin',('HEAD:refs/heads/'+$taskBase))|Out-Null
    $probe=[Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,0);$probe.Start()
    try {$taskPort=$probe.LocalEndpoint.Port} finally {$probe.Stop()}
    Write-TaskJSON 'manifest.json' @{schema='roadmap-browser-live/v1';root=$taskRoot;repository=$taskRemote;base_branch=$taskBase;base_commit=$taskBaseCommit;web_address=('127.0.0.1:'+$taskPort);source_commit=$taskSHA;source_dirty=$taskDirty}
    Write-TaskJSON 'home\web.json' @{version=1;address=('127.0.0.1:'+$taskPort)}
    Write-TaskJSON 'home\execution.json' @{version=3;max_generate_concurrency=2;runner=@{executable=$taskRunner;version=$taskVersion.version;sha256=$taskRunnerSHA};default_execution_profile='roadmap-live';execution_profiles=@(@{id='roadmap-live';name='Roadmap live';execution=@{adapter='codex-app-server';target=@{kind='local'};settings=@{executable=$taskCodex;model='gpt-6.1-sol';reasoning_effort='low'}};workflow=@{id='development';version=2}})}
    $query=Get-Content -LiteralPath (Join-Path $taskSource 'debug-template\home\config-source\SteerlineTest.live.json') -Raw -Encoding utf8|ConvertFrom-Json
    $query.repository=$taskProject;$query.branch=$taskBase
    $query|Add-Member -NotePropertyName shutdown_grace -NotePropertyValue '45s' -Force
    Write-TaskJSON 'home\etc\RoadmapLive.json' $query
    $env:STEERLINE_HOME=$taskHome
    Invoke-TaskNative $taskBinary @('install')|Out-File -LiteralPath (Join-Path $taskRoot 'evidence\install-backend.log')
    Invoke-TaskNative $taskWeb @('install')|Out-File -LiteralPath (Join-Path $taskRoot 'evidence\install-web.log')
    $taskStarted=$true
    Invoke-TaskNative $taskBinary @('start','--no-open')|Out-File -LiteralPath (Join-Path $taskRoot 'evidence\start.json')
    $env:STEERLINE_ROADMAP_BROWSER_ROOT=$taskRoot
    $env:STEERLINE_WEB_EVIDENCE=Join-Path $taskRoot 'evidence\browser'
    Push-Location $taskSource
    try {
        $ErrorActionPreference='Continue'
        & npm.cmd --prefix web run test:e2e -- roadmap-live.spec.ts *> (Join-Path $taskRoot 'evidence\browser.log')
        $taskBrowser=$LASTEXITCODE -eq 0
        $ErrorActionPreference='Stop'
        if(!$taskBrowser){throw 'Roadmap browser test failed; inspect evidence/browser.log.'}
    } finally {Pop-Location}
} catch { $taskFailure=($_|Out-String) }
finally {
    if($taskStarted) {
        try {
            Invoke-TaskNative $taskBinary @('stop')|Out-File -LiteralPath (Join-Path $taskRoot 'evidence\stop.log')
            Invoke-TaskNative $taskBinary @('verify-stop','--runner-exe',$taskRunner,'--json')|Out-File -LiteralPath (Join-Path $taskRoot 'evidence\stop-verification.json')
        }
        catch {$taskCleanupErrors+=($_|Out-String)}
    }
    # Even an unknown PR response is reconciled by its original head/base.
    try {
        $factsPath=Join-Path $taskRoot 'evidence\browser-facts.json'
        if(Test-Path -LiteralPath $factsPath) {
            $facts=Get-Content -LiteralPath $factsPath -Raw -Encoding utf8|ConvertFrom-Json
            if($facts.document_id -match '^[0-9a-f]{32}$') {$taskHead='codex/roadmap-'+$facts.document_id}
        }
        if($taskHead) {
            $prs=Invoke-TaskNative 'gh' @('api',('repos/'+$taskRemote+'/pulls?state=all&head=erranliu:'+ $taskHead+'&base='+$taskBase+'&per_page=100'))|ConvertFrom-Json
            $taskPRMatches=@($prs|Where-Object {$_.head.ref -eq $taskHead -and $_.base.ref -eq $taskBase -and $_.head.repo.full_name -eq $taskRemote})
            if($taskPRMatches.Count -gt 1){throw 'Multiple PRs for original test head; preserve refs.'}
            if($taskPRMatches.Count -eq 1){
                $taskPR=$taskPRMatches[0]
                Write-TaskJSON 'evidence\external-pr.json' $taskPR
                if($taskPR.state -eq 'open') {Invoke-TaskNative 'gh' @('pr','close',"$($taskPR.number)",'--repo',$taskRemote)|Out-Null}
            }
        }
        foreach($branch in @($taskHead,$taskBase)) {
            if(!$branch){continue}
            if(($branch -eq $taskBase -and $branch -notmatch '^codex/roadmap-e2e/[0-9-]+[0-9a-f]+/base$') -or ($branch -eq $taskHead -and $branch -notmatch '^codex/roadmap-[0-9a-f]{32}$')){throw 'Ref cleanup identity differs.'}
            $present=Invoke-TaskNative 'git' @('-C',$taskProject,'ls-remote','--refs','origin',('refs/heads/'+$branch))
            if($present){Invoke-TaskNative 'git' @('-C',$taskProject,'push','origin',(':refs/heads/'+$branch))|Out-Null}
        }
        $after=Invoke-TaskNative 'gh' @('api',('repos/'+$taskRemote+'/git/ref/heads/'+$taskMain),'--jq','.object.sha')
        if($after -ne $taskMainBefore){throw 'Default branch moved during the test; inspect before/after evidence.'}
        Write-TaskJSON 'evidence\remote-after.json' @{repository=$taskRemote;default_branch=$taskMain;commit=$after;deleted_refs=@($taskHead,$taskBase)}
    } catch {$taskCleanupErrors+=($_|Out-String)}
    $processes=@(Get-CimInstance Win32_Process|Where-Object {$_.ExecutablePath -and $_.ExecutablePath.StartsWith($taskRoot+'\',[StringComparison]::OrdinalIgnoreCase)})
    $listeners=@(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue|Where-Object {$_.OwningProcess -in $processes.ProcessId -or ($taskPort -and $_.LocalPort -eq $taskPort)})
    $taskCleanup=$taskCleanupErrors.Count -eq 0 -and $processes.Count -eq 0 -and $listeners.Count -eq 0
    Write-TaskJSON 'result.json' @{schema='roadmap-browser-live/v1';passed=($taskBrowser -and $taskCleanup -and !$taskFailure);browser_passed=$taskBrowser;cleanup_complete=$taskCleanup;failure=$taskFailure;cleanup_errors=$taskCleanupErrors;processes=$processes.Count;listeners=$listeners.Count;pull_request=$taskPR.html_url;base_branch=$taskBase;head_branch=$taskHead}
    $env:STEERLINE_HOME=$taskPreviousHome;$env:STEERLINE_ROADMAP_BROWSER_ROOT=$taskPreviousRoot;$env:STEERLINE_WEB_EVIDENCE=$taskPreviousEvidence
}
if(!$taskBrowser -or !$taskCleanup -or $taskFailure){throw "Roadmap live browser/PR test failed: $taskRoot"}
Write-Output "Roadmap browser and external PR passed: $taskRoot"
