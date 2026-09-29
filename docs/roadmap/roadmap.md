# SteerLine Roadmap

SteerLine 管理任务、原 worker 会话、用户反馈和真实 PR 交付。worker 负责实现、提交和创建 PR；测试、修复、审查要求来自任务 Spec/Plan。后续产品方向见 [产品愿景](product-vision.md)，接口及并行条件见 [执行依赖](execution-map.md)。

## 执行、候选与版本

优先交付 #36。多候选在每个候选内部保持同一 worker，创建与交付作为完整功能交付；不再先做旧步骤规则提取或全面 SQL 搬包。版本入口可独立准备，提升在持续会话基线后完成。

| 任务 ID | 任务名称 | 交付范围 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- |
| 36 | 单 worker 持续执行与统一待决策 | 原会话持续交互、统一待决策、移除中心项目验证、真实 PR 交付 | 无 | [Spec](../spec/worker-flow.spec.md) · [Plan](../spec/worker-flow.plan.md) |
| 37 | 多候选会话与唯一PR交付 | 候选创建与完整运行、反馈隔离、唯一交付及落选事实 | `#36` | [Spec](../spec/candidate-workers.spec.md) · [Plan](../spec/candidate-workers.plan.md) |
| 38 | 候选观察界面 | 候选列表/选择和原会话反馈的 TUI 增量 | `#37` | [Spec](../spec/candidate-console.spec.md) · [Plan](../spec/candidate-console.plan.md) |
| 39 | 多候选本机与远端真机验收 | 多候选本机/远端竞争和混合后端增量验收 | `#38` | [Spec](../spec/candidate-native-acceptance.spec.md) · [Plan](../spec/candidate-native-acceptance.plan.md) |
| 40 | 固定版本与双入口 | 不可变构建、stable/dev 固定入口和身份 | 无 | [Spec](../spec/versioned-runtime.spec.md) · [Plan](../spec/versioned-runtime.plan.md) |
| 41 | dev 隔离与 stable 提升 | 独立 home、静止可恢复会话下的受控提升 | `#40`、`#36` | [Spec](../spec/runtime-isolation-promotion.spec.md) · [Plan](../spec/runtime-isolation-promotion.plan.md) |
| 42 | 第一阶段集成验收 | 版本组合、stable 24 小时、dev 并行和提升恢复 | `#39`、`#41` | [Spec](../spec/phase1-release-acceptance.spec.md) · [Plan](../spec/phase1-release-acceptance.plan.md) |

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

- #36、#40、#43、#44 可在隔离资源上准备；后继只在其硬前置真实交付后开始。
- #37 → #38 → #39 是多候选增量链；#40/#36 → #41，与 #39 一起汇入 #42。
- #36 → #45 后分为规划链与执行 UI；#53 汇合原生页面及第一阶段组合验收。

## 规划约束

[架构约束](architecture-constraints.md) · [控制台约束](console-constraints.md) · [第一阶段交付](phase-1-delivery.md)
