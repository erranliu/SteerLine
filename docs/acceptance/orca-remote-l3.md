# 远端 Orca 正式路径与 L3 验收

## 当前分步流程能力缺口

当前接线已通过 PR #284 改为单终端 local/environment 共用 CLI 原语；结果见[单终端验收](orca-terminal-transport.md)和[真实调试记录](orca-terminal-real-flow-2026-10-10.md)。本机有限样本三阶段交付通过，最终 draft 保护未重新跑真机；远端产品闭环未通过，原生启动信任/模型准备仍有缺口。正式 L3 注册表仍不包含新版远端交付，历史 RO 项的“已验收”不覆盖当前路线。

### supervised 结果接线时点（历史）

下面至原 RO 表均为单终端替换前的来源说明，不表示当前 adapter/驱动仍保留或原生链路已通过。

2026-10-10 更新：[单消息结果接线](orca-supervised-results.md)已补齐 worker_declared 来源、原 Command/Dispatch 绑定、结果冻结、结算核验与正文入库，并开放 StructuredFinalEvidence。新增定向正式领取回归验证通过；本机/远端公开接口见[原生探针](orca-supervised-result-probe-2026-10-10.md)。真实 execute/report/publish/反馈 L3 驱动尚未重新准入，下面历史交付结果不代表当前分步链路已获完整真实验收。

2026-10-09 的历史阻塞：当时 supervised adapter 未声明 StructuredFinalEvidence，ObserveLifecycle 只提供 worker_done 摘要；正式开发流程在原生探测/领取副作用前明确拒绝。该行为已由本次结果接线替代，无副作用的能力拒绝仍由通用不支持实现与原生不可达回归保留。

用户已要求删除或更新过期测试。本次维护将旧单轮 `delivery.orca-remote-codex` 移出 suites.json，保留驱动并显式 retired/skip；runner 拒绝选择旧 ID。当前注册范围不含远端交付，不把该退役或历史成功算作当前能力通过。supervised local/environment 注册与配置回归仍运行。

RO2-A–RO6-A 的当前真实验收仍待补 execute/report/publish 及反馈场景。消息结果来源已接线，但 worker_done 摘要、最后一段 transcript 或私有 agent 文件仍不作为完整结果。Orca 的 structured worker 模式与本次 JSON 结果协议是不同能力，其原会话接续仍按[双目标范围](../spec/orca-supervised-targets.plan.md)及实际平台能力处理。

以下表格与原始记录保持历史范围；新的回归结果归本次全项目维护记录。

唯一归属：[远端 Orca 正式路径与 L3 Spec](../spec/orca-remote-l3.spec.md)及[Plan](../spec/orca-remote-l3.plan.md)。本聊天直接开发；此前已分配 #85 保留，不纳入 roadmap 调度。

| 验收项 | 方法/安排 | 状态 | 通过条件/证据 |
| --- | --- | --- | --- |
| RO1-A | 自动，owner/定向集成及 live 目标预检 | 已验收 | opaque agent、单一 Orca codec、可选通用启动选择及无效目标拒绝；见新边界复验 |
| RO2-A | 自动，L3 正式创建/领取 | 已验收 | 真实远端执行与固定中心/平台身份；见新边界复验 |
| RO3-A | 自动，L3 首轮发布 | 已验收 | 真实修改验证及 PR，合并前 B 阻塞；见新边界复验 |
| RO4-A | 自动，L3 原会话反馈 | 已验收 | 第二轮原公开输出源/工作区，更新同一 PR；见新边界复验 |
| RO5-A | 自动，L3 精确 head 合并 | 已验收 | 唯一 Delivery、后继就绪未启动，原公开输出不变；见新边界复验 |
| RO6-A | 自动，L3 公开输出与 teardown | 已验收 | 精确 transcript/sourceIdentity/provider/PR；usage 未暴露如实登记；所有 owned 资源清理核实 |

环境：Windows 本机 Orca 1.4.209、台式机64g Orca 1.4.215，实际运行登记版本/runtime、source commit/dirty 和构建摘要。远端自身原生 agent/GitHub 登录为执行前置，不复制中心认证。无本任务人工项；环境缺失、真实失败、未知副作用或清理失败仍按自动失败记录。

命令按 Plan；产物使用 runner 临时目录，仓库只登记最小结果摘要和绝对证据位置。RO1-A–RO6-A 以本页最终运行及定向回归验证通过，历史失败保持原事实。

## 2026-10-09 定向回归与真实失败

直接在工作区实现，source HEAD `692a784e05479b11e5818fb5a209971949e3064a`、dirty=true；各 live runner 另登记构建 SHA-256，不将原 HEAD 声称为已包含修改。owner/app/integration 定向、runner/native/evidence/testenv/boundary 与共享控制/生命周期定向已通过；受控 `delivery.basic` 通过，证据 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T025421.970148900Z/summary.json`。

前序真实运行均为失败，不计验收通过：

| runner 临时目录（共同根 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/`） | 真实发现与处置 |
| --- | --- |
| `20261009T021654.989130300Z` | 能力报告丢失 claim branch/base；修正并保留失败 |
| `20261009T022033.964053300Z` | 远端暂不可达、启动未知；用户恢复连接后 runtime 不变；精确原工作区与 shell 已核实清理 |
| `20261009T022650.424155700Z` | 缺远端资源封套，中心停止；补齐 owner codec。原 worker 发布被旧清理提前删除 base 影响；已改未知清理保留夹具，原资源及分支另核实清理 |
| `20261009T023354.931452900Z` | 原 worker 真正完成/PR 已建，中心遗漏最终正文；改为先保存原 worker_done，再核对 settlement；自动清理通过 |
| `20261009T023854.760698200Z` | 观察随 mutation context 取消，未消费完成消息；分离观察生命周期并补回归；期限失败，自动清理通过 |
| `20261009T025007.086505000Z` | 平台启动未得到完整回执，正式领取超时；未重复派发，自动清理通过，测试增加透明 metadata trace |
| `20261009T025806.628945900Z` | 首轮 PR/中心等待合并通过；usage 扫描无关历史会话失败；改为本次 worktree 预筛与最小字段；清理独立 context 接线修正。原 #164 已关闭，原 worker/worktree、base/head 另核实清理 |
| `20261009T030545.549789500Z` | 首轮与原生 usage 通过，正式反馈/平台第二轮完成且同一 PR 更新；中心选中旧轮映射，已改 pending Command 优先并补回归；原 #165 已关闭，原 worker/worktree、base/head 另核实清理 |

部分失败因旧 teardown 未核实退出或业务 context 已取消而保留 scratch；远端资源的后续人工触发 CLI 对账/清理记录不改写原 runner 的失败和 cleanup=false。原始产物仅保留在上述临时目录，无本任务人工验收项。

## 原边界 L3 通过（历史）

北京时间 2026-10-09 11:23:51，delivery.orca-remote-codex 完整通过，1 test/0 skip，366.46 秒含独立收尾。报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T031726.558472800Z/summary.json`：passed=true、coverage=selected-L3；scenario.json 的 cleanup_complete=true、usage_verified=true。source HEAD 同上，dirty=true；产品二进制 SHA-256 `ed01a9f5effee7067cf5e8b6e789c11cb8beabf67d44865f05e4bc17542e2295`，helper `02a9b234538e429ccce37cb2e5702a72d3ed474635910a020ab064d404c7e863`。

真实 [SteerlineTest PR #166](https://github.com/erranliu/SteerlineTest/pull/166) 两轮始终同一 PR，最新 head `3dd3c8404b966cf4cbc5df292327d3faa519f3b8` 经精确 head 约束自动合并，merge commit `ac973adba6f28fa7d7c346df09a5f5b8aa96ebf2`。正式中心生成唯一 Delivery、A closed、B dependency-ready 且未启动；测试 owned 远端 worker、工作区、fallback/helper/coordinator、GitHub base/head、scratch 自动清理核实。PR 历史保留。

原生 root session `01a11ead-462c-7dd0-aaf9-5520d16cfb02` 与工作区保持，两个实际 turn ID 不同；gpt-6-luna/medium 全程核实。累计 total tokens 首轮 220664、第二轮 372665，最终合并/轮询后保持 372665；累计样本不相加。状态、原始生命周期回执、usage 精选字段与清理在上述 scenarios/delivery.orca-remote-codex 下。

最终产品与测试主体已按真实结果验证；之后仅修正 runner 的 native_acceptance_note，避免把远端真实 GitHub provider 标为 loopback，并定向回归 runner；不改写原报告的既有文本。未执行 L2/full/恢复/断联/取消竞争/ask-reply，也不据此宣称这些扩展通过。本聊天按用户授权提交实现 PR；普通开发不修改其他 execution-map 任务。

最终运行后的补充回收尝试：删除旧失败运行的本地 scratch `steerline-e2e-613773971/delivery.orca-remote-codex` 与 `steerline-e2e-4134180205/delivery.orca-remote-codex` 被自动审批拒绝（blocked by policy），未执行，两个目录保留；对应远端资源与测试 base/head 已核实回收。此事实不改变最终独立运行自身 cleanup_complete=true。

## 提交 PR 前的主干集成

提交前集成主干 #252/#253 的 native 无模型场景与可选 PR 反馈，并保留两种环境的登记/报告校验。远端第二轮反馈改为正式纯文字输入，不再指定已移除的 continue 选项。owner/app/integration 定向、runner/E2E 支持/boundary、两条反馈阶段回归、e2e tag 场景枚举和文档链接检查通过；未重复调用真实模型，前述 live 证据仍绑定原构建和范围。随后集成 #254 的独立 console 改动，无本任务文件冲突或接线变化。

## Orca 作为宿主的新边界复验

本轮依用户要求移除 Codex 专属依赖。历史通过仅证明原先实现的链路，不证明新边界；RO1-A–RO6-A 已按修订后的 Spec 重新验收。agent 名称透传；使用 Orca 自有 codec v2、通用启动选择和当前格式资源。RO6 改为 Orca 公开 transcript/sourceIdentity/PR 证据；当前公开接口无 token usage，真实状态为 not_exposed_by_orca，不采用私有文件补验、不标 usage_verified=true。

北京时间 2026-10-09 12:03:03，`delivery.orca-remote-codex` 完整通过，1 test/0 skip，274.46 秒含独立清理。报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T035817.126874700Z/summary.json`：passed=true、cleanup_complete=true、coverage=selected-L3；scenario.json 的 remote_public_output_verified=true、usage_verified=false、remote_usage_status=not_exposed_by_orca。source HEAD `05b55552ee5cf9243fa729625bb3ce1b940cdf2f`、dirty=true；产品二进制 SHA-256 `0595fe08d8b753bddd5a23be0190922a39eff71eefc886a323d8ac573fb20ef8`，helper `4a36d389af7726ce82633ba253d837e65f1e08e714183d159cca8306f3d208e4`。本次通过绑定这些构建，不将 source HEAD 声称为已包含当前未提交修改。

真实 [SteerlineTest PR #168](https://github.com/erranliu/SteerlineTest/pull/168) 双轮更新后自动合并，head `e85c1535a972667f09312496b14514b005eabfae`，merge commit `a09f1946926c579bfb0a32282e4cc281382f2752`。两轮 Dispatch 分别为 `ctx_6d236df3a176`、`ctx_5bed69fba2e4`；Orca 公开 source=transcript、sourceExact=true、provider=codex（场景选择的 agent 名称），原输出源 sourceIdentity 始终为 `SsVEJt58gh1xHxnPdgPzg3cRXgdzvIA_`，消息数从 25 到 37，最终合并/轮询后公开输出保持第二轮结果。原 Execution/Attempt/worktree/terminal/incarnation 不变，唯一 Delivery、A closed、B ready 且未启动通过。worker/worktree、helper/fallback/coordinator、测试 base/head 与 scratch 自动清理核实。

RO1-A 定向回归通过：远端 owner 全包；execution directory/registry/profile/fixed identity；app 与 integration 的 Remote、SupervisedOrca、PS2FixedConfiguration、ConfigLoadsLocalAndRemote、WorkerFlowMode；runner、E2E 支持及 boundary 包。新增 opaque agent 透传及单一 Orca codec/禁止私有文件依赖回归通过。文档链接检查通过。未运行 L2/full 或扩大恢复、断联、取消竞争、问答场景。

live 通过后更新本页、Spec/Plan、夹具说明与回归名称，新增报告门槛用例；定向 runner 和原生默认配置回归通过。生产执行实现保持该次 live 构建对应内容。
