# 架构解耦第一轮验收

日期：2026-10-17。归属：用户直接授权的第一轮实现，无 roadmap Task ID。基线为 `2fb07ba1e74dfe6b16326d7ed462da9c57a5dec0` 加本次工作区变更；原始发现见[架构审视](architecture-decoupling-review.md)，当前图见[代码架构](../architecture/current-code-architecture.md)。

## 范围与实现

- `httpcontract.AllowedProjectRead` 持有后端与 Web 共用的既有读取白名单；删除 Web 对 `applicationhttp` 的 import。后端专用 Task 写入/回执路由、认证、命令封套和实际转发保持原行为。
- `SQLiteStore.Candidate` 在公开存储端口使用现有 `persistenceError`；不存在映射为 `storage/ports.ErrNotFound`，保留底层诊断。事务内部的 `readRunnerCandidate` 继续按 SQLite 事实处理，避免改变领取、重放与版本检查。
- `taskrunner.Start` 改用 `errors.Is(err, storageports.ErrNotFound)`；其他存储错误直接返回，不进入后续准入/领取。取消和关闭数据库不会被解释为候选不存在。
- Web 服务/命令加入 HTTP 消费端守卫根；taskrunner 增加适合自身职责的生产传递依赖规则，允许 Task 用例与公开 Runner 客户端，阻止 SQL、具体存储、app、产品 query/client。覆盖全标签源码、间接 helper 和 Windows/Linux 生产依赖。
- 新增候选错误回归、用例错误分类测试；前者所在 SQLite 包已在 L1 quick-rules，后者加入 quick-execution；守卫反例纳入已有 `TestTM04ExecutionBoundary`，沿用 L1 quick-execution-boundary。

未修改业务状态机、数据库格式或交付事务；原子交付、同快照读取和 ProjectRuntime 资源所有权保持。本轮不包含剩余的读取投影/DTO、Task 类型归属或 app 包拆分。

## 自动验收与最小验证

| ID | 验收项与通过条件 | 场景 / 命令映射 | 环境、时点与结果 | 证据 |
| --- | --- | --- | --- | --- |
| DCR1-A01 | Web 编译依赖不再到达 `applicationhttp/httpapi/query/storage/execution` 或 SQL；直接、间接、其他平台文件中的反例可被拒绝 | `TestHTTPConsumerBoundary`、`TestTM04ExecutionBoundary`；命令 C1；Web `go list -deps` | 实现后 Windows/amd64；守卫包含 Windows/Linux 生产图和全标签源码；通过 | E1，`web-dependencies.txt` |
| DCR1-A02 | Web 与后端沿同一共享白名单读取，路由、认证、原请求和转发行为维持 | `internal/product/applicationhttp`、`web/server` 现有包测试；命令 C2 | 实现后隔离 HTTP/文件系统夹具；通过，2 包、14 测试项、0 跳过，15.200 秒 | E2 |
| DCR1-A03 | 候选缺失以端口错误表示；取消/数据库故障不被当作缺失；保留首次领取、固定材料与原请求/回执重放 | `TestRunnerCandidateReadErrors`、`TestRunnerStartClassifiesCandidateLookupErrors`、`TestRunnerCandidateReservationAndProjection`、`TestRunnerTaskServiceFreezesOriginalStart`、`TestRunnerControlRecoversCommittedStartAfterReceiptLoss`；命令 C1 | 实现后隔离 SQLite/内存端口；通过 | E1 |
| DCR1-A04 | taskrunner 无 SQL/具体存储/app/query/client 生产依赖，合法 Task 用例仍可依赖，helper/其他平台不能绕过 | `TestTM04ExecutionBoundary`；命令 C1；taskrunner `go list -deps` | 实现后；通过；C1 合计 4 包、32 测试项（含子用例）、0 跳过，22.755 秒 | E1，`taskrunner-dependencies.txt` |
| DCR1-A05 | 新增用例纳入既有 L1 选择，不改变固定覆盖与定向选择行为 | `TestQuickTargetedIntegrationDoesNotReplaceFixedCoverage`、`TestQuickTargetedCoverageRejectsZeroMatches`；命令 C3 | L1 登记后；通过，1 包、4 测试项（含子用例）、0 跳过，2.524 秒 | E3 |
| DCR1-A06 | 本轮文档链接和差异无错误 | `python scripts/check_doc_links.py`、`git diff --check` | 最终文档同步后；通过，本地链接零错误、差异无空白错误 | E1 的 `doc-links.txt` / `diff-check.txt` |

测试项计数采用 test-feedback 报告，包含子用例。以上为定向集合，未宣称完整 L1 或 Full 通过。未选 L2/L3/L4、真实模型和浏览器 E2E：本轮没有执行协议、生命周期、业务流程或 React 行为变更，已有隔离用例覆盖读取/命令转发与候选启动。无需安装或真实 PR 现场，未使用 `C:/SteerlineInstall` 或 `C:/SteerlineRuntime`。

证据目录（均在系统临时目录）：

- E1：`C:/Users/lmas1/AppData/Local/Temp/steerline-decoupling-round1-20261017-core/`。
- E2：`C:/Users/lmas1/AppData/Local/Temp/steerline-decoupling-round1-20261017-http/`。
- E3：`C:/Users/lmas1/AppData/Local/Temp/steerline-decoupling-round1-20261017-selection/`。

各测试目录保存 `summary.json`、`go-test.jsonl` 和 `go-test.stderr.txt`；依赖采样只证明生产依赖，不代替运行测试。

## 复现入口

新运行为每个命令使用独立输出目录；`-out` 与路径分开传参。

```powershell
# C1：存储、Task 用例与依赖守卫
$coreOutput = Join-Path $env:TEMP ('steerline-decoupling-core-' + [guid]::NewGuid().ToString('N'))
go run ./cmd/test-feedback -out $coreOutput -- ./internal/testkit/boundary ./internal/app ./internal/storage/sqlite ./internal/taskrunner '-run=^(TestTM04ExecutionBoundary|TestHTTPConsumerBoundary|TestRunnerCandidateReadErrors|TestRunnerCandidateReservationAndProjection|TestRunnerTaskServiceFreezesOriginalStart|TestRunnerControlRecoversCommittedStartAfterReceiptLoss|TestRunnerStartClassifiesCandidateLookupErrors)$' -count=1

# C2：共享路由与 Web HTTP 消费
$httpOutput = Join-Path $env:TEMP ('steerline-decoupling-http-' + [guid]::NewGuid().ToString('N'))
go run ./cmd/test-feedback -out $httpOutput -- ./internal/product/applicationhttp ./web/server -count=1

# C3：L1 选择登记
$selectionOutput = Join-Path $env:TEMP ('steerline-decoupling-selection-' + [guid]::NewGuid().ToString('N'))
go run ./cmd/test-feedback -out $selectionOutput -- ./cmd/test-feedback '-run=^(TestQuickTargetedIntegrationDoesNotReplaceFixedCoverage|TestQuickTargetedCoverageRejectsZeroMatches)$' -count=1
```
