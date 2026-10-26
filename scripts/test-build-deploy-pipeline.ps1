param([Parameter(Mandatory=$true)][string]$OutDir)
$ErrorActionPreference = 'Stop'
$OutDir = [IO.Path]::GetFullPath($OutDir)
if (Test-Path -LiteralPath $OutDir) { throw 'OutDir must be new.' }
$source = Join-Path $OutDir 'source'
$runner = Join-Path $OutDir 'runner'
# The deploy script deliberately rejects C:\SteerlineDebug as an install home.
# Only this owned temporary fixture is used; its EXEs simulate CLI responses.
$fixtureHome = Join-Path ([IO.Path]::GetTempPath()) ('steerline-deploy-pipeline-'+[Guid]::NewGuid().ToString('N'))
foreach ($directory in @((Join-Path $source 'scripts'), (Join-Path $source 'examples'), (Join-Path $runner 'scripts'), $fixtureHome)) {
    New-Item -ItemType Directory -Path $directory -Force | Out-Null
}
foreach ($name in @('build-deploy.ps1','deployment-stop.ps1','deployment-runner.ps1','build-timing.ps1')) {
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot $name) -Destination (Join-Path $source 'scripts')
}
[IO.File]::WriteAllText((Join-Path $runner 'scripts\build.ps1'), "throw 'Runner must be selected through build.ps1.'")
$program = @'
using System;
using System.IO;
using System.Security.Cryptography;
using System.Web.Script.Serialization;
class Fixture {
    static string Get(string[] args, string key) { return args[Array.IndexOf(args,key)+1]; }
    static void Json(object value) { Console.WriteLine(new JavaScriptSerializer().Serialize(value)); }
    static int Main(string[] args) {
        string name=Path.GetFileName(System.Reflection.Assembly.GetExecutingAssembly().Location);
        string events=Environment.GetEnvironmentVariable("SL_DEPLOY_EVENTS");
        File.AppendAllText(Path.Combine(events,"calls.log"),name+" "+String.Join(" ",args)+Environment.NewLine);
        string mode=Environment.GetEnvironmentVariable("SL_DEPLOY_FAIL");
        string home=Environment.GetEnvironmentVariable("STEERLINE_HOME");
        switch(args[0]) {
            case "version": Json(new {ok=true,protocol="steerrunner-cli/v1",data=new {version="fixture-v1",source_commit="fixture"}}); return 0;
            case "verify-runner-takeover":
                if(mode=="preflight") return 7;
                Json(new {candidate_sha256=Get(args,"--runner-sha256"),state_dir=Get(args,"--state-dir")}); return 0;
            case "prepare-deployment":
                if(mode=="stop") return 8;
                string digest;
                using(var hash=SHA256.Create()) digest=BitConverter.ToString(hash.ComputeHash(File.ReadAllBytes(Get(args,"--runner-exe")))).Replace("-","").ToLowerInvariant();
                Json(new {contract="steerline-deployment-preparation/v1",home=home,goal=Get(args,"--goal"),invocation_id=Get(args,"--invocation-id"),package_sha256=Get(args,"--package-sha256"),outcome="prepared",verified_at="2026-10-26T00:00:00Z",verifier_sha256=digest}); return 0;
            case "install":
                Directory.CreateDirectory(Path.Combine(home,"bin"));
                File.Copy(System.Reflection.Assembly.GetExecutingAssembly().Location,Path.Combine(home,"bin",name),true); return 0;
            case "start": return mode=="start" ? 9 : 0;
            case "status":
                if(mode=="ready") Json(new {backend=new {state="ready"},web=new {state="ready"},degraded=true,projects=new[]{new {config_id="fixture",runtime_state="failed",online=false,diagnostic="injected failure"}}});
                else Json(new {backend=new {state="ready"},web=new {state="ready"},degraded=false,projects=new object[0]});
                return 0;
        }
        return 10;
    }
}
'@
$fixtureExe = Join-Path $OutDir 'fixture.exe'
Add-Type -TypeDefinition $program -Language CSharp -OutputAssembly $fixtureExe -OutputType ConsoleApplication -ReferencedAssemblies System.Web.Extensions.dll
$fakeBuild = @'
param([switch]$Web,[switch]$ForceWebInstall,[switch]$SerialBuild,[string]$RunnerRepo,[string]$StageDirectory,[string]$TimingPath)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'scripts\build-timing.ps1')
Initialize-BuildTiming 'fixture-build'
$ok=$false
try {
    @{web=[bool]$Web;force=[bool]$ForceWebInstall;serial=[bool]$SerialBuild;runner=$RunnerRepo;stage=$StageDirectory} | ConvertTo-Json | Set-Content (Join-Path $env:SL_DEPLOY_EVENTS 'build-arguments.json')
    Invoke-BuildStage 'fixture.build' {
        if ($env:SL_DEPLOY_FAIL -eq 'build') { throw 'Injected build failure.' }
        New-Item -ItemType Directory -Path $StageDirectory -Force | Out-Null
        foreach ($name in @('steerline.exe','steerline-web.exe','steerrunner.exe')) {
            Copy-Item -LiteralPath $env:SL_DEPLOY_EXE -Destination (Join-Path $StageDirectory $name)
        }
    }
    $ok=$true
} finally { Write-BuildTiming $TimingPath $ok }
'@
[IO.File]::WriteAllText((Join-Path $source 'build.ps1'), $fakeBuild)
$sha = (Get-FileHash -LiteralPath $fixtureExe -Algorithm SHA256).Hash.ToLowerInvariant()
$config = @{version=3;runner=@{executable=(Join-Path $fixtureHome 'bin\steerrunner.exe');version='fixture-v1';sha256=$sha};execution_profiles=@(@{id='local-luna-medium'},@{id='custom'});default_execution_profile='custom'}
$config | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $source 'examples\execution.json')
& git -C $source init -q
if ($LASTEXITCODE) { throw 'Fixture git init failed.' }
& git -C $source -c user.name=Fixture -c user.email=fixture@example.test commit --allow-empty -qm fixture
if ($LASTEXITCODE) { throw 'Fixture git commit failed.' }
$saved = @{}
foreach ($key in @('SL_DEPLOY_EXE','SL_DEPLOY_EVENTS','SL_DEPLOY_FAIL','STEERLINE_HOME')) { $saved[$key]=[Environment]::GetEnvironmentVariable($key) }
$checks = [Collections.Generic.List[string]]::new()
function Assert([bool]$Value,[string]$Message) { if (-not $Value) { throw $Message } }
function Run-Deploy([string]$Name,[bool]$ShouldSucceed=$true,[string[]]$Extra=@()) {
    $events=Join-Path $OutDir $Name
    New-Item -ItemType Directory -Path $events | Out-Null
    $env:SL_DEPLOY_EVENTS=$events
    $report=Join-Path $events 'timings.json'
    $preference=$ErrorActionPreference
    try {
        $ErrorActionPreference='Continue'
        & powershell.exe -NoProfile -ExecutionPolicy Bypass -File (Join-Path $source 'scripts\build-deploy.ps1') -InstallHome $fixtureHome -RunnerRepo $runner -NoPause -TimingPath $report @Extra *> (Join-Path $events 'output.log')
        $code=$LASTEXITCODE
    } finally { $ErrorActionPreference=$preference }
    Assert (($code -eq 0) -eq $ShouldSucceed) "$Name unexpected exit $code; inspect $events"
    $result=Get-Content -LiteralPath $report -Raw | ConvertFrom-Json
    Assert ($result.succeeded -eq $ShouldSucceed) "$Name timing outcome incorrect"
    Assert (@($result.stages | Where-Object name -eq 'fixture.build').Count -eq 1) "$Name omitted imported build timings"
    $checks.Add($Name)
    return $result
}
try {
    $env:SL_DEPLOY_EXE=$fixtureExe
    $env:SL_DEPLOY_FAIL=''
    $null=Run-Deploy 'initial' $true @('-UpdateRunner','-ForceWebInstall','-SerialBuild')
    $forwarded=Get-Content (Join-Path $env:SL_DEPLOY_EVENTS 'build-arguments.json') -Raw | ConvertFrom-Json
    Assert ($forwarded.web -and $forwarded.force -and $forwarded.serial -and $forwarded.runner -eq $runner) 'Build switches were not forwarded.'
    $calls=Get-Content (Join-Path $env:SL_DEPLOY_EVENTS 'calls.log') -Raw
    Assert ($calls.Contains(' start') -and $calls.Contains(' status --json')) 'Deployment omitted start/ready checks.'
    # Existing installation fixtures preserve custom profiles and use stop preparation.
    $config | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $fixtureHome 'execution.json')
    $normal=Run-Deploy 'reuse'
    $forwarded=Get-Content (Join-Path $env:SL_DEPLOY_EVENTS 'build-arguments.json') -Raw | ConvertFrom-Json
    Assert (-not $forwarded.runner) 'Pinned Runner reuse rebuilt Runner source.'
    $calls=Get-Content (Join-Path $env:SL_DEPLOY_EVENTS 'calls.log') -Raw
    Assert ($calls.Contains(' prepare-deployment ')) 'Existing installation skipped stop verification.'
    $preserved=Get-Content (Join-Path $fixtureHome 'execution.json') -Raw | ConvertFrom-Json
    Assert ($preserved.default_execution_profile -eq 'custom' -and $preserved.execution_profiles.Count -eq 2) 'Deployment changed profiles.'
    $null=Run-Deploy 'no-start' $true @('-NoStart')
    $calls=Get-Content (Join-Path $env:SL_DEPLOY_EVENTS 'calls.log') -Raw
    Assert (-not $calls.Contains(' start') -and -not $calls.Contains(' status')) 'NoStart started or checked readiness.'
    foreach ($mode in @('build','stop','start','ready')) {
        $env:SL_DEPLOY_FAIL=$mode
        $failed=Run-Deploy ($mode+'-failure') $false
        Assert (@($failed.stages | Where-Object outcome -eq 'failed').Count -gt 0) "$mode failure missing failed timing"
        if ($mode -in @('build','stop')) {
            $callsPath=Join-Path $env:SL_DEPLOY_EVENTS 'calls.log'
            $calls=if (Test-Path $callsPath) { Get-Content $callsPath -Raw } else { '' }
            Assert (-not $calls.Contains(' install')) "$mode failure installed files."
        }
    }
    $env:SL_DEPLOY_FAIL='preflight'
    New-Item -ItemType Directory -Path (Join-Path $fixtureHome 'application-api') -Force | Out-Null
    @{sha256=('b'*64);state_dir=(Join-Path $fixtureHome 'original-state')} | ConvertTo-Json | Set-Content (Join-Path $fixtureHome 'application-api\runner-build.json')
    $null=Run-Deploy 'preflight-failure' $false
    $calls=Get-Content (Join-Path $env:SL_DEPLOY_EVENTS 'calls.log') -Raw
    Assert (-not $calls.Contains(' prepare-deployment') -and -not $calls.Contains(' install')) 'Failed preflight stopped or installed files.'
    @{passed=$true; fixture_home=$fixtureHome;checks=@($checks)} | ConvertTo-Json -Depth 5 | Set-Content (Join-Path $OutDir 'result.json')
    Write-Host "Deployment pipeline fixture passed ($($checks.Count)); no real installation used; evidence: $OutDir"
} finally {
    foreach ($key in $saved.Keys) { [Environment]::SetEnvironmentVariable($key,$saved[$key]) }
}
