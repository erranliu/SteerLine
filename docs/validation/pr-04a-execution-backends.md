# PR 4A execution backend validation

日期：2026-09-24。基线：`origin/main` commit `dcc40a6`，Windows/amd64；Go `1.26.4 windows/amd64`，原生测试使用 Codex CLI `0.156.1` 和 Orca `1.4.209`。

本阶段在 Windows 验收。Linux 保持公共契约和平台实现的架构兼容；Linux 原生运行、生命周期测试及交叉构建不作为 4A 通过或转正式 PR 的条件。

## 已实现并验证的部分

- 同一 `ExecutionBackend` 接口承载 local `local_process` 和 Orca `orca_terminal_v2` 启动；service 仍独占 claim、槽位和业务状态转换。
- 当前 runtime 格式为 v5，旧 v1–v4 明确拒绝；版本化 `execution/v1` binding/operation/capability 文件保存执行身份与证据。启动意图先持久化；binding 固定 backend/protocol/target/agent/workspace/external identity；相同 Operation ID 的异负载被拒绝。
- local retry 和 Orca 同 incarnation 决策回复接入适配器边界。旧 runtime 格式与缺少当前执行绑定的状态拒绝副作用；不从当前配置补造平台身份。
- 服务停服调用固定 backend 的 StopExecution，先保存 stop Operation，再观察原身份；只有退出证据确认后记录 confirmed，超时或失联保留 unknown/slot。真实 Orca close 回执核对了原 runtime/terminal 与 `ptyKilled`。
- local 原始 JSONL 与 Orca terminal stream 均提供有界内容及 attempt/ref/source 绑定的 opaque cursor，不猜 phase；local DetachObservation 结束事件观察而不停止进程，stdout 继续落盘。
- 领取前要求匹配目标和 CLI 版本的 start/observe/stop 能力为 `verified`；`certify-backend` 核对原生证据并复制到持锁运行目录，证据内容变化使后续准入失败。
- 重启时对账 Operation/binding/runtime：可从固定 binding 保守重建缺失投影，损坏或矛盾记录拒绝启动；同 ID/同负载返回已持久化结果，无法查询的平台在途动作保留未知。

原生证据：[local Service 启动/输出/停止](pr-04a-local-native-2026-09-24.json)、[local retry](pr-04a-local-retry-native-2026-09-24.json)、[Orca agent worktree 启动](pr-04a-orca-agent-native-2026-09-24.json)、[Orca adapter 输出/重连/停止](pr-04a-orca-native-2026-09-24.json)、[Orca 同会话续跑](pr-04a-orca-continuation-native-2026-09-24.json)、[Orca workspace 核验](pr-04a-orca-workspace-native-2026-09-24.json)。local 两项已在 runtime v5 代码上重新执行；Orca 证据仅验证平台动作，尚未覆盖 v5 Service 完整派发。Orca 测试 worktree 保留，隔离仓库没有产生 Git 改动。

## 转正式 PR 前的验收差距

| 验收 | 当前证据与缺口 |
| --- | --- |
| B-A1 | 两后端共用 Service 启动入口，claim/槽/结果规则保留；Orca 决策续跑后的监督已收进适配器。仍需 Orca 经完整 Service 派发的原生证据和最终 diff 核对。 |
| B-A2 | Operation 记录和控制崩溃回归已覆盖部分边界；仍需注入启动发送后断连、回执落盘失败与重启，核对外部调用次数及残留资源。 |
| B-A3 | 固定 ref、PID 创建身份和 Orca incarnation 检查已实现；仍需改默认 host/agent/连接、PID 复用及 terminal 换代的双后端场景证据。 |
| B-A4 | fake 已验证 stop 接受但 terminal 未退出时保留 unknown/槽；local DetachObservation 的重复调用和进程存活通过定向回归。仍需 stop 失败/超时、Orca Detach 与停止并发的完整矩阵。 |
| B-A5 | 领取前能力守卫、版本/目标绑定、证据内容校验和 Orca 预检不依赖未使用的本机 Codex 已实现；原生 local/Orca 核心能力已归档。资源结算仍未验证且不启用。 |
| B-A6 | 较早 attempt 的迟到事实只记入历史活动，定向回归覆盖当前 claim、产物和槽位不受影响。 |
| B-A7 | local 受限 retry 与 Orca 原 terminal/同 incarnation 两轮回复均有真实平台证据；仍需完整 Service 的 Orca 派发路径与未知执行不替换的进一步核对。 |
| B-A8 | 已测试缺失 runtime 投影从 binding 保守恢复、损坏 binding 阻止启动和已记录回执查询；还缺意图/绑定/投影各写入边界、磁盘满、残留临时文件和完整重启故障矩阵。 |
| B-A9 | local 大单行输出保留原始日志并继续读后续事件，Detach 可重复调用；仍需慢观察、取消与停止并发、无跨 ref 取消和 goroutine 泄漏证据。 |
| B-A10 | runtime v5 的真实 Windows local Service 启动/输出/停止与 retry，以及 Orca adapter/agent 平台证据已归档；仍需 Orca 经完整 Service 派发路径、真实服务窗口挂载真实 worker 的客户端退出/重连、关窗/控制 stop 证据。PR 3 窗口回归由完整测试运行。 |

两后端 `SettleResources` 仍显式 unsupported。`ReconcileOperation` 可查询本地已持久化结果；local/Orca terminal v2 没有平台请求查询时，外部在途动作保持 unknown，不重发。4B 的会话 phase/turn 解码不属于此阶段。

## 本轮命令结果

- `go test ./... -count=1 -timeout 10m`：通过；`internal/steerline` 419.341 秒，remoteprotocol 与 viewer 通过。一次与原生验收并行的运行中，候选发布失败后的清理测试超时；该测试独立连续 3 次通过，无并行负载的完整重跑通过。
- 定向契约/输出/决策/control-crash/workspace 回归：通过。
- `go vet ./...`：通过。
- `go build ./...`：Windows/amd64 通过。Linux 仅保留架构兼容要求，本轮不把原生或交叉构建列入验收。
- `git diff --check`：通过。
- race 定向命令因环境不可运行：`go env CGO_ENABLED` 为 `0`，PATH 中无 gcc/clang；race detector 未执行。
- Windows 原生 local 验收在 v5 代码上重新运行真实 `codex-cli 0.156.1`；Orca `1.4.209` 创建了独立测试 worktree 与 Codex agent，并以精确 terminal 停止回执收尾。证据文件不保存认证信息。

因此本报告不宣告 B-A1～B-A10 全部通过。Linux 不在未完成验收项内；转正式 PR 需先完成上表的 Windows 与公共契约门槛。race 检查若工具链仍不支持，按 Plan 保留环境阻塞记录。
