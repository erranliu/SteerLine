# 重构与维护最小闭环：验收记录

最小界面与后台流程已实施。2026-11-11，用户授权的固定 Runner、本机 Codex 和 GitHub 专用分支联验通过。两轮维护自动收尾，真实冲突解决后更新原 draft PR 和报告。测试资源已清理，本期未部署。[Spec](../../spec/refactoring-maintenance.spec.md) · [Plan](../../plans/refactoring-maintenance.plan.md) · [测试选择](../../plans/refactoring-maintenance.tests.json)

## 前置和证据

- SteerLine 调查基线 `1f64b0ffeb936358786cfe4734f2096765bc4008` 加本聊天文档；各正式报告保存实际受测源码身份，最终 dirty 文件清单及逐文件 SHA-256 固定于证据根 `final-source-snapshot.json`。
- Runner 前置 #141 / `c404d6e5c50cc32650f9412f9b8b3449c5789e15` 已在独立目录构建固定 EXE，身份见下文；旧 bin/安装 pin 不作为新能力证据。
- 依 Plan 在独立 scope 保存命令、summary、原始输出、浏览器截图/trace、公开请求与停止证明；安装环境与已有调试现场不动。
- 新 controlled/live 场景、正式清单、可读分级、driver 和同名 tests.json 已同步登记。真实场景沿已确认的隔离范围执行，详见本轮联验记录。
- 先前[只读能力核实](refactoring-runner-capabilities.md)描述 #141 前的实际构建，保留其历史结论；Runner 合入与本仓验收分别表达。

## 自动项

下表关联范围均为本 Spec。具体测试 ID、时点、特殊准备与入口归 Plan 第 4 节；实施执行前在本文登记完整实际命令、构建和证据目录。已登记回归引用正式清单，拟登记场景在完成前不得标为可执行。

| ID | 方法和最小通过条件 | 环境 / 触发 | 阻塞范围 | 状态 / 结果证据 |
| --- | --- | --- | --- | --- |
| RM-A01 | 自动：远端 HEAD/配置材料冻结、单项目名额及请求事务，保存失败无提交、并发唯一、双项目不串 | isolated + controlled；P1/P3/P5 后 | 启动资格 | 已验证最小准入/事务/CAS/原请求，见 final-app-02、final-core-04、storage-01 与维护整链 |
| RM-A02 | 自动：实际只读决策、真实文件材料、缺失和超限拒绝，独立审计上下文，项目写入只在 Runner | owner + 固定 EXE；P2 后 | 决策与执行边界 | 原文件供给及来源反例通过。native-09 的真实只读审计核对实际代码和已核实的工作区提交 |
| RM-A03 | 自动：三次以上 execute 的同 worktree/origin/identity/分支，失败或停止后继续最新持有者，原 Repository 不随主线变化 | controlled；P1/P4 后 | 多轮与复用 | native-09 的四次写入均使用同一 worktree 和分支，来源为最近持有者，会话分别独立。失败及停止后来源另有 owner 反例 |
| RM-A04 | 自动：美元/时间/轮次及组合边界，失败计轮、精确金额、有界费用观察、预算耗尽仍收尾，真实无改动有报告 | owner + controlled；P3/P4 后 | 自动推进与预算 | 确定性预算与无改动路径通过。native-09 在两轮额度用完后自动收尾 |
| RM-A05 | 自动：三策略不扩权、必要问题及原题答复、过期/未知不改投，预算不造人工问题 | owner + Edge controlled；P3/P5 后 | 人工闭环 | 策略输入、问题/答复身份及预算规则通过。native-06 出现实际产品问题，记录保留；人工体验签收仍见 M01 |
| RM-A06 | 自动：停止决策和可写输入、迟到结果零后继、封存/写者责任、停止后新 Run 收尾、未知/重启/排空与项目隔离 | owner + lifecycle controlled；P4 后 | 停止与恢复 | 冷启动、原停止及复用回归通过。native-05/08 通过原命令停止，native-09 原资源已核实结算 |
| RM-A07 | 自动：混合 scope 分页、main 计数/Token 验证不含 internal，内部 pending/uncovered、冻结价格、去重聚合、未覆盖披露与原候选回归 | isolated + published fixture；P1/P3 后 | 费用与美元资格 | 混合 scope 与费用边界通过。native-09 采到参考用量，冻结金额和覆盖缺口在两次报告中分别一致 |
| RM-A08 | 自动：一个原 repo/head/base 的 PR，原结果及只读平台核验、报告版本/正文一致；无改动/失败/未知发布不伪报、不重建 PR | controlled；P4 后 | 交付 | 受控来源和无改动路径通过。native-09 两次交付均核验同一真实 draft PR 与报告正文 |
| RM-A09 | 自动：并行目标变化、真实 Git 冲突与收益变化，原 PR 最新来源续作，报告陈旧/新版本、已关闭和在途并发拒绝 | controlled；P4/P5 后 | 冲突续作 | native-09 两文件真实冲突，原 PR 更新，两个简化和并行注释保留，独立 Node 检查通过。报告说明初始化取舍 |
| RM-A10 | 自动：最小页面所有默认/状态、两个运行按钮、答复/历史/续作、宽窄屏/键盘/断线，活动来源与深链，GET 零推进 | web.unit + Edge controlled；P5 后 | 界面自动验收 | 已验证最小页面、人工答复、历史/深链及刷新零命令。live-fix-maintenance-05 浏览器实际发起，答复/冲突经公开 API，web-02 单测覆盖表单答复 |
| RM-A11 | 自动：固定新 Runner、本地 Codex 隔离真实链，至少两轮、预算收尾、一个 PR/报告和冲突后更新，全部 Run/服务结算 | L4/live；其他自动核心项通过后 | 完整真实闭环声明 | 通过，`test-results/native-09`。真实 draft PR #796、同一分支和 worktree、两版报告、冲突和全部资源清理均已核验 |
| RM-A12 | 自动检查 + 独立审核：必要回归、依赖方向、真实提示词正文及字节、固定文件供给、测试登记和长期文档一致 | P2/P6；受测版本固定后 | 实现交付声明 | 已独立审阅并通过本地检查；正文/字节及审批说明见下文 |

RM-A11 不要求将测试 PR 合入业务主线。专用测试 base、PR 状态与保留/清理策略在执行前按正式调试规范固定；发布与资源停止分别有证据。真实失败、未取得环境或缺能力仍为自动项未通过，不能改称人工体验后置。

## 人工体验

| ID | 参与与步骤 | 最小判断 / 阻塞范围 | 安排与状态 |
| --- | --- | --- | --- |
| RM-M01 | 用户在隔离页面体验启动、轮次原因、人工答复、预算收尾、PR/报告与冲突收益说明 | 关键状态、下一步及费用范围可理解；仅阻塞体验签收，不替代任何自动断言 | 自动闭环后集中后置；参与人/窗口待安排，待验收 |

## 本轮材料检查

文档链接、测试清单与 manifest 一致性、原始命令选择及 diff 检查均通过。下列原始报告记录各自实际源码/dirty 状态；后续修改按影响补验，不据本地与协议夹具扩大真实模型资格。

## 本轮实现与审计

实现采用一个维护业务 owner，纯模型子包供 SQL/查询读取；只读决策包不持有 Runner、数据库或项目写入端口。沿现有命令、配置派生、共享生成准入、ProjectRuntime 关闭及 Runner 排空接线，没有新建通用 Agent 引擎、调度框架或假 Task/维护 Spec/Plan。

独立只读审计由 `/root/audit_design`、`/root/audit_lifecycle` 完成并复核修正：去除多余 bootstrap；首次固定提交快照、后续原 dirty 目录；停止/提交派发门、单 worker 推进门、共享配额释放；失败原生结果不作为交付；冷态原 stop 的不存在/拒绝/未知分别处理；原 PR、head repo、正文与报告绑定；预算耗尽不落入选题问题。既有 API 的纯 billing 依赖检查改为复用原依赖矩阵，没有放宽 SQL/执行边界。

首次实现时提示词文件基线均为不存在（0 B）。真实联验中规则和只读短模板的修复差异见下方记录。

| 来源 | UTF-8 文件字节（前→后） | 实际指令字节 | 业务原因 |
| --- | --- | --- | --- |
| [maintenance-decision.md](../../../internal/prompts/maintenance-decision.md) | 0→180 | 179 | 引用完整冻结文件，限定只读决策返回格式 |
| [maintenance-execute.md](../../../internal/prompts/maintenance-execute.md) | 0→548 | 547 | 明确 Runner 可写身份及原结果文件交接，保留操作身份 |
| 当时的 `internal/prompts/materials/maintenance.md` | 0→3876 | 完整文件供给 | 五方向、三策略、独立审计、收尾、费用范围、冲突收益、原 PR 与完整提交绑定；交付后不合并 |

首次实现规则 SHA-256 为 `ba643f9a5bd00e48c10bad3ab5c81d15f3d22e9461b87970c114b1a2eb4b9063`。两个短模板黄金与实际渲染逐字一致。实际请求测试验证完整 rules、原提示词 CRLF/结尾空格、来源互斥及报告 as-of。独立审计批准正文、文件供给、消费与对应测试变化。

## 固定 Runner 与自动证据

独立 EXE：`C:\SteerlineDebug\refactoring-maintenance\runner-c404d6e5\steerrunner.exe`；构建 `0.2.0-dev.4-maintenance`，源码 `c404d6e5c50cc32650f9412f9b8b3449c5789e15`、dirty=false。SHA-256：`d83152a9712486025b18304d3496e62f57072d3d51088f3315632f8c24275773`，resource digest：`732a1c7e211563648ea5b4d5c155345419693f4c78e29c724bb3168244714502`。未替换旧 bin 或安装 EXE。

证据根为 `C:\SteerlineDebug\refactoring-maintenance\`。下表保留初次实现与复验的原目录和结果。本轮 `live-fix-*` 与 `native-*` 的完整命令、测试终态和清理见各自 `summary.json` 及原始日志。上轮的原始结果目录当前未在原路径找到，其条目沿用当时验收记录。正式 ID 的命令以机器清单为准。

| ID / 范围 | 证据叶目录 | 结果 |
| --- | --- | --- |
| l1.minimum | test-results/live-fix-l1-09 | 通过，最终完整最小集合 |
| smoke.delivery | test-results/live-fix-l2-04 | 通过，最终普通任务交付及清理 |
| go.http | test-results/http-01 | 通过；新增维护深链反例另见 final-app-02 |
| go.storage | test-results/storage-01 | 通过；最小回执及维护事务补验见 final-core-04 |
| go.environment | test-results/environment-01 | 通过，正式完整源范围 |
| go.deployment | test-results/deployment-01 | 通过，正式完整源范围；不表示部署安装 |
| go.runner-contract | test-results/runner-contract-01 | 通过，固定实际 EXE 的能力、配置派生和绑定 |
| go.runner-result | test-results/runner-result-04 | 通过，固定实际 EXE、原生协议夹具及维护新增契约 |
| web.unit | test-results/web-02 | 通过，前端全集 |
| runtime.refactoring-maintenance | test-results/live-fix-maintenance-05 | 通过，浏览器表单发起、两轮、失败修复、原 PR/报告、公开 API 答复与冲突续作、宽窄屏、刷新零命令及清理 |
| refactoring.local-codex | test-results/native-09 | 通过，固定 Runner 真实两轮、原 draft PR 的初次交付和实际冲突续作、两版报告及资源清理 |
| runtime.async-activities / shared-runner-projects / service-lifecycle | test-results/runtime-01 | 三项通过，原范围与清理完整 |
| 维护/决策/提示词/费用/依赖 owner | diagnostics/final-owners-02、final-core-04 | 通过，真实非零命中；不是 go.full |
| App 维护/PR/冷启动及 Web 新路由 | diagnostics/final-app-02 | 通过，真实非零命中 |

`runner-result-04` 证明连续实际 execute 的最新持有者、origin/identity/工作区/分支、新会话、停止封存和 dirty 文件保留；generate.directory 固定原目录并校验两份 file manifest，随后写入仍接回原持有者。原生对端为协议夹具，不能据此声称模型实际调查了 dirty 文件、完成收益审计或发布真实 PR。

## 收尾检查

最终 L1、L2、受控维护整链和 L4 真实场景均已通过。正式测试清单与可执行 manifest、精确选择及 live 白名单检查通过。`maintenance-07` 的初次浏览器整链截图已目视检查，最终受控场景再次通过宽窄屏及刷新断言。

独立审阅已批准浏览器补验接线：就绪来自公开环境流程，首次启动由浏览器实际提交，Go 读取同一个原信封后继续核验与重放，没有写入就绪状态或替浏览器预先启动。该证据证明表单发起、结果显示与刷新纯读；人工答复与冲突命令通过公开 API，不能称为浏览器全部操作均已整链验证。

## 保留的失败与未验边界

- maintenance-01 至 04 保留准入引用、测试 DTO 读取和 Web 深链/项目路由识别失败；修复后 05、06 完整通过。
- l1-02 揭示新 live 场景遗漏精确选择白名单，已补齐选择及拒绝其他组合的检查，l1-03 通过。
- runner-result-01 误选 CLI 夹具导致超时，02 使用正确原生协议夹具通过；新增复用验证的 03 夹具遗漏 execute override，修正为生产模板/schema 后 04 通过。以上失败均未用真实模型补跑。
- diagnostics/app-01 是额外 App 全集诊断：旧依赖断言未采用现有 billing 纯模型规则，且全集达到默认 10 分钟超时；未计为通过，也未声称 go.full 通过。相关断言、维护新增根及正式 environment/deployment 范围已分别补验。
- diagnostics/final-owners-01 保留新增测试括号编译失败，修正后 02 及后续 owner 通过。
- 上一轮自动审批因缺少真实模型和测试远端写入的明确授权，拒绝了真实联验。用户本轮已授权，实际结果如下。

## 2026-11-11 真实联验

使用固定 Runner、本机 Codex 隔离登录副本和合成项目。每次尝试使用 `erranliu/SteerlineTest` 的随机专用 base，清理以本轮原 PR 和分支为范围。

| 尝试 | 结果及证据 |
| --- | --- |
| `test-results/native-01` | 原独立 EXE 路径缺失，驱动在启动场景前失败。按 `c404d6e5c50cc32650f9412f9b8b3449c5789e15` 重建，SHA-256 和首次构建完全一致。 |
| `test-results/native-02` | 首次实施接到只读调查指令，独立审计发现实际无改动。修复指令提前发布，draft PR #793 使用了自行命名的 head。后续只读审计触发原生提问，失败结果没有成功材料清单，调用方卡在审计，场景超时。原反馈及失败状态保存在 `observed-early-publication.json`、`observed-failed-decision.json`。`remote-cleanup-verified.json` 确认原 PR 已关闭且未合并、测试分支已删除、Runner 已停止且隔离项目目录已清理。 |
| `test-results/native-03` | 首轮执行已结算，审计原生结果含多余顶层内容，Runner 返回 `invalid_json`。修复后的调用方保留实际诊断并进入阻断，场景及时结束。未创建 PR，远端及本机资源清理已另行核实。 |
| `test-results/native-04` | 首次选择仍触发原生提问。`maintenance-view.json` 保留原失败、Run 与已采集参考费用。未创建 PR，远端及本机资源清理已另行核实。 |
| `test-results/native-05` | 首轮提交并通过 Node 验证。独立审计认可代码，但把晚到用量当作报告缺口，发起了补费用修复。用公开停止命令终止该次尝试并完成清理，未创建 PR。原反馈、停止回执与清理证据分别见 `maintenance-view.json`、`observed-fee-repair-stop.json`、`remote-cleanup-verified.json`。 |
| `test-results/native-06` | 冻结费用快照修复后未再出现补报。独立审计能读取实际变更，但 Git 元数据不可核实时通过产品问题要求原 worktree 或 diff 证据。保存 `maintenance-view.json`，未创建 PR，隔离资源清理已另行核实。 |
| `test-results/native-07` | 两轮通过独立审计，预算自动收尾。首次收尾决策 JSON 无效，同阶段一次新请求成功并保持轮次，见 `decision-recovery-proof.json`。原分支 draft PR #794、初版报告、实际两个文件和 Node 均通过。`conflict-proof.txt` 证实专用基线推进后两文件均有真实冲突。测试在多次远端检查后仍使用旧行动版本，冲突命令被拒。PR 已关闭，远端分支和本机资源清理已另行核实。 |
| `test-results/native-08` | 原分支 draft PR #795 已创建。末审识别出收尾决策所见金额 `0.730418` 与随后发布重新采样的 `0.815105` 不一致，产生重复费用修复。通过原行动的公开停止命令结束本轮。`maintenance-view.json` 保留原反馈，`observed-snapshot-mismatch-stop.json` 保留停止回执，PR 与分支清理另行核实。 |
| `test-results/native-09` | 通过。2 轮和 4 次原生写入。两处真实 Git 冲突处理后，原 draft PR #796 与审计报告同步更新。场景 1080.77 秒并完成清理。 |

修复将单切片实施及限定范围的修复直接委托给可写 Runner。推送和 PR 更新由收尾负责，发布以原 Run 的 `managed_branch` 为准。人工问题以 SteerLine 决策 JSON 返回。失败的只读 Run 结算时保留实际诊断，成功结果继续校验原请求、材料和 Final 来源。

原生提问或无效 JSON 的只读 Run 明确停止后，同阶段只重新生成一次。新请求保留原失败及参考费用，重复失败阻断。专项回归覆盖连续两种真实失败形状、原请求新身份、无轮次增加和无第三次提交。合成夹具 `AGENTS.md` 由 354 缩至 231 字节，留下文件范围、行为和测试入口，发布要求继续经原启动请求交给收尾。实际差异见 `diagnostics/fixture-instructions-live-fix.diff`。两项调整均已独立审阅。

完整规则差异在证据根 `diagnostics/rules-live-fix.diff`、`diagnostics/report-cost-live-fix.diff` 和 `diagnostics/workspace-evidence-live-fix.diff`。规则依次由 3876 增至 4464、4746、4955 UTF-8 字节，最终 SHA-256 为 `c3385287247e3b8ce1e6a1652eefc990c3eb3119d8135ecfc9295868f205b073`。只读短模板明确要求只在最终回复中输出一个 JSON 对象，文件由 180 增至 252 字节，渲染由 179 增至 251 字节，差异在 `diagnostics/decision-live-fix.diff`。可写短模板仍为 548 文件字节和 547 渲染字节。独立审计核对实际正文差异、字节、原分支、费用快照与工作区事实供给、失败结算及新增测试。摘要与黄金样本在批准后更新。

轮次审计只检查工程成果和验证。发布或冲突续作从紧前原收尾决策的冻结 file 材料取得金额、采集时间与覆盖范围，写入操作和末审沿用该事实。实际文件交接测试推进时钟并补齐原用量，仍核实报告使用 0.75 的原快照及原缺口，行动累计更新至 2.25。该接线和测试已独立审核，固定正文保持原版本。

审计材料包含 Runner 已核实的最近写 Run 的原 worktree HEAD、干净状态及观察时间。实际工作区路径与起始基线分开提供，原执行配置和 Repository 绑定保持冻结。固定 Runner 公开 `workspace-git` 的本地 execute 路径已独立核查，真实场景要求至少两份已核实样本。

复验场景取回两次交付的 Git 提交，检查两处冗余均消失，并用 Node 检查普通输入、字符串累加和带访问器的读取行为。冲突续作后保留并行改动。场景分别核验两次均为同一 draft PR、正文与报告一致，且专用 base 只有这一个 PR。原 Run 绑定须显示相同 worktree、分支及最新持有者来源，每次写入使用独立会话。

冲突测试先验证原报告的 head、base 与正文，准备真实冲突后重新读取原行动版本。只有收到明确版本冲突拒绝时才允许在相同来源上用新请求再试一次。独立审核确认产品的陈旧请求校验与原 PR、双 Git 树断言均保持。

`native-09` 的 `refactoring-live-proof.json` 记录 4 份已核实的原工作区 Git 样本。最终 head 为 `f4427e06de7fd5997ed39d8cfca026250bd59ce5`，base 为 `2501382dec9d35a1ab84bc0fd27a927237358350`。`conflict-proof.txt` 证明 `alpha.cjs` 与 `beta.cjs` 的实际文本冲突。初次和冲突后 Git 树均通过独立 Node 检查。最终报告记录保留两处简化和并行注释，并说明采用等价初始化表达式的取舍。

`decision-execute-fee-proof.json` 与 `conflict-fee-proof.json` 分别确认两次收尾的决策与执行使用同一参考费用快照，金额为 `0.591715` 和 `1.203929` USD。行动观察到的累计参考费用为 `1.684702` USD，覆盖未完结。参考金额按合成夹具价表计算，未覆盖子 Agent、工具内模型和不可观测调用。

`remote-cleanup-verified.json` 另行核对 [测试 PR #796](https://github.com/erranliu/SteerlineTest/pull/796) 已关闭且未合并、专用 base 与原 head 分支已删除、隔离 Runner 及项目目录已清理。真实联验范围为本次合成项目与固定本地 Codex 配置，人工体验签收保留在 RM-M01。

## 审阅 PR 的主线整合复验

`native-09` 验证的源码对应功能提交 `b2eeec4e`。审阅分支整合主线 `32b452bc` 后保留活动阅读与重构维护的独立入口，统一使用 SQLite schema 47。既有显式放弃流程可只读检查维护表出现前的库；常规启动继续执行当前 schema 校验。提示词和对应黄金正文相对功能提交逐字不变，独立审核已复核整合后的组装和测试接线。整合后的复验使用受控环境。

| 范围 | 结果及证据 |
| --- | --- |
| L1、L2 | 通过，`test-results/pr-main-l1-02`、`test-results/pr-main-l2-01`。 |
| Web、SQLite、维护和活动阅读入口 | 前端全集及 SQLite 正式范围通过，见 `test-results/pr-main-web-01`、`test-results/pr-main-storage-01`。App、查询和 HTTP 的维护与活动阅读专项通过。 |
| 显式放弃旧库 | schema 44/46 离线流程和 schema 44/45 任务图检查通过。schema 46 夹具实际缺少维护表。 |
| `runtime.refactoring-maintenance` | 独立 L3 通过，见 `test-results/pr-main-maintenance-01` 的 `summary.json`。浏览器启动、报告、历史与刷新检查通过，场景完成清理。 |
| 活动中心浏览器回归 | `test-results/pr-main-l3-01` 首次使用陈旧 Web 产物。重新构建后，`test-results/pr-main-l3-02` 的抽屉与独立阅读页通过，额外的活动反馈用例等待两次候选启动提交时仅观察到一次，整项 `runtime.async-activities` 仍失败。该用例及候选启动前端代码与整合基线一致，未在纯主线单独复现。 |

本次 PR 整理未再次运行真实模型或写入测试远端。真实联验证据仍以 `native-09` 为准。

## 启动前的运行环境复验

受测源码为 `e694d7963fe19335772144d2943ea72bdbd964d5` 加本次工作树改动。正式证据位于 `C:\SteerlineDebug\wt-5144-refactoring-readiness\test-results`，各报告保存实际测试范围与工作树状态。

| 验证 | 结果及证据 |
| --- | --- |
| 维护表单定向测试及生产构建 | 7 项通过。核对未就绪原因、处理入口、点击和回车阻断、配置切换、查询失败及后端拒绝后保留表单。生产构建通过。 |
| L1 `l1.minimum` | 通过，`L1/1502631e6e884f5e90c86bb4758b8900/summary.json`。 |
| L2 `smoke.delivery` | 通过，`L2/1502631e6e884f5e90c86bb4758b8900/summary.json`。 |
| L3 `runtime.refactoring-maintenance` | 通过，`L3/1502631e6e884f5e90c86bb4758b8900/summary.json`。公开接口读到就绪后撤销资格，启动被拒且行动和 Runner 提交均未增加。重新检测通过后从浏览器启动并完成原受控交付流程。 |
| L4 `go.http` | 通过，`L4/27259a70509044b09ab10bb604b6d129/summary.json`。 |

本次复验覆盖组件和受控流程。真实模型联验沿用上文历史记录的受测版本。

## 原项目目录与维护 worktree 的接线

2026-11-11，`merge-h-308-code` 的首轮选题因整个 Git 树超出 Runner 的快照限额而卡在提交。用户确认 SteerAgent 从原项目目录只读调查，Runner 在首次写入时创建 worktree，后续执行复用该 worktree。决策上下文通过 Runner 回传的结果、差异、验证和已核实 Git 事实判断成果。此次验证覆盖应用接线，未增加操作系统目录隔离。

维护请求采用 `generate.directory`，原项目目录为唯一绑定目录。新决策请求只保留所需的 Runner 反馈字段，保留原生问题、封存结果和 Git 事实。另验证 Runner 的 `directory_unavailable` 退出码 4 保留原诊断。

提示词的完整规则从 4905 增至 5161 个 UTF-8 字节。只读和可写短指令文件分别维持 252 和 548 字节。正文改为从原项目目录调查并使用 Runner 回传证据，Runner 的封存结果需说明相关差异、实际验证及影响判断的未提交成果。实际正文差异见证据根 `diagnostics/20261111T105539067Z-runner-contract/maintenance-rules.diff`。独立审核发现的原生问题字段丢失已修复，并以实际决策请求断言复核。固定仓库生成的其他调用方仍核验仓库快照能力。

受测源码为 `b0b5a4913450a0d9345534d73bd69d6b2c179421` 加本次工作树改动。证据根为 `C:\SteerlineDebug\wt-d8f3-maintenance-directory\`。

- 四个 Go owner 包通过，覆盖维护服务、Runner CLI、固定提示词和决策输出。Web 生产构建通过。
- L1 `l1.minimum` 通过，见 `test-results/L1/20261111T105539067Z/summary.json`。
- L2 `smoke.delivery` 通过，见 `test-results/L2/20261111T105539067Z/summary.json`。
- L3 `runtime.refactoring-maintenance` 通过，见 `test-results/L3/20261111T105539067Z/summary.json`。浏览器发起维护，首次及后续只读决策都使用原项目目录，原 worktree 完成多轮执行和冲突续作，更新同一 PR 夹具及报告。
- L4 `go.runner-result` 通过，见 `test-results/L4/20261111T105539067Z/summary.json`。固定 Runner `c404d6e5` 与同源原生协议夹具核对原项目目录、两份冻结材料、四次执行复用同一 worktree 和未跟踪文件保留。

本次未部署。以上验证使用受控执行或协议夹具，真实模型及远程 worktree 尚未联验。

## 维护表单后台刷新

2026-11-12，受测源码为 `400e15dcfab24074d0062caeb46d35043a5e09e8` 加本次工作树中的前端改动。当前配置已有就绪结果时，后台刷新期间可切换方向并提交。首次进入或切换执行配置须等本次查询完成，刷新返回未就绪或失败后禁用提交。提交沿用服务端启动校验。

- `npm --prefix web test -- src/features/refactoring-maintenance/refactoring-maintenance.test.tsx src/app/project-switch.test.tsx` 通过 30 项，覆盖后台刷新、首次就绪、切回已有缓存的配置及服务端拒绝回执。
- `node web/node_modules/typescript/bin/tsc --project web/tsconfig.json` 通过。
- `go run ./cmd/test-feedback -level=L1` 通过，报告为 `C:\SteerlineDebug\wt-6cef-refactoring-readiness\test-results\L1\c17a4b2b-f98b-4174-a0a9-553e0ed0b8f0\summary.json`。
- `go run ./cmd/test-feedback -level=L2` 通过，报告为 `C:\SteerlineDebug\wt-6cef-refactoring-readiness\test-results\L2\1e283e31-ed9b-4bc2-9d8a-5fe9137e4f94\summary.json`。

本次未部署，未运行浏览器整链及真实模型联验。

## 后台刷新与操作状态回归

2026-11-12，受测源码为 `400e15dcfab24074d0062caeb46d35043a5e09e8` 加本次工作树改动。维护表单保留已读取的方向和就绪结果，启动条件失效时在按钮旁显示原因。服务端校验到方向版本变化时，在原回执的 `code` 字段返回 `refactoring_direction_changed`。前端等待所选方向的新版本并展示正文，保留填写内容。提示词正文、版本算法及校验条件沿用原实现。

环境总计与明细共用检测状态，读取失败的行计为待核实。Roadmap 保留已核实的空列表、已读取的对话和正在编辑的输入。活动列表加载后续页时保留原分页，发现首页变化后提示手动刷新，计数可独立更新。

证据根为 `C:\SteerlineDebug\wt-6cef-background-refresh\test-results\`。

| 检查 | 结果与报告 |
| --- | --- |
| Web 全量 `web.unit` | 54 个文件、482 项通过。覆盖就绪状态变化、刷新失败、启动拒绝时保留输入和分页请求竞态。见 `L4/95c0ac33-41df-4613-9fbd-c4966015fc57/summary.json`。 |
| Web 生产构建 | `npm --prefix web run build` 通过。 |
| L1 `l1.minimum` | 通过，见 `L1/5459269d-537c-4a0b-bb9b-d3176fe5059e/summary.json`。 |
| L2 `smoke.delivery` | 通过，见 `L2/67af2838-06ce-421d-86e7-a0f746d0c17c/summary.json`。 |
| L3 `runtime.refactoring-maintenance` | 通过，受控 HTTP 校验旧方向版本及回执错误码，Edge 使用当前版本启动并完成维护。见 `L3/e6becfe6-c3cb-4917-b67e-96b38a75285f/summary.json`。 |
| L4 `go.http` | 通过，见 `L4/11d73652-3d7c-41c3-958b-3cd89a72306d/summary.json`。 |

本次未部署，未运行真实模型联验。
