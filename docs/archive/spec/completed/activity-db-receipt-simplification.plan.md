# Plan：活动存储与回执处理精简

行为依据：[Spec](activity-db-receipt-simplification.spec.md)。实施顺序：活动镜像消费者删除与数据库断言 → 原操作/请求 helper → 结果与启动回执共同检查 → 定向/quick/受控闭环 → 净行数与旧断言映射 → 新 PR。

## 测试计划

Windows amd64，临时 STEERLINE_HOME、Git/SQLite/协议 fixture、loopback provider；Linux 只编译。证据归 bin/activity-db-receipt-simplification/，摘要归 docs/validation/activity-db-receipt-simplification/。

| 级别/组或场景 | 命令 | 时点 | 通过条件/证据 |
| --- | --- | --- | --- |
| L1 定向：规范活动/会话/旧 Attempt | `go run ./cmd/test-feedback -out bin/activity-db-receipt-simplification/activity-final -- ./internal/steerline ./internal/steerlineclient -run '^TestActivity|^TestConversationJournalReplay|^TestSimplifiedCanonicalActivity|^TestCM71.*Attempt' -count=1` | 镜像删除后 | 数据库保存/去重/原归属/reopen、镜像目录不可写也不影响事务、原始日志/会话采集保持，actual run/pass |
| L1 定向：请求/回执/stop/冻结/失败 | `go run ./cmd/test-feedback -out bin/activity-db-receipt-simplification/receipts -- ./internal/steerline -run '^TestWorker.*(Intent|Receipt|Commit)|^TestExecutionStop|^TestAppServer|^TestSimplifiedLifecycle|^TestSimplifiedCleanup' -count=1` | helper 稳定后 | 原 intent/身份/unknown 无重发、接收/拒绝和提交失败/附件/下一步屏障保持；旧断言继续命中 |
| L1 quick | `go run ./cmd/test-feedback -level=L1 -out bin/activity-db-receipt-simplification/quick` | 最终源码稳定后 | 所有组/parent/subtest complete/pass，0 fail/skip；新旧请求守卫与 all tags/跨平台依赖边界保持 |
| L3 controlled：delivery.basic、recovery.waiting-merge-restart | `go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled -out bin/activity-db-receipt-simplification/controlled` | 完整消费者切换后 | worker 真 Git/loopback PR、原身份重启、receipt/ack、唯一交付和清理完成 |
| 编译/静态 | 全仓 build-only、Windows/Linux amd64 CLI build、diff --check、镜像与重复消费者清单 | 提交前 | 无旧调用/镜像写入/悬空断言，编译/边界通过 |

不默认选择 Full、完整 L2 或 live 模型：本次保留原生动作协议和权限，只取消诊断副本及复用现有逻辑；确定性 Service 集成覆盖本机/终端重启与失去回执，受控闭环验证真实 Git/交付。若发现原生特有未覆盖问题，再追加最小必要场景。

最终编译命令：`go run ./cmd/test-feedback -build-only -out bin/activity-db-receipt-simplification/build-final`；完整执行命令与 parent/subtest 命中见 validation.json。无需新增环境或场景能力。
