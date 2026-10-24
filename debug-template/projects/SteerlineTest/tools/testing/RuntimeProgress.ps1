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
        $document=[Text.Json.JsonDocument]::Parse($utf8.GetString($bytes))
        try {
            if ($document.RootElement.ValueKind -ne [Text.Json.JsonValueKind]::Object) { throw 'Progress requires a boolean object.' }
            $fields=[Collections.Generic.Dictionary[string,bool]]::new([StringComparer]::Ordinal)
            foreach ($item in $document.RootElement.EnumerateObject()) {
                if ($fields.ContainsKey($item.Name) -or $item.Value.ValueKind -notin @([Text.Json.JsonValueKind]::True,[Text.Json.JsonValueKind]::False)) { throw 'Progress requires unique boolean fields.' }
                $fields.Add($item.Name,$item.Value.GetBoolean())
            }
        } finally { $document.Dispose() }
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
            try { [IO.File]::Move($temporary,$path,$true); break }
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
