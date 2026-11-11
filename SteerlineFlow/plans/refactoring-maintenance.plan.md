# 重构与维护最小闭环 Plan

审计修订及最小实现已完成。2026-11-11，用户授权的本机 Codex 与专用 GitHub 分支真实联验通过，原测试 PR 和分支已清理。本期未部署。[Spec](../spec/refactoring-maintenance.spec.md) · [测试选择](refactoring-maintenance.tests.json) · [专属验收](../testing/acceptance/refactoring-maintenance.md)

## 1. 基线与前置

SteerLine 调查基线 `1f64b0ffeb936358786cfe4734f2096765bc4008`，包含本聊天尚未提交的产品设计、解耦方案与只读核实记录；实施时保留这些文档并重新核对最新代码。Runner 前置为已合入的 #141、`c404d6e5c50cc32650f9412f9b8b3449c5789e15`，公开字段以该提交的契约及实际固定 EXE 为准。

本机 Runner `bin/steerrunner.exe.build.json` 当前仍指向 `dce4dff8` 的旧构建；不能据源码合入直接用于联验。实施 P1 在隔离目录从合入提交或明确后继构建新 EXE，记录 source/version/SHA/resource digest，并检查实际 worktree_reuse、generate/file、execute/result、配置派生及内部调用计量声明。安装环境不升级，缺必要能力只阻断维护新请求。

| 现有入口 | 本期复用或修改 |
| --- | --- |
| internal/runnercontract、runnerclient | 增加真实复用及用量字段、准入和来源校验；原协议/原业务控制继续沿既有资格 |
| internal/steeragent/suggestion、steeragentbridge/suggestion | 只参考纯处理与宿主分离方式；不复用其候选待答身份、advisory-v1 或完整文本通道例外 |
| internal/app/project_readiness.go、projectmaintenance | 复用统一项目资格、远端基线与预留锁；不伪造 Task，不因普通任务推进而重写维护 base |
| internal/environmentadaptation、runnerbinding | 参考 execute/result-file 原请求和停止模式，复用纯绑定；不把环境业务状态机复制成维护循环 |
| internal/billing、app/runner_usage.go | 复用价格与金额计算，拆分 scope 后共享纯计算；先修正原候选消费新事件的行为，再接维护聚合 |
| storage/sqlite、product、app、web | 沿现有项目运行时、产品端口、原命令信封、后台生命周期和页面模式接线 |

## 2. 模块与最小记录

新增 `internal/refactoring` 作为唯一业务 owner，由它持有行动、轮次、决策/执行请求和 worker；纯值与预算规则放在 model 子包，供 SQL/查询依赖，不把 worker 或 Runner 客户端带入存储。新增 `internal/steeragent/maintenance` 只放纯输入输出、提示词准备和结果校验。app 注入读取、决策 Runner、可写执行 Runner、存储与计费窄接口；不增加新的通用 bridge 包、插件系统或协调框架。

SQLite 保存行动及其从属操作/轮次、当前持有者、问题、消耗和版本报告。只有一份行动权威；CAS 与事务保证同项目在途唯一和意图落盘后才发送。具体分表按查询需要选择，避免通用事件溯源或两份可独立推进的 JSON 状态；schema 使用实施时下一唯一版本，不迁移或双写未发布旧格式。

每个 Runner 请求保存 purpose（准备/决策/实施/审计/修复/收尾/冲突）、关联轮次、原字节及摘要、Runner 构建/实例/namespace、受理/终态/停止事实。可写执行有 latest_owner_run_id；只有绑定正确的新 execute 被受理时更新，不等它成功。只读 generate 不接管 worktree，不进入可写持有者链。操作未知时保留原意图并阻止第二个后继。

用户的方向和策略选择映射为固定提示材料；预算与发布资格仍由业务规则控制。模型结果用有限信封承载提示词及摘要、人工问题或结束建议，schema 不含任意工具调用/项目状态写入指令。平台不从自然语言提取 shell 动作，也不替模型决定工程收益。

## 3. 实施顺序

### P1：公开契约与费用读取先收口

1. 在 RunInput/Run 投影加入 `reuse_worktree_from_run_id`、`workspace_origin_run_id`、`workspace_identity`；在 catalog 解码 worktree_reuse，并核对 input_property、execute/1、原身份排他/新会话/非任意目录、文件写入结算等实际声明。只对维护组合增加准入；原观察/停止不依赖该新声明。
2. 新 execute 复制行动冻结的完整 Repository、adapter、target 和明确派生 execution；引用最新持有者。校验来源/origin/identity、request、input digest 和 correlation。状态/身份错误按 Runner 原义保存，不把 workspace_not_settled 转成已成功或重新选源；有界观察与后续显式控制沿生命周期。
3. UsageEvent 增加 scope、call_id、call_source、source_result_id、coverage_status、command_id、attempt_id；UsageSnapshot 增加 model_call_coverage。scope 省略按公开契约归 main_session；未知 scope 不猜测。main_session 事件单独验证 counted 数、Token 合计；runner_internal 与内部 covered/uncovered/pending 计数分别验证，保持同 revision 全量分页、去重及上下界校验。
4. 原候选费用入口继续只汇总 main_session，展示内部范围说明，不能因新内部事件误判摘要不一致，也不能悄悄计入原主会话总量。抽取少量按明确 scope 计算的纯价格代码供维护复用；不伪装 internal 为 main_session。新 Runner 原生夹具补到既有 runner-contract/result 专项，固定 EXE 验证生产 schema。

### P2：材料、规则和只读决策

1. 新建集中短模板与完整规则，覆盖准备、只读决策、五方向、三策略、执行交接、审计、收尾和冲突收益复核；仅在 prompts 包组装，不在 app/业务字符串补固定规则。先保持规则结构少量共用文件，不为每阶段建立一套提示词引擎。
2. 将平台固定规则与本行动动态上下文冻结成实际文件。只读请求使用 Runner 正式 file 材料及原 manifest 校验，固定源码/项目依据使用 generate.repository 或明确公开的只读工作区事实；绝不沿用 advisory 的完整文本特例。maintenance 新的正式文件供给方式在 prompt-governance 中准确登记并经独立审核，静态规则不得以缺仓库材料为由内联兜底。
3. execute/1 的 Documents 只作受理形状，不触发 implementation 的材料提示。引用已核实的真实项目测试要求和正式清单两条不同路径；不增加准备 execute、维护 Spec/Plan 或项目规范物化。唯一操作上下文和规则通过 result_file.initial_content_base64 冻结，Runner 初始化文件后首个实际 execute 直接完成切片；缺真实参考材料时作为启动前置报告，不用模型制造规范文件。
4. 首次 result-file 初始化使用生产限额和固定摘要，超限在提交前明确失败，不截断或改走 inline。后续每个 execute 使用独立结果文件路径和原封存，避免覆盖前一 Run 的输入/结果。文件物理写入与 close 未核实前不发起接管。
5. 补解码 Runner 已有 generate.directory，实际权限与目录 access 均为 read_only。首决策通过 generate.repository 读取冻结的远端完整提交快照，后续通过 generate.directory 读取公开 Run.Execution.Workspace，写入与读取串行，完整规则与事实走 file 材料。directory 与 repository 不混用；所选模型由 source execution 明确派生。独立审计另有只读上下文；修改仍仅经新的 execute。证据显示 dirty/untracked 可见且决策无项目写权。

执行结果文件使用小型业务信封，明确本次行动/操作身份、outcome、summary、证据、必要问题/阻断和实际 head/base；报告只在收尾/冲突操作要求。宿主校验字段和来源，SteerAgent 理解工程语义。原生提问导致单次 execute 结束时保留其真实结果，不能发 reply/resume 或假装原轮继续；补充依据后使用新的受管复用 Run。

P2 的资产与供给是本期实际实现和独立审核内容，不能仅写样本后称已接线。原结果/材料缺失时保留阻断，不让模型依赖隐式上下文继续。

只有已绑定且已停止的只读结果明确为 `human_input_required` 或 `invalid_json` 时，在同阶段保存一次新请求。原失败、用量和来源保留，第二次输出失败进入阻断。用真实失败形状的回归核对单次边界及成功来源校验。

已结算的写 Run 用 Runner `workspace-git` 查询原工作区。核实 Run ID、完整 HEAD、干净状态及观察时间，再将最近持有者的事实与实际目录供给审计。没有已核实样本时保留证据缺口。原执行请求仍使用冻结的完整 Repository。

### P3：行动、轮次、预算和人工决定

实现 Start/Get/List/Answer/Finish/Stop/UpdateConflict 的业务用例和后台串行推进，复用现有命令信封/CAS/请求回执，接口命名不成为跨领域通用动作系统。预留、在途名额和原输入同时保存；数据库失败、重复点击、并发请求分别有反例。

先取得统一就绪和远端固定 HEAD，冻结配置、方向、策略、材料和价表依据。首准备、只读决策、正式轮次和收尾分别计数；新轮次递增一次，失败不返还；内部审计/修复不另开轮次。每次后继只有一个持久意图，模型完成、用户停止、额度变化和迟到回执由 CAS 仲裁，停止/结束意图优先于新输入。

预算规则写成小型纯函数，金额用既有精确计算。跟踪实际运行区间和每个 Run 的原用量快照；去重重算行动值，不累加每次查询返回的累计值。美元使用用户已确认的“main_session + 已登记 runner_internal 已覆盖参考费用”，页面固定展示 untracked。已登记来源 pending/uncovered 或价格缺失不算零；有界读取后仍不能核实就转自动收尾，没有美元预算则保留费用未知并继续其他额度。

三策略只选择受审固定材料；人工问题保持原版本，原答复和模型产生的后续提示词各自冻结。已保存未投递、投递未知、原题失效分别处理，不让问题刷新覆盖待核实输入。用户结束仍可等待真正必要的授权/环境决定，预算本身不创建待决问题。

### P4：停止、资源责任和交付

立即停止先撤销新输入授权，再对当前决策/执行保存原 stop 意图；未绑定 Run 的未知 submit 也保留观察和停止责任。停止后完成封存与写者结算，通过最新来源创建收尾新 Run，原 Run 不恢复。映射资源不可用、被转交、原 identity 缺失、未知停止，保持旧作用域与错误。

将全部维护执行、generate、未知请求和停止义务接入 ProjectRuntime 关闭、home Runner 排空、同构建恢复及 reset 的现有集合。重启不清预算，不自动重发或重建输入；核实后仅按原用户明确的结束交付继续已有成果。旧构建活动不跨版本接管，测试使用独立 home。

收尾通过提示词要求 Runner 完成必要验证、独立审计、报告、提交与 PR 操作。execute 本身无 publication 自动节点，维护宿主复用 provider 的只读能力核验原 repo/head/base/提交并采用封存结果；不借 TaskDelivery，也不由 app 写 Git/PR。发布结果未知先查原执行与平台事实，不能盲目再次创建 PR。报告与 PR 正文同源，含失败/撤销/遗留及费用范围，按 head/base 版本保存。发布和冲突操作复用对应原收尾决策已保存的金额、时间与覆盖状态，末审以同一快照核对。晚到用量只更新行动统计。

手动冲突更新取得原行动项目名额，先只读核实原 PR 未关闭/合并，再沿最新来源新建 execute；保持原 PR 与方向，不消耗新轮次或要求追加预算。Runner 解决冲突并重新验证，决策生成收益复核要求，保存新报告及旧版本。代码相关变化使旧报告待复核，自动合并/持续监控不接入。

### P5：最小页面和只读活动

新增 `web/src/features/refactoring-maintenance`，在项目导航加入分页；一个表单、一个当前卡片、底部历史列表，沿现有异步操作反馈与原请求核实。保持默认谨慎/1轮、预算输入、就绪原因、两个运行按钮、卡片内人工答复、结果及手动冲突按钮，首版不做在途编辑或子页面工作台。

产品查询组装行动投影和后端允许动作，GET 不对账或触发执行；费用范围、陈旧观察、停止未核实和发布未知都可见。活动中心新增一个维护行动来源及 maintenance 分类/深链，操作按原 owner 办理，不把每个辅助 Run 变成重复活动，不改变 baseline_check 来源。复用列表/详情轮询、断线和项目切换隔离。

### P6：验证、审阅和长期文档收尾

执行下节正式与专项安排，固定真实受测源码、EXE 和场景版本；独立审核代码、提示词实际正文及其接线/测试，修复后按影响复验。更新产品、架构、Runner 接入、客户端协议、数据模型定位及活动说明，以实际实现更新状态，不改写旧能力核实记录或上游验收。

## 4. 测试安排

遵循[测试指南](../testing.md)、[正式清单](../testing/test-list.json)和[路径清单](../paths.md)。同名 JSON 已同步正式回归与新场景 ID，均有序唯一；测试清单、可读 L3/L4、E2E manifest/driver 同步维护。安排在原请求冻结前确定，不在运行时生成。

本功能已登记两个 ID： `runtime.refactoring-maintenance`（L3/controlled）和 `refactoring.local-codex`（L4/live）。驱动与受管证据校验已经接线，实际执行状态见专属验收，不能由普通任务或旧项目维护场景代替。

下表 Axx 均指 Spec 的 RM-Axx；同一正式 ID 在不同阶段出现时，开发期按影响定向，最终固定版本完整执行一次，不无故重复整套回归。

| Spec 验收 | 自动入口、时点与原因 | 特殊准备及必要断言 |
| --- | --- | --- |
| A01/A02/A03/A04/A05/A06/A07/A12 | P1–P4 定向 owner，经 test-feedback 的隔离 Go 入口 | 新 refactoring、steeragent/maintenance、prompts、runnerclient、billing、storage/query/app 相关根；真实非零。覆盖冻结、latest owner、停止/未知/CAS、费用双 scope 与只读权限，不写镜像纯常量测试 |
| A01/A05/A08/A10 | go.http、go.storage，P3/P5 后最终版本各一次 | 命令身份、事务名额、问题版本、报告与查询纯读；原并发压力按正式清单串行，不改计数/期限 |
| A01/A02/A03/A06/A07 | go.environment、go.runner-contract、go.runner-result，P1/P2/P4 后 | 本地源配置派生、固定新 Runner schema、复用字段、来源/结果封存、混合 scope 分页与真实公开接口；缺能力零提交 |
| A06 | go.deployment、runtime.service-lifecycle、runtime.shared-runner-projects，P4 后 | 非实际部署；辅助 Run/未知请求参与关闭、停止和排空，单项目不终止其他任务，重启不重发 |
| A10 | web.unit，P5 后 | 全部前端回归，新增表单/卡片/问题/范围/错误/历史/深链反例，不只跑新文件 |
| A01/A03/A04/A05/A06/A08/A09/A10 | runtime.refactoring-maintenance，L3/controlled，P4/P5 后 | 真实 Go/Git/SQLite/Web/Edge 与公开 Runner/provider fixture；首次及至少三次后继 execute（四次）、失败后最新持有者、停止后收尾、预算组合、人工答复、同步冲突及无文本冲突收益变化、原 PR 更新、双项目/重复/未知/清理；浏览器与公开请求证据齐全 |
| A10 | runtime.async-activities，L3/controlled，P5 后 | 原来源不回退，新维护来源与 request_links 分类、定位和计数正确；GET 零业务推进 |
| A01–A10/A12 必要回归 | l1.minimum、smoke.delivery，最终必跑 | 保持提示词/依赖守卫、正式任务开发和唯一交付；不把这些通过当作新维护功能通过 |
| A03/A04/A06/A08/A09/A11 | refactoring.local-codex，L4/live，P1–P5 自动项通过后 | 固定合入后新 EXE/模型/价表、独立调试包、测试仓库与专用 PR base；真实至少两轮、预算自动收尾、报告与一个 PR、目标推进后手动冲突续作和资源停止。预算机制确定性边界仍由 controlled 补足；不得触及业务主线或安装现场 |
| A12 | 提示词正文/字节和依赖审核、文档与清单静态检查，P2/P6 | 资产、组装、Runner 实际完整输入及 manifest；独立审核不能以 golden 更新替代 |

owner 定向使用 `go run ./cmd/test-feedback -out <独占目录> -- <相关包> -count=1`。app 新增根按行为命名并登记实际命中，不使用会零匹配的宽泛正则；纯协议/定向不代替整链。正式通用命令和断言引用清单，不在本 Plan 复制另一份定义。新场景在驱动完成前只保留上述拟议身份，不填写通过。

本地 Codex live 只运行本功能所需的一条明确选择，独立 base/分支/PR 和干净调试 home；相关真实费用、源构建、报告/PR 及清理证据登记到专属验收。后端不支持时先记录具体原因，不把真实自动项改成后置人工。体验 RM-M01 可在自动闭环后集中后置，参与人/窗口待定。

## 5. 提示词与证据交接

本次为新能力，新增短模板、规则、golden 的基线为不存在（0 B），实际新增字节和完整正文在实施时测量，不预填估计值。任何既有正文/供给的变化列实际前后字节和完整差异；业务原因分别说明方向、策略、只读边界、反馈、审计及交付。先核对正式固定文件来源、file/result 初始输入和真实 Documents，再审正文；缺材料时拒绝，不补内联规则。

证据 scope 建议 `C:\SteerlineDebug\refactoring-maintenance\`，每次执行新叶目录，遵循 paths 的 test-results/diagnostics/packages 层级。记录 source、dirty 状态、Runner EXE/version/SHA/resource digest、profile/model、用量覆盖及原始输出/清理。失败与补验分别保留；未知费用不写成零，Runner 上游单测和本仓受控结果不冒充真实模型链通过。

实施与验证结果、失败及补验分别登记到专属验收；真实模型与远端测试副作用须取得明确授权，不部署安装环境。
