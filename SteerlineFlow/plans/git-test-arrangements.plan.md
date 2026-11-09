# Plan：Git 管理任务测试安排

日期：2026-11-09。状态：用户已授权实施并提 PR，验证进行中。需求归 [Spec](../spec/git-test-arrangements.spec.md)，具体步骤、待验及后续结果归[专属验收](../testing/acceptance/git-test-arrangements.md)。本次为普通开发材料，不申请 Task ID、不改 execution-map、不启动 roadmap-runner。

## 1. 输入与现有代码调查

源码基线 `221812ec058ec7dd608e447f8aeaeafb8587e376`。已读取 `SteerlineFlow/spec/SpecGuide.md` 全文；其最后修改提交为 `227e23eea0bf36e448a19d91ccab829700af2d8e`，文件 SHA256 为 `efef07665c308a9ec98c44d73f05eea828f38a774c68c89578ff4609f4ec7da2`。本轮先按用户要求编写并审计文档，随后获授权实施并提 PR，不采用产品转正流程来创建本任务。

实施前按真实最新主干核对下表所有者、消费者和正在进行的修改；若代码位置变化，保留职责并调整定位，不消费猜测接口。

| 位置 | 已有能力及整合方式 |
| --- | --- |
| `internal/testmanagement/model/inputs.go`、`documents.go`、`project_entry.go`、`records.go` | 复用路径推导、严格 JSON/清单解析、选择展开、项目入口和原 Snapshot；删除 GenerationID/生成输入，不增加来源对象或可推导字段 |
| `internal/projectmaintenance/rules.go`、`model/`，`internal/app/project_materials.go`、`test_documents.go` | 替换生成输入/结果匹配为固定 Git 安排读取；保留批量 Git 读取缓存、协议/writer 核验和候选公开文件通道；新鲜度只纳入所选清单定义 |
| `internal/roadmap/model/model.go`、`generation.go`、`documents.go`，`internal/roadmap/documents.go` | 为原 Documents 增加测试 ID 数组；扩展 schema 和原文件集组装，平台编码同名文件；复用原确认规划/来源核实/问题返回，不另起模型调用 |
| `internal/app/roadmap_documents.go`、`roadmap_publication.go`、`roadmap_admission.go` | 将 JSON 纳入原 diff、确认摘要、文件发布和合并核验；入图从 Plan 推导验证，不扩 TaskContract 创建参数 |
| `internal/app/project_readiness.go`、`internal/storage/sqlite/project_readiness.go`、`runner_candidates.go` | 以 Git 材料和完整快照替换成功 Generation 资格；保留全项目未完成任务检查、事务内重核、回滚及已预留重放优先级 |
| `internal/app/maintenance_generation.go`、`test_management.go`，`internal/testmanagement/generation*.go`、相关 storage 端口/SQLite | 审计引用后删除任务安排生成专用服务、队列/批次/配置及状态持久化；共享 Runner、roadmap、环境准备和其他生成 owner 保留 |
| `web/src` 的项目维护、任务材料、候选测试列表、roadmap 审阅及活动中心 | 删除生成操作及状态，接入 Git 来源和错误；保留查询隔离、原请求和现有进度视图 |
| `internal/prompts/`、`debug-template/`、`tests/e2e/`、测试清单及指南 | 同步生成/执行材料规则、真实项目种子和驱动；删除夹具主动补生成的假前置，测试不得再借旧生成记录放行 |

保留任务、执行、产品查询与宿主原边界。此次不增加 Runner 仓库修改或跨仓前置；真实联验只消费公开构建和契约。

## 2. 实施步骤

### P1：确定 Git 安排解析与快照

1. 在现有 testmanagement 模型中复用 `ArrangementPath`、`Select` 和完整定义摘要，提供一个固定读取入口；输入是明确提交和 Plan 路径，读取项目声明的唯一清单及同名 JSON，拒绝工作区/跨提交兜底。
2. 保留原 Snapshot 的 Commit、PlanPath、Project、Definitions/Digest，删除 GenerationID/Inputs 及服务依赖。JSON 路径直接推导，文件来源沿原采用提交查证。原材料摘要的清单部分改用所选定义摘要，不再混入整份清单摘要；无需增加 source manifest、新鲜度服务或第二份来源记录。
3. 维持空安排说明及严格格式检查；语义覆盖归材料生成和正常审阅，不新增脚本判断自然语言或运行时自动加项。
4. 以纯模型、Git 读取和现有 App owner 测试先验证非法输入、精确来源和相关/无关清单变化，避免到 E2E 才定位基础错误。

### P2：打通 Roadmap 同批生成与发布

1. 在原 `Documents` 与 output schema 增加必需的测试 ID 数组，复用现有严格选择校验。无需新增一个带 path/body 的 JSON Document，也不要求模型转义一层 JSON 字符串；沿原大小、任务身份及文件集限制处理。
2. 文档生成提示明确完整选择和项目必跑项、Plan/JSON 分工、缺 ID 时返回 questions；沿原固定项目材料读取，不内联规范、不新增一次 generate。
3. 在原 `publicationFiles` 文件集组装处由测试数组统一编码真实 JSON，并通过 Plan 推导路径加入。diff、确认摘要、发布和合并核验复用该结果；原文档编辑机制处理测试数组，不建设 JSON 编辑器或保存另一份 JSON 正文。所有编辑撤销原确认，选择不变允许保留安排。
4. 在实际合并正文核验与 admissionBatch 前置校验中检查同名文件。生成采用时使用原 CodeCommit 的清单；合入时使用实际合并提交的清单，所选定义变化沿原差异视图提示；不锁整份清单或增加二次模型审核。缺失、正文不匹配或引用无效均零部分入图。原发布未知及原入图请求继续对账，不重新发布或分配编号。
5. 更新受控结果、发布夹具及实际文档生成驱动，证明一次生成覆盖全部文件，不手工在生成后追加 JSON 冒充通过。

### P3：替换维护和候选准入

1. `materialInput` 改为读取 Git 安排并形成快照；维护后台仅执行有限静态读取，远端检查/更新后刷新投影，不等待模型、不登记生成批次。查询仍只读投影；启动前重新核对当前基线和静态材料。
2. 替换材料就绪、task readiness、候选准备和 SQLite 预留中的 Generation 条件；Git 材料缓存不依赖执行 profile，当前所选执行能力继续由原候选输入校验负责，保留全部未完成任务门槛及事务竞态防线，不只改前端按钮。
3. 将实际 JSON 加入候选材料读取/差异和来源核验；原公开 workflow 上下文、安排文件通道和 progress seed 继续使用同一冻结定义。冻结安排与采用提交不一致时拒绝预留。
4. 保留原候选不可变、精确请求重放、同构建恢复、旧 PR 交付及所有资源停止义务。补反例：兄弟任务安排损坏、读取未知、预留前版本变化、事务前后定义变化均不产生半候选或 Runner submit。

### P4：删除专用补安排链路

1. 从命令/查询、项目 worker、活动中心、配置、存储及 CLI/Web 调用逐一断开任务测试生成服务。删除无消费者的服务、批次、profile 设置、类型、端口、表和旧提示词；共享 owner 必须按实际调用核对后保留。
2. 维护页保留 Git 检查/同步和材料错误；任务/候选显示采用提交、JSON 路径和完整选择。取消准备、排队、等待模型、重试及取消生成，不增加第二个采用/批准页面。
3. 当前开发 schema 直接调整，无迁移、旧库导入、JSON 回填或双读分支。安装中原活动的停止和升级属于原维护流程；本任务不操作真实现场或跨构建恢复。新格式验证使用全新隔离 home。
4. 同步 API/CLI/TypeScript 类型、活动分类和相关测试。删除入口需要验证确已不可调用；不能留下无 UI 但仍可触发生成的命令旁路。

### P5：同步材料和验证入口

1. 更新项目维护设计、生命周期的材料段、SpecGuide 及其嵌入来源、Spec/Plan 编写规范、测试架构/项目指南和路径说明；说明未来 Plan 引用 ID 与清单，减少通用定义抄写。本 Plan 的测试表按当前规范写全，不提前要求现行工具支持新格式。
2. 更新 roadmap 固定规则/短提示、生成和执行实际请求样本，删除任务测试生成专用资产。保留协议文件、正式 writer 及 Runner 原文所有权。
3. 更新 debug-template、E2E 项目种子、driver 和受控 Runner 结果，确保材料提交本身带 JSON，启动前没有专用生成 profile/批次。不批量改写历史 Spec/Plan 或真实任务。
4. 原 L3 `testing.material-generation` 随独立生成能力删除退役，删除其 driver 和登记，不做别名。Git 安排解析、引用及零模型调用反例由现有模型/App owner 验证；公开多版本流程补进既有 `runtime.project-maintenance`，不再新增一个覆盖重叠的 L3 场景。
5. 调整 `runtime.project-maintenance` 中旧准备步骤，保留多版本候选、Git 更新、交付和清理断言；复用 `testing.runtime-progress` 验证候选按 Git 快照初始化和汇报。现有 L4 `testing.management-local-codex` 的夹具改为预先提交 JSON，删除其独立生成断言，保留真实候选进度目的。本任务未改 Runner 契约，不把该 live 场景再设为默认必跑；只有实际公开传输变化或受控证据不足时才追加并说明原因。同步 `tests/e2e/suites.json`、正式清单、L3/L4 列表及执行器校验，不改测试通过条件或预算。
6. 如删除旧测试使精确 owner 根集合变化，同步 go.roadmap 等正式定义并取新报告；不通过放宽非零、skip 或源漂移检查消除失败。

## 3. 验证安排

此前文档阶段只运行 T0；用户随后授权实施并提 PR，当前执行 T1–T6。以下 T1–T6 是后续实施安排；用例补进现有 owner 和场景，不要求另建测试命名体系、场景框架或人工签收。所有结果写专属验收，不回写为本 Plan 的完成状态。

下表 A01–A07 对应 Spec 的 GAT-A01–GAT-A07；每行给出执行安排，不另维护一张重复的验收映射表。

环境：Windows、Go/Git、Node 24+/npm；受控测试使用真实隔离 Git/SQLite 和公开 Runner fixture。Web 先 `npm --prefix web ci`，需要浏览器的场景先 `npm --prefix web run build`，构建串行。证据根沿 [paths.md](../paths.md) 使用 `C:\SteerlineDebug\<scope-id>\`；每次测试分配新 `test-run-id`，下表 `<out>` 是该次独占绝对目录，命令中的定向参数不得缩窄正式 ID。

| 项 | 验收映射 | 级别/入口、环境与命令 | 时点、通过条件与证据 |
| --- | --- | --- | --- |
| T0 | 文档交付；A07 的规划部分 | 静态：`python scripts/check_doc_links.py`；`git diff --check` | 本轮及最终交付前；本地导航无新增错误、无空白错误。只证明文档，不登记产品自动项通过 |
| T1 | A01–A05 | 定向 Go：`go run ./cmd/test-feedback -out <out> -- ./internal/testmanagement/... ./internal/projectmaintenance/... ./internal/roadmap/... -count=1`；App：`go run ./cmd/test-feedback -out <out> -- ./internal/app -run 'Test(PMS|ProjectReadiness|MaintenanceCandidateAdmission|TaskReadiness|Roadmap)' -count=1 -timeout=25m` | P1–P4 后；将新反例补进已有 owner，按实际测试名调整最终选择并登记。覆盖非法/空安排、来源、编辑撤销确认、相关/无关定义变化、重复检查零生成、兄弟任务阻断和预留回放；summary/JSONL，必须实际非零 |
| T2 | A01–A07 必要回归 | 固定 L1：`go run ./cmd/test-feedback -level=L1 -out <out>`；固定 L2：`go run ./cmd/test-feedback -level=L2 -out <out>` | 实施完成及最终集成后；固定集合完整通过，清理完成，超时目标如实记录；独立 summary/managed/scenario |
| T3 | A02、A04、A05 | L4 isolated：`go run ./cmd/test-feedback -level=L4 -suite=go -scenario <ID> -out <out>`，将 `<ID>` 分别替换为 `go.roadmap`、`go.storage`、`go.http`、`go.deployment`、`go.environment`，每次独立输出 | 更新正式定义后；精确 owner 全量、非零/零跳过，覆盖入图、事务内 Git 快照重核/回滚、已预留重放和 API 删除；本次删除生成责任还影响原停服/部署核验，故补部署专项；各自 summary/JSONL。HTTP 压力与 E2E 串行 |
| T4 | A02、A05 | L4 isolated：`go run ./cmd/test-feedback -level=L4 -suite=web -scenario web.unit -out <out>`；`npm --prefix web run build` | UI/类型更新后；原生 Vitest 全集通过，roadmap diff、Git 来源和错误可见，旧生成操作不可触发，生产构建通过；summary/Vitest 与构建日志 |
| T5 | A03–A06 | L3 controlled：`go run ./cmd/test-feedback -level=L3 -suite=testing -env=controlled -scenario testing.runtime-progress -out <out>`；L4 controlled：`go run ./cmd/test-feedback -level=L4 -suite=runtime -env=controlled -scenario runtime.project-maintenance -out <out>` | 驱动更新后；真实隔离 Git/SQLite/浏览器与公开 Runner fixture 验证 JSON 读取、缺失/坏 ID 提示、重复检查/重启/代码推进零生成、多版本候选/旧 PR 交付及停止结算。runtime-progress 用受控公开快照验证输入绑定、进度变化与异常消费，不证明原生模型实际执行；两份 summary/scenario，长场景不能以部分通过替代 |
| T6 | A02、A07 | 现有 live 专项：`go run ./cmd/test-feedback -out <out> -- -tags=e2e ./internal/app -run '^TestRoadmapLocalCodexPromotion$' -count=1 -timeout=20m` | 受控通过后；固定独立 Runner EXE/版本摘要、本机 Codex 登录、`STEERRUNNER_EXE` 和 `STEERLINE_ROADMAP_EVIDENCE` 沿原驱动提供；一次真实生成返回合法测试数组，平台编码 JSON，经原隔离 provider 发布/合并/入图，原 Run 停止完整。不得在驱动中自行选择测试掩盖模型遗漏；保留原结果、Git 实际内容和清理证据 |

实施审计另发现 `runtime.async-activities` 及其浏览器仍要求已退役的测试准备活动；同步为检测/候选可见、测试准备缺席，并按 T5 同一受控入口补跑该正式 ID，不放宽其余重启、交付与清理断言。

T6 仅在原入口所需构建、认证和明确外部操作授权具备后执行；缺前置保留待验，不能用 controlled 结果宣称真实生成通过。未改 Runner 原生工作流、writer 和文件通道，候选进度的本次来源/绑定风险由 T1/T5 覆盖，无需再为它启动一轮模型。不扩大为全部 live、Go Full、远端/Orca 全链或真实安装更新；新影响和实际失败另据证据追加范围。

## 4. 提示词治理与交付

实施 P2/P4/P5 时，先定位实际调用方正文，再记录修改/删除的原文与新文、前后 UTF-8 字节数、删除重复规则的业务理由；删除记为原值→0，不能只报文件大小或摘要。

在专属验收中保存实际差异和测试定位，按[提示词治理](../../docs/prompt-governance.md)运行 prompts owner、固定 L1/L2 和受影响场景。扩展 schema、文件读取和模板接线纳入同一独立只读审核；审核者复核实际正文及输入/来源边界，修正确认问题后复核，不以黄金样本更新代表批准。此步骤属于未来实施验收，本轮不虚构独立审核已完成。

界面操作由 T4/T5 自动验证，不新增人工验收窗口或签收阶段。实施结果同步 acceptance 和 Spec 索引；本任务未纳入 roadmap，不向 execution-map 添加伪执行状态。提交、PR、合并和部署按届时用户授权，文档交付本身不触发这些动作。
