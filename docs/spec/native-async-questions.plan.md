# 原生异步问答实施 Plan

对应 [Spec](native-async-questions.spec.md) 和 [验收](../acceptance/native-async-questions.md)。

1. 在 Decision/EventFact/PendingQuestion 增加交互类型，业务状态与待回答问题分开；查询页派生当前阻塞信息，复用控制台选项编辑及答案事务。
2. Codex adapter 解码原生 async message 的结构化问题，绑定原 item/turn/command，加入 durable observation 和恢复重放；排除其终态结果资格。
3. 活动轮次以 `turn/steer` 投递保存的答案；原 input identity 与内容支持只读核对，原轮次停止不降级为动态工具回复或盲目重发。
4. 保留原问题贯穿报告，根据有效结果及成果核验等待或失效；已结束的原轮次通过绑定来源步骤继续。原固定发布归属随步骤提供。

## 测试计划

| 自动验收项 | 级别 / 场景 | 环境与命令 | 时点 / 通过条件 / 证据位置 |
|---|---|---|---|
| AQ-A1、AQ-A2、AQ-A3、AQ-A4 | 定向 unit/SQLite/controlled WebSocket；`TestAsync*` | Windows 既有隔离夹具；`go run ./cmd/test-feedback -out C:/SteerlineDebug/dev-20261010-163230-97ece424/evidence/async-questions-final -- ./internal/execution ./internal/execution/adapters/codex ./internal/console ./internal/product/query ./internal/app -run '^TestAsync' -count=1` | 实现后；全部通过、无 skip；原日志和 summary.json 存于指定目录，结果登记 AQ-A1–A4 |
| AQ-A5 | 定向既有同步/问答/流程回归 | Windows 隔离夹具；`go run ./cmd/test-feedback -out C:/SteerlineDebug/dev-20261010-163230-97ece424/evidence/async-regression-final -- ./internal/execution ./internal/execution/adapters/codex ./internal/app -run '^(TestCD62NativeFactsAndQuestionRoute|TestAppServerStartReplayPreservesNativeSavedAnswer|TestWorkerPromptPublicationTargetOnlyForPublication|TestDecision.*|TestWorkerStageTransitions)$' -count=1` | 新增场景通过后；同步身份与答案规则、流程及发布上下文通过；原日志和 summary.json 存于指定目录，登记 AQ-A5 |

最终增量验证命令：`go run ./cmd/test-feedback -out C:/SteerlineDebug/dev-20261010-163230-97ece424/evidence/async-final-verified -- ./internal/execution ./internal/execution/adapters/codex ./internal/console ./internal/product/query ./internal/app -run '^(TestAsync.*|TestCD62NativeFactsAndQuestionRoute|TestAppServerStartReplayPreservesNativeSavedAnswer|TestWorkerPromptPublicationTargetOnlyForPublication|TestDecision.*|TestWorkerStageTransitions)$' -count=1`。覆盖 AQ-A1–A5，包括成果不匹配仍保留原问题。控制台任何修改后按最小范围重验 AQ-A4：`go run ./cmd/test-feedback -out C:/SteerlineDebug/dev-20261010-163230-97ece424/evidence/async-console-final -- ./internal/console -run '^TestAsync' -count=1`。

新增 controlled WebSocket fixture 支持 async message、活动轮次 steer 及停止后的明确拒绝。既有分级夹具保持独立；不跑 full/L2/L3：此次主要为事件和输入状态接线，定向 SQLite、真实 WebSocket 与控制台测试覆盖最小自动验收。真实模型能否稳定产生异步问题的人工窗口后置，不将模拟通过标作 live。
