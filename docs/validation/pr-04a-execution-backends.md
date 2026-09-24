# PR 4A execution backend validation

日期：2026-09-24。基线：`origin/main` commit `6aa8ae9`，Windows/amd64；Go `1.26.4 windows/amd64`，已安装 Codex CLI `0.156.1` 和 Orca CLI `1.4.209`。仅读取版本不构成原生执行验收。

本阶段在 Windows 验收。Linux 保持公共契约和平台实现的架构兼容；Linux 原生运行、生命周期测试及交叉构建不作为 4A 通过或转正式 PR 的条件。

## 已实现并验证的部分

- 同一 `ExecutionBackend` 接口承载 local `local_process` 和 Orca `orca_terminal_v2` 启动；service 仍独占 claim、槽位和业务状态转换。
- 版本化 `execution/v1` binding/operation/capability 文件记录不改 runtime v4。启动意图先持久化；binding 固定 backend/protocol/target/agent/workspace/external identity；相同 Operation ID 的异负载被拒绝。
- local retry 和 Orca 同 incarnation 决策回复接入适配器边界。旧运行只从 runtime 已有字段映射，缺少身份的旧 terminal 不续跑。
- 服务停服调用固定 backend 的 StopExecution，先保存 stop Operation，再观察原身份；只有退出证据确认后记录 confirmed，超时或失联保留 unknown/slot。Orca close/exit 路径有 fake 验收。
- local 原始事件输出提供最多 1 MiB 的 JSONL 批次、attempt 绑定 opaque cursor、截断 gap 和前后采样来源变化检查；不猜 phase。
- 参数化 DTO/记录测试、决策续跑 fake 平台测试、control 崩溃边界和 workspace 失败回归。

## 转正式 PR 前的验收差距

| 验收 | 当前证据与缺口 |
| --- | --- |
| B-A1 | 两后端共用 Service 启动入口，claim/槽/结果规则保留；决策续跑后的 Orca 监督仍由 Service 直接构造，需收进适配器并核对最终 diff。 |
| B-A2 | Operation 记录和控制崩溃回归已覆盖部分边界；仍需注入启动发送后断连、回执落盘失败与重启，核对外部调用次数及残留资源。 |
| B-A3 | 固定 ref、PID 创建身份和 Orca incarnation 检查已实现；仍需改默认 host/agent/连接、PID 复用及 terminal 换代的双后端场景证据。 |
| B-A4 | fake 已验证 stop 接受但 terminal 未退出时保留 unknown/槽；local DetachObservation 仍返回 unsupported，stop 失败/超时和重复 Detach 的契约验收未齐。 |
| B-A5 | 能力报告目前全部为 `not_tested`，但 Service 仍允许启动；需完成目标版本证据、领取前预检和副作用能力守卫。 |
| B-A6 | 旧 attempt 的迟到事实隔离、claim/产物不变和不释放另一执行槽尚缺针对性证据。 |
| B-A7 | local 受限 retry、Orca 同终端续跑有 fake 回归；仍需核对固定 ref、无 fallback 及未知执行不替换的完整场景。 |
| B-A8 | 文件记录已有意图和绑定顺序；还缺意图/绑定/runtime 投影写入失败、磁盘满、损坏记录及重启对账矩阵。同 ID 同负载目前安全拒绝重发，尚不能恢复原回执。 |
| B-A9 | 慢观察、大输出、取消与停止并发、无跨 ref 取消及观察 goroutine 退出的验收未齐。 |
| B-A10 | Windows 隔离仓库里的真实 local Codex 与本机 Orca 启动/输出/停止/重连，以及 PR 3 服务窗口生命周期回归仍需归档；fake 测试不代替。 |

此外，Orca `ReadOutput`、两后端 `SettleResources` 和 `ReconcileOperation` 仍显式 unsupported；需要按 Spec 明确能力边界并完成相关必需语义。4B 的会话 phase/turn 解码不属于此阶段。

## 本轮命令结果

- `go test ./... -count=1 -timeout 10m`：通过；`internal/steerline` 407.608 秒，remoteprotocol 与 viewer 通过。随后新增的 StopExecution 正常/未确认 shutdown 定向测试也通过。
- 定向契约/输出/决策/control-crash/workspace 回归：通过。
- `go vet ./...`：通过。
- `GOOS=windows GOARCH=amd64 go build ./...`：通过。额外的 Linux amd64 交叉构建也通过，仅作为架构兼容检查，不计入 4A 验收。
- `git diff --check`：通过。
- race 定向命令因环境不可运行：`go env CGO_ENABLED` 为 `0`，PATH 中无 gcc/clang；race detector 未执行。
- `codex --version` → `codex-cli 0.156.1`；`orca --version` → `1.4.209`。未启动真实 agent 或创建 Orca 资源。

因此本报告不宣告 B-A1～B-A10 全部通过。Linux 不在未完成验收项内；转正式 PR 需先完成上表的 Windows 与公共契约门槛。race 检查若工具链仍不支持，按 Plan 保留环境阻塞记录。
