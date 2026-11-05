# 自动交付平台查询与项目 actor 解耦 Plan

对应[Spec](../spec/automatic-delivery-pr-query.spec.md)和[专项验收](../testing/acceptance/automatic-delivery-pr-query.md)。实现仅改 SteerLine 自动交付路径，以实施时主干为起点。

## 1. 先证明事务在持续观察下可提交

- 在 `runner_result` 原事务内接入自动交付的冻结依据，取证前与事务内共用提取和校验。保存原查询时点和实例归属。沿用 `BindRunnerPR`、平台事实的身份与合入完整性规则、业务资格及同一笔提交和重放。把自动交付按绑定判定的入口限定在 `TickPR`，现有同步调用和废弃的版本规则独立保留。
- 先让回归复现旧路径的严格 revision 冲突。用相同原候选在查询未完成时持续写入有效观察页和 Run revision，断言同一份原平台证据可提交。逐项修改绑定依据及业务状态，断言事务拒绝且交付、后继、原回执和停止义务无部分写入。增加过期和原提交重放用例。

## 2. 再移动平台等待

- 把 `TickPR` 拆成 actor 选取并固定依据、后台执行只读平台查询、actor 消费一次结果。复用现有 actor 唤醒及项目生命周期，为每项目限定一个在途项，避免增加通用任务队列。查询超时沿现有 provider，提交使用 Spec 的独立有效窗口。按当前周期有序推进，在每项开始前读取当前资格，合并重复 tick，明确取消和错误后的释放条件。
- 让结果准入与既有 stop 通知使用同一优先顺序。若结果唤醒先被 select 选中，提交前仍要检查已通知的紧急控制，并先完成 inbox 消费。若控制已通过事务，依当前资格拒绝旧结果。复用工作取消和实例结束接线，确保迟到结果无写入权、关停可等完自身查询、重启从原记录重新查询。检查 `serviceReadyHook` 到项目关停的实际接线。
- 使用独立的结果唤醒和单槽位，在候选观察的安全边界以及普通 tick 之前提供交接。用会消耗全额五秒预算的辅助观察证明完整合入证据能在原有效期内被消费。交接时保留原 `CheckedAt` 和事务校验，纯让出取消沿原诊断处理边界消化。
- 用 channel 屏障验证调度时序，避免依靠睡眠或平台平均响应时间。至少两条参与候选构造合法 stop，单测保留最后候选拒绝的规则。故障覆盖 provider 挂起、重复 tick、超时、结果过期、关停及重启。确认候选扫描不会因排在前面的未合入 PR 永远跳过后续候选。

## 3. 验证

按[测试指南](../testing.md)和[路径清单](../paths.md)为每次验证使用独占 `-out`。实际构建、命令、原始结果和未验范围登记在专项验收。

| 验收项 | 环境与时点 | 入口与通过条件 |
| --- | --- | --- |
| ADP-A01 | isolated，事务接线后 | `go run ./cmd/test-feedback -- ./internal/taskrunner ./internal/storage/sqlite -count=1` 及 L4 `go.storage`。覆盖持续 revision 推进仍一次交付，原绑定、任务与图变化拒绝，事务原子性和原回执重放。 |
| ADP-A02 | isolated，调度接线后 | 定向 `./internal/execution/scheduler ./internal/taskrunner ./internal/storage/sqlite ./internal/app`。故意挂起查询，观察和派发继续，stop 在释放查询前完成。核对就绪竞争、合法撤回、最后候选、单在途、后续候选机会及实际项目关停。 |
| ADP-A03 | controlled，交付前 | `go run ./cmd/test-feedback -level=L1`、`-level=L2` 以及 `-level=L3 -suite=recovery -env=controlled -scenario recovery.waiting-merge-restart`。覆盖自动交付唯一、后继释放、重启后沿原 PR 重新查询和资源清理。 |
| ADP-A04 | isolated，交付前 | 定向 `./internal/execution/publication ./internal/taskrunner ./internal/storage/sqlite`。command 与 Gitea 沿原 provider 和认证引用，取消、超时、错误、过期、未合入或绑定冲突均保持当前业务状态。复跑既有废弃事务反例，确认本次自动入口没有改变其严格检查。 |
