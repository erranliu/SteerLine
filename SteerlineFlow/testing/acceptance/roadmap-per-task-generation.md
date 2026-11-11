# Roadmap 按任务并行生成：验收安排

唯一归属：[Spec](../../spec/roadmap-per-task-generation.spec.md) / [Plan](../../plans/roadmap-per-task-generation.plan.md)。2026-11-10 按用户后续授权实现并验证；不分配开发 Task ID、不进入 execution-map、不修改安装现场。规格阶段记录作为历史保留，最新实现结果见下文。

## 本次文档交付

用户要求先 rebase 再编写 Spec/Plan。原 worktree 为干净 detached HEAD `3ce94c753f4b610aa3f6c7d70b98d194ad4ecf2e`；获取 origin 后创建 `codex/roadmap-per-task-generation`，成功 rebase 到 `1f64b0ffeb936358786cfe4734f2096765bc4008`，无冲突。未提交、推送或部署。

文档静态检查结果在实际运行后记录于本文末尾；静态通过不表示任何产品行为已实现。

## 自动验收

以下为最初验收定义；当前状态与证据在“实现验证结果”登记。正式 ID 的环境/命令/断言见[测试清单](../test-list.json)，具体时点及专属补充入口见 Plan。实施前须补齐夹具和断言，不得沿历史报告预填通过。

| ID | 方法与最小通过条件 | 入口 / 正式覆盖 | 原始证据要求 |
| --- | --- | --- | --- |
| RPG-A01 | 完整任务和必要协作/提供关系纳入确认；缺失/冲突/规划材料变化阻断；无关代码提交不强制重新解析，不增加全仓调查或隐藏模型补规划。 | roadmap/model/App；go.roadmap、web.unit | 固定规划输入/材料、确认回执、拒绝及无关 HEAD 变化正例、模型调用数。 |
| RPG-A02 | 六项受控夹具分别一个 Run/session、目标代号明确且仅一组完整文档；复用相同两份 file 材料，已有正式任务不生成。 | roadmap/App 受控；go.roadmap；两任务真实专项另验证原生接线 | 原文档记录→local_key→generation_id→Run/Attempt/session/input 映射，逐项结果与 manifest；无子 Roadmap 记录或 target 文件。 |
| RPG-A03 | 五槽首批五项同时在途、第六项等待；共享额度/收尾占位正确，业务依赖不串行化生成；重生成使用新 CreatedAt，不越过既有等待项，重开次序不变。 | queue/App；go.roadmap、runtime.async-activities，原 queue owner 定向补充 | 同步屏障、逐项创建时间/队列时序、其它用途占位、重开和停止结算日志。 |
| RPG-A04 | 保存/提交/回执/重启/停止竞态无重复 Run；未知只查原号，原 Roadmap 恢复/停止枚举不遗漏任何执行，全部结算后释放；CAS 冲突不丢数组更新。 | SQLite/App/host/HTTP；go.storage、go.roadmap、go.http | 单记录原子保存、请求及命令数、重开状态、迟到受理和原停止证明，保留失败现场。 |
| RPG-A05 | 一项失败保留五项成功；一次重生成只追加该任务执行，无多选或当前选择索引；同来源其它活动/发布或更新批次阻断历史重生成，竞争操作不能绕过资格。 | roadmap/SQLite/HTTP/Vitest；go.roadmap、go.storage、go.http、web.unit | 重生成请求/local_key、新旧 generation_id/Run、成功摘要、旧观察 CAS 反例，以及历史重生成与新建/发布竞争中拒绝方零新增副作用。 |
| RPG-A06 | 非法/缺失/漂移/问题结果不采用；缺项不发布，全组通过后一次性采用；编辑后观察/重开不覆盖正文，原单 PR 与原子入图正确。 | model/App/provider 受控整链；go.roadmap、go.storage、runtime.async-activities | 原始结果、采用事实、编辑后观察/重开正文摘要、审阅摘要、发布/merge 核验与入图回执。 |
| RPG-A07 | 正确显示和操作批次/逐项状态/身份，历史和跨项目不串线，宽窄屏可用且轮询零副作用。 | web.unit、go.http、runtime.async-activities 及专属 Playwright | 1440/768/320px 截图、trace、命令信封、GET 和写入计数、清理结果。 |
| RPG-A08 | 两任务真实本机 Runner/Codex 夹具的两个独立 session 执行区间重叠，两组正文完整且全部 stopped；六任务五槽和失败重生成由受控链覆盖。 | Plan 指定的显式 e2e 原生入口；普通清单不新增假 live ID | 固定 EXE 版本/SHA/能力、模型配置、两项原生输入/session/时间/终稿/停止证据；无安装或真实远端 PR 副作用。 |

L1/L2 为实现后的固定必要回归，不因本次文档静态检查而提前运行或登记通过。正式报告必须实际非零、完整、无 skip 且清理通过；定向、浏览器和原生专项独立登记范围。

## 人工验收

| ID | 操作及通过条件 | 参与/窗口与阻塞范围 | 状态 |
| --- | --- | --- | --- |
| RPG-M01 | 查看确认前完整任务/协作信息；观察六项生成、部分失败和仅失败项重生成；确认逐项诊断、复制身份、停止和最终审阅操作易理解。 | 参与人及窗口待安排；自动验证后后置体验，不阻塞无关交付，不覆盖自动失败或真实并发证据缺失。 | 待验收（仅规划） |

## 本轮规格自审与收敛

2026-11-10 按用户“警惕过度设计和结构债”要求，当前聊天对照原模型、存储、观察、恢复和确认代码审计并修改规格。以下为设计自审，不冒称独立实现审核；没有运行或改动业务代码。

| 问题 | 风险与证据 | 本轮处理 |
| --- | --- | --- |
| P1：把单任务执行建成完整 Roadmap 子记录 | 原 Record 含 Source/Analysis/Documents/Publication/Admission 等无关字段，SaveRoadmap 只有单记录 CAS；增加子用途、父子关联和整组初始化事务会扩大所有查询/停止/发布分支。 | 维持一个 purpose=documents 记录，内置窄 generations 数组。一次原子保存完整首轮，单 Run 逻辑在原包内复用，无新记录类型/表/执行框架。 |
| P2：确认前强制全仓调查和整个代码基线不变 | 原任务字段已覆盖范围/约束；原 documentContext 负责生成时基线准备。把完整调查前移会使轻量任务确认承担第二次设计，也因无关提交使规划失效。 | 仅补一段 coordination、每项 provides_tests 和必要测试材料；所有权/交接复用原字段。详细代码调查保留在每任务生成，基线仍在文档记录创建时固定一次。 |
| P2：重复状态及批量重生成机制 | 同时保存历史、当前轮次映射、进度/计数和多选目标集合，会出现一致性维护及幂等分支；用户要求只需单独重生成失败任务。 | 按任务取最后追加执行推导状态；一次命令一个 local_key，沿原 CAS/请求幂等保存，逐行按钮。保留必要未知/停止责任，不另设恢复状态机。 |
| P2：目标材料和验证范围外扩 | 目标代号是允许直接传入的少量事实，路径已由单项 schema 固定；额外 target 文件重复来源。另要求部署流程修改及六个真实模型会话，超出接线和规模各自需要的证据。 | 复用两份原 file 材料，短模板增加目标事实。只改原 Roadmap 资源枚举；删除默认 go.deployment，六任务五槽用受控链，两个真实 session 验证原生并发。 |

收敛后保留用户确认的核心行为：确认前完整任务信息、每任务独立 session、共享额度并行、失败任务单独重生成、程序汇总及整批审阅/发布。数据规模取舍为：沿现有文档记录保存执行历史并经 CAS 更新，适用于当前批次；本轮不预建分页仓储或事件存储。实施须按现有单项/业务总量限制验证实际数据，不能默默截断。

## 独立子 Agent 规格审计

2026-11-10 用户明确要求后，独立只读子 Agent `/root/audit_spec_plan` 对照当前源码审计 Spec、Plan 及配套安排，首轮报告两项 P2；当前聊天核实后直接修正，未启动产品/模型测试。

| 发现 | 核实依据 | 已修正内容及验收 |
| --- | --- | --- |
| P2：历史批次重生成缺少同来源互斥门槛 | `GenerateDocuments` 已检查同来源未结算请求及在途发布；初稿新重生成路径仅检查自身状态，可在更新批次存在时再次生成。 | 重生成仅对最新文档记录开放，复用生成锁及同来源资格，首次发布意图同步重核；不新增 latest 指针。RPG-A05 补历史重生成与新建/发布竞争反例。 |
| P2：执行项缺少 CreatedAt，重生成/重开会破坏队列公平性 | `ContentQueue.UpdateContent/AcquireContent` 用 created+key 排序；沿外层批次旧时间会使后追加执行插队。 | generation 保存不可变创建时间，重生成取新时间，首轮并列按既有 ID 排序，重建沿原值。RPG-A03 补先等待、后重生成及重开顺序反例。 |

独立复核修订结果：同日原审计子 Agent 读取修订后的 Spec/Plan，确认两项 P2 均已闭合；同来源检查已明确排除当前文档记录，不妨碍同批其它任务继续运行时重生成合格失败项。复核未发现修正引入新的矛盾或过度设计；没有增加 latest 指针、队列、锁类型或执行框架，锁范围仅限资格判定和意图保存。此次结论仅为规格/计划审计，不代表实现、产品测试或提示词变更独立审批已完成。

独立审计修订后的静态复验：链接检查 743 份 Markdown / 4635 个本地链接、0 错误；7 个正式测试 ID 登记且唯一，验收映射一致；UTF-8/末尾换行/行尾空白与 `git diff --check` 通过。

## 提示词维护与独立审核

实施后按[治理规则](../../../docs/prompt-governance.md)逐项登记解析/生成实际短正文、完整规则、默认静态资产、组装接线及对应测试的 Git 正文差异、UTF-8 字节前值/后值、业务理由、原请求输入断言和验证。当前未修改这些资产，字节数与审核均未发生，不预填“通过”。

独立审核须覆盖任务规划前移后的完整性、每次单任务输入、全组共享契约和文件来源，以及队列/重启/停止/重生成边界；黄金样本更新不代替批准。实际审核者、所审提交、发现/修复和复核结论在实施后登记。

## 证据位置和结果登记

后续 scope 为 `C:\SteerlineDebug\wt-4ef7-roadmap-per-task`，按[路径清单](../../paths.md)为每次调用建立独立叶目录。实施验收每次登记实际受测提交/dirty 状态、源清单版本、完整命令、选择 ID、模型与固定 Runner、summary/managed/原生/浏览器绝对路径、退出码及清理事实。失败与补验分开保留，不自动重试，不搬动原运行现场。

2026-11-10 文档静态检查：

- `python scripts/check_doc_links.py`：743 份 Markdown、4635 个本地导航链接，0 错误。
- `git diff --check` 通过；四份新文件另检查 UTF-8 可读、末尾换行及无行尾空白。
- 初稿同名测试安排的 8 个 ID 均登记且无重复；本轮自审删除范围外的 go.deployment，修订后的 7 个 ID 与验收覆盖在自审结束后重新核验。Spec 的 RPG-A01–A08 / RPG-M01 保留。
- `git rev-list --left-right --count HEAD...origin/main` 为 `0 0`，本次 rebase 无冲突。

以上仅验证本次规格、计划及导航；未运行产品测试、真实模型、浏览器或独立实现审核，未修改提示词、业务代码和安装现场。

本轮自审后的文档静态复验：链接检查 743 份 Markdown / 4635 个本地链接，0 错误；7 个正式测试 ID 登记且唯一，Plan 覆盖全部选择，RPG-A01–A08 / RPG-M01 映射一致；新文件 UTF-8、末尾换行和行尾空白检查通过。已核对 Spec/Plan 无 local_keys 多选载荷及 document-target.json 新路径；文档 `git diff --check` 通过。产品测试、真实模型和独立实现审核仍未执行。

## 实现中的验证归属收敛

2026-11-10：六任务完整链扩展 `TestRoadmapPublicationAndAdmissionWithRealGit/six-task-retry`，沿原真实 Git/SQLite/provider、活动和发布/入图 owner 验证五项同时在途、第六项等待、一次失败只追加一个执行及单 PR/整批入图；不在 E2E CLI peer 再实现第二套 Roadmap 业务结果构造器。`runtime.async-activities` 保留真实应用 HTTP 与活动原请求零副作用回归，逐任务页面另由专属生产构建浏览器覆盖。Plan 已同步此归属；验收要求未降低。

初次生成保留原 generationMu 覆盖只读 Git 材料准备至请求保存，以防原请求对账把仍在准备的命令判为未提交；不为缩短既有锁引入 preparing 状态、两阶段提交或额外在途索引。模型执行和远端发布在该锁外。重生成和首次发布意图仍串行重核并持久化资格。

## 固定提示词变更供独立审核

短模板按 prompts 的 UTF-8/LF、去单个终止换行规则计数（含事实占位符）；黄金样本使用 fixture 固定事实，给出实际短指令字节。完整材料按原 loader 返回的 UTF-8/LF 正文计数，包含末尾换行。完整正文差异见同目录 [提示词差异](roadmap-per-task-generation.prompts.diff)，采用零上下文 Git diff 保留全部变更行，包含短模板、完整材料、黄金样本、组装接线及断言变化；不是以样本更新代替审核。

| 正文来源 | 变更前字节 | 变更后字节 |
| --- | ---: | ---: |
| `internal/prompts/roadmap-parse.md` | 363 | 507 |
| `internal/prompts/roadmap-documents.md` | 546 | 592 |
| `internal/prompts/materials/roadmap-parse.md` | 1495 | 2060 |
| `internal/prompts/materials/roadmap-documents.md` | 2637 | 2762 |
| `internal/prompts/testdata/roadmap-parse.golden.md` | 356 | 500 |
| `internal/prompts/testdata/roadmap-documents.golden.md` | 539 | 579 |

业务原因：解析新增必要协作安排与测试提供关系，并读取项目入口声明材料，使用户确认时已看到完整任务信息；文档生成由整批改为短指令指定一个内部代号，固定该项 provides_tests，保留共享规划/代码快照、调查、严格 JSON 和大小限制。组装函数只新增 local_key 事实，所有行为规则仍在原 prompts 文件，无 inline 兜底。

静态初始化仍从原 `RoadmapParseRules`/`RoadmapDocumentRules` 读取上述完整材料，缺失或不同源阻断；默认 SpecGuide 无行为冲突，本次不改。仓库工作树的 parse-rules.md/document-rules.md 为本次显式准备的同正文资产，字节与对应完整材料一致。CI job 和源码守卫范围不变，黄金样本与资产摘要仅同步实际新正文。验证包括 prompts owner、准备/固定材料/单项目标断言、完整 L1 与独立实现/提示词审核；审核结果在完成后登记。

实现自查发现 `application_verify_stop.go` 仍从文档记录根部枚举单 Run，须改为原 `Record.Executions()`；否则新批次的原停止资源证据会缺项。已说明并同步 Plan/测试安排，新增 `go.deployment`，不把最初“七集”静态选择当成最终范围。原部署夹具补两项执行：零证据拒绝、只有一项证据拒绝、全部原 Run/input 停止证明齐备才通过；没有新增部署协议或兼容路径。

## 实现验证结果（持续更新）

代码基线 `1f64b0ffeb936358786cfe4734f2096765bc4008`，分支 `codex/roadmap-per-task-generation`，工作树未提交。输出 scope 为 `C:\SteerlineDebug\wt-4ef7-roadmap-per-task`；所有正式报告均检查 passed/complete、实际非零及无 skip。诊断报告不冒称全集。

| 范围 | 结果 | scope 下的证据目录 |
| --- | --- | --- |
| L1 / l1.minimum | passed，complete；最终复跑约24秒 | `formal/l1-final-20261110-1149/summary.json`（原轮次亦保留） |
| L2 / smoke.delivery | passed，cleanup_complete；约122秒 | `formal/l2-20261110-1001/summary.json` |
| go.roadmap | passed，三组完整；含六任务整链及已修正提供关系夹具 | `formal/roadmap-20261110-1022/summary.json` |
| go.storage | passed，complete；审计修正后复跑通过 | `formal/storage-audit-fixed-20261110-1221/summary.json`（原轮次亦保留） |
| go.http | passed，complete；审计修正后复跑通过 | `formal/http-audit-fixed-20261110-1217/summary.json`（原轮次亦保留） |
| web.unit | passed，complete；审计修正后复跑通过 | `formal/web-audit-fixed-20261110-1225/summary.json`（原轮次亦保留） |
| 六任务真实 Git/SQLite/provider 整链诊断 | 2/2 测试通过；五槽并行、仅一项重生成、完整审阅、单 PR/整批入图 | `diagnostics/six-task-chain-20261110-0938/summary.json` |
| 补充不变量诊断 | 四包 55/55 通过，无 skip；单项 schema/共享固定输入、确认材料漂移、存储重开/不可变/CAS、活动结算和原回执恢复 | `diagnostics/late-assertions-20261110-1029/summary.json` |
| 生产浏览器专项 | 1/1 通过；1440/768/320px、混合状态/历史、复制 generation/Run/session、双击只重生成一个任务、整批停止；GET 零生成写入 | `browser/generation-final-adopted-20261110-1154` 的截图、trace.zip、proof.json |
| go.deployment | passed，complete；含逐执行停止证明与原部署回归，App 96 项无失败 | `formal/deployment-20261110-1043/summary.json` |
| runtime.async-activities | passed，cleanup_complete；真实 HTTP/Edge 活动、原请求观察零生成副作用与资源清理通过 | `formal/activities-20261110-1139/summary.json` |
| 两任务真实模型专项 | passed，完整生成、原生并行、单 PR/整批入图及资源结算通过；仅人工算术项目 | `native/report-synthetic-native-20261110-1128/summary.json` 与 `native/two-task-synthetic-native-20261110-1128` |
| 独立实现/提示词审计 | 完成；1项P2已修正并独立复核关闭，本地提示词审核通过 | 下文独立审计记录及 `diagnostics/audit-projection-20261110-1208/summary.json` |

失败现场保留：最初 owner 诊断暴露旧断言按一个 Run/一份静态材料计数，已随新契约修正；最初 go.roadmap 的 planned-tests 夹具在文档阶段才新增 provides_tests，与前移确认不符，修正为解析前声明后完整专项通过。新回执恢复测试的原请求空 Input 经 JSON 存储成为 null，已修正夹具为合法空对象。浏览器最初两次失败分别为旧按钮文案及停止命令原 expected_version=1 的断言，修正后通过；没有放宽生产行为。短超时发布诊断保留超时证据，完整正式入口采用原 25m/p1/parallel1 并通过。

停止竞态补充：共享额度已准入、保存发送意图之前插入持久化 Cancelled，适配合并不得把仍 pending 的项变为“已开始但从未发送”的未知请求；派发重核返回的取消事实。显式同步夹具验证零 Runner 提交、没有虚构原请求义务且整批 settled，诊断 `diagnostics/stop-race-20261110-1051` 通过。保存回执报错时若能读回原请求绑定，文档创建/重生成命令按已保存意图回执 applied，模型完成沿活动独立观察。

## 真实本机并行结果

风险复核允许后，两个环境预检因 WindowsApps 路径 Go EvalSymlinks 返回 Access is denied 结束，没有生成模型结果，原 Runner 均核实停止；没有对失败模型结果自动重跑。只读探针 `native/probe-native.go` 定位后，选择本机现有 npm 原生 Codex 0.166.0，通过版本/路径/PE 检查。没有复制或更新安装内容。

实际模型执行固定 Runner `C:\Project\SteerRunner\bin\steerrunner.exe`，`0.2.0-dev.4` / commit `dce4dff8e5be8529a2a1a1d6a300f8cfd01087c1` / SHA256 `8839e70fe60d3852048d12aad92d6bee1427e13db8a3c3b9fd585be42d636596`（以 runner-build.json 原值复核），Codex 为 `.local/bin/node_modules/@openai/codex/node_modules/@openai/codex-win32-x64/vendor/x86_64-pc-windows-msvc/bin/codex.exe`，模型 `gpt-6.1-sol` / low / read_only。入口强制不读取真实项目来源，只在空临时 Git 仓库建立算术样例；provider/remote 留在本机临时夹具。

`concurrent-native-sessions.json` 在 `2026-11-10T09:23:16.9481847Z` 观测两个不同原生 session 均已 turn_started 且 Run running：T1 `01a1c4c4-d091-74f1-a727-8b8212555cdf` / Run `bfa7ed60cc372b0e4f397eff70b94efa`；T2 `01a1c4c4-d075-72d2-8e01-04ceb1f609b1` / Run `93708c5824d883f23ecbaa18abeee7ae`。公开事件分别在 `09:24:27.7624793Z`、`09:24:18.2467376Z` 提交结果，两份完整正文通过来源/JSON/业务校验，全部 completed/stopped。原请求、同快照、事件分页、采用结果、入图及 `runner-settlement.json` 齐备，专项约223秒通过。此项证明两任务人工样例的真实并行与完整结果，不泛化为任意复杂 roadmap 的模型质量保证。

最终 owner 定向收口 `diagnostics/final-owners-20261110-1117/summary.json` 通过（约883秒，保留实际 package/test 终态；不冒称 go.full）。覆盖后续身份从投影派生、取消竞态、原请求恢复及 App Roadmap 真实发布链。完整部分成功预览和发布中仍显示已审阅正文的前端断言在最终定向 Vitest 中验证（44/44 通过），专属生产浏览器补测目录 `browser/generation-final-20261110-1136`，1/1 通过。

独立只读实现审计 `/root/audit_implementation` 已在上述测试通过后启动，覆盖代码、Spec/Plan、提示词全文/字节数/接线/资产和实际证据；结论待登记，不预填批准。主聊天保持实现源码稳定，审计期间仅复跑收口测试与更新验收。

最后一处 UI 收口：正式 Documents 已采用后，不随 publishing/admitted 状态变化重新显示原始生成项正文，以保持审阅编辑为展示权威；历史原始结果仍在存储保留。定向前端 44/44、生产构建和 `browser/generation-final-adopted-20261110-1154` 专项 1/1 通过。最终静态检查为745份 Markdown、4636个本地导航链接，0错误，`git diff --check` 通过。L2 最终复跑输出 `formal/l2-final-20261110-1156`，已通过，cleanup_complete。

## 独立实现审计与修正

独立只读子 agent `/root/audit_implementation` 审阅实现、相关消费者、Spec/Plan、完整提示词差异/字节数/静态副本及实际正式/真实模型证据，确认1项P2：新 generations 数组的活动聚合未读取原生状态与 resume_required，把已受理项全当 running；单项页面也遗漏排队、受阻和等待状态。

当前聊天已修正：SQL 最小投影补原生 resume_required，查询按各 local_key 最后执行的原生事实推导状态与注意事项，保留 n/N；公共单项 DTO 从原 Run 派生 flag/诊断，页面区分排队、运行、输入/外部等待、收尾及恢复待核实。没有新增持久状态、恢复框架或模型调用。新增数组反例覆盖旧轮次隔离、queued、blocked、waiting_input、waiting_external、completed 未 stopped、running+resume_required；SQLite 验证 flag 穿透真实 JSON 及请求隐私，App 验证公共 DTO。定向 Go 诊断 `diagnostics/audit-projection-20261110-1208` 通过，相关前端45/45通过。

同一独立 agent 随后只读复核明确“原P2已关闭，未发现补丁引入新的可确认问题”；首轮其余生命周期、CAS/幂等、资格、采用/编辑、发布/入图路径未确认新缺陷，也未提出新增框架/状态层的必要性。独立核算六组提示词字节与验收表一致，两份静态副本逐字一致，本地提示词审核通过；后续补丁未改提示词。此结论不代表远端保护或 PR 平台审批。生产浏览器及受影响正式集合补测在实际完成后登记。

审计修正后的生产构建通过；`browser/generation-audit-fixed-20261110-1215` 1/1通过，补 accepted 原生排队/受阻/等待输入/外部等待/完成收尾/恢复标记的 GET 刷新验证，写入数仍仅显式单项重生成和停止两次。独立复核已确认P2关闭且无新确认问题。L2最终复跑 `formal/l2-final-20261110-1156` passed/cleanup_complete。受影响正式集合补测按实际结果继续登记。

审计修正的 Go 定向报告为40/40通过、完整且无 skip；HTTP正式全集 `formal/http-audit-fixed-20261110-1217` 已 passed/complete。该修正仅影响现有状态读投影和展示，不改变原模型请求、并行、资源生命周期或发布/入图；真实模型结果仍与相同生成接线对应。

审计修正后的存储正式全集 `formal/storage-audit-fixed-20261110-1221` passed/complete；最终前端正式集合 `formal/web-audit-fixed-20261110-1225` 已 passed/complete。其余正式集合和真实专项均已通过；人工体验 RPG-M01 保持待验，不影响已完成自动验证，不冒称部署或远端审批。

## 收口结论

八个选择的正式测试集合均已通过；审计修正后 HTTP、存储、前端全集以及40项定向反例、45项相关前端测试和生产浏览器专项再次通过。独立实现审计确认的问题已修正并复核关闭，本地提示词审核通过。真实两原生 session 同时运行、两组完整文档、单 PR/整批入图及停止结算已有实际证据。人工体验 RPG-M01 后置待验。

实现与验证阶段仅交付当前工作树的实现、Spec/Plan/治理和验证记录。用户随后已授权提交、推送并创建 PR，后续交付状态以 Git/PR 实际记录为准；未授权合并或部署。测试中的 PR/provider/Git均为隔离本机夹具，原安装现场未改动。

提交前同步：当前实现无冲突 rebase 到 `origin/main` 的 `50b8fdb5c24fa2a3f6dd32fe10e8799d271061f2`。按最新测试指南复用上述正式证据，仅验证整合影响：`diagnostics/pr-integration-20261110-1810/summary.json` 为112项定向测试 passed/complete，覆盖提示词治理、Roadmap 及 Runner 选择；`diagnostics/pr-runner-pin-20261110-1814/summary.json` 的新增发布哈希固定反例通过。生产构建通过，文档链接检查748份 Markdown / 4652个本地导航链接、0错误；本次整合未改业务实现或提示词正文。

## PR494 提示词精简

用户要求保留必要职责、删除重复约束和审计式提醒。本轮以前一提交 `67a336905c8c23fd0fd3628ca4e5c838dd391bb3` 为基线，只改固定短模板、完整规则、两份静态副本及对应黄金样本/摘要；组装函数、业务接线、schema、CI 和源码守卫不变。[本轮实际完整差异](roadmap-per-task-generation.prompt-slimming.diff)采用零上下文 Git diff，包含全部正文、资产及测试期望变化；此前正文差异和字节表保留为历史。

短指令保留固定文件入口与目标事实，字段职责只在完整规则定义；解析保留必要协作、共享修改所有权/交接以及新增测试提供关系，删除重复的阶段禁令、问题出口及“额外契约表/全仓调查”审计式提醒。文档生成保留单目标、其它任务仅作上下文及沿用已确认 provides_tests，合并重复禁止句，tests 改为当前单项表述。空数组、唯一提供者及测试选择约束仍有明确规则和原程序校验；没有以程序校验替代必要模型职责。

字节统计沿用上表口径：短模板去一个末尾换行，黄金样本为实际渲染正文，完整材料为 LF 正文且含末尾换行。

| 正文来源 | 本轮前字节 | 本轮后字节 |
| --- | ---: | ---: |
| `internal/prompts/roadmap-parse.md` | 507 | 463 |
| `internal/prompts/roadmap-documents.md` | 592 | 561 |
| `internal/prompts/materials/roadmap-parse.md` | 2060 | 1787 |
| `internal/prompts/materials/roadmap-documents.md` | 2762 | 2626 |
| `internal/prompts/testdata/roadmap-parse.golden.md` | 500 | 456 |
| `internal/prompts/testdata/roadmap-documents.golden.md` | 579 | 548 |

按实际短指令加完整规则计：解析 2560→2243（减少317字节），生成 3341→3174（减少167字节）。解析仍较改造前1851字节增加392字节，用于前移确认必要的协作与测试规划；生成已与改造前3176字节基本持平。该统计不是 token 或整次模型输入量，两个阶段及静态副本不叠加到同一次请求。

验证安排：按提示词治理运行 prompts/roadmap owner、完整 L1/L2，再做独立只读子 agent 审核。范围未变的 UI、存储/HTTP/部署、浏览器和真实原生并行链复用此前本条验收中各原受测版本的正式证据；本轮没有业务代码、请求结构或执行生命周期改动，不重跑模型。当前 prompts/roadmap owner `diagnostics/prompt-slim-owner-20261110-1830/summary.json` 已通过；L1/L2 与独立审核以实际完成结果补记，不预填批准。

本轮实际受测版本为 `67a33690` 加上述未提交精简差异，原报告保留 dirty 状态。prompts/roadmap owner 为112项 passed/complete、零失败/零测试跳过；完整 L1 `formal/prompt-slim-l1-20261110-1830/summary.json` 和完整 L2 `formal/prompt-slim-l2-20261110-1830/summary.json` 均已通过。链接检查748份 Markdown / 4653个本地导航链接、0错误，`git diff --check` 通过。测试完成后独立只读子 agent `/root/audit_prompt_slimming` 开始审核，本地批准结论待实际返回。

独立审核实际结论：`/root/audit_prompt_slimming` 只读审阅本轮全部差异、原正文、实际请求组装、schema/服务端验证及上述原报告，未发现可确认问题，本地提示词审核通过。必要职责保留，删减属于重复说明或审计式提醒；两份静态副本逐字节一致，黄金样本与渲染正文一致，摘要及六组字节数匹配，专属差异文件与实际 Git diff 完全一致。独立核实 L2 的 `TestDeliverySmoke` 实际非零执行、组内 passed/complete 且 `cleanup_complete=true`。没有新增规则、测试或框架；此结论不代表 GitHub 平台审批。

## PR494 合并冲突处理

用户要求解决冲突后，将最新 `origin/main`（`9334bef8`，含 PR495/496）合入当前分支，保留已有提交历史。唯一文本冲突是 Spec 索引首部的两条新增记录，完整保留 Roadmap 按任务生成和活动阅读页两项入口。自动合并的 Roadmap 存储同时保留执行历史校验与阅读索引写入，活动最小投影保留 generations 原生状态/恢复标记及 main 新增轻量阅读信息；提示词正文、组装、黄金样本与本地独立审核范围均未改变。

本轮按整合影响补测，不重跑无关完整集合：`diagnostics/merge-main-9334bef8/summary.json` 对 SQLite、产品查询及 Roadmap 的活动、阅读、Roadmap 和 schema 定向用例通过；前端 Roadmap、活动列表和活动阅读页三文件66项通过，生产构建通过。文档链接检查752份 Markdown / 4691个本地导航链接、0错误。原始受测版本为 `d4614273` 加合并中差异；前述 L1/L2、提示词及真实模型证据保留原版本和范围。
