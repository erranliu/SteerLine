$ErrorActionPreference = 'Stop'
$proof = & '__ROOT__/bin/debug-helper.exe' sandbox-probe '__ROOT__/sandbox-negative/sentinel.txt'
if ($LASTEXITCODE -ne 0) { throw 'Native sandbox token/temp/sibling probe failed' }
$native = $proof | ConvertFrom-Json
if (-not $native.passed -or -not $native.app_container -or -not $native.temp_in_kernel -or -not $native.sibling_write_denied) { throw 'Native sandbox proof is incomplete' }
$maintenance = (& git config --get maintenance.auto).Trim()
$gc = (& git config --get gc.auto).Trim()
if ($maintenance -ne 'false' -or $gc -ne '0') { throw 'Worker Git auto-maintenance is enabled' }
if (-not (Test-Path -LiteralPath './docs/spec/task.spec.md') -or -not (Test-Path -LiteralPath './publish.ps1')) { throw 'Fixture project check failed' }
[ordered]@{ passed = $true; powershell = $PSVersionTable.PSVersion.ToString(); app_container = $true; temp_in_kernel = $true; maintenance = $maintenance; gc = $gc; project_check = $true; sibling_write_denied = $true } | ConvertTo-Json -Compress
