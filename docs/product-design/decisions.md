# 问题、反馈与恢复输入

Runner 拥有原生问题、会话、输入和执行状态。SteerLine 将其公开 question、controls 和 revision 投影为界面 Decision，保存调用方请求与投递进度；Decision 不是另一套持久原生问题或执行状态机。本文维护输入含义与受理边界，通用生命周期见[Worker 生命周期](worker-lifecycle.md)。

## 问题与答案

Runner 在原任务范围内调查和解决问题；需要补充任务依据、改变任务范围或协调其他任务时，按[职责边界](project-workflow.md#4-steerline-与-runner-的职责边界)交回 SteerLine 组织解决。当前通道负责展示问题、保存和投递答案；主 Agent 首批可由用户手动分析材料问题并审阅提交原题澄清或创建 Spec/Plan 修订 PR，详见[主 Agent](main-agent.md)。自动补齐材料及跨任务协调仍未接入。

| 输入 | 当前含义与资格 |
| --- | --- |
| 原问题回答 | 对 Runner 公布的原 question 回复；是否可回答、选项及自由文字能力由当前公开事实决定 |
| 业务反馈 | PR 就绪等允许反馈的位置，显式向原任务提交非空文字；不把等待合并变成必答 Continue |
| 恢复输入 | 仅在 CanResume 允许时显式选择恢复；不夹带业务文字或代替原问题回答 |
| 后续消息 | 在 CanMessage 及调用方业务资格允许时沿原 Run 发送，例如手动解决合并冲突；输入串行和结果由 Runner 管理 |

调用方同时核验候选参与、停止义务、候选 revision、Runner revision、原问题和必要的 Attempt/恢复版本。旧草稿或新页面选择不改写原目标；缺原问题、过期版本或不支持能力时明确不可提交。原生后端的提问路由和恢复能力取所选 Runner 契约，不能从其他路径推定。

多题答案必须完整匹配原题集，每题按原 ID 提供答案；重复/额外 key、空答案、未提供选项或不允许的附加文字拒绝。结构化 answers 不与 option/note 混用。最终实际投递文字（含选项解析及多题 JSON）须为合法 UTF-8，最多 2,000 字节；超限在投递前拒绝，不截断。完整校验入口为 [InputForAnswer](../../internal/taskrunner/input.go)。

## 送达事实

SteerLine 在外部发送前持久保存完整请求及原产品请求到 Runner 请求的映射。同一产品 ID/内容返回原事实，同 ID 改内容拒绝；原答案和已受理结果不靠新请求覆盖。

| 事实 | 能说明什么 |
| --- | --- |
| 本地已保存 / pending | 输入意图已保存，尚不证明 Runner 或 Agent 收到 |
| unknown | 可能已经发生，沿原请求核实，不换 ID 重发 |
| Runner accepted | Runner 已受理原请求；后续 action status、原生开始与最终结果分别观察 |
| rejected / cancelled | 原请求被拒绝或尚未发送的授权已撤回；不改写已经发生的外部事实 |

pending、unknown 及未结算的消息不能与新输入并发绕过。接受、开始、执行完成和业务要求已落实分别展示，模型回复或自然语言总结不能代替原回执。接口、信封与回执查询见[本机协议](local-client-protocol.md)，浏览器 journal 与切换隔离见 [Web README](../../web/README.md)。

## 恢复与宿主事项

SteerLine 重启撤回旧未发授权，unknown 转为只读观察；不自动制造 submit/reply/message/resume。Runner 在原持久授权下补收结果或接续已证明未发送的后继，与调用方新授输入分别判断。查询缺失不授权创建替代输入；同一服务实例内的原 reply/resume outbox 仍按既有精确去重契约投递同 ID/正文，message 一经进入调用即只观察原请求。重启后的 observe-only 不因 not_found 恢复发送权。

原会话、工作区、版本或旧写者资格无法核实时保留具体阻断，不改绑新会话冒充恢复。人工退出候选后不得新投递反馈；Task 终态拒绝新业务输入，迟到事实仍需保存和收尾。步骤、报告、修复预算及下一阶段归 Runner，调用方不解释“继续”来选择 implement/publish。

补充信息或材料本身不自动授权新业务执行。澄清属于原目标和约定范围时，沿原问题及当前公开输入资格处理；需要改变目标、成果或依赖时，由 SteerLine 重新确认正式材料与任务依据，按更新后的契约固定执行输入，不通过一条回答改写原 Run 的冻结请求或扩大任务范围。

权限、登录和环境阻断与业务问题分开显示，处理条件不等于业务反馈。查看原生会话只定位已绑定目标，不表示批准、修复或新业务执行；具体动作按公开能力和授权开放。停止、重启及 v8 升级规则归生命周期和[Runner 接入](../integration/steerrunner.md)。

## 维护与来源

当前来源为 [taskrunner 输入投影](../../internal/taskrunner/input.go)、[runnerbridge 原请求控制](../../internal/runnerbridge/bridge.go)及[原 Run 续作验收](../../SteerlineFlow/testing/acceptance/pr-conflict-resolution.md)。旧 TUI 操作、中心 Decision/Operation 写表及 development/v3 分步规则从[历史入口](../../SteerlineFlow/archive/design/runner-reliability-history.md#第四批整理来源)追溯，不作当前操作说明。
