# 控制台打开本机 Codex CLI

> 2026-10-15 TUI 退役：本文保留原交付/验收事实，TUI 专属操作和未验项不再适用，不记为通过。共享后台/API 要求继续归原记录；当前范围见 [TUI 退役](../spec/tui-retirement.spec.md)。

归属：2026-10-09 用户直接授权的普通开发，不纳入 roadmap DAG。历史操作契约见[已归档的控制台设计](../archive/design/execution-console.md)。

## 自动验证

| ID | 验收项 | 结果与证据 |
| --- | --- | --- |
| CLI-A1 | Ctrl+N 定位所选本机候选、拦截重复打开与未开始轮次的空会话、报告失败且不提交任务控制命令 | `TestNativeSessionShortcutOpensSelectedCandidateAndReportsFailure` 通过 |
| CLI-A2 | 使用冻结配置和精确 run/execution/attempt/session/generation；旧目标、其他后端及重复绑定拒绝 | `TestNativeSessionUsesFrozenProfileAndExactCandidate` 通过 |
| CLI-A3 | CLI 连接原 endpoint/session/workspace，参数不包含提示词或权限覆盖，拒绝非法 endpoint | `TestNativeViewerAttachesWithoutPromptOrPolicyOverrides` 通过 |
| CLI-A4 | Windows 新窗口有交互终端句柄；中文/空格路径和特殊字符参数保持字面值 | `TestNativeConsoleHasInteractiveHandlesAndLiteralArguments` 通过，实际创建短暂原生 helper 窗口 |

本机 Windows，2026-10-09，最小定向入口及报告：

- `go run ./cmd/test-feedback -- ./internal/app ./internal/console ./internal/product/client ./internal/execution/adapters/codex -run 'TestNativeSession|TestNativeViewer' -count=1`：通过；`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T092504.936017200Z/summary.json`。
- `go run ./cmd/test-feedback -- ./internal/app/host -run '^TestNativeConsole' -count=1`：通过；`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T092637.542385500Z/summary.json`。
- `go run ./cmd/test-feedback -- ./internal/console ./internal/product/client ./internal/testkit/boundary/... -count=1`：225 项通过；`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T092737.202611800Z/summary.json`。初次边界检查指出平台宿主错误文本包含实现名称；改为通用原生终端能力错误后复验通过，未放宽检查。
- 原生检查发现空会话尚无可恢复 rollout；增加未开始轮次的拦截后，第一条定向命令重新通过：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T093650.877461500Z/summary.json`。
- 最终控制台、产品客户端和架构边界回归 225 项通过：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T093844.132571800Z/summary.json`；`build.ps1` 成功生成源码目录的 `bin/steerline.exe`，本地 Markdown 导航与差异格式检查通过。

不跑 Full 或模型交付 L2/L3：本次增加原生查看入口，不修改派发、worker 业务输入或交付协议。以上证据不代表真实 Codex CLI 实时呈现已完成人工验收。

## 本机 CLI 连接检查

2026-10-09 用实际 Codex CLI 0.159.2 和 app-server 创建独立检查会话；provider 设置为 loopback 地址及缺失的专属密钥，原生轮次明确失败于 `STEERLINE_VIEWER_PROBE_MISSING_KEY`，没有模型执行。CLI 通过实际 `resume --remote` 连接，显示原检查轮次和原生错误。`/status` 核对原 endpoint、session、工作目录、provider/model，权限保持 `Read Only (never)`、上下文使用 0；正常退出 CLI 的原生提示为断开查看、原工作继续，不发送停止。

检查 session 为 `01a12003-0483-7780-8d16-9fe467d28826`，endpoint 为 `ws://127.0.0.1:61308`，原生读取证据与日志在 `C:/Users/lmas1/AppData/Local/Temp/steerline-native-viewer-probe-3114037383/thread-before.json`、`appserver.log`；终端呈现及 `/status` 原始输出保留在本次聊天工具记录中。临时 app-server 与 CLI 均退出，检查 session 经原生 `thread/archive` 和 `thread/delete` 成功清理，检查期间新增的三个专属目录信任条目已清理。前两次空会话连接失败不冒充通过；由此增加空会话门槛。

此检查覆盖实际 CLI 恢复与身份/权限呈现，不覆盖真实模型连续执行输出或经产品数据库完整驱动的人工窗口。

## 人工验收（待验）

窗口由使用者在本机已有任务中后置验证，不阻塞其他开发；环境为 Windows、Codex CLI 0.159.2、运行中的本机 Codex app-server 候选。

1. 用本次构建的控制台选中候选，按 Ctrl+N；确认新 CLI 窗口显示原任务历史、工作目录和随后到达的实时输出。
2. 保持 CLI 仅查看，在 SteerLine 回复原问题，确认原候选继续且 CLI 能看到该轮输出。
3. 正常退出 CLI 查看窗口，确认 SteerLine 原 worker 继续由服务管理；切换候选后再次打开，确认窗口对应新选择。
4. 原服务已停止时再次打开，确认控制台报告原进程不可用。

验收时登记原 run/execution/attempt/session 身份、截图或原生输出与实际结果；人工窗口时间待使用者安排。
