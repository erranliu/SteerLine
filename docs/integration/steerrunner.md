# SteerRunner 依赖与运行

SteerLine 只通过 `steerrunner-cli/v1` 调用独立 EXE。Runner 同一个 EXE 提供短命 CLI 与常驻服务；CLI 连接同一 state-dir 的现有实例，读请求不自动启动服务。任务、DAG、候选参与及 TaskDelivery 留在 SteerLine。

多 workflow 与冻结/换代的修订方案见[协作 Spec](../spec/runner-collaboration.spec.md) / [Plan](../spec/runner-collaboration.plan.md)，已实现调用方与 Runner 组合 schema 增量。公开字段/schema 固定后 caller 可用 fixture 开发，实际 EXE/资源身份及定向证据是联验/交付前置，样例不进生产 capabilities。空闲契约查询不证明 Run 读取接通，须在同一真实 Run 经正式详情/历史验证。Spec 4.2 的同构建服务换代仅在显式重启 SteerLine、重建 ProjectRuntime 时接纳新实例；单独重启 Runner 不触发旧运行时重新绑定，实例变化时拒绝新业务输入并提示重启，旧停止请求不影响新实例。未结算阻断换构建，本期不接管旧状态。实际结果与固定构建见[本次协作验收](../acceptance/runner-collaboration.md)；下列基线仅保留首次接入来源，不代表当前所需能力。

首次接入基线来自 [SteerRunner](https://github.com/erranliu/SteerRunner)，执行实现 commit `41908e641b34a890448158d2f21a9edc597d08cb`（已可从远端获取）。Windows/amd64 验证构建使用 Go 1.26.4、版本 `0.1.0-dev.1`，EXE SHA-256 为 `5eb9c3a9162d13449d55d363dd136e0ee50dea0d1e06ac5afaa8b439b73e5a71`。当前没有正式 Release；[Runner 草稿 PR](https://github.com/erranliu/SteerRunner/pull/1)与[接入验收](../acceptance/steerrunner-cutover.md)记录实际通过和剩余项。

在 Runner 仓库检出所选提交后运行 `./scripts/build.ps1`。构建输出 EXE 和 `.build.json`；保留其来源和 SHA。运行不需要 Runner 或 SteerLine 源码、模板、Node 或 Python；Git、Codex / Orca 以及既有原生登录属于执行环境依赖，不打包认证或用户配置。

将 [execution.json](../../examples/execution.json) 放入自己的 SteerLine home，填入所选 EXE 的绝对路径、真实 version 和 SHA。示例占位 SHA 会拒绝启动，不能直接当作已安装配置。后端 settings 是 Runner 发布的公开 JSON，不再接受 implementation_configs、内部 codec 或旧版本配置。

调试先用 `./scripts/new-debug-release.ps1 -SteerRunnerExe C:/Tools/SteerRunner/steerrunner.exe` 生成新包，需要固定原生程序时可传 `-AgentExecutable <absolute-codex.exe> -AgentModel gpt-6-luna -AgentReasoningEffort medium`。生成器把 EXE 固定到包内并核对 SHA。执行方案启动后冻结，换 EXE / 方案创建新包；禁止在安装现场调试。

SteerLine 为 Runner 指定本地卷根下的短目录：Windows 为 `<drive>:\slr\<26位base32摘要>`，摘要绑定完整应用 run-dir（含 home 和项目）。深安装/调试目录不会增加 Runner 的认证 socket 路径，标准本地卷下当前 socket 为 98 字节；Runner 自身 106 字节限制不变。非 Windows 使用系统临时目录下同规则的 `slr/`，并检查相同预算。Task 数据库保留在 home；Runner 的 SQLite、执行材料、缓存、工作区与 `logs/` 归短目录。候选保存准确 state-dir，排查须同时提供 state-dir 和 Run ID。新布局不读取或迁移旧 `run-dir/runner`。

查询不创建目录；显式服务启动才创建当前用户私有目录，拒绝重定向/网络卷。重置必须先证明原 Run 和服务已停止，随后只删除该项目的摘要目录，不能删除共享 `slr` 根。实现与实际结果见[短目录验收](../acceptance/runner-short-state-path.md)。SteerLine 仍只通过 CLI 读取公开 Run/事件，不读取其私有数据库或原生日志。

恢复、回答和反馈需要当前公开 revision 的新授权。未知操作只查原 request_id，不能换 ID 重发；重启不自动恢复旧答案或业务输入。Runner 的原生查看能力以 capability 为准，不能直接猜测原会话路径或进程。

原生查看通过 `open-native-session --run-id R --attempt-id A --session-id S --generation G` 调用；只有 Runner 发布的 `native_session_viewer.adapters` 中相应 adapter 可用时启用按钮。原服务负责私有 binding 和实时进程/thread/workspace 核验，调用方不读取这些私有材料。详情与本次构建/验证范围见[原会话查看验收](../acceptance/native-session-viewer.md)。

本次支持验收范围为 Windows/amd64 本机 Codex。用户已取消本机和远端 Orca 路径验收，适配器保留为未验能力，不声明真机支持。

绑定运行时的业务和查询使用 `--expected-instance-id`，必须选择发布 `instance_binding.expected_instance_id: true` 的 Runner。该参数是实际请求的前置，不能用先查询 status 代替；绑定调用不会隐式启动替代服务。显式重启 SteerLine 后先只读核对原 Run/输入或原 submit 请求，再开放准入。原 pending 输入撤销，unknown 输入只读对账，不自动重发。该增量随两仓协作 PR 同步交付，旧构建不能用于新调用方联验。
