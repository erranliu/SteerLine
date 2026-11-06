param([Parameter(Mandatory=$true)][string]$OutDir)
$ErrorActionPreference = 'Stop'
$OutDir = [IO.Path]::GetFullPath($OutDir)
if (Test-Path -LiteralPath $OutDir) { throw 'OutDir must be new.' }
$source = Join-Path $OutDir 'source'
$runner = Join-Path $OutDir 'runner'
# The deploy script deliberately rejects C:\SteerlineDebug as an install home.
# Only this owned temporary fixture is used; its EXEs simulate CLI responses.
$fixtureHome = Join-Path ([IO.Path]::GetTempPath()) ("steerline-deploy-pipeline-"+[Guid]::NewGuid().ToString('N')+"-quote' __HOME__ & space")
foreach ($directory in @((Join-Path $source 'scripts'), (Join-Path $source 'examples'), (Join-Path $runner 'scripts'), $fixtureHome)) {
    New-Item -ItemType Directory -Path $directory -Force | Out-Null
}
foreach ($name in @('build-deploy.ps1','deployment-stop.ps1','deployment-start.ps1','deployment-runner.ps1','deployment-files.ps1','build-timing.ps1')) {
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot $name) -Destination (Join-Path $source 'scripts')
}
[IO.File]::WriteAllText((Join-Path $runner 'scripts\build.ps1'), "throw 'Runner must be selected through build.ps1.'")
$program = @'
using System;
using System.IO;
using System.Security.Cryptography;
using System.Web.Script.Serialization;
using System.Runtime.InteropServices;
class Fixture {
    [DllImport("kernel32.dll")] static extern bool IsProcessInJob(IntPtr process, IntPtr job, out bool result);
    [DllImport("kernel32.dll")] static extern bool QueryInformationJobObject(IntPtr job, int kind, byte[] buffer, uint length, IntPtr returned);
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
            case "verify-runner-upgrade":
                if(mode=="preflight" || mode=="drain") return 7;
                if(File.Exists(Path.Combine(Get(args,"--home"),"bin","steerline.exe")) && !File.Exists(Path.Combine(events,"stopped"))) return 11;
                Json(new {contract="steerline-runner-upgrade/v1",home=Get(args,"--home"),candidate_sha256=Get(args,"--runner-sha256"),state_format="steerrunner-state/v8",state_dir=Path.Combine(Get(args,"--home"),"runner-state")}); return 0;
            case "prepare-deployment":
                if(mode=="stop") return 8;
                File.WriteAllText(Path.Combine(events,"stopped"),"original service exited");
                string digest;
                using(var hash=SHA256.Create()) digest=BitConverter.ToString(hash.ComputeHash(File.ReadAllBytes(Get(args,"--runner-exe")))).Replace("-","").ToLowerInvariant();
                Json(new {contract="steerline-deployment-preparation/v1",home=home,goal=Get(args,"--goal"),invocation_id=Get(args,"--invocation-id"),package_sha256=Get(args,"--package-sha256"),outcome="prepared",verified_at="2026-10-26T00:00:00Z",verifier_sha256=digest}); return 0;
            case "install":
                Directory.CreateDirectory(Path.Combine(home,"bin"));
                File.Copy(System.Reflection.Assembly.GetExecutingAssembly().Location,Path.Combine(home,"bin",name),true); return 0;
            case "start":
                bool inJob;
                if(!IsProcessInJob(new IntPtr(-1),IntPtr.Zero,out inJob)) return 12;
                if(inJob) {
                    var limits=new byte[144];
                    if(!QueryInformationJobObject(IntPtr.Zero,9,limits,(uint)limits.Length,IntPtr.Zero) || (BitConverter.ToUInt32(limits,16)&0x2000)!=0) return 13;
                }
                File.WriteAllText(Path.Combine(events,"startup-lifetime-verified"),"no kill-on-close Job");
                return mode=="start" ? 9 : 0;
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
        if ($env:SL_DEPLOY_RUNNER_CHANGE -eq '1') {
            [IO.File]::AppendAllText((Join-Path $StageDirectory 'steerrunner.exe'), 'changed-runner-build')
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
foreach ($key in @('SL_DEPLOY_EXE','SL_DEPLOY_EVENTS','SL_DEPLOY_FAIL','SL_DEPLOY_RUNNER_CHANGE','STEERLINE_HOME')) { $saved[$key]=[Environment]::GetEnvironmentVariable($key) }
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
    if ($ShouldSucceed) {
        $acl = Get-Acl -LiteralPath (Join-Path $fixtureHome 'deployment.json')
        $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User
        $rules = @($acl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier]))
        Assert ($acl.AreAccessRulesProtected -and $acl.GetOwner([Security.Principal.SecurityIdentifier]) -eq $sid -and $rules.Count -eq 1 -and $rules[0].IdentityReference -eq $sid -and $rules[0].FileSystemRights -eq [Security.AccessControl.FileSystemRights]::FullControl -and -not $rules[0].IsInherited) "$Name wrote a deployment record rejected by private lifecycle readers"
    }
    Assert (@($result.stages | Where-Object name -eq 'fixture.build').Count -eq 1) "$Name omitted imported build timings"
    $checks.Add($Name)
    return $result
}
try {
    $env:SL_DEPLOY_EXE=$fixtureExe
    $env:SL_DEPLOY_RUNNER_CHANGE=''
    $invalidEvents=Join-Path $OutDir 'discard-without-update'
    New-Item -ItemType Directory -Path $invalidEvents | Out-Null
    $env:SL_DEPLOY_EVENTS=$invalidEvents
    $preference=$ErrorActionPreference
    try {
        $ErrorActionPreference='Continue'
        & powershell.exe -NoProfile -ExecutionPolicy Bypass -File (Join-Path $source 'scripts\build-deploy.ps1') -InstallHome $fixtureHome -RunnerRepo $runner -DiscardRuns -NoPause *> (Join-Path $invalidEvents 'output.log')
        $invalidCode=$LASTEXITCODE
    } finally { $ErrorActionPreference=$preference }
    Assert ($invalidCode -ne 0 -and -not (Test-Path (Join-Path $invalidEvents 'calls.log')) -and -not (Test-Path (Join-Path $invalidEvents 'build-arguments.json'))) 'DiscardRuns without UpdateRunner built or touched an installation.'
    $checks.Add('discard-without-update')
    $env:SL_DEPLOY_FAIL='preflight'
    $null=Run-Deploy 'fresh-preflight-failure' $false @('-UpdateRunner')
    $calls=Get-Content (Join-Path $env:SL_DEPLOY_EVENTS 'calls.log') -Raw
    Assert (-not $calls.Contains(' prepare-deployment') -and -not $calls.Contains(' install')) 'Fresh preflight failure installed files.'
    $env:SL_DEPLOY_FAIL=''
    $null=Run-Deploy 'initial' $true @('-UpdateRunner','-ForceWebInstall','-SerialBuild')
    $forwarded=Get-Content (Join-Path $env:SL_DEPLOY_EVENTS 'build-arguments.json') -Raw | ConvertFrom-Json
    Assert ($forwarded.web -and $forwarded.force -and $forwarded.serial -and $forwarded.runner -eq $runner) 'Build switches were not forwarded.'
    $calls=Get-Content (Join-Path $env:SL_DEPLOY_EVENTS 'calls.log') -Raw
    Assert ($calls.Contains(' start') -and $calls.Contains(' status --json')) 'Deployment omitted start/ready checks.'
    Assert (Test-Path (Join-Path $env:SL_DEPLOY_EVENTS 'startup-lifetime-verified')) 'Startup inherited a kill-on-close Job or lost caller environment.'
    # Existing installation fixtures preserve custom profiles and use stop preparation.
    $config | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $fixtureHome 'execution.json')
    $normal=Run-Deploy 'reuse'
    $forwarded=Get-Content (Join-Path $env:SL_DEPLOY_EVENTS 'build-arguments.json') -Raw | ConvertFrom-Json
    Assert (-not $forwarded.runner) 'Pinned Runner reuse rebuilt Runner source.'
    $calls=Get-Content (Join-Path $env:SL_DEPLOY_EVENTS 'calls.log') -Raw
    Assert ($calls.Contains(' prepare-deployment ')) 'Existing installation skipped stop verification.'
    Assert ($calls.IndexOf(' prepare-deployment ') -lt $calls.IndexOf(' verify-runner-upgrade ')) 'Drainage was queried before automatic stop.'
    $preserved=Get-Content (Join-Path $fixtureHome 'execution.json') -Raw | ConvertFrom-Json
    Assert ($preserved.default_execution_profile -eq 'custom' -and $preserved.execution_profiles.Count -eq 2) 'Deployment changed profiles.'
    $env:SL_DEPLOY_RUNNER_CHANGE='1'
    $null=Run-Deploy 'runner-upgrade' $true @('-UpdateRunner')
    $calls=Get-Content (Join-Path $env:SL_DEPLOY_EVENTS 'calls.log') -Raw
    Assert ($calls.Contains('--goal replace') -and $calls.IndexOf(' prepare-deployment ') -lt $calls.IndexOf(' verify-runner-upgrade ') -and $calls.IndexOf(' verify-runner-upgrade ') -lt $calls.IndexOf(' install')) 'Runner upgrade did not stop, verify drainage, then install.'
    Assert ((Get-FileHash (Join-Path $fixtureHome 'bin\steerrunner.exe')).Hash.ToLowerInvariant() -ne $sha) 'Runner upgrade fixture did not change the release.'
    $env:SL_DEPLOY_RUNNER_CHANGE=''
    $null=Run-Deploy 'discard-runs' $true @('-UpdateRunner','-DiscardRuns','-NoStart')
    $calls=Get-Content (Join-Path $env:SL_DEPLOY_EVENTS 'calls.log') -Raw
    Assert ($calls.Contains('--goal discard-runs') -and $calls.IndexOf(' prepare-deployment ') -lt $calls.IndexOf(' verify-runner-upgrade ') -and -not $calls.Contains(' start')) 'Explicit discard did not prepare before upgrade or ignored NoStart.'
    $null=Run-Deploy 'no-start' $true @('-NoStart')
    $calls=Get-Content (Join-Path $env:SL_DEPLOY_EVENTS 'calls.log') -Raw
    Assert (-not $calls.Contains(' start') -and -not $calls.Contains(' status')) 'NoStart started or checked readiness.'
    # Reproduce billing's non-inheritable home ACL: Windows strips inherited
    # access from existing bin/etc. The actual entry must repair before its
    # DiscardRuns existence check, keeping protected lifecycle records private.
    . (Join-Path $PSScriptRoot 'deployment-files.ps1')
    $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User
    $privateBefore = (Get-Acl -LiteralPath (Join-Path $fixtureHome 'deployment.json')).Sddl
    $originalHashes = @{}
    foreach ($name in @('bin\steerline.exe','bin\steerline-web.exe','bin\steerrunner.exe','execution.json')) { $originalHashes[$name] = (Get-FileHash -LiteralPath (Join-Path $fixtureHome $name)).Hash }
    function Restrict-FixtureHome {
        $acl = [Security.AccessControl.DirectorySecurity]::new()
        $acl.SetOwner($sid)
        $acl.SetAccessRuleProtection($true, $false)
        $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($sid, [Security.AccessControl.FileSystemRights]::FullControl, [Security.AccessControl.AccessControlType]::Allow))
        [IO.Directory]::SetAccessControl($fixtureHome, $acl)
    }
    Restrict-FixtureHome
    Assert (@((Get-Acl -LiteralPath (Join-Path $fixtureHome 'bin')).Access).Count -eq 0) 'Fixture did not reproduce empty bin ACL.'
    $denied = $false
    try { $null = Test-Path -LiteralPath (Join-Path $fixtureHome 'bin\steerline.exe') -PathType Leaf -ErrorAction Stop } catch [UnauthorizedAccessException] { $denied = $true }
    Assert $denied 'Fixture did not reproduce the reported Test-Path access denial.'
    Repair-DeploymentHomeInheritance $fixtureHome
    foreach ($name in $originalHashes.Keys) { Assert ((Get-FileHash -LiteralPath (Join-Path $fixtureHome $name)).Hash -eq $originalHashes[$name]) "ACL repair changed $name" }
    Assert ((Get-Acl -LiteralPath (Join-Path $fixtureHome 'deployment.json')).Sddl -eq $privateBefore) 'ACL repair altered a protected lifecycle record.'
    $repaired = (Get-Acl -LiteralPath $fixtureHome).Sddl
    Repair-DeploymentHomeInheritance $fixtureHome
    Assert ((Get-Acl -LiteralPath $fixtureHome).Sddl -eq $repaired) 'ACL repair was not idempotent.'
    Restrict-FixtureHome
    $null=Run-Deploy 'restricted-home-recovery' $true @('-UpdateRunner','-DiscardRuns','-NoStart')
    $calls=Get-Content (Join-Path $env:SL_DEPLOY_EVENTS 'calls.log') -Raw
    Assert ($calls.Contains('--goal discard-runs') -and -not $calls.Contains(' start')) 'ACL recovery bypassed the requested deployment preparation.'
    foreach ($mode in @('build','stop','start','ready','drain')) {
        $env:SL_DEPLOY_FAIL=$mode
        $before=@{}
        foreach ($name in @('steerline.exe','steerline-web.exe','steerrunner.exe','..\execution.json')) { $before[$name]=(Get-FileHash (Join-Path $fixtureHome ('bin\'+$name))).Hash }
        $failed=Run-Deploy ($mode+'-failure') $false @('-UpdateRunner')
        Assert (@($failed.stages | Where-Object outcome -eq 'failed').Count -gt 0) "$mode failure missing failed timing"
        if ($mode -in @('build','stop','drain')) {
            $callsPath=Join-Path $env:SL_DEPLOY_EVENTS 'calls.log'
            $calls=if (Test-Path $callsPath) { Get-Content $callsPath -Raw } else { '' }
            Assert (-not $calls.Contains(' install')) "$mode failure installed files."
            foreach ($name in $before.Keys) { Assert ((Get-FileHash (Join-Path $fixtureHome ('bin\'+$name))).Hash -eq $before[$name]) "$mode failure changed $name" }
            if ($mode -eq 'drain') { Assert ($calls.Contains(' prepare-deployment') -and -not $calls.Contains(' start')) 'Drain failure did not stop first or restarted business.' }
        }
    }
    $env:SL_DEPLOY_FAIL='stop'
    $null=Run-Deploy 'discard-stop-failure' $false @('-UpdateRunner','-DiscardRuns')
    $calls=Get-Content (Join-Path $env:SL_DEPLOY_EVENTS 'calls.log') -Raw
    Assert ($calls.Contains('--goal discard-runs') -and -not $calls.Contains(' install') -and -not $calls.Contains(' start')) 'Failed discard proceeded with file replacement or startup.'
    $env:SL_DEPLOY_FAIL='preflight'
    New-Item -ItemType Directory -Path (Join-Path $fixtureHome 'application-api') -Force | Out-Null
    @{sha256=('b'*64);state_dir=(Join-Path $fixtureHome 'original-state')} | ConvertTo-Json | Set-Content (Join-Path $fixtureHome 'application-api\runner-build.json')
    $null=Run-Deploy 'preflight-failure' $false
    $calls=Get-Content (Join-Path $env:SL_DEPLOY_EVENTS 'calls.log') -Raw
    Assert ($calls.Contains(' prepare-deployment') -and -not $calls.Contains(' install') -and -not $calls.Contains(' start')) 'Failed drainage verification installed files or restarted business.'
    @{passed=$true; fixture_home=$fixtureHome;checks=@($checks)} | ConvertTo-Json -Depth 5 | Set-Content (Join-Path $OutDir 'result.json')
    Write-Host "Deployment pipeline fixture passed ($($checks.Count)); no real installation used; evidence: $OutDir"
} finally {
    foreach ($key in $saved.Keys) { [Environment]::SetEnvironmentVariable($key,$saved[$key]) }
}
