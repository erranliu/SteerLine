# 最终回复与必要会话事实

中心保存完整最终回复、session/turn 状态、宿主等待及采集缺口。问题、背景、选项和答案由 Decision 保存，发送意图与回执由 Command/Operation 保存。完整工具过程、普通消息、增量、终端正文和未知原始事件留在原生工具，中心不维护其副本。此次收口见[Spec](../../SteerlineFlow/archive/spec/completed/product-simplification-04-conversation-recovery.spec.md)及[验收](../../SteerlineFlow/testing/acceptance/archive/completed/product-simplification-04-conversation-recovery.md)。

## 原生采集

本地 Codex 沿原 `codex-app-server` 历史读取接口工作。先核验原 session、workspace 和 Codex home，再以有界 turn 页读取必要状态与 final。只有明确 `agentMessage.phase=final_answer`、原生 turn 有非空 `completedAt` 且正文实际存在时，中心保存完整正文。显式空字符串是完整空回复；缺失 text、未知 phase、失败或取消且没有 final 均保留真实状态与明确缺口，不采用最后一条文字冒充 final。

Orca 本机/environment 通过同一原终端的请求回执与主代理状态核验本轮结束，再保存完整普通 `lastAssistantMessage`；当前只支持 Codex 主代理。订阅和独立 `worktree.ps` 查询核对发送前基线、原 runtime/terminal/incarnation/pane/provider 及原输入；working 通知不是必需条件。无 Task/Dispatch、输出材料 relay、base64 标记、屏幕完成推断或 agent 私有 home/history 扫描。正文缺失、可能截断、原身份/输入归属或正文冲突时报告缺口，不推进下一阶段；原生 outcome 缺失仍保留 unknown，不能因正文受理推定工具或 PR 成功。已受理原文及摘要不可替换，最终正文沿现有规范会话提交保存。详细规则与证据见[完成 Spec](../../SteerlineFlow/spec/orca-completion-query.spec.md)及[验收](../../SteerlineFlow/testing/acceptance/orca-completion-query.md)。

正在运行的 turn 只保存原 turn 开始事实，不保存部分正文。结束但 final 尚未出现的原 turn 保留有界重访锚点；迟到的 final 仍属于原 turn，不新建会话。原生历史读取游标与最终正文在事务完成后推进；读取失败保存 gap，不推进游标。重复读取按原 source/session/turn/item 身份幂等。终态 final item 内容变化被拒绝，保留原正文并报告缺口。迟到的完整 final 在最终回复索引中解除该 turn 先前的缺正文提示；诊断查询仍可追溯该提示。

采集独立于阅读窗口，活动来源优先、历史来源按有界轮转读取。准备轮最终回复不作为业务最终成果展示；原始后端、候选、Attempt 和源身份始终核对。其他 agent/远端保持原支持与缺证限制，不从终端文字推断 final。

## 存储与阅读

SQLite 保存必要事件、不可变完整 final 正文和原生历史 checkpoint；删除 raw 表、增量 current-message 表、JSONL 会话 mapper 与独立日志重放队列。当前格式来源见[数据模型](data-model.md#当前格式来源)，旧开发数据库/事件/查询游标明确拒绝，不迁移或双写。

产品读取只提供 `ReadConversation`、`ReadMessageBody` 与必要能力报告。事件页默认 200、上限 1000、总 envelope 1 MiB；正文上限 16 MiB，按需以最多 256 KiB 的 UTF-8 安全页读取，绑定原 execution/attempt/item、不可变正文摘要与 revision。正文缺失、损坏、超限和截断均不得报告完整。中心没有 raw 读取/导出器，也不从日志回填。

控制台保持当前三窗及最近活动摘要；`f` 按需读取完整最终回复，[ / ] 查看已采集历史最终回复。预览仍最多 4096 字节、800 字符及六行；失败/缺口使旧预览失效，异步结果不能跨项目、候选、Attempt、turn 或正文身份。完整原生过程通过已有 session/terminal/日志定位到原工具查看。

## 操作与诊断边界

`appserver.jsonl` 及原生操作日志仍供工具请求、原答案结果、停止与恢复对账使用；删除的是重复会话存储和独立重放，不删除这些协议证据。宿主 wait/gap 仅保存事实，不保存附带终端正文，也不直接创建业务 Decision。Activity、最终正文、Decision 和交付事实各有原所有者，不互相冒充。

UI 操作 journal 继续保存草稿与原请求身份；未知只查原请求，不重新发送。不同窗口的原有隔离保持。本次未采用单窗口、文字问答格式或服务启动方式取舍。

原生样本的历史能力证据只证明原测试时点；当前采集/恢复收口的实际结果与未执行范围由本项验收记录维护，controlled 不作为 native/live 证明。
