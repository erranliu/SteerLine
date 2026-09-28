# #3/#4 原中心 Task 交付试次与服务保护

2026-09-28 使用现有 SteerLine 中心、手动派发和固定 TaskContract；#3 选择本机 Orca Luna xhigh，#4 选择台式机64g 受监管远端 Luna xhigh。此前通过的 SteerlineTest 验收未重跑。本记录只记此次原中心 Task 的新运行事实，不把验收文档直接改成 Task Delivery。

- #4 原 Run `e6a0886877369bf07f0fa11d46c78914`：prepare、implement 成功；verify Attempt `dc2a08c8295fbfe4989d124339d674b2` 在 Orca `worker-show` 返回 `identity_changed` / `exactWorker=false` 后，中心保存 `outcome_unknown` 并保留原 Dispatch `ctx_06a3ae2ebfff` 与终端 `term_0ac115ff-11f6-4b4c-9427-588310ad3ecb`。直接 `terminal show` 当时仍显示同一 incarnation 的连接终端，不能凭此把原 Dispatch 当作已结算或重新派发。远端 worker 的结构化 status、escalation 和一次 worker_done 均被 Orca 拒绝，未形成中心 verify 回执；没有 PR/Delivery。
- 该 verify worker 的只读后续说明称固定检查中 `go_test` 失败、`go_vet`/`go_build` 通过，具体失败涉及 11 个本机生命周期/启动及 native Task 测试；未留下可供中心验收的完整结果文件。此说明是 agent 对终端输出的转述，不是成功检查回执。原目标端结果保持不确定。
- #3 原 Run `c17f5ab4481a4125d45f64f99e4f8612`：prepare 成功；本机 implement Attempt `3ba2287c194b2827d9ab0c20c584b959` 在服务退出清理时终止，中心记录 `failed/blocked`，无 PR。原精确终端 `term_b93f1594-7f44-4332-a8ed-efa8c8075ddd` 已显示 disconnected/`operator_close`，工作树仍在固定 base 且干净。服务重开后，中心 `retry_run` 命令 `81f9ff3cc5ea09e0561b8ec697e04689` 接受确认依据，在**同一 Run/工作树**创建新 implement Attempt `704344f3064a1bb0a6115cd9f780b7fe`；这不是重建 Task 或重放原 Attempt。

[只读中心与 Orca 状态快照](blocked-runs.json)取于重试前。没有对 #4 的 unknown 执行 worker-stop、abandon、release、mark-stopped 或重派；远程取消仍 unsupported。此次也没有获得新的 #6 步骤转换 4 秒证据：首个提前监听器在 verify 创建时因本机 GBK 解码错误退出，后续服务中断使下一次计时无样本。

后续处置与上段快照按时间区分：#4 的 Orca `worker-abandon` 请求 `f7c7934e-d984-4239-b965-e0108709e981` 只隔离原 Dispatch，回执为 `processAction=none`。verify worker 完成最终回复后，人工关闭**原绑定**终端，Orca 回执 `ptyKilled=true`，再次 `terminal show` 为同一 incarnation 的 `connected=false` / `operator_close`。同工作树另有一个未绑定到此 Dispatch、显示“conversation is open in another app”的终端，未据此推断原 worker 仍运行，也未关闭它。中心 `mark_stopped` 命令 `19cb2345f471556a11f7cab7a3b47666` 被 `recovery_rejected` 拒绝：minimal-v1 必须对账原 Attempt/Operation，停止终端本身不足以结算步骤；#4 仍为 `outcome_unknown`，未重派。

#3 新 implement Attempt 随后明确返回 `succeeded=false`、`failure_kind=already_implemented_on_input_revision_no_task_diff`：固定 Spec/Plan 哈希核验通过，既有实现已在 base，工作树干净，worker 拒绝制造无关提交。原结构化结果保留在中心状态目录 `runs/c17f5ab4481a4125d45f64f99e4f8612/steps/704344f3064a1bb0a6115cd9f780b7fe/result.json`，未复制进仓库；此处不把失败重试写成成功交付。后续如继续原 Task，需明确有价值的任务相关变更和固定版本验证。

代码修复将已持久化的 Task 级 unknown 留在原 Run/占槽，服务循环继续监督其他 Run；服务停止时对受监管远端只解除本地观察，不发送取消。原状态与回执保留，后续恢复仍须按精确身份核实。
