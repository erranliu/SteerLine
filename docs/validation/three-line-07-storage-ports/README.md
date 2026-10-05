# #79 存储目录与消费者端口验证

依据：[Spec](../../spec/three-line-07-storage-ports.spec.md) · [Plan](../../spec/three-line-07-storage-ports.plan.md) · [迁移归属](ownership.md) · [测试映射](test-mapping.json)

## 结果

- PR #223 在最新 `main` `d532c5f13c1d3269c019a8cdee6df008580c0446` 上完成 rebase。#224 的 Full owner 分区、资源注入、managed Git/heartbeat、task document 与 E2E deadline/cleanup 更新均保留。
- 实现和回归代码已提交为 `810130c9a4520ad9fb5ac4ae517d0534ab13a153`。L1 summary 记录其运行时 HEAD 为父提交 `820d3fa65e7aace9f2ff8fe2af7d622f3611e208`，并记录了当时完整修改工作树；测试之后没有再改源码或测试，因此提交树与已验证实现一致。随后只更新了本 README 与 test mapping。
- 修复了独立复审确认的 3 项遗漏：worker-flow 服务预检调用 execution 所有者的 `ValidateStoredExecutions`；SQLite 在同一只读事务采 checkpoint、TaskGraph/authority 和 execution facts；命令、Workflow(id)、Activity 改用 task/candidate/workflow/attempt 范围的值和前值读取。
- 最终 L1 通过：12 组、432/432 测试通过，0 失败、0 跳过，执行 374.1 秒。该报告来自集成 `d532c5f` 后的候选：[L1 summary](L1-20261005T154237/summary.json)。各 quick 组原始 Go JSON 与 stderr 在同目录。
- 全仓编译诊断 `go test -run '^$' ./...` 退出码为 0，只证明编译；stdout、stderr 和实际退出码：[stdout](compile-only-01-20261005T154223/stdout.log) · [stderr](compile-only-01-20261005T154223/stderr.log) · [exit code](compile-only-01-20261005T154223/exit-code.txt)。
- 未运行 Full、L2、L3 或 live。#224 的 Full/十场景报告不是本次 #79 行为测试证据。schema 14 和 checkpoint/runtime schema 18 未变。

## 新发现的回归

| 组 | 命中与结果 | 报告 |
| --- | --- | --- |
| `preflight-01` | 1/1 通过；真实 app service factory 对当前 schema execution 缺失 `Environment.Preparation` 返回 `unsupported_schema`，且数据库 revision 不变 | [summary](preflight-01-20261005T153851/summary.json) · [Go JSON](preflight-01-20261005T153851/go-test.jsonl) |
| `runtime-snapshot-01` | 2/2 通过；WAL reader 在 checkpoint 采样后遇到真实 WorkerCommit 和 authority 写入仍只见完整旧 revision；下一次读到完整新 checkpoint、authority、execution；真实 app Runtime 使用所采样的 checkpoint/authority | [summary](runtime-snapshot-01-20261005T153912/summary.json) · [Go JSON](runtime-snapshot-01-20261005T153912/go-test.jsonl) |
| `bounded-read-01` | 1/1 通过；B 的 operation JSON 损坏后，重新打开的 app reader 仍能读取 A 的 Workflow(id) 和 Activity，事件及执行/attempt 归属保持固定 identity | [summary](bounded-read-01-20261005T153931/summary.json) · [Go JSON](bounded-read-01-20261005T153931/go-test.jsonl) |
| `bounded-command-01` | 1/1 通过；B 的 operation JSON 损坏后，真实 app command 可为同一任务启动 C；前值包含完整 TaskGraph 和该任务全部三个候选，没有 project-wide Query 能力 | [summary](bounded-command-01-20261005T153954/summary.json) · [Go JSON](bounded-command-01-20261005T153954/go-test.jsonl) |

## 定向回归和主干集成

| 组 | 命中与结果 | 报告 |
| --- | --- | --- |
| `sqlite-01` | 11/11 通过；回滚、项目/活动事务、只读和 conversation 写屏障 | [summary](sqlite-01-20261005T151028/summary.json) · [Go JSON](sqlite-01-20261005T151028/go-test.jsonl) |
| `execution-01` | 19/19 通过；MC1/MC7 候选领取、完整历史、结果 CAS/唯一交付、丢弃仲裁 | [summary](execution-01-20261005T151039/summary.json) · [Go JSON](execution-01-20261005T151039/go-test.jsonl) |
| `product-01` | 8/8 通过；storage ports、页面投影和 501 候选顺序 | [summary](product-01-20261005T151651/summary.json) · [Go JSON](product-01-20261005T151651/go-test.jsonl) |
| `storage-boundary-01` | 17/17 通过；TM04/TM07 正负边界 | [summary](storage-boundary-01-20261005T151659/summary.json) · [Go JSON](storage-boundary-01-20261005T151659/go-test.jsonl) |
| `product-boundary-01` | 37/37 通过；#77/#78 全源码、Windows/Linux 依赖图与负向注入 | [summary](product-boundary-01-20261005T151713/summary.json) · [Go JSON](product-boundary-01-20261005T151713/go-test.jsonl) |
| `wiring-01` | 1/1 通过；app SQLite wiring、消费者端口与 Close 回调 | [summary](wiring-01-20261005T151724/summary.json) · [Go JSON](wiring-01-20261005T151724/go-test.jsonl) |
| `wiring-regression-01` | 11/11 通过；CD67 app-server、terminal、fixed input 和恢复回归 | [summary](wiring-regression-01-20261005T151750/summary.json) · [Go JSON](wiring-regression-01-20261005T151750/go-test.jsonl) |
| `mainline-runner-01` | 9/9 通过；迁移后的 execution Full owner 有非零测试，coverage 反例拒绝缺失、重复、旧 owner 与错报 owner；未启动 Full | [summary](mainline-runner-01-20261005T152916/summary.json) · [Go JSON](mainline-runner-01-20261005T152916/go-test.jsonl) |
| `mainline-fixtures-01` | 9/9 通过；file resource、WorkerFlow 当前 profile 拒绝、SQLite host fixture、managed Git task document | [summary](mainline-fixtures-01-20261005T153516/summary.json) · [Go JSON](mainline-fixtures-01-20261005T153516/go-test.jsonl) |
| `mainline-e2e-01` | 11/11 通过；business deadline、候选 feedback 和第二次 cleanup 保留第一次冻结的业务结果 | [summary](mainline-e2e-01-20261005T153835/summary.json) · [Go JSON](mainline-e2e-01-20261005T153835/go-test.jsonl) |

最终 L1 分组的实际父/子测试名、退出码和分组计数见 [L1 summary](L1-20261005T154237/summary.json)。复审前的 431 事件 L1 仍保留作历史证据，不替代当前 432 事件结果。

## 集成与诊断轨迹

- 最新 main 的父提交为原集成基线 `f8b5959`，PR #224 合并后的 tip 是 `d532c5f`。基于该 tip rebase；四个冲突集中在 execution capability 测试与 core host fixture。保留了上游 file resource 断言，并把 capability report fixture 接到当前 `ResourcePort`；host fixture 使用新的 `storage/sqlite` 与 `ExecutionStateReader`；其他 managed Git、heartbeat、Full owner、候选样本复用和 E2E deadline/cleanup 变更保留。rebase 后 runner/fixture/E2E 定向组和最终 L1 均通过。
- 诊断编译失败均有独立产物 `compile-only-attempt-01` 至 `compile-only-attempt-08`；没有将其写成行为测试通过。最终 compile-only 结果保存在上表路径。
- 初版 `preflight-01` 在 `150645` 失败，因为 fixture 误以为本地 claim 已包含 preparation；后续测试改为通过真实 `CommitWorker` 写入缺 preparation 的当前格式记录，再验证 app factory 拒绝，未更改拒绝条件。报告：[summary](preflight-01-20261005T150645/summary.json)。
- 主干夹具诊断 `mainline-fixtures-01-20261005T152931` 暴露旧错误文案断言和 core test fixture 缺 app storage hooks；`153248` 又暴露 test-only `ReadRuntime` 回调递归。已将 fixture 改为明确注入 reader/validator/project initializer，并在测试回调中直接读取 SQLite 快照；最终 `153516` 通过。失败/不完整原始报告均保留，未覆盖。
- 每次定向运行通过 `test-feedback` 使用新空报告目录、Windows/amd64、受控 Git/SQLite fixture，无原生 opt-in。E2E live、模型请求和远端 provider 验收未运行。
- 原手动外层临时根 `C:\s79r`、`C:\s79l1` 仍由先前执行遗留；之前删除请求被桌面文件操作策略拒绝，本次没有重试或绕过。

具体所有者、父/子测试和报告映射见 [test-mapping.json](test-mapping.json)；当前接口与 fixture 所有权见 [ownership.md](ownership.md)。
