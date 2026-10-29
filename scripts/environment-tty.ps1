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
