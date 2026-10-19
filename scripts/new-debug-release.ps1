param(
    [string]$DebugRoot = 'C:\SteerlineDebug',
    [string]$WebAddress = '',
    [ValidateSet(1,2)][int]$WebProjects = 1,
    [string]$SteerRunnerExe = '',
    [switch]$Orca,
    [string]$OrcaEnvironment = '台式机64G',
    [string]$OrcaAgent = 'codex',
    [string]$OrcaExecutable = 'orca',
    [string]$OrcaRuntimeExecutable = '',
    [string]$OrcaRuntimeSHA256 = '',
    [string]$AgentModel = '',
    [string]$AgentExecutable = '',
    [ValidateSet('','medium','high','xhigh')][string]$AgentReasoningEffort = ''
)
$ErrorActionPreference = 'Stop'
if ($Orca -and -not $OrcaEnvironment) {
    if (-not $OrcaRuntimeExecutable -or $OrcaRuntimeExecutable -match '[\x00\r\n]' -or
        $OrcaRuntimeExecutable -notmatch '^(?:[A-Za-z]:[\\/]|/)') {
        throw 'Orca requires -OrcaRuntimeExecutable <absolute execution-host steerrunner-agent path>'
    }
    if ($OrcaRuntimeSHA256 -notmatch '\A[a-fA-F0-9]{64}\z') {
        throw 'Orca requires -OrcaRuntimeSHA256 <execution-host steerrunner-agent SHA256>'
    }
    $OrcaRuntimeSHA256 = $OrcaRuntimeSHA256.ToLowerInvariant()
} elseif ($OrcaRuntimeExecutable -or $OrcaRuntimeSHA256) {
    throw 'Helper runtime parameters apply only to Orca local; remote Orca uses the native agent'
}
if (-not $WebAddress) {
    $portProbe = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    $portProbe.Start()
    try { $WebAddress = '127.0.0.1:' + $portProbe.LocalEndpoint.Port } finally { $portProbe.Stop() }
}
if ($WebAddress -and ($WebAddress -notmatch '^127\.0\.0\.1:([1-9][0-9]{0,4})$' -or [int]$Matches[1] -gt 65535)) { throw 'WebAddress must be a fixed 127.0.0.1:<1..65535>' }

function Assert-PlainPath([string]$Path) {
    $cursor = [IO.Path]::GetFullPath($Path)
    while ($cursor) {
        $item = Get-Item -LiteralPath $cursor -Force -ErrorAction SilentlyContinue
        if ($item -and (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $item.LinkType -eq 'HardLink')) {
            throw "Linked path is not allowed: $cursor"
        }
        $parent = [IO.Path]::GetDirectoryName($cursor)
        if ($parent -eq $cursor) { break }
        $cursor = $parent
    }
}
function Is-Within([string]$Parent, [string]$Child) {
    return $Child.Equals($Parent, [StringComparison]::OrdinalIgnoreCase) -or $Child.StartsWith($Parent.TrimEnd('\') + '\', [StringComparison]::OrdinalIgnoreCase)
}

$runnerSource = $null
if (-not $SteerRunnerExe -and $env:STEERRUNNER_EXE) { $SteerRunnerExe = $env:STEERRUNNER_EXE }
if (-not $SteerRunnerExe) {
    $runnerCommand = Get-Command steerrunner.exe -ErrorAction SilentlyContinue
    if (-not $runnerCommand) { throw 'Select a released Runner with -SteerRunnerExe <absolute exe>' }
    $SteerRunnerExe = $runnerCommand.Source
}
if ($SteerRunnerExe) {
    if (-not [IO.Path]::IsPathRooted($SteerRunnerExe)) { throw 'SteerRunnerExe must be absolute' }
    $SteerRunnerExe = [IO.Path]::GetFullPath($SteerRunnerExe)
    foreach ($installedHome in @('C:\SteerlineInstall','C:\SteerlineRuntime')) {
        if (Is-Within $installedHome $SteerRunnerExe) { throw 'SteerRunnerExe must not use the installed environment' }
    }
    Assert-PlainPath $SteerRunnerExe
    if (-not (Test-Path -LiteralPath $SteerRunnerExe -PathType Leaf)) { throw 'SteerRunnerExe does not exist' }
    $runnerVersionText = & $SteerRunnerExe version --json
    if ($LASTEXITCODE -ne 0) { throw 'Cannot query the selected SteerRunner executable' }
    $runnerVersion = $runnerVersionText | ConvertFrom-Json
    if (-not $runnerVersion.ok -or $runnerVersion.protocol -ne 'steerrunner-cli/v1' -or 'steerrunner-cli/v1' -notin $runnerVersion.data.protocols) { throw 'SteerRunner protocol is unsupported' }
    $runnerSource = [ordered]@{ version=$runnerVersion.data.version; source_commit=$runnerVersion.data.source_commit; protocol='steerrunner-cli/v1'; resource_digest=$runnerVersion.data.resource_digest; sha256=(Get-FileHash -LiteralPath $SteerRunnerExe -Algorithm SHA256).Hash.ToLowerInvariant() }
}

$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if (-not [IO.Path]::IsPathRooted($DebugRoot)) { throw 'DebugRoot must be absolute' }
$debugBase = [IO.Path]::GetFullPath($DebugRoot).TrimEnd('\')
if ((Is-Within 'C:\SteerlineInstall' $debugBase) -or (Is-Within $debugBase 'C:\SteerlineInstall') -or (Is-Within 'C:\SteerlineRuntime' $debugBase) -or (Is-Within $debugBase 'C:\SteerlineRuntime') -or (Is-Within $repo $debugBase) -or (Is-Within $debugBase $repo)) {
    throw 'Debug output must be disjoint from the installed home and source checkout'
}
Assert-PlainPath $debugBase
if ($debugBase.IndexOfAny([char[]]"%'`r`n") -ge 0) { throw 'DebugRoot contains unsupported launcher/template characters' }
if (-not (Is-Within 'C:\SteerlineDebug' $debugBase)) { throw 'DebugRoot must stay under C:\SteerlineDebug' }
$template = Join-Path $repo 'debug-template'
Assert-PlainPath $template
$files = @(Get-ChildItem -LiteralPath $template -Recurse -Force)
foreach ($item in $files) {
    Assert-PlainPath $item.FullName
    if ($item.Name -in @('.git', 'auth.json', 'steerline.db') -or $item.Extension -eq '.exe') { throw "Runtime state in copy source: $($item.FullName)" }
}
foreach ($command in @('git', 'go', 'node', 'npm')) { Get-Command $command -ErrorAction Stop | Out-Null }
$commit = (& git -C $repo rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0) { throw 'Cannot resolve source commit' }
$dirty = -not [string]::IsNullOrWhiteSpace((& git -C $repo status --porcelain | Out-String))
$created = [TimeZoneInfo]::ConvertTimeFromUtc([DateTime]::UtcNow, [TimeZoneInfo]::FindSystemTimeZoneById('China Standard Time'))
$release = 'dev-' + $created.ToString('yyyyMMdd-HHmmss') + '-' + $commit.Substring(0, 8)
if ($dirty) { $release += '-dirty' }
$target = Join-Path $debugBase $release
if (Test-Path -LiteralPath $target) { throw "Debug release already exists: $target" }
New-Item -ItemType Directory -Path $debugBase -Force | Out-Null
New-Item -ItemType Directory -Path $target | Out-Null
$utf8 = New-Object Text.UTF8Encoding $false
$baseBranch = 'codex/debug/' + $release + '/base'
$manifest = [ordered]@{ version = 1; release = $release; source_commit = $commit; source_dirty = $dirty; created_at = $created.ToString('yyyy-MM-ddTHH:mm:ss+08:00'); base_branch = $baseBranch; default_scheme = 'live'; status = 'building' }
function Write-Manifest { [IO.File]::WriteAllText((Join-Path $target 'manifest.json'), ($manifest | ConvertTo-Json -Depth 10), $utf8) }
Write-Manifest
try {
    $packageAcl = Get-Acl -LiteralPath $target
    $packageAcl.SetAccessRuleProtection($true, $false)
    $currentUser = [Security.Principal.WindowsIdentity]::GetCurrent().User
    $packageRule = [Security.AccessControl.FileSystemAccessRule]::new(
        $currentUser,
        [Security.AccessControl.FileSystemRights]::FullControl,
        [Security.AccessControl.InheritanceFlags]::ContainerInherit -bor [Security.AccessControl.InheritanceFlags]::ObjectInherit,
        [Security.AccessControl.PropagationFlags]::None,
        [Security.AccessControl.AccessControlType]::Allow
    )
    $packageAcl.SetAccessRule($packageRule)
    Set-Acl -LiteralPath $target -AclObject $packageAcl
    foreach ($item in Get-ChildItem -LiteralPath $template -Force) { Copy-Item -LiteralPath $item.FullName -Destination $target -Recurse }
    foreach ($dir in @('bin', 'logs', 'evidence', 'provider', 'provider-b', 'remotes', 'home\temp', 'home\codex', 'home\gh', 'home\state', 'home\etc')) {
        New-Item -ItemType Directory -Path (Join-Path $target $dir) -Force | Out-Null
    }
    if ($runnerSource) {
        $runnerDestination = Join-Path $target 'bin\steerrunner.exe'
        Copy-Item -LiteralPath $SteerRunnerExe -Destination $runnerDestination
        if ((Get-FileHash -LiteralPath $runnerDestination -Algorithm SHA256).Hash.ToLowerInvariant() -ne $runnerSource.sha256) { throw 'SteerRunner changed during packaging' }
        $manifest.runner = $runnerSource
    }
    $rootText = $target.Replace('\', '/')
    $templateHashes = [ordered]@{}
    foreach ($item in $files | Where-Object { -not $_.PSIsContainer }) {
        $relative = $item.FullName.Substring($template.Length + 1)
        $templateHashes[$relative] = (Get-FileHash -LiteralPath $item.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        $output = Join-Path $target $relative
        $content = [IO.File]::ReadAllText($output).Replace('__ROOT__', $rootText).Replace('__BASE__', $baseBranch).Replace('__WEB_ADDRESS__', $WebAddress).Replace('__RUNNER_VERSION__', $runnerSource.version).Replace('__RUNNER_SHA256__', $runnerSource.sha256)
        # Windows PowerShell 5 reads a BOM-less script using the system code page.
        $encoding = $utf8
        if ($item.Extension -eq '.ps1') { $encoding = New-Object Text.UTF8Encoding $true }
        [IO.File]::WriteAllText($output, $content, $encoding)
    }
    if ($Orca) {
        foreach ($mapping in @(
            @('task.spec.md', 'projects\SteerlineTest\docs\spec\task.spec.md'),
            @('task.plan.md', 'projects\SteerlineTest\docs\spec\task.plan.md'),
            @('AGENTS.md', 'projects\SteerlineTest\AGENTS.md'),
            @('publish.ps1', 'projects\SteerlineTest\publish.ps1')
        )) {
            $source = Join-Path (Join-Path $template 'orca') $mapping[0]
            $destination = Join-Path $target $mapping[1]
            $content = [IO.File]::ReadAllText($source).Replace('__BASE__', $baseBranch)
            $overlayEncoding = if ($mapping[0] -eq 'publish.ps1') { New-Object Text.UTF8Encoding $true } else { $utf8 }
            [IO.File]::WriteAllText($destination, $content, $overlayEncoding)
        }
        if ($OrcaAgent -ne 'codex') { throw 'This Runner release supports the Codex Orca agent' }
        $executionTarget = @{kind='orca_local'}
        if ($OrcaEnvironment) { $executionTarget=@{kind='orca_environment';environment=$OrcaEnvironment} }
        $launchExecutable = if ($AgentExecutable) { $AgentExecutable } else { $OrcaAgent }
        $settings = @{
            transport=@{executable=$OrcaExecutable;wait_timeout='40s'}
            agent_launch=@{
                executable=$launchExecutable
                args=@('--no-daemon')
            }
        }
        if (-not $OrcaEnvironment) {
            $settings.agent_launch['runtime_executable']=$OrcaRuntimeExecutable
            $settings.agent_launch['runtime_sha256']=$OrcaRuntimeSHA256
        }
        $catalog = @{version=3;runner=@{executable="$rootText/bin/steerrunner.exe";version=$runnerSource.version;sha256=$runnerSource.sha256};default_execution_profile='live';execution_profiles=@(@{
            id='live';name='Orca single-terminal debug';execution=@{adapter='orca-terminal';target=$executionTarget;settings=$settings};workflow=@{id='development';version=2}
        })}
		if ($AgentModel -or $AgentReasoningEffort) {
			if (-not $AgentModel) { throw 'AgentReasoningEffort requires AgentModel' }
			$settings.agent_launch.model=$AgentModel
            if ($AgentReasoningEffort) { $settings.agent_launch.reasoning_effort=$AgentReasoningEffort }
		}
        [IO.File]::WriteAllText((Join-Path $target 'home\execution.json'), ($catalog | ConvertTo-Json -Depth 10), $utf8)
        $manifest.execution_host='orca'
        $manifest.execution_environment=$OrcaEnvironment
        if (-not $OrcaEnvironment) { $manifest.orca_runtime=@{executable=$OrcaRuntimeExecutable;sha256=$OrcaRuntimeSHA256} }
    }
    if (-not $Orca -and ($AgentExecutable -or $AgentModel -or $AgentReasoningEffort)) {
        $executionPath = Join-Path $target 'home\execution.json'
        $catalog = [IO.File]::ReadAllText($executionPath) | ConvertFrom-Json
        $settings = ($catalog.execution_profiles | Where-Object id -eq 'live').execution.settings
        if ($AgentExecutable) { $settings.executable = $AgentExecutable }
        if ($AgentModel) { $settings.model = $AgentModel }
        if ($AgentReasoningEffort) { $settings.reasoning_effort = $AgentReasoningEffort }
        [IO.File]::WriteAllText($executionPath, ($catalog | ConvertTo-Json -Depth 10), $utf8)
    }
    $manifest.web_projects = $WebProjects
    Write-Manifest
    $manifest.template_sha256 = $templateHashes
    $manifest.generator_sha256 = (Get-FileHash -LiteralPath $PSCommandPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $savedGoFlags = $env:GOFLAGS
    $env:GOFLAGS = ''
    Push-Location $repo
    try {
        $dirtyText = if ($dirty) { 'true' } else { 'false' }
        $ldflags = "-X github.com/erranliu/SteerLine/internal/app.buildSourceCommit=$commit -X github.com/erranliu/SteerLine/internal/app.buildSourceDirty=$dirtyText"
        $productBuildArgs = @('build', '-trimpath', '-ldflags', $ldflags)
        foreach ($webPath in @('web','web\server','web\server\dist','web\node_modules')) {
            Assert-PlainPath (Join-Path $repo $webPath)
        }
        $webSeedPath = Join-Path $target 'seed\provider.json'
        $webSeed = [IO.File]::ReadAllText($webSeedPath) | ConvertFrom-Json
        $webSeed.web_feedback = $true
        [IO.File]::WriteAllText($webSeedPath, ($webSeed | ConvertTo-Json -Depth 10), $utf8)
        Push-Location (Join-Path $repo 'web')
        try {
            & npm ci *> (Join-Path $target 'evidence\install-web.log')
            if ($LASTEXITCODE -ne 0) { throw 'Web dependency install failed; see evidence/install-web.log' }
            & npm run build *> (Join-Path $target 'evidence\build-web.log')
            if ($LASTEXITCODE -ne 0) { throw 'Web build failed; see evidence/build-web.log' }
        } finally { Pop-Location }
        & go build -trimpath -tags webui -ldflags $ldflags -o (Join-Path $target 'bin\steerline-web.exe') ./cmd/steerline-web
        if ($LASTEXITCODE -ne 0) { throw 'Web executable build failed' }
        $manifest.web_projects = $WebProjects
        [IO.File]::WriteAllText((Join-Path $target 'home\web.json'), (@{version=1;address=$WebAddress} | ConvertTo-Json), $utf8)
        $manifest.web_address = $WebAddress
        $productBuildArgs += @('-o', (Join-Path $target 'bin\steerline.exe'), './cmd/steerline')
        & go @productBuildArgs 2> (Join-Path $target 'evidence\build-product.log')
        if ($LASTEXITCODE -ne 0) { throw 'Product build failed; see evidence/build-product.log' }
        & go build -trimpath -o (Join-Path $target 'bin\debug-helper.exe') ./tests/e2e/testdata/debug-helper 2> (Join-Path $target 'evidence\build-helper.log')
        if ($LASTEXITCODE -ne 0) { throw 'Helper build failed; see evidence/build-helper.log' }
    } finally { Pop-Location; $env:GOFLAGS = $savedGoFlags }
    $savedSteerlineHome = $env:STEERLINE_HOME
    try {
        $env:STEERLINE_HOME = Join-Path $target 'home'
        & (Join-Path $target 'bin\steerline.exe') install *> (Join-Path $target 'evidence\install-product.log')
        if ($LASTEXITCODE -ne 0) { throw 'Product install failed' }
        & (Join-Path $target 'bin\steerline-web.exe') install *> (Join-Path $target 'evidence\install-web-product.log')
        if ($LASTEXITCODE -ne 0) { throw 'Web install failed' }
    } finally { $env:STEERLINE_HOME = $savedSteerlineHome }
    Copy-Item -LiteralPath (Join-Path $target 'home\config-source\SteerlineTest.live.json') -Destination (Join-Path $target 'home\etc\SteerlineTest.json')
    & (Join-Path $target 'bin\debug-helper.exe') debug init *> (Join-Path $target 'evidence\init.log')
    if ($LASTEXITCODE -ne 0) { throw 'Debug initialization failed; see evidence/init.log' }
    if ($WebProjects -eq 2) {
        & git clone --quiet --no-hardlinks (Join-Path $target 'projects\SteerlineTest') (Join-Path $target 'projects\SteerlineTestB')
        if ($LASTEXITCODE -ne 0) { throw 'Second independent clone failed' }
        & git -C (Join-Path $target 'projects\SteerlineTestB') remote set-url origin 'https://github.com/erranliu/SteerlineTest.git'
        & git -C (Join-Path $target 'projects\SteerlineTestB') checkout --quiet -b ($baseBranch + '-b')
        if ($LASTEXITCODE -ne 0) { throw 'Second base branch failed' }
        foreach ($key in @(@('user.name','SteerLine Debug'),@('user.email','debug@example.invalid'),@('commit.gpgsign','false'),@('core.autocrlf','false'),@('gc.auto','0'))) {
            & git -C (Join-Path $target 'projects\SteerlineTestB') config $key[0] $key[1]
            if ($LASTEXITCODE -ne 0) { throw 'Second clone configuration failed' }
        }
        foreach ($name in @('AGENTS.md','publish.ps1','docs\spec\task.spec.md','docs\spec\task.plan.md')) {
            $material = Join-Path $target ('projects\SteerlineTestB\' + $name)
            $content = [IO.File]::ReadAllText($material).Replace($baseBranch,($baseBranch + '-b'))
            $encoding = if ($name.EndsWith('.ps1')) { New-Object Text.UTF8Encoding $true } else { $utf8 }
            [IO.File]::WriteAllText($material,$content,$encoding)
        }
        & git -C (Join-Path $target 'projects\SteerlineTestB') add .
        & git -C (Join-Path $target 'projects\SteerlineTestB') commit --quiet --allow-empty -m 'Bind second SteerlineTest fixture base'
        if ($LASTEXITCODE -ne 0) { throw 'Second clone contract commit failed' }
        foreach ($mode in @('live','simulate')) {
            $content = [IO.File]::ReadAllText((Join-Path $target "home\config-source\SteerlineTest.$mode.json"))
            $content = $content.Replace('/projects/SteerlineTest','/projects/SteerlineTestB').Replace('/home/worktrees','/home/worktrees-b').Replace('/provider/state.json','/provider-b/state.json').Replace($baseBranch,($baseBranch + '-b'))
            [IO.File]::WriteAllText((Join-Path $target "home\config-source\SteerlineTestB.$mode.json"), $content, $utf8)
        }
        Copy-Item -LiteralPath (Join-Path $target 'home\config-source\SteerlineTestB.live.json') -Destination (Join-Path $target 'home\etc\SteerlineTestB.json')
        & (Join-Path $target 'bin\debug-helper.exe') debug init-b *> (Join-Path $target 'evidence\init-b.log')
        if ($LASTEXITCODE -ne 0) { throw 'Second project initialization failed; see evidence/init-b.log' }
    }
    $manifest.web_binary_sha256 = (Get-FileHash -LiteralPath (Join-Path $target 'bin\steerline-web.exe')).Hash.ToLowerInvariant()
    $manifest.binary_sha256 = (Get-FileHash -LiteralPath (Join-Path $target 'bin\steerline.exe')).Hash.ToLowerInvariant()
    $manifest.helper_sha256 = (Get-FileHash -LiteralPath (Join-Path $target 'bin\debug-helper.exe')).Hash.ToLowerInvariant()
    $manifest.status = 'ready'
    Write-Manifest
    [pscustomobject]@{ release = $release; root = $target; start = (Join-Path $target 'start.bat'); stop = (Join-Path $target 'stop.bat') } | ConvertTo-Json -Compress
} catch {
    $manifest.status = 'failed'
    $manifest.error = $_.Exception.Message
    Write-Manifest
    throw
}
