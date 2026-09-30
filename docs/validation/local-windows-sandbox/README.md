# 本地 Windows sandbox 修复验证

日期：2026-09-30。基线：a05f797。问题：Windows local app-server 继承全局 unelevated，无法执行 linked worktree 的分散可写目录策略。

## 自动检查

- `go test ./internal/steerline/codexapp ./internal/steerline/codexworker`：通过。覆盖默认/显式 mode、unelevated + workspace-write 拒绝、read-only 和非 Windows 边界、受管参数冲突，以及进程启动前拒绝。
- 核心定向测试：本地 Windows 配置冻结/恢复、原生领取持久化、五套全局配置、服务选定 Profile、worker-flow 配置，以及 ExecutionProfile/GlobalConfig/AppServer 相关现有回归通过。
- 控制台 ExecutionProfile/ProfileChoice 定向测试：通过。
- `go build ./cmd/steerline`：通过；候选可执行文件保存在机器临时目录，没有替换安装中的二进制。
- `git diff --check`：通过。没有执行全量测试。

## 原生配置读回

隔离启动本机 Codex CLI 0.159.0 的 app-server，使用 `--config windows.sandbox="elevated"`。初始化后只调用 `config/read`，有效 `config.windows.sandbox` 为 `elevated`。原用户配置为 unelevated，用户配置没有修改；验证进程在读回后关闭。

这是配置加载证据，不是沙箱命令执行或 PR 发布闭环证据。没有触发 Windows sandbox setup、修改系统账户/ACL/防火墙，也没有恢复或改写现存任务 101/102。已有 local Windows 冻结 Profile 的摘要按新有效字段重新验证；不迁移旧执行，部署前需按项目当前状态处理旧运行。

Orca 只复用新增配置字段；本次没有实现或宣称交付 worker-environment-readiness Spec 的原会话探针和宿主恢复功能。
