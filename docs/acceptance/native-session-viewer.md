# 恢复原会话查看验收

归属：[实现 Plan](../plans/native-session-viewer.plan.md)。普通缺陷修复，不修改 roadmap。

| ID | 自动验收项 | 状态及证据 |
| --- | --- | --- |
| NSV-A01 | Runner 核对原身份及原生进程/thread/workspace，只读连接、零业务输入；独立控制台参数和交互句柄 | 通过；Runner `.evidence/native-session-viewer/runner-tests.log`，四个包均实际执行；含 9 个 Engine 反例/正例、5 个 live-binding 场景和真实 Windows helper |
| NSV-A02 | 固定 EXE/候选 state-dir/输入/分支；正确能力投影；HTTP 原请求不重复打开 | 定向通过；`.evidence/native-session-viewer/caller-tests.log`、`http-tests.log`、`startup-final.log`、`boundary-tests.log`；实际 Web→app 回调另由 A04 覆盖 |
| NSV-A03 | Web 对有效绑定启用，并区分离线、缺能力及缺会话；构建 | 通过；`web-tests.log` 2 项及 `web-tests-node.log`（同两项，node 环境）通过，`web-build.log` 构建通过 |
| NSV-A04 | 新 debug 包实际产品入口的原候选查看接线 | 通过；`C:/SteerlineDebug/dev-20261016-133808-6f40f2dd-dirty/evidence/smoke.log`：1 个浏览器场景通过（1.5m）；同目录 `smoke-cleanup.json` 的 passed/browser_passed=true，自有进程和监听均为 0，stop_errors 为空 |

人工项 NSV-M01：真实 Codex CLI 持续显示原会话、关闭窗口后原任务继续；待可用人工窗口。自动协议 fixture 和控制台 helper 不等价于真实模型会话体验。安装环境未改动。

源码基线：SteerLine `6f40f2dd70c2226c38e1599f81546652f475d563` 加本次工作树；Runner `95d24e5a44df8813b736f052e2ba831363a97b20` 加本次工作树，独立工作区 `C:/Users/lmas1/.codex/worktrees/7b31/SteerRunner`。Runner `go build ./...` 通过；受控 driver/fixture 只编译通过，不计行为验收。未运行 full、L2/L3 或真实模型。

首次临时启动 `C:/SteerlineDebug/native-viewer/dev-20261016-132518-6f40f2dd-dirty` 受执行宿主 kill-on-close Job 限制，未通过；原 browser 进程树已按本次唯一命令/PID/创建时间核对后结束，保留 `.evidence/native-session-viewer/browser.log`、`browser-aborted.json` 和包内启动日志。最终浏览器验证改用项目 `scripts/test-web-smoke.ps1` 的独立测试进程及新包，不放宽产品启动检查。

独立启动首次现场 `dev-20261016-133059-6f40f2dd-dirty` 未通过：探测服务尚未就绪的 native stderr 被 Windows PowerShell 的 Stop 规则提升成错误，引发停止，并与 seed create 形成版本冲突；不是查看能力验收通过。测试显式捕获探测输出，并等待两项调试任务就绪。第二现场 `dev-20261016-133453-6f40f2dd-dirty` 未通过：测试没有容忍启动瞬间 `runs:null`，已修正 poll 的空值判断。两场景清理报告均为 processes/listeners=0、stop_errors 为空；保留其 smoke.log/失败截图/trace。未修改产品启动或版本核验来规避失败。

最终场景使用本次 Runner 构建 SHA-256 `f68eda99d9627978efc0bcc12dc30682c23574fdb7cf0ca788ab6a6ce9df3cd2` 生成新包后明确选择 simulate。浏览器实际点击启用的“在 Codex 中查看”，HTTP 返回 applied；重发原 envelope 仍返回 applied，caller fixture 仅记录一次 `open-native-session`，唯一业务命令仍为原 submit，Run/Attempt/Session/Generation/Turn 均保持不变。该 fixture 回执不打开真实 Codex；原生绑定/只读协议和实际 Windows 窗口由 A01 的独立测试证明。截图及原目标附件位于包内 `evidence/browser/native-session-native-sess-898fe-replay-through-product-HTTP/`；汇总 `.evidence/native-session-viewer/browser-complete.log`。模拟夹具没有原生 last-message 采样，截图中的消息诊断不作为本次原生流验收。

本机现有 `codex.exe resume --help` 已核对 `--remote`、`--cd`、SESSION_ID 及可选 PROMPT（`.evidence/native-session-viewer/native-resume-help.txt`）；构建命令省略 PROMPT。该只读帮助检查不等价于真实会话体验。
