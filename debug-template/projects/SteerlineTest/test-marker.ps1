param([Parameter(Mandatory=$true)][string]$Out)
$ErrorActionPreference='Stop'
if (Test-Path -LiteralPath $Out) { throw 'Use a new execution output directory' }
$beforeCommit=(& git rev-parse HEAD).Trim()
$beforeStatus= & git status --porcelain --untracked-files=all
$beforeClean=$LASTEXITCODE -eq 0 -and -not $beforeStatus
New-Item -ItemType Directory -Path $Out -Force | Out-Null
$catalog=Get-Content -Raw -Encoding utf8 -LiteralPath (Join-Path $PSScriptRoot 'docs/testing/test-list.json') | ConvertFrom-Json
$definition=$catalog.tests | Where-Object {$_.id -eq 'fixture.marker'}
$ordered=[ordered]@{id=$definition.id;name=$definition.name;level=$definition.level;environment=$definition.environment;command=$definition.command;check=$definition.check}
$bytes=[Text.Encoding]::UTF8.GetBytes(($ordered | ConvertTo-Json -Compress))
$sha=[Security.Cryptography.SHA256]::Create()
try {$digest=([BitConverter]::ToString($sha.ComputeHash($bytes))).Replace('-','').ToLowerInvariant()} finally {$sha.Dispose()}
$inputPath=Join-Path $PSScriptRoot '.steerrunner/artifacts/fixture-input.txt'
$observed=if(Test-Path -LiteralPath $inputPath){[IO.File]::ReadAllText($inputPath)}else{''}
$passed=$observed -eq 'fixture-ok'
$message=if($passed){'Marker assertion passed'}else{'Marker assertion failed: actual input differs from fixture-ok'}
$summary=[ordered]@{test_id='fixture.marker';passed=$passed;complete=$true;expected='fixture-ok';actual=$observed;message=$message}
$utf8=[Text.UTF8Encoding]::new($false)
[IO.File]::WriteAllText((Join-Path $Out 'summary.json'),($summary|ConvertTo-Json),$utf8)
$afterCommit=(& git rev-parse HEAD).Trim()
$afterStatus= & git status --porcelain --untracked-files=all
$afterClean=$LASTEXITCODE -eq 0 -and -not $afterStatus
$status=if($passed){'passed'}else{'failed'}
$report=[ordered]@{id=[Guid]::NewGuid().ToString('N');sequence=[uint64][DateTime]::UtcNow.Ticks;selection=@('fixture.marker');environment='isolated';definitions=@{'fixture.marker'=$digest};before_commit=$beforeCommit;after_commit=$afterCommit;before_clean=[bool]$beforeClean;after_clean=[bool]$afterClean;results=@(@{test_id='fixture.marker';status=$status;message=$message;log='summary.json'})}
$pending=Join-Path $Out '.managed.tmp'
[IO.File]::WriteAllText($pending,($report|ConvertTo-Json -Depth 8),$utf8)
Move-Item -LiteralPath $pending -Destination (Join-Path $Out 'managed-fixture.marker.json')
if($passed){exit 0}else{exit 1}
