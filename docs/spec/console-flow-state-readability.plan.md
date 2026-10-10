# Plan：任务列表与流程状态可读性

绑定 [Spec](console-flow-state-readability.spec.md)，结果登记[专属验收](../acceptance/console-flow-state-readability.md)。直接在当前聊天执行。

1. 列表固定单行/三列，使用原状态及阻塞事实生成提示，增加 8 阻塞筛选，概览对齐字段并补前置/启动原原因；保留原选择和操作规则。
2. Activity 持久化结构不变，默认投影仅 step_id/status，拆分同时间双变化、附当前流程及短原因，技术字段和其他活动进入 d 原始详情。
3. SQLite RecentActivity 同事务合并五条可读变化、五条最近原始变化及十条其他活动，按原预算优先可读变化；UI 接收最多十条原始变化并限制五条可见消息。
4. 定向回归事实阻塞、未知/缓存、中文对齐/长标题/窄屏/大量任务、双变化拆分、无变化及技术噪声，检查原字段、诊断和目标绑定；同步当前产品文档。

## 测试计划

环境为 Windows 现有隔离 console/SQLite 夹具，命令日志及渲染样本写入新的系统临时证据目录，结果按 [CRD-A1–A4](../acceptance/console-flow-state-readability.md) 登记。

| 自动项 | 级别/场景及命令 | 时点、通过条件 | 证据 |
| --- | --- | --- | --- |
| CRD-A1 | 定向 console：`go test ./internal/console -count=1 -timeout=2m`，TestTaskListUsesBlockerFactsAndAlignedSingleRows、TestWorkerStatusSingleRowsKeepSelectionVisible、既有任务/离线/焦点回归 | 步骤 1 后；open 不误判，真实阻塞/筛选可读，活动执行不受旧启动阻塞误导，固定列/行高/原值/选择保持 | console.log、acceptance |
| CRD-A2 | 同一 console 命令，TestFlowStateTimelineSeparatesMessagesAndHidesTechnicalNoise、TestFlowStateTimelineCountsVisibleMessagesAndKeepsCurrentSnapshot、TestFlowStateReadableSampleAcrossWindowSizes；`go test ./internal/console -run '^TestFlowStateReadableSample' -count=1 -v` | 步骤 2 后；一条一种变化，五条消息，当前状态/未知原值保留，技术字段隐藏且详情完整，160/120/80 列样本无溢出，问题/回执/提交资格不变 | console.log、render.log、acceptance |
| CRD-A3 | 定向 SQLite/query：`go test ./internal/storage/sqlite ./internal/product/query -count=1 -timeout=2m`，TestPS1RecentActivityReadsLatestTenFromExactAttempt、既有 TM05Activity 预算/绑定回归 | 步骤 3 后；工具和连续二十条技术变化不挤掉最近五条可读变化，原身份、顺序、原始记录和预算保持 | query.log、acceptance |
| CRD-A4 | `go vet ./internal/console ./internal/storage/sqlite ./internal/product/query`；`go build -o <临时目录>/steerline.exe ./cmd/steerline`；`python scripts/check_doc_links.py`、`git diff --check` | 交付前；静态检查、构建、文档导航和格式无错误 | static.log、acceptance |

不选择 Full、L2/L3/live：本次只改变既有展示和有界读取，状态生产与三轮执行链路不变；现有夹具已能验证，不需要新驱动或人工窗口。分级入口见 [tests/e2e/README](../../tests/e2e/README.md)。
