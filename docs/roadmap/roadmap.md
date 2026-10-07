# SteerLine Roadmap

SteerLine 管理任务、原 worker 会话、用户反馈和真实 PR 交付。worker 负责实现、提交和创建 PR；测试、修复、审查要求来自任务 Spec/Plan。后续产品方向见 [产品愿景](product-vision.md)，接口及并行条件见 [执行依赖](execution-map.md)。

## 执行、候选与版本

第一阶段统一只交付和验收本地 Codex 与本机 Orca。远程准入、派发、反馈、恢复、取消及本机/远端混合流程暂移出一期，也不列作一期阻塞或待补验；后续另行规划。保留既有远端代码、协议定义及历史证据，不将它们声明为当前支持或完成条件。Git remote 和托管 PR 发布仍属于本机交付链。

一期由外部提供稳定roadmap，复用已具备的持续worker、原会话反馈、恢复及唯一交付基础。分叉只用于人工对比执行方案：默认一个，各自冻结不同配置；开始时已达5个拒绝，允许竞态偶尔超出；合并前可放弃退出实验，合并后拒绝。保留原会话、原请求、资源事实和唯一交付，不建严格名额、自动评分/融合或实验调度器。#39 的本机双后端候选、独立反馈、唯一交付、原会话恢复与放弃增量已完成；后续按需复用[当前证据](../validation/candidate-acceptance.md)，不要求追读已交付规格。

近期运行目标收敛为一个本机安装版本与独立源码调试路径共存，完整 stable/dev 双 channel 和受控提升延后。已有分级 E2E 降低回归成本；安装入口和长期运行仍须验证，不能用 runner 临时构建或短时 smoke 代替。

| 任务 ID | 任务名称 | 交付范围 | 硬前置 | 执行规格 |
| --- | --- | --- | --- | --- |
| 83 | 本机安装与调试路径隔离 | 固定安装 exe、显式调试 exe/home、状态隔离与共存 | 无 | [Spec](../spec/local-runtime-isolation.spec.md) · [Plan](../spec/local-runtime-isolation.plan.md) |
| 42 | 第一阶段集成验收 | 单安装版双本机链、24 小时、调试共存与同构建重启 | `#39`、`#83` | [Spec](../spec/phase1-release-acceptance.spec.md) · [Plan](../spec/phase1-release-acceptance.plan.md) |

## 第一阶段剩余验证

持续 worker、本机双后端候选、独立原会话反馈/重启、真实 PR 核验、唯一 TaskDelivery 与后继 base 已具备；候选控制台的 CC5 人工输入也已确认通过。#42 按需复用[本机验收入口](../validation/candidate-acceptance.md)，在精确安装产物上完成两条组合链、24 小时与同构建重启；不做版本提升。24 小时可在安装/执行能力稳定后安排，不阻塞二期 API/UI 按自身前置研发。

以下仅保留尚未被当前证据完整覆盖的范围，后续验证 Plan 按实际改动选择，不重新加入已完成的单会话或真实 PR 链：

- **定向 race detector**：具备 CGO/C 工具链后验证 app-server 并行、恢复与回复；既有 A/B 服务级闭环证据按需从[状态入口](../validation/pending-acceptance.md#local-codex-app-server)核对。
- **Orca 活动轮次输入（ON3）**：仍需核验活动轮次原 question/message 的纯文字及选项＋备注回复、重复提交与原 Dispatch 归属；轮次结束后的候选反馈与原请求重放已具备。
- **Orca 消息竞争与故障（ON5、ON7）**：仍需定向覆盖活跃 send/worker_done 竞争、迟到 question/旧 Dispatch、Orca 自身断联、agent 未识别、审批阻断和身份变化。已具备的本地 Codex 连接故障及 Orca 兄弟继续证据可复用。
- **Orca 输出与独立生命周期（ON8、ON9）**：规范最终正文与本次 owned 资源清理已具备；完整 transcript 边界、中心单独停止观察时的原终端/工作区事实仍需定向核验。

第二种 Orca agent 在具备实际目标和环境后另行规划，不属于当前仅 Codex 的一期交付前置。

## 原生 UI 与受控规划

二期采用[草稿roadmap与审核准入目标](phase2-draft-roadmap-admission.md)：主agent自由规划，独立审核内容及前置成熟度，SteerLine受控纳入正式任务集合。该新增机制目前是目标级规划，不分配任务ID或新Spec；下表 #43–#53是既有接口/会话/UI基础，涉及草稿、发布与准入的范围在实现前按新目标核对并形成正式修订，不能把现有Spec宣称为已包含审核agent。

近期产品主线优先 #45 本机 API，再由 #44/#45 交付 #50 执行与待处理界面，复用已有阶段与反馈。#43 先验证规划 agent 的实际能力，#44 提供原生输入基座；两者可独立准备。不等待完整双版本管理或全面目录整理。草稿、审核、发布与准入范围在正式实施前对齐二期目标。

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

- #83 先交付最小共存；#43/#44/#45 可按各自前置准备，共享启动器、宿主与客户端接线串行集成。
- 本机候选基线已具备；#83 → #42 验单安装版、组合链、当前格式重启与 24 小时运行，不依赖完整双版本管理。
- #36 → #45，优先 #44/#45 → #50；规划链按能力验证及新审核准入方向继续细化，#53 汇合四个页面与修订后的 #42。

本页只修改仓库规划，不自动暂停中心派发、删边或改写已冻结契约。实施前核对中心 Task 的 commit/摘要/依赖；移出规划的旧任务若仍在中心待派发，须经受控操作处置。#42/#53 若已固定不同范围，应先受控替换并同步身份及消费者依赖，保留旧历史和未结算资源事实，不能把文档新图当作 Ready 回执。

## 规划约束

[架构约束](architecture-constraints.md) · [控制台约束](console-constraints.md) · [第一阶段交付](phase-1-delivery.md)
