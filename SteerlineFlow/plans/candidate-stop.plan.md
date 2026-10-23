# Plan：候选停止参与

依据 [Spec](../spec/candidate-stop.spec.md) 实现当前普通开发，不进入 roadmap。验收归属 [candidate-stop](../testing/acceptance/candidate-stop.md)。

## 实施

1. 复用 WorkflowRun 状态定义参与资格和统一停止声明，保存停止时间、原因、操作者；启动失败复用该声明。以现有 SQLite 候选事务受理人工停止，原子保存 Command、停止事实及事件，重放原请求，保护最后一个参与候选。
2. 接入 `stop_candidate` 能力、严格控制协议、HTTP 映射及客户端。停止使用候选版本，不检查 Task 状态或派发模式。
3. 移除 retired 恢复/完成及外发例外。复用事件和回执事实写入，停止后不推进业务；准备、派发、反馈、恢复和交付均检查参与资格。复用服务 actor 串行处理停止和派发，在途结果只留事实；重启不主动恢复终态候选。
4. 控制台 F9 使用现有确认区及请求跟踪，固定目标并保存请求身份；成功刷新后按权威快照选择剩余候选，历史只读，编号不变。同步产品文档。
5. 补充必要受控用例，更新失效的旧退出契约测试及验收证据；既有 E2E 驱动替换旧放弃请求与字段，只做编译校验，不恢复已退役的真实候选场景。

## 测试计划

| 验收项 | 级别 / 场景与命令 | 环境 / 时点 | 通过条件 / 证据 |
| --- | --- | --- | --- |
| [CST-A1、A2](../testing/acceptance/candidate-stop.md) | 定向 Go：`go run ./cmd/test-feedback -- ./internal/execution -run TestCandidateStop -count=1`；必要事务屏障在同组用例实现 | 隔离 SQLite 与受控 Service；服务端完成后 | 幂等与回滚、任务状态独立、并发最后一个保护、名额和编号成立；报告登记验收文档 |
| [CST-A3、A4](../testing/acceptance/candidate-stop.md) | 定向 Go：同上，并选择 `TestLifecycleEffectForwardsWorkspaceCreationCheckpoint`、`TestReleasedWorkerStartConvergesAcrossRestart`、`TestSimplifiedCleanupRetainsFrozenDeliveryEvidence`、`TestWorkerShutdownDrainsCommittedDeliveryAck`、`TestExecutionStopIntentAndUnknownReceiptUseOriginalBinding` | 隔离服务、假适配器调用计数、重启夹具；门禁收敛后 | 无停止引起的新外部动作，原在途事实及核验失败前的回执保留，无复活或重启恢复；调用记录与报告登记验收文档 |
| [CST-A5](../testing/acceptance/candidate-stop.md) | 定向 Go：`go run ./cmd/test-feedback -- ./internal/console -run 'TestCandidateStop|TestCandidateConfirmationSelectsIndependentPendingHistory|TestCandidatePendingStartSurvivesJournalReopen' -count=1`；HTTP `TestCandidateStopHTTPContract` | 控制台模型及 HTTP 隔离夹具；接线完成后 | 固定身份、确认/取消、最后一个保护、原请求查询、选择与内容清空、历史只读及停止文案安全；报告登记验收文档 |
| [CST-A6](../testing/acceptance/candidate-stop.md) | 定向 Go：CandidateStop 事务/交付竞争及既有基线/PR 相关用例 | SQLite 与受控 Git/PR 夹具；完整接线后 | 交付不能使用停止候选，保留历史/共同基线及原 PR 核验规则；报告登记验收文档 |

最终执行相关包编译和架构边界检查；不默认跑 Full、L2/L3、真实模型或真实 PR，此功能可由受控事务、适配器和控制台夹具验证。具体选中的测试名称、实际命令及报告在验收文档登记，Plan 不维护实时结果。CST-M1 人工复验可后置，调试须新建 SteerlineTest 独立包，窗口与结果登记专属验收文档。
