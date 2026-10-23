# Plan：原生任务模型

任务 ID：7；任务名称：原生任务模型；目标：`TASK-CORE`。唯一执行依据：[Spec](task-core.spec.md)，共同要求见 [原生任务共同约束](../../../spec/native-task-constraints.md)。前置为中心执行模型完整交付，本 Plan 仅实施原生身份、存储与基础查询。

## 基线与接口准备

1. 从当时最新主线读取前置实现及 Spec，确认中心 SQLite、Task/WorkflowRun、QueryService、CommandService 和 Operation/outbox 已交付，并核对其复用的双后端契约；不额外要求独立 4A/4B 阶段先于中心模型落地。前置未完成时只做文档/样本准备，不先造第二套存储或调度器。
2. 记录当前数据库、runtime/候选投影、命令、工作流、会话和技能上下文的接口。固定 native-tasks/v1 基础查询协议；后继命令/导出协议由各自 Spec 定义，不在本任务实现。
3. 盘点当前 `beads.go`、`service.go`、`config.go`、`workspace.go`、`worker_types.go`、`completion.go`、`prompt.tmpl`、`result.schema.json`、`control_service.go`、`retry.go`、viewer 和 CLI 中的任务源耦合。前置重构已移动文件时，按职责更新清单，不把旧文件名当作创建重复实现的理由。
4. 固定本任务输出的 Task/权威与基础查询边界，记录后继对事务和身份的需求；不在本任务实施创建入图、丢弃、派发上下文或图形展示。
5. 准备相互隔离的临时 Git 仓库、测试数据库、PR 平台替身和 worker 调用记录器。真实验证另用专门测试仓库/worktree，不能领取生产任务；把拆分、回退和任意依赖修改列为无产品入口的边界检查。

产物：当前接口清单、现存调用与目标归属清单、API/Schema 样例及测试场景，随本任务实现保存到 `docs/validation/task-core/`。不得用准备文档代替已交付前置。

## 实施步骤

1. 在中心领域层定义 Spec C2 的原生 Task、TaskContract、ProjectTaskAuthority、TaskBatch、TaskDependency 和 TaskDelivery。建立外键/唯一性约束；已规划数字 task_id 沿用中心模型的同一 Task 身份。DAG 的业务写入由后继 TASK-DAG 实现，不把 Beads DTO 直接重命名成原生模型。
2. 扩展当前 SQLite 结构，加入权威和契约记录；在干净状态目录初始化。旧 runtime、旧数据库和 Beads 数据不导入，检测到时拒绝有副作用启动。
3. 实现 native 项目初始化和项目身份匹配；原生执行生产入口暂不启用，初始化为 manual/paused。
4. 实现单 Spec、单 Plan 和共同文档引用校验；按共同约束固定契约序列化和 canonical JSON/digest，核对 Git blob、验收章节与仓库边界，覆盖缺 Plan、重复键、多 Spec/Plan 拒绝和 CRLF checkout。文件系统读取不得超出绑定仓库，远端工作区读取保留在执行适配器。
5. 增加 TaskDetail/ListTasks 和 `task show/list`，复用中心只读服务。返回 Spec C3 的版本、规格、交付/运行关联；原生详情不调用 bd show。此阶段使用仓储 fixture 创建测试记录，不提前暴露绕开后继原子创建的任务写入口。
6. 更新模型、数据契约和配置文档，说明单一 native 权威、干净目录初始化及旧状态拒绝。与中心模型共同审阅 schema 和当前运行关联。

## 验证与完成

本任务验收：NT01–NT03、NT21-CORE。重点验证单一任务权威、预分配数字 task_id 不重新编号、重复初始化与故障原子性、规格文件摘要/路径/换行、旧状态拒绝和只读查询版本；先运行针对性测试，再按 [共同约束](../../../spec/native-task-constraints.md) 的验证原则运行完整 Go 测试、静态检查及差异检查。

证据保存在 `docs/validation/task-core/`，逐项列出验收编号、输入摘要、工具/协议版本、命令、实际结果与证据位置。未运行项不得以 skipped/fake 记为完成。当前文档调整不代表实现验收通过。

本 Plan 的所有实施步骤构成同一个任务，由一个独立实现 PR 交付；不与前置或后继任务合并，也不把内部步骤各自当成独立交付 PR。完成自己的全部验收、兼容回归和文档更新后再交付，不代替后继任务验收。
