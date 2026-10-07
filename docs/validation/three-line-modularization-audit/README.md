# 解耦目标与产品愿景对齐审计

审计对象：[任务执行控制台解耦与目录整理roadmap](../../archive/roadmap/three-line-modularization.md)。本次是目标文档审计，依据实际仓库代码、产品愿景与既有Spec，不是实现或原生能力验收。

## 结论

单Go module、任务挑选与执行派发分开、执行负责交付和关闭决定、UI消费产品契约、存储保留原子提交的方向合理。原稿需要补齐多候选、规划主agent和跨语言UI的约束；否则容易把当前单worker模型固化，或把未来规划领域塞进执行包。下表确认的缺口已在目标roadmap修正，产品愿景与既有Spec的业务范围和硬前置未改动。

本目标是产品发展的结构支撑，不能替代一阶段分叉执行或二阶段主agent/UI的产品任务，也不能成为它们的隐藏整体前置。建议优先推进 #37–#39，全面迁移基于已交付候选模型；#43/#44可独立准备，#45继续沿 #36前置推进。

## 审计依据

- [产品愿景](../../roadmap/product-vision.md)：一阶段同Task多候选、独立会话/工作区/PR和唯一交付；二阶段Codex主agent、人的确认、规划和原生UI。
- [第一阶段交付](../../roadmap/phase-1-delivery.md)、[多候选Spec](../../archive/spec/completed/candidate-workers.spec.md)、[候选界面Spec](../../archive/spec/completed/candidate-console.spec.md)：首次领取、后续候选、精确身份、集合竞争、迟到合并和资源结算。
- [二阶段技术架构](../../product-design/planning-architecture.md)、[共同接口约束](../../spec/phase2-constraints.md)、[本机协议Spec](../../spec/phase2-local-api.spec.md)：Go服务与C#/WinUI分离，HTTP/JSON/SSE、领域接线、离线读取和版本语义。
- [规划草稿Spec](../../spec/phase2-planning-drafts.spec.md)、[规划会话Spec](../../spec/phase2-planning-sessions.spec.md)、[文档发布Spec](../../spec/phase2-document-publish.spec.md)、[主agent能力Spec](../../spec/phase2-agent-capabilities.spec.md)：独立规划身份、固定worktree、写入guard、受控工具、人确认和部分副作用恢复。
- 当前源码的 `contracts/conversation.go` 必填Execution/Attempt，`persistence/ports.go` 同时引用任务与执行纯值，`clientcontracts` 仍公开Domain/Runtime：用于核对迁移风险，不把当前约束误当作未来规划来源的契约。

## 确认的问题与修正

| 编号 | 原稿问题及影响 | 修正与归属 |
| --- | --- | --- |
| A01 | 领取与结果交接未明确同Task多个Workflow；后续候选可能再次挑选/领取，执行命令可能退回按Task找最新Run | 区分Task挑选与执行候选；首次领取、后续新增及精确Workflow/Execution身份沿 #37；M01/M02/M04负责迁移时保留 |
| A02 | 单个候选结果与任务级结果未充分区分；迁移可能以停止/失败关闭Task，或遗漏多个PR及迟到合并 | 候选事实留执行，任务只应用已核验唯一交付；保留集合版本竞争、歧义、交付后资源与冲突；M02/M04/M10覆盖 |
| A03 | 三条线及目录没有说明规划业务的独立归属，易把主agent当worker或把文档Publication放进执行PR发布 | 明确未来planning拥有提案/会话/guard/文档发布；受控草稿与人工命令分开，协议复用不复用worker权限；实际功能仍归 #43/#46–#48 |
| A04 | 全部会话纯值归执行而未区分worker来源与规划session；当前必填Execution/Attempt可能迫使未来伪造身份 | 会话规范按来源作用域组织；worker保持强身份，规划使用自己的session/turn/workspace/generation；M01审查归属，#47实际增加支持 |
| A05 | 产品层仅写Go CLI/TUI唯一客户端，未说明C#/WinUI协议边界与Git文档版本 | 明确Go客户端和跨语言产品语义区别；HTTP/SSE归 #45，WinUI归 #44及页面，planning保持document/proposal/graph独立版本；M05/M08核对接入 |
| A06 | task允许依赖storage/ports，但矩阵又传递禁止执行纯值；共享ports引用执行记录时规则无法同时成立 | 业务/具体实现禁依赖继续传递检查；纯值间接依赖与实际端口消费分别审查，task不直接引用执行模型、不获得Execution/PR/session记录或聚合写权限 |
| A07 | 十阶段结构链缺少与产品优先级的明确安排，可能误读为候选、API和UI必须等待全部整理 | 明确 #37–#39优先、候选模型稳定后全面迁移；#43/#44/#45可独立推进，不改产品任务固定前置；M01–M10只约束本目标 |

## 检查范围与限制

后续产品范围已按用户确认收缩：一期稳定任务做人工配置实验，数量仅为开始时5个检查，允许竞态偶尔超额；合并前单候选放弃立即退出实验，停止事实独立，已知合并后拒绝放弃。二期独立审核同时检查Spec质量与前置成熟度，见[草稿准入目标](../../roadmap/phase2-draft-roadmap-admission.md)。原A01–A07是结构审计问题，不能把其中旧single/multiple或无数量检查描述作为新功能依据；具体一期范围以修订后的 #37–#39 Spec/Plan为准，本文不宣称新增能力已实现。

已核对愿景、阶段规划与既有Spec的身份、领域、权限、交付、版本、客户端和依赖语义，并修改roadmap的阶段交付、产品对齐、依赖矩阵及完成标准。相对链接、阶段依赖表与Mermaid一致性、无环关系和文档差异做静态检查。

本次没有创建新Spec/Plan、预留任务ID、修改运行时DAG、改实现或运行产品测试。主agent权限仍须 #43原生证据，多候选竞争仍由 #37–#39交付，WinUI/API/规划闭环仍由 #43–#53各自交付；文档中的未来路径不是已存在能力。后续实现Plan必须按最小必要范围补具体命令与证据，不能用本次文档审计替代行为验证。
