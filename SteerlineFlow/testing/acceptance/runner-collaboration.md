# SteerLine / SteerRunner 协作与 Workflow 接入验收

日期：2026-10-17。归属：本次普通开发“协作边界与多 Workflow 接入”，只覆盖本任务。关联 [Spec](../../spec/runner-collaboration.spec.md) / [Plan](../../plans/runner-collaboration.plan.md)。两仓已实施并完成本任务自动验收；固定构建、失败重验及清理记录如下。人工 RCB-M01 后置，不继承 Runner 历史审计通过结论。

## 自动验收

### RCB-A01

- 状态：通过，2026-10-17；证据和范围见下方实施登记。
- 行为：按 Spec 所有权表核对 Task/候选/配置/运行/交付 owner、公开 JSON 依赖与状态目录；修正正式文档的旧 owner、版本轴和身份映射，无第二配置来源或内部数据库读取。
- 验证：Plan D0；逐项引用/文档清单与链接、格式检查。设计检查不能替代实际模块边界检查。

### RCB-A02

- 状态：通过，2026-10-17；证据和范围见下方实施登记。
- 行为：名称/键顺序不变参数摘要，模型/权限/workflow/覆盖变化改变摘要；整体配置字节变化仍阻断当前宿主新候选。候选预留冻结名称/base/材料引用/完整信封；Runner 受理冻结规范化参数、材料及模板，后续各轮完整 prompt/model/来源在发送前保存；原生身份按原生命周期绑定。已存输入恢复不重渲染，同 ID 改正文冲突，事务失败零提交。
- 三类变化：同构建换预设只让显式重启后的新候选使用新值；新 Runner 实例仅在用户显式重启 SteerLine、重建 ProjectRuntime 的初始化时接纳，原 Run/input_digest 保持、旧回执归属不变，零自动旧业务输入。旧运行时发现替代实例拒绝新业务输入、不重新绑定。换构建前原执行/检查/请求全部结算，pending/unknown/活动对象、未核实停止或原构建不可用均阻断，新构建只能用无旧执行状态的新环境。
- U1 必需断言一（subtest `reject_replacement_in_live_runtime`）：保留绑定 A 的旧 ProjectRuntime，仅让公开 CLI fixture 变为同构建/目录的 B；旧运行时拒绝新候选、回答/反馈/resume/检查等输入，准入、客户端及停服/结算绑定仍为 A，业务派发计数不增加，并提示显式重启 SteerLine。
- U1 必需断言二（subtest `accept_replacement_after_explicit_restart`）：结束旧运行时，通过显式重启 SteerLine 的宿主路径重建 ProjectRuntime，核验并接纳 B，准入/查询控制/停服结算一致绑定 B；原 Run/input_digest 和已存输入保持、零自动旧业务输入。重放 A 的停止请求不能停止 B，也不能作为 B 的结算证明；B 使用独立新停止请求及原 B 实例最终证明。
- 验证：Plan U1/U3；U1 两个 subtest 均需命中，记录原/新运行时与实例、业务 CLI 派发计数、A/B 原停止请求及结算目标。R2 回链 Runner 原逐轮冻结恢复测试，不在 caller 重建执行状态机。配置变化不阻断沿原绑定的观察/对账/停止，但不能据此放宽实例匹配或接纳替代实例。

### RCB-A03

- 状态：通过，2026-10-17；证据和范围见下方实施登记。
- 行为：实际固定 EXE 发布两流程与组合 schema，Runner 仓库内正反样例核对 schema 与生产纯配置准入同判；caller 选择普通/审计并保留 audit_model/省略 effort，不支持组合、缺能力、未知流程、资源错配明确拒绝且无降级。不新增运行时样例字段、传输格式或加载器。
- 验证：Plan R1（Runner 专属证据回链）+ U1 + I1；U1 可用已固定契约 fixture 先开发，I1 必须通过实际 EXE 的 capabilities/schema 和本仓配置用例验证，不以手写 capabilities 代替真实配置契约。I1 不产生业务 Run，不证明 audit/materials/轮次模型读取接通。

### RCB-A04

- 状态：通过，2026-10-17；证据和范围见下方实施登记。
- 行为：详情/历史/Web 使用固定名称/请求和当前公开观察；展示 workflow、请求模型、审计来源/findings/failure/repair_count。无审计、旧报告、未知、兄弟 Run 不显示伪 pass，不从结果自行推导控制资格或 Task 成功。
- 验证：Plan U2/W1 核对转换链及 mounted 页面；真实字段接入必须复用同一 N1，分别记录 SteerLine 正式详情查询的 workflow/audit/materials/各轮 requested model 与来源，以及正式历史查询的 workflow/预设摘要和候选/Runner Run 绑定。Runner inspect 仅为对照，不能绕过产品查询；历史不必新增完整材料/审计正文，无额外真实 Run。

### RCB-A05

- 状态：通过，2026-10-17；证据和范围见下方实施登记。
- 行为：检查改权限模式不污染业务预设，不传 workflow/audit_model；现有原请求/未知/revision/重启零自动输入、迟到接受停止和唯一交付守卫保持。
- 验证：Plan U2/U3/W1/C1；controlled 明确 gate-only，不能当作原生流程或完整 L2 通过。

### RCB-A06

- 状态：通过，2026-10-17；证据和范围见下方实施登记。
- 行为：SteerLine 正式入口选择审计预设，两种不同请求模型沿 Runner 原会话实施/审计/切回发布；真实 SteerlineTest 专属 PR 核验并合并，唯一 TaskDelivery、后继 ready 未启动；资源停止/清理按原身份核实。
- 验证：Plan N1，新增 L3 场景 `workflow.local-codex-audited`；记录原请求/Run/会话/轮次请求模型、公开材料/审计来源、PR/merge、Task 事务和正式产品详情/历史响应及 revision。与 RCB-A04 共用该次运行，不增加第二条模型链；无需强迫模型产生 finding，不重复 Runner 内部故障矩阵。
- 前置：A 实际构建/证据交接且 I1/C1 通过；所选 Runner 实际权限/登录就绪、驱动可运行、两种模型和真实 PR 权限可用。开发可先按固定公开契约进行，只有联验/交付受此前置约束；缺任一项不转模拟通过。

## 人工验收

### RCB-M01

- 状态：待定，尚未预约参与窗口。
- 参与：用户；新生成 simulate 包，Web 集成完成后方便时检查。
- 步骤：选择普通/审计预设，查看请求参数和运行观察的区别，检查普通无审计、审计通过/失败/未核实三类呈现及反馈/恢复提示。
- 通过条件：术语与数据来源易懂，能辨别 Task 交付和 Runner 执行；界面不声称实际原生型号或缺证据的审计通过。
- 阻塞范围：可后置，不阻塞无关实现与自动验收；发现真实身份/授权缺陷按正常缺陷处理。

## 证据登记

原始证据放每次新的临时目录，长期只提交最小摘要及必要精选事实。实现后逐组记录：日期、两仓 commit/dirty、固定 EXE version/SHA/CLI/resource_digest、配置摘要、测试选择与实际命中数、命令/退出码、日志路径、原 Run/request/PR/Task 和清理状态。失败、补测、旧构建分别登记；模型秘密与认证正文不落仓。

| 组 | 实际结果 | 证据与范围 |
| --- | --- | --- |
| D0 首版规划文档检查 | 2026-10-17 通过，仅首版内容 | 466 份 Markdown、2845 个本地导航链接、0 错误；diff/围栏/JSON/验收映射通过。不能代替本轮修订检查；主聊天自查，不计功能实施完成 |
| D0 四项审查修订检查 | 2026-10-17 通过，仅文档 | `python scripts/check_doc_links.py`：466 份 Markdown、2853 个本地导航链接、0 错误；`git diff --check` 通过；8 份相关文档空白/围栏、1 个 JSON 示例、RCB-A01–A06 / M01 映射和四项修订结构核对通过。R2 既有测试入口已只读确认但未运行，git 变更仅 Markdown。证据见本次聊天工具输出；主聊天自查，不计功能项通过 |
| D0 最后实例准入 P2 修订检查 | 2026-10-17 通过，仅文档 | 466 份 Markdown、2853 个本地导航链接、0 错误；`git diff --check` 通过。8 份相关文档空白/围栏、1 个 JSON 示例、RCB-A01–A06 / M01 映射和 U1 两个计划 subtest 存在性核对通过，旧的模糊换代措辞已移除。仅改预期 Markdown；本次聊天工具输出为证据，主聊天自查，不计测试代码实现、测试执行或功能通过 |
| R1 / R2 / U1 / I1 | 通过 | 固定构建、最新组与补充断言见下方实施登记；I1 仅空闲契约 |
| U2 / W1 / U3 / C1 | 通过 | 产品投影、检查隔离、Web 与 caller 回归；U3 一项其他功能的可选真实契约测试未启用，见下方范围 |
| N1 | 首次失败，修订夹具后重验通过 | 新审计真实链、正式详情/历史、同会话五个 final；PR #431 合并并完成清理 |
| RCB-M01 | 未安排 | 用户体验，可后置 |

## 交付状态

RCB-A01–A06 自动项通过，RCB-M01 留用户方便时验收。两仓代码、示例、Spec/Plan、验收和设计文档已同步；未提交/推送实现变更，未部署或修改安装环境。真实 PR 仅为 SteerlineTest 本次专属测试分支；失败现场清理后重建，不复用旧冻结方案。未修改 roadmap。

## 四项审查修订追踪

| 审查点 | 本轮文档处理 | 后续实现验证 |
| --- | --- | --- |
| 预设/服务实例/构建更换混用 | Spec 4.2 分三类，明确未结算定义、旧回执归属、新构建只用新环境 | RCB-A02：U1/U3，见实施登记 |
| 受理冻结包含未来数据 | Spec 3 / 4.1 区分受理参数/模板、逐轮发送意图和动态身份 | RCB-A02：Runner R2 + caller U1，见实施登记 |
| 空闲 I1 不能证明 Run 查询 | I1 只验配置，N1 同次经正式详情/历史读取真实字段 | RCB-A03/A04/A06：I1、U2/W1、N1，见实施登记 |
| 样例生产协议与过强跨仓前置 | 去掉生产样例集合；测试资产留仓；固定契约即可开发，实际构建只挡联验/交付 | RCB-A01/A03；D0 / R1 / I1，见实施登记 |
| 最后 P2：新实例接纳入口未限定 | 只在显式重启 SteerLine、重建 ProjectRuntime 时核验接纳；旧运行时拒绝替代实例且不自动/局部重绑 | RCB-A02：U1 两个计划 subtest 验证旧拒绝/新接纳与旧停止隔离，见实施登记 |

## 2026-10-17 实施与实跑登记

### 实现和来源

- SteerLine 基线 48c5671926466fe83c23de7fee5377bc61f87188，当前 worktree dirty=true；Runner 基线 529b6c360d9abb6ee225c82cc4393a9cb29d685e，dirty=true。两仓业务实现留本地待审阅，没有发布版本。
- Runner 本次 EXE：C:/Project/SteerRunner/.evidence/collaboration-1792198113555/steerrunner.exe，version=0.1.0-dev.1，steerrunner-cli/v1，Windows/amd64，SHA-256=0a89bc55f7a745a764d53484d1e74f17dc47897088f46a5aa713036d8e978393，resource_digest=414ab604b99fc1085045fd94f39045258000bebb7ba65619394cf9a29bc4f638。build.json 保存 source_commit/dirty/SHA；不沿用旧接入构建。
- Runner 专属增量验收：C:/Project/SteerRunner/docs/acceptance/run-configuration-publication.md，回链本 Spec/Plan。组合 schema 与生产纯配置准入使用同源 workflow 目录；生成 schema 纳入 resource_digest；生产不发布配置样例集合。
- SteerLine schema 23，格式 3 不变。候选同事务冻结 ID/名称/参数摘要/完整提交与构建；项目 checkpoint 固定构建/state-dir，instance 只属本次 ProjectRuntime。查询、控制、检查和停服使用同一初始化绑定，无运行中重绑。未知/原输入围栏沿既有 bridge；JSON 保留显式 null/空对象与省略的差别，原始配置拒绝重复键，published schema 负责准入。
- 冻结名称/请求与当前 Runner workflow/audit/materials/model 分开投影；workflow version 与 CLI/native protocol 分开。Web 只展示公开值，不判审计放行；检查重新解析独立 settings 后应用显式 access_mode，零 workflow/audit_model。

### 最终结果与证据

SteerLine 证据根：C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/rcb-1792198769033。下表子目录均归该根；完整命令、源码来源、真实匹配、退出码和详细日志在各 summary.json / go-test.jsonl 或 E2E raw/JSON 报告中。编译不计功能通过。

| 组 | 实跑结果 | 证据 / 范围 |
| --- | --- | --- |
| R1 | 14 个 schema/生产纯准入正反 subtest 通过，无 skip；无模型/Git freeze | Runner .evidence/collaboration-1792198113555/configuration-extra.log；初次 11 项见 configuration.log，新增 null/空 effort/override 反例；已有控制字符 regex 编译问题已修正 |
| R2 | 4 个逐轮冻结恢复 subtest 通过 | 同 Runner 目录 input-restoration.log：audit_answer、business_feedback、automatic_repair、report_retry；不重渲染输入/模型/来源 |
| U1 | U1-json 16 个测试/子测试通过，0 skip；最新 JSON 补充也通过 | U1-json/summary.json；U1-recorded/go-test.jsonl 记录两个必需换代断言的 A/B、真实 runtime 重建、原 Run/input_digest/提交/名称保持、零新业务、旧 stop 拒绝及 B 独立最终证明。json-presence/summary.json 核对显式空对象摘要；最新 client 契约追加 null overrides/unknown 字段也通过 |
| I1 | 实际 EXE 空闲发现/组合 schema/嵌套编码 6 个测试/子测试通过，0 skip，原实例停止与结算通过 | I1-json/summary.json；capabilities.json 保存实际公开契约。无模型/PR，不证明 Run 读取 |
| U2 | 6 个投影/检查隔离测试/子测试通过，0 skip | U2/summary.json；pass/failure/absent/unknown、冻结名称、版本轴、来源与独立 settings |
| W1 | 4 文件 25 测试通过；TypeScript/Vite 构建通过 | W1.log、web-build.log；mounted 组件验证冻结参数/当前模型、审计来源、失败/无结论、普通流程无审计；其他选择/控制资格回归保持 |
| U3 | 218 测试通过，无失败；1 个可选外部契约测试未启用 | U3-json/summary.json；skip 为 TestEnvironmentCheckPublishedContract，未提供该旧 SREC 独立前置，不计本任务通过。当前配置真实契约由 I1 覆盖，非环境检查 Agent/沙箱验收 |
| D0 owner | 依赖边界 3 个测试/子测试、466 份 Markdown / 2857 个本地链接和两仓 diff 格式检查通过 | boundary/summary.json；Windows/Linux 依赖检查，无退役执行 owner 或 Runner 内部包 |
| C1 | controlled smoke.delivery 通过，清理完成 | C1-final/summary.json；gate-only，非 full-L2；未改变普通业务交付语义 |
| N1 首次 | 失败，已清理；PR #430 关闭，专属引用清理 | N1/summary.json、scenario.json、runner-inspect-blocked.json。模型将实现/发布提前合并，审计报历史流程违规，修复无法倒改历史，最终 blocked；旧驱动等待至业务期限。未把失败记为通过 |
| N1 重验 | workflow.local-codex-audited / selected-L3 通过，清理完成 | N1-retry/summary.json、scenarios/workflow.local-codex-audited/scenario.json。实施/报告/两项审计/发布同一原会话，5 个 distinct canonical final；Spec/code audit 均 pass、repair_count=0；执行 Luna medium、审计 6.1-sol high、发布切回 Luna medium |
| N1 同次记录交叉核对 | 通过，0 新模型调用 | N1-recorded-cross-check.json、validate-recorded-n1.py：正式产品各阶段响应与原候选/CLI inspect 公开投影的 Run/input_digest、模型、材料来源/摘要、审计结果/源报告、历史关联和同会话 final；后补驱动身份断言也在该次记录核对，不新增真实 Run |

N1 的产品原响应为 audited-product-implement.execute.json、audit.spec.json、audit.code.json、publish.json、pr_ready.json 及 audited-product-task-history.json；原 caller/Runner 公开投影在 live-open.json / live-completed.json。成功清理已删除原 Runner state-dir，因此保留的是原 CLI inspect 公布值的投影，不能声称另存了完整原始 CLI 信封；驱动后续运行会额外保存即时 inspect 对照。产品接口本身已在该次真实运行核验，不以 Runner 直查替代。

N1 重验仅调整新夹具：业务 Spec/Plan 描述产物，阶段指令放 Runner 公开 prompt_overrides；正常/审计产品规则未放宽。驱动对 blocked/failed 及时失败，20 分钟业务预算覆盖两项审计和原修复预算，清理独立有界。重验通过后增加的 caller JSON presence 边界只影响省略/null/空对象保真，已补 U1/U3/I1，无新增模型链。

### 精选真实事实与清理

- Run 60d97eb7f186d06ae22cb9c0e602cc07；candidate 9331b8ad977345288f22d891c3223aa2；input_digest 沿产品响应及候选固定。
- 原 session 01a1476b-3840-76e3-84c0-3e2dc4208223；5 个 canonical 来源均属该会话/候选/Run。
- 两项审计共用 source_report_command_id=588a7d5350a8f1f99cfbc973a526ba3a；具体审计 command/turn 保存在正式响应。
- [真实测试 PR #431](https://github.com/erranliu/SteerlineTest/pull/431) 已合并，merge=7a101dbb4c179e6fcc52ca429d17cab56e3998cf；唯一 TaskDelivery，后继 ready 且未启动；专属 base/head、原 Runner 停止和目录清理均已核实。
- 首次失败 PR #430 仅属本次测试，未合并；失败及重验 cleanup_complete 均 true。成功路径停服后的额外诊断读取 discovery 不存在，发生在已经正常 stop 后，不是业务/清理失败，保留原日志。
- 未运行完整 L2、完整 L3/L4、Orca 或重复原生故障矩阵；未触碰 C:/SteerlineInstall / C:/SteerlineRuntime。RCB-M01 未安排，允许后置。


## 本轮实现复审修复（2026-10-17）

本轮修复实际请求的实例竞态、重建准入未核对原公开事实，以及普通 Codex 模型 schema 与生产不一致三项。Runner 增量见 [PR #20](https://github.com/erranliu/SteerRunner/pull/20)，调用方依赖其 `instance_binding.expected_instance_id: true`、实际 `--expected-instance-id` 围栏及修订后的组合 schema；先合入 Runner，再启用选择新 SHA 的 SteerLine。不会凭相同 version 接纳旧 EXE。

- RCB-A02：业务/查询请求保留预期实例，Runner 发现和服务受理端均核对，绑定调用不隐式启动服务。重建时先以公开 CLI 对账原 Run/input_digest 或原 submit request_id，再开放准入；冲突/缺失/不可用不上线。pending 原输入撤销；unknown submit/控制标为只观察，查无原请求也不自动重发。原信封、停服正文和旧实例结算身份不改写。
- RCB-A03：普通 Codex model 的首尾空白、全空白、换行与生产同判；Runner R1 为 18 个正反例，R3 四个包通过；细节归 Runner 专属验收。真实 EXE I1 已通过，包括匹配实例查询、直接 stale-instance 拒绝与原实例结算。
- 固定 Runner 修复构建：`C:/Project/SteerRunner/.evidence/collaboration-fix-7f82fb0599514709af3590f77c1ca661/steerrunner.exe` 与 build.json；SHA `74a3ab9129f287fd362696ba8ba862a88701c46aa443c22b4d33879605696992`，resource `2cd6bac5029ab42d939fce9b25c97316a5dda404056e9a000616ee24726de1f5`，version/CLI 仍为 0.1.0-dev.1 / steerrunner-cli/v1。测试时来源 529b6c3 + dirty 实施，随后提交为 Runner `49c4ff7`；未覆盖安装或原构建。
- U1 首轮含 33 个测试/subtest：app 正向/拒绝替代/拒绝重启摘要冲突及 bridge 定向项均通过；runnerclient 的模拟 query 错把 request_id 输出为空串而非 null，导致 2 项失败。仅修复该 fixture 信封，runnerclient 复验 20261017T034230.312157200Z 通过；没有把失败记录改为通过。原失败日志为 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261017T033936.394880200Z/summary.json`。主干合入后再执行完整 U1，结果追加如下。
- U3：`20261017T034207.288275400Z/summary.json` 通过，覆盖 bridge/taskrunner/environmentcheck/sqlite，包括 pending/unknown 原 submit 零重发及原 Run/请求身份冲突；77 项全部通过，零跳过。
- I1：`C:/Users/lmas1/AppData/Local/Temp/steerline-collaboration-fix-18e97677397a434891e2688601444622/cli-report/i1/summary.json` 通过；同目录 capabilities.json 来自真实修复 EXE，并更新仓库 fixture 的对应公开字段。
- C1：`20261017T034151.106617800Z/summary.json` 通过且清理完成；范围仍为 controlled gate-only。
- 合入主干 `65f7d6c4` 后，保留其执行位置复制及 Orca 接线；产品投影相关定向回归 `20261017T034549.647979900Z/summary.json` 通过，覆盖 U2 与执行位置投影。文档检查为 469 份 Markdown / 2866 个链接，零错误。

以上相对 test-feedback 路径均位于 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/`。原 N1 证据只证明原固定构建；本轮新 EXE 的 N1 正在独立分级隔离夹具中执行，最终结果、真实 PR 与停止清理证明追加登记，不用原 N1 代替新构建验收。人工 RCB-M01 继续后置。

- 合并后 W1：5 个文件、31 项测试通过（包含执行位置复制与协作 UI），TypeScript/Vite 生产构建通过。

- 合并后最终 U1：`20261017T034703.352034700Z/summary.json` 通过；app/sqlite/runnerclient 精确执行 Plan U1，包含原两条必需换代断言、预检后换代拒绝及重启原摘要冲突不上线；零失败、零跳过。
- 新构建 N1 首次复验：`C:/Users/lmas1/AppData/Local/Temp/steerline-collaboration-fix-18e97677397a434891e2688601444622/n1/summary.json` 失败，cleanup_complete=true、零真实 PR。正式产品响应证实原会话已完成实施与两项审计（均 pass，repair_count=0），切回执行模型后的 publish 在 `sandbox_readiness_failed: native_sandbox_command; no business input sent` 被阻断；未绕过就绪门或补发原请求。该失败不计 A06 通过，未重写旧构建 N1 事实。使用同一固定 Runner EXE、主干合并后的 caller 新建独立 `n1-retry` 现场复验，不复用或重置失败现场。

实现 PR：[SteerLine #347](https://github.com/erranliu/SteerLine/pull/347) 与 [SteerRunner #20](https://github.com/erranliu/SteerRunner/pull/20)。PR 不代表部署或合并；Runner 是新调用方启用前置。


### 修复构建 N1 最终结果

`n1-retry/summary.json`：passed=true、cleanup_complete=true、零 errors。本次 caller 来源为已合入 main 的 `292737c39154eb3c07cf1fbf77adacb2ef5d7769`，Runner 为上方同一固定修复 EXE/SHA；之后 caller 与 Runner 只追加验收文档，执行代码未变。证据根：`C:/Users/lmas1/AppData/Local/Temp/steerline-collaboration-fix-18e97677397a434891e2688601444622/n1-retry/`。

- 真正执行/审计/发布：同一原生 session 的 5 个 final；执行请求 gpt-6-luna/medium，audit.spec 与 audit.code 请求 gpt-6.1-sol/high，均 pass、repair_count=0，再切回原执行请求模型发布。
- 原 Run `5191ea00d9323efebdf4159790a72c23`，候选 `c03e3d6455034d4fbf42422a5c508df0`，input_digest `617a63ffd31c079ad2fdd9bd4bd1c5475dd8a03f7c13f53ac89a32e4177a8156`。详情核对来源、冻结材料、两项审计和轮次请求模型；Task 历史核对同一候选/Run 与冻结预设，另存即时公开 CLI inspect 投影作对照。
- 本次专属 [SteerlineTest PR #432](https://github.com/erranliu/SteerlineTest/pull/432) 已合并，merge `45891b62c0a516b163119f7adda58618490200bb`；唯一 Delivery 计数 1，后继 ready=true、unstarted=true。scenario.json、live-completed.json、audited-product-*.json、audited-product-task-history.json、audited-runner-comparison.json 保留证据。
- 原实例停止和最终清理均核验；live-runner-cleanup.json 为 confirmed=true、error=<nil>，scenario cleanup_complete=true。未部署安装环境，未复用失败现场，也未将首次失败覆盖为成功。

本轮 RCB-A02/A03 修复、U1/U3/I1/C1 及新构建 N1 已通过；RCB-A04/A06 的真实产品链证据对应本次新构建。RCB-M01 仍待人工确认，不声明 full-L2/L3 或 Orca 原生矩阵通过。
