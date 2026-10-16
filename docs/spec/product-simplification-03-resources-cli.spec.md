# Spec：保留资源人工处置与精简命令入口

2026-10-14 用户修订：任一已绑定候选 PR 核验已合并即可提交唯一交付并将 Task 转为 closed；其他候选已合并/状态未知或阶段推进不阻塞该候选交付。closed 后停止该任务全部候选的 PR 状态查询，重启保持。下文“所有候选 PR 可核实且只有一个合并”以及 closed 后迟到合并监控的历史要求不再适用；当前规则见[数据模型](../product-design/data-model.md#人工候选实验)，验证见[专属验收](../acceptance/candidate-first-merged-close.md)。

2026-10-13 用户修订：完整移除单候选放弃功能（控制台、CLI、API、服务端及专用状态字段）；任务级丢弃和唯一交付保留。控制台任务提示只显示中文状态，并增加全部执行候选数量列。下文涉及单候选放弃或旧列表列布局的历史要求不再适用；当前操作以[执行控制台](../archive/design/execution-console.md)为准。

目标：`PRODUCT-SIMPLIFICATION`；批次：3。状态：实现、质量审计及最小验证完成，交付见PR #238，未分配 Task ID；本批[实施 Plan](product-simplification-03-resources-cli.plan.md)、[独立实现 PR #238](https://github.com/erranliu/SteerLine/pull/238)。依据：2026-10-07 用户要求编写审计后的三批规格，覆盖修订后的取舍 3、8。

参考源码基线：`75474dcece26d807367e106a89a992f01f53fafd`。硬前置：[第二批](product-simplification-02-configuration.spec.md)已通过[PR #236](https://github.com/erranliu/SteerLine/pull/236)合入；实际实施基线记录前两批合入后的提交。遵守 [第一批](product-simplification-01-console.spec.md)“目标与共同边界”。

单候选退出后保留 PR/worktree 已是当前行为，本批不重复宣称交付此能力。实际新增取舍是：整个 Task 丢弃后也不自动关闭 PR、删除工作区或分支，由人处置；精简重复 CLI 和高级 DAG 查询/文件导出。

本 Spec 替代 [固定 DAG](../archive/spec/completed/task-dag.spec.md)、[原生任务共同约束](native-task-constraints.md)及原接线中 Task 丢弃后的破坏性自动清理要求，以及 [关系视图](../archive/spec/completed/dag-view.spec.md)的高级筛选/专用导出要求。丢弃资格、停止/对账、必要协议收尾、唯一交付与基本 DAG 行为继续成立。历史 Spec 不回写。

## 退出、丢弃与人工处置

1. 单候选放弃继续以精确 Task/WorkflowRun/版本/原请求提交。只退出该实验，禁止新业务推进/反馈，保留历史；停止未知不回滚放弃、不阻塞兄弟或新增候选。Task 丢弃继续核验全部已知候选 PR 和未结算动作，已合并拒绝，未知沿原请求待核实；提交后保留契约、边、领取与运行，不生成交付、不解锁后继、不自动重新领取。
2. 候选放弃、Task 丢弃和获胜/落选均不授权中心关闭 PR、删除 worktree、删除分支或清空原日志/证据。Task 丢弃事务不再生成 `pull_request_close`、`workspace_cleanup` 自动待办；删除相应消费循环、删除/关闭副作用、进度回执与无消费者字段。只保留其他仍有消费者的纯查询/资源事实。
3. PR URL/历史关联、原后端与目标、工作区路径或原生 ID、分支、业务状态和实际 worker 状态仍在候选详情可定位。显示“保留，需人工处置”及采样时间；“保留”不是已确认存在，“人工处置”不是业务阻塞状态。人工删除后未经观察不能显示已清理，用户文字不代替 provider 合并事实。基本详情在资源缺失时仍能读取历史定位。
4. 沿既有后端能力停止本次明确 owned 的原执行，停止请求/回执与业务退出分别保存。结果未知只查原请求，不重复 kill，不影响兄弟资源；不支持远端取消时保持未核实/需人工停止。服务停止、观察断开、候选退出和 worker 实际停止继续分别表达，不删除执行停止与恢复身份核验。
5. 保留消息消费 ack、worker 原生结算/release、协调终端排空后的协议关闭、已创建连接/句柄关闭、必要探针/反馈临时文件收尾。适配器名为 Cleanup 的动作须按实际资源逐项分类，不能整类删除；它们继续沿原 intent/receipt/ack 屏障，未知只对账。协议收尾不得删除业务 worktree、分支、PR 或证据。
6. 退出/丢弃与后台启动、反馈、停止竞争时，晚到回执仍保存原资源与投递事实，继续观察必要输出和已知 PR，不复活候选、发新业务输入或新建替代 worker。Task 丢弃后发现 PR 合并仍记录冲突并要求人工处置；放弃候选的合并冲突、获胜后的迟到合并均不覆盖唯一交付或再次解锁后继。
7. 保留资源不是活动执行，不因 workspace/开放 PR 人工处置未完成就占用派发槽或阻止所有兄弟。反之实际活动/未知执行仍按原规则占用与阻断相关动作，不能借删除 cleanup_pending 虚构已停止。后继资格只按 Task/规范 Delivery 判断。
8. 不建设中心统一手动清理按钮、删除请求、垃圾回收器或人工清理结算状态机；用户在提供方和执行目标原生工具中处置，中心只保存/展示已有事实。测试环境清理本次拥有的 scratch/进程/PR 是测试基础设施职责，继续保留，不按产品取舍删除。

## 命令收口

命令入口固定如下，不以名称相似删除语义不同的入口，不取消脚本调用与原请求对账能力。

| 入口 | 处理 |
| --- | --- |
| `console` | 保留第一批轻量控制台及项目选择 |
| `run` / `start-service` / `stop-service` | 保留前台宿主、受管启动和确认退出三种不同语义；通用 `control` 中服务控制仍是同一命令通道 |
| `control` | 保留首候选/新增/放弃、派发与环境动作、原 request_id 查询/等待；身份/版本/回执规则不变 |
| `answer` | 保留精确原候选/Decision 的文字/选项/备注反馈，不另造第二种答案协议 |
| `task validate/create/discard/add-predecessors/show/list` | 全部保留；第 6 项“固定依赖”未获采用，补前置资格和事务不变 |
| `task graph` | 只保留按项目的有界分页节点/边读取，参数为 config/cursor/limit；不再支持 task-id/status/direction/depth 邻域组合 |
| `check`、`home`、`install`、`launchers` | 保留必要诊断、安装与定位能力 |
| 顶层 `tasks` / `stop` / `projects` | 删除重复列表、停止别名和交互式项目菜单；分别使用 task list、stop-service、console 或显式 config |
| `task export` | 删除独立格式/输出路径/覆盖参数及专用原子文件发布；基本查询继续输出 JSON，用户可自行重定向保存 |
| `certify-backend` | 已由第二批退役，本批核对 help/文档无遗留，不再次计算删除收益 |

`codex-session-launch` 等有实际内部消费者的启动入口、测试 runner、原生 probe 不属于可删别名，保留精确调用。删除公开旧命令，不留兼容 alias 或转发；调用时给出未知/已退役错误，在任何副作用前结束。

基本 task show/候选详情继续显示直接前置、后继、dependency_ready、未满足前置/丢弃前置、startup blockers、Spec 和 Delivery 定位。从同一数据库快照查询，保留分页、采样版本及截断边界；不把未展示节点说成完整图，也不以离线 Ready 授权启动。

删除高级图投影/过滤契约及独立文件导出中没有消费者的内容。保留任务核心环检测、稳定顺序、依赖资格、基本图分页和补前置；不为删展示查询改写业务算法、整图权威或后续规划准入机制。

## 所有权与格式

重点收口 `internal/execution/execution_discard_cleanup.go`、native cleanup/丢弃提交、`workspace` 删除路径、`publication.Close`、model/storage cleanup 字段及 `cmd/steerline`、product/taskgraph/query/client。

`publication.Close`/关闭命令配置、worktree/分支删除接口仅在已无产品消费者时删除；provider 的查询、认证引用、Git remote/base/branch 绑定与 PR 历史核验保留。必要协议收尾留在各 adapter，不将远端路径交给中心本机文件 API。测试自身关闭/删除资源的工具由 testkit/E2E 所有者继续提供，不新增测试专用生产路径。

实施 Plan 固定需变更的记录/配置/产品协议版本；旧开发格式在副作用前拒绝并保留现场，不回填人工清理状态、不迁移、双写或回退。更新 README、CLI/help、启动器说明及受影响测试 fixture；历史 Spec 只由本 Spec 明确覆盖，不重写历史验收。

## 最小验收

| ID | 场景与通过条件 |
| --- | --- |
| PS3-01 | 两候选中放弃一个及单独丢弃整个 Task，均不调用 provider Close/worktree Remove/branch Delete；资源/PR定位保留，兄弟不被清理，后继不因丢弃放行 |
| PS3-02 | 停止支持/未知/不支持如实显示；丢回执和重启沿原请求；晚到启动/反馈保存真实资源但不复活；必要 ack/release/协调收尾仍确认原身份 |
| PS3-03 | 放弃/丢弃后 PR 合并和多合并保持冲突；正常唯一交付只解锁一次；人工资源处置未完成不冒充活动执行或阻断无关候选 |
| PS3-04 | 退役命令及高级参数在副作用前拒绝；保留 CLI、内部启动和 test driver 接线可用；基本任务关系/分页/离线采样正确，补前置规则未退役 |
| PS3-05 | 一个 controlled 交付/等待合并重启代表场景通过；资源保留不改变交付，测试自身清理仍完成，无产品 Close/Delete 调用 |

PS3-01–04使用 provider/资源调用计数、可控双候选与远端协议、临时 SQLite、公开 CLI 和当前格式重启代表测试；复用已有未知停止、迟到回执和唯一交付断言，不穷举交错。PS3-05选 `delivery.basic` 或 `recovery.waiting-merge-restart` 中最贴合受影响路径的一项；仅当两种边界分别改动且无法由定向测试覆盖时才补第二项。

实施 Plan 按 [测试指南](../testing.md)和 [E2E 约定](../../tests/e2e/README.md)列最小测试计划，包含级别、组/场景 ID、环境、命令、时点、通过条件及证据位置；一次 L1、最小定向回归及上述一个 L3 controlled 场景。无原生协议行为变化不默认 live；若实际修改 release/ack 等原生动作，补对应目标最小证据，不能以 fake 声称原生通过。不默认 Full 或完整 L2。证据根目录：`docs/acceptance/product-simplification-03-resources-cli/`。

## 交付边界

提交删除/保留消费者和副作用分类清单、三批同口径净行数汇总。候选退出原本保留资源、第一批共享阅读代码、第二批认证命令的收益不得重复计算。不实施依赖冻结、版本提升简化、规划/审核外移或新的原生 UI；这些不是本系列已采用取舍。

实施、消费者分类、格式、失败说明与三批行数见[验收记录](../acceptance/archive/completed/product-simplification-03-resources-cli/README.md)。
