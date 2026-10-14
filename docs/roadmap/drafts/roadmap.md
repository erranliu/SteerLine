# SteerLine 当前 Roadmap 草稿

2026-10-14 按 [RoadmapGuide](../RoadmapGuide.md)选定本页。原五项均保留Spec/Plan与永久编号；#92/#93已确认纳入[execution-map](../execution-map.md)。#92经[PR #311](https://github.com/erranliu/SteerLine/pull/311)真实合并及验收同步后移出草稿任务表，HTTP命令登记/对账与消费端口成为已交付基线；#93经[PR #312](https://github.com/erranliu/SteerLine/pull/312)独立复审、验证和真实合并后也移出草稿任务表，journal/跨进程终态保护与窄observer成为已交付基线。#94/#95/#96因Worker三层重构的职责/接线影响继续留草稿，暂不安排实施；编号不复用。

调查基线 `5a24abec3c10d184a6e57962e154854bc6011a9f`。规划约束和生成规范按该版本 [RoadmapGuide](../RoadmapGuide.md)、[SpecGuide](../../spec/SpecGuide.md)、[并行指南](../../architecture/parallel-development.md)核对；实际启动另固定已提交Spec/Plan路径/commit，执行起点核对最新主干。任务草稿不维护实时状态，正式任务/依赖及已验/待验汇总归execution-map与专属验收。

## 本轮目标与限定范围

保留 `TUI → HTTP API → Go 服务`，提高 HTTP传输、固定开发流程、任务最终交付和worker运行时的独立修改/测试能力。保持一个业务actor、一个SQLite权威、原身份/请求/版本与唯一TaskDelivery原子提交。Codex/Orca adapter和Orca本机/environment路由复用，不增加第二业务/执行链。

本轮不改公共wire/模型/JSON持久格式、步骤/恢复/权限/交付语义，不建设通用命令/工作流/连接/调度框架。结构收敛之外唯一明确的小范围修复是#93客户端journal的跨窗口后续更新/终态保护，允许增加journal私有辅助锁文件；基线首次登记原子性不能代替这项修复。其余行为保持，如实际必须改变冻结契约，先重定范围/所有者与真实前置，不能借重构扩大行为。结构完成要求旧重复逻辑/状态删除、小端口可独立验证，而不只是文件移动或宽对象换接口。

## 草稿任务

| 任务 ID | 任务名称 | 成果 / 主要owner | 硬前置与并行/集成安排 | Spec / Plan | 专属验收 |
| --- | --- | --- | --- | --- | --- |
| 94 | 固定开发流程规则独立 | development/v3纯规则包、原actor规则接线 | 仅保留草稿，暂不纳入；Workflow规则与正在实施的三层重构直接重叠，待成果合入后重审 | [Spec](../../spec/development-flow-rules.spec.md) · [Plan](../../spec/development-flow-rules.plan.md) | [验收](../../acceptance/development-flow-rules.md)：DF94-A1–A4，纯规则/事务/双后端controlled，无新增人工 |
| 95 | 任务最终交付用例独立 | delivery窄端口用例、CompleteNativeTask薄接线，保留原PR绑定 | 仅保留草稿，暂不纳入；交付抽取有剩余收益，actor/PR桥接与记录映射正在重组，待稳定后重审 | [Spec](../../spec/task-delivery-usecase.spec.md) · [Plan](../../spec/task-delivery-usecase.plan.md) | [验收](../../acceptance/task-delivery-usecase.md)：TD95-A1–A4，用例/原子事务/两条L3 controlled，无新增人工 |
| 96 | worker 运行时资源所有权收敛 | workerruntime观察/句柄/在途资源、actor结果接线；不重写业务恢复 | 仅保留草稿，暂不纳入；生命周期effect/资源/确认接线与三层重构交叉，不并行建立旧runtime边界 | [Spec](../../spec/worker-runtime-ownership.spec.md) · [Plan](../../spec/worker-runtime-ownership.plan.md) | [验收](../../acceptance/worker-runtime-ownership.md)：WR96-A1–A4，owner/确认屏障/四条L3 controlled，无新增人工 |

已有#93/#92/#91/#45、任务规则/窄端口、后台效果、恢复事实和提交屏障是已交付基线，不重新创建“契约基础任务”。本批#92/#93均实现完成，实际执行/验收状态归execution-map；成果及独立复审/合并分别见各自专属验收。#94/#95/#96的原独立交付判断须结合三层重构成果重新核验，编号和文档存在不产生开工授权。

## 原拆分的收益与独立交付判据

独立交付要求在兄弟任务均未实施的调查基线上也能完成真实调用接线、删除旧主体/重复状态并通过自己的自动验收，不以只有新包/interface或stub通过交付。兄弟实际已合入时补最小组合验证。#92/#93是同包职责与能力隔离，不宣称包级编译隔离。下表保留原收益评估；#94/#95/#96不因该评估自动纳入，须按后面的冲突核对重新定范围。

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
| 94/95：未知publish与真实merged、交付gate | 原规则不新增成功发布结项门槛；三层重构正调整Workflow/actor/PR桥接，旧文件所有权不再作为并行依据 | 两项均只留草稿，原测试/交接供重审参考；#94不并行另建development owner，#95待真实交付入口稳定后核验剩余抽取范围 |
| 94/95/96：Service、Signal、资源收尾 | 三层Spec将技术可靠性归ReliableExecution、资源/effect/提交归actor组装；#96旧runtime与这一owner划分交叉 | 不同时实施旧runtime和三层边界。确认/清理反例可作为后续需求核对，不能直接把三层验收写为旧任务已验 |
| 边界guard/文档/公共测试 | 每项新增自己的boundary测试文件；旧check.go策略不放宽，确需共同注册由后合入者一次整合。新私有fixture留owner，公共driver/runner冻结；每项更新自己的实际架构行 | 不另建只写接口/文档代管/最后统一测试任务。共有文件冲突按局部owner和合入次序处理，不伪造功能硬前置 |
| 真实资源与测试 | 普通owner/fixture每次独立临时目录；93独立simulate驱动生成SteerlineTest包；L3 controlled按已有串行runner | 代码开发可并行，原生/重型测试资源单独排期。不能以同仓worktree推定仓库锁、home、账号或终端已隔离 |

当前执行批次只有#92/#93，二者可独立并行；与三层重构共享app文件时，#92只改HTTP endpoint构造段，三层owner保留worker/adapter与记录转换，后合入者复验实际app接线。#93只改客户端。三层保持公开HTTP/DTO的约定成立时无新增硬前置；实际公共契约改变须重新核对，不把文件局部冲突误记成已交付接口。#94/#95/#96暂不纳入。

## 与Worker三层重构的冲突核对

2026-10-14用户告知Worker三层职责重构已在实施。本轮只读取另一worktree的Spec/Plan，不修改其文档、源码或执行状态。参考路径为 `C:\Users\lmas1\.codex\worktrees\0998\SteerLine\docs\spec\worker-three-layer.spec.md` 与同目录Plan；其调查代码基线同为 `5a24abec3c10d184a6e57962e154854bc6011a9f`。读取时Spec SHA256为 `f7f63f9567047163f90847dd8243fb2bdff399f3c1db971aac9f7397950e688e`，Plan为 `4a7a5fde870408ff9fd3fcd093d4d87e7664ea200bfa3b5a5d2c18ef23b73c47`；文档头仍写设计待实施，实际在途事实按用户告知处理，本文不据该头认定已完成或尚未开工。跨worktree未提交材料只作本次范围核对，不作为新执行任务的固定规格。

| 候选 | 三层Spec/Plan交叉点 | 当前安排 |
| --- | --- | --- |
| #94 | Workflow拥有development/v3步骤、提示词、业务JSON、report source与后继；P3直接搬迁staged_flow/staged_result，拟议workflow与旧development是同一职责的不同owner | 保留草稿、不纳入；三层合入后看剩余规则是否仍值得独立重构，不同时建立两套纯流程包 |
| #96 | ReliableExecution拥有原操作对账/原身份/恢复与结果资格，actor组装队列/资源；P2/P4及lifecycle_effect/receipt、worker生命周期和确认交接与旧workerruntime提取交叉 | 保留草稿、不纳入；不要求三层实施者消费#96旧接口或按旧owner分法实现 |
| #95 | 三层明确PR平台与TaskDelivery不进入可靠执行层，因此最终交付抽取未被取代；但P3/P4重新组织actor/PR桥接、workerFlowContext与记录映射，#95当前冻结接线/fixture条件不能直接沿用 | 保留草稿、不纳入；真实桥接稳定后重新核定delivery剩余收益与Spec/Plan，区别于#94/#96的直接重叠 |
| #92/#93 | 三层保持公开HTTP/DTO，改动集中执行内部；#92仅共用app文件的HTTP endpoint段，#93为纯客户端 | 已确认进入execution-map，可并行，局部组装冲突由后合入者保留双方成果并执行各自最小组合验证 |

五项均保留原身份和文档。对三层的确认/终态清理等建议只作为重审参考，不修改在途规格、不另建前置任务、不把旧任务或三层验收自动标通过。后续成果进入主干再确认#94/#95/#96的剩余范围和开工资格。

## 验收与交付安排

每项已有专属验收，自动项逐项映射到Plan中的场景/命令、环境、时点、通过条件和证据位置。新增测试/新包明确是实施步骤，当前不存在；实施时补齐、枚举并执行，零匹配/skip不通过。每项PR前一次固定L1，另按改动选择owner/真实app/受控事务与必要L3，不跑默认Full或付费全链。

本轮无新增独立人工签收活动：交互与平台行为保持，旧#91/#84/一期人工项继续归原记录。跨任务组合验证由相关后合入任务按自身Plan执行，分别登记到其专属验收，不共用一份多任务验收。当前仅规划不跑产品/模型验收；原始产物留新临时目录，长期只写最小证据摘要。#92/#93已交付，自动验收及限制已在execution-map和各自专属记录登记；其余三项仍未实施/未验，不安排旧组合场景执行。

组合条件依据最终候选的实际基线commit及已包含成果，不仅看任务编号或草稿建议次序。条件未触发明确记录基线未包含该成果，不登记通过、不等待兄弟；触发但入口缺失/零匹配失败。固定L1不包含全部新反例，不能代替Plan列出的定向组合；同候选已执行的相同命令可引用证据，避免重复。2026-10-14审计修订已落实#93写入仲裁、#94解析双向基准、#96确认权交接以及各项条件组合，其中#92/#93现已交付，其他草稿仍为待实施/待验规格。

#92/#93实际启动时固定已提交Spec/Plan commit及路径，核对最新主干与在途修改；实现、自查、独立复审、必要验证和真实合并核验后同步execution-map/acceptance，再移除本页完成条目。其余三项留草稿，后续另行确认范围。

## 保留候选与历史

存储/查询公共纯值、会话/final采集与执行控制、TUI Model刷新/输入状态保留为未编号候选，不纳入当前执行批次。会话采集待运行时实际边界成熟后重审；不为增加并行数提前创建空领域。

#91经 [PR #299](https://github.com/erranliu/SteerLine/pull/299)交付TUI/HTTP/Go与WinUI删除；[Spec](../../spec/tui-http-client.spec.md)、[Plan](../../spec/tui-http-client.plan.md)、[验收](../../acceptance/tui-http-client.md)保留，TH-M1剩余人工在正式清单登记。#45为已交付HTTP基线。

旧#43/#46–#52/#40/#41及#88–#90不恢复派发、不复用编号；旧重构的[收敛记录](bounded-refactoring.md)保留原范围/副作用执行器暂缓判断，#96原提案复用后台机制、未实施；因三层重构交叉暂留草稿，不恢复旧异步框架提案。WebUI、SSE、主Agent、远程HTTP部署未规划任务。历史见[归档说明](../../archive/roadmap/README.md)，一期独立待验继续归execution-map/acceptance。
