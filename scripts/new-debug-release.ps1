param([string]$DebugRoot = 'C:\SteerlineDebug')
$ErrorActionPreference = 'Stop'

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

$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if (-not [IO.Path]::IsPathRooted($DebugRoot)) { throw 'DebugRoot must be absolute' }
$debugBase = [IO.Path]::GetFullPath($DebugRoot).TrimEnd('\')
Assert-PlainPath $debugBase
if ((Is-Within 'C:\SteerlineRuntime' $debugBase) -or (Is-Within $debugBase 'C:\SteerlineRuntime') -or (Is-Within $repo $debugBase) -or (Is-Within $debugBase $repo)) {
    throw 'Debug output must be disjoint from the installed home and source checkout'
}
if ($debugBase.IndexOfAny([char[]]"%'`r`n") -ge 0) { throw 'DebugRoot contains unsupported launcher/template characters' }
if (-not (Is-Within 'C:\SteerlineDebug' $debugBase)) { throw 'DebugRoot must stay under C:\SteerlineDebug' }
$template = Join-Path $repo 'debug-template'
Assert-PlainPath $template
$files = @(Get-ChildItem -LiteralPath $template -Recurse -Force)
foreach ($item in $files) {
    Assert-PlainPath $item.FullName
    if ($item.Name -in @('.git', 'auth.json', 'steerline.db') -or $item.Extension -eq '.exe') { throw "Runtime state in copy source: $($item.FullName)" }
}
foreach ($command in @('git', 'go')) { Get-Command $command -ErrorAction Stop | Out-Null }
$commit = (& git -C $repo rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0) { throw 'Cannot resolve source commit' }
$dirty = -not [string]::IsNullOrWhiteSpace((& git -C $repo status --porcelain | Out-String))
$version = 'dev-' + $commit.Substring(0, 8)
if ($dirty) { $version += '-dirty' }
$created = [TimeZoneInfo]::ConvertTimeFromUtc([DateTime]::UtcNow, [TimeZoneInfo]::FindSystemTimeZoneById('China Standard Time'))
$release = $version + '-' + $created.ToString('yyyyMMdd-HHmmss')
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
    foreach ($item in Get-ChildItem -LiteralPath $template -Force) { Copy-Item -LiteralPath $item.FullName -Destination $target -Recurse }
    foreach ($dir in @('bin', 'logs', 'evidence', 'provider', 'remotes', 'home\temp', 'home\codex', 'home\gh', 'home\state', 'home\ui\views')) {
        New-Item -ItemType Directory -Path (Join-Path $target $dir) -Force | Out-Null
    }
    $rootText = $target.Replace('\', '/')
    $templateHashes = [ordered]@{}
    foreach ($item in $files | Where-Object { -not $_.PSIsContainer }) {
        $relative = $item.FullName.Substring($template.Length + 1)
        $templateHashes[$relative] = (Get-FileHash -LiteralPath $item.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        $output = Join-Path $target $relative
        $content = [IO.File]::ReadAllText($output).Replace('__ROOT__', $rootText).Replace('__BASE__', $baseBranch)
        # Windows PowerShell 5 reads a BOM-less script using the system code page.
        $encoding = $utf8
        if ($item.Extension -eq '.ps1') { $encoding = New-Object Text.UTF8Encoding $true }
        [IO.File]::WriteAllText($output, $content, $encoding)
    }
    $manifest.template_sha256 = $templateHashes
    $manifest.generator_sha256 = (Get-FileHash -LiteralPath $PSCommandPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $savedGoFlags = $env:GOFLAGS
    $env:GOFLAGS = ''
    Push-Location $repo
    try {
        $dirtyText = if ($dirty) { 'true' } else { 'false' }
        $ldflags = "-X github.com/erranliu/SteerLine/internal/app.buildSourceCommit=$commit -X github.com/erranliu/SteerLine/internal/app.buildSourceDirty=$dirtyText"
        & go build -trimpath -ldflags $ldflags -o (Join-Path $target 'bin\steerline.exe') ./cmd/steerline 2> (Join-Path $target 'evidence\build-product.log')
        if ($LASTEXITCODE -ne 0) { throw 'Product build failed; see evidence/build-product.log' }
        & go build -trimpath -o (Join-Path $target 'bin\debug-helper.exe') ./tests/e2e/testdata/debug-helper 2> (Join-Path $target 'evidence\build-helper.log')
        if ($LASTEXITCODE -ne 0) { throw 'Helper build failed; see evidence/build-helper.log' }
    } finally { Pop-Location; $env:GOFLAGS = $savedGoFlags }
    Copy-Item -LiteralPath (Join-Path $target 'home\etc\SteerlineTest.live.json') -Destination (Join-Path $target 'home\etc\SteerlineTest.json')
    & (Join-Path $target 'bin\debug-helper.exe') debug init *> (Join-Path $target 'evidence\init.log')
    if ($LASTEXITCODE -ne 0) { throw 'Debug initialization failed; see evidence/init.log' }
    $manifest.binary_sha256 = (Get-FileHash -LiteralPath (Join-Path $target 'bin\steerline.exe')).Hash.ToLowerInvariant()
    $manifest.helper_sha256 = (Get-FileHash -LiteralPath (Join-Path $target 'bin\debug-helper.exe')).Hash.ToLowerInvariant()
    $manifest.status = 'ready'
    Write-Manifest
    [pscustomobject]@{ release = $release; root = $target; start = (Join-Path $target 'start.bat'); console = (Join-Path $target 'console.bat') } | ConvertTo-Json -Compress
} catch {
    $manifest.status = 'failed'
    $manifest.error = $_.Exception.Message
    Write-Manifest
    throw
}
