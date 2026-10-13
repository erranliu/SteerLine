# 核心断连恢复验收

日期：2026-10-12。归属：一个普通开发范围“Codex / Orca 核心断连恢复”。2026-10-12 基线的七项自动验收已完成，2026-10-13 主干整合后按受影响范围复验；无 Task ID，不进入 execution-map，未部署 Runtime。

[Spec](../spec/worker-disconnect-recovery.spec.md) · [Plan / 测试计划](../spec/worker-disconnect-recovery.plan.md#测试计划) · [L3 场景及命令](../../tests/e2e/suites/recovery/README.md)

## 范围变更

本轮审计替代原全面草案 DR-A01–DR-A12 与六条 controlled/三条 live 规划。原项未实施/注册/执行，无通过证据可继承。当前自动验收清单为六项功能 CR-A01–CR-A06 加一项结构 CR-A07；结构项复用已有测试及三组受控链；受控入口依 5 分钟上限拆分，覆盖范围未扩大。延后事项不列成当前待验阻塞。

核心场景为原观察重接、原轮次明确失败退出 reconciling、Codex 死亡进程后的同 thread 恢复及通过新 implement 核验人工成果。Orca 仅原终端重接/结果重放，lost terminal 明确阻断。完整远端真实链、资源重建、问答/发布全窗口、硬件断电不属于本轮保证。

## 自动验收项

执行前固定实际 source commit/dirty/tree、exe/CLI 版本、目标与 command。执行后逐项填写日期、分支、通过/失败原因、evidence 路径和清理；下表不是执行结果。

| ID | 最小通过条件 | 测试映射 | 状态 |
| --- | --- | --- | --- |
| CR-A01 | 中心/观察断连接回原绑定，活动输入不重发；完整原成功结果只补记一次；原轮次结束不显示为任务成功 | C1、V1、V2 | 已通过（见下节证据） |
| CR-A02 | implement/report 明确失败或中断，无未知操作且工作区可核实时，缺 final 不永久 reconciling；无旧 Decision 也有继续入口；成功缺 final/未知输入仍未核实，同事实不换版本 | C1 | 已通过（见下节证据） |
| CR-A03 | Codex 原 thread/cwd 实际公开可读，原受管树/旧写者已停止后同 Execution/worktree/thread 新 Attempt/generation；会话缺失、写者未停止或冻结身份不符拒绝；不 thread/start 替代 | C2、C3、V1 | 已通过（见下节证据） |
| CR-A04 | 仅一个 continue 入口；失败 implement/source缺失或变化回到新 implement 核验，生成新 report source，旧失败不覆盖、人工 final 不充当旧结果；主干 development/v3 下，仅未发送首次 report 可沿用原 source，已发送失败报告重新实施 | C2、V1 | 已通过（见下节证据） |
| CR-A05 | 原操作未知、原会话人工轮次仍活动/空闲不可核实、旧授权/错误 target/资源代次拒绝；重复 answer、一次已接受但回执未知后的重启、结果已保存后的重启不重复发送/结算；Orca local/environment 原请求与 native pane/provider/状态基线一致，丢失 terminal/source gap 不替代、不补 Enter | C1、C3、V2 | 已通过（见下节证据） |
| CR-A06 | 控制台 u 正常与错误路径均有界返回，无 panic、无新 Command/Attempt/控制状态写；报告显示原因和继续资格，正式 answer 才执行恢复 | C1 | 已通过（见下节证据）；历史小修复另记 |
| CR-A07 | 恢复规则归口 worker，观察/资格/反馈入口共享身份、未知操作、分类与版本/Decision 规则；两 adapter 只提供原生证据/动作、不持有中心写权限、不交叉调用恢复；原资源复用不依赖 ManagedProcess 真值；只读核验不要求运行态资源；无新恢复框架与残留重复路径 | Plan B1 定向边界/fixture、C1–C3 断言及源码审查 | 已通过（见下节证据） |

场景与通过条件详见 [L3 规划](../../tests/e2e/suites/recovery/README.md)。新增 L3 的正常交付需保持未合并不释放后继、合并后唯一 Delivery；发布超时的新增强不作为 CR 项。相同后端规则经 environment fake 不代表远端 live 已通过。

## 环境与证据

- C1–C3：现有独立 E2E home/Git/SQLite，正式二进制与恢复 answer；当前 Codex 协议 helper、Orca 单终端 local/environment fake。只补原生身份/轮次、一次断读/丢回执、owned kill、计数和 u 输入驱动，不写业务数据库。
- V1：真实本地 Codex，与本次独立 thread/workspace；公开读取和同 thread resume 必须真实通过，最终正文来源与正数 usage 留证。
- V2：真实本机 Orca 当前单终端，独立 owned terminal；原请求接受/开始、native working→done、完整普通回复及空 draft 留证。未公开 usage 则记录 unavailable。
- PR 仅 erranliu/SteerlineTest 的本次专属 base/head。模型、临时故障和清理失败如实失败；skip/零匹配不算通过。安装现场和任务 102 不作夹具。

每个场景保存 summary.json、scenario.json、必要分支 result.json、before/after 身份/版本/状态、正式 Decision/Command/Operation、新旧 Attempt、原生业务发送计数、final/source/snapshot、Git/PR/Delivery 及 owned cleanup。敏感认证、私有会话数据和完整过程正文不入仓库。逐项登记实际 evidence 路径。

## 后续验证边界

真实远端断连、Orca runtime/terminal 丢失后同会话重建、首次准备和原生答案/发布的全部回执窗口、真实机器重启/断电另行评估，不预占本轮验收 ID 或任务。实际硬件验收需用户/管理员在独立测试主机参与，当前无执行窗口；受控进程 kill 不证明磁盘断电耐久性。未验前不得声称完整断电恢复保证。

## 审计静态检查

2026-10-12 首次范围收敛：六项功能验收与五条规划场景的映射检查通过。

2026-10-12 按用户要求补 worker 归口/两路径解耦：文档导航检查通过（338 份 Markdown、2035 个本地导航链接、0 错误）；四份范围文档的相对路径/章节锚点检查通过，六项功能加一项结构验收均映射到 Plan，五条新增场景无登记冲突且未提前注册；B1 引用的四个既有测试入口存在，git diff --check 通过。没有执行 Go/L3，入口存在不等于本次结构已验收。

审计阶段仅执行静态检查，产品项当时未执行；当前实施结果见下节。先前 u 修复保持原状，没有启动/修改安装现场或恢复任务 102。

## 相关历史

[只读核验崩溃修复](recovery-inspection-crash-2026-10-10.md)只证明该小修复；[旧 worker 恢复](worker-stop-recovery.md)与[Orca 单终端](orca-terminal-transport.md)保留其原证据边界，不扩大为本轮通过。

## 实施与验证进展（2026-10-12）

源码基线 6d4b4fa2d67b77156eeda64d103ce71c95f3450b，dirty；保留先前 u 修复。Codex CLI 0.159.2。没有修改或部署 C:/SteerlineRuntime，没有用任务 102 作夹具。Orca 原生操作使用当前 orca-cli 指引，真实测试尚待受控门槛通过。

- 核心 worker/两 adapter 定向用例通过：20261012T040658.698375400Z/summary.json，位于系统临时 steerline-test-feedback；覆盖通用资源资格、失败 report 缺 final、成功缺 final、人工活动 turn、Orca draft/身份拒绝。
- 既有 PR 合并门槛与只读装配回归通过：20261012T040253.399192700Z/summary.json。
- 受控 observation-reconnect 在 20261012T041503.009659600Z/e2e-1.jsonl 中通过，215.39 秒；该轮另一场景失败，不能将顶层 summary 标为全部通过。
- 受控测试发现并修复：可信原生成功不能带入观察 EOF 为业务 Failure；每个 Command 都要写历史绑定；CrashService 后 readiness 须核对新 owned PID，不能接受旧 ready checkpoint。正在重新验证。
- 架构检查发现既有 workerreturn 纯契约未纳入允许/纯依赖分类，已补纯依赖检查和负例；之前失败报告 20261012T035429.966234700Z/summary.json，待修复后复验。
- 受控入口按实测拆分为 11 个（仍三组）以保持每入口 5 分钟；live 2 个已实现/注册，尚未执行。没有跳过必需分支或扩大超时。

实时结果后续在此节追加并更新 CR 各项；上述定向通过与单场景通过不等于全部交付。

## 收敛实施的已验结果

证据根为 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/`。下面登记独立场景的实际通过；包含其他失败场景的报告明确保留该限制，不将整份 summary 算为通过。

| 范围 | 通过证据 | 结论 |
| --- | --- | --- |
| 固定 L1 | `20261012T102826.170076700Z/summary.json` | 最终 worker 结算修正后完整通过；首次 L1 因无测试文件的 protocol 夹具包被误选而不完整，已修正固定清单并复验 |
| B1 架构与通用实现、冻结配置、runner 选择 | `20261012T075359.734722100Z/summary.json` | 全部通过；workerreturn 纯依赖闭包及负例、非进程资源复用、冻结 home/command/身份拒绝 |
| worker 核心定向 | `20261012T085328.934513900Z/summary.json` | 26 个含子用例通过、零失败/跳过；共享资格与单次决策提交、迟到绑定/版本拒绝、旧死亡进程入口、原树 proof、只读装配与 Attempt 原子提交 |
| 已提交回执后的结果恢复 | `20261012T091650.499555100Z/summary.json` | pending report 的原请求已 confirmed 时仍读取原结果、补记原启动与 Command、结算一次，无新增输入/操作 |
| C1 observation / report failed无final / successful缺final / Orca local | `20261012T041503.009659600Z`、`20261012T043132.008463400Z`、`20261012T050223.023650700Z` 中对应场景的 `scenario.json` 与 `e2e-1.jsonl` | 独立通过；各轮 summary 含其他失败，不能算整轮通过。u 正常与读取错误前后业务快照相同，无 panic |
| C1 Orca environment、C2 同thread人工成果、R1 | `20261012T080130.763184800Z` 对应场景 | 独立通过；该 summary 含其他失败。人工成果经新实施核验、同 thread/worktree 新 Attempt、正常发布/唯一交付；待合并重启保持原身份 |
| C2 未变report source、C3 answer重放与四种terminal拒绝 | `20261012T083204.216539600Z` 对应场景 | 独立通过；该 summary 含未知发送驱动失败。report仅重试、重复原答案/过期不同答案零新发送；lost/runtime/incarnation/gap分别核验新拒绝决策 |
| C2 变化report source | `20261012T085431.841267400Z/scenarios/recovery.report-source-changed` | 通过；原资源复用、新实施/新report source、旧失败保留及唯一交付。该 summary 的未知发送场景暴露结算缺口，随后修复 |
| C3 原生输入已接受但回执未知 | `20261012T092142.421142000Z` | 本机/environment 与整轮均通过；原请求零重发、已存结果脱离 terminal 重放 |

受控逻辑仍为 C1–C3 三组，按原 5 分钟业务上限拆成 16 个入口；两条 live 已分别通过。四种拒绝使用独立现场，答案重放与变化 report source 分开，未增加产品能力/故障范围或扩大超时。

关键结算修正：观察 EOF 不污染可信原生结果；每 Command 保存公开历史绑定；资格变化与决策替换一次提交；死亡但未完成的旧入口只接受实际资格核验；已核实原输入的 completed 回执进入既有结算，queued 授权门槛不得隐藏原结果；补记原启动时统一移动 pending/current Command。测试没有写业务 SQL 注入故障，没有访问安装现场。

| 真实链 | 通过证据 | 结果与边界 |
| --- | --- | --- |
| V1 Codex | `20261012T102637.045635100Z/summary.json`、对应 `scenario.json` | 整轮通过；原 app-server 死亡后同 Execution/thread/worktree 新 Attempt，三段新受管结果、正数 usage、[专属 PR #225](https://github.com/erranliu/SteerlineTest/pull/225) 实际合并、唯一 Delivery/后继就绪、清理通过 |
| V2 Orca 本机单终端 | `20261012T094738.505687600Z/scenarios/recovery.orca-local-reconnect-live/scenario.json` 与 `e2e-1.jsonl` | 独立场景通过；该轮 Codex 失败使整体 summary 失败。原 terminal/Attempt/nonce/source/cursor 与三次唯一业务输入，[专属 PR #224](https://github.com/erranliu/SteerlineTest/pull/224) 实际合并、唯一 Delivery/后继就绪、清理通过；usage 如实为 unavailable |
| 公开历史读取修正 | `20261012T100713.734555700Z/summary.json`、`20261012T102521.859761900Z/summary.json`、V1 | reader 在 stdin EOF 后有界等待正常退出，超时才 kill；初始化错误保留 8 KiB 有界 stderr。本机 CLI 公开 schema 允许 completedAt 为 null，精确原轮次在旧树/写者已证明停止时可恢复；其余人工轮次仍须终态证据，不把可空时间的旧结果当作成功 |

真实验证先前发现的 SQLite runtime/thread-history I/O 错误和过严空闲门槛保留在失败报告 `20261012T093013.926322300Z`、`20261012T094738.505687600Z`、`20261012T100709.351357200Z`。随后修正 reader 退出与精确原轮次判断并取得 V1 通过；不读取/修改 Codex 私有 DB/rollout。新 live ID 已落实 Plan 原定 10 分钟时限，controlled 仍为 5 分钟。

本轮七项自动验收在上述范围内已通过。源码仍在当前工作树，未部署、替换或修改 `C:/SteerlineRuntime`，没有恢复/改动任务 102。没有声明远端 live、Orca 替代终端重建或硬件断电耐久性已验。

## 2026-10-13 实现审计与主干整合

审计基线从 `6d4b4fa2` 更新至 `efd492ca`，保留主干 PR #295（实际 PR 合并交付、development/v3、删除成果快照）和 #296（Orca 原生主 agent 状态）。上节 2026-10-12 原始报告为整合前历史证据；不会以它们声明当前新 Orca 路径的全部 L3/live 已重跑。

确认并修复：

- 恢复 BindRecovery 拒绝前原地退休 Attempt/清除 Decision，会被调用者的错误持久化带入数据库；现在先构造局部候选，绑定通过后统一提交。
- 异步资格遗漏 pending Command/turn，迟到读取可能修改新的待投递输入资格；现在同时校验 current/pending 绑定。
- Orca 资格用屏幕 idle，或仅比较两次当前查询，可能在实际主 agent working 或原 provider/pane 已替换时授权；现在只读查询并核对已保存原轮次的 pane/provider 与原生主 agent，拒绝 working/waiting、未知身份、旧 done、取消和有草稿的终端。原生普通 done 不代表任务成功。
- Codex 旧 PID 身份无法核实时曾被当作存活资源；现在必须正向证明原进程匹配或已退出，读取和继续均不使用不明原身份。

保持 worker 单写者归口，不新增 RecoveryManager/状态框架/平台耦合。Orca adapter 仅新增 native 资格取证；加密 fake 仅在隔离测试环境，未新增生产钩子。移除主干已删除 workerreturn 的 L1 选择及旧白名单；报告恢复不恢复主干已删除的快照字段。发送回执丢失时缺少原接受/开始证据则保持未核实、零重发；不会由新 done/普通正文补造旧请求。

复现：`20261013T023715.591620800Z/summary.json` 两项回归失败；修正后 `20261013T023836.470102700Z/summary.json` 通过。主干整合后 worker 回归 `20261013T024437.613329500Z/summary.json` 通过；Codex 历史/身份规则 `20261013T025359.206035400Z/summary.json` 通过。所有目录位于系统 TEMP 的 `steerline-test-feedback/`。后续 L1/L3 与真实链结果在交付前登记。

此次审计及验证未访问或修改 `C:/SteerlineRuntime`，未操作安装现场 102。Orca 替代终端、远端 live、硬件断电耐久性仍不承诺。

已完成的整合后回归：固定 L1 `20261013T030242.001510100Z/summary.json` 整体通过、complete=true；主干 Orca native state gate 集成 `20261013T030117.806236800Z/summary.json` 整体通过（local/environment）；精确 worker/Codex/Orca/证据回归 `20261013T030746.920162600Z/summary.json` 整体通过，无失败/skip；恢复报告 continue/纯文本两种反馈路由 `20261013T031524.179577500Z/summary.json` 通过；原生资格/provider 守卫 `20261013T031721.618578800Z/summary.json` 通过。早期 L1 `024935` 的旧 workerreturn 选择与 `025552` 的过宽正则匹配 opt-in live skip 均未计为通过，已分别清理选择和收窄回归。

L3 首轮 `20261013T024846.917946600Z/summary.json` 整体失败：environment reconnect 的场景/清理证据通过；本机重接 Go PASS 但 evidence 期限超限，不计通过；Codex answer 被 stale_instance 拒绝、unknown-send 准备命令超时均保留失败记录。已补拒绝时原 service/执行状态取证，并在报告流程删除快照后改用实际 PR HEAD 验证人工成果处理；后续选择受影响场景单独复验，不提高既定期限。

补充守卫：Orca 当前查询的 pane/provider 必须匹配已保存原轮次，不因同一 PTY 内启动另一个 Codex 会话而授权继续；`20261013T032049.939717200Z/summary.json` 的当前原生资格/provider/pane 规则通过。纯文本恢复反馈与选择 continue 同样回到实施核验；这不改变普通非恢复问题的路由。

`20261013T031312.972022900Z/summary.json` 整体 L3 通过：terminal-local-reconnect（202.42 秒，含已存结果失去 terminal 后重开）、codex-resume（176.22 秒）、codex-report-resume（145.46 秒）、terminal-local-unknown-send（128.10 秒），场景证据和清理全部通过。它修正前轮 failed/incomplete 记录，没有提高期限或使用安装现场。

后续同步主干 `da325443`（PR #297）：保留本机 pairing Bootstrap、就绪等待、明确 rejected 与 unknown 的区别。审计补齐 prepared/rejected 输入的正式恢复门槛，只有新授权创建新 Command，原 rejected 请求不重发；unknown 不得被归为 unsent。这些业务判断集中在 worker。`20261013T033820.360242600Z/summary.json` 为该基线的固定 L1 + worker/Orca 定向回归，整体 passed=true、complete=true，无失败/skip；包含 prepared/rejected/unknown、原回执拒绝及 #297 原生 input readiness 用例。Orca controlled 配对使用本场景 synthetic runtime metadata，不复制真实认证。

`20261013T034616.190134500Z/summary.json` 未计为通过：environment unknown-send 的场景/清理通过（150.03 秒）；本机原结果结算将回执默认 false 覆盖会话资格，下一 probe 使旧答案过期，已删除该非资格写入；`20261013T035728.962077700Z/summary.json` 的资格保留回归通过。gap 门槛旧断言要求 native working 时也出现未核实 Decision，不符合 #296 原生活动事实；现有 InputWait 诊断未投影则为真实缺口，已投影到 Execution.Error，并改为断言具体诊断、原结果仍为空、无继续授权和零重发。此前 `035442`/`035625` 单测失败为初始 running→completed 的合法版本变化未在断言中区分，修正为“已完成资格下回执缺省不可覆盖”后通过，未放宽正式答案版本拒绝。

结果可用性与 InputWait 投影的最终定向回归：`20261013T040822.574846100Z/summary.json` 整体通过，两项规则只补真正的诊断/资格缺口，不捏造结束或结果。当前主干整合后的 live V1/V2 未在本次审计重跑，2026-10-12 的真实链证据只作历史记录；本次最小验收采用受影响 controlled L3、当前原生规则与主干集成回归，不声明全矩阵或主机断电保证。

最终受影响复验：`20261013T040545.642042800Z/summary.json` 整体 L3 passed=true，terminal-local-reconnect（253.11 秒，含已存结果失去终端重开）及 terminal-gap-gate（235.78 秒）场景证据/清理通过；原输入仍只发送一次，gap 明确展示原生订阅原因，没有结果或新继续授权。`20261013T041119.643582200Z/summary.json` 为最后生产修改后的固定 L1，passed=true、complete=true，0 fail/skip。结合 `033820` 的 #297 worker/Orca 定向回归、`040822` 的结果资格/诊断专项，以及 `031312` 的 Codex 两条恢复和 unknown-send、`034616` 的 environment unknown-send 单场景证据，覆盖本轮受影响最小范围。`034616` 整体仍为 failed，不以该单场景通过替代整组通过。最终文档链接检查 358 文件/2161 个本地链接，0 错误；diff 格式通过。未部署 Runtime。
