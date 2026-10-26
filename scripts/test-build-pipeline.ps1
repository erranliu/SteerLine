param([Parameter(Mandatory=$true)][string]$OutDir)
$ErrorActionPreference = 'Stop'
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$OutDir = [IO.Path]::GetFullPath($OutDir)
if (Test-Path -LiteralPath $OutDir) { throw 'OutDir must be new.' }
$source = Join-Path $OutDir 'source with spaces'
$runner = Join-Path $OutDir 'runner'
$tools = Join-Path $OutDir 'tools'
foreach ($directory in @($source, (Join-Path $source 'scripts'), (Join-Path $source 'web'), (Join-Path $runner 'scripts'), $tools)) {
    New-Item -ItemType Directory -Path $directory -Force | Out-Null
}
Copy-Item -LiteralPath (Join-Path $repo 'build.ps1') -Destination $source
foreach ($name in @('build-paths.ps1', 'build-timing.ps1', 'build-web-assets.ps1')) {
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot $name) -Destination (Join-Path $source 'scripts')
}
[IO.File]::WriteAllText((Join-Path $source 'web\package.json'), '{"name":"fixture","version":"1.0.0"}')
[IO.File]::WriteAllText((Join-Path $source 'web\package-lock.json'), '{"lockfileVersion":3}')
# Real child processes exercise PowerShell 5.1 jobs, PATH inheritance, exit codes,
# spaces, locks and branch completion. No application or installation is started.
$toolScript = @'
param([string]$Tool)
$ErrorActionPreference = 'Stop'
function Event([string]$Name) {
    [IO.File]::WriteAllText((Join-Path $env:SL_BUILD_EVENTS $Name), [DateTime]::UtcNow.ToString('o'))
}
switch ($Tool) {
    'node' { '{"version":"' + $env:SL_BUILD_NODE + '","platform":"win32","arch":"x64"}'; exit 0 }
    'npm' {
        if ($args[0] -eq '--version') { $env:SL_BUILD_NPM; exit 0 }
        if ($args[0] -eq 'config') { @{fixture=$env:SL_BUILD_CONFIG} | ConvertTo-Json -Compress; exit 0 }
        if ($args[0] -eq 'ci') {
            Event 'npm-ci'
            if ($env:SL_BUILD_FAIL -eq 'ci') { exit 7 }
            $modules = Join-Path (Get-Location) 'node_modules'
            foreach ($directory in @('.bin', 'typescript', 'vite')) { New-Item -ItemType Directory -Path (Join-Path $modules $directory) -Force | Out-Null }
            foreach ($tool in @('tsc.cmd', 'vite.cmd')) { [IO.File]::WriteAllText((Join-Path $modules ('.bin\'+$tool)), 'fixture') }
            [IO.File]::WriteAllText((Join-Path $modules '.package-lock.json'), '{"packages":{"node_modules/typescript":{},"node_modules/vite":{}}}')
            exit 0
        }
        if ($args[0] -eq 'run' -and $args[1] -eq 'build') {
            Event 'web.start'
            Start-Sleep -Milliseconds 800
            Event 'web.end'
            if ($env:SL_BUILD_FAIL -eq 'web') { exit 8 }
            New-Item -ItemType Directory -Path server/dist -Force | Out-Null
            [IO.File]::WriteAllText((Join-Path (Get-Location) 'server\dist\index.html'), 'built')
            exit 0
        }
        throw "Unexpected npm arguments: $args"
    }
    'go' {
        if ($args[0] -ne 'build') { throw "Unexpected go arguments: $args" }
        $name = if ($args -contains './cmd/steerline-web') { 'go.web' } else { 'go.steerline' }
        Event ($name+'.start')
        if ($name -eq 'go.web' -and -not (Test-Path web/server/dist/index.html)) { throw 'Web Go build preceded assets.' }
        Start-Sleep -Milliseconds 800
        Event ($name+'.end')
        if ($env:SL_BUILD_FAIL -eq 'go' -and $name -eq 'go.steerline') { exit 9 }
        $outputIndex = [Array]::IndexOf($args, '-o') + 1
        [IO.File]::WriteAllText($args[$outputIndex], $name)
        exit 0
    }
}
'@
[IO.File]::WriteAllText((Join-Path $tools 'tool.ps1'), $toolScript)
foreach ($tool in @('node', 'npm', 'go')) {
    $command = '@echo off' + "`r`n" + 'powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0tool.ps1" ' + $tool + ' %*' + "`r`n" + 'exit /b %ERRORLEVEL%'
    [IO.File]::WriteAllText((Join-Path $tools ($tool+'.cmd')), $command)
}
$runnerScript = @'
$ErrorActionPreference = 'Stop'
[IO.File]::WriteAllText((Join-Path $env:SL_BUILD_EVENTS 'runner.start'), [DateTime]::UtcNow.ToString('o'))
Start-Sleep -Milliseconds 800
[IO.File]::WriteAllText((Join-Path $env:SL_BUILD_EVENTS 'runner.end'), [DateTime]::UtcNow.ToString('o'))
if ($env:SL_BUILD_FAIL -eq 'runner') { throw 'Injected Runner build failure.' }
$bin = Join-Path $PSScriptRoot '..\bin'
New-Item -ItemType Directory -Path $bin -Force | Out-Null
[IO.File]::WriteAllText((Join-Path $bin 'steerrunner.exe'), 'runner')
'@
[IO.File]::WriteAllText((Join-Path $runner 'scripts\build.ps1'), $runnerScript)
$savedPath = $env:PATH
$savedEnvironment = @{}
foreach ($name in @('SL_BUILD_EVENTS','SL_BUILD_NODE','SL_BUILD_NPM','SL_BUILD_CONFIG','SL_BUILD_FAIL')) {
    $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name)
}
$checks = [Collections.Generic.List[string]]::new()
function Assert([bool]$Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
function Run-Build([string]$Name, [bool]$ShouldSucceed=$true, [string[]]$Extra=@()) {
    $directory = Join-Path $OutDir $Name
    New-Item -ItemType Directory -Path $directory | Out-Null
    $env:SL_BUILD_EVENTS = $directory
    $reportPath = Join-Path $directory 'timings.json'
    $stagePath = Join-Path $source ('bin\snapshot-'+$Name)
    $savedPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        & powershell.exe -NoProfile -ExecutionPolicy Bypass -File (Join-Path $source 'build.ps1') -Web -RunnerRepo $runner -StageDirectory $stagePath -TimingPath $reportPath @Extra *> (Join-Path $directory 'output.log')
        $code = $LASTEXITCODE
    } finally { $ErrorActionPreference = $savedPreference }
    Assert (($code -eq 0) -eq $ShouldSucceed) "$Name unexpected exit $code; see $directory\output.log"
    if (-not $ShouldSucceed) {
        Assert (-not (Test-Path -LiteralPath $stagePath)) "$Name produced a deployable snapshot after failure."
    } else {
        foreach ($exe in @('steerline.exe','steerline-web.exe','steerrunner.exe')) {
            Assert (Test-Path -LiteralPath (Join-Path $stagePath $exe)) "$Name missing staged $exe"
        }
    }
    $checks.Add($Name)
    if (Test-Path -LiteralPath $reportPath) {
        $report = Get-Content -LiteralPath $reportPath -Raw | ConvertFrom-Json
        Assert ($report.succeeded -eq $ShouldSucceed) "$Name incorrect report outcome"
        return $report
    }
    Assert (-not $ShouldSucceed) "$Name omitted timing report"
}
function Expect-Install($Report, [string]$Outcome) {
    $stage = @($Report.stages | Where-Object name -eq 'web.dependencies.install')
    Assert ($stage.Count -eq 1 -and $stage[0].outcome -eq $Outcome) "Expected dependency install $Outcome"
}
try {
    $env:PATH = $tools + ';' + $env:PATH
    $env:SL_BUILD_NODE = 'fixture-node-1'
    $env:SL_BUILD_NPM = 'fixture-npm-1'
    $env:SL_BUILD_CONFIG = 'one'
    $env:SL_BUILD_FAIL = ''
    $cold = Run-Build 'cold'
    Expect-Install $cold 'succeeded'
    $warm = Run-Build 'warm'
    Expect-Install $warm 'skipped'
    $runnerStage = $warm.stages | Where-Object name -eq 'go.runner'
    $backendStage = $warm.stages | Where-Object name -eq 'go.steerline'
    $frontend = $warm.stages | Where-Object name -eq 'web.dependencies.check'
    $frontendEnd = $warm.stages | Where-Object name -eq 'web.assets'
    Assert (([DateTime]$frontend.started_at -lt [DateTime]$backendStage.completed_at) -and ([DateTime]$frontendEnd.completed_at -gt [DateTime]$runnerStage.started_at)) 'Frontend did not overlap the Go pipeline.'
    $serial = Run-Build 'serial' $true @('-SerialBuild')
    Expect-Install $serial 'skipped'
    $backendEnd = $serial.stages | Where-Object name -eq 'go.steerline'
    $serialFrontend = $serial.stages | Where-Object name -eq 'web.dependencies.check'
    Assert ([DateTime]$serialFrontend.started_at -ge [DateTime]$backendEnd.completed_at) 'SerialBuild still overlapped branches.'
    Expect-Install (Run-Build 'force' $true @('-ForceWebInstall')) 'succeeded'
    Add-Content -LiteralPath (Join-Path $source 'web\package.json') -Value ' '
    Expect-Install (Run-Build 'package-change') 'succeeded'
    Add-Content -LiteralPath (Join-Path $source 'web\package-lock.json') -Value ' '
    Expect-Install (Run-Build 'lock-change') 'succeeded'
    $env:SL_BUILD_NODE = 'fixture-node-2'
    Expect-Install (Run-Build 'node-change') 'succeeded'
    $env:SL_BUILD_NPM = 'fixture-npm-2'
    Expect-Install (Run-Build 'npm-change') 'succeeded'
    $env:SL_BUILD_CONFIG = 'two'
    Expect-Install (Run-Build 'config-change') 'succeeded'
    [IO.File]::WriteAllText((Join-Path $source 'web\.npmrc'), '# changed npm configuration')
    Expect-Install (Run-Build 'npmrc-change') 'succeeded'
    $modules = Join-Path $source 'web\node_modules'
    Remove-Item -LiteralPath (Join-Path $modules '.bin\vite.cmd')
    Expect-Install (Run-Build 'missing-tool') 'succeeded'
    Remove-Item -LiteralPath (Join-Path $modules 'vite')
    Expect-Install (Run-Build 'missing-package') 'succeeded'
    Add-Content -LiteralPath (Join-Path $modules '.package-lock.json') -Value ' '
    Expect-Install (Run-Build 'installed-lock-change') 'succeeded'
    [IO.File]::WriteAllText((Join-Path $modules '.steerline-install.json'), 'bad json')
    Expect-Install (Run-Build 'bad-marker') 'succeeded'
    $env:SL_BUILD_FAIL = 'ci'
    $failed = Run-Build 'ci-failure' $false @('-ForceWebInstall')
    Expect-Install $failed 'failed'
    Assert (-not (Test-Path -LiteralPath (Join-Path $modules '.steerline-install.json'))) 'Failed install retained its success marker.'
    Assert (-not ($failed.stages | Where-Object name -eq 'go.web')) 'Web Go build ran after frontend failure.'
    $env:SL_BUILD_FAIL = ''
    Expect-Install (Run-Build 'retry-after-ci-failure') 'succeeded'
    foreach ($mode in @('web', 'go', 'runner')) {
        $env:SL_BUILD_FAIL = $mode
        $failed = Run-Build ($mode+'-failure') $false
        Assert (-not ($failed.stages | Where-Object name -eq 'go.web')) "$mode failure still built Web EXE."
        Assert (Test-Path -LiteralPath (Join-Path $env:SL_BUILD_EVENTS 'web.end')) "$mode failure abandoned frontend work."
        Assert (@($failed.stages | Where-Object outcome -eq 'failed').Count -gt 0) "$mode failure missing failed timing."
    }
    $env:SL_BUILD_FAIL = ''
    $lock = [IO.File]::Open((Join-Path $source 'bin\.build.lock'), [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
    try { $null = Run-Build 'source-lock' $false } finally { $lock.Dispose() }
    Assert (-not (Test-Path -LiteralPath (Join-Path $env:SL_BUILD_EVENTS 'runner.start'))) 'Conflicting build started work.'
    $lock = [IO.File]::Open((Join-Path $runner 'bin\.steerline-build.lock'), [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
    try { $null = Run-Build 'runner-lock' $false } finally { $lock.Dispose() }
    Assert (-not (Test-Path -LiteralPath (Join-Path $env:SL_BUILD_EVENTS 'web.start'))) 'Conflicting Runner build started frontend.'
    @{passed=$true; powershell=$PSVersionTable.PSVersion.ToString(); checks=@($checks)} | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $OutDir 'result.json')
    Write-Host "Build pipeline checks passed ($($checks.Count)); evidence: $OutDir"
} finally {
    $env:PATH = $savedPath
    foreach ($name in $savedEnvironment.Keys) { [Environment]::SetEnvironmentVariable($name, $savedEnvironment[$name]) }
}
