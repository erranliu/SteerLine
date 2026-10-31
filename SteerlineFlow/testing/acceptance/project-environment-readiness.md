# 验收与文档审计：项目 × 环境准备状态与统一派发门禁

2026-10-30最新签收：用户确认本机Codex、远端Orca两条检测流程验收完成；本机Codex按固定受测版本完整正向L4通过，远端Orca按新固定版本完成真实三轮TTY负向闭环，环境仍not_ready，正式正向L4不改写。本机Orca最新续验已进入真实工具执行，但pwsh缺项且报告未完成，见[2026-10-31续验](local-orca-environment-20261031.md)，仍未通过。权威汇总及原始证据见[两条链路签收](environment-tty-failure-report.md)，本机正向细节见[发布范围验收](environment-publication-scope.md)。Runner通用输入提交问题本轮不继续追查；未部署安装环境。

以下保留2026-10-29阶段记录：实施与受控自动验证完成；两个项目的同版本材料已维护、独立审核并经用户批准合并，当时三条原生路径均已实际执行但未正向通过，明确阻断如下。[Spec](../../spec/project-environment-readiness.spec.md) · [Plan/执行命令](../../plans/project-environment-readiness.plan.md)。当前聊天按用户授权实施、测试和独立复审；未分配 Roadmap 任务编号。

调查源码 `7617cddc7efac5716c8c38bb63a0157a100acac0`。不继承历史环境适配、安装ready或任务恢复的功能通过结论。

## 2026-10-29阶段性检测流程结果与环境结论

`not_ready` 只表示尚无充分资格、须阻断派发，不等于已证明环境缺少能力。必须分别判断检测流程是否成功完成、环境要求是否被有效检查。有效执行并取得原始失败证据的具体检查项，才可认定该项要求不满足；检测器、Runner 接线、原生工具或取证失败时，未完成项保持未验证，不能归因于目标环境缺工具/依赖。

| 路径 | 已知检测流程结果 | 当前可成立的环境结论 |
| --- | --- | --- |
| 本机 Codex | TTY、原 L1/L2 有通过证据；发布未成功完成，原生报告 GitHub 403，另有暂存范围诊断 | 已验证项目执行/测试部分；发布能力未获完整证明。403 的账号/权限/接线根因尚未区分，不能断言环境必然缺权限，更不能断言缺构建依赖 |
| 本机 Orca | Runner 在 Agent 启动前拒绝当前模式；已证明固定 Runner 的本机专用 runtime 前置与严格同流程要求存在冲突 | 检测通道/集成能力阻断，项目环境尚未检测；不是已检测出项目环境缺少工具或依赖 |
| 远程 Orca | 原生 final 报告 TTY CreateProcess 错误，原工具回执未独立核验；验收结果仍为初始内容，prepare/L2未完成 | 检测执行未完成，环境要求未验证；不能把该工具错误直接写成缺 PowerShell、Unity 或其他项目能力 |

因此本轮三条正向 L4 均未通过，但没有证据支持“三个环境都已检测出缺能力”。对具体能力的有效负向检测，仍须保留检测命令、原工具退出码/输出、适用要求及原报告绑定；环境要求未满足与检测流程失败应分别说明，二者都不授予 ready。

## 实施前文档审计与修订

用户要求审计Spec/Plan、警惕过度设计及结构债。审计读取两份文档及原execution配置、环境service/model、SQLite活动保存、app准备/占用/预留接线、taskrunner.Start/Dispatch、材料准备和页面投影；未修改这些业务代码。

| 编号 / 优先级 | 确认问题与依据 | 已修订的边界 |
| --- | --- | --- |
| R01 / P2 | 原Spec §3新增environments/profile.environment_id/generation，又从profile重复核对其上下文，扩大配置版本与保存器。现有ExecutionInput已完整声明adapter/target/settings | 由现有execution统一派生环境键；配置格式/保存器保持；外部变化按项目显式失效，不增全局generation/epoch |
| R02 / P1 | 原Spec §4将六阶段、通过摘要和最后通过指针再存一份，要求每次活动完成双写；原Record已保存状态/材料/报告，容易出现冲突资格 | 当前表只保存活动选择/失效及revision；完成只更新原活动，资格与六阶段统一投影；开始的引用/活动原子创建，失败不回退历史成功 |
| R03 / P1 | 原Spec宣称模型变化可复用准备，但原app.materialAt的Prepared.Inputs包含完整execution；只改变环境键无法兑现资格复用，删执行字段又会破坏原冻结 | 环境键与准备basis共用同一规范上下文；完整execution/profile digest/Run原请求保持，不以准备投影重组原执行；增加共享basis和原字节不变反例 |
| R04 / P1 | 原Plan只强调最终Reserve门禁；实际Start先调用PrepareExecution/PrepareTestPlan，可能先进入材料生成，最终拒绝仍已有新副作用；Dispatch每tick先分配新ID | 增加选定profile后、准备回调前的窄准入回调，app装配同一规则；自动派发在分配新ID前检查；最终事务复核保留，不增队列或通用准入框架 |
| R05 / P2 | 原Spec要求运行故障自动撤销资格，却未给出可核验的公开环境故障分类来源，容易扩张Runner契约或把测试失败误判成环境故障 | 首版只有有限依据比较和用户显式失效；普通运行失败保持原诊断/恢复，不增自动归因或跨项目失效机制 |
| R06 / P2 | 原Plan新增人工清单确认/awaiting_user持久阶段，并强制回归因新环境注册表而扩张的配置保存流程，形成非必要命令和测试范围 | 清单为只读说明，既有开始操作承担验收授权；删除确认状态/命令及配置改造/对应强制E2E；保留门禁所需的owner、单个L3和参数化真实驱动 |
| R07 / P2 | 原Plan把“同事务更新活动/当前状态”直接接原SaveEnvironment，后者自行BeginTx；外层当前引用初始0与原项目/活动版本也未分清 | 原保存校验提为同包私有tx helper共用，不嵌套事务/复制校验；开始/失效payload使用expected_current_revision，外层及停止原活动revision保持各自语义 |

上述为本轮确认并已修订的文档问题，非已实现的代码修复。保留必要的新增量：一个准备上下文纯规则、一份最小当前引用、一个准备回调前预检及原候选事务复核；现有活动/配置/Runner继续各自持有原事实。没有安排通用环境平台、双状态机、跨库事务或新恢复框架。

## 自动验收计划

| ID | 方法、时点与通过条件（命令见Plan第3节） | 当前状态 / 影响 |
| --- | --- | --- |
| PER-A01 | P1，纯模型及app：原execution派生键；模型/effort/name共用，目标/权限/Agent入口分离，跨项目独立；配置格式不扩张 | 已通过纯模型、app 分组及受控项目隔离；原生组合另见 A10 |
| PER-A02 | P1/P2，真实SQLite：开始引用/活动原子性、原回执、当前失效、迟到结果、不回退旧成功；观察只更新活动，展示零持久副本 | 已通过真实 SQLite 当前引用/CAS/失效/迟到与原回执核验 |
| PER-A03 | P2，公开Runner fixture及原owner回归：三路径同标准，材料/清单/check无资格；TTY/L2/PR/封存/收尾缺一拒绝 | 原硬标准 owner 与受控公开准备链通过；真实三路径尚未全部正向通过，见续验节 |
| PER-A04 | P3，service/store：手动/自动/默认/新增候选；未准备时该Start的PrepareExecution/PrepareTestPlan及Runner写调用为零，必要原实例读取保持；事务拒绝无领取/编号/候选/submit，自动tick不堆新ID/失败命令 | 已通过早期刷新/零准备调用、自动 tick、真实预留及公开拒绝断言 |
| PER-A05 | P3，SQLite屏障/目标fixture：开始/失效与预留两种提交顺序、整体回滚、同目标不同准备键占用、锁顺序/重启重建 | 已通过两种变更×两种提交顺序、旧 proof 回滚/重放及本机同 target 占用 |
| PER-A06 | P1–P3，真实Git/纯规则：完整原execution字节/digest不变，准备basis共用；有限变更/显式失效拒绝，普通提交不机械失效，无故障归因旁路 | 已通过完整字节保留、Codex/Orca 模型分组及有限真实 Git 依据变化反例 |
| PER-A07 | P4，Web/HTTP及生产构建：服务/环境/派发区分，分组/投影优先级、清单/下一步/历史、任务禁用/停止可用、缓存隔离/可见性 | Web 236项、生产构建与 HTTP/server 通过；包含超20条历史、缺资格禁新开始和失效信封 |
| PER-A08 | P2/P4，原owner及恢复L3：枚举全部原活动而非仅当前引用；原Run/候选/PR/停止/换代未知零重发，不新增资源owner | 原环境 owner 与两个恢复 L3 分别通过；原活动继续为全部资源清单 |
| PER-A09 | P5登记后，L3 controlled `testing.project-environment-readiness`：两项目两环境公开准备/门禁/失效闭环和精确Task/Candidate/Runner计数及清理 | 已通过正式受控 L3，385.0s；两项目/两上下文/失效/旧交付及清理 |
| PER-A10-LC | 受控通过后，独立新包本机Codex L4 live：实际工具/TTY、完整L2、真实PR/收尾、正式门禁；不用安装任务 | 已执行三轮：TTY/L1/L2通过，发布暂存诊断及 GitHub 403 阻断；正式失败，停止/封存和补充清理见续验节 |
| PER-A10-LO | 同一本机参数化驱动，另一新包本机Orca L4 live，独立输入和原生收尾证明，不能借用LC结果 | 已执行：共享外部取证通过；固定 Runner 本机专用 runtime 前置不满足严格同流程，零业务输入；原活动正常停止，正式失败 |
| PER-A10-R | 现有远端驱动扩展，独立remote profile/项目SHA/Runner/预算；真实完整正向验收/PR/收尾和门禁，负向正确不算ready | 已执行：固定 fixture/原注册仓库不变已核实；已核实身份的原生 final 报告 TTY CreateProcess 失败，原工具回执未独立核验；prepare/L2/PR未通过、正式失败，收尾范围见续验节 |

PER-A10-LC/LO/R共同对应Spec的PER-A10，分别记录；能力不足必须明确阻断，不能将前置缺口改记人工待验。新增驱动/场景在实现并登记前不运行零匹配命令。局部owner结果不替代L1/L2或产品全链。

## 人工体验

| ID | 参与和步骤 | 状态 / 影响 |
| --- | --- | --- |
| PER-M01 | 用户或指定参与人，窗口待安排；独立新包自动项通过后，从未准备找到材料/责任、处理失败、理解任务阻断并定位原报告 | 未执行人工体验；可后置，不替代自动项或真实能力 |

## 执行与证据

实现执行前固定源码/文档SHA、Runner EXE版本/SHA及公开能力、项目完整SHA/材料/验证base、原execution与派生环境键。live另固定模型/实际账号权限/PR provider、独占资源和明确开始授权，不记录凭据正文。

原始报告放 `C:\SteerlineDebug\wt-5069-project-environment-readiness\` 独立scope，按[路径清单](../../paths.md)创建每次新叶目录，登记准确路径/命令/终态/清理。保留状态引用/原活动/原请求回执、basis和冻结执行对比、CAS交错、Task/Candidate/Runner计数、Web断言、live PR/原Run及停止证明。失败/补测分目录，未知资源保留定位；不读取安装库作夹具。

## 文档检查

只证明当前文档静态一致性，不改变任何功能验收项。

| 检查 | 状态 | 结果 |
| --- | --- | --- |
| 修订后本地导航链接 | 已检查 | 2026-10-29，`python scripts/check_doc_links.py`：610份Markdown、3878个本地导航链接、0错误；exit 0 |
| `git diff --check`及新增文件空白/表格/围栏/验收映射 | 已检查 | 2026-10-29，diff检查exit 0；三份新增文档UTF-8/末尾换行/空白、表格列、代码围栏及PER-A01–A10/PER-M01映射通过；未执行功能测试 |


## 实施范围与结构收口

核心只新增纯上下文/资格值、一张最小 current 表及原候选事务复核。SQLite 新格式为34，不加旧版迁移/读取/双写；execution 配置仍为3，不加环境注册表、profile.environment_id、generation、人工 ready 或全局失效服务。current 只承载选择与撤销，原活动是执行状态唯一来源；晚到结果不写 current。新候选保留最小 proof，原重放优先返回原事实。

Start 在选定方案后、PrepareExecution/PrepareTestPlan 前核验并沿原维护刷新；Dispatch 在分配新请求前廉价只读核验。原 home target 临界区覆盖本机，SQLite 内无 Git/Runner。页面共用按项目/实例隔离的结构化查询，保留只读材料、轮次/报告、原停止及历史；所选活动不依赖最近20条窗口。旧 Spec 与 HTTP 协议已同步替代范围。

受控系统 helper 经正式 environment_start、公开 Runner 报告与 closed 结果取得资格；只在 owner 夹具构造准备记录。没有生产测试开关或本机免门禁。完整 owner 夹具显式装配准备端口并经过生产预留校验，未知请求仍计入业务额外输入。

## 实施独立复审

独立只读审阅者 `/root/readiness_review` 完成初审、修复复查与最终快照复核；最后结论：无剩余确定代码阻断，提示词正文及接线审核通过。未由主会话自审替代，也未操作安装环境。

| 发现 | 修复及证据 |
| --- | --- |
| 开始预检原先未刷新当前材料观察，可能先生成作业 | Start 专用 refresh，复用原 maintenance.check；PrepareExecution 前拒绝反例通过 |
| environment port 构造时 maintenance 尚未赋值 | 原 runtime 创建后接回同一 maintenance；受控正式 Start/查询通过 |
| 失效提交未知可能把已提交 applied 覆写为 rejected | 只对账原 command；不能读取时维持 outcome_unknown，不覆盖；SQLite/app 反例通过 |
| 当前报告被挤出最近20条时请求却不显示 | 选中详情补入 displayed；Web 原报告与停止反例通过 |
| 真实预留提交交错/本机占用覆盖不足 | 四种真实 Reserve 顺序与旧 proof、原重放、无领取/编号及本机不同 key 同 target 反例通过 |
| 新本机 L4 取证仍强制远端且硬套单项目格式 | 仅本机 Codex 按原 Run workspace 只读取证；本机/远端 Orca 共用 repo/terminal；固定提交唯一识别两套已有完整原验证器；未知/歧义前置阻断；实际 pwsh 读取/失败/相对路径测试通过 |
| 真实材料不同源可能在创建 ref 后才失败 | L4 输入阶段复用原 SameRules，先于构建/项目/远端 ref 阻断，不补材料、不写资格 |

## 提示词实际正文与批准

业务原因：本机 Codex/Orca 与远端复用同一固定执行标准；原“仅远端”的主语会错误限制本机。只扩展执行位置，不改 TTY、用户准备责任、三轮、完整 L2、真实 PR、未知只对账、禁止修环境/修改标准/手写报告或收尾条件。小型短指令模板未改，无 inline 兜底。`prompts_test.go` 仅更新两份已批准实际材料的 SHA，未靠黄金样本代替独立批准。

| 文件（UTF-8、LF正文） | 前字节 | 后字节 | 实际正文替换 |
| --- | --- | --- | --- |
| internal/prompts/materials/environment-execution.md | 4257 | 4263 | `Runner启动的远端Agent`→`Runner启动的目标Agent`；`用户负责在远端预先准备`→`用户负责在目标环境预先准备` |
| internal/prompts/materials/environment-generation.md | 5488 | 5500 | `验证目标：远端完整L2`→`验证目标：目标环境完整L2`；`同一远端Agent`→`同一目标Agent`；`用户在远端预先安装`→`用户在目标环境预先安装` |
| SteerlineFlow/environment-execution.md | 4257 | 4263 | 与第一份逐字节一致 |
| SteerlineFlow/environment-generation.md | 5488 | 5500 | 与第二份逐字节一致 |
| SteerlineFlow/environment.md | 3834 | 3834 | 标题`远端环境`→`执行环境`；`Runner 远端 Agent`→`Runner 目标 Agent`；`同一远端 Agent`→`同一目标 Agent`；磁盘 CRLF 另计3878字节 |

完整原行正文 diff 与前后字节/摘要记录于本任务外部 `implementation-audit-final-957b2b58b18143b489cff50f819c62b5/prompt-body.diff` 和 `snapshot.json`。独立审阅按实际 diff 批准上述范围，材料镜像一致；原强制标准保留。验证为 L1 固定提示词、DocumentDrivenRuleCopies、原环境 caller/HTTP owner 及受控正式环境链，原生通过仍独立待验。

## 本次原生输入查证与阻断

用户要求自行查找 profile。没有读取安装配置作为运行回退：从 `C:/SteerlineDebug/dev-20261029-102205-f55ac94e/home/execution.json` 取得真实 local Codex 路径和既有 luna/medium 方案；Orca 使用独立 `C:/SteerlineTestConfig/l4-remote-profile.json` 的稳定 transport/access/Agent 参数，改为 local 并使用已找到的本机 Codex 路径，保存为新的独立测试 profile。后者只证明固定配置可读取，不证明原生启动或 TTY 可用。

固定输入在 `C:/SteerlineDebug/wt-5069-project-environment-readiness/diagnostics/live-inputs-f669a99b91724934b1ed54dd0a041233/`，含两个 profile、项目配置、原始 offline capabilities/version 及 input-source.json。独立 Runner：`C:/Project/SteerRunner/bin/steerrunner.exe`，版本0.2.0-dev.3-r3、源码af29ee471587d7eef41a5eb9ff7d52aefb815183、资源f5cce294299a52cf641eb4c9de26e1dc74477a4fa5eb9a7edc386edd573aff7b；EXE SHA 见原输入记录。离线契约查询 exit0。

本机两次使用 SteerlineTest `0341d001d030f931c8f8392d6053ede603d2a02d`、900秒；远端使用308 `0f0b4f133d088600e538ee3639d9b2d1e32791e6`、独立远端 profile、1200秒。三次正式入口均在同源执行规则预检失败；groups=0、未创建 Agent、PR或远端 ref，scratch 清理已核实。需要先经项目正常维护提交与本构建同版本的执行材料，再以新 SHA 运行；本任务没有静默改写两个项目来绕过门禁。未运行真实工具/TTY/L2/PR，不记 ready，也不把负向结果视为 PER-A10 正向通过。


## 自动执行结果与原始证据

证据根固定 `C:/SteerlineDebug/wt-5069-project-environment-readiness/`，下表目录均相对此根；每次失败与补测使用不同目录，未覆盖原日志。源码 HEAD 仍为7617cddc7efac5716c8c38bb63a0157a100acac0，功能为未提交 worktree diff，未声称是已合入提交；各正式系统报告冻结自身 EXE/test binary SHA 与 source_dirty。最终 Spec/Plan、变更文件 SHA 与实际正文 diff 另存外部 audit 快照，便于核对本轮内容。

| 范围 / 正式入口 | 结果 | 原始证据目录 |
| --- | --- | --- |
| 固定 `-level=L1` | 通过；准备0.4s、执行10.9s；不是 Full | `test-results/L1/final-966354d1023b409eb0c41ae88e518467/` |
| 固定 `-level=L2` | 通过；核心交付161.2s，清理true；夹具先正式准备 | `test-results/L2/20261029T055420020Z-caa351251d4b48bfa985fb531446385d/` |
| `-level=L3 -suite=testing -env=controlled -scenario=testing.project-environment-readiness` | 通过；385.0s，清理true；2项目/2上下文/2候选，各唯一交付 | `test-results/L3/20261029T055417207Z-a1243666aa6f4f6e90276341730084b9/` |
| recovery.runner-wait-retry、recovery.waiting-merge-restart | 两项分别 passed、清理true；330.4s/174.0s。该联合调用的总summary失败来自当时新readiness场景超时，不能把联合调用整体登记通过 | `test-results/L3/20261029T054734027Z-a194b61abd36450587f7141b8cdea9d5/managed-recovery.runner-wait-retry.json` 及同目录 `managed-recovery.waiting-merge-restart.json`；原package e2e-1通过 |
| `scripts/test-web.ps1`（pwsh） | 生产构建通过；32文件/236客户端断言通过；HTTP/server通过30.6s | `diagnostics/web-20261029T055501418Z-db15a7dada054a248cf67e0a0cf3140b/`；server原报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261029T055526.428578400Z/summary.json` |
| 受影响 owner 全包：sqlite、taskrunner、environmentadaptation/...、remotetest、test-feedback、testenv | 7包270测试 passed、零skip；44.8s | `diagnostics/owners-ce4b2e01807d446fbb3a4b2fc7fc4cf5/` |
| 最终 app 原环境与 readiness 定向（Plan原前缀） | 16测试 passed、零skip；234.0s；新增HTTP缺失/null引用版本反例 | `diagnostics/environment-final-a2a5d900533e4a2fb685806def2da91a/` |
| L4 driver/input/原summary读取/登记选择定向 | 28测试 passed、零skip；5.8s；真实 pwsh 原工作目录证据读取成功/失败均覆盖 | `diagnostics/l4-driver-090940c4e8f24c19bff382f33128cd45/` |
| 本机Codex正式 `testing.project-environment-local` | 前置失败；原项目规则不同源，未执行原生验收，清理true | `test-results/L4/native-codex-5b814fbeff2840fc8d7857e75f587e3a/` |
| 本机Orca同一参数化正式入口 | 前置失败；同上，清理true | `test-results/L4/native-orca-8d5b51c4f59e48579e68b4938405e98d/` |
| 远端正式 `testing.project-environment-orca` | 前置失败；308原项目规则不同源，未执行原生验收，清理true | `test-results/L4/native-remote-de1fdae3e6894a9dbeef93769dbfeaf9/` |

owner最终 app 命令：`go run ./cmd/test-feedback -out <表中独立目录> -- ./internal/app -run 'TestProjectEnvironmentReadiness|TestProjectEnvironmentExecution|TestProjectEnvironmentWorkflow|TestProjectEnvironmentAdmission|TestProjectEnvironmentGeneration|TestCallerPromptConvergenceEnvironment' -count=1`。其余实际完整 argv、选择和源码环境见各原 summary，未执行默认 Full 或全部L3/L4。

先前失败保留：diagnostics/20261029T053544424Z-* 为候选错误文字断言差异；20261029T054204804Z-*、20261029T054710670Z-*、20261029T055016353Z-* 暴露 owner 夹具缺 graph revision、绝对 state dir 或先行能力前置，补齐后真正到达环境门禁；owners-c86a8d9938424f46bf3c11047fddc814 暴露两个旧 service 夹具未装配新门禁，另一个 actor超时在并行构建争用下超过旧7秒界限，保留原界限原样重跑通过。首轮 L2 只在交付后错误混算环境请求失败；首轮新 L3 用5分钟普通场景时限，双项目操作未完成即取消，改为明确10分钟窗口并重新完整运行通过，不靠放宽业务轮次预算。

Web前几轮分别暴露新 GET 在连接核验前启动、旧mock无readiness、所选远端缺ready、旧服务文案断言及lazy页面1秒等待不稳；正式版 GET 沿 online 守卫，离线保留历史原入口，所有新开始缺资格即拒绝。lazy加载断言使用5秒界限。未删除失败断言、未降低资格或修改生产门禁来取得通过。

最终静态检查：`python scripts/check_doc_links.py`：610份Markdown/3879条本地链接/0错误；`git diff --check` exit0。原资产正文与镜像、固定模板、规范/计划/验收映射核验通过。安装部署、业务任务派发或旧库改写均未执行；人工体验及三路径真实资格仍按上述状态保留。

## 同版本材料维护及原生续验

本节补充上述历史快照，不覆盖原失败报告。材料更新证据位于 `C:/SteerlineDebug/wt-5069-project-environment-readiness/native-completion-20261029/`；本节后续 `diagnostics/` 与 `test-results/` 现场路径仍相对任务证据根 `C:/SteerlineDebug/wt-5069-project-environment-readiness/`，不位于材料子目录。在普通独立 clone 上仅更新五份材料，脚本、测试规范、清单及 PR 标准保持原字节：

| 项目 | 固定材料提交 / 已批准合并 | 实际 UTF-8 LF 字节变化 |
| --- | --- | --- |
| SteerlineTest | `1e794704e72461c065fcd029e5eec0b910336834`；[PR 694](https://github.com/erranliu/SteerlineTest/pull/694) → main `9e0489047af5f16f14879fbcbe08d4e1f43ee702` | execution 4257→4263，generation 5488→5500；environment 4486→4492 |
| 308 | `49c4517565880ffff27f3abbb20cabfbefdff997`；[PR 440](http://192.168.0.66:3000/zymobile/merge-h-308-code/pulls/440) → AIRefactor `4e1bc3fe31f8bc0ec388e74f022ba26b48e3c555` | execution 4257→4263，generation 5488→5500 |

实际正文仍仅扩展“远端”为“目标环境”，与上节同源规则相同，完整 diff、字节账本与静态校验见 `steerlinetest-materials.diff`、`308-materials.diff`、`materials-update.json`、`materials-static-check.json`。独立审阅者 `/root/native_material_review` 已批准实际正文及接线。SteerlineTest 自身 L1 五项、L2 一项均通过；308 按原规则纯材料变更做静态核验，未运行 Unity 或修改工具安装。经用户明确批准合并两 PR，原干净源码 checkout 仅快进到上述精确合并提交，见 `source-adoption.json`。

用户最终口径：本机 Codex、本机 Orca 分别是独立环境；Orca 本机/远端执行、判定、外部取证严格同流程，仅显式目标选择器不同。原 repo/terminal 操作共用并缓存同一 runtime 绑定及连续日志序号；不为本机 Orca 直读工作区。本机 Codex 可直读原 Run 工作区。三项正式 L4 均必须 ready，负向证据保留但不能记正向通过。

实测暴露的窄接线修复：项目输入复用 app 原严格解码与默认值，显式空值仍拒绝；配置验证成功后才发布自有 ref；Gitea 只读取证使用该项目既有 tea 登录，与 GitHub 使用 gh 一致，不提取/复制凭据或放宽规则；可信 `blocked` 最终结果沿原 stop→stopped→closed 收口，未 verified 仍不提前停止。独立复审确认上述收口及统一流程无新增确定代码问题、无过度设计。回归 `diagnostics/unified-final-d8e927e8152e4372adfcdc4e5fb9d6b5/` 188项通过、零跳过，含新增 blocked 反例；最新 e2e 编译 `diagnostics/unified-tagged-f49308576c3a4a8d940274f851de07fe/` 通过。

新真实现场原失败保留：`test-results/L4/codex-materials-2bfa657df13f4c0791374f928889dd6e/` 为可选配置空值接线失败；`codex-materials-e119029d63b74b1a96f9a69c5d9a9e76/` 为原 sandbox 对临时目录写入权限失败；`codex-full-access-8ce948ccf21e4b68ad8095f1233484fc/` 为编译失败，未创建原生资源；`codex-full-access-c0d4d08ccc454e81ba0e67ee88c0259d/` 已完成真实 TTY、L1/L2，但 GitHub API 403 阻止发布，不能记 ready。前两次配置/运行失败及 403 现场均保留原报告；原运行的公开 `service verify-stopped` 与补充 `post-run-cleanup.json` 核实该现场全部 Run 结算及自有 ref 清理，未将原报告改为通过。新的 full_access profile 是显式独立上下文，原 sandbox profile 保留，不实现运行时权限回退。

继续使用现有本机 GitHub 登录的独立上下文 `test-results/L4/codex-auth-context-2e3a6dc82a574ba08f77f47a6c62deae/`：三轮真实 TTY、完整 L1/L2 通过，发布分别在暂存范围检查、GitHub main API 403 阻断，三轮原 Run 均停止封存，原活动 failed/not_ready。首轮暂存错误只保留模型最终诊断，不能认定为固定脚本错误。外部负证据另暴露查询使用了 Gitea 的 `base_branch/limit`，GitHub 忽略过滤参数而返回不相关历史 PR；现按 provider 使用 GitHub `base/per_page`、Gitea `base_branch/limit`，精确 base 不变。定向测试 `diagnostics/provider-filter-a66762972a6c49998c321a74b29ceadd/` 及 e2e 编译 `diagnostics/provider-filter-tagged-b0b195689bc448ff89cf914350647d79/` 通过，独立复审批准。该次原正式失败报告保持不变，`original-base-pr-reconciliation.json` 独立核实无本次 PR，`308-runner-cleanup.json` 确认原资源结算，测试器按原流程移除已结算 namespace，`post-run-cleanup.json` 另记自有 ref 删除。

本机 Orca 正式执行 `test-results/L4/orca-local-unified-d3aba010f823442981af2eeb7c458f1d/`：同一公开 repo/terminal 取证链已核实固定 fixture `a57afa1aee504970627aca701d49bb399611982c`，注册仓库原 HEAD/状态不变，诊断终端已关闭。原活动 `bf12b22d0b9648dca93339a937c5791b`、原 Run `dace27c15a12a5f172842ac9226f23ab` 在 preparation_blocked 阻断，公开诊断为 `remote_sandbox_authentication_unsupported`，`turn_started=false`、workspace/branch 为空、业务输入为零。固定 Runner af29ee4 的本机 Orca GitHub 路径要求另配 `runtime_executable`；远端不走该路径。不以该本机专用 runtime 路径违反用户的严格统一要求，亦不切换 Codex 或伪造报告。保存 `original-preparation-blocked.json`，经正式 environment_stop 原命令 applied、活动 cancelled，未等待已确认能力不足的三轮重试。正式 L4 保持失败，`308-runner-cleanup.json` 与 `post-run-cleanup.json` 分别保留原资源结算、无 PR 和自有 ref 清理证据；不能宣称本机 Orca 原生支持已完成。

最后一次代码变更后的必跑回归：`test-results/L1/provider-filter-final-43fdcc763eb1415e85a634d29b4c45ad/` passed；`test-results/L2/provider-filter-final-b6031e18a8ab4973bc3f93ce8ee122af/` passed、cleanup_complete=true。这不替代三条真实正向资格。

远程 Orca 正式执行 `test-results/L4/orca-remote-unified-900d93908e7f43eeb412407128422a13/`：同一公开 repo/terminal 取证链核实固定 fixture `c9511d85bd82f88e4205598ecd7fb224f0b5cc0a`、注册根 HEAD/状态不变。原活动 `5cbd074d9b234e4fa46e9f3489aecbb0`、原 Run `6df90914a2ad7c6d59284403e2d39bd8` 取得真实原生身份，`turn_started=true`。已核实身份的原 Agent 原生 final 报告：其 `exec_command(tty:true)` 启动 pwsh TTY 探针时，CreateProcess 返回 `OS Error -1073283067`，prepare、完整 L2 及 PR 均未执行。未另取原工具错误回执，final verified 仅核实该报告的来源，不能当作 TTY 检测通过或错误原因的独立证明。结果仍为初始内容、closed/version1、completed=false，不能判 ready；原 Run stopped，未发送额外 reply/resume 或替代 shell。保存 `original-tty-blocked.json`，正式 environment_stop 回执 applied 后原活动 cancelled/settled，只有一轮，未声称完整三轮。辅助观察器随后遇到 project_unavailable 是测试器已停止该应用，不能误报停止命令失败。

该远端正式 summary 仍 passed=false/cleanup_complete=false；`308-runner-cleanup.json` confirmed 后测试器按原流程移除已结算 namespace，`original-base-pr-reconciliation.json` 独立核实该 Gitea base 无 PR，`post-run-cleanup.json` 另记只删除两条精确 fixture 租约 ref。原 Runner 明确收尾范围为原 Orca 终端，未核实或撤销 detached background processes / execution-host credentials；补证保留这一限制，不把它扩大成全部远端主机资源均已核验。材料、原失败报告、停止回执和 scratch 均保留；未安装工具、修改目标环境配置、切换路径或部署安装环境。
