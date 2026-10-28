# 调用方提示词收敛验收记录

## 2026-10-28 最新 main rebase 与完整 L2/L3 复验

本次按用户要求 rebase 到 `origin/main` 的 `82161c42e925d81f705c4e0496b47e293897db5b`，随后重跑目录内全部 L2/L3，发现失败后直接修复。下方“前次文档驱动实施记录”中的旧基线、局部验证与失败状态保留历史，以本节为最新结果。

rebase 冲突保留 main 的 Plan 历史，并把 PR399 的新增执行约束迁入唯一完整规则文件：允许规范必需的环境/输入/输出参数，但不得缩小测试选择；只报告当前步骤事实，不因索要未来 PR URL 阻断报告。`internal/prompts/materials/testing.md` 的 LF UTF-8 字节从 2720 增至 2963（+243），9 个短模板未变；同步规则摘要与既有快照测试，不恢复已移除的内联长指令。

受测源码为干净提交 `82b3158dfef9d07a9a8dc423c51785999934c3e7`，Windows/amd64、Go 1.26.4。以下证据相对独立根 `C:/SteerlineDebug/wt-2352-all-l2-l3/`；L2/L3 选择集合逐项等于目录内对应级别的完整集合，均为 controlled，零跳过，`cleanup_complete=true`。L3 最多两个场景并发。

| 最终复验 | 结果 | 准备 / 执行秒数 | 原始 summary |
| --- | --- | --- | --- |
| 全部 L2 | `smoke.delivery`，1/1 通过 | 15.591 / 85.452 | `test-results/L2/20261028T134428036Z-final/summary.json` |
| 全部 L3 | 7/7 通过 | 28.746 / 534.644 | `test-results/L3/20261028T133535726Z-all-after-fixes/summary.json` |
| L1 | 85 pass / 0 skip | 0.415 / 11.931 | `test-results/L1/20261028T134648591Z-final/summary.json` |
| PR 元数据关联回归 | 11 pass / 0 skip | — / 18.984 | `diagnostics/20261028T134648384Z-baseline-regression/summary.json` |

L3 包括 `configuration.execution-profiles`、`recovery.runner-wait-retry`、`recovery.waiting-merge-restart`、`runtime.dual-home-operation`、`runtime.service-lifecycle`、`runtime.shared-runner-projects`、`testing.runtime-progress`。正式 L1/L2/L3 报告已通过原 `test-feedback record` 导出到[受测提交的 Git 测试历史](../records/82b3158dfef9d07a9a8dc423c51785999934c3e7.json)。后续提交仅补本节与该记录，不改变受测代码。

两次完整 L3 的失败原报告和原生导出均保留，不覆盖为通过：

- `test-results/L3/20261028T125150367Z-5e298df6/summary.json`，源码 `ddb3f4e064c92bcb52800b1928f9513cf73013f8`：远端重试夹具缺结果文件能力及环境就绪前提；原始场景为 6 pass / 1 fail，管理汇总为 5 pass / 1 fail / 1 incomplete（同包合并恢复场景）。[原生失败记录](../records/ddb3f4e064c92bcb52800b1928f9513cf73013f8.json)保留该状态。修复受控能力声明和真实 Git 固定材料，通过公开 `environment_start` 完成原 Run 停止、v1 结果封存、报告通过与环境准入，再启动重试任务；不写数据库绕过生产门禁。保留重启、重试耗尽、零额外业务输入与取消断言；夹具报告只证明 controlled 流程，不计原生环境资格。
- `test-results/L3/20261028T130910070Z-full-final/summary.json`，源码 `0e39cf9a303768df26db7dc509c85b51ba7eb433`：6 pass / 1 fail，进度场景在自动材料准备查询阶段超时，[原生失败记录](../records/0e39cf9a303768df26db7dc509c85b51ba7eb433.json)保留。确定性回归进一步复现 GitHub PR 关联查询会绕过所选自定义 provider、从 PATH 调用 `gh` 的缺陷：修复前两个边界子例失败，修复后通过。现在仅配置为 `gh`/`gh.exe` 的 provider 执行该能力，并使用配置的完整 executable；不支持的自定义 provider 保留原诊断，Gitea 路径不变。定向回归覆盖真实 Git 基线比较/阻断、Gitea 关联及命令选择边界。自动材料等待同时保留最近成功轮询，便于定位后续准备问题。

两项修复各自定向复验后，在同一干净源码上重新执行上述完整 L2/L3，全部通过。Web 测试前置依赖安装及构建通过；未执行 L4、真实模型/Orca、部署或合并，不将 controlled 通过升级为原生资格或完整 Full 验收。

## 前次文档驱动实施记录（历史）

关联 [Spec](../../spec/caller-prompt-convergence.spec.md) / [Plan](../../plans/caller-prompt-convergence.plan.md)。2026-10-28 按用户授权，在 PR407 固定模板机制上整合 PR406 的文档驱动修订。PR406 参考版本为 `e6b97c354f5467d7e847dadeb3d497a2ceca8f0b`；已对 `origin/main` 执行 rebase，基线仍为 `a5c12eafcb2bf6e442de07ded2c6fd4affeb9541`，无新增提交需要重放。未合并 PR406/407、修改 Runner 或部署。

本轮受测为 `345887d1d98224195a44b7744655c10420e3f221` 加本 PR 工作树，原 summary 保留源码状态；不冒充干净提交或真实 Agent 资格。Windows/amd64、Go 1.26.4，独立证据根 `C:/SteerlineDebug/wt-2352-document-driven/`。初次治理及 PR405 的结果保留历史，不继承其通过结论。

## 当前实现及边界

9 个短模板仍从 `internal/prompts` 唯一组装；5 份完整规则保存在同包 `materials/`。新生成请求使用原 `generate.repository` 固定提交，`generate.materials` 为空；原 schema 字段结构及结果验证保留。Roadmap 的 evidence 原文行枚举已从 schema 移除，结果仍逐字对照原文，不从 schema 旁路传送原文。

项目文件必须按正常 Git 流程准备，程序只核验，不导出或自动提交材料：测试 `.tests.json` 展开后逐项等于完整冻结定义及顺序；协议/writer 只允许 LF/CRLF 差异；Roadmap 原文、图、映射、确认规划逐字核对原权威；环境公开配置和完整前轮报告逐字对照 context JSON。详细固定路径和 JSON 格式见 Spec，不新增第二份安排权威、数据库字段、材料发布服务或临时仓库。

环境报告字段只维护在 `environment-execution.md`，生成与执行均须核验/读取。全部成功、失败、incomplete、未知和清理事实完整保留。当前渠道不能把刚封存的完整报告送入下一 Run 的新工作区，因此原轮完成失败后保存准备不足并停止自动新轮次；首次完整成功仍可正常就绪。此缺口不是“三轮自动链通过”。

用户回答按原通道发送，完整实际文本最多 2000 UTF-8 字节，多问题序列化后的文本合计；覆盖 1999/2000/2001、中文、emoji、选项及多题。超限返回实际字节和上限，未生成投递信封，不截断/摘要/转文件。已冻结请求仍按原身份观察、恢复和原 CAS，不重组装或重复提交。

| 验收项 | 已验证范围 | 状态与未验边界 |
| --- | --- | --- |
| CPC-D01 | 实际请求无长规则/源码/原规划 inline；9 个短正文黄金样本及完整请求/schema 反例 | 受控已验；原 Runner/用户正文继续由原来源决定，不把 caller 统计当总模型输入 |
| CPC-D02 | 真实固定 Git 文件、完整提交、规则/原文/确认数据一致、结果来源 manifest | 文件可用性/来源及受控读取已验；真实模型/远端 Agent 读取资格未验 |
| CPC-D03 | 缺 commit/reader/文件、无权限、错误原文/图/映射/规则、过期 context、缺/变 writer 阻断 | 已验；准备不足反例不计为业务完整流程成功 |
| CPC-D04 | 完整冻结安排/note/顺序、真实选择核验、步骤权限、原预留/CAS及进度恢复 | 已验受控链；未触碰旧现场 |
| CPC-D05 | 真实 Git 受控 Roadmap 生成/发布/入图；完整前轮报告文件；环境首次成功与失败阻断 | 受控已验；项目材料须正常 Git 准备，自动下一轮完整报告通道仍缺失 |
| CPC-D06 | 2000 字节边界、合法原回答、原冻结请求观察与恢复 | 已验 owner/受控；真实远端投递未验 |
| CPC-D07 | 已支持条件、准备缺口、原报告和未验资格分别登记 | 已登记；独立审核与远端保护仍待对应验收 |

## 正文变更与门禁

同一合成输入比较 `345887d1` 的黄金样本与本轮短模板，单位 UTF-8 字节，仅统计 caller 新增正文：

| 用途 | 原值 | 当前 |
| --- | ---: | ---: |
| 测试执行，同源 writer | 2183 | 458 |
| 含用户原文的步骤组合 | 2211 | 486 |
| 测试只读步骤 | 303 | 163 |
| 测试安排生成 | 415 | 258 |
| Roadmap 解析 | 765 | 245 |
| Roadmap 文档生成 | 1478 | 343 |
| 环境生成 | 5762 | 294 |
| 环境 prepare | 1022 | 272 |
| 环境 publish | 788 | 259 |

缺 writer 的旧内联样本已删除，新操作直接准备不足。长规则以真实文件完整提供，当前 LF 资产分别为 testing 2720、roadmap-parse 766、roadmap-documents 1363、environment-generation 4731、environment-execution 2877 字节。前轮报告保持完整，不计为删除的信息。长规则使用测试内少量字节摘要快照触发显式审阅，不再复制正文，也不是运行时签名或预算；短模板仍逐字黄金比较。更新期望值不替代独立审核。

## 自动验证与失败留痕

以下路径均相对独立证据根；未导出 Git 测试历史：

- `test-results/L1/20261028T195638654Z/summary.json`：固定 `l1.minimum` 通过，85 pass / 0 skip，准备 0.359 秒、执行 9.670 秒；包含规则字节快照、正文样本与有限源码守卫。
- `test-results/L2/20261028T194844659Z/summary.json`：固定 `smoke.delivery` 通过，准备 15.849 秒、执行 93.904 秒，`cleanup_complete=true`。
- `test-results/L3/20261028T194924885Z/summary.json`：`testing.runtime-progress` 通过，准备 15.353 秒、执行 122.348 秒，`cleanup_complete=true`；实际准备固定协议/writer/安排，保留原进度、重跑、错误、恢复/重启和输入不重复断言。
- `diagnostics/20261028T195810260Z-owners-pass/summary.json`：prompts/taskrunner/testmanagement/roadmap/environmentadaptation/runnerclient/sqlite 完整 owner 回归通过，307 pass / 3 预期 opt-in skip，28.671 秒。三个真实公开环境/Runner 专项未执行，不算通过。
- `diagnostics/20261028T195115621Z-app-verify/summary.json`：应用定向集 42 pass / 1 fail，376.743 秒；真实 Git Roadmap 全链及材料核验通过。唯一失败为旧环境用例仍期待自动第二轮就绪；已修正为首次成功准入、缺完整报告渠道拒绝新 Run 两条路径，`diagnostics/20261028T195605729Z-workflow/summary.json` 3 pass / 0 skip，58.454 秒。不能把前一失败报告记为全绿。
- `diagnostics/20261028T195351573Z-materials/summary.json`：完整前轮报告固定文件、过期 context/被改规则拒绝及生成队列专项，3 pass / 0 skip，71.957 秒。
- `diagnostics/20261028T200030406Z-frozen-recovery/summary.json`：为原已停止 Run 夹具正常 Git 准备协议及正式 writer 后，部署中断、Runner 替换/接管、启动取消 outbox 和原 Run 绑定回归通过，37 pass / 0 skip，280.188 秒；保留旧请求、身份和零额外业务投递检查。
- `diagnostics/20261028T195419564Z-final-owners/summary.json`：扩到既有结构检查的定向运行整体失败，225 pass / 1 skip / 2 fail；两项为 `TestTM04ExecutionBoundary` 及其 `repository_all_tags_windows_linux` 子项，仍是未改动的 `sqlite/environments.go -> environmentadaptation` 禁止依赖。已在首次治理的 main 基线快照与修订版本双向复现，原证据见 [治理验收](prompt-governance.md)。本次有限提示词守卫/L1 通过，不声称完整架构门禁全绿。

本轮早期编译/夹具不完整、缺文件和旧预期失败的原报告均保留在 diagnostics，未覆盖或自动重试。后续按具体失败修正并使用新目录复验；冻结请求恢复的早期缺协议失败已通过正常 Git 夹具准备修复，结果见上项。启动器的 `TestInstallLaunchersWritesCRLF` 在未改的 main 快照上也失败（期望写出文件数与现有四个文件不符），基线复现为 `diagnostics/20261028T200101353Z-baseline-launcher/summary.json`，13.394 秒；该问题不在本次提示词范围，不宣称全 app / Full 通过。

现有调试包装的受控 Runner seed 已组合既有公开 repository capability/schema，避免包内夹具继续声明旧 inline-only 输入；仅做公开夹具适配，PowerShell 语法解析通过，未执行新的调试发布或部署。

未开展真实模型、Orca/Unity 探针、业务 PR、原 Task18 两轮、旧运行注入、跨会话消息或子代理。未启用 GitHub 必需检查/独立审批保护；合入前仍需独立审核，不以本聊天自查替代。

以下各节均为 PR405 已交付范围的历史记录，原失败、证据位置及未验边界不覆盖。

## PR405 第一阶段实现与范围（历史）

实现基于 main `ab66befe2ae4bf13de3e580d62ca2e39d51c84df`；PR399 调查版本为 `48ad01467976aff768c11f5deaaf521efbe717ee`。仅整合本次需要的完整 writer、逐 ID 接线约束、发布/报告只读边界及规范/Plan 必跑并集，不合入 PR399 其他业务改动。未修改 Runner、存储 schema、模型执行入口、预算、旧运行或安装现场。

完整安排仍原样保存，紧凑提示包括有序 ID、来源、定义摘要和原快照摘要；准备阶段从固定 Git blob 核对 Plan、规范、逐项完整定义及 note。writer 只允许 CRLF/LF 等价，缺失或不同源仍完整内联。Roadmap 规范正文保留在冻结记录用于 manifest 校验，模型通过既有固定仓库读取；原 Markdown、确认规划及其他生成的必要 inline 材料不变。环境派生事实仅省去成功项冗长 message，原报告及全部失败、未完成、PR 未知/清理事实保留；阶段从原结果初始 JSON 取关联字段。

## PR405 自动验收（历史）

| 验收项 | 已验证范围 | 当前结果 |
| --- | --- | --- |
| CPC-A01 | 完整快照不改、有序 ID、定义/note/digest/Plan/commit 不一致及必需文件缺失拒绝、mutable checkout 不影响固定来源 | 已验收 |
| CPC-A02 | 无 helper 完整源码、同源/LF/CRLF 路径引用、不同源完整内联、逐入口约束；SQLite 拒绝错误 writer 分发；公开/用户正文保持、发布/报告只读 | 已验收 |
| CPC-A03 | 测试材料和输出 schema 保持；Roadmap 非快照原文和确认输入完整，规范 manifest 缺失/错摘要拒绝，真实 Git 受控文档生成/发布/入图链保持；环境 Report 结构说明完整 | 已验收 |
| CPC-A04 | 长成功明细投影、全部检查/L2 的 ID/状态、完整失败/incomplete 原因、PR URL/未知/清理、无报告诊断、原报告不变及确定性；prepare 保留诊断，publish 不重复前轮问题 | 已验收 |
| CPC-A05 | Unicode、多问题/选项/自由文字原样传递；已冻结环境请求不重准备/重发；原候选 CAS/恢复、L3 重启不增加业务输入 | 已验收 |
| CPC-A06 | 同材料 UTF-8 组成测量、有/无 writer、多 ID 长 note、长前轮报告及固定仓库规范；必要大输入保留 | 已验收 |
| 固定回归 | L1 `l1.minimum`、L2 `smoke.delivery` | 已验收 |

独立证据根：`C:/SteerlineDebug/cpc-9588/`。每次命令的完整参数及受测源码状态见该次 summary，原始日志不进仓库：

- `diagnostics/core-02/summary.json`：`./internal/testmanagement/... ./internal/runnerclient ./internal/storage/sqlite -count=1`，210 pass / 3 预期 opt-in skip，30.214 秒；三个 skip 为真实公开环境检查/result 往返/Runner CLI 绑定专项，本次不执行 live，不能算通过。
- `diagnostics/generation-01/summary.json`：Roadmap、app 的选定 44 项通过，包含真实 Git 受控流程及固定源反例；该次环境包编译失败，整次状态为失败，不能登记为整体通过。
- `diagnostics/environment-02/summary.json`：修正测试调用名称后，环境 owner 全部 30 项通过，15.735 秒。
- `diagnostics/answers-01/summary.json`：原生答案完整性用例通过，2.532 秒。
- `diagnostics/semantics-01/summary.json`、`diagnostics/measure-02/summary.json`：原安排、完整 writer、前轮事实、阶段身份/冻结恢复及最终安排测量通过。
- `test-results/L3/progress-03/summary.json`：`-level=L3 -suite=testing -env=controlled -scenario=testing.runtime-progress`，1 场景通过，`cleanup_complete=true`；准备 16.292 秒、执行 126.479 秒。新增紧凑输入到候选检查，保留原进度/重跑/格式错误/断连恢复/重启/原输入和清理检查。该次在提交前工作树执行。
- 同一干净提交的最终回归：`test-results/L1/final-01/summary.json`（44 pass / 0 skip，准备 0.546 秒、执行 10.123 秒）、`test-results/L2/final-01/summary.json`（`smoke.delivery` 通过、cleanup 完整，准备 14.542 秒、执行 99.831 秒）、`test-results/L3/final-01/summary.json`（`testing.runtime-progress` 通过、cleanup 完整，准备 17.702 秒、执行 132.549 秒）。正式报告已通过原 `test-feedback record` 导出到[受测提交的 Git 历史](../records/210790fa4f721db67cbad3dcbec7794f1e4f2bfe.json)；后续仅补验收说明及测试记录，不改受测实现。
- `python scripts/check_doc_links.py`：597 文件、本地链接、0 错误；`git diff --check` 通过。

## PR405 实际减量（历史）

以下为组件的 UTF-8 字节，不是最终模型总输入或投递稳定性结论。测试安排/选择/环境提示以前述 PR399 固定 Git blob 为比较基线；Roadmap 用 main 固定规范。未重复计算请求 JSON 中的持久化副本。

| 相同材料组成 | 整改前 | 整改后 | 保留内容 |
| --- | ---: | ---: | --- |
| 两 ID 长 note 执行上下文；同源 helper | 10893 | 2260 | 全部 ID/顺序/来源/digest，完整 5496 字节快照未改，规范/清单固定文件可读 |
| 同一安排；缺 helper | 10893 | 6004 | 仍完整提供正式 3253 字节 writer 及接线要求，不为减量省源码 |
| 测试选择指令 | 988 | 415 | 规范与 Plan 必跑并集、影响追加、空选择例外、原 schema 和全部 inline 材料 |
| 环境生成固定说明（Git blob LF） | 5896 | 5570 | 完整环境 Report 字段/语义、L2、真实 PR、用户准备及清理约束 |
| 环境 prepare 指令；同一活动/轮次/源码/摘要/失败原因 | 1228 | 1102 | 身份由原结果初始 JSON 提供，完整失败原因仍在 prepare |
| 环境 publish 指令；同一材料 | 994 | 788 | 保留本阶段约束，关联字段由原初始 JSON 提供 |
| 长前轮原报告 / 派生事实 | 15675 | 740 | 全部 ID/状态、完整失败/未完成原因、来源与 PR 事实；原报告未改写 |
| Roadmap 规范相关正文 + inline | 1319 + 10344 | 1917 + 0 | 规范完整固定仓库内容/来源校验；原 Markdown、确认规划仍完整 inline |

规范 5919 的历史工作区数字与本次 Git blob LF 的 5896 区别来自行尾，未据此虚报语义减量。小安排或缺 helper 项目仍可能有较大输入，不设比例门禁。测量脚本与固定来源/组成保存在 `diagnostics/measurements/measure.py`、`components.json`，安排/报告合成夹具及测量日志见对应 Go 用例。

## PR405 原失败与边界（历史）

- `generation-01` 的新增测试误调用不存在的 `Tick`，改为正式 `Observe` 后完整环境 owner 补测通过；保留原编译失败日志。
- L3 首次参数未经 PowerShell 字面引用，在参数校验阶段退出，未执行场景。正式调用对含点号参数加引号。
- `test-results/L3/progress-02` 的新增断言错误要求只发布 implementation 的受控 peer 也提供 publish 提示，等待超时；原产品候选实际已正常接受。改为核对已发布步骤，完整发布/报告权限隔离由真实 workflow 形状的 owner fixture 验证。该次失败且 cleanup 完整；新现场 `progress-03` 已通过。

本次没有新增人工必验项，未执行真实模型、Orca、Unity、真实业务 PR 或原 Task18 两轮。受控/静态结果不声称模型实际阅读、真实发布或输入投递稳定性通过。原现场及历史验收保持原状。
