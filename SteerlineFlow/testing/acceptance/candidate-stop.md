# 候选停止参与与终态收敛验收规划

仅归属本次普通开发，[Spec](../../spec/candidate-stop.spec.md)。2026-10-14 状态：已实现，自动验收通过；人工复验待定。以下结果来自本次验证，不沿用前项候选创建验收结果。

## 自动验收

| ID | 最小验收范围 | 拟验证方式 / 环境 | 状态与证据登记 |
| --- | --- | --- | --- |
| CST-A1 | 请求固定项目/服务/候选身份和版本；同 ID 重放与冲突拒绝；停止事实、命令和事件原子提交；停止不因 Task 状态、版本、领取、DAG 或 Delivery 限制资格，且不修改这些记录；目标可参与且存在另一参与候选时，Task closed/discarded、auto/暂停均不单独阻止停止 | 服务器控制入口及 SQLite 隔离夹具，表驱动资格/身份测试，注入事务失败核对未发布成功 | 通过；E1 / E2 / E4 |
| CST-A2 | 人工不能停止最后一个参与候选，失败/已停止/已完成历史不能充数；并发停止最后两个最多一个成功；自动明确启动失败可停止最后一个；停止释放名额，准备中与未停止未知执行计入五个上限；新编号跨停止、失败、重启和并发创建不复用 | SQLite/受控服务夹具，事务竞争屏障及创建/停止重放；复用候选创建和限额夹具 | 通过；E1 / E2 / E4 |
| CST-A3 | 人工停止与启动失败使用同一终态声明和参与判定；停止提交与派发有确定先后，提交后不为该候选交出新的业务/生命周期动作；晚到准备结果、原操作回执和 worker 输出仅留事实，不复活、不补发；绑定错误拒绝、重复事实去重；无专用 retired 执行/恢复通路或资源动作例外 | 受控适配器调用记录及必要屏障核对派发竞争；只计该停止候选的新增动作，允许停止前在途动作返回及原通道传输确认；代码复审核对路径收敛 | 通过；E1 / E2 / E3 / E4；源码静态复查确认旧 retired 执行/恢复函数及旧放弃 API 引用已移除 |
| CST-A4 | 停止不调用 worker/session stop、关闭 PR、Git 删除或其他资源清理，不伪造资源退出；已有活动/未知资源事实不被停止动作改写，真实晚到事实可更新；重启不重连、轮询、恢复或补结算停止候选；准备期缺基线/会话的停止历史可以原身份加载 | 隔离服务重启及受控适配器/资源夹具，核对停止候选的外部调用、资源绑定与执行事实；允许 WorkflowRun 停止事实和命令结果按契约更新 | 通过；E1 / E2 / E3 |
| CST-A5 | F9 固定目标确认、Enter 提交/Esc 取消、草稿保存失败不操作；本地占位不可停止，也不能充作最后一个保护的剩余参与候选；未知结果查询原 ID；成功后正确切换并清空旧正文/问题/活动；用户已换候选时晚到回执不抢选择；历史可读且业务操作不可用，资源显示不伪装停止 | 控制台隔离模型/HTTP 客户端夹具及必要键序列；核对确认区、能力声明和请求/回执接线，不另建多窗口渲染验收矩阵 | 通过；E1 / E5 / E6 |
| CST-A6 | 停止与交付竞争由权威事务裁决，停止候选不能成为交付对象；唯一交付证据保护保留；历史 PR/分支/编号/共同基线不删除或重绑定，不缩减既有新增候选历史 PR 核验范围；停止历史未取得基线时不阻止其余候选取得首个基线 | SQLite、受控 Git/PR provider 与原交付夹具，事务竞争及准备期停止场景；复用既有 PR 核验用例，不新增整套 PR 策略测试 | 通过；E1 / E2 / E4 |

实施 Plan 应为各项落实实际可执行入口；一个用例可覆盖多个项，不要求逐项增加独立脚本。新增驱动仅限当前受控夹具缺少的必要屏障、调用计数或渲染能力，不只记录“后续补验”。默认定向 Go 回归并保存 `cmd/test-feedback` 报告；不默认要求 Full 或 L2/L3。

## 人工复验

| ID | 内容 | 窗口 / 环境 | 阻塞范围与状态 |
| --- | --- | --- | --- |
| CST-M1 | 连续创建 P1/P2/P3，停止其中一个，回看历史并确认新编号不复用；只剩一个参与候选时原因可读；停止成功或晚到回执不会把旧正文带入新选择 | 人工窗口待定；如进行产品调试，先生成 `C:/SteerlineDebug` 下的独立 SteerlineTest 调试包，按冻结执行方案操作 | 可后置，不阻塞无关开发；待验，不把受控测试替代为人工已验 |

## 证据

源码基准：`740bbdf8ebbc82979d20c0b5ec728682c7c7d4b8` 加本聊天未提交修改；各报告保存执行时的工作区状态。均使用隔离 Go/Git/SQLite、受控适配器和控制台/HTTP 夹具，无真实模型或真实 PR 验收。

| 证据 | 实际范围 / 结果 | 报告位置 |
| --- | --- | --- |
| E1 | 最终 `go run ./cmd/test-feedback -- ./internal/execution ./internal/console -run TestCandidateStop -count=1`，6 项通过；事务回滚/幂等/并发保护、Task 独立、编号/名额、在途未知结果不继续核对、重启、交付竞争、界面固定目标/草稿失败/未知结果/晚到选择 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261014T050826.347617600Z/summary.json` |
| E2 | 定向 execution 回归：候选停止、创建失败留档、并发五个名额、MC2、工作区创建回执；11 项通过。实际命令含 `TestCandidateStop\|TestLifecycleEffectForwardsWorkspaceCreationCheckpoint\|TestCandidateStartReservationsRetainFailuresAndBoundParticipation\|TestCandidateReservationConcurrentLimit\|TestOriginalFailedStartReceipt\|TestMC2`，其中 OriginalFailedStartReceipt 无匹配，明确失败另由 E3 的实际用例覆盖 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261014T044959.399052200Z/summary.json` |
| E3 | 定向 execution：`TestCandidateStopControlReplayAndLatePreparation\|TestCandidateStartReservationsRetainFailuresAndBoundParticipation\|TestReleasedWorkerStartConvergesAcrossRestart\|TestSimplifiedCleanupRetainsFrozenDeliveryEvidence\|TestWorkerShutdownDrainsCommittedDeliveryAck\|TestExecutionStopIntentAndUnknownReceiptUseOriginalBinding`，6 项通过；核对现有启动失败、显式交付清理/停服资源授权与冻结证据 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261014T045958.446715400Z/summary.json` |
| E4 | 集成 `TestMC8` 在途准备时由真实服务控制线程停止、`TestMC9` 停止释放名额且编号连续通过；其同轮旧 MC7 空实验/丢弃断言失败。移除与最后一个保护冲突的旧断言，改为保留参与候选与历史；重新执行 `TestMC7$\|TestMC6$` 两项通过，覆盖 app 构造/历史和交付后重复停止保留原事实 | 首轮 `20261014T045204.820443700Z/summary.json`；修正后 `20261014T050352.723373400Z/summary.json`；均在上述 `steerline-test-feedback` 根目录 |
| E5 | `go run ./cmd/test-feedback -- ./internal/console ./internal/product/httpapi ./internal/product/httpclient ./internal/testkit/boundary/... -count=1`，341 项通过，含 HTTP 严格请求与原意图重放、客户端候选版本映射、架构边界 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261014T044754.803710700Z/summary.json` |
| E6 | `go run ./cmd/test-feedback -- ./internal/console ./internal/product/httpapi ./internal/product/query ./internal/execution/reconstruction ./internal/storage/sqlite -count=1`，349 项通过，含界面新增用例、只读投影、重构与存储回归 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261014T045725.630174700Z/summary.json` |

首次停止事务测试使用缺 Delivery 的伪 closed Task，被既有数据库完整性检查拒绝；已改用真实交付构造 closed Task，并保留完整性校验。首次 HTTP 新用例存在回执指针类型编译错误，已修正。失败报告 `20261014T043535.808633600Z`、`20261014T044005.834682800Z` 同根目录保留，不算通过证据。

人工 CST-M1 未执行；未使用安装目录调试，未运行 Full、L2/L3 或已退役真实候选场景。全项目 `go build ./...` 及 `go test -tags=e2e ./tests/e2e/... -run ^$ -count=1` 编译检查通过，后者未运行场景；文档链接与空白检查通过。

## 修改质量审计复验

2026-10-14 修复异步 Reconcile 失败/未知时覆盖初次回执的问题：在结果交给 actor 前保留原创建检查点及两次诊断，停止期间在途核验的结果仍走既有事实接收。停止候选列表改为明确“不占参与名额”，操作者等外部文本经过终端转义过滤。新增控制台原启动请求重开验证；统一保存入口原本已维护 Outstanding，没有新增另一套请求日志。

| 范围 | 结果 / 证据 |
| --- | --- |
| 候选停止、连续创建/五个上限、失败重启、原回执、显式交付清理/停服及控制台/HTTP定向回归 | 首轮 16 项中 15 项通过；新增核验检查点断言误按明文检查嵌套字节，改为解码后核对。失败记录 `20261014T055432.325259000Z/summary.json` 保留；代码未因该测试失败修改 |
| `TestCandidateStopControlReplayAndLatePreparation`、`TestLifecycleEffectForwardsWorkspaceCreationCheckpoint` | 修正断言后 2 项通过，覆盖停止发生在首次执行后和核验在途期间两种边界；`20261014T055815.894617900Z/summary.json` |
| 完整控制台、HTTP 服务/客户端、架构边界 | 343 项通过；`20261014T055547.305139700Z/summary.json` |
| 全项目编译及带 e2e tag 的测试编译 | 通过；E2E 场景未运行 |
| 同步最新主干后的 `go run ./cmd/test-feedback -level=L1` | 12 组全部通过，132.4 秒；含此前无法编译的集成边界组。源码提交 `617a0329e64d475378a766522e512ae4957dac9b`，基于主干 `5a24abec`；`20261014T060020.699562200Z/summary.json` |

报告均位于上述 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/` 根目录，保留执行时工作区状态；人工 CST-M1 仍待验。
