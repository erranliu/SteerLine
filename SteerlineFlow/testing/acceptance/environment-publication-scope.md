# 环境检测发布范围收敛验收

2026-10-30。[Spec](../../spec/environment-publication-scope.spec.md) · [Plan](../../plans/environment-publication-scope.plan.md)。用户确认先收缩脚本到正常交付需要的能力、再重跑本机Codex；实施及本机Codex正式L4已通过，不改Runner权限，不部署安装环境。用户现已签收本机Codex正向流程、远端Orca负向检测流程；远端最新三轮负报告/封存/门禁/清理完成，环境仍not_ready，见[两条链路签收](environment-tty-failure-report.md)。本机Orca仍等Runner修复。下文保留远端早期启动超时、TTY失败及严格流程未通过的原始历史，不据此覆盖后续验收，也不将历史失败改写为通过。

## 已实施边界

Agent保留TTY、工具检查、原L1/完整L2、专属commit/push、唯一原PR创建/查询。报告version2只记原URL/base/head/head_revision及脚本自有资源清理/未知，移除Agent检查/审核/合并通过字段。SQLite当前格式36，旧状态不迁移、不兼容，安装库未改。

ready须原Runner的provider、仓库及head仓库、URL/base/head/SHA、可用PR状态与封存报告匹配，QueryError为空；缺事实、关闭未合并、错绑定或空URL查询错误保持unverified，不开下一轮重复发布。TTY/L2/三轮/原请求/停止与result封存规则保留。Web显示“已发布”，不将其写成“已合并”。

外部L4驱动按原源码分支规则核实checks/独立审核和原L2后，对该精确PR以预期head SHA合并。首次写前保存独占merge意图并flush，未知只读对账，已存在意图不能重发。外部结果不回写Agent报告；外部规则/合并/清理不足仍使正式L4失败。没有新生产审核阶段、后台合并器、配置平台或Runner环境检测能力。

## 提示词实际正文及独立审核

实际diff把Agent内分支策略读取和合并职责移给外部驱动，将报告改为version2/head_revision；保留TTY、完整L2、用户准备、三轮、未知只对账和停止封存。增长用于明确新职责边界及禁止虚报未执行动作。短指令和运行时拼装规则没有新增inline兜底。

| UTF-8 LF正文/脚本 | 前字节 | 后字节 |
| --- | --- | --- |
| environment-execution.md（内部及三项目镜像） | 4263 | 4579 |
| environment-generation.md（内部及三项目镜像） | 5500 | 5834 |
| SteerLine environment.md / verify-environment.ps1 | 3834 / 11355 | 4282 / 10169 |
| SteerlineTest environment.md / verify-environment.ps1 | 4492 / 11911 | 4962 / 10821 |
| 308 environment.md / verify-environment.ps1 | 5953 / 15274 | 6453 / 14482 |

独立只读审阅者 `/root/publication_scope_review` 审计Spec/Plan、实际正文/字节、项目脚本与接线；execution SHA256 `7da6fd6727babcd01cac71d32c72cdde45dfb0e9e3a71e527387ebca0b364d2c`、generation SHA256 `fe987ed7dba8957d9ba55d1e3efb05ac596d1ace3372430360a5f7e15c78728b` 获批准后才更新摘要测试。其指出自身脚本策略遗漏、关闭PR和空URL查询错误重试出口，均修复并获复核批准。必需checks与独立审批匹配循环沿原调用提成testenv小函数，补缺失/pending/failure、作者自批/撤销/驳回反例，不新建验证平台。最终全量diff复审及L4结果继续独立补记。

## 固定项目与受控证据

本节原始证据根 `C:/SteerlineDebug/wt-5069-environment-publication-scope/`，每次失败/补测独立目录，不覆盖历史。

- SteerlineTest固定提交 `ced83a41a774246a6275b4fd095cd1e571f9b547`，[PR695](https://github.com/erranliu/SteerlineTest/pull/695)；原完整L1五项/L2一项通过、零跳过。报告在独立clone的 `.steerrunner/artifacts/publication-scope-1b38bc8b13ff4011b6fd258d7b201a61-l1`、`-l2`。
- 308固定提交 `c7ddc68a71ca5097b4c32d4c6fba372f96b56028`，[PR441](http://192.168.0.66:3000/zymobile/merge-h-308-code/pulls/441)；实际PowerShell AST和diff检查通过，未在本机运行Unity/L1/L2。两项目PR均未自动合入。

| 已验范围 | 本节证据根下原始位置 |
| --- | --- |
| 完整环境owner：report2、旧字段拒绝、原PR事实/TTY/L2/空URL/关闭状态、未知零新轮 | `diagnostics/final-report-owner-032c31216ee2410e85c724e473ea53ca/` |
| 报告/提示词/输入及正式L4证据校验 | `diagnostics/contracts-final-9799429750024d3c86b8d985bb0a5a46/` |
| app/SQLite环境资格owner | `diagnostics/app-sqlite-6c6599288f36451a8d58eb9769f0944e/` |
| 受控外部合并/未知回执零重发/原报告不改写、本机取证边界 | `diagnostics/external-merge-d1744ae1c26e4bf9b337fd4abbf94c2e/` |
| 最终外部规则门禁正反例及原合并/取证回归 | `diagnostics/external-gates-final-c3f5bea9340f4b32b197670081d9caf4/` |
| Web环境页8项及生产构建 | `diagnostics/web-63627f1945a548e8b4efe59a498b0544/` |
| 固定L1通过 | `test-results/L1/final-bef65cfe7f964bacb574b88698ab80ef/` |
| 完整L2通过、清理完整 | `test-results/L2/final-d1bd11863367415990712d7a96a3c3d2/` |
| 正式L3准备门禁通过313.57秒、清理完整 | `test-results/L3/readiness-0177b0c1b59e44c4b59a5fb0a58a459f/` |
| 固定干净实现d9d71198的L1、完整L2通过，L2清理完整 | `test-results/L1/fixed-658f13295a3e45b6b36268b1bc7ce302/`、`test-results/L2/fixed-6a1f2d73adc94f7392f67ac5ca419eef/` |

早期编译暴露cmd/test-feedback仍消费旧合并字段，已同步独立验证器；早期owner断言仍期待在原PR身份丢失后开第二轮，现改为保持unverified且禁止重复发布，并新增真实事实冲突反例。未删除TTY/L2或身份断言，不把编译失败计入通过。实际修改前后字节/SHA账本在 `diagnostics/material-edits/bytes.json`；项目固定提交及语法检查在 `diagnostics/project-validation/fixed-projects.json`。

## 真实本机Codex续验

两次均固定干净实现 `d9d71198329da62759379e4b3c1bea0c1c00593e`、原full_access profile及原独立Runner SHA256 `7b57b06472ae7c36befa71222cdc6e61f7bc2fda410adee6d6aaf32e58fdc2b5`，未改账号、代理、PATH或工具。原生检测已证明ready，正式L4的后续准入/停止失败仍分别保留，不把局部通过写成整体验证通过。

| 正式L4原报告 | 环境检测与外部PR结果 | 整体结果与收尾 |
| --- | --- | --- |
| `test-results/L4/codex-publication-14129cb5d0df4c43ba0752b86c8c44e2/` | 原活动6337eeb9042c43d6a1c3e0c9fc3dad5c、Run31b57c0454ca0fa1bad443360a812453，一轮ready，实际TTY/L1/完整L2/原PR发布和停止封存通过；[PR696](https://github.com/erranliu/SteerlineTest/pull/696)由外部精确合并，原403未再出现 | 整体failed，准入探针缺项目版本化runtime-progress.md，零探针Run；cleanup_complete=true |
| `test-results/L4/codex-materials-a0afc567a5274fe6bd5fb2de24f7f3ac/` | 一轮ready；[PR697](https://github.com/erranliu/SteerlineTest/pull/697)由外部精确合并；补齐材料后原start_task创建实际Runc0b27815b7565f5475d9e487966371f9 | 整体failed，测试驱动原stop_candidate信封漏TaskID，CLI在提交前拒绝；原Runner随后经公开cleanup confirmed=true，原summary保留cleanup_complete=false与scratch，外部ref补证另存 |
| `test-results/L4/codex-stop-b4a05ba287bb42d994f724783c747dc7/`（固定实现5632650a） | 一轮ready；[PR698](https://github.com/erranliu/SteerlineTest/pull/698)外部精确合并；原准入Run482971610a56e18c79c73244873cf4a7实际启动 | 整体failed；补TaskID后的stop_candidate被last_candidate规则拒绝，原Runner后续cleanup confirmed=true，原summary保留cleanup_complete=false。说明驱动误把停止原生执行当作退出唯一候选，不能放宽生产规则使其通过 |
| `test-results/L4/codex-native-stop-87d6e26f9934405db923269be5c384f3/`（固定实现a5b2f0ca） | 一轮ready；原Run0d3fb776d6fedae36ccffd344d7ecbb8完成实际TTY/L1/完整L2/原PR发布封存，[PR700](https://github.com/erranliu/SteerlineTest/pull/700)由外部精确合并 | 整体failed；原准入Run38f65ab11f5994b991749dfc378bc493已启动，但caller输入摘要尚未投影，停止helper非空守卫拒绝。后续Runner cleanup confirmed=true；原summary保留失败和scratch |

材料缺口经正常Git修复：SteerlineTest新增同源 `SteerlineFlow/testing/runtime-progress.md` 及 `tools/testing/RuntimeProgress.ps1`，308新增协议，既有writer未改。新增均0→3196 UTF-8 LF字节；协议SHA256 `587708f00a71eb6a992072c5b12c5aa7f92ed327e2c614e11e33fa4e2e604542`、writer `d39d89f2f3353116c7f507da998f518a27564aa418082faba9a20b68755bfcb4`。独立审阅者逐字核对权威源后批准；原未存在的文件没有运行时inline兜底。账本 `diagnostics/project-validation/runtime-materials-lf.json` 明确LF与磁盘字节：308既有writer正文3196、CRLF磁盘3253，未改变。

最新项目固定提交：SteerlineTest `20610e10285d9ab9bb4ea3be0c07410158692fc0`（PR695），原完整L1五项/L2一项通过且零跳过，原报告为独立clone `.steerrunner/artifacts/runtime-materials-257904f6ccf44ffb858ecfcbbe28ed63-l1`、`-l2`；308 `ab3ff27eb1dd8a902258a8fbac6feb1bbabf50a6`（PR441），语法检查通过、未本机执行Unity/L1/L2。两材料PR均未自动合入。

第二次现场补充清理证据为 `diagnostics/second-native-ref-cleanup-369112a9e794410aba9c5afe771a07b8/`：原PR697精确身份/合并核实、准入source base无PR、原Runner清理confirmed后，按原SHA租约原子删除三条自有ref，另核实准入Run的远端分支不存在。原失败报告和scratch保留，未将补证回写为原清理通过。

第一次驱动修正只补原candidate.TaskID，未解决停止动作选错的问题。最终复用原progress L4的公开Runner停止循环，提为testenv内部窄helper；保留原Run/input_digest、五次结构化stale_revision零受理刷新上限、原请求/回执日志、未知不重发及原Run stopped核验。环境探针保留唯一candidate active，caller核对原candidate/Run/digest及stopped投影；不新增生产接口、第二候选或停止状态机。此实际diff已获独立复核批准。原生progress调用复用helper且原日志名和请求ID保持，定向tagged testenv11项通过。固定后再跑L1/L2与独立L4，前三次失败不覆盖。未修改安装环境或Runner权限。

第三次现场自有ref清理补证为 `diagnostics/native-ref-cleanup-0dfa225e800348e8b9ed5a10ebcf2194/`。第四次运行前固定实现a5b2f0ca的L1通过，原报告 `test-results/L1/native-stop-0dee7a880629401da7a088359b74eafd/`；完整L2通过，原报告 `test-results/L2/native-stop-f7f8b108988946b5a29147a09486c6cb/`。

第四次失败说明候选RunID可先于caller的InputDigest/Projection出现。等待条件改为与既有progress L4完全一致：三者齐备才记录原绑定并停止；非空守卫、原Run/digest/active/stopped断言保持。此单行实际diff经独立复核批准后固定提交，再独立补测；不改变生产逻辑或将失败现场冒充通过。

第四次现场清理补证为 `diagnostics/native-ref-cleanup-b183061efbce4c2880afbc8fdc124935/`：核实原PR700精确已合并、原Runner资源confirmed、准入source无PR及准入远端分支不存在后，以原SHA租约原子删除三条自有ref；原失败报告与scratch保持原样。

## 最终真实本机Codex通过

固定干净源码 `edb20d721342409aa77acbe3ef24f0a109ff5d4b`、SteerlineTest `20610e10285d9ab9bb4ea3be0c07410158692fc0`，沿用原profile/Runner/900秒预算，输入在 `diagnostics/live-inputs-42fde6c89f8c445ab3038cce45e49d92/manifest.json`。实际命令为 `go run ./cmd/test-feedback -level=L4 -suite=testing -env=live -scenario=testing.project-environment-local -out C:/SteerlineDebug/wt-5069-environment-publication-scope/test-results/L4/codex-bound-stop-8df423d78e4c40e3ba79df31c4af1e98`，通过固定manifest显式设置五项环境变量，非默认安装配置。

- 同提交L1通过：`test-results/L1/binding-4b42531d557d4f2bb243358fb02f0152/`；完整L2通过、清理完整：`test-results/L2/binding-8413fa3244b245ab979aa6769171e847/`。
- 正式L4原报告 `test-results/L4/codex-bound-stop-8df423d78e4c40e3ba79df31c4af1e98/summary.json`，passed=true；受管报告确认before/after提交相同、源码均干净。实际场景passed=true、cleanup_complete=true，无清理错误。
- 原活动 `7327c4728d5147e2ad694418d1331e51` 一轮ready，原环境Run `3da5efe29a5f851b462ceb8e7f76e074`。六项检查含实际TTY、原L1及完整L2通过且零跳过。result-file/v1已closed，正文为report2，SHA256 `2fe1382922fa58aea584fbfee1c1e72400c0256908702690ccecfe54d74f53f3`；原Run stop_status=stopped。
- [验证PR701](https://github.com/erranliu/SteerlineTest/pull/701)由Agent真实发布，head `92fefc73fc02dfdfd34709da8cce2460d293b737`。外部驱动读取原规则（required_checks为空、最低审批数0）、核实原完整L2，再以预期head合并；merge commit `0971dedd56cdd607adfb6c14664c114ea935779f`，不合入业务main，不回写原result。
- 准备前start_task被environment_not_ready拒绝、零候选Run；ready后实际准入Run `5be6a6a8901445aea6e4f64d25b848bc`，唯一candidate `e697c4ceb09e44f08d095b7575138821`，input_digest `c291d3848f75745c1ee0ae886bc68a3e9a5ff24629f88ba6610c64c717d0cc42`。公开停止前四次均为结构化stale_revision零受理，刷新原Run revision后第五次受理；没有未知重发或扩大五次上限。原Run stopped且caller核对同candidate/Run/digest/active/stopped，原请求与回执保留为admission-stop-request/response-0至4.json。
- 同一场景内 `308-runner-cleanup.json` confirmed=true、`remote-ref-cleanup.json` confirmed=true，四条自有source/validation/环境Run/准入Run分支均核实清理；无需事后补证。此前四份失败报告及其独立补充清理证据原样保留。

本机Codex在现有权限与工具配置下已证明所需执行/发布能力可用；此前403发生于额外分支策略请求，未保留原HTTP响应体，具体代理归因仍仅为源码推断，不把本次通过当作其来源证明。结论仅覆盖固定项目提交与本机Codex环境；308 Unity、本机Orca及远端Orca未因此获得ready。未部署安装环境，材料PR695/441与主PR412未自动合并。

最终独立只读审阅者 `/root/publication_scope_review` 已复核固定源码L1/完整L2/正式L4原报告、原result摘要及PR/准入/停止/清理证据和本轮验收正文，批准且无剩余确定缺陷；四份失败记录继续保持失败。收尾仅提交验收文档，不再修改已验实现。

## 远端Orca重连后续验

2026-10-30，用户要求核查远端检测并明确已重新连接。新增证据根 `C:/SteerlineDebug/wt-5069-orca-remote-check/`，不覆盖上述本机通过或历史远端失败。断连原证据为 `diagnostics/connection-fee5d913ad1f489396b02327eed5e630/`：原environment的repo list/status均返回remote_runtime_unavailable，已经在工具沙箱外复核。重连后同一environment `e68f4edf-8a83-42c2-a9da-6d404d3aa3d8` 的公开repo list恢复，绑定runtime `3ef455fc-27dd-435f-b43f-1537eb0ea19b`、308仓库 `59f29cc4-7282-47f4-9929-e8fe97fe1628` / `D:/Projects/merge-h-308-code`。

本轮固定干净SteerLine `096a837980573fc167356c0dbc6722ca2029dc8a`、308材料 `ab3ff27eb1dd8a902258a8fbac6feb1bbabf50a6`，沿原远端profile、development/2、full_access、gpt-6-luna/high及1200秒轮次预算。独立Runner EXE SHA256仍为 `7b57b06472ae7c36befa71222cdc6e61f7bc2fda410adee6d6aaf32e58fdc2b5`，version及离线capabilities原回执保留于 `diagnostics/live-inputs-eb190dab15b943bfafc67b8f4a75b887/`，同目录manifest固定原配置。正式命令为 `go run ./cmd/test-feedback -level=L4 -suite=testing -env=live -scenario=testing.project-environment-orca -out C:/SteerlineDebug/wt-5069-orca-remote-check/test-results/L4/308-reconnected-a6f4672f8cdd40c5847bedc96772fc91`，显式使用manifest中的Runner/profile/项目配置/SHA/预算五项变量。

结论为**执行通道失败，尚未运行项目检测**。原正式报告 `test-results/L4/308-reconnected-a6f4672f8cdd40c5847bedc96772fc91/summary.json` passed=false，原场景cleanup_complete=false与scratch保留，不事后改写。准备前派发门禁拒绝通过；同一公开repo/terminal链核实夹具 `379dc1637927a5a0dabf150c4f027e06f99be8de`，原注册根HEAD/状态未变。原活动 `c27f2d0ee4f04d66ad2d67bc36a4acfa`、Run `f045e3198bac656a70d1fe8635a24c3e` 受理并创建原工作区，但远端Codex在TUI bootstrap退出，原终端错误为：

```text
Error: account/read failed during TUI bootstrap: account/read failed: workspace routing discovery timed out (code -32603)
```

直接证据为 `diagnostics/original-run-ba0227362074426cb8419e3238c66a2a.json`、`original-terminal-list-b638507768f84228975df1a1b1a82e39.json`、`original-terminal-read-e16b09596e2e46c4988a44e5a0ffc3d2.json`，均为原公开接口只读回执；runtime、terminal handle `term_804b9725-ba5f-465c-bdbb-41cfa1a0537b`、incarnation `fe7b2779-6435-4ccf-975c-37d62a81f84a` 与原Run session一致。原terminal read显示启动错误后回到PowerShell提示符；terminal本身为running仅表示壳PTY仍在，不能当作Codex仍在执行。原Run turn_started=false、turn_id为空、final不可用；TTY、prepare、完整L2、PR均未执行。未绕过Runner直接跑脚本、改shell/profile、重发或恢复业务输入，不能将错误解释成Unity/Python等项目能力缺失，亦未复验此前TTY故障是否消除。

确认启动阻断后，经正式environment_stop原请求 `2e25367b86f7e932a5b916693f8560ff` applied，只取消这一原活动，未开第二轮或新活动绕过预算。停止请求及原回执在 `diagnostics/startup-blocked-stop-6f3e1e6784c844099c4d608823b79fd1/`（沿用诊断工具原permission-failure-*文件名，不表示本次是权限错误）。观察器随后project_unavailable/stopped是测试器已关闭原应用；停止结算以原environment-proof为准：state=cancelled、一轮、原Run stopped，result-file/v1 closed，SHA256 `93213b4650335e5ff231359e79155d436e968056c389e197866d33e11c879e2c`；report2仅初始completed=false、checks/l2为空，不算检测完成或三轮验收。

原场景 `308-runner-cleanup.json` confirmed=true。独立补充清理 `diagnostics/original-ref-cleanup-de63885f89f645be8991240de80fd868/` 查询两个精确Gitea base均无PR、原生执行分支从未发布；原Run停止后，按夹具SHA租约原子删除本轮source/validation两条自有ref并核实不存在。原正式报告及scratch保持失败。Runner的公开结算仅确认原Orca终端停止，明确未验证或撤销detached background processes / execution-host credentials，不扩大为整台远端主机清理证明。

上述远端运行未修改实现或项目材料，不部署安装环境。远端连接已恢复，但Codex原生启动仍阻断；需要先查明远端Codex的workspace routing discovery超时，再以新独立现场验证脚本。现有事实不能证明登录失效，也不能据此改检测标准或Runner权限。本轮独立审阅者 `/root/publication_scope_review` 已批准原生证据和上述事实/收尾边界。

### 续验中发现的发布短提示词一致性修正

独立复核发现已合入的完整规则与 `internal/prompts/environment-publish.md` 短指令仍有职责冲突：后者保留旧“核实原 PR、检查/审核、合并及资源清理”。这是收缩发布范围的遗漏，不是本次bootstrap超时的原因；原Run还未执行publish。按原Spec仅将该句改为“核实原 PR 发布身份及脚本自有资源清理”，同句更新对应可读黄金样本；固定材料读取、prepare完整通过前置、原子报告及原PR URL保留，无新规则、inline兜底或接线变更。短模板UTF-8 LF **260→260字节**，实际渲染正文及无末尾换行黄金样本 **259→259字节**；短模板文件SHA256由 `0bf5359610974f4395e98f09528d970704a4e7e9c6f743922e27d4db83f157ca` 变为 `3b88a840a05b1896988953065110fcd1f7cf8d98b42785dc319d3dd5043a974e`。

最终实际before/after正文、模板/渲染黄金样本分别计字节的账本在本证据根 `diagnostics/publish-short-final-5a7e2cf137574f63b2e1f3b0d09a8097/`。独立审阅者先批准实际diff和260→260字节值，再更新黄金样本，不能用期望值更新替代批准。早期 `diagnostics/publish-short-prompt-1072c19486fd4fb0b345e7c1e1422110/` 仍保留：其中黄金样本更新误带末尾LF，首轮 `diagnostics/prompt-owner-7a92971319224f2cb94a70ffdddc212a/` 正确报expected260/rendered259；已仅恢复原黄金样本无末尾换行约定，未改生产render或放宽断言。完整环境owner `diagnostics/environment-owner-caf385e10dce4a258c45bf4ca8d2f836/` 70项通过、零跳过。模板/接线复验与固定L1/完整L2另取独立目录；本次早于修正的远端失败不是新短指令的原生验收，远端仍未ready。

最终模板/固定材料/app材料接线复验 `diagnostics/prompt-owner-final-677b10da1fbf47e68624aff85613a3b6/` 27项通过、零跳过。固定干净提交 `ba857494` 的L1 `test-results/L1/publish-instruction-a886fc2897504dd39978a0e832d5bd1b/` 通过；完整L2 `test-results/L2/publish-instruction-0e261517f20740d39b6b80b6c666ca9d/` 通过、清理完整，受管报告before/after提交相同且均clean。独立审阅者 `/root/publication_scope_review` 已批准固定四文件diff、实际正文、最终字节约定、受控回归与原远端事实边界，无剩余确定问题或新增结构债；随后仅补验收结果文档。原PR412已由外部合入，本增量通过独立后续PR维护，不改写原失败或声称已修复远端启动。

### 用户要求再试：取得负向检测，但严格TTY链仍未通过

用户明确要求“再试”后，新建原活动而非恢复旧Run。证据仍在本远端scope，固定干净源码 `21cf5ebc6b4d8f4926a01ed976b5a1f60561c77d`、原308材料/Runner/profile/1200秒预算不变；输入为 `diagnostics/retry-inputs-cdf9c74cd38643e4a3938835056c1b8a/`。正式入口同上，实际新报告 `test-results/L4/308-retry-0232248593014011857f738d084934eb/summary.json`，passed=false、原cleanup_complete=false，scratch保留。原活动 `be3974f767d8442e834f8e247446dfbf`，夹具 `ad6751665710d50ff5f3192929ac756aef69e479`；原注册根工作区未变，准备前派发仍被门禁拒绝。

| 原轮次 / Run | 原执行及封存事实 | 可得结论 |
| --- | --- | --- |
| 1 / `f150dce3fa1fc9a756886578a960fbd0` | 已越过此前bootstrap超时并执行检测任务。原终端先出现两次CreateProcessW启动失败（含最小TTY入口），Agent经cmd.exe /c启动pwsh探针及prepare，脚本返回1并写report2 completed=true；13项为8通过、3失败、2未完成。原Run stopped、result closed，SHA256 `0553ccfb60d35170f37522f072892ed68eeefe792283ef05b3dcc0da9a158514` | 脚本的负向观察可保留；按“工具无法启动即报告阻断”规则，此轮失败后换启动封装继续执行不算严格流程遵守，不能宣称链路通过 |
| 2 / `e4fe66d63b918543035f9290dd01eac5` | 原verified final报告exec_command TTY入口CreateProcessW失败，OS Error -1073283067 / FormatMessageW 317；未改启动封装继续，prepare/L1/L2/PR未执行。该错误码来源为已核实身份的final，未独立取得完整工具参数/错误回执。原Run stopped、result closed，SHA256 `b5b18078f1aee1b71a9c9691214421228bdb782d50ccef8d679d438923a9d943`，正文仍为初始completed=false | 保持unverified/not_ready；不能将round1 previous中的checks当作本轮结果，不能认定检测完成 |

Round1封存脚本的具体缺项为 `STEERLINE_GITEA_LOGIN` 未配置、`UNITY_EDITOR_PATH` 未设置、当前进程PATH无法解析dotnet；Android模块和Unity许可因此未验证。PowerShell 7.6.9、Git LFS、longpaths、固定源码/干净工作区、Python 3.13.1、pytest 9.1.1及原适配器分类收集、299个Bundle字节校验与脚本进程TTY检查通过。L1/L2未执行，三项L2均incomplete，零PR；不证明机器未安装Unity/.NET或登录失效。原报告不是模型总结替代品，但也不能弥补原TTY启动路径失败后的执行违规。

原公开终端证据 `diagnostics/retry-terminal-a5dece8be79f47e2898ad71d912609dd/{list,read}.json` 精确绑定runtime、handle/incarnation与原Run；`retry-run-05b48ce681a24d088360e1ba5818343b.json`、`retry-second-run-5c06341c98404c58a95f6e3125ad870e.json` 保留两个原Run的公开final与身份。`retry-activity-3373801090934df8a2f9845c25503dce.json` 和 `retry-activity-bea2902c3760463ca62605cb57a7454e.json` 是原caller只读观察；最终以场景environment-proof.json中各轮封存报告为准。终端TUI未展示完整exec_command参数JSON，因此只确认脚本自身stdin/stdout均未重定向，不用Agent自述证明tty:true flag；cmd.exe本身并非违规依据，违规点是工具启动失败后没有按规则立即阻断。此口径已获 `/root/publication_scope_review` 独立复核。

第二轮停止封存后活动进入unverified，不会继续第三轮。经原environment_stop请求 `18308fd0cce5d534d780ef340ed40714` applied后最终cancelled，只有上述两轮，不声称完成三轮负向验收。原回执在 `diagnostics/retry-stop-b2203f43767c4831894e484536136712/`；观察器其后project_unavailable仍为原应用已停止，不等于停止失败。原Runner cleanup confirmed=true；`diagnostics/retry-ref-cleanup-51cf53bf84f546e694914ca66618ab1b/` 独立核实两个精确Gitea base无PR、两个原生分支均未发布，在原Run停止后按夹具SHA租约删除本轮source/validation两条自有ref。原报告和scratch不改写，收尾范围仍仅原Orca终端，不扩大为detached进程/主机凭据证明。

本次仅复测并补证据，未改实现、固定规则、项目要求或目标配置，未部署。远端检测尚不能认定严格稳定；默认TTY启动失败与脚本已检出的三个配置缺项分别处理，不能通过放宽规则、只补变量或采用首轮替代封装结果将整链标为通过。

两份续验文档及原始两轮/停止/补充清理证据已获 `/root/publication_scope_review` 最终只读审核批准，无剩余确定事实缺陷。纯文档收尾检查：616份Markdown/3907条本地链接零错误，测试清单/场景登记一致性两项通过，git diff --check通过；本次未改实现，不重跑或借用L1/L2结果声称远端通过。
