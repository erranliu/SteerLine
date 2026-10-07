# Spec：测试执行提速

状态：已按配套 [Plan](test-execution-efficiency.plan.md) 独立实施；未分配 Task ID、创建中心任务或加入 DAG。实现结果见[验证记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/test-execution-implementation/results.md)。范围仅为测试执行；不包含构建缓存、编译参数、依赖替换或产品性能优化。

## 已完成的定位与固定方案

源码基线为 `543f0d4`，Windows/amd64、Go 1.26.4，原生验收开关未启用。[测试定位记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/test-execution-diagnosis/README.md)保存原始输出与实验补丁。当前完整串行套件耗时 658.633 秒，其中核心包 603.209 秒；原用例经三个独立进程分组后，全套测试执行为 349.326 秒，全部 460 个测试/子测试终态一致且无失败。分组实验的核心测试程序预先编译，其准备时间单列，不作为构建提速证据。

进一步按 T5 精简重复工作，叠加已选定的夹具/规则分层并改用固定权重均衡后，完整实验为 264.871 秒，核心三个分组约 222/222/210 秒，原 460 个终态及状态仍一致。该结果是组合效果，不能把全部节省归因于删除重复断言。最长本机/Orca 闭环及两条决策路径均保留。

实施范围固定为：交付日常最小闭环与验证范围标记；删除有明确替代覆盖的重复测试段落、缩小任务图及配置选择 fixture；原生 Git 夹具复用只读种子；五个 DAG 规则反例改用已有纯规则入口；现有测试工具增加按已测权重均衡的三个独立进程完整入口。实现按下述清单完成，计时仅用于验证效果。轻量 Git 代理、无实测收益的决策 fixture 替换、任意调度框架及生产 SQLite/超时调整均不纳入。

## T0：日常快速检查与完整检查时点

在 `native_tasks_test.go` 增加 `TestNativeMinimalClosedLoopSmoke`。它在隔离的本机项目使用真实 Git、SQLite 和本机测试 worker，只创建任务 A 及其后继 B：领取 A，依次执行当前合法的 implement、verify、publish_pr 步骤；保持等待 PR 合并时 B 被阻塞，重启中心后仍接回 A 的同一 WorkflowRun；由测试 PR 提供者返回与固定 head 相符的 merged 证据，中心写入唯一 TaskDelivery、关闭 A、使 B 就绪。B 不再次运行；该用例不承担失败重试、未知派发、人工决策、资源清理或受监管 Orca 的故障矩阵。测试 PR 提供者的合并回执不能算真实托管平台验收。

现有 `TestNativeDeliveryNeedsExactMergedPRAndReleasesSuccessorOnce` 保留在快速入口，用于核对错误 PR/合并证据不能交付；同时运行 taskgraph 与 remoteprotocol 包的全部确定性规则测试。快速入口使用 `-count=1`，缺少用例、失败、截断日志、只有部分包完成或出现意外 skip 时失败；一次执行保存命令、源码、通过/跳过、失败和墙钟。参考机上的先期实验为 39.170 秒，其中最小闭环约 29.25 秒；该用例当时来自测试专用 overlay，现已并入测试源码。[实验记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/test-execution-diagnosis/smoke-probe/README.md)保留输入和结果。

开发中每次一般代码修改先运行快速入口，再运行受修改路径影响的定向测试：会话/答案改动补本机回答应用与 unknown 恢复用例，Orca 适配改动补对应 Orca 契约或模拟执行用例，SQLite/命令改动补相关 CAS、幂等与回执故障用例，控制台改动补 console/integration，用于发现跨包接线的异常。纯文档修改做身份、链接、引用与语义检查。改动范围无法判清或快速入口无法覆盖新增能力时，明确扩展定向集，不把“快速通过”解释为全部能力已验收。

同一稳定候选在最终集成或实现 PR 交付前运行一次完整无缓存套件；涉及数据库格式、恢复、外部执行身份、PR 交付或共享接口的大改在集成前提前运行完整套件。原中心任务已固定的完整回归及真机门槛继续按其契约执行，不能因本草案修改其交付证据。新增快速结果报告 `scope=quick`，完整结果报告 `scope=full`；只有后一种具有全部确定性回归的证据，真机验收仍独立记录。日常小改不会因快速检查自动触发完整套件，最终交付不使用快速结果冒充完整检查。

## T1：三个独立测试进程

扩展现有 `cmd/test-feedback` 和 `internal/testfeedback`，提供固定快速入口、显式 `-workers=3` 的完整确定性套件入口，保留 `-workers=1` 的串行对照和原定向 Go 参数入口。三进程模式只用于全套测试；与定向参数或 build-only 混用时明确拒绝，不静默改范围。快速入口只运行 T0 的固定核心用例及两个规则包，可追加显式定向测试；它的 summary 不能使用 full 状态。本任务不建设通用作业调度器。

运行开始用当前源码取得 Go 包清单和核心包测试程序，在执行计时开始前记录测试程序准备成本；从核心测试程序的 `-test.list=.` 输出自动枚举所有 Test、Example 和 Fuzz 种子入口。使用本轮串行基线的核心顶层用例耗时作为固定权重，按权重降序、名称升序排列，逐项放入累计权重最小的组，平局选较小组号；新用例默认权重 1 秒，已有非正/极小耗时下限 0.05 秒。权重只决定分组，不能决定是否执行；当前清单未出现的历史条目忽略，新增用例自动加入且只加入一组。运行正则完整转义并首尾匹配，不维护测试白名单，不在运行前另跑测试来重新决定方案。

权重以一个小型数据文件随工具固定，来源、源码与生成规则可追踪，不增加平台调优服务或配置矩阵。旧哈希实验把两个最长闭环放在同组，形成约 140/307/215 秒的长尾；本任务用上述确定算法替代该分配方式，保留完整用例清单与各组实际耗时。

每组一个 OS 测试进程，组内维持 `-test.parallel=1` 和单次执行，测试顺序沿 Go 的注册顺序。每组独立 STEERLINE_HOME 和临时输出目录；现有 t.Setenv、包变量和 fixture 生命周期仅作用于所属进程。Git worktree、bare remote、数据库、端口及模拟平台可写目录仍逐例隔离，不能跨组共享正在运行的服务或平台资源。其他包按原 Go 全量语义执行一次，排除已经分组的核心包；总共不超过三个由工具调度的测试进程，其他包阶段可沿用串行方式。

三进程模式在任何测试启动前拒绝启用真实平台/窗口验收的环境：覆盖现有 enabledNativeSwitches，以及 STEERLINE_REAL_*、STEERLINE_NATIVE_*、STEERLINE_LOCAL_CAPABILITY_*、STEERLINE_LIVE_* 和现有窗口/前台 viewer 探针入口。拒绝时显示变量名并要求用串行原生入口，不删除环境变量或把相应测试改成跳过。普通 STEERLINE_HOME 和测试进程内部的 fake/helper 环境不等同于外部验收开关。

## T2：结果、覆盖与退出

保存包/测试清单、分组清单、命令、源码、Go/机器/并发条件、原生开关、准备与执行墙钟，以及每组未修改的 test2json 输出和 stderr。汇总保留规范包名和组身份；包墙钟取实际执行时间，不把三份同名包互相覆盖，也不累加父子测试耗时冒充墙钟。

对每个枚举出的顶层用例核验恰好一次 run 和一个 pass/fail/skip 终态，对每个动态子测试核验其终态。其他包也必须有终态。用例遗漏、重复、正则未匹配、测试程序退出但日志缺失、非法 JSON、截断日志和失败均使总体失败；skip 单列，不作为原生验收通过。不能只取最后一个子进程的退出码。

进程退出、取消和超时仍受有界控制。清理只作用于本次工具拥有的测试进程树和临时目录；Windows 使用精确进程/Job 所有权，Unix 使用对应进程组或等价机制，不能按全局进程名终止。保留已经产生的日志，未完成组明确标为未完成，不能留后台测试继续占用资源后报告结束。测试超时沿用项目现有门槛，不以缩短超时取得提速。

## T3：原生 Git 夹具的固定改动

修改 `native_tasks_test.go` 的 `nativeTaskFixture`，在同一测试进程内只初始化一次三份固定规格文件及 Git worktree/bare remote 种子，使用现有测试产物生命周期保存和清理。每例复制为自己的可写仓库与 bare remote，再重设 origin 并计算自己的仓库身份。种子仅包含原有初始材料，不包含运行中的数据库、服务、Task 或 PR 状态。

复制必须是独立文件内容，不能用共享可写 Git 对象、硬链接或指向原种子的 worktree；使用当前 Go 已有文件复制能力即可。测试对目录、配置、index、refs 和 objects 的后续修改不影响种子或其他用例。原有测试仍使用真实 Git、相同项目初始化和 SQLite 设置；不能跳过待测试的契约、base、身份或事务校验。

为种子复制补充一次明确隔离检查：A/B 两例的项目与 remote 身份不同，A 修改/提交/push 后 B 和种子的 HEAD、文件及 refs 不变；任意一例清理不会破坏另一例。初始化或复制失败必须立即失败，不能回退到共享可写现场。

## T4：DAG 反例的固定改动

调整 `task_dag_test.go` 的 `TestTaskBatchRejectsDuplicateAndCrossProjectDependencies`。保留其首个跨项目错误经过真实 CommandService 的接线检查；其余五个反例——契约 ID 冲突、重复边、自环、缺前置、已有节点作为批次后继——用合法契约和明确图快照调用现有 `validateTaskBatchAgainstGraph(..., false)`。保留原错误码和全部断言；每个反例仍可单独定位。

这里只省去重复验证相同 Git blob 的准备/调用。`ValidateTaskContract` 的规范摘要校验仍执行；生产 `ValidateTaskBatch/CreateTaskBatch` 仍验证真实 Git 文件，禁止改变生产 verifyFiles 的取值或增加测试专用绕过入口。`TestTaskContractPinsCommittedBlobsAndRejectsInvalidReferences`、符号链接拒绝、配置漂移、创建原子性/回滚/幂等、领取竞争和完整闭环等原用例继续保留。

## T5：删除重复段落与缩小 fixture

按[覆盖映射](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/test-execution-diagnosis/reduction-audit/coverage-map.md)完成以下固定改动，不以单纯减少用例条数为验收：

1. `TestCreateTaskBatchAtomicIdempotentAndQueryable` 从五个节点的菱形图缩为 20 → 21 加独立高优先级 24。保留创建、幂等/异内容冲突、固定字段 CAS、优先级接线、游标失效及丢弃 ID 不复用。双前置阻塞继续由图查询的 3/5 → 4 场景覆盖；为 `taskDependencyState` 补无前置交付、一个已交付、全部已交付的内存矩阵，确保 AND 语义无需反复创建完整批次。
2. 删除 `TestTaskGraphViewSnapshotAndBoundary` 尾部重复创建两份契约并验证环的段落。真实命令的环错误码与确定路径由 `TestTaskBatchStrictDecodeAndDeterministicCyclePath` 保留，并在那里核对失败前后的 database/graph revision、Task/edge 数量不变；图算法仍由 taskgraph 包覆盖。
3. 仅将 `TestServiceStartControlDispatchesChosenProfile` 的批次创建准备改为测试内部的合法 Batch/Task/Contract 记录，通过原 SQLite 类型化事务建立。测试全部原控制、两个真实 worker、固定配置和重启断言保持；实际 claim 的契约/base/能力检查不能绕过。创建命令的 Git/原子性/回滚/幂等覆盖仍由专属测试和 CLI 测试承担。

两个决策正常/unknown 完成路径、本机及 Orca 的真实 worker 闭环、清理回执失败后恢复、精确停止和未知启动不重派继续保留。这些段落有独有检查，当前不能用普通存储/规则测试替代。没有对应存活断言的测试不得仅按耗时删除或移到默认跳过入口。

## T6：接入原验证流程

在开发文档中列出完整三进程入口、串行排障入口和定向用例命令。`scripts/steerline-verify.py` 只将 go_test 阶段接入新的完整测试入口，报告写到临时目录，避免污染固定候选工作区；required_checks 顺序、go_test 名称、固定 revision、候选干净检查、失败回执及实际覆盖保持。go_vet/go_build 阶段及其命令不变。日志和汇总失败也必须使 go_test 失败，不能由后续成功检查覆盖。

## 验收

| 编号 | 通过条件 |
| --- | --- |
| TE1 | 自动枚举的核心顶层用例恰好分配一次，三个进程隔离；同一候选的串行/三进程测试与子测试集合、状态相同，其他包无遗漏或重复 |
| TE2 | 对重复/缺失/截断/非法输出、子进程非零退出、取消和超时的测试正确失败；进程树和日志按本次所有权处理；原生开关下三进程入口在启动测试前拒绝 |
| TE3 | 十个 fixture 的三次配对测量中，种子版本的准备耗时中位数不高于原版 75%；初始化只做一次，每例保留独立 Git/remote/项目身份，隔离检查通过 |
| TE4 | 指定 DAG 用例保留六种拒绝行为，五个规则反例不再重复走真实 Git 文件核验；三次配对耗时中位数不高于原版 70%；原命令/契约/事务接线检查通过 |
| TE5 | 在记录所用 Windows 参考机、相同源码/原生开关/缓存条件下，完整三进程执行墙钟不超过 300 秒，且不高于同源码串行入口的 75%；完整测试零失败，无新增默认跳过；分别报告准备时间和执行时间，不把参考机绝对值外推到其他主机 |
| TE6 | 原生 verify 的 go_test 成败准确映射，固定候选与其余检查不变；现有定向/串行/日志重汇总入口仍可用，代码与证据明确不涉及构建优化或产品状态变更 |
| TE7 | T5 的重复段落、三节点 fixture 和配置选择准备完成，原断言到存活/新增断言映射完整；AND 内存矩阵、环拒绝无副作用、配置控制两个 worker 和重启通过，独有恢复/真实进程用例未删或默认跳过 |
| TE8 | 最小闭环通过真实本机 Git/SQLite/worker、中心重启、固定 PR 合并证据及后继解锁；错误 PR 门槛与两个纯规则包同在快速入口。参考机快速入口不超过 60 秒；单项或日志失败使整次快速检查失败，结果明确为 quick，不能用于 full/真机交付声明 |
| TE9 | 日常定向路径及完整触发点写入开发说明和验证脚本；既有任务的固定完整/原生门槛保留，稳定候选交付前 full 确认一次，不因每次日常修改重复运行；quick/full 报告及下游 go_test 证据范围不能串用 |

实现验证以现有确定性套件及临时真实 Git/SQLite 为准。完整串行与三进程各跑一次，受影响夹具/反例组做三次配对；只有失败、新变更或具体未解决风险才扩大重测。实际结果和证据见[验证记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/test-execution-implementation/results.md)。
