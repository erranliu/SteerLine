# SteerLine Roadmap

SteerLine 管理任务、原 worker 会话、用户反馈和真实 PR 交付。worker 负责实现、提交和创建 PR；测试、修复、审查要求来自任务 Spec/Plan。后续产品方向见 [产品愿景](product-vision.md)，接口及并行条件见 [执行依赖](execution-map.md)。

## 执行、候选与版本

复用[持续 worker 基础 #36](../spec/worker-flow.spec.md)。多候选在每个候选内部保持同一 worker，创建与交付作为完整功能交付；版本入口可独立准备，提升保持现有会话和恢复门槛。

| 任务 ID | 任务名称 | 交付范围 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- |
| 37 | 多候选会话与唯一PR交付 | 候选创建与完整运行、反馈隔离、唯一交付及落选事实 | `#36` | [Spec](../spec/candidate-workers.spec.md) · [Plan](../spec/candidate-workers.plan.md) |
| 38 | 候选观察界面 | 候选列表/选择和原会话反馈的 TUI 增量 | `#37` | [Spec](../spec/candidate-console.spec.md) · [Plan](../spec/candidate-console.plan.md) |
| 39 | 多候选本机与远端真机验收 | 多候选本机/远端竞争和混合后端增量验收 | `#38` | [Spec](../spec/candidate-native-acceptance.spec.md) · [Plan](../spec/candidate-native-acceptance.plan.md) |
| 40 | 固定版本与双入口 | 不可变构建、stable/dev 固定入口和身份 | 无 | [Spec](../spec/versioned-runtime.spec.md) · [Plan](../spec/versioned-runtime.plan.md) |
| 41 | dev 隔离与 stable 提升 | 独立 home、静止可恢复会话下的受控提升 | `#40`、`#36` | [Spec](../spec/runtime-isolation-promotion.spec.md) · [Plan](../spec/runtime-isolation-promotion.plan.md) |
| 42 | 第一阶段集成验收 | 版本组合、stable 24 小时、dev 并行和提升恢复 | `#39`、`#41` | [Spec](../spec/phase1-release-acceptance.spec.md) · [Plan](../spec/phase1-release-acceptance.plan.md) |

## 第一阶段残留

本地 Codex app-server 迁移的 A/B 服务级验收已补齐；保留完成记录便于 #42 第一阶段集成验收复用。定向 race detector 待具备工具链时补跑。范围和通过条件以 [迁移 Spec](../spec/local-codex-app-server.spec.md#第一阶段残留2026-09-29) 为准，证据见 [验收状态](../validation/local-codex-app-server/status.md)。

- [x] **服务级双任务真机闭环（A）**：真实 SteerLine 服务覆盖并行、运行中问答、原 thread/worktree 结束后反馈和重启恢复；原生身份和答案归属已核对。
- [x] **独立进程生命周期与隔离（B）**：独立宿主的服务反馈三例通过；真实服务宿主进程退出后，待答 turn 存活，新进程重接并完成原请求。
- [ ] **并发检测环境补验**：在具备 CGO/C 编译器的环境，对 app-server 并行、恢复和回复路径运行定向 Go race detector。

### 本地 Orca 原生 worker 后续验证

同一 worker 的两轮真实 Codex 问答与续聊、中心重启恢复、SQLite 故障回执对账和 provider 归属回归已有[实施证据](../validation/worker-flow/orca-native-implementation-2026-09-29.json)。以下项目继续按 [Orca Spec](../spec/worker-flow-orca-native.spec.md#9-验收与交付边界) 验证，并分别记录实际结果。

- [ ] **真实统一回答入口（ON3）**：通过 UI/CLI 的 `answer_decision` 对运行中的 Orca 中心提交纯文字及选项＋备注，核对回复归属原 question/message ID、重复提交不重复发送，随后继续原 Dispatch。
- [ ] **真实消息竞争与故障（ON5、ON7）**：记录活跃 `send` 与 `worker_done` 竞争、迟到 question/旧 Dispatch、断联、agent 未识别、审批阻断及身份变化时的可观察结果；核对不串线、不替换原 worker、不误关闭其他 terminal。
- [ ] **真实输出与清理（ON8、ON9）**：确认当前 Orca 版本可取得的有界输出和 transcript 完整度；在最终交付后核对精确 terminal close、原 worktree 内容及中心停止观察时的资源状态。无法取得完整 transcript 时如实标记。
- [ ] **隔离仓库真实 PR 链（ON10）**：核对 provider 上的仓库、base、执行分支与 PR 身份；由用户合并测试 PR，随后确认唯一 TaskDelivery、后继 DAG 解锁和清理结果。SQLite/provider 自动化回归已通过。
- [ ] **第二种真实 agent（ON11）**：本机具备另一种 Orca agent 后，沿相同 Run/Task/Dispatch 与问答路径验收无代码修改切换；当前仅 Codex 可用，参数转发契约已通过。

## 原生 UI 与受控规划

本机 API 直接消费 #36，不等待多候选或全面代码搬包。规划权限和文档发布核验继续保留；执行界面使用五个主阶段和统一反馈。

| 任务 ID | 任务名称 | 交付范围 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- |
| 43 | Codex 规划主 agent 能力验证 | 规划 agent 权限与受控工具能力证明 | 无 | [Spec](../spec/phase2-agent-capabilities.spec.md) · [Plan](../spec/phase2-agent-capabilities.plan.md) |
| 44 | WinUI 基座与原生输入 | Windows 原生窗口、IME/多行输入和导航基座 | 无 | [Spec](../spec/phase2-winui-foundation.spec.md) · [Plan](../spec/phase2-winui-foundation.plan.md) |
| 45 | 本机客户端协议与历史查询 | 五阶段/原会话反馈的本机 HTTP/SSE 与离线查询 | `#36` | [Spec](../spec/phase2-local-api.spec.md) · [Plan](../spec/phase2-local-api.plan.md) |
| 46 | 规划提案与受控草稿 | 规划工作区、提案/ID 预留、草稿与文档权威 | `#45` | [Spec](../spec/phase2-planning-drafts.spec.md) · [Plan](../spec/phase2-planning-drafts.plan.md) |
| 47 | Codex 规划会话服务 | 受控规划会话、草稿工具和写入仲裁接线 | `#43`、`#46` | [Spec](../spec/phase2-planning-sessions.spec.md) · [Plan](../spec/phase2-planning-sessions.plan.md) |
| 48 | 正式文档发布与任务创建 | 正式文档版本、目标分支集成与中心创建 | `#46` | [Spec](../spec/phase2-document-publish.spec.md) · [Plan](../spec/phase2-document-publish.plan.md) |
| 49 | Roadmap 与 DAG 原生浏览 | Roadmap/DAG 原生浏览与精确关联 | `#44`、`#48` | [Spec](../spec/phase2-roadmap-ui.spec.md) · [Plan](../spec/phase2-roadmap-ui.plan.md) |
| 50 | 执行与待处理原生界面 | 五阶段、完整会话与统一反馈原生页面 | `#44`、`#45` | [Spec](../spec/phase2-execution-ui.spec.md) · [Plan](../spec/phase2-execution-ui.plan.md) |
| 51 | 规划对话原生界面 | 规划会话和受控草稿工具的原生对话 | `#44`、`#47` | [Spec](../spec/phase2-conversation-ui.spec.md) · [Plan](../spec/phase2-conversation-ui.plan.md) |
| 52 | 文档审阅与确认原生界面 | 规划文档差异、固定版本确认与发布回执 | `#44`、`#48` | [Spec](../spec/phase2-review-ui.spec.md) · [Plan](../spec/phase2-review-ui.plan.md) |
| 53 | 第二阶段完整闭环验收 | 规划到真实任务交付再回规划的完整产品链 | `#49`、`#50`、`#51`、`#52`、`#42` | [Spec](../spec/phase2-integration.spec.md) · [Plan](../spec/phase2-integration.plan.md) |

## 推进顺序

- #40、#43、#44 可在隔离资源上准备；#36 仅作为已具备基础保留，后继仍按各自固定 Spec 开工。
- #37 → #38 → #39 是多候选增量链；#40/#36 → #41，与 #39 一起汇入 #42。
- #36 → #45 后分为规划链与执行 UI；#53 汇合原生页面及第一阶段组合验收。

## 规划约束

[架构约束](architecture-constraints.md) · [控制台约束](console-constraints.md) · [第一阶段交付](phase-1-delivery.md)

## 主包与执行实现解耦

2026-10-02：#65 已合并；#66 外层组装与主包生产隔离已实现并独立复审修复，完全访问组合 core 6/rules 90/targeted 22 全通过，见[交付与复审记录](../validation/core-decoupling-05-composition/README.md)。本 PR 真实合并后才启动 #67；活动表保留剩余任务。

[主包解耦规划](core-implementation-decoupling.md)继续完成通用业务与具体实现的边界。#63 的配置和原生绑定契约已交付，任务身份及验收依据保留在其 Spec/Plan；本活动清单从 #67 继续。既有执行策略、工作区、发布和需求契约作为基础复用，不再列入活动实施清单；剩余原生验收限制见[既有验收记录](../validation/execution-isolation-cleanup/README.md)。本目标严格要求主包直属生产/测试文件名与源码无平台关键字，且没有具体实现的直接或传递依赖，不能靠改名或转移可写 Service 达标。

| 任务 ID | 任务名称 | 交付范围 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- |
| 67 | 主包隔离守卫与测试归位 | 主包测试清零、结构守卫与外层第三实现闭环 | #66 | [Spec](../spec/core-decoupling-06-boundary.spec.md) · [Plan](../spec/core-decoupling-06-boundary.plan.md) |
| 68 | 主包解耦重构后代码清理 | 最终调用关系下的死代码/重复规则/无用辅助清理与索引同步 | #67 | [Spec](../spec/core-decoupling-07-cleanup.spec.md) · [Plan](../spec/core-decoupling-07-cleanup.plan.md) |
| 69 | 中心写入端口收窄 | 窄提交契约接通命令/worker/任务/DAG/交付/会话，保留原子边界 | #68 | [Spec](../spec/core-decoupling-08-write-ports.spec.md) · [Plan](../spec/core-decoupling-08-write-ports.plan.md) |
| 70 | SQLite写入实现归包 | 连接/schema/SQL/事务归唯一存储模块，复用只读 Reader | #69 | [Spec](../spec/core-decoupling-09-sqlite.spec.md) · [Plan](../spec/core-decoupling-09-sqlite.plan.md) |
| 71 | 产品客户端适配层与CLI接入 | 产品纯契约、唯一默认 adapter 和 CLI 产品读取/命令消费者 | #68 | [Spec](../spec/core-decoupling-10-client.spec.md) · [Plan](../spec/core-decoupling-10-client.plan.md) |
| 72 | TUI与中心实现解耦 | TUI 消费产品接口，删除中心/存储依赖及默认 I/O 薄桥 | #71 | [Spec](../spec/core-decoupling-11-tui.spec.md) · [Plan](../spec/core-decoupling-11-tui.plan.md) |

先按 #65 → #66 → #67 → #68 串行交付，再分为可并行的存储 #69 → #70 与客户端 #71 → #72。#65 消费[生命周期端口](../validation/core-decoupling-03-lifecycle/README.md)，#61 的需求基础及 #65–#67 共同要求见[约束](../spec/core-decoupling-constraints.md)。每阶段迁移完整消费者并当场删除对应原实现，#66 完成生产清零，#67 只负责测试归位和边界守卫；#68 不接收这些前置应完成的删除。

#68–#72 的范围及共享修改见[后续维护约束](../spec/core-maintenance-constraints.md)。存储线拥有提交契约/Store/schema，客户端线拥有产品 DTO/CLI/TUI；共享 contracts 形状及组装/quick 段顺序集成。两线均保持原子提交和当前格式恢复，完整实现只在各自硬前置整体交付后开始，不扩展 #45 传输或 UI 范围，也不通过规划变化创建中心任务。
