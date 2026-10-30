# 产品读取、命令入口与 Web 容器重构 Spec

2026-10-29：本文涉及的旧 Runner Check 专属 submit/文件已按[退役规格](legacy-environment-check-retirement.spec.md)删除，相关分析只作原版本历史；普通命令仲裁和 native viewer 的边界仍有效。


2026-10-16，完成审计后，用户授权实施并提 PR；本 Spec 的 A/B/C 范围已落地，自动/人工结果以专属验收为准。审计基线 `57fadfce9390982748d5ff39e6e378c47f0cf5fc`，包含已合并的应用/项目生命周期收拢。配套 [Plan](../plans/product-read-command-web-refactor.plan.md) 与[专属验收](../testing/acceptance/product-read-command-web-refactor.md)。普通开发，不占 Task ID，不修改 roadmap。

## 1. 审计结论

建议实施经过收窄的三批改动。理由是已有规则重复、语义混用及跨职责副作用，文件长度只是定位线索；没有证据支持现在重建全部读取模型或引入命令框架。

| 建议 | 值得实施的部分 | 收益与风险 | 排除的扩大范围 |
| --- | --- | --- | --- |
| 候选到产品展示转换 | 明确参与关系、提交状态、Runner 观察与交付的来源；统一详情/历史的原始状态规则，Web 使用显式参与事实 | 收益最高；修正可静态确认的不一致。涉及 DTO 和展示，需要后端/Web 同批交付 | 暂不替换整套 reconstruction.Runtime/RunView，不新建 CQRS/物化视图/读取仓库，不机械删除全部旧字段 |
| 命令入口 | 保留一个请求仲裁 owner；把业务载荷校验、原意图映射、派发和权威查询按职责分开 | 中等收益、中高风险；防止增加命令时反复修改多段混杂 switch，原受理顺序必须受回归保护 | 不引入插件注册、通用 command bus、反射/DSL；不强迫三种命令流程使用同一模板 |
| Web 容器 | 提取连接生命周期、原请求对账 hooks；设置页和操作回执组件独立 | 中等收益；连接/取消/重放风险高于文件搬动，需要保留现有 StrictMode 与切换测试 | 不新增全局 store、第二套缓存/journal、通用控制器、SSE；不拆每个 JSX 块 |

三批可以分别审查。命令入口整理不依赖候选读取重构；Web 的结构拆分也不需要等待命令入口重写。推荐先固定候选 DTO 语义，再整理命令，再提取 Web，属于实施顺序，不增加无关前置。

## 2. 基线审计证据与保留部分

### 2.1 候选状态

- [taskrunner/projection.go](../../internal/taskrunner/projection.go) 的 projectCandidate 在有公开投影时将 completed 改写为 delivered；[query/runner_history.go](../../internal/product/query/runner_history.go) 的 CandidateSummary 则用 participation 原值覆盖。相同候选会得到不同 workflow_status。无投影时，详情的 workflow_status 未设置，历史却使用 SubmitState。
- [runs/state.ts](../../web/src/features/runs/state.ts) 用 queued/running/blocked 等状态名单推断参与关系；这不能表达 active 但尚无公开投影的候选，也把停止意图与执行观察混在一起。candidateState 先判断 execution_status=running，再判断 delivery_outcome=winner，因此已交付候选的迟滞 running 观察可能压过交付标签。
- [query/pages.go](../../internal/product/query/pages.go) 已有由 TaskDelivery.SelectedWorkflowRunID 产生 winner/loser 的规则；[SQLite delivery](../../internal/storage/sqlite/runner_result.go) 与 [withdrawal](../../internal/storage/sqlite/runner_stop.go) 已分别持久化 completed/abandoned 和停止义务。复用这些事实，不能让展示重新决定业务结算。
- [app/client.go](../../internal/app/client.go) 当前存在 RunnerSnapshot → taskrunner.Project → Runtime/RunView → query.CandidateView → DTO 的多次映射。但 Runtime 还服务于 checkpoint、宿主观察及决策可用性，整体替换会扩散到身份与持久化边界。当前证据只支持先收拢状态解释；不以减少 copy 行数作为重写依据。

以上描述审计基线；实现由跨视图和 Web 状态组合用例验证差异，不能当作当前代码仍有这些缺陷。

### 2.2 命令入口

[command_registry.go](../../internal/product/httpapi/command_registry.go) 已是一个实例内的请求仲裁 owner，request_records.go 是唯一文件记录 owner；应保留，不再造第二个中心。实际问题是 validateActionEnvelope、mapIntent、dispatch、matchesCommand/matchesControl 分散解释同一 action；answer_decision、discard_task 和控制载荷在正常路径多次解码、重复构造。

普通 Submit、[环境检查 submitEnvironment](https://github.com/erranliu/SteerLine/blob/7617cddc7efac5716c8c38bb63a0157a100acac0/internal/product/httpapi/environment_checks.go) 与 OpenNative 的顺序刻意不同：普通命令可持久化拒绝，环境检查先验证再登记，原生窗口必须先保存 outcome_unknown 才执行副作用。它们共用请求 ID 命名空间和锁，不等于必须共用受理流程。重复查询、注册后的重新读取、派发前的新鲜资格核验也不是可直接删除的重复代码。

现有 command_boundary、reconciliation、API 测试已覆盖并发同 ID、跨 action 冲突、注册窗口竞争、固定映射与权威回执。架构守卫有部分按 command_registry.go 文件名生效，拆文件时必须防止绕过。

### 2.3 Web 容器

[app.tsx](../../web/src/app/app.tsx) 同时包含 App 的 session/catalog/URL 选择、Workspace 的 Api 生命周期/snapshot/联网资格、journal 轮询和 mutation、Operations、SettingsPage 以及布局。值得隔离的是副作用所有权，不是 JSX 数量。

[queries.ts](../../web/src/shared/api/queries.ts)、[client.ts](../../web/src/shared/api/client.ts)、[journal.ts](../../web/src/shared/api/journal.ts) 已分别拥有缓存键/失效、原信封提交与恢复、浏览器持久化；[context.tsx](../../web/src/app/context.tsx) 已提供功能页接口。现有 app/project-switch 测试覆盖 StrictMode 预取、断网、旧实例回执、切换取消与多标签选择，继续使用，不重新建设客户端架构。

## 3. 批次 A：候选读取语义

### 3.1 事实来源

| 维度 | 真相源 | 约束 |
| --- | --- | --- |
| Task 状态、版本与依赖 | 同一读取事务中的 Task/DAG | 不从 Runner 成功推导 Task 关闭 |
| 候选身份、参与、提交与停止意图 | runnerstate.Candidate | active/abandoned/completed、SubmitState、StopRequested 分别保留 |
| 执行状态/阶段/问题/恢复能力 | Candidate.Projection 中的公开 Runner Run | 没有投影就没有原生执行结论，不从 caller 结束推导 stopped |
| winner/loser 与交付 | 当前 TaskDelivery 的所选候选及原绑定 | completed、PR URL 或 pr_ready 本身都不是交付证明 |
| 展示标签 | 产品投影与 Web 纯展示函数 | 不进入 Task/Runner 写入、调度、停止、恢复或权限判断 |

### 3.2 最小实现边界

1. 在现有 runnerstate 内提供必要的纯观察状态函数，建议 `ObservedStatus(Candidate)`：有 Projection 时返回其原 Status；否则返回 SubmitState。它不折叠 participation，不决定终态，不引入产品 DTO/存储依赖。两个生产转换共同调用；不另建读取模型包。
2. 为实际消费的 CandidateSummary、WorkflowSummary，以及中间 RunView/CandidateView 补齐 `participation`、`submission_status`、`stop_requested`。这些是当前候选事实，不能再从展示状态倒推。Go/Web 同批修改，当前版本不增加旧字段缺失时的兼容回退或双写路径。
3. `workflow_status` 统一表示上述观察状态，不再把 abandoned/completed/delivered 混入 Runner 的执行状态。`execution_status` 保留现有公开观察/无观察时提交状态的用途；参与与停止意图通过新字段表达。历史可以比详情少字段，但相同输入的公共字段必须相同。
4. Web participating 只判断明确 participation；业务操作还须满足原 Task/连接/版本/公开决策资格及停止意图门槛，不能因 active 就获得输入或恢复权限。未知观察不显示为已结束，也不能凭 active 自动启用回答/恢复。停止按钮仍由服务最终核验，不把前端提示变成业务权威。
5. 页面展示分别保留参与状态、执行观察和资源状态：abandoned 可显示“已退出参与”而原执行仍 running；带有效 winner 事实的详情以已交付为业务标签，同时保留未停止的原资源观察。历史没有交付上下文时只显示 completed 参与事实，不能补造 winner；不为统一标签追加另一组数据库查询。
6. 等待合并、必答问题、可选业务反馈仍使用公开 Run/Decision 事实及现有业务门槛。PR ready 不自动交付，可选反馈不变成阻塞回答。未知状态保留原值与现有保守操作门槛，不新建 Runner 状态枚举翻译层。
7. 现有 SlotOccupied/ReservedTask、checkpoint、Decision/InputForAnswer、TaskDelivery 和停止事务的资格/写入规则保持。若某读取使用者依赖混合的 workflow_status 判资格，改为消费已有原事实以保持原门槛，不能让展示重构放宽写入资格。所有调用点须列入实施引用审计。

API 命名中的 RunID/ExecutionID、WorkflowSummary 等暂不整体改名；Host/Agent 在候选与 profile 中含义不同，不强行通过一个万能转换器统一。保留有消费者的旧形状，后续若要整体替换另行证明收益。

### 3.3 最小组合矩阵

| 输入事实 | 必须保留的输出/约束 |
| --- | --- |
| active + pending/unknown + 无 Projection | 参与仍 active；提交状态准确，详情/历史公共观察状态一致；不造问题或停止证明 |
| active + pr_ready + 开放 Task | 待合并、仍参与；原可选反馈资格不变 |
| abandoned + 最后观察 running | 退出参与与 running 同时可见；不得显示资源已停止 |
| completed + 有原 TaskDelivery winner + 最后观察 running | 详情交付标签不被 running 覆盖；资源仍按原观察展示 |
| completed + 无交付上下文的历史 | completed 参与事实，不能自行宣称 winner |
| StopRequested + 迟滞投影/异步问题 | 不新放行业务输入；原请求/原执行绑定不改 |
| 不认识的 Runner 状态、已关闭/丢弃 Task | 保留未知原值；业务资格不因状态映射获得放行 |

## 4. 批次 B：命令职责收拢

### 4.1 保留的权威与顺序

- commandRegistry 仍唯一持有仲裁锁、requestRecordAccess 与命令端口；fileRequestRecordOwner 仍唯一维护原记录格式/原子注册。命令 family 不新增锁、记录路径、SQL/HTTP client 或具体 product client。
- 普通命令、环境检查、native viewer 保留各自当前 orchestrator 和先后顺序。共用原信封比较、摘要、响应组装等真正相同的纯原语，不引入控制流程的模式开关或通用 pipeline。
- 原同 ID 的权威读取/冲突判断优先；已登记 mapped intent 不重算、不改绑。首次有效请求的载荷可一次严格解析并产生 typed mapped intent，但仍保留登记后重新读取、显示版本/当前能力/就绪核验及必要的首次派发前资格复核。
- 正常新请求不得在 dispatch 再构造另一个独立的 DecisionInput/DiscardInput/ControlInput；使用已登记的明确映射。不得顺便删除现存“登记存在、意图尚未保存”等分支：逐一以当前格式可达性和原回执证明审计，保留必要未知处理，不能把未知当形式不存在后重放。
- 读取原请求不派发新的业务/native 副作用；原观察/查询路径保持，权威不存在、查询失败、未知结果严格区分。原 request_id/envelope/body、requested_by、project/instance、expected revisions、native/check 的原绑定与返回码保持；已退役 stop_service 提交继续拒绝，历史原回执保持只读。

### 4.2 目标代码组织

建议沿现有 httpapi 包拆成普通受理/仲裁、业务意图（Task/Decision/Control）、权威查询与匹配、native viewer、环境检查五处。可命名 command_registry.go、command_intents.go、command_authority.go、command_native.go、environment_checks.go；文件名不是硬验收门槛，不要求每个 action 独立文件或 handler 类型。

职责约束比文件数量重要：增加一个业务载荷规则，应在一个 family 的解析/校验与映射处完成；原映射派发和相应权威匹配显式可查，不再在正常 dispatch 路径重新解码同一 payload。业务边界验证可以重复，不能为了“一次解析”缓存会过期的资格。

环境检查当前已拆专用文件，本次不重新实现 Runner inspection/schema/cleanup；native viewer 的写入未知回执屏障也不并入普通命令派发。架构守卫覆盖拆出的所有 production helper；增加一个移到新文件的违规夹具，证明改名不能绕过原 owner 限制。

## 5. 批次 C：Web 容器

在现有目录中提取少量有明确生命周期的函数，名称可按实际类型微调：

| 位置/出口 | 独占职责 | 不接管 |
| --- | --- | --- |
| app/use-project-connection.ts：应用项目连接 hook | session/catalog/identity 查询、URL 与 home 偏好选择；保留原深链接/页面类型/多标签规则 | 不发业务命令，不创建第二个 QueryClient |
| 同层 Workspace 连接 hook | Api memo 与 activate/deactivate、项目/实例查询取消、snapshot、online、预取和手动 refresh 组合 | 不重建 Api 的重放、journal 或请求锁规则 |
| app/use-original-requests.ts | 原 journal 查询、单个自动对账循环、手动对账/发送结果提示、原范围失效 | 不做新请求 ID、重定向旧信封或新增重试策略 |
| features/settings/settings.tsx、app/operations.tsx | 设置与回执的现有渲染；操作组件调用 hook 提供的回调 | 不自行启动第二个对账循环或隐藏的独立提交 |
| app.tsx | App/Workspace 组合、路由与布局、本地折叠/搜索等简单 UI 状态 | 不保留被提取副作用的另一份实现 |

保留 WorkspaceContext 的消费接口、Api 客户端、journal 记录格式、home/project/instance query keys、轮询周期、缓存 staleTime、懒加载和失效范围。设置页读共享 presentation，不绕开缓存。旧实例回执只更新原实例范围；项目切换、离线、卸载均终止对应循环/请求，不把旧结果装入新实例缓存。StrictMode 的建立/清理/重建仍正确，不通过永久 ref 标记跳过应有的重新激活。

自动对账和手动“查询原请求”继续调用同一 Api.reconcile，由 Api 的既有资格决定能否以原信封重发；hook 不推导自己的“查询失败即不存在”，也不声称所有 unknown 都永不重发。请求正文持久化先于 POST、形式不存在才按原资格重发、native 未知不重开等既有规则不变。

## 6. 不做与风险控制

不新增数据库/schema/migration、额外 Runner 请求/轮询、恢复机制、业务动作、路由、用户设置或外部依赖。展示字段变化限于上述明确语义，不能夹带 UI 重设计。读取不新增 Task/Runner 写入；读取模型不作为调度/交付或权限权威。

保留三条可核查收益线：同一候选公共状态只用一条规则；原请求意图只有一份有效映射并由唯一 owner 仲裁；每类前端生命周期副作用只有一个持有者。若某一步只能改变文件位置而不能达到这些条件，应缩减或取消，不能以类型/文件/行数计收益。

## 7. 验收要求

| ID | 要求 |
| --- | --- |
| RCW-A01 | 同一候选的详情/历史公共事实一致；上述七组组合覆盖，participation/SubmitState/Runner/TaskDelivery 不互相冒充，交付不被迟滞 running 覆盖 |
| RCW-A02 | 各转换无外部副作用、无新读取事务；原 checkpoint/身份/决策资格、TaskDelivery 和停止义务保持；Go/Web DTO 消费者同时更新且无旧字段回退 |
| RCW-A03 | 普通/环境/native 三种受理顺序、并发同 ID/跨 action 仲裁、原映射/权威匹配/未知处理和历史只读保证保持 |
| RCW-A04 | 拆分后结构守卫覆盖 helper，正常派发不重建载荷意图，无第二个锁/记录 owner 或具体资源依赖 |
| RCW-A05 | Web 连接/原请求副作用归口，StrictMode、切换/离线/卸载取消、原 home/实例失效范围与 journal/重放资格保持 |
| RCW-A06 | 设置及操作组件搬动后原路由/缓存/回执和相关功能测试、TypeScript/Vite build 通过；选定受控命令链的隔离边界通过并清理 |

自动项的最小命令、环境、时点、通过条件和证据位置由 Plan 映射。统一 worker 规则仍归 [worker-lifecycle](../../docs/product-design/worker-lifecycle.md)：关联 W12/W17 的展示与原请求边界及 V06/V08；不改变任何 W/E 节点、异常分类或原生恢复/停止矩阵。当前仅审计和设计，不以文档通过声称产品验收完成。
