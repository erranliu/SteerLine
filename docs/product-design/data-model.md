# 任务、执行与交付数据模型

SteerLine 与 SteerRunner 分别单写自己的 SQLite。当前只验本机 Codex；用户已取消本机与远端 Orca 的本次验收。历史 Orca 结果保留，不推断为通过。repository 的 remote 指 Git 托管来源。

SteerLine 尚未发布。项目显式使用 task_authority=native 与 workflow_protocol=worker-flow-v2，全局执行配置为格式 3；旧数据库、配置和内部执行格式直接拒绝，不迁移、双写、回填或回退。

## 权威与身份

SteerLine 自有库保存 Task、固定 TaskContract、DAG、候选参与关系、原 CLI outbox、公开 Runner 事件副本、唯一 TaskDelivery、测试生成原请求与结果、原候选测试收集请求及不可变产物和自身服务 checkpoint。schema 26 不含旧 WorkflowRun/Execution/Attempt/Decision/Operation 或原生会话写表。测试生成记录不创建 Candidate/Task；候选 payload 另固定 Plan 测试快照。产品仍使用的纯值声明不构成执行写权。

Runner 自有库保存 Run、冻结材料、workflow 输入/结果、Attempt、原生资源、问题、输入、恢复与停止事实。SteerLine 不读该库、原生 codec、IPC 或私有日志；只消费固定 EXE 公布的 CLI/Run/事件。

项目以规范化 Git common-dir 标识；数据库另有稳定 database_id 与 database_revision。Task version、authority_version、graph_revision、候选 revision 和 Runner revision 分属不同权威，不能互相代替。读页面/资格/候选在同一 Task 库事务中采样；服务进程活性与 checkpoint 内容分别核实。

候选 ID 与 Runner Run ID 分开。候选原子领取时固定完整 Spec/Plan 引用、原预设 ID/名称/参数摘要、原执行/流程配置、目标、原 request_id/body 与共同 base；响应丢失只查原请求。项目 checkpoint 固定 Runner EXE/version/SHA/resource_digest/state-dir，候选同时记录该构建绑定；instance_id 仅属于本次 ProjectRuntime，不永久写死到 Run。接受后 Run、输入摘要、分支和 PR 绑定不能替换。候选参与状态不冒充 Runner 原生执行状态。

人工候选退出在原候选 payload 固定 `withdrawal`（request_id、expected_revision、requested_by、at），与 `participation=abandoned` 同事务保存；原请求重放不重写首次事实。`stop_requested` 专指独立的 Runner 停止义务，不由人工退出设置。退出与名额统计在写事务中重检，至少保留一个参与候选；Task 终态之后仍可另行增加停止义务，保留原 withdrawal。当前 Web 及调用方验证见[专属验收](../acceptance/web-candidate-stop.md)。

## 固定契约与领取

当前规则（2026-10-21）：[项目基线与材料维护](project-maintenance.md#task-材料更新与首次启动冻结)分开保存“Task 最近确认材料”与“首次预留不可变执行契约”。目标 repository/remote/branch 不可借材料确认变更；未预留任务按 Task version 确认内容。首次 fresh fetch 后同 commit 解析引用/测试快照，在原项目库事务重核实际消费的确认，固定唯一 Task 快照、base、候选/outbox；不跨应用文件/项目库伪造事务，不比较无关 Plan/轮询时间。后续候选/拒绝/未知重放沿原版本；原 base 被历史改写排除时拒绝新候选而不换基线。材料规则不迁移旧库或改历史。材料依据和确认请求分别保存于 maintenance_approvals/maintenance_requests，唯一执行快照保存于 task_execution_snapshots；Generation payload 保存不可变输入、待审阅结果和逐次单文件应用意图/事实。

TaskContract 固定目标仓库、remote、base 分支及 Spec/Plan/其他材料的 commit、路径和 SHA-256。启动前核对 Task 版本、DAG 就绪、authority、图版本、目标仓库及前置合并是否进入当前 base；最终领取事务再次进行同图 CAS。领取与完整 CLI outbox 一次提交，失败零派发。正常 implement/report/publish 不创建额外 DAG Task。

同一 Task 的候选共享原 base。新的参与候选只生成自己的 caller 候选 ID、原始提交和 Runner Run，不能用旧候选换绑出新执行。旧提交未知、迟到接受和终态后接受仍沿原记录对账。

## 输入与恢复

CLI 请求 ID 和正文不可变，同 ID 换正文冲突。SteerLine 保存原 host 控制信封和 caller 语义摘要，回执对外保留原信封身份；不能以重新序列化或替换 ID 掩盖未知结果。

原生问题、反馈和 resume 由 Runner 公布资格、原问题/Attempt 和 revision。SteerLine 校验当前候选及 Runner 版本，再持久保存输入 outbox。中心重启将旧未发输入撤销或仅观察原请求，不能自动重授旧答案/反馈/resume。已接受与未知输入只对原请求查询。

产品 Runtime 和 RunView 是只读 UI 投影，由 Task 图、service checkpoint 和公开 Runner facts 生成；不从旧执行表重建，不读取原生文本推断业务结果。Runtime 不保存旧 WorkflowRun/Execution/Attempt/Decision/Operation 集合，RunView 不承载旧工作区意图或人工停止确认；持久化只使用独立 ServiceCheckpoint，候选与停止事实仍来自 caller 记录和公开 Runner 投影。

## 会话与事件

公开 Runner 事件按候选绑定、事件 ID、游标和版本事务保存。完整 final 保留来源、原 session/turn/item/Attempt 与正文 hash；重复事件去重，冲突拒绝。缺 final 不推进业务，日志和模型总结不能代替来源核验。正文分页读取只读副本。

Runner 状态使用[短目录规则](../integration/steerrunner.md)：Windows 为本地卷根 `slr/<128-bit base32 key>`，key 绑定含 home 的完整项目 run-dir；候选保存准确 state-dir。材料、缓存、工作区和日志仍按项目独立，业务库继续在原 home。旧 `run-dir/runner` 不迁移或兼容读取。重置须先证明 Runner 服务退出及原 Run 停止，只清理本项目摘要目录，不用父进程死亡或窗口关闭代替停止。

## PR 与交付

代理/用户给出的 URL 只是线索。Runner 核对固定 provider、仓库、base/head 与当前执行分支；Task 交付前通过 CLI 再刷新 PR。Task 版本、候选绑定、原 PR 归属和真实 merged/merge commit 在提交事务重核。

Task 关闭、唯一 TaskDelivery、依赖后继释放和同组候选停止义务一次提交；失败全部回滚，重复读取和迟到事件不重复交付。不要求候选先到 pr_ready，也不因兄弟候选未知阻塞正确已合并候选。

作废同样固定原请求与任务版本；未知 PR/执行事实保持明确阻断。Task 终态保存每个候选的停止义务，迟到接受的原 Run 仍须撤销输入并停止。逻辑撤销、服务停服与原生停止分别取证；本次无中心 PR/分支/工作区删除状态机。

## 当前格式来源

| 格式/权威 | 当前值或归属 | 源码 |
| --- | --- | --- |
| Task SQLite | schema 26 | [sqlite/reader.go](../../internal/storage/sqlite/reader.go) |
| 全局执行配置 | version 3，固定 Runner EXE/version/SHA | [app/runner_config.go](../../internal/app/runner_config.go) |
| Runner 公开协议 | steerrunner-cli/v1 | [runnercontract](../../internal/runnercontract) |
| TaskContract / DAG | Task 自有纯值 | [task/model](../../internal/task/model) |
| 候选/outbox/公开投影 | caller 参与关系 | [runnerstate](../../internal/runnerstate) |
| 单事务候选领取 | 图/任务版本 CAS | [runner_candidates.go](../../internal/storage/sqlite/runner_candidates.go) |
| 交付与后继 | Task 自有事务 | [runner_result.go](../../internal/storage/sqlite/runner_result.go) |
| 公开事件 | 游标和事件 ID 去重 | [runner_events.go](../../internal/storage/sqlite/runner_events.go) |
| 执行 workflow / 原生格式 | Runner 独立 owner；development/1 | [Runner 接入](../integration/steerrunner.md) |

节点和异常规则见[生命周期](worker-lifecycle.md)，配置见[执行方案](execution-profiles.md)，实际通过/未验见[接入验收](../acceptance/steerrunner-cutover.md)。历史 Spec 的迁出前源码链接固定在原 Git 提交，只作追溯。


环境检查另存 environment_checks / environment_check_requests：项目、原 start/stop、固定 Runner pin、公开输入/首个权威摘要和最近快照；本地 CAS revision 与 Runner revision 分开。它们不生成 Task、候选、交付或新的执行状态权威，亦不推进任务数据库 revision。结果未知只读原请求，reset 必须保留未知操作及未结算资源的恢复记录。[接入规格](../spec/environment-check.spec.md)与[验收](../acceptance/environment-check.md)分别定义行为与实际前置状态。
