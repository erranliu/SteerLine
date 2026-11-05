# 用量事实与计费职责划分 Spec

日期：2026-11-05。状态：用户已授权实施，隔离环境验证中；不启动 roadmap、不分配 Task ID，暂不部署安装版。Runner 源码变更在独立副本交付，验收结果单独登记。

实施安排见 [Plan](../plans/billing-ownership.plan.md)，场景、状态和证据见[专属验收](../testing/acceptance/billing-ownership.md)。

## 1. 成果与当前差异

Runner 负责返回可核验的 Token 消耗事实；SteerLine 负责维护计费设置，把这些事实转换为费用估算并展示。价格变化不改变用量事实，费用变化不改变执行或交付状态。

调整前的 [调用方](../../internal/app/runner_usage.go)先读取 Runner 的 `run-usage/v1`，再调用 `estimate-run-cost`。`execution.json.pricing_policy` 是由 Runner 读取的价表文件路径。调整前 SteerLine [用量 DTO](../../internal/runnercontract/usage.go)只消费汇总；2026-11-05 核验安装版 Runner 已公开 `run-usage-events`，其 schema 已含实际 provider/model、call_time、metadata_basis/time_basis、decision 和分类 Token。应先接入这项现有公开能力，再补齐尚未表达的事实/语义，不能把调用方未消费误写成 Runner 没有明细。

| 所有者 | 本次职责 | 不承担 |
| --- | --- | --- |
| Runner | 原生用量采集、来源资格、Run/会话归属、去重、计数归一化、实际模型及时间事实、完整性、只读版本化查询 | 价表、费率、币种、价格匹配、金额计算、SteerLine 业务汇总 |
| SteerLine | 应用级价表管理与版本、用量协议校验、价格匹配和计算、候选绑定、查询与展示 | 解析原生日志、访问 Runner 私有数据库、猜测实际消耗、因查费用而启动或恢复执行 |

本文以既有 `run-usage/v1` + `run-usage-events` 为契约基线，§3 列出必须补齐或确认的语义。只对必要增量进行版本化扩展，不强制为职责迁移另建名为 v2 的接口或合并两个命令。拟议新字段/能力须由 Runner 正式 schema、CLI、能力矩阵和固定构建交付；接线前固定唯一正式契约，不在代码里猜字段或并列两套接口。

## 2. 首期范围

- 同一 SteerLine home 共用一套计费设置，维护多模型费率和历史价表版本；不增加项目、执行 profile 或候选的覆盖配置。
- 一并补齐远端 Codex 用量读取的配置与诊断闭环：区分未配置授权、配置已保存待重启、运行中有效策略与采集失败；价格设置和采集授权分别维护，不为显示 Token 自动扩权。
- 执行页当前所选候选的 Runner Run，范围固定为 `main_session`。包括该范围内的多轮、多 Attempt 和实际模型切换，失败、取消或放弃候选仍可查看已发生用量。
- 只做 Token 按固定单价换算的估算；界面称为“费用估算”，标注“按配置价表估算，非实际账单”。
- 本次不实现任务/项目/应用总额、子 agent 消耗扩围、生成/环境准备等其他 Run 的费用入口、订阅/credits/实际扣款、余额/充值/发票、自动抓价、汇率换算、预算停机或计费驱动的执行 gate。
- 首期不增加持久化费用账本或 SteerLine 用量数据库副本。历史候选能否重算取决于原 Runner 公开快照是否仍可读；原事实不可用时明确不可用，不能用累计历史页面拼接账单。

未来汇总须另定业务归属、各 Run 范围和去重规则；不得只统计胜出候选或将父 Run 已含的子会话再次相加。本次不预建汇总接口。

## 3. Runner 需要交付的公开用量契约

### 3.1 快照与分页

保留既有两步查询：先 `run-usage --run-id R` 取得汇总与 revision，再 `run-usage-events --run-id R --snapshot-revision V [--after C] [--limit N]` 取得同一快照明细；后续页使用返回的 next_cursor 作为 --after。现有接口已支持指定 revision，分页最大值和错误须按所选构建正式发布值校验，不改造为猜测的 --cursor 参数。能力目录需要明确 schema、query_only、同快照承诺、分页限制及 adapter/target 采集矩阵；新增的计价事实随正式 schema 发布，nullable 字段均明确。

| 数据 | 约束 |
| --- | --- |
| `run_id`、`snapshot_revision`、`scope`、`observed_at` | 绑定原 Run；revision 非零且独立于业务 revision；scope 为 `main_session`；观察时间不是用量发生时间 |
| `status`、`tokens`、`coverage`、`issues` | 保留 pending/available/partial/unavailable、可空计数、预期/核实来源数、是否 finalized、诊断；精确定义 available 与完整性的关系 |
| `events`、`next_cursor`；目标计数 `record_count` | events 沿既有明细页返回；Runner 裁定 decision=counted 的记录构成计价集合，其他裁定不得再次入账；该集合的总条数/类型计数须能与summary核对，同一快照内身份唯一。record_count 为包含interval时的拟议增量，不强制改名events |
| `access_cleanup_status` | 继续表达采集访问回收事实；不决定价格规则，不作为执行/交付凭证 |

全部页必须固定同一 revision、Run、scope 和完整性元数据，不混入后续快照。Runner 明确快照保留/过期行为；无法再读取指定快照时返回公开的 `snapshot_expired`，不得改读 latest 并声称原 revision。游标串用、错误 Run、无效/重复页或非终止游标均拒绝。调用方必须设置整次查询的取消、总时间和记录数/字节预算；超出预算作为费用查询失败，不能把截断当成计价部分。

查询只能读取 Runner 已采集事实，不启动服务、worker、模型、日志补采或新的远端授权。现有完整实例绑定与 `--expected-instance-id` 作用于每次实际请求，不用先查 status 代替。用量能力缺失仅使本功能不可用，不新增任务执行准入门槛。

本次固定实现继续使用 v1：run_usage 增加 `same_snapshot=true`、`events_schema=run-usage-events/v1`、`max_events=500`、`access_requirements=true`；summary 可带 source-linked access_requirements。调用方每页200、整次30秒/16MiB/50,000事件预算，逐页固定完整实例绑定和 revision，并核对计数与各分类汇总。首期没有发布 interval 扩展；当前 events 仍按既有 counted_responses 核对，不推断缺失原生事实。纯计算 owner 保留中性的 interval 区间判定，但不据此宣称任一适配器已经交付 interval。

### 3.2 用量记录

一条记录代表一次响应或一个无法再细分的非重叠用量区间，不能把原生累计值当增量不断相加。

| 拟议字段 | 语义 |
| --- | --- |
| `event_id`、`source_ref`（领域内映射为record/source身份） | 复用既有稳定公开身份；跨采集重放保持去重，不暴露私有日志路径或凭据 |
| `granularity` | `response` 或 `interval`；同一消耗不能同时出现在响应和区间记录中 |
| `attempt_id`、`generation`、`native_session_id`、`native_turn_id`、`native_response_id`、`command_id` | 沿既有明细表达原执行归属；未知项遵循正式schema，不造 ID 充当原生证明；interval不冒充原生响应 |
| `provider`、`model`、`metadata_basis` | 复用既有实际模型及来源，可空；只有已核实observed元数据可计价。配置意图、别名和当前 profile 不作为实际模型替代值 |
| `call_time`、`time_basis` | 复用既有时间事实，可空，UTC；首期使用已核实request时间，不默认以completion/observed_at/文件mtime替代；只有完成时间时Token可显示而费用未计价 |
| `interval_start`、`interval_end` | interval 的实际覆盖区间，可空，采用左闭右开；无法确定边界时保留未知 |
| `tokens`、`metadata_issues`、`decision` | 复用既有计数与裁定；counted Token仍可能有模型/时间冲突，必须消费metadata_issues并拒绝相应计价；非负整数或null，已知零为0 |

response 必须使用明确request时间匹配价格；interval 只有在已知区间完全落入同一条价格规则且模型已核实时才可计价。跨价格生效边界且无法细分的区间保持未计价，SteerLine 不按时间或 Token 比例摊分。缺实际模型或用量时间、元数据冲突的记录仍保留可信Token，并报告不能计价的原因。granularity/interval是拟议扩展；即使无法提供，也必须保留现有Token汇总并明确无法计算的覆盖，不能推断响应记录已齐全。

同一 revision 的记录集合不得有累计/增量混算或重叠；来源纠正、补采、原生计数重置可产生新 revision，旧记录在新集合中替换，不要求累计 Token 永远增加。Runner 必须公开每个 Token 字段的覆盖语义：

- 计价分量为 `input_uncached`、`cache_read`、`cache_write`、`output`；前三者互斥，output 包含 reasoning。
- `reasoning` 是 output 的子集，仅展示，不重复加价；`input_total`、`total` 是汇总字段，不另计费。
- 能完整归一化时，`input_total = input_uncached + cache_read + cache_write`，`total = input_total + output`。不能证明归一化的分类为 null，并带原因；Runner 不能为了凑齐等式补零。
- cache_write 等原生语义若无法映射到上述互斥分量，公开未知/不支持；不同缓存档位无法表达时不能默认为单一已知档位。分类归一化由 Runner 负责，不由 SteerLine 按 provider 私有规则解析。
- 汇总与记录集合使用相同归一化和缺失传播规则；已知部分可展示，但不能把缺字段的求和称为完整总量。summary 与明细矛盾属于契约错误。

原生源只有累计 Token 时，Runner 可以发布 interval 记录和未知模型/时间/分类；这证明有消耗，不证明已具备完整计价条件。采集矩阵须区分“能展示 Token”与“具备哪些计价事实”，不以合成模型填补本机 Codex 或远端的缺项。

### 3.3 移出计价职责

Runner 的目标公开能力不再承担 `estimate-run-cost`、`run_cost`、`run-pricing-policy/v1` 或价表文件读取；删除其价格校验和金额 DTO/测试/文档，不因 SteerLine 仍在展示费用而保留第二套计算器。`usage_policy`、远端访问授权及回收继续归 Runner，SteerLine 按既有部署路径传递；它与计费设置分开，不随改价热更新。

两仓交付需要匹配构建。SteerLine 不保留调用旧估价器、缺少计价事实时猜模型、读取旧价表或双算比对作为正式运行后备路径。升级、原现场排空与停止继续按现有部署规则，不通过迁移用量/价表来绕过。

### 3.4 本次一并处理的远端用量问题

2026-11-05 对安装home `C:\SteerlineInstall`、项目 `C:\Projects\SteerlineTest`、Task 101、候选 `3b21b9e861fc40f79324e9afa1afadc5` 只读核验：实际Runner Run为 `0b2ac9b03e2d8fdf05ac1d4785bccf03`。公开run-usage的revision 8返回unavailable/tokens=null、verified_sources=0/expected_sources=1，原因是REMOTE_LOG_ACCESS_NOT_CONFIGURED及COVERAGE_INCOMPLETE；同revision明细为空。服务有效策略为enabled=true、remote_log_access.mode=disabled/grants=[]。这是采集授权未配置，非价格缺失或已证实的前端丢Token；启用授权后的原日志/绑定/解析是否成功仍须另验。

本次增加以下闭环，不把用户手改JSON作为唯一产品操作：

1. SteerLine在全局执行配置增加“用量采集”设置，沿现有execution.json.usage_policy owner条件保存；配置启用/关闭、远端temporary模式、environment、经核实的稳定host_id和受限session_roots。原样传给Runner，由Runner校验/采集/临时目录登记与回收。billing.json不保存这些权限。
2. 沿公开service status读取实际usage_policy/digest/revision，与已保存配置分开展示“运行中生效”或“已保存，待正式重启”；服务离线显示未知/离线，不能拿文件值冒充当前有效策略。保存采集策略不热更新、自动停服或自动续发输入；正常停启及未结算资源规则保持。
3. Runner的缺授权诊断须能关联原source_ref和冻结environment；配置所需host_id与合规最小日志叶目录必须来自已核实公开事实/只读发现，不能从profile、IP、主机名或workspace猜测。若当前公开接口不足以提供这些事实，将最小需求诊断/只读发现作为Runner增量前置；不读取Runner私有checkpoint补齐。无法核实则阻断该授权保存/应用并给原因，不自动放宽根目录。
4. 仅允许Runner认可的session日志叶目录（如Codex日期叶目录）；不使用账户home、认证根、宽泛.codex根或通配符。temporary登记不等于远端只读ACL或自动TTL；界面明确范围及Runner报告的回收状态，不新增远端helper或日志读取终端。
5. 执行页摘要直接显示“远端用量未授权”，并提供采集设置入口，不把主因藏在折叠内或只显示“费用未估算/未配置价表”；分别呈现Token采集诊断与计价诊断，保留source关联及retryable。HOST_OFFLINE、LOG_ACCESS_DENIED、绑定不支持和格式不支持均区别于缺价表，不要求改价解决采集故障。
6. 正式停启使新许可生效后，Runner对仍有可信原绑定的缺口重新进行有界观察；SteerLine根据新实例/策略身份失效原“不可重试”的用量缓存，重新读取公开快照。不得为补Token另建候选、resume/reply原Run或执行模型；日志已缺失/不可绑定继续报告真实原因，不承诺历史必能补齐。旧实例、grant失效/主机变化或回收未核实沿原规则处理。

现有UI已有该错误码的折叠说明和停止轮询规则；本次补的是设置、摘要提示、有效策略/待生效状态和新许可后的观察闭环，不重建日志解析器。该安装现场当前只读诊断，不修改配置、远端登记、进程或原Run；未来现场恢复须在新实现隔离验证后，核实该主机/目录与正式停启条件再单列记录。

## 4. SteerLine 计费设置

### 4.1 唯一权威与版本

拟议使用 `<home>/billing.json` 作为应用级唯一持久配置，独立于 execution.json 和项目数据库。中性配置存储模块统一提供读取、校验和条件保存；Web/应用只通过该 owner 访问，不各写一份。读取不创建文件，缺文件表示未配置。

文件含 schema、单调 `generation`、`active_policy_version` 和不可变价表版本集合。每次保存费率生成新版本并原子切换 active；保留原版本供复核。切换已有版本只改变 generation/active，不修改版本内容。不支持原地改版本、删除被保留版本或静默覆盖。版本身份由 owner 生成，摘要基于规范化内容；摘要算法、规范化和上限在实现时固定为一套。

保存携带上次读取的摘要/ETag，跨进程锁内重核后原子替换；首次创建使用明确的 create-only 前置，缺少前置、并发冲突、校验或写入失败不覆盖已保存文件。不支持的旧配置版本明确拒绝，不迁移。复用现有私有目录/普通文件/链接拒绝和原子写入规则；设置读取或校验失败只使费用不可用，不影响 Token 和执行。

### 4.2 价表内容与匹配

每个版本含 ID、版本、创建时间、来源说明/可选来源链接、币种及费率规则；不内置供应商现价，不联网查价。每个版本只允许一种币种，版本之间可改币种；金额只在各自币种内解释，不折算历史结果。

规则固定实际 `provider + model` 精确匹配，带 UTC 生效区间 `[effective_from, effective_to)`（结束可为空）、四类计价分量的十进制单价和固定单位“每一百万 Token”。同一模型规则的区间不得重叠，不使用通配符、模糊别名、默认模型价或最近价格补洞。非负单价必填，允许明确设置 0；拒绝负数、科学计数、非法数字、无效时间、重复身份和歧义区间。

首期每类 Token 仅一个固定费率，不实现上下文阶梯、batch/service tier、缓存写入多档、地区折扣或逐请求附加费。价表明确 `basis=manual_flat_token_estimate`，表示用户选择的固定费率估算；这些未建模条件作为可见假设，不宣称与供应商实际结算逐项一致。需要这些条件精确计价时另扩契约，不能偷偷从 profile 推导。

设置页位于全局配置的“计费”入口，零项目、Runner 离线时仍可读取、校验和保存；显示当前版本/币种、费率及生效时间、来源和固定费率假设。支持编辑后保存为新版本、选择保留版本为 active，给出字段错误、冲突和未保存提示。

保存/切换成功后立即用于下一次费用查询，不重启 SteerLine/Runner，不修改 ConfigDigest、profile digest、候选冻结输入或 Runner 参数。只清理费用查询缓存；Token 观察及执行资格不失效。

## 5. SteerLine 计算与结果

### 5.1 纯计算与一致性

新增 `internal/billing` 纯领域计算模块，输入为已验证的固定用量快照和一个不可变价表版本，输出金额、覆盖信息、原因与可复核匹配信息。该模块不读文件、调 CLI/HTTP、推进业务状态或访问前端状态；配置存储与查询组装通过端口调用它。

每次查询开始固定 active 版本及摘要，再固定用量 revision，完整消费其明细后计算。中途保存新价表只影响下一次查询；明细页缺失/过期/非法时不能返回基于不完整分页的费用。已独立取得且有效的用量摘要仍可展示，同时显示费用查询错误。

每条记录按实际模型和发生时间匹配唯一规则，对可核实分量计算：

```text
记录金额 = (非缓存输入 × 输入费率 + 缓存读取 × 读取费率
          + 缓存写入 × 写入费率 + 输出 × 输出费率) / 1,000,000
```

不使用 `total × 单一费率`，不重复计 reasoning，不把未知分量当 0。部分分量可计价时可以计入已计价部分，并保留其他分量未知的原因。使用精确十进制运算，单价最多 12 位小数，金额以十进制字符串返回；先精确汇总，显示时统一舍入，不逐条舍入后相加。格式/位数/记录数超限显式失败，不能溢出或截断。

结果至少包含 Run/scope、usage revision、policy ID/version/digest、设置 generation、币种、计算规则版本、估算 basis、记录数、完整/部分/未计价记录数、匹配规则与缺项原因的聚合明细、covered amount 和可空 total。三种记录数互斥且之和等于 record_count，不把 interval 数标作“响应数”。同版本输入得到同样结果；缓存键至少包含原 home/项目/候选/Runner 绑定、Run/scope、usage revision、policy digest 和计算规则版本。

### 5.2 状态

| 状态 | 条件与展示 |
| --- | --- |
| `estimated` | 用量 available 且 finalized，预期来源已知且全部核实、明细完整，所有记录及必需分类可计价；covered 与 total 相等 |
| `partial` | 至少一项可核实分量完成计价，但仍在采集、覆盖不全、存在缺价/缺模型/缺时间/未知分类等；只给 covered，total 为 null |
| `unavailable` | 未配置/配置无效、能力不支持、查询失败，或无任何可计价分量；金额为 null，给明确原因 |

空记录集在用量完整 finalized 且明确总消耗为 0、价表有效时可得完整零费用。任何“总费用 0”都必须有完整覆盖和已知零消耗或明确零费率的证据；部分金额恰为 0 仍显示“已计价部分 0”。未 finalized 不能因当前记录都可计价而显示最终总额。费用失败不隐藏有效 Token，不作为任务失败或恢复授权。

## 6. API 与展示

- 保留现有候选用量查询入口和项目/候选/Run/state-dir/实例精确校验；替换其内部估价来源，公开费用 DTO 改为 SteerLine 所有，不再映射 Runner 金额 DTO。协议变化直接升级调用方/前端，不保留旧响应字段的兼容分支。
- 用量诊断DTO保留category/code/retryable/source_ref及可公开的授权需求引用，不再仅压成字符串列表；运行策略身份与保存配置身份分别传输，前端据明确原因和新许可事实决定刷新，不从elapsed时间推定授权已生效。
- 计费设置提供应用 home 范围的读取、校验、条件保存与 active 切换；沿既有 Web 会话认证、同源、CSRF 和请求体限制，浏览器不得直接指定任意本地价表路径。具体 Web/应用传输路径在 Plan 的接线步骤固定，保存和计算共用同一配置 owner。
- 执行页保留当前候选的“用量与费用”摘要与折叠明细。展示总 Token、分类、范围、采集是否完成、费用状态/币种、已计价覆盖、价表版本、原用量观察时间、估算假设和未计价原因；无价表提供全局计费设置入口。明细展示费用计算摘要，不把 Runner 私有来源路径暴露给页面。
- 本次按**当前 active 价表版本重算可读历史用量**，不在候选创建时冻结价格。旧价格版本保留用于复核，切换 active 会重算；页面说明“按当前价表重算”，不是历史账单。范围和模型变化按同一快照解释。
- 保留可见且在线时轮询、切换/隐藏取消、手动只读刷新及同查询失败保留最后结果的行为。价表变更必须使费用查询失效并触发重算，即使用量已 finalized、轮询停止；不同候选或价表版本的迟到结果不能覆盖新结果。失败保留的旧结果带旧版本和刷新失败标记，不能冒充当前 active 已计算结果。其他标签页通过轻量设置 generation/status 检查发现变更，不依赖候选用量继续轮询。

## 7. 实施与兼容边界

Runner 正式目标契约（含既有接口复用与必要增量）和匹配固定构建交付是接线/真实联验的硬前置；可先按本 Spec 编写 SteerLine 纯计算及配置实现，但不能用夹具通过冒充真实 Runner 支持。所选Runner不符合必要用量能力时明确“用量契约不支持”，不退回旧估价接口，也不阻断无关执行与停止。只缺部分计价元数据时仍展示可信Token，不把整个用量能力一概禁用。

实施时删除 SteerLine 的 `PricingPolicy` 路径、`RunCostSupported`、`EstimateRunCost`、Runner cost DTO 及对应旧测试和 UI 文案，更新执行配置 schema/示例/保存测试；`usage_policy` 原样保留。旧 execution.json 的 pricing_policy 字段不自动转换或继续读取，由用户按新格式配置。移除旧字段需沿现有严格配置校验与明确诊断，不默默丢字段后声称价表已保存。

本次方案涉及遥测、价格设置及既有采集授权的显式配置闭环，不修改 [worker 生命周期](../../docs/product-design/worker-lifecycle.md)、业务 revision、TaskDelivery、模型执行权限或远端授权/停止/恢复/升级的既有规则。新增配置入口不构成默认扩权。实现中若触及提示词及其接线/资产/对应测试，须先单列影响并遵循提示词治理，不能夹带到本计费改动中。

## 8. 通过条件

| 验收 ID | 必须满足的成果 |
| --- | --- |
| BILL-A01 | Runner 正式 schema/能力/分页/快照/事实语义交付并可核验，原生缺项诚实表达；计价职责从 Runner 退出 |
| BILL-A02 | 客户端校验原身份、同 revision 全页、去重/一致性及过期错误；不补采、不执行、不降级 |
| BILL-A03 | 全局配置可离线管理，版本不可变、条件保存跨进程原子、价格区间无歧义；即时生效且不影响执行配置 |
| BILL-A04 | 多模型/多时段/分类/精度/零/未知/部分的纯计算正确，金额与快照/价表严格绑定且可复现 |
| BILL-A05 | 候选 API 与故障隔离成立；费用由 SteerLine 计算，旧路径/估价调用及配置字段已删除 |
| BILL-A06 | 设置页和执行页范围、状态、来源、版本、重算、缓存失效/取消/迟到隔离及宽窄屏自动验证通过 |
| BILL-A07 | 匹配固定 Runner 构建的公开接口与 SteerLine 集成联验通过；真实来源的缺项不伪造，不重复验证 Runner 私有采集机制 |
| BILL-A08 | 固定 L1/L2、受影响 owner/Web/HTTP 和文档一致性回归通过，原失败与补验分别有证据 |
| BILL-A09 | 远端采集策略显式配置、最小根/主机核验、有效/待重启状态、错误主因与新许可后缓存恢复的受控闭环通过 |
| BILL-A10 | 固定新构建/隔离home在远端Codex完成最小授权→Token可读→重读不重复→回收的真实联验；未授权/日志或主机不可用时真实报告原因 |

人工体验 BILL-M01 单列后置；它不替代以上自动条件。具体场景与状态只在专属验收维护。本次文档完成不表示任何产品功能或 Runner 契约已通过。
