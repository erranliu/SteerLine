# Roadmap Markdown 解析与转正实现 Plan

日期：2026-10-26；接入更新：2026-10-27。状态：用户已授权实施及对接已合入 Runner。完整业务路径已接通，真实本机 Codex 解析/调查/六份文档、隔离 PR 合并与入图通过；正式 Edge 浏览器整链、真实 GitHub 文档 PR、编号/依赖、正常停服及清理最终已通过，实际结果见[专属验收](../testing/acceptance/roadmap-promotion.md)。普通开发，不改本仓 execution-map、不分配本仓开发任务号、不部署安装环境。

## 1. 调查结论与实现边界

代码基线：2c1cd9682b3e421f21b5935ccd73a06624015e42。下表为实施前调查位置；本轮新增模块、测试和命令接线已落地，文档生成契约已按实际 Runner 交付接入。

| 位置 | 已有职责 | 本功能安排 |
| --- | --- | --- |
| internal/runnerclient/content.go、internal/runnercontract/content.go | generate/1 公共调用、只读配置、结构化输入/结果 | 解析直接复用，文档阶段仅在 Runner 正式能力/schema 交付后消费 |
| internal/testmanagement/generation.go、generation_queue.go | 测试生成原请求/结果、共享并发准入、恢复 | 复用机制与配置，避免保存 Roadmap 到 tests 记录；必要的共用调用辅助窄化后下沉，不建通用 Agent 框架 |
| internal/task/batch.go、commands.go、model/batch.go | Task 批次规则、事务端口与显式编号输入 | 在同一 Task owner 扩展 Roadmap 入图，复用批次校验/写入/回执；编号分配与来源映射加入同事务 |
| internal/storage/ports、internal/storage/sqlite | Task/命令/生成持久化 | 保存规划/发布原事实与来源唯一约束，编号事务原子性；不要求 Runner 或 Web 写业务库 |
| internal/app/task_command_store.go、runtime_task_commands.go、taskrunner | 存储适配、actor 串行变更与产品组装 | 加窄端口接线，联网/模型/Git/provider 调用放事务及 actor 长阻塞之外 |
| internal/app/project_baseline.go | 既有 Git 和 PR 信息观察 | 复用 Git 执行/目标检查机制；新增文档发布所需窄 provider 适配，不谎称已有完整发布 API |
| internal/product/api、httpapi、httpcontract，web/src/shared/api | 公开值、原信封命令与路由白名单 | 加规划查询/显式动作及类型/契约测试，沿现有请求与观察边界 |
| web/src/app、features/dag，新增 web/src/features/roadmaps | 页面路由、查询、来源入口 | 只读草稿→依赖确认→文档审阅→发布→入图与映射，沿现有缓存和项目作用域 |
| 新增 internal/roadmap | 本版规划纯规则与业务记录 | 仅含读取快照、解析/确认、生成结果和转正编排；Task 写入仍走 internal/task |

下表记录的是实施前基线；本轮新增 internal/roadmap（只读与内容调用）、internal/roadmap/model（持久化纯值）、app 窄发布适配和 Task 事务内自动编号。原型只是交互材料；原型里的确定性解析不是生产 Markdown 解析器或模型替代。

## 2. 前置与 Runner 独立交接

### 可先安排的范围

在正式开始实施后，可先交付只读文件/来源模型、现有 generate 解析、用户确认、Task 入图纯规则与受控 fixture；这些不证明文档阶段可用，也不释放未满足的端到端验收。

### 文档阶段已交付与接入方式

Runner aa8c392f9798d05184788d83947dd7bf4afc75a6 已交付 generate/1 的可选 repository={local_path,commit}，公开 generate capability / generate_input schema 与 repository_source。最终 JSON 上限 1 MiB，manifest 证明固定版本可用文件，不证明模型逐文件已读；业务调查引用须核对路径/摘要。

复用现有内容记录：解析确认后创建 purpose=documents、parent_id 指向原确认的独立请求记录，冻结代码 commit/tree、规范、原 Markdown 和确认摘要。两种用途共用 submit/observe/stop/队列/接管，无新运行时或恢复机制。Schema 固定本批内部代号和语义路径，返回逐任务 Spec/Plan/验收正文、整体设计、调查引用和问题；问题结果不采用文档。仅已 completed、原生来源 verified、资源 stopped、Run/结果仓库来源一致时采用完整文档。

页面按所选已核实目录开放能力，旧 Runner 保持具体缺能力诊断；支持完整发布目标、原/新正文（含 JSON 与 execution-map）预览。入图仍使用既有 Task 事务分配编号。

## 3. 实施步骤

1. **固定业务契约与来源身份。** 在新增 roadmap 模块定义 roadmap_id/local_key、来源字节/摘要、解析/确认/生成/发布快照、已有关联和入图映射。首版一份草稿整体转正，代号只在所属 Roadmap 内有效。定义结构化输出及问题类型，不接受模型分配正式编号。读取沿现有普通文件/编码/大小检查，保留未提交来源与代码基线的区别。
2. **接入解析和确认。** 复用 GenerateRunInput/GenerationExecution、能力校验和原请求生命周期，传完整 Markdown 与已有正式 Task/DAG/相关材料；新记录按用途保存，复用共享并发额度。解析结果分原有/拟议依赖和变化依据，确定性核对引用/环/身份。确认只保存结构化规划，不创建 Task 或编号。明确失败显式重试，unknown 原号核对；补齐关闭/重启/未绑定生成的停止和结算。
3. **落实入图事务。** 扩展既有 Task 批次内部准备/写入契约，使 Roadmap 入图输入只含内部身份和固定正式文档。不要在 UI/规划业务预填显式 task_id 后调用旧命令冒充入图分配。BatchTx 内确定未用编号、转换依赖、写 Task/Contract/来源映射/命令回执；数据库增加仓库编号及来源唯一约束，处理同号重放、异号重建、并发竞争、故障回滚。保留仍有真实消费者的现有手工批次入口，但共用校验和唯一写入 owner，不增加旧数据兼容逻辑。
4. **接入文档新能力。** Runner 前置交付后，固定完整代码/规范/已确认规划，按公开契约调查并生成完整文档。校验来源、文件集、路径、编码、大小、跨任务安排、Spec/Plan一致性和依赖。以语义路径保存结果；修改撤销确认，任务/依赖变化退回重新确认。测试计划只写要求，不改变现有系统测试生成/候选冻结。
5. **实现文档发布。** 在隔离文档分支准备本次文件，原 Markdown不变；发布前展示完整 diff/目标及动作授权范围。新增窄 Git/provider 适配，外部动作先保存固定意图与原回执，未知只查原分支/PR。核实真实合入目标主分支后保留待入图状态，零新编号；不借 generate 进行写操作。
6. **串联入图与页面。** 用户显式入图后调用步骤 3 的同一批次，展示原回执及正式映射，双向链接来源。一个批次全部提交或全部回滚；丢回执显示待核实，通过原请求恢复，不按 UI 行数解释成部分创建。沿现有正常派发资格运行，用户预知自动派发的可能结果。
7. **测试、文档与交付。** 实现期间补齐下表未存在的测试/fixture/driver及登记，执行获授权的最小验证，逐项登记实际结果和失败；更新产品设计、API、模块所有权及必要用户说明，同步 SpecGuide 的身份/编号时点，保持与本次确认一致。原型的“逐行部分入图”故障演示收敛为“整批提交回执丢失/待核实”。所有要求满足后才可称产品流程已实现，生成或文档 PR 合并本身不算完成。

步骤 4 已按真实公开契约接入；不新增 workflow 或自行调用模型。范围内共享的 schema/Task事务/原请求组装由当前实现者统一整合，不拆多个彼此写权重叠的模块。

## 4. 状态与恢复落实

遵循 [Worker 生命周期](../../docs/product-design/worker-lifecycle.md)与当前生成/服务换代规则。本次差异仅为两类内容操作（解析、文档生成）、文档发布和 Task 入图的业务身份及结果。内容 Run 不转成 Candidate，发布记录不伪装 Runner 执行状态。

原请求绑定用途、项目/home/仓库、固定输入、profile和构建，保存后才调用。结果必须核实来源/完整性，再以原确认版本应用；迟到旧结果仅属历史。未确定/取消/停止/资源结算分别保存，不因页面刷新或重启换号重发。

Git 与 SQLite 不跨系统开长事务：先核实原文档合并，再在单一 Task 事务完成入图；Git 成功而入图未完成保持可恢复。事务成功而客户端失联时，通过原命令回执获取完整映射；不能重开文档 PR、提前保留一组可用正式号或调用第二套写库入口。

## 5. 测试计划

本节按实际入口维护验证安排。为避免重复驱动框架，原拟两项新 L4 场景改为复用既有 test-feedback 定向入口与同一个生产 owner 链测试：controlled 和 e2e tag 的真实 Codex 分别运行。它们不登记为不存在的 L4 场景，不声称浏览器完整整链已验。实施时先实现/登记和核对枚举，零匹配/skip/仅编译不算验收。自动行为明细唯一归[专属验收](../testing/acceptance/roadmap-promotion.md)。

### 环境与新增入口

- Owner 测试：Go + 临时 Git/SQLite + 公开 Runner/provider fixture；不调用真实模型或安装环境。
- Web：沿当前 Vitest/Playwright，纯 DTO/交互用模拟公开 HTTP；正式整链使用隔离 Go/Web/Edge。
- Controlled：TestRoadmapPublicationAndAdmissionWithRealGit 通过生产 action/use-case 从读取、解析、确认、固定提交文档生成到实际 Git 合并/SQLite 入图；公开 Runner fixture 和 loopback provider。
- Live：e2e build tag 的 TestRoadmapLocalCodexPromotion 复用同一条路径，只将内容执行替换为固定实际 Runner/本机 Codex。Provider 继续使用隔离本地服务；不创建外部 PR。
- 不为这两条路径新增运行时/调度框架。前次验证时浏览器全链与外部 provider 尚未验；后续用户授权的完整验证单列于第 7 节和专属验收。

证据放 [路径清单](../paths.md)规定的仓库外 scope。示例范围根为 C:\SteerlineDebug\wt-1285-roadmap-promotion，实施在别的 worktree 时换为该 worktree 的唯一 scope；每次命令使用新 UTC+UUID 叶目录。

| 计划编号 / 验收覆盖 | 级别与执行方式 | 环境、时点、通过条件及登记 |
| --- | --- | --- |
| V1：RP-A01–A04、A06、A07 | Owner 定向：go run ./cmd/test-feedback -- ./internal/roadmap/... ./internal/task/... ./internal/storage/sqlite ./internal/runnerclient ./internal/app -run '^TestRoadmap' -count=1 | 实施各步骤后运行；新增 TestRoadmap* 必须覆盖读取/解析/确认/文档校验/原事务/重放和故障。各修改 owner 的对应新测试非零、断言通过；证据归 diagnostics 新目录并登记各验收项 |
| V2：RP-A03–A08 | Web 定向：npm --prefix web test -- src/features/roadmaps src/app src/features/dag；npm --prefix web run build | Web 接线后；代号、建议差异、能力不足、只读、待合并/待入图、原号恢复、映射跳转、跨项目和失联展示正确，构建通过。新 features/roadmaps 文件必须已存在；日志/截图归 diagnostics |
| V3：RP-A05–A09 | controlled owner 整链：go run ./cmd/test-feedback -- ./internal/app -run '^TestRoadmapPublicationAndAdmissionWithRealGit$' -count=1 | 真实临时 Git/SQLite + 公开 Runner/provider fixture；原 PR 回执丢失后查询/继续，文档合并前零编号，入图事务与原映射恢复正确。非浏览器整链 |
| V4：RP-A02、A04、A05、A09 | live owner 整链：go run ./cmd/test-feedback -- -tags=e2e ./internal/app -run '^TestRoadmapLocalCodexPromotion$' -count=1 -timeout=20m | 显式固定 STEERRUNNER_EXE、可选 STEERLINE_ROADMAP_CODEX、STEERLINE_ROADMAP_EVIDENCE；真实 Runner/Codex 与版本调查，六份完整文档，provider/remote 仍隔离；原资源正常停止，不发布外部 PR |
| V5：必要回归 RP-A06–A08 | go run ./cmd/test-feedback -level=L1；go run ./cmd/test-feedback -level=L2 | 按项目规则在产品代码修改后、提交前执行；既有批次/候选/交付核心链不回退；原始报告归 L1/L2 独立目录。回归通过不替代新功能验收 |
| V6：RP-A08 / 边界回归 | go test ./internal/testkit/boundary/... ./internal/product/httpapi/... ./internal/product/httpcontract/... -count=1 | 公开接线完成后；模块依赖/路由白名单/命令仲裁无旁路，新增类型与隔离守卫有效；日志归 diagnostics |
| V7：文档交付 | python scripts/check_doc_links.py；git diff --check | 当前文档及实施交付时；链接、路径、状态和场景登记说明一致。静态检查只证明文档，不算RP自动项实现通过 |

执行时给 test-feedback 各命令传本次唯一 -out；Web证据沿 STEERLINE_WEB_EVIDENCE/专用日志目录。新入口脚本按项目[测试指南](../testing.md)实现记录和完整性判断，不把这里的计划命令当作当前已存在能力。

V1 的新增 TestRoadmap* 按验收ID分组命名并固定枚举；本 Plan 不要求跑所有普通 Go 包或无关原生/Orca/沙箱矩阵。V4 只验本机 Codex 和本次隔离 PR，不继承旧测试生成 live 的通过结论，不增加第二个发布平台覆盖。

### 自动验收覆盖对应

| 验收项 | 最小执行安排 |
| --- | --- |
| RP-A01 | V1 来源/文件边界、刷新零副作用；V2只读入口 |
| RP-A02 | V1 GenerateSchema/Profile/Result，V2原文/建议，V4真实解析 |
| RP-A03 | V1 身份/图/确认/版本，V2建议确认及跨项目，V3整链拒绝 |
| RP-A04 | V1能力/文档结果/路径，V2受控修改和前置，V4真实调查/多文档 |
| RP-A05 | V1发布状态与未知对账、V2明确权限范围、V3真实Git+provider fixture、V4真实内容与隔离PR链；第7节补正式浏览器/GitHub真实PR |
| RP-A06 | V1SQLite并发/事务/来源唯一约束、V3正式入图及丢回执、V5既有Task回归 |
| RP-A07 | V1原请求/迟到/停止、V3重启/未知外部结果、V6原仲裁边界 |
| RP-A08 | V2/V3完整页面和隔离、V5既有候选/派发回归、V6路由/owner |
| RP-A09 | V3/V4分别形成完整结果与清理证明，缺任何一项保持未验 |

## 6. 人工安排与完成边界

RP-M01 在新隔离包中由用户或指定体验人确认中文代号/正式编号区别、依赖变化、发布权限范围和映射导航，窗口待安排，可后置且不阻塞无关开发。真实模型/PR、事务错误、能力缺失或安全边界失败均不是可后置人工项。

本轮产品接入按用户后续授权实施与验证；浏览器及真实 GitHub PR 已由第7节实际证据确认，安装部署与人工体验仍未验。

## 7. 后续授权：正式浏览器与真实外部 PR 联验

用户追加授权补齐浏览器整链与真实外部 PR，失败直接修复。使用 `scripts/test-roadmap-browser-live.ps1` 与 `web/tests/roadmap-live.spec.ts`，按独立 L4 live 专项执行，不虚构受管场景 ID。前置、模型消耗、专属远端分支、完整性判断与清理见 [E2E 指南](../../tests/e2e/README.md)。覆盖 RP-A01–A09 的正式 HTTP/Web 串联，真实 GitHub 测试库只使用随机 base/head；其他平台与人工体验仍单独记录。失败保留原始结果，修复后在新 scope 复验，结果追加专属验收文档。

联验发现并纳入本次修复：客户端项目路由与 Go Web 页面回退同时登记 roadmaps，保证直接打开、刷新、项目切换及返回；正文/选中来源使用当前深色主题，浏览器计算文字对比度；Roadmap 与入图后测试安排的 Close 等待原 Run 停止。Runner 明确拒绝 stale_revision 后，下一次观察可按新 revision 形成新的内部 stop 信封；未知或已接受的信封保留不替换。新增两类 owner 故障测试区分确定拒绝与未知结果，资源未结算不得报通过。

补充：发布临时 checkout 保存在正式 owner 的 `roadmap-publications` 状态目录；本机 preflight 允许该受管普通目录，仍拒绝文件替代、目录重定向和未知根。离线 verify-stop 与重开状态共用同一守卫，因此把有发布 checkout 的目录加入主机 owner 回归。

真实 SQLite 补充：停止信封的拒绝事实先独立保存；只有原行已为 rejected，且下一次新观察准备 unknown/不同 Request ID 时，存储层才允许更新内部停止信封。unknown/accepted 信封保持不可变，用户 Stop ID、Run、输入、原生成请求等绑定保持不变。用真实 SQLite 同时验证两类 owner Close 的 revision 竞态，避免内存 store 掩盖持久化约束；重启 pending_stops 沿同一转换。
