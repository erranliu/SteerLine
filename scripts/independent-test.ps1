# Native lifecycle tests must start with one consistent user token and outside
# a caller's kill-on-close Job. This is the same WMI context as the browser driver.
function Invoke-IndependentTest([string]$ScriptPath, [string[]]$TestArguments) {
    $brokerRoot = Join-Path $env:TEMP ('steerline-native-test-' + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $brokerRoot | Out-Null
    $brokerLog = Join-Path $brokerRoot 'test.log'
    $brokerResult = Join-Path $brokerRoot 'result.json'
    function Quote-IndependentValue([string]$Value) { return "'" + $Value.Replace("'", "''") + "'" }
    $invoke = '& ' + (Quote-IndependentValue $ScriptPath) + ' -IndependentProcess'
    foreach ($arg in $TestArguments) {
        if ($arg -match '^-[A-Za-z][A-Za-z0-9-]*$') { $invoke += ' ' + $arg }
        else { $invoke += ' ' + (Quote-IndependentValue $arg) }
    }
    $body = '$ErrorActionPreference=''Stop''; [Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); $OutputEncoding=[Console]::OutputEncoding; '
    $body += '$env:PATH=' + (Quote-IndependentValue $env:PATH) + '; '
    if ($env:CODEX_HOME) { $body += '$env:CODEX_HOME=' + (Quote-IndependentValue $env:CODEX_HOME) + '; ' }
    $body += '$taskExit=0; try { ' + $invoke + ' *> ' + (Quote-IndependentValue $brokerLog) + '; if (-not $?) { $taskExit=1 } } catch { $_ | Out-File -Append -LiteralPath ' + (Quote-IndependentValue $brokerLog) + '; $taskExit=1 } finally { @{exit_code=$taskExit} | ConvertTo-Json | Set-Content -Encoding utf8 -LiteralPath ' + (Quote-IndependentValue $brokerResult) + ' }; exit $taskExit'
    $encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($body))
    $process = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{CommandLine=('powershell.exe -NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -EncodedCommand ' + $encoded)}
    if ($process.ReturnValue -ne 0) { throw "Independent test process creation failed: $($process.ReturnValue)" }
    $deadline = [DateTime]::UtcNow.AddMinutes(20)
    while (-not (Test-Path -LiteralPath $brokerResult)) {
        if ([DateTime]::UtcNow -ge $deadline) { throw "Independent test result unverified; inspect $brokerRoot (PID $($process.ProcessId))" }
        Start-Sleep -Milliseconds 250
    }
    Get-Content -LiteralPath $brokerLog
    $result = Get-Content -LiteralPath $brokerResult -Raw | ConvertFrom-Json
    if ($result.exit_code -ne 0) { throw "Independent test failed; inspect $brokerRoot" }
}
