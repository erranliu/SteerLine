# 远端 Dispatch 失效修复验收

唯一归属 [Spec](../spec/orca-remote-dispatch-health.spec.md) / [Plan](../spec/orca-remote-dispatch-health.plan.md)。用户直接授权产品修复，不恢复现场 101。

| ID | 验收 | 结果 |
| --- | --- | --- |
| DH1 | 初始/反馈包含真实 assigned workspace/branch/fixed base | 定向初始/反馈派发验证及 L1 通过 |
| DH2 | 原 Dispatch 失效在空 inbox/timeout 时可见 | 定向回归通过，含正常检查之后身份丢失 |
| DH3 | 不重派、不伪造 started/completed | 定向回归及 L1 通过 |
| DH4 | 真实远端双轮唯一原 Codex、PR/交付及 owned 清理 | 独立 L3 通过，含公开 terminal switch 显示原 worker |
| DH5 | Orca 默认打开 worktree 不重复恢复原 conversation | 未修；需 Orca 源码与实际恢复入口验证能力 |

## 现场证据

2026-10-09 北京时间 12:30–12:31，101 WorkflowRun `6d1d02099debf1b149017a866d1ff00f`，Orca Dispatch `ctx_03271d0a74de`。原终端 `term_5a5c2b70-86f6-4fea-b8b6-b03c412e74dc`、incarnation `5a352d86-50fe-4cc1-ab44-2729e6cc71a6` 与 SteerLine 持久化身份相同。原 Codex 报缺少 assigned branch/base，ask/send/worker_done 均未送达；center worker-show 的 observation 为 identity_changed/exactWorker=false。

第二个 Codex `term_f014b516-a825-4a9d-9d06-4c8a80002a99` 回放相同输出，并显示 `This conversation is open in another app`。另有普通 shell `term_aa9c913d-310a-4626-9439-d7153da712a4`。两个 Codex 的创建因果尚未证明；没有把 shell 计为第二个 agent，也没有声称是 SteerLine 重复派发。现场仅一个 worker-start 记录。用户未留意第二个会话出现时间。

## 本次验证结果

定向 Go 回归通过，87.915 秒，报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T044308.812160900Z/summary.json`；完整 orcaremote 包通过，7.309 秒，报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T044801.996203800Z/summary.json`。源码 HEAD `e20c28eff3844d8ba74df7e0ad9c1672cd318257`，dirty=true。

L1 最小回归通过，preparation=0.651 秒、execution=218.541 秒，报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T044801.607983800Z/summary.json`。补充正常空轮询后失效、实际初始派发上下文与 inventory 区分 shell/renderer orphaned/恢复副本的回归通过，83.312 秒，报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T045232.836731900Z/summary.json`。

候选运行文件 `C:/SteerlineRuntime/bin/steerline-dispatch-health-20261009.exe` 已构建并通过正式 check，SHA-256 `101b55f4a21ff2c214da363e1a728abcc4c0a43e8769402bf874cf445c5cbb22`；source_commit 同上、source_dirty=true。现有服务/控制台仍运行原 `steerline.exe`，未替换运行中的服务，未重发 101。

第一次独立 L3 报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T044348.636855300Z/summary.json`，passed=false：首轮已创建真实 PR #173，新增 inventory 断言错误地将原终端的 renderer orphaned=true 视为身份失效，而 worker-show 对原 terminal/incarnation 的 exactWorker=true、liveness=live。该断言已改为检查实际原身份、连接和额外 agent，不以 renderer orphaned 代替 worker 存活；新增 inventory 回归覆盖此区别。这次失败不证明第二个 Codex，保留失败记录，重新运行 DH4。单元通过不替代 DH4。

北京时间 2026-10-09 12:57:44，第二次独立 `delivery.orca-remote-codex` 通过，379.15 秒含独立清理，报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T045049.276416100Z/summary.json`：passed=true、coverage=selected-L3、cleanup_complete=true。两轮原 terminal/incarnation/worktree 未变；首轮公开 terminal switch 的 focus.handle 等于原 worker，navigated=true；首轮、反馈及最终 inventory 均仅一个 Codex，普通 shell 不计。真实 PR #175 第二轮更新并于 04:57:12Z 合并，唯一交付及后继未启动验证通过。remote_public_output_verified=true、usage_verified=false、remote_usage_status=not_exposed_by_orca。产品二进制 SHA-256 与上述候选完全相同；helper SHA-256 `7f4e1b4a7a8c19caa4d54de6f40316962c29a672bf856bfc72f0409cbc623cfa`。这只验证精确原 terminal switch，不把 DH5 的默认 worktree 打开/恢复问题改称已修复。

## 用户第二次复现

WorkflowRun `74e3c22da63d4bfd2135252ba08b2b00`、Dispatch `ctx_30dc8384657c`、原 worker `term_57870871-d3b9-4af3-b20f-7506d676952f`。该轮实际完成了 PR #172，worker_done `relay_0868483c00e1` 成功送达并结算；SteerLine 保存 idle / waiting_pr_merge，不能把该轮原 worker 描述为执行中断。

用户描述打开 worktree 后自动显示占用报错。Orca 公开清单还列出 `term_5197f176-fc43-4a4c-8f13-a53a054780d8`，输出起始 `Resuming session…`，随后出现 `This conversation is open in another app`。原 worker 与该恢复终端是两个不同 PTY/incarnation，虽然界面没有两个可见对话。这将问题定位到显示工作区时的重复恢复，不证明具体 Orca 内部调用链已修复；SteerLine 代码没有使用 resume 启动这个终端。
