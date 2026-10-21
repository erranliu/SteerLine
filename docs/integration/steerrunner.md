# SteerRunner 依赖与运行

SteerLine 只通过 `steerrunner-cli/v1` 调用独立 EXE。Runner 同一个 EXE 提供短命 CLI 与常驻服务；CLI 连接同一 state-dir 的现有实例，读请求不自动启动服务。任务、DAG、候选参与及 TaskDelivery 留在 SteerLine。

当前恢复与更新按[服务换代恢复契约](../spec/service-generation-recovery.spec.md)：相关契约兼容时保留原 Run/授权与执行资源，使用 verify-takeover 核对候选构建，不以全部资源先停止为固定前提。下述首次协作 Spec 4.2 的同构建限制和“未结算阻断换构建”仅保留历史范围；删除目录、重置和破坏性迁移仍须取得原范围资源结算证明。资源丢失、不兼容或未知原操作不能通过更新解除。

开发候选在预留事务内分配页面显示的候选序号，并将 `name_prefix`（C1 为 `sl-<task_id>`，C2 起为 `sl-<task_id>-c<Number>`）与最终 submit 一起冻结。Runner 将其作为通用命名标识，Orca worktree 使用 `<name_prefix>-<Runner Run ID>`；未提供时前缀为 `sr`。该字段不从 correlation 推导，也不改变原资源绑定或分支规则；需要发布了此字段/schema 的 Runner 构建，两仓验证见[专属验收](../acceptance/runner-name-prefix.md)。

所有 LLM 执行操作统一经 Runner 承担，职责声明见[LLM 执行边界](../architecture.md#llm-执行边界)。SteerLine 的默认预设选择与 Runner 内部 LLM 操作的默认链路分别说明于[两层默认链路](../product-design/execution-profiles.md#两层默认链路)；实际接入使用所选构建发布的公开配置与能力契约。

测试安排生成已接入 Runner PR #33 / `9586ac4` 的 `generate/1`，复用指定本机 Codex profile 的 executable/model/reasoning_effort，并显式构造 read_only；开发认证、权限、audit 和提示词不随之复制。内容流程的公开目录允许空 prompt_steps；开发候选拒绝 generate/conversation workflow。只接生成所需的 submit/inspect/content_result/stop，conversation 操作留待规划台。2026-10-20 核对 Runner PR #34 的 R2 已随历史固定版本 `bf3ec58` 发布：`artifacts discover/collect/list/read` 和 `workspace-git`，capabilities 的 run_evidence/schema 给出正式约束。SteerLine 已接入这五个命令及能力/schema 校验：原候选的 collect 请求先持久化，未知只 inspect 原请求，read 核验 Run/artifact/长度/base64/SHA256 后保存不可变副本；只读查询结合近期 verified commit/clean 计算当前结果。原 Run 使用项目忽略的 `.steerrunner/artifacts/<唯一执行ID>/`；首版收集 managed 报告与同目录 summary.json。独立生成对象已纳入兼容 takeover、启动前原 stop outbox 与离线结算核验，保留历史 build pin 和原请求。历史 bf3ec58 的 generate 接管误校验已由 Runner #37 修复：freeze/takeover 按 workflow 能力及实际 parser 意图判断结果解析配置。干净主干 13ef123 的同 EXE 真实 generate 及原 k 包只读复验通过；历史 A/B 接管另按其固定构建取证，不改写原 pin 或扩大恢复授权。命令/格式见[测试规范](../testing.md#测试管理文档与结果)，版本及验证边界见[本任务验收](../acceptance/test-management.md)。

多 workflow 与分层冻结见[协作 Spec](../spec/runner-collaboration.spec.md) / [Plan](../spec/runner-collaboration.plan.md)，已实现调用方与 Runner 组合 schema。实际 workflow ID/version 取固定构建的公开目录，不从首次接入版本推定。样例不进入生产 capabilities；空闲契约查询不证明 Run 读取接通，须在同一真实 Run 经正式详情/历史核验。运行中的 ProjectRuntime 保持原实例绑定，单独重启 Runner 不授权自动重绑；显式重启 SteerLine 后核验新实例，旧停止请求不影响新实例。跨构建接管与未结算资源按[服务换代恢复契约](../spec/service-generation-recovery.spec.md)逐项核验。实际固定构建及首次协作验证见[协作验收](../acceptance/runner-collaboration.md)，首次接入基线仅用于追溯。

首次接入基线来自 [SteerRunner](https://github.com/erranliu/SteerRunner)，执行实现 commit `41908e641b34a890448158d2f21a9edc597d08cb`（已可从远端获取）。Windows/amd64 验证构建使用 Go 1.26.4、版本 `0.1.0-dev.1`，EXE SHA-256 为 `5eb9c3a9162d13449d55d363dd136e0ee50dea0d1e06ac5afaa8b439b73e5a71`。当前没有正式 Release；[Runner 草稿 PR](https://github.com/erranliu/SteerRunner/pull/1)与[接入验收](../acceptance/steerrunner-cutover.md)记录实际通过和剩余项。

在 Runner 仓库检出所选提交后运行 `./scripts/build.ps1`。构建输出 EXE 和 `.build.json`；保留其来源和 SHA。运行不需要 Runner 或 SteerLine 源码、模板、Node 或 Python；Git、Codex / Orca 以及既有原生登录属于执行环境依赖，不打包认证或用户配置。

将 [execution.json](../../examples/execution.json) 放入自己的 SteerLine home，填入所选 EXE 的绝对路径、真实 version 和 SHA。示例占位 SHA 会拒绝启动，不能直接当作已安装配置。后端 settings 是 Runner 发布的公开 JSON，不再接受 implementation_configs、内部 codec 或旧版本配置。

调试先用 `./scripts/new-debug-release.ps1 -SteerRunnerExe C:/Tools/SteerRunner/steerrunner.exe` 生成新包，需要固定原生程序时可传 `-AgentExecutable <absolute-codex.exe> -AgentModel gpt-6-luna -AgentReasoningEffort medium`。生成器把 EXE 固定到包内并核对 SHA。执行方案启动后冻结，换 EXE / 方案创建新包；禁止在安装现场调试。

每个 home 由应用宿主持有一个 Runner 服务，项目借用同一固定客户端；整体关闭统一结算，单项目 caller 退出保留原 Run/检查事实，已有执行可能继续。产品请求号与 Runner 随机请求号分别持久化，跨项目同号不冲突。当前增量见[共享服务验收](../acceptance/shared-runner-service.md)。

SteerLine 为 Runner 指定本地卷根下的短目录：Windows 为 `<drive>:\slr\<26位base32摘要>`，摘要绑定带用途标记的规范绝对 home，同 home 项目共用一目录。深安装/调试目录不会增加 Runner 的认证 socket 路径，标准本地卷下当前 socket 为 98 字节；Runner 自身 106 字节限制不变。非 Windows 使用系统临时目录下同规则的 `slr/`，并检查相同预算。Task 数据库保留在 home；Runner 的 SQLite、执行材料、缓存、工作区与 `logs/` 归短目录。候选保存准确 state-dir，排查须同时提供 state-dir 和 Run ID。新布局不读取或迁移旧项目短目录或 `run-dir/runner`。应用在 `home/application-api/runner-build.json` 保留不可变构建 pin，先核验再启动共享服务；项目 checkpoint/候选/检查保存原操作绑定。

查询不创建目录；显式服务启动才创建当前用户私有目录，拒绝重定向/网络卷。共享目录的整包 reset 须核对全部项目、检查、生成对象和原 Run 的停止/结算，不能只看当前项目；删除只针对该 home 的准确摘要目录，不能删除共享 `slr` 根。短路径来源见[短目录验收](../acceptance/runner-short-state-path.md)，当前共享/reset 边界见[共享服务验收](../acceptance/shared-runner-service.md)。SteerLine 只通过 CLI 读取公开 Run/事件，不读取 Runner 私有数据库或原生日志。

新提交的恢复、回答和反馈需要当前公开 revision 的显式授权。兼容服务重启保留 Runner 原授权、会话与等待状态，可补收原结果并接续已证明未发送的后继，不因重启批量 resume；原资源丢失、不兼容、业务阻断和显式 stop 仍按原门槛处理。SteerLine 不自动生成新的 submit/reply/resume；旧调用方 pending 控制撤回，unknown 只查原 request_id。Runner 已受理且绑定原问题/执行范围的未发送答案按原授权对账，旧 epoch 或错范围仍需人工确认。不能换 ID 重发未知操作。详见[换代恢复契约](../spec/service-generation-recovery.spec.md)。

交付摘要只读取已有 Run 投影和原命令回执，分别展示受理、原生结果、业务阶段、PR 与资源停止；命令 completed、pr_ready 和 resources_settled 不能互相替代。刷新不发送输入，不创建资源；完整 environment-check、纯 validate 和 R2 收集按需使用，Run 内的权限/来源/发布门槛保持。摘要实现与验收见[可靠性交付简化](../acceptance/reliable-delivery-simplification.md)。

原生查看通过 `open-native-session --run-id R --attempt-id A --session-id S --generation G` 调用；只有 Runner 发布的 `native_session_viewer.adapters` 中相应 adapter 可用时启用按钮。原服务负责私有 binding 和实时进程/thread/workspace 核验，调用方不读取这些私有材料。详情与本次构建/验证范围见[原会话查看验收](../acceptance/native-session-viewer.md)。

绑定运行时的业务和查询使用 `--expected-instance-id`，必须选择发布 `instance_binding.expected_instance_id: true` 的 Runner。该参数是实际请求的前置，不能用先查询 status 代替；绑定调用不会隐式启动替代服务。显式重启 SteerLine 后先只读核对原 Run/输入或原 submit 请求，再开放准入。原 pending 输入撤销，unknown 输入只读对账，不自动重发。该增量随两仓协作 PR 同步交付，旧构建不能用于新调用方联验。
远端 Orca 使用执行主机已有 Codex/Git/gh 登录，通过原生 Orca 链路启动，不部署或运行 SteerRunner helper。远端 agent_launch 的 runtime_executable/runtime_sha256 键（含空值）被公开 schema 拒绝；本机 Orca 受管实现仍可使用 helper。模型、effort、access_mode 只承诺按冻结配置传递。

调试生成器远端入口使用 -Orca -OrcaEnvironment <environment>，可传 -AgentExecutable/-AgentModel/-AgentReasoningEffort，不传 helper 参数。完整远端环境检查为 unsupported，不阻断普通 Run。service verify-stopped 的逐 Run stop_evidence_scope=orca_terminal 由 Runner 只读推导；SteerLine 展示该字段，部署继续只依据 resources_settled，旧未知仍阻断。见[本次接入验收](../acceptance/orca-remote-native.md)。

Runner 远端机制已随主干 `d4969d0`（PR #18）合入，其独立验收见 [Orca 远端记录](https://github.com/erranliu/SteerRunner/blob/d4969d0/docs/acceptance/orca-remote-20261017.md)。SteerLine 产品接线的本次结果独立登记在[专属验收](../acceptance/orca-runtime-wiring.md)，不以 Runner 独立结果替代产品验证或改写旧安装候选。


原生结果契约：execution.native_outcome / final.native_outcome 为 succeeded、failed、cancelled、unknown 或 unreported。unreported 表示接口未提供 outcome，不表示底层成功；只有 Runner 已核实原回合、完整结果及既有业务门后才能推进状态。SteerLine 原样保存/传递；任务状态、可用操作及 PR 事实仍由 Runner 给出，部署仍看 resources_settled。显式 unknown、失败、取消、未知输入不放行；历史记录不重新分类或自动恢复。CLI 结构/version v1 不变，不新增消费端容错开关。
