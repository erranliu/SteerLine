param(
    [Parameter(Mandatory)][ValidateSet('Controlled','Live')][string]$Mode,
    [Parameter(Mandatory)][string]$OutRoot,
    [string]$RunnerExe,
    [string]$CodexExe
)
$ErrorActionPreference='Stop'
$slRoot=[IO.Path]::GetFullPath($OutRoot)
if (!$slRoot.StartsWith('C:\SteerlineDebug\',[StringComparison]::OrdinalIgnoreCase) -or (Test-Path -LiteralPath $slRoot)) {
    throw 'OutRoot must be a fresh directory under C:\SteerlineDebug.'
}
if ($Mode -eq 'Live') {
    if (!$RunnerExe -or !(Test-Path -LiteralPath $RunnerExe -PathType Leaf)) { throw 'Live mode requires the pinned Runner EXE.' }
    $slOptions=@{Planning=$true;SteerRunnerExe=$RunnerExe;OutRoot=$slRoot}
    if ($CodexExe) { $slOptions.CodexExe=$CodexExe }
    & (Join-Path $PSScriptRoot 'test-roadmap-browser-live.ps1') @slOptions
    return
}
[IO.Directory]::CreateDirectory($slRoot) | Out-Null
Push-Location (Join-Path $PSScriptRoot '..')
try {
    & go test ./internal/app ./web/server -run '^TestRoadmapPlanning|^TestRoadmapBrowserRoutes$' -count=1 -v *> (Join-Path $slRoot 'app.log')
    if ($LASTEXITCODE) { throw 'Controlled Roadmap planning tests failed.' }
    & npm.cmd --prefix web run build *> (Join-Path $slRoot 'web-build.log')
    if ($LASTEXITCODE) { throw 'Web build failed.' }
    & npm.cmd --prefix web test -- src/features/roadmaps/planning.test.tsx *> (Join-Path $slRoot 'web-test.log')
    if ($LASTEXITCODE) { throw 'Roadmap planning UI tests failed.' }
    @{passed=$true;mode='controlled'} | ConvertTo-Json | Set-Content -Encoding utf8 (Join-Path $slRoot 'result.json')
} finally { Pop-Location }
