param(
    [switch]$ForceWebInstall,
    [Parameter(Mandatory=$true)][string]$TimingPath
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'build-paths.ps1')
. (Join-Path $PSScriptRoot 'build-timing.ps1')
$sourceRoot = Resolve-CanonicalPath (Join-Path $PSScriptRoot '..')
Initialize-BuildTiming 'web-assets'
$succeeded = $false

function Get-TextHash([string]$Text) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return [BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($Text))).Replace('-', '').ToLowerInvariant() }
    finally { $sha.Dispose() }
}

function Get-WebInstallFingerprint {
    $nodeIdentity = & node -p 'JSON.stringify({version:process.version,platform:process.platform,arch:process.arch})'
    if ($LASTEXITCODE -ne 0) { throw 'Cannot identify Node.' }
    $npmVersion = & npm --version
    if ($LASTEXITCODE -ne 0) { throw 'Cannot identify npm.' }
    # Hash effective config in memory only; never write registry credentials to a report.
    $rawConfig = & npm config list --json
    if ($LASTEXITCODE -ne 0) { throw 'Cannot read effective npm configuration.' }
    $config = ($rawConfig -join "`n") | ConvertFrom-Json
    if ($config.'dry-run' -or $config.'package-lock-only' -or $config.global) {
        throw 'Web build requires a real, local dependency installation.'
    }
    $parts = [Collections.Generic.List[string]]::new()
    $parts.Add('steerline-web-install/v1')
    $parts.Add(($nodeIdentity -join "`n"))
    $parts.Add(($npmVersion -join "`n"))
    $parts.Add((Get-Command npm -ErrorAction Stop).Source)
    $parts.Add(($rawConfig -join "`n"))
    $parts.Add("NODE_ENV=$env:NODE_ENV;NODE_OPTIONS=$env:NODE_OPTIONS")
    foreach ($file in @('package.json', 'package-lock.json')) {
        $parts.Add((Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash)
    }
    # Include project/user/global npmrc contents as well as their effective values.
    foreach ($path in @((Join-Path $sourceRoot '.npmrc'), (Join-Path $sourceRoot 'web\.npmrc'), $config.userconfig, $config.globalconfig)) {
        if ($path -and (Test-Path -LiteralPath $path -PathType Leaf)) {
            $parts.Add((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash)
        } else { $parts.Add('absent') }
    }
    return Get-TextHash ($parts -join "`n")
}

function Test-WebInstall([string]$Fingerprint) {
    if (-not (Test-Path -LiteralPath $marker -PathType Leaf)) { return $false }
    try {
        $installed = Get-Content -LiteralPath $marker -Raw -Encoding UTF8 | ConvertFrom-Json
        if ($installed.version -ne 1 -or $installed.fingerprint -ne $Fingerprint -or -not $installed.hidden_lock_sha256) { return $false }
        if (-not (Test-Path -LiteralPath $hiddenLock -PathType Leaf)) { return $false }
        if ((Get-FileHash -LiteralPath $hiddenLock -Algorithm SHA256).Hash -ne $installed.hidden_lock_sha256) { return $false }
        foreach ($tool in @('tsc.cmd', 'vite.cmd')) {
            if (-not (Test-Path -LiteralPath (Join-Path $modules ('.bin\' + $tool)) -PathType Leaf)) { return $false }
        }
        $lock = Get-Content -LiteralPath $hiddenLock -Raw -Encoding UTF8 | ConvertFrom-Json
        foreach ($entry in $lock.packages.PSObject.Properties) {
            # Hidden lock paths are relative to web/, and must remain in node_modules/.
            $path = [IO.Path]::GetFullPath((Join-Path (Join-Path $sourceRoot 'web') $entry.Name))
            if (-not (Test-PathWithin $modules $path) -or -not (Test-Path -LiteralPath $path -PathType Container)) { return $false }
        }
        return $true
    } catch { return $false }
}

try {
    foreach ($relative in @('web', 'web\server', 'web\server\dist', 'web\node_modules', 'web\node_modules\.steerline-install.json', 'web\node_modules\.package-lock.json')) {
        Assert-SourceBuildPath (Join-Path $sourceRoot $relative) 'Web build path'
    }
    $modules = Join-Path $sourceRoot 'web\node_modules'
    $marker = Join-Path $modules '.steerline-install.json'
    $hiddenLock = Join-Path $modules '.package-lock.json'
    Push-Location (Join-Path $sourceRoot 'web')
    try {
        $dependencyCheck = Invoke-BuildStage 'web.dependencies.check' {
            $fingerprint = Get-WebInstallFingerprint
            return @{fingerprint=$fingerprint; reuse=(-not $ForceWebInstall -and (Test-WebInstall $fingerprint))}
        }
        $fingerprint = $dependencyCheck.fingerprint
        if ($dependencyCheck.reuse) {
            Add-SkippedBuildStage 'web.dependencies.install' 'dependency inputs, toolchain, npm configuration and installed tree match'
        } else {
            $reason = if ($ForceWebInstall) { 'forced' } else { 'missing, changed or incomplete dependency installation' }
            Write-Host "Web npm ci required: $reason"
            Invoke-BuildStage 'web.dependencies.install' {
                # Invalidate before npm starts, including when npm exits unsuccessfully.
                if (Test-Path -LiteralPath $marker) { Remove-Item -LiteralPath $marker -Force }
                & npm ci | Out-Host
                if ($LASTEXITCODE -ne 0) { throw "Web npm ci failed (exit $LASTEXITCODE)." }
                if ((Get-WebInstallFingerprint) -ne $fingerprint) { throw 'Dependency inputs changed during npm ci; rerun the build.' }
                if (-not (Test-Path -LiteralPath $hiddenLock -PathType Leaf)) { throw 'npm ci did not produce its installed dependency lock.' }
                Assert-SourceBuildPath $marker 'Web install marker'
                $value = @{version=1; fingerprint=$fingerprint; hidden_lock_sha256=(Get-FileHash -LiteralPath $hiddenLock -Algorithm SHA256).Hash}
                [IO.File]::WriteAllText($marker, ($value | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
                if (-not (Test-WebInstall $fingerprint)) {
                    Remove-Item -LiteralPath $marker -Force
                    throw 'Installed Web dependency tree is incomplete.'
                }
            }
        }
        Invoke-BuildStage 'web.assets' {
            & npm run build | Out-Host
            if ($LASTEXITCODE -ne 0) { throw "Web assets failed (exit $LASTEXITCODE)." }
        }
        $succeeded = $true
    } finally { Pop-Location }
} finally { Write-BuildTiming $TimingPath $succeeded }
