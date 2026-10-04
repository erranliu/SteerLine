# SteerLine Roadmap

SteerLine 管理任务、原 worker 会话、用户反馈和真实 PR 交付。worker 负责实现、提交和创建 PR；测试、修复、审查要求来自任务 Spec/Plan。后续产品方向见 [产品愿景](product-vision.md)，接口及并行条件见 [执行依赖](execution-map.md)。

## 执行、候选与版本

一期由外部提供稳定roadmap，复用已具备的持续worker、原会话反馈、恢复及唯一交付基础。分叉只用于人工对比执行方案：默认一个，各自冻结不同配置；开始时已达5个拒绝，允许竞态偶尔超出；合并前可放弃退出实验，合并后拒绝。保留原会话、原请求、资源事实和唯一交付，不建严格名额、自动评分/融合或实验调度器。版本线保持原目标。

| 任务 ID | 任务名称 | 交付范围 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- |
| 39 | 多候选本机验收 | 本地 Codex/本机 Orca 双候选竞争、反馈隔离、唯一交付与恢复/放弃验收 | `#38` | [Spec](../spec/candidate-native-acceptance.spec.md) · [Plan](../spec/candidate-native-acceptance.plan.md) |
| 40 | 固定版本与双入口 | 不可变构建、stable/dev 固定入口和身份 | 无 | [Spec](../spec/versioned-runtime.spec.md) · [Plan](../spec/versioned-runtime.plan.md) |
| 41 | dev 隔离与 stable 提升 | 独立 home、静止可恢复会话下的受控提升 | `#40`、`#36` | [Spec](../spec/runtime-isolation-promotion.spec.md) · [Plan](../spec/runtime-isolation-promotion.plan.md) |
| 42 | 第一阶段集成验收 | 版本组合、stable 24 小时、dev 并行和提升恢复 | `#39`、`#41` | [Spec](../spec/phase1-release-acceptance.spec.md) · [Plan](../spec/phase1-release-acceptance.plan.md) |

## 第一阶段残留

- [ ] **候选控制台 CC5 人工补验**：中文组词、选项加备注、候选导航、窄窗口及滚动尚无真实通过证据；用户已允许实现先合入并继续 #39 开发，人工证据仍单独补齐。见[操作与待验记录](../validation/candidate-console/windows/README.md)。

本地 Codex app-server 迁移的 A/B 服务级验收已补齐；保留完成记录便于 #42 第一阶段集成验收复用。定向 race detector 待具备工具链时补跑。剩余范围为具备CGO/C工具链后的定向race detector补验，执行要求由后续验证Plan明确；必要时按需查[现有验收状态](../validation/local-codex-app-server/status.md)。

- [x] **服务级双任务真机闭环（A）**：真实 SteerLine 服务覆盖并行、运行中问答、原 thread/worktree 结束后反馈和重启恢复；原生身份和答案归属已核对。
- [x] **独立进程生命周期与隔离（B）**：独立宿主的服务反馈三例通过；真实服务宿主进程退出后，待答 turn 存活，新进程重接并完成原请求。
- [ ] **并发检测环境补验**：在具备 CGO/C 编译器的环境，对 app-server 并行、恢复和回复路径运行定向 Go race detector。

### 本地 Orca 原生 worker 后续验证

同一 worker 的两轮真实 Codex 问答与续聊、中心重启恢复、SQLite 故障回执对账和 provider 归属回归已有[实施证据](../validation/worker-flow/orca-native-implementation-2026-09-29.json)。以下仅列仍需补验的原生边界，沿原Task/session/Dispatch、权限及provider身份记录实际结果；已具备实现不重新作为待开发任务。

- [ ] **真实统一回答入口（ON3）**：通过 UI/CLI 的 `answer_decision` 对运行中的 Orca 中心提交纯文字及选项＋备注，核对回复归属原 question/message ID、重复提交不重复发送，随后继续原 Dispatch。
- [ ] **真实消息竞争与故障（ON5、ON7）**：记录活跃 `send` 与 `worker_done` 竞争、迟到 question/旧 Dispatch、断联、agent 未识别、审批阻断及身份变化时的可观察结果；核对不串线、不替换原 worker、不误关闭其他 terminal。
- [ ] **真实输出与清理（ON8、ON9）**：确认当前 Orca 版本可取得的有界输出和 transcript 完整度；在最终交付后核对精确 terminal close、原 worktree 内容及中心停止观察时的资源状态。无法取得完整 transcript 时如实标记。
- [ ] **隔离仓库真实 PR 链（ON10）**：核对 provider 上的仓库、base、执行分支与 PR 身份；由用户合并测试 PR，随后确认唯一 TaskDelivery、后继 DAG 解锁和清理结果。SQLite/provider 自动化回归已通过。
- [ ] **第二种真实 agent（ON11）**：本机具备另一种 Orca agent 后，沿相同 Run/Task/Dispatch 与问答路径验收无代码修改切换；当前仅 Codex 可用，参数转发契约已通过。

## 原生 UI 与受控规划

二期采用[草稿roadmap与审核准入目标](phase2-draft-roadmap-admission.md)：主agent自由规划，独立审核内容及前置成熟度，SteerLine受控纳入正式任务集合。该新增机制目前是目标级规划，不分配任务ID或新Spec；下表 #43–#53是既有接口/会话/UI基础，涉及草稿、发布与准入的范围在实现前按新目标核对并形成正式修订，不能把现有Spec宣称为已包含审核agent。

本机API直接消费 #36，不等待分叉或全面搬包；#43/#44能力与UI基座可独立准备，执行界面复用原阶段与反馈。

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

## 任务执行控制台解耦与目录整理

目标：`TASK-EXECUTION-CONSOLE-DECOUPLING`。完整范围、目标目录、所有权、阶段依赖与验收方向见 [目标roadmap](three-line-modularization.md)。任务负责挑选工作，执行负责并发派发、交付核验与结项并向任务提交结果，控制台消费稳定产品契约。

[#73 纯定义基础](../validation/three-line-01-types/README.md)承接 #37；#74 可与 #38/#39 产品链并行；#75/#76 在边界接通后分线，#77 另等 #38。#39 使用隔离固定构建，实际受影响入口在迁移后定向重验。#43/#44/#45 不等待全部整理，其他产品任务保持自己的固定前置。

| 任务 ID | 任务名称 | 交付范围 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- |
| 74 | 任务选择与执行结果边界 | 任务工作/原子领取与规范化结果边界、执行核验归属 | `#73` | [Spec](../spec/three-line-02-task-execution-boundary.spec.md) · [Plan](../spec/three-line-02-task-execution-boundary.plan.md) |
| 75 | 任务应用用例归包 | 任务创建/依赖/挑选/结果应用归 task | `#74` | [Spec](../spec/three-line-03-task-usecases.spec.md) · [Plan](../spec/three-line-03-task-usecases.plan.md) |
| 76 | 执行应用用例归包 | 候选/会话/恢复/交付/清理归 execution | `#74` | [Spec](../spec/three-line-04-execution-usecases.spec.md) · [Plan](../spec/three-line-04-execution-usecases.plan.md) |
| 77 | 产品查询与控制台契约收窄 | 公开 DTO 收窄、唯一 query/client 和 console 归包 | `#74`、`#38` | [Spec](../spec/three-line-05-product-console.spec.md) · [Plan](../spec/three-line-05-product-console.plan.md) |
| 78 | 执行适配与资源目录归组 | 后端协议与 Git/PR/进程资源归组 | `#76` | [Spec](../spec/three-line-06-adapters-resources.spec.md) · [Plan](../spec/three-line-06-adapters-resources.plan.md) |
| 79 | 存储目录与消费者端口收口 | 唯一 SQLite 归 storage、消费者窄端口与原子提交 | `#75`、`#76`、`#77` | [Spec](../spec/three-line-07-storage-ports.spec.md) · [Plan](../spec/three-line-07-storage-ports.plan.md) |
| 80 | 应用组装与服务宿主收口 | 配置构造归 app、服务承载归 host，删除旧主包/contracts | `#75`、`#76`、`#77`、`#78`、`#79` | [Spec](../spec/three-line-08-app-host.spec.md) · [Plan](../spec/three-line-08-app-host.plan.md) |
| 81 | 测试支持与集成目录整理 | testkit 与 tests/integration、runner/断言映射 | `#80` | [Spec](../spec/three-line-09-testkit-integration.spec.md) · [Plan](../spec/three-line-09-testkit-integration.plan.md) |
| 82 | 独立开发证明与最终清理 | 三线实际独立改动证明、残留清理与最小交付/恢复组合 | `#81` | [Spec](../spec/three-line-10-independence-cleanup.spec.md) · [Plan](../spec/three-line-10-independence-cleanup.plan.md) |

## 推进顺序

- #40、#43、#44 可在隔离资源上准备；#36 仅作为已具备基础保留，后继仍按各自固定 Spec 开工。
- 已具备 #37 → #38 → #39 是多候选增量链；#40/#36 → #41，与 #39 一起汇入 #42。
- [#73 基础](../validation/three-line-01-types/README.md)之后由 #74 接通边界，再分为 #75/#76/#77（#77 另等 #38）；#76 → #78，#75/#76/#77 → #79，相关五项 → #80 → #81 → #82。解耦与产品验收在隔离构建/资源上并行，共同类型与组装段顺序集成。
- #36 → #45 后分为规划链与执行 UI；#53 汇合原生页面及第一阶段组合验收。

## 规划约束

[架构约束](architecture-constraints.md) · [控制台约束](console-constraints.md) · [第一阶段交付](phase-1-delivery.md)
