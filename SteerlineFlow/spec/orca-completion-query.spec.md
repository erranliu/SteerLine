# Spec：Orca 普通终端完成补查询

日期：2026-10-13。普通开发，用户已授权实现；不占 roadmap Task ID。

[Plan](../plans/orca-completion-query.plan.md) · [专属验收](../testing/acceptance/orca-completion-query.md)

## 问题与范围

SteerlineTest 105 的原请求已有 input_accepted/turn_started 回执，原主代理已 done 并保留完整报告；适配器未捕获 working，因此既不能补查询，也不能接受结束事实。本次替代 [原状态切换 Spec](orca-native-state-transitions.spec.md) 中必须观察 working 的门槛。

保持普通终端、统一阶段提示词和现有 worker 职能/接口。只修改 Orca 原生协议解码、适配器完成判定、补查询及必要验证入口；不使用结构化对话、屏幕核验、agent 私有日志、模型标记、受监督结算或历史消息平台。不扩展当前 Codex agent 支持范围，不修改安装环境或恢复旧任务。

## 受理规则

1. 保存发送前原主代理的 idle/done 基线及原 runtime、terminal/incarnation、pane、provider session，输入摘要和原请求 ID。原请求回执必须绑定同一实例，明确 input_accepted 且 turn_started；仅 accepted 不足以自动结算。
2. working 是可选观察证据。即使所有状态通知均未消费，也允许通过 worktree.ps 查询同一原主代理的新 done。必须核对原实例、发送前有效基线、当前输入的 prompt 预览、主代理结束时刻严格晚于基线、快照未早于其主代理记录，并排除明确的 sessionBoundary、restoredUnconfirmed、中断/非成功 outcome 和工具输出正文。未知 native outcome 保持 unknown。
3. prompt 预览只是辅助匹配，不能替代原请求回执。同一原终端一次只有一个 SteerLine 输入在途，下一输入须等本轮结束及回复受理。无论来自可信通知还是补查询，观察到本轮新 working/done 的额外输入边界、不同 prompt、初始化/未确认恢复，或实例/provider 变化、冲突正文时，保存不可清除的拒绝事实；后续 waiting、匹配 done 和 worker 重启不能覆盖它。没有原生轮次 ID 时不能证明未观察到的人工同文输入；不宣称覆盖任意外部并发输入。
4. 不补造 working 时间。完整结束记录本身在上述绑定条件下证明本轮执行发生并停止。只有已接受且明确开始的原请求才能向现有 worker 返回 Started/Completed；已保存的结果重读不重复交付。
5. done 和正文分开核验。正文为空或达到原生 8000 UTF-16 单元截断边界时继续补查；完整原文及其摘要固定后不可被不同正文覆盖。相同正文可以来自不同轮次，不以文本变化作为完成依据。

## 补查询与恢复

输入接受后每 30 秒对原主代理执行一次有界查询；本轮新的 done 提示可立即查询。每个观察器仅一项查询在途，重复提示不加速查询。记录最近查询尝试，查询失败也按间隔重试，不由工作状态更新无限推迟。

订阅仅用于及时发现变化；订阅打开失败、断开或缓冲缺口不阻断独立 QueryWorktreeAgents/QueryState。查询前后保留原 CLI 实例核验。订阅打开、接收和查询任一处的明确原身份/额外边界/正文冲突是拒绝条件，不能靠重连清除。网络错误、查询页暂缺原记录或 provider 元数据暂缺是可重试诊断，不据此推断身份替换；成功核对可以解除。公开 worktree.ps 目前只取限额页，无按原 worktree 的定位参数，缺行时继续等待查询证据，不宣称页外记录一定可达。订阅与查询不能产生第二业务写者。

worker 重启后，使用保存的原请求、基线、查询时间和冲突事实恢复只读观察；原代理离线期间完成也能补查结算一次。后续业务输入仍遵守已有 worker 恢复确认门槛，补查询不得绕过该门槛。恢复首读失败、未交付观察器时由适配器关闭已打开的订阅。

持续无法核实时沿既有 InputWait/Diagnostic 返回明确原因；保持原请求和最后可信事实，允许后续只读重试，不重发输入/Enter、不创建替代终端、不声称原 worker 已退出。CommandTimeout 是每次读操作的期限，不误作模型整轮执行期限；本次不新增业务总期限或配置。

## 验证范围

最小自动验证覆盖 Q-A01（状态规则/恢复）、Q-A02（共用阶段推进）、Q-A03（真实本机/远端短回合只查询）。步骤、命令、通过条件和证据位置见 Plan；结果仅登记专属验收。实际字段不能核实新旧结束/初始化边界时记录失败，不继续放宽受理。
