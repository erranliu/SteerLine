# CC5：用户授权后仍被原生 UI 产品策略阻断

2026-10-04，用户追加“给你授权”后再次实际诊断；仅完成工具诊断和隔离应用准备/清理，**没有原生终端输入/IME/窗口/滚动通过证据**。准备模式的 `prepare/summary.json` 为通过，仅证明同一受控服务/五候选现场及 CLI 构建可以保持并正常关闭；其 return 在人工保持段结束，未执行普通接线后半段，不能计作完整 CC1–CC4 或 CC5。

`diagnostic.json` 保存实际应用/项目配置 SHA-256 和服务实例；`fixture.json` 保存本次临时现场身份。收到 `.stop` 后本次服务、worker、Git/SQLite scratch 已清理，fixture 中的路径是历史定位，不是可继续运行的现场。没有保存或伪造截图，也没有使用 Go KeyMsg、粘贴整段中文或模拟输入替代中文组词。

当前用户已明确授权终端自动化。授权后的实际 `@oai/sky` 调用 `sky.launch_app({app:"C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe"})` 返回 `isError=true`，原文：

> product policy blocks this app: C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe

[授权后实际诊断](authorized-20261004/diagnostic.json)、[构建/配置身份](authorized-20261004/identity.json)、[隔离准备与清理](authorized-20261004/prepare/summary.json) 均保留。第一启动缺失受控 Git helper 的环境绑定，产品查询在副作用前拒绝；补齐原 fixture 的三项固定环境变量后，公开 task show 成功。shell 启动的 PowerShell 进程窗口句柄为 0，工具窗口枚举未返回该任务终端；尝试由原生 UI 工具启动交互终端时收到上述实际策略拒绝。没有绕过该拒绝，没有原生中文/按键/截图证据。

`STEERLINE_CC5_MANIFEST` 现正式包含 `STEERLINE_TEST_REMOTE_URL`、`STEERLINE_FAKE_GIT_FETCH_REMOTE`、`STEERLINE_INTEGRATION_HISTORY_FILE` 三项本次隔离 fixture 绑定，供外部产品 CLI 查询/控制使用；不涉及凭据、真实远端或 worker 权限更改。授权后准备模式通过与之前相同，仅证明现场准备/保持/清理，不代表完整 wiring 或 CC5 通过。

CC5 保持未通过，原因是实际工具产品策略，不再是缺少用户授权。[旧 diagnostic.json](diagnostic.json) 仅为授权前历史记录，不能代表当前权限意图。

## 使用同一隔离现场完成实际操作

具备人工终端/IME 操作条件时，第一终端以仓库为工作目录，选择一个新的空证据目录并启动保持模式：

```powershell
$taskCC5Directory = Join-Path (Get-Location) ('docs/validation/candidate-console/windows/manual-' + (Get-Date -Format 'yyyyMMdd-HHmmss'))
$env:STEERLINE_CC5_MANIFEST = Join-Path $taskCC5Directory 'fixture.json'
go test ./internal/steerlineintegration -run '^TestCandidateConsoleWiring$' -count=1 -v -timeout=20m
```

等待 `CC5 isolated native fixture ready` 后，第二个真实 Windows 终端读取本次 manifest，使用它的隔离 home 和产品 CLI（不是开发者/生产 SteerLine 配置）：

```powershell
$taskCC5Fixture = Get-Content '<本次新目录绝对路径>/fixture.json' -Raw | ConvertFrom-Json
$env:STEERLINE_HOME = $taskCC5Fixture.home
$env:STEERLINE_TEST_REMOTE_URL = $taskCC5Fixture.environment.STEERLINE_TEST_REMOTE_URL
$env:STEERLINE_FAKE_GIT_FETCH_REMOTE = $taskCC5Fixture.environment.STEERLINE_FAKE_GIT_FETCH_REMOTE
$env:STEERLINE_INTEGRATION_HISTORY_FILE = $taskCC5Fixture.environment.STEERLINE_INTEGRATION_HISTORY_FILE
& $taskCC5Fixture.executable console -config $taskCC5Fixture.config
```

实际人工验收需要逐项记录：

1. Task 901 只有一条主行；`d` 查看五候选，`{`/`}` 导航，确认所选 workflow_run_id/config/session 和正文保持目标。
2. `a` 打开选项，键盘选择 `continue`，`Tab` 进入备注；使用本机中文输入法逐键组词，例如“候选比较备注”。保留输入法候选窗、组词提交和编辑后的真实屏幕证据，不能以 `type_text`/直接粘贴完整中文代替。
3. `F2` 显式提交选项加备注，核对原候选/Execution/session 的规范反馈与轮次；截图和操作记录保留原 request/command 身份。另一个候选不得收到备注。
4. 缩窄真实终端，检查中文折行、候选选择、选项/备注和说明没有错位；用 `PgUp/PgDn`/方向键实际滚动详情、长问题和正文，并核对返回原候选位置。
5. `z` 确认展示候选和配置、不可撤销退出实验、worker 可能仍运行。放弃后历史保留、无反馈入口，停止待核实仍可定位。`q` 仅退出 UI。
6. 保留实际操作时间、Windows 终端/输入法版本、窗口尺寸、产品与配置 hash、服务实例、所选 WorkflowRun/Execution/session、规范反馈回执和必要截图。写清每项通过/失败；未完成项不记通过。

完成并退出 UI 后，在第三终端写本次 manifest 给出的 `.stop` 文件；第一终端结束并清理临时现场。保持模式最长 15 分钟，超时为失败，仍保存诊断。请把原生证据独立放入本次 `manual-*` 目录，不覆盖本次历史 `prepare/`。

补充清理说明：两次现场的 home/config/repository 均已不存在，原诊断客户端 PID 16988/31868 已退出，测试服务/worker 通过原 fixture stop_file 正常结束。现有构建 helper 的两份静态 `sl-product-cli-*` 缓存保留：尝试仅删除这两份已核对身份的文件时，exec 自动审批在执行前返回 `blocked by policy`，未重试或绕过；详情见两份 cleanup.json。静态缓存不构成活动服务/任务现场。
