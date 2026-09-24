# Plan：原生任务、固定 DAG 与 Beads 退出

依据：[Spec](native-task-dag.spec.md)。实施基础：[中心执行模型](02-execution-model-and-adapter.md)、[共同执行约束](execution-architecture.spec.md)。本 Plan 覆盖 TASK-CORE → TASK-DAG → BEADS-EXIT → DAG-VIEW，按四个可独立审核的实现阶段推进；当前文档编写不代表开始实现或完成任何验收。

## P0：确认基线与接口职责

1. 从当时最新主线读取前置实现及 Spec，确认中心 SQLite、Task/WorkflowRun、QueryService、CommandService 和 Operation/outbox 已交付，并核对其复用的双后端契约；不额外要求独立 4A/4B 阶段先于中心模型落地。前置未完成时只做文档/样本准备，不先造第二套存储或调度器。
2. 记录数据库、runtime/候选投影、命令、工作流、会话、技能上下文的实际版本和支持矩阵。为新增业务结构分配未占用版本；task-batch/v1、native-task/v1、native-tasks/v1、native-task-graph/v1、beads-import/v1 按 Spec 固定，不与既有协议版本混用。
3. 盘点当前 `beads.go`、`service.go`、`config.go`、`workspace.go`、`codex.go`、`completion.go`、`prompt.tmpl`、`result.schema.json`、`control_service.go`、`retry.go`、viewer 和 CLI 中的任务源耦合。前置重构已移动文件时，按职责更新清单，不把旧文件名当作创建重复实现的理由。
4. 固定接口交接：CORE 修改 Task/来源/权威；DAG 修改发布、ready、claim、delivery 和图查询语义；EXIT 修改接入/导入；VIEW 只消费查询。与远程闭环共同使用的 AttemptSpec/任务上下文由既有接口扩展，保留现有身份和消息大小限制。
5. 准备相互隔离的临时 Git 仓库、测试数据库、PR 平台替身和 worker 调用记录器。真实验证另用专门测试仓库/worktree，不能领取生产任务；把拆分、回退和任意依赖修改列为无产品入口的边界检查。

产物：版本/兼容矩阵、现存调用与目标归属清单、API/Schema 样例及测试场景，随阶段实现保存到 `docs/validation/native-task-dag/`。不得用准备文档代替已交付前置。

## P1：TASK-CORE——任务身份与存储

1. 在中心领域层定义 Spec N2 的原生 Task、TaskContract、ProjectTaskAuthority、TaskBatch、TaskDependency、TaskDelivery 和 ExternalTaskRef。先建立模型及外键/唯一性约束；DAG 的业务写入在 P2 开放。不要把 Beads DTO 直接重命名成原生模型。
2. 复用原数据库迁移框架，加入权威和契约记录；已经有内部 Task ID 的历史按来源映射复用。无数据库旧 runtime 的导入仍走 Spec 02，不新增另一种恢复路径。
3. 实现 task_source 配置分流、authority_version 匹配及新原生空项目初始化。默认旧配置保持 beads，新项目显式 native；原生执行生产入口暂不启用，初始化为 manual/paused。记录 membership，历史只读对象不得进入原生 ready。
4. 实现规格文件引用、Git blob 摘要、章节与仓库边界校验；固定契约序列化和 canonical JSON/digest 规则，覆盖重复键和 CRLF checkout。文件系统读取不得超出绑定仓库，远端工作区读取保留在执行适配器。
5. 增加 TaskDetail/ListTasks 和 `task show/list`，复用中心只读服务。返回 N5 的版本、来源、规格、交付/运行关联；原生详情不调用 bd show。此阶段使用仓储 fixture 创建测试记录，不提前暴露绕开发布的任务写入口。
6. 更新实际模型/数据契约和配置文档，说明 native 与 beads 的权威范围及历史只读语义。与中心模型共同审阅 schema、旧运行关联和不兼容客户端拒绝。

完成门槛：NT01–NT03 及本阶段相关 NT21 有证据；数据库失败/重复迁移不改变既有历史；原生身份与外部 ID 分离。合入后才进行 P2。

## P2：TASK-DAG——发布、领取和交付

1. 实现 task-batch/v1 严格解码和引用校验。固化 A → B → C、菱形汇合、断开组件、循环/悬空/重复/跨项目边和规格错误的样本；不引入父子节点、自动拆分或修图算法。
2. 接入内部 GraphAlgorithms 适配层。核验 dominikbraun/graph 的发布版本和必要 API，固定 go.mod/go.sum 与版本证据；封装环路径、稳定遍历/拓扑行为，不将库类型写入数据库或 DTO。不因第三方排序不稳定改变候选优先级。
3. 增加 ValidateTaskBatch 和 PublishTaskBatch，CLI 使用中心命令通道传入固定 payload。批次映射、任务、契约、边、事件、revision 和命令回执同事务提交；实现同 request_id 同 payload 复用及异 payload 冲突。
4. 实现同项目硬前置查询和稳定候选顺序。把 dependency_ready 与资源/暂停/能力阻塞分开，并返回具体前置。发布成功不主动启动 worker；明确 auto 模式下正常轮询可开始领取。
5. 将 ready/claim 准入接到既有 start_task 与自动派发路径，复用槽位与命令生命周期。固定主线 base、核验前置成果可达后，事务保存领取、运行和外部意图；提交后走原工作区/启动操作。不得把 claim 提交当作 start_task applied。
6. 增加中心原生交付事务：选用结果和固定门槛、精确 PR/merge 核验 → TaskDelivery/closed/WorkflowRun completed/事件提交。取消原生路径对 Beads close 的依赖；不要把完成写成后继状态批量更新。
7. 通过平台/worker 替身验证重试、迟到消息、领取竞争、提交前后中断、回执丢失和 unknown 守卫。控制端不得因测试简化而添加通用 task.status 更新入口。
8. 提供图查询的公共 DTO、database revision/graph_revision 和分页失效语义供 VIEW 使用；只固定业务查询，不在此阶段渲染图。更新中心状态/数据契约文档及 CLI 示例。

完成门槛：NT04–NT11、相关 NT21 通过。此阶段隔离闭环证明 DAG 规则，不宣称真实无 Beads 项目已经交付。合入后进行 P3。

## P3：BEADS-EXIT——原生接入与一次性迁移

### P3a：无 Beads 的新项目

1. 按调用清单清除原生分支的 bd/bv 预检、数据库身份核验、BEADS_DIR/BEADS_ACTOR 注入、旧提示词与旧 completion。Beads 兼容分支维持原契约，不能统一删掉导致未迁移项目失效。
2. 将 native-task/v1 任务上下文接入 Spec 02 的 minimal-v1/project_steps。明确仓库/主线/base、task/contract、claim、run/execution/attempt 身份；实际工作区证明走现有适配器。旧整段 start/finish 不自动变为分步骤实现。
3. 提供已提交到测试仓库主线的新项目配置及项目步骤样例，验证 implement/verify/review/publish_pr 的边界。审阅委托技能/脚本，排除 bd/BV、聚合/后继解锁和必须执行的旧结项副作用；不靠标记或纯提示词规避真实依赖。
4. local 与已支持本机 Orca 各跑一次隔离原生任务闭环，记录版本、工作区/base、步骤回执、PR 和合并核验证据。受监管远程已上线时追加同一接口回归及 Spec 03 必需门槛；未上线时不启用该能力，也不把未执行写成通过。
5. 无 bd/bv 的验证环境中，创建 A → B、启动 A、完成发布，等待用户合并后确认中心关闭 A 和 B 就绪。在领取/等待合并阶段重启并验证恢复。合并不是测试程序或 agent 的自动动作。

### P3b：旧项目导入与权威切换

1. 实现只读 Beads 导出器与 beads-import/v1。先通过真实安装版本的 help/JSON 样本确认命令和依赖关系语义，再固定可支持版本/样本；不读取内部数据库，不静默丢弃未知边类型。
2. 导出完整未完成任务与前置闭包、ready、原状态/元数据和规格映射；生成可以人工补充历史交付依据/accepted_baseline 的输入。为聚合、自定义后继、外部 assignee、缺规格/前置、重复 PR 和无证据 closed 准备拒绝样本。
3. 实现 dry-run，输出来源/目标、摘要、ready 差异、ID 复用映射、历史基线和拒绝原因。确认中心已暂停且旧运行/回调/未知动作全部结清；暂停不能替代资源/外部副作用核对。
4. 实现导入命令及报告/差异确认参数，提交前复核源摘要，事务内重核中心版本和静默条件。任务/边/来源/基线、迁移标记与 native 权威一次提交；数据与命令幂等以相同快照和来源身份验证。
5. 覆盖部分写入故障、提交后失联、同快照重复、源在预演后变化、配置仍为 beads 和投影失败。数据库已切换时禁止回到旧来源；修正配置后仍暂停，由用户显式恢复派发。
6. 在隔离的旧项目副本演练迁移并与原 ready 比较，核验旧 Task/WorkflowRun/PR 身份未重建。原生后续运行用调用记录器断言无 Beads 读写；最终无 Beads 环境验证不能仅靠 mock 返回成功。
7. 随实现更新 skill-contract、completion-command、state-model、data-contract、worktree-preparation、README 和配置示例。说明旧项目兼容条件、原生项目契约及不支持的导入结构，不为人工拆分/回退增加产品入口。

完成门槛：NT12–NT17、相关 NT21 通过，NT12 的真实平台证据齐全；有未验证环境时明确限制该环境启用，不能以本机证据代替远端。新项目不依赖迁移工具安装 Beads；导出器只是旧项目切换工具。合入后进行 P4。

## P4：DAG-VIEW——关系查询与只读图

1. 基于 P2 DTO 实现项目/状态/任务邻域查询，完成 version/cursor、过滤边界、深度和数量限制；在同一数据库采样事务读取节点、边与交付，不拼接不同版本的查询。
2. 控制台增加任务前置/后继、具体阻塞、规格与交付来源详情，保持稳定 task_id 选择、离线时间及历史来源区别。复用 CONSOLE 查询入口，不按每个节点调用 Beads 或平台。
3. 实现 JSON 快照和 DOT 导出，明确它们不可作为任务发布输入。添加纯文本转义、边界提示、超限错误、临时文件原子发布及显式 overwrite，失败保留已有输出。
4. 可选调用 Graphviz dot 生成 SVG，参数数组和 stdin 传入，固定 30 秒/20 MiB 限制；阻塞操作在查询/渲染客户端或独立工作单元进行，不进入调度服务循环。缺工具仅阻塞 SVG；不增加 Web 服务和交互编辑器。
5. 用 100/500/1000 节点链/汇合/不连通图、中文长标题和恶意文本样本生成并人工查看 SVG；核验布局、箭头、截断/过滤信息、时间版本及运行详情一致。记录 dot 版本、耗时、体积；失败预算路径也须验证。
6. 发布关系查询和导出用法及可复现样例，把真实导出样本与验证报告链接起来；不将静态图标成实时调度结果。

完成门槛：NT18–NT20、相关 NT21 通过，控制台既有查询/会话行为不回归；渲染工具缺失不影响执行。

## 验证与故障注入

| 层次 | 必需证据 |
| --- | --- |
| 数据/协议 | Schema 解码、引用/哈希、同项目外键、唯一约束、canonical digest、权威/版本和全部拒绝条件 |
| 事务 | 发布/领取/交付/导入每个事务提交前后进程中断及写入失败；比较数据库与 worker 调用次数，不只断言报错文本 |
| 外部副作用 | 领取后准备失败、启动响应丢失、PR 查询未知、旧尝试迟到；不重复派发、不提前交付，固定身份和现场保留 |
| 迁移 | ready 差异、闭包完整性、历史 Task/PR 映射、人工基线来源、特殊结构拒绝、数据库提交但配置/投影未更新 |
| 查询与显示 | 相同读取快照图/列表一致、游标失效、过滤边界、离线显示、中文/转义、DOT/SVG 预算和缺工具降级 |
| 真实闭环 | 无 Beads 的 A → B 开发/发布/用户合并/核验、local 与已支持 Orca、中心重启；记录实际平台与工具版本 |

每阶段先运行有针对性的包/集成测试，再运行 `go test ./... -count=1 -timeout 10m`、`go vet ./...` 与 `git diff --check`；适用平台补充 race 检查和原生构建。文档编写阶段只检查链接、引用方向及规格一致性，不声称运行了上述实现验收。

验证材料位于 `docs/validation/native-task-dag/`，报告逐项对应 NT 编号，包含环境、输入摘要、命令、运行/任务身份、实际结果与证据位置。不可执行项记录原因，不以 skipped/fake 充当真机通过。

## 完成与交接

- 四阶段各自按验收门槛交付并审核；同一阶段允许拆成多个实现提交，但不能删除验收或绕过前置。
- 完整 diff 核对所有原生路径的 Beads 调用均已退出，兼容调用只在未切换项目中保留；CLI/UI/worker 不直接修改业务表。
- Spec、输入输出样例、实际协议版本与实现文档一致；所有 NT 条目均有证据，目标环境缺口明确限制启用范围。
- 本阶段不实现拆分、回退、父子聚合、动态依赖编辑、并行选择器或交互式 Web 图。完成 Task 交付不限制用户在系统外决定人工处置方式。
