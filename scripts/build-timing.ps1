# Dot-source in each process. Child frontend timings are merged by the build owner.
function Initialize-BuildTiming([string]$Operation) {
    $script:buildOperation = $Operation
    $script:buildStarted = [DateTime]::UtcNow
    $script:buildClock = [Diagnostics.Stopwatch]::StartNew()
    $script:buildStages = [Collections.Generic.List[object]]::new()
}

function Start-BuildStage([string]$Name) {
    Write-Host "[stage] $Name started"
    return @{ name=$Name; started=[DateTime]::UtcNow; timer=[Diagnostics.Stopwatch]::StartNew() }
}

function Complete-BuildStage($Stage, [string]$Outcome) {
    $Stage.timer.Stop()
    $script:buildStages.Add([pscustomobject]@{
        name = $Stage.name; outcome = $Outcome; started_at = $Stage.started.ToString('o')
        completed_at = [DateTime]::UtcNow.ToString('o'); elapsed_seconds = [Math]::Round($Stage.timer.Elapsed.TotalSeconds, 3)
    })
    Write-Host ("[stage] {0} {1} ({2:N2}s)" -f $Stage.name, $Outcome, $Stage.timer.Elapsed.TotalSeconds)
}

function Invoke-BuildStage([string]$Name, [scriptblock]$Action) {
    $stage = Start-BuildStage $Name
    $outcome = 'failed'
    try {
        & $Action
        $outcome = 'succeeded'
    } finally {
        Complete-BuildStage $stage $outcome
    }
}

function Add-SkippedBuildStage([string]$Name, [string]$Reason) {
    $now = [DateTime]::UtcNow.ToString('o')
    $script:buildStages.Add([pscustomobject]@{
        name = $Name; outcome = 'skipped'; reason = $Reason
        started_at = $now; completed_at = $now; elapsed_seconds = 0
    })
    Write-Host "[stage] $Name skipped: $Reason"
}

function Import-BuildTiming([string]$Path) {
    if (Test-Path -LiteralPath $Path) {
        $report = Get-Content -LiteralPath $Path -Raw -Encoding UTF8 | ConvertFrom-Json
        foreach ($stage in $report.stages) { $script:buildStages.Add($stage) }
    }
}

function Write-BuildTiming([string]$Path, [bool]$Succeeded) {
    # Reports may be explicitly outside the checkout, but must never follow a
    # directory alias or overwrite a hard link to somebody else's file.
    $cursor = [IO.Path]::GetFullPath($Path)
    while ($cursor) {
        $item = Get-Item -LiteralPath $cursor -Force -ErrorAction SilentlyContinue
        if ($item -and (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $item.LinkType -eq 'HardLink')) {
            throw "Linked timing report path: $cursor"
        }
        $parent = [IO.Path]::GetDirectoryName($cursor)
        if ($parent -eq $cursor) { break }
        $cursor = $parent
    }
    $report = [ordered]@{
        version = 1; operation = $script:buildOperation; succeeded = $Succeeded
        started_at = $script:buildStarted.ToString('o'); completed_at = [DateTime]::UtcNow.ToString('o')
        elapsed_seconds = [Math]::Round($script:buildClock.Elapsed.TotalSeconds, 3)
        stages = @($script:buildStages | Sort-Object started_at)
    }
    $directory = Split-Path -Parent $Path
    New-Item -ItemType Directory -Path $directory -Force | Out-Null
    [IO.File]::WriteAllText($Path, ($report | ConvertTo-Json -Depth 10), [Text.UTF8Encoding]::new($false))
    Write-Host ("[total] {0}: {1:N2}s; succeeded={2}; timings: {3}" -f $script:buildOperation, $script:buildClock.Elapsed.TotalSeconds, $Succeeded, $Path)
}
