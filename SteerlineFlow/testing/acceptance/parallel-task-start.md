# 手动任务并行启动修复验收

归属：2026-10-09 用户直接要求修复的“101 启动期间不能开始 105”问题。本次直接修改源码，不通过 roadmap-runner，不分配或变更执行 DAG 的 Task ID。基线为 `origin/main` / `d8d66d6c5696f41d7df1ed3ff50d4b3ae77b9da3`，分支 `codex/parallel-task-start`。

提交 PR 前 rebase 到最新 `origin/main` / `e20c28eff3844d8ba74df7e0ad9c1672cd318257`（PR #255）。唯一冲突是旧全局启动阻塞代码的诊断文本增补；该路径已由按请求领取替代，保留本次删除。修复涉及的控制台、启动请求、worker 预检查和 Codex 工作区实现与 rebase 前一致。

控制台按任务保护未确认的启动请求；普通 `start_task` 以原 request ID 保存领取和执行声明，不占项目级 `Control.Operation`。领取前检查、领取后能力检查、Codex 工作区准备及会话准备在后台执行，中心 actor 统一校验和写入结果。启动回执只有在原 worker 身份及启动操作被持久化确认后才为 `applied`。

## 自动验收

| ID | 要求 | 场景/入口 | 结果 |
| --- | --- | --- | --- |
| PST-A1 | 101 卡在启动检查、领取后能力检查、工作区或会话准备时，105 独立启动；两套 profile、Execution、session 不混用 | `TestTaskStartControlDoesNotSerializeIndependentTasks`：`preflight`、`worker-preflight`、`workspace`、`session` | 已通过 |
| PST-A2 | 控制台允许不同任务提交，禁止同任务重复提交；停止跟踪未知请求不授权重复启动 | `TestStartSubmissionScopesPendingRequestToTask`、`TestStartSelectedRestoredRequestDeduplication` | 已通过 |
| PST-A3 | 保留 DAG、手动派发、暂停和版本校验；预检查期间改变 authority 不可提交过期领取 | PST-A1 的依赖拒绝断言、`TestTaskStartControlRejectsAuthorityChangedDuringPreflight`、`TestNativeClaimAllowsManualParallelButKeepsAutoSlot` | 已通过 |
| PST-A4 | 两个已领取但未启动的请求重开中心后保留原 request/run/Execution；丢失 applying 回执可从原事务恢复，不重新领取 | `TestTaskStartControlRecoversReservationsWithoutReplay`，包含新 service instance 的 owner 接管检查和领取后、回执前故障注入 | 已通过 |
| PST-A5 | 101 的检查失败或原外部操作结果未知不占项目级阻塞；未知副作用仍须核对，不能当作纯领取重放 | PST-A1 的 `worker-preflight-failure`、`TestTaskStartControlKeepsUncertainEffectsScopedToOriginalTask`、`TestCandidateResumeErrorDoesNotStarveSibling` | 已通过 |
| PST-A6 | Codex 在固定工作区准备完成前不准备会话；修改工作区 intent 或丢失已有会话的原工作区不能创建替代资源 | `TestWorkspaceLifecyclePrecedesSessionAndRejectsRetargeting`、`TestStartTurnFailureRetainsOriginalSessionIdentity` | 已通过 |
| PST-A7 | 真实产品入口、Git、SQLite 和受控 app-server 完成启动、PR/交付与依赖释放，清理完毕 | L3 controlled `delivery.basic` | 已通过：`passed=true`、`cleanup_complete=true`、无清理错误 |

## 验证命令和证据

证据根目录：`C:\Users\lmas1\AppData\Local\Temp\steerline-parallel-start-20261009-123349`。

```powershell
go test ./internal/execution ./internal/console ./internal/execution/adapters/codex -run 'TestTaskStartControl|TestStartSubmissionScopesPendingRequestToTask|TestStartSelectedRestoredRequestDeduplication|TestWorkspaceLifecycle|TestStartTurnFailure|TestOperationProgress' -count=1 -v -timeout=6m
go test ./internal/execution -run 'TestTaskStartControlRecoversReservationsWithoutReplay|TestTaskStartControlKeepsUncertainEffectsScopedToOriginalTask|TestNativeClaimAllowsManualParallelButKeepsAutoSlot|TestCandidateResumeErrorDoesNotStarveSibling' -count=1 -v -timeout=5m
go run ./cmd/test-feedback '-level=L1' '-out' '<证据根目录>\L1-final'
go run ./cmd/test-feedback '-level=L3' '-suite=delivery' '-env=controlled' '-scenario=delivery.basic' '-out' '<证据根目录>\L3-delivery-serial'
./build.ps1
```

- `targeted-final.log`：最终五个并行/失败子场景、重启/原请求恢复、authority 变更、未知结果隔离及控制台/adapter 回归通过；execution 286.636 秒，console 2.873 秒，Codex adapter 1.336 秒。
- `recovery-and-claim-tests.log`：补充手动并行/auto 占用保护、候选失败隔离、真实 owner 接管与未知效果保护通过。
- `L1-final/summary.json`、`L1-final-run.log`：最小 L1 `passed=true`，执行 208.7 秒；包含事务/幂等、存储接线、架构边界及控制台回归。
- `build-final.log`：Windows 构建成功，产物为当前 worktree 的 `bin/steerline.exe`。
- `L3-delivery-final/summary.json`：并行运行额外端到端时，`task validate` 通过，`task create` 在驱动命令期限内超时；尚未进入服务启动，场景失败但 `cleanup_complete=true`。此失败保留，不以早期通过的 `L3-delivery` 替代最终版本证明。
- `L3-delivery-serial/summary.json`、`scenarios/delivery.basic/scenario.json`：结束本聊天的 L1 和定向测试后单独运行，最终版本 `passed=true`、`cleanup_complete=true`、`cleanup_errors=null`；北京时间 2026-10-09 12:57 完成。验证真实产品的任务创建、启动、受控 worker、Git/PR 交付和依赖释放；前一轮超时不能据此认定为已定位的产品缺陷或已确定的环境原因。
- PR rebase 后证据：`C:\Users\lmas1\AppData\Local\Temp\steerline-parallel-start-pr-rebase-20261009\targeted.log`，重新验证双请求恢复、未知结果隔离、控制台按任务去重和 Codex 固定工作区意图，三个包均通过；同目录 `build.log` 为 rebase 后 Windows 构建成功记录。前述完整定向、L1 与受控 L3 的范围和原始基线保留。

未选择 full、真实模型或远端验收：本次范围是手动启动调度与请求恢复，定向回归、L1 和受控产品链路覆盖相关自动项。本机 `CGO_ENABLED=0` 且未发现 GCC，未运行 race detector。

## 人工验收与安装状态

PST-M1：安装更新后的 Windows 可见服务和控制台，使用两个 Ready 且互不依赖的任务，在第一个仍显示“启动中”时提交第二个，核对各自方案和回执。参与窗口待定，当前待验；不阻塞源码交付。

当前只生成源码树的 Windows 产物，未替换 `C:\SteerlineRuntime\bin\steerline.exe`，未停止或切换其运行中的服务。安装更新和真实用户控制台操作不以受控测试替代。
