# 主 Agent 提示词建议模块：实施 Plan

状态：按用户确认的阻断草稿阶段实施中。只执行[对应 Spec](../spec/main-agent-advisory.spec.md)规定的 advisory 能力，不继续 PR475 的答案/材料发布方向。实际结果唯一登记于[专属验收](../testing/acceptance/main-agent-advisory.md)。用户已授权实施与提 Draft PR，未授权合并或部署。

阶段调整：原生能力前置未通过后，用户明确选择“保留严格边界，先交付模块重构草稿 PR，生成保持阻断”。因此先完成本 Plan 的核心、宿主记录、活动及只读界面结构，删除旧执行出口；不实现或发布任何正向 Runner capability、不增加生产模型提交路径。第 2 节与第 3 节第 6 项及真实端到端验收保持未完成。来源适配的明确原因码仅做受控接线，实际生产方可用性未验；准备失败时部分材料展示也未完成。这是经授权的阶段交付，不替代完整 Spec 的完成条件。

本 Plan 的主线是**先完成模块边界，再做功能最小接入**。结构完成条件是：核心只依赖自身契约和固定提示词组装；宿主独占来源读取/推理生命周期；业务 owner 独占原问题与执行；Web/活动只消费公开投影。功能完成条件只包括一个明确来源自动触发、异步生成一段提示词、现有详情查看/复制与活动导航。它们用于证明模块可以协作，不扩展成主 Agent 工作台。

本轮不实现 Web 草稿编辑/恢复、补充后重新生成、材料编辑选择、多轮讨论、专用重试、历史工作台或通用来源/动作框架。优先复用既有记录、队列、停止和活动读接口；必要的新桥接只服务本模块，不趁机重构全项目生成基础设施。

## 1. 基线、依赖与先后关系

SteerLine 调查基线为 `e9ee368f753322145457f1730b2309a68d566f83`。已读当前主 Agent core/model/schema、app 上下文/命令/发布/恢复、SQLite、前端、模型设置和提示词治理；复用和删除安排见 Spec 第 2 节。实施前核对实际 PR475/main 状态与新基线，按最终边界重做原 PR 或另提替代 PR，不以合并旧执行能力作为前置。

另已调查活动中心现有 `AsyncActivityReader`、静态来源集合（已含 Roadmap）、纯投影和 Web details_ref 导航。复用本地只读池、每来源预算、分页/计数与 request links；不按产品文档旧“四来源”文字假定当前集合。本次给主 Agent 接入一个新来源，不重建活动框架。

Runner 调查来源为本机独立仓库 `C:/Project/SteerRunner`，提交 `9243a79b847415b29480d6e418dfc76d01307fcd` 的公开 `docs/workflows/generate.md`（该文件无工作树修改）：generate/1 允许原生只读工具、文件材料要求模型读取。此结论是能力缺口，不是已实现无工具的声明。实际实施须记录所用 Runner commit、EXE SHA、资源摘要和公开 capability/schema；不将本机任意源码视为已发布 EXE 的能力。

顺序：先验证本地 Codex 原生链能否强制零工具且排除项目/个人指令隐式注入 → 在独立 Runner worktree 补齐并发布最小公开能力 → SteerLine 核心/宿主/界面收敛 → 受控与真实验收。原生链无法满足时，记录具体障碍并保持未完成；不改用 read_only、不直连其他模型、不扩大当前边界。

## 2. Runner 能力前置

Runner 修改必须在其独立 worktree 进行，遵循该仓库 AGENTS、Spec/Plan 和测试规范，不在 `C:/Project/SteerRunner` 日常 checkout 或本 SteerLine worktree 修改 Runner 源码。

1. 在 Runner worktree 固定配套 Spec/Plan。目标是一次纯文本生成：明确声明 provided-context-only、tools-disabled；输入承载完整规则及材料正文，输出完整 text；不接仓库、目录/file 读取提示或持久会话记忆。
2. 核验固定本地 Codex 原生版本与实际 app-server 初始化/turn 接口。验证关闭所有原生工具面及 MCP/Apps/hooks/子 Agent、禁用应用侧项目与个人指令自动附加的真实机制；原生无法保证时不得发布正向 capability。保留正常认证及模型推理必需的基础设施，不把认证读取误记为模型获得文件工具。
3. 使用 Runner 原有内容生成入口提供一个明确的限制模式；具体字段及 capability 名称在配套 Runner 契约一次固定，连同配置/请求 schema、原始输入摘要及结果来源公布。现有 generate/1 的其他消费者不改变语义，SteerLine 此用途强制选择新模式且禁止回退。
4. 原生工具请求在调用前被执行层禁止；不以允许调用后再检查工具事件取代。结果公开资料应能核验实际模式、来源与资源结算；需要工具策略字段时仅补该窄事实，不开放通用工具白名单配置。
5. 正/反例验证通过后生成独立 EXE 与 capability 证据，交给 SteerLine 适配。旧 EXE 或不支持当前原生组合明确拒绝，零业务轮次。

当前 Spec 不假设上述开关已存在。不为等待该能力保留旧主 Agent 有工具路径；原 Runner 资源仍由原版本结算，不迁移或接管到新构建。

## 3. SteerLine 核心与宿主的整合

1. **固定业务契约。** 新建 `internal/mainagent/contract`，实现 Spec 的 request/context/material/suggestion DTO、advisory-v1 身份及大小/UTF-8/唯一材料 ID 校验。所有者只依赖标准库。删除现有 Candidate/Question/Final、Answer/Changes/Publication 等作为模块公共协议的类型；不设计 action bus 或通用场景注册器。
2. **核心只准备明确输入和接收文本。** 整理 `internal/mainagent`，由核心校验请求、通过 prompts 包组装固定规则与给定事实、校验返回的唯一 prompt。异步宿主可分别调用准备和结果采用函数，不把它们展示成新的用户流程。核心不持有 Runner Client、Catalog、BuildPin、Store、Git、来源查询回调或业务命令能力。
3. **复用原生成可靠性，集中在宿主适配层。** 将 `service.go` 中必要的固定请求、排队、提交、观察、来源核验、取消和结算放到 `internal/mainagentbridge`；此包只对接核心契约、现有 runnerclient/runnerbinding、窄存储和共享 generation 配额，不导入 app。app 只装配及提供已冻结的上下文。该适配的必要性是把现有真实 Runner 协议隔离于纯建议核心，不增加第二条模型链。
4. **状态只有一个 owner。** bridge 管理推理作业状态及其原 Runner 关联，SQLite 写路径只提供 payload 原子存取、唯一性和 CAS，不导入 bridge 运行包；只读路径依据明确的持久字段提供 summary/活动事实。app 启动、排空、构建核验调用窄的生成生命周期入口，不再直接改分析 Record。完整 Runner 投影仅在 bridge 私有记录中，产品查询返回显式 DTO。共享 schema 按实际新结构更新，不迁移 schema 42 数据。
5. **宿主显式提供上下文。** app 候选入口将 Spec/Plan、公共/验收引用、原问题、verified final 和必要测试/输入事实转为带来源的正文材料。只使用用户可见的明确选择，不自动扩展引用图或代码调查。核心不判断候选控制资格；外部入口负责读取/展示权限。取消和失败不触碰原任务状态。
6. **接入新 Runner 模式。** bridge 核验固定 EXE 的正向 capability/schema，传入完整文本、模型/强度与无工具模式；不提供 repository、项目 directory 或供模型自行打开的材料路径。核验冻结输入摘要、原请求/Run/final 来源及停止结算。能力缺失或正文超限时零提交。

异步记录、模型配置和资源停止只为一次推理服务；不建设主 Agent 内部讨论 session、长期记忆或多 Agent 编排。后续请求要使用旧输出时，由外部明确将正文加入新上下文，不递归自动注入 Prior。

### 3.1 上游状态到异步工作的接线

在来源 owner 保存明确材料问题状态后，提供持久 AdviceNeeded（仅要求不明确/冲突）。固定 source/occurrence/revision 和上下文事实来源；普通 native Question 无明确业务类型时继续原问答路径，不用文案解析或模型分类凑触发。实施时补齐实际生产方与消费接线，在集成测试中从业务状态保存开始跑通，不能只单测手工构造的信号。

首批只接一个候选材料问题适配器。AdviceNeeded 是读取原业务记录得到的窄值，不单独建立通用事项表、事件注册中心或来源插件；若原记录缺少明确原因，只补本来源必要字段及去重关联。

自动输入的 goal 取原事项明确的问题正文；额外固定的建议生成要求放在 prompts 文件，由同包组装，不在自动触发业务代码拼接行为规则。上游无法提供有效问题正文时记录缺失并失败，不让模型自由调查补齐。

宿主在独立于 Web 的后台预算中消费持久事实，先用 Spec 唯一键原子创建 queued 记录和 request 关联，再异步读取限定材料、冻结完整输入、排队及提交。来源与生成之间的崩溃窗口通过可重读的源事实和唯一约束收敛，不新建通用消息总线或双写活动表；若来源未持久化，不得依赖仅进程内通知保证恢复。

对同事项的重复事件、心跳和普通版本更新只返回原关联，显式新生成另有请求身份。未尝试提交的 queued 重启后允许继续首次提交；started/unknown 只核实原请求，失败/取消不自动重开。来源解除时按是否提交决定取消排队或停止原资源；已完成产物保留。读取来源的观察异常不伪装成事项解除。初次上下文准备错误、能力缺失及超限均保留正式记录和诊断。

自动状态接入不要求候选可写或原问题 can_reply；来源 owner 确定业务处理需要，宿主核验项目作用域、状态仍有效及材料可读。建议成功、用户查看/编辑或复制都不写回源问题状态。

### 3.2 注册活动来源

在 `internal/product/query/async_activities.go` 静态注册 `main_agent_advisory`，沿 `storage/ports.AsyncActivityReader`、SQLite 只读来源与产品纯投影接入。kind 使用 `prompt_suggestion`，稳定 activity ID 由项目/来源/原生成 ID 编码。SQL 对筛选只粗筛超集，原 `MatchesAsyncActivity` 最终解释；补齐读池预算、窗口载荷、分页/计数和 request links 的真实 SQLite 组合用例。

读取本次生成的最小持久事实：源事项身份/当前关联、原请求、上下文阶段、模型提交/观察事实、结果、资源结算、业务完成/最终完成及观察时间。自动生成无需浏览器 journal，沿既有 request links 精确关联，不另建本用途的浏览器提交占位。纯投影不导入 bridge/service，不调用它的 Observe/Submit/Cancel，不构造独立活动状态机。

映射 queued/running/waiting/succeeded/failed/cancelled，未知和结果已出但未结算均 waiting；只有 prompt 已核验且资源结算才 succeeded。来源仍需处理时，已生成建议或失败可以 needs_attention；来源解除后历史不重复提醒，不把完成建议当成问题已解决。GET 错误只降级该来源及观察质量，不改变生成结果或释放资源。

Web activities/model 增加 kind 和 `details_ref` 的确切候选/建议定位，复用现有卡片样式。导航只读定位已有 ID，抽屉不提供取消、重新生成或执行按钮，不自动展开或抢焦点。更新活动中心设计/接入说明，明确这是新增来源而非修改旧来源语义，不修改其他来源交互。

## 4. 删除专用执行能力与收敛 UI

- 删除 `main_agent_answer`、`main_agent_publish`、`main_agent_prepare` 的命令/路由接线和回执分支；删除 `main_agent_publication.go`、`main_agent_files.go` 及其专用 provider 依赖，旧请求不兼容处理。候选原 answer_decision、Roadmap 发布等原功能仍按各自协议运行。
- 主 Agent 不再要求项目复制 `SteerlineFlow/main-agent-rules.md`，删除其专用准备函数和 readiness 门禁。平台唯一规则源保留在 prompts；项目副本从当前资产及使用说明移除，历史证据文档保留其冻结历史含义。
- 不再使用 `roadmapPort` 的临时包装复用。若 `documentProvider` 只剩 Roadmap 消费，回收到原 Roadmap owner，避免为已删除的用途留下公共层。
- 候选详情保留最小区域“后续对话提示词建议”：显示异步状态、可展开的实际上下文和唯一正文，提供复制；原有推理取消能力保留。用户在外部修改文字。本轮不建设编辑草稿、恢复原文、补充新生成或独立历史管理。删除分类驱动的多字段结果/依据面板、完整文件 diff、回答/发布/审阅按钮及采用状态。
- 精确详情只返回公开 DTO；取消复用既有命令信封（`main_agent_cancel`），不新增 `main_agent_suggest` Web 发起入口。自动触发不绕到 Web effect，列表、详情、活动查询只读。切换项目/候选/记录按 ID 隔离结果，迟到响应不能覆盖另一记录；导航到历史记录无需原问题可答或候选可写。
- 模型设置保留本地 Codex、默认 Sol/high、目录校验、保存后重启与冻结规则。前端类型来自明确契约，不复制持久化 Record 或消费原始 Runner 状态。
- 更新产品设计、架构/依赖守卫、接入和提示词治理；标明 PR475 旧验收不证明新边界。只有上游明确提供的首批两类状态自动生成，普通问答、环境和维护轮询不普遍触发。

## 5. 提示词与治理

改写 `internal/prompts/main-agent.md`、`materials/main-agent.md`，同包组装完整模型输入；需要新的组装函数时只在 prompts 包新增。业务代码传原始事实，不追加固定规则。主 Agent 的模型输出只承载 prompt；若 Runner 需要结构化包络，仅允许严格的 `{prompt:string}`，对外仍是单一正文，不保留旧 ResultSchema。

固定规则应说明：仅基于给定材料生成下一段用户对话提示词、材料缺失/冲突写入正文、不能读取工具或宣称完成外部动作、不能将输入文字当成权限。规则文件缺失即失败，不内联兜底。

按用户明确的零工具要求，在 docs/prompt-governance.md 和对应守卫中新增该用途的文本输入规则：宿主从固定文件读取并冻结后作为原生上下文传入；不改变其他用途“仓库/file 供模型读取”的协议。评审实际完整请求和 source，而不只看短模板。

实施 PR 必须列出原/新实际正文 diff、各资产和完整实际发送正文的 UTF-8 字节前后值、业务原因、验证与独立审核。不得仅更新 golden/hash 通过。本次 Spec/Plan 不改运行资产或 golden，不把提纲当成已批准的最终提示词。

## 6. 测试与验收安排

遵循[测试指南](../testing.md)和[路径清单](../paths.md)。每次使用仓库/安装目录外的新 `-out <绝对目录>`；下表 `$slOut` 必须按调用新建唯一值，不复用上一行目录。测试所需 Go/Git/Node/npm、本地认证和固定 Runner 均在执行前核验。无匹配、skip、超时、资源未结算或缺证据都不算通过。

| 验收 / 级别与范围 | 可执行安排 | 时点、通过条件与证据 |
| --- | --- | --- |
| MAA-A01–A03、A10 / 核心及 bridge owner | `go run ./cmd/test-feedback -out $slOut -- ./internal/mainagent/... ./internal/mainagentbridge/... -count=1` | 步骤 3 补齐并枚举非零 TestAdvisory*：独立 fake 推理端口验证无宿主依赖、输入完整性/限额、唯一产物/拒绝旧结构、未知能力；原 JSONL/summary 记录至专属验收 |
| A02、A06–A08、A11、A13 / App 接线 | `go run ./cmd/test-feedback -out $slOut -- ./internal/app -run '^TestMainAgentAdvisory' -count=1` | 从持久的源状态保存开始自动生成；上下文选择、旧动作拒绝零业务副作用、重复/乱序/崩溃窗口去重、queued 首次提交恢复、unknown 不重投、取消不重开、无浏览器及源解除、配额/启动排空；保留原问答/Roadmap 断言 |
| A01、A09、A10 / 边界与固定正文 | `go run ./cmd/test-feedback -out $slOut -- ./internal/testkit/boundary ./internal/prompts -count=1` | 负例证明 core/contract 无 Runner、app、storage、进程/文件/网络依赖；固定正文、字节、缺资产阻断和新模式接线测试；正文差异经独立审核 |
| A01、A06–A08 / 完整 HTTP、存储、部署 | 分别执行 `go run ./cmd/test-feedback -level L4 -suite go -scenario go.http -out $slOut`；将 scenario 分别换为 `go.storage`、`go.deployment` | 接线与 schema/lifecycle 修改后逐项执行，登记完整定义和通过结果；压力约束沿 testing.md，不能用定向诊断替代全集 |
| A06 / 原发布回归 | `go run ./cmd/test-feedback -level L4 -suite go -scenario go.roadmap -out $slOut` | 删除共享 provider 后完整核验 Roadmap；不修改原发布/合并断言换取通过 |
| A11–A13 / 活动纯映射、SQLite、App | `go run ./cmd/test-feedback -out $slOut -- ./internal/product/query ./internal/storage/sqlite ./internal/app -run 'Test(MainAgentAdvisory\|AsyncActivity)' -count=1` | 步骤 3.2 新增并枚举 TestAsyncActivityMainAgent*：活动准备阶段可发现、正常/失败未结算/未知、attention 与源解除、无 Runner/Git/写副作用、部分故障/预算、分页/计数/request links、跨项目/实例。定向诊断之外仍执行完整 go.http/go.storage |
| A03、A06、A08、A12、A13 / Web 全集与构建 | `go run ./cmd/test-feedback -level L4 -suite web -scenario web.unit -out $slOut`；`npm --prefix web run build` | 自动出现已有生成、只读正文/复制、无编辑/新生成/执行按钮、精确活动导航与跨范围结果隔离、关闭抽屉/切页不取消生成；保留 settings/项目切换原断言；完整 vitest 报告和构建日志 |
| A01–A13 / 固定基础回归 | 分别执行 `go run ./cmd/test-feedback -level L1 -out $slOut`、`go run ./cmd/test-feedback -level L2 -out $slOut` | 集成候选完成后，固定选择完整通过；L1/L2 不替代真实零工具证据 |
| A04、A05、A07 / 固定 EXE、本地 Codex 定向真实验收 | 沿既有 runneracceptance 测试入口补 `TestAdvisoryContextOnlyNative`：`go run ./cmd/test-feedback -out $slOut -- -tags=runneracceptance ./internal/mainagentbridge -run '^TestAdvisoryContextOnlyNative$' -count=1 -timeout=6m` | 实施时替换旧真实只读分析测试、枚举非零且不得 skip；冻结 Runner/native/模型，验证零工具、隐式上下文排除及结算。明确登记为定向真实证据，不冒称新 L4 全集 |
| A08、A12 / 人工体验 | 隔离调试包接收待处理状态后，不开详情也能在活动中心看到生成；导航精确建议、查看输入并复制，用户在外部修改文字 | 自动边界通过后安排；体验窗口仅阻塞文案/复制/可用性验收，不代替权限证明；不投递真实原问题或创建 PR |

不默认选择 go.full、全部 live、所有 E2E 或计费/远端专项。本次主链是本地给定上下文；增加专项仅基于实际影响。Runner 的原生能力/隔离测试由配套 Runner Spec/Plan 规定命令，证据同时关联此处 A04/A05，不把 SteerLine fixture 当成原生证明。

## 7. 最小真实驱动与证据

复用既有 `runneracceptance` 和 `cmd/test-feedback` 的定向诊断入口，替换旧主 Agent 的真实只读分析测试，补齐本次零工具与上下文边界反例；不为此新建测试 suite、调度器或 E2E 框架。测试必须显式固定 EXE/原生配置/证据路径，缺前置失败而不 skip。不把真实模型加入默认 L1/L2，不将定向证据登记为不存在的 L4 测试集通过。

驱动使用全新 home/临时仓库/Runner namespace 和固定 EXE；至少有一次正常建议，以及诱导读仓库外文件、URL/shell/MCP/子 Agent 的反例。外部 canary 文件和隐式项目/个人指令均不放入请求；证据同时包含生效的禁止工具配置、原始已发送上下文及原生限制证明。模型没读到 canary 或事件为空只能辅助核验，不能代替调用前关闭工具的证明。不能通过工具请求的反例应按原生协议确定拒绝或只返回提示词，不允许真实调用。

取消、正常结束和失败均核验原 Run/原生资源/服务停止，保留未核实现场。不为了采证读取用户正式登录文件或 Runner 私有数据库；原生日志/初始化参数的验证在 Runner 自有测试现场由其驱动负责，SteerLine 消费公开契约和配套证据。

## 8. 收口

验收记录所有测试的真实 SHA/dirty、命令、版本、结果和报告目录；静态审核、受控验证、真实权限验证和人工体验分别登记。执行 `python scripts/check_doc_links.py`、`git diff --check`；测试登记变更另跑现有清单一致性测试。提示词变更须有独立审核；不因黄金样本通过跳过。

本次只编写文档，当前可验证范围是 Spec/Plan/验收映射一致、链接和差异检查；代码、Runner 能力、真实模型和 UI 均保持未验。后续实施须完整删除旧执行路径，不保留“暂时隐藏按钮”的后台能力；不创建正式任务、不自动合并或部署。
