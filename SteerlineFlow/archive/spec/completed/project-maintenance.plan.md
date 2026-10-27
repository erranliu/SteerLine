# Plan：项目基线与材料统一维护

> 归档于 2026-10-27：本文保留原交付契约和历史验证；已被[项目更新与候选版本收敛](../../../spec/project-maintenance-simplification.spec.md)部分替代，当前行为以[项目维护设计](../../../../docs/product-design/project-maintenance.md)为准。

后续实施计划见[项目更新与候选版本收敛](../../../plans/project-maintenance-simplification.plan.md)。2026-10-21 新方案已确认但尚未实施；本文是上一批的实施/验证安排，人工采用、显式应用及Task级冻结不能作为新方案继续保留的要求，原验收结果不改写。

状态：2026-10-21 已获用户实施授权，沿审查后最简方案开发和验证。范围归 [Spec](project-maintenance.spec.md)，行为正文归[项目维护设计](../../../../docs/product-design/project-maintenance.md)，结果归[专属验收](../../../testing/acceptance/project-maintenance.md)。不改 roadmap 或安装环境；仅在独立 SteerlineTest 夹具验证。实施前已快进至主干 `9e02854ef66e1b1e4b1fbdff35a9eeca294b26a5`。

## 输入与现有代码整合

调查基线 `57fc5631976e7f7ec5a8f6404b14f860f1ef9fd3`。已读 [SpecGuide](../../../spec/SpecGuide.md)全文，路径 docs/spec/SpecGuide.md，最后修改 commit `94e704300485b82d54e44991148074841ed66019`，SHA256 `85b656090674e1f953320b6be62d17b6888248ef7eec5fe044b9d3f408bfe4de`。采用[通用测试规范](../../../../SteerlineProjectGuideDoc/testing-architecture.md)、[本项目指南](../../../testing.md)与 [E2E 约定](../../../../tests/e2e/README.md)。

| 现有位置 | 保留/调整 |
| --- | --- |
| [taskrunner/base.go](../../../../internal/taskrunner/base.go)、[service.go](../../../../internal/taskrunner/service.go) | 保留 fetch、祖先/前置检查及原请求重放；首次准备完整材料，后续候选先读冻结快照 |
| [task/model](../../../../internal/task/model)、[task/documents.go](../../../../internal/task/documents.go)、[test_documents.go](../../../../internal/app/test_documents.go) | 提取实际共用 blob/路径/限额能力；最近确认 contract 与执行 contract 分开，测试沿同 commit 解析 |
| [runner_candidates.go](../../../../internal/storage/sqlite/runner_candidates.go)、[runnerstate](../../../../internal/runnerstate) | 扩展原项目库预留事务，固定同一 Task 执行快照/候选/outbox，保留不可变字段规则 |
| [testmanagement](../../../../internal/testmanagement)、[test_generations.go](../../../../internal/storage/sqlite/test_generations.go) | 保留严格解析、原 generation/profile/信封/停止，分离生成结果与单文件应用；不叠加通用 outbox |
| [application_service.go](../../../../internal/app/application_service.go)、[product/api](../../../../internal/product/api)、[web/src](../../../../web/src) | 原宿主安排观察，原项目 actor 写依据/操作，沿原认证/路由和 typed API；不在 Web 建状态权威 |
| [项目添加 Spec](../../../spec/project-onboarding.spec.md)、[结构方案](../../../../docs/product-design/project-structure.md) | 仍未实现；本批提供窄公共检查与初始化模式适配，全套 PS/登记/候选入口继续后置，无强依赖环 |

不新增 Runner 能力；只需现有共享 generate/1、查询/停止及结算。W01/W02、E01/E03–E07、V01/V03/V06–V08 差异同步[生命周期](../../../../docs/product-design/worker-lifecycle.md)，执行/原生恢复矩阵不在本仓重建。

## 实施步骤

1. **共用规则与最小模型。** 新增 internal/projectmaintenance，复用 testmanagement/model 的纯文档解析，固定输入快照、范围选择、影响/核验结果和人工采用依据。初始化/基线/局部只选择范围/来源，规则可在未建库时纯读取调用；局部草稿检查/采用不以远端观察成功为前置，也不产生远端就绪事实；不实现预纳管候选库/路由或完整 PS。从 Task/testmanagement 提取共用普通文件、精确 Git blob、UTF-8/限额/路径检查，不为调用构造假 TaskContract。生成的三个完整输入做依赖摘要，文本 CRLF→LF只用于新鲜度；原请求/Task SHA/文件 CAS 仍用真实字节。已配置项目依据放原项目库，清单是投影，不新增通用作业框架。
2. **观察与影响评估。** 复用窄 Git 适配器、规范仓库身份和现有 owner；宿主每五分钟/手动观察按仓库目标合并在途读取，观察结果由原项目运行时加锁保存，最终预留重核同一观察；材料/Task/生成写入仍沿项目 actor，不跨长事务联网。记录目标删除/非快进/失败时间，只提示正常 Git 更新，不实现 sync 命令/按钮。无基准检查全范围；分清源码适用性提示、材料变化必要待办、无关 Plan。三个完整生成输入/规则/输出一致可沿用到新 commit，不按所选定义做另一套优化。
3. **人工采用与单项生成。** 固定项目/Plan/输入来源及快照、目标原值、profile/执行配置和唯一 Runner 请求，保留原 Generation 作为生成记录；结果待审阅，不自动保存。每次只一个 Plan/一次模型调用，不做批量调度或自动重试。人工采用同样读取实际输入/输出和操作者，CAS确认；缺依据的新材料可采用，无须造 Git sidecar/历史回执。规范/清单先人工准备，初始化适配不启动 Runner。
4. **单文件应用与恢复。** 扩展原 generation/应用回执，预览只读计算覆盖项目/原生成/路径/输出与当前HEAD/分支、输入/目标条件的review_digest；应用按该摘要和expected generation revision重新读取核对，项目actor先持久应用意图再原子替换。取消及应用串行裁决，重启/丢回执只核验；未取消可显式原命令续写，取消后旧号不得新写，均不自动重生成。HEAD变化重新评估/审阅后用新摘要/请求复用仍适用输出，不改原Runner信封。多项各自成功/冲突，无多文件事务/回滚/部分事务续办。unknown只查原请求，失败新尝试检查原终态/资源资格；停止/迟到绑定沿共享围栏，已应用输出也继续只读观察原 Run 直到核实停止，保留文件事实并允许显式停止；空安排勾选绑定 review_digest，不跨结果/新审阅条件沿用；不为预纳管增加生成资源。
5. **Task 确认与首次冻结。** 最近确认的材料沿 Task version，目标 repository/remote/branch 不可借材料确认变更。首次执行快照唯一归 Task，含完整执行 contract/base/测试快照；区分确认摘要与执行摘要，不继续混用旧 ContractDigest 条件。共用核验输出包含完整选中定义，首预留直接构造同次测试快照，避免 gate 后第二次读取失败仍派发无效快照。各首次启动路径成功 fetch 后读取同 commit 全 refs/测试文件；原项目库事务内重核 Task/图/目标及相关材料确认并固定快照/候选/outbox，不能只在事务外判断“未启动”或材料已就绪。只比较本次消费的确认/摘要，不用全局维护 revision或轮询时间制造冲突。事务前已观察到目标前进则重新评估。拒绝/unknown/退出不解除冻结，后续候选先取原快照再做原祖先/前置/profile/Runner资格，原请求重放不刷新材料。保持原 Delivery/报告来源，直接调整当前格式无旧数据迁移。
6. **最小消费者。** 在原应用/项目协议固定必要 DTO/错误/原请求回执；维护面板提供观察、Git提示、人工采用、单项生成/diff/应用和Task确认。不增加通用 project maintenance CLI 或Task修订CLI。原 tests generate/generation/stop-generation 与新增 tests apply 走同一服务；全部旧自动保存入口收口，CLI/API/自动派发/手动启动复用步骤5。缓存按项目/操作/版本隔离，不转移原授权，PlanTests仍查询原候选快照。
7. **最小验证能力与登记。** 按下表安排 TestPM* 定向验证；新规则/编排包只登记 quick-owner-project-maintenance，原 app/Task/SQLite 新测试进入其原 owner，更新 coverage 分区避免重复。唯一 new-debug-release.ps1 增加受控维护夹具（-ProjectMaintenance），含SteerlineTest clone/bare remote与全部本次必要材料，不要求全套未实现纳管结构。新增一个 L3 runtime.project-maintenance 及 web/tests/project-maintenance.spec.ts，E2E与薄脚本 scripts/test-project-maintenance.ps1 委托同一个driver；脚本只包装唯一生成器/驱动，不反向嵌套分级总入口。受控启动固定 --simulate。既有测试管理live驱动的一次generate调整为审阅/显式应用，保留原报告/页面/清理验证，不另加后端矩阵或真实PR。
8. **按表验证并登记。** 记录source commit/dirty、Runner pin、实际选择/输入/原请求、文件与事务/API/页面证据及清理。新增可执行项落实后同步 suites.json、E2E可读清单、SteerlineFlow/testing/test-list.json及相关Plan，不提前登记不可运行条目。自查三入口共用调用、新旧生成零自动保存、首次与后续候选分流。实际结果只进专属验收；提交/部署另按用户指令。

## 测试计划

下列 TestPM*、L3 场景、脚本/参数和 Web 文件已由步骤7落实。执行前验证测试实际存在、逐项断言与选择匹配，零匹配/意外跳过不算通过。<证据> 是新建空目录，<发布版Runner> 为安装环境之外的固定EXE。全部产品结果归[专属验收](../../../testing/acceptance/project-maintenance.md)，实际结果归专属验收，不回填到 Plan。

| 验收项 | 级别/组与场景 | 环境、命令与时点 | 通过条件与证据 |
| --- | --- | --- | --- |
| [PM-A01](../../../testing/acceptance/project-maintenance.md#pm-a01) | L1 owner / pm.remote | 步骤2后，临时 clone/bare remote、受控时钟/失败；`go run ./cmd/test-feedback -out <证据>/remote -- ./internal/projectmaintenance ./internal/app -run '^TestPMRemote' -count=1` | 周期/手动/首次fetch、在途合并、删除/force push/失败及时间诊断，零工作区/模型写入；保存调用与refs前后事实 |
| [PM-A02、PM-A03](../../../testing/acceptance/project-maintenance.md#pm-a02) | L1 quick-owner-project-maintenance / pm.rules、pm.review | 步骤1/3后，隔离 blob/工作区/窄存储；`go run ./cmd/test-feedback -out <证据>/rules -- ./internal/projectmaintenance ./internal/app -run '^TestPM(Rules|Review)' -count=1` | 三入口相同输入复用规则；增删改/完整清单变化/CRLF草稿合入/无基准/人工采用CAS；源码只提示、必要待办范围正确，无初始化DB或纳管门槛；局部无远端观察也可核对且不冒充远端就绪；规则命中和文件/DB调用报告 |
| [PM-A04、PM-A05、PM-A06](../../../testing/acceptance/project-maintenance.md#pm-a04) | L1 owner / pm.generation、pm.apply、pm.recovery | 步骤3/4后，公开Runner fixture/临时文件与故障注入；`go run ./cmd/test-feedback -out <证据>/generation -- ./internal/projectmaintenance ./internal/testmanagement ./internal/app -run '^TestPM(Generation|Apply|Recovery)|^TestTMGeneration' -count=1` | 单Plan/一次调用；旧CLI也待审阅；草稿可用但未就绪；原字节/路径/HEAD保护、取消先/应用先及取消后旧号零新写、写后丢回执、重启零补写、HEAD变化后显式复用输出、unknown/迟到及失败资源资格；已应用后继续核实停止、期间零文件写入/模型调用，核实后显式新尝试可用；原信封/文件快照/计数/停止事实 |
| [PM-A03、PM-A07](../../../testing/acceptance/project-maintenance.md#pm-a07) | L1 原Task/SQLite owner / pm.freeze、pm.transaction | 步骤5后，真实临时Git/SQLite；`go run ./cmd/test-feedback -out <证据>/freeze -- ./internal/task ./internal/taskrunner ./internal/storage/sqlite ./internal/app ./internal/product/httpapi ./cmd/steerline -run '^TestPM(Task|Freeze)|^TestRunner(Base|CandidateReservation|TaskDelivery|DeliveryReplay)' -count=1` | 最新同commit全refs，Task确认与首预留竞态、目标变更拒绝、相关材料/图竞态零派发；无关Plan/检查时间不冲突；空安排允许；原拒绝/未知/退出重放、后续候选原版本/非快进拒绝、Delivery保持；gate 后 Git 适配器不可用时仍消费原已核验快照；事务前后/原请求证据 |
| [PM-A01、PM-A03–PM-A08](../../../testing/acceptance/project-maintenance.md#pm-a08) | L1 HTTP/CLI/Vitest / pm.consumers | 接线后；`go run ./cmd/test-feedback -out <证据>/contracts -- ./internal/product/applicationhttp ./internal/product/applicationclient ./cmd/steerline ./web/server -run '^TestPM' -count=1`；新增组件后 `npm --prefix web test -- project-maintenance.test.tsx`；`npm --prefix web run build` | 新旧入口同一owner、原认证/项目/摘要检查、取消/应用回执、diff/人工采用/启动待办与切项目不串授权；命令/API/组件报告，mock不能代替Go闭环 |
| [PM-A01–PM-A05、PM-A07、PM-A08](../../../testing/acceptance/project-maintenance.md#pm-a08) | L3 controlled runtime.project-maintenance | driver落实后，新debug包/真实Go-Web-Edge、bare remote、模拟Runner；`go run ./cmd/test-feedback -level=L3 -suite=runtime -scenario=runtime.project-maintenance -env=controlled -out <证据>/flow`；同一driver可由 `./scripts/test-project-maintenance.ps1 -SteerRunnerExe <发布版Runner> -Out <证据>/browser` 进入 | 初始化模式零建库、基线/局部同流程、Git更新提示、单项准备/审阅/应用、测试驱动实际提交到隔离remote、首次冻结/PlanTests、后续候选原版本和另一项目不变；截图/trace/API/真实commits、调用计数及退出/清理 |
| [PM-A04–PM-A06、PM-A08](../../../testing/acceptance/project-maintenance.md#pm-a08) | L3 live复用 testing.management-local-codex | controlled通过后，唯一生成器新包/真实Runner+本机Codex；`./scripts/test-test-management.ps1 -SteerRunnerExe <发布版Runner> -Out <证据>/live` | 一次正式generate→待审阅/零提前写→显式应用，原通过/失败报告和PlanTests保持；原Run/input/output/apply回执与公开资源结算/owned引用清理，不新增真实PR |
| [PM-A02、PM-A06、PM-A07](../../../testing/acceptance/project-maintenance.md#pm-a07) | L1必要原owner/边界回归 | 收口后；`go run ./cmd/test-feedback -out <证据>/regression -- ./internal/taskrunner ./internal/testmanagement ./internal/runnerbinding ./internal/testkit/boundary -count=1`；`go run ./cmd/test-feedback -out <证据>/selection -- ./cmd/test-feedback -run 'TestQuick|Test.*Owner|Test.*Boundary' -count=1` | 原outbox/冻结/结果隔离/Delivery、依赖所有权及quick分区保持；summary/JSONL；新修改涉及未覆盖owner才补定向项 |
| 文档 | 静态 | 实施收尾；`python scripts/check_doc_links.py`、`git diff --check`，验收映射/新文件空白/相关锚点检查 | 链接/空白通过、8个自动项有命令与能力补齐；结果只记验收末节，不计产品通过 |

新L3独有范围是“基线/材料维护/首次冻结”，不重复L2发布合并。独立home、repo/bare remote、DB、端口、provider命名空间和证据；生成包阶段独占源码Web构建目录，包内运行可与其他资源隔离项并行。新增登记时同步机器/可读/展示清单；新增可执行项已同步登记。

不选Full、全部L1、完整L2、全部L3/L4或Orca/原生恢复矩阵。局部异常由L1注入，受控L3证明组合接线，只复用一次既有live证明新增审阅/应用路径。缺现场或资源结算不记通过，不以历史live结果替代。

PM-M01：自动项通过后，用户在新独立包检查提示/diff/启动待办；窗口届时安排，可后置，仅阻塞体验签收。禁止使用C:\SteerlineInstall、C:\SteerlineRuntime或复制其数据库/配置/认证准备现场；真实模型用新debug包，受控明确--simulate。

## 实现定位与入口

公共规则：[projectmaintenance/rules.go](../../../../internal/projectmaintenance/rules.go)；文档解析：[testmanagement/model/inputs.go](../../../../internal/testmanagement/model/inputs.go)；Git/工作区适配与三入口装配：[app/project_maintenance.go](../../../../internal/app/project_maintenance.go)。默认评估未启动任务绑定的 Plan，显式 Plan 可独立维护；不扫描并重写历史候选。

人工依据与 Task 确认：[sqlite/maintenance.go](../../../../internal/storage/sqlite/maintenance.go)；首次冻结沿原候选事务，最后预留同时核对已观察的基线；后续候选/Delivery 读唯一 Task 快照。生成沿原 Generation，单文件应用/只核验恢复：[testmanagement/apply.go](https://github.com/erranliu/SteerLine/blob/b9e5002d38da4722dc8457338e063ce56817fb48/internal/testmanagement/apply.go)。当前格式为 schema 26，无旧数据迁移。

Web 维护动作沿 `/commands`；原 tests CLI 增加 apply。`runtime.project-maintenance` 复用唯一生成器和双项目隔离包，场景不使用 `t.Parallel`，避免同包生成时争用源码 Web 构建目录；其余已准入 runtime 场景仍有原并行预算。原 E2E fixture 补最小公共材料并由外部驱动显式采用；不在产品中自动采用，也不为调用方协议夹具虚构受管测试通过。
