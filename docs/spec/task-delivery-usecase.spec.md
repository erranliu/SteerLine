# Spec：任务最终交付用例独立

任务 ID：95；任务名称：任务最终交付用例独立。

状态：2026-10-14 草稿规划，未纳入执行、未实施。配套 [Plan](task-delivery-usecase.plan.md)、[专属验收](../acceptance/task-delivery-usecase.md)、[本轮草稿](../roadmap/drafts/roadmap.md)。调查基线 `5a24abec3c10d184a6e57962e154854bc6011a9f`。

候选核对：2026-10-14继续保留草稿，未纳入execution-map，当前不安排实施。最终交付用例不被三层重构替代，但当前Plan依赖的actor/PR桥接、记录转换和共同fixture正在重组，待实际接线稳定后重新核定剩余范围。三层成果进入主干后重审Spec/Plan与验收，不自动恢复旧方案；原调查和测试安排保留作参考，以下旧并行建议不能作为开工资格。冲突说明见[本轮草稿](../roadmap/drafts/roadmap.md#与worker三层重构的冲突核对)。

## 问题与成果

[native_delivery_command.go](../../internal/execution/native_delivery_command.go) 的 CompleteNativeTask 消费宽 CommandStore、完整 Config/runtime 和具体 publication.Client 类型核对，同时负责结果声明、真实 PR 事实、唯一交付及丢弃后迟到合并审计。交付测试仍依赖执行应用组装。

建立 `internal/execution/delivery` 用例 owner，消费同快照候选/任务与契约读取、该选定 workflow 的冻结发布上下文解析、带明确 provider 身份的 PR事实端口、任务结果声明和既有结果/审计提交端口。返回现有交付回执/错误，不取得 worker/session控制、完整 execution.Config、TaskCommands、具体 SQLite 或上层 Service。

原 CommandService.CompleteNativeTask 保留现有调用契约，仅组装本用例所需能力和适配错误，不复制交付规则；这是已有入口向独立用例接线，不新增第二业务路径或旧版兼容入口。task 管理继续拥有 Declare/契约/DAG规则，adapter/provider 提供真实外部事实。

## 交付语义

- 先读原 task/workflow/execution 的一致快照，再依据该 workflow 的冻结 Profile 解析 provider/仓库/base，不能用当前默认方案替代。原配置/仓库目标校验通过消费端口提供必要事实；取得事实后核对该事实的目标归属。
- provider 身份必须以现有 ProviderRef 等固定值核对，与真实查询端口绑定；接口化不能删除原具体 Client 分支中的身份保障。PR URL、number、平台实例、repository、base/head 与选定工作分支、合并时间/commit 均按现有规则核实。
- 复用原 DeliveryGate、TaskResultPort 的任务结果声明及 [ResultCommit](../../internal/storage/ports/task_result.go)。TaskDelivery、Task closed、选定 WorkflowRun 和图修订仍在同一 SQLite 提交内重核版本/绑定及唯一约束，原生 PR查询在事务外。
- 交付重试返回原同候选/版本回执；另一候选不能覆盖已记录交付。其他候选未知/阶段推进不阻塞合法选定交付；closed 后停止任务全部候选PR轮询。丢弃后的真实合并只登记原审计冲突，不生成正常 TaskDelivery，不释放后继。
- 不新增“成功 publish/报告/全部步骤完成”门槛：当原发布结果未知/失败而原绑定 PR 已真实合并时，actor 仍可沿既有只读核验/绑定入口调用本用例。缺身份或合并事实仍拒绝。
- 返回交付后，actor 继续刷新快照、记录活动及管理资源收尾；资源清理失败不回滚真实交付或重开 Task。

## 并行与所有权

本项拥有 native_delivery_command 的用例提取/薄调用接线、新 delivery 包、所属测试及必要的交付调用局部接线。不得修改 staged_flow 的发布推进/PR绑定/重试、worker 运行时、领取/丢弃的执行停止规则、shared model/ResultCommit/schema/provider codec。

无新增硬前置；#94 和本项通过现有 CompleteNativeTask 调用边界整合，建议 #94→#95 有序合入而非任务依赖。保留发布未知且真实合并的共同反例：#94 负责该 actor 路径，#95 负责最终用例；本项用自己的原调用基线完成验证，不等兄弟提供测试 helper。shared fixture/配置、model 或签名确需变化时重新协调，不暗中形成硬前置。

最终合入候选基线实际含#94时，按Plan定向运行其未知发布真实合并actor反例；实际含#96时，复验交付后的资源清理失败仍保留closed与唯一交付。未含这些成果仍完成本项独立用例/事务/controlled验证，不等待兄弟；后合入者承担已存在组合的验证责任。固定L1不是组合验证的替代。

## 最小验收

| ID | 自动通过条件 |
| --- | --- |
| TD95-A1 | 用例仅依赖窄端口/值，独立 stub 可验证，旧交付规则单一；生产闭包不依赖 execution父包、worker控制、app/host 或具体 SQL |
| TD95-A2 | 冻结 provider/仓库/任务/候选身份、完整真实合并与原错误拒绝保持；发布未知但合法合并可结项，非merged/错误PR不可结项 |
| TD95-A3 | 唯一交付、版本/候选竞态、原回执重试、事务失败回滚、丢弃迟到合并审计及后继资格保持；清理问题不重开交付 |
| TD95-A4 | 正式创建/领取到受控合并交付及待合并重启场景通过，原身份与交付仅一次 |

无新增人工项。本项重构复用正式provider与提交协议，不新增真实平台能力声明；自动结果归专属验收。
