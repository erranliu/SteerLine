function Start-DeploymentApplication([string]$Exe, [string]$InstallDirectory, [string]$PackageDirectory) {
    foreach ($path in @($Exe, $InstallDirectory, $PackageDirectory)) {
        if (-not [IO.Path]::IsPathRooted($path)) { throw 'Deployment startup requires absolute paths.' }
        Assert-Plain $path
    }
    $invocation = [Guid]::NewGuid().ToString('N')
    $resultPath = Join-Path $PackageDirectory ('startup-' + $invocation + '.json')
    $logPath = Join-Path $PackageDirectory ('startup-' + $invocation + '.log')
    $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    # Encode code with quoted path literals, never concatenate paths into shell
    # syntax. EnvironmentVariables keeps caller credentials off disk/argv and
    # preserves PATH, user profile, proxy and adapter settings across the broker.
    $worker = @'
$ErrorActionPreference = 'Stop'
$code = 1
try {
    if ([Security.Principal.WindowsIdentity]::GetCurrent().User.Value -ne '__SID__') { throw 'Startup broker changed the Windows user.' }
    $env:STEERLINE_HOME = '__HOME__'
    & '__EXE__' start *> '__LOG__'
    $code = $LASTEXITCODE
    if ($null -eq $code) { throw 'Startup command returned no exit status.' }
} catch {
    $_ | Out-String | Add-Content -LiteralPath '__LOG__' -Encoding UTF8
    $code = 1
}
$temporary = '__RESULT__.tmp'
@{invocation_id='__INVOCATION__';exit_code=$code} | ConvertTo-Json -Compress | Set-Content -LiteralPath $temporary -Encoding UTF8
Move-Item -LiteralPath $temporary -Destination '__RESULT__'
exit $code
'@
    # Replace in a single pass so a path containing another placeholder stays data.
    $literals = @{SID=$sid;HOME=$InstallDirectory;EXE=$Exe;LOG=$logPath;RESULT=$resultPath;INVOCATION=$invocation}
    $worker = [regex]::Replace($worker, '__(SID|HOME|EXE|LOG|RESULT|INVOCATION)__', {
        param($match)
        $literals[$match.Groups[1].Value].Replace("'", "''")
    })
    $encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($worker))
    $powershell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    # A brokered launcher survives the deployment host's kill-on-close Job.
    # The installed product still performs all of its own lifetime checks.
    $startup = New-CimInstance -ClassName Win32_ProcessStartup -ClientOnly -Property @{
        ShowWindow = [uint16]0
        EnvironmentVariables = [string[]]@(Get-ChildItem Env: | ForEach-Object { $_.Name + '=' + $_.Value })
    }
    $created = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{
        CommandLine = '"' + $powershell + '" -NoProfile -WindowStyle Hidden -EncodedCommand ' + $encoded
        CurrentDirectory = $InstallDirectory
        ProcessStartupInformation = $startup
    }
    if ($created.ReturnValue -ne 0 -or -not $created.ProcessId) {
        throw "Windows could not launch the installed application (Win32_Process.Create=$($created.ReturnValue))."
    }
    Write-Output "Startup launcher pid=$($created.ProcessId); log: $logPath"
    $deadline = [DateTime]::UtcNow.AddSeconds(180)
    while (-not (Test-Path -LiteralPath $resultPath -PathType Leaf)) {
        if ([DateTime]::UtcNow -ge $deadline) { throw "Startup outcome is unknown; do not replay it automatically. Inspect $logPath and installed status." }
        Start-Sleep -Milliseconds 200
    }
    Assert-Plain $resultPath
    $result = Get-Content -LiteralPath $resultPath -Raw -Encoding UTF8 | ConvertFrom-Json -ErrorAction Stop
    if ($result.invocation_id -ne $invocation -or $null -eq $result.exit_code -or $result.exit_code -ne 0) {
        throw "Installed application startup failed or returned an unrelated result; inspect $logPath."
    }
}
