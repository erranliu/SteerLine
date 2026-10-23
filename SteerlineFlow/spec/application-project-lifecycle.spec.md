# 应用与项目生命周期收拢 Spec

2026-10-16，用户直接授权的普通开发。基线 `ae6673e864437de32fd264b8842d7204f2694006`；本版已按审计收窄，配套 [Plan](../plans/application-project-lifecycle.plan.md) 和[专属验收](../testing/acceptance/application-project-lifecycle.md)。不进入 roadmap。

## 1. 目标、收益与范围

沿用[统一启动](unified-startup.spec.md)的共享后端、独立 Web、项目独立 actor/store/Runner 拓扑。本批交付五项可核验成果：

| 成果 | 结构收益 |
| --- | --- |
| 删除无生产消费者的逐项目 start/run/window/listener 链 | 当前产品只有统一启动拓扑，删除旧资源 owner 与专属测试 |
| ProjectRuntime 持有全部项目资源 | store、锁、日志、product/handler 与 Runner 生命周期各有一个最终关闭者 |
| 构造失败保留部分取得信息，共用收尾 | 不再由 factory、actor、运行时分别清理；不通过 bind 回调偷渡 store |
| 分开本地关闭与 Runner 结算 | Runner 未证不误报 store 尚未关闭；同一个 error 不再决定全部停机字段 |
| 提取只读 frozen catalog | verify-stop 不构造活动目录；Close-before-Start 不隐式启动 |

不新增恢复机制、生命周期数据库、通用资源注册框架、逐项目用户启停、热加载或前端功能。不重写失败停机保留锁/宿主的策略，不增加锁释放后的第二套证明，不改变 Runner 原停机 request_id、重试和最终结算契约，不设计新的两阶段停止回执。

## 2. 重构前问题与目标所有权

现有 `application_directory.go` 混合配置快照和活动目录；Close 调 Start，缺结果兜底全部 true。`project_runtime.go` 多个失败分支自行 Close，`runner_service_wiring.go` 又开关 store/Runner；`Actor.Close` 混合 Runner 错误和 store.Close，且与 EndActor 重复收尾。`app.go`、`execution_service_wiring.go`、`host/service_client.go` 保留旧单项目进程启动链；当前 applicationclient 和进程内 httpapi 已拒绝 stop_service。

| 所有者 | 保留职责 |
| --- | --- |
| 应用组件协调 | 后端/Web start、status、stop，精确进程与 home 租约核验 |
| ServeApplication | 应用端点、目录、最终 shutdown.json 与宿主退出；失败保留所有权策略沿用 |
| ApplicationDirectory | 固定成员、身份普查、冲突检查、有界启动、全体停止门槛、原行结果汇总 |
| ProjectRuntime | 项目资源取得/运行/失败收尾、Runner 停机与结算协作、借用调用排空及最终锁释放 |
| Task actor | Task/DAG/候选/原请求业务单写、ready/最终 checkpoint；借用 store，不拥有 Runner 停机与 store.Close |
| runnerclient / 进程工具 | 既有公开协议、原身份/EXE/目录证明与实际 OS 操作；不持有项目业务写权 |

只在现有 app 包中收拢资源，必要时使用相邻 `project_resources.go`、`project_runner.go`。不为少量固定资源增加通用接口层。产品客户端和 handler 对项目 store 均为借用关系。

## 3. 目录与启动

提取 `loadApplicationCatalog(home)`，仅返回固定配置集合、正文、全局配置快照及读取诊断，不创建 context、goroutine、锁、目录、store 或 Runner。活动目录构造与离线 verify-stop 都消费这个快照；后者不构造 ApplicationDirectory。

ApplicationDirectory 的 Start 幂等；BeginStop/Close 不启动项目。Close-before-Start 明确记录本次未取得资源并结束。每个配置行的停机事实明确初始化或由真实 runtime 返回；运行中的缺结果为未完成，不以零值推导三个 true。配置坏行/未启动行的零义务仅指本宿主本次未取得任何资源，不能掩盖已尝试或已接管 Runner 的不确定结果。

保持现有最多 512 项、1MiB 配置限制、30 秒/8 并发身份普查、全体冲突检查、项目独立准备/初始化预算和 manual 默认。项目失败只结束该项目，兄弟继续；应用 stop/信号才围栏全体。所有 I/O 和等待在目录锁外。

项目运行入口在任何资源取得前建立 owner，顺序如下：

1. 解析冻结字节、复核身份、取得 repo/run-dir 锁，再复核路径；沿现有 preflight 拒绝非法/旧格式。
2. 在已获锁的 owner 中初始化或打开 store，记录所有权；移除初始化短暂开关库后再次组装开库的重复接线。
3. 取得日志与固定 Runner 客户端，沿主干 `runnerstatepath.Prepare/Directory` 使用项目独立短目录，执行现有 start/复用与 catalog/profile 校验；错误也保留已知 CLI/实例及是否取得收尾义务。
4. factory 只接受借用的 store/CLI 并组装 actor/service；构造函数不 Close、不另起 listener。
5. actor 执行既有恢复围栏并发布 ready checkpoint，项目 handler 才发布；停止中迟到回调不能恢复可写状态。

每项取得后立即登记到 owner。正常退出、任意取得点失败和初始化取消统一进入同一收尾函数。新建 Runner 成功后 catalog 失败仍清理原实例；已有 Runner 尚未通过接管校验时不擅自停止。启动未知没有可信实例时保留未知，不补启动、不停止不确定目标、不补造 checkpoint。

## 4. 停止与调用排空

整体 BeginStop 先封闭全体项目/启动中项目的发布与新业务门槛，再取消原业务 context；不等待某项目收尾才通知兄弟。已受理请求保持原身份、事务和迟到接受停止义务，继续采用 taskrunner/runnerbridge 的既有围栏。

唯一项目收尾顺序：

1. 关闭业务资格，等待 actor 循环结束；actor EndActor 仅保存本地 stopped/ready=false checkpoint，不承担 Runner/store 关闭。
2. 封闭并排空借用项目资源的 Read/Command/Task 调用，再关闭 handler/product 的自有资源。进程内端口使用简单 RWMutex/等价现有机制；Identity 仅返回原安全身份。不能仅依赖 socket HTTP Shutdown。
3. 项目 owner 按原 Runner 绑定办理停机与公开最终结算；沿用已确认停机后仅补读证明、停机未知时保留原 request_id 的规则。
4. 无论 Runner 结果如何，都尝试关闭实际取得的 store/日志。重复收尾返回同一个关闭结果，不重复本地 Close。
5. 本地关闭成功后按现有锁 API 释放 repo/run-dir 锁；本地关闭未知继续保留锁。生成原项目结束结果，目录只汇总。

Read/Command/Task 的门槛必须覆盖已经取得旧 port 引用的调用；关闭等待持有资源的实际调用完成，不能因客户端取消就认为后台使用者已经退出。只读查询可能刷新既有公开观察并落库，因此同样属于 store 借用者。

`LocalClosed` 仅由本地资源关闭事实决定；`ResourcesSettled` 只由原公开最终证明或本次明确零义务决定；`LocksReleased` 沿原本地成功释放路径。Runner 错误不阻止本地 Close，不从 error 文本推导布尔值。启动/业务失败保留在目录/日志；若已经完成全部收尾，不必永久阻止宿主干净停止。实际收尾失败进入 shutdown Diagnostic。

本地未关闭时沿用当前保留必要锁和宿主租约、stop 超时返回未核实的策略；本批不增加重试、修复、解除等待入口或中间 shutdown 覆盖协议。本地已关闭、Runner 未证则允许本地锁释放和失败结果退出，原资源恢复继续受既有授权围栏约束。

## 5. 原请求与最终证据

当前项目 stop_service 产品入口已经退役。核对引用后删除其旧 PID 停止等待、文件停止采样和专属回执接线，不将它改写为新两阶段提交，也不映射为全局 StopAll。有效 start_task/answer/resume 等控制文件、原回执和业务事务保持。

Runner 停机原 request_id、固定实例/EXE/state-dir 和 last_shutdown 校验复用现有实现。原业务请求零重发及迟到 Run 停止义务不变；不以旧 running/stopped 投影、空候选列表或自然语言替代最终资源证明。环境检查资源由 Runner 自身结算，SteerLine 不重建 native 矩阵。

沿用 `Shutdown/ProjectShutdown` 当前格式及最后停机结果，不增加持久状态或格式迁移。status 展示原事实，正常 stop 要求原结果、精确进程退出、租约均合格；verify-stop 继续用显式暂存 Runner 核验原 config/checkpoint/实例/锁/Run/input_digest，旧失败和 DB/配置不改写。

应用 start 180 秒、stop 调用 120 秒、verify-stop 60 秒与原 Runner 单调用预算保持。收尾使用独立有界 context，业务取消不使 cleanup 立即失效；重复等待不生成新停机身份。

## 6. 删除与保留

删除 app 旧 StartService/RunConfiguredService、ServiceLifetime、serviceLocalAPI 和旧 window 启动闭包。保留 host 当前控制收发、连接观察、原格式拒绝、锁、launcher 管理及精确进程能力。引用检查包含生产、测试、平台/build tag 和脚本；仍覆盖当前契约的测试迁至实际 owner。

不要求机械拆分 application_start.go；只提取实际共享的只读快照/校验。不得以新增类型、文件数量或行数减少单独宣称收益。

## 7. 验收与完成

统一 worker 规则仍归 [worker-lifecycle](../../docs/product-design/worker-lifecycle.md)，关联 W01、W08–W14、W17–W18 / E04、E05、E07 / V01、V03、V06–V08。本次只调整调用方资源/停止接线，不改原生恢复和流程。

| ID | 通过条件 |
| --- | --- |
| ALC-A1 | 无生产旧单项目启动/listener/停止链；只读 catalog 与 Close-before-Start 零启动；依赖守卫及跨平台生产/默认测试编译通过 |
| ALC-A2 | 项目 store/锁/log 各一个最终关闭者，factory/actor 无 Close 副本；部分取得与失败清理、重复关闭、本地失败/Runner 未证分别核验 |
| ALC-A3 | 单项目停止不影响兄弟、整体门槛关闭后迟到 ready 不复活；借用调用完成后才关库；原请求/PR 绑定/Runner 最终证明语义保持 |
| ALC-A4 | 明确区分 never-started/pending/finished 结果；缺结果不冒充成功，失败项目已清理可正常停机；保留锁与失败停机策略不扩展恢复 |
| ALC-A5 | verify-stop 使用只读 catalog，原身份/成员/锁/证明反例及部署暂存入口保持；旧失败和 DB/配置不改写 |
| ALC-A6 | 既有 controlled runtime.service-lifecycle、recovery.waiting-merge-restart 和 runtime.service-write-isolation 通过并清理完整 |

实现 Plan 映射全部自动项。固定 L1、定向 owner、编译和选定三个受控场景构成最小范围，不默认 Full/live。Windows 独立测试脚本补 Scenario 透传，以执行相同子集；不新增 E2E ID。结果只登记专属 acceptance。
