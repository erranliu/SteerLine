# 任务候选 Run 同快照观察验收

对应[Spec](../../spec/task-run-observation.spec.md)和[Plan](../../plans/task-run-observation.plan.md)。

## 设计审计

- 审计基线为 SteerLine `92639608` 与主干 `aa08dfee` 的相关调用路径，Runner 基线为 `fddffa7a`（#106）。基线已存在正常快照分支。Runner 从同一只读事务读取 Run 与事件页，公开契约适合成为常规观察的唯一来源。拆分后备只有跨请求的 revision 上界约束，无法给出同事务证明。[Bridge](../../../internal/runnerbridge/bridge.go)、[客户端](../../../internal/runnerclient/observation.go)
- `observe` 只按 Run ID 返回投影与事件，原回执和启动前缺失证明各有独立来源。`InspectBeforeStart` 的调用者包括启动停止收集、部署预检和兼容接管，仍需保留。[启动前读取](../../../internal/runnerclient/views.go)、[原绑定](../../../internal/app/runner_startup_inspect.go)、[停止收集](../../../internal/app/runner_takeover.go)
- 当前 stop 用 effect 前 revision 构造请求，明确拒绝与结果未知分流。受理回执先保存，之后 `observe` 已能读取新 revision。清理范围可限于删除末尾拆分后备，并回归检查对账失败、采样失败及恢复。[stop 信封](../../../internal/runnerbridge/stop.go)、[候选对账](../../../internal/runnerbridge/bridge.go)
- 当前同快照能力已经是 Orca 准备等待功能的前提，普通候选仍允许拆分后备。将它提升为正常项目运行的前提会拒绝不具备此能力的构建。启动前停止收集仍先执行，在线 catalog 检查依托已接线的实例停机与结算路径。[原能力门禁](../../../internal/runnerclient/execution_wait.go)、[应用启动顺序](../../../internal/app/application_runner.go)
- 实施审计确认启动前停止收集和线上观察准入必须分开。回归通过实际应用启动接线捕获交接的 `pending_stops`，再注入缺少观察能力的 catalog，检查原实例仍能正常结算。stop 回归分别注入 `stale_revision`、受理后丢回执和受理后观察失败。

## 实施验收安排

以下均为实施后的自动验收。SteerLine 使用 `aa08dfee` 加本 PR 源码，正式验证输出根为 `C:\SteerlineDebug\task-run-observation-20261105`。Runner EXE 从干净主干 `fddffa7a5ea12a25f65c8dd85511da1750c46bda` 构建，版本 `0.2.0-dev.4`，SHA-256 `b41720bc67bdadbbf18122f0157b3785a1e57d6934435c6c39ff86049a354f91`。公开离线 catalog 声明 `steerrunner-run-observation/v1`、`same_snapshot: true`、`max_events: 500`，在线公开契约组也通过。实际安装环境未验。

| ID | 方法与安排 | 通过条件及阻塞范围 | 状态与证据 |
| --- | --- | --- | --- |
| TRO-A01 | isolated，Bridge、客户端与 SQLite 定向测试 | 只用同快照、分页完整、错误和事务冲突无部分提交。失败阻塞观察切换。 | 已验收，`owner-final/summary.json` |
| TRO-A02 | isolated，原请求和应用启动定向测试 | 原回执身份、只读规则和启动前离线证明成立，停止交接先于 worker，缺能力拒绝正常运行且仍能按原身份停服结算。失败阻塞切换和准入。 | 已验收，`owner-final/summary.json` |
| TRO-A03 | isolated，停止及原信封对账定向测试 | 回执与新 revision 分开持久化，已知拒绝与未知结果分流，观察失败恢复不丢事件，停止抢占通过。失败阻塞控制链路交付。 | 已验收，`owner-final/summary.json` |
| TRO-A04 | controlled，固定 L1、L2、L3 重启及固定 Runner 的 L4 公开接口测试 | 正常流程与旧候选恢复通过，公开能力和清理有证据。失败或缺固定构建阻塞交付。 | 已验收，`L1-final`、`L2-fixed`、`L3-serial` 与 `runner-contract` 的 `summary.json` |
| TRO-A05 | 隔离任务 101 原 Git 提交和受控 Agent，固定 Runner 公开接口 | 原就绪与启动安排字节相同，受控 Agent 读到原摘要，stop 后 `observe` 看到新 revision，结算完成。 | 已验收，`owner-final/summary.json` 与 `runner-result-native-fixture/summary.json` |

## 2026-11-05 验证记录

- `owner-final` 的受管 Go 定向测试共 73 项通过、零失败。任务 101 固定提交为 `9d81d9c2652177faf6354d4e9f9f3aec79535d11`，来自隔离 bare 仓库 `task101.git`。就绪与启动使用同一份 111 字节安排，SHA-256 为 `b9a819296f6f3c6b0e9a3ff9ce4059f89a111a9a7633ed73baa373d0475f6cc4`，固定 Git 树无同名旁置 JSON。实际 Go 参数及原日志随报告保存。
- `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.runner-result` 通过。同一 Runner 源码构建的 `tests/helpers/native` 受控 Agent 从任务 101 工作区读出同一摘要。Run `530ffead83e3b471e566dfe4afbbeb9c` stop 前 revision 为 `9`，stop 后公开 `observe` 得到 `12` 及 `stopped`。原服务结算通过，证据见 `runner-result-native-fixture/go-owner/test.jsonl`。结果渠道的一般 round-trip 和 101 缺安排负例同时通过。
- 固定 L1 `l1.minimum`、L2 `smoke.delivery`、L3 `recovery.waiting-merge-restart` 及 L4 `go.runner-contract` 通过。L2 和 L3 的 `cleanup_complete=true`。L3 首次执行在自动材料准备时报原 Runner 查询不可用且该轮清理未证实，原现场及 `L3-fixed/summary.json` 保留。相同源码和场景单独复跑通过，见 `L3-serial/summary.json`。
- 早期 `owner-initial`、`actual-101`、`L2` 与 `L3` 保留测试编译失败，后续补齐新增测试导入及 E2E 测试驱动的 `Inspect` 签名。`owner-fixed` 暴露既有 idle catalog 缺少结果支持目标，补齐测试夹具后定向通过。`runner-result` 首轮确认冻结 schema 与 Runner 当前公开声明存在差异，随后同步测试副本，继续对当前发布 schema 做完整相等检查。生产结果文件校验沿原实现。
- `runner-result-fixed` 误选 Runner 的 CLI 夹具，运行在 Agent 启动前被阻断。该失败轮已停止，独立服务完成结算，见 `wrong-fixture-final-settlement.json`。正式通过轮使用 `tests/helpers/native`。
- 真实 Codex 复验未启动。自动审批因可能向模型服务发送私有任务仓库内容而拒绝执行，继续采用独立的受控 Agent 复验。
