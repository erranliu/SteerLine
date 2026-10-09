# 远端 Orca 正式路径与 L3 双轮交付

本聊天直接授权开发；不由 roadmap-runner 调度，不改变已有 execution-map 的任务顺序。此前已分配 #85，保留该身份。本 Spec 扩展既有远端适配的当前支持范围，不合并本机 terminal-v2 与远端 supervised 两条协议。原一期默认测试范围仍保持本机。

## 正式执行契约

使用 host=orca_supervised、agent=codex。Profile 固定 environment、paired-device connection、远端已注册 Git repo、显式 model/effort；可选 bootstrap coordinator 只作显式预检，每个 Execution 创建自己的中心 coordinator。Orca Run/Task/Dispatch 与中心 Task/WorkflowRun/Execution 分属不同命名空间。

移除无条件“远端权限缺证”封锁，改为实际版本族、声明能力、保存连接、runtime、仓库 origin 及启动回执核验。远端由 Orca 使用该 host 的原生 agent 启动设置，不注入本机 Windows 参数、不探测中心 Codex、不声称具有本机 unelevated/auto_review 的等价权限。模型及推理强度必须在原生启动回执兑现；不支持、断联、身份漂移、未知副作用均明确阻断，不回退或重复派发。

首轮创建独立远端 worktree、中心 coordinator 与 Orca Run/Task，然后 worker-start --on。后续已结算反馈在原 terminal/incarnation/worktree 新建 Task/Dispatch；保存原历史身份。只以正式 PR/provider 合并事实完成唯一 TaskDelivery 和依赖解锁。中心不使用本机文件/Git API 操作远端路径。取消竞争、断联/重启恢复和问答不扩展为本次验收场景；既有安全拒绝仍保留。

## L3 场景

新增 delivery.orca-remote-codex（L3/live/delivery），固定 gpt-6-luna/medium，真实配对远端 Windows Orca 与已注册 erranliu/SteerlineTest。每次独立中心 clone/home/SQLite、随机 GitHub base/head 与 owned 远端 worktree；合同/脚本通过本次 Git base 到达远端。远端使用预备的自身 Codex/GitHub 认证，不传中心认证、不修改全局设置。测试驱动可自动合并本次精确 PR head；不改 main。

| 验收项 | 必需结果 |
| --- | --- |
| RO1-A | 远端 Profile 静态可选；缺显式 model、目标/版本/能力/仓库不符时拒绝；本机不承担远端权限证明 |
| RO2-A | 正式创建 A 与依赖 B、公开领取启动真实远端 worker；保存不同命名空间的精确资源与回执 |
| RO3-A | 首轮真实修改、验证、commit/push/create PR；中心 waiting_pr_merge，B 未释放 |
| RO4-A | 正式反馈完成第二轮、更新同一 PR；原 Execution/Attempt/worktree/terminal/incarnation/原生会话不变，Task/Dispatch 与原生 turn 不同 |
| RO5-A | 精确最新 head 的真实 PR 合并，唯一 Delivery、A closed、B ready 且未启动；无额外模型轮次 |
| RO6-A | 精确远端 session/cwd/model/effort、正数 usage 与两轮累计连续性取证；成功/失败均记录并清理本次 owned 资源，未知清理不判通过 |

实际结果统一登记于[专属验收记录](../acceptance/orca-remote-l3.md)。测试不绕过产品准入、不写 SQL 或制造 worker_done/模型 usage。默认不扩大为 full、L2 或全部 L3，也不把直接 Orca 探针算作正式产品场景。
