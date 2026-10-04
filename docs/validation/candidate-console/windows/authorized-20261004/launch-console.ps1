param([Parameter(Mandatory=$true)][string]$Manifest)
$ErrorActionPreference = 'Stop'
$taskNativeFixture = Get-Content -LiteralPath $Manifest -Raw | ConvertFrom-Json
$env:STEERLINE_HOME = $taskNativeFixture.home
$env:STEERLINE_TEST_REMOTE_URL = $taskNativeFixture.environment.STEERLINE_TEST_REMOTE_URL
$env:STEERLINE_FAKE_GIT_FETCH_REMOTE = $taskNativeFixture.environment.STEERLINE_FAKE_GIT_FETCH_REMOTE
$env:STEERLINE_INTEGRATION_HISTORY_FILE = $taskNativeFixture.environment.STEERLINE_INTEGRATION_HISTORY_FILE
[Console]::InputEncoding = New-Object System.Text.UTF8Encoding
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding
$Host.UI.RawUI.WindowTitle = 'SteerLine CC5 authorized isolated console'
$taskSize = $Host.UI.RawUI.WindowSize
$taskSize.Width = [Math]::Min(120,$Host.UI.RawUI.MaxPhysicalWindowSize.Width)
$taskSize.Height = [Math]::Min(40,$Host.UI.RawUI.MaxPhysicalWindowSize.Height)
$taskBuffer = $Host.UI.RawUI.BufferSize
$taskBuffer.Width = [Math]::Max($taskBuffer.Width,$taskSize.Width)
$taskBuffer.Height = [Math]::Max($taskBuffer.Height,$taskSize.Height)
$Host.UI.RawUI.BufferSize = $taskBuffer
$Host.UI.RawUI.WindowSize = $taskSize
& $taskNativeFixture.executable console -config $taskNativeFixture.config
exit $LASTEXITCODE
