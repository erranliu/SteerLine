# Spec：Runner 准备查询等待与执行页展示

日期：2026-10-25。状态：SteerLine 调用方已实施；Runner #57 已合入，正式构建的扫描/只读/原触发路径受控复验通过，真机联验待执行。历史隔离尚未唯一归因，验证及限制见[专属验收](../testing/acceptance/runner-wait-retry.md)。关联 [Plan](../plans/runner-wait-retry.plan.md)。普通开发，不修改 roadmap、不恢复生产 Task 18 或部署。

## 1. 目标、权威与独立边界

SteerLine 调查基线 `9607fb0099dfc00d9f3de35429d4c43fcdc80407`，Runner 基线 `af5e244ddae05d4c19069852d56c5a74a68ab1ce`。当前问题是准备查询在 CLI 排队阶段耗尽旧 15 秒预算，尚未启动模型便进入 blocked/人工继续。目标是显示真实等待、只读重试和耗尽原因，并保持原操作资格。

唯一跨仓契约来源为 Runner 的 `docs/spec/orca-cli-wait-retry.spec.md`，契约 `steerrunner-execution-wait/v1`，配套同目录 Plan；本次编辑仓库 `C:/Project/SteerRunner`。开发前固定实际文档/构建/schema 版本。Runner 独立实现和验收，无须 SteerLine 才有收益；本仓仅消费公开状态，不在 Runner 完成条件中附加 Web 联验。

审计将首版收敛为 remote repository_inventory / local runtime_status 两项准备读取。共用 CLI 同进程 FIFO、单层读取重试归 Runner；不扩大到写操作预算、通用三类调度、后台采样退避或未知结果的新状态机。

复用 [worker 生命周期](../../docs/product-design/worker-lifecycle.md) W02/W04/W08–W11/W18、E01/E03–E05/E07 与 V01/V03/V06–V08；恢复按[协作契约](runner-collaboration.spec.md)、[服务换代契约](service-generation-recovery.spec.md)和[可靠交付架构](../../docs/architecture/reliable-delivery-chain.md)。未知创建/发送/停止继续沿原操作核验，不能因本项自动重投 submit/reply/resume。

## 2. 所有权与范围

| 所有者 | 职责 |
| --- | --- |
| Runner | 原 CLI 通道公平等待、两项准备查询的预算/重试/持久恢复、controls、公开同快照事实及原资源核验 |
| SteerLine 后端 | 能力/schema 验证、原 Run/实例/revision/CAS 投影、现有 API 与活动映射 |
| Web | 等待/重试/耗尽文案、源时间/近似时间与陈旧状态；沿原 controls/Decision 显示操作 |

不新增 SteerLine 重试器、wait 队列/表/Decision、专用执行端点或第二权威状态。Task/DAG、候选参与/胜负、PR、交付及资源结算规则不变。范围仅为开发候选摘要/详情和活动；生成队列、environment-check 和 Runner Viewer 改版另议。

## 3. 公开契约与投影

在原 ProjectRuntime 初始化核验固定 Runner EXE/实例的 capabilities.execution_wait：contract 匹配、same_snapshot=true、run_property=execution_wait 及自包含 schema。不每次刷新探测，不因支持声明改绑运行中的替代实例。真实接入固定 source/EXE SHA/version/resource 与公开样例。

不支持本契约时明确显示能力不足，拒绝本项所需的新 Orca 接入；其它 adapter 依原能力独立判断。已有候选的只读/停止沿其固定构建与原资格，不重提候选。无旧开发记录迁移、缺字段回退或英文错误推断。

Runner inspect.RunView / observe.run / history 完整 RunView 的 execution_wait 是对象或显式 null。本仓沿原 observe 同快照/CAS 保存，对外仅增加 `CandidateSummary.runner_delivery.execution_wait`，不在顶层重复一份。Go/HTTP/TS 保持原字段，不读 Runner SQLite 或导入其私有 Go 包。

| 字段 | 消费约束 |
| --- | --- |
| contract / operation_id | steerrunner-execution-wait/v1、当前原准备操作；不是新的查询操作或候选 |
| query | repository_inventory / runtime_status |
| stage | queue / calling / backoff / exhausted |
| reason_code | orca_cli_busy / orca_query_running / orca_query_transient / wait_budget_exhausted / query_attempts_exhausted |
| started_at / updated_at / deadline_at | 原窗口起点/最后持久更新/有效截止 UTC 时间；GET 不刷新源时间 |
| calls_started / max_calls | 本准备查询的保守调用边界次数/上限，非模型轮次或写操作次数 |
| next_attempt_at | 仅 backoff 为下一读取时间，其余 null |
| diagnostic | 去敏、限长原原因，仅展开显示，不驱动状态或控制资格 |

组合约束按 Runner schema：阶段/原因对应；queue 可以有过查询，不能推断次数为 0；backoff 有至少一次且尚未耗尽；exhausted 不安排自动读取。无 disposition/effect/dispatch_state 或新查询子操作。窗口复用原父调用的 90 秒上限、单次最多 15 秒、5 次、1/2/4/8 秒退避由 Runner 保存，原准备/caller context 更早期限优先。Web 使用实际字段，不写另一套预算或截止裁决。

身份错配、revision 倒退或坏对象不保存；保留最后有效快照，经原查询错误/连接诊断通道显示不可核实，不写入 Runner diagnostic 冒充执行事实。不用缓存冒充新成功观察，命令仍核验新鲜资格。

未知副作用、权限/配置问题、停止及原人工问题继续使用原 Run/operation/controls 和现有 UI；不为它们新增平行 execution_wait 状态。预算内准备等待不生成 runner_resume Decision，模型是否启动须另有原生事实。

## 4. 界面与操作

| 事实 | 默认显示 |
| --- | --- |
| queue | 等待 Orca 调用通道；本次查询已等待多久，Runner 将自动继续 |
| calling | 按 query 显示正在查询 Orca 仓库/运行状态，不冒充模型执行 |
| backoff | 正在重试查询；已尝试 n/max 次、约多久后再查询 |
| exhausted | 按 reason_code 显示等待预算或查询次数耗尽；原 controls/Decision 决定能否继续 |
| null | 移除当前等待块，历史活动保留 |

原 unknown 仍显示核实原结果，权限/身份/人工问题沿原 blocker；不会出现“重发输入倒计时”。列表复用短标题，详情展开原 Run/operation、源时间和 diagnostic。没有队列排名、确定 ETA 或锁持有者猜测。

时间显示复用现有时间格式/可见性和查询缓存：有可信后端采样基准才结合本地单调增量显示近似倒计时，否则显示源绝对时间。不新增后端时钟协议。next_attempt_at 或 deadline 到达而无新快照，只显示等待 Runner 更新，不能自行判失败、清空对象或开放继续操作；耗尽时间停在 updated_at。

查询失败/断线/契约异常保留最后有效内容并标最后已知/不可核实；陈旧期间不显示确定重试倒计时。项目/候选/实例切换按原查询作用域隔离，旧响应不覆盖新对象。每秒更新文字不增加网络请求或业务 revision，关闭页面不停止 Runner 等待。

自动等待不要求点继续；manual 恢复仍通过原 Decision/recovery/controls、原 Run/Attempt/revision 信封与 outbox，重复点击复用原 request，绝不自动答复或 resume。没有新增“取消等待”写接口：退出候选 participation 仍是 stop_candidate，只有原 Runner stop 义务才取消未调用读取并结算资源。accepted 不代表资源已停。

## 5. 事件、重启与验收

消费原流的 execution_wait_changed，Event 顶层 execution_wait 为对象或清除时 null，其它事件不附带。不新建 payload/游标/日志服务；原 Run revision、controls 和事件同快照校验/CAS。重投幂等，解除只移除本项显示，不覆盖 stop 或其它问题。无每秒 tick。

宿主/Web 重启只恢复观察，deadline/计数/next time 由 Runner 保留；不重新提交输入或自动继续旧答案。原 TaskDelivery、参与关系及 pending stop 不受等待字段影响。

| ID | 自动通过条件 |
| --- | --- |
| SLW-A01 | 能力/schema、对象/null、原绑定/revision/CAS 正确；坏对象保留最后有效投影 |
| SLW-A02 | queue/calling/backoff/耗尽/清除全链路可见，零额外候选/输入/自动答案或 resume |
| SLW-A03 | 文案/时间/次数、到期等待、断线/陈旧/时区/切换正确，旧 unknown/blocker 显示保留 |
| SLW-A04 | 自动等待不强制继续，人工仍原资格/信封/幂等；候选退出与 Runner stop 区分 |
| SLW-A05 | 宿主/Web 重启保持原身份/预算/事件，迟到或替代实例不能覆盖/推进 |
| SLW-A06 | 本仓独立包与固定真实 Runner/Orca，>15 秒竞争后 API/Web 观察同候选自动恢复、取消/清理正确 |

公开 fixture 固定 Runner 合入源码 `5df57b69e98382abcd63676b04a73079fd939fa6` 的 schema。正式构建已在本仓新独立包完成远端 Orca 的 >15 秒等待、同候选/Run/Attempt 自动恢复、Edge 文档重开、原 Run 停止及宿主/offline 结算；SLW-A06 归本仓，不倒置成 Runner 独立交付前置，也不扩张为本机 Orca、真实 Web 服务进程重启或上游完整原生矩阵通过。历史 Defender 告警尚未唯一归因，若遇新拦截须停止并保留失败；人工体验可后置。固定身份、失败、实际执行及未验范围由 Plan 和专属验收登记。
