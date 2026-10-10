# SteerLine 草稿 Roadmap

本页为本轮选定草稿，用于一期收尾及二期三节点规划。编号/文档不产生执行授权。用户已确认的 #84/#45/#44 列入[execution-map](../execution-map.md)，由 roadmap-runner 直接调度 Codex，启动前核对当前状态、依赖与固定 Spec/Plan。已合入的安装/调试隔离能力见[交付验收记录](../../acceptance/local-runtime-isolation.md)，对应完成条目已从本草稿移除。

规划按[RoadmapGuide](../RoadmapGuide.md)和[并行开发指南](../../architecture/parallel-development.md)。开发执行范围与依赖由用户确认后维护于 execution-map；本文不改变外部产品 Task/DAG。每个执行任务保持独立 Spec/Plan、一个实现 PR及专属 acceptance。

## 一期收尾

一期包含本地 Codex、本机 Orca 与远端路径交付；远端契约与结果见[Spec](../../spec/orca-remote-l3.spec.md)及[专属验收](../../acceptance/orca-remote-l3.md)。已具备持续 worker、同任务人工多候选、独立原会话反馈、恢复、真实PR核验、唯一TaskDelivery及后继base。默认一个候选，开始时参与数达5拒绝，不建严格名额、自动评分/融合或实验调度器。

已交付一个本机安装版本与源码调试共存的路径隔离、实际 exe/home/config 传递和受控共存验证；不建设 stable/dev 双 channel、提升或跨开发格式兼容。人工组织的一期组合/24小时/同构建恢复见[一期活动](../../acceptance/phase1.md)，仍待签收，不是任务，不阻塞无关二期研发。

### 已交付的 worker 分步能力

#84 已通过 PR #262 合入并完成合入后独立复审；完成任务条目已移除，验收来源、待人工 WFS-M1 及后续 Orca 接入路线的规范 final 能力缺口见[专属记录](../../acceptance/worker-staged-flow.md#2026-10-10-真实合并与合入后独立复审)。通用流程用普通代码定义 implement 执行/报告与 publish 单轮，复用原会话/Attempt、唯一交付和既有生命周期端口；后续平台路线的能力不由历史通过推定。

一期仍须固定新安装构建，满足当前支持路线的实际能力后组织组合链/24 小时/同构建恢复及签收。剩余 #45→#44，硬依赖保持；#45/#50 消费实际公共状态/步骤字段，同快照投影和接线保持一套。原生差异归 adapter，恢复只继续原会话，不提供新会话替代。

定向待补仍见[app-server race](../../acceptance/local-codex-app-server.md)、[Orca活动输入/故障/生命周期](../../acceptance/worker-flow-orca-native.md)、[停服恢复](../../acceptance/worker-service-recovery.md)。按当前支持及产品精简核对适用范围；完整transcript阅读不再作为新界面交付要求，未验证的执行/恢复风险不能因此略过。

## 已确认近期任务：只固定确定会使用的能力

本批已按用户确认纳入 Codex 执行规划，状态以execution-map为准；草稿保留用于范围讨论，不维护实时状态。共同宿主接线指定单一修改所有者并串行合入。

| 任务 ID | 任务名称 | 交付范围 / 所有者 | 剩余拟议前置 | 执行规格 | 专属验收 |
| --- | --- | --- | --- | --- | --- |
| 45 | 本机客户端协议与历史查询 | 产品传输/宿主：现有任务、双本机候选、基本图、反馈/活动/final/PR及原请求回执 | 无 | [Spec](../../spec/phase2-local-api.spec.md) · [Plan](../../spec/phase2-local-api.plan.md) | [验收](../../acceptance/phase2-local-api.md) |
| 44 | WinUI 基座与原生输入 | Windows客户端：工程/真实窗口、IME与多行、最小客户端及真实查询联调 | #45 | [Spec](../../spec/phase2-winui-foundation.spec.md) · [Plan](../../spec/phase2-winui-foundation.plan.md) | [验收](../../acceptance/phase2-winui-foundation.md) |

#45 拥有 HTTP 接线，消费已合入的 exe/home/服务发现/启动器边界，复核共同宿主集成面。#44最终交付需要真实 #45，所以为硬前置；输入/窗口可提前设计，不以设计准备获得领取资格。

近期不包含 SSE/持久通知平台、完整离线查询、新规划DTO、通用artifact/人工事项框架、最终规划导航或完整视图偏好。#44人工输入体验可在自动通过后集中后置到2.1签收前；已知误发/串线必须修复。

## 二期三个验收节点

| 节点 | 可用成果与验收范围 | 成熟度 / 规划文档 |
| --- | --- | --- |
| 2.1 DAG与执行UI | 人工提供稳定任务契约，原生UI替代现有控制台日常操作，基本DAG/候选/反馈/PR与阻塞准确 | 能力边界已定，最小页面原型待确认；[节点验收](../../acceptance/phase2-execution.md) |
| 2.2 Roadmap草稿、artifact与人工事项 | 没有主agent也能人工维护草稿、查看关联产物、办理后验收及必要前置操作 | 流程/模型/原型继续迭代；[节点验收](../../acceptance/phase2-planning.md) |
| 2.3 主agent接入 | 消费已具备草稿/人工事项/执行能力，接入真实会话、权限、工具及确定后的固定审核/授权准入 | 流程/原型继续迭代；[节点验收](../../acceptance/phase2-main-agent.md) |

节点为人工参与的验收活动，不占Task ID、不入执行DAG。各任务专属验收仍独立维护；节点签收不自动阻塞无关后续开发。二期流程归[推进边界](phase2-delivery.md)，审核方向归[目标草稿](phase2-draft-roadmap-admission.md)。当前原型含旧演示，不是已确认实现契约。

## 下一批：2.1最小原型确认后

按用户最新要求，#49/#50已移出execution-map的本批执行范围，继续保留在本草稿；原型确认并补齐 Spec/Plan 与验收安排后，经用户确认纳入执行。原Task ID、Spec/Plan及专属验收链接保留。

| 任务 ID | 任务名称 | 交付范围 | 剩余拟议前置 | 执行规格 | 专属验收 |
| --- | --- | --- | --- | --- | --- |
| 49 | DAG 与任务原生浏览 | 真实任务/直接前置后继、阻塞、基本图分页、Spec/Plan及交付定位 | #44、#45；纳入执行前确认2.1原型 | [Spec](../../spec/phase2-roadmap-ui.spec.md) · [Plan](../../spec/phase2-roadmap-ui.plan.md) | [验收](../../acceptance/phase2-roadmap-ui.md) |
| 50 | 执行与待处理原生界面 | 当前控制台日常操作、双本机多候选、原问题反馈、有界活动/final/PR与原请求对账 | #44、#45；纳入执行前确认2.1原型 | [Spec](../../spec/phase2-execution-ui.spec.md) · [Plan](../../spec/phase2-execution-ui.plan.md) | [验收](../../acceptance/phase2-execution-ui.md) |

#49不再等待#48，也不在本任务实现完整roadmap/草稿浏览。#49/#50可分开实现，共用客户端/输入和最小导航；先合入的页面拥有共享导航接线，后合入者复核。原型未确认前不提前铺大量页面/视图模型。2.1真实验收后再决定控制台退役范围。

## 待重定候选：不提前冻结2.2/2.3

以下身份保留，但旧正文是参考方案，不进入近期送审/派发范围，不成为#44/#45/#49/#50的隐式要求。流程/原型成熟后先确定独立成果，再修订Spec/Plan和专属验收；确有新增成果再分配新ID，不强行套旧任务。

| 任务 ID | 任务名称 | 本轮处理 | 参考材料 | 专属验收 |
| --- | --- | --- | --- | --- |
| 46 | 规划提案与受控草稿 | 2.2待重定：受控自由草稿及快照；旧提案/槽/全锁不预先采用 | [Spec](../../spec/phase2-planning-drafts.spec.md) · [Plan](../../spec/phase2-planning-drafts.plan.md) | [验收](../../acceptance/archive/superseded/phase2-planning-drafts.md) |
| 48 | 正式文档发布与任务创建 | 2.2/2.3待重定：固定内容、授权/审核、目标规格核验及原请求准入；旧Publication算法不冻结 | [Spec](../../spec/phase2-document-publish.spec.md) · [Plan](../../spec/phase2-document-publish.plan.md) | [验收](../../acceptance/archive/superseded/phase2-document-publish.md) |
| 52 | 文档审阅与确认原生界面 | 待重定；人工后验收/必要前置界面先推导，不直接套旧发布页面 | [Spec](../../spec/phase2-review-ui.spec.md) · [Plan](../../spec/phase2-review-ui.plan.md) | [验收](../../acceptance/archive/superseded/phase2-review-ui.md) |
| 47 | Codex 规划会话服务 | 2.3待重定；接入门槛在本任务验证实际权限/受控草稿工具，复用已有会话能力 | [Spec](../../spec/phase2-planning-sessions.spec.md) · [Plan](../../spec/phase2-planning-sessions.plan.md) | [验收](../../acceptance/archive/superseded/phase2-planning-sessions.md) |
| 51 | 规划对话原生界面 | 2.3待重定；等待确定的会话/草稿交互，不先造对话骨架 | [Spec](../../spec/phase2-conversation-ui.spec.md) · [Plan](../../spec/phase2-conversation-ui.plan.md) | [验收](../../acceptance/archive/superseded/phase2-conversation-ui.md) |

#43“Codex规划主agent能力验证”不再安排独立探测项目；身份和[旧Spec](../../spec/phase2-agent-capabilities.spec.md)/[旧Plan](../../spec/phase2-agent-capabilities.plan.md)/[专属记录](../../acceptance/archive/superseded/phase2-agent-capabilities.md)保留，必要接入验证纳入重定后的#47。不是放宽权限门槛，也不声明中心旧Task已取消。

## 推进与人工安排

近期剩余任务按 #45 → #44 推进，#45 集成已合入 #84 的实际公共字段/接口；#49/#50仍等待原型确认和用户纳入执行。人工一期窗口不作为二期传递硬前置。同期继续2.1原型和2.2/2.3流程讨论；人工后验收、必要前置与artifact交互先用代表流程推导，见[2.2验收规划](../../acceptance/phase2-planning.md)，不立通用事项/审批/提醒引擎。

允许后置人工项登记参与人/窗口或待安排、截止节点及阻塞范围；真实失败/关键能力缺口不得后置。启动前核对 execution-map 的依赖交付与固定 Spec/Plan。#40/#41完整版本管理继续延期；#42/#53人工活动旧编号不复用。
