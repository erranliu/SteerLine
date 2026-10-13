# 工作流核验清单与收敛记录

归属：2026-10-13 用户直接授权的工作流核验梳理与修复。用户明确要求移除所有依赖屏幕显示内容的核验；核验依据为原始输出。范围为任务创建/领取、执行准备、提交/结果、阶段报告、发布/等待合并、交付/依赖、反馈与恢复；不枚举普通配置解析的每个字段。实施与测试安排见[Plan](../spec/workflow-verification.plan.md)。

## 101 / 105 事实

安装环境只读查询：`C:/SteerlineRuntime/state/SteerlineTest-b55e8cc42ae8/steerline.db`。安装程序 source commit 为 `75d427a66a4e52b0f070168af1fb2bef75b30540`。不复制数据库/认证/配置建立测试现场。

- 101：事件 54，北京时间 13:51:57，implement.execute 进入 reconciling，共一次。原始正文已有绝对路径 Markdown 链接；终端显示相对目标路径，正文关联比对拒绝。输入框为空。将同一目标缩为相对路径后关联哈希与屏幕一致，证明是展示转换。
- 105：事件 29、42、62，分别为 13:50:59 implement.execute、13:51:27 implement.report、13:52:46 publish。前两次短暂对账后已推进。发布原始正文为有效 JSON，含 `outcome=done`、summary 与 `https://github.com/erranliu/SteerlineTest/pull/229`；原 native main-agent 结束记录已有。诊断是屏幕缺少可见输入/完成边界，未进入业务 JSON/PR 查询阶段。未在本次查询 PR 外部状态，不把模型声称发布成功记为 provider 已核实。

两例均非模型格式失败，也没有重发业务输入。服务日志混合任务，不能按日志中 reconciling 行数归到 101。

## 核验清单与取舍

| 环节 / 核验 | 依据与作用 | 取舍及理由 |
| --- | --- | --- |
| 创建任务：项目 authority、唯一 ID、任务类型、合同、Spec/Plan commit/path/digest | 中心 DAG 与实际 Git 文档 | 保留。防止执行错误项目、变动后的文档或无效合同 |
| 创建依赖：存在性、重复、自依赖、环 | 中心 task graph | 保留。不是模型回答质量核验，保证可调度 DAG |
| 领取：Task version、前置 Delivery、状态、候选、固定 base/profile | 中心事务、Git 和冻结执行配置 | 保留。防并发冲突、越过前置或执行错误版本 |
| 服务/配置/权限/写根/远端目标前置 | 固定配置和本地主机/原生 API | 保留。失败发生在启动前，不能靠看起来可用的回复豁免 |
| 原资源：runtime、terminal/incarnation、worktree、branch、agent、generation | terminal.show 原生结构与冻结记录 | 保留。防串项目、替换终端或误收旧资源结果；是否被 UI 收为可见 tab 不作门槛 |
| 启动命令与唯一投递请求 | 原 request、accepted、processIncarnation、turn_started 回执 | 保留。未知回执只读对账，不重复启动或补发输入 |
| 就绪的 tui-idle、屏幕 draft、准备时屏幕可读性 | 终端显示/解析 | 全部删除。仅查询原 main-agent/provider/pane；不存在原始就绪事实则不能凭屏幕推断 |
| 启动失败时屏幕文字预览 | screen tail | 删除。诊断使用原始状态、身份或 API 错误；不根据菜单猜信任/权限状态 |
| 原输入内容完整性 | 固定 prompt digest、command、request、attempt | 保留。确保恢复观察同一输入，不擅自重发 |
| 主代理开始：pane/provider、hook authority/revision、root 时钟、prompt 预览、投递 started | 原始 hook / 状态字段 | 保留。当前接口未在 hook 提供请求 ID，开始事实仍需这些原生关联；prompt 预览是原生字段，不是屏幕文本 |
| 标题/聚合 done/子代理结束 | 状态通知 | 仅作为查询提示，不能作为完成证据。工作树或子代理活动不能结束原主代理轮次 |
| 主代理结束：原 pane/provider、开始后 root 时钟、非 interruption/非失败 | 原 main-agent 状态 | 保留。working/waiting/blocked/旧 done 不作为完成；未提供 outcome 不凭空解释为业务 done |
| 最终回复的可见输入边界、Worked for、最后一个 bullet、展开/滚屏状态 | 屏幕渲染 | 全部删除。101/105 的原生结束/正文已经足以采集本轮回复 |
| 最终回复 Markdown、链接路径、换行归一化与原文一致性 | 原始正文对屏幕文本 | 全部删除。保留原生正文，不再以展示形式决定业务结果 |
| 最终回复一秒稳定、完成后 draft 为空 | 两次屏幕截图内容 | 删除。已经完成的原轮次不取决于当前 UI 或后来草稿 |
| 原始正文非空、未触及公开 hook 的 8000 UTF-16 单元截断边界 | 原 lastAssistantMessage | 保留。7999 的 surrogate-safe 边界也可能为裁剪结果，不能将前缀当全文；这是来源能力限制，不是格式核验 |
| 原始正文 first-wins、持久化字节摘要、资源 envelope 完整性 | 原正文与存储 | 保留。不再对正文做渲染归一化；资源 codec 4 不读取旧屏幕候选结构 |
| 本地 Codex 完成与 final_answer | app-server thread/turn/item 及 completion 原始事件 | 保留。已直接使用原始输出，无屏幕路径；commentary/async 不替代 final_answer |
| 接收阶段结果：command/turn/session、step/input version、source binding、重复结果 | 冻结 stage input 与 adapter 原证据 | 保留。防跨轮、跨阶段推进，重格式化不能把原失败变成功 |
| implement.execute 的输出必须 JSON | 普通完成正文 | 不要求，保持普通文字；进入独立 implement.report 生成业务结果 |
| 报告必须裸 JSON、拒绝所有附加字段、控制字符串两端空白 | 模型报告展示形式 | 放宽。接受整个 JSON 围栏和附加字段，仅已知业务字段驱动流程；原正文仍原样存储 |
| 报告必须可解码、单一对象、无重复键、字段类型正确、outcome 已知 | 原始 JSON | 保留。不能从多段回复猜对象、选最后一次 outcome 或把未知词猜成 done |
| summary、needs_input/question、blocked/action/error/category 与互斥关系 | 共用业务合同 | 保留。summary 是给用户与后续报告的结果说明；问题/阻塞原因要能形成可操作决策，矛盾字段不能自动推进 |
| 发布 done 需要可解析 PR URL | 原报告中的候选线索 | 保留。只是后续 provider 查询入口，不以链接文字证明发布成功；publish 非结构化回复仍走既有 publish.report |
| PR 仓库、实例/URL、base、head 仓库/branch、状态、复用同一 PR | provider 原始查询与固定执行分支 | 保留。模型给出链接不能替代事实；head SHA 可随后续 push 变化，不要求永远等于最初 SHA |
| provider 暂不可用/原提交未知 | 原 API 错误与操作记录 | 只读重试，保持 reconciling；不可通过报告/重发掩盖未知。缺正文/查询失败每 30 秒重查，删除会永久停止查询的单次标志 |
| PR 合并、merge commit/时间、选定 WorkflowRun、Delivery 唯一性、解锁前置 | provider 与中心 TaskDelivery 事务 | 保留。模型 done 不等于任务交付，未合并不释放后继 |
| 用户反馈与恢复：Decision ID/version/route、原资源、已保存答案、服务 owner、未知输入 | 中心记录与原生恢复事实 | 保留。防过期答案错投与重启后重复执行；需要明确恢复的事实不因回复可用而取消 |
| 停止/清理的原进程、terminal 和 ptyKilled 回执 | 原资源 API/受管进程 | 保留。UI 消失不能证明进程停止，不使用屏幕或标题核验退出 |

重复出现的身份检查分别位于外部 I/O 前后、adapter 接口与中心事务提交边界，用于防检查后资源变动与旧异步结果写入，并非多次要求模型证明同一事实。本次不将这些边界检查合并为一次。

## 验收明细

| ID | 自动验收项 | 状态 / 证据 |
| --- | --- | --- |
| WV-A1 | 101 路径渲染、105 边界缺失均不阻塞完整原正文；全生命周期不读屏幕 | 定向 Go 通过，见验证结果 R1 |
| WV-A2 | 原始就绪状态检查，working/waiting/blocked/原始来源不可用时零输入；不使用 tui-idle/draft | 定向 Go 通过，R1 |
| WV-A3 | 旧轮次、未开始、替换资源、截断正文、原生取消不被接受；本机/远端同实现 | 定向 Go 通过，R1；不代替真机 |
| WV-A4 | 两次缺正文或原查询失败后仍可原来源只读恢复；持久化原正文恢复不重发 | 定向 Go 通过，R1 |
| WV-A5 | 业务 JSON 展示容错与关键语义核验；阶段/报告/原 PR 路径保持 | R2 已完整通过的必要流程用例 + R3 解码用例通过；不宣称宽选择整组完成 |
| WV-A6 | 本地 Codex 原生来源、PR 身份/合并核验和依赖边界回归 | 定向 Go 通过，R1 |
| WV-A8 | E2E codec/原始回复来源同步，真机恢复驱动使用原始结束事实；旧轮/其他 provider/pane 不完成 | R4 通过；testenv 编译通过，不宣称真机执行 |
| WV-A7 | 无生产屏幕核验路径、文档导航与 diff 格式有效 | 生产 adapter 搜索只剩注释/无关 CLI 参数；导航 357 文件/2139 本地链接，0 错误；git diff --check 通过 |

| ID | 人工复验 | 窗口 / 阻塞范围 |
| --- | --- | --- |
| WV-M1 | 新独立 debug 包，本机/远端真实 Orca 连续三阶段与原资源恢复 | 待安排；只限制真机稳定性声明，不阻碍本次源码交付。安装环境旧 101/105 未被修改或恢复，新 codec 默认不兼容旧状态，不宣称现有任务已解卡 |

## 验证结果

R1：`go run ./cmd/test-feedback -- ./internal/execution/adapters/orcatransport ./internal/execution/adapters/protocol/orca ./internal/execution/publication ./internal/execution/adapters/codex ./internal/testkit/boundary/... -count=1`，6 包通过，226 个测试（包含子测试）通过，2 个原生 opt-in 测试按未启用跳过。耗时 56.995 秒。证据目录：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T061017.580315600Z/`，含 summary.json、go-test.jsonl、go-test.stderr.txt。源码基线 `4b389807d34682332c9c68cda8a5990e8c367d7a` + 本次工作区修改。

R2：开始使用较宽的 `-run '^TestWorkerStage'` 选择，随后收窄到本次必要范围，终止宽选择，不将其报告记为整组通过。已完整通过且未再修改相关实现的用例为：TestWorkerStageUnverifiedImplementationEntersReport、TransientFactsReconcileOriginalTurn、PublicationReadRetrySettlesOpenPR、PublicationReportReadRetrySettlesOpenPR、ProviderCommandFixture、ReconcileRejectsReplacementPR、FeedbackUsesExactFrozenNativePrompt、UnsupportedBeforeNativeProbe、PublicationMismatchShowsActualBranch、ClosedPublicationKeepsPRPolling、Transitions（后列名称均沿用 TestWorkerStage 前缀）。这些日志可复核阶段推进、原 PR 核验及恢复；未执行完的其余分支不登记为通过。证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T061017.399469300Z/go-test.jsonl`。报告解码另以最小命令 R3 验证。

R3：`go run ./cmd/test-feedback -- ./internal/execution -run '^TestWorkerStage(StrictResult|PresentationDoesNotBlockResult)$' -count=1` 通过，2 个测试通过，耗时 9.316 秒。证据目录：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T061602.527353800Z/`。

最终静态检查通过。所有自动验收项在本页各自登记；未运行 Full、L2/L3 live，未启用两项原生 opt-in。WV-M1 保持待验。安装现场未修改，当前旧任务是否继续/新版本安装不属于本次通过声明。

## 提 PR 前质量审计

集成基线已更新到 `bbf067ca`（PR #298 断线恢复已合入）。确认并修复：

1. 新就绪逻辑对 initializing/working 立即拒绝，可能阻塞正常慢启动。改为原始状态有界等待；waiting/blocked 仍零输入拒绝，超时不投递。
2. 同步测试启动异步 observer 后立即关闭，observer 可能抢走原生事件，造成不稳定测试。改用同步 submit；新增真正消费/确认 observer 事件的独立回归，核对 final 只交付一次。
3. JSON struct 解码会接受大小写别名，原重复键检查未覆盖 `outcome/Outcome`、`pr_url/PR_URL`，可能被后值覆盖。按已知业务字段路径检查别名冲突；不把忽略的 metadata 中不同大小写键判为业务冲突。
4. 新主干恢复资格检查重新带入 ReadScreen/draft 门槛。集成时移除，保留原 pane/provider/资源、原生 idle/完成与失败事实检查；screen draft 不决定恢复资格。
5. E2E 证据固定旧 codec 3 / rendered-reply source，真机恢复驱动仍使用 tui-idle。统一引用 adapter 的 StateVersion/NativeReplySource；离线等待改用原主代理结束事实，并按原 provider/pane/开始时间绑定，新增旧 codec/source 和错误轮次/身份拒绝回归。

R4：最终 Orca/protocol/evidence/native/boundary 定向组通过，证据 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T062846.521111000Z/`，耗时 31.980 秒。覆盖原始异步 final、慢启动、恢复无屏幕、E2E 来源/codec 与离线结束原生绑定。原生 opt-in 按未启用跳过。

R5：合并新主干后，Transitions、TransientFactsReconcileOriginalTurn 及两项解码测试通过，证据 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T062353.179662900Z/`，耗时 25.242 秒。随后只收紧已知字段别名冲突的作用域，最终两项解码测试再次通过：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T062850.420445000Z/`，13.679 秒。真机恢复驱动 `go test ./tests/e2e/internal/testenv -run '^$'` 编译通过，未执行真机。

审计中间验证曾捕获测试声明错误、主干恢复代码残留 ReadScreen 引用、旧 draft 预期；已修复，以上 R4/R5 为修复后的通过证据，中间失败不计通过。原始日志留在 test-feedback 的对应临时目录。

R6：最终主干集成后的 publication / 本地 Codex 两包回归通过，证据 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T063122.529259400Z/`，18.215 秒。最终文档导航检查 361 个文件 / 2167 个本地链接，0 错误；diff 格式检查通过。生产与当前 E2E 驱动的 ReadScreen、tui-idle、renderedReply、matchesVisible 搜索无结果。源码审计确认的五项问题均已修复，没有遗留的已确认阻塞项；真机待验 WV-M1 保留。
