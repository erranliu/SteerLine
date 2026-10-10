# 任务 #44 验收记录：WinUI 基座与原生输入

唯一归属 #44；固定 [Spec](../spec/phase2-winui-foundation.spec.md)、[Plan](../spec/phase2-winui-foundation.plan.md) 和 [二期共同约束](../spec/phase2-constraints.md) 均取自 commit `550234ab0fb0aa74a3676ff09dde6e4bd4325295`。硬前置 #45 已交付并包含在代码起点；未调用 SteerLine 中心准入或创建外部 Task/DAG。

当前实现候选为 `74326ae26c4423305e7d708a2c3acc55d8d38e5d`（tree `2f3855e4eb4554c1e8a90e94091eb4be631d6d4e`），从最新 `origin/main` commit `fbde4163a42639d5570dbe718b32cd42e95dc676` 集成开始；原工作树基线为 `550234ab0fb0aa74a3676ff09dde6e4bd4325295`。实现 PR 尚待创建并独立复审。本次结果不将后置人工项记为通过。

## 自动验收

| 项目 | 结果 | 证据与范围 |
| --- | --- | --- |
| UI1 | 已通过 | Windows 11 x64 上以固定 .NET SDK 8.0.416 / Windows App SDK 1.8.260921001 构建 self-contained `win-x64` 应用，并启动真实 WinUI 窗口。系统为 Windows 11 Home 10.0.26200；项目目标 `net8.0-windows10.0.19041.0`。restore/build 日志及 exe hash 见 final package 的 `evidence/dotnet-restore-app.log`、`dotnet-build-ui.log`、`task44-environment.json`。 |
| UI2-A | 已通过 | 10 个定向测试通过，0 失败、0 跳过；覆盖无目标时禁用输入、显式本地意图不调用业务命令、目标草稿隔离、异步保存期间切目标和迟到响应保护。真实窗口 UIA 另验证 Task 1/2 切换期间编辑框立即绑定新目标，详情返回不覆盖输入，回切后恢复原草稿。 |
| UI3 | 已通过 | final package 的受控 Windows 服务和 SQLite 中 Task 1/2 的状态均为 `open`。WinUI 客户端核验 `steerline-client/v1`、仓库身份、哈希 project ID、实例、PID/创建时间、可执行文件、loopback 地址及当前 SID 的 discovery/credentials ACL；公开 `/v1/identity` 与任务/详情查询均返回 HTTP 200，客户端显示两项真实任务及实际阻塞。任务页 `project_id` 按实际 DTO 表达仓库身份路径，已按公开响应核验。 |
| UI4-A | 已通过 | 10 个定向测试中可访问名称、输入状态断言通过；UIA 在实际 WinUI 窗口读取到配置、项目、任务列表、状态、多行输入和本地意图控件名称。无任务目标时输入框和意图按钮均禁用。 |
| UI5 | 已通过 | 两次关闭实际窗口均通过 `CloseMainWindow` 正常退出；关闭后服务 discovery 的实例 `ac600258a76472e69be2304f67b1630d`、PID 31180 保持 ready，HTTP identity/tasks 查询仍为 200，Task 1/2 状态不变。第二次启动窗口后重新读取 identity 与任务列表，仍为同一服务实例；两个任务的本地草稿均恢复。UI 没有 stop 或其他业务命令路由。 |

最终代码候选测试命令：

```powershell
dotnet restore ui/SteerLine.Windows/SteerLine.Windows.csproj
dotnet test ui/SteerLine.Windows.Tests/SteerLine.Windows.Tests.csproj -c Debug
dotnet build ui/SteerLine.Windows/SteerLine.Windows.csproj -c Debug
```

结果为 restore exit 0；tests 10 passed / 0 failed / 0 skipped；build exit 0 / 0 warnings / 0 errors。实际 exe 为 `ui/SteerLine.Windows/bin/Debug/net8.0-windows10.0.19041.0/win-x64/SteerLine.Windows.exe`，SHA-256 `4fc7f44ec2d0a018f5137d5313bd8766df2dc40abcc1a83c8abc8a876bffcf96`。NuGet 缓存隔离在本次包的 `nuget-cache/`；精确 transitive 依赖保存在三个 `packages.lock.json` 中，Windows SDK BuildTools 为 `10.0.26100.4654`。未改用户全局 NuGet 配置、安装依赖或 `C:\SteerlineRuntime`。

真实联调使用统一生成器创建的 final package `C:\SteerlineDebug\dev-74326ae2-20261010-110711`，manifest `source_commit=74326ae26c4423305e7d708a2c3acc55d8d38e5d`、`source_dirty=false`、状态 `ready`；实际运行命令 `start.bat --simulate --no-pause`，首次启动后冻结为 simulate。配置与仓库均位于该包，SQLite 路径是其 `home\state\SteerlineTest-ccbb16af2c06\steerline.db`；受控 Git helper、服务、worker（未启动）均绑定此包。无 live 模型、真实 PR 或用户凭证请求。

原始日志、manifest 对账、UIA/HTTP 查询、窗口生命周期和清理回执位于该包 `evidence/`，包括 `task44-environment.json`、`task44-ui-integration.json`、`task44-ui-reopen.json` 与 `task44-cleanup.json`。UI 关闭后才按生成器持有的 console 生命周期发起最终服务清理；原 stop receipt 为 `applied`，进程退出且锁释放。测试包保留任务定义、契约和 Git/provider 事实，未执行重置。

## 待人工验收

| 项目 | 状态与参与安排 | 步骤和通过条件 | 阻塞范围 |
| --- | --- | --- | --- |
| UI2-M：中文 IME 和原生编辑 | 待验；参与人/真实 Windows IME 窗口待安排；最迟 2.1 签收前 | 对已选 Task 在中文 IME 组词期间按 Enter，确认只确认候选、不记录/发送意图；再验证粘贴多行、选区、撤销、Enter 换行、显式按钮只显示“未发送业务命令”，Task 1/2 草稿不串线。 | 阻塞 2.1 原生输入签收；不阻塞无关开发。 |
| UI4-M：键盘与常用缩放/窄窗 | 待验；参与人/真实 Windows 窗口待安排；最迟 2.1 签收前 | 用键盘 Tab/Shift+Tab 到达项目配置、任务列表、输入框和显式按钮；在常用显示缩放和窄窗口下确认任务/状态/输入/按钮仍可操作、可见名称准确。 | 阻塞 2.1 可达性签收；不阻塞无关开发。 |

普通 Enter 的实际窗口测试确认文本框产生 CR 换行且意图状态不变；这不替代中文 IME 组词验收。人工项仍为待验，不作通过声明。阶段验收引用本记录，不扩大 #44 范围。

## 调试准备中的修复记录

保留的逐次证据按包对应如下，所有路径均相对 C:\SteerlineDebug：

- dev-550234ab-20261010-100945 与 dev-550234ab-dirty-20261010-101147：生成初始化失败，原始诊断在各自 evidence/init.log。
- dev-550234ab-dirty-20261010-101237：startup ACL 和首次 UI 连接准备事实在 evidence/task44-environment.json 与 task44-development-attempts.json。
- dev-fbde4163-dirty-20261010-101735：公开 API、nil slice、草稿串线及异步编辑覆盖记录分别在 evidence/task44-public-api-probe.json、task44-ui-detail-null-list-failure.json、task44-draft-race-failure.json、task44-editor-late-assignment-failure.json。
- dev-e78aa2ed-20261010-110217：连接时隐式目标串线记录在 evidence/task44-explicit-target-draft-failure.json。
- final package dev-74326ae2-20261010-110711：source_dirty=false 的最终构建、UI 生命周期、重开、清理和请求/身份事实在本页所列 evidence 文件。

首轮 550 基线调试包暴露两个准备缺陷：Windows CRLF/文本归一化使 fixture contract hash 与已提交内容不一致；包根继承 ACL 缺少服务收紧 discovery ACL 所需权限。最终实现包含按 Git commit blob 计算 pinned hash、fixture `.gitattributes` 固定 Markdown LF，以及生成包根目录仅授予当前 SID 的 ACL。此前 failed/临时包均保留在 `C:\SteerlineDebug\`，其中故障日志和根因见各包 `evidence/init.log`、`evidence/task44-environment.json`；最终 clean package 已真实启动、联调并清理通过。

UI 迭代中还保留了 helper Git 参数、#45 TaskPage `project_id` 实际表示仓库身份、nil blocker slices、延迟草稿保存、连接时隐式首任务选择等失败记录；修复后均在 final candidate 自动/真实窗口验收中复核。早期失败 JSON 见 final package 之外的 source-550/source-dirty 调试包 `evidence/task44-development-attempts.json`、`task44-public-api-probe.json`、`task44-ui-detail-null-list-failure.json`、`task44-draft-race-failure.json`、`task44-editor-late-assignment-failure.json` 和 `task44-connect-target-race-failure.json`。这些记录保留原失败，不改变 final package 的最终通过结论。
