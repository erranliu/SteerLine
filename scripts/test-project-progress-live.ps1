param(
    [Parameter(Mandatory)][string]$ProjectRepository,
    [Parameter(Mandatory)][string]$ProjectCommit,
    [Parameter(Mandatory)][string]$SteerRunnerExe,
    [Parameter(Mandatory)][string]$OutRoot,
    [switch]$IndependentProcess
)
$ErrorActionPreference='Stop'
if (!$IndependentProcess) {
    . (Join-Path $PSScriptRoot 'independent-test.ps1')
    Invoke-IndependentTest -ScriptPath $PSCommandPath -TestArguments @('-ProjectRepository',$ProjectRepository,'-ProjectCommit',$ProjectCommit,'-SteerRunnerExe',$SteerRunnerExe,'-OutRoot',$OutRoot) -Timeout ([TimeSpan]::FromMinutes(18))
    return
}
$sourceRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
foreach ($path in @($ProjectRepository,$SteerRunnerExe,$OutRoot)) {
    if (![IO.Path]::IsPathRooted($path)) { throw 'Inputs must be absolute paths.' }
}
$scope=[IO.Path]::GetFullPath($OutRoot)
if (!$scope.StartsWith('C:\SteerlineDebug\',[StringComparison]::OrdinalIgnoreCase)) { throw 'Live progress scope must be below C:\SteerlineDebug.' }
if (Test-Path -LiteralPath $scope) { throw 'OutRoot must be new.' }
[IO.Directory]::CreateDirectory($scope)|Out-Null
foreach ($directory in @('bin','reports','scratch')) { [IO.Directory]::CreateDirectory((Join-Path $scope $directory))|Out-Null }
Push-Location $sourceRoot
try {
    $sourceCommit=(& git rev-parse HEAD).Trim()
    $sourceDirty=if (& git status --porcelain) {'true'}else{'false'}
    $nativeHome=$env:CODEX_HOME
    if (!$nativeHome) { $nativeHome=Join-Path ([Environment]::GetFolderPath('UserProfile')) '.codex' }
    & go build -ldflags "-X github.com/erranliu/SteerLine/internal/app.buildSourceCommit=$sourceCommit -X github.com/erranliu/SteerLine/internal/app.buildSourceDirty=$sourceDirty" -o (Join-Path $scope 'bin/steerline.exe') ./cmd/steerline
    if ($LASTEXITCODE) { throw 'SteerLine build failed.' }
    & go build -o (Join-Path $scope 'bin/helper.exe') ./tests/e2e/testdata/controlled-helper
    if ($LASTEXITCODE) { throw 'Git/provider helper build failed.' }
    & go test -tags=e2e -c -o (Join-Path $scope 'bin/progress.test.exe') ./tests/e2e/internal/testenv
    if ($LASTEXITCODE) { throw 'Live acceptance build failed.' }
    $env:STEERLINE_E2E_RUN=@{Root=$sourceRoot;Binary=(Join-Path $scope 'bin/steerline.exe');Helper=(Join-Path $scope 'bin/helper.exe');Runner=$SteerRunnerExe;Output=(Join-Path $scope 'reports');Scratch=(Join-Path $scope 'scratch');AuthHome=$nativeHome;Level='L4';SourceCommit=$sourceCommit;SourceDirty=$sourceDirty}|ConvertTo-Json -Compress
    $env:STEERLINE_PROGRESS_PROJECT=$ProjectRepository
    $env:STEERLINE_PROGRESS_COMMIT=$ProjectCommit
    & (Join-Path $scope 'bin/progress.test.exe') '-test.run=^TestSteerlineTestLiveProgress$' '-test.v' '-test.timeout=14m' *> (Join-Path $scope 'test.log')
    $testExit=$LASTEXITCODE
    $scene=Get-Content -Raw -LiteralPath (Join-Path $scope 'reports/scenarios/testing.project-progress-local-codex/scenario.json')|ConvertFrom-Json
    @{passed=($testExit -eq 0 -and $scene.passed -and $scene.cleanup_complete);exit_code=$testExit;cleanup_complete=$scene.cleanup_complete;scope=$scope;source_commit=$sourceCommit;project_commit=$ProjectCommit}|ConvertTo-Json|Set-Content -Encoding utf8 -LiteralPath (Join-Path $scope 'result.json')
    if ($testExit -ne 0 -or !$scene.passed -or !$scene.cleanup_complete) { throw "Live project progress failed: $scope" }
    Write-Output "Live project progress PASSED: $scope"
} finally { Pop-Location }
