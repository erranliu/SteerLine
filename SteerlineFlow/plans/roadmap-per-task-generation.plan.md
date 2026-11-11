# Roadmap 按任务并行生成 Plan

依据 [Spec](../spec/roadmap-per-task-generation.spec.md)、[编写规范](../../SteerlineProjectGuideDoc/spec-plan-guidelines.md)、[测试指南](../testing.md)和[验收流程](../testing/acceptance/README.md)。当前按用户授权直接实现并验证本文，不启动 roadmap、业务任务或安装更新。测试选择见同名 [tests.json](roadmap-per-task-generation.tests.json)，结果独立登记到[专属验收](../testing/acceptance/roadmap-per-task-generation.md)。

## 调查基线与整合

文档基线为 rebase 后 `1f64b0ffeb936358786cfe4734f2096765bc4008`。实现开始前刷新实际代码基线，核对以下消费者，规格差异须先修订本文。

| 当前位置 | 调查结论与修改所有权 |
| --- | --- |
| `internal/roadmap/model/model.go`、`model/generation.go` | Analysis/Node 已有任务范围和依赖，确认摘要目前只绑定 Analysis；DocumentSchema 仍要求一个 Run 输出全部 PlannedDocuments。只补 coordination、provides_tests 及文档记录内的窄执行数组，不增加另一层 Roadmap 记录。 |
| `internal/roadmap/documents.go`、`generation.go` | 已支持确认并生成、原请求先保存、共享队列、原 Run 观察和停止。把文档用途的单执行改成数组，保留原文档记录和单 Run 处理规则。 |
| `internal/roadmap/model/test_dependencies.go` | 提供者当前从生成文档提取。前移到确认规划，单项结果核对冻结声明，tests 仍由文档生成选择并经正式清单验证。 |
| `internal/app/roadmap_documents.go`、`roadmap_generation_test.go` | 文档生成前固定代码/规范、逐项调查与发布预览继续复用。解析只补项目声明的测试材料读取，不调用整套文档上下文准备；复用真实 Git/provider/SQLite 链夹具。 |
| `internal/runnerbinding/content_queue.go`、`internal/app/application_runner.go` | home 级队列已有可配置额度、默认五。按 generation_id 登记占位，不引入第二个限流器或提交线程池。 |
| `internal/storage/sqlite/roadmaps.go`、`async_roadmaps_test.go` | 原文档 JSON 及请求关联已有不可变绑定与 CAS。执行数组随一份文档记录原子保存，不新增父子表、子用途或多记录初始化事务。 |
| `internal/app/roadmap_recovery.go`、`roadmap_activity.go`、`roadmap_publication.go`、`roadmap_admission.go` | 恢复/停止遍历执行数组，活动从其推导；最终 Documents、发布未知和整批原子入图继续归原 owner。只改 Roadmap 的资源枚举，不扩展部署工作流。 |
| `internal/product/httpapi/roadmaps.go`、命令注册及 Web Roadmaps/activities | 原确认并生成命令和请求回执保留；增加按任务重生成与批次/子项读模型，不使前端成为规划或调度权威。 |
| `internal/prompts/roadmap-parse.md`、`roadmap-documents.md`、`materials/roadmap-*.md` | 规划规则须前移、文档规则改为单任务。只改原唯一来源、组装及调用方事实，材料继续固定文件供给。 |

全部实施归同一普通开发变更，按下面步骤顺序集成。Runner 已提供独立 generate Run 和原生 session，不修改其仓库或协议。删除被替换的整批生成路径及对应断言；不保留双实现或旧记录迁移/兼容读取。

## P1. 完整规划前移并纳入确认

1. Analysis 只新增一段 coordination，Node 只新增 provides_tests；原 scope/constraints/acceptance 已能承载每项范围、所有权和交接说明，不再为同义事实增设嵌套契约模型。保留原依赖/正式关联和逐字来源规则，新增 ID 不生成完整测试定义。
2. 通过原 ReadCommitted/Git 材料读取获取项目入口声明的测试清单，固定其来源和正文供 ParseSchema/确认验证。解析仓库 paths 仅按需增加这些材料，原文/任务图/映射沿原 file 输入；不提前执行 documentContext 的完整基线准备，也不要求解析调查整仓。
3. 完整规划在解析结果内展示，扩展原确认摘要，集中一个摘要函数覆盖 Analysis 及规划材料，更新生成/发布消费处，避免新旧摘要算法并存。确认并生成命令重核原 revision、来源/图及相关规划材料；无关 HEAD 变化不额外阻断。沿用独立确认里程碑和确认成功但生成未提交的丢回执处理。
4. 提供者索引改从已确认规划构建，验证唯一性、已有 ID 冲突和依赖关系。文档记录创建时仍只调用一次 documentContext，固定所有单项共享的代码/规范/清单；清单与确认提供关系冲突时重新规划，否则各 session 只调查本任务相关代码，不重决定公共所有权或 ID 归属。

## P2. 在原文档记录中保存单项执行

1. 保留 purpose=documents 和原 parent_id，在原 Record 增加 generations 数组。窄执行项只包含 generation_id、local_key、不可变 CreatedAt 和原单 Run 请求/提交/观察/结果/停止字段，不继承完整 Record 的规划/发布/入图字段。
2. 复用 PlannedDocuments 从固定 Analysis 推导目标及顺序，创建所有首轮请求后一次 SaveRoadmap 保存原文档记录，再允许派发。原请求幂等返回同一记录；不另存目标集合、当前轮次指针或完成计数。文档用途不同时填写根级单 Run 请求/结果字段和数组内相同事实；根级单 Run 字段仅供解析用途，固定构建等共用上下文仍在外层。
3. 扩展原 CAS/不可变校验，执行项的任务/请求/已知 Run/input 和原结果不可换绑；数组只追加新生成项。格式变更沿现有数据库版本拒绝策略更新，无兼容读取。失败项重生成的请求关联在同一记录保存事务登记，不增加多行初始化事务。
4. 单项请求复用 generate/1 及原 source/confirmed-planning 两份 file 材料。由 prompts 模板接收目标 local_key 这一事实，DocumentSchema 只固定本任务文件集，不新增 target 文件或材料协议。
5. 将 DocumentSchema/DecodeDocuments 分清单任务校验及完整集核验，沿原 manifest、调查引用、严格 JSON/大小边界；汇总后继续生成原 Documents 供编辑/发布。执行原结果与编辑正文不混写。

## P3. 并发派发、停止与失败项重生成

1. 队列身份使用 generation_id，批次不占名额；UpdateContent 和重建均使用执行项 CreatedAt，首轮同刻沿原 ID 规则排序，重生成追加时固定新时间，不能借批次旧时间插队。现有 ObserveNext 遍历任务执行；复用原 dispatch/observe/stop 的单 Run 处理规则，改为更新选中执行项再 CAS 保存外层记录。只在 roadmap 包内拆小函数，不新建执行器接口、通用批次框架或后台提交线程池；逐项短提交后由 Runner 异步并行。
2. 每项分别观察/结算，失败但未 stopped 保持占位；按 local_key 取最后一项推导进度。一个保存冲突须重读原执行事实，不能覆盖整份数组；任何提交未知只核实该项原请求。
3. 停止先保存原文档记录 Cancelled，再持久化/逐项履行停止责任。原 Roadmap 包提供一处执行枚举，供派发、队列重建及 App 恢复/停止读取，避免各消费者分别猜根级/数组形态；同步 Settled/ObservationComplete 按全部执行事实判定。复用 [Runner 调用生命周期](../../docs/product-design/runner-lifecycle.md)，不另建重启流程或部署协议。
4. 增加窄命令 `roadmap_retry_documents`：target 为原文档记录，expected_version 为其 revision，payload 为 `{local_key:...}`。一次只处理一个失败且结算任务；停止、有问题需重规划、发布中/已发布及其它不合格状态拒绝。页面逐行提供按钮，不新增多选状态。资格还须沿原同来源检查排除其它未结算记录/在途发布，并拒绝同来源已有更新文档批次的历史记录；重生成检查跳过当前文档记录，允许其它任务继续运行时重生成已结算失败项。从既有 CreatedAt/ID 推导最新批次，不维护 latest 字段。新建生成/重生成及其原回执核实共用既有 generationMu；首次发布意图的资格检查/保存也在一致锁序的既有写入边界串行重核。模型及远端发布操作在该锁外；初次生成沿用既有 generationMu 覆盖只读 Git 材料准备至意图保存，保证原请求核实不会把仍在准备的请求判为未提交。此处不新增 preparing 状态、两阶段提交或额外在途索引；重生成与首次发布意图检查/保存仍为短临界区。
5. 原请求号绑定文档记录和 local_key，幂等写入新执行及请求关联。最后追加项自然成为该任务当前执行，历史/成功项保留；已有观察按 generation_id 更新对应项并经 CAS 保存，不能按数组下标或旧快照写回整个记录。不重确认、不改固定上下文。

## P4. 汇总并接回原审阅、发布和活动

1. 所有任务最后执行通过且 settled 后，一次性汇总到原 Documents，沿既有 DocumentsVerified 完成采用，复用编辑、TestsJSON、review_digest、发布和入图事务。后续观察不得重新汇总覆盖已编辑正文；补“编辑后再次观察/重开保持正文”的断言。共用安排取确认规划，各项代码调查保留原引用，不调用汇总模型。保留原审阅/发布里程碑，进度从执行事实推导，不增加另一套批次执行状态机。
2. 批次的发布业务限额与单项模型 1 MiB 限额分别检查，超限明示；不得把完整批次再次送入“单 Run 输出”解码器。部分成功只读预览、禁止发布/入图；编辑后的 provides_tests/范围契约仍受确认快照核验。
3. 更新 API 类型、命令注册/host request digest、读投影及原请求关联。操作仍以原文档记录为目标，复制信息定位具体 generation_id/Run；沿已有详情读取，不为执行项新增一套资源路由、分页仓储或独立活动源。列表/活动不复制全部正文或伪造整批 Run。
4. 扩展 Roadmaps 的既有任务详情展示完整规划及协作信息；生成区域显示总数和逐项状态、错误、所选失败项重生成及整批停止。复用现有操作反馈/活动入口、历史隔离和跨项目版本守卫。
5. 在 `roadmap_recovery.go`、`application_verify_stop.go` 等原 Roadmap 资源枚举处展开全部执行，供原停机/排空流程消费；验证父记录无根级 Run 时仍能覆盖活动资源。离线验真消费者原来只读取批次根 Run，已明确补为同一 Executions 枚举；只改资源集合，不新增部署协议，因触及停机验真 owner 补选 go.deployment，沿原夹具验证每项停止证据。

## P5. 提示词与文档治理

解析短指令/完整材料补必要协作及提供关系，文档短指令/完整材料明确只完成目标 local_key，并保留本任务代码调查；同步实际受影响的默认 SpecGuide、静态准备资产、调用正文黄金样本和守卫。只从原 prompts 包/材料组装规则，业务层仅传代号等事实，缺材料阻断。

实施时在[专属验收](../testing/acceptance/roadmap-per-task-generation.md)保存每个受影响实际正文的完整 Git 差异、UTF-8 字节前后值、业务理由和验证；解析、单任务生成、初始化资产与相关测试接线逐项覆盖。按照[提示词治理](../../docs/prompt-governance.md)安排独立审核；更新黄金样本不能代替审批。实现阶段修改上述提示词、资产和接线；字节数、测试及审核结论仅按实际证据登记，不预填通过。

同步 `docs/product-design/roadmap-promotion.md` 的逐任务生成行为和 `SteerlineFlow/paths.md` 的执行身份说明，以及实际受影响的测试清单/执行器断言。没有新 target 路径需要登记。产品说明和路径文档随实现同步，实际验证范围在专属验收记录。

## 验证计划

正式测试定义、环境和通用命令唯一引用 [test-list.json](../testing/test-list.json)，选择见[同名安排](roadmap-per-task-generation.tests.json)。实现新增行为进入现有 owner/HTTP/SQLite/Vitest 测试及现有受控活动场景，不新增重复的长期测试集。

| 时点/性质 | 验收映射、正式 ID | 具体范围与通过要求 |
| --- | --- | --- |
| 本次文档交付 | 文档一致性，非产品验收 | 文档链接、同名测试 ID 登记/唯一性、Spec/Plan/验收覆盖及 `git diff --check`；不运行模型或 L1/L2 产品测试。 |
| P1–P4 每步后，定向诊断 | RPG-A01–A06；后续由 go.roadmap/go.storage/go.http 覆盖 | 扩展 roadmap/model、queue、SQLite、App/host/API 原测试；必须观测到多项 simultaneously in-flight，以阻塞受控 Runner 和显式同步屏障验证，不能仅断言 goroutine 数或提交次数。补齐下述失败矩阵，定向执行按实际新增测试名固定，不冒称全集通过。 |
| P4/P5 完成后，正式 owner | RPG-A01–A06；`go.roadmap`、`go.storage`、`go.http`、`go.deployment` | 规划完整性、单记录数组保存/不可变/重开、幂等、并发/恢复/停止、原 PR 未知/整批入图，以及新重生成命令权限/revision/摘要通过。原 Roadmap 启停夹具验证全部 Run 的停止证明，停机离线验真补测两项执行的缺证据/部分证据拒绝及全部证据通过，选择 go.deployment 覆盖实际触及的 owner。扩展现有 App 测试根内子项；新增顶层根须同步登记，保留原参数/断言。 |
| P4 完成后，前端及真实受控整链 | RPG-A03–A07；`web.unit`、`runtime.async-activities` | 扩展现有 Vitest 和 runtime 活动场景；六任务确认→并行生成→单项失败/重生成→完整审阅→单 PR/入图扩展 go.roadmap 的原真实 Git/SQLite/provider 整链夹具；runtime.async-activities 运行既有真实应用 HTTP/活动观察及原请求无副作用场景，批次读模型另由 SQLite/App 和生产浏览器验证。复用既有整链避免在外部 E2E peer 再实现一套 Roadmap 解码和发布协议。计数、零匹配/skip、停止与清理按正式集合核验。 |
| 代码完成及提交前，固定回归 | RPG-A01–A07；`l1.minimum`、`smoke.delivery` | 按 L1/L2 正式入口分别使用全新输出目录；提示词正文/守卫及核心交付均通过。 |
| 生产构建后，专属浏览器自动 | RPG-A07 | 新增 `web/tests/roadmap-per-task-generation.spec.ts`，`npm --prefix web run build` 后用 `npm --prefix web run test:e2e -- roadmap-per-task-generation.spec.ts`。沿现有生产构建+受控 API，覆盖1440/768/320px、确认协作详情、混合状态/历史、重复点击、重生成目标和整批停止；GET/轮询零生成副作用。结束释放服务，保存截图/trace/命令与源码身份；此项不冒称 web.unit 浏览器覆盖。 |
| 受控通过后，隔离真实专项 | RPG-A02/A03/A08 | 复用现有 `internal/app/roadmap_live_test.go` / `roadmapPromotionChain` 的两任务夹具，断言两个独立原生 session、执行区间重叠及两组完整文档；六任务五槽规模留给受控链。入口仍为 `go run ./cmd/test-feedback -- -tags=e2e ./internal/app -run '^TestRoadmapLocalCodexPromotion$' -count=1 -timeout=20m`。沿既有独占 home/项目、固定独立 STEERRUNNER_EXE 和 STEERLINE_ROADMAP_EVIDENCE，provider/remote 保持隔离，不接安装服务或真实远端 PR。保存原生输入/身份/时间/结果和全部停止证据，不自动重试失败。不在普通 tests.json 中伪造 live ID。 |
| 自动通过后，人工体验 | RPG-M01 | 参与人/窗口待安排，少量体验后置；不能将真实并发验证缺失降格成人工待验。 |

受控失败矩阵至少包含：目标文档缺失/错误代号、非法 JSON 尾部、provides_tests 漂移/重复/自提供测试、questions 与正文并存、manifest 变化、保存前零提交、提交后丢回执、未知原号重核、收尾占位、共享额度、部分成功重启、停止与派发竞态/迟到受理、单失败项重生成、同请求改目标和并发观察 CAS 冲突。补“其它执行先排队，旧批次失败项后重生成，重开后仍不插队”以及“旧批次重生成与新批次创建/发布竞争，资格失败方零新增副作用”。已有发布/入图断言沿原整链复用，不重新造一套。各反例验证实际副作用/绑定，不只对照实现字段。

本任务使用 `C:\SteerlineDebug\wt-4ef7-roadmap-per-task` 的新 scope，分级与 diagnostics 每次创建独立叶目录并按[路径清单](../paths.md)显式传入。定向日志、正式 summary/managed、浏览器、真实模型证据分别保留并登记；源码 Web 构建与 E2E/重型回归串行。缺环境、skip、超时或资源未结算均按未通过记录，不自动放宽范围或重试。

## 交付边界

实现应交付必要规划补充、原文档记录内的执行数组、并发/生命周期、单项重生成和页面投影，复用原发布入图，并完成提示词治理与上述验证。实际正文差异和独立审核只在实施后登记。本 Plan 不授权提交、推送、创建 PR、合并或部署；本次实现与验证进度登记到专属验收。
