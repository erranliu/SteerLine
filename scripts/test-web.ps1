param([string]$DebugConfig = '', [string]$EvidenceDir = '')
$ErrorActionPreference = 'Stop'
$webTestRepo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if (-not $EvidenceDir) { $EvidenceDir = Join-Path ([IO.Path]::GetTempPath()) ('steerline-web-check-' + (Get-Date -Format 'yyyyMMdd-HHmmss-fff')) }
if (Test-Path -LiteralPath $EvidenceDir) { throw 'EvidenceDir must be a new directory' }
New-Item -ItemType Directory -Path $EvidenceDir | Out-Null
Push-Location $webTestRepo
try {
    & npm --prefix web run build *> (Join-Path $EvidenceDir 'build.log')
    if ($LASTEXITCODE -ne 0) { throw 'Web build failed' }
    & npm --prefix web test *> (Join-Path $EvidenceDir 'client-tests.log')
    if ($LASTEXITCODE -ne 0) { throw 'Web client tests failed' }
    & go run ./cmd/test-feedback -- ./web/server ./internal/product/httpapi -count=1 *> (Join-Path $EvidenceDir 'server-tests.log')
    if ($LASTEXITCODE -ne 0) { throw 'Web server tests failed' }
    if ($DebugConfig) {
        $savedWebConfig = $env:STEERLINE_WEB_CONFIG
        $savedWebEvidence = $env:STEERLINE_WEB_EVIDENCE
        try {
            $env:STEERLINE_WEB_CONFIG = $DebugConfig
            $env:STEERLINE_WEB_EVIDENCE = Join-Path $EvidenceDir 'browser'
            & npm --prefix web run test:e2e *> (Join-Path $EvidenceDir 'browser.log')
            if ($LASTEXITCODE -ne 0) { throw 'Real browser smoke failed' }
        } finally { $env:STEERLINE_WEB_CONFIG = $savedWebConfig; $env:STEERLINE_WEB_EVIDENCE = $savedWebEvidence }
    }
    Write-Output "Web checks passed; evidence: $EvidenceDir"
} finally { Pop-Location }
