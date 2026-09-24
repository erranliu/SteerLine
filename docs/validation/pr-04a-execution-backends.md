# PR 4A execution backend validation

日期：2026-09-24。基线：`origin/main` commit `8957905`，Windows/amd64；Go `1.26.4 windows/amd64`，原生测试使用 Codex CLI `0.156.1` 和 Orca `1.4.209`。

本阶段在 Windows 验收。Linux 保持公共契约和平台实现的架构兼容；Linux 原生运行、生命周期测试及交叉构建不作为 4A 通过或转正式 PR 的条件。

## 已实现并验证的部分

- 同一 `ExecutionBackend` 接口承载 local `local_process` 和 Orca `orca_terminal_v2` 启动；service 仍独占 claim、槽位和业务状态转换。
- 当前 runtime 格式为 v5，旧 v1–v4 明确拒绝；版本化 `execution/v1` binding/operation/capability 文件保存执行身份与证据。启动意图先持久化；binding 固定 backend/protocol/target/agent/workspace/external identity；相同 Operation ID 的异负载被拒绝。
- local retry 和 Orca 同 incarnation 决策回复接入适配器边界。旧 runtime 格式与缺少当前执行绑定的状态拒绝副作用；不从当前配置补造平台身份。
- 服务停服调用固定 backend 的 StopExecution，先保存 stop Operation，再确认原身份；只有原进程退出或 Orca 精确 terminal 的 `ptyKilled` 回执才记录 confirmed，超时或失联保留 unknown/slot。真实 Orca close 回执核对了原 runtime/terminal；后续较弱的失联观察不覆盖已确认的 PTY 停止证据。
- local 原始 JSONL 与 Orca terminal stream 均提供有界内容及 attempt/ref/source 绑定的 opaque cursor，不猜 phase；local DetachObservation 结束事件观察而不停止进程，stdout 继续落盘。
- 领取前要求匹配目标和 CLI 版本的 start/observe/stop 能力为 `verified`；`certify-backend` 核对原生证据并复制到持锁运行目录，证据内容变化使后续准入失败。
- 重启时对账 Operation/binding/runtime：可从固定 binding 保守重建缺失投影，损坏或矛盾记录拒绝启动；同 ID/同负载返回已持久化结果，无法查询的平台在途动作保留未知。

原生证据：[local Service 启动/输出/停止](pr-04a-local-native-2026-09-24.json)、[local retry](pr-04a-local-retry-native-2026-09-24.json)、[真实服务窗口 Ctrl+C](pr-04a-local-window-native-2026-09-24.json)、[真实服务窗口控制 stop](pr-04a-local-window-control-native-2026-09-24.json)、[真实服务窗口关窗/重连](pr-04a-local-window-close-native-2026-09-24.json)、[Orca agent worktree 启动](pr-04a-orca-agent-native-2026-09-24.json)、[Orca adapter 输出/重连/停止](pr-04a-orca-native-2026-09-24.json)、[Orca 同会话续跑](pr-04a-orca-continuation-native-2026-09-24.json)、[Orca workspace 核验](pr-04a-orca-workspace-native-2026-09-24.json)、[Orca 两阶段派发](pr-04a-orca-staged-native-2026-09-24.json)、[Orca 完整 Service 派发/输出/停止](pr-04a-orca-service-native-2026-09-24.json)。local 与 Orca Service 证据已在 runtime v5 代码上执行；隔离测试 worktree 保留，没有产生 Git 改动。

## 验收矩阵

| 验收 | 结果与证据 |
| --- | --- |
| B-A1 | 通过。两后端共用 Service 派发和 claim/槽/结果规则；真实 Orca 经完整 Service 启动、固定 ref、输出及停止。最终 diff 核对确认 Service/决策/重试/结项没有直接调用 Codex 或 Orca CLI；local 类型判断仅用于现有故障注入 hook。 |
| B-A2 | 通过。启动响应丢失、binding/投影写失败、回复发送后丢回执、停止确认写失败及重启不重发均有故障注入；同 Operation ID 异负载拒绝，已记录同负载结果只返回原回执。 |
| B-A3 | 通过。固定 ref 不随默认 backend/agent/连接变化；改默认 backend 停旧 local 运行在平台调用前拒绝，历史 agent 不借新能力报告；PID 创建身份复用、Orca incarnation/连接替换均被拒绝。 |
| B-A4 | 通过。精确 Orca `ptyKilled` 回执不被弱观察降级；stop 接受但未退出、close 失败和确认写失败保留 unknown/槽。local/Orca Detach 可重复，Orca 取消一个 ref 不关闭另一 terminal。 |
| B-A5 | 通过。领取前与执行边界核对目标、agent、CLI 版本和证据哈希；Orca 原生 agent 不要求未使用的本机 Codex。local/Orca 核心能力有原生证据；资源结算保持显式 unsupported。 |
| B-A6 | 通过。较早 attempt 的迟到事实只记入其历史活动，当前 claim、产物和槽位不受影响。 |
| B-A7 | 通过。local 受限 retry 和 Orca 原 terminal/同 incarnation 两轮回复均有真实平台证据；回复丢回执保持原 ref 的 unknown，重启不重发，无跨后端 fallback。 |
| B-A8 | 通过。意图、binding 和投影边界故障注入覆盖不重启替代 worker；缺失投影按固定 binding 保守恢复，损坏记录阻止副作用。注入 `ENOSPC`、Windows 替换失败和残留临时文件检查均保留旧快照；v1–v4 明确拒绝。 |
| B-A9 | 通过。local 大单行输出保留原始日志并继续后续事件；两后端 Detach 可重复。Orca 同 ref 观察串行、gate 清理、慢观察期间另一 ref 精确停止、取消无跨 ref 传播均有测试。race detector 因当前工具链不可运行，见下文。 |
| B-A10 | 通过。runtime v5 的真实 Windows local 与 Orca Service 启动/输出/停止证据已归档；Orca 两阶段同终端派发回执含 `input_accepted` 和 `turn_started`。真实服务窗口挂载真实 Codex worker 后，客户端退出/重连、Ctrl+C、控制 stop、关窗清理及重启不重复执行均通过；PR 3 窗口回归也已单独启用并通过。 |

两后端 `SettleResources` 仍显式 unsupported。`ReconcileOperation` 可查询本地已持久化结果；local/Orca terminal v2 没有平台请求查询时，外部在途动作保持 unknown，不重发。4B 的会话 phase/turn 解码不属于此阶段。

## 本轮命令结果

后续质量审计发现并修复：Orca 非 live 终端仍可续跑、发送回执缺少身份/accepted 校验、答案摘要未包含文件内容、结果监督绕过 incarnation 检查、观察结束误释放槽位、重启忽略 stop/reply 记录，以及未启动进程的 local 准备失败无法 retry。人工停止确认现绑定原 attempt，重启不会重新施加已人工结算的资源占用，也不伪造平台 Operation 成功；原生 Orca 与 Codex 覆盖启动的能力证据已隔离。新增 `execution_audit_test.go` 对以上路径逐项复现，详见 [质量审计记录](pr-04a-quality-audit.md)。

审计后的真实 Orca Service 复验见 [回执与工作区核验后的原生证据](pr-04a-orca-service-audit-2026-09-24.json)。

- 审计修复后的 `go test ./... -count=1 -timeout 15m`：通过；`internal/steerline` 568.162 秒，remoteprotocol 与 viewer 通过。此前的 10 分钟运行因测试进程与运行中重新构建的 CLI 使用不同能力报告键出现一次窗口用例失败，并触及包级时限；冻结代码后该用例独立通过，随后完整回归通过。15 分钟仅为测试包总时限，平台调用和场景超时没有放宽。
- 定向契约/输出/决策/control-crash/workspace 回归：通过。
- `go vet ./...`：通过。
- `go build ./...`：Windows/amd64 通过。Linux 仅保留架构兼容要求，本轮不把原生或交叉构建列入验收。
- `git diff --check`：通过。
- race 定向命令因环境不可运行：`go env CGO_ENABLED` 为 `0`，PATH 中无 gcc/clang；race detector 未执行。
- Windows 原生 local 验收在 v5 代码上重新运行真实 `codex-cli 0.156.1`；真实服务窗口分别通过 Ctrl+C、控制 stop 和关窗场景。Orca `1.4.209` 经 Service 创建独立测试 worktree 与 Codex agent，读取原始输出并以精确 terminal 停止回执收尾。证据文件不保存认证信息。

当前 B-A1～B-A10 的 Windows 与公共契约证据齐备。`SettleResources` 的资源释放能力明确 unsupported，工作区和产物默认保留；没有平台请求查询能力的在途动作继续保持 unknown。Linux 不在 4A 验收内；race 检查按 Plan 记录工具链阻塞。
