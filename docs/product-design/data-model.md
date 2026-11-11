# 任务、候选与交付数据模型

本文维护调用方实体、身份与事务不变量。SteerLine 与 Runner 分别单写自己的数据库，只通过公开契约协作；正常运行不读取、迁移、双写或回退旧开发格式；显式放弃旧运行的只读归档例外见[安装部署](../integration/install-deploy.md#显式放弃旧运行后升级-runner)。当前格式唯一定位在[源码表](#当前格式来源)，历史字段及版本见[历史入口](../../SteerlineFlow/archive/design/runner-reliability-history.md#第四批整理来源)。

## 权威与身份

| 实体或事实 | 负责方与存储范围 |
| --- | --- |
| Task、TaskContract、DAG 与 TaskDelivery | SteerLine：目标、依赖、固定依据、项目权威和唯一交付 |
| Candidate 与原请求 | SteerLine：参与、独立冻结快照、原提交和控制 outbox、停止义务、公开 Run 投影及事件副本 |
| 维护、生成、环境和 Roadmap | SteerLine：各业务活动、固定输入、结果、当前引用及正式入图映射 |
| 宿主 checkpoint | SteerLine：服务身份、构建绑定、关闭及锁事实，不替代任务或执行状态 |
| Run、Attempt、原生资源、问题和逐轮输入/结果 | Runner：执行与恢复事实；SteerLine 不读其数据库、codec 或原生日志 |

项目以规范 Git common-dir 标识，数据库保留 database_id / database_revision。Task、authority、graph、候选和 Runner revision 分属不同事实，不互相代替。候选 ID 与 Run ID 分开，原生会话/轮次也不作为项目任务身份。

产品 Decision、Runtime 和 RunView 是公开事实与任务/checkpoint 的投影，不恢复旧中心 WorkflowRun/Execution/Attempt/Decision/Operation 写表或原生执行状态机。显示名、缓存、配置和诊断不成为第二业务权威。

## 仓库与工作区身份

项目状态位于仓库外的受管目录；同 common-dir 的 linked worktree 共用仓库身份。项目运行时持有仓库和状态目录锁，不允许另一 home 或 linked worktree 绕过同仓写者；数据库身份、格式及归属在业务写入前核验。现有 `.beads`、`.beads.db` 或 `beads.db` 状态阻断原生初始化，保留供人工处理，不自动删除或迁移，见[状态前置](../../internal/app/host/native_state_preflight.go)。

Runner 按调用方固定 commit 创建专属工作区并管理原生资源。候选保存公开路径、分支/base 和 Run 绑定，不能把旧工作区重建成同名新执行。成功新建后的启动条件与未知创建的恢复分别按[生命周期](worker-lifecycle.md)处理。

应用 home 内项目共用一个 Runner 服务和准确 state-dir，任务库仍按项目隔离。目录布局与清理目标归[路径清单](../../SteerlineFlow/paths.md)，执行准备归[Runner 接入](../integration/steerrunner.md)。未知请求、原资源或停止义务未核实前不删除恢复依据，不用进程退出或窗口消失代替资源证明。

## 候选参与与原请求

预留事务同时核验 Task/图版本、依赖、当前项目资格和材料，并固定候选自己的 Contract、base、材料摘要、测试快照、预设 ID/名称/参数摘要、Runner 构建和完整原提交。项目最新材料与候选快照分开；新候选采用当次有效版本，原号重放不升级旧输入。具体新鲜度和准备要求归[项目维护](project-maintenance.md)。

接受后 Run/input_digest、分支及 PR 不能替换；实例 ID 属于当前 ProjectRuntime，不永久写死到历史 Run。构建摘要记录来源，实际服务准入及 v8 升级依当前公开契约判断，不由历史摘要授予权限。

人工退出将不可变 withdrawal（原 request_id、显示 revision、操作者及时间）与 `participation=abandoned` 同事务保存。当前参与与名额在事务内重核，至少保留一个参与候选；`stop_requested` 是独立 Runner 停止义务，人工退出不设置它。Task 终态仍可为已退出候选保存停止责任，原 withdrawal 保持。

产品请求与 Runner 请求分别持久化，完整正文和映射不可变；同 ID 改正文冲突。外部调用前保存意图，保存失败零发送；外部成功而本地失败只核实并补保存原事实。原提交未知、迟到接受及终态后接受均沿原记录结算，不换候选或新请求重做。

问题、反馈和恢复输入固定原目标及当前资格后保存控制 outbox。重启撤回未发授权，未知沿原请求观察，不自动重授；原生投递事实归 Runner。会话、事件和游标按绑定及版本同事务保存，正文引用与阅读规则归[会话设计](execution-observability.md#公开结果来源)。

## PR 与交付

Agent 或用户给出的 URL 只是线索。Runner 核验工作流发布；Task 交付前由 SteerLine 按候选冻结的 provider 直接查询平台原 PR，核对仓库、base/head、原绑定及真实 merged、merge commit 和时间，再在本地事务重核 Task、图和候选参与/版本。

唯一 TaskDelivery、Task 关闭、依赖释放及全部候选停止义务一次提交，失败回滚；重复原交付返回原事实，不重复事件或改变回执版本。不要求先到 pr_ready，不因兄弟查询未知、候选材料过期或新材料准备失败阻塞合法交付；使用所选候选原契约。

作废也绑定原请求及任务版本；未知 PR/执行事实保持明确原因。Task 终态后停止其全部候选的 PR 查询并拒绝新业务输入，迟到接受仍须绑定并结算原停止义务。参与退出、逻辑取消、服务退出和资源停止分别记录；无调用方 PR/分支/工作区删除状态机。核验链路归[可靠性交付](../architecture/reliable-delivery-chain.md)。

## 辅助业务记录

| 业务 | 关键不变量 |
| --- | --- |
| 项目材料与测试生成 | 维护记录保存最新材料及生成来源；Generation 固定输入与结果，不创建 Task/Candidate，不写旁置安排文件。候选引用自己的完整测试快照及进度副本 |
| 项目环境 | 活动与 Current 分开，绑定项目、环境身份、依据摘要、原 Run/request/input 和封存结果。项目写 ready/message/details，平台只核验信封与结算；依据变化持久撤销 Current，回退旧字节也不恢复资格 |
| 项目测试入口 | version1 定位公共材料和正式测试清单；解析后的来源随生成与候选冻结。环境 manifest 与其独立，不把任务测试目录作为环境检测要求 |
| Roadmap | 解析、确认和文档生成记录保存原来源、输入及请求；解析/确认不分配正式编号。发布先核实固定文档 PR 的实际合并，再通过 Task 批次事务入图 |

Roadmap 编号按仓库权威从已有最大 Task 之后递增，不回收或补洞。编号、Task/Contract、依赖、来源/内部代号映射和原批次回执同事务提交或回滚；不同来源的同名 T1 互不混用，正式文档路径与原 Markdown 不回填编号。细节归[Roadmap 设计](roadmap-promotion.md)。

候选中的环境免检标记只记录预留事实，由 writer 的可信策略确定，不能反过来授予免检资格；配置和策略规则归[开发者配置](execution-configuration.md#开发者配置与环境就绪阻塞)。辅助活动的停止和 reset 仍覆盖原未绑定未知请求，不因没有 Candidate 而遗漏。

## 当前格式来源

| 格式或事实 | 当前来源 |
| --- | --- |
| Task SQLite | [reader.go](../../internal/storage/sqlite/reader.go) 的 `SchemaVersion = 46`；数值只在此表定位，正文不保留旧版本副本 |
| 项目契约配置 | [app/config.go](../../internal/app/config.go) 要求显式 `task_authority=native`、`workflow_protocol=worker-flow-v2`；后者不代表 Runner 当前 workflow 版本 |
| 全局执行配置 | [executionconfig](../../internal/executionconfig/config.go) 的严格解码与版本校验；构建选择由 [app](../../internal/app/runner_config.go) 校验 |
| TaskContract 与 DAG | [taskcontract](../../internal/taskcontract) 的结构及 [task/model](../../internal/task/model) 的规则 |
| 候选、outbox 与公开投影 | [runnerstate](../../internal/runnerstate/candidate.go) |
| 原子预留、交付及事件 | [候选事务](../../internal/storage/sqlite/runner_candidates.go) · [交付事务](../../internal/storage/sqlite/runner_result.go) · [公开事件](../../internal/storage/sqlite/runner_events.go) |
| checkpoint 与宿主事实 | [hostcontract](../../internal/hostcontract) |
| Runner 公开协议及执行格式 | [runnercontract](../../internal/runnercontract) 与所选固定构建的公开能力；workflow、CLI 和自身存储分别版本化 |
| 进度、项目接口与结果 | [进度通道](../integration/runtime-progress.md) · [环境工作流](../../SteerlineFlow/spec/project-environment-workflow.spec.md)；各格式由原 owner 维护 |

历史数字、字段和旧验收结论按[原版本来源](../../SteerlineFlow/archive/design/runner-reliability-history.md#第四批整理来源)取证，不能据此读取旧库或回填运行。本文不证明安装版已部署或各原生路径已通过验收。

## 异步活动只读投影

当前活动来源为检测、显式主分支检查、候选、Roadmap 和提示词建议；原测试准备批次及关联已退役。`source_observations` 独立保存最近成功观察及错误，不递增业务版本或赋予执行权限。

独立阅读页的 `reading_fields` 只保存原记录字段指针、来源身份、512 字节预览、长度/摘要和显示元数据，正文仍归原持久记录；其拥有者在原事务维护索引。`reading_versions` 只为跨来源关联的读取索引提供廉价变化标记，不是业务 revision、消息增量流或执行 checkpoint。`message_observations` 仅按候选原 Run/Attempt/session 保存最后一条合法 assistant 样本及错误，不保存采样历史、不授予执行能力。新 schema 拒绝旧格式，不增加迁移或双写。

查询返回来源事实，由产品 query 只读聚合；不存在统一活动业务写表或调度器。完整职责与接入约定见[异步活动中心](async-activities.md)。
