# Plan：候选流程状态字段命名收敛

普通开发，按 [Spec](state-naming.spec.md) 实施。专属验收 [SN-A1–A3](../acceptance/state-naming.md)，不修改 roadmap。

1. 改名 RunView、CandidateView、CandidateSummary、WorkflowSummary、worker_state from/to 及转换/控制台读取；保留领域步骤及其他状态字段。恢复检查删除派生 DeliveryStatus，直接带流程阶段/状态。
2. 删除 FinishTaskStatus 和 finishStatus，逐个替换筛选、答题、恢复、候选动作资格和终态判断。待合并必须匹配阶段与状态；保留异步问题及原版本/身份校验。
3. 补新格式序列化与原生事件→SQLite→查询的字段断言，旧活动缺新目标字段或 from/to 字段类型损坏时产生诊断。补候选终态、非合并外部等待、异步问答、恢复和历史失败的最小回归。受控发布夹具匹配当前模板发布前缀，使 HTTP 三轮查询场景可执行；不改生产发布流程。
4. 更新当前状态契约文档和导航，执行下列测试，证据及审计修复摘要登记专属验收。

## 测试计划

| 验收项 | 级别/场景与命令 | 环境/时点 | 通过条件/证据 |
| --- | --- | --- | --- |
| SN-A1 | 定向 Go：`go run ./cmd/test-feedback -- ./internal/execution/model ./internal/execution/reconstruction ./internal/product/api ./internal/product/query ./internal/product/client -count=1`；新增状态命名序列化/活动验证场景 | 新建隔离模型/查询夹具，字段改名完成后 | 新字段端到端一致，旧字段不输出，原领域步骤保留；默认临时报告路径登记验收 |
| SN-A1、SN-A2 | 定向 Go：`go run ./cmd/test-feedback -- ./internal/execution -run '^(TestWorkerStateActivityCommitAndRollback|TestMC5|TestWorkerFeedbackPhaseCommitIsDurableAndRejectsStaleActor|TestAppServerStartReplayPreservesNativeSavedAnswer|TestAppServerLateReplyCannotModifyTerminalTask|TestRuntimeViewCannotWriteBusinessState|TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace)$' -count=1`；另定向运行 TestAppServerStartReplayPreservesNativeSavedAnswer、TestAppServerLateReplyCannotModifyTerminalTask、TestCommandIdempotencyAndDecisionVersionArbitration | 现有隔离 SQLite/worker 夹具，删除派生字段后 | 事件事务/回滚、待答/反馈/关闭候选与原身份保持；默认临时日志登记验收 |
| SN-A2、SN-A3 | 定向 Go：`go run ./cmd/test-feedback -- ./internal/console -count=1`；`go run ./cmd/test-feedback -- ./internal/app ./tests/integration -run 'TestAsyncDecisionClientPreservesInteraction|TestBoundAnswerRejectsAttemptOrRecoveryRetarget|TestAPIWorkerStageQueryProjection' -count=1`；SQLite RecentActivity 预算定向 TestPS1RecentActivityReadsLatestTenFromExactAttempt | 隔离渲染/交互夹具，完整实现后 | 待合并组合、终态/异常/异步问答、标签与筛选动作正确，宽窄布局及其他状态保持；默认临时日志登记验收 |
| SN-A3 | `go test ./... -run '^$'`；`go vet` 受影响生产包；`python scripts/check_doc_links.py`；`git diff --check` | 编译及静态检查，交付前；不执行全量测试 | 所有普通 Go 测试编译，定向 vet/文档/格式通过；临时日志与验收摘要 |

新增场景为 TestStateNamingPublicWorkflowFields、TestStateNamingDecodeRequiresCurrentFields、TestStateNamingActivityRejectsMissingWorkflowFacts、TestStateNamingMergeWaitUsesStageAndTerminalFacts、TestStateNamingClosedInputRemainsHistory、TestStateNamingAttentionUsesTheAbnormalAxis、TestStateNamingActivityReportsMalformedTransitionFields、TestStateNamingTerminalInputDoesNotOccupySlot；既有事件事务场景增加新字段的真实查询断言。不选择 L2/L3/full：本次不改领域持久化或流程推进，隔离投影/事件/交互接线足以覆盖范围；不启动真实模型、安装环境或真实 PR。无独立人工项。
