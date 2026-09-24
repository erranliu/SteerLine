# Spec：原生任务模型

任务 ID：7；任务名称：原生任务模型；目标：`TASK-CORE`。状态：待实施。配套 [Plan](task-core.plan.md)。本文件只定义该任务，由一个独立实现 PR 交付。

## C1：范围与前置

前置：[中心执行模型](02-execution-model-and-adapter.md) 的 Task/WorkflowRun、SQLite、单写者、Query/Command 和 Operation/outbox 已交付。引用 [原生任务共同约束](native-task-constraints.md) 全部条款；原生模型扩展已有权威，不等待整个远程闭环，也不额外要求独立 4A/4B 先于中心模型落地。

本任务交付原生身份、契约、项目权威、数据库结构和基础只读查询。任务创建并入 DAG、丢弃命令、DAG 计算、原生 claim/交付、项目执行接入与关系图展示均不在本任务范围。

建立后续所需记录的 schema 与约束不等于开放业务写入口；测试通过仓储 fixture 注入数据。新原生项目仅初始化结构和查询，默认 manual/paused，不启用生产执行；正式接入由 [Beads 退出 Spec](beads-exit.spec.md) 验收。

## C2：模型与契约

复用中心执行模型的数据库、项目身份、仓库锁、事件、命令队列及 Operation/outbox。CLI/UI 只查询或提交命令，不直接改业务表；图算法库和渲染器均不是写者。

| 记录 | 必需字段/关系 | 不变量 |
| --- | --- | --- |
| ProjectTaskAuthority | project_id、authority_version、graph_revision | 数据库为权威；配置声明必须与它一致。graph_revision 在创建、丢弃和交付改变图或任务读模型时递增 |
| Task | task_id、project_id、batch_id、title、type、priority、contract_id、status、claimant、version、created_at、discarded_at/reason（若有） | 沿用规划阶段的非负整数 task_id，不在创建时再分配；数字 ID 在规范仓库身份内唯一，多个项目配置指向同一仓库时不得重复 |
| TaskContract | contract_id、目标 repository/remote/branch、spec_ref、plan_ref、common_refs、acceptance_refs、digest | 恰好一个独立 Spec、一个配套 Plan 和至少一个验收引用；common_refs 仅引用共同约束/协议。每个文件引用固定仓库相对路径、完整 Git commit 和 blob 内容 SHA-256 |
| TaskDependency | project_id、predecessor_id、successor_id | 外键必须同项目、无自环/重复/环；业务 API 只在后继创建时添加，丢弃不删除边或释放后继 |
| TaskBatch | batch_id、project_id、schema_version、payload_digest、created_at、command_id | 一次创建并加入 DAG 的事务边界；不是先建任务、再发布的独立状态；批次内局部 ID 到 task_id 的映射持久保存 |
| TaskDelivery | task_id、selected_workflow_run_id、PR 合并证据、recorded_at | 每任务至多一份权威交付；必须关联已选用的运行及核验过的 PR |

Task 状态为 open、in_progress、discarded、closed：创建并入 DAG 时为 open，领取为 in_progress；PR 合并和交付事务核验后变为 closed。open 或 in_progress 在 PR 未合并时可由后继任务的显式命令变为 discarded，丢弃不生成 TaskDelivery、不得复用 ID；active 执行另行保留清理事实。claimant 创建时为空，领取后保留作审计。没有 closed → discarded/open、discarded → open/in_progress、in_progress → open 或直接关闭未领取原生任务的普通命令。

规划数字 ID 在原生任务创建入图前即可存在；本任务复用 [中心执行模型](02-execution-model-and-adapter.md) 的 Task 身份，不负责分配规划 ID，也不读取规划文件。task_id 的规范值是无前导零的十进制数字，不加来源前缀、不重新编号。后继 [固定 DAG Spec](task-dag.spec.md) 在创建事务中使用固定输入提供的同一 task_id；重复数字拒绝。

一个 Task 只允许一个权威活动 WorkflowRun，沿用执行模型的重试资格。不同尝试/候选的身份与任务分开，不能添加限制未来多个 Execution 的 `UNIQUE(workflow_run_id)`。任务重试不改 TaskContract、不重新领取同一任务；不存在领取的新任务通道与重试通道同时派发的情况。

[固定 DAG Spec](task-dag.spec.md) 实现 `ready` 派生查询，满足：已创建入图、open、未领取、类型允许、全部前置有有效 TaskDelivery 且 closed。丢弃的前置使后继保持阻塞，不自动删边或视为交付。槽位、派发暂停、预检失败分别是启动资格，不伪装为依赖边或任务状态。失败、未知结果、等待决策/合并由执行模型呈现。

规格路径必须在目标仓库内，拒绝绝对路径、越界路径及跟随到仓库外的链接。创建入图时读取已提交 blob 核对摘要和验收章节存在；不要求理解自然语言来证明规格充分。领取前与执行端改代码前核对实际文件内容，使用 blob 字节/一致的 Git 规范化读取，不能因 Windows 换行转换误报；不要求执行 base 等于规格引用的 commit。实际规格不符则阻塞，不能静默换成最新版。所有规格与 Plan 必须与该任务同仓库。

任务范围与验收的语义充分性由用户/规格审阅负责。系统检查结构、引用、身份与依赖，不能把通过环检测描述为任务已具备正确业务前置。

## C3：配置与基础查询

新项目只初始化 native 权威，要求干净状态目录并默认 manual/paused。旧 Beads 状态、未知 schema 或配置与持久项目身份不符时拒绝有副作用启动，不因改配置创建第二数据库或转换旧任务。

基础 TaskDetail/ListTasks 返回 task_id/project_id、固定契约/验收定位、status、claim、priority、version、discarded_at/reason、关联运行和已存交付证据。CLI 提供 `task show -task-id <id>` 和 `task list`，均接受 `-config`；详情不调用 bd show。

查询协议为 `native-tasks/v1`，包含 project_id、authority_version、graph_revision、database_revision、sampled_at。数据在同一数据库读取快照中采样；默认每页 100、最大 500。游标绑定数据库采样版本，版本不可读时返回 cursor_invalidated，不能拼接不同版本。

本任务尚未实现 DAG 判定，dependency_ready 和依赖/启动阻塞查询能力显式标为未提供，不返回伪造的 true 或空阻塞列表；[固定 DAG Spec](task-dag.spec.md) 实现后在同一协议能力声明下提供这些字段。

本任务错误至少含 unsupported_schema、contract_mismatch、authority_mismatch、stale_version、cursor_invalidated 及既有存储错误。离线只读快照必须展示采样时间，不能用作启动许可。

## C4：初始化与交接

在 Spec 02 的当前数据库结构上扩展原生记录，复用同一 Task ID。初始建库、schema 校验或写入失败不得留下部分可执行任务；旧 runtime 和数据库不导入。向后继交接固定身份、事务与只读查询。

交给 [固定 DAG Spec](task-dag.spec.md) 的输出为 C2 模型、固定契约/摘要、项目权威 CAS、仓储事务及 C3 查询版本。后继任务实现创建入图、丢弃、领取、交付和依赖字段；本任务不替它们建立执行入口。

## 验收

| 编号 | 场景与通过条件 |
| --- | --- |
| NT01 | 预分配数字 task_id 的 Task 可存取且不重新编号；project 隔离；Task/Execution 身份分离；重复数字拒绝 |
| NT02 | 原生权威持久化；open/in_progress/discarded/closed 与丢弃审计字段准确，discarded 无交付且 ID 不复用；旧状态和配置冲突拒绝有副作用启动 |
| NT03 | 单 Spec/单 Plan 与共同引用校验；缺少 Plan 或把多个任务 Spec/Plan 装入同一契约均拒绝；revision、摘要、验收引用、越界/缺文件检查正确，CRLF checkout 不误报 |
| NT21-CORE | 当前格式内 Task/运行/PR 关联和数据投影重启后保持一致；无第二任务权威，重复初始化不重复创建 Task。 |

本任务的全部验收必须在一个独立实现 PR 内完成；配套 Plan 只负责本任务。证据不足不得标为完成。
