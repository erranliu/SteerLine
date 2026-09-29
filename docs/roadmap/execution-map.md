# 执行依赖与交接

本图对应[主 roadmap](roadmap.md)中的 18 项活动规划。每项执行以独立 Spec/Plan 为准，运行时资格以中心 DAG 为权威。

## 固定前置

| 任务 ID | 任务名称 | 交付范围 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- |
| 36 | 单 worker 持续执行与统一待决策 | 原会话持续交互、统一待决策、移除中心项目验证、真实 PR 交付 | 无 | [Spec](../spec/worker-flow.spec.md) · [Plan](../spec/worker-flow.plan.md) |
| 37 | 多候选会话与唯一PR交付 | 候选创建与完整运行、反馈隔离、唯一交付及落选事实 | `#36` | [Spec](../spec/candidate-workers.spec.md) · [Plan](../spec/candidate-workers.plan.md) |
| 38 | 候选观察界面 | 候选列表/选择和原会话反馈的 TUI 增量 | `#37` | [Spec](../spec/candidate-console.spec.md) · [Plan](../spec/candidate-console.plan.md) |
| 39 | 多候选本机与远端真机验收 | 多候选本机/远端竞争和混合后端增量验收 | `#38` | [Spec](../spec/candidate-native-acceptance.spec.md) · [Plan](../spec/candidate-native-acceptance.plan.md) |
| 40 | 固定版本与双入口 | 不可变构建、stable/dev 固定入口和身份 | 无 | [Spec](../spec/versioned-runtime.spec.md) · [Plan](../spec/versioned-runtime.plan.md) |
| 41 | dev 隔离与 stable 提升 | 独立 home、静止可恢复会话下的受控提升 | `#40`、`#36` | [Spec](../spec/runtime-isolation-promotion.spec.md) · [Plan](../spec/runtime-isolation-promotion.plan.md) |
| 42 | 第一阶段集成验收 | 版本组合、stable 24 小时、dev 并行和提升恢复 | `#39`、`#41` | [Spec](../spec/phase1-release-acceptance.spec.md) · [Plan](../spec/phase1-release-acceptance.plan.md) |
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

```mermaid
flowchart LR
    W36["#36 持续 worker"] --> C37["#37 候选与交付"] --> U38["#38 候选 UI"] --> A39["#39 候选验收"]
    V40["#40 固定版本"] --> V41["#41 隔离/提升"]
    W36 --> V41
    A39 --> A42["#42 第一阶段组合"]
    V41 --> A42
    W36 --> API45["#45 本机 API"]
    API45 --> P46["#46 规划草稿"]
    P46 --> S47["#47 规划会话"]
    C43["#43 规划权限能力"] --> S47
    P46 --> P48["#48 文档发布"]
    UI44["#44 WinUI 基座"] --> E50["#50 执行 UI"]
    API45 --> E50
    UI44 --> R49["#49 Roadmap UI"]
    P48 --> R49
    UI44 --> C51["#51 规划对话 UI"]
    S47 --> C51
    UI44 --> R52["#52 文档审阅 UI"]
    P48 --> R52
    R49 --> A53["#53 产品闭环"]
    E50 --> A53
    C51 --> A53
    R52 --> A53
    A42 --> A53
```

## 接口和共享修改

| 所有者 | 交接内容 | 集成规则 |
| --- | --- | --- |
| #36 | 主阶段、原会话输入/输出、PR 关联和 TaskDelivery | #37/#41/#45 消费；不另建步骤或专用恢复状态机 |
| #37 | 精确候选身份、查询/反馈和唯一交付 | #38/#39 消费；#45 初版不依赖多候选 |
| #40/#41 | 固定版本选择、真实工作轮次及恢复条件 | 配置/服务恢复入口串行集成；空闲会话不被直接视作活动进程 |
| #43/#47 | 规划 agent 的权限、受控工具及会话 | 可复用底层会话能力，规划 guard 与任务 worker 权限分别管理 |
| #45/#46/#48 | 传输、草稿权威、正式发布与中心创建 | 复用 Query/Command；发布/反馈各自按同一个写者和原请求对账 |
| #44 | 原生输入和导航组件 | 页面复用同一客户端、草稿和正文组件；共享导航串行集成 |

## 验收分工

#36 验单 worker 持续交互；#39 验多候选竞争；#42 验固定版本、24 小时和提升组合；#53 验完整规划/UI/执行闭环。上游证据可复用，接口或平台改变的部分定向重验，不重复构造已删除的五步与答案应用证明矩阵。

不同任务人工并行和自动派发策略沿既有能力；共享工作区、平台宿主、构建或 SQLite 接线时串行协调。新任务只有固定前置整体交付后可领取，没有接口提前完成的放行状态。
