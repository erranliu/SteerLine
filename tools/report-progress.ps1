param([Parameter(Mandatory)][string]$Name, [switch]$Completed, [switch]$Reset)
$ErrorActionPreference='Stop'
if ($Completed.IsPresent -eq $Reset.IsPresent) { throw 'Choose exactly one of -Completed or -Reset.' }
. (Join-Path $PSScriptRoot 'testing/RuntimeProgress.ps1')
Set-RuntimeProgress -Name $Name -Completed $Completed.IsPresent
