# SteerRunner 依赖与运行


2026-10-28 调用方提示词改为短指令与固定文档：新请求仅保留动作、引用及少量参数，完整安排、规则、原文和报告通过既有固定仓库/工作区读取；缺文件或版本不符在投递前阻断，无 inline 兜底。generate 使用固定 `generate.repository` 且 materials 为空；schema 仅保留结构，结果仍核对原文和 manifest。完整冻结安排与原报告保存在原 owner；环境下一轮缺少完整报告到新工作区的正式渠道时停止创建新 Run。用户完整回答最多 2000 UTF-8 字节，超限拒绝；旧请求仅观察原冻结输入。[设计与供给边界](../../SteerlineFlow/spec/caller-prompt-convergence.spec.md)、[实际验收](../../SteerlineFlow/testing/acceptance/caller-prompt-convergence.md)。

2026-10-26：项目环境适配已接入Runner主干`74d594c5f00e32e97d2ee911db5ac787302022bc`的最终result快照。要求独立`steerrunner-result-file/v1`、公开`collection=end_only`及`query_reads_file=false`；progress原契约不变。环境适配在pr_ready等原流程状态触发stop，确认停止后验收唯一closed快照；closed/version=0保留采集失败原因，不能刷新或放行。固定EXE公开接口和受控Run回传/结算、L1/L2及Web已验证；真实远端L2+完整PR业务验收单独安排，详见[环境适配验收](../../SteerlineFlow/testing/acceptance/project-environment-adaptation.md)。

2026-10-25：调用方已接入准备查询等待 `steerrunner-execution-wait/v1`。新 Orca 候选要求 `capabilities.execution_wait` 的公开 nullable schema 及原 `run_observation` 同快照能力；不足时仅该 profile 不可启动，原候选读取/停止和其它 adapter 依原资格。原 Run 对象或显式 null 随 controls/revision/事件原子保存，经 `runner_delivery.execution_wait` 展示。SteerLine 不重试 Orca CLI，也不从显示时间推导 blocked 或自动 resume。Runner #57 已合入 `5df57b6`，fixture 已固定其正式 schema；正常正式构建已通过扫描/只读与原触发路径受控复验，历史 Defender 告警单列保留；后续固定 Runner 与远端 Orca 的同候选等待、恢复和停止联验已通过，完整 Web 进程重启等限制仍见专属验收。具体范围见[Spec](../../SteerlineFlow/spec/runner-wait-retry.spec.md)和[验收](../../SteerlineFlow/testing/acceptance/runner-wait-retry.md)。

2026-10-24：候选测试汇报改用 [progress-file 单文件通道](runtime-progress.md)，调用方冻结布尔字段、独立保存快照并解释展示。候选侧原 R2 收集、workspace-git 当前通过判定和自动转测试历史已移除；下文 2026-10-20 的 R2 接入只记录历史版本。独立标准报告与显式登记保留。安装及旧现场不自动迁移。


执行页“用量与费用”消费 Runner #47 / `61d9317524b56f148b2b31e2f23a4acc1de58081` 的 `run-usage`、`estimate-run-cost`，以同一用量快照估算候选累计主会话费用。页面不触发日志补采或执行输入；缺价表仍显示 Token，未知不显示零，部分费用标记“已计价部分”。当前资格为 Orca 本机 Codex/Pi/DSH 和远端 Codex；`codex-app-server` 与远端 Pi/DSH 未由本次接入扩展。接入与测试范围见[Plan](../../SteerlineFlow/plans/run-usage-cost-ui.plan.md)和[验收](../../SteerlineFlow/testing/acceptance/run-usage-cost-ui.md)。

`execution.json` 的可选顶层 `pricing_policy` 为 Runner 所在机器上的绝对 JSON 文件路径，例如 `"pricing_policy": "C:/MySteerLine/pricing-policy.json"`。价表使用所选 Runner 发布的 `run-pricing-policy/v1`，明确 ID、版本、来源、币种、生效区间和模型费率；不内置价格、不联网抓价。金额是 token 的 API 等价估算，不是实际扣费，排除订阅/credits 结算与工具/外部 API 附加费。价表在估价时由 Runner 独立读取，刷新可以按新策略重算，用量事实不变；设置或更换文件路径需保存配置并显式重启 SteerLine。

本机采集沿 Runner 默认策略。远端采集需要在 `execution.json` 顶层显式配置 Runner 的 `usage_policy`，启动时原样传入原 `service start`；例如关闭远端新增目录登记的策略为 `{"enabled":true,"remote_log_access":{"mode":"disabled","grants":[]}}`。允许远端登记时使用 Runner 公开 schema 的 `temporary` 模式，并显式指定 environment、已核实的稳定 host_id 和受限 session_roots。不要使用账号 home 或凭据根作为授权范围；具体根资格由 Runner 验证。策略省略不扩权，页面读取不授权；更改须经正式停服/启动，冲突不热更新。Web 编辑执行 profile 时保留这两项部署字段。

SteerLine 只通过 `steerrunner-cli/v1` 调用独立 EXE。Runner 同一个 EXE 提供短命 CLI 与常驻服务；CLI 连接同一 state-dir 的现有实例，读请求不自动启动服务。任务、DAG、候选参与及 TaskDelivery 留在 SteerLine。

当前恢复与更新按[服务换代恢复契约](../../SteerlineFlow/spec/service-generation-recovery.spec.md)：相关契约兼容时保留原 Run/授权与执行资源，使用 verify-takeover 核对候选构建，不以全部资源先停止为固定前提。下述首次协作 Spec 4.2 的同构建限制和“未结算阻断换构建”仅保留历史范围；删除目录、重置和破坏性迁移仍须取得原范围资源结算证明。资源丢失、不兼容或未知原操作不能通过更新解除。

开发候选在预留事务内分配页面显示的候选序号，并将 `name_prefix`（C1 为 `sl-<task_id>`，C2 起为 `sl-<task_id>-c<Number>`）与最终 submit 一起冻结。Runner 将其作为通用命名标识，Orca worktree 使用 `<name_prefix>-<Runner Run ID>`；未提供时前缀为 `sr`。该字段不从 correlation 推导，也不改变原资源绑定或分支规则；需要发布了此字段/schema 的 Runner 构建，两仓验证见[专属验收](../../SteerlineFlow/testing/acceptance/runner-name-prefix.md)。

所有 LLM 执行操作统一经 Runner 承担，职责声明见[LLM 执行边界](../architecture.md#llm-执行边界)。SteerLine 的默认预设选择与 Runner 内部 LLM 操作的默认链路分别说明于[两层默认链路](../product-design/execution-profiles.md#两层默认链路)；实际接入使用所选构建发布的公开配置与能力契约。

测试安排生成已接入 Runner PR #33 / `9586ac4` 的 `generate/1`，复用指定本机 Codex profile 的 executable/model/reasoning_effort，并显式构造 read_only；开发认证、权限、audit 和提示词不随之复制。内容流程的公开目录允许空 prompt_steps；开发候选拒绝 generate/conversation workflow。只接生成所需的 submit/inspect/content_result/stop，conversation 操作留待规划台。2026-10-20 核对 Runner PR #34 的 R2 已随历史固定版本 `bf3ec58` 发布：`artifacts discover/collect/list/read` 和 `workspace-git`，capabilities 的 run_evidence/schema 给出正式约束。当时 SteerLine 曾接入这五个命令及能力/schema 校验（候选侧收集现已移除）：原候选的 collect 请求先持久化，未知只 inspect 原请求，read 核验 Run/artifact/长度/base64/SHA256 后保存不可变副本；只读查询结合近期 verified commit/clean 计算当前结果。原 Run 使用项目忽略的 `.steerrunner/artifacts/<唯一执行ID>/`；首版收集 managed 报告与同目录 summary.json。独立生成对象已纳入兼容 takeover、启动前原 stop outbox 与离线结算核验，保留历史 build pin 和原请求。历史 bf3ec58 的 generate 接管误校验已由 Runner #37 修复：freeze/takeover 按 workflow 能力及实际 parser 意图判断结果解析配置。干净主干 13ef123 的同 EXE 真实 generate 及原 k 包只读复验通过；历史 A/B 接管另按其固定构建取证，不改写原 pin 或扩大恢复授权。当前报告格式见[测试规范](../../SteerlineFlow/testing/reports.md)，历史版本及验证边界见[本任务验收](../../SteerlineFlow/testing/acceptance/test-management.md)。

多 workflow 与分层冻结见[协作 Spec](../../SteerlineFlow/spec/runner-collaboration.spec.md) / [Plan](../../SteerlineFlow/plans/runner-collaboration.plan.md)，已实现调用方与 Runner 组合 schema。实际 workflow ID/version 取固定构建的公开目录，不从首次接入版本推定。样例不进入生产 capabilities；空闲契约查询不证明 Run 读取接通，须在同一真实 Run 经正式详情/历史核验。运行中的 ProjectRuntime 保持原实例绑定，单独重启 Runner 不授权自动重绑；显式重启 SteerLine 后核验新实例，旧停止请求不影响新实例。跨构建接管与未结算资源按[服务换代恢复契约](../../SteerlineFlow/spec/service-generation-recovery.spec.md)逐项核验。实际固定构建及首次协作验证见[协作验收](../../SteerlineFlow/testing/acceptance/runner-collaboration.md)，首次接入基线仅用于追溯。

首次接入基线来自 [SteerRunner](https://github.com/erranliu/SteerRunner)，执行实现 commit `41908e641b34a890448158d2f21a9edc597d08cb`（已可从远端获取）。Windows/amd64 验证构建使用 Go 1.26.4、版本 `0.1.0-dev.1`，EXE SHA-256 为 `5eb9c3a9162d13449d55d363dd136e0ee50dea0d1e06ac5afaa8b439b73e5a71`。当前没有正式 Release；[Runner 草稿 PR](https://github.com/erranliu/SteerRunner/pull/1)与[接入验收](../../SteerlineFlow/testing/acceptance/steerrunner-cutover.md)记录实际通过和剩余项。

在 Runner 仓库检出所选提交后运行 `./scripts/build.ps1`。构建输出 EXE 和 `.build.json`；保留其来源和 SHA。运行不需要 Runner 或 SteerLine 源码、模板、Node 或 Python；Git、Codex / Orca 以及既有原生登录属于执行环境依赖，不打包认证或用户配置。

将 [execution.json](../../examples/execution.json) 放入自己的 SteerLine home，填入所选 EXE 的绝对路径、真实 version 和 SHA。示例占位 SHA 会拒绝启动，不能直接当作已安装配置。后端 settings 是 Runner 发布的公开 JSON，不再接受 implementation_configs、内部 codec 或旧版本配置。

调试先用 `./scripts/new-debug-release.ps1 -SteerRunnerExe C:/Tools/SteerRunner/steerrunner.exe` 生成新包，需要固定原生程序时可传 `-AgentExecutable <absolute-codex.exe> -AgentModel gpt-6-luna -AgentReasoningEffort medium`。生成器把 EXE 固定到包内并核对 SHA。执行方案启动后冻结，换 EXE / 方案创建新包；禁止在安装现场调试。

每个 home 由应用宿主持有一个 Runner 服务，项目借用同一固定客户端；整体关闭统一结算，单项目 caller 退出保留原 Run/检查事实，已有执行可能继续。产品请求号与 Runner 随机请求号分别持久化，跨项目同号不冲突。当前增量见[共享服务验收](../../SteerlineFlow/testing/acceptance/shared-runner-service.md)。

SteerLine 为 Runner 指定本地卷根下的短目录：Windows 为 `<drive>:\slr\<26位base32摘要>`，摘要绑定带用途标记的规范绝对 home，同 home 项目共用一目录。深安装/调试目录不会增加 Runner 的认证 socket 路径，标准本地卷下当前 socket 为 98 字节；Runner 自身 106 字节限制不变。非 Windows 使用系统临时目录下同规则的 `slr/`，并检查相同预算。Task 数据库保留在 home；Runner 的 SQLite、执行材料、缓存、工作区与 `logs/` 归短目录。候选保存准确 state-dir，排查须同时提供 state-dir 和 Run ID。新布局不读取或迁移旧项目短目录或 `run-dir/runner`。应用在 `home/application-api/runner-build.json` 保留不可变构建 pin，先核验再启动共享服务；项目 checkpoint/候选/检查保存原操作绑定。

查询不创建目录；显式服务启动才创建当前用户私有目录，拒绝重定向/网络卷。共享目录的整包 reset 须核对全部项目、检查、生成对象和原 Run 的停止/结算，不能只看当前项目；删除只针对该 home 的准确摘要目录，不能删除共享 `slr` 根。短路径来源见[短目录验收](../../SteerlineFlow/testing/acceptance/runner-short-state-path.md)，当前共享/reset 边界见[共享服务验收](../../SteerlineFlow/testing/acceptance/shared-runner-service.md)。SteerLine 只通过 CLI 读取公开 Run/事件，不读取 Runner 私有数据库或原生日志。

新提交的恢复、回答和反馈需要当前公开 revision 的显式授权。兼容服务重启保留 Runner 原授权、会话与等待状态，可补收原结果并接续已证明未发送的后继，不因重启批量 resume；原资源丢失、不兼容、业务阻断和显式 stop 仍按原门槛处理。SteerLine 不自动生成新的 submit/reply/resume；旧调用方 pending 控制撤回，unknown 只查原 request_id。Runner 已受理且绑定原问题/执行范围的未发送答案按原授权对账，旧 epoch 或错范围仍需人工确认。不能换 ID 重发未知操作。详见[换代恢复契约](../../SteerlineFlow/spec/service-generation-recovery.spec.md)。

交付摘要只读取已有 Run 投影和原命令回执，分别展示受理、原生结果、业务阶段、PR 与资源停止；命令 completed、pr_ready 和 resources_settled 不能互相替代。刷新不发送输入，不创建资源；完整 environment-check 和纯 validate 按需使用，候选测试进度由独立 progress-file 查询同步，Run 内的权限/来源/发布门槛保持。摘要实现与验收见[可靠性交付简化](../../SteerlineFlow/testing/acceptance/reliable-delivery-simplification.md)。

原生查看通过 `open-native-session --run-id R --attempt-id A --session-id S --generation G` 调用；只有 Runner 发布的 `native_session_viewer.adapters` 中相应 adapter 可用时启用按钮。原服务负责私有 binding 和实时进程/thread/workspace 核验，调用方不读取这些私有材料。详情与本次构建/验证范围见[原会话查看验收](../../SteerlineFlow/testing/acceptance/native-session-viewer.md)。

绑定运行时的业务和查询使用 `--expected-instance-id`，必须选择发布 `instance_binding.expected_instance_id: true` 的 Runner。该参数是实际请求的前置，不能用先查询 status 代替；绑定调用不会隐式启动替代服务。显式重启 SteerLine 后先只读核对原 Run/输入或原 submit 请求，再开放准入。原 pending 输入撤销，unknown 输入只读对账，不自动重发。该增量随两仓协作 PR 同步交付，旧构建不能用于新调用方联验。
远端 Orca 使用执行主机已有 Codex/Git/gh 登录，通过原生 Orca 链路启动，不部署或运行 SteerRunner helper。远端 agent_launch 的 runtime_executable/runtime_sha256 键（含空值）被公开 schema 拒绝；本机 Orca 受管实现仍可使用 helper。模型、effort、access_mode 只承诺按冻结配置传递。

调试生成器远端入口使用 -Orca -OrcaEnvironment <environment>，可传 -AgentExecutable/-AgentModel/-AgentReasoningEffort，不传 helper 参数。完整远端环境检查为 unsupported，不阻断普通 Run。service verify-stopped 的逐 Run stop_evidence_scope=orca_terminal 由 Runner 只读推导；SteerLine 展示该字段，部署继续只依据 resources_settled，旧未知仍阻断。见[本次接入验收](../../SteerlineFlow/testing/acceptance/orca-remote-native.md)。

Runner 远端机制已随主干 `d4969d0`（PR #18）合入，其独立验收见 [Orca 远端记录](https://github.com/erranliu/SteerRunner/blob/d4969d0/docs/acceptance/orca-remote-20261017.md)。SteerLine 产品接线的本次结果独立登记在[专属验收](../../SteerlineFlow/testing/acceptance/orca-runtime-wiring.md)，不以 Runner 独立结果替代产品验证或改写旧安装候选。


原生结果契约：execution.native_outcome / final.native_outcome 为 succeeded、failed、cancelled、unknown 或 unreported。unreported 表示接口未提供 outcome，不表示底层成功；只有 Runner 已核实原回合、完整结果及既有业务门后才能推进状态。SteerLine 原样保存/传递；任务状态、可用操作及 PR 事实仍由 Runner 给出，部署仍看 resources_settled。显式 unknown、失败、取消、未知输入不放行；历史记录不重新分类或自动恢复。CLI 结构/version v1 不变，不新增消费端容错开关。


## Roadmap 固定版本文档生成（2026-10-27）

已接入 Runner aa8c392f9798d05184788d83947dd7bf4afc75a6 的 generate/1 扩展。输入 generate.repository={local_path,commit}；Runner 从已有本地 Git 对象提供固定提交的只读快照，不采用脏工作树。公开 capabilities.generate / configuration_schemas.generate_input 判断能力，结果仍由 inspect 的 content_result.json 交付；Run 与结果均有 repository_source（commit/tree/files/blob/sha256/size_bytes）。manifest 证明可用文件版本，不证明模型逐文件已读或推论正确。

整个 JSON 上限 1 MiB，无分页；SteerLine 固定内部代号和语义文档路径，校验完整文件集、原文/确认摘要、代码提交及调查引用。原请求、停止/资源结算和接管沿既有机制；文档生成不创建 Task/Candidate、不分配编号、不写 Git/PR。规范/版本缺失或结果不完整保持阻断。

真实接入构建和本机 Codex 验证见 [Roadmap 验收](../../SteerlineFlow/testing/acceptance/roadmap-promotion.md)。使用时在本环境 execution.json 选择支持该 capability 的 Runner EXE，并核对实际版本与 SHA；不修改安装版或复用旧服务现场。流程与 API 见 [产品设计](../product-design/roadmap-promotion.md)。
