$ErrorActionPreference = 'Stop'
$settings = Get-Content -LiteralPath '__ROOT__/manifest.json' -Raw | ConvertFrom-Json
$branch = (& git symbolic-ref --short HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $branch -eq $settings.base_branch -or $branch -eq 'main') { throw 'Publish only the assigned candidate head' }
& git -c credential.helper= push origin "HEAD:refs/heads/$branch"
if ($LASTEXITCODE -ne 0) { throw 'Push failed' }
& gh pr create --repo erranliu/SteerlineTest --base $settings.base_branch --head $branch --draft --title "SteerLine debug $($settings.release)" --body 'Independent SteerlineTest debug environment.'
if ($LASTEXITCODE -ne 0) { throw 'PR creation failed' }
