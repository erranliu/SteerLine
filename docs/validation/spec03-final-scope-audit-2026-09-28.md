# Spec 03：第一阶段当前目标最终范围核对

核对对象是有人值守的远程执行 MVP：中心 Orca 1.4.209、目标 `台式机64g` Orca 1.4.215、原生 minimal-v1 五步、人工启动/问答/合并、每任务单候选。现行 viewer 门槛为当前主机 **4 秒**；早期 `home` 的 2 秒回执仍按其原测量范围保留。已通过且未受实现变更影响的版本族、SQLite 和本机仲裁回执复用，不为凑次数重跑。每项原始试次及失败边界见[R01–R19 逐项矩阵](spec03-fault-matrix-2026-09-28.md)。

| 门槛 | 当前核对依据 | 边界 |
| --- | --- | --- |
| R01 正常问答到真实合并 | [台式机64g 原生五步与业务答案](spec03-desktop64g-followup-2026-09-28/README.md)；[SteerlineTest Task 105](spec03-viewer-task105-2026-09-28/README.md)的固定 head PR #23、原 Task close 和五次 release | 当前目标真机通过。 |
| R02 worker 已启动而回执丢失 | [原 worker-start 响应丢失与精确结算](../archive/spec/retired/orca-probe/samples/spec03-supervised-r02-worker-start-response-loss-2026-09-26.json) | 原身份、unknown 与不重派规则在已测版本族复用；本轮未声称当前主机再注入一次丢包。 |
| R03 收消息但本地事务未提交 | [原矩阵](spec03-fault-matrix-2026-09-28.md)中的 SQLite 提交/ack 回归 | 中心存储边界，未因目标主机变化改写。 |
| R04 提交后 ack 前退出 | [原矩阵](spec03-fault-matrix-2026-09-28.md)中的原 Delivery 重放和幂等回归 | 同一 Delivery 身份；不把日志文字当 ack。 |
| R05 双 viewer 答案仲裁 | [#3 验收与原中心交付](phase1-task-closure-2026-09-28/README.md)及[原矩阵](spec03-fault-matrix-2026-09-28.md) | SQLite 单写者保证首答，未声称两个远端桌面同时可见。 |
| R06 答案离线与 reply 回执丢失 | [原问题/Attempt 的真机双故障](../archive/spec/retired/orca-probe/samples/spec03-supervised-reply-loss-2026-09-25.json)及[原矩阵](spec03-fault-matrix-2026-09-28.md) | 同一受测协议版本族复用；未知状态仍不得补发。 |
| R07 步骤完成窗口停服 | [home 各步骤完成窗口](spec03-fault-matrix-2026-09-28.md)；[当前主机三个活跃 worker 停服重接](spec03-current-host-affected-2026-09-28/README.md) | 当前主机样本证实原 Run/Dispatch/Attempt 不重派；#15 虽在 Orca 显示 completed，采样时中心未确认推进，不能算新增离线完成成功。旧完成窗口仍按版本族证据复用。 |
| R08 网络/runtime/身份变化 | [home runtime 更替及旧绑定拒绝](../archive/spec/retired/orca-probe/samples/spec03-supervised-r08-runtime-replacement-2026-09-26.json)；[当前主机身份不可核实与精确人工结算](spec03-current-host-affected-2026-09-28/README.md) | 当前主机观察为 `identity_changed`/`exactWorker=false`，未自动重派；不声称 Orca 自发产生同句柄新 incarnation。 |
| R09 无结果、错身份、重复完成 | [原矩阵的受监管结果负向回归](spec03-fault-matrix-2026-09-28.md)；原 #6 当前主机错误 `workflow_run_id` 的 status 被中心拒绝并保留原 unknown Attempt；[PR #107](https://github.com/erranliu/SteerLine/pull/107)收紧 sender/结果身份 | 负向拒绝真实发生；失败 Run 不计交付成功，另由[新 Run](spec03-task6-closure-2026-09-28/README.md)独立完成。 |
| R10 输出分页与缺口 | [原矩阵](spec03-fault-matrix-2026-09-28.md)及[协议输出真机](../archive/spec/retired/orca-probe/samples/p03-output-2026-09-24.json) | 仅承诺已保留内容可读，缺失显式标记。 |
| R11 较早 Attempt 消息迟到 | [原矩阵的跨 Attempt 回归](spec03-fault-matrix-2026-09-28.md) | 旧消息归旧身份；不改变当前结果。 |
| R12 合并/未合并/部分结项 | [Task 105 固定 head 真实合并与原 TaskDelivery](spec03-viewer-task105-2026-09-28/README.md)和[原矩阵负向回归](spec03-fault-matrix-2026-09-28.md) | PR 合并必须由 provider 和固定提交核验。 |
| R13 旧来源/中心路径预检拒绝 | [原矩阵](spec03-fault-matrix-2026-09-28.md) | 当前原生 Task 权威，不启用 Beads/completion 回退。 |
| R14 viewer/服务窗口与取消不可用 | [原可见窗口故障回执](../archive/spec/retired/orca-probe/samples/spec03-native-r07-visible-window-2026-09-27.json)；[当前主机停服后原 Dispatch 仍可观察](spec03-current-host-affected-2026-09-28/README.md) | 当前样本是受控停服，不冒充再次关窗；远程取消始终 unsupported。 |
| R15 claim 回执与领取者变化 | [原矩阵](spec03-fault-matrix-2026-09-28.md)中的原生领取/身份回归 | claim 由中心 Task/版本权威核对。 |
| R16 verify/review 修复预算 | [真机有限修复循环](../archive/spec/retired/orca-probe/samples/spec03-supervised-r16-repair-cycle-2026-09-26.json)、[review 零预算阻断](../archive/spec/retired/orca-probe/samples/spec03-supervised-r16-review-budget-2026-09-26.json)和[当前主机 P13 两项负向](spec01-p13-steerlinetest-2026-09-28/README.md) | 当前主机负向是中心入口单字段注入，明确标注来源；长期无人值守成功率不归此项。 |
| R17 verify 后代码改变 | [原矩阵](spec03-fault-matrix-2026-09-28.md)中的 revision 与旧结果守卫 | 不使用 agent 摘要替代实际检查提交。 |
| R18 同 worktree 跨步骤 | [SteerlineTest Task 105](spec03-viewer-task105-2026-09-28/README.md)的五个独立 Dispatch/Attempt 与资源释放 | 当前目标原生闭环通过。 |
| R19 越步 PR 与发布回执丢失 | [原 publish Delivery 丢失真机对账](../archive/spec/retired/orca-probe/samples/spec03-supervised-r19-retry2-publish-2026-09-26.json)及[原矩阵](spec03-fault-matrix-2026-09-28.md) | 同一 PR/Delivery 固定身份，不重复创建。 |

当前主机可见 viewer：[业务 Decision 2.215 秒上界](spec03-desktop64g-followup-2026-09-28/README.md)、[步骤转换 1.334 秒上界](spec03-viewer-task105-2026-09-28/README.md)，各自满足 4 秒门槛。P01–P13 的当前支持范围与原中心 #3/#4 Delivery 已[另行结项](phase1-task-closure-2026-09-28/README.md)。

原中心 #6 已由[五步真实 TaskDelivery](spec03-task6-closure-2026-09-28/README.md)关闭，PR #110 的固定 head 经 verify/review、真实合并和中心事务核验。本表按已测目标与版本族范围结项；表中没有测出的当前主机自然故障或原样本局限仍保持其原意，不改写为额外通过次数。24 小时固定 stable、长期无人值守矩阵、资源趋势与 dev/stable 提升仍由 #32 验收。远程取消继续 unsupported。
