# 全局执行配置

当前实现支持本机 Codex、Windows/amd64 本机 Orca/Codex、Pi、DSH 及远端 Orca environment/Codex。详见[Web 执行配置 Spec](../spec/web-execution-config.spec.md)及[Plan](../spec/web-execution-config.plan.md)，实际验证和人工验收独立登记。执行由独立 SteerRunner 管理。SteerLine 保留多执行方案与默认方案选择，不维护业务执行的原生 codec、权限策略、会话或 workflow 状态机。所有 LLM 操作的职责以[架构边界](../architecture.md#llm-执行边界)为准，本页维护两层默认链路的配置含义。

所有项目共享 `<STEERLINE_HOME>/execution.json`，当前严格格式为 3。`runner` 固定独立 EXE 的绝对路径、版本与 SHA-256；每次 CLI 调用前核对 SHA，同目录常驻实例也须匹配所选 EXE。旧版本、implementation_configs、内部 codec 和项目内旧执行方案不兼容，不迁移或回退。

按 [公开示例](../../examples/execution.json)填写实际路径和 SHA。`execution_profiles` 每项包含 id/name、execution 与 workflow；execution 的 adapter/target/settings 直接使用 Runner 发布的公开契约，SteerLine 按公开 schema 验证，不能通过文件或网络加载额外 schema。项目文件保留仓库、Git/PR provider、Task 契约和 publication_policy；执行侧不从项目复制认证或用户配置。

本机方案使用 codex-app-server 和 local target。远端方案使用 orca-terminal、orca_environment 与明确 environment；transport 指定 Orca CLI，agent_launch 只配置 native executable/args/model/reasoning_effort 和 access_mode，不接受 helper runtime 路径/SHA。远端使用已有登录，停止只证明原 Orca 终端；完整 Check unsupported 不阻断普通 Run。执行主机须已注册匹配仓库并取得固定 base commit；未知操作只对账原资源。

普通与审计开发 workflow 的 ID/version 以所选 Runner 构建的公开目录为准，审计预设提供 workflow.audit_model；Runner 发布 workflows、workflow schema 和自包含的 run_configuration 组合 schema，caller 不补执行默认值。Runner 内嵌流程、提示词与 schema，原生代理在 implement、report、publish 的独立输入中执行。受理时固定 Spec/Plan 材料、规范化参数与提示词模板/覆盖正文；每轮最终输入在结果/反馈与授权确定后、发送前保存，恢复保留原正文和模型参数，不在受理时预生成未来轮次或原生身份。定制正文仍须保留固定材料和机器结果契约。SteerLine 只选择公开方案，不解释原生终端文本推进业务。

[协作 Spec](../spec/runner-collaboration.spec.md) / [Plan](../spec/runner-collaboration.plan.md)定义预设、提交、受理数据及审计 workflow 接入。原 Run 参数、候选名称/参数摘要/完整提交保持冻结；参数摘要仅覆盖 execution + workflow 的有序对象 JSON。新 Runner 实例在显式重启 SteerLine、重建 ProjectRuntime 时核验接纳，运行中的旧实例围栏保持。更换构建按后续[兼容接管契约](../spec/service-generation-recovery.spec.md)核验原候选、请求、检查及生成对象；只更新当前应用服务 pin，不改历史构建/输入/PR/Delivery 事实。应用构建 pin 位于 `application-api/runner-build.json`，项目 checkpoint 保留原操作绑定。缺兼容或原资源证明仍阻断，销毁/reset 仍要求结算。实际结果归[协作验收](../acceptance/runner-collaboration.md)与[换代验收](../acceptance/service-generation-recovery.md)。

本功能只消费外部配置能力：provider的API key/OAuth、base URL、自定义模型由目标Orca中的Agent原生配置管理。Web统一通过Orca的status/agents/modelCatalog查询本机或远端目录，不读取Pi凭据文件或另接provider发现。目录Agent事实与Web/Runner执行组合分开；Pi按目录提供Provider筛选，保存既有完整provider/model。目录可见不代表凭据或调用已验证，当前默认账号/工作区上下文仍未核实；不提供多套账号配置选择。远端Pi/DSH目录可被发现，但本期执行范围保持Codex，不能因目录而自动开放。

## Web 配置管理

[Web 执行配置管理 Spec](../spec/web-execution-config.spec.md) / [Plan](../spec/web-execution-config.plan.md)规定管理入口 `/settings/execution-configs`：Web 前后端拥有能力目录查询、预设编辑/生成及 `execution.json` 条件保存；Codex/Orca 由 Web 后端直接查询。Go 执行服务只消费配置、独立校验并冻结参数，提供原加载文件摘要的只读投影；Runner 不新增探测接口。能力目录/缓存不是执行资格或第二配置来源。首版保存后沿既有正式重启流程生效，不热加载、不自动停服或改投；冻结调试包只读。边界与实际结果见[专属验收](../acceptance/web-execution-config.md)。

本机 Orca 新增设计使用 Web 自有 named pipe 目录客户端，配置仍是 `orca-terminal + target.local`；Go 执行服务只消费配置，Runner 不新增探测接口。本机受管 helper 路径/SHA 是部署模板输入，缺少时不得用目录查询或任意程序路径代替；远端仍拒绝本机 helper 字段。新增验收为 WEC-A10–A14/M02，不能继承前版本机 Orca 被拒绝的测试结论。

## 两层默认链路

SteerLine 与 SteerRunner 各有一层默认链路，初始默认均为本地 Codex；两者分别服务调用方选择和 Runner 内部执行。

| 层次 | 默认链路的用途 | 配置归属 |
| --- | --- | --- |
| SteerLine | 未显式选择执行预设时，选择交给 Runner 的执行方案 | `<STEERLINE_HOME>/execution.json` 的 `default_execution_profile` 指向 `execution_profiles` 中的预设，预设携带 execution 与 workflow |
| SteerRunner | 为 Runner 内部需要 LLM 的操作提供默认执行链路，例如 workflow 的 parse | 默认链路的配置、解析及执行归 Runner，由其公开契约定义实际入口和允许的覆盖 |

显式选择 SteerLine 预设时使用所选预设；未选择时才使用 `default_execution_profile`。配置必须指向存在的预设，不因本地 Codex 是初始默认就允许缺失配置。SteerLine 将所选方案提交给 Runner，由 Runner 解释执行参数及其默认值。

Runner 内部操作有显式链路配置时按其契约执行，未显式指定时可使用 Runner 自身的默认链路。SteerLine 的默认预设选择不等于修改 Runner 内部默认配置；例如候选选择远端 Orca，不能据此推定 Runner 内部 parse 也使用 Orca。两层默认均为本地 Codex，也不表示共享同一份配置或同一原生会话。

默认链路用于各自范围内的缺省选择，不覆盖显式配置或已冻结运行，不作为执行失败时的隐式改投路径。Runner 内部默认链路的具体字段、操作入口和覆盖优先级须以所选构建发布的公开契约为准；本页不为其新增 SteerLine 配置字段，也不将上述职责设计视为所有相关能力已经接入或验收。

## 权限与登录

[环境检查接入 Spec](../spec/environment-check.spec.md) / [Plan](../spec/environment-check.plan.md)已对齐 Runner 正式公开契约，消费 `execution.settings.access_mode=sandbox|full_access`，缺省 sandbox；检查与业务启动使用同源冻结模式。caller/HTTP/Web 自动验证及固定版本本机 full_access 真机正例见[专属验收](../acceptance/environment-check.md)，完整人工体验仍待安排；其余权限、远端和恢复组合分别核验。认证类型和准备由 Runner 原 owner 决定；主动 Agent 检查不替代业务无模型确定性预检，历史报告不替代新工作区核验，SteerLine 不生成原生权限参数或增加登录回退。

本机 Codex 的固定原生权限资源、MXC AppContainer、Git 管理根及临时目录规则由 Runner 管理。原生拒绝与未核实事实如实记录；SteerLine 不扩大写根、不添加权限回退，也不代代理发布。

Git、Codex、Orca 和原有登录属于执行环境。生成调试包不复制 auth、token、hooks、sandbox 凭据、数据库或安装配置。远端使用该 environment 的原生权限与登录，不套用本机 Codex 沙箱结论。

## 运行、停止与恢复

同一个 Runner EXE 提供短命 CLI 和常驻服务。应用宿主在项目 census 后初始化一个 home 级 Runner，各项目借用同一固定客户端，通过公开 CLI 读写 Run/事件；只读查询不会启动服务。Runner 状态归规范绝对 home 派生的独立短目录（Windows 为卷根 `slr/<摘要>`），由 Runner 单写，具体布局见[接入说明](../integration/steerrunner.md)；Task 数据库按项目隔离，SteerLine 保存候选绑定、原 CLI outbox 和公开事件投影。

accepted 仅表示请求已持久受理。未知结果查询原 request_id；重启不重新发送旧答案、反馈或 resume。回答和恢复须使用当前公开 revision 的新授权。Task 终态的停止义务留在 SteerLine，原 Run 停止与原生资源停止分别取证；不能用父进程退出、终端空闲或服务停服回执代替执行停止。

[接入与固定构建](../integration/steerrunner.md)、[生命周期规则](worker-lifecycle.md)、[接入验收](../acceptance/steerrunner-cutover.md)分别维护使用方法、统一规则和实际结果。
