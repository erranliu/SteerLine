param([Parameter(Mandatory=$true)][string]$Package)
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath($Package).TrimEnd('\')
if (-not $taskRoot.StartsWith('C:\SteerlineDebug\', [StringComparison]::OrdinalIgnoreCase)) { throw 'Use a generated debug package' }
$manifest = Get-Content -LiteralPath (Join-Path $taskRoot 'manifest.json') -Raw | ConvertFrom-Json
if ($manifest.status -ne 'ready' -or -not $manifest.web_binary_sha256) { throw 'Ready dual executable package required' }
$taskPreviousHome = $env:STEERLINE_HOME
$env:STEERLINE_HOME = Join-Path $taskRoot 'home'
$business = Join-Path $taskRoot 'bin\steerline.exe'
$web = Join-Path $taskRoot 'bin\steerline-web.exe'
$taskAppProcess = $null
$taskWebProcess = $null
try {
    foreach ($exe in @($business,$web)) { & $exe install; if ($LASTEXITCODE -ne 0) { throw "Install failed: $exe" } }
    & $business launchers
    if ($LASTEXITCODE -ne 0) { throw 'Launcher installation failed' }
    $taskAppProcess = Start-Process -FilePath (Join-Path $env:STEERLINE_HOME 'bin\steerline.exe') -ArgumentList 'serve' -WindowStyle Hidden -PassThru
    $taskDeadline = [DateTime]::UtcNow.AddSeconds(15)
    while ($true) {
        $taskStatus=& $business status --json | ConvertFrom-Json
        if ($LASTEXITCODE -eq 0 -and $taskStatus.backend.state -eq 'ready') { break }
        if ($taskAppProcess.HasExited -or [DateTime]::UtcNow -gt $taskDeadline) { throw 'Application startup failed' }
        Start-Sleep -Milliseconds 50
    }
    & $business install 2>&1 | Out-File -LiteralPath (Join-Path $taskRoot 'evidence\install-running-app.log')
    if ($LASTEXITCODE -eq 0) { throw 'Running application target was replaced' }
    $taskWebProcess = Start-Process -FilePath (Join-Path $env:STEERLINE_HOME 'bin\steerline-web.exe') -ArgumentList 'serve' -WindowStyle Hidden -PassThru
        $taskDeadline = [DateTime]::UtcNow.AddSeconds(15)
    while ($true) {
        $taskStatus=& $business status --json | ConvertFrom-Json
        if ($LASTEXITCODE -eq 0 -and $taskStatus.web.state -eq 'ready') { break }
        if ($taskWebProcess.HasExited -or [DateTime]::UtcNow -gt $taskDeadline) { throw 'Web startup failed' }
        Start-Sleep -Milliseconds 50
    }
    & $web install 2>&1 | Out-File -LiteralPath (Join-Path $taskRoot 'evidence\install-running-web.log')
    if ($LASTEXITCODE -eq 0) { throw 'Running Web target was replaced' }
    & $web stop
    if ($LASTEXITCODE -ne 0) { throw 'Web normal stop failed' }
    if ($taskAppProcess.HasExited) { throw 'Stopping Web stopped application' }
    & $business stop
    if ($LASTEXITCODE -ne 0) { throw 'Application normal stop failed' }
    $taskAppProcess.WaitForExit(10000) | Out-Null
    $taskWebProcess.WaitForExit(10000) | Out-Null
    if (-not $taskAppProcess.HasExited -or -not $taskWebProcess.HasExited) { throw 'Exit was not observed' }
    @{passed=$true;home=$env:STEERLINE_HOME;source_commit=$manifest.source_commit;source_dirty=$manifest.source_dirty;business_sha256=(Get-FileHash -LiteralPath $business).Hash;web_sha256=(Get-FileHash -LiteralPath $web).Hash;running_replacement_rejected=$true;independent_stop=$true} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $taskRoot 'evidence\web-switch-build.json')
} finally {
    if ($taskWebProcess -and -not $taskWebProcess.HasExited) { & $web stop }
    if ($taskAppProcess -and -not $taskAppProcess.HasExited) { & $business stop }
    $env:STEERLINE_HOME = $taskPreviousHome
}
