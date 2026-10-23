# SteerLine / SteerRunner 协作与 Workflow 接入 Plan

日期：2026-10-17。状态：实施方案；用户已授权当前聊天直接实施。配套 [Spec](../spec/runner-collaboration.spec.md) 与[专属验收](../testing/acceptance/runner-collaboration.md)。不创建执行聊天/worktree、Task ID 或 roadmap 条目；本次不部署安装环境。本 Plan 维护步骤和测试安排，实时结果只写 acceptance。

## 1. 范围与依赖

先固定公开字段和 schema 约定，Runner 契约实现与 SteerLine 配置/冻结、投影/Web 可以分别推进；实际 Runner 构建只作为跨仓联验和交付前置，不是全部 caller 开发的开工门槛。B/C 的类型按已固定约定协调，D 用同一实际构建联验。源码只读梳理不代表 Runner 已发布可接入的最终 EXE；普通与审计流程各自选择，不把审计变成全局必经门槛。

| 批次 | 仓库 / 交付物 | 开发前置 / 联验与交付前置 |
| --- | --- | --- |
| A | Runner：组合 schema、仓库内正反契约测试、固定构建和定向证据 | 开发：复核基线并固定契约；交付：实际 EXE/资源身份与测试证据 |
| B | SteerLine：多 workflow 客户端、参数摘要、候选提交快照与配置守卫 | 开发：固定公开契约，可用公开 CLI fixture；联验：A 实际构建 |
| C | SteerLine：公开投影、产品 DTO、详情/历史/Web、环境检查隔离 | 开发：固定公开 Run 和 B 的快照字段约定；可用 fixture；联验：A 构建与 B 接线 |
| D | SteerLine：真实配置契约核对、审计闭环及产品查询、正式文档校准 | I1：A/B 可用；N1：A–C、I1/C1 及驱动/原生环境可用 |

Runner 实施在其仓库维护本次 schema 增量和测试资产的专属验收并回链本文；SteerLine 不接管 Runner 的历史 WA/SA/SREC 项。缺少支持矩阵信息时补同一公开契约，不让 caller 增加平行规则。

开工交接固定：workflows/组合 schema 的形状、版本轴、audit_model 与公开 Run 字段/来源语义，记录约定所在文档 commit/path；真实交接另增加：Runner 实际源码 commit/dirty、EXE/build.json、SHA、version、protocol/resource_digest 及对应定向结果/验收路径。SteerLine 再固定自身测试源码版本。契约调整时先同步约定及受影响 fixture，不把未实现假数据当真实能力；测试样例留各仓测试中，不增加生产样例传输格式。相同 version 字符串不能替代程序身份。

## 2. 实施步骤

### A. 固定 Runner 发布契约

1. 核对所选版本的 `internal/contracts/run.go`、`internal/resources/resources.go`、`settings.schema.json`、workflow 目录、RunView 和生产受理路径。建立字段消费清单：workflows、workflow schema、audit_model、audit results/source、materials、requested model/effort。
2. 在 `configuration_schemas` 增加自包含 `run_configuration` schema，根为 `{execution,workflow}`；同源描述普通/审计、adapter/target、显式执行/审计模型、覆盖步骤和 settings。复用内置 workflow 选择来源与生产校验，不能只在测试中实现组合规则。schema 不从文件/网络追外部引用。
3. 正反样例仅作为 Runner 仓库内测试资产：至少普通合法、审计合法/effort 省略、普通带审计模型、审计缺模型、审计非 local/非支持 adapter、非法 prompt step。用占位模型核对 schema 与生产纯配置准入同判，不调用 freeze/Git，不运行模型。capabilities 只增加必需的组合 schema，不发布样例集合、样例加载器或独立样例协议。
4. 现有 submit/inspect/CLI v1、审计路由和原生执行保持。受理仅冻结参数/材料/模板；逐轮最终输入在发送前原子保存，恢复不重渲染；动态身份沿原生命周期取证。复用既有冻结恢复定向测试核对该语义，不为文档澄清重构运行存储。若有效公开字段尚未稳定，在 Runner 原 owner 补齐，不返回私有快照/codec。新增契约测试 `TestSRPublishedRunConfiguration`后构建，保留 build.json、EXE SHA 与 capabilities 原文。不部署或替换已有运行目录。
5. 补齐公开 `--expected-instance-id` 与 `capabilities.instance_binding.expected_instance_id`；绑定业务/查询经发现端与服务受理端核验，禁止隐式 Ensure 启动替代服务。CLI 诊断与原 service stop 正文身份保持各自职责。R3 用无原生 L1 覆盖不匹配、请求抵达服务后的核验及绑定调用零启动；真实 I1 核验匹配查询与 stale_instance。

### B. SteerLine 配置、契约和冻结

1. `runnercontract` 增补 workflow.audit_model 和实际需要的公开 Run 字段；`runnerclient.Catalog` 消费 workflows 与 workflow/组合 schema，沿无外部 schema loader 验证。目录重复、未知版本、缺必需能力/资源身份、组合非法明确失败，不读旧单 workflow 作 fallback。A 实际 EXE 未交接时使用已固定公开契约的 CLI fixture 实施和定向测试，不增加运行时样例消费者。
2. `RunnerExecutionConfig` 保留 version 3 与文件形状，内部用“部署配置/预设目录/执行参数”说明职责。执行语义不在 app 补默认值。提供普通/审计两份示例，审计仅明确配置时选用，不修改默认方案；调试生成器保持唯一模板源和冻结规则，新增参数仅在必要时提供并纳入模板生成。
3. 将现有 profile digest 改为 execution + workflow 的严格、确定性 JSON SHA-256；排除 ID/name、保留缺省区别；排序嵌套 settings 对象键。加载原始配置时即拒绝重复键，避免先解为 struct/map 后丢失重复信息。沿原 ID + digest 校验，不另增 profile revision。整个配置文件摘要仍按原字节且保留准入职责。
4. 候选预留事务同时保存原预设名称/ID/参数摘要、完整提交信封和固定 base/材料，复用项目级持久绑定记录原 EXE/资源身份和 state-dir。服务 instance_id 在单个 ProjectRuntime 生命周期内固定，不将首次实例永久写死到每个 Run；仅显式重启 SteerLine、重建 ProjectRuntime 时建立新的本次绑定。新运行时初始化将同一核验结果传给准入闭包、查询/控制客户端、停服与结算闭包；不在旧运行时修改共享 info 或替换部分闭包，原停止请求/回执仍属旧实例。复用 candidate/outbox/宿主记录，读取不补名或重拼原请求，持久格式调整同步来源/拒绝测试、不加迁移。
5. 按 Spec 4.2 实现三种准入：同构建换预设只影响显式重启后新候选；单独 Runner 换代而旧 ProjectRuntime 尚存时拒绝新业务输入并提示显式重启 SteerLine，保留原绑定，不因 ready/轮询自动接纳。仅在 SteerLine 显式重启后，新 ProjectRuntime 核对目录/构建、原 Run/input_digest 或原 submit request_id，初始化新实例绑定。更换构建前用原构建结算全部执行/检查/请求；未知/活动对象或原构建不可用则拒绝，新构建仅进入无旧执行状态新环境。无运行中切换入口、多构建路由或旧目录接管。
6. 新增最小测试组 `TestRunnerPresetFreeze`、`TestRunnerWorkflowContract`，放到实际 app/taskrunner/runnerclient owner，覆盖摘要/信封/事务/非法契约及三类变化。U1 中新增以下两个计划 subtest，使用公开 CLI fixture 和真实运行时接线验证；不把修改旧运行时的 info 当成显式重启：
   - `reject_replacement_in_live_runtime`：原 ProjectRuntime 绑定实例 A，fixture 改为同 EXE/资源/state-dir 的实例 B；旧运行时拒绝新候选、回答/反馈/resume 和检查等新业务输入，不修改准入/客户端/停服/结算的 A 绑定，零新业务 CLI 派发，并给出显式重启提示。
   - `accept_replacement_after_explicit_restart`：结束旧运行时，沿显式重启 SteerLine 的既有宿主路径重建 ProjectRuntime，再核验接纳 B；准入/查询控制/停服结算均使用 B，原 Run/input_digest 和已存输入保持，零自动旧业务输入。重放 A 的原停止请求，断言 B 仍存活且未被当作结算；B 的停止使用其独立新请求并核对 B 的最终证明。
7. U1 额外验证状态预检与实际派发之间换代时拒绝，显式重启遇公开原 Run 摘要冲突时不上线；U3 用 `TestSteerRunnerRestartOriginalBinding` 覆盖原 Run/请求冲突、pending 撤销及 unknown 查无原请求仍零重发。初始化先完成只读原事实核验，再开放准入，不借 Step 发出停止/恢复/刷新操作。上述两断言与既有崩溃观察/对账、pending/unknown 请求、活动检查、未核实停止阻断换构建等反例同组执行；原恢复未知事实保留，配置变化不拦原绑定的观察/停止，冷启动 pin 冲突明确阻断。`runnerbridge` 不生成 prompt，fixture 不重建审计状态机。无需新增 E2E 场景或真实模型运行。

### C. 产品查询、Web 与检查

1. 从同一 candidate 原提交和公开 Run 投影生成详情/历史；字段贯穿 runnerstate → taskrunner/product query → API → Web。名称和预设请求参数来自提交快照，阶段/audit/requested model 来自绑定 Runner；保留 revision/input_digest/PR 身份围栏。修正当前 WorkflowVersion 填 CLI protocol 的混用，workflow ID/version 从冻结输入取，CLI protocol 保留独立字段；测试普通/审计均核对这两个版本轴。
2. 方案下拉及设置页展示 workflow ID/version、请求模型、审计模型、权限模式；缺省写“由 Runner 决定”。详情区分请求和公开观察，展示当前有效审计结果/来源、findings、failure、repair_count；不判断可发布、不推测实际原生型号、不添加流程控制按钮。
3. 普通流程无 audit 仍保持普通显示；未知/旧观察不显示审计通过；历史使用原预设名称，旧报告/兄弟 Run 不合并到当前结果。`WorkflowRunID` 等历史字段只补映射说明，本次不改全体 API 身份。
4. 环境检查从预设深拷贝 execution 后显式应用该次 access_mode，再持久化完整原检查请求；workflow/audit_model 不传入检查，不污染原预设。保留自定义 execution 与现有报告/cleanup 分离，不将历史检查作为业务准入凭证。
5. 增补 `TestRunnerWorkflowReadProjection` / `TestRunnerCheckPresetIsolation`和现有 Web 定向测试，覆盖 audit pass/failure/不存在、模型切换请求值、冻结名称、来源身份、未知与控制资格、共享 map 不被检查改写。

### D. 接线、验收与文档收口

1. 在 `tests/integration` 增加 `TestRunnerPublishedWorkflowContract`，用独立 build tag `runnercontract` 显式选择：显式 `STEERRUNNER_EXE` 和本次独立 state-dir，固定实际 EXE，启动空闲服务读取真实 workflows/schema，由 caller catalog 对本仓少量公开配置用例验证，核对嵌套 audit_model/effort 的输入编码。零模型/PR，按原实例证明停止；缺 EXE 必须失败。I1 只证明配置发现、schema 校验和提交字段编码，不声称验证实际 Run 的 audit/materials/轮次模型；后者归 D3/N1。
2. 扩展既有 live driver 支持传入完整审计预设及两种显式模型，增加 `workflow.local-codex-audited` / L3 live / `TestWorkflowLocalCodexAudited`。同步 `tests/e2e/suites.json`、选择校验、驱动映射、E2E 可读清单和证据校验；规划文档不提前登记为可运行。此场景不作为现有 L2 的别名，也不默认加入完整 L2。
3. 新场景在分级隔离夹具内准备 SteerlineTest 专属 base/head/任务，从 SteerLine 正式入口选择审计预设，走 Runner/Codex 实施、报告、两项审计、发布、真实 PR 合并、唯一交付和后继 ready。在同一 Run 的实施、审计及发布时点，经 SteerLine 正式详情查询核对 workflow ID/version、materials 的固定来源/摘要、当前轮次 requested_model/effort；两项审计完成后核对 audit 的模型、结果/来源/repair_count。经正式历史查询核对其预设/workflow 摘要与候选/Runner Run 关联，历史不必新增完整材料/审计正文。记录实际 revision 与原 CLI inspect 的公开投影作为对照，保留产品响应和原候选记录；不能绕开 caller 直接查 Runner 后宣称产品字段接通。合理轮询但不新增模型轮次。驱动不代写业务实现、commit/push/PR，不强迫模型制造 finding。修复预算/故障矩阵仍在 Runner。
4. 现有 controlled `smoke.delivery` 作为业务交付回归；新 L3 验证新增 workflow 的真实产品接线。真实链需要目标构建的执行权限/登录就绪，失败记录真实缺口，不降低为模拟成功。停止/清理按原资源和 PR/ref SHA 核对，失败保留诊断。
5. 修正 `docs/architecture.md`、`architecture/current-code-architecture.md`、`product-design/execution-profiles.md`、`product-design/worker-lifecycle.md`、`product-design/README.md`、`integration/steerrunner.md` 及数据模型来源说明：当前 workflow owner/版本、四种数据、摘要职责、短目录和身份映射。旧 development/v3/worker-flow-v2 只在其历史/Task 契约语境出现，不再写作 SteerLine 当前执行引擎。冻结 commit 的历史证据不改写。实际测试结果登记本任务验收；全局正文只陈述本次真正实现的契约。

## 3. 测试计划

采用[通用测试规范](../../SteerlineProjectGuideDoc/testing-architecture.md)、[本仓入口](../testing.md)和 [E2E 清单规则](../../tests/e2e/README.md)。下列入口随实现补齐，实际结果只登记 acceptance；不得用零匹配、skip、编译或旧构建证据代替通过。

证据：每次用新的临时空目录，Go 默认 test-feedback 目录；Web/静态/契约输出另存该次子目录。统一在[专属验收的证据登记](../testing/acceptance/runner-collaboration.md#证据登记)记录路径、两仓源码、EXE/资源摘要、命令与实际命中数、退出码、失败和清理；不提交大日志/秘密。下表编号是本 Plan 测试组，不自动成为 E2E 场景 ID。

| 验收项链接 | 级别 / 组或场景 | 环境 / 命令 | 时点与通过条件 / 证据 |
| --- | --- | --- | --- |
| [RCB-A01](../testing/acceptance/runner-collaboration.md#rcb-a01) | 静态 D0 / 文档与 owner 审查 | 本仓；`python scripts/check_doc_links.py`、`git diff --check`；按 Spec 所有权表核对实际引用/入口 | 规划交付和 D 收口；链接/格式零错、逐项定位 owner，未实现处标规划；D0 输出和审查清单 |
| [RCB-A03](../testing/acceptance/runner-collaboration.md#rcb-a03) | Runner L1 R1 / 公开配置契约 | Runner 隔离 unit；`go test ./internal/resources -run '^TestSRPublishedRunConfiguration$' -count=1` | A 完成；schema/生产纯准入/样例同判，合法和各非法类均命中，零 skip；Runner 专属验收路径回链 |
| [RCB-A02](../testing/acceptance/runner-collaboration.md#rcb-a02)、[RCB-A03](../testing/acceptance/runner-collaboration.md#rcb-a03) | Runner L1 R3 / 实际实例围栏 | Runner 隔离 unit；`go test ./internal/cli ./internal/host ./internal/resources ./internal/contracts -count=1` | A 完成；预期实例在请求接收端核验、绑定检查零隐式启动、普通模型首尾空白/空白串/换行与生产同判；Runner 专属验收登记 |
| [RCB-A02](../testing/acceptance/runner-collaboration.md#rcb-a02) | Runner L1 R2 / 已有逐轮冻结恢复 | Runner 隔离 unit；`go test ./internal/application -run '^TestSRFrozenInputRestoration$' -count=1`（已有） | A 交接时；既有逐轮 prompt/model/来源保存及恢复不重渲染通过，先核对断言实际覆盖后登记；不足仅在原 owner 补定向断言，不重建 caller 矩阵；Runner 原输出回链 |
| [RCB-A02](../testing/acceptance/runner-collaboration.md#rcb-a02)、[RCB-A03](../testing/acceptance/runner-collaboration.md#rcb-a03) | L1 U1 / 预设、冻结与换代准入 | 本仓隔离 Git/SQLite/CLI fixture；`go run ./cmd/test-feedback -- ./internal/app ./internal/storage/sqlite ./internal/runnerclient -run '^(TestRunnerPresetFreeze\|TestRunnerWorkflowContract)$' -count=1` | B 完成，可先于 A 构建；必须命中 B6 两个 subtest：旧运行时拒绝替代实例且零新业务派发；显式重启后新运行时接纳 B、原 Run/输入保持、旧 A 停止不影响 B 且 B 用独立停止证明。另验摘要/事务/三类准入/未结算守卫；summary 与两 subtest 的派发/绑定日志 |
| [RCB-A03](../testing/acceptance/runner-collaboration.md#rcb-a03) | L1 集成 I1 / 固定真实 EXE 配置契约 | Windows 本次临时 state-dir；先显式设置 `STEERRUNNER_EXE`，再 `go run ./cmd/test-feedback -- -tags=runnercontract ./tests/integration -run '^TestRunnerPublishedWorkflowContract$' -count=1` | A/B 完成；实际 EXE 两流程目录、组合 schema、caller 配置验证/输入编码一致，零模型/PR，服务结算完整；capabilities 原文/EXE 身份/summary；不覆盖真实 Run 查询 |
| [RCB-A04](../testing/acceptance/runner-collaboration.md#rcb-a04)、[RCB-A05](../testing/acceptance/runner-collaboration.md#rcb-a05) | L1 U2 / 读取与检查 | 本仓隔离 fixture；`go run ./cmd/test-feedback -- ./internal/app ./internal/environmentcheck -run '^(TestRunnerWorkflowReadProjection\|TestRunnerCheckPresetIsolation)$' -count=1` | C 完成；详情/历史同来源、旧/兄弟报告不污染、检查零修改预设/零 workflow；summary |
| [RCB-A04](../testing/acceptance/runner-collaboration.md#rcb-a04)、[RCB-A05](../testing/acceptance/runner-collaboration.md#rcb-a05) | Web W1 / 预设和审计投影 | jsdom；`npm --prefix web test -- src/features/runs/runs.test.tsx src/features/runs/run-task.test.tsx src/features/tasks/task-detail.test.tsx src/features/environment-checks/environment-checks.test.tsx`；`npm --prefix web run build` | C 完成；原选择、冻结名称、请求/观察、audit/未知和控制资格断言，0 失败/意外 skip；保存输出 |
| [RCB-A02](../testing/acceptance/runner-collaboration.md#rcb-a02)、[RCB-A05](../testing/acceptance/runner-collaboration.md#rcb-a05) | L1 U3 / caller 事务和恢复 | 本仓既有隔离 fixture；`go run ./cmd/test-feedback -- ./internal/runnerbridge ./internal/taskrunner ./internal/environmentcheck ./internal/storage/sqlite -count=1` | B/C 集成后；原 ID/授权/未知、迟到停止、唯一交付、公开快照事务保持；summary |
| [RCB-A05](../testing/acceptance/runner-collaboration.md#rcb-a05) | L2 controlled C1 / `smoke.delivery` | 已有隔离 CLI fixture；`go run ./cmd/test-feedback -level=L2 -env=controlled` | D 接线后；实际命中 controlled gate，TaskDelivery/后继及清理成功，明确 gate-only；scenario/summary |
| [RCB-A04](../testing/acceptance/runner-collaboration.md#rcb-a04)、[RCB-A06](../testing/acceptance/runner-collaboration.md#rcb-a06) | L3 live N1 / `workflow.local-codex-audited` | Windows 本机 Codex、固定实际 Runner、SteerlineTest 专属引用；D2 补支持并显式设置 `STEERRUNNER_EXE`、`STEERLINE_AUDIT_EXECUTION_MODEL`、`STEERLINE_AUDIT_MODEL` 后执行 `go run ./cmd/test-feedback -level=L3 -suite=workflow -scenario=workflow.local-codex-audited -env=live` | A–C 和驱动完成、C1 及 I1 通过后；两型号不同，正式选择/原会话审计/发布/真实合并/唯一交付/后继 ready 未启动；正式详情核对 workflow/audit/materials/轮次模型和来源、正式历史核对摘要及绑定；保存产品响应/revision/Runner 对照/cleanup，非 full-L2，不新增真实运行 |

U1 落到 app、sqlite 和 runnerclient，U2 落到 app 和 environmentcheck；每个列出的包至少有对应命中；若实际组织合并测试入口，则同步表格为精确包名/命令，不能让空匹配蒙混通过。新增 N1 的模型变量由驱动实施为必填、支持明确 effort（本次固定执行 medium、审计 high），在创建现场前校验；不同模型只是该验收的区分条件。所需登录/原生版本/权限按所选 Runner 能力准备，不从本机配置复制秘密。

本次不默认跑 Full、完整 L3/L4、Orca 或原生故障矩阵：caller 规则由 U1–U3 覆盖，I1 只覆盖真实配置契约，N1 同时覆盖真实产品读取与交付，Runner 原逐轮冻结用 R2 定向核对，审计故障仍归其既有测试。普通 live L2 的内容和入口保留，本任务不重复跑第二条真实普通流程；C1 + 新 N1 不宣称完整 L2 通过。若实施改动了普通交付语义，须追加既有完整 L2 并更新验收映射。

既有分级隔离夹具可以直接使用；临时/AI/浏览器调试必须先运行 `scripts/new-debug-release.ps1` 生成 `C:/SteerlineDebug` 新包，AI 显式 simulate，真实链显式 live。不得触碰 `C:/SteerlineInstall` / `C:/SteerlineRuntime`，不得复制安装配置、DB 或认证；换方案新包。非分级调试的真实 PR 同样只针对 SteerlineTest 本次专属 base/head。

## 4. 人工与完成条件

人工 [RCB-M01](../testing/acceptance/runner-collaboration.md#rcb-m01) 在 Web 集成后由用户方便时确认，环境为新 simulate 包；只评价术语、方案区分、审计/失败可读性，可后置，不阻塞自动实现、无关任务或 Runner 单仓契约交付。若发现真实授权/身份错误，按缺陷修复，不以“人工后置”豁免。

规划交付完成仅表示 Spec/Plan/验收清单及生命周期差异可审阅。B/C 可先完成 fixture 下的实现与定向验证；A 实际构建和证据未交接时，仅 I1/N1 及跨仓功能交付受阻，不将开发工作整体挂起。RCB-A01–A06 全部实跑登记后才记本任务自动验收完成，尤其 A04 的真实读取需 N1，不能以 U2/W1 或空闲 I1 替代。缺原生前置时如实保留待验；示例/协议/存储/Web 类型同批一致，历史结果和原冻结 Run 不改写。
