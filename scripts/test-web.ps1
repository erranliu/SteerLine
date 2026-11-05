param([string]$DebugConfig = '', [string]$EvidenceDir = '')
# Build once, then delegate Web and all seven HTTP/metadata owner packages to
# their registered definitions. Every HTTP package must execute real tests.
$ErrorActionPreference = 'Stop'
function Invoke-WebTestFeedback([string[]]$Arguments, [string]$LogPath) {
    # Windows PowerShell treats native stderr as ErrorRecord even on exit 0.
    # Preserve it in the log and use the native exit code as the result.
    $feedbackGo = (Get-Command go -CommandType Application -ErrorAction Stop | Select-Object -First 1).Source
    $savedNativeAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $global:LASTEXITCODE = 127
        & $feedbackGo @Arguments *> $LogPath
        $feedbackExit = $LASTEXITCODE
    } finally { $ErrorActionPreference = $savedNativeAction }
    if ($feedbackExit -ne 0) { throw "Registered test failed; preserve $LogPath" }
}
$webTestRepo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if (-not $EvidenceDir) { $EvidenceDir = Join-Path ([IO.Path]::GetTempPath()) ('steerline-web-check-' + (Get-Date -Format 'yyyyMMdd-HHmmss-fff')) }
if (Test-Path -LiteralPath $EvidenceDir) { throw 'EvidenceDir must be a new directory' }
New-Item -ItemType Directory -Path $EvidenceDir | Out-Null
Push-Location $webTestRepo
try {
    & npm --prefix web run build *> (Join-Path $EvidenceDir 'build.log')
    if ($LASTEXITCODE -ne 0) { throw 'Web build failed' }
    Invoke-WebTestFeedback -Arguments @('run', './cmd/test-feedback', '-level=L4', '-suite=web', '-scenario', 'web.unit', '-out', (Join-Path $EvidenceDir 'web-unit')) -LogPath (Join-Path $EvidenceDir 'client-tests.log')
    Invoke-WebTestFeedback -Arguments @('run', './cmd/test-feedback', '-level=L4', '-suite=go', '-scenario', 'go.http', '-out', (Join-Path $EvidenceDir 'go-http')) -LogPath (Join-Path $EvidenceDir 'server-tests.log')
    if ($DebugConfig) {
        $savedWebConfig = $env:STEERLINE_WEB_CONFIG
        $savedWebEvidence = $env:STEERLINE_WEB_EVIDENCE
        try {
            $env:STEERLINE_WEB_CONFIG = $DebugConfig
            $env:STEERLINE_WEB_EVIDENCE = Join-Path $EvidenceDir 'browser'
            & npm --prefix web run test:e2e -- smoke.spec.ts *> (Join-Path $EvidenceDir 'browser.log')
            if ($LASTEXITCODE -ne 0) { throw 'Real browser smoke failed' }
        } finally { $env:STEERLINE_WEB_CONFIG = $savedWebConfig; $env:STEERLINE_WEB_EVIDENCE = $savedWebEvidence }
    }
    Write-Output "Web checks passed; evidence: $EvidenceDir"
} finally { Pop-Location }
