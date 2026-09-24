# Plan：分阶段远程闭环与 viewer 决策

任务 ID：6；任务名称：分阶段远程闭环。执行依据：[Spec](03-remote-execution-and-viewer.md)。开发输入为 [远程协议 Spec](01-orca-remote-protocol.md) 的真实证据及限制、[中心模型 Spec](02-execution-model-and-adapter.md) 已交付的模型和接口；生产启用另受剩余协议门槛约束。

## 实施顺序

1. 核对中心模型、共同后端/会话契约、已测 Orca 版本与固定远端身份；建立隔离测试仓库和目标，不从 viewer 代启 Windows 服务窗口。
2. 实现受限 supervised 适配器的预检、固定 base/worktree、分步骤派发、消息/输出读取、问答与 Operation 对账。发送前限制结构化消息大小，生产远程取消入口返回不支持并保留活跃资源占用。
3. 在中心状态机中逐步执行 implement、verify、review、publish_pr；每步固定独立尝试及结果 revision，失败按预算返回修复，不让 agent 自行越过门槛。无 agent 脚本路径若采用，完成文件回执与重启后新终端取证。
4. 接入已有 verify 校验器：中心向其提交已接受的 worker_done 身份、预期检查清单和执行端 HEAD。缺回执、非零退出、身份或 commit 不符时阻止审查/发布，完成协议 P13 的端到端验收。
5. 复用 QueryService/CommandService 与会话读模型，在同一 viewer 展示阶段、缺口、待决策和真实应用证明。Windows 关闭服务窗口只结束中心监督；用户重新打开后按持久身份恢复消息和派发判断，不把远端 worker 误判已停。原生任务层若已提供未合并任务丢弃命令，则沿同一 Task 状态停止新步骤/结项，保留仍活跃的远端身份与人工清理提示；若尚未交付，先用接口替身验证，不把它当成远程取消能力。
6. 核验真实 PR 身份、head/merge 和结项来源回读。Beads/native 来源分别走已有权威事务；未支持的 completion 在预检拒绝，不转到中心同名路径执行。

## 验证与交接

- 用协议替身和存储故障覆盖 Spec R01–R19；真机至少完成 Spec 要求的 R01、R02、R06、R07、R08、R16、R18 及测试 PR 合并/结项路径。
- 生产启用前完成 [远程协议剩余门槛](orca-probe/remaining-gates.md)与 P13、中心模型交付及本 Spec 验收；真机版本、身份、残留资源和未测项分别记录，不以替身结果代替。
- 更新 hosting、状态、会话、决策和交付文档，运行相关 Go 测试、静态检查及 `git diff --check`；不扩大未验证版本、远程取消或消息大小能力。
