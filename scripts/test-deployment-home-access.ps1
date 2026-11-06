param([Parameter(Mandatory=$true)][string]$OutDir)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'deployment-files.ps1')
if (Test-Path -LiteralPath $OutDir) { throw 'OutDir must be new.' }
New-Item -ItemType Directory -Path $OutDir | Out-Null
$sid = [Security.Principal.WindowsIdentity]::GetCurrent().User
$checks = [Collections.Generic.List[string]]::new()
function Assert([bool]$Value, [string]$Message) { if (-not $Value) { throw $Message } }
function Set-TestACL([string]$Path, [Security.AccessControl.DirectorySecurity]$ACL) {
    if ($PSVersionTable.PSEdition -eq 'Desktop') { [IO.Directory]::SetAccessControl($Path, $ACL) }
    else { [IO.FileSystemAclExtensions]::SetAccessControl([IO.DirectoryInfo]::new($Path), $ACL) }
}
function New-TestACL([Security.AccessControl.InheritanceFlags]$Inheritance = [Security.AccessControl.InheritanceFlags]::None) {
    $acl = [Security.AccessControl.DirectorySecurity]::new()
    $acl.SetAccessRuleProtection($true, $false)
    $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($sid, [Security.AccessControl.FileSystemRights]::FullControl, $Inheritance, [Security.AccessControl.PropagationFlags]::None, [Security.AccessControl.AccessControlType]::Allow))
    return $acl
}
$missing = Join-Path $OutDir 'missing'
Repair-DeploymentHomeInheritance $missing
Assert (-not (Test-Path -LiteralPath $missing)) 'ACL repair created an absent home.'
$checks.Add('absent-home')
foreach ($mode in @('ordinary','inheritable','additional-user','explicit-deny')) {
    $path = Join-Path $OutDir $mode
    New-Item -ItemType Directory -Path $path | Out-Null
    if ($mode -ne 'ordinary') {
        $acl = New-TestACL
        if ($mode -eq 'inheritable') {
            $acl = New-TestACL ([Security.AccessControl.InheritanceFlags]'ContainerInherit,ObjectInherit')
        } elseif ($mode -eq 'additional-user') {
            $users = [Security.Principal.SecurityIdentifier]::new('S-1-5-32-545')
            $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($users, [Security.AccessControl.FileSystemRights]::Read, [Security.AccessControl.AccessControlType]::Allow))
        } else {
            $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($sid, [Security.AccessControl.FileSystemRights]::Write, [Security.AccessControl.AccessControlType]::Deny))
        }
        Set-TestACL $path $acl
    }
    $before = (Get-Acl -LiteralPath $path).Sddl
    Repair-DeploymentHomeInheritance $path
    Assert ((Get-Acl -LiteralPath $path).Sddl -eq $before) "Repair changed unsupported ACL: $mode"
    $checks.Add($mode)
}
$fixtureHome = Join-Path $OutDir 'restricted'
New-Item -ItemType Directory -Path (Join-Path $fixtureHome 'bin') -Force | Out-Null
$exe = Join-Path $fixtureHome 'bin\steerline.exe'
[IO.File]::WriteAllText($exe, 'fixture executable')
$private = Join-Path $fixtureHome 'deployment.json'
Write-JSON $private @{fixture=$true} -Private
$privateACL = (Get-Acl -LiteralPath $private).Sddl
Set-TestACL $fixtureHome (New-TestACL)
$denied = $false
try { $null = Test-Path -LiteralPath $exe -PathType Leaf -ErrorAction Stop } catch [UnauthorizedAccessException] { $denied = $true }
Assert $denied 'Fixture did not reproduce denied executable lookup.'
Repair-DeploymentHomeInheritance $fixtureHome
Assert ([IO.File]::ReadAllText($exe) -eq 'fixture executable') 'Repair did not restore executable access.'
Assert ((Get-Acl -LiteralPath $private).Sddl -eq $privateACL) 'Repair changed protected record ACL.'
$before = (Get-Acl -LiteralPath $fixtureHome).Sddl
Repair-DeploymentHomeInheritance $fixtureHome
Assert ((Get-Acl -LiteralPath $fixtureHome).Sddl -eq $before) 'Repair was not idempotent.'
$checks.Add('restricted-home-recovery')
@{passed=$true;shell=$PSVersionTable.PSVersion.ToString();checks=@($checks)} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $OutDir 'result.json')
Write-Output "Passed deployment ACL guards ($($checks.Count)): $OutDir"
