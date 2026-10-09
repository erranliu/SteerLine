# 远端 Orca 正式路径与 L3 验收

唯一归属：[远端 Orca 正式路径与 L3 Spec](../spec/orca-remote-l3.spec.md)及[Plan](../spec/orca-remote-l3.plan.md)。本聊天直接开发；此前已分配 #85 保留，不纳入 roadmap 调度。

| 验收项 | 方法/安排 | 状态 | 通过条件/证据 |
| --- | --- | --- | --- |
| RO1-A | 自动，owner/定向集成及 live 目标预检 | 已验收 | 有效远端配置可选择、无效目标拒绝；不借用本机权限证明 |
| RO2-A | 自动，L3 正式创建/领取 | 已验收 | 真实远端执行与固定中心/平台身份 |
| RO3-A | 自动，L3 首轮发布 | 已验收 | 真实修改验证及 PR，合并前 B 阻塞 |
| RO4-A | 自动，L3 原会话反馈 | 已验收 | 第二轮同会话/工作区，更新同一 PR |
| RO5-A | 自动，L3 精确 head 合并 | 已验收 | 唯一 Delivery、后继就绪未启动，无额外模型轮次 |
| RO6-A | 自动，L3 usage 与 teardown | 已验收 | 精确 root 会话/模型/累计 usage；所有 owned 资源清理核实 |

环境：Windows 本机 Orca 1.4.209、台式机64g Orca 1.4.215，实际运行再登记版本/runtime、source commit/dirty 和构建摘要。远端自身 Codex/GitHub 登录为执行前置，不复制中心认证。无本任务人工项；环境缺失、真实失败、未知副作用或清理失败仍按自动失败记录。

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

## 最终 L3 通过

北京时间 2026-10-09 11:23:51，delivery.orca-remote-codex 完整通过，1 test/0 skip，366.46 秒含独立收尾。报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T031726.558472800Z/summary.json`：passed=true、coverage=selected-L3；scenario.json 的 cleanup_complete=true、usage_verified=true。source HEAD 同上，dirty=true；产品二进制 SHA-256 `ed01a9f5effee7067cf5e8b6e789c11cb8beabf67d44865f05e4bc17542e2295`，helper `02a9b234538e429ccce37cb2e5702a72d3ed474635910a020ab064d404c7e863`。

真实 [SteerlineTest PR #166](https://github.com/erranliu/SteerlineTest/pull/166) 两轮始终同一 PR，最新 head `3dd3c8404b966cf4cbc5df292327d3faa519f3b8` 经精确 head 约束自动合并，merge commit `ac973adba6f28fa7d7c346df09a5f5b8aa96ebf2`。正式中心生成唯一 Delivery、A closed、B dependency-ready 且未启动；测试 owned 远端 worker、工作区、fallback/helper/coordinator、GitHub base/head、scratch 自动清理核实。PR 历史保留。

原生 root session `01a11ead-462c-7dd0-aaf9-5520d16cfb02` 与工作区保持，两个实际 turn ID 不同；gpt-6-luna/medium 全程核实。累计 total tokens 首轮 220664、第二轮 372665，最终合并/轮询后保持 372665；累计样本不相加。状态、原始生命周期回执、usage 精选字段与清理在上述 scenarios/delivery.orca-remote-codex 下。

最终产品与测试主体已按真实结果验证；之后仅修正 runner 的 native_acceptance_note，避免把远端真实 GitHub provider 标为 loopback，并定向回归 runner；不改写原报告的既有文本。未执行 L2/full/恢复/断联/取消竞争/ask-reply，也不据此宣称这些扩展通过。本聊天按用户授权提交实现 PR；普通开发不修改其他 execution-map 任务。

最终运行后的补充回收尝试：删除旧失败运行的本地 scratch `steerline-e2e-613773971/delivery.orca-remote-codex` 与 `steerline-e2e-4134180205/delivery.orca-remote-codex` 被自动审批拒绝（blocked by policy），未执行，两个目录保留；对应远端资源与测试 base/head 已核实回收。此事实不改变最终独立运行自身 cleanup_complete=true。
