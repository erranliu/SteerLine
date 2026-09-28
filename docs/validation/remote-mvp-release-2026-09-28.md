# 远程执行 MVP 本机发布回执（2026-09-28）

## 发布范围

本次按用户要求发布有人值守的远程执行 MVP 到 `C:\SteerlineRuntime`。来源为 `origin/main` 的 `8b79952`，范围文档提交为 `72834b3`；运行代码与该 main 基线相同，没有新增绕过预检的开关。安装二进制 SHA-256：`be7b242af31541d2460fb46c37de9bdb4077e9e4618b642852b0d7dcb59567df`。

SteerLine、SteerlineTest 各新增 `orca-remote-luna-xhigh` 方案，显示名为“Orca 台式机64g Codex Luna xhigh”，固定 `host=orca_supervised`、`agent=codex`、`model=gpt-6-luna`、`reasoning_effort=xhigh`，沿用各项目已有 sandbox 和四步契约，仅将步骤执行位置设为 target。远端取消仍不可用。本机默认方案及自动派发默认值未修改。

用户明确选择旧 Beads 项目 merge-h-308-code 保持只读；该配置逐字节未改动。

## 实际验证

- `go test ./... -count=1 -timeout 20m` 全部通过，核心包 484.480 秒；`go vet ./...` 与 `go build -o bin/steerline.exe ./cmd/steerline` 通过。任务身份检查覆盖 33 个身份文件。
- 两个项目分别通过实际远端连接、Git origin、repo selector、专用协调终端、固定步骤入口与原生 authority 预检。发布前后均由中心现有 Probe/能力加载函数验证六项必需能力及其内容哈希。
- 基础能力报告来自同一台式机64g、同一版本组合的已成功五步 Run `8cee9a0ae48b51cfb79d77928fe2a8ff`；原 manifest 的 SHA-256 为 `ec0622cf9d0c0e86e35916002a30a3e5a4cdc7d3a28243e657e35b7307089eb9`。复制到每个正式状态目录的 evidence 目录，只重定位内容绑定引用，不修改能力结论或取证时间。原始 `project_identity` 仍保留隔离验收项目，不伪称该 Run 在正式 SQLite 上发生。
- 当前新主机实际执行 `codex exec --model gpt-6-luna -c model_reasoning_effort="xhigh" --sandbox read-only --json`，返回 `REMOTE_LUNA_XHIGH_OK`、`turn.completed`、退出码 0。CLI 实际版本为 0.156.1。这个模型启动冒烟不代替新的五步验收；五步/业务问答/命令式发布仍引用[原真机记录](spec03-desktop64g-2026-09-28.md)。
- 发现远端原先没有 Go，Python 仅有 Windows Store alias。安装官方 Go 1.26.4 ZIP（校验官方 SHA-256）和 Python 3.13.1 embeddable 到用户工具目录；`python.exe`、`go` 及 Python subprocess 调用 Go 均通过。Go 的 GOROOT 固定为 `C:/Users/Administrator/.local/go`，可执行入口位于原已在 Orca PATH 中的 `.local/bin`，无需重启 Orca。
- 两个远端仓库均保持工作树干净；读取 GitHub 仓库身份成功，并 fetch 当前 main 核对验证脚本/发布技能入口存在。没有为发布冒烟领取正式业务任务、创建新业务 PR 或修改远端主工作树。

## 安装和状态保留

安装前两个项目均无活动或 unknown Execution。SteerLine 留有一个已停止历史 Execution；SteerlineTest 三个历史 Execution 已成功。四条历史 pending `claim_task` Operation 均属于既有历史，保留原记录，不通过改库清理。

通过正式 stop-service 命令取得两份 `stopped/process_exited/locks_released` 回执。备份目录为 `C:\SteerlineRuntime\backup\remote-mvp-20260928-1245`，包含原二进制、项目配置、状态及启动入口。构建产物调用 `install` 后，与目标二进制 SHA-256 相同。

两个项目已从原 service-window 入口以 manual 重启，均为 connected、ready、config_matched，控制台也已重开。升级前后对 tasks、task_contracts、task_dependencies、task_deliveries、workflow_runs、executions、step_attempts、decisions、decision_answers、operations、execution_operations、execution_bindings 做逐表有序内容哈希比较，全部一致。没有任务库清空、迁移或执行身份重绑定。

正式安装的详细配置、服务回读、状态哈希和远端工具/模型冒烟见 `C:\SteerlineRuntime\releases\remote-mvp-20260928`。两个专用协调终端保持打开；本次独立诊断终端已精确关闭。运行说明为 `C:\SteerlineRuntime\REMOTE-EXECUTION.md`。

## 未扩大承诺

持续无人值守、新主机故障矩阵和 viewer 2 秒验收、精确轮次及真实平台同句柄换代仍开放。#3/#4/#6 不关闭，#29/#32/#24 的完整交付前置不解除。部署只固定当前构建和配置，不实现 #30/#31 的 stable/dev 版本管理与升级框架。
