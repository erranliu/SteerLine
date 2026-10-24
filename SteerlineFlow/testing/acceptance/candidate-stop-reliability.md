# 验收：候选停止参与的可靠受理与核实

2026-10-25。归属 [Spec](../../spec/candidate-stop-reliability.spec.md) / [Plan](../../plans/candidate-stop-reliability.plan.md)。状态：用户授权自审收敛后实施，源码实现完成，CSR-A01–A08 自动项通过，CSR-M01 保持待验；实际覆盖和证据见末节。原停止历史结果仍见 [Web 候选停止](web-candidate-stop.md)，不继承为本次通过。

## 现场来源与边界

调查源码 `9607fb0099dfc00d9f3de35429d4c43fcdc80407`。Home `C:/SteerlineInstall`，业务仓库 `C:/Projects/merge-h-308-code`，Task 18，候选 C3 `6022d7445336420b93fcf253bf9743fd`，Runner Run `3b4a6d872a59641a987d6f45a7c28635`。

原停止请求 `e414cda1e2dc4f9996c2e06139cf2f5f`，观察版本 2297：北京时间 2026-10-25 06:47:58.664 提交，06:49:22.284 拒绝，耗时约 83.62 秒，`stale_version / candidate changed before withdrawal`。只读数据库观察为 participation=active、withdrawal=nil；正式原请求 GET 返回 200/rejected，原身份与原信封完整。现场证据来源为安装 home 对应项目 state 下原 `application-commands`、`control/requests`、`control/receipts`，未修改。

已确认版本门禁和代码阻塞风险；未取得用户浏览器 journal/网络状态，不能声称已复现该界面一直待核实的直接原因，也不能将 83.62 秒直接等同于某条 CLI 的测量耗时。安装现场不作为故障注入或可写验收环境。

## 自动验收

| ID | 行为及故障注入 | 最小通过条件 |
| --- | --- | --- |
| CSR-A01 | 展示版本后持续增加候选观察 revision，切换派发模式/暂停，再确认停止；另测未来版本、错项目/Task/实例和同 ID 改正文 | 无关变化不拒绝；非法/未来版本和身份冲突拒绝；其他回答/恢复/派发动作原版本门禁不放宽 |
| CSR-A02 | 最后两个候选并发退出、退出与交付并发、自动失败、停止时 Task 状态改变；注入退出事务回滚 | 最后一个保护和唯一交付成立；已结束原因明确，退出事实/编号固定，回滚无部分效果；停止零 Runner/PR/Git 外部检查 |
| CSR-A03 | 退出事务回滚、业务提交后 HTTP 丢失、外层 receipt 写失败、服务重启/换实例后查原 ID；同 ID 改正文或外层回执损坏 | withdrawal 原子保存并可恢复原操作结果，不新增命令事务接口；未知不推测失败，重放不改事实；旧实例原结果可读，新效果不转投；损坏不冒认成功 |
| CSR-A04 | 挂住候选 observe、生成 observe 与普通控制处理；核对 Task/PR/actor.Call 同样使用原工作 context，阻塞后提交停止 | 正常存储下 inbox 至终态 ≤5 秒，停止等待不累加全部候选；原 actor 继续存活，无 goroutine/锁泄漏；其他请求失败不阻塞本请求 |
| CSR-A05 | 在 CLI 写请求可能发送后触发让路、过期后才开始受理、查询跨有效期、提交边界 context 取消、真实存储错误与取消同时到达 | 未知输入不回退 pending/重发，worker 不被停止；受理前过期持久拒绝，已提交结果不因过期降级；真实错误未被 cancellation 掩盖 |
| CSR-A06 | 一个 GET 永不返回、其他请求正常；navigator.locks 被另一标签占用；断网/5xx/404/身份错误；退避与重连 | GET ≤5 秒、POST ≤10 秒，均含锁等待；最多 3 个核实并发、每 ID 一个在途；其他请求可收敛，停止 404 零自动 POST，未知零新 ID |
| CSR-A07 | submitted→rejected、unknown→applied、晚到旧状态；applied 后快照延迟/失败、快照先到、用户中途切换、项目/实例切换与卸载 | rejected 解除全部 pending；applied 停止结果核实且禁旧业务操作，快照刷新单独提示；未知 30 秒后无永久忙碌动画；不抢选择/串项目，不降级终态 |
| CSR-A08 | 复用现有 L2 交付/L3 恢复场景，所属模块补充阻塞+退出/原回执与 Web 组合故障 | 使用真实应用/SQLite与公开 Runner fixture；UI 显示最终拒绝/退出或带原因的未知，停止副作用计数正确，原请求/快照身份一致，资源清理完整 |

CSR-A01–A07 由 Plan 指定的新 `TestCandidateStopReliability*`、Web 所属测试执行，A08 复用现有隔离场景及共享 Api/journal/hook/Runs 测试，不新增测试框架。固定 L1/L2 亦必须通过；不能仅凭 owner 定向测试通过宣称登记级别通过。

并发测试使用屏障固定处理先后，计时使用单调时钟；Web 退避/网络超时用可控计时器，真实浏览器体验按 M01 后置。既有 startup、换代和未知输入边界按受影响范围补反例，不扩大成真实模型/真实 PR 全集。

## 人工体验

CSR-M01：可后置、非自动验收替代项。责任人为用户，窗口待安排；在独立调试包选择两个以上参与候选，打开停止确认后等后台变化再提交，观察失败原因/退出结果、待核实时长、手动核实及切换选择；确认措辞明确“退出业务参与、保留资源”。未安排前保持待验。

## 结果登记

实施时每次登记源码完整 SHA/dirty、配置和包身份、原命令、实际用例数量、报告路径、耗时、失败/补测和清理证据。原始证据放 `C:/SteerlineDebug/wt-83d4-candidate-stop/` 下各次独占目录，仓库只保存最小摘要。

规划阶段只有文档检查；下列实施结果另记。安装程序、数据库、候选参与和原请求没有变更。

2026-10-25 规划检查：`python scripts/check_doc_links.py` 检查 576 份 Markdown、3603 条本地链接，0 错误；`git diff --check` 通过。该结果仅证明文档导航/空白检查，不代表 CSR 自动项通过。

## 2026-10-25 自审收敛与实现

删除新增停止事务 API、GET 推断 not_submitted、任意 16 条批次及新 E2E 框架要求，保留既有 withdrawal/Command 原请求恢复。停止只放宽显示候选/派发版本，不放宽内部 CAS 或其他动作；外层回执缺失时由原业务证据只读恢复，损坏则拒绝。

原 actor 接入发布后通知及可取消工作 context；普通控制也必须在同一让路边界内。被中断候选的未知 submit/reply/resume 复用 ObserveOnly 标记，只查询原请求，未触及的其他候选不被批量撤销。registry 仲裁按原 request ID 隔离并有可取消等待，不保留全项目长调用锁。

Web 原 owner 内有界并发核实、锁+网络预算、单请求合并及只读退避。明确拒绝解除两处 pending；applied 与候选列表刷新分别呈现，禁用旧业务输入，历史问题仍只读可见。没有复制业务参与状态或新增参与版本。

### 已执行证据（源码 HEAD `9607fb0099dfc00d9f3de35429d4c43fcdc80407` 加本次未提交修改）

- 定向 Go：`C:/SteerlineDebug/wt-83d4-candidate-stop/diagnostics/20261024T232538884Z/summary.json`，八包实际选择 `TestCandidateStopReliability|TestRunnerCandidateStop|TestRunnerControlStop|TestCandidateStopHTTPContract|TestHTTPNativeQueuedReadiness|TestCancellation|TestSteerRunnerOutboxBoundaries|TestCandidateWithdrawal`，passed/complete，13.955 秒；覆盖候选观察/生成观察/普通控制三种阻塞及取消后的单次发送。
- Web：`C:/SteerlineDebug/wt-83d4-candidate-stop/diagnostics/web-20261024T231915142Z/`，build.log 与 tests.log，TypeScript/Vite 通过，29 文件 194 项通过。随后增加 POST 10 秒边界，client.test.ts 单独 27 项通过；最终全套结果待补。
- 固定 L1 首轮：`C:/SteerlineDebug/wt-83d4-candidate-stop/test-results/L1/20261024T231958021Z/summary.json`，通过，准备 0.5 秒、执行 11.5 秒。
- 固定 L2 首轮：`C:/SteerlineDebug/wt-83d4-candidate-stop/test-results/L2/20261024T232049979Z/summary.json`，通过，真实隔离应用/SQLite/公开 Runner fixture；清理以 scenario 为准。
- L3：`C:/SteerlineDebug/wt-83d4-candidate-stop/test-results/L3/20261024T232119033Z/summary.json`，recovery.waiting-merge-restart，通过；普通控制及原候选换代身份保持。L3 首次调用因 PowerShell 未引号的点号参数被拆分，报 Go test arguments must follow --，未开始测试；改用字符串参数数组后执行上列结果。

补强取消后 ObserveOnly 和回执项目身份后已重跑固定 L1/L2，最终结果见下表。早先失败包括 Web 初次缺 node_modules（按锁文件 npm ci），过期的“版本增长必须禁确认”断言，以及项目切换取消文案/历史只读展示回归；均已修复后补测。原失败和后续证据分开，不以重试覆盖。

### 最终验证

| 范围 | 结果与证据 |
| --- | --- |
| CSR-A01–A07 定向组 | `diagnostics/final-20261024T233134095Z/summary.json`，8 包、33 项通过，0 失败/跳过，11.310 秒；追加跨项目已存在回执反例后，`diagnostics/identity-20261024T233427832Z/summary.json` 单独通过。上述路径均位于本记录的外部证据根 |
| 核心模块完整 owner 回归 | `diagnostics/owners-20261024T232912407Z/summary.json`，taskrunner/scheduler/runnerbridge/httpapi 138 项全部通过，0 跳过；普通命令缺回执仍为 submitted 的细化及跨项目原回执另由最后的 app 回归证明 |
| 固定 L1 | `test-results/L1/20261024T233448069Z/summary.json`，passed/complete；准备 0.6 秒、执行 6.5 秒 |
| 固定 L2 | `test-results/L2/20261024T233448605Z/summary.json`，通过；准备 15.585 秒、执行 78.168 秒，scenario cleanup_complete=true、无清理错误 |
| L3 恢复 | `test-results/L3/20261024T233155482Z/summary.json`，recovery.waiting-merge-restart 通过；准备 16.062 秒、执行 124.545 秒，原候选/Run/请求保持，cleanup_complete=true、无清理错误 |
| Web 构建/全集 | `diagnostics/web-final-20261024T233042068Z/build.log`、`tests.log`，TypeScript/Vite 通过，29 文件、195 项全部通过 |

三种阻塞（candidate/generation/ordinary_control）在定向日志中的 inbox 通知至停止回执分别为 74.7084ms、103.4895ms、106.7188ms；不是原生网络/模型耗时测量。核实 GET 5 秒、POST 10 秒及锁等待预算使用 fake timers 和支持 abort 的网络/锁夹具验证。组件证明 rejected 清除 pending、applied 与旧快照分离且保留历史问题；没有进行真实 Edge/远端 Orca 窗口签收，CSR-M01 仍未验。

实际修改保留一个 actor/inbox/业务权威，停止原子效果依旧在 withdrawal 事务中，未增新表/参与版本或第二套结束流程。文档检查 576 份 Markdown、3607 条本地链接，0 错误；`git diff --check` 通过。没有部署安装环境或重新提交 Task 18 的停止操作；所有源码结果为当前 HEAD 加未提交修改，未导出为干净提交的 Git 测试历史。

### 提 PR 前代码审计（2026-10-25）

按用户要求在实施后完成一次代码自审，覆盖停止准入与事务、通知/取消/单 actor 调度、原请求恢复和身份绑定、HTTP 请求仲裁、浏览器锁/超时/核实调度及界面终态收敛。确认一处恢复路径校验缺口：外层回执缺失时读取 Command.Result，没有检查协议版本与终态值；现已复用既有协议版本和终态规则补齐，新增损坏版本/状态反例。定向 app 回归 `diagnostics/audit-20261024T234042196Z/summary.json` 通过。

本轮未发现其他阻断项；没有引入新状态机、持久轮询记录或第二套业务写入口。整理两处前端 import/格式后，提交源码再运行固定 L1/L2，干净受测提交及导出记录在后续条目登记。真实浏览器 CSR-M01 仍未验，安装现场不在本 PR 的执行范围。

### 干净提交验证

受测源码为 `d1b55b687cc155b59c22bf8a0a48e8a5faad205f`。固定 L1/L2 开始与结束时源码干净；由正式 record 入口导出 [Git 测试历史](../records/d1b55b687cc155b59c22bf8a0a48e8a5faad205f.json)，后续提交仅补本记录和导出的 JSON，不冒充验证后续文档提交。

- L1：`test-results/L1/pr-20261024T234121050Z/summary.json`，passed/complete，准备 0.435 秒、执行 6.825 秒。
- L2：`test-results/L2/pr-20261024T234120699Z/summary.json`，passed，准备 15.397 秒、执行 82.266 秒；scenario passed=true、cleanup_complete=true、无清理错误。
- Web：`diagnostics/web-pr-20261024T234203526Z/tests.log`，29 文件、195 项全部通过；同目录 `build.log`，TypeScript/Vite 构建通过。

上述路径仍以 `C:/SteerlineDebug/wt-83d4-candidate-stop/` 为根。审计修复只补严丢失回执恢复的协议校验；L3 恢复及核心 owner 回归沿用前表的独立原始证据，未把它们导出为该干净 SHA 的历史结果。
