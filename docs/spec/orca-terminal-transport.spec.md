# Spec：Orca 单终端短提示词传输

普通开发，由当前聊天直接实施。用户确认一个终端、本机 Orca CLI 投递和读取；不引入辅助终端、远端回调或输入/结果文件。旧 supervised 直接删除。[Plan](orca-terminal-transport.plan.md) · [验收](../acceptance/orca-terminal-transport.md)。已 rebase 主干 `872dba44`，保留最新短提示词与发布分支核验。

## 职责与流程

共用 development/v2：implement.execute → implement.report → publish → await_merge；发布回复缺少结构化 PR URL 时才追加一次 publish.report。短业务提示词、报告重试、候选 SHA 的快照核验、PR 真值与 TaskDelivery 均归上层。Orca 只执行 worktree / terminal 原语，不解业务结果 JSON，不自行派生阶段或选择后继。

host=orca 本机与 environment 使用相同能力。全部调用由本机显式 CLI 发起，远端仅增加固定 environment。工作区创建用固定 base、独立 name、setup=skip、opaque agent 标识和 activate，不传 prompt；只采用其唯一原 agent 终端，不建 coordinator、Run、Task、Dispatch 或辅助终端。激活用于让原生会话可操作，不创建第二个终端。模型、权限和原生启动方式由执行主机 Orca 管理，不增加代理专用参数或日志解析。

## 输入与结果

共用 worker 根据 CompactTerminalReply 能力为输入增加简短结果约定，冻结实际 TurnInput 后交给 adapter 原样发送。输入连同约定最多 2000 字符，超限拒绝，不截断、分批向 agent 粘贴或改用文件。Spec / Plan 本来位于固定工作区，由 agent 自己阅读。

最终回复是有界的单行 SL_RESULT / SL_END JSON 帧，包含本轮 Command 作为 nonce、succeeded/failed 和业务回复 UTF-8 正文的 base64 reply_b64；含定界符的整帧最多 2048 UTF-8 字节。指令没有完整有效帧示例，输入回显不当作结果。JSON 拒绝重复/未知字段和非法 outcome；不同轮次忽略，同轮不同结果保持冲突未核实。模型报告 JSON 原样作为 reply 文本交给上层既有解析器。

每轮只读 PrepareSession 保存原输出游标与实际输入摘要，再进行一次 terminal send --enter --wait-submit --retry-request。accepted 不等于 turn_started。服务恢复沿用已保存的游标，不重新捕获 latest、不补 Enter、不重发输入，也不根据标题查找替代终端。

准备和投递前都核对原生 idle、身份及显式 screen 的 draft。输入框不为空则拒绝本次投递，不将业务输入拼接到用户或原生设置输入之后。draft 不属于输出正文。目录信任、可用模型与账号由原生主机预先准备；当前 Orca idle 对启动菜单可能误报就绪，空 draft 也不能证明不存在菜单，此能力缺口必须据实登记，不能以代理专用菜单匹配或自动补 Enter 隐藏。

通过 terminal read 的每轮固定来源读取，固定每轮初始 source；stream 按原游标分页，拒绝游标重置、缺口和裁剪；screen 显式使用 --screen，完整帧必须在当前视图中可见，limited 和来源替换均拒绝。screen 的 truncated 只表示屏外历史，不能据此否定当前完整帧，也不声称 screen 有完整会话历史。ASCII token/base64 的显示换行空白归一化后，业务正文的原 UTF-8 字节保持不变。只保存有界未完成帧、唯一结果及读取游标；原 runtime / terminal / incarnation / agent / worktree / branch 在读取前后均一致，完整帧匹配且原终端 idle、无未提交 draft 后才归一化为 worker_declared 结果。它不冒充 Orca 原生 session/turn/final。已结算正文随既有资源封套与业务事务持久化，不再依赖 live 终端或外部文件。

## 能力边界

不支持完整原生会话全文、原生问题路由、执行中反馈或 agent 专用模型覆盖。本机和远端都只通过 worktree show 比较 revision；inspection_scope=revision 不代表未提交工作区干净或报告强只读。中心停服只解除观察；明确停止时核对原身份后关闭唯一终端。ptyKilled 仅证明该 PTY 关闭，不扩大为所有子进程写入者消失。

旧 host=orca_supervised、旧 Orca 适配器/codec/Task-Dispatch 生命周期及其专属夹具直接删除，无兼容、迁移、双写或回退。移除本次未交付的 HTTP 与文件回传原型；历史探针证据仍为历史。
