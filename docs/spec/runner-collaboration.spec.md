# SteerLine / SteerRunner 协作、执行预设与 Workflow 接入 Spec

日期：2026-10-17。状态：用户已授权实施；实现与实跑范围登记在专属验收，不自动部署。配套 [Plan](runner-collaboration.plan.md) 与[专属验收](../acceptance/runner-collaboration.md)。普通开发，不占 Task ID，不修改 roadmap。

## 1. 问题、基线与目标

当前执行已经迁入 SteerRunner，但“执行方案”同时指部署配置、可选参数和一次运行输入；公开 DTO、单 workflow 校验及旧设计文档尚未对齐 Runner 的多 workflow 能力。本次明确数据所有权和冻结时点，并接入已有 `development-audited/1`，不重建执行引擎。

源码梳理基线：SteerLine `48c5671926466fe83c23de7fee5377bc61f87188`；本地 Runner 源码 `d4969d07e41406c8a85179b6a483d42ee367b1e9`。Runner 来源为 `C:/Project/SteerRunner`，这里不是对已安装 EXE 的支持声明。实施时固定实际选用的 Runner commit、EXE SHA、version、CLI protocol 和 resource digest；不能直接沿用接入文档中的旧构建摘要。

Runner 既有能力以其固定版本的[流程选择 Spec](https://github.com/erranliu/SteerRunner/blob/d4969d07e41406c8a85179b6a483d42ee367b1e9/docs/spec/workflow-selection-audit.spec.md)、[公开类型](https://github.com/erranliu/SteerRunner/blob/d4969d07e41406c8a85179b6a483d42ee367b1e9/internal/contracts/run.go)和实际 EXE 的 capabilities/schema 为依据。本文约束调用方集成，不另定义审计结果裁决、重试或执行状态机。

## 2. 职责和数据权威

| 范围 | SteerLine | SteerRunner |
| --- | --- | --- |
| Task / DAG | 定义、资格、固定契约、依赖、领取与后继释放的唯一写者 | 不写 Task/DAG；关联标识只用于追踪 |
| 候选 | 参与关系、共同 base、候选到 Run 绑定、原请求与停止义务 | 各 Run 的执行与资源，不裁决候选胜负 |
| 执行预设 | 保存目录、ID/名称、默认项和用户选择 | 定义 execution/workflow 参数语义、支持组合和默认值 |
| 固定输入 | 从项目、任务契约和所选预设组装完整 RunInput，调用前持久化 | 验证、解析环境依赖、冻结材料、流程资源与实际输入 |
| Workflow | 选择、传递、只读展示 | 唯一推进实施、报告、审计、修复和发布；管理预算和原生输入 |
| 工作区 / 会话 / 权限 | 消费公开身份和能力，提交当前版本授权 | 准备、观察、恢复、停止、清理及认证机制的唯一 owner |
| PR / TaskDelivery | 请求刷新绑定 PR，核对事实与版本；唯一交付、Task 关闭、后继释放同事务提交 | 执行发布、查询 provider 并提供公开核验事实；不自行关闭 Task |
| 服务 | 固定 EXE、项目独立 state-dir、启停协调和原实例结算核对 | 同目录单写服务、逐 Run 原生资源及最终停止证据 |

PR 平台是实际 PR/合并事实来源；Runner 的 provider 核验是供调用方消费的事实。自然语言成功、审计通过、轮次结束、pr_ready、服务退出均不等于 TaskDelivery。

SteerLine 数据库存任务、候选、原 outbox、公开投影及交付；Runner 数据库存执行和原生资源。公开状态副本不是第二执行权威。SteerLine 不导入 Runner 内部 Go 包，不读其 SQLite、codec、原生日志或私有认证文件，不解释模型正文来驱动流程。

身份链为 `Task ID → Candidate ID → Runner Run ID → Attempt / Session / Turn`。保留现有接口中历史命名的字段，但注明实际映射：当前 `WorkflowRunID`/候选视图 `RunID` 中的 caller 值不能当作 Runner Run ID；后者取显式 execution/run binding。此次不批量重命名公共身份或添加另一套 ID。

## 3. 执行配置的四种数据

| 名称 | 内容 / 所有者 | 是否可用于重建已受理运行 |
| --- | --- | --- |
| Runner 部署配置 | SteerLine home 内固定 EXE 路径、version、SHA；项目派生 state-dir；本次 ProjectRuntime 初始化时绑定已核验服务实例 | 不得替换原 Run 的构建/目录绑定；新实例只在显式重启 SteerLine、重建 ProjectRuntime 时按 4.2 接纳 |
| 执行预设（execution profile） | SteerLine 保存 ID/name、execution、workflow；默认项为目录选择 | 只用于新候选，不能覆盖旧输入 |
| 任务提交快照 | SteerLine 保存候选预设 ID/名称、参数摘要及完整原 CLI 信封/RunInput；含任务材料、仓库/base、发布目标和 correlation | 原请求对账的唯一正文来源 |
| 已受理运行快照 | Runner 在受理时保存规范化执行/工作流参数、材料内容/摘要、流程配置及提示词模板/覆盖正文 | 原 Run 的固定配置来源；逐轮最终输入与后生成的原生身份另按 4.1 绑定 |

Runner 在同一 Run 记录内继续保存逐轮输入意图和动态执行事实：前者包含实际发送的完整 prompt、model/effort 和来源，后者包含准备后取得的 Attempt/Session/Turn 与资源状态。它们不是在受理时一次性冻结的全部内容；无需新增快照服务或另一套存储模型。

预设仍存 `<STEERLINE_HOME>/execution.json`；保留 `version: 3`、`runner`、`default_execution_profile`、`execution_profiles` 和项目配置边界。逻辑分责不要求拆文件、双目录、profile 同步服务或新增预设版本体系。配置新增的 workflow 字段必须经所选 Runner 契约验证；不接受旧格式回退。

预设由 `execution={adapter,target,settings}` 与 `workflow={id,version,prompt_overrides?,audit_model?}` 组成。工作区、Task、base、Spec/Plan、publication 不进入全局预设；在候选启动时绑定。项目决定仓库/发布目标，Runner 解释 publication 的公开 provider settings。认证只传公开引用，不把 token/私钥正文或用户认证复制到请求、材料或调试包。

默认值、可执行程序解析、权限编译、审计 effort 继承、流程提示词合并均属于 Runner。SteerLine 不补齐缺省模型/effort/权限来冒充 Runner 的生效值。

## 4. 冻结、摘要与配置变化

### 4.1 冻结时点

1. **宿主加载**：读取同一份全局和项目配置快照，固定 Runner EXE/资源身份与项目 state-dir；仅在 ProjectRuntime 初始化时核验并绑定本次服务实例，所有入口复用。本次 ProjectRuntime 存活期间不替换该绑定；后续接纳新实例必须显式重启 SteerLine、重建 ProjectRuntime，不将本次 instance_id 当作 Run 的永久配置。当前整体文件摘要仍用于配置匹配与准入，不能热加载部分字段。
2. **候选预留事务**：复核原配置、Task/图版本及材料契约，确定共同 base；固定预设 ID、名称、参数摘要和完整 CLI 信封，在领取/候选/outbox 同事务成功后才允许外部提交。后续目录改名、默认项变化不改变候选的展示名称或请求。持久化失败为零提交。
3. **Runner 受理**：Runner 按自身契约验证并解析执行环境，冻结材料、规范化执行/工作流参数、流程配置、提示词模板及覆盖正文，提交后返回 accepted。后续审计/修复/反馈的最终 prompt 和尚未取得的原生会话身份不在此时预填。接受前的已有 Git/准备行为沿 Runner 契约，不宣称纯字段校验等于现场预检。accepted 只证明受理，不能显示为已经执行或成功。
4. **每轮输入意图**：Runner 在各阶段结果/问题/反馈及授权确定后，用受理时的模板和固定配置生成本轮完整 prompt、model/effort、源报告/command/turn 关联，在实际发送前原子保存。审计/修复/模型回切沿既有 workflow，不由 SteerLine 生成。已保存但未发送的输入恢复时保留完整正文、模型及来源；不能重新渲染或套用当前默认值，Attempt/command 的必要关联更新仍沿原恢复规则。
5. **执行身份与后续恢复**：Attempt、Session、Turn 和资源 generation 在原 owner 取得事实时绑定、按已有生命周期更新；不能在受理时伪造，也不能被当前预设覆盖。reply/resume 固定原 Run、当前公开 revision 和授权；不能借输入修改冻结的 workflow、模型配置、材料或权限。重启恢复观察和原请求对账，零自动旧答案/反馈/resume；受理参数、逐轮意图及动态事实各按原用途恢复。

更换 workflow、模型配置、权限、模板/覆盖正文或材料必须新候选/Run；已有运行中按原流程选择实施/审计角色模型不是修改配置。既有 Run 仍使用自己的受理快照和已保存逐轮输入。

### 4.2 配置变化、重启与构建更换

同构建指 EXE SHA 与 resource 身份一致，不能只比 version 字符串。Run 的构建/目录/受理参数绑定保持；服务实例在单个 ProjectRuntime 生命周期内固定，但不永久固定为 Run 首次受理时的 instance_id。**本期仅在用户显式重启 SteerLine、销毁旧 ProjectRuntime 并重建新 ProjectRuntime 的初始化过程中接纳新 Runner 实例。** 单独重启 Runner、发现 service ready、刷新页面或轮询命中另一实例均不授权当前运行时重新绑定；不增加运行中实例切换入口。

| 情况 | 准入与既有运行 | 明确禁止 |
| --- | --- | --- |
| 同构建更换预设/默认项 | 文件改变后当前宿主拒绝新候选；普通 home 显式停止/重启并核验本次实例后，新候选才采用新目录。旧 Run 的观察/原请求对账/停止继续从持久绑定取值 | 用新预设覆盖旧 Run、重新拼旧提交；热加载部分配置 |
| 同构建服务重启，预设可保持不变 | 若旧 ProjectRuntime 仍存活，发现实例不同即拒绝新业务输入并提示显式重启 SteerLine，保留原绑定。用户显式重启后，新 ProjectRuntime 沿既有启动/写锁规则在同一 state-dir、同一构建核验本次实例及原 Run/input_digest 或原 submit request_id；准入、查询/控制客户端和停服/结算闭包在新运行时初始化时取得同一绑定。旧停服请求/回执仍指向旧实例，新停服动作使用新实例及独立原请求。崩溃后停止事实未核实的，新运行时只按既有规则恢复观察/对账，不能据 ready 宣称资源结算或自动续发业务 | 旧运行时自动接纳替代实例或只改部分绑定；用旧实例回执证明新实例已停止；因换代自动恢复业务输入、重置预算；为最终停止证明新增启动/恢复业务 |
| 更换 Runner 构建 | 先用原构建对账并停止原现场，只有所有原执行/检查资源及原请求结算可证明时，才允许选择新构建；本期新构建使用现有新环境流程产生的无旧执行状态 home/run-dir/state-dir | 有未结算对象时换构建；多个构建共享一个 state-dir；旧状态迁移/改绑或通过新构建“接管”原 Run |

未结算包括：pending/unknown submit 或控制请求，活动、等待回答/继续但尚未正式停止的 Run/检查，未完成停止义务，以及停止/清理事实缺失。已核实原停止、输入授权撤销且资源结算的历史 Run，不仅因保留 resume_required 等历史字段就算未结算；仍须核对原请求和检查的结算。Task 已 closed、PR 已合并或父进程退出均不豁免。原构建/目录不可用或身份对不上时保留未核实并阻断更换，不能用当前配置猜旧绑定。本期不实现跨构建旧状态恢复、自动路由或并行维护多套 Runner；旧目录保留原归属，不因换构建删改。

配置变动后的观察/停止由当前已核验项目运行时及其持久原绑定提供，不能要求先让新配置通过新候选准入；该规则不放宽实例匹配要求。运行中发现替代实例时，可读服务诊断和已保存事实，不能据此更新绑定、自动派发提交/回答/反馈/resume/检查输入，或把原停服请求/结算目标改指新实例。冷启动发现部署 pin 与未结算记录不一致时拒绝新构建，提示恢复原构建正式对账/停止；不另启替代服务。仅预设改变且构建相同的显式重启不重解释旧提交。调试包始终更严格：启动后冻结，换 EXE/方案须新建包，reset 不解冻。

实例检查必须作用于实际 CLI 请求：绑定客户端在 resident 业务/查询命令上传递 `--expected-instance-id`，Runner 在公开受理路径校验，返回 `stale_instance` 时不派发业务；绑定调用不隐式启动服务。`capabilities.instance_binding.expected_instance_id: true` 是接入前置，缺失明确拒绝。`service status` 仍供诊断，`service stop` 继续使用原正文的 expected_instance_id，不改写已冻结的业务信封。

显式重建时，在开放服务准入前只读核验原 Run/input_digest，未绑定 Run 的 unknown submit 查询原 request_id；不匹配、缺失或查询不可用时阻断准入。未发 pending submit/控制意图撤销；unknown submit/控制请求仅观察原 ID，查无原请求也不自动重发。

本次至少验证两条 caller 断言：①旧 ProjectRuntime 观察到替代实例，即使 EXE/资源/state-dir 相同，也拒绝新业务输入，原绑定及停服目标不变；②显式重启 SteerLine 后，新 ProjectRuntime 核验并接纳新实例，原 Run/input_digest 保持且零自动旧业务输入；重放旧实例的停止请求不能停止新实例，新运行时的停止/结算使用新实例及独立请求。断言归 RCB-A02 / Plan U1，不新增原生故障矩阵。

### 4.3 摘要职责

| 摘要 | 内容与用途 | 边界 |
| --- | --- | --- |
| 配置快照摘要（现有 ConfigDigest） | 项目文件 + 全局文件原始字节；宿主/客户端匹配与加载冻结 | 空白、名称、其他预设变化也可能不匹配，不表示执行语义变化 |
| 预设参数摘要（沿用 profile digest 接口） | 仅 `execution` + `workflow` 的确定性 JSON，含显式覆盖/模型/权限 | 排除 ID/name/默认项；绑定时同时核对 profile ID，不替代配置准入 |
| 原请求身份/摘要 | 原 request_id 与完整不可变信封；重复请求相等性和对账 | 标题/correlation 等也属于原正文，不能重新拼装 |
| Runner input_digest | Runner 返回的规范化输入摘要，首次绑定后不可变 | 与 profile digest/原请求摘要不作相等比较 |
| Runner resource_digest / EXE SHA | 固定流程提示词/schema 资源与实际程序身份 | 不等同 workflow version；各自保存和核对 |
| Task contract digest | 固定材料与业务契约的来源约束 | 不替代上述任一执行身份 |

预设参数摘要采用严格 JSON 值解析后确定性对象键排序、数组顺序保留、无多余空白的 SHA-256；拒绝重复键/无效输入，缺省与显式填写保持区别，不通过 Runner 默认值进行归一化。不要求引入通用跨语言 canonicalization 框架。

名称改变不改变参数摘要，但仍触发整体配置冻结；模型/流程/权限/提示词变化必须改变参数摘要。该区别不赋予运行时编辑或跨构建旧数据兼容能力。持久格式需调整时按当前格式拒绝不支持数据，不迁移、双写或补造历史名称。

## 5. 多 Workflow 发现、校验与选择

### 5.1 单一契约来源

`runnercontract` 是调用方持有的公开 JSON 投影，不是 Runner 内部模型或第二份协议定义。Runner 发布 self-contained schema 和 workflows 目录；正反样例归其测试资产，不增加运行时样例字段或样例加载协议。SteerLine 只保留实际消费的类型，以真实 EXE 的配置契约验证和真实 Run 的产品查询分别验证两类接口。

消费 `capabilities.workflows` 与 `configuration_schemas.workflow`，按 ID/version 查找，校验允许覆盖的 prompt_steps、supported_adapters、audit_model_required 及已发布 schema；沿现有 adapter/provider schema 验证 settings。校验还须检查 Runner 已声明的目标/模型组合要求；发布信息不足以表达组合时，先在 Runner 补齐同一公开 schema/目录，再接入调用方，不在 SteerLine 新建审计专用规则表。

本轮最低接入契约须能验证完整 `execution + workflow` 组合（含 audited 仅 local、显式执行 model）；采用 capabilities.configuration_schemas 中新增的自包含 `run_configuration` schema，输入为 `{execution,workflow}`，与现有 workflow/adapter schema 同源。Runner 的最终受理检查仍是权威；其仓库测试用正反样例核对 schema 与生产纯配置准入一致，不把样例作为 capabilities 的生产契约。

跨仓按两个门槛交接：开发前置是固定目录/schema/公开字段约定，SteerLine 可用该约定的公开 fixture 开发配置、冻结和投影/Web；跨仓联验与功能交付前置才是 Runner 已实现的契约、实际 EXE/build.json 和同构建定向证据。前置未到不阻塞无关 caller 实现，但不能启用不满足契约的真实 Runner 或宣布真实集成通过。交接内容和测试范围见 Plan，不以强制顺序等待代替版本固定。

缺少多流程目录、所需 schema/能力、唯一 ID/version 或匹配资源摘要时明确配置/能力错误，不退回单 `workflow` 字段、不改投普通流程。旧 EXE 可继续由旧调用方使用，不在本次增加兼容层；新接入须选择满足契约的固定构建。CLI 仍用 `steerrunner-cli/v1` 和现有 submit，不新增加载外部 workflow 的接口。

配置文件 `version: 3`、CLI `steerrunner-cli/v1`、workflow 的 `id/version`、Runner EXE version/SHA、resource version/digest、双方各自的存储格式是独立版本轴。项目保留的 `worker-flow-v2` 是既有 Task 契约字段，不代表调用方拥有执行流程；公开候选的 workflow 版本必须取冻结 workflow.version，不能填 CLI protocol，协议另用现有 protocol 字段展示。

### 5.2 首批接入范围

| 流程 | 预设输入 | 调用方行为 |
| --- | --- | --- |
| `development/1` | 现有 execution；不带 audit_model | 原普通流程、问题和交付行为保持 |
| `development-audited/1` | local + codex-app-server；显式 execution.settings.model；workflow.audit_model.model 必填，reasoning_effort 可省略 | 原样提交；不注入修复、审计或切模型命令 |

两种模型可以相同；生产不强制不同。双模型验收用不同型号以证明传递。审计 effort 省略时由 Runner 继承规范化执行 effort。提示词覆盖只限目录允许步骤，不能删除固定材料/结果契约；更改覆盖须新 Run。

Runner 管理两项审计顺序、findings 裁决、修复上限和模型回切。本期值以固定流程契约为准，SteerLine 不新增自动修复预算，不通过 audit pass 自行派发 publish。预算耗尽、坏 JSON、模型不可用、缺原 final、未知发布继续由 Runner 公开状态和允许动作表达；调用方不改为成功或自动新建候选。

提供一个普通和一个审计预设示例；示例模型/路径/SHA 明确为需替换的值，不改变用户默认方案。首期按现有文件配置预设、Web 选择完整预设，不新增 workflow 编辑器、任意步骤编排或模型目录服务。

审计预设的相关片段如下（不是完整 execution.json，模型标识需替换为账号可用值）：

```json
{
  "id": "local-audited",
  "name": "本机开发与审计",
  "execution": {
    "adapter": "codex-app-server",
    "target": { "kind": "local" },
    "settings": {
      "model": "execution-model-id",
      "reasoning_effort": "medium",
      "access_mode": "sandbox"
    }
  },
  "workflow": {
    "id": "development-audited",
    "version": 1,
    "audit_model": { "model": "audit-model-id", "reasoning_effort": "high" }
  }
}
```

普通预设使用 `workflow: {"id":"development","version":1}`，不带 audit_model；部署 pin 仍在顶层 runner，仓库/publication 仍在项目配置，本次任务和材料仅在完整提交中出现。

## 6. 查询、展示与环境检查

Run 公开投影增加实际消费的 `workflow`、`audit`、`materials` 和 `execution.requested_model/requested_reasoning_effort`，保持 Runner 字段原义；audit 仅展示当前有效报告来源的结果、findings、failure 和 repair_count，不自行拼接历史 pass。透传 source_report_command_id、command_id、turn_id，避免兄弟 Run/旧报告污染。公开 revision 单调、原 input_digest/Run/PR 绑定规则保持。

查询接入的真实证据必须在已有 N1 Run 上经 SteerLine 正式产品查询取得：详情验证 workflow/audit/materials/轮次请求模型及来源，历史验证所声明的 workflow/预设摘要与同一候选/Runner Run 映射。历史不要求复制完整材料或全部审计正文。空闲服务的 capabilities 只能证明配置发现/校验，不能证明这些运行字段接通；原始 Runner inspect 仅作对照，不能代替产品查询断言。

Web 方案列表显示预设名称、workflow ID/version、请求执行模型、请求审计模型和权限模式；省略参数标记“由 Runner 决定”。候选详情/历史读取固定候选名称与原提交，显示 Runner 公开的当前阶段、审计摘要和轮次请求模型；“请求模型”不宣称原生实际使用型号。无 audit 不造 pass，无新投影保留最后观察及未核实语义。任务/候选参与、执行、交付、资源结算分开显示，原按钮资格仍来自原控制入口。

两个快照的比对视图只需利用原提交和已有公开 inspect 字段；不要求 Runner 返回完整私有运行快照或新增查询命令。缺失的规范化字段显示未提供，不从本机路径或当前预设猜测。历史不新增完整审计轮次查询。

环境检查只消费 execution、repository 和可选 publication，不接收 workflow/audit_model、不生成 Task/Candidate。显式选择 access_mode 时先深拷贝预设，固定该次检查请求，再由 Runner 校验/规范化；不修改共享预设，不把检查结果当普通业务启动就绪证明。业务和检查默认值含义同归 Runner；模式不同须显示实际所测模式，不能借历史报告扩大结论。本次保留现有检查自定义 execution 能力，不建设第二份检查预设库。

## 7. 生命周期规则与非目标

统一节点、异常和验证规则归 [worker-lifecycle](../product-design/worker-lifecycle.md)；本次对应差异已登记其“协作边界与多 Workflow 接入规划”。W01 补齐预设/契约冻结，W08–W14 消费新公开字段和原请求，W15–W16 只展示 Runner 的审计/发布，W17 仍由 SteerLine 唯一交付，W18 保留原实例停止取证。E01–E07 的分类、重试预算、未知零盲发、恢复授权均不改变，不新增 W/E 编号或另一套状态机。

本次不做：原生权限/认证增强、Orca 新支持、任意 workflow 引擎、热加载、共享数据库/内部 SDK、配置迁移、自动选优/融合、批量字段改名、安装部署。审计原生专项属于 Runner；调用方只增加其请求/投影/业务闭环覆盖。

## 8. 验收要求

具体步骤、待验状态和证据归[专属验收](../acceptance/runner-collaboration.md)，Plan 映射全部自动项。

| ID | 要求 |
| --- | --- |
| RCB-A01 | 配置/请求/执行/交付所有权清楚；现有运行文档修正 owner、版本轴、短目录和身份映射；无第二配置来源或内部依赖 |
| RCB-A02 | 冻结职责和三类变化规则清楚；旧 ProjectRuntime 拒绝替代实例，仅显式重启 SteerLine 后重建的运行时核验接纳；旧停服请求不作用新实例，原 Run/输入保持且零自动重放；未结算阻断换构建，持久失败零提交 |
| RCB-A03 | 固定真实 EXE 发布并验证两流程和组合 schema；正反样例留测试资产；caller 保留 audit_model、拒绝非法/缺能力配置且无降级 |
| RCB-A04 | 详情/历史/Web 来源一致，原请求和公开观察分开，审计来源/模型/未知/参与/交付及控制资格不混用 |
| RCB-A05 | 检查与业务输入隔离、预设不被修改；未知/重启/停止、迟到接受和唯一交付现有 caller 边界保持 |
| RCB-A06 | 固定两仓构建经 SteerLine 选择审计预设、原会话执行/审计/发布、真实 PR 合并、唯一交付和后继释放；同一 Run 经正式详情/历史验证新增公开字段，不以 Runner 原始查询或单仓证据代替 |
| RCB-M01 | 可后置人工确认方案选择、请求/观察区分与审计失败提示易读，不阻塞无关实现 |
