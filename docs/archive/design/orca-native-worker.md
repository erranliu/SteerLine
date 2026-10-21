# Orca 单终端 worker

> 归档于 2026-10-21：迁出前 Orca 单终端实现说明；执行现归独立 SteerRunner，当前接入见[入口](../../integration/steerrunner.md)。固定 commit/原路径及证据内容继续按原记录取证。

2026-10-14 核对 `5a24abec`：当前接线使用 host=orca 与 orca_terminal_v3。本机与 environment 共用同一 adapter；Orca 承载 worktree / terminal 及原生主代理状态，上层共用 development/v3 的提示词、报告、发布及交付检查。当前完成证据只支持 Codex 主代理，不能从可配置 agent 字段推定其他 agent 已可执行。共用实现不代表两个目标的准备条件或实测结果相同。

工作区创建不携带业务 prompt；沿唯一原终端完成冻结的 agent 启动命令及原主代理就绪核验，准备回执和业务输入分别持久化。工作区创建/表面准备的有界等待及只读恢复见[启动 Plan](../../plans/orca-workspace-startup.plan.md)。每回合先保存 Command、原请求、完整输入摘要、主代理基线和 runtime/terminal/incarnation/pane/provider 身份，再一次 terminal send --enter；input_accepted 与 turn_started 分别核验。当前不采用屏幕draft/tui-idle作为准入或完成证据，见[核验收敛记录](../../acceptance/workflow-verification.md)；目录信任、模型和账号须在目标主机准备好。

原生订阅用于及时提示变化，独立 worktree.ps 补查询核验本轮新的主代理 done。working 观察是可选证据；原请求已接受且明确开始、有效发送前基线与新结束记录均匹配时，可在没有 working 通知的情况下结算。输入接受后每 30 秒有界补查，新 done 提示可立即查询；订阅不可用不阻断独立查询。结束与正文分开判断，只保存可证明完整的本轮普通 lastAssistantMessage，不以屏幕、文本变化、Worked for 或模型标记猜测完成。原生字段截断边界、冲突正文、额外输入边界或身份变化保持未核实；native outcome 缺失仍为 unknown。详见[当前完成规则](../../spec/orca-completion-query.spec.md)。

没有辅助终端、文件回传、HTTP 回调、Orca Run / Task / Dispatch / coordinator / worker_done，也不读取代理私有日志或构造代理模型命令。提示词与本地流程共用，不添加 SL_RESULT/base64/nonce 回传指令。完整过程全文与原生强制只读不在该路径能力内；报告只比较两个主机均能由 CLI 获得的 revision 事实。旧 supervised 实现和专属夹具直接删除。

验证分开记录：2026-10-10 的[历史真实调试](../../acceptance/orca-terminal-remote-retest-2026-10-10.md)证明当时实现的有限样本，不作为现行普通正文/主代理判定的验收。后续[状态切换验收](../../acceptance/orca-native-state-transitions.md)中远程首轮真实 PR 已交付、本机首轮未完成，第二轮取消；[完成补查询验收](../../acceptance/orca-completion-query.md)覆盖两端短回合仅查询、恢复/反例及共用阶段回归，未重跑真实 PR 发布。完整正文指本轮可核验 final，不是会话全文；历史失败和未验保持原记录。

[单终端基线](../../spec/orca-terminal-transport.spec.md) · [当前完成 Spec](../../spec/orca-completion-query.spec.md) · [Plan](../../spec/orca-completion-query.plan.md) · [验收](../../acceptance/orca-completion-query.md)。普通开发由当前聊天实施，不进入 roadmap，也不改安装环境。
