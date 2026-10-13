# Spec：Orca 折叠输入回显的回复采集

后续完成判定、状态 codec 与原生认证以[原生状态切换 Spec](orca-native-state-transitions.spec.md)、[Plan](orca-native-state-transitions.plan.md)及[验收](../acceptance/orca-native-state-transitions.md)为准；本文保留该次实现记录。

普通开发，用户于 2026-10-12 授权实现并完成最小验证后提 PR。[Plan](orca-collapsed-input.plan.md) · [验收](../acceptance/orca-collapsed-input.md)。补充并修订[单终端传输 Spec](orca-terminal-transport.spec.md)的可见输入门槛。

105 的 Codex 已完成实现，但公开 screen 将原提示词折叠成带省略号的一行，并显示前次工具输出的末尾。adapter 将这段显示内容与原提示词全文比较，拒绝完整的最终回复，工作流停在 implement.execute。

删除屏幕输入回显的全文比较。可见用户消息标记只划分结果区段，不证明原始输入文本；输入归属继续由持久化 Command/request/input digest、原 runtime/terminal/incarnation 和 input_accepted/turn_started 回执验证。

原用户消息边界、最终 assistant 正文和 Worked for 完成行仍须可见。提示词折叠、前置工具历史裁剪不阻断结果；最终正文缺失、折叠或非法 JSON 仍不能结算。保留发送前完成区段基线、原终端身份、开始事实、空 composer、idle 与两次至少间隔一秒的稳定观察。旧完成区段不得仅因区段外 header/footer 变化而结算新回合。

恢复只读同一原终端和持久化候选，不重发输入。worker 提示词、正文归一化、terminal_observed/derived 来源、执行目录版本、状态 codec 和数据库结构不变；不新增兼容或迁移路径。该方案以 SteerLine 专属原终端为前提，不宣称 screen 提供原生 turn/final 身份。

最小验收覆盖折叠输入及前置工具历史、旧回复/未开始/非 idle/draft/变化中正文/替换终端拒绝、候选恢复不重发、本机与 environment 受控三阶段及一个独立原生 screen 样本。
