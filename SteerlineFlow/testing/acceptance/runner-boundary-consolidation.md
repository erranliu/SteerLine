# SteerLine 与 SteerRunner 职责边界审计及验收

关联 [Spec](../../spec/runner-boundary-consolidation.spec.md) 与 [Plan](../../plans/runner-boundary-consolidation.plan.md)。2026-11-05 审计基线为 SteerLine `57e3c52e` / Runner `e7876c0d`。S1–S4 已实施，约定自动验收通过，运行证据集中于仓库外 `C:/SteerlineDebug/wt-4ba6-boundary/`。普通开发直接实施；Runner 修改在独立 `codex/steerline-boundary-spec-plan` worktree，未操作安装版。独立只读审核已完成实现、提示词接线和受影响测试选择审查；最终范围及原失败按下文实际结果登记。

## 1. 建议审计及修正

| 发现 | 现行证据 | 处理 |
| --- | --- | --- |
| 原生执行迁出后，Web 仍持 Codex/Orca 协议 | web/server/discovery/codex.go、orca.go、orca_local.go | 优先迁 Orca 到 Runner 现有能力层，再迁 Codex，调用固定无服务 CLI |
| 策略槽位已经有公开来源 | Runner resources.ModelInputContract.workflow.project_policies.slots；本仓 app/project_policy_slots.go 仍有重复常量 | 取消新目录设计，S3 消费已有字段，固定提交预检归 SteerLine |
| 缓存键含私有配对身份 | discovery.Query 的 env.pairingKey/local.identity | 取消迁移组合的跨请求新鲜缓存，保留在途合并和过期展示 |
| Web 当前依赖守卫拒绝 runnerclient | internal/testkit/boundary/check.go 的 webConfigDependency/httpConsumerForbidden | 沿 Web 现有 nativequery 的 metadata 传输，公用值放 runnercontract，避免放开后端依赖 |
| Roadmap 停服证据借测试生成 model | app/roadmap_recovery.go 的 roadmapBinding、application_verify_stop.go | 中性 caller 事实归 runnerbinding，逐调用点保留原判断和 Store/CAS |
| 环境与任务 PR 事实容易混合 | environmentadaptation/service.go 的原生参数选择；storage/sqlite/runner_result.go 的交付事务 | 环境执行构造另审，唯一 TaskDelivery 继续由 SteerLine 在候选事务确认 |

## 2. 自动验收矩阵

| ID | 行为、反例及完成判断 | 状态 |
| --- | --- | --- |
| SLB-A01 | 固定 EXE/pin/schema/source 校验，无服务查询；能力不足只限制对应查询；超限/错误 JSON/退出码映射准确 | 通过：core-second、published-runner-web-final-matched、http-final |
| SLB-A02 | 本机/远端 Orca、Codex 目标与 profile 一致，失败零回退；无凭据出 Web；完整/部分/空目录及原始 ID/effort 准确 | 通过：迁移的 Runner 原生矩阵、Web/HTTP 与 live-native-final |
| SLB-A03 | 两并发、在途合并、最后订阅取消、Web 关闭排空、父 CLI 结束及清理失败有证据 | 通过：discovery/nativequery、Runner 最终完整 Controlled 的公开目录 CLI 精确 PID/Job 证据 |
| SLB-A04 | 配置页零项目可用；新选择拒绝旧结果；迁移路径不使用旧五分钟缓存；原生缓存时间、过期展示及产品支持交集正确 | 通过：late-config/expired/sharing 回归、http-final、live-page-final |
| SLB-A05 | 策略目录同源且严格校验，固定 commit 预检；可选缺失/不可读区分；显式空数组/顺序/完整请求字节有对照及独立审核 | 通过：policy-byte-comparison、core-second，正文逐字节相等，独立审核批准 |
| SLB-A06 | 原 request/Run/input/project/object/workflow/revision 比较集中，错误关联拒绝；Roadmap 不转换为 Generation | 通过：runnerbinding、原 TM/GQ/App 回归、go.roadmap |
| SLB-A07 | 两个生成服务从正常入口处理原结果、manifest、确认和额度；超时只查原请求，在线缺失不作冷读缺失证明 | 通过：core-second、正式 L3 material-generation、go.roadmap |
| SLB-A08 | 原停止信封先持久化后发送、保存失败零发送、unknown 不重发、新 revision 仅在明确拒绝后采用，停止未核实继续保留义务 | 通过：StopEnvelope/原 TM/GQ/App 回归，零项目独立停服/离线核验通过 |
| SLB-A09 | 源码/依赖守卫、固定 L1/L2 与受影响 owner 正式集合通过，旧原生实现/槽位常量/跨领域转换删除 | 通过：L1/L2、守卫、HTTP/Web/Roadmap/L3 及 go.deployment |
| SLB-A10 | 同最终固定 Runner 构建、独立 home 的真实无模型目录到正式页面联验，三目标分别记录来源和清理；执行资格另核对 | 通过：最终同 EXE 六项 CLI 查询、published-runner-web-final-matched、live-page-final |

全部为可自动验证项。条件不足的真实目标保留待验，不以受控 fixture 推导目标资格。人工 UI 主观体验可选，不新增阻塞项。提示词/接线/资产及对应测试选择按独立审核要求处理。

## 3. 实施与证据

Web 删除原 Codex RPC 和 Orca 本机/远端连接，仅以 `executionconfig`→`nativequery` 调固定 Runner。可选 schema 单独校验，能力错误不污染普通配置读取；公开响应校验目标、版本、EXE 摘要和 resource_digest。保留两并发和在途合并，上一结果只能标记过期；配置/pin 变化拒绝晚到的结果及其数组。源码守卫拒绝原生 net/pipe/进程/加密依赖回流，原 Web 对常驻 runnerclient 的限制保留。

策略预检直接消费冻结 Catalog 的既有有序 slots，保留缺失/不可读、显式空数组、固定 commit 和脏工作区对照。`runnerbinding.Content` 只持中性事实与纯比较，Roadmap 的非空 digest 要求和 Generation 的原规则分别保留，Store/CAS、原信封持久化及发送顺序未转移 owner。删除 Roadmap→Generation 转换。

| 正式/定向证据（均相对此次仓库外根） | 结果 |
| --- | --- |
| `test-results/L1/final/summary.json` | passed，36.77 秒 |
| `test-results/L2/final/summary.json` | passed，完整 L2，cleanup_complete=true |
| `test-results/L4/http-final/summary.json` | passed，最终串行 HTTP/30 秒压力集合 |
| `test-results/L4/web-unit-first/summary.json` | passed |
| `test-results/L4/roadmap-first/summary.json` | passed |
| `test-results/L4/deployment-final/summary.json` | passed，完整正式部署集合，零失败/skip，原超时断言未修改 |
| `test-results/L3/material-generation-first/summary.json` | passed，正式受控材料生成入口 |
| `diagnostics/core-second/summary.json` | passed，合同/客户端/内容生成/App 代表性正常与异常入口 |
| `diagnostics/boundary-first/summary.json` | passed，源码及 Windows/Linux 依赖守卫 |
| `diagnostics/policy-byte-comparison/summary.json` | passed，21.35 秒，含正常 PrepareTestPlan 字节对照 |
| `diagnostics/zero-project-stop/summary.json` | passed，stop/restart 与残留/未结算反例 |
| `diagnostics/published-runner-web-final-matched/summary.json` | passed，实际匹配 `TestNativeCatalogPublishedRunner`，真实最终 EXE→Web |
| `live-page-final/result.json` | passed，浏览器 1/1，19.6 秒，stop_exit=0、cleanup_exit=0 |

实际执行中保留首轮失败：缺少可选 slots 的历史 fixture 导致材料预检失败后补齐同源目录；HTTP fixture 由旧 app-server 改用公开目录；首轮页面查询通过但零项目清理误要求 Runner StateDir，补充严格空 scope 分支后最终整条页面驱动通过；部署首轮根清单漂移补齐正式集合后，第二轮仅 `TestRunnerPresetFreeze/reject_mismatched_run_after_explicit_restart` 的 Git 子进程超时，保留报告并复跑，不放宽生产或测试超时。一次最终 EXE 定向调用误写测试名而零匹配，不列作通过，改用正确名称另存 `*-final-matched`。

## 4. 提示词、资产与独立审核

实际模型正文差异为空。`TestProjectPolicySlotsPinnedCatalogAndRequestBytes` 在正常服务入口比较硬编码旧构造与新公开目录构造的完整 Run 输入：UTF-8 **2706→2706 字节、逐字节相等**；四份完整 PromptOverrides 同样相等：implement.execute **830→830**、implement.report **312→312**、publish **372→372**、publish.report **319→319**。原完整正文保存在既有 `internal/prompts/` 文件和对应组装函数，没有 inline 兜底或黄金样本替换。

业务变化是去掉 caller 重复目录，使用 Runner 现有生产 slots；测试 capabilities 只增加同源描述和实际公开 native_catalog schema。Runner 新增 `native-catalog.schema.json` **0→11039 字节**，属于查询元数据 schema，未进入模型提示词/Workflow 正文。原生 RPC 夹具迁至 Runner，公开 CLI 新增一个 Controlled 根，原 27 根保持，当前 28 根。测试范围及正常驱动/helper 退役反例由独立只读 reviewer 审核。

独立审核发现并修正：第二次 Orca listing 响应未清空旧标志、local 空 environment/schema 与生产 gate 偏差、配置变化后旧数组仍被发布为 ready；补入 metadata 路径/相对 PATH 反例及原 CLI 强杀的精确子 PID 退出断言。后续审核确认纯绑定保持原语义，提示词完整字节相等，零项目空 scope 分支仍要求原 host/成员/退出证据，最终 helper 夹具接受原生 driver 并拒绝退役 helper。生产 gate 未为了测试放宽。

文档独立复核又发现隐藏/重复/空 ID 与 effort 的反例证据缺项，已补 Codex hidden/includeHidden/default 与四项身份反例、Orca 共用解析六项反例；错误测试要求真实校验错误和未到期 context，不能用超时冒充拒绝。`runner/catalog-identities-final/test.jsonl` 两包通过，独立审核批准。Runner 完整 L1 第三轮出现既有 evidence 释放后等待 30 秒超时及 application 整包 20 分钟上限；前者原断言定向重跑通过，后者基于先前 1159 秒的整包通过证据，仅将 Go 整包 watchdog 增至 30 分钟，经独立审核后 `formal-final` 完整复跑，原单用例/产品期限未改。

Runner `formal-final` L1 已通过：853 根、21 扩展、原 16 个允许 opt-in/helper skip，application 1625.679 秒、覆盖率 80.9%；完整 Controlled 28 根中的两个历史 terminal 夹具暴露当前 structured 契约不一致，原报告整体仍为 failed。独立审核后只适配七个排队/七个创建恢复窗口，精确区分原辅助 shell 关闭和从未创建的业务 session，保持未知创建不得收养/结算。定向两组均通过，最终完整 28 根在 `runner/controlled-reviewed-final` **exit0、28/28、零失败、零 skip**，完成于 16:31:51 +08:00。正式 L1 后仅一行 map 缩进及三份集成测试/helper 变化，`post-l1-source-delta.json` 保留原指纹和精确差异，resources/CLI owner 已重跑通过，不把原前置证明改写为新指纹。`final-verification.json` 核对全部正式报告、当前 catalog/实际根数、同 EXE 字节及真实目录/页面清理，passed=true。

## 5. 固定构建与真实目标

固定最终 EXE：`runner/reviewed-final-build/steerrunner.exe`；version=`0.2.0-dev.4`，base commit=`e7876c0dcbfbbc87a92543a66fdf14fb1de84834`，source_dirty=true，source_digest=`3b0dd992f94a37465c2f749ffd4b6d36b822edb63567e4dd416e77447c857d3f`，SHA256=`87d23fecaeccf253bb9920f0d6124be66cb1ed671e18dfd79160ecca8120e742`，resource_digest=`d575984a2d64b37a5325edf5abe8313a1c8ab7c98f8ba14adf6daf4e0aa124bb`。补测文件及最终 map 缩进进入构建来源清单，故刷新 source_digest；实测与此前 `runner/final-build`、`runner/final-qualified-build` 的 EXE 字节 SHA 完全相同，以下真实目录/Web 证据按相同固定 EXE 字节复用。

`runner/live-native-final/summary.json` 六项 ready/exit=0：登记环境 2、本机 Orca Agent 8/模型 8、明确远端 `e68f4edf-8a83-42c2-a9da-6d404d3aa3d8` Agent 7/模型 8、本机 Codex 模型 8。Orca 原生版本 `1.4.246`/protocol=3，Codex `0.166.0-alpha.6`。完整公开响应单独保留，包含原始 ID/effort/来源时间及可核实 runtime，不含凭据。另一个登记远端 `9cf377a1-42de-4f2b-8874-70f63a7bbab2` 在早期查询返回 unavailable，其原因未确证，不计入通过目标。

同 EXE 的 `live-page-final` 使用独立包 `packages/dev-20261105-144154-57e3c52e-dirty`，启动前只移除本包生成的项目登记，保存文件名/SHA；页面断言零项目，实际查询本机 Codex、本机 Orca 及指定远端。完整截图与公开目录见其 `browser/`；原 host 与 Web 已停止、空 Runner scope 离线核验成功。只验证目录，不新增模型执行/PR/全 Agent 执行资格。

## 6. 文档验证

`python scripts/check_doc_links.py` 实际通过：685 份 Markdown、4251 条本地导航、0 错误。11 份新增文件 UTF-8、末尾换行及逐行空白通过；按仓库原有换行归一化执行 `git diff --check` 通过。运行证据与文档静态检查分开，不以静态检查代替产品通过。
