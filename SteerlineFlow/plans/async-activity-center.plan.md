# 异步活动中心 Plan

状态：2026-11-06 按用户授权实施；首期代码及界面已接入，独立代码审核发现的三项问题已修复并复核关闭，实际验证见[专属验收](../testing/acceptance/async-activity-center.md)。对应 [Spec](../spec/async-activity-center.spec.md)，不部署、不改安装现场或 roadmap。

## 1. 实施前固定依据

实施基线为 `a54280510802c1208d9a1a3ff0ec473cad4ebd24` 上的本工作区变更；schema 从 39 更新到 40，无旧格式迁移。受控测试使用公开 Runner fixture，固定实际二进制及摘要随每次报告保存，不声明真实模型链通过。下文保留实施安排，实际结果只在专属验收登记。

按 [Runner 调用生命周期](../../docs/product-design/runner-lifecycle.md) 保持原请求、未知副作用、恢复和结算规则。读取[测试指南](../testing.md)、[路径清单](../paths.md)及[验收流程](../testing/acceptance/README.md)。普通开发直接实施，不因文档存在分配 Task ID 或进入 roadmap。

先将以下行为冻结为回归基线：环境 `Start` 的材料读取与持久化边界；显式 `maintenance_check` 的 CommandRecord 和请求回执；准备 `RegisterPreparation` 的原请求/批次事务及后台 worker；候选预留/outbox/Runner 投影/TaskDelivery；原 `useOriginalRequests` 与候选启动协调。不得借活动接入改掉原先未知、取消或资格处理。

## 2. 纯值契约与来源边界

新增 `internal/product/api/activities.go` 定义 DTO，新增 `internal/product/query/async_activities.go` 实现只读聚合及纯映射；具体文件可随同 owner 内结构调整，职责不能转移到 HTTP handler 或 React 应用壳。

- 在 storage read ports 定义中性来源事实和有界列表读取；纯映射只消费必要字段，不导入 SQLite、app、Runner 客户端或业务 service。
- 通过 app 组装显式注册四个来源：environment_detection、baseline_check、test_preparation、task_candidate。来源接口只允许读取/分页，结果含来源身份、完整性和观察诊断。首期使用静态注册，不建立运行时插件框架。
- Activity ID 使用项目＋来源＋源 ID 的无歧义编码；request_ids 仅由持久关联产生。记录独立的 source_revision，返回响应实例身份以便客户端拒绝迟到结果。
- 编写来源映射表，覆盖业务状态、等待原因、需处理、结束原因、已知结果及资源结算。按 Spec 的来源相关性读取当前环境引用、最新显式检查、批次/Material 精确引用及候选有效参与关系；历史待答、部分重试和不同输入不能被笼统“后续成功”处理。复用现有纯投影能表达的事实，缺失时在原投影 owner 补只读能力，避免把 `web/src/features/runs/state.ts` 整体复制为第二份业务规则。
- 将阶段、计量名称、分母含义、新鲜度证据写入契约测试。未知原枚举、空时间和缺观察元数据均有明确输出，不生成虚假百分比或心跳。

交付检查：AAC-A01/A02 的映射单测通过，依赖方向可审计；活动查询的能力集合中不存在推进或控制方法。

## 3. 在原 owner 补齐事实与历史

### 3.1 检测

- 环境来源从 `Environments` / `Environment` 的只读事实发现，沿用 Record.ID、Requests、Round、Settled 和原提交诊断。增加有界分页读端口，不能每次查询全历史后再截断。环境资格与历史活动结果分别投影。
- 主分支检查从 `project_maintenance` CommandRecord 的原信封 action=`maintenance_check` 筛选，用 CommandID 为源 ID。读取原命令终态/时间，不调用 `maintenanceReceipt`、`m.check`、`source`、`observedSource` 或 Git。重启时命令仍 pending/unknown 就保持待核实，不能借新一次后台观察证明旧命令完成。
- 内部共享检查及定时检查不新增顶层活动；原 `checking` 合并机制和 actor 行为保留。本期不把同步检查迁移为另一套后台执行器。

### 3.2 测试准备

- 修改 `internal/projectmaintenance/model/generation_batch.go` 与 SQLite `materials.go` 的批次持久化：按项目＋batch.ID 保留唯一批次事实，settings 只保留当前批次引用。现有 `GenerationBatch(project)` 接口继续返回当前批次，不保留两份可分别更新的 JSON 状态。
- `RegisterPreparation` 在原事务内保存命令对应批次/关联和当前引用；`SaveGenerationBatch` 只更新相应批次。忙碌期间重复请求复用当前 ID，在原请求结果中保留该关联，不能另开一次工作。
- 补齐来源拥有的 CreatedAt、业务终结时间与当时计数快照；成功、失败和合法零项均有业务结果依据。业务结果摘要来自准备 worker 的同一事实提交，不能在 GET、浏览器或 `batchProgress` 读投影时补终态/时间；业务终结不能直接写成活动全部结束。
- 在原生成登记路径保存批次与实际 GenerationID / Inputs / TaskID 的关联，覆盖部分生成已提交后 `refreshMaterials` 或 `failPreparation` 失败的路径；关联必须在外部提交前可恢复，复用原请求登记事实，不产生额外生成。新批次不覆盖旧关联；不能只在准备循环最后成功刷新 Items 时才补关联。
- 业务终结时冻结该次结果与计数快照；沿用原批次实际工作与去重口径，不把后续 Material 或新 Generation 的结果套到旧批次。来源只读独立汇总原关联生成的运行/结算；业务失败但 Generation 仍运行/未知/未 Settled 时投影 waiting，结算齐全后才转失败终态。当前批次被替换后仍能按原 ID/关联读取，不能由观察层重跑历史准备、补写完成或自行停止。
- 原 GenerationService 的后台观察继续覆盖既有生成，不增加活动专属推进器；若结算时间未记录，由原 owner 保存适用元数据。活动 finished_at 纯推导为业务终结与关联结算的较晚时间，必需时间缺失时留空。原始业务失败快照和后续结算摘要分别保存/读取，不把后来的子生成成功推断为原批次成功。
- 现有调用使用的当前批次查询语义、生成名额、原生成 ID、停止/重试资格及材料读取流程保持；schema 若改变按未发布项目规则拒绝旧格式，不迁移旧活动。

### 3.3 候选与观察信息

- 从候选/图/TaskDelivery 的现有读取边界投影，区分 c.ID、RunID、TaskID、participation 和 withdrawal。待合并保持等待，winner 与其他候选的结果独立；不改 `runnerbridge` 原请求/同快照核验与游标推进规则，也不改 TaskDelivery 的写入规则。
- 测试摘要只从已持久化 `model.Progress` / `ProgressView` 读取；缺记录允许无进度。候选主状态与测试文件观察质量分开。
- 核对各来源可用时间字段；确实缺失的最近成功观察/最近失败事实由环境、候选桥接、测试生成等原后台观察 owner 保存。成功读取不变 revision 也能更新观察元数据；失败不更新成功时间；批次只对未结算子生成取最旧成功观察，任一缺失为 unverified，已知错误为 unavailable，本地 worker tick 不替代外部观察。预算超时诊断通过独立、有界的元数据写入保存，不延长外部取证或重试。不得挪用业务 revision 或 CheckedAt 作为其他动作的观察成功证明。
- 观察元数据若独立存放，须能与原 source ID/业务 revision 对应且无外部执行决策权；活动查询不补写。来源永久缺证据时显示 unverified，不能以当前 API 读取时间填补。

交付检查：AAC-A01–A04 的 SQLite/owner 测试覆盖保存原子性、重复原请求、批次替换、失败和重启；原业务回归保持。新增记录是原领域事实，不创建统一活动写服务。

## 4. 统一只读 API

- 沿现有 product client / runtimeProjectPort 注入读取能力，HTTP handler 只解析筛选/游标并返回 DTO。拟新增相对项目 API 的 `GET /activities`，同步 `internal/product/httpcontract/routes.go`、httpclient、applicationhttp 路由和 Web 代理白名单及契约测试。
- 列表筛选、完整集合计数、分页在来源读层有界完成；最近结束取 50，进行中/需处理可持续翻页。排序和游标统一，源读取避免全历史扫描和 N+1；加入适用索引。静态数据集翻页不漏不重，活动变化后客户端可重开查询，不声称跨请求冻结全体来源。
- 使用项目只读事务/快照取得各来源内部一致事实，响应说明来源采样，不宣称跨来源执行原子性。某来源超时/坏记录时返回该来源故障与已取得的其他结果，不能把故障伪装成空集。
- 同范围读可合并在途查询，但缓存是可丢弃的投影；不增加常驻采集器或调用外部 Runner。失败重试有界，只重读数据库。
- 输出计数不完整时标未知/部分结果。来源成功返回空集合、来源失败、未加载其他页三种情况分别测试；客户端使用完整性信息决定何时清除旧项。
- 在同一只读接口增加有界 `request_ids` 关联查询（每批最多 100），从来源的精确持久关联返回正式活动 ID/最小状态，不受列表筛选和分页限制。查找失败与明确未匹配分开，不调用原请求对账；测试正式活动位于第二页或其他筛选时仍不产生重复占位。

交付检查：AAC-A03/A05 的 HTTP/存储集成验证包含查询次数与零写入、跨项目游标拒绝、旧实例响应及部分故障；读取时不占用任务 actor 执行预算。

## 5. 顶部入口与右侧抽屉

按 Spec 第 6.2–6.4 节实施“顶部常驻入口＋右侧抽屉”。拟新增 `web/src/features/activities/` 及共享 API 类型/query options。在 `web/src/app/app.tsx` 的 App 层（keyed Workspace 外）保存抽屉打开状态和按 home/项目分隔的筛选偏好；Workspace 内组装绑定当前 home/项目/实例的观察 hook，通过回调/props/slot 将纯值接到 `web/src/app/app-shell.tsx` 现有 `topbar-actions`。当前 Workspace 会因 home/project/instance key 重挂载，不能把要求跨该重挂载保留的 UI 状态放入其中。应用壳不读取业务 API 或决定执行状态。保持现有 `Operations`、环境检测、项目维护、准备提示、执行卡片和相关原查询入口；只增加顶部入口、抽屉及最小定位能力。

### 5.1 入口与显示组件

- 顶部入口与连接状态并列，零活动保留；正式进行中、需处理、临时提交和未知/部分计数分别显示。只在项目作用域出现，确保标题与数据同属当前项目。
- 建立 ActivityEntry、ActivityDrawer、ActivityItem 等独立显示组件（名称拟定），样式沿用现有色彩/字号/间距；桌面右侧覆盖、独立滚动，不修改原业务页布局。窄屏按 Spec 切换全宽模态覆盖，使用同一数据/状态来源，不派生另一份列表。
- 卡片按标题/对象、状态/阶段/计量、等待原因、时间/诊断和详情入口组织。实现加载、三种空态、首次失败、保留旧信息、部分来源失败、分页及历史批次摘要；原始技术状态不作为主要用户文案。
- 排序与 Spec 对齐并由查询契约固定；抽屉控件区保持可见，刷新不跳动当前可见位置，最近结束与需处理筛选的计数重叠不相加。长诊断折叠与长项目/任务名称换行不能挡住关闭/详情按钮。

### 5.2 观察与交互协调

- Workspace 内观察 hook 管理 home/project/instance/query scope、轮询、分页、保留最后有效数据和来源诊断；筛选选择与展开状态由上层 UI owner 持有，不写入业务记录，不携带旧 scope 的查询数据进入新实例。
- 显式覆盖 Spec 的三类发起动作，只读合并当前 home/project/instance 的 journal 与现有 `candidateStarts.items`。本地等待首次启动项尚无 journal，按原 LocalStart.id 显示并沿相同原 ID 合并 journal/正式候选；没有原时间时留空。不调用 start/submit/reconcile、不提前创建信封或写新 journal；本地未发送项不承诺刷新恢复。
- 原实例未匹配 journal 项留在原回执历史，不生成新实例当前占位或计数；正式活动的关联查询不受发起实例影响，以持久事实精确定位。增加本地→journal→正式记录一次显示和旧 applied/unknown 请求不会复活的断言。
- 展开/收起/隐藏时按 Spec 切换只读轮询；切项目立即隔离数据并取消旧观察请求，不能取消原业务请求。关闭抽屉不触发停止或卸载原工作 owner。
- 首次打开默认进行中，之后恢复当前 home/项目的上次筛选；同项目页面导航保持抽屉，切项目或同项目换实例时保留上层打开状态但清除旧观察数据/游标并重新读取；离开项目作用域或更换 home 时关闭。覆盖 Workspace 真实卸载/重挂载，不能只测同组件 props 更新。新活动/需处理/结束及轮询变化不自动打开、切筛选、抢焦点或逐次弹通知。
- 顶部按钮切换、关闭按钮、Escape 及窄屏遮罩按 Spec 工作；桌面点击原页面不关闭抽屉。请求关联使用独立查询键，不因新增 journal 请求重置已加载活动页。通过现有导航保护确认跳转成功后再关闭，取消导航保留抽屉状态；只读历史摘要展开不跳路由。
- “查看详情 / 去处理”复用原详情定位；缺少候选定位时仅增加路由参数。成功后聚焦/滚动原对象，不执行业务动作。抽屉仅提供观察、导航和摘要展开，不移入原页面停止/重试/回答/恢复按钮。

### 5.3 可访问性与布局验证

- 桌面命名非模态区域，窄屏命名模态对话框；实现相应焦点进入、限制/释放、Escape、按钮语义、背景可交互性和滚动锁。断点切换保留筛选/数据与有效焦点，关闭后按 Spec 返回入口，详情导航后移交页面焦点。
- 覆盖 320px 窄屏、桌面及 200% 缩放，检查文本换行和按钮可达；遵循减少动态效果偏好。筛选键盘切换和 aria-live 仅播报重要变化，不随计时器重复播报。

交付检查：AAC-A01/A05/A06 的 Vitest 与受控浏览器验证通过；旧页面测试不以更新快照掩盖交互变化。

## 6. 验证与验收映射

下表为实施验证安排，不替代实际结果。所有实际调用按[路径清单](../paths.md)使用独占外部输出目录，并在[专属验收](../testing/acceptance/async-activity-center.md)记录 commit、实际命令、summary、日志和清理证据。

| 覆盖 | 测试 ID / 环境 / 时点 | 入口与通过条件 |
| --- | --- | --- |
| AAC-A01/A02 | 定向诊断，isolated，契约/映射完成后 | `go run ./cmd/test-feedback -out <独占目录> -- ./internal/product/query ./internal/projectmaintenance/... ./internal/environmentadaptation/... ./internal/taskrunner ./internal/testmanagement/...`；逐源状态、未知枚举、稳定身份、批次失败但生成未结算、候选/交付/停止组合、各来源提醒相关性/部分重试/历史问题排除、进度 false/重跑/零项断言实际命中 |
| AAC-A03/A04/A05 | 定向诊断，isolated，持久化/API 接线后 | `go run ./cmd/test-feedback -out <独占目录> -- ./internal/storage/sqlite ./internal/product/httpapi ./internal/product/applicationhttp ./internal/product/httpclient ./internal/app`；实际 SQLite 重开、部分提交失败的原生成关联、批次换代后独立结算与失败快照留存、事务回滚、无观察执行、零额外业务写入/外部调用、分页和部分故障通过 |
| AAC-A01/A05/A06 | `web.unit`，isolated，前端接线后及交付前 | `go run ./cmd/test-feedback -level=L4 -suite=web -scenario web.unit -out <独占目录>`；本地未发送项→journal→正式项合并、跨页去重、旧实例未匹配项不复活、失败保留、零活动/未知计数入口、筛选记忆、Workspace 实际重挂载时打开状态保持/数据隔离、不自动展开/切筛选、旧实例迟到、可见性及原界面回归通过；键盘/定位覆盖桌面与模态语义 |
| AAC-A03/A05/A06 | `go.http`、`go.storage`、`go.environment`，isolated，交付前各独立执行 | 分别 `go run ./cmd/test-feedback -level=L4 -suite=go -scenario <ID> -out <独占目录>`；路由/owner/存储/环境完整集合实际通过；新 App 测试若不在专项选择内，由定向诊断显式覆盖，不能靠专项名称推断 |
| AAC-A07 及必要回归 | `l1.minimum`（isolated）、`smoke.delivery`（controlled），实现完成及交付前 | 分别 `go run ./cmd/test-feedback -level=L1 -out <独占目录>`、`-level=L2 -out <独占目录>`；固定集合非零、零实际 skip、原核心交付与清理通过 |
| AAC-A03/A04/A07 | 新增 `runtime.async-activities`，L3 / controlled，完整接线后 | `go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled -scenario runtime.async-activities -out <独占目录>`；真实 SteerLine/Git/SQLite/HTTP 与公开 Runner fixture，三类工作并行、关闭浏览器继续、重启原身份、查询故障不推进/重试、批次失败/替换后原关联生成继续按原 owner 观察并结算、旧失败摘要及资源事实保留均有证据 |
| AAC-A04/A07 必要回归 | `runtime.project-maintenance`（L4）、`recovery.waiting-merge-restart`（L3），controlled，交付前 | 分别用 `go run ./cmd/test-feedback -level=L4 -suite=runtime -env=controlled -scenario runtime.project-maintenance -out <独占目录>` 和 `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled -scenario recovery.waiting-merge-restart -out <另一独占目录>`；原准备、交付/恢复及清理不回退，项目维护场景按现有共享 Web 构建约束串行 |
| AAC-A06/A07 | 受控浏览器专项，驱动已接入，接线后 | 在新增活动场景复用现有 E2E browser driver，固定构建和 Edge；按专属验收 AAC-A06 交互矩阵，实测顶部位置/跨路由、桌面非模态/窄屏模态、320px/200% 缩放、滚动/焦点、导航取消、详情定位和页面刷新；保留截图与断言，不能用仅组件测试替代 |
| AAC-M01 | 人工体验，交付后集中安排 | 参与人/窗口待安排；并行观察与需处理可理解性按专属验收单列，允许后置，不覆盖自动失败 |

`runtime.async-activities` 已登记为 L3/controlled，具有独立 home/repo/DB/端口/浏览器/证据隔离；复用预先构建的只读 Web 资产，场景内不再构建共享目录。已同步 `tests/e2e/suites.json`、项目 JSON、L3 可读列表及实际选择器。不修改提示词或其接线来让测试通过。

受控浏览器使用构建后的只读资产，实施时执行 `npm --prefix web ci` 和 `npm --prefix web run build`，记录实际构建身份。定向诊断通过不表述为整个专项通过。真实模型、真实 PR、安装部署不属于本期必需证据，也不以 fixture 宣称原生链通过。

## 7. 文档交付与停止点

实现时补充长期架构/产品说明与新来源接入约定，保留本 Spec 的解耦边界；原页面向统一投影迁移另行规划。更新专属验收实际结果，不因 PR 或实现完成自动填通过。

当前交付包含产品代码、schema 41、顶部入口与右侧抽屉、测试与长期设计。独立审核三项修复和复核结论见专属验收；原界面操作和提示词保持，暂不部署。人工体验与实际浏览器缩放限制如实登记，不因实现完成自动通过。

## 8. 本轮边界收紧与 PR 交付

沿用原 owner 与 worker 生命周期。本轮补齐用户批准的资源隔离、来源预算、关联查询和 SQL 粗筛契约，不改提示词。整合已合入 main 的原生执行配置/项目就绪门禁后，两条开发分支原本均占用 schema 40；最终数据库改为唯一 schema 41，不新增迁移或旧版兼容。

1. SQLite 写入 store 增加专用 `mode=ro/query_only` 观察池（最多 2、idle 0、busy timeout 100ms），观察故障不使业务 Open/保存失败；随 store Close 关闭，禁止观察失败回退业务连接。实际只读 inspector 沿已有只读池。测试占住业务唯一连接仍可观察、占满/关闭观察池仍可保存、禁写及取消昂贵 SQL、句柄随 owner 关闭。
2. query 按静态四来源并行采样，source context 总预算 1 秒，覆盖摘要/列表/关联与投影；上下文取消和错误只改变来源报告。验证慢来源不耗尽健康来源预算、父请求取消退出、保留有效事实和诊断；不增加常驻采集器或执行调度。
3. 增加 `mode=links` 与最小 `AsyncActivityLinks` DTO；禁止携带列表参数。关联专用函数每来源仅一次读取，测试 probe 断言零 summary/list 调用。抽屉保留独立 query key，关联分批不重置分页、不重算摘要。
4. 最终筛选统一为 `MatchesAsyncActivity`；真实 SQLite 四来源组合契约分别证明粗筛超集、最终分页/计数一致，未知/缺失状态和 nil projection 均有反例。发现遗漏时修粗筛，不能放宽最终投影、删除测试或改业务事实。
5. 将本聊天已有首期实现与本轮收紧一并提交 `codex/async-activity-center` PR；用户已授权提交和提 PR。独立审核及必要修复/补验完成后再 commit/push/建 PR，不合并、不部署。

| 覆盖 | 测试 ID / 环境 / 执行时点 | 命令、通过条件及证据 |
| --- | --- | --- |
| AAC-A08/A09 | 定向诊断 / isolated / 接线后 | `go run ./cmd/test-feedback -out <独占目录> -- ./internal/product/query ./internal/storage/sqlite ./internal/app -run TestAsyncActivity -count=1`；资源禁写/隔离/关闭、context 超时、关联零摘要、四来源粗筛超集和最终分页计数全部实际命中 |
| AAC-A08/A09 | `go.storage`、`go.http` / isolated / 交付前 | 分别 `go run ./cmd/test-feedback -level=L4 -suite=go -scenario <ID> -out <独占目录>`；完整专项非零、零实际 skip，无原资源/查询回退 |
| AAC-A08 及原交互 | `web.unit` / isolated / 交付前 | `go run ./cmd/test-feedback -level=L4 -suite=web -scenario web.unit -out <独占目录>`；专用关联参数与已加载页保留、原交互通过；另 `npm --prefix web run build` |
| AAC-A03/A07/A08 | `runtime.async-activities` / L3 controlled / 完整接线后 | `go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled -scenario runtime.async-activities -out <独占目录>`；三类来源、故障保留、浏览器交互、重启原 ID/原 Run、零额外业务输入及清理实际通过；与 HTTP 压力串行 |
| 固定回归 | L1、L2 / isolated、controlled / 交付前 | 分别 `go run ./cmd/test-feedback -level=L1 -out <独占目录>`、`-level=L2 -out <另一目录>`；固定选择完整通过 |
| 文档及独立审核 | 交付前 | 文档链接、`git diff --check`；独立子代理核对读写/预算边界、SQL 粗筛契约及完整 PR 范围，问题修复后复核 |

本轮输出使用 `C:\SteerlineDebug\wt1369-activities\test-results` 下新目录，实际报告和原失败保留在专属验收。先前通过不冒充本轮变更后通过；环境业务 owner 未再变更，既有环境完整专项不因新增 pool 重复跑，无需真实模型/安装验证。

整合基线更新为 `b3c654ed`（main，PR #461–#463）；保留上游 execution identity 校验、FormatVersion、项目 readiness 路由、原请求回执核实及原生配置模板，首期观察只增加自身字段/路由/资源。所有最终报告重新固定该基线，不沿用整合前通过结论。

独立审核修复追加：环境观察原先仅有成功时间，最近一次已知 Inspect 失败仍可显示 current。由环境 owner 在原保存中增加独立观察诊断，健康恢复清除该观察诊断及其遗留公开错误，保留其他业务诊断。新增 `TestEnvironmentActivityObservationErrorAndRecovery`，真实 owner→SQLite→纯投影验证成功/错误/恢复及不清无关业务诊断。修改后重跑该定向、固定 L1/L2、受影响 go.environment 及活动受控 E2E，再由原独立审核者复核；其他未变专项保留本轮最新 main 上的实际通过报告。
