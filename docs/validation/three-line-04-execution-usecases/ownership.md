# #76 执行应用归属与验收

实现基于 #74 后的 `c4650056d701a158353836616668bfad25011552`。#75 的 task batch/固定工作所有者继续留在 `internal/task`；#76 将唯一 execution actor 与候选/会话/Decision/恢复/交付/清理命令和其断言移入 `internal/execution`。旧根包的对应生产定义已删除。

`file-mapping.json` 列出从基线移出的 root 文件及原 SHA、最终位置或合并位置；`moved-symbols.json` 是当前 execution 顶层导出定义清单；`test-mapping.json` 列出基线旧包每个受影响测试 parent 和断言所有者。execution 的 9 个内部 MC parent 原样保留；应用集成包新增真实 app 外层 parent `TestMC1/3/5/6/7/8/9`。CD66 中缺目录用例随实际 app 构造迁移；CD67 原生/provider fixture 继续提供外部效果验证。

## 唯一生产归属

| 能力 | 唯一实现 | 依赖方向 |
| --- | --- | --- |
| actor、私有/提交后状态、signal/replay/heartbeat/恢复/Decision | `internal/execution` | task/model、backend/model、contracts 和注入的工作/结果/persistence/宿主端口 |
| Task 文件固定核验 | `execution.TaskWorkPort.VerifyContractFiles`，app 注入 `task.Service` | 执行在 claim 事务之前；无重复核验器或成功 no-op |
| 运行记录/selector/runtime 的纯重建 | `internal/execution/reconstruction` | 纯 task/model、execution/model/backend、contracts；不查库、不构造 product DTO |
| 控制文件、进程 identity/worker-absent 和文件原子写 | 根宿主 `HostPort` 与 `ResourcePort`；只读 process facts 为 `internal/execution/process` | execution 注入实际 OS/file capability，保留旧状态验证 |
| loop 与启动/锁/控制传输 | `internal/steerline/host` + 根宿主 | 通用 `Scheduler[T]` 不导入 execution；app 绑定唯一 actor |
| task 声明事务 | `internal/storage/ports` 与 SQLite adapter | 原 ClaimCommit/ResultCommit/DiscardCommit 和版本断言不变 |

`product/api.JournalCursor` 的全部消费者已转至 `execution.JournalCursor`，旧定义已删除。`internal/steerline/projection` 已删除，执行态重建迁至纯 execution 子包；面向用户的 API/read model 留给 #77。

## 外层真实应用断言

| 父用例 | 真实入口与保留断言 |
| --- | --- |
| `TestMC1` | app commands 给同一已领取任务新增 5 个独立 workflow/execution 与 workspace intents；任务版本不重复领取，公开任务查询保留完整候选历史 |
| `TestMC3` | 同一 actor 启动两个 app candidate，向精确 B 运行提交答案，确认 B 原 session/process 第二轮和 A 未变 |
| `TestMC5` / `TestMC6` | 既有隔离 provider 和真实 app server；唯一 delivery、原 merge receipt、清理次数与已合并冲突门槛 |
| `TestMC7` | 两候选退出不停止执行或重开 task；新 app 实例读取原历史并可新建 candidate/discard |
| `TestMC8` | 受控端口在真实 app 的异步生命周期 intent 之后等待；actor 仍可处理 candidate exit；只接受原请求/attempt 的迟到 identity |
| `TestMC9` | 真实 app API 精确五个开始拒绝第六个，原请求重放不替换；候选退出立刻释放名额且保留完整历史 |
| `TestTM04ExecutionWiring` | app claim + SQL fault injection 确认提交失败无 task/graph/authority/runtime 改动，解除故障后原请求提交，重放 receipt/版本/DB 证据不变；原 app-server 双轮和 native acknowledgement/recovery gate |

内层 execution MC1–MC9 是迁移后私有 actor/事务的原断言；外层 MC 是跨 app/actor/task/query 边界断言，不以 mock 成功代替事务或 provider 事实。具体测试 parent/subtest 与证据路径见 [test-mapping.json](test-mapping.json)。

## 验收结果

实际测试反馈报告按 [README](README.md) 归档。每个入口记录 OS、实际 parent/subtest 命中、pass/fail/skip 和未启用的原生开关；L1 与 L3 controlled 为最终 scheduler/app 接线验收。未运行 Full、L2 或 live。
