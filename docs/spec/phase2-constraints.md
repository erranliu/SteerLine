# 第二阶段共同接口约束

本文件是独立 Spec 可引用的共同约束，不是执行任务，不承担多个实现 PR 的合并验收。每个任务各有独立 Spec、Plan 和实现 PR；交付范围与硬前置见各任务 Spec。

## 权威与固定依赖

Go 服务复用现有 SQLite、QueryService/CommandService、Operation 和规范事件/正文存储。UI 是 C# WinUI 3 客户端，主 agent 仅支持本机 Codex app-server；任务 worker 使用 #36 持续会话的 local Codex / Orca 后端。规划 session、任务 Execution 和外部 thread 身份分开。主 agent 不直接启动任务、创建 DAG、运行实现或调用子 agent。

适用[执行共同约束](execution-architecture.spec.md)、[原生任务共同约束](native-task-constraints.md)、[服务生命周期](../archive/spec/completed/pr-03-service-lifecycle.spec.md)、[会话契约](../archive/spec/completed/pr-04b-conversation-events.spec.md)、[固定 DAG](../archive/spec/completed/task-dag.spec.md)。当前开发版本不建设旧状态迁移或自动覆盖；当前格式重启、幂等、交付核验必须保留。引用历史 Spec 时只复用当前有效接口，历史兼容说明不扩展本组任务范围。

所有未完成前置均是同项目 AND 硬依赖：前置真实交付且领取 base 含其成果后才可执行。公共接口在本组 Spec 中定义，不把任务内部的接口里程碑当成 DAG 放行事件。无依赖关系只意味着可以独立开发，不改变现有执行槽数量或调度策略。

## 公共传输契约

本机产品协议版本为 steerline-client/v1；HTTP 路由以 /v1/projects/{project_id} 为根。该版本与 Codex JSON-RPC、Orca 和 #36 的 worker 协议版本分开。服务发现和认证由本机客户端协议任务实现；业务 DTO 和路由由所属领域任务实现，传输层不提前伪造未交付领域。

读响应携带 project_id、service_instance_id（离线为 null）、sampled_at、来源标志 live/offline、对象版本；多对象结果区分 database_revision、document_version 与各自游标，不制造跨 Git/SQLite 的全局原子快照。列表 items、next_cursor、has_more 显式返回；游标失效返回 cursor_invalidated，客户端刷新而不拼接新旧分页。

所有业务命令包含 request_id、project_id、actor（服务校验的本机用户身份）、目标对象和 expected_version、领域 payload。相同 request_id 和规范 payload 返回原记录，不同内容拒绝 idempotency_conflict。重连后按原 ID 查询；响应为 submitted/accepted/applied/rejected/outcome_unknown 之一及 error、领域 result。accepted 不表示副作用或 agent 已应用。未知动作在对账前不得重新发起。

客户端在发送前持久保存完整命令信封及摘要，区分仅本地已保存与服务已登记。对原项目权威查询明确返回 command_not_found 时，可以将原 ID、原 payload、原 expected_version 幂等补交到同一项目权威；不得换 ID、自动改版本或借此重复调用 Codex/Orca/Git。服务必须先以唯一请求记录完成仲裁再执行任何外部动作，并发旧发送和补交只能产生一个业务操作。已有 accepted/outcome_unknown 回执只对账；连不上服务或身份不符不视作未收到。此规则覆盖发送前客户端退出、请求未送达、服务登记前中断和登记后响应丢失。

SSE 只通知持久变化，带可查询的事件身份/游标；先入库后通知，历史不足时明确 reset_required 并重新查询。各领域在自己的业务提交后产生事件，客户端补读结果为准。错误至少含 code、message、object_ids、field，UI 按 code 分支。没有能力返回 unsupported，未知结果返回 outcome_unknown，冲突返回 stale_version/workspace_conflict；不得用空成功代替拒绝。

## 领域归属及最小交接

| 领域 | 实现归属 | 交接对象 |
| --- | --- | --- |
| 身份发现、认证、执行查询/命令、SSE 传输、离线只读承载 | 本机客户端协议任务 | 现有任务/图/会话 DTO 及命令回执 |
| 规划 worktree、提案、ID 预留、草稿、文档读模型、共享写入仲裁 | 规划提案与受控草稿任务 | PlanningWorkspace、Proposal、DraftFile、DocumentSnapshot、GoalRef、写入 guard |
| Codex 进程、thread/turn 绑定、规范会话事件、审批与输入 | 规划会话任务 | PlanningSession、PlanningTurn、InputReceipt、Approval；使用共享 guard |
| 固定版本发布、Git/中心创建对账、正式文档版本推进 | 正式文档发布任务 | Publication、固定 commit/manifest、task batch 回执；使用共享 guard |

PlanningWorkspace 绑定项目、规范仓库、固定 worktree 路径/分支、当前正式 document_version、写入 owner 和 generation。Proposal 含 proposal_id、mode（documents_only/task_batch）、version、base_document_version、goal_refs、预留 task_ids、文件清单和状态。DraftFile 含服务分配 file_id、正式目标路径、正文摘要与版本。DocumentSnapshot 以固定 Git commit 和路径/blob 清单定位一套正式内容；GoalRef 显式关联稳定 goal_id 与文档路径/锚点，不以标题相似性推断身份。

PlanningSession 绑定 workspace_id 和实际 Codex thread_id；PlanningTurn 绑定 session、generation、外部 turn_id、请求、状态与来源能力。Publication 固定 proposal/version、基线、完整 manifest 摘要、人工确认记录、原 request_id、Git 操作与中心 batch 操作身份。领域任务可增加实现必需字段，不能改变以上身份或交接语义；共同变更须同步所有消费者规格。

## 共享写入与读取规则

规划提案任务拥有唯一的项目规划写入仲裁。agent 轮次、草稿编辑、预留、文档发布和基线同步均经该入口，服务单写；session 局部锁不能替代它。owner/generation 与未完成动作持久化，获取和释放均在服务事务内校验；运行时文件访问再验证 token，旧进程/迟到工具请求不能凭旧 token 写入。发布必须等待活动轮次结算，不抢占锁或仅靠 TTL 判断旧执行消失。

服务启动先恢复未完成的规划副作用与原 owner，再开放新写入；无法核验原进程或发布结果则保留阻塞和只读历史。正式文档查询只读已发布 commit/blob，不直接读正在逐文件更新的 worktree；离线只读入口同样读取已提交版本。外部 worktree 修改引发 workspace_conflict，阻止发布和新写入，保留差异，不自动 stash/reset。

原始 worktree 路径不承诺给外部无锁进程提供多文件原子切换；服务查询的正式版本必须始终完整。Codex 只读仓库的轮次在发布/恢复期间禁止启动，已有轮次结算后才可切换文件，避免看到半套正式路径。

## UI 共用规则与验证

项目选择、导航、输入框与本地视图状态由 WinUI 基座提供；后续页面使用同一导航与客户端实例。未发送输入按项目/session 保存，阅读位置与跟随状态独立；界面缓存不赋予操作资格。UI 关闭不停止服务、agent 或 worker；离线时展示固定服务窗口入口，用户主动重开服务。客户端不能自动开替代服务、升级权限或访问 SQLite/私有日志。

共享 Go 接线由后合入任务复核当前接口并跑受影响回归；共享 WinUI 导航在基座交付后接页面，不复制项目选择和连接状态。真实 Windows 验收固定构建、平台、身份和证据路径；模拟测试只证明列明范围。每项实现交付自己的验收表、可重复命令及未测项，行为、状态和最小结果记录于 docs/acceptance/<任务文件名前缀>.md，原始证据放临时目录。完整产品闭环另由[二期验收活动](../acceptance/phase2.md)承担，不进入执行 DAG。

本组涉及 Go 生产代码的实现 PR 完成定向测试后运行 go test ./... -count=1 -timeout=45m -p 1 -parallel 1、go vet ./... 及 Windows/Linux Go 构建；本组将既有共同约束的 10 分钟全套上限明确改为 45 分钟，以容纳现有 Windows 串行套件，不减少用例。仅 C#/UI 改动运行原生构建、对应 UI/契约测试和真实 Windows 验收；不宣称 Linux UI 通过。纯规格编写只做身份、链接、引用方向、依赖与语义检查，不运行实现测试或宣称平台能力通过。
