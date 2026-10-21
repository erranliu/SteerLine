# Plan：项目更新与候选版本收敛

状态：2026-10-21 方案已确认并实施；实际验证结果仅登记专属验收。范围归 [Spec](../spec/project-maintenance-simplification.spec.md)，统一规则归[项目维护设计](../product-design/project-maintenance.md)，实际结果归[专属验收](../acceptance/project-maintenance-simplification.md)。普通开发，不占任务编号、不改 roadmap、不创建执行聊天或独立 worktree。

## 固定输入与代码整合

调查源码 b9e5002d38da4722dc8457338e063ce56817fb48。已读 docs/spec/SpecGuide.md 全文，文件最后修改 commit 94e704300485b82d54e44991148074841ed66019，SHA256 85b656090674e1f953320b6be62d17b6888248ef7eec5fe044b9d3f408bfe4de；普通开发沿用户授权，不启动 roadmap 转正流程。采用[通用测试规范](../../SteerlineProjectGuideDoc/testing-architecture.md)、[项目测试指南](../testing.md)和 [E2E 约定](../../tests/e2e/README.md)。

| 调查位置 | 现有消费者与整合 |
| --- | --- |
| app/project_maintenance.go、project_runtime.go、maintenance_routes.go | 复用项目 runtime/actor、合并在途 fetch 和宿主观察；删模式/人工采用/确认编排，增加固定目标的快进命令和自动材料刷新 |
| projectmaintenance/rules.go、model/records.go | 保留文件/严格 JSON/ID 校验与文本摘要；替换人工 Approval 就绪判定为版本化生成结果就绪；区分材料摘要和生成输入摘要 |
| testmanagement/generation.go、apply.go、model/records.go、app/test_management.go | 复用持久原请求、generate/1、profile 裁剪、原结果和停止 owner；去工作区目标/写文件/审阅应用依赖，结果直接进入项目库 |
| taskrunner/service.go、base.go、task/model/work.go、sqlite/runner_candidates.go、maintenance.go | 删除 Task 唯一执行快照和公共 base 约束；候选保存完整不可变契约/base/测试来源，新建与重放分别处理 |
| testmanagement/evidence.go、app/test_management.go、product/query、runnerstate/candidate.go | 新鲜度独立投影，过期不等于退出；证据收集/展示共用同一次当前性判断，保留原历史 |
| taskrunner/actor.go、sqlite/runner_result.go、task/model 交付校验 | 交付读取所选候选契约；材料过期不增加拒绝条件；唯一交付/图释放/全部候选停止义务保持 |
| web/features/project-maintenance、tasks、runs、shared/api、app/use-candidate-starts | 一个更新面板、任务最新材料、候选差异/保留状态；各视图按同一材料版本计名额，原请求不跨项目 |
| driver/project_maintenance.go、test_management.go、debug-template、suites.json | 改现有夹具/两条场景，不新增并行业务链；移除夹具显式批准及旁置安排前置 |

上表路径均相对 internal、web/src 或 tests/e2e/internal。候选与材料共用的纯 TaskContract/FileReference 声明抽至 internal/taskcontract；task/model 保留类型别名及全部校验/DAG/交付行为，Runner事实层不依赖Task业务模型。同步架构守卫并保留禁止业务依赖的反例。

不新增通用 Git 管理器、维护任务 DAG、生成器队列框架或新的 Runner 协议；公开材料能力须按最新Runner契约核对；内容生成用inline，开发文档用worktree路径，系统安排绑定原执行提示，不伪造文件路径。

## 实施步骤

1. **替换旧权威和数据所有者。** 在现有项目库中保存最新远端材料观察及按生成输入摘要定位的原 Generation；候选保存完整不可变执行契约、base、材料摘要和所用生成/测试快照。删除 task_execution_snapshots 及其端口、reset/读取消费者，消除只换表名仍共用 Task 契约的残留。任务身份/目标/DAG 保持；当前任务材料可更新，不能改写候选快照。材料版本与生成输入分别散列：前者覆盖所有关联文档及规范/清单，后者覆盖实际三个生成输入与规则版本；commit/profile/原请求等来源单独保存。未发布格式直接调整，无旧库迁移/导入。

2. **补齐远端、本地和快进能力。** 沿现有 Git commandRunner 读取分支/HEAD、双侧 rev-list 数量、提交时间/标题/作者和分页日志；PR 关联复用配置 provider 的只读访问，必要时补窄查询适配，不猜 PR 号/只解析合并标题。PR 失败独立于 Git 状态。增加明确的更新本地命令：在现有 actor 保存固定目标/原请求后，按 Git/worktree 操作锁和真实分支/HEAD/干净状态执行快进；执行前重核条件，更新后核验 HEAD，回执丢失只核实原目标。检查命令不会自动快进；HEAD、远端或文件在操作期间变化时保留事实并报告重新检查/人工处理，不追赶新目标。不在长数据库事务内 fetch/调用 Git/provider。新 DTO 包含最后成功与当前失败、前后目标、更新结果和可读原因。

3. **自动刷新和生成。** 宿主装载/五分钟检查、显式检查及新候选准备使用同一远端快照刷新路径；HTTP GET/前端轮询只读。任务原路径自动解析，无人工确认版本门槛。为未完成任务缺失/变化的生成输入登记唯一初始意图，交还原 owner 执行；一个 Plan 同输入共享有效结果，不因 commit/工作区换行/轮询重生成。新版本尚未提交的旧意图可跳过，已受理/未知的旧调用继续原请求核验和资源结算；同 Plan 后继在既有资格允许后执行，不能覆盖或套用旧结果。没有合法 profile 显示一次性设置项；明确失败后用户可点重试，未知不换 ID。重启恢复既有意图，对新版本的首次自动生成与失败重试分开，禁止把每次启动当新尝试。任务关闭后取消未提交需求，已受理按原义务结算，保留其他任务引用。校验输出后直接存储选中完整定义，删除人工审阅/应用/空数组勾选和 .tests.json 运行门槛。

4. **所有候选按最新版本分别冻结。** start_task、start_candidate、自动派发和 CLI/API 调用共用准备逻辑：原请求先查原事实，否则 fresh fetch、同 commit 解析任务全部引用并校验前置，绑定对应生成结果。尚未就绪返回可读状态，不预留无效候选、悄悄排队创建或退回旧材料。预留事务重核 Task/authority/graph、目标及材料/生成摘要，原子保存候选和 submit 意图；请求预留成功即冻结，即使 Runner 拒绝/未知也保持。去 base.go、SQL reservation 的跨候选同 base/contract 限制和五名额总历史计数，改为当前材料版本最多五个参与候选；保留总编号递增、原请求幂等和实际运行资源围栏。每个旧候选的继续/反馈/发布仍基于原输入；远端历史改写不改旧 base，新候选独立核验新 base 与前置，不再要求全部旧 base 位于当前目标。

5. **统一新鲜度、测试及文档差异。** 后端输出材料 current/outdated/unknown 和变化文件，作为各视图、测试收集和名额的共同依据；不新增 Runner workflow 状态或把旧候选置 abandoned。比较实际内容，代码单独前进不使文档过期；观察失败保留最后已知状态并标记当前待核实。扩展 PlanTests DTO：当前关联是否可用、原因、原快照、历史结果；过期不发起新 collect，原未知收集只对账，晚到报告只存原候选历史。当前候选仍核对 Run/input_digest/定义/clean/commit，不用模型 final 推断通过。所有文件（含测试快照）的候选版本与最新版本可读及对比；分页有完整读取入口且不能拼混两个远端版本。删除文件和当前材料不合法时，原文仍可查、最新侧显示具体问题。

6. **修正多版本交付与停止。** Deliver/TickPR、SQLite result 和 task/model 的契约校验读取所选候选快照，而非任务最新材料或已删 Task 快照。过期只是展示/测试限制，不改 Participation，不阻断原反馈/PR 核验。旧候选先合并可完成 Task，采用其真实契约摘要登记结果；候选快照目标必须匹配任务固定仓库/分支和原 PR。显式退出/stop、Task discarded 和身份错误仍拒绝。跨版本两个 PR 竞争、生成完成与交付竞争按原事务裁决，唯一 Delivery、后继释放与全候选停止义务不能遗漏；若最新材料/生成失败也允许旧 PR 正常交付。显示实际完成版本，关闭后零新候选/生成，无自动重开。

7. **收拢 Web、协议、配置及旧入口。** 默认面板展示远端/本地提交摘要、时间/PR/日志、检查/更新及问题；任务自动最新材料；候选当前/过期分组、差异和测试历史。移除模式/手填路径/逐项确认/生成 ID/应用文件操作。一次性测试生成 profile 设置沿现有配置与表单，冻结到每次原请求，不因 profile 变化伪装 Plan 更新。新增更新命令沿项目 v2 原信封、CSRF、实例和原请求回执；查询/缓存按项目、候选及材料来源隔离。CLI 去掉旧 apply/人工保存旁路，原查询/停止能力按自动生成记录消费同一服务；不能留下只有 Web 自动而 CLI 仍要求旁置 JSON 的第二条规则。debug-template、项目准备提示词和现用测试指南同步改为维护 Plan/规范/清单/程序，不要求生成安排入仓库；原版本 Spec/验收只加后续入口、不改原通过结果。

8. **落实最小验证能力并执行登记。** 新增 TestPMS* 的纯规则、Git/DB/API 及交付反例，修改被本方案取代的 TestPM*/TestTM* 断言；用已有 owner 分区覆盖新用例，不复制到多个 L1 组。扩展既有 runtime.project-maintenance 与 web/tests/project-maintenance.spec.ts，连续准备三版 Plan、同版本两个候选、过期测试禁用和旧 PR 交付；Git/provider 异常留 L1。扩展既有 testing.management-local-codex，移除人工 apply，证明一次真实自动生成→候选原提示中的系统快照→实际程序报告→API/浏览器链，无需再跑两遍真实多版本矩阵。生成inline材料/候选提示快照、只允许绑定材料的引用校验在 runnerclient/model/SQL 消费者补齐；旧证据隔离规则保持。可执行后同一实现变更同步 manifest、E2E 可读清单、docs/testing/test-list.json、相关指南和 driver/reset；扩展原场景和断言，不把旧场景通过当作新行为证据。执行下表并在专属验收记录源码/构建、选择、失败与补验、证据和清理。

步骤依赖为 1 → 2/3 → 4 → 5/6 → 7 → 8；2/3 的独立内部实现不要求拆成任务或调用子代理。Worker 差异统一维护在[生命周期矩阵](../product-design/worker-lifecycle.md#项目基线与材料维护2026-10-21)：W01 为候选级冻结，W08–W14 生成采用已确认自动策略，W17 保留过期交付，W18 保留原生成/全候选停止义务。

## 测试计划

证据根为本次新建空目录 <证据>，具体路径在[专属验收](../acceptance/project-maintenance-simplification.md)登记。所有新 TestPMS*、场景新增断言、inline 输入与 provider 夹具能力须由步骤8落实后再执行；现存入口运行成功不等于新行为通过。包存在但零命中不计覆盖，必需用例未实现/skip/超时/缺证据均不得通过。

| 入口 | 级别 / 组或场景 | 环境、命令与时点 | 通过条件与证据 |
| --- | --- | --- | --- |
| T1 | L1 / 原 project-maintenance、test-management、app/SQLite/taskrunner/HTTP owner 的 TestPMS*，含受影响 TestPM*/TestTM* | 步骤1–6及协议接线后；隔离临时 Git/bare remote、SQLite、公开 Runner/provider 夹具；命令见下方 T1 | 实际测试枚举覆盖 PMS-A01–A07/A08；正常/反例完整，零安装写入；<证据>/focused/summary.json、JSONL 与 Git/DB/API 前后事实 |
| T2 | L1 / Web Vitest：维护、任务详情、候选/计划测试、开始请求及 API | 步骤7后；mock API/隔离浏览器缓存；命令见下方 T2，再执行 npm --prefix web run build | PMS-A01/A02/A03/A05–A08 的状态/操作/缓存边界正确，生产构建成功；<证据>/web 的输出/构建日志 |
| T3 | L3 / runtime.project-maintenance（扩展现存场景） | 步骤8后；新独立 debug 包、SteerlineTest、真实 Go/Web/Edge、受控 Runner/provider；命令见下方 T3，driver 固定 --simulate | PMS-A09 受控部分：三版材料、FF更新、新候选/旧候选、同版本测试、完整差异、旧 PR 交付及隔离；<证据>/controlled/summary.json、scenarios/runtime.project-maintenance/scenario.json，包内 evidence/project-maintenance 的 API/trace/截图/清理 |
| T4 | L3 / testing.management-local-codex（扩展现存场景） | T1–T3通过后；唯一生成器创建全新 live 包、固定真实 Runner、本机已登录 Codex；命令见下方 T4 | PMS-A09 真实部分：自动生成原 Run、系统结果持久化、零 apply、原提示候选快照、真实程序报告与页面，原资源结算；<证据>/live 的 summary/scenario、公开来源与截图/清理 |
| T5 | 文档静态核对 | 本轮文档完成及实施收尾；python scripts/check_doc_links.py；git diff --check | 本地导航/锚点、Spec/Plan/验收 ID 覆盖、旧/新行为状态和新文件空白正确；证据登记本验收“文档核对”，不记产品通过 |

T1（仓库根目录，使用新的输出目录）：

    go run ./cmd/test-feedback -out <证据>/focused -- ./internal/projectmaintenance ./internal/testmanagement ./internal/app ./internal/taskrunner ./internal/storage/sqlite ./internal/runnerclient ./internal/product/httpapi ./internal/product/applicationhttp ./cmd/steerline -run '^(TestPMS|TestPM|TestTM|TestRunnerPresetFreeze|TestRunnerBase|TestRunnerCandidateReservationAndProjection|TestRunnerTaskDeliveryWithoutWorkflowReady)' -count=1

T2（重定向 stdout/stderr 到 <证据>/web，并保留实际退出码）：

    npm --prefix web test -- src/features/project-maintenance/project-maintenance.test.tsx src/features/tasks/task-detail.test.tsx src/features/runs/plan-tests.test.tsx src/features/runs/candidate-documents.test.tsx src/features/runs/runs.test.tsx src/features/runs/candidate-starts.test.tsx src/app/use-candidate-starts.test.tsx src/shared/api/client.test.ts src/shared/api/queries.test.ts
    npm --prefix web run build

T3 / T4（固定 EXE 由安装目录以外提供；E2E driver 负责唯一 new-debug-release 及实际清理）：

    $env:STEERRUNNER_EXE = '<固定真实Runner EXE>'
    go run ./cmd/test-feedback -level=L3 -suite=runtime -scenario=runtime.project-maintenance -env=controlled -out <证据>/controlled
    go run ./cmd/test-feedback -level=L3 -suite=testing -scenario=testing.management-local-codex -env=live -out <证据>/live

受控脚本 scripts/test-project-maintenance.ps1 可包装同一 T3 driver，不重复作为第二项验收；Windows Job 限制时沿既有 independent-test 启动上下文，不另建测试链。T3/T4 的包各自独立，不把受控冻结方案切为 live。

### 自动验收覆盖映射

| 验收项 | 场景/命令 | 主要必要断言 | 结果登记位置 |
| --- | --- | --- | --- |
| [PMS-A01](../acceptance/project-maintenance-simplification.md#pms-a01) | T1 TestPMSBaseline*、T2、T3 | 数量/分叉、时区/分页、PR缺失/多关联/失败与Git独立 | 同项，引用 focused/web/controlled |
| [PMS-A02](../acceptance/project-maintenance-simplification.md#pms-a02) | T1 TestPMSFastForward*、T2、T3 | 成功、重复/丢回执、脏/未跟踪/错分支/锁/进行中操作、领先/分叉/删除/认证失败、执行前并发变更；候选工作区不变 | 同项，引用 focused/controlled 和原更新请求 |
| [PMS-A03](../acceptance/project-maintenance-simplification.md#pms-a03) | T1 TestPMSCandidateSnapshot*、T2、T3 | 自动文档、不同base/契约、缺路径/格式、CAS、前置、原号重放不刷新/零额外submit | 同项，引用 focused/controlled 的快照/调用计数 |
| [PMS-A04](../acceptance/project-maintenance-simplification.md#pms-a04) | T1 TestPMSGeneration*、T3、T4 | 初始及Plan/规则/清单触发、同输入复用/共享、profile固定、输出ID/空安排校验、零文件写入/apply；规范/程序缺失不冒充通过 | 同项，引用 focused/controlled/live 的原生成来源及仓库摘要 |
| [PMS-A05](../acceptance/project-maintenance-simplification.md#pms-a05) | T1 TestPMSGenerationUnknown* / TestTMGeneration*、T2、T3 | profile缺失、失败显式重试、unknown仅原号、重启不重复、迟到旧结果、取消/停止及关闭任务引用保护 | 同项，引用 focused/controlled 的请求及停止事实 |
| [PMS-A06](../acceptance/project-maintenance-simplification.md#pms-a06) | T1 TestPMSGenerationFreshness* / TestPMSExpiredEvidence*、T2、T3 | 仅相关内容过期、两当前候选独立测试、停新collect/晚到历史、clean/来源保留、未知不宣称当前通过、差异及五名额/资源分离 | 同项，引用 focused/web/controlled 的报告/页面 |
| [PMS-A07](../acceptance/project-maintenance-simplification.md#pms-a07) | T1 TestRunnerTaskDeliveryWithoutWorkflowReady、T3 | 最新生成未就绪时旧PR交付，停止参与反例，新旧同时merge唯一提交、全候选停止、后继就绪/关闭后不生成 | 同项，引用 focused/controlled 的交付/图/停止证据 |
| [PMS-A08](../acceptance/project-maintenance-simplification.md#pms-a08) | T1 TestPM* / TestTM* / HTTP owner、T2、T3 | API/CLI统一、只读GET零模型/文件副作用、按钮/原因、项目与版本缓存、旧apply等入口无旁路 | 同项，引用 focused/web/controlled |
| [PMS-A09](../acceptance/project-maintenance-simplification.md#pms-a09) | T3 + T4 | 受控多版本流程及一次真实自动生成/报告接线分别通过，实际公开来源、非零匹配/完整终态、资源清理 | 同项，分别引用 controlled/live 原报告 |

不选 Full、完整 L1/L2、全部 L3/L4、Orca/原生恢复专项：本次风险由局部 Git/事务/版本/交付 L1 及两条已有 L3 的独有流程覆盖。真实跨版本异常和多次模型生成不再复制受控矩阵；保留一次真实自动生成到程序证据消费的接线验证。实施若改变额外 owner 或出现未覆盖失败，再有依据地追加定向测试。

PMS-M01：T1–T4通过后，用户在新独立包浏览提交/PR、处理一次快进失败、比较过期候选并核对旧PR完成解释。窗口待安排，可后置，仅阻塞体验签收，不阻断无关后继开发。严禁使用/修改/重置 C:/SteerlineInstall、C:/SteerlineRuntime，或复制其数据库/认证/配置准备现场；不得通过改安装库验证新格式。分级回归仍用自身隔离夹具。

## 最新Runner路径语义适配（2026-10-21继续实施）

用户要求继续适配最新Runner，不依赖旧构建；远端main与本机干净源码均核对为4d11148165ca36e4400df46e6bdadc30812fee25。开发文档只传worktree相对路径，原候选契约/内容摘要仍归SteerLine；generate/1保留其正式inline输入。系统安排不成为虚构文件、不写候选工作区或Git，通过公开workflow states的prompt正文与prompt_overrides组合，追加到实施/代码审计执行提示中；保留原模板/自定义正文、报告/发布提示、默认模型及原未知/停止规则。原预留请求不重组新提示。缺公开模板时明确失败，不回退旧材料复制。

实施：消费窄的公开prompt元数据；路径化Spec/Plan/common/acceptance/规范/清单；将系统快照与测试说明写入候选不可变提示正文；SQL预留核对路径及快照正文，删除inline伪文件旁路；保持配置profile摘要与实际冻结请求分别表达。E2E改核对Runner公开Run.workflow中的实际快照正文以及worktree Material路径/commit/空复制摘要，不再要求旧snapshot_path。

最小测试计划增量（证据根独立登记专属验收）：

| 验收项 | 级别/组或场景 | 环境、命令与时点 | 通过条件/证据 |
| --- | --- | --- | --- |
| PMS-A03/A04/A06/A08 | L1 原runnerclient/app/taskrunner/SQLite owner | 实施后，go run ./cmd/test-feedback -out <证据>/focused -- ./internal/runnerclient ./internal/taskrunner ./internal/storage/sqlite ./internal/app ./internal/testmanagement -run '^(TestPMS|TestTM|TestRunnerExplicit|TestRunnerBase|TestRunnerCandidate|TestRunnerTaskDelivery|TestRunnerPresetFreeze)' -count=1 | 最新公开模板/自定义模板保留，普通及审计上下文，缺模板拒绝，路径/篡改快照/旧inline拒绝，原请求重放与新版本独立；summary/JSONL |
| PMS-A08 | L1 架构及选择守卫 | Go定向通过后，go test ./internal/testkit/boundary ./cmd/test-feedback -count=1 | 纯值/Task/Runner边界、owner登记仍通过；日志 |
| PMS-A04/A09 | L3 testing.management-local-codex | 前两项通过后；最新4d11148真实EXE、唯一生成器创建全新live包；go run ./cmd/test-feedback -level=L3 -suite=testing -scenario=testing.management-local-codex -env=live -out <证据>/live | 自动真实生成，系统结果进入实际冻结提示，全部Material为真实worktree路径，零虚拟文件/复制快照，程序通过/失败及API/Web链与原资源/专属refs清理完整；summary/scenario与包内proof/binding |
| PMS-A08 | 文档 | 完成时python scripts/check_doc_links.py及git diff --check | 新规则/实施状态/实际结果一致 |

不重复Full、其他L2/L3、此前多版本T3或整套Web单测：未更改Web、版本比较、PR交付和生成调度；这些范围由原证据及本轮定向回归覆盖，新增风险是最新Runner消费传输，以一次真实L3直接验证。审计分支用最新公开模板的L1验证，不再额外启动审计模型。结果仅写专属验收，不在本Plan维护实时状态。
