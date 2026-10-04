# #77 产品所有权与顺序集成清单

实际开工基线：main `621707d534b62e4b7df015463c53949715ba8be3`；分支 `codex/product-console-77`。
#74 PR214、#38 PR208/PR213、#75 PR215、#39 PR216 已合入。没有旧暂停条件。

## 专属文件

| 原路径 / 责任 | 唯一目标 | 状态 |
| --- | --- | --- |
| clientcontracts 的 Project/Observation/Recovery 宽公开值 | product/api 页面摘要、目标、回执 | 字段盘点与独立 DTO 实施中 |
| steerlineclient 的唯一 Go client | product/client，注入读取/命令/连接端口 | 待真实消费者切换 |
| steerline/console 所有生产与测试文件 | console，只有 api 与中性 UI fixture | 待真实消费者切换 |
| query 的读取/游标/正文规则 | product/query | 待真实读取接口顺序集成 |
| taskview 的纯图投影 | product/query/taskgraph，页面值归 api | 待切换 |
| activityview 的纯页面渲染 | product/query/activity | 待切换 |
| projection 的 actor Runtime/Run 重建 | execution 唯一所有者（#76） | #77 不迁 actor 私有状态或复制重建规则 |

公开摘要去掉：完整 TaskRecord/TaskContract、WorkflowRunRecord、ExecutionRecord、StepAttemptRecord、DecisionRecord、OperationRecord、固定执行配置及 implementation envelope；Config、SQL、Go 句柄不进入 api。
保留：项目、Task/图/DB 修订、精确 WorkflowRun/Execution/Attempt/session/turn、方案 ID/digest/显示字段、决策内容与历史、blocker、PR/资源显示事实、原 request/实例/版本及真实命令结果。

## 共享接线具体清单（须协调指定顺序）

1. `internal/steerlineapp/client.go`：`ClientResources/clientResources`；OpenReader、OpenCommands、Connect、Observe、ProbeProcess、Control、Decision、TaskSnapshot、ResolveStartTask、InspectRecovery、DecisionAvailability；`clientReader` 嵌入的 query/ConversationService 和 Domain/Command/ActivityEvents 方法；项目方案显示映射。#77 拥有产品工厂，#76 先迁 Config/执行 namespace 后本任务定向切换实际签名。
2. `internal/steerlineapp/client_command_wiring.go`、`client_discard_commands.go`：仅产品命令适配的 DTO/端口签名；任务与执行写者仍唯一注入。
3. `internal/steerlineapp/console.go`：api.ConsoleClient 与 console 实际构造；`cmd/steerline/main.go`：client/API/graph 查询 import、命令/回执输出；共同 Config/执行构造由 #76。
4. `cmd/test-feedback/scoped.go`：迁入产品包、父/子测试精确选择；不重写 #76 执行测试组。
5. `internal/steerline/persistence/connections.go` 及 QueryReader 产品接口、`internal/steerline/sqlite` 产品读取实现：同一 DB 事务输入快照及 api 页面输出，保留唯一 SQL；不添加第二 SQL 绕过。
6. `internal/steerlineintegration` 的共享 AST/import 守卫和真实产品构造测试：#76 namespace 合入后按实际消费者切换；扫描全部 OS/tag 及测试，不放宽反例。

#79 读取需求：同事务项目元数据/任务图/完整候选编号历史/精确 execution/attempt/decision 页面输入；会话 events/body/raw/capabilities 固定身份读取；command 原请求与结果读取；DB 身份/修订用于缓存与游标。读取端口不引用 product/query，纯投影不经端口回到自身。

已识别额外定义冲突：`internal/product/api/observations.go` 的 `JournalCursor` 含 Path/FileID/Generation/Offset/ScanOffset/Pending/Dropping/DroppingStart/SkipUntilNewline/AnchorOffset/AnchorLength/AnchorHash/HeaderLength/HeaderHash/Initialized。真实消费者为 `conversation_position.go` 的 journalCursorOf、`conversation_replay.go` 的 replay、`journal_reader.go` 的 JournalBatch/ReadJournalBatch/journalAnchorsMatch/captureJournalAnchors/captureLineAnchor 及 journal_reader_test；全部属于执行日志采集，公开 API 不应携带这些私有扫描状态。已交协调确定唯一执行侧归属及顺序；本任务不抢改 #76 的模型和 replay 消费者。

控制台中的终端宽度/颜色/截断是 UI 格式化，迁入 console 的独立展示函数；规范活动来源和身份/顺序投影归 product/query。执行 audit 的旧 activityview 消费者在 #76 真实 namespace 后核对，不能让 actor 反向依赖产品 query。
