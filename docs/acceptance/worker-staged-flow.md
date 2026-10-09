# Worker 阶段流程验收规划

任务 ID：84；任务名称：Worker 分步执行与结构化状态推进；目标：`WORKER-STAGED-FLOW`。以 [Spec](../spec/worker-staged-flow.spec.md) 为流程/提示词唯一正文，实施及测试选择见 [Plan](../spec/worker-staged-flow.plan.md)，一期顺序见 [roadmap 草稿](../roadmap/drafts/roadmap.md)。已由用户确认纳入 [execution-map](../roadmap/execution-map.md)，实现与验证执行中，最终精确候选的产品自动/人工结果待登记；已完成的文档静态检查不证明产品能力。#83 已合入，作为隔离基线复用。

## 最小验证安排

实施时复用当前规则、提交、adapter 和 UI 入口。以下测试名是拟新增用例，必须实际实现并确认非零匹配后才执行；不是已存在的运行能力。使用隔离 exe/home/clone 与临时日志，不操作现有 SteerlineRuntime 任务或用户终端。

| 项 / 级别 | 场景与环境 / 命令 | 时点与通过条件 | 状态 / 证据 |
| --- | --- | --- | --- |
| WFS-DOC：文档静态 | 导航、身份、差异；`python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --check` | #84 身份、独立 Spec/Plan 及专属验收一致；旧流程正文转为 Spec 不重复，主 roadmap 收尾顺序明确；流程/步骤两层和三个短模板保持 | 本轮静态结果见下文；产品能力仅规划 |
| WFS-A1：L1 与纯规则/提交 | 拟新增 `TestWorkerStageTransitions`、`TestWorkerStageResultBinding`、`TestWorkerStageReportRound`；`go run ./cmd/test-feedback -level=L1 -- ./internal/execution -run '^TestWorkerStage' -count=1` | 有效结果经业务核验原子改变 status/step_id/投递意图；开发流程正常三轮；无事实证明的 done 不推进，unknown 只对账，失败/格式/缺能力保持阻断；重试/旧来源不重复推进 | 待验收；runner 默认临时目录 |
| WFS-A2：结构化输入与 UI | 拟新增 `TestWorkerStagePresentation`；`go run ./cmd/test-feedback -- ./internal/console -run 'TestWorkerStage\|TestWaitingPR' -count=1`，输入冻结测试归 A1 | 步骤位置与 status 同快照，三段短模板/结果契约固定且不随 backend 分叉；内部身份由程序绑定，源失败不被报告成功覆盖，publish 无虚构 report、待合并无 Continue | 待验收；临时输入快照及报告 |
| WFS-A3：双 adapter controlled 接线 | 拟新增 `TestWorkerStageLocalRoundTrip`、`TestWorkerStageOrcaRoundTrip`；`go run ./cmd/test-feedback -- ./tests/integration -run '^TestWorkerStage' -count=1`，串行隔离 | 同一开发流程经两个现有 adapter 正常三轮；规范 final 来源/原生 purpose 映射、能力不足阻断、步骤边界重开和未知对账一致。复用既有 fixture 及少量端口替身断言，不实现第三 adapter 或另一套模拟框架 | 待验收；原生 fixture 回执与 SQLite/输入结果定位 |
| WFS-A4：双本机 live 抽样 | 本地 Codex、本机 Orca 各一条；复用 L3 delivery.local-codex / delivery.orca-codex；`go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.local-codex,delivery.orca-codex'`，沿 Plan 更新材料/断言 | 真实 implement 两轮和 publish 单轮、产物/PR 身份及真实 usage；实现报告无业务写入，工具限制能力据实记录。重开/未知分支主要归 A3，不强制在每条 live 链重复故障矩阵 | 待验收；实际构建、配置及原生准备待固定；临时现场证据 |
| WFS-M1：人工审阅/合并 | A4 的隔离现场；参与人/窗口待安排 | 发布核验后辨认阶段/阻断，人工合并后核验唯一交付与正确后继资格 | 待验收；可后置至组合验收，不进执行 DAG |

权限失败分支归 A3 的定向受控测试：原发布命令需要审核时沿原请求处理；拒绝/不可用保持 blocked，不换工具绕过。A4 据实记录实际触发的审核与工具约束，未触发不记该分支通过；发现真实缺口保留待验，不强制增加完整权限故障矩阵。

实现报告失败覆盖放在 A1/A3：缺证据不补成功，报告格式错/原生失败保留源执行，report 重试不增加实现次数；发送未知不盲重投，迟到结果/事实变化不推进。publish 单轮覆盖推送成功但 PR 未完成、JSON 错误/缺失与结果未知：保留部分成果，只读对账，不新建报告轮或为补格式重推送。报告工具限制不足时单独记录保证缺口，不把只读提示词算作权限验收通过。

边界验证复用原依赖守卫与同组测试：状态规则不得导入具体 adapter；adapter 不取得中心写权或业务转移权；提示词渲染/业务 JSON 解析不在各后端复制。用少量端口替身断言 worker 只消费规范事实，按实际改动选择既有 `internal/testkit/boundary` 定向入口，在实施 Plan 固定。第一版不以完整演示流程或第三 adapter 作为扩展性验收门槛；实际增加流程/后端时再补对应验证。

本次不默认跑 Full、完整 L2 或任意流程组合；选定规则、双 adapter controlled 和各一条 live 即可覆盖本次边界。实际实现新增风险时由 Plan 增选相关组。缺失 final 来源、身份/权限未知、真实失败和明确前置不能用人工后置豁免。实施 Plan 仍需列明所选级别/组/场景、环境、命令、时点、通过条件与证据位置。

#84 自动能力和本次最小验证完成后交接给[一期活动](phase1.md)：一期安装基线须包含 #83/#84 成果，再开展组合链、24 小时和同构建恢复。WFS-M1 可与 P1A1-M 同一现场集中安排，分别记录各自结论；#84 不承担整场一期验收，也不为人工活动新增 DAG 节点。此安排只约束新一期签收，不以旧运行/旧短时测试证明新流程通过。

## 已验与待验记录

本次只修改文档，未改变运行程序、数据库、DAG 或已冻结任务。历史 101/102 的问题只作为设计输入，不是新流程的验收通过证据。

2026-10-08 原版静态记录：259 个 Markdown、1573 个本地链接，零错误；差异通过；当时六份模板的七个变量匹配。该数量属于精简前记录，不代表当前仍有六份模板。原日志：`C:/Users/lmas1/AppData/Local/Temp/steerline-worker-flow-docs-20261008-234d4eca1c93438eb57f2919cd006526/`。

该次全量编号检查因既有编号 49 不一致失败：工作树文件和 HEAD 为“Roadmap 与 DAG 原生浏览”，共享 Git ref 为“DAG 与任务原生浏览”。本目标未修改该身份或共享 ref，不把总检查记为通过。

2026-10-09 精简结果：主题文档从 10 份收敛为 2 份，六份策略模板改为两个主阶段短提示词和结构化附加输入；系统补齐结果身份。251 个 Markdown、1556 个本地链接，零错误；差异检查和本地 11 条草稿/0 条正式行的身份检查通过。已核对无旧模板/设计文件链接，execution-map 和任务编号文件未改动。全量编号检查仍报上述既有问题。日志：`C:/Users/lmas1/AppData/Local/Temp/steerline-worker-flow-compact-20261009-376e49e5729c46d586727e61589c403c/`。

2026-10-09 两轮调整：仍为两份主题文档，两个执行短提示词不要求结构化结果，另有一个通用报告短提示词。静态核对执行失败、报告失败/缺失、只重试报告及重开只补报告的规则，并移除旧的一轮式输出要求。251 个 Markdown、1556 个本地链接，零错误；差异及本地身份检查通过，全量编号仍有原 49 问题。日志：`C:/Users/lmas1/AppData/Local/Temp/steerline-worker-flow-two-round-20261009-28c0e0dfd3e84bd0b51fd3498836c83a/`。这些静态检查不证明运行能力或只读隔离已经实现。

2026-10-09 后续收窄：仅 implement 分执行/报告轮，publish 单轮发布并结构化收尾；前一段为被收窄方案的历史记录。静态核对 publish 不调度 report、不为补 JSON 重做发布，正常三次业务轮次及来源/恢复判据一致。主题文档仍为 2 份，251 个 Markdown、1556 个链接零错误，差异/本地身份检查通过；全量编号仍有原 49 问题。日志：`C:/Users/lmas1/AppData/Local/Temp/steerline-worker-flow-publish-single-20261009-e3ca4a6bcdfd4adca9709345d6f31630/`。

2026-10-09 通用机制确认：补齐统一状态集合、结构化结果到事件/转移/动作的规则、内核/流程/adapter 边界，以及源码能力与改造表。两份主题文档、三个短提示词保持；251 个 Markdown、1570 个链接零错误，差异及本地 11 条草稿/0 条正式行检查通过。未调整实现代码、execution-map 或编号；全量编号检查仍有原 49 问题。日志：`C:/Users/lmas1/AppData/Local/Temp/steerline-worker-flow-mechanism-20261009-3a8488d6669846fdbbd1f82cba4c3448/`。

2026-10-09 架构收敛：改为现有 worker 内的流程/步骤两层及普通转移函数，三个短模板和状态边界保持；验收删除额外演示流程及第三 adapter 的完整模拟要求，复用双后端 fixture 和少量端口断言。两份主题文档及两处导航同步。251 个 Markdown、1570 个本地链接零错误；差异、本地 11 条草稿/0 条正式行身份及设计一致性检查通过；全量编号仍报既有 49 与共享 ref 不一致。仅文档调整，未执行产品/模型测试，未修改实现、任务编号或 execution-map。日志：`C:/Users/lmas1/AppData/Local/Temp/steerline-worker-flow-simple-28bb82f584b247d3a5e9627d1832b2c2/`。

2026-10-09 一期收尾规划：以共享计数器预留 #84，加入主 roadmap 的 #83 后集成、一期活动前位置；旧流程正文转为独立 Spec，仅新增配套 Plan，主题正文现在为 Spec/Plan/本验收三份。Plan 列明最小规则、UI、双 controlled 和双 live 选择，阶段人工活动仍独立。分配前按当前 HEAD/文档核对并用原 rename/CAS 机制修正共享 ref 的 #49 名称，ID 未变；新编号未绕过共享计数器。静态检查通过：252 个 Markdown、1592 个本地链接零错误；85 个身份文件、12 条草稿/0 条正式行，next ID 为 85；差异无错误。原“49 不一致”失败属于前述历史时点，本次已修正。未改变运行代码/安装程序、execution-map 或外部 Task/DAG，未执行产品/模型测试。证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-worker-roadmap-9ce943d987574bb0a2f114543d628755/`，含修改前共享 ref 和本次静态输出。

2026-10-09 Rebase 同步：基线更新为 `origin/main` 的 `692a784e05479b11e5818fb5a209971949e3064a`，已包含 #83 的 PR #247 及收尾记录。#83 不重新加入草稿待执行表；#84 以已交付隔离能力为基线，保留在新一期收尾草稿，未纳入 execution-map，不改主干 #45→#44 的已确认安排。Spec/Plan 同步当前 Codex 开发流程和原会话恢复限制；原共享 #49 名称按新主干的“DAG 与任务原生浏览”对齐，#84 身份及编号保持。文档静态检查通过：269 个 Markdown、1724 个本地链接零错误，85 个身份文件、10 条草稿/3 条正式行，差异无错误。现有修复的 L1/定向回归归[独立修复记录](waiting-pr-feedback.md)，不作为尚未实现的 #84 验收通过。Rebase 及静态证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-worker-rebase-183c5bbd0c944ab39372b1e44beb334d/`。

2026-10-09 纳入执行规划：用户确认 #84 进入 execution-map，沿用原 ID/名称和独立 Spec/Plan，状态为待执行；无未完成硬前置。剩余顺序为 #84→#45→#44，共同接线顺序集成，#44 依赖 #45 保持；WFS-M1 可后置至一期组合验收，人工窗口待安排。Spec/Plan、草稿及导航已同步纳入状态，未启动实现、未创建外部产品 Task/DAG，产品自动/人工项仍待验收。静态检查通过：269 个 Markdown、1732 个本地链接零错误，85 个身份文件、10 条草稿/4 条正式行；差异、#84 成员/状态及 Plan 验收映射核对通过。证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-worker84-admission-a467151c855c47d1a728356d363286e8/`。以上只证明文档纳入与一致性，不是产品功能通过。

后续结果记录实际源码/二进制/流程摘要、环境、原步骤/输入/PR 绑定与证据定位；实际启动或开始验收后，按真实 PR/验证结果同步 execution-map。产品测试中的原 Task/DAG/交付回执仍按真实接口核对。

## 2026-10-09 开发启动与当前验证

固定契约：`c4bcbec06289c9f5b086c8f1f98859a0f858d7e5` 内 worker-staged-flow.spec.md / worker-staged-flow.plan.md；代码起点同 SHA。原工作树 b123 干净，远端 main 同 SHA，分支 codex/task-84-worker-staged-flow。用户指定开发模型 gpt-6.1-sol/high，仅覆盖执行聊天；产品 live 使用原 gpt-6-luna/medium 及固定原生权限。草稿选择 docs/roadmap/drafts/roadmap.md，不修改固定 Spec/Plan。

开发期 dirty tree 的 A1 规则/提交用例、console 定向用例，以及双 adapter controlled 正常三轮/重开检查已实际运行；这些是开发诊断，最终 clean candidate 验收另行登记。首次 L1 +定向 A1 通过，原始报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T025901.138445000Z/summary.json`。此前三轮受控用例暴露的未提交原生操作误走对账问题已修复；失败输出保留在原聊天，不能算为最终通过。WFS-A4 两条 live 尚未执行，WFS-M1 保持待验。

### e74 精确候选与双 live 首轮

源码 `e74f1ec881661064e5b616e42e6fa06cec923d5d`，tree `67da2fb04605fddc07ec99bbcaee2b571f91f973`，clean tree。L1 + A1：475 tests / 0 skip / 13 groups，报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T033430.204647100Z/summary.json`；A2：18 tests / 0 skip，报告 `.../20261009T033431.183657300Z/summary.json`；A3：双正常三轮及 Orca 未知报告/重开共 3 tests / 0 skip，报告 `.../20261009T033431.009393200Z/summary.json`。这些结果仅对应 e74，不冒充后续修改的通过。

WFS-A4 首轮目录 `C:/Users/lmas1/AppData/Local/Temp/steerline-worker84-evidence-a214d3a014c74100a2ad8d2b933199cb/live-e74/`：local-codex 真实三轮、唯一交付、原生 usage 与清理通过（163.52 秒）；orca-codex 失败，汇总 passed=false。两场景 scenario.json 均 cleanup_complete=true。Orca 后续 Dispatch 的精确 worker-read 返回 session_not_reported，原生 rollout 实际含正确 Task 头、final_answer 与完成；旧接线未保存准备时的根 transcript，故拒绝结果并耗尽一次报告重试。这是自动验收失败，不归人工后置。原始记录、规范快照、保存的 rollout 和 archived-read 诊断保留；local 结果不覆盖 Orca 失败。usage 文件是各绑定轮次的原生累计快照，不相加计费。

协调通知 PR #254 已进入 main `d8d66d6c5696f41d7df1ed3ff50d4b3ae77b9da3`。原 worker 在 live 已退出且清理核实后，用 merge commit `bf64c1429782903a0eceae4831e26571950b889d` 集成，保留 e74 测试来源；console 操作进度/原回执修复与步骤投影均保留，无冲突。固定 Spec/Plan 仍在 c4bcbec。后续候选将修复原来源冻结、复验 affected console / controlled / 双 live，未将本段失败写成通过。

### 新主干原生 Orca 接线与明确验收调整

历史 clean 候选 `a204a8ce9831c96eb0f42c0720958f3e27e64e01` 的 L1+A1 478 tests / 0 skip、console 21 tests / 0 skip、Orca controlled 2 tests / 0 skip 通过。双 live `live-a204/summary.json` passed=false：local 209.01 秒通过，Orca 420.62 秒失败，根因是实际 native msg ID 不能当 transcript 文件路径；两场景 cleanup_complete=true。旧失败不由后续受控通过覆盖。证据根沿用本页前述 worker84-evidence 目录。

旧受管 launcher 的 public-history 修复已保存于 `e04c3786`；该来源的受控正常三轮通过，但未在该候选补跑 live。随后集成主干 `ade7042bef9d858ae7cc4cdcaae957704c407b8f`（含 PR #255–#261），merge `38653b5c944dcab15403cb0f885eb01c70bcb7a3`、tree `515464ba20c77c40908eb25a75933b4eb541cd0b`。主干已删除旧 Orca launcher/readiness/relay，当前消费者改用主干原生 output material + original Dispatch settlement。旧 history/launcher 证据只证明旧 source；未恢复已删除框架。新三轮 live 仍待执行并登记。

2026-10-09 本聊天向用户明确说明：固定 Plan 要求 Orca 逐轮原生 usage，而新原生 opaque agent 的公开接口不提供 usage。用户回复“调整 Orca usage 验收，保留真实闭环要求”。此授权仅改变 WFS-A4 的 Orca usage 条件：登记 not_exposed_by_orca 及每步原 Command/Dispatch/final source，不能声称 token 核验通过；仍须真实三轮、原身份、候选/PR head、唯一交付及精确清理。Local Codex 的逐轮原生 usage 及无额外调用检查保持。固定 Spec/Plan 的 commit/path 未修改，原文与本段授权分别可追溯。

当前保留原生 Orca opaque identity、原生权限/资源/恢复/停止/清理和远端目录注册。worker-flow-v2 要求规范 final 能力；远端 Orca 当前未声明此能力，因此在准备工作区/模型输入前显式 unsupported。本任务不新增远端流程或 live，也不借本机证明远端支持。console 原生 PR 链接与步骤/status 同快照并存。
新版最小验证：clean `38653b5c` 的 L1+A1 490 tests / 0 skip / 13 groups（20261009T064532.351292900Z）；console 步骤/等待PR/操作进度回执/PR链接 23 tests / 0 skip（20261009T064502.288447500Z）；规范输出及模型选择定向 21 tests / 0 skip（20261009T064502.915077600Z）。报告均在 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/`。新版 controlled 的 local 和 Orca 正常三轮分别通过；同轮 unknown 用例的原先一次性丢回执被原生缓存立即对账，旧等待断言失败，未把失败记通过。

将 unknown 故障保持至原服务退出后，真正揭示 BeginTurn 拒收 reconciling 下原输入开始事实的缺口。规则修复仅允许匹配原已接受 pending turn 的开始观察，不授予新发送资格。定向规则 1 test / 0 skip（20261009T065323.802469800Z）及实际 Orca 丢回执/重开场景 1 test / 0 skip（20261009T065326.171509600Z，64.979 秒）通过：原服务未发布，恢复原两轮结果、一个 Attempt、三个冻结输入，publish 保持 waiting_input，native Dispatch 仍恰好两次。原失败记录 20261009T065106.138101900Z 保留。新 native launch 证据断言 6 tests / 0 skip（20261009T064831.408077100Z）核验实际保存的首轮原 Task/Dispatch/agent/model/effort，任一身份/选择变化拒绝。这些增量测试在 `38653b5c` 加对应明确修改的工作区执行；下一 clean 源码/构建及 live 报告另行登记，不能把本段称为该后续完整矩阵。