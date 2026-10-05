# SteerLine Roadmap

SteerLine 管理任务、原 worker 会话、用户反馈和真实 PR 交付。worker 负责实现、提交和创建 PR；测试、修复、审查要求来自任务 Spec/Plan。后续产品方向见 [产品愿景](product-vision.md)，接口及并行条件见 [执行依赖](execution-map.md)。

## 执行、候选与版本

第一阶段统一只交付和验收本地 Codex 与本机 Orca。远程准入、派发、反馈、恢复、取消及本机/远端混合流程暂移出一期，也不列作一期阻塞或待补验；后续另行规划。保留既有远端代码、协议定义及历史证据，不将它们声明为当前支持或完成条件。Git remote 和托管 PR 发布仍属于本机交付链。

一期由外部提供稳定roadmap，复用已具备的持续worker、原会话反馈、恢复及唯一交付基础。分叉只用于人工对比执行方案：默认一个，各自冻结不同配置；开始时已达5个拒绝，允许竞态偶尔超出；合并前可放弃退出实验，合并后拒绝。保留原会话、原请求、资源事实和唯一交付，不建严格名额、自动评分/融合或实验调度器。版本线保持原目标。#39 的本机双后端候选、独立反馈、唯一交付、原会话恢复与放弃增量已完成；后续按需复用[当前证据](../validation/candidate-native-acceptance/README.md)，不要求追读已交付规格。

| 任务 ID | 任务名称 | 交付范围 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- |
| 40 | 固定版本与双入口 | 不可变构建、stable/dev 固定入口和身份 | 无 | [Spec](../spec/versioned-runtime.spec.md) · [Plan](../spec/versioned-runtime.plan.md) |
| 41 | dev 隔离与 stable 提升 | 独立 home、静止可恢复会话下的受控提升 | `#40`、`#36` | [Spec](../spec/runtime-isolation-promotion.spec.md) · [Plan](../spec/runtime-isolation-promotion.plan.md) |
| 42 | 第一阶段集成验收 | 本机双链与版本组合、stable 24 小时、dev 并行和提升恢复 | `#39`、`#41` | [Spec](../spec/phase1-release-acceptance.spec.md) · [Plan](../spec/phase1-release-acceptance.plan.md) |

## 第一阶段剩余验证

持续 worker、本机双后端候选、独立原会话反馈/重启、真实 PR 核验、唯一 TaskDelivery 与后继 base 已具备；候选控制台的 CC5 人工输入也已确认通过。#42 按需复用[本机验收入口](../validation/candidate-native-acceptance/README.md)，仍须独立完成固定版本、两条组合链、24 小时及提升验收。

以下仅保留尚未被当前证据完整覆盖的范围，后续验证 Plan 按实际改动选择，不重新加入已完成的单会话或真实 PR 链：

- **定向 race detector**：具备 CGO/C 工具链后验证 app-server 并行、恢复与回复；既有 A/B 服务级闭环证据按需从[状态入口](../validation/local-codex-app-server/status.md)核对。
- **Orca 活动轮次输入（ON3）**：仍需核验活动轮次原 question/message 的纯文字及选项＋备注回复、重复提交与原 Dispatch 归属；轮次结束后的候选反馈与原请求重放已具备。
- **Orca 消息竞争与故障（ON5、ON7）**：仍需定向覆盖活跃 send/worker_done 竞争、迟到 question/旧 Dispatch、Orca 自身断联、agent 未识别、审批阻断和身份变化。已具备的本地 Codex 连接故障及 Orca 兄弟继续证据可复用。
- **Orca 输出与独立生命周期（ON8、ON9）**：规范最终正文与本次 owned 资源清理已具备；完整 transcript 边界、中心单独停止观察时的原终端/工作区事实仍需定向核验。

第二种 Orca agent 在具备实际目标和环境后另行规划，不属于当前仅 Codex 的一期交付前置。

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

纯类型、任务选择/结果小端口及 task 应用用例已具备。剩余从 #76 的执行用例迁包继续，产品/控制台、适配、存储和宿主各消费现行小端口；本机候选迁移按实际接线影响定向重验。当前源码归属按需查[架构索引](../architecture/current-code-architecture.md)。#43/#44/#45 不等待全部整理。

| 任务 ID | 任务名称 | 交付范围 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- |
| 76 | 执行应用用例归包 | 候选/会话/恢复/交付/清理归 execution | `#74` | [Spec](../spec/three-line-04-execution-usecases.spec.md) · [Plan](../spec/three-line-04-execution-usecases.plan.md) |
| 77 | 产品查询与控制台契约收窄 | 公开 DTO 收窄、唯一 query/client 和 console 归包 | `#74`、`#38` | [Spec](../spec/three-line-05-product-console.spec.md) · [Plan](../spec/three-line-05-product-console.plan.md) |
| 78 | 执行适配与资源目录归组 | 后端协议与 Git/PR/进程资源归组 | `#76` | [Spec](../spec/three-line-06-adapters-resources.spec.md) · [Plan](../spec/three-line-06-adapters-resources.plan.md) |
| 79 | 存储目录与消费者端口收口 | 唯一 SQLite 归 storage、消费者窄端口与原子提交 | `#75`、`#76`、`#77` | [Spec](../spec/three-line-07-storage-ports.spec.md) · [Plan](../spec/three-line-07-storage-ports.plan.md) |
| 80 | 应用组装与服务宿主收口 | 配置构造归 app、服务承载归 host，删除旧主包/contracts | `#75`、`#76`、`#77`、`#78`、`#79` | [Spec](../spec/three-line-08-app-host.spec.md) · [Plan](../spec/three-line-08-app-host.plan.md) |
| 81 | 测试支持与集成目录整理 | testkit 与 tests/integration、runner/断言映射 | `#80` | [Spec](../spec/three-line-09-testkit-integration.spec.md) · [Plan](../spec/three-line-09-testkit-integration.plan.md) |
| 82 | 独立开发证明与最终清理 | 三线实际独立改动证明、残留清理与最小交付/恢复组合 | `#81` | [Spec](../spec/three-line-10-independence-cleanup.spec.md) · [Plan](../spec/three-line-10-independence-cleanup.plan.md) |

## 推进顺序

- #40、#43、#44 可在隔离资源上准备；#36 仅作为已具备基础保留，后继仍按各自固定 Spec 开工。
- 本机候选基线已具备；#40 → #41，再由 #42 汇合本机候选与版本能力，硬前置 ID 保留在任务表。
- #76/#77 的固定基础已具备；#76 → #78，#76/#77 完成后满足 #79 的剩余前置，相关任务整体交付后进入 #80 → #81 → #82。共享类型、构造和提交协调段顺序集成。
- #36 → #45 后分为规划链与执行 UI；#53 汇合原生页面及第一阶段组合验收。

## 规划约束

[架构约束](architecture-constraints.md) · [控制台约束](console-constraints.md) · [第一阶段交付](phase-1-delivery.md)

#76 已由 PR219 合入；#77 在独立工作树继续产品读取/客户端工作，#78 基于该硬前置迁移适配器与资源目录。#79–#82 未由当前任务启动，后续仍按固定 Spec 与 DAG 前置执行。共享构造按[现行接口交接](../validation/three-line-04-execution-usecases/interface-handoff.md)和各任务所有权顺序集成。
