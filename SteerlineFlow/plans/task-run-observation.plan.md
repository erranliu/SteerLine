# 任务候选 Run 同快照观察 Plan

对应[Spec](../spec/task-run-observation.spec.md)和[验收记录](../testing/acceptance/task-run-observation.md)。以实施时 SteerLine 主干和固定 Runner 构建为起点。

## 1. 先确认构建和准入顺序

- 用预期交付的 Runner EXE、版本和 SHA256 验证在线 catalog，复用 runnerclient 对观察 contract、`same_snapshot` 和 `max_events` 的判定。能力不足时在在线 catalog 接线完成前返回清晰诊断。检查项目 actor 未启动，原 Runner instance 的停机和结算处理已接线。能力门禁放在既有启动前 `inspect`、stop 收集及 `StartServiceWithStops` 之后。
- 保留冷读取、原身份校验、租约、启动前停止信封保存和 `pending_stops` 接线。先补一条包含已有停止义务且在线 catalog 不支持观察的用例，证明义务仍按原流程交接，正常任务运行被阻断且仍可清理所用服务。

## 2. 收口候选桥接

- 将 `Observe` 纳入候选 Bridge 的客户端接口，复用现有 `observe` 校验与保存。无待处理控制的分支直接观察，有控制的分支保留 effect 前读取、原请求对账、受理回执落盘，最后重新观察。删除拆分的事件拉取与末尾 inspect 后备，审计无其它调用后删除 SteerLine 的 `runnerclient.Events` 包装。
- 复核 SteerLine 的 `Inspect` 调用均不使用 `--refresh-pr` 后，移除无用的 refresh 形参和拼参，保留普通及离线 inspect 以及原响应绑定校验。更新调用方和夹具的签名。CLI 夹具分别提供 Run 快照、按 ID 的回执和启动前查询。核对真实生产接线与固定测试驱动都发布相应 capability。
- 保留原 `prepareStop`、`deliver`、`VerifyBinding` 和启动前对账的状态规则。用调用顺序和持久化断言证明，受理后观察故障仍保有原回执，游标只随成功的投影与事件提交推进。同步[接入说明](../../docs/integration/steerrunner.md)及[可靠交付链](../../docs/architecture/reliable-delivery-chain.md)中的当前链路描述，保留历史审计基线。

## 3. 验证

按[测试指南](../testing.md)和[路径清单](../paths.md)为每次测试分配独占 `-out` 目录，固定版本、命令和报告写入专项验收。定向用 `go run ./cmd/test-feedback -out <绝对目录> -- <Go 测试参数>`。

| 验收项 | 环境 / 时点 | 入口与通过条件 |
| --- | --- | --- |
| TRO-A01 | isolated / 收口后 | `./internal/runnerbridge ./internal/runnerclient ./internal/storage/sqlite`。断言单次 `observe`，禁止 `events` 后备，覆盖无效身份、revision 回退、事件超界、分页、空页后新事件、读失败重试及 CAS 整页失败。 |
| TRO-A02 | isolated / 收口后 | `./internal/runnerbridge ./internal/taskrunner ./internal/app`。覆盖原提交迟到、未知输入只读、受理输入按原命令 ID 完成、启动前 ready 和离线查无、已有停止信封先于 worker，及缺能力时项目阻断和原服务清理。 |
| TRO-A03 | isolated / 收口后 | `./internal/runnerbridge ./internal/app ./internal/storage/sqlite`。注入停前读到 revision N、并发变更产生明确拒绝、后续按重读 revision 准备信封；另测受理但响应丢失只对账原信封，以及受理后 `observe` 读到更新 revision 或失败后从原游标恢复。复跑停止抢占回归。 |
| TRO-A04 | controlled / 交付前 | `go run ./cmd/test-feedback -level=L1`、`-level=L2`、`-level=L3 -suite=recovery -env=controlled -scenario recovery.waiting-merge-restart`，以及带固定 Runner 的 `-level=L4 -suite=go -scenario go.runner-contract`。正常任务、原候选重启恢复、公开观察能力和隔离资源清理通过。 |

受控测试走公开客户端入口。另用任务 101 原固定提交在隔离现场复验（TRO-A05），通过 `TestNative101GeneratedArrangementStartsWithoutSidecar` 对比就绪与启动安排，再用 `TestNative101PublishedArrangement` 验证固定 Runner EXE、受控 Agent 读到同一份安排及 stop 后 `observe` 读到新 revision。复用[原任务 101 证据规则](../testing/acceptance/fresh-worktree-startup.md)。真实 Codex、正式安装与部署分别安排。
