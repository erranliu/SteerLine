$ErrorActionPreference = 'Stop'
$settings = Get-Content -LiteralPath '__ROOT__/manifest.json' -Raw | ConvertFrom-Json
$branch = (& git symbolic-ref --short HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $branch -eq $settings.base_branch -or $branch -eq 'main') { throw 'Publish only the assigned candidate head' }
$existing = & gh pr list --repo erranliu/SteerlineTest --base $settings.base_branch --head $branch --state all --json url
if ($LASTEXITCODE -ne 0) { throw 'Existing PR query failed' }
[array]$matches = ConvertFrom-Json -InputObject ($existing -join "`n")
if ($matches.Count -gt 1) { throw 'Multiple matching PRs' }
# Runner supplies the native tool's scoped authentication configuration.
# Preserve it; generated packages never receive copied host credentials.
& git push origin "HEAD:refs/heads/$branch"
if ($LASTEXITCODE -ne 0) { throw 'Push failed' }
if ($matches.Count -eq 1) { $url = $matches[0].url } else {
    $url = & gh pr create --repo erranliu/SteerlineTest --base $settings.base_branch --head $branch --draft --title "SteerLine debug $($settings.release)" --body 'Independent SteerlineTest debug environment.'
    if ($LASTEXITCODE -ne 0) { throw 'PR creation failed' }
}
$proof = & gh pr view $url --repo erranliu/SteerlineTest --json url,state,headRefName,baseRefName
if ($LASTEXITCODE -ne 0) { throw 'PR verification failed' }
$pr = $proof | ConvertFrom-Json
if ($pr.state -ne 'OPEN' -or $pr.headRefName -ne $branch -or $pr.baseRefName -ne $settings.base_branch) { throw 'PR identity mismatch' }
Write-Output $pr.url
