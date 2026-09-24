# PR 4A execution backend validation

日期：2026-09-24。基线：`origin/main` commit `6aa8ae9`，Windows/amd64；Go `1.26.4 windows/amd64`，已安装 Codex CLI `0.156.1` 和 Orca CLI `1.4.209`。仅读取版本不构成原生执行验收。

## 已实现并验证的部分

- 同一 `ExecutionBackend` 接口承载 local `local_process` 和 Orca `orca_terminal_v2` 启动；service 仍独占 claim、槽位和业务状态转换。
- 版本化 `execution/v1` binding/operation/capability 文件记录不改 runtime v4。启动意图先持久化；binding 固定 backend/protocol/target/agent/workspace/external identity；相同 Operation ID 的异负载被拒绝。
- local retry 和 Orca 同 incarnation 决策回复接入适配器边界。旧运行只从 runtime 已有字段映射，缺少身份的旧 terminal 不续跑。
- 服务停服调用固定 backend 的 StopExecution，先保存 stop Operation，再观察原身份；只有退出证据确认后记录 confirmed，超时或失联保留 unknown/slot。Orca close/exit 路径有 fake 验收。
- local 原始事件输出提供最多 1 MiB 的 JSONL 批次、attempt 绑定 opaque cursor、截断 gap 和前后采样来源变化检查；不猜 phase。
- 参数化 DTO/记录测试、决策续跑 fake 平台测试、control 崩溃边界和 workspace 失败回归。

## 必需门槛未完成

- 真实 local Codex 启动/输出/停止、Windows 进程树和 Linux 原生退出/观察分离证据未执行。
- 真实 Orca terminal-v2 的 worktree/terminal 身份、重连、输出、stop/close 故障证据未执行。Orca ReadOutput、SettleResources 和 ReconcileOperation 明确不支持；StopExecution/close 只有 fake 证据，原生能力仍是 `not_tested`。
- 完整独立 DetachObservation 尚不支持 local；Operation/绑定文件写入失败、磁盘满、原生身份替换和恢复矩阵未逐项注入。
- 相同 Operation ID/相同负载目前会安全拒绝重复发送，但还没有通用 reconcile 流程恢复原回执；平台 operation reconciliation 不支持。
- Orca ReadOutput source-bound cursor 与独立事件解码尚待 4B；能力报告现有项保持 `not_tested`，不得作为平台原生验证。
- 启动目前会记录 `not_tested` 能力报告，但尚未实现“无 verified start 能力则拒绝副作用”的 Service 守卫；该能力门槛仍需补齐。

## 本轮命令结果

- `go test ./... -count=1 -timeout 10m`：通过；`internal/steerline` 407.608 秒，remoteprotocol 与 viewer 通过。随后新增的 StopExecution 正常/未确认 shutdown 定向测试也通过。
- 定向契约/输出/决策/control-crash/workspace 回归：通过。
- `go vet ./...`：通过。
- `GOOS=windows GOARCH=amd64 go build ./...`：通过；`GOOS=linux GOARCH=amd64 go build ./...`：通过。Linux 仅交叉编译，不是原生生命周期证据。
- `git diff --check`：通过。
- race 定向命令因环境不可运行：`go env CGO_ENABLED` 为 `0`，PATH 中无 gcc/clang；race detector 未执行。
- `codex --version` → `codex-cli 0.156.1`；`orca --version` → `1.4.209`。未启动真实 agent 或创建 Orca 资源。
- WSL Ubuntu 已安装但没有 Go；在 `/tmp` 下载隔离 Go 工具链尝试 2 分多钟无数据后中止，未安装系统包。Linux 原生 Go 生命周期测试未执行；此前 Linux amd64 结果只是 Windows 上的交叉构建。

因此本报告不宣告 B-A1～B-A10 全部通过；上述缺失原生证据、能力守卫和 race 检查是明确未完成项。
