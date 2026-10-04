# #37 多候选会话与唯一 PR 交付验证

执行范围只限 #37；工作区为 `C:\Users\lmas1\.codex\worktrees\2dd4\SteerLine`，分支 `codex/candidate-workers-37`。起点/开工时最新 main 均为 `941386575e9278b8434d0a472d3e4716d82cdb20`（PR #204）。

## 实际命令与结果

| 目录/入口 | 实际命令 | 结果 |
| --- | --- | --- |
| [identity-count-01](./identity-count-01/summary.json) | `go run ./cmd/test-feedback -out docs/validation/candidate-workers/identity-count-01 -- ./internal/steerline -run ^TestMC[1279]$ -count=1` | 通过；14 个测试事件，0 fail/skip |
| [flow-abandon-01](./flow-abandon-01/summary.json) | `go run ./cmd/test-feedback -out docs/validation/candidate-workers/flow-abandon-01 -- ./internal/steerline -run ^TestMC[34568]$ -count=1` | 通过；20 个测试事件，0 fail/skip |
| [storage-02](./storage-02/summary.json) | `go run ./cmd/test-feedback -out docs/validation/candidate-workers/storage-02 -- ./internal/steerline/sqlite ./internal/steerline/projection -run TestSQLiteV(3|4|5|12)|TestNativeDatabaseInitializesCurrentSchema|TestReaderPortsUseCurrentDatabaseAndPreserveReadOnlyBinding|TestBuildOwnsRecords|TestProjection -count=1` | 通过；9 个测试事件，0 fail/skip |
| [receipt-final-02](./receipt-final-02/summary.json) | `go run ./cmd/test-feedback -out docs/validation/candidate-workers/receipt-final-02 -- ./internal/steerline -run ^TestMC[58]$|^TestSimplifiedCleanup|^TestAppServerLateReply -count=1` | 通过；9 个测试事件，0 fail/skip |
| [settlement-final-02](./settlement-final-02/summary.json) | `go run ./cmd/test-feedback -out docs/validation/candidate-workers/settlement-final-02 -- ./internal/steerline -run ^TestMC7$|^TestWorkerFlowDeliveryCommitsWithoutStepGatesAndUnlocksSuccessor$|^TestTaskDiscardRequiresPreciseUnmergedPRAndRetainsUnknownIntent$ -count=1` | 通过；5 个测试事件，0 fail/skip |
| [最终 L1](./l1-02/summary.json) | `go run ./cmd/test-feedback -level=L1 -out docs/validation/candidate-workers/l1-02` | 7 个固定最小组完整通过；准备 0.8s、执行 197.2s |
| [Linux 代表编译](./cross-os-final.txt) | `GOOS=linux go test -c -o <unique TEMP binary> ./internal/steerline`（PowerShell 精确环境恢复命令见 Plan） | exit 0；SHA-256 已记录，只是编译，不是 Linux 原生运行 |

MC1–MC9 顶层真实非零命中：identity-count-01 包含 MC1/MC2/MC7/MC9，flow-abandon-01 包含 MC3/MC4/MC5/MC6/MC8。Go JSON 记录领取/新增/退出回执、PR/唯一 Delivery、完整丢弃、原停止身份与 disposition；夹具内 SQLite、Git 工作区与 provider 都独立。MC4 worker 子进程真实 commit/push/create，中心只 query/bind/verify。

## 自查与格式

自查已读完整实际 diff 并核对生产调用链/消费者：control inbox、首个 claim 与人工 candidate、全部 Run 投影/精确 Execution、反馈/blocker/恢复、worker-only 发布、历史与放弃 PR、交付和丢弃、停止/清理、CLI 与只读客户端。启动数只在事务前观察；事务仅复核固定 base/已知 PR、普通任务/图版本与原请求，不用候选成员数 CAS、预留或槽位。

退出立即持久化且不可撤销，不依赖停止确认。后台原请求晚到仍保存事实；已确认停止必须和原 stop Operation 同事务提交，未证明的 stop 标记被拒绝。TaskDelivery/closed/后继资格原子唯一；落选保留真实资源状态，已放弃/历史/交付后迟到合并只追加冲突。候选耗时为预留起至退出/交付的墙钟时间，进行中用查询采样时间；StartedAt/EndedAt 仍为当前轮次采样，不伪造成本或缺失指标。

SQLite schema **13**、runtime schema **18**；旧开发格式及旧单候选索引在外部恢复/副作用前拒绝，原文件保留，无迁移/双写/日志回退。

## 失败诊断与支持范围

失败证据未覆盖：storage-01 的获胜者投影语义已恢复并新增独立 resource_status；receipt-final-01 的夹具未走 pending→in_flight→confirmed 原停止屏障，已补完整屏障；settlement-final-01 的旧 provider 缺少 head 身份，已补齐，并恢复未结算 PR 与迟到冲突的原字段断言。对应最终目录全部通过。l1-01 的先前通过报告也保留；最终代码使用 l1-02。

无未解决的 #37 验收项。没有运行 Full/L2/L3 或真实模型矩阵；真实混合配置/原生放弃增量由 #39 验收，TUI 增量由 #38 负责。远端取消/未确认停止沿已有能力如实报告；中心只核验已知 PR，跨系统放弃与外部合并的竞态通过冲突事实处理。未使用生产业务服务、任务库或产品 CLI 领取任务。
