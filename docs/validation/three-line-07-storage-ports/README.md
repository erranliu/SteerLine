# #79 存储目录与消费者端口验证

依据：[Spec](../../archive/spec/completed/three-line-07-storage-ports.spec.md) · [Plan](../../archive/spec/completed/three-line-07-storage-ports.plan.md) · [迁移归属](ownership.md) · [测试映射](test-mapping.json)

## 当前结果

- PR #223 已基于集成基线 `d532c5f13c1d3269c019a8cdee6df008580c0446` rebase。#224 的 Full owner 分区、资源注入、managed Git/heartbeat、task document 与 E2E deadline/cleanup 更新均保留。
- 实现提交为 `810130c9a4520ad9fb5ac4ae517d0534ab13a153` 和 `9e17f8513f9889c4afd95c856fe373a12069d107`。后者修复复审发现的跨 revision app project snapshot。`test-mapping.json` 记录当前被验证的源提交；测试后只有验证文档变化。
- 最新最终 L1 报告通过 12 组、434/434 测试，0 失败、0 跳过，用时 379.6 秒：[L1 summary](L1-20261005T171014/summary.json)。quick-owner-app 包含原有、未削弱的 `TestFileQuerySourceReadsCombinedProjectView`；quick-execution 包含 `TestWorkerFeedbackPhaseCommitIsDurableAndRejectsStaleActor`。
- 全仓编译 `go test -run '^$' ./...` 退出码 0，只作为编译证据：[stdout](compile-only-02-20261005T170800/stdout.log) · [stderr](compile-only-02-20261005T170800/stderr.log) · [exit code](compile-only-02-20261005T170800/exit-code.txt)。
- 未运行 Full、L2、L3 或 live。#224 的 Full/十场景报告不是本次 #79 行为测试证据。schema 14 与 checkpoint/runtime schema 18 未变。

## 最新复审修复与证据

- `steerlineapp.clientReader.ReadProject` 通过单次 `ReadApplicationProjectSnapshot` 读取产品视图、checkpoint、execution state 和 authority，并用同一快照重建 Runtime；不再分两次采样。
- E2E `SnapshotContext` 使用同一 application snapshot，并拒绝数据库 ID、revision、authority、graph version 或时间戳不一致的页面/执行数据。host fixture 也从单次 execution snapshot 读取 checkpoint 与执行状态。
- SQLite WAL 测试在产品视图采样后提交真实 WorkerCommit 与 authority 写入，确认当次组合读取仍完整返回旧 revision，下一次读取返回新 revision。该组保留原有 app assertion。

| 组 | 命中与结果 | 报告 |
| --- | --- | --- |
| `project-snapshot-01` | 2/2 通过；SQLite WAL writer interleave 与未更改的 `TestFileQuerySourceReadsCombinedProjectView` | [summary](project-snapshot-01-20261005T170812/summary.json) · [Go JSON](project-snapshot-01-20261005T170812/go-test.jsonl) |
| `runtime-snapshot-01` | 2/2 通过；真实 app Runtime 使用采样 authority/checkpoint，worker fixture 使用 atomic execution snapshot | [summary](runtime-snapshot-01-20261005T170829/summary.json) · [Go JSON](runtime-snapshot-01-20261005T170829/go-test.jsonl) |
| `mainline-e2e-01` | 12/12 通过；包含 `TestApplicationProjectSnapshotRejectsDifferentDatabaseRevisions` | [summary](mainline-e2e-01-20261005T170854/summary.json) · [Go JSON](mainline-e2e-01-20261005T170854/go-test.jsonl) |

## 最新候选定向回归

下列报告均在 `9e17f851` 源码上重跑，避免把更早候选的报告引用为当前证据。历史失败和诊断产物仍保留在原目录，并在测试映射中单独列为 `diagnostic_attempts`。

| 组 | 命中与结果 | 报告 |
| --- | --- | --- |
| `sqlite-01` | 11/11 通过；回滚、项目/活动事务、只读和 conversation 写屏障 | [summary](sqlite-01-20261005T172035/summary.json) · [Go JSON](sqlite-01-20261005T172035/go-test.jsonl) |
| `execution-01` | 19/19 通过；候选领取、完整历史、结果 CAS/唯一交付和丢弃仲裁 | [summary](execution-01-20261005T172046/summary.json) · [Go JSON](execution-01-20261005T172046/go-test.jsonl) |
| `product-01` | 8/8 通过；storage ports、页面投影和 501 候选顺序 | [summary](product-01-20261005T172124/summary.json) · [Go JSON](product-01-20261005T172124/go-test.jsonl) |
| `storage-boundary-01` | 17/17 通过；TM04/TM07 正负边界 | [summary](storage-boundary-01-20261005T172133/summary.json) · [Go JSON](storage-boundary-01-20261005T172133/go-test.jsonl) |
| `product-boundary-01` | 37/37 通过；全源码依赖图与负向注入 | [summary](product-boundary-01-20261005T172145/summary.json) · [Go JSON](product-boundary-01-20261005T172145/go-test.jsonl) |
| `wiring-01` | 1/1 通过；app SQLite wiring、消费者端口与 Close 回调 | [summary](wiring-01-20261005T172158/summary.json) · [Go JSON](wiring-01-20261005T172158/go-test.jsonl) |
| `wiring-regression-01` | 11/11 通过；app-server、terminal、fixed input 和恢复回归 | [summary](wiring-regression-01-20261005T172537/summary.json) · [Go JSON](wiring-regression-01-20261005T172537/go-test.jsonl) |

## 主干集成与剩余定向组

| 组 | 命中与结果 | 报告 |
| --- | --- | --- |
| `preflight-01` | 1/1 通过；真实 app service factory 对当前 schema execution 缺少 `Environment.Preparation` 返回 `unsupported_schema`，数据库 revision 不变 | [summary](preflight-01-20261005T153851/summary.json) · [Go JSON](preflight-01-20261005T153851/go-test.jsonl) |
| `bounded-read-01` | 1/1 通过；损坏的其他 operation JSON 不阻止按固定 identity 读取 Workflow(id) 和 Activity | [summary](bounded-read-01-20261005T153931/summary.json) · [Go JSON](bounded-read-01-20261005T153931/go-test.jsonl) |
| `bounded-command-01` | 1/1 通过；真实 app command 可为同一任务启动候选 C，前值限定为该任务的 TaskGraph 与候选集合 | [summary](bounded-command-01-20261005T153954/summary.json) · [Go JSON](bounded-command-01-20261005T153954/go-test.jsonl) |
| `mainline-runner-01` | 9/9 通过；execution Full owner 非空，coverage 反例拒绝缺失、重复、旧 owner 和错报 owner；未启动 Full | [summary](mainline-runner-01-20261005T152916/summary.json) · [Go JSON](mainline-runner-01-20261005T152916/go-test.jsonl) |
| `mainline-fixtures-01` | 9/9 通过；file resource、WorkerFlow 当前 profile 拒绝、SQLite host fixture、managed Git task document | [summary](mainline-fixtures-01-20261005T153516/summary.json) · [Go JSON](mainline-fixtures-01-20261005T153516/go-test.jsonl) |

## 集成与诊断轨迹

- `d532c5f` 是集成基线。rebase 冲突集中在 execution capability 测试与 core host fixture；保留上游 file resource 断言，并把 capability report fixture 接到当前 `ResourcePort`；host fixture 使用新的 `storage/sqlite` 与 `ExecutionStateReader`。managed Git、heartbeat、Full owner、候选样本复用和 E2E deadline/cleanup 更新均保留。
- 最新 L1 及定向报告按当前提交更新在 `test-mapping.json` 中。此前集成后 432 测试的 L1 是较早候选证据；更早 431 事件报告也保留为历史数据，没有覆盖或重新标记。
- 失败的编译诊断 `compile-only-attempt-01` 至 `compile-only-attempt-08`、初期夹具和 preflight 失败报告均保留；它们不算通过证据。主干 fixture 诊断发现旧错误文案断言、缺少 app storage hooks 和 test-only reader 递归，均已修复，最终 `mainline-fixtures-01` 通过。
- 定向运行通过 `test-feedback` 使用新的空报告目录、Windows/amd64、受控 Git/SQLite fixture，无原生 opt-in。E2E live、模型请求和远端 provider 验收未运行。

具体所有者、父/子测试、当前源码提交和报告映射见 [test-mapping.json](test-mapping.json)；当前接口与 fixture 所有权见 [ownership.md](ownership.md)。
