# One writer contract shared by L1-L4 entry points and explicit progress calls.
function Set-RuntimeProgress {
    param([Parameter(Mandatory)][string[]]$Name, [Parameter(Mandatory)][bool]$Completed, [switch]$IfDeclared)
    $path = [Environment]::GetEnvironmentVariable('STEERLINE_PROGRESS_FILE')
    if ([string]::IsNullOrEmpty($path)) { return }
    if (![IO.Path]::IsPathRooted($path)) { throw 'Progress file must be an absolute runtime-injected path.' }
    $lock = [IO.File]::Open($path+'.lock', [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::ReadWrite)
    $locked=$false; $temporary=$null
    try {
        $deadline=[DateTime]::UtcNow.AddSeconds(3)
        while (!$locked) {
            try { $lock.Lock(0,1); $locked=$true }
            catch [IO.IOException] {
                if ([DateTime]::UtcNow -ge $deadline) { throw }
                Start-Sleep -Milliseconds 15
            }
        }
        $bytes=[IO.File]::ReadAllBytes($path)
        if ($bytes.Length -gt 65536) { throw 'Progress exceeds 64 KiB.' }
        $utf8=[Text.UTF8Encoding]::new($false,$true)
        # The channel is a JSON object of unique, case-sensitive boolean fields.
        # Validate that small grammar before decoding string tokens; ConvertFrom-Json
        # alone would discard duplicate keys and conflate differently cased names.
        $string='"(?:[^"\\\x00-\x1f]|\\(?:["\\/bfnrt]|u[0-9a-fA-F]{4}))*"'
        $member='(?<name>'+$string+')[ \t\r\n]*:[ \t\r\n]*(?<done>true|false)'
        $pattern='\A[ \t\r\n]*\{[ \t\r\n]*(?:'+$member+'(?:[ \t\r\n]*,[ \t\r\n]*'+$member+')*[ \t\r\n]*)?\}[ \t\r\n]*\z'
        $match=[regex]::Match($utf8.GetString($bytes),$pattern,[Text.RegularExpressions.RegexOptions]::None,[TimeSpan]::FromMilliseconds(250))
        if (!$match.Success) { throw 'Progress requires a boolean object.' }
        $fields=[Collections.Generic.Dictionary[string,bool]]::new([StringComparer]::Ordinal)
        for ($i=0;$i -lt $match.Groups['name'].Captures.Count;$i++) {
            $key=ConvertFrom-Json -InputObject $match.Groups['name'].Captures[$i].Value
            $null=$utf8.GetBytes($key)
            if ($fields.ContainsKey($key)) { throw 'Progress requires unique boolean fields.' }
            $fields.Add($key,($match.Groups['done'].Captures[$i].Value -ceq 'true'))
        }
        $changed=$false
        foreach ($key in $Name) {
            if (!$fields.ContainsKey($key)) {
                if ($IfDeclared) { continue }
                throw ('Progress name was not declared for this Run: '+$key)
            }
            if ($fields[$key] -ne $Completed) { $changed=$true; $fields[$key]=$Completed }
        }
        if (!$changed) { return }
        $output=$utf8.GetBytes((ConvertTo-Json -InputObject $fields -Compress))
        if ($output.Length -gt 65536) { throw 'Progress exceeds 64 KiB.' }
        $temporary=Join-Path ([IO.Path]::GetDirectoryName($path)) ('.progress-'+[guid]::NewGuid().ToString('N')+'.tmp')
        $stream=[IO.File]::Open($temporary,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
        try { $stream.Write($output,0,$output.Length); $stream.Flush($true) } finally { $stream.Dispose() }
        $deadline=[DateTime]::UtcNow.AddSeconds(1)
        while ($true) {
            try { [IO.File]::Replace($temporary,$path,[NullString]::Value); break }
            catch [IO.IOException] {
                if ([DateTime]::UtcNow -ge $deadline) { throw }
                Start-Sleep -Milliseconds 15
            }
        }
    } finally {
        if ($temporary -and [IO.File]::Exists($temporary)) { [IO.File]::Delete($temporary) }
        if ($locked) { $lock.Unlock(0,1) }
        $lock.Dispose()
    }
}
