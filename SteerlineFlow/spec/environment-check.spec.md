# Spec：执行权限与 Git 环境检查接入

日期：2026-10-16。状态：SteerLine caller/HTTP/Web 对齐 Runner 已合入 check/v1 契约，固定 EXE 集成结果见验收。[Plan](../plans/environment-check.plan.md) · [本仓专属验收与审查记录](../testing/acceptance/environment-check.md)。普通开发，无 Task ID，不改 roadmap/execution-map。

## 1. 目标与规格所有权

SteerLine 提供主动环境检查入口：选择项目、目标 Worker 和权限模式，调用独立 Runner，让实际 Agent 验证执行与 Git/必需 PR 能力，展示报告和待处理资源。构建、测试及业务资源检查后置，后续由项目定制用例负责；资源/模型目录和执行方案组合管理不在本期。

Runner 契约与原生验收归属 `C:/Project/SteerRunner`。本轮固定其已合入提交 `ea1bfd982c63818de02870c70fa2f7b1c9f258b2`（PR #14）；[固定 Runner Spec](https://github.com/erranliu/SteerRunner/blob/ea1bfd982c63818de02870c70fa2f7b1c9f258b2/docs/spec/environment-check.spec.md)、[Plan](https://github.com/erranliu/SteerRunner/blob/ea1bfd982c63818de02870c70fa2f7b1c9f258b2/docs/spec/environment-check.plan.md)及其 acceptance 是前置来源。固定 EXE/version/SHA/resource_digest 记入本仓验收；不使用浮动 main 或本地草稿代表交接。

| 所有者 | 唯一维护内容 |
| --- | --- |
| SteerRunner | access_mode 语义/原生映射/认证准备；检查 CLI 与 schema；检查执行、原生工具与权限取证、分项结论、停止/清理；支持组合与原生专项验收 |
| SteerLine | 项目与公开输入装配；用户授权/原请求/检查绑定；HTTP/Web；历史索引、公开快照缓存与当前选择适用性提示；Task/DAG/交付仍由原用例负责 |

SteerLine 不固定 MXC 参数、签发/注入凭据、解析 rollout/终端、判定原生权限通过或指挥 Runner 内部节点。Runner 不依赖 SteerLine Task/项目状态来运行检查，project ID 仅为不透明关联。两仓各自拥有 Spec/Plan/acceptance；本仓不再复制 Runner 状态/策略/原生测试矩阵。

调查读取 `docs/spec/SpecGuide.md`（最后修改 `94e704300485b82d54e44991148074841ed66019`），SteerLine HEAD `6f40f2dd70c2226c38e1599f81546652f475d563`；本次编辑前有上一轮未提交文档，未把它们当作已合入实现。已读[生命周期](../../docs/product-design/worker-lifecycle.md)、[执行配置](../../docs/product-design/execution-profiles.md)、[测试入口](../../tests/e2e/README.md)。代码调查：`runner_config.go`/`runnerclient/catalog.go` 提供公开方案与 schema；`runnerclient/client.go` 只有现有 CLI 白名单；`runnerbridge` 绑定 Candidate，不能伪造候选复用；HTTP 项目认证/请求注册、SQLite 与 Query 可复用。

## 2. 与沙箱认证工作的边界

Runner 的在途沙箱认证在 `8876/SteerRunner` 工作树（读取 HEAD `e04d07fde31710b5b93d3186010074927a44effc`）维护 SA01–SA08，不在本次修改范围。Runner 新 Spec 提供逐项交接表和最终整合前置：

- SA02 的同权限确定性预检保持业务准备门槛，不调用模型；本功能的 Agent 实测是用户主动触发的独立活动，不插入每次业务启动。
- SA04/SA08 的零审批/零 bypass 作为 sandbox 分支契约；显式 full_access 独立，不充作沙箱通过或沙箱失败回退。最终 SA 规格仍若全局禁止 full_access，Runner 双模式集成继续阻断，不能由 SteerLine 覆盖。
- SA01/SA03 的认证来源和生命周期归 Runner；若沙箱目前只支持 GitHub App，缺引用明确不可用，SteerLine 不能退回宿主 gh/OAuth。不因环境检查新增认证类型。
- SA07 远端执行主机认证、同权限确定性预检与真实工具证据继续是前置；Agent 自述或 full_access 均不能绕过。Orca 缺原生能力时显示 unsupported，不由 SteerLine 建第二条远端执行路径。

具体时点以 Runner Spec §2.1 为准：设计/fixture/Controlled 可先行；真实发布/集成须复用的 SA 最终提交合入并满足其自身交付门槛。当前 SA 验收中的有效凭据正例及其要求的 L2 smoke 尚未通过，不能以本功能受控通过、原生无效 token 负例替代。full_access 复用认证 owner，其同模式确定性预检由 Runner 本功能补齐，不要求 SA 反向依赖本功能，亦不能直接套用 MXC 证据。

## 3. 用户流程与检查上下文

设置提供一个“环境检查”入口，项目页可携项目进入同一页面。必须选定已纳管项目，但项目纳管不以检查通过为前置。

选择已有执行方案可预填，也可填写本次所需的公开 execution 参数；页面只提供目标/Agent/必要模型参数/权限模式，不建设通用 JSON schema 表单编辑器，不保存第二套执行方案。权限模式使用 Runner 发布的 `sandbox` / `full_access`；缺省 sandbox，完全访问须明确选定，缺支持时显示原因。

开始前展示项目/仓库、目标、Agent、模式含义、模型消耗及本次专属 Git 分支/测试 PR 的创建与关闭范围；用户点击开始是对该清单的执行授权，不逐步追加 SteerLine 审批。检查不修改全局默认，不改变现有业务执行权限。后续普通执行只有使用同一公开模式，才与检查具有可比上下文；原执行反馈/恢复保持冻结值。

报告逐项显示实际执行身份、检查时间、通过/失败/未核实/不适用、原因与脱敏证据。读取/fetch 成功不冒充 push/PR 权限，历史能力不冒充当前新工作区核验。能力结果与清理分别展示，存在残留时显示“资源待处理”；始终注明“未检查构建、测试和业务资源”。结论由 Runner 返回，SteerLine 不根据文字或部分成功重算通过。

## 4. 公开接口与最小持久化

消费 Runner 已发布的 `environment-check start/inspect/stop` 三命令，不维护自己的原生命令或复制其错误分类。start/stop 是写请求；inspect 按 check_id 或原 request_id 返回有界快照/revision。首期通过现有 Query 可见轮询刷新，不增加检查专用 events/cursor/SSE。

命令参数、CheckReceipt、Inspection、错误码的唯一规范见 Runner Spec §4/§4.1：start/stop 使用 v1 `--input -`，inspect 恰选一个 `--check-id` 或 `--request-id`。读取原 stop 请求必须校验原停止命令身份，不能把同 Check 的 start 回执当作停止结果；明确拒绝且没有 Check 的原请求也要展示。写响应回显外层 request_id，查询外层为 null。受理后环境检查 failed/unverified 是成功获取的报告，不是 inspect 传输失败；缺现场凭据与缺原生实现能力分别呈现。

装配完整 Repository/ExecutionInput 和项目实际 Publication；当前项目配置了发布要求就必须传入，不得因缺权限省略。Runner 根据明确输入固定必需项，不读取 SteerLine workflow；如果独立调用的报告未包含 PR，不能在项目页面解释为发布就绪。真实业务 base 仅作来源核对，检查 PR 只针对 Runner 分配的本次检查 base/head。

HTTP 使用现有项目命令信封提供 start/stop，提供项目范围 list/detail 查询。list 为本仓已提交的检查关联索引，不要求 Runner 新增全局 list；detail 最佳努力查询 Runner，源不可用时显示带源时间/错误的缓存，不能把旧缓存标成新检查成功。

本仓仅持久化项目绑定、不可变公开输入/摘要、原 start/stop 请求、Runner check_id 与固定 EXE 身份、最近公开 revision/报告。Runner 单写执行事实；本仓快照仅用于历史/离线展示，不建立实时检查状态机或重试调度器。目标/Agent/模式等与当前选择不一致时提示“结果不适用于当前选择”；外部认证可能变化，展示源时间，不承诺持续有效。

Runner 的 input_digest 是首个已核实 Check 返回的权威值，本仓保存并校验后续不变，不用自己的 JSON 序列化/缺省展开重算。校验请求上下文与本次授权相符，区分 Check 输入与 stop 请求输入。低 revision 不能覆盖高 revision；同 revision 内容冲突、Check 身份/摘要/冻结必需项清单漂移或缺必需报告字段时保留旧可信快照并显示未核实，不重算通过。缓存保留 Runner 源时间，轮询时间不是源更新时间。

写请求出站前保存原 ID/输入；重复点击复用原请求，不同输入同 ID 拒绝。未知结果只查询原 request_id；即使查询返回 command_not_found/request_not_found，浏览器也不进入通用补交 POST 路径。重启不自动重发 start/stop，不创建替代检查。接受晚到或本地绑定提交失败仍按原请求找回，并只绑定原项目/检查。Runner 不可用时保留原未知事实，不切换 EXE/方案重发。

取消与资源待处理后的显式收尾都使用 stop 的新授权/当前 revision，由 Runner 判断允许动作。原 start 结果未知、尚无可信 check_id/revision 时仅查询原请求，不发送空目标/猜测身份的 stop；取得绑定后用户再明确提交。关闭页面只停止轮询，不取消。再次检查建立新检查；原写者/未知操作未结算时阻断，首期不做失败项拼接重验。

查询/进入页面不启动服务或 Agent。项目 runtime 仍沿既有启动路径拥有常驻 Runner；检查 start/stop 只复用其原实例/配置准入校验，不另启或替换常驻服务。原实例离线或变化时明确拒绝新检查写请求；原请求与历史读取不要求新输入准入。缺检查 capability 只阻止该检查，不能让设置/历史页不可读。普通执行仍走自己的必要预检，不新增“必须有检查报告才能启动 Task”的门槛。

## 5. 支持范围、依赖与验收

本期设计目标仍为本机 Codex 与 Orca 远端 Codex 的 sandbox/full_access；按 Runner 实际声明支持的组合启用。Runner 本机能力可以先发布并接入；远端缺 SA07/原生接口时保留目标及待前置状态，不能宣称远端可用，也不阻塞已支持组合的 UI/caller 交付。远端后续声明支持必须有其自身真实验证。

capabilities 表示已实现且声明支持的机制，不表示某机器当前登录/资源就绪。原生机制缺失为 unsupported；支持组合当前缺凭据、401/权限拒绝或超时进入本次报告，不通过隐藏组合或标 not_applicable 掩盖失败。集成使用 Runner Controlled 校验过的同版公开请求/响应样例构造 fixture；本仓产品通过仍须固定真实 Runner 契约核对，fixture 不能替代其原生/SA 验收。

Runner 实现的内部准备、模型轮次数、预算、Git/PR 对象守卫和资源收尾仅引用 Runner Spec，不在本仓规定。共用节点/异常语义见[生命周期](../../docs/product-design/worker-lifecycle.md)，仅保持 W03/W07/W18 和 E/V 的语义衔接。

本仓验收 SLEC-A01–A04、人工 SLEC-M01 见[专属记录](../testing/acceptance/environment-check.md)；Runner 前置另验 SREC-A01–A06，SA 工作仍验自己的 SA01–SA08，互不共用通过状态。本轮按用户授权实现 SteerLine；未实现 Runner、未启动真实 Agent 或操作外部 Git/PR。调用方自动结果与仍待前置的真实契约核对分别登记，不据夹具宣称远端或原生权限通过。


## 6. SteerLine 实现绑定（2026-10-16）

- `internal/environmentcheck` 只负责原请求、绑定及公开报告缓存；纯记录由 `internal/runnerstate` 定义。`internal/app/environment_checks.go` 注入固定 EXE 和同项目 SQLite，复用已有 TaskRepositoryTarget 的静态仓库/发布目标/非秘密 URL 校验，装配实际 Publication 与本地已存在的目标 base commit。此复用不包含 Task 领取或 DAG 资格。读取本地 Git 元数据不证明远端权限；未取得来源 commit 时明确拒绝，不偷偷 fetch 或换成 HEAD。
- `product/api` 的公开投影保留 Runner 文档为 JSON，不依赖 Runner 进程客户端；应用装配处完成投影。HTTP 继续使用既有项目认证、请求 ID 仲裁及 readiness，环境检查不进入 Task/control 用例。
- 消费 `capabilities.environment_check.contract=steerrunner-environment-check/v1` 和 `targets(adapter/target/access_modes/mechanism/platform)`，只允许 implemented 组合；schema 从同一 capabilities 的 `environment_check_schema` 读取并冻结，使用其 `$defs/input|receipt|inspection`，不远程加载引用。报告原样消费 `input/actual/result/cleanup_status/items[].required/allowed_actions`，资源完成以 `cleanup_status=completed` 为准，stop 权限以 `allowed_actions` 为准。资源摘要仍由 Runner 提供。Check DTO 见[公开值](../../internal/runnercontract/environment_check.go)；[公开同版样例](../../internal/testkit/checkfixture/published-v1.json)直接冻结 Runner 内嵌发布数据，[caller fixture](../../internal/testkit/checkfixture/v1.json)只派生测试上下文与失败报告，不是原生资格证据。删除旧草稿字段，不新增兼容回退。
- `GET /environment-checks/options` 提供预填与能力；`GET /environment-checks` 只读本仓历史；`GET /environment-checks/{key}` 在有未知操作时只 inspect 该原请求，已取得绑定后按 check_id 读快照，离线保留源时间与诊断。key 是原 start 的 32 位 request_id，与原生 check_id 分开。无额外 events、cleanup 或启动型 GET。
- `POST /commands` 的 `start_environment_check` target 为本项目 project_id、expected_version=0，payload 为 `{profile_id,access_mode}` 或 `{execution,access_mode}`；stop_environment_check target 为本地 key，expected_version 为显示时的 Runner report.revision、payload 为 `{}`。两者不带 task/control/graph revision。command receipt 的 applied 只表示原写请求已确认受理，不代表 Agent 已执行或能力通过。
- SQLite schema 22 新增 environment_checks 与 environment_check_requests，保存固定身份、原请求与缓存；CAS 不改变 Task/DAG/database_revision。schema 21 直接拒绝，不迁移。调试 reset 在检查资源未结算或任何原操作未知时保留恢复记录；已证明安全时按请求关联→检查记录顺序清除本包状态。
- 页面位于 `/projects/{project_id}/environment-checks`，从执行设置进入。按可见页面轮询选中报告；选择变化提示适用性，权限模式默认沙箱且完全访问须显式选。保存原授权依赖既有浏览器 journal/目标锁和服务端不可变请求，页面关闭不 stop。

本次不修改执行方案默认值或 debug-template 的冻结执行配置：本次检查参数只属于独立请求。使用新调试包的同 SHA Runner 核对公开 schema/EXE；人工体验及原生组合适用范围按 acceptance 独立记录。
