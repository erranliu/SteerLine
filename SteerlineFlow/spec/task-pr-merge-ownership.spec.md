# SteerLine 任务 PR 合入核验 Spec

状态：已实施，受控验证及未验范围见专项验收。适用现有 Runner 候选的 TaskDelivery 和 TaskDiscard。对应[实施 Plan](../plans/task-pr-merge-ownership.plan.md)和[专项验收](../testing/acceptance/task-pr-merge-ownership.md)。

## 1. 职责与成果

| 事实或动作 | 权威与消费方式 |
| --- | --- |
| Workflow 内的发布、原 PR 绑定、适用时的已审计 head 核验、后继及停止 | Runner 按[协作契约](runner-collaboration.spec.md)和[Runner 调用生命周期](../../docs/product-design/runner-lifecycle.md)自主处理。SteerLine 从公开 Run 投影消费原候选与 PR 的绑定。 |
| PR 此刻是否已合入 | PR 平台是权威。SteerLine 的交付、自动交付轮询和废弃共用一条只读平台查询路径。 |
| 关闭任务、记录唯一交付、释放后继、废弃及候选停止义务 | SteerLine 在原有 SQLite 事务中核对绑定、版本和权限后提交。Runner 继续执行原有停止请求并报告资源结算。 |

发布已核实的 PR 可以先于 Runner 的 `pr_ready` 里程碑出现，仍可作为交付候选。交付依据原绑定 PR 已实际合入的证据。PR head 与历史审计 SHA 的比较继续由 Runner 在现有 Workflow 节点按原规则处理，任务交付的接纳条件保持现有语义。Runner 的独立 `inspect --refresh-pr` 继续提供给独立调用方。progress、result_file 和其他启动边界在本项范围之外。

## 2. 原绑定和配置

查询对象从已核实的原候选绑定取得。核对候选、原 Run ID、输入摘要、工作区、会话、执行分支、固定 PR URL、provider 实例、仓库及 PR 编号。平台响应须与目标 URL、仓库、base 分支、head 仓库和候选分支匹配。PR 绑定与项目权威目标有冲突时停止本次结论。正常 Run 观察仍可补收尚未投影的绑定，查询已知 PR 时直接使用已核实的公开绑定和冻结配置。Provider 响应没有替换 PR 绑定或推进 Workflow 的权限。

查询使用候选原 `Submit.Input.publication` 中的 provider 类型、base URL、仓库、base 分支、`query` 和 `authentication_refs`。当前项目配置更新后，原候选仍按原查询配置和认证引用解析。凭证值在 SteerLine 查询时按原引用解析，不写入请求、任务证据或诊断。解析失败显示待核实，允许下一次按原对象发起新的只读查询。

- `command` 使用冻结的 executable 和 argv，并仅按原规则展开 `{pr_url}`、`{number}` 和 `{repository}`。沿用原查询输出契约和运行时环境认证方式，受管理进程、有界执行、无 shell 拼接。认证值与标准 provider 凭证从诊断脱敏。实施验收须覆盖应用宿主可见的 executable、PATH、工作目录约定和凭证引用，尤其是已支持的相对命令配置。
- Gitea 对冻结实例发送只读 `GET /api/v1/repos/{repository}/pulls/{number}`。按原引用取得运行时凭证，保留原 HTTP 认证、响应上限、超时和拒绝重定向规则。校验显式合并状态和返回的 PR 编号。

## 3. 查证与提交

平台查询在写事务外执行，形成仅供本次操作消费的 SteerLine 平台观察。观察记录查询时段、原候选及版本、PR 绑定和可核对的平台字段。观察与 Run 投影分别归属，SteerLine 的查询不修改 Runner Run revision，也不将平台观察写进候选的 Runner 投影。已持久化的 TaskDelivery 为产品读取提供合入证据，晚到的旧 Run 投影不会覆盖它。

交付只查询指定候选。平台须明确返回已合入、有效合入时间和 merge commit。事务内重新核对候选身份、绑定、revision、参与及停止状态，任务版本、权威与 DAG 版本，原有唯一 PR 归属、幂等、原子关闭和停止义务规则继续适用。另一候选的未知状态与该次交付分别处理。

废弃对当前候选集合查证，每个有原 Run 的候选须有可核实的原 PR，并得到本次查询确认的 `open` 或 `closed_unmerged`。已合入拒绝废弃，缺失绑定、平台不可用、响应不完整、或原提交可能已受理时保留待核实。尚无原 Run 且提交状态明确允许废弃的候选沿现有规则处理。事务内重新核对候选集合及各版本，再原子提交废弃和停止义务。成功操作的原回执重放无需重新查询。

超时、凭证缺失、平台响应冲突或版本变化均不产生新交付或废弃。既有自动交付轮询继续按原节奏执行；普通未合入和查询失败保持可区分。交付及废弃调用不需要 Runner 在线执行 `--refresh-pr`，已知绑定的查证可在 Runner 暂离线时完成，停止义务仍按现有方式独立结算。

## 4. 通过条件

| ID | 自动通过条件 |
| --- | --- |
| PMO-A01 | command 和 Gitea 都按候选冻结配置查询，项目配置变更后仍指向原对象。认证缺失、错误响应和越界目标被拒绝，诊断与持久数据无凭证值。 |
| PMO-A02 | 交付、自动轮询和废弃共用 SteerLine 查询入口。原 PR 已绑定时，即使 Runner 无法提供在线刷新，仍可凭本次平台证据判定。Runner 公开投影保持来源真实，已交付的读取显示正确合入事实。 |
| PMO-A03 | 原候选与任务图的事务校验仍有效。覆盖旧事实、并发版本变化、另一候选未知、废弃时已有合入或未知、成功回执重放、唯一交付、后继释放和停止义务。 |
| PMO-A04 | Runner 发布核验、独立刷新以及文件通道继续沿现有契约运行。交付允许 Runner 未到 `pr_ready` 时已有原绑定 PR 的情况。 |
