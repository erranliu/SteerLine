# SteerLine 当前 Roadmap 草稿

2026-10-14 按 [RoadmapGuide](../RoadmapGuide.md)选定本页。用户授权将 HTTP 接入后的解耦审计落为五个草稿任务及各自 Spec/Plan；本次只建规划，不纳入 [execution-map](../execution-map.md)、不创建执行聊天/worktree或外部Task、不实施。永久编号已分配，之后纳入沿用，不复用历史ID。

调查基线 `5a24abec3c10d184a6e57962e154854bc6011a9f`。规划约束和生成规范按该版本 [RoadmapGuide](../RoadmapGuide.md)、[SpecGuide](../../spec/SpecGuide.md)、[并行指南](../../architecture/parallel-development.md)核对；实际启动另固定已提交Spec/Plan路径/commit，执行起点核对最新主干。任务草稿不维护实时状态，正式任务/依赖及已验/待验汇总归execution-map与专属验收。

## 本轮目标与限定范围

保留 `TUI → HTTP API → Go 服务`，提高 HTTP传输、固定开发流程、任务最终交付和worker运行时的独立修改/测试能力。保持一个业务actor、一个SQLite权威、原身份/请求/版本与唯一TaskDelivery原子提交。Codex/Orca adapter和Orca本机/environment路由复用，不增加第二业务/执行链。

本轮不改公共wire/模型/JSON持久格式、步骤/恢复/权限/交付语义，不建设通用命令/工作流/连接/调度框架。结构收敛之外唯一明确的小范围修复是#93客户端journal的跨窗口后续更新/终态保护，允许增加journal私有辅助锁文件；基线首次登记原子性不能代替这项修复。其余行为保持，如实际必须改变冻结契约，先重定范围/所有者与真实前置，不能借重构扩大行为。结构完成要求旧重复逻辑/状态删除、小端口可独立验证，而不只是文件移动或宽对象换接口。

## 草稿任务

| 任务 ID | 任务名称 | 成果 / 主要owner | 硬前置与并行/集成安排 | Spec / Plan | 专属验收 |
| --- | --- | --- | --- | --- | --- |
| 92 | HTTP 服务端命令边界收敛 | httpapi消费端口、同包信封登记/对账owner、HTTPendpoint接线 | 无新增硬前置；与93/94可并行，wire/API冻结 | [Spec](../../spec/http-command-boundary.spec.md) · [Plan](../../spec/http-command-boundary.plan.md) | [验收](../../acceptance/http-command-boundary.md)：HC92-A1–A3，owner/HTTP/真实app定向，无新增人工 |
| 93 | HTTP 客户端原请求与停服观察解耦 | httpclient journal/跨进程终态保护、本机observer、consoleapp构造，保留原连接 | 无新增硬前置；与92/94可并行，新增仲裁仅在私有journal | [Spec](../../spec/http-client-request-lifecycle.spec.md) · [Plan](../../spec/http-client-request-lifecycle.plan.md) | [验收](../../acceptance/http-client-request-lifecycle.md)：CL93-A1–A3，确定性交错/Windows/独立simulate，无新增人工 |
| 94 | 固定开发流程规则独立 | development/v3纯规则包、原actor规则接线 | 无新增硬前置；与92/93可并行，与95保持现有调用契约可并行；建议先94后95合入 | [Spec](../../spec/development-flow-rules.spec.md) · [Plan](../../spec/development-flow-rules.plan.md) | [验收](../../acceptance/development-flow-rules.md)：DF94-A1–A4，纯规则/事务/双后端controlled，无新增人工 |
| 95 | 任务最终交付用例独立 | delivery窄端口用例、CompleteNativeTask薄接线，保留原PR绑定 | 无新增硬前置；既有调用/冻结模型下可作为第四条并行线；默认94→95有序合入 | [Spec](../../spec/task-delivery-usecase.spec.md) · [Plan](../../spec/task-delivery-usecase.plan.md) | [验收](../../acceptance/task-delivery-usecase.md)：TD95-A1–A4，用例/原子事务/两条L3 controlled，无新增人工 |
| 96 | worker 运行时资源所有权收敛 | workerruntime观察/句柄/在途资源、actor结果接线；不重写业务恢复 | 无新增硬前置；默认94/95后集成，降低共享actor冲突；该建议不转成DAG依赖 | [Spec](../../spec/worker-runtime-ownership.spec.md) · [Plan](../../spec/worker-runtime-ownership.plan.md) | [验收](../../acceptance/worker-runtime-ownership.md)：WR96-A1–A4，owner/确认屏障/四条L3 controlled，无新增人工 |

已有#91/#45、任务规则/窄端口、后台效果、恢复事实和提交屏障是已交付基线，不重新创建“契约基础任务”。这五项均有独立成果和一份Spec/Plan、一个预期实现PR；仅编号不产生开工授权。

## 独立交付与重构收益

独立交付要求在兄弟任务均未实施的调查基线上也能完成真实调用接线、删除旧主体/重复状态并通过自己的自动验收，不以只有新包/interface或stub通过交付。兄弟实际已合入时补最小组合验证；没有其成果不构成等待理由。#92/#93是同包职责与能力隔离，不宣称包级编译隔离。

| 任务 | 本项交付后可独立完成的改动/验证 | 必须消除的耦合及收益 |
| --- | --- | --- |
| #92 | 新命令的原信封登记/对账可用小记录和authority stub测试，handler可用消费端口测试 | Server不再持有具体Client，登记owner不持HTTP对象；减少传输/持久记录/业务调用混合，收益高 |
| #93 | journal写入/重开/并发原回执保护可在临时目录验证，停服OS观察可单独注入 | Console不实现文件持久化或硬编码observeExit；删除旧文件逻辑并修复跨窗口终态回退，收益中等、范围有限 |
| #94 | 修改结果解释、重试和下一输入构造可在纯值fixture验证 | development不需Service、SQL或生命周期句柄；原解析/纯规则只留一份，保持额外说明字段与围栏接受行为，收益高 |
| #95 | 最终交付核验可用窄读取/provider/声明/提交stub验证，并经原正式入口提交 | 用例不获取宽Config/CommandStore或worker控制，旧交付主体删除；任务规则、外部事实和唯一交付提交职责明确，收益中高 |
| #96 | 观察/句柄/在途调用、取消与确认权交接可在小端口/通道验证 | runtime不持RunView/apply业务闭包，actor不保留第二套可写资源map；资源写/关与确认owner单一，收益高但集成风险最高 |

## 共享修改与真实交接

| 关系 / 共享面 | 唯一修改归属与保持条件 | 可固定 / 开工 / 合入条件 |
| --- | --- | --- |
| 92 ↔ 93：wire/API、请求/回执、httpfs | 本轮共同契约只读，92只改server/app HTTPendpoint，93只改client/consoleapp。文件格式/ACL不变；各用自己的私有stub | Spec已固定保持边界后可各自实现/验证，谁后合入谁核对真实客户端/服务组合；不等待兄弟未交付新API |
| 94 ↔ 95：未知publish与真实merged、交付gate | 94负责staged_flow纯规则和原未知对账路径，95只替换最终交付用例；CompleteNativeTask原调用契约、DeliveryGate/ResultCommit保持。发布成功不是结项新门槛 | 可独立实现；后合入者运行TestDevelopmentUnknownPublicationMergedDelivery：94自建且始终验，95在基线实际含94时条件验。若必须共改model/签名，先重新核定交接，不以文件不同证明独立 |
| 94/95 ↔ 96：Service、Signal、资源收尾 | 94不改资源map/Signal，95不改worker控制，96只接运行时资源/结果；领域候选/commit、流程/恢复/PR规则冻结。成功ack留actor，入队前失败通知由runtime、入队后由actor，每道一次 | 不设置硬依赖。96始终验三个原stage入口与自建TestWorkerRuntimeDeliveredCleanup，实际含94/95时加未知合并/事务组合；94/95若后合入且基线含96则复验清理失败场景。准确命令归各Plan，真实需要未交付接口时才重新定义依赖 |
| 边界guard/文档/公共测试 | 每项新增自己的boundary测试文件；旧check.go策略不放宽，确需共同注册由后合入者一次整合。新私有fixture留owner，公共driver/runner冻结；每项更新自己的实际架构行 | 不另建只写接口/文档代管/最后统一测试任务。共有文件冲突按局部owner和合入次序处理，不伪造功能硬前置 |
| 真实资源与测试 | 普通owner/fixture每次独立临时目录；93独立simulate驱动生成SteerlineTest包；L3 controlled按已有串行runner | 代码开发可并行，原生/重型测试资源单独排期。不能以同仓worktree推定仓库锁、home、账号或终端已隔离 |

建议先准备92/93/94三条独立线，95在上述既有调用边界保持时可并行准备；96作为后续集成批次。这里的箭头仅表示默认合入次序，不是新增任务依赖。正式纳入时分别确认硬前置、开工条件、共享合入次序，不能把表中建议自动转成DAG边。

## 验收与交付安排

每项已有专属验收，自动项逐项映射到Plan中的场景/命令、环境、时点、通过条件和证据位置。新增测试/新包明确是实施步骤，当前不存在；实施时补齐、枚举并执行，零匹配/skip不通过。每项PR前一次固定L1，另按改动选择owner/真实app/受控事务与必要L3，不跑默认Full或付费全链。

本轮无新增独立人工签收活动：交互与平台行为保持，旧#91/#84/一期人工项继续归原记录。跨任务组合验证由相关后合入任务按自身Plan执行，分别登记到其专属验收，不共用一份多任务验收。当前仅规划不跑产品/模型验收；原始产物留新临时目录，长期只写最小证据摘要。

组合条件依据最终候选的实际基线commit及已包含成果，不仅看任务编号或草稿建议次序。条件未触发明确记录基线未包含该成果，不登记通过、不等待兄弟；触发但入口缺失/零匹配失败。固定L1不包含全部新反例，不能代替Plan列出的定向组合；同候选已执行的相同命令可引用证据，避免重复。2026-10-14审计修订已落实#93写入仲裁、#94解析双向基准、#96确认权交接以及各项条件组合，仍仅为待实施/待验规格。

以后正式纳入/执行时固定规格、实施、自查、独立复审及必要验证，真实合并核验后同步execution-map/acceptance，移除本页完成任务条目；目前执行图保持原状态。

## 保留候选与历史

存储/查询公共纯值、会话/final采集与执行控制、TUI Model刷新/输入状态保留为未编号候选，不纳入本次五项。会话采集待运行时实际边界成熟后重审；不为增加并行数提前创建空领域。

#91经 [PR #299](https://github.com/erranliu/SteerLine/pull/299)交付TUI/HTTP/Go与WinUI删除；[Spec](../../spec/tui-http-client.spec.md)、[Plan](../../spec/tui-http-client.plan.md)、[验收](../../acceptance/tui-http-client.md)保留，TH-M1剩余人工在正式清单登记。#45为已交付HTTP基线。

旧#43/#46–#52/#40/#41及#88–#90不恢复派发、不复用编号；旧重构的[收敛记录](bounded-refactoring.md)保留原范围/副作用执行器暂缓判断，本次96复用现有后台机制、只收拢资源，不恢复旧异步框架提案。WebUI、SSE、主Agent、远程HTTP部署未规划任务。历史见[归档说明](../../archive/roadmap/README.md)，一期独立待验继续归execution-map/acceptance。
