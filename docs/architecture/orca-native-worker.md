# Orca 单终端 worker

2026-10-10 的当前接线使用 host=orca 与 orca_terminal_v3。本机与 environment 共用同一实现和 CLI 端口；Orca 只承载 worktree / terminal 原语，上层共用 development/v2 的短提示词、报告、发布及交付检查。共用实现不代表两个目标的原生准备条件或实测结果相同。

工作区创建时激活唯一原 agent 终端，不投递提示词；每回合先保存输入摘要、实际输出来源及游标，再一次 terminal send --enter。CLI 按固定来源读取带 Command 标识的紧凑 base64 结果，原终端空闲、输入框无 draft 且完整结果匹配后交由上层。stream 要求游标连续；screen 要求完整结果帧可见，不承诺会话全文。身份变化、来源改变、实际缺口或冲突保持未核实，不补 Enter、不重发或寻找替代终端。准备与发送前均拒绝非空原生 draft；主机原生目录信任、模型和账号仍须准备好。

没有辅助终端、文件回传、HTTP 回调、Orca Run / Task / Dispatch / coordinator / worker_done，也不读取代理私有日志或构造代理模型命令。完整全文与原生强制只读不在该路径能力内；报告只比较两个主机均能由 CLI 获得的 revision 事实。旧 supervised 实现和专属夹具直接删除。

当前验证范围见[真实调试记录](../acceptance/orca-terminal-remote-retest-2026-10-10.md)：本机三阶段/真实 PR 合并/唯一交付仅有限样本通过，本机最后的 draft 保护由单元和两目标受控回归覆盖，未重新跑本机真机；远端升级 Codex CLI 并预先确认目录信任后，有限样本三阶段/中心重启/真实合并交付通过。首次目录信任正确阻塞零业务投递，但被拒绝原任务的自动继续仍未通过。紧凑回传是绑定 Command 的 worker 声明结果，不是原生 turn/final 全文。历史 supervised 的规范 final 缺口不再作为当前单终端能力描述，但其原始失败保留。

[Spec](../spec/orca-terminal-transport.spec.md) · [Plan](../spec/orca-terminal-transport.plan.md) · [验收与待验项](../acceptance/orca-terminal-transport.md)。普通开发由当前聊天实施，不进入 roadmap，也不改安装环境。
