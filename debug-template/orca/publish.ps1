$ErrorActionPreference = 'Stop'
$base = '__BASE__'
$repository = 'erranliu/SteerlineTest'
$branch = (& git symbolic-ref --short HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $branch -eq $base -or $branch -eq 'main') { throw 'Publish only the assigned candidate' }
& git -c credential.helper= -c 'credential.helper=!gh auth git-credential' push origin "HEAD:refs/heads/$branch"
if ($LASTEXITCODE -ne 0) { throw 'Push failed using native host authentication' }
$raw = & gh pr list --repo $repository --base $base --head $branch --state open --json url
if ($LASTEXITCODE -ne 0) { throw 'PR query failed' }
[array]$matches = ConvertFrom-Json -InputObject ($raw -join "`n")
if ($matches.Count -gt 1) { throw 'Ambiguous matching PRs' }
if ($matches.Count -eq 1) { $url = $matches[0].url } else {
    $url = & gh pr create --repo $repository --base $base --head $branch --draft --title "SteerLine Orca debug $base" --body 'Independent single-terminal Orca flow verification.'
    if ($LASTEXITCODE -ne 0) { throw 'PR creation failed' }
}
$raw = & gh pr view $url --repo $repository --json url,state,headRefName,baseRefName
if ($LASTEXITCODE -ne 0) { throw 'PR verification failed' }
$proof = ConvertFrom-Json -InputObject ($raw -join "`n")
if ($proof.state -ne 'OPEN' -or $proof.headRefName -ne $branch -or $proof.baseRefName -ne $base) { throw 'PR identity mismatch' }
Write-Output $proof.url
