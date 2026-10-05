# #78 定向与 L1 验证

基线：`4288c43b05287554de0b542e9eeb90d366fe063f`（PR219 / #76 已合入）。环境：Windows/amd64、Go 1.26.4；native acceptance 开关为空；未启动真实 Codex/Orca 会话、L2/L3/live 或 remote 操作。所有 feedback 命令用隔离 runner 输出目录，旧 #39 报告未覆盖。

## 定向结果

| 范围 | 实际命中 | 结果与证据 |
| --- | ---: | --- |
| Codex config/session args/recovery | 3 parents、12 events | 0 fail/skip；[summary](adapters-codex/run-20261005-01/summary.json) |
| 本机 Orca binding/identity | 3 parents、3 events | 0 fail/skip；[summary](adapters-orca/run-20261005-01/summary.json) |
| 存量 Orca remote refusal/protocol | 5 parents、5 events | 0 fail/skip；无远端准入；[summary](adapters-orcaremote/run-20261005-01/summary.json) |
| Codex app-server codec | 4 parents、4 events | 0 fail/skip；[summary](codexapp-protocol/run-20261005-01/summary.json) |
| Workspace | 2 parents、2 events | 0 fail/skip；[summary](workspace/run-20261005-01/summary.json) |
| Publication | 3 parents、9 events | 0 fail/skip；[summary](publication/run-20261005-01/summary.json) |
| App/resource wiring | `TestTM06ResourceWiring` 1 parent | 0 fail/skip；真实 app process Host 写隔离文件并运行本机子进程；三种 lifecycle/backend factory 归属核验；[summary](wiring/run-20261005-01/summary.json) |
| Process API caller | 1 parent | Windows `StartManaged` 调用方继续编译并跑隔离 project binding；[summary](process-api/run-20261005-02/summary.json) |
| Codex fixture path | 1 parent | 移包后读取原 JSONL 成功；[summary](adapters-codex/path-repair-20261005-02/summary.json) |
| Orca readiness fixture paths | 4 parents、14 events | 移包后原 JSON evidence 与 wrapper cases 通过；[summary](adapters-orca/readiness-path-20261005-02/summary.json) |
| Adapter boundary source fixtures | 2 parents、24 events | direct task/usecase/storage/SQL/app/UI/product/testkit import 均被拒；必要纯 contract/resource/protocol 依赖通过；[summary](boundary/run-20261005-03/summary.json) |
| Repository import closure | `TestCD67CoreBoundary` 1 parent、36 events | 全部 OS/tag source scan 与 Windows/Linux 生产传递图无 violation；[summary](integration-boundary/run-20261005-02/summary.json) |

所有列出的定向 reports 都是非零命中，0 fail、0 skip。

## L1

最终命令：

```powershell
go run ./cmd/test-feedback -level=L1 -out docs/validation/three-line-06-adapters-resources/l1/run-20261005-03
```

最终结果为 12/12 groups passed、310/310 test events passed、0 failed、0 skipped，用时 339.0 秒；summary 记录 `complete=true`，native acceptance switches 为空：[run-20261005-03 summary](l1/run-20261005-03/summary.json)。各组结果如下：

| L1 group | test events | 通过 |
| --- | ---: | ---: |
| `quick-core` | 1 | 1 |
| `quick-rules` | 154 | 154 |
| `quick-execution` | 15 | 15 |
| `quick-execution-boundary` | 24 | 24 |
| `quick-owner-remote` | 15 | 15 |
| `quick-owner-codec` | 14 | 14 |
| `quick-owner-identity` | 17 | 17 |
| `quick-owner-codexapp` | 4 | 4 |
| `quick-owner-workspace` | 2 | 2 |
| `quick-owner-publication` | 9 | 9 |
| `quick-owner-app` | 7 | 7 |
| `quick-boundary` | 48 | 48 |

首轮 L1 `run-20261005-01` 保留在 [summary](l1/run-20261005-01/summary.json)，没有用最终通过结果覆盖。它在 292 个 events 中 287 pass、5 fail、0 skip：Windows process test 看不到 `ContainsProcess`，conversation fixture 仍按旧目录深度读 JSONL，adapter transitive guard 把旧 conversation contracts 通过的纯 `product/api`/`taskview` 值误判为直接产品依赖；后续 quick package-set 失败由 `quick-core` 编译失败引起。已把 `StartManaged` 恢复为具体 OS handle 返回，按新深度改 fixture 路径，并让守卫区分禁止的 direct import 与仅由现存 pure contract 引入的依赖 closure，同时保留 direct negative fixtures。post-repair targeted reports 全部通过；最后一轮 L1 见 `run-20261005-03`。

首轮之后的 `run-20261005-02` 曾通过 12 groups/309 events；自查又补了生产 adapter 不能直接导入 testkit 的负例，最终以当前 `run-20261005-03` 重跑。首轮失败的原因及保留证据见上文。任务/DAG、MC1–MC9 精确候选、五个宽松检查、candidate exit 不停止执行、唯一 provider delivery 与 UI 关闭不代替 worker stop 的原 assertions 仍由 #76 的 `internal/execution/candidate_workers_test.go` 和现有 app integration parents 唯一拥有；映射见 [test-mapping.json](test-mapping.json)。本次没有重写、复制或缩弱这些 assertions。

本次不重跑原生模型流程：进程句柄、Codex session、Orca identity、参数、权限和协议 wire behavior 未改变。#39 未失效证据定位见 [native impact](native-impact.md)；remote 只验证迁移编译、原协议代表和拒绝保护。

## 协调独立复审

原候选复审与路径例外漏洞修复、失败复现及最终守卫验证见 [coordinator-review](coordinator-review/README.md)。上述 worker 报告保留其真实原候选状态；最终 TM06.4 已加入 L1 固定选择。
