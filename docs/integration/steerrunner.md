# SteerRunner 依赖与运行

SteerLine 只通过 `steerrunner-cli/v1` 调用独立 EXE。Runner 同一个 EXE 提供短命 CLI 与常驻服务；CLI 连接同一 state-dir 的现有实例，读请求不自动启动服务。任务、DAG、候选参与及 TaskDelivery 留在 SteerLine。

当前接入基线来自 [SteerRunner](https://github.com/erranliu/SteerRunner)，执行实现 commit `41908e641b34a890448158d2f21a9edc597d08cb`（已可从远端获取）。Windows/amd64 验证构建使用 Go 1.26.4、版本 `0.1.0-dev.1`，EXE SHA-256 为 `5eb9c3a9162d13449d55d363dd136e0ee50dea0d1e06ac5afaa8b439b73e5a71`。当前没有正式 Release；[Runner 草稿 PR](https://github.com/erranliu/SteerRunner/pull/1)与[接入验收](../acceptance/steerrunner-cutover.md)记录实际通过和剩余项。

在 Runner 仓库检出所选提交后运行 `./scripts/build.ps1`。构建输出 EXE 和 `.build.json`；保留其来源和 SHA。运行不需要 Runner 或 SteerLine 源码、模板、Node 或 Python；Git、Codex / Orca 以及既有原生登录属于执行环境依赖，不打包认证或用户配置。

将 [execution.json](../../examples/execution.json) 放入自己的 SteerLine home，填入所选 EXE 的绝对路径、真实 version 和 SHA。示例占位 SHA 会拒绝启动，不能直接当作已安装配置。后端 settings 是 Runner 发布的公开 JSON，不再接受 implementation_configs、内部 codec 或旧版本配置。

调试先用 `./scripts/new-debug-release.ps1 -SteerRunnerExe C:/Tools/SteerRunner/steerrunner.exe` 生成新包，需要固定原生程序时可传 `-AgentExecutable <absolute-codex.exe> -AgentModel gpt-6-luna -AgentReasoningEffort medium`。生成器把 EXE 固定到包内并核对 SHA。执行方案启动后冻结，换 EXE / 方案创建新包；禁止在安装现场调试。

SteerLine 为 Runner 指定本地卷根下的短目录：Windows 为 `<drive>:\slr\<26位base32摘要>`，摘要绑定完整应用 run-dir（含 home 和项目）。深安装/调试目录不会增加 Runner 的认证 socket 路径，标准本地卷下当前 socket 为 98 字节；Runner 自身 106 字节限制不变。非 Windows 使用系统临时目录下同规则的 `slr/`，并检查相同预算。Task 数据库保留在 home；Runner 的 SQLite、执行材料、缓存、工作区与 `logs/` 归短目录。候选保存准确 state-dir，排查须同时提供 state-dir 和 Run ID。新布局不读取或迁移旧 `run-dir/runner`。

查询不创建目录；显式服务启动才创建当前用户私有目录，拒绝重定向/网络卷。重置必须先证明原 Run 和服务已停止，随后只删除该项目的摘要目录，不能删除共享 `slr` 根。实现与实际结果见[短目录验收](../acceptance/runner-short-state-path.md)。SteerLine 仍只通过 CLI 读取公开 Run/事件，不读取其私有数据库或原生日志。

恢复、回答和反馈需要当前公开 revision 的新授权。未知操作只查原 request_id，不能换 ID 重发；重启不自动恢复旧答案或业务输入。Runner 的原生查看能力以 capability 为准，不能直接猜测原会话路径或进程。

原生查看通过 `open-native-session --run-id R --attempt-id A --session-id S --generation G` 调用；只有 Runner 发布的 `native_session_viewer.adapters` 中相应 adapter 可用时启用按钮。原服务负责私有 binding 和实时进程/thread/workspace 核验，调用方不读取这些私有材料。详情与本次构建/验证范围见[原会话查看验收](../acceptance/native-session-viewer.md)。

Orca GitHub 执行必须显式配置 `execution.settings.agent_launch.runtime_executable` 与 `runtime_sha256`，指向执行主机已安装的可信 `steerrunner-agent.exe` 及其实际 SHA256；`agent_launch.executable` 是同主机的 native Codex。Runner 在执行主机使用已有 gh 登录准备单仓库认证，并在实际 app-server/权限模式内完成无模型预检；SteerLine 不传递凭据、不代做预检。缺 helper 时 Runner 在业务输入前拒绝，普通终端默认 Codex 启动不能替代这个能力。

调试生成器的 Orca 入口要求 `-OrcaRuntimeExecutable <执行主机绝对路径> -OrcaRuntimeSHA256 <sha256>`，并始终生成 agent_launch；可另传 `-AgentExecutable <执行主机 native Codex> -AgentModel <model> -AgentReasoningEffort high`。路径在执行主机解析，不能用调用方 Test-Path 判断；应先核对执行主机的实际 helper/native 构建身份。生成器不会安装远端 helper 或复制认证，首次启动后配置仍冻结。

Runner 远端机制已随主干 `d4969d0`（PR #18）合入，其独立验收见 [Orca 远端记录](https://github.com/erranliu/SteerRunner/blob/d4969d0/docs/acceptance/orca-remote-20261017.md)。SteerLine 产品接线的本次结果独立登记在[专属验收](../acceptance/orca-runtime-wiring.md)，不以 Runner 独立结果替代产品验证或改写旧安装候选。
