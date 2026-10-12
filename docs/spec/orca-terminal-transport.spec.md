# Spec：Orca 单终端短提示词传输

普通开发，由当前聊天直接实施。用户最新确认：Codex 与 Orca 同一 worker 阶段必须使用相同提示词，去掉 SL_RESULT、base64 和 nonce 的额外要求；屏幕换行、回复采集和完成判定属于 Orca adapter。一个终端，不引入辅助终端、远端回调或输入/结果文件。旧 supervised 直接删除。[Plan](orca-terminal-transport.plan.md) · [验收](../acceptance/orca-terminal-transport.md)。提 PR 时已重放主干 `5809c3e1`，保留状态命名、agent_launch 和发布分支核验。

## 职责与流程

共用 development/v2：implement.execute → implement.report → publish → await_merge；发布回复缺少结构化 PR URL 时才追加一次 publish.report。短业务提示词、报告重试、候选 SHA 的快照核验、PR 真值与 TaskDelivery 均归上层。Orca 只执行 worktree / terminal 原语，不解业务结果 JSON，不自行派生阶段或选择后继。

host=orca 本机与 environment 使用相同能力。全部调用由本机显式 CLI 发起，远端仅增加固定 environment。工作区创建用固定 base、独立 name、setup=skip 和 activate，不传业务 prompt；默认由原生 agent 启动。若上层冻结了 agent_launch 命令，在唯一原 shell 终端启动该命令并核实原 agent 身份；不建 coordinator、Run、Task、Dispatch 或辅助终端。激活用于让原生会话可操作，不创建第二个终端。模型和启动偏好可由上层 agent 编译器冻结为通用命令；Orca adapter 原样执行，不解释模型参数，不改变 worker 提示词。权限由启动命令与执行主机设置决定，不读取代理私有日志。

## 输入与结果

共用 worker 只生成阶段业务提示词和业务 JSON 结果要求。Codex 与 Orca 接收相同阶段的同一原始文本；后端不追加结果帧、编码、nonce 或传输长度指令。Command ID、原请求、runtime / terminal / incarnation 与输入摘要仅在 adapter 内部关联，不要求模型回填。

准备和投递前核对原生 idle、精确终端身份及公共 screen draft。PrepareSession 保存本轮原输入及发送前最近完成区段摘要；StartTurn 一次原样投递，记录 Orca 的 request、input_accepted 与 turn_started 事实。accepted 不等于开始，未知投递不通过重新输入恢复。

当前普通回复采集只验证 Codex TUI 的公开渲染边界：完整本轮输入回显、其后的最终 assistant 区段、Worked for 完成行及原终端 idle。两次间隔至少一秒的 idle 观察具有相同区段摘要、composer 空且开始已确认后，冻结回复。旧区段、错误输入、折叠/裁剪或缺少边界、未知 renderer 均不能证明完成。原生主机缺乏 session/turn/final 接口，因此来源记为 terminal_observed、身份为 derived，不冒充原生 canonical final；普通输出显示归一化后由同一 development/v2 业务流程解析。

适配层保留正文空格、中文和 JSON 转义，JSON 键及 URL 字符串内的显示折行拼接，正文字符串的显示换行转义保留，避免因 CLI 去掉行末空格而连接单词；JSON 结构外换行保留。screen 不提供软/硬换行元数据；若正文歧义、原输入或最终回复不完全可见，保持未核实。正文最多 32 KiB，是读取层资源上限，不注入业务提示词。恢复只读同一终端并使用已持久的输入/基线/候选/回复，不发送额外输入、不补 Enter、不替换终端。

这次状态 codec 为 2，旧紧凑结果帧/扫描状态删除，无旧版读取或迁移。当前验收见[统一提示词专属记录](../acceptance/orca-unified-worker-prompts.md)。

## 能力边界

不支持完整原生会话全文、原生问题路由或执行中反馈。Orca codec 不解释 agent 模型字段，上层冻结的启动命令继续按当前主干规则执行。本机和远端都只通过 worktree show 比较 revision；inspection_scope=revision 不代表未提交工作区干净或报告强只读。中心停服只解除观察；明确停止时核对原身份后关闭唯一终端。ptyKilled 仅证明该 PTY 关闭，不扩大为所有子进程写入者消失。

旧 host=orca_supervised、旧 Orca 适配器/codec/Task-Dispatch 生命周期及其专属夹具直接删除，无兼容、迁移、双写或回退。移除本次未交付的 HTTP 与文件回传原型；历史探针证据仍为历史。
