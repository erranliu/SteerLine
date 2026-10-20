# 近期合入 PR 补验与人工操作导航

交付复核：用户授权两仓提交、提 PR 并检查后合并。SteerLine 提交前六包定向 54 项通过、complete=true、零失败/skip；关键原始证明与汇总保存到[仓库证据目录](evidence/merged-pr-regression-20261020/README.md)。Runner 接上已合入的 #37，保留其按 workflow 能力及 parser 意图校验的生产实现，本轮只补测试与验收记录。此前 A/B 真机使用的是下文已固定的 `c122d641` 加修复构建，不将其改记成 #37 构建的真机资格。历史“未提交”描述保留为当时状态；未执行安装部署。

当前自动结果：首轮 L1/受控流程与本轮 Runner 修复、真实 A→B 生成接管、同包程序/报告/API/浏览器/资源结算均通过；原 i/j 遗留和本包 base/head 均已清理。最终资格文件为第二轮证据根的 `qualification.json`。下文首轮阻断保留历史，当前完成项详见文末及[测试管理最终结果](test-management.md#2026-10-20-自动缺口继续处理)。剩余为人工主观体验，以及当前 Orca 公开原创建查询接口的上游前置。

日期：2026-10-20。归属本次用户直接授权的合并后回归活动；核对 GitHub 已合入的 PR #352–#365（2026-10-19 至 20 日），源码为 `5612f97d6754b98e79b5be409ba688e204cd8832`。各任务仍在各自专属验收记录维护通过/待验状态；本页仅汇总本次跨模块回归和人工操作导航，不新增 Task/DAG 或 roadmap 安排。

## 自动补验范围

原始证据根目录：`C:/Users/lmas1/AppData/Local/Temp/steerline-merged-acceptance-20261020-125058/`。`merged-prs.json` 保存 GitHub merge commit/时间，`sources.json`、`source-diff.patch` 保存构建身份及本轮测试修正。首次 L1 为干净主干；其后为该提交加本轮测试/选择器及文档修改，未提交。不得把 dirty 报告计成产品测试管理中“干净同 commit 的当前通过”。

| PR / 范围 | 本轮执行及证据 | 结果 / 边界 |
| --- | --- | --- |
| #352/#354/#355：停止/原生契约消费者 | 固定 L1、`scripts/test-deployment-stop.ps1`、真实发布契约检查 | 仅验证 SteerLine 消费端；不重复 Runner 的 Orca 原生矩阵 |
| #353：共享 Runner；#360/#362：启动撤销/接管 | 精确 L3 `runtime.shared-runner-projects,recovery.waiting-merge-restart`，`controlled/L3/`；新真实双项目空闲包 `idle-shared/` | 两项 selected-L3 通过/清理完整；真实空闲包实跑通过/结算确认，无模型 |
| #356：近期活动 | 三个相关 Web 文件 26/26，`web-tests-final.log`；TypeScript/Vite 构建 `web-build.log`；SQLite 原事件读取修正复验 | 通过；不代替 RAD-M01 主观体验 |
| #357/#358/#360/#362/#364：部署门槛 | Windows PowerShell 5.1 实跑 `scripts/test-deployment-stop.ps1`，`deployment-script.log` | 通过；strict settlement、兼容 takeover、pin 复用及失败门槛；未运行安装部署 |
| #359：后台控制台 | 固定 L1 中 `TestBackgroundCommandsNoConsole` | 由真实 detached 进程检查 HWND；桌面视觉仍属 SCW-M01 |
| #361：边界文档 | 文档链接与 diff 检查 | 文档变更不新增独立模型/流程验收 |
| #363：长期文档原型 | [专属记录](planning-long-documents.md)，`planning-prototype.cjs/json/png` | 五项实际浏览器交互检查通过；演示页面不计产品后端能力 |
| #365：测试管理 | 固定 L1；Web 26 项；真实 Runner `check/v1` 契约补验 `published-contract/summary.json`；生成接管首轮拒绝及修复后真机 A/B | 发布契约 1 项通过、零 skip；TM-A04/TM-A08 已在继续补验中通过，见[专属记录](test-management.md) |

复现命令与本轮实际选择：

```powershell
go run ./cmd/test-feedback -level=L1 -out <新目录>
go run ./cmd/test-feedback -out <新目录> -- ./internal/app ./internal/storage/sqlite ./cmd/test-feedback -run '^(TestUnifiedStartupLifecycleSharedPIDIndependentInstances|TestUnifiedStartupUnresolvedCensusBlocksRunnerStart|TestRunnerPublicReadsKeepOriginalFinalAndCallerBinding|TestQuick)' -count=1 -timeout=120s
npm --prefix web test -- src/features/runs/runs.test.tsx src/features/runs/plan-tests.test.tsx src/shared/api/queries.test.ts
npm --prefix web run build
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-deployment-stop.ps1
./scripts/test-runtime-isolation.ps1 -Level L3 -Suite 'recovery,runtime' -Scenario 'runtime.shared-runner-projects,recovery.waiting-merge-restart' -OutDir <新目录>
```

L2 controlled、真实公开契约和双项目空闲包通过证据根目录下的 `controlled-smoke.ps1`、`published-contract.ps1`、`idle-shared.ps1` 复用仓库 `scripts/independent-test.ps1`，在独立同用户进程执行，避免工具 kill-on-close Job 干扰。实际命令由各 `summary.json` 和 wrapper 保存；临时脚本不是新增长期测试入口。

## 发现与修正

1. 首次 L1 `l1/summary.json`：passed=false、complete=false，execution=238.420s。SQLite 测试仍期待“实施报告”，而 PR #356 的契约已要求英文 Workflow；改为断言 `implement.report`，保留中文状态、原诊断、噪音预算和原事件绑定检查。
2. 同轮旧双项目用例混入无法解析的配置，却仍期待共享服务启动。PR #360 明确要求无法读出全部原停止义务时禁止启动。将正常双项目用例与坏 census 反例拆开：正例继续核验同 PID、不同项目实例、原锁及独立关闭；新增反例核验诊断、零在线项目、零 Runner 启动/目录和零项目数据库创建。新反例登记到既有 `quick-owner-unified-startup`。
3. 定向补测 `focused-fixes/summary.json`：22 项（含子用例）通过、complete=true、零失败/skip，38.025s。未修改生产行为、接管门槛或超时预算。
4. 本工作树初始没有 Web 依赖，首次定向命令因缺 `vitest` 未执行；随后按 lockfile `npm --prefix web ci`，26 项及构建通过。准备失败保留在 `web-tests.log`，不计测试通过。

5. 修正后第二轮 `l1-fixed/summary.json` 未通过：并行的隔离构建/流程执行期间，累积的 `quick-owner-unified-startup` 达到包内原 180s 预算；已完成的断言没有失败，但启动撤销用例未终态，后续七项未执行，不计通过。该组原有测试累计已接近预算，正常双项目正例恢复执行及新坏 census 反例后余量更小。
6. 将原组中的八项接管/共享 Runner 测试移至 `quick-owner-runner-startup`，其余项目/宿主用例留原组；固定用例全集仅新增上面的坏 census 反例，原断言、每组三分钟及总三 worker 并发预算保持。覆盖检查按固定分区核对包终态，并拒绝跨 owner 重复测试。`selector-partitions/summary.json`：86 项通过、零 skip、complete=true，26.746s；随后补强“用同一组替换另一组”反例，`partition-replay-final/summary.json` 的 missing/replayed/replaced 三种反例全部通过。

最终固定 L1 `l1-partitioned/summary.json`：passed=true、complete=true，execution=200.4s，原三 worker / 每组三分钟预算，零失败/skip；`managed-l1.minimum.json` 同目录保存原生报告封套，before_clean/after_clean=false，仍不计产品中干净同 commit 的当前通过。`partition-coverage.json` 核对 HEAD 的 80 项 owner 测试全部保留，新选择仅增加坏 census 反例至 81 项，零移除/重复。失败、超时和最终通过分别留档。最终结果摘要为证据根目录 `verification-summary.json`。

已通过的流程证据：L2 `controlled-smoke/summary.json` 为 gate-only、passed=true / cleanup_complete=true；L3 `controlled/L3/summary.json` 为 selected-L3、passed=true / cleanup_complete=true，仅覆盖精确选择的两项。真实空闲包 `idle-shared/summary.json` 的 `TestDebugReleaseSharedRunnerService` 一项通过、零 skip、145.421s；包内 `evidence/shared-runner-service.json` 为 business_runs=0、cleanup=confirmed，最终 shutdown clean=true / resources_settled=true，两个项目的本地关闭/锁释放均 true。

首轮收尾文档检查：497 个 Markdown、3058 个本地导航链接，零错误；`git diff --check` 通过。该轮安装/保留运行环境未使用，Runner 源码及原 bin 未修改。其后用户授权继续自动处理，Runner 修复及第二轮结果见文末；修改仍留工作区，未提交或部署。

没有运行 Full、完整 L2/live、审计 live 或新的真实业务 PR；#364 已有的真实兼容更新→PR #625 合并→唯一交付证据继续固定原构建，不改标为本轮源码。

## 首轮发现的工程阻断

TM-A04：从 Runner 干净 `c122d641677593d4b694e2eda7502001510d4e8b` 用其 `scripts/build.ps1` 构建到本轮临时目录，version=`0.2.0-acceptance-20261020`，SHA256=`d94760b643a4ce8469edf3c3a4494b370be6160dc0aa8aa9c046474761a92a1f`。只读复核成功结算过的 k 包：开发 Run compatible，已完成 generate Run 仍 unsupported，诊断为 `unsupported or out of range result acquisition configuration`。因此真实生成对象接管仍未验收。这是 Runner 工程前置，不归为人工体验，也不通过重置、改记录或跳过预检消除。

此前 i/j 未核实结算及其 base 继续按[原记录](test-management.md)保留；本轮没有启动/重置这些现场。Orca 原创建成功丢回执的公开原请求查询能力仍按[兼容接管记录](service-generation-recovery.md)列为外部前置，既有本机实测不代替它。

## 人工操作

参与人：用户；窗口：待安排。下面的体验均可后置，不阻碍无关开发。结果分别登记到对应任务专属记录，附实际时间、包/版本、操作、通过或失败及截图。

### 双项目与 Windows 启动体验

包：`C:/SteerlineDebug/dev-20261020-125527-5612f97d-dirty/`，由统一生成器创建，双项目、固定上述 Runner，Web 地址 `127.0.0.1:56381`。本轮自动空闲联验后保留包供体验，已正式停止且资源结算确认。

1. 双击该包的 `start.bat`，保留启动器窗口，等浏览器显示两个项目就绪。若没有自动打开浏览器，运行该包 `bin/steerline-web.exe open`，并在该命令的环境中设置 `STEERLINE_HOME` 为本包 `home`（具体命令见下方）。
2. **SCW-M01**（[闪窗修复](startup-console-windows.md)）：观察启动期间是否连续闪出短命控制台；预期只保留启动器自身窗口，浏览器正常打开。只看真实桌面才能签收这一项。
3. **SRS-M01**（[共享服务](shared-runner-service.md)）：切换“当前项目”的 SteerlineTest / SteerlineTestB，查看任务与执行页，确认项目内容独立、切换清晰。当前正常空闲包只供双项目启动/整体停止体验；错误项目“已有执行可能继续”的文案需要已有原执行且项目绑定失败的独立现场，仍待安排，不在本包制造坏配置或改库。
4. 双击同一包 `stop.bat`，确认成功停止及错误诊断可读；再次 `start.bat` 检查重复使用同一冻结方案。体验完成后再 `stop.bat`。本包已冻结 live；需要模拟时生成新包，以 `start.bat --simulate` 首次启动。

```powershell
$env:STEERLINE_HOME = 'C:/SteerlineDebug/dev-20261020-125527-5612f97d-dirty/home'
& 'C:/SteerlineDebug/dev-20261020-125527-5612f97d-dirty/bin/steerline-web.exe' open
```

### 执行页与测试结果可读性

**RAD-M01**（[近期活动](recent-activity-display.md)）与 **TM-M01**（[测试管理](test-management.md)）仍待主观签收。已有真实 k 包的两轮截图和 trace 可先做视觉检查，位于：

继续补验后已有当前可体验包：`C:/SteerlineDebug/dev-20261020-133842-5612f97d-dirty`，已停止且资源已结算，固定修复后的 Runner B。双击该包 `start.bat`，在“执行与待处理”选择 Task 90001 / Test management evidence，查看已有候选的“近期活动”和“计划测试”，展开历史及日志，体验结束后运行同包 `stop.bat`。无需再次启动任务；该包的两次实际报告和浏览器截图/trace 在 `evidence/test-management/browser-pass/` 与 `browser-fail/`。此包为单项目，双项目/闪窗体验仍用前节专属双项目包。

`C:/SteerlineDebug/dev-20261020-112514-0752537e-dirty/evidence/test-management/browser-pass/`、同级 `browser-fail/`，其子目录均包含 `execution-tests.png` 和 `trace.zip`。它们固定原 `0752537e` dirty 构建，不能作为当前包的真实重启验收。

1. 分别打开通过/失败截图，检查中文文案、每行信息密度、状态色和长 ID 是否易读；通过汇总应为 1/0/0，失败汇总应为 0/1/0，失败页历史保留两次。
2. 如需查看当时的浏览器 DOM 和操作，从仓库 `web` 目录运行 `npx playwright show-trace '<完整 trace.zip 路径>'`。这是历史回放，不能代替当前 live 点击/刷新体验。
3. 完整 RAD-M01/TM-M01 可在上述当前包查看：进入“执行与待处理”，选择候选，在“近期活动”检查最新在前、英文 Workflow 与中文状态、技术事件默认折叠；在“计划测试”展开条目，检查失败原因、历史、原日志和 Run/artifact/SHA256 引用，切换候选后不串内容。旧 k 包的原 Runner 仍有缺陷，仅供原证据查看；当前包已固定修复后的 B。
4. “看完截图”仅能记录视觉子范围；折叠/刷新/日志的当前交互仍未操作时，保留待验，不整体签收。窗口待安排。

### 规划原型

PR #363 的人工步骤见[专属记录](planning-long-documents.md)。直接用浏览器打开仓库 `docs/product-design/prototypes/planning-ui/index.html`；它是页面内演示，不需启动产品或模型。

## 继续自动处理：Runner 内容生成接管

用户随后明确要求先完成能自动处理的部分。本轮直接修复 `C:/Project/SteerRunner` 中的不适用 parser 配置误校验，并将内容测试夹具校正为生产实际的空 parser 配置；没有修改安装环境或原 Runner bin。

原始证据另存 `C:/Users/lmas1/AppData/Local/Temp/steerline-auto-closeout-20261020-131948/`。Runner 83 项定向（四包）通过、零失败/skip；缺配置与未知 schema 的开发反例另独立复验通过；三个内容流程 Controlled 全部通过，scenario_count=3 / exit_code=0。SteerLine 的生成/发布接线定向 16 项通过，更新 ready 等待后再验同范围通过；新增 driver 的工具支持边界用正确 owner 入口实跑 1 项通过。前一条误选到根 boundary 包的零匹配命令不计通过。

旧 i/j 两包的原开发 Run 现在均已通过原停止请求完成资源结算，两个专属 base 在精确 commit/lease 和无 PR 核对后删除。各包原失败 caller shutdown/proof 保持失败，未 reset 或改写；只追加新服务的公开停止/资源证明。具体绑定与日志在上述证据根下两个 `dev-20261020-11*/` 目录及[测试管理专属记录](test-management.md#2026-10-20-自动缺口继续处理)。

真实换构建补验经统一生成器新建包，只更新 Runner 传输 pin；live 方案、模型、权限和材料冻结。首个新包真实生成/报告/API/浏览器/结算通过，但候选参数被 prepare 清理，脚本严格按缺换代证据失败。第二个准备包因驱动参数与业务候选变量重名而编译失败，无原生运行；修正并定向通过后再建包。随后一包通过正式 A/B 兼容预检和 verify-restart，但驱动过早查询 B 就绪而失败；B 未启动，公开 verify-stopped 已确认原生成资源全部结算，caller 的未核实启动记录保留。下一包完成 B 实际接管，但控制回执文件在动作已派发后写入失败，整场不通过且正常结算；驱动现保存原输入、只提交一次，对该公开未知错误仅查询原 ID，正反例通过。

最终新包 `dev-20261020-133842-5612f97d-dirty` 的真实 A/B 接管、原生成 API、不变输入/final/result、B 上程序通过/失败、两轮正式浏览器、原两个 Run/整包资源结算全部通过。源代码定向 19 项与工具支持边界通过；模型完整阶段只在该一包中核对，不拼接前面的失败。进一步远端核对发现 head 已推送但因“无提交差异”未创建 PR，原驱动只清了 base；已补 head/base 精确 commit/lease、无 PR 及删除后补查，并以同包独立后验实跑通过（1 项、24.496s），没有重新跑模型或改原 proof。修正后的清理例程已接入后续场景正式收尾。`qualification.json` 复核了同一批次全部原始证据、实际 Runner SHA、两个原 Run 的结算及实际 base/head 均为空、PR 数 0，passed=true。

最终清理例程接入后复验 `final-directed/summary.json`：四包 20 项通过、complete=true、零失败/skip，14.843s；包括原请求回执找回正反例及驱动工具支持边界。`final-summary.json` 汇总本轮固定版本、83 项 Runner 定向、三个受控场景、当前真机资格与 i/j 收尾；源码补丁和全部新增驱动原文/摘要一并保留。Runner 历史验收中两个已不在当前检出的临时日志路径改为明确文字来源，不伪造缺失证据；两仓文档链接和 diff 检查收尾通过。两仓改动未提交、未部署。

Orca 1.4.224 的 worktree/terminal create 公开帮助已重新只读核查，仍未暴露原请求幂等键/按原创建请求查询入口，帮助原文保存在本轮证据根；该上游接口前置不改为人工体验，也不以名称/列表猜测认领。
