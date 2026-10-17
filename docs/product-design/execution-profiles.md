# 全局执行配置

当前范围为本机 Codex 与远端 Orca environment；本机 Orca 已放弃。执行由独立 SteerRunner 管理。SteerLine 保留多执行方案与默认方案选择，不维护原生 codec、权限策略、会话或 workflow 状态机。

所有项目共享 `<STEERLINE_HOME>/execution.json`，当前严格格式为 3。`runner` 固定独立 EXE 的绝对路径、版本与 SHA-256；每次 CLI 调用前核对 SHA，同目录常驻实例也须匹配所选 EXE。旧版本、implementation_configs、内部 codec 和项目内旧执行方案不兼容，不迁移或回退。

按 [公开示例](../../examples/execution.json)填写实际路径和 SHA。`execution_profiles` 每项包含 id/name、execution 与 workflow；execution 的 adapter/target/settings 直接使用 Runner 发布的公开契约，SteerLine 按公开 schema 验证，不能通过文件或网络加载额外 schema。项目文件保留仓库、Git/PR provider、Task 契约和 publication_policy；执行侧不从项目复制认证或用户配置。

本机方案使用 `codex-app-server` 和 `target.kind=local`，settings 包含固定 native executable、model 和 reasoning_effort。远端方案使用 `orca-terminal`、`target.kind=orca_environment` 与明确的 environment 名称；transport 指定 Orca CLI，agent_launch 使用公开 executable/args/model/reasoning_effort，并显式固定执行主机的 `runtime_executable`（可信 steerrunner-agent）和 `runtime_sha256`。GitHub 业务不能仅靠 Orca 默认 TUI 启动；Runner 需先取得 helper 的同权限认证准备回执。执行主机须已注册匹配仓库，并取得固定 base commit；身份或权限缺证时阻断。接线与本次验证见[专属验收](../acceptance/orca-runtime-wiring.md)。

普通 workflow 为 development/1，审计预设显式选择 development-audited/1，并提供 workflow.audit_model；Runner 发布 workflows、workflow schema 和自包含的 run_configuration 组合 schema，caller 不补执行默认值。Runner 内嵌流程、提示词与 schema，原生代理在 implement、report、publish 的独立输入中执行。受理时固定 Spec/Plan 材料、规范化参数与提示词模板/覆盖正文；每轮最终输入在结果/反馈与授权确定后、发送前保存，恢复保留原正文和模型参数，不在受理时预生成未来轮次或原生身份。定制正文仍须保留固定材料和机器结果契约。SteerLine 只选择公开方案，不解释原生终端文本推进业务。

[协作 Spec](../spec/runner-collaboration.spec.md) / [Plan](../spec/runner-collaboration.plan.md)定义预设/提交/受理数据及审计 workflow 接入。其 4.2 分开规定同构建换预设、服务换代与更换构建：原 Run 参数不变；新 Runner 实例仅在用户显式重启 SteerLine、重建 ProjectRuntime 的初始化时核验接纳，旧运行时发现替代实例拒绝新业务输入，不自动重新绑定。未结算对象阻断换构建，本期新构建使用无旧执行状态的新环境。调用方已实现上述准入和多 workflow 展示；构建 pin 持久在项目 checkpoint，候选冻结原名称/参数摘要/完整提交，参数摘要仅覆盖 execution + workflow 的有序对象 JSON；不支持升级/接管旧现场。实际结果归[专属验收](../acceptance/runner-collaboration.md)。

## 权限与登录

2026-10-16 接入：SteerLine caller/HTTP/Web 已实现，真实 Runner 发布交接待前置。[环境检查接入 Spec](../spec/environment-check.spec.md) / [Plan](../spec/environment-check.plan.md)消费 Runner 独立规格发布的 `execution.settings.access_mode=sandbox|full_access`，缺省 sandbox；检查与业务启动使用同源冻结模式，均不依赖人工/自动审批。认证类型和准备由 Runner 原 owner 决定，不新增宿主登录回退。主动 Agent 检查不替代业务无模型确定性预检；历史报告不替代新工作区必要核验。Runner 沙箱认证的作用域冲突及远端前置须在其最终版本落实，SteerLine 不生成原生权限参数，不据文档宣称双模式或远端已可用。

本机 Codex 的固定原生权限资源、MXC AppContainer、Git 管理根及临时目录规则由 Runner 管理。原生拒绝与未核实事实如实记录；SteerLine 不扩大写根、不添加权限回退，也不代代理发布。

Git、Codex、Orca 和原有登录属于执行环境。生成调试包不复制 auth、token、hooks、sandbox 凭据、数据库或安装配置。远端使用该 environment 的原生权限与登录，不套用本机 Codex 沙箱结论。

## 运行、停止与恢复

同一个 Runner EXE 提供短命 CLI 和常驻服务。SteerLine 显式启动或连接项目 state-dir 对应服务，通过 CLI 读写公开 Run/事件；只读查询不会启动服务。Runner 状态归完整应用 run-dir 派生的独立短目录（Windows 为卷根 `slr/<摘要>`），由 Runner 单写，具体布局见[接入说明](../integration/steerrunner.md)；SteerLine 只保存候选绑定、原 CLI outbox 和公开事件投影。

accepted 仅表示请求已持久受理。未知结果查询原 request_id；重启不重新发送旧答案、反馈或 resume。回答和恢复须使用当前公开 revision 的新授权。Task 终态的停止义务留在 SteerLine，原 Run 停止与原生资源停止分别取证；不能用父进程退出、终端空闲或服务停服回执代替执行停止。

[接入与固定构建](../integration/steerrunner.md)、[生命周期规则](worker-lifecycle.md)、[接入验收](../acceptance/steerrunner-cutover.md)分别维护使用方法、统一规则和实际结果。
