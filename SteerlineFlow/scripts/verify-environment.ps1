# Project tool detection only. Compatible with Windows PowerShell 5.1.
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$utf8 = New-Object System.Text.UTF8Encoding($false)
$resultPath = $env:STEERLINE_RESULT_FILE
if ([string]::IsNullOrWhiteSpace($resultPath) -or -not [IO.Path]::IsPathRooted($resultPath)) {
    throw 'STEERLINE_RESULT_FILE must be an absolute pre-existing result file.'
}
$report = [IO.File]::ReadAllText($resultPath, $utf8) | ConvertFrom-Json
if ($report.version -ne 3) { throw 'Unsupported environment result contract.' }
$checks = New-Object 'System.Collections.Generic.List[object]'

function Test-ToolVersion([string]$Name, [string]$Argument, [string]$Pattern, [version]$Minimum) {
    $process = $null
    $reason = 'Tool is missing from PATH.'
    try {
        $command = Get-Command $Name -CommandType Application -ErrorAction Stop | Select-Object -First 1
        $reason = 'Version query could not start.'
        $start = New-Object System.Diagnostics.ProcessStartInfo
        $start.FileName = $command.Source
        $start.Arguments = $Argument
        if ([IO.Path]::GetExtension($command.Source) -eq '.cmd') {
            $start.FileName = Join-Path $env:SystemRoot 'System32/cmd.exe'
            $start.Arguments = '/d /s /c ""' + $command.Source + '" ' + $Argument + '"'
        }
        $start.UseShellExecute = $false
        $start.CreateNoWindow = $true
        $start.RedirectStandardOutput = $true
        $start.RedirectStandardError = $true
        $process = New-Object System.Diagnostics.Process
        $process.StartInfo = $start
        if (-not $process.Start()) { throw 'Version query could not start.' }
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        if (-not $process.WaitForExit(15000)) {
            $reason = 'Version query timed out; cleanup unverified.'
            # Stop this query's own process tree, including a .cmd launcher child.
            $cleanupStart = New-Object System.Diagnostics.ProcessStartInfo
            $cleanupStart.FileName = Join-Path $env:SystemRoot 'System32/taskkill.exe'
            $cleanupStart.Arguments = '/PID ' + $process.Id + ' /T /F'
            $cleanupStart.UseShellExecute = $false
            $cleanupStart.CreateNoWindow = $true
            $cleanup = [System.Diagnostics.Process]::Start($cleanupStart)
            try {
                if (-not $cleanup.WaitForExit(5000)) {
                    $cleanup.Kill()
                    throw 'Version query timed out; cleanup unverified.'
                }
            } finally { $cleanup.Dispose() }
            if (-not $process.WaitForExit(5000)) { throw 'Version query timed out; cleanup unverified.' }
            $reason = 'Version query timed out.'
            throw 'Version query timed out.'
        }
        # Bound pipe collection too: inherited handles must not block completion.
        $reason = 'Version query output did not close.'
        if (-not $stdout.Wait(1000) -or -not $stderr.Wait(1000)) { throw $reason }
        $output = $stdout.Result + "`n" + $stderr.Result
        $match = [regex]::Match($output, $Pattern)
        $reason = 'Version query failed or returned an unrecognized version.'
        if ($process.ExitCode -ne 0 -or -not $match.Success) { throw $reason }
        $actual = [version]$match.Groups[1].Value
        $reason = 'Version ' + $actual + ' is below required ' + $Minimum + '.'
        if ($actual -lt $Minimum) { throw $reason }
        $checks.Add([ordered]@{ tool = $Name; ready = $true; message = ('Version ' + $actual) })
    } catch {
        # Do not include arbitrary tool output, paths or credentials in the report.
        $checks.Add([ordered]@{ tool = $Name; ready = $false; message = $reason })
    } finally {
        if ($null -ne $process) { $process.Dispose() }
    }
}

try {
    $document = [IO.File]::ReadAllText((Join-Path (Get-Location).Path 'SteerlineFlow/environment.md'), $utf8)
    $blocks = [regex]::Matches($document, '(?ms)^```steerline-environment\r?\n(.*?)\r?\n```(?=\r?\n|$)')
    if ($blocks.Count -ne 1) { throw 'Expected one environment requirements block.' }
    $manifest = $blocks[0].Groups[1].Value | ConvertFrom-Json
    if ($manifest.version -ne 6) { throw 'Unsupported environment manifest.' }
    $requirements = $manifest.requirements
    # Validate all minima before querying tools. Empty values must not become 0.0.
    foreach ($name in @('go', 'node', 'python')) {
        if ($requirements.$name -notmatch '^\d+\.\d+(?:\.\d+)?$') { throw 'Invalid tool minimum.' }
        $null = [version]$requirements.$name
    }
    Test-ToolVersion 'git.exe' '--version' 'git version (\d+\.\d+(?:\.\d+)?)' ([version]'0.0')
    Test-ToolVersion 'go.exe' 'version' 'go version go(\d+\.\d+(?:\.\d+)?)' ([version]$requirements.go)
    Test-ToolVersion 'node.exe' '--version' 'v(\d+\.\d+\.\d+)' ([version]$requirements.node)
    Test-ToolVersion 'npm.cmd' '--version' '(?m)^(\d+\.\d+\.\d+)\s*$' ([version]'0.0')
    Test-ToolVersion 'python.exe' '--version' 'Python (\d+\.\d+\.\d+)' ([version]$requirements.python)
} catch {
    $checks.Add([ordered]@{ tool = 'requirements'; ready = $false; message = 'Cannot read valid tool requirements from SteerlineFlow/environment.md.' })
}
$failed = @($checks | Where-Object { -not $_.ready })
$report.completed = $true
$report.ready = ($failed.Count -eq 0)
$report.message = if ($report.ready) { 'Project base tools are available.' } else {
    'Project tools are not ready. Prepare or repair: ' + (($failed | ForEach-Object { $_.tool }) -join ', ') + '; then run detection again.'
}
$report | Add-Member -MemberType NoteProperty -Name details -Value ([ordered]@{ checks = @($checks.ToArray()) }) -Force
$temporary = $resultPath + '.' + [guid]::NewGuid().ToString('N') + '.tmp'
try {
    [IO.File]::WriteAllText($temporary, ($report | ConvertTo-Json -Depth 32), $utf8)
    [IO.File]::Replace($temporary, $resultPath, [System.Management.Automation.Language.NullString]::Value)
} finally {
    if ([IO.File]::Exists($temporary)) { [IO.File]::Delete($temporary) }
}
