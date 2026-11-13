# SteerRunner 接入与运行

SteerLine 通过独立 Runner EXE 的公开 `steerrunner-cli/v1` 使用模型、原生 Agent 和执行工作流。Task、依赖、候选参与及唯一交付由 SteerLine 管理；工作区、会话、逐轮输入和执行资源由 Runner 管理。长期职责见[项目流程](../product-design/project-workflow.md)，异常及恢复见[Runner 调用生命周期](../product-design/runner-lifecycle.md)。

[SteerAgent](../product-design/steer-agent.md) 维护 SteerLine 的智能工作能力分类及协作边界。业务类型不与 Runner workflow 一一对应，分类不新增执行权限；本页继续维护实际接口与构建要求。

本文说明当前接入。所选构建的公开能力及对应验收决定实际可用范围；源码接通不表示安装版已部署，旧构建通过不表示当前组合已验。首次接入构建、旧接管和历次修复统一从[历史证据入口](../../SteerlineFlow/archive/design/runner-reliability-history.md)追溯。

费用按 Runner 冻结阶段输入的 `step_id/configured_model/configured_reasoning_effort` 分组估算，要求公开能力 `configured_model_attribution=frozen_stage_input`、`observed_model_field=model`（Runner #133，`9243a79b`）。workflow 可编排 execution/audit 模型切换，Runner 不自主降级或换模型。SteerLine 按配置模型和候选创建时间匹配当前价表；实际观测模型与时间缺失不阻断估算，未分配配置模型或价格歧义的用量保留未计价。详见[计费职责](../../SteerlineFlow/spec/billing-ownership.spec.md)及[本次验证](../../SteerlineFlow/testing/acceptance/configured-model-billing.md)。

## 依赖与固定构建

在独立 Runner 仓库检出所选提交，使用其构建入口生成 EXE 和来源记录，保留真实 version、SHA-256 和资源摘要。运行不需要 Runner 源码、模板、Node 或 Python；Git、Codex/Orca 和已有原生登录是执行环境依赖，不打包用户认证。

在应用 home 配置 [execution.json](../../examples/execution.json)，填写 EXE 绝对路径、真实版本和摘要；示例占位值不能用于启动。部署选择、执行预设和每次任务输入各有固定时点，细节归[执行方案](../product-design/execution-configuration.md)。新配置不改写旧候选的原提交，流程 ID/version 取所选构建的公开目录。

调试使用独立包，例如：

```powershell
./scripts/new-debug-release.ps1 -SteerRunnerExe C:/Tools/SteerRunner/steerrunner.exe
```

需要指定原生程序或模型时使用生成器的 `-AgentExecutable`、`-AgentModel`、`-AgentReasoningEffort`。换 EXE 或冻结方案须创建新包；构建、安装及隔离调试步骤见[安装部署](install-deploy.md)和[独立调试](../runtime-install-debug.md)。

## 服务、目录与实例

Runner 同一 EXE 提供短命 CLI 和常驻服务。应用宿主持有每 home 一个固定 Runner 服务，各项目借用客户端；读取不自动启动服务。项目退出保留原 Run 与停止责任，整体关闭统一结算。

Runner state-dir 由规范绝对 home 和当前 v8 命名空间派生：Windows 位于本地卷根 `<drive>:\slr\<26位base32摘要>`，非 Windows 使用系统临时目录下的同类短目录。业务库仍按项目隔离，Runner 状态、材料、工作区和日志归其短目录。排查从运行信息取得准确 state-dir 和 Run ID，不手拼摘要或读私有数据库。布局与隔离规则见[路径清单](../../SteerlineFlow/paths.md)。

应用在 `home/application-api/runner-build.json` 保存构建 pin，启动前核验。查询不创建目录；显式启动才准备私有目录，拒绝重定向和网络卷。旧目录、旧 pin 或旧格式不自动迁移。reset 须核对全部项目、生成、环境准备及未知原请求，准确清理授权目标，不能删除共享 `slr` 根。

运行时绑定原服务实例及构建、目录和资源摘要。实际请求须带所选能力要求的期望绑定；不以先查询 status 代替实际命令围栏。存活的 ProjectRuntime 发现 Runner 替代实例时拒绝新业务，显式重启 SteerLine 后先核对原 Run/输入及停止义务，再开放准入。

## 配置与能力要求

| 能力 | 当前接入边界 |
| --- | --- |
| 原生目录 | 使用固定 EXE 的无服务 `native-catalog`；Runner 管原生连接、认证及元数据进程，Web 管在途查询、产品支持判断和保存 |
| 执行配置 | 无服务 `execution-config` 提供描述、派生、构造及来源 identity；Runner 解释原生 settings 和模型/effort 组合，Web 编辑逻辑选择，服务端重验可信保存版本 |
| 工作流 | 按公开目录及组合 schema 验证预设；Runner 决定步骤、审计及修复预算，SteerLine 选择并展示 |
| 候选观察 | 在线目录须提供 `steerrunner-run-observation/v1`、`same_snapshot: true` 及至少 100 条每页；正常同步用 `observe` |
| Orca 观察重连 | Runner 在原轮次上有界退避重连。SteerLine 将 `running / observation_reconnecting` 显示为「正在重连」，将要求人工恢复的阻断显示为「需处理」 |
| 原请求与控制 | 原请求对账、启动前离线读取及控制前 revision 使用 `inspect` 等既有入口；按钮资格来自公开 controls 与调用方业务资格 |
| 等待状态 | 准备等待展示 `execution_wait` 对象或显式 null，保留原 revision；SteerLine 不重试 Orca CLI，不根据倒计时自动恢复 |
| 原会话查看 | 仅公开 `native_session_viewer.adapters` 声明支持时启用；Runner 校验原 Run/Attempt/session/generation 并打开，不创建替代业务会话 |

缺少必要 schema、能力或固定材料时明确显示不可用，不退回旧字段、普通流程或内联材料。目录查询不取得项目就绪资格，也不能用空闲能力查询证明真实 Run 读取已接通。配置详情见[执行方案](../product-design/execution-configuration.md)，接线及已验范围见[目录与边界验收](../../SteerlineFlow/testing/acceptance/runner-boundary-consolidation.md)、[执行配置验收](../../SteerlineFlow/testing/acceptance/native-execution-configuration-boundary.md)。

## 执行、观察与交付

SteerLine 固定项目 commit、任务材料、执行方案和完整请求后提交；Runner 按固定 commit 创建专属 worktree。成功创建后进入所选工作流，正常新启动不再叠加 HEAD、diff 或洁净度预检。原资源身份和未知操作沿生命周期核实，旧 Run 恢复独立处理。

候选序号、显示名及工作区 `name_prefix` 随原提交固定，Runner 不从业务关联字段猜命名；规则和已验范围见[命名前缀 Plan](../../SteerlineFlow/plans/runner-name-prefix.plan.md)及[验收](../../SteerlineFlow/testing/acceptance/runner-name-prefix.md)。

完整规则、测试安排和原文按各自固定文件来源供给；缺文件或版本不符时阻断，不内联补发。Runner 受理时固定运行依据，逐轮保存实际输入及来源；配置刷新不重建旧请求。具体供给规则归[提示词维护](../prompt-governance.md)。

SteerLine 校验公开 Run、输入、事件与游标绑定并保存投影。`native_outcome` 的失败、取消、unknown 或 unreported 均按原义保留；缺失不推定成功。公开阶段、原生结束、PR 就绪与资源结算分开展示。

PR 就绪后允许的显式 message 沿原会话串行执行，不新增正常准备或现场预检；未知只查原请求。手动解决冲突由 SteerLine 提供业务入口，见[续作验收](../../SteerlineFlow/testing/acceptance/pr-conflict-resolution.md)。Runner 自主核验工作流发布；任务交付由 SteerLine 按候选冻结的 provider 直接查询平台并提交唯一交付，完整链路见[可靠性交付](../architecture/reliable-delivery-chain.md)。

## 停止、重启与升级

同构建重启先核对原执行、停止义务和会话。Runner 保留原授权并可接续已证明未发送的后继；调用方 pending 输入撤回，unknown 仅观察原请求，SteerLine 不自动生成新的 submit/reply/message/resume。新反馈或恢复需符合当前 revision、controls 和显式授权。

当前状态格式为 `steerrunner-state/v8`。换构建前由原版本排空业务、原生资源及未完成控制；原服务退出、PR 就绪或资源停止其中一项均不足。已删除 `service verify-takeover`，不支持活动任务跨构建接管。公开 `verify-stopped` 与离线 inspect 按原 Run/input 集合核验；失败保留原 EXE、配置、状态和诊断。

v7 及更早状态由原安装收尾，新安装使用分离的 home 与当前命名空间；不迁移、双写或回退读取。未来格式兼容且已排空的构建可按正式核验复用目录，历史构建摘要只表明来源。部署与显式放弃旧运行的完整操作归[安装部署](install-deploy.md#同构建更新与重启)，本页不授权安装维护。

## 原生路径与业务入口

当前配置覆盖本地 Codex、Windows/amd64 本机 Orca 和远端 Orca，逐项能力按所选构建和验收核对。远端 Orca 使用执行主机已有登录及原生链路，不部署 Runner helper；远端 `agent_launch.runtime_executable/runtime_sha256` 键连同空值均被拒绝，本机受管路径仍可使用其支持的 helper。模型、effort 和权限只按冻结配置及公开事实说明。

远端调试使用生成器的 `-Orca -OrcaEnvironment <environment>`。停止证据的 `orca_terminal` scope 仅证明原终端，不扩大为后台进程或凭据已清理；缺证据仍阻断。实际范围见[远端原生验收](../../SteerlineFlow/testing/acceptance/orca-remote-native.md)。

| 业务 | 唯一说明入口 |
| --- | --- |
| 项目材料与测试安排生成 | [项目维护](../product-design/project-maintenance.md)；生成 Run 不伪造开发候选 |
| Roadmap 解析及固定版本文档生成 | [Roadmap 设计](../product-design/roadmap-promotion.md)；文件版本证明不等于模型逐文件已读或结论正确 |
| 项目环境检测与结果封存 | [项目环境工作流](../../SteerlineFlow/spec/project-environment-workflow.spec.md)；使用普通 Run，旧专用 Check 已退役 |
| 重构与维护 | [实现边界](../architecture/refactoring-maintenance.md)。仅本地 Codex 完整组合。generate 首次使用固定提交只读目录，后续决策和报告使用文件材料。所有写入经 execute/1，后继引用最新 `reuse_worktree_from_run_id`。能力目录须公布正式文件材料、最终封存、`worktree_reuse` 和 `result_file.required_initialization`，维护可写请求设置 `result_file.require_initialization`。费用分 main_session 与 runner_internal 验证，已有任务费用口径为主会话。自定义维护 PR query 须返回目标分支当前提交 `base_revision`、PR 的 `head_revision` 和 `body`。GitHub 调试模板从 `baseRef.target.oid` 读取目标分支提交。固定受测 EXE 与验证范围见[交付验收](../../SteerlineFlow/testing/acceptance/refactoring-agent-delivery.md)。 |
| 候选测试进度 | [progress-file](runtime-progress.md)；旧 R2 候选产物收集已移除 |
| 用量、费用及远端采集授权 | [计费职责](../../SteerlineFlow/spec/billing-ownership.spec.md) · [验收](../../SteerlineFlow/testing/acceptance/billing-ownership.md)；Runner 提供用量，SteerLine 管理价表和估算，采集授权独立 |

本文只维护当前使用方法与能力要求。版本 SHA、历史修复、通过与未验明细留在对应验收；[v8 接入记录](../../SteerlineFlow/testing/acceptance/runner-v8-install-reset.md)及[历史证据入口](../../SteerlineFlow/archive/design/runner-reliability-history.md)保留原范围，不据此扩大其他组合的资格。
