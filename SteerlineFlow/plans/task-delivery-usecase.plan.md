# Plan：任务最终交付用例独立

任务 ID：95；任务名称：任务最终交付用例独立。

依据 [Spec](../spec/task-delivery-usecase.spec.md)，结果归[专属验收](../testing/acceptance/task-delivery-usecase.md)。2026-10-14 草稿；调查基线/RoadmapGuide/SpecGuide版本 `5a24abec3c10d184a6e57962e154854bc6011a9f`；启动另固定实际文档版本与主干。

候选核对：2026-10-14继续保留草稿，未纳入execution-map，当前不安排实施。最终交付用例不被三层重构替代，但当前Plan依赖的actor/PR桥接、记录转换和共同fixture正在重组，待实际接线稳定后重新核定剩余范围。三层成果进入主干后重审Spec/Plan与验收，不自动恢复旧方案；原调查和测试安排保留作参考，以下旧并行建议不能作为开工资格。冲突说明见[本轮草稿](../Roadmap/drafts/roadmap.md#与worker三层重构的冲突核对)。

## 实施步骤与现有代码整合

1. 调查 CompleteNativeTask、recordDiscardedMergeConflict、commitNativeDelivery、DeliveryGate、CommitTaskResult 和 actor 的 nativeCompleteMergedTask。明确最终交付不依赖成功步骤、原冻结配置先按选定workflow解析，现有事务负责候选/版本/唯一性。保留task Declare和全部原错误语义。
2. 新建 `internal/execution/delivery`。定义本消费者所需的候选/契约读取、原结果读取、冻结发布上下文解析、带ProviderRef的PR查询、仅Declare的任务结果端口、ResultCommit及丢弃合并审计提交端口。后者复用既有审计事务声明，不把宽CommandStore或完整Config复制成新的门面。
3. 用例先获取一致快照和选定workflow，再调用原冻结Profile解析能力。执行应用的最小接线适配保留原目录/配置/仓库事实校验，给用例必要固定值；不将default profile或已验证标记替代实际事实。PR查询端口显式提供真实provider身份；可给publication.Client增加读取现有ProviderRef的最小方法，或由当前组装绑定该值与查询能力，禁止保留只对具体Client检查的分支。复用ProviderIdentity/ValidateWorkerPRIdentity/PRNumber等真实规则，不复制一份provider解析。
4. 迁移选定候选/真实merge、原结果重试、任务声明/提交和丢弃迟到合并审计。用例返回原类型化回执/错误，CommandService入口只绑定实际小端口并调用；actor仍负责快照刷新、活动和资源收尾。删除旧主体规则，不增加转发服务或自动选择候选/重试。
5. 新增纯用例stub `TestDeliveryUseCase`、boundary中的 `TestDeliveryBoundary` 和actor/SQLite fixture中的 `TestDeliveryAtomicCommit`（计划名称，当前不存在）。前者验证原失败/unknown publish合法merged、provider错身份、同候选重交和丢弃审计；后者注入提交失败和两个候选竞态，验证Task/WorkflowRun/Delivery/图版本无部分提交、只选一个交付、清理失败不重开。边界独立文件检查all-tag及Windows/Linux依赖闭包，拒绝父execution、app、worker控制/adapter、具体SQL；允许现有model/ports和publication事实值/规则，不放宽旧守卫。
6. 按测试表执行用例/真实事务及已有两条controlledE2E。核对最终候选基线实际包含哪些兄弟成果：已含#94时定向运行TestDevelopmentUnknownPublicationMergedDelivery，已含#96时运行TestWorkerRuntimeDeliveredCleanup。未含者明确记条件未触发，不记通过或等待；已含而缺入口/零匹配失败。自己的用例未知发布/事务/controlled验证始终运行，不靠兄弟能力满足本项独立验收。通过注册runner正式选择，复用driver和隔离fixture；无需扩展原生/环境能力或新增测试场景注册。
7. 更新本项架构/并行指南，向专属验收登记实际结果、失败/skip、精确命令/源码/构建和临时证据。正式纳入后按真实复审/合并核验同步execution-map；当前仅草稿。

## 修改所有者与合入

#95拥有delivery包、native_delivery_command薄入口、其私有读取/上下文适配与用例测试；必要publication身份读取由本项维护，不能改变provider现有codec/查询行为。#94拥有staged_flow和未知发布actor反例；#95保持CompleteNativeTask调用签名/错误，使用自己的fixture独立验证。execution_delivery_poll只作既有最终用例调用的局部适配，不改PR轮询/步骤或resource cleanup规则。

无新增硬前置。#94/#95可各自实现/验证；默认#94先合入，#95后合入核验组合。共享model、ResultCommit、task规则、schema、CommandService构造及公共fixture冻结；若实现需要修改这些共同段，先重新确定契约与所有者，不消费未交付假接口。#96后集成时保留交付与清理结果分离。

## 测试计划

新增包/测试须在步骤2/5完成后实际枚举，零匹配/skip不算通过。runner默认将summary.json和原始Go日志/stderr放系统临时 `steerline-test-feedback/<运行时间>/`；每行的实际目录、选择、源码/dirty与结果登记专属验收“执行记录”。E2E产物按runner同目录scenarios保留，不提交数据库/原日志。

| 验收项 | 级别/组/场景、环境 | 命令 | 时点/通过条件 | 证据位置 |
| --- | --- | --- | --- | --- |
| [TD95-A1](../testing/acceptance/task-delivery-usecase.md#自动验收计划) | 定向用例/新增TestDeliveryUseCase、TestDeliveryBoundary；stub及Windows/Linux闭包 | `go run ./cmd/test-feedback -- ./internal/execution/delivery -run '^TestDeliveryUseCase$' -count=1`；`go run ./cmd/test-feedback -- ./internal/testkit/boundary -run '^TestDeliveryBoundary$' -count=1` | 提取后、PR前；窄能力独立/旧规则删除，Linux仅结构证据 | 两个runner目录；TD95-A1 / owner、boundary |
| [TD95-A2](../testing/acceptance/task-delivery-usecase.md#自动验收计划) | 定向正式用例/actor；隔离SQLite与provider，消费TD95-A1中的全部身份反例 | `go run ./cmd/test-feedback -- ./internal/execution -run '^(TestWorkerFlowProviderDeliveryRejectsWrongPRAndClosedUnmerged|TestWorkerFlowDeliveryCommitsWithoutStepGatesAndUnlocksSuccessor|TestWorkerFlowDeliveryGateUsesSessionAndPRWithoutStepResults|TestCandidateClosedPRPollDeliversLaterMerge)$' -count=1` | 接线后、PR前及#94已合入的最终候选；错误事实拒绝，未知合法合并可交付 | runner目录；TD95-A2 / facts |
| [TD95-A3](../testing/acceptance/task-delivery-usecase.md#自动验收计划) | 定向事务/新增TestDeliveryAtomicCommit及原CAS；实际SQLitefixture | `go run ./cmd/test-feedback -- ./internal/execution -run '^(TestDeliveryAtomicCommit|TestTM02ResultCAS)$' -count=1` | 提交接线后、PR前；原结果/唯一性/版本竞态/丢弃审计和失败回滚完整，无部分交付或后继误释放 | runner目录；TD95-A3 / transactions |
| [TD95-A4](../testing/acceptance/task-delivery-usecase.md#自动验收计划) | L3 controlled / delivery.basic、recovery.waiting-merge-restart；现有正式driver隔离项目 | `go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled '-scenario=delivery.basic,recovery.waiting-merge-restart'` | PR前及最后有共享提交/构造变化的候选；精确两个scenario均通过/cleaned，依赖解锁及重启唯一交付 | E2E runner目录；TD95-A4 / scenarios |
| [TD95-A2](../testing/acceptance/task-delivery-usecase.md#自动验收计划) / #94已合入时 | 定向actor/SQLite/provider，#94交付的未知发布真实合并反例 | `go run ./cmd/test-feedback -- ./internal/execution -run '^TestDevelopmentUnknownPublicationMergedDelivery$' -count=1` | 最终候选基线含#94时必跑；未知publish经原绑定到新delivery仍可交付，错误/替代PR拒绝 | runner目录、基线commit/触发条件；TD95-A2 / development-combination |
| [TD95-A3](../testing/acceptance/task-delivery-usecase.md#自动验收计划) / #96已合入时 | 定向actor/SQLite与#96受控清理失败fixture | `go run ./cmd/test-feedback -- ./internal/execution -run '^TestWorkerRuntimeDeliveredCleanup$' -count=1` | 最终候选基线含#96时必跑；新delivery提交后资源清理失败不重开Task/改写唯一Delivery，图及后继资格保持 | runner目录、基线commit/触发条件；TD95-A3 / runtime-combination |
| 必要回归 | 固定L1 | `go run ./cmd/test-feedback -level=L1` | PR前一次，passed/complete与既有提交边界通过 | L1目录；回归记录 |

不选择Full、L2/live：本项不改平台provider协议/权限，正式SQLite与两条controlled链验证协调；不将其称为真实GitHub PR验收。若后续确需改变原生查询命令/认证，必须先重新定范围及原生验证，不能以现计划豁免。无新增人工窗口。
