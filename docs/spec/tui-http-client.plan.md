# Plan：TUI 经 HTTP 接入 Go 后端

任务 ID：91；任务名称：TUI 经 HTTP 接入 Go 后端；目标：`TUI-HTTP-GO`。按用户直接授权在当前聊天实施；无未完成硬前置，复用 #45 与现有 TUI，一个实现 PR。范围见 [Spec](tui-http-client.spec.md)，结果归[专属验收](../acceptance/tui-http-client.md)。

## 实施与所有权

1. 从 console/interfaces、native_session、原应用组装逐项固定功能映射和 HTTP 路由/DTO/action；先补恢复核验、原会话查看与停服缺口，再换客户端，保留键位/布局。
2. 分开纯 HTTP 契约、客户端和 handler（拟采用 internal/product/httpcontract 与 httpclient，只为实际接口建包）。客户端不依赖 app/host、execution、query、sqlite 或 httpapi handler；product/api 只提供纯值。共享协议、服务接线和 console 构造由本任务统一修改，服务端内部适配器保留。
3. 实现 TUI HTTP 消费、原请求保存/等待/回读与身份换代，分开本机连接发现/凭证/必要 OS 观察与通用 HTTP 调用。离线能进入诊断/草稿，删除业务库及宿主文件回退，不初始化业务现场。
4. 公开恢复只读查询、类型化原会话查看，复用原冻结身份核验。落实退出前 HTTP 回执和本机精确退出/锁观察，无证据明确未核实，不建第二查询服务或替代会话。
5. 删除 ui/SteerLine.Windows、ui/SteerLine.Windows.Core、ui/SteerLine.Windows.Tests 及仅供它们使用的 global.json 等配置/入口；先核对专属归属，保留历史记录，不改安装目录。
6. 补下面新增测试/驱动，再执行全部自动验收与最小回归。记录源码/构建/命令/结果/证据到 acceptance，同步 execution-map；TH-M1 可后置，自动失败和功能缺失必须修复。

## 测试能力补齐

下列入口由 #91 补齐；实际运行结果只登记专属验收：

- `TestTUIHTTPBoundary`（internal/app）：检查真实 console 构造、直接/传递客户端闭包及禁止文件访问；共用 CLI 的服务端闭包不算客户端违规，也不能掩盖客户端依赖。
- `TestTUIHTTPContract`（internal/product/httpclient）：httptest/受控客户端核验身份、DTO、原请求、版本/故障，断言正式处理次数。
- `TestTUIHTTPInteraction`（internal/console）：通过真实 HTTP 适配驱动任务/候选、输入/final、迟到响应和切项目，不能仅测试 API 请求格式。
- `TestDecisionEligibilityHTTPDoesNotBlockUI`（internal/console）：阻塞中的资格 HTTP 不阻塞 Update、Escape 或退出，迟到/原版本变化后不打开答案。`TestTUIHTTPStopReopenUsesOriginalProcess`（httpclient/Windows）：真实隔离子进程下，原 applied 在 UI 重开及当前连接换代后仍按保存的原进程代际/锁核验；进程仍活着时不得成功。Routes 的 recent activity 子场景核对活动超过首批后的刷新和错绑定拒绝。
- `TestPendingStopVerificationSurvivesJournalReopen`（console）：实际保存/释放窗口日志后重开，待核实停服、原 outstanding 与草稿保留且日志仍可写。Contract 的关闭场景阻塞 snapshot/资格 HTTP，验证关闭先取消自己的请求。将既有 CM72 与 CD66 消费夹具改为 HTTP 在线/离线，保留 CLI 服务端读取与只读数据库版本断言。
- `TestAPIStopPreservesDisplayedRevision`（internal/product/httpapi）：使用正式 host 请求登记入口核对 TUI 携带的停服 revision 保留；提交、回读和重复请求只能登记一次，原实例停服不新增 revision 拒绝条件。与原 `TestAPIMappedControlPayloadBinding` 的异 payload/revision 负例共同验证。
- `scripts/test-tui-http.ps1 -DebugRoot <root> -OutDir <fresh-dir>`：新增单一受控驱动，先调用现有 new-debug-release.ps1 生成独立 SteerlineTest 包，再显式 `--simulate` 启动/冻结方案。真实服务配受控 worker/PR provider，使用本次专属分支，不创建远端真实 PR或发模型请求。驱动实际 TUI 模型/HTTP/Go 接线，验证开始、关闭后 worker 继续、重开/重复、规范 final 与正常停服/清理。丢响应、旧实例/版本、丢停服回执与恢复/会话错绑定由上述定向测试及服务端 TestTUIHTTPRoutes 注入并断言。

真实原生窗口操作归 TH-M1；受控打开器只验证目标绑定和无新业务轮次，不推定真机支持。新增入口必须非零枚举、实际断言，零匹配/skip 不算通过；过滤名如调整须在交付前同步 Plan/acceptance。

## 测试计划

全部项链接[TH-A1–A6、TH-M1](../acceptance/tui-http-client.md)；下表映射全部自动验收，原始输出使用本次新的 evidence 目录，结果登记到对应 ID。

| 验收项 / 回归 | 级别/组或场景 | 环境与命令 | 时点及通过条件 | 证据位置 |
| --- | --- | --- | --- | --- |
| TH-A1 | 定向 Go / 新增 Boundary | Windows 源码；`go test ./internal/app -run '^TestTUIHTTPBoundary$' -count=1 -v` | 接线后；非零检查客户端闭包/文件访问，无业务直连 | `<evidence>/boundary.log`；acceptance TH-A1 |
| TH-A2–A5 | 定向 Go / 新增 Contract、Interaction | Windows/httptest；`go test ./internal/product/httpclient -run '^TestTUIHTTPContract$' -count=1 -v`；`go test ./internal/console -run '^TestTUIHTTPInteraction$' -count=1 -v` | 接口/消费后；身份/内容/冲突/原请求/停服未知/关闭断言通过 | `<evidence>/contract.log`、`interaction.log`；对应 ID |
| TH-A4；TH-A2 历史 | 定向 Go / 新增 Routes | Windows/httptest；`go test ./internal/product/httpapi -run '^TestTUIHTTPRoutes$' -count=1 -v` | 路由接线后；恢复只读、错绑定/重复/unknown 查看与跨 Attempt 历史通过 | `<evidence>/routes.log`；TH-A2/TH-A4 |
| TH-A3、TH-A5 停服原请求 | 定向 Go / 正式 host 登记回归 | Windows/httptest；`go test ./internal/product/httpapi -run '^(TestAPIStopPreservesDisplayedRevision\|TestAPIMappedControlPayloadBinding\|TestAPIAuthoritativeControlBinding)$' -count=1 -v` | 字段映射修正后；原停服 revision 完整保留、重复不重登记、异内容不匹配 | `<evidence>/stop-revision-regression.log`；TH-A3/TH-A5 |
| TH-A3、TH-A5 迟到资格/重开停服 | 定向 Go / HTTP 与隔离子进程 | Windows；`go test ./internal/console -run '^TestDecisionEligibilityHTTPDoesNotBlockUI$' -count=1 -v`；`go test ./internal/product/httpclient -run '^TestTUIHTTPStopReopenUsesOriginalProcess$' -count=1 -v` | 独立复审修复后；界面可取消/退出，迟到不串线；原进程退出前未核实，退出/锁释放后可由原回执完成 | `<evidence>/review-regression.log`；TH-A3/TH-A5 |
| TH-A1、TH-A5 既有消费与日志恢复 | 定向 Go / CM72、CD66、窗口重开 | 隔离 Go 夹具；`go test ./cmd/steerline -run '^TestNativeTaskBatchCLIValidatesCreatesAndDiscardsFixedTaskIDs$' -count=1 -v`；`go test ./tests/integration -run '^TestCD66ExplicitCatalogAndEntrypoints/offline_persisted_query_and_console_survive_missing_support$' -count=1 -v`；`go test ./internal/console -run '^TestPendingStopVerificationSurvivesJournalReopen$' -count=1 -v` | 复审修复后；CM72 使用实际 HTTP 服务/客户端查询原 CLI 回执，CD66 离线只留连接定位且数据库未变，待核实日志能恢复 | `<evidence>/cli-http-fixture.log`、`offline-http-fixture.log`、`pending-stop-reopen.log`；TH-A1/TH-A5 |
| TH-A2–A5 | 独立调试 / 新增 `tui-http.local`（专属驱动，非现有 L3 注册项） | Windows/Go/Git；`powershell -NoProfile -File scripts/test-tui-http.ps1 -DebugRoot C:\SteerlineDebug -OutDir <fresh-evidence>` | 定向通过后；真实接线、处理次数/停止证据/重连/清理完整通过 | `<fresh-evidence>/summary.json` 与包 evidence；对应 ID |
| TH-A6 | 清理/构建 | Windows；`go run ./cmd/test-feedback -build-only -out <fresh-build-evidence>`；删除清单及下行文档命令 | 删除后；无 WinUI 专属工程/入口，原 Go 构建和导航通过 | build summary、删除清单；TH-A6 |
| TH-A6；本次规划 | 文档静态 | `python scripts/check_doc_links.py`；`python scripts/roadmap_task_id.py check`；`git diff --check` | 规划及实施交付前；导航/身份/格式通过，非业务验收 | 规划见 roadmap-planning；实现见 TH-A6 |
| 必要回归，非替代验收 | 定向 Go、L1 | `go test ./internal/product/httpapi ./internal/console -count=1`；`go run ./cmd/test-feedback -level=L1 -out <fresh-l1-evidence>` | 最终 Go 接线后一次；非零完整通过、核对 skip 范围 | regression.log、L1 summary；acceptance 回归栏 |
| TH-M1 | 人工 Windows 终端 | 新独立包、原支持平台；原键位/中文输入/候选/final/原会话查看，参与窗口待安排 | 自动通过后可后置，最迟任务签收前；无误提交/串线，入口可用 | 对应 ID、精选操作证据 |

不选 Full：有界客户端/传输改造。L2/既有 L3/live 不默认运行：现有场景不证明新 TUI 接线，专属受控驱动覆盖本任务；不重跑无关平台模型矩阵。若实际改动产生执行语义风险，按影响补最小场景并同步计划。临时/AI调试先生成 C:\SteerlineDebug 包，不使用、修改或重置 C:\SteerlineRuntime。

执行基准固定为 `83a6cc4a7321994a55e5a9ae0f308927eb9fb1b3` 的本 Spec/Plan 路径。用户直接授权实施，不启动 roadmap-runner；结果与代码/构建版本只登记 acceptance/execution-map，Plan 不维护实时结果。
