# Rebase 后 Full 复验与 SQLite 取消泄漏修复

用户直接授权 rebase 后重新运行 Full；沿用本聊天的失败修复与 PR 交付范围，不占 roadmap Task ID。

最终结果：修复版 Full `complete=true / passed=true`，1772 条父/子测试记录中 1753 通过、19 默认跳过、0 失败；全部七个分区退出码为 0，覆盖检查无遗漏或重复。验证实现 commit 为 `beec0e358d6b904dd318924accd0e5b8ceeef34c`，启动时工作区干净；后续仅登记文档结果。

## 范围与发现

- 已无冲突 rebase 到 `origin/main` 的 `d20a72284705b4daac476d4fa5770151dca03e56`，该提交合入上一轮 [PR #315](https://github.com/erranliu/SteerLine/pull/315)。
- 本轮选择 `go run ./cmd/test-feedback -workers=3`，覆盖全部未启用 e2e tag 的确定性 Go 回归；不重新执行真实模型、L2/L3 或浏览器场景。
- 首轮 `TestAPIServiceLifecycle` 的业务断言完成后，Windows 删除临时 SQLite 文件失败。只读 Restart Manager 查询确认仍由该次 `integration.test.exe` 持有文件；原始失败保留。
- 临时 Go overlay 记录数据库连接与 goroutine，仅用于诊断，不进入交付源码。该 API 用例两组重复诊断分别 3/3、10/10 通过，不能据此将首轮失败记为通过。
- 新增 `TestCanceledSQLiteQueryReleasesStatement`：在受控 SQL 函数执行期间取消查询，要求返回取消错误后，同一连接仍能 checkpoint，随后关闭资源并正常清理。旧依赖上实际出现 `interrupted (9)` 及同类数据库占用失败。

## 修复

将 `modernc.org/sqlite` 从 `v1.38.2` 更新至 `v1.40.1`，并使用其要求的 `modernc.org/libc v1.66.10`。该 SQLite 版本的 Git commit 正是上游取消时关闭 Rows 的合入提交 `d33e568298f6d0da40718843d3562d716f7c4f1b`。参见[上游修复与原因](https://gitlab.com/cznic/sqlite/-/merge_requests/81)：查询已经生成 Rows 后取消时，旧驱动丢弃 Rows 而不关闭，可能留下未结束的读事务和文件占用。

本次修复依赖资源释放行为；不修改 worker 流程、恢复授权、产品超时或测试业务通过条件，不删除失败用例，不添加旧数据迁移。

## 验证与证据

原始证据根目录：`C:\Users\lmas1\AppData\Local\Temp\steerline-full-rebase-20261015-064300715`。

| 验证 | 命令 / 证据 | 状态 |
| --- | --- | --- |
| 首轮 Full | `go run ./cmd/test-feedback -workers=3 -out <root>`；根目录 `summary.json` 与各分区日志 | 已结束，passed=false / complete=false；1751 通过、19 默认跳过、1 失败；保留失败，不作为修复后通过证据 |
| 旧依赖反例 | `go run ./cmd/test-feedback -out <root>/cancel-probe-old -- ./internal/storage/sqlite -run '^TestCanceledSQLiteQueryReleasesStatement$' -count=1 -timeout=1m` | 失败，checkpoint 与 Windows 清理均复现 |
| 修复后取消回归 | 相同场景，`-count=20 -timeout=2m`；`cancel-probe-fixed/summary.json` | complete / passed，20/20，无跳过 |
| 修复后 API 与命名 SQLite 场景 | `go run ./cmd/test-feedback -out <root>/fixed-sqlite-api -- ./internal/storage/sqlite ./tests/integration -run '^Test(CanceledSQLiteQueryReleasesStatement\|SQLite.*\|APIServiceLifecycle)$' -count=1 -timeout=5m` | complete / passed，9/9，无跳过；含原失败 API 用例 |
| 修复后 SQLite 整包 | `go run ./cmd/test-feedback -out <root>/fixed-sqlite-package -- ./internal/storage/sqlite -count=1 -timeout=3m` | complete / passed，58/58，无跳过 |
| 修复后 Full | `go run ./cmd/test-feedback -workers=3 -out <root>/final-full`；`final-full/summary.json` | complete / passed；1753 通过、19 默认跳过、0 失败；准备 31.1 秒、执行 3363.7 秒 |
| 依赖完整性 | `go mod verify` | all modules verified |
| 文档与差异 | `python -X utf8 scripts/check_doc_links.py`、`git diff --check` | 409 个 Markdown、2568 个本地导航链接，0 错误；差异检查通过 |

修复后 Full 的分区明细：

| 分区 | 通过 | 默认跳过 | 失败 |
| --- | ---: | ---: | ---: |
| core-1 | 131 | 0 | 0 |
| core-2 | 141 | 0 | 0 |
| core-3 | 108 | 0 | 0 |
| other-packages | 1260 | 11 | 0 |
| integration-1 | 20 | 1 | 0 |
| integration-2 | 27 | 5 | 0 |
| integration-3 | 66 | 2 | 0 |

`final-full/summary.json` 的 SHA-256 为 `dfb8408cb9a85ca6cebf62ade6bf00f957a22b1f9d0d78343c2d4992cb66b8c1`。报告记录 Windows/amd64、Go 1.26.4、未启用原生验收开关。新增取消回归与首轮失败的 API 用例均在该次完整 Full 中通过。

最终结果仅登记实际完整结束的报告；临时 overlay 诊断与旧依赖反例不计入最终 Full 的通过数量。
