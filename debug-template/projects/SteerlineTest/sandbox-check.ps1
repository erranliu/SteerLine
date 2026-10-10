$ErrorActionPreference = 'Stop'
$admin = (& git rev-parse --absolute-git-dir).Trim()
if ($LASTEXITCODE -ne 0) { throw 'Git admin resolution failed' }
$expected = [IO.Path]::GetFullPath((Join-Path $admin 'steerline-temp'))
foreach ($name in @('TEMP', 'TMP')) {
    $value = [Environment]::GetEnvironmentVariable($name)
    if ([string]::IsNullOrWhiteSpace($value) -or [IO.Path]::GetFullPath($value) -ne $expected) { throw "$name is not this task's temporary directory" }
}
$file = Join-Path $expected ('probe-' + [Guid]::NewGuid().ToString('N') + '.txt')
try {
    [IO.File]::WriteAllText($file, 'sandbox temp probe')
    if ([IO.File]::ReadAllText($file) -ne 'sandbox temp probe') { throw 'Temporary round trip failed' }
} finally { if (Test-Path -LiteralPath $file) { Remove-Item -LiteralPath $file } }
$maintenance = (& git config --get maintenance.auto).Trim()
$gc = (& git config --get gc.auto).Trim()
if ($maintenance -ne 'false' -or $gc -ne '0') { throw 'Worker Git auto-maintenance is enabled' }
if (-not (Test-Path -LiteralPath './docs/spec/task.spec.md') -or -not (Test-Path -LiteralPath './publish.ps1')) { throw 'Fixture project check failed' }
$sibling = '__ROOT__/sandbox-negative'
$sentinel = Join-Path $sibling 'sentinel.txt'
$denied = $false
if (Test-Path -LiteralPath $sentinel) {
    try { [IO.File]::WriteAllText($sentinel, 'unexpected sibling write') }
    catch [UnauthorizedAccessException] { $denied = $true }
    if (-not $denied) { throw 'Sibling write was allowed' }
}
# Only metadata and booleans are emitted; credentials and inherited env stay private.
[ordered]@{ passed = $true; powershell = $PSVersionTable.PSVersion.ToString(); temp_in_admin = $true; maintenance = $maintenance; gc = $gc; project_check = $true; sibling_write_denied = $denied } | ConvertTo-Json -Compress
