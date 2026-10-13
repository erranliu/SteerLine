# 任务 #44 验收记录：WinUI 基座与原生输入

> 2026-10-13 归档说明：本文件仅保留原方案/交付历史，不是当前执行或验收要求。旧编号不复用；当前规划仅 #91，见[execution-map](../../../roadmap/execution-map.md)。#44 的实现删除待 #91 实施；用户撤销后续人工验收要求，原未验保持未验，不记为通过。

唯一归属 #44；固定 [Spec](../../../archive/spec/retired/phase2-winui-foundation.spec.md)、[Plan](../../../archive/spec/retired/phase2-winui-foundation.plan.md) 和 [二期共同约束](../../../spec/phase2-constraints.md) 均取自 commit `550234ab0fb0aa74a3676ff09dde6e4bd4325295`。硬前置 #45 已交付并包含在代码起点；未调用 SteerLine 中心准入或创建外部 Task/DAG。

2026-10-10，#44 已经实现、独立复审、局部修复及新的独立终审，PR #273 于 UTC 05:54:56 真实合并；head `b3b52c24fba2a96a1746d3249756518dbf1f9d3b`、merge `f04fdf089a58723909279d44c497377427b868dd`。关闭草稿竞态由 `/root/fix44_round1` 修复于 source `f1846b9bd628f43603658d265db0364e1cb87750`，`/root/review44_round2` 终审无新确认阻塞缺陷；[实际审核记录](https://github.com/erranliu/SteerLine/pull/273#issuecomment-6094396874)。新编译定向测试 19/0/0、WinUI 构建 0 warning/error，独立复跑现有编译测试 19/0/0；不是新 source 真实窗口验证。历史 `c5c9eb47` 窗口、公共查询与清理证据保留原来源。UI2-M/UI4-M/UI5-M 按用户指示待人工、最迟 2.1 签收，登记 `passed_with_manual_pending`，未宣称全部验收完成。 固定契约仍为 `550234ab`；原始实现和实际主干集成历史保留如下。

## 自动验收

| 项目 | 结果 | 证据与范围 |
| --- | --- | --- |
| UI1 | 已通过 | 历史运行候选 commit `c5c9eb470835c6ab28f7343b40b41f3648f086cf` 在 Windows 11 Home 10.0.26200 x64 上以 .NET SDK 8.0.416 / Windows App SDK 1.8.260921001 构建 self-contained `win-x64` 并启动真实 WinUI 窗口。restore/build 日志、manifest 与 exe SHA-256 见包 `dev-20261010-122449-c5c9eb47/evidence/`。 |
| UI2-A | 已通过 | 历史 c5c9 候选的 15 个定向测试通过，0 失败、0 跳过；覆盖无目标时禁用输入、本地意图不调用业务命令、目标草稿隔离、异步保存期间切目标和迟到响应保护。真实中文 IME 组词行为仍留给 UI2-M。 |
| UI3 | 已通过 | 包 `dev-20261010-122449-c5c9eb47` 中，WinUI 使用配置的 Git executable/args 连接现有 `steerline-client/v1` 服务并显示公开任务查询；受控 worker 运行时 Task 1 为 `in_progress`、Task 2 为 `open`。协议、仓库身份、project ID、实例、PID、当前 SID 私有 ACL 与详情目标均核验。TaskPage `project_id` 表示仓库身份路径。未读取 SQLite 或私有日志。 |
| UI4-A | 已通过 | 15 个定向测试中的可访问名称和输入状态断言通过；最终真实 WinUI 窗口可访问树显示配置、项目、任务列表、状态、多行输入和本地意图控件。 |
| UI5 | 待人工验收 | 最终候选在受控 worker 运行期间成功连接并显示 Task 1；本次没有确认正常关闭 WinUI 后服务与 worker 的原 instance/PID 持续不变，因此不记自动通过。按用户指示留待人工检查。 |

最终代码候选测试命令：

```powershell
dotnet restore ui/SteerLine.Windows/SteerLine.Windows.csproj
dotnet test ui/SteerLine.Windows.Tests/SteerLine.Windows.Tests.csproj -c Debug
dotnet build ui/SteerLine.Windows/SteerLine.Windows.csproj -c Debug
```

最终同源日志：app/tests restore exit 0；WinUI tests 15 passed / 0 failed / 0 skipped；WinUI build exit 0 / 0 warnings / 0 errors；`go test ./internal/product/httpapi ./internal/task/model ./internal/execution/adapters/codex ./internal/execution/process` 通过。exe SHA-256 `249a7f7238fc6db052f5d2b7bf28d12567b61afc60a0be3df0ca4b18d3ef5a71`。日志、NuGet 缓存和锁定依赖位于 `dev-20261010-122449-c5c9eb47/evidence/` 与 `nuget-cache/`；未改全局 NuGet 配置、安装依赖或 `C:\SteerlineRuntime`。

真实联调使用统一生成器按时间优先命名创建 final package `C:\SteerlineDebug\dev-20261010-122449-c5c9eb47`；manifest `source_commit=c5c9eb470835c6ab28f7343b40b41f3648f086cf`、`source_dirty=false`。运行 `start.bat --simulate --no-pause`，方案冻结为 simulate。服务 instance `8ee753591f5548ca244a34fb1a67f374` / PID 12660；受控 worker PID 49340 是该服务的子进程，使用 simulate profile。WinUI PID 39452 连接并显示任务详情。没有 live 模型或外部 PR。

最终包的生成日志、manifest、WinUI 截图、身份/任务查询、受控 worker start/resume/stop 回执和清理结果位于 `C:\SteerlineDebug\dev-20261010-122449-c5c9eb47\evidence\`。stop receipt `applied`，服务和 worker 退出、discovery 删除、锁可独占打开；任务定义与 provider/Git 事实仍在，未调用 reset。该包中 UI5 的正常关闭时点没有核验，详见 `task44-ui-reopen.json`，并转列人工验收。

较早包 `dev-74326ae2-20261010-110711` 的 `task44-ui-integration.json` tree hash 转录错误已修正；原始文件和修正记录均保留在该包 evidence。最终候选 `c5c9eb470835c6ab28f7343b40b41f3648f086cf` 的 tree 为 `b4a7a265c03a7aaecc9a749ba3d5eebab36f7df0`。

## 待人工验收

| 项目 | 状态与参与安排 | 步骤和通过条件 | 阻塞范围 |
| --- | --- | --- | --- |
| UI2-M：中文 IME 和原生编辑 | 待验；参与人/真实 Windows IME 窗口待安排；最迟 2.1 签收前 | 对已选 Task 在中文 IME 组词期间按 Enter，确认只确认候选、不记录/发送意图；再验证粘贴多行、选区、撤销、Enter 换行、显式按钮只显示“未发送业务命令”，Task 1/2 草稿不串线。 | 阻塞 2.1 原生输入签收；不阻塞无关开发。 |
| UI4-M：键盘与常用缩放/窄窗 | 待验；参与人/真实 Windows 窗口待安排；最迟 2.1 签收前 | 用键盘 Tab/Shift+Tab 到达项目配置、任务列表、输入框和显式按钮；在常用显示缩放和窄窗口下确认任务/状态/输入/按钮仍可操作、可见名称准确。 | 阻塞 2.1 可达性签收；不阻塞无关开发。 |
| UI5-M：受控 worker 生命周期 | 待验；参与人/窗口待安排；最迟 2.1 签收前 | 在新建 simulate 包中记下 service instance/PID，启动受控 Task worker 并记录 worker PID/parent PID。WinUI 连接并查询任务后正常关闭窗口；核对没有 stop 请求、原 service/worker 仍运行且 instance/PID 不变，并用公开 identity/tasks GET 返回 200 复核。重开 UI 再查同一身份；最后通过包 console 停服并核对 applied receipt、进程退出和锁释放。 | 阻塞 2.1 生命周期签收；不阻塞 PR 交付或无关开发。 |

普通 Enter 的观察不替代中文 IME 组词验收。UI2-M、UI4-M 和 UI5 生命周期均待人工验收；按用户 2026-10-10 指示先交付 PR，三项最迟在 2.1 签收前完成，仅阻塞 2.1 签收，不阻塞 PR 交付或无关后续开发。

## 调试准备中的修复记录

保留的逐次证据按包对应如下，所有路径均相对 C:\SteerlineDebug：

- dev-550234ab-20261010-100945 与 dev-550234ab-dirty-20261010-101147：生成初始化失败，原始诊断在各自 evidence/init.log。
- dev-550234ab-dirty-20261010-101237：startup ACL 和首次 UI 连接准备事实在 evidence/task44-environment.json 与 task44-development-attempts.json。
- dev-fbde4163-dirty-20261010-101735：公开 API、nil slice、草稿串线及异步编辑覆盖记录分别在 evidence/task44-public-api-probe.json、task44-ui-detail-null-list-failure.json、task44-draft-race-failure.json、task44-editor-late-assignment-failure.json。
- dev-e78aa2ed-20261010-110217：连接时隐式目标串线记录在 evidence/task44-explicit-target-draft-failure.json。
- final package dev-74326ae2-20261010-110711：source_dirty=false 的最终构建、UI 生命周期、重开、清理和请求/身份事实在本页所列 evidence 文件。

首轮 550 基线调试包暴露两个准备缺陷：Windows CRLF/文本归一化使 fixture contract hash 与已提交内容不一致；包根继承 ACL 缺少服务收紧 discovery ACL 所需权限。最终实现包含按 Git commit blob 计算 pinned hash、fixture `.gitattributes` 固定 Markdown LF，以及生成包根目录仅授予当前 SID 的 ACL。此前 failed/临时包均保留在 `C:\SteerlineDebug\`，其中故障日志和根因见各包 `evidence/init.log`、`evidence/task44-environment.json`；最终 clean package 已真实启动、联调并清理通过。

UI 迭代中还保留了 helper Git 参数、#45 TaskPage `project_id` 实际表示仓库身份、nil blocker slices、延迟草稿保存、连接时隐式首任务选择等失败记录；修复后均在 final candidate 自动/真实窗口验收中复核。早期失败 JSON 见 final package 之外的 source-550/source-dirty 调试包 `evidence/task44-development-attempts.json`、`task44-public-api-probe.json`、`task44-ui-detail-null-list-failure.json`、`task44-draft-race-failure.json`、`task44-editor-late-assignment-failure.json` 和 `task44-connect-target-race-failure.json`。这些记录保留原失败，不改变 final package 的最终通过结论。

## 2026-10-10 独立复审第一轮局部修复

复审指出最后窗口的 `Closed` 异步保存可能晚于应用退出。修复 source commit `f1846b9bd628f43603658d265db0364e1cb87750`（tree `23c940561d2a86f4254a5fb2a19874f3a3c53d93`）改为在 `AppWindow.Closing` 取消实际关闭，禁用编辑区域，等待草稿保存成功再关闭。重复关闭不重复保存；保存失败保留窗口、草稿和客户端观察连接，显示失败与重试提示，不发送服务 stop。

本轮仅补 UI2-A 草稿生命周期定向验证：`dotnet test ui/SteerLine.Windows.Tests/SteerLine.Windows.Tests.csproj -c Debug --no-restore` exit 0，重新编译并运行 19 passed / 0 failed / 0 skipped；`dotnet build ui/SteerLine.Windows/SteerLine.Windows.csproj -c Debug --no-restore` exit 0，0 warnings / 0 errors。新增用例覆盖延迟保存前不能关闭、重复关闭、失败保留草稿/连接与可见提示、成功重试。实际 WinUI 关闭事件的接线经真实工程编译，尚未执行本轮真实窗口关闭观察。

日志与修复报告位于 `C:/Users/lmas1/.codex/task-state/roadmap-runner/roadmap-steerline-20261008-01a11a74/fix44-round1-evidence/` 和同级 `fix44-round1.md`。前文 c5c9 包的真实窗口/UI3 与历史联调证据保留，只支持其原 source；不据此声称本轮 source 已完成真实 UI 验收。UI2-M、UI4-M、UI5-M 仍人工待验且最迟 2.1 签收，UI5 正常关闭 service/worker 连续性未经本轮验证。未运行 Full、Go L1、live 模型；未创建后端或调试包、未触安装环境。需协调 push 与新独立终审后再结算交付。


## 2026-10-10 真实合并与交付结算

2026-10-10，#44 已经实现、独立复审、局部修复及新的独立终审，PR #273 于 UTC 05:54:56 真实合并；head `b3b52c24fba2a96a1746d3249756518dbf1f9d3b`、merge `f04fdf089a58723909279d44c497377427b868dd`。关闭草稿竞态由 `/root/fix44_round1` 修复于 source `f1846b9bd628f43603658d265db0364e1cb87750`，`/root/review44_round2` 终审无新确认阻塞缺陷；[实际审核记录](https://github.com/erranliu/SteerLine/pull/273#issuecomment-6094396874)。新编译定向测试 19/0/0、WinUI 构建 0 warning/error，独立复跑现有编译测试 19/0/0；不是新 source 真实窗口验证。历史 `c5c9eb47` 窗口、公共查询与清理证据保留原来源。UI2-M/UI4-M/UI5-M 按用户指示待人工、最迟 2.1 签收，登记 `passed_with_manual_pending`，未宣称全部验收完成。

人工安排来源：原执行聊天 `01a12391-bf38-7f41-8b15-44ed43cddf19` 真人消息 `01a1240c-879b-7a60-9608-174e3c08d28d`：“这边在验收什么，效率太低了，可以考虑留给人工验收，先交付pr”。仅调整当前 UI5 观察时点；保存原条件、未验事实与 2.1 门槛，不豁免已知缺陷、清理失败或未知副作用。参与人和窗口待安排，具体步骤见本页人工验收表。
