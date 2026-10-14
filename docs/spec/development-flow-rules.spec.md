# Spec：固定开发流程规则独立

任务 ID：94；任务名称：固定开发流程规则独立。

状态：2026-10-14 草稿规划，未纳入执行、未实施。配套 [Plan](development-flow-rules.plan.md)、[专属验收](../acceptance/development-flow-rules.md)、[本轮草稿](../roadmap/drafts/roadmap.md)。调查基线 `5a24abec3c10d184a6e57962e154854bc6011a9f`；实际启动另固定文档与代码版本。

## 问题与目标

[staged_flow.go](../../internal/execution/staged_flow.go) 的结果消费同时调用会话提交、生命周期、PR查询与 actor 提交；[staged_result.go](../../internal/execution/staged_result.go) 的严格结果解析实际是固定流程规则。纯步骤变更仍须构造完整 Service。

建立 `internal/execution/development` 纯规则 owner，收拢 development/v3 的严格业务 JSON解析、步骤结果判断、报告重试、基础提示词/结果指令和下一 StageInput 构造。规则接收固定输入/源结果、当前步骤、已核验本轮事实、报告权限描述及 actor 提供的继续资格/原因值，返回具体的结果与下一步骤/输入声明，不执行动作、不提交状态。当前仅一个固定流程，不新增 DSL、插件注册、通用 effect/事件框架或第二份步骤状态。

## 规则与 actor 的分工

| owner | 范围 |
| --- | --- |
| development | 本次严格结果类型/解码、implement/report/publish/publish.report 的结果与源绑定规则、允许的重试及本轮下一输入正文/步骤声明；输入值独立复制，调用不修改调用方 slice/map/记录 |
| execution actor | 原会话/Attempt/Command/turn 和 final 来源核验、native outcome与业务 outcome区分、Decision/恢复/环境资格、PR查询/绑定/未知副作用对账、生成 ID和时间、将声明与已核验事实一次提交 |
| adapter/runtime | 原平台操作/原文/回执和规范事实，不解析业务 JSON 或选择下一步骤 |

规则不得接收 Service、可写 RunView、完整 Config、数据库或生命周期句柄；输入中的继续资格不是新业务权威，actor 仍核验并提交。传入实际规则所需字段，不转发整个 Service 或用任意 map 承载扩展协议。

保留 `worker-flow-v2` 与 `development/v3` 两个版本轴、现有字段/模板/大小限制、重复 first-wins 和冲突拒绝。正常 report 最大一次自动重试，publish 格式失败仅补一次 publish.report、不重做发布；源失败不能被报告成功覆盖。当前无独立 artifact 快照，已提交 report 的继续仍按既有原会话重核规则。结果、状态转移和下一输入同事务提交，失败回滚，不先发布规则结果再补写。

严格解析保持当前含义：拒绝重复JSON键、被消费业务字段的大小写别名冲突、错误字段类型及尾随多个对象/正文；额外说明字段仍忽略，完整json/普通代码围栏、CRLF和既有业务字符串trim仍接受。忽略的metadata中不同大小写的不同解释键不被强行合并为业务字段，同名重复键仍拒绝。不增加DisallowUnknownFields或从正文抽取某段JSON；以既有TestWorkerStageStrictResult和TestWorkerStagePresentationDoesNotBlockResult的拒绝/接受样本为回归基准，提取不是改变模型输出容忍度。

## 与交付及运行时的共同边界

本项拥有 staged_result 纯规则、staged_flow 的规则接线及所属测试。`dispatchQueuedStage`、`reconcileStageFacts`、`acceptStagePublication` 中的外部调用、PR绑定和最终交付仍由原 actor 路径负责，不整体搬入纯规则包。

发布未知时，原结果保持未知；只读对账发现原绑定 PR 已真实合并，仍可进入既有最终交付核验，不能新增“必须先有成功 publish 结果/所有步骤完成”的结项门槛。#95 仅替换最终交付用例；本项保留该调用入口和输入/错误语义，不修改 DeliveryGate/ResultCommit/schema。共享流程字段、提交屏障和资格规则冻结。

无新增硬前置，可与 #92/#93 并行。与 #95 在既有调用契约下可并行；默认建议先 #94 后 #95 合入，由后合入者复验组合。与 #96 仅有共享 actor 接线的集成顺序建议，不设置 #94→#96 的任务硬依赖；#96 不改本项规则接线。

## 最小验收

| ID | 自动通过条件 |
| --- | --- |
| DF94-A1 | 纯包无 I/O/业务能力、可脱离 Service 独立测试；解析/基础输入构造单一实现，拒绝歧义且保留额外说明字段/围栏/trim的接受行为，旧重复规则删除，调用方值不被修改 |
| DF94-A2 | 正常三轮、report 重试上限、publish.report 单次、源失败/needs_input/blocked/unknown、重复和错绑定、恢复/异步问题继续资格保持 |
| DF94-A3 | 真实 actor 结果/转移/下一输入提交原子性保持；发布未知但真实合并的原路径继续可交付，错误/替代 PR仍拒绝 |
| DF94-A4 | 本地 Codex 与 Orca local/environment 受控三阶段接线保持原会话/Attempt、输入次数、正文和业务阶段；规则不依赖后端实现 |

自动项不要求新增付费或人工验收；结构测试和 controlled 不扩大为 native 通过声明。详情归专属验收。
