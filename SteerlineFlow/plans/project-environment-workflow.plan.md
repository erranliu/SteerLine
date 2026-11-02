# Plan：项目检测闭环与平台结果消费

2026-11-02 后续材料接线以[共用材料 Plan](shared-environment-materials.plan.md)为准：项目公共入口/manifest5，按链路保留原资格；下文已执行步骤保留原实施版本事实。

2026-11-01 实施修订稿，**用户已授权修三处并开始实施，实现与所选本机自动验证完成，未验范围见验收记录**。[Spec](../spec/project-environment-workflow.spec.md) · [指导文档](../../SteerlineProjectGuideDoc/environment-adaptation.md) · [历史验收](../testing/acceptance/project-environment-workflow.md)。本轮按修订 Spec 实施环境调用与结果消费；不维护安装现场，不实现项目初始化适配，不启动 roadmap。

## 1. 基线与交付

2026-11-02 最简边界修正：环境入口不读取或运行项目测试，不消费测试安排/报告；业务文档、Spec/Plan、roadmap 不进入环境资格依赖。SteerlineTest 四个环境只保留公共工具检测脚本一项依赖，检查 PowerShell/Git/Python；删除 SteerLine 已退役的测试/发布式环境脚本。Git 材料读取使用有限路径的一次树查询与批量 blob 读取，不延长后台 35 秒预算、不增加重试；原命令错误保留原因，暂时读取失败不持久撤销资格。

验证：固定 L1、L2 和 `web.unit`；`go.environment` 覆盖批量 52 文件、精确字节/路径/类型、读取错误及文档/测试增删不改资格；SteerlineTest 按其 L1/L2/L4 入口验证项目工具，并在仅有环境脚本的隔离目录验证正负结果。实际材料/规则正文差异、UTF-8 字节数、独立审核和原始报告定位归[本次验收](../testing/acceptance/environment-runtime-only.md)。

基线 `7960d1e725082a9f9d757f5af81de2ad32a990f6`，已有 PR [#417](https://github.com/erranliu/SteerLine/pull/417)。其环境生成、manifest3/checks/gaps、业务报告裁决和真实 G02/V05 是旧边界实现及历史证据；本稿全部自动项重新待验，不能沿用“已解耦完成”的状态。

本轮交付：项目自有检测入口 → 一次调用 → 原执行结算/结果封存 → 消费 ready/not_ready 及原因/明细 → 原资格与准入。项目自行生成、验证、检测和判定，平台不负责复核业务结论；保留项目×运行机×链路、统一 Qualify、原请求/当前引用事务。

不实现项目初始化生成器、自检修复循环、报告插件、依赖发现、TTL、签名认证、多版本框架或新 Runner 服务。不为保留旧实现而增加协议副本同步器、兼容旧 report2/manifest3、空 PR 或模拟发布。

## 2. Owner 与撤除范围

| owner | 本轮实施变更 |
| --- | --- |
| internal/environmentadaptation/model | 调用描述改 manifest4，仅入口/解释器/依赖；结果改 version3 身份信封、completed/ready/message 和可选 details；去除 checks/L2/PR 业务裁决 |
| internal/app 环境材料/适配/资格 | 环境不读 environment-project.json/测试清单，不核验项目协议副本；仅读取显式调用材料并计算 inputs_digest，目标事实在原请求冻结 |
| internal/environmentadaptation 与 SQLite | 一次检测调用一次，取消日常生成和业务三轮编排；冻结/持久化新信封，沿用停止封存、未知对账和不可改绑；必要格式换代，无迁移/回退 |
| internal/prompts 及材料/样本 | 平台材料由平台交付，短指令只调用项目入口并引用原关联数据；删除 tty/prepare/publish 业务阶段及项目复制/同源前置 |
| Query/Command、HTTP/客户端/Web | 移除环境日常生成入口，展示项目结论、失败原因、可选明细与独立生命周期状态；保留切换/缓存/权限/revision 守卫 |
| testmanagement/候选与 Runner 接线 | 任务测试来源保持原职责；候选只复用环境 Qualify，不重新读取业务 checks/L2/PR；核验实际调用与结果通道能力 |
| 规范/测试夹具及 test-feedback | 更新与本稿冲突的现行文档、选择器与夹具；移除环境验收中的生成/协议复制/项目原始报告复核前置，保留任务测试和平台生命周期回归 |

现有 environment-project.json 的任务测试用途不在本轮重构；环境入口不再依赖它。复用原 environment.md、脚本、dependencies、Run/result、Current 和 journal，不加文件层或第二就绪状态。

## 3. 实施步骤

### R0：先核实调用能力（A02、A08、A11、A16、A18）

查明固定独立 Runner 已公布的 Run/workflow/result 能否完成“一次项目入口、不强制追加平台发布、停止后封存”。特别核对现有 development workflow 的发布要求，不能将项目不需要 PR 的成功结果强行塞进旧发布流程。确需平台完整文件时，验证在本机/既有远端边界可读且版本可追溯，不要求项目 Git 存副本。

产出实际可用接线及限制，再进入 R1；如现有公开能力不足，明确缺口并补本 Plan 的具体接线后再实施相关路径，不虚构支持，也不擅自扩展 Runner 契约或新建框架。项目无需先 ready 才能运行本次检测；保留执行能力/授权门槛，零 Task 领取。

### R1：最小调用与结果契约（A03、A04、A06、A14、A15）

按 Spec §3 实现 manifest4/version3，不保留旧字段兼容。沿用安全相对路径、regular blob、UTF-8/大小及重复键/未知字段拒绝；项目仅显式声明影响资格的文件，不由平台遍历源码/测试目录。

结果平台预置关联字段，项目入口负责最终 completed、ready、message 及可选 details。not_ready 必须给出可读失败/未验证原因；details 可以携带项目原始检查、日志摘要或证据定位，平台有界保存并安全展示，不解析其业务格式、不复核、不二次裁决。严格信封校验不等于审核项目检测内容。

删除生成路径、gaps 阻断、required_checks 精确集合、全部 L2/TTY/PR/validation_base 的平台环境要求和项目格式专属报告解析。环境入口仅检查运行要求，不调用项目测试或发布流程，平台不用明细重新算 ready。普通 Task 测试安排、writer 与发布自身契约不随之删除。

### R2：绑定、失效和生命周期（A05、A08–A10、A17）

活动先冻结身份/原请求/源提交/显式依据，再产生 Runner 副作用。inputs_digest 只覆盖 Spec §4 的文档/脚本/声明文件和Spec 封闭的全部固定稳定调用字段/接口语义版本，不包含生成指导、模型或整个源提交；activity_id/round、摘要自身、Current revision、Run/request ID、时间戳及临时结果路径均只作绑定，不计摘要。稳定字段为 project_identity/environment_key/call_context.adapter/target.kind/target.environment，全部计入，不筛选项目实际使用的字段。补既定 argv、工作区根 cwd、STEERLINE_RESULT_FILE 和公开原生终态断言。验证相同材料/稳定上下文、不同原调用身份的摘要相同而信封仍不可改绑。证明 A→B 无关提交可复用绑定 A 的旧结果，改变任何声明依据则失效；报告原 source_commit 不重写。

原维护/观察 owner 确认材料摘要变化、声明文件已删除/非法或调用描述失效后，以观察到的 activity_id/revision 事务持久撤销并递增 Current revision；在该基线观察处理完成前不开放候选准入。确认变更后回退原字节也不得复活；网络/超时等纯读取故障只暂阻断，不撤销，恢复再核验。查询不写撤销，未观察中间提交不虚构检测。补 A→B→A、读失败恢复及撤销与新检测竞争反例。

新检测以 expected_current_revision 受理并立即切换 Current；失败/取消/未知不回退旧通过，迟到结果不恢复人工撤销或新引用。查询、预检和候选事务共用 Qualify；事务内再比 revision/依据，不新增缓存或另一份 ready。

不因业务 not_ready 自动三轮或修复生成；一次调用内的业务闭环归项目。受理未知只对账原请求；超时停止原 Run，原执行停止和结果封存未确认前不放行。异常退出不能凭 ready=true 放行，负报告可以保留；平台只消费原封存结果。项目失败与无报告/结算未知分别登记，不将平台异常伪装成项目已判失败。

Current 选择复用目标占用→维护基线门禁→存储事务的顺序，补冻结 A 后观察 pending/已发布 B 的反例。取消恢复仅同构建：使用公开 inspect（无驻留写者时显式 --offline、原租约、只读当前格式、零 worker），核对原 Run/请求/输入及最新 revision，再沿既有启动 pending_stops 在 worker 前受理；原提交冷缺失可判未受理，暖缺失保留未知。保留原 stop 重放，明确拒绝先持久化再保留历史/重建；Closed 原 Run 不加 stop，不伪造封存或业务完成。Runner 保留已受理 PendingStop，拒绝第二个不同 stop，覆盖 late-final/stop 回执结算及公共 EXE 冷恢复回归；不恢复退役 takeover，不增加接管或协议框架。

### R3：页面与规范收敛（A01、A07、A09、A11、A16）

保留按环境观察和检测模型参数；移除环境生成按钮、协议同步/导出及业务阶段进度，把 ready、message、details 和 Runner 结算状态分开呈现。明细仅展示，不能根据出现的 L2/PR 状态或报告 URL 改资格；不自动抓取证据链接进行复核，沿用原安全文本/链接展示边界。

同步 docs/prompt-governance.md、路径/产品设计及既有环境规范的现行声明，明确“平台材料经固定文件提供”不等于“受管理项目复制协议”。通用固定材料仍由 prompts 包所有，业务代码不追加规则、schema 不夹长正文，缺交付能力明示阻断。最终实际正文/接线/样本/测试资产差异及 UTF-8 字节按 AGENTS 独立审核，旧批准及黄金样本更新不代替批准。

### R4：验证与交付（全部自动项，M01 后置）

复用原 owner、受控流程、Web 和正式测试入口，以实际对外契约验证消费边界。项目 A 无测试清单/平台协议副本且不需要 PR，项目 B 自行运行内部多步检查并返回丰富失败信息；平台走同一调用路径，不复核任一业务结论；用明细包含 passed/failed 字样而 ready 不同的输入，证明平台不据明细重判。平台收到 ready 但执行未结算、错身份、过期 Current 等反例必须阻断。

真实链只验证实际调用、正负结果原字节、结果来源、停止封存、当前资格、实际准入及清理，不先跑环境生成或复核项目内部流程才算平台通过。不把受控结果当原生成功，不将历史 G/V 当新协议证据。项目测试和真实发布验收独立执行，不进入环境检测链。

步骤依赖为 R0 → R1 → R2 → R3 → R4；R3 的静态页面整理可在接口冻结后开展，但不跳过 R0 的能力核实。R0 已由用户授权的独立 Runner 最小扩展补齐，R1–R3 已实施；R4 固定回归、受控、Web及真实本机正负调用已按原证据入口完成，Orca原生和人工体验范围见验收。

## 4. 验证计划

遵循[测试指南](../testing.md)、[路径清单](../paths.md)和[验收流程](../testing/acceptance/README.md)。未来工程/实时结果继续写原专属验收文档的新边界段，保留旧失败和原结果。本次不另建文件。未来证据用 `C:/SteerlineDebug/wt-68d4-project-environment-workflow/result-consumption/<唯一轮次>/`，每个命令独立新 out；本轮实际测试结果以原报告为准，旧通过不继承。

| 时点 / 映射 | 最小入口或环境 | 必要断言 |
| --- | --- | --- |
| 当前 D01 | python scripts/check_doc_links.py；git diff --check；三文档正文/UTF-8/字节与独立只读审核 | 边界一致，无协议复制/业务检查编排/前置循环；初始化仍未来，旧通过不继承 |
| R0；A02/A16/A18 | 固定独立 Runner 公开目录及原调用 owner；实际能力小范围验证 | 不强制平台 PR，无资格启动循环；平台固定文件在实际执行侧可读，能力不足明确拒绝 |
| R1/R2；A02–A06/A08–A10/A15/A17 | test-feedback 新 out，定向 internal/environmentadaptation/...、internal/app 的 Environment owner、internal/storage/sqlite 的 Environment owner | 非零命中，新格式/身份/不重发/生命周期与所有失效分支；真实正负项目结果不被平台业务复核 |
| R1/R3；A03/A04/A11/A14/A16 | internal/prompts、runnerclient 及对应实际请求测试；testmanagement 与候选相关 owner | 无复制协议/测试清单前置/内联兜底，任务测试功能独立保持；最终独立审阅实际正文/接线 |
| R3；A01/A07/A09 | scripts/test-web.ps1 -EvidenceDir <新目录>；原 configuration-navigation.spec.ts；必要 HTTP owner | 项目原因/明细原文可读，缺明细不复核、迟到不串目标；无生成/同步按钮或历史假 ready |
| R4；A04/A05/A08/A10/A12/A14/A17 | 复用 L3 testing.project-environment-readiness 受控场景，新 out | 两项目、两环境、无清单/无 PR、丰富负结果、当前引用失效/迟到/准入和清理 |
| 代码修改后与交付前；全部受影响自动项 | 分别 go run ./cmd/test-feedback -level=L1 -out <新目录>；同样运行 -level=L2 | 固定 l1.minimum/smoke.delivery 完整通过，无 skip/零命中/未知清理 |
| R4；A13/A18 | 改造既有 L4 testing.project-environment-local，独立 home/clone/固定 Runner，精确 live 选择 | 真目标正负结果、停止封存、身份/当前资格与实际准入及资源清理；不以内部明细复核授予 ready |
| 后置 M01 | 用户走查 | 能区分项目检测结论、失败原因、平台结算和失效；当前不声称体验通过 |

实施时同步原测试 JSON、suite manifest、选择器和专属验收映射；不把新测试计划写成已经存在的用例或已通过状态。无需默认跑 Full；只有受影响工程改动后运行上表业务入口，当前不跑模型或真实外部 PR。

## 5. 范围及授权

用户已明确授权修复本轮三处审计问题后开始实施。新实现必须遵循 R0 能力前置及独立正文审核；真实外部副作用仍按实际范围授权，不部署安装现场。项目初始化适配、生成自检循环、自动 Git 提交/纳管留未来；不新增文件层、框架、版本协商或旧数据兼容。

项目维护责任包括标准/声明完整性、业务结论和自有资源；SteerLine 可读取、保存并展示最终丰富结果，但不复核证据、重新执行检查或替项目判定。即使项目误报 ready，只要调用/身份/结算均有效，平台也不具备发现业务误判的保证；这是本边界的明确取舍。

## 6. 本轮实际正文与独立审阅

旧环境生成/准备/发布短模板 675 / 260 / 262 UTF-8 字节删除；旧生成/执行完整规则 3862 / 6683 字节删除。新 environment-invoke.md 为 412 字节，代表请求466字节；只固定 cwd/argv、关联结果文件、一次调用及禁止改写项目结果，不调查项目或编排内部检查。完整正文与接线以本轮 Git diff 为准，未把长规则移入业务代码或 schema。新模板 SHA256 `d84781af6aa0835aca77d74e04b8067acf2d3a348a1037b44658d2da4972e98b`。

指导原基线7983字节；三处边界修订后8717字节，最后仅明确公开成功状态还须 status=completed，最终8741字节，SHA256 `2e5b01b228d16bda8b64601d28bcfc015f5730c6e7d0859211a7928d64a99059`。Runner 新 execute 默认正文69字节，prompts.json 1088→1174字节；该工作流要求调用方唯一非空 execute 覆盖，不依默认提示猜测操作。

`/root/prompt_review` 已独立阅读两仓实际正文、schema/资产、接线、相应测试及源码守卫并批准，无剩余可执行 P0–P2。审核发现并修正：单次工作流额外输入/恢复漏洞、基线观察 pending 与最终准入竞争、failed Run 不能被项目 ready 覆盖，以及额外 Task 探针不得覆盖项目声明依据（含大小写别名）。黄金样本和资产期望更新未代替批准；最终文档状态与真实结果继续独立登记。

## 7. Runner 能力与本轮状态

R0 实际核实旧固定 EXE（e78c0db6，SHA256 `326c467d0ce650aca28c54a4f1b68dec616739f4f8ebc6ad699c6ab04bf1dcb4`）及当时 main e272956 均没有无发布可写单次调用+最终封存组合，原证据在 result-consumption/R0/01。已明确向用户报告缺口，用户授权“允许最小扩展 SteerRunner，不增加新框架”，并要求“扩展runner在worktree上执行”。

Runner 独立 worktree 为 `C:/Users/lmas1/.codex/worktrees/runner-project-detection/SteerRunner`、分支 codex/project-detection-execute，提交 `eed760d2331d73cac1ef1f1c3df05c233add5f75`；原 C:/Project/SteerRunner 未修改。新增通用 execute/1：普通可写工作区、唯一调用方 execute 正文、原可信 final、停止及 result_file 最终封存；没有 publication/审计/发布/项目策略或环境检测知识，不接受 progress 或多轮控制，提问/stop 永久关闭，未知仅原身份对账。沿用既有引擎、适配器和结果 owner，不新增框架。

实施轮冻结 Runner 构建在 result-consumption/runner/final，source_dirty=false，SHA256 `2d9f261d09837ba0bfa5017aa0fbc969edf729d1f88e98f67750c9f5cf57189a`。SteerLine 只在实际能力公布 execute/1、原执行适配器/目标及最终 result_file 支持时提交，旧固定 Runner 明确在提交前拒绝。

固定 SteerLine L1/02、L2/01、受控环境 L3/readiness-03、最终 Web/web-03 已通过；Runner 正式 L1/01、TestSRResultFile 公开封存及最终公开契约定向回归通过。受影响 owner 全回归、真实本机正负/清理、原失败和后置人工体验状态以[本轮验收](../testing/acceptance/project-environment-workflow.md)最新段为准，不把受控结果当原生证明。项目初始化未实现；未部署或合并业务主分支。

提交前按用户要求重新审计并整合最新主干：SteerLine main=1f2618da，Runner main=8bed4c4。Runner当前调用验证pin为50f12791、SHA256 f247b1332fb4e5f24aea92db8f9c14dcf044fb0c783a5996ee75e13ed97a570c，保留主干v7有界对账及best-effort文件初始化。独立复审实际整合差异通过，正式Runner L1（728+29）、SteerLine L1/L2与新pin真实本机L4均通过并清理完整。项目规范中“环境共用Task清单”的旧表述已修并独立审核；无新框架或业务检查前置。当前PR结果以验收文末为准，不把前轮pin外推，用户仅授权提两仓PR、不得合并。
