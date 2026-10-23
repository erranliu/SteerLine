# 产品读取、命令入口与 Web 容器重构 Plan

配套 [Spec](../spec/product-read-command-web-refactor.spec.md) 与[专属验收](../testing/acceptance/product-read-command-web-refactor.md)。2026-10-16 已获用户实施与提 PR 授权；A/B/C 按本文实施，实际验证与剩余人工项归专属验收。本 Plan 定义交付安排，不维护实时结果。

## 1. 实施顺序与依赖

采用三个可单独审查的批次，优先 A，其次 B、C。A 只要求其 Go/Web 字段消费同批完成；B 与 A 无实现依赖；C 的结构提取以当时已稳定的 DTO 为输入，不必须等待 B。各批都有可核验收益，不以累计拆文件作为完成。

### A. 候选读取一致性

1. 固定 Spec 的七组输入/预期，逐一核对 projection.go、runner_history.go、productServicePage、query.Candidate/ServicePage、TaskDetail/history DTO 和 Web 状态/过滤/答案/停止提示的所有消费者。检索 workflow_status、execution_status、participation、DeliveryOutcome、RunView；记录控制/资格调用者，避免漏掉同名字段不同含义。
2. 在 runnerstate 内增加小型纯 ObservedStatus 函数，明确有/无 Projection 的来源；无 I/O、不引入 api/SQL。详情与历史共用它，去掉由 caller 参与状态覆盖公开执行状态的混合规则。
3. 沿已有 Candidate → RunView → CandidateView → CandidateSummary 及历史 WorkflowSummary 传递 participation/submission_status/stop_requested；更新 Web 类型、fixtures 与所有展示消费者。保留当前公共形状和身份字段，不替换 Runtime/读取仓库，不加旧字段缺失回退。
4. Web 从显式事实判断参与；交付标签、退出参与、原执行和资源观察分开。只调整 Spec 明确的展示差异，保持等待合并/可选反馈/问题、Task 关闭/丢弃、停止意图和未知时的保守资格；权威业务判定仍在原服务。历史没有交付上下文时不补造 winner，不增加查询。
5. 新增 app owner 的 `TestCandidateReadFactsAcrossViews`（计划名称）：同一输入经过真实转换链与历史转换，核对公共字段一致、原事实无篡改和 Spec 的展示差异；轻量表覆盖七组而非完整笛卡尔积。现有 SQLite 一致事务、唯一交付/原绑定、actor checkpoint 与输入测试保持。前端状态、列表、答案和任务详情选择性回归，新增用例优先补到已有文件。
6. 验收 [RCW-A01](../testing/acceptance/product-read-command-web-refactor.md#rcw-a01)/[RCW-A02](../testing/acceptance/product-read-command-web-refactor.md#rcw-a02) 后同步读取/字段说明与架构。候选新的纯规则须有实际消费者；不得把 UI 投影用于 SlotOccupied、checkpoint、调度、停止事务或输入资格。

### B. 命令职责整理

1. 先写三条当前受理顺序清单并链接现有测试：普通、环境检查、native viewer。列明共用锁/ID、读取旧记录与权威、首次映射、注册/重读、资格复核、派发/副作用、未知保存和最终响应的顺序；把有意义的例外保留下来。
2. 提取权威查询/匹配与 native 方法，再移动 Task/Decision/Control 的解析、校验和意图映射。沿同一个 commandRegistry、records owner 和端口，不引入 handler 容器/动态注册。
3. 新有效普通请求在首次映射阶段解析一次载荷并保存明确 intent；正常派发消费记录中固定的类型和值，不重新构造独立 payload。登记后重读、原图/版本/能力、决策和就绪资格复核保持；解析结果不替代动态资格。对已有记录和不完整意图沿当前可达性保留未知/权威处理，不能借 refactor 增加自动重放。
4. 环境和 native 保留专用流程及相同的原信封比较规则；不要求它们改变“何时登记拒绝/未知”以迁就普通命令。retired stop_service 提交拒绝/历史只读路径继续受原测试保护。
5. 扩大已有架构守卫的实际覆盖至拆出的生产 helper，而非只检查 command_registry.go 文件名；增加一个新文件下偷读记录/另建 owner 的反例夹具，不新增通用静态分析框架或按行数的门禁。
6. 跑 [RCW-A03](../testing/acceptance/product-read-command-web-refactor.md#rcw-a03)/[RCW-A04](../testing/acceptance/product-read-command-web-refactor.md#rcw-a04) 的完整受影响 transport 测试；只有被替代且已无当前契约覆盖价值的专属测试才删除。保持现有注册竞争、正式消费者抢先、同 ID/跨 action、未写 receipt、权威读失败等断言，不以更弱的 helper mock 测试替代。

### C. Web 副作用归口

1. 先提取 SettingsPage 和 Operations 渲染，保留 App/Workspace 对外导出及 WorkspaceContext 接口，设置路由/共享 presentation 和原按钮文案不变。单纯搬动不新增同构测试。
2. 提取应用项目连接与 Workspace 连接 hooks：各自持有 session/catalog/identity 与 Api lifetime/snapshot/online。保留 URL、home 偏好、多标签独立、懒加载、预取及缓存键；建立/清理顺序沿现有 layout effect，而非改成渲染时 activate。
3. 提取原请求 hook，独占 journal query、自动对账 timer/controller、错误请求提示与手动对账/发送回调；Operations 消费回调，原 effect/mutation 删除。保留 Api 的原信封和重放资格，不把网络失败解释为请求不存在。
4. 利用现有 mounted App/Workspace 测试证明 StrictMode、prefetch/observer、离线/卸载/切换、原实例回执、journal 刷新期间单轮对账。需要补的断言放这些 fixture 中，不为每个 hook 写一套实现镜像测试。
5. 按 [RCW-A05](../testing/acceptance/product-read-command-web-refactor.md#rcw-a05)/[RCW-A06](../testing/acceptance/product-read-command-web-refactor.md#rcw-a06) 运行前端定向与 build；最终执行一次固定 L1、编译及选定 L4 受控命令边界，登记结果。用户可见语义改动仅限 A 的明确状态修正。

## 2. 测试计划

测试全部使用隔离临时资源或 jsdom/fake-indexeddb。原始 Go/E2E 证据用 test-feedback 新目录；Web 输出用 `T = $env:TEMP 下本次新建的空证据目录`，事先创建并用 Tee-Object 保存 stdout/stderr/exit code；在专属 acceptance 登记实际路径、source commit/dirty、命令/选择、通过条件及清理。无有效工具/依赖时先按仓库依赖入口补齐，不记为通过。

| 验收项 | 级别 / 组或场景 | 环境与命令 | 时点、通过条件、证据 |
| --- | --- | --- | --- |
| [RCW-A01](../testing/acceptance/product-read-command-web-refactor.md#rcw-a01)/[RCW-A02](../testing/acceptance/product-read-command-web-refactor.md#rcw-a02) | L1 定向 / 读取纯规则与原输入 | `go run ./cmd/test-feedback -- ./internal/taskrunner ./internal/product/query -count=1` | A 完成时；两包实跑，含未知/原问题与回复资格，0 失败；runnerstate 新纯函数由下行 app 跨视图实际消费测试覆盖，不列空包/零匹配充数，summary 登记 |
| [RCW-A01](../testing/acceptance/product-read-command-web-refactor.md#rcw-a01)/[RCW-A02](../testing/acceptance/product-read-command-web-refactor.md#rcw-a02) | L1 定向 / 跨 owner 投影与事务 | `go run ./cmd/test-feedback -- ./internal/app -run '^TestCandidateReadFactsAcrossViews$' -count=1`；`go run ./cmd/test-feedback -- ./internal/storage/sqlite -run '^(TestRunnerPublicReadsKeepOriginalFinalAndCallerBinding\|TestRunnerApplicationSnapshotSharesTransactionAcrossWriterCommit\|TestRunnerTaskDeliveryWithoutWorkflowReady\|TestRunnerActorUsesHostScheduler)$' -count=1` | A 完成时；先补可用的跨视图用例再执行，非零匹配；七组事实、同一事务、交付和 checkpoint 保持，summary/go-test.jsonl 登记 |
| [RCW-A01](../testing/acceptance/product-read-command-web-refactor.md#rcw-a01)/[RCW-A02](../testing/acceptance/product-read-command-web-refactor.md#rcw-a02)/[RCW-A06](../testing/acceptance/product-read-command-web-refactor.md#rcw-a06) | Web / 候选展示 | `npm --prefix web test -- src/features/runs/state.test.ts src/features/runs/runs.test.tsx src/features/runs/answer.test.tsx src/features/tasks/task-detail.test.tsx` | A Go/Web 同步后；参与与执行分开、winner 优先标签且原 running 不丢、未知不放宽、原过滤和答案有效；T 下保存输出 |
| [RCW-A03](../testing/acceptance/product-read-command-web-refactor.md#rcw-a03)/[RCW-A04](../testing/acceptance/product-read-command-web-refactor.md#rcw-a04) | L1 定向 / 请求仲裁及 transport 边界 | `go run ./cmd/test-feedback -- ./internal/product/httpapi ./internal/product/applicationhttp ./internal/testkit/boundary -count=1` | B 完成时；受理顺序、不可变映射/原 ID、三类冲突与未知、历史只读、移到新文件的违规夹具均有效；summary 与组日志 |
| [RCW-A05](../testing/acceptance/product-read-command-web-refactor.md#rcw-a05)/[RCW-A06](../testing/acceptance/product-read-command-web-refactor.md#rcw-a06) | Web / 连接、缓存及原请求 | `npm --prefix web test -- src/app/app.test.tsx src/app/project-switch.test.tsx src/shared/api/client.test.ts src/shared/api/queries.test.ts` | C 完成时；现有 mounted 测试保留并补必要竞态，StrictMode/单轮/切换/原实例等断言通过，0 skip；T 输出 |
| [RCW-A01](../testing/acceptance/product-read-command-web-refactor.md#rcw-a01) / [RCW-A02](../testing/acceptance/product-read-command-web-refactor.md#rcw-a02) / [RCW-A03](../testing/acceptance/product-read-command-web-refactor.md#rcw-a03) / [RCW-A04](../testing/acceptance/product-read-command-web-refactor.md#rcw-a04) / [RCW-A05](../testing/acceptance/product-read-command-web-refactor.md#rcw-a05) / [RCW-A06](../testing/acceptance/product-read-command-web-refactor.md#rcw-a06) | 最终固定 L1 / 编译 | `go run ./cmd/test-feedback -level=L1`；`go build ./...`；`go run ./cmd/test-feedback -build-only` | 三批集成后一次；固定集合 complete/passed、0 失败/跳过；把跨视图新用例纳入实际 app owner 选择，默认测试编译成功；summary/编译回执 |
| [RCW-A03](../testing/acceptance/product-read-command-web-refactor.md#rcw-a03)/[RCW-A05](../testing/acceptance/product-read-command-web-refactor.md#rcw-a05)/[RCW-A06](../testing/acceptance/product-read-command-web-refactor.md#rcw-a06) | L4 controlled / `runtime.dual-home-isolation` | `./scripts/test-runtime-isolation.ps1 -Level L4 -Scenario runtime.dual-home-isolation` | 最终集成后；新独立隔离现场，不同 home 的相同 ID 重放回执/修订保持、另一 home 不变、无多余候选，passed/cleanup_complete=true；summary 与 scenarios/runtime.dual-home-isolation/scenario.json |
| [RCW-A01](../testing/acceptance/product-read-command-web-refactor.md#rcw-a01)/[RCW-A05](../testing/acceptance/product-read-command-web-refactor.md#rcw-a05)/[RCW-A06](../testing/acceptance/product-read-command-web-refactor.md#rcw-a06) | Web 类型与产物 | `npm --prefix web run build` | A 同步和最终 C 集成后；tsc/Vite 成功，无旧 DTO 回退，产物不提交；T build 输出 |
| 全项文档 | 静态 | `python scripts/check_doc_links.py`；`git diff --check` | 文档同步后，0 错误；acceptance 记录 |

测试职责：runnerstate 若仍只有无独立测试的值/轻函数，通过 app 的跨视图实际消费测试验证，不额外创建无意义空测试。实施命令需按实际包测试分布收敛，但不能删掉任何验收项覆盖；各带 -run 的命令必须实际匹配。新增跨视图用例、架构守卫反例及 mounted 竞态补充均列在本次实现步骤，不留“以后补验”。

本次不选 Full、真实 L2/live、完整 L3/L4、原生执行矩阵或默认浏览器自动化：没有更改原生执行、业务交付或页面布局；既有公开 CLI/事务/HTTP 与 mounted Web 已覆盖这些变化，另用一个真实 SteerLine 命令的 controlled L4 边界核对重放和隔离。没有新增场景 ID，不改 manifest/分级含义。没有 OS 专属实现变更，不默认增加 Linux 运行专项；若实施引入平台差异再明确补最小编译/场景。

人工仅可后置确认设置页导航、参与/交付与资源标签是否易读，记录 RCW-M01；无需人工前置，不阻塞无关批次。若需真实浏览器调试，先 `scripts/new-debug-release.ps1` 新建 C:/SteerlineDebug 包，AI 显式 simulate；不能复用安装目录或已有冻结包。

## 3. 完成判据

- A：公共事实/标签来源清楚，详情与历史不再各自解释 completed/无观察状态；前端不从 Runner 状态名单猜 participation，权威写入规则未变。
- B：请求 owner/记录/锁仍唯一，正常派发不再重建独立意图；命令 family 可定位，所有顺序/原绑定与例外保留，守卫不能因拆文件失效。
- C：每类连接/对账副作用只由对应 hook 持有，app.tsx 组合组件；原客户端和查询基础设施继续复用，没有额外循环和缓存。
- 全部自动项实跑并登记；设计静态通过、编译、受控链与人工体验分别陈述，不相互替代。
