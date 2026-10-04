# CC5：原生终端验收未通过

2026-10-04，仅完成工具诊断和隔离应用准备/清理，**没有原生终端输入/IME/窗口/滚动通过证据**。准备模式的 `prepare/summary.json` 为通过，仅证明同一受控服务/五候选现场及 CLI 构建可以保持并正常关闭；其 return 在人工保持段结束，未执行普通接线后半段，不能计作完整 CC1–CC4 或 CC5。

`diagnostic.json` 保存实际应用/项目配置 SHA-256 和服务实例；`fixture.json` 保存本次临时现场身份。收到 `.stop` 后本次服务、worker、Git/SQLite scratch 已清理，fixture 中的路径是历史定位，不是可继续运行的现场。没有保存或伪造截图，也没有使用 Go KeyMsg、粘贴整段中文或模拟输入替代中文组词。

当前 `mcp__cua_repl` 明确禁用 native computer API；另一个可用 `@oai/sky` 模块加载和窗口枚举成功，但所应用 [computer-use 技能](C:/Users/lmas1/.codex/plugins/cache/openai-bundled/computer-use/26.928.31416/skills/computer-use/SKILL.md) 的 [guidance.md](C:/Users/lmas1/.codex/plugins/cache/openai-bundled/computer-use/26.928.31416/docs/guidance.md) 明确规定：

> Do not automate terminal applications such as Windows Terminal, Command Prompt, or Windows PowerShell.

因此本次未向 Windows 终端注入输入、操作 IME 或修改窗口。CC5 保持未通过；不扩大权限、不变更断言、不改变本机环境安全设置。

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
