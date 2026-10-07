# #3 / #4 既有合并实现的验收索引

原中心 Task 创建前，实现 PR 已合并：#3 对应 [PR #39](https://github.com/erranliu/SteerLine/pull/39)（head `c3bc30f25a4834fe1f7f3e647d3740b95b106c81`，merge `a8aca9769a4687501a94718480c212b70c268592`）；#4 对应 [PR #16](https://github.com/erranliu/SteerLine/pull/16)（head `292483eb389b188d5d93d048cc587d6e8e50bada`，merge `1a9f95ba96880f2ff7a9aea36325c70b2653e783`）。中心的 `accept-existing` 命令仍须向 PR provider 读取完整合并身份、固定 head、合并时间和本地 merge 祖先，并验证原 TaskContract 与本报告的 Git blob。原生失败/unknown Run 保留，不改写为成功。

## #3：H-A1–H-A16

下表指向[逐项原记录](pr-05-intervention.md)。该文件保留早期试次的失败或局部结论；后续回执按各门槛单列。

| ID | 结论与证据 |
| --- | --- |
| H-A1 | 当前范围通过；十二选项、长文本、中文输入与粘贴见[原记录](pr-05-intervention.md)。 |
| H-A2 | 当前范围通过；选项、自由文本组合与校验见[原记录](pr-05-intervention.md)。 |
| H-A3 | 当前范围通过；同题双客户端仲裁见[原记录](pr-05-intervention.md)。 |
| H-A4 | 当前范围通过；陈旧草稿和轮次身份见[原记录](pr-05-intervention.md)。 |
| H-A5 | 当前范围通过；在线 UI/CLI 回答及离线边界见[原记录](pr-05-intervention.md)。 |
| H-A6 | 当前范围通过；真实活跃 worker 的[单字段 incarnation 注入](pr-05-h6-incarnation-2026-09-28.json)证明身份拒绝、保留原 unknown/占槽和重启不重派。平台自行换代未声称已观察。 |
| H-A7 | 当前范围通过；[合格本机重试](pr-05-recovery-confirmation-2026-09-28.json)和原资格回归见[原记录](pr-05-intervention.md)。 |
| H-A8 | 当前范围通过；受控重试确定性失败后服务仍存活见[原记录](pr-05-intervention.md)。 |
| H-A9 | 当前范围通过；结果冻结与 PR 身份校验见[原记录](pr-05-intervention.md)。 |
| H-A10 | 当前范围通过；无完整 Run 的阻断恢复见[原记录](pr-05-intervention.md)。 |
| H-A11 | 当前范围通过；旧状态拒绝和共用业务命令见[原记录](pr-05-intervention.md)。 |
| H-A12 | 当前范围通过；持久化边界与不重发回归见[原记录](pr-05-intervention.md)。 |
| H-A13 | 当前范围通过；能力/资格入口和未测路径拒绝见[原记录](pr-05-intervention.md)。 |
| H-A14 | 当前范围通过；[95012 同 Attempt 绑定应用证明](pr-05-intervention-live-gitfix-2026-09-27.json)。 |
| H-A15 | 当前范围通过；[原续跑回执丢失](pr-05-h15-response-loss-2026-09-28.json)和[同 Attempt 重启对账](pr-05-intervention-live-gitfix-2026-09-27.json)均未重发。 |
| H-A16 | 当前范围通过；[SteerlineTest Task 104](pr05-ha16-steerlinetest-2026-09-28/README.md)的真实可见 TTY 完成折叠、展开、再折叠和精确正文导航。 |

## #4：P01–P13

以下按[固定 Spec](../archive/spec/retired/01-orca-remote-protocol.md)中的编号索引已有记录。旧 home 目标的协议与版本族证据按原测量范围复用；台式机64g 的 UAC 和 P13 受监管负向另有目标专属样本。[协议汇总](../archive/spec/retired/orca-probe/remaining-gates.md)保存早期失败试次，不将其改记为通过。

| ID | 结论与证据 |
| --- | --- |
| P01 | 通过；[新目标独立能力与五步](spec03-desktop64g-2026-09-28.md)记录真实远端身份与非驻留执行。 |
| P02 | 通过；[远端固定 base 与仓库入口](../archive/spec/retired/orca-probe/remaining-gates.md)及[当前目标闭环](spec03-desktop64g-followup-2026-09-28/README.md)。 |
| P03 | 通过；[分页、缺口与 source_changed](../archive/spec/retired/orca-probe/samples/p03-output-2026-09-24.json)。 |
| P04 | 通过；[真实业务问题与同 Attempt 应用](spec03-desktop64g-followup-2026-09-28/README.md)。 |
| P05 | 通过；[受监管结果及失败回执](../archive/spec/retired/orca-probe/remaining-gates.md)。 |
| P06 | 通过；[暂断与原 Dispatch 恢复](../archive/spec/retired/orca-probe/samples/p06-transient-2026-09-24.json)。 |
| P07 | 通过；[中心派发、消息、回答边界重启](../archive/spec/retired/orca-probe/samples/p07-center-restarts-2026-09-24.json)。 |
| P08 | 通过；[有/无 request ID 响应丢失](../archive/spec/retired/orca-probe/samples/p08-response-loss-2026-09-24.json)。 |
| P09 | 通过；[当前目标五个 Dispatch 结算与释放](spec03-desktop64g-followup-2026-09-28/README.md)。 |
| P10 | 当前支持版本范围通过；[真实回复超 8 KiB 发送前拒绝](../archive/spec/retired/orca-probe/samples/p10-home-live-reply-size-2026-09-27.json)与[台式机64g UAC 人工拒绝/批准](spec03-desktop64g-2026-09-28.md)。 |
| P11 | 通过；[脚本文件回执、退出和重连](../archive/spec/retired/orca-probe/samples/p11-file-receipt-2026-09-24.json)，内建终端 stdout 丢失仍按已记录限制处理。 |
| P12 | 通过；[当前目标原生五步同工作树、逐步新 Dispatch](spec03-desktop64g-followup-2026-09-28/README.md)。 |
| P13 | 通过；[真实正向和非零退出负向](../archive/spec/retired/orca-probe/remaining-gates.md)，以及[SteerlineTest 两项中心入口单字段负向注入](spec01-p13-steerlinetest-2026-09-28/README.md)。 |

结项前，中心 #4 的本次原生 verify Attempt 为 `outcome_unknown`；原 Dispatch 已被精确 abandon，原绑定 terminal 后续由人工关闭并观察到退出。结项命令在中心完成这条 unknown 的代码核验对账前必须拒绝关闭 #4。该对账与 #3/#4 最终 Delivery 已完成，见[原中心回执](phase1-task-closure-2026-09-28/README.md)。两个失败 Run 均保留为真实历史，不把既有 PR 冒充为新候选。
