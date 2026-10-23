# Spec：SteerRunner 拆分后的测试精简

日期：2026-10-15。状态：已实施。用户直接授权的普通开发文档，不分配 Task ID，不修改 roadmap 或 execution-map。配套 [Plan](../plans/runner-test-convergence.plan.md)及[专属验收](../testing/acceptance/runner-test-convergence.md)。

## 1. 目标与已确认边界

SteerRunner 已独立拥有执行流程、原生适配器、工作区、会话、输入、结果核验、发布及恢复。SteerLine 的测试随职责收敛：保留 Task/DAG、候选参与、Runner 公开 CLI 调用方、投影、唯一交付、应用宿主和 Web；执行层专项测试由 SteerRunner 项目维护。

用户明确要求保留完整 L2 smoke：通过真实 SteerLine 调用真实 Runner，使用本机 Codex 实际执行任务并创建真实 PR，完成合并、TaskDelivery 和后继释放。L2 因而继续覆盖 Runner 正常执行及发布能力，不能改成仅验证协议、模拟模型、模拟 PR 或只提交到 waiting_pr_merge。

本次不修改产品 worker、重试或恢复规则。规则归属沿用 [worker-lifecycle](../../docs/product-design/worker-lifecycle.md)：Runner 管执行事实与恢复资格，SteerLine 管原 CLI 请求、绑定、事件投影和 W17 TaskDelivery。仅调整测试所有者、夹具、选择及证据要求，不另立状态机。

## 2. 当前调查

最初规划基线为 `f3e353732d57431b7cbe6a94e23d991f23a355c0`，当时注册 26 项、调查时工作树干净。本轮实施输入已 rebase 为 `20f347cdec99dc0f0701b41311112f7e330389b6`，当前调查表按该基线的 28 项登记。已读取 `docs/spec/SpecGuide.md`，其 Git blob 为 `cb77f165b80f6e7bbb09fe190d0f86b97201e2c3`；其中 roadmap 转正流程不用于本次普通开发。

| 位置 | 当前行为及处理结论 |
| --- | --- |
| [suites.json](../../tests/e2e/suites.json) | 注册 28 项，含 17 项 recovery controlled，主干已删除 nativeworkspace 并将 runtime 拆为四项；Orca terminal/environment、原生资源与 Codex 恢复专项仍在 SteerLine |
| `tests/e2e/smoke/delivery_test.go`、`suites/delivery/delivery_test.go` | 基础交付大量重叠；合并到 smoke，保留独有的交付后静默和唯一交付断言 |
| `tests/e2e/live/live_test.go`、`internal/testenv/live.go` | L2 本机 Codex 已使用真实 SteerlineTest PR；其他 live 重复交付或附加执行恢复。保留前者，移出后者 |
| `internal/testenv/staged.go`、`permissions.go` | 公开 Run/final 来源与绑定检查可用于 L2；受控 Task 测试不应再要求原生 session、三轮 dispatch 或工作区内容 |
| `internal/controlled`、`internal/native`、`internal/testkit/protocol` | 存在 Codex/Orca 协议模拟、终端状态、原生进程故障、rollout/usage 和沙箱测试；仅承担执行能力的部分迁出，调用方夹具改为公开 Runner CLI |
| `cmd/test-feedback/e2e.go` 及其测试 | 仍允许 Orca chain/native 环境并校验 usage、远端和沙箱证据；同步删除退役选择及专属规则，保留 L2 门槛和真实 PR 证据核验 |
| `scripts/test-orca-state-transitions.ps1` | 仍运行已迁出 adapter 包；失效入口与其他 Orca 专属探针一起清理 |
| [测试总入口](../testing.md)、[E2E 指南](../../tests/e2e/README.md) | 当前说明混入旧目标、失效命令和原生验收要求；实施时更新，历史事实保留原范围 |

Runner 既有覆盖须按实际固定源码和 owner 测试核对，不能据旧删除审计中的“已迁入”文字直接认定等价覆盖；本轮去向与实际运行证据统一登记专属验收。

### Rebase 后的调整

2026-10-15 已 rebase 到 `20f347cd`（PR #327）。保留主干四项 runtime 的独立断言、最多二项并行、独立证据与原期限，不恢复已删除的 nativeworkspace、debug-runtime/test-debug-runtime-isolation 脚本或旧聚合场景。当前注册表为 28 项，本次目标为 7 项。

Runner 的三个已有工作区均存在他人未提交改动；本次使用独立源码副本固定到已提交 `af54556` 核实并运行其已有专项，避免触碰其他工作区。

## 3. 测试归属

| 测试内容 | 所有者及要求 |
| --- | --- |
| Orca 本机/远端路径、配对、environment、终端身份、未知发送、断连及工作区删除重启 | Runner 专项，移出 SteerLine 的当前执行入口 |
| Codex 同 thread/turn 恢复、原生问答、权限、沙箱、usage、rollout、原生故障注入 | Runner 专项；SteerLine 不保留独立实现及解析器 |
| workflow 阶段推进、final 来源核验机制、补报、发布重试和资源结算机制 | Runner 专项；L2 仍实际走通正常实现、报告和发布，并消费公开结果 |
| EXE 固定、公开协议/schema 使用、stdout/退出码/超时及未知回执 | SteerLine caller；使用公开 CLI 子进程夹具 |
| 原请求持久化、outbox、绑定、事件/游标、授权 revision、事务回滚 | SteerLine caller/SQLite/HTTP；无原生模型依赖 |
| Task 领取、候选共同 base/前置、唯一交付、后继释放及全部候选停止义务 | SteerLine；不得因清理专项而删掉业务边界 |
| 展示公开问题/final/状态/能力并提交原操作，应用实例和多项目隔离 | SteerLine；保留现有 HTTP/Web/宿主 owner 测试 |
| 新包生成、冻结方案、安装/调试隔离、重置约束 | SteerLine；保留产品调试能力与测试 |

混合测试按断言拆分。例如未知原生输入不重发归 Runner；CLI 提交结果未知时保存原 request_id/body、只查询原回执归 SteerLine。Runner 停服不代表原生已停的机制归 Runner；SteerLine 不把 not_running 误记为已结算、保存停止义务仍归 SteerLine。

## 4. 长期场景与稳定入口

注册表由 28 项收敛为以下 7 项，不再单列重复的 `delivery.basic`：

| 级别 | 场景 ID | 环境 | 范围 |
| --- | --- | --- | --- |
| L2 | `smoke.delivery` | controlled | Task 领取、公开 Run 绑定、PR 合并事实、唯一交付、后继释放及交付后静默；Runner/PR 均为隔离夹具 |
| L2 | `smoke.local-codex` | live，`local-codex` | 真实 Runner＋本机 Codex＋真实 SteerlineTest PR 的完整流程 |
| L3 | `recovery.waiting-merge-restart` | controlled | SteerLine 等待合并时重启，恢复原绑定/游标，不重复提交/授权，合并后仅一次交付 |
| L3 | `runtime.source-build-isolation` | controlled | 保留主干的源码构建、启动文件与实际构建来源隔离 |
| L3 | `runtime.path-isolation` | controlled | 保留主干路径、符号链接及安装/调试越界反例 |
| L3 | `runtime.service-write-isolation` | controlled | 保留主干服务运行、任务写入及状态目录隔离 |
| L3 | `runtime.dual-home-isolation` | controlled | 保留主干同仓双 home 与应用宿主/锁隔离 |

`go run ./cmd/test-feedback -level=L2` 保持先 controlled 后 live。受控执行、证据或清理任一失败，必须在构造/启动真实模型环境前阻断 live。`-level=L2 -env=controlled` 只运行门槛，报告仍为 gate-only，不能声明完整 L2 通过。

L3 继续按 suite/scenario 精确选择。已移出的场景、`env=native` 和 Orca chain 明确拒绝，不别名到其他测试、不静默忽略。不新增旧参数兼容。L1/Full 不自动运行 live；Full 继续覆盖全部保留的确定性测试，但不扫描或调用已退役测试入口。

## 5. L2 真实链路的最低要求

1. 使用按版本与 SHA 固定的独立 Runner EXE，经真实 SteerLine 产品命令领取任务、提交并查询原 Run；不能直接运行 Runner 来绕开 Task 接线。
2. Runner 通过本机 Codex 的正常 development 流程完成实现、报告及发布。使用固定任务 Spec/Plan、模型与 effort，实际写出并验证最小产物、提交并推送工作分支。现有固定模型为 `gpt-6-luna` / `medium`，选择经 Runner 公开配置与 schema 传递。
3. 在 `erranliu/SteerlineTest` 的本次专属 base/head 创建真实 PR。任务发布脚本可以是固定 fixture，但必须由真实执行链调用；测试驱动不得代做任务实现、业务 commit/push 或创建候选 PR 来伪造成功。驱动允许准备专属 base、查询、转 ready、合并及精确清理。
4. 消费 Runner 公开 Run、阶段结果和 canonical final，核对来源与原 Run/候选的一致性；正常三阶段各有对应公开结果。验证 PR 的 repository/base/head/head SHA，并从该 Git 提交核对任务产物，不能只采信模型自述或 PR 正文。
5. 通过真实 provider 核实 PR 已合并，SteerLine 提交唯一 TaskDelivery；所选候选及 PR 身份一致，后继变为 ready 且未自动启动，重复查询/对账不产生第二次交付或新业务提交。
6. 保存源码、配置/Runner 版本及 SHA、公开 Run/事件、Git 产物核验、PR URL/精确分支与 merge 事实、TaskDelivery/后继和清理证据。真实 PR 历史保留；成功与失败均核实本次服务及资源收尾，未知不能写成已停止。

L2 覆盖 Runner 正常能力，但不扩大成其恢复、Orca、沙箱或原生协议矩阵。移除从原生 rollout 解析 usage、强制 token 为正以及 OS 权限探针等独立门槛；真实执行证据由固定 live 配置、真实 Runner 调用、来源匹配的公开结果、Git 产物和真实 PR 共同提供。公开接口未提供的 usage 不补造、不作为本次通过条件；移除旧报告门槛须与新证据校验同时落地，不能简单令 `usage_verified=true`。

## 6. 夹具、环境与清理

确定性 caller 测试只在公开 CLI 边界模拟 Runner 的响应、事件页和调用回执；脚本化夹具不实现原生 transport、workflow 或恢复状态机。复用现有 `internal/testkit/protocol/runner_service.go` 的 EXE 子进程方式，补齐必要能力，避免并列多套模拟服务。受控场景断言调用次数/内容和 Task 事实，不再计数原生 session/dispatch。

L2 live 继续使用真实 Runner；受控夹具与真实链路明确分开，真实场景缺 EXE、登录、发布权限、公开结果或 PR 证据时失败/阻断，不转模拟或当作 skip 后成功。普通确定性入口不要求安装 Codex、Orca、远端主机或真实 Runner；需要 EXE 的接线测试使用自产公开 CLI 夹具。

临时/AI 调试及真实 PR 调试先由 `scripts/new-debug-release.ps1` 建立 `C:\SteerlineDebug` 下全新独立包；L2 可复用现有分级隔离夹具，不接触 `C:\SteerlineRuntime`。测试项目均为 SteerlineTest，真实 PR 仅操作本次专属分支。认证引用宿主已有登录，不复制安装库、认证或用户配置，不下发凭据到产物。运行后方案冻结，换方案新建环境。保留现有独立业务期限及清理预算，不用自动重跑或延长窗口制造通过。

### 调用方 UI 的真实状态接线

Web 必须消费 Runner 实际返回的 `pr_ready`，将仍参与且 Task 未结束的候选显示为待合并并允许提交原公开反馈。不能让测试 fixture 把状态伪装成旧 `waiting_external` 来掩盖问题。这里只补齐既有调用方状态识别，不改 Runner 状态机、HTTP 信封、授权 revision、交付门槛或已结束任务的只读规则。

## 7. 迁移及非目标

实施前形成逐项清单：原测试/断言、SteerLine 保留位置或 Runner 对应位置、已有覆盖证据或本次需补齐的最小测试。已有等价覆盖的重复项直接删除；独有执行要求先在 Runner 落实对应测试，SteerLine 不长期保留重复测试。迁移记录链接实际 Runner commit/文件和定向测试结果；未验状态不得改成通过。

用户已取消的 Orca 真机验收不重新启动，不成为本次精简前置。其已有专项按所有者迁移，取消和历史失败事实保留。Runner 新增专项的执行取其最小必要范围，不要求重跑全量或全部真机目标。

本次不删除产品对 Runner/Orca 的配置支持，不变更公开业务协议、工作流、权限或原生恢复策略，不做产品功能重构、数据库迁移、第二条执行链路或新测试框架。Web 既有独有测试继续保留，不借此次精简重复建设浏览器综合场景。

## 8. 验收要求

全部为自动验收，具体步骤、迁移明细和实时结果在[专属验收](../testing/acceptance/runner-test-convergence.md#验收项)登记。

| ID | 通过条件 |
| --- | --- |
| RTC-A1 | 注册表精确为 7 项；L2 固定两阶段、失败阻断和 gate-only 语义正确，退役选择拒绝，报告检查不漏项/零匹配 |
| RTC-A2 | caller/Task/SQLite/HTTP 侧的原请求、授权、事件、绑定、事务、交付与停止义务保留，公开 CLI 夹具覆盖所需正常及异常响应，无原生状态机 |
| RTC-A3 | L2 live 实际完成本机 Codex 实现/报告/发布、真实 PR 合并、唯一交付和后继释放，公开结果、Git/PR 及清理证据完整；不能模拟或绕过 |
| RTC-A4 | 五项保留的 L3 controlled 实际通过，等待合并重启无重复副作用，隔离/调试/重置能力保留 |
| RTC-A5 | 执行专项和失效脚本从 SteerLine 当前测试闭包移出，独有断言有明确 Runner 覆盖；L2 必要 live 配置/公开证据与产品能力未误删 |
| RTC-A6 | L1、Full 选择及测试依赖闭包与实际 owner 一致，普通及 e2e 编译通过；现行文档、命令与证据规范一致，历史验收事实保留 |

不新增必须人工参与的验收；已有登录和 GitHub 权限为 live 前置。前置缺失即记录 RTC-A3 待验/阻断，不能用静态检查替代。规划交付仅执行文档检查，不运行本节的产品验收。

模拟模板的 PR 轮询调整为 15 秒，适配公开 CLI 调用成本，避免原先 100ms 测试参数不断使用户回答版本过期；保持产品默认配置、当前版本校验和 unknown 原请求规则。生成后仍冻结方案，不改运行现场配置。

### 提 PR 前主干整合

本分支再次 rebase 到 `7cd2ebe1`（安装部署 #328、执行展示/最近消息 #329）。保留新增的部署与可观测性指南入口、公开消息/状态读取能力及 `C:\SteerlineInstall` / `C:\SteerlineRuntime` 双安装目录保护；七场景收敛范围和真实 L2 契约不变。原实际执行证据继续保留其采集时 commit，不将 rebase 后的 SHA 冒充已跑源码。
