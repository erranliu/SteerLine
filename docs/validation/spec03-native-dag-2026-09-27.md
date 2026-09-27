# Spec 03 远端原生任务与 DAG 验收（2026-09-27）

目标为本机 Orca 1.4.209、home Orca 1.4.210、远端 Codex CLI 0.157.1 及隔离的 `erranliu/SteerlineTest` 仓库。中心使用独立 clone、独立 SQLite 和 manual dispatch；任务 103 与后继 104 固定为一条 DAG 边。本次没有操作旧 Beads 状态或其他 Orca 终端。

## 原生任务闭环

中心领取 103 后，在同一 Orca Run `run_60b8b05cff18` 与工作树上，五个独立 Dispatch 依次完成 prepare、implement、verify、review、publish_pr。implement 只提交 `evidence/native-103.md`，候选为 `e28f0388d416d1b6c199ddcf173a44a7d1abbe13`；verify 对同一候选的 `identity`、`evidence`、`changed_paths` 三项均记录实际 exit 0。远端发布 [测试 PR #16](https://github.com/erranliu/SteerlineTest/pull/16)，中心按原 publish attempt 绑定；PR 以该 HEAD 真实合并，merge commit 为 `e7af046e6b04600fd7dee656afec8699fc75237f`。中心写入 TaskDelivery、关闭 103，104 从依赖未就绪转为就绪，保持 open，未自动领取。[闭环回执](../spec/orca-probe/samples/spec03-native-dag-2026-09-27.json)。

## 能力、断线与资源

当前目标的六项必需能力 `start`、`observe`、`workspace_inspection`、`output_reading`、`message_inbox`、`resource_settlement` 已用这次五个真实 Dispatch 的中心消息、精确归档输出和释放状态重新认证，并在隔离项目状态中保存带 SHA-256 证据引用的 CapabilityReport。本机/远端完整版本分别为 1.4.209/1.4.210；`stop`、`remote_cancellation` 保持 unsupported，未把 `output_turns`、`output_final_answer` 等未测能力提升。[能力样本](../spec/orca-probe/samples/spec03-native-dag-capability-2026-09-27.json)。

一次控制回执丢失使原 prepare 领取停在 `outcome_unknown`，中心保留原身份和 blocker，没有重派；原 Dispatch 完成后正常 release。完成闭环的隔离 run 又在 review、publish 的 Enter 接受后暂缺 `turn_started` 时保留原 attempt 为 unknown。分别核验原 worker 完成和精确终端释放后，重开**同一**中心 SQLite，仅对账原结果，review 与 publish 均转为 succeeded；没有第二个 Dispatch。五个 worker 最终均 `completed/succeeded/released`、终端 exited，归档 transcript 可按原 Dispatch 读取，工作树保留。[故障与恢复回执](../spec/orca-probe/samples/spec03-native-dag-recovery-2026-09-27.json)。

## 宿主权限

在 home 的独立 `workspace-write -a on-request` Codex 终端，请求提升权限运行只读 `whoami`。菜单把一次性批准、永久放行和拒绝分开；通过远端 computer-use 选择**一次性**批准后，命令返回 home 用户身份，未修改文件，测试终端随后精确关闭。Orca 的 `agentWait` 在最终轮次后仍残留旧 prompt-text 值，不能单凭该字段判定仍在等待。[权限样本](../spec/orca-probe/samples/spec03-native-host-permission-2026-09-27.json)。

本轮将中心回复的长度检查改为**序列化后**计数，并预留 512 字节平台封套空间；定向测试用只有 4000 个换行字符的答案验证转义后超限会在 Orca 观察或发送前拒绝。该测试是协议替身上的发送入口检查，没有在活跃 home 问题上制造超限发送。仍未取得管理员/UAC 设置弹窗的完整可见回执、结构化消息在真实远端发送入口的超限前拒绝，以及 home runtime 换代下活跃 worker 的专项复测。上述验收不提升远程取消或精确 turn/final 能力。
