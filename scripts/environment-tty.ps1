# Called in the Agent's actual exec_command(tty:true) process, not by a host-side shell probe.
function Get-EnvironmentTTYCheck {
    try {
        $inputRedirected = [Console]::IsInputRedirected
        $outputRedirected = [Console]::IsOutputRedirected
        if ($inputRedirected -or $outputRedirected) {
            return [pscustomobject]@{ id = 'terminal.tty'; status = 'failed'; message = "TTY required: stdin_redirected=$inputRedirected stdout_redirected=$outputRedirected; pid=$PID. Run this entry through the Agent exec_command with tty:true; do not retry with tty:false." }
        }
        return [pscustomobject]@{ id = 'terminal.tty'; status = 'passed'; message = "STEERLINE_TTY_OK stdin_redirected=False stdout_redirected=False pid=$PID" }
    } catch {
        return [pscustomobject]@{ id = 'terminal.tty'; status = 'incomplete'; message = 'Unable to inspect the actual process terminal; TTY has not been verified.' }
    }
}

function Assert-EnvironmentTTY {
    $check = Get-EnvironmentTTYCheck
    if ($check.status -cne 'passed') { throw $check.message }
    return $check.message
}

# Records a known TTY prerequisite failure; never probes or launches anything.
function Set-EnvironmentTTYFailureReport {
    param($Report, [System.Collections.IDictionary]$Progress, [string[]]$CheckIds, [string[]]$L2Ids, [string]$FailureReason)
    function Require-Keys($Value, [string[]]$Required, [string[]]$Optional = @()) {
        $actual = @($Value.PSObject.Properties.Name)
        if (@($Required | Where-Object { $_ -cnotin $actual }).Count -or @($actual | Where-Object { $_ -cnotin ($Required + $Optional) }).Count) { throw 'Invalid original report fields' }
    }
    if ([string]::IsNullOrWhiteSpace($FailureReason) -or [Text.Encoding]::UTF8.GetByteCount($FailureReason) -gt 8192) { throw 'Original TTY failure reason must be nonempty and at most 8192 UTF-8 bytes' }
    Require-Keys $Report @('version','activity_id','round','source_commit','script_digest','completed','checks','l2','pr','message') @('previous')
    foreach ($name in @('activity_id','source_commit','script_digest','message')) { if ($Report.$name -isnot [string]) { throw 'Invalid original report value' } }
    if (($Report.version -isnot [int] -and $Report.version -isnot [long]) -or $Report.version -ne 2 -or $Report.activity_id -cnotmatch '^[a-f0-9]{32}$' -or $Report.source_commit -cnotmatch '^(?:[a-f0-9]{40}|[a-f0-9]{64})$' -or $Report.script_digest -cnotmatch '^[a-f0-9]{64}$' -or ($Report.round -isnot [int] -and $Report.round -isnot [long]) -or $Report.round -notin 1,2,3 -or $Report.completed -isnot [bool] -or $Report.checks -isnot [array] -or $Report.l2 -isnot [array]) { throw 'Invalid original environment input' }
    Require-Keys $Report.pr @('url','base','head','head_revision','cleanup_completed','outcome_unknown')
    foreach ($name in @('url','base','head','head_revision')) { if ($Report.pr.$name -isnot [string]) { throw 'Invalid original PR field' } }
    if ($Report.pr.cleanup_completed -isnot [bool] -or $Report.pr.outcome_unknown -isnot [bool] -or $Report.pr.url -or $Report.pr.head_revision -or $Report.pr.outcome_unknown) { throw 'Publication facts or unknown outcome must not be overwritten' }
    if (!$CheckIds.Count -or 'terminal.tty' -cnotin $CheckIds -or !$L2Ids.Count -or @($CheckIds | Select-Object -Unique).Count -ne $CheckIds.Count -or @($L2Ids | Select-Object -Unique).Count -ne $L2Ids.Count) { throw 'Invalid declared check or L2 IDs' }
    $checks = @{}; $l2 = @{}
    foreach ($item in $Report.checks) {
        Require-Keys $item @('id','status','message')
        if ($item.id -cnotin $CheckIds -or $checks.ContainsKey($item.id) -or $item.status -cnotin @('passed','failed','incomplete') -or $item.message -isnot [string]) { throw 'Invalid or duplicate original check' }
        $checks[$item.id] = $item
    }
    foreach ($item in $Report.l2) {
        Require-Keys $item @('test_id','status','message')
        if ($item.test_id -cnotin $L2Ids -or $l2.ContainsKey($item.test_id) -or $item.status -cnotin @('passed','failed','incomplete') -or $item.message -isnot [string]) { throw 'Invalid or duplicate original L2 item' }
        $l2[$item.test_id] = $item
    }
    $progressIds = @('pr') + @($L2Ids | ForEach-Object { 'test.' + $_ })
    if ($null -eq $Progress -or $Progress.Count -ne $progressIds.Count) { throw 'Invalid declared progress' }
    foreach ($key in $progressIds) {
        if (!$Progress.Contains($key) -or $Progress[$key] -isnot [bool]) { throw 'Missing or invalid declared progress' }
    }
    if ($Progress['pr']) { throw 'Publication progress must not be overwritten' }
    foreach ($id in $L2Ids) {
        if ($Progress['test.' + $id] -ne [bool]($l2.ContainsKey($id) -and $l2[$id].status -ceq 'passed')) { throw 'Original L2 progress differs from the report' }
    }
    if ((!$Report.completed -and ($checks.Count -or $l2.Count)) -or ($Report.completed -and !$Report.pr.cleanup_completed)) { throw 'Partial report or unsettled producer must use original stop and reconciliation' }

    $message = 'TTY prerequisite failed; no replacement detection was run: ' + $FailureReason
    $Report.checks = @($CheckIds | ForEach-Object {
        if ($_ -ceq 'terminal.tty') { [pscustomobject]@{ id = $_; status = 'failed'; message = $FailureReason } }
        elseif ($checks.ContainsKey($_)) { $checks[$_] }
        else { [pscustomobject]@{ id = $_; status = 'incomplete'; message = 'Not verified: TTY prerequisite failed before this phase' } }
    })
    $Report.l2 = @($L2Ids | ForEach-Object {
        if ($l2.ContainsKey($_)) { $l2[$_] }
        else { [pscustomobject]@{ test_id = $_; status = 'incomplete'; message = 'Not executed: TTY prerequisite failed' } }
    })
    $Report.completed = $true
    $Report.pr.cleanup_completed = $true
    $Report.message = $message
    if ([Text.Encoding]::UTF8.GetByteCount(($Report | ConvertTo-Json -Depth 20 -Compress)) -gt 65536) { throw 'Channel report exceeds 64 KiB' }
    return $Report
}
