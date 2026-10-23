# Spec：候选流程状态字段命名收敛

用户直接授权的普通开发规格，2026-10-10。不纳入 roadmap、不分配 Task ID。配套 [Plan](../plans/state-naming.plan.md)、[专属验收](../testing/acceptance/state-naming.md)。

## 1. 目标与范围

候选快照的 `step_id` 与裸 `status` 没有清楚表达归属；`finish_task_status` 又派生重复的流程状态。本次只完成三项：候选流程阶段 `step_id` 改为 `workflow_stage`，候选流程状态 `status` 改为 `workflow_status`，删除 `finish_task_status` 及其派生逻辑。

Task 表达整个任务生命周期；Workflow 表达某个候选 P1/P2 的业务阶段及状态；Worker 表达执行资源和轮次。`workflow_stage` 回答“走到哪里”，`workflow_status` 回答“在该阶段处于什么情况”。两者分别保存，不合成阶段×状态枚举。

本次不修改 task_status、execution_status、resource_status、恢复字段、轮次结果及对应中文标签；不拆分 Worker 轮次、不新增轮次身份字段。不重做任务列表、候选聚合、主状态合成、详情布局或活动消息样式。任务“进行中”与候选“待合并”的完整展示收敛留给后续独立范围，本次不承诺解决全部显示重复。

## 2. 重命名契约

| 现有 Go / JSON | 新 Go / JSON | 含义 |
| --- | --- | --- |
| StepID / step_id（候选快照中的业务阶段） | WorkflowStage / workflow_stage | 当前候选的流程阶段 |
| BusinessStatus / status（候选快照中的业务状态） | WorkflowStatus / workflow_status | 当前候选的流程状态 |
| FinishTaskStatus / finish_task_status | 删除 | 消费点直接判断权威事实 |

命名贯穿执行重建 RunView、product/query CandidateView、product/api CandidateSummary、公开 WorkflowSummary、app/client 转换和控制台；不能只改 JSON 标签而保留中间层旧名称。WorkflowSummary 的 StepID/Status 同样改为 WorkflowStage/WorkflowStatus。

恢复检查中的 DeliveryStatus/delivery_status 当前是 finish_task_status 的别名，删除此别名，改为直接提供 WorkflowStage/WorkflowStatus；恢复界面显示“执行/流程”并读取原流程事实。其他恢复字段不改。

worker_state 事件 from/to 的业务字段也改为 WorkflowStage / workflow_stage、WorkflowStatus / workflow_status；同步活动查询/解码，保持事件身份、顺序、读取预算和最新主干的可读变化规则：阶段变化时只显示流程变化，阶段不变时才显示状态变化，诊断跟随该条消息。字段改名不恢复旧的双消息展示。控制台原始字段标签和读取用新名称，现行字段文档及测试样本同步更新。

阶段值保持 implement.execute、implement.report、publish、publish.report、await_merge、delivered。流程状态值保持 queued、running、waiting_input、blocked、reconciling、waiting_external、completed、abandoned。对应现有中文翻译保持。

为控制工作量，重命名边界是候选视图与状态变化快照：权威领域记录 WorkflowRunRecord.StepID / step_id、WorkflowRunRecord.Status / status 保持，通过明确映射生成新快照字段。StageInput、StageResult、StepAttemptRecord、ExecutionRecord.CurrentStep、报告/提示中的步骤标识和外部原生协议保持原名。它们属于有明确对象上下文的内部/步骤契约，不在本次批量命名范围。Task、Execution、Decision、Operation 对象内的 Status 也保持。

禁止全局替换 StepID 或 status；按对象及快照边界识别字段。

## 3. 删除 finish_task_status

删除 RunView、CandidateView、CandidateSummary 和转换中的 FinishTaskStatus，删除 reconstruction.finishStatus；不另建同义字段、持久化列或另一套状态机。消费点读取 Task、Workflow、Abandonment、Decision、环境、恢复及执行事实；可复用明确语义的布尔判断，不返回另一个派生枚举。

| 旧字段用途 | 新判断来源 |
| --- | --- |
| 放弃 | 原 Abandonment/流程 abandoned 事实，保留原控制门槛 |
| 完成 | workflow_status=completed；Task 交付独立由 Task/Delivery 决定 |
| 待合并 | 候选业务仍有效，workflow_stage=await_merge 且 workflow_status=waiting_external |
| 待输入/回答 | workflow_status=waiting_input，加原 Decision 类型、状态及同步/异步规则 |
| 阻塞/需处理 | workflow_status=blocked，加原 Task 丢弃、环境、恢复、未知操作及执行异常规则 |
| Task 关闭后的候选归属 | Task 终态及原 Workflow/Delivery/Abandonment，不能把所有历史候选当成完成或待合并 |

不得继续把所有 waiting_external 当成 waiting_pr_merge。已有待合并 PR 的非阻塞异步问题仍独立待回答，不自动改成阻塞等待。closed/discarded/abandoned/completed 的候选不进入有效待合并集合。

原待合并、待回答、需处理筛选和候选继续/退出资格、恢复、反馈行为同步移除派生字段依赖。领取、候选上限、问答、发布、合并核验、唯一交付和依赖解锁门槛保持。不能依赖 UI 文本判定命令资格，也不能因删字段导致历史失败被忽略或关闭任务重新可执行。

## 4. 格式与兼容约束

直接采用新候选快照和 worker_state 事件格式，不实现旧记录读取、迁移、双写、字段别名、旧事件解码或旧协议回退。候选快照没有独立格式版本，不为本次改名新增全局协议；现有活动 V 标识通用事件信封，保持不变。worker_state 读取必须校验 from/to 的已知字段类型及新 payload 中目标阶段和流程状态，字段损坏或目标缺失则登记采集诊断，不将旧 payload 解释为有效新变化。客户端既有严格 JSON 解码拒绝旧候选字段，不增加宽松回退。

领域记录未改名是本次范围边界，不是旧格式兼容路径。历史归档不批量改写；现行快照/事件契约及测试夹具同步更新。既有规格关于保留原字段的要求，在本次候选视图范围内由新命名替代。

不读取、修改或重置 C:\SteerlineRuntime。后续回归使用新建隔离夹具；若需要临时/AI 调试，先运行 scripts/new-debug-release.ps1，生成 C:\SteerlineDebug 下的独立 SteerlineTest 环境。

## 5. 修改量判断

整体为中小规模跨层修改，主要是快照字段改名和派生字段消费点替换；不涉及数据库字段改名、Worker 轮次模型或 UI 布局重构。删除派生字段后的待合并/待回答/终态判断是主要语义风险。

2026-10-10 工作区检索：BusinessStatus 出现在 21 个 Go 文件（11 个非测试），FinishTaskStatus 在 29 个（14 个非测试）；StepID 在 69 个（33 个非测试），但包含大量本次保留的内部步骤。计数相互重叠，不能相加为修改文件数。主要影响 reconstruction、product/query/api、app/client、console、相关测试驱动和少量文档；实际文件数由后续 Plan 逐类型确定。

## 6. 最小验证与交付边界

自动验收 SN-A1–A3 见专属文档。Plan 逐项映射定向场景/命令、环境、时点、通过条件及证据登记位置，补齐需要的断言/夹具；实时验收结果只在专属文档登记。

最小验证覆盖新字段序列化/事件接线、删派生字段后的判断、控制台标签/筛选/动作资格及受影响构建。复用模型/重建/查询/console 隔离夹具，不默认跑全量、L2/L3 或真实模型/PR；领域持久化状态推进不变，无需仅因命名调整重跑完整交付链。若实现意外触及事务/状态机，则先重新明确范围及必要定向验证。无新增独立人工验收窗口。
