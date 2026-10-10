# Worker 提示词收敛与三轮流程恢复验收

当前提示词以本文末尾的 2026-10-10 精简补充为准，发布补报流程见 [Spec](../spec/worker-publish-report.spec.md)。

仅归属本次普通开发，绑定 [Spec](../spec/worker-prompt-convergence.spec.md)、[Plan](../spec/worker-prompt-convergence.plan.md)；不改变 #84 历史交付结算或 roadmap。

| ID | 自动验收内容 | 当前结果 |
| --- | --- | --- |
| WPC-A1 | 三模板统一，事实引用保留，反馈/恢复只附数据，Orca 不改提示词 | 通过：四项 prompt/冻结输入场景及 adapter 透传场景 |
| WPC-A2 | 原执行缺正文可进 report；已核实报告和候选可进 publish；源失败/未知、报告重试及恢复边界保持 | 通过：WorkerStage 组及受影响增量 |
| WPC-A3 | 原完成结算与公开 transcript 回复分离，原 Task 段/来源绑定、分页/迟到/裁剪、保存与恢复 | 通过：双 adapter 包回归和新增 completion/回复场景，本机/远端保存与恢复 |
| WPC-A4 | 公共领取、三轮流程及状态消费回归，构建、边界与文档检查 | 通过：公共领取、local 三轮、app/console、boundary/helper 与 controlled delivery.basic；文档检查见下文 |

环境为 Windows 现有隔离夹具，不启动真实 worker、不操作安装版。没有独立设计可行性探针，也不把无模型测试算作原生模型交付。

## 2026-10-10 实现验证

源码为当前工作树 `42f36428857a198822a69b342950fd4465f0caf9` 加本次未提交改动。流程 development/v2、Orca 状态 codec 13，不迁移旧格式。未提交/推送或安装运行版。

原始产物目录：`C:/Users/lmas1/AppData/Local/Temp/steerline-worker-prompt-restore-20261010-122859/`。

- WPC-A1：`prompts.log`（4 项，13.192s）及 `results.log` 的 TestSupervisedResultPromptIsUnchanged。新增透传测试首次因 fixture 未绑定 setup snapshot 失败，补齐夹具后通过；未修改生产身份核验来放行。
- WPC-A2：本聊天 `go test ./internal/execution -run '^TestWorkerStage' -count=1 -timeout=4m -v`，159.443s，通过；缺正文仍可核验报告后进入 publish、原结果不改写及原轮次对账增量 28.218s，通过；最终 prompt/反馈事实格式增量见 prompts.log。
- WPC-A3：双 adapter 包回归（orcaremote 3.138s、orca 5.030s）通过，工具输出见本聊天；最终新增结果场景见 `results.log`，包含无手工 payload 的完成/回复分离、分页、延迟回复、来源/裁剪负例及重启/释放后的冻结读取。原完成消息 ack 与正文采集分开，不要求完成工具产生结果正文。
- WPC-A4：`integration.log`（155.476s），公共 unreachable/claim 两项与 local 三轮通过。旧 TestWorkerStageOrcaRoundTrip/UnknownReport 按既有退役规则 skip，不能算作 supervised 三轮覆盖；当前 supervised 定向来自 WPC-A3 及 `consumers.log`。`boundary-helpers.log` 全通过；`controlled/summary.json` 的 delivery.basic 通过（真实 Git/SQLite、模拟 app-server/PR），不替代真实 Orca 模型链。
- `go vet` 通过，见 `vet.log`；构建产物为该目录 `steerline.exe`。文档链接和 diff 检查结果登记在 `static.log`。

没有运行 Full、完整 L2/L3、live 模型或新可行性探针；历史原生模型交付的未验范围保持原记录。

## 提交前审计（2026-10-10）

用户授权审计、修复并提 PR。本聊天复核输入冻结、三轮/反馈/恢复及采集/保存边界。发现 transcript 收集只识别后续 Task preamble，可能把无绑定的后续 user 输入之回复当成原轮结果；现在拒绝跨越未绑定的 user 输入，不根据文本猜测其归属。新增该负例并通过 adapter 全包（2.308s）；正文及结果仍只归原 Task 输入段，没有新协议包装提示词。

本次仍未运行真实 Orca 三轮交付；公开 transcript 缺少原生 final phase，来源继续标明应用绑定的 step reply。对未知来源/正文、未核实工作区内容等事实维持对账，不补造来源或内容摘要，不以本次无模型回归推定原生交付通过。

最终集成候选基线 `origin/main f04fdf089a58723909279d44c497377427b868dd`，主体提交 `49b2546f3a740bce538fef10f500520ff3382916`。全部受影响 execution 定向通过（179.015s），双 Orca adapter 全包通过（orcaremote 3.096s、orca 5.431s），query/SQLite/boundary 与模型重建通过，app 原会话查看定向通过；vet、构建、文档导航通过。主体代码测试后只补控制台诊断和验收文字，不改变三轮规则。原始记录见本聊天及 `C:/Users/lmas1/AppData/Local/Temp/steerline-worker-console-pr-audit-20261010-141126/`；不把以前的未提交/未安装快照改成已验现场。

## 2026-10-10 提示词进一步精简

WPC-A5（自动）：四阶段仅保留动作、文档路径/发布目标和报告格式；正常报告利用原会话，失败/证据缺口及报告重试才补事实。反馈/恢复不发送内部身份、恢复状态和工作区；源绑定、候选/PR 核验保持。Windows 隔离夹具，未使用安装环境或真实模型。

WPC-A5 通过。基线为 rebase 后 `origin/main 79f17c0d` 加本次工作树改动，证据为本聊天工具输出：

- 模板/输入裁剪、报告证据缺口与重试源、发布目标阶段选择通过（最终 prompt 组 1.756s）。
- 快照绑定候选、错误模型 SHA 拒绝和报告格式重试通过（42.045s）；源绑定/推进/发布补报、失败/未知/写入/PR 不匹配/恢复等定向场景通过。初次运行的失败来自旧提示词断言；已更新断言，分组重跑。
- 原会话与工作区保持、自由文字/选项附注/已有 PR 反馈通过（75.397s）。反馈测试原先要求旧拼接字符串，改为核对独立用户文字和选中答案。
- 本地三轮直达及四轮补报集成通过（249.444s）；模型报告省略 SHA 的最终四轮集成再次通过（117.576s），程序核验和绑定候选。controlled 夹具最终通过（8.209s）。
- go vet 改动包、git diff --check 与受影响文档链接通过。

未运行 Full 或真实模型，不修改安装目录，不将无模型验证算作真实 PR 交付。
