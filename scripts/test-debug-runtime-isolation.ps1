param(
    [Parameter(Mandatory = $true)]
    [string]$Out,

    # Requires Windows symbolic-link creation privilege or Developer Mode.
    [switch]$IncludeSymbolicLinks
)

$ErrorActionPreference = 'Stop'
if (-not [IO.Path]::IsPathRooted($Out) -or (Test-Path -LiteralPath $Out)) {
    throw 'Out must be a new absolute evidence directory.'
}
$script = Join-Path $PSScriptRoot 'debug-runtime.ps1'
$buildExe = Join-Path (Split-Path -Parent $PSScriptRoot) 'bin\steerline.exe'
if (-not (Test-Path -LiteralPath $buildExe -PathType Leaf)) {
    throw "Build the real source executable first: '$buildExe'"
}
$powershell = (Get-Command powershell.exe -CommandType Application).Source
$utf8 = New-Object Text.UTF8Encoding -ArgumentList $false
New-Item -ItemType Directory -Path $Out | Out-Null
$results = @()

function Write-Fixture([string]$Path, [string]$Content) {
    New-Item -ItemType Directory -Path (Split-Path -Parent $Path) -Force | Out-Null
    [IO.File]::WriteAllText($Path, $Content, $utf8)
}

function File-SHA256([string]$Path) {
    $algorithm = [Security.Cryptography.SHA256]::Create()
    $stream = [IO.File]::OpenRead($Path)
    try {
        return [BitConverter]::ToString($algorithm.ComputeHash($stream)).Replace('-', '')
    } finally {
        $stream.Dispose()
        $algorithm.Dispose()
    }
}

function Snapshot-Files([string]$Root) {
    $snapshot = [ordered]@{}
    foreach ($item in @(Get-ChildItem -LiteralPath $Root -Recurse -Force | Sort-Object FullName)) {
        $value = if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            'alias:' + [string]$item.Target
        } elseif ($item.PSIsContainer) {
            'directory'
        } else {
            (File-SHA256 $item.FullName)
        }
        $snapshot[$item.FullName.Substring($Root.Length)] = $value
    }
    return ConvertTo-Json -InputObject $snapshot -Compress
}

function Invoke-Debug([string]$CaseRoot, [string]$Operation) {
    $local:ErrorActionPreference = 'Continue'
    $output = & $powershell -NoProfile -ExecutionPolicy Bypass -File $script `
        -DebugHome (Join-Path $CaseRoot 'debug') -InstalledHome (Join-Path $CaseRoot 'installed') `
        -Config (Join-Path $CaseRoot 'debug\etc\project.json') -Operation $Operation `
        -CodexAuthHome (Join-Path $CaseRoot 'auth-source') 2>&1
    $code = $LASTEXITCODE
    $text = ($output | ForEach-Object { $_.ToString() }) -join "`r`n"
    [IO.File]::WriteAllText((Join-Path $CaseRoot ($Operation + '.log')), $text, $utf8)
    return @{ code = $code; output = $text }
}

function New-Fixture([string]$Name, [bool]$MissingAuth = $false) {
    $root = Join-Path $Out $Name
    $debug = Join-Path $root 'debug'
    $installed = Join-Path $root 'installed'
    New-Item -ItemType Directory -Path (Join-Path $root 'repository'), (Join-Path $installed 'codex') -Force | Out-Null
    Write-Fixture (Join-Path $debug 'etc\project.json') (ConvertTo-Json -InputObject @{ repository = (Join-Path $root 'repository') })
    Write-Fixture (Join-Path $installed 'bin\steerline.exe') 'synthetic installed executable sentinel'
    Write-Fixture (Join-Path $installed 'service-instance.json') '{"instance_id":"synthetic-unchanged-instance"}'
    Write-Fixture (Join-Path $installed 'task-facts.json') '{"task_id":83,"phase":"synthetic-sentinel"}'
    Write-Fixture (Join-Path $installed 'console.bat') 'synthetic installed console sentinel'
    Write-Fixture (Join-Path $installed 'service-window.bat') 'synthetic installed service sentinel'
    Write-Fixture (Join-Path $installed 'README.txt') 'synthetic installed readme sentinel'
    Write-Fixture (Join-Path $installed 'state\state-sentinel.txt') 'synthetic installed state sentinel'
    Write-Fixture (Join-Path $installed 'ui\views\ui-sentinel.txt') 'synthetic installed UI sentinel'
    if (-not $MissingAuth) {
        Write-Fixture (Join-Path $installed 'codex\auth.json') '{"synthetic":"installed-auth-sentinel"}'
    }
    Write-Fixture (Join-Path $root 'auth-source\auth.json') '{"synthetic":"debug-auth-source"}'
    return $root
}

# Run the real script, not extracted path helpers. Missing leaves exercise
# canonical resolution through an existing aliased parent.
$cases = @(
    @{ name = 'bin-junction'; operation = 'launchers'; path = 'bin'; target = 'bin'; type = 'Junction' },
    @{ name = 'state-junction'; operation = 'launchers'; path = 'state'; target = 'state'; type = 'Junction' },
    @{ name = 'ui-junction'; operation = 'launchers'; path = 'ui'; target = 'ui'; type = 'Junction' },
    @{ name = 'codex-junction-missing-auth'; operation = 'launchers'; path = 'codex'; target = 'codex'; type = 'Junction'; missingAuth = $true },
    @{ name = 'exe-hard-link'; operation = 'launchers'; path = 'bin\steerline.exe'; target = 'bin\steerline.exe'; type = 'HardLink' },
    @{ name = 'auth-hard-link'; operation = 'launchers'; path = 'codex\auth.json'; target = 'codex\auth.json'; type = 'HardLink' },
    @{ name = 'console-hard-link'; operation = 'launchers'; path = 'console.bat'; target = 'console.bat'; type = 'HardLink' },
    @{ name = 'service-hard-link'; operation = 'launchers'; path = 'service-window.bat'; target = 'service-window.bat'; type = 'HardLink' },
    @{ name = 'readme-hard-link'; operation = 'launchers'; path = 'README.txt'; target = 'README.txt'; type = 'HardLink' }
)
if ($IncludeSymbolicLinks) {
    $cases += @(
        @{ name = 'exe-file-alias'; operation = 'launchers'; path = 'bin\steerline.exe'; target = 'bin\steerline.exe'; type = 'SymbolicLink' },
        @{ name = 'auth-file-alias'; operation = 'launchers'; path = 'codex\auth.json'; target = 'codex\auth.json'; type = 'SymbolicLink' },
        @{ name = 'auth-dangling-file-alias'; operation = 'launchers'; path = 'codex\auth.json'; target = 'codex\auth.json'; type = 'SymbolicLink'; missingAuth = $true },
        @{ name = 'console-file-alias'; operation = 'launchers'; path = 'console.bat'; target = 'console.bat'; type = 'SymbolicLink' },
        @{ name = 'service-file-alias'; operation = 'launchers'; path = 'service-window.bat'; target = 'service-window.bat'; type = 'SymbolicLink' },
        @{ name = 'readme-file-alias'; operation = 'launchers'; path = 'README.txt'; target = 'README.txt'; type = 'SymbolicLink' }
    )
}
foreach ($case in $cases) {
    $root = New-Fixture $case.name ([bool]$case.missingAuth)
    $debug = Join-Path $root 'debug'
    $installed = Join-Path $root 'installed'
    $alias = Join-Path $debug $case.path
    $target = Join-Path $installed $case.target
    New-Item -ItemType Directory -Path (Split-Path -Parent $alias) -Force | Out-Null
    New-Item -ItemType $case.type -Path $alias -Target $target | Out-Null
    $installedBefore = Snapshot-Files $installed
    $debugBefore = Snapshot-Files $debug
    $result = Invoke-Debug $root $case.operation
    if ($result.code -eq 0 -or $result.output -notmatch 'Writable (path must resolve inside|file must not be a hard link)') {
        throw "Expected writable-path refusal for $($case.name): code=$($result.code) output=$($result.output)"
    }
    if ((Snapshot-Files $installed) -ne $installedBefore -or (Snapshot-Files $debug) -ne $debugBefore) {
        throw "Rejected $($case.name) changed installed or debug files before refusal."
    }
    $results += @{ name = $case.name; operation = $case.operation; refused = $true; installed_unchanged = $true; debug_unchanged = $true; log = (Join-Path $root ($case.operation + '.log')) }
}

$positive = New-Fixture 'ordinary-paths'
$installedBefore = Snapshot-Files (Join-Path $positive 'installed')
foreach ($operation in @('launchers', 'launchers')) {
    $result = Invoke-Debug $positive $operation
    if ($result.code -ne 0) {
        throw "Ordinary $operation failed: $($result.output)"
    }
}
$debug = Join-Path $positive 'debug'
if ((Test-Path -LiteralPath (Join-Path $debug 'bin\steerline.exe')) -or
    [IO.File]::ReadAllText((Join-Path $debug 'codex\auth.json')) -ne '{"synthetic":"debug-auth-source"}' -or
    (Snapshot-Files (Join-Path $positive 'installed')) -ne $installedBefore) {
    throw 'Ordinary launcher/auth generation changed installed files or copied the source exe into the debug home.'
}
$expectedExecutable = 'set "BIN=' + $buildExe + '"'
foreach ($launcher in @('console.bat', 'service-window.bat')) {
    $contents = [IO.File]::ReadAllText((Join-Path $debug $launcher))
    if (-not $contents.Contains('set "CODEX_HOME=%SHOME%\codex"') -or -not $contents.Contains($expectedExecutable)) {
        throw "Missing selected exe/CODEX_HOME binding in '$launcher'"
    }
}
$results += @{ name = 'ordinary-paths'; operations = @('launchers', 'launchers'); passed = $true; installed_unchanged = $true; debug_executable_not_copied = $true }
$summary = @{
    passed = $true
    test = 'debug-runtime writable-path preflight'
    candidate_head = (& git -C (Split-Path -Parent $PSScriptRoot) rev-parse HEAD)
    script_sha256 = (File-SHA256 $script)
    build_exe = $buildExe
    build_sha256 = (File-SHA256 $buildExe)
    fixture_facts = 'synthetic file sentinels; no running service/task database assertion'
    symbolic_links_selected = [bool]$IncludeSymbolicLinks
    cases = $results
}
[IO.File]::WriteAllText((Join-Path $Out 'summary.json'), (ConvertTo-Json -InputObject $summary -Depth 8), $utf8)
Write-Output "Passed $($results.Count) cases. Evidence: $Out"
