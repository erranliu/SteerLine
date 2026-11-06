function Assert-Plain([string]$Path) {
    $cursor = [IO.Path]::GetFullPath($Path)
    while ($cursor) {
        $item = Get-Item -LiteralPath $cursor -Force -ErrorAction SilentlyContinue
        if ($item -and (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $item.LinkType -eq 'HardLink')) { throw "Linked deployment path: $cursor" }
        $parent = [IO.Path]::GetDirectoryName($cursor)
        if ($parent -eq $cursor) { break }
        $cursor = $parent
    }
}

function Repair-DeploymentHomeInheritance([string]$Path) {
    Assert-Plain $Path
    try {
        $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    } catch [System.Management.Automation.ItemNotFoundException] { return }
    if (-not $item.PSIsContainer) { throw "Installation home is not a directory: $Path" }
    $acl = Get-Acl -LiteralPath $Path -ErrorAction Stop
    $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User
    $rules = @($acl.GetAccessRules($true, $true, [Security.Principal.SecurityIdentifier]))
    # Billing once secured the shared home as a private API directory. Repair
    # only that exact current-owner/current-user ACL; preserve all other ACLs.
    if ($acl.GetOwner([Security.Principal.SecurityIdentifier]) -ne $sid -or -not $acl.AreAccessRulesProtected -or $rules.Count -ne 1) { return }
    $rule = $rules[0]
    if ($rule.IdentityReference -ne $sid -or $rule.AccessControlType -ne [Security.AccessControl.AccessControlType]::Allow -or $rule.FileSystemRights -ne [Security.AccessControl.FileSystemRights]::FullControl -or $rule.IsInherited -or $rule.InheritanceFlags -ne [Security.AccessControl.InheritanceFlags]::None -or $rule.PropagationFlags -ne [Security.AccessControl.PropagationFlags]::None) { return }
    $inheritance = [Security.AccessControl.InheritanceFlags]::ContainerInherit -bor [Security.AccessControl.InheritanceFlags]::ObjectInherit
    # Submit only the DACL change through .NET. Set-Acl can request SACL
    # privileges that an ordinary directory owner does not have.
    $repaired = [Security.AccessControl.DirectorySecurity]::new()
    $repaired.SetAccessRuleProtection($true, $false)
    $repaired.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($sid, [Security.AccessControl.FileSystemRights]::FullControl, $inheritance, [Security.AccessControl.PropagationFlags]::None, [Security.AccessControl.AccessControlType]::Allow))
    if ($PSVersionTable.PSEdition -eq 'Desktop') {
        [IO.Directory]::SetAccessControl($Path, $repaired)
    } else {
        [IO.FileSystemAclExtensions]::SetAccessControl([IO.DirectoryInfo]::new($Path), $repaired)
    }
    Write-Output "Restored current-user access inheritance on installation home: $Path"
}

function Write-JSON([string]$Path, $Value, [switch]$Private) {
    Assert-Plain $Path
    $temporary = $Path + '.' + [Guid]::NewGuid().ToString('N') + '.tmp'
    $stream = $null
    try {
        $bytes = [Text.UTF8Encoding]::new($false).GetBytes(($Value | ConvertTo-Json -Depth 50))
        if ($Private) {
            $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User
            $acl = [Security.AccessControl.FileSecurity]::new()
            $acl.SetOwner($sid)
            $acl.SetAccessRuleProtection($true, $false)
            $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($sid, [Security.AccessControl.FileSystemRights]::FullControl, [Security.AccessControl.AccessControlType]::Allow))
            # Attach the protected ACL at creation, before any record bytes exist.
            if ($PSVersionTable.PSEdition -eq 'Desktop') {
                $stream = [IO.FileStream]::new($temporary, [IO.FileMode]::CreateNew, [Security.AccessControl.FileSystemRights]::Write, [IO.FileShare]::None, 4096, [IO.FileOptions]::None, $acl)
            } else {
                $stream = [IO.FileSystemAclExtensions]::Create([IO.FileInfo]::new($temporary), [IO.FileMode]::CreateNew, [Security.AccessControl.FileSystemRights]::Write, [IO.FileShare]::None, 4096, [IO.FileOptions]::None, $acl)
            }
            $stream.Write($bytes, 0, $bytes.Length)
            $stream.Flush($true)
            $stream.Dispose()
            $stream = $null
            if (-not ('SteerLine.DeploymentFiles' -as [type])) {
                Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
namespace SteerLine {
    public static class DeploymentFiles {
        [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
        [return: MarshalAs(UnmanagedType.Bool)]
        public static extern bool MoveFileEx(string source, string target, uint flags);
    }
}
'@
            }
            Assert-Plain $Path
            # ReplaceFile preserves the OLD destination ACL. MoveFileEx keeps
            # this new file's private ACL when atomically replacing the record.
            if (-not [SteerLine.DeploymentFiles]::MoveFileEx($temporary, $Path, 9)) {
                throw [ComponentModel.Win32Exception]::new([Runtime.InteropServices.Marshal]::GetLastWin32Error())
            }
        } else {
            [IO.File]::WriteAllBytes($temporary, $bytes)
            Move-Item -LiteralPath $temporary -Destination $Path -Force
        }
    } finally {
        if ($stream) { $stream.Dispose() }
        if (Test-Path -LiteralPath $temporary) { Remove-Item -LiteralPath $temporary -Force }
    }
}
