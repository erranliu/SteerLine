# SteerRunner 依赖与运行

SteerLine 只通过 `steerrunner-cli/v1` 调用独立 EXE。Runner 同一个 EXE 提供短命 CLI 与常驻服务；CLI 连接同一 state-dir 的现有实例，读请求不自动启动服务。任务、DAG、候选参与及 TaskDelivery 留在 SteerLine。

当前接入基线来自 [SteerRunner](https://github.com/erranliu/SteerRunner)，执行实现 commit `c85f4e71e88e1e9c51d73284a12d3034e6778f4e`（已可从远端获取）。Windows/amd64 验证构建使用 Go 1.26.4、版本 `0.1.0-dev.1`，EXE SHA-256 为 `f58aa9eb850df89d768957b740490eafc7cb89312ab762b66d53740e39ebe8dd`。当前没有正式 Release；[Runner 草稿 PR](https://github.com/erranliu/SteerRunner/pull/1)与[接入验收](../acceptance/steerrunner-cutover.md)记录实际通过和剩余项。

在 Runner 仓库检出所选提交后运行 `./scripts/build.ps1`。构建输出 EXE 和 `.build.json`；保留其来源和 SHA。运行不需要 Runner 或 SteerLine 源码、模板、Node 或 Python；Git、Codex / Orca 以及既有原生登录属于执行环境依赖，不打包认证或用户配置。

将 [execution.json](../../examples/execution.json) 放入自己的 SteerLine home，填入所选 EXE 的绝对路径、真实 version 和 SHA。示例占位 SHA 会拒绝启动，不能直接当作已安装配置。后端 settings 是 Runner 发布的公开 JSON，不再接受 implementation_configs、内部 codec 或旧版本配置。

调试先用 `./scripts/new-debug-release.ps1 -SteerRunnerExe C:/Tools/SteerRunner/steerrunner.exe` 生成新包；选择远端时再传 `-Orca -OrcaEnvironment 台式机64G`。生成器把 EXE 固定到包内并核对 SHA。执行方案启动后冻结，换 EXE / 方案创建新包；禁止在安装现场调试。

SteerLine 为 Runner 指定项目运行目录下的 `runner/`。Runner 的 SQLite、执行材料、仓库缓存、工作区与日志均归这个目录；SteerLine 只通过 CLI 读取公开 Run/事件，不读取其数据库、原生 codec 或私有日志。停止 SteerLine 不等于原生已停止，重置必须先证明原 Run 已停止和 Runner 服务已退出。

恢复、回答和反馈需要当前公开 revision 的新授权。未知操作只查原 request_id，不能换 ID 重发；重启不自动恢复旧答案或业务输入。Runner 的原生查看能力以 capability 为准，不能直接猜测原会话路径或进程。

本机 Orca 已按用户决定放弃，不列支持或待验项；本机 Codex 与远端 environment 的原生结果分别登记，互不代替。
