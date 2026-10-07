# L1 固定范围精简与并行 Plan

用户要求按现有耗时分析调整 L1，并重新执行 L1 验证。最近通过的基线为固定历史 commit `172cea6c6530b6693b742324a9c1d4f9aeb17b6d` 的 `docs/validation/product-simplification-01-console/l1-release/summary.json`：准备 4.09 秒、执行 1147.33 秒，其中集成组 815.86 秒。

## 实现范围

1. 固定 L1 保留任务/产品/存储规则、生产及 E2E 依赖边界、执行身份和交付证据规则、一次正式领取事务回滚/幂等验证与 SQLite 接线。移出固定范围的恢复、多回合、候选调度、远端复杂协议与重服务查询仍可定向执行，Full 不减少覆盖。
2. 集成选择精确到子测试，避免 app-server 双回合函数和 Orca 恢复变体固定重复执行。定向追加始终在固定范围之外运行，不替换固定覆盖。
3. L1/quick 默认三个独立进程并发，允许显式 `-workers=1`；每个分组继续 `-p=1 -parallel=1`，独立 STEERLINE_HOME/TEMP/TMP/TMPDIR、日志及子进程所有权。报告按固定次序收集，记录 worker 数与组起止时间；覆盖检查仍拒绝缺失、重复终态、skip、零匹配及日志截断。
4. 同一边界测试进程复用真实仓库扫描结果；负例临时仓库单独扫描。更新测试总入口及 E2E 导航，记录定向命令和新基线。

## 测试计划

| 级别 / 组 | 场景 / 范围 | 环境 | 命令 | 时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- | --- |
| 定向 Go / runner | L1 默认/显式 worker 参数、并发上限/顺序、固定选择、缺失/重复/skip/截断/零匹配及 Full 覆盖守卫 | Windows，无模型 | `go test ./cmd/test-feedback -run 'Test(Quick\|Scoped\|LevelSelections\|FullCoverage)' -count=1` | 实现后 | 所选断言通过，Full 选择保持完整 | 本次临时产物目录 `runner.jsonl` / `runner.stderr.txt` |
| 定向 Go / rebase-runner | 衔接 #233 的全入口临时输出，保留 L1 选择及仓库内/外产物过滤 | 同上 | `go test ./cmd/test-feedback -run 'Test(Quick\|Scoped\|LevelSelections\|FullCoverage\|FilterGeneratedRunOutput\|GeneratedRunOutput)' -count=1` | rebase 最新 main 后 | 参数与覆盖守卫通过；随后在新输出目录重跑固定 L1 | `runner-rebased.jsonl` / `runner-rebased.stderr.txt` 与 `l1-rebased/summary.json` |
| 定向 Go / codex-fixtures | #233 删除历史证据后遗漏的双回合、取消及失败样本 | Windows，无模型 | `go test ./internal/execution/adapters/codex -run '^Test(ConversationMapperNativeTwoTurnFixture\|HistoryTerminalOutcomeFixturesKeepFailureAndCancellationDistinct)$' -count=1` | 将三个原样本迁入对应包 testdata 后 | 两个父测试通过；新文件字节等于固定历史 Git blob，随后完整重跑 L1 | `codex-fixtures.jsonl` / `codex-fixtures.stderr.txt`、`fixture-integrity.json` 与 `l1-final/summary.json` |
| L1 固定 | quick-core、quick-rules、quick-execution、quick-execution-boundary、quick-owner-*、quick-boundary | Windows，独立 home 与临时目录，无模型 | `go run ./cmd/test-feedback -level=L1 -out=<本次新目录>` | 定向测试通过后 | 退出码 0，passed=true、complete=true，固定用例全部终态且无 skip，最多三个命令并发 | 本次临时目录 `l1/summary.json` 与各组日志；仓库保存摘要及产物路径 |
| 文档静态 | 链接、差异格式 | 本地仓库 | `python scripts/check_doc_links.py`；`git diff --check` | 文档完成后 | 导航及格式无新增问题 | 本次验证摘要 |

新增能力仅为 L1 有界进程调度及其证据字段，不新增 E2E 场景或真实环境。未选择 Full/L2/L3：此次只调整确定性测试的范围和调度，不改产品行为，不需要真实模型、PR 发布或交付恢复验收。串行选择通过参数及调度单测验证，不重复完整 L1；仅在并发现场失败时使用串行诊断。

## 自动验收覆盖与结果登记

任务专属记录：[验收文档](../acceptance/archive/completed/l1-optimization.md)。原测试计划已有环境/命令/时点/通过条件与证据，以下补齐验收项映射；原结果仅绑定原源码，不因改 Plan 重跑或重标通过。

| 自动验收项 | 对应现有入口 / 覆盖要求 |
| --- | --- |
| L1A1 固定覆盖与定向追加 | 现有 runner 的 Quick/Scoped/LevelSelections/FullCoverage 与零匹配拒绝；固定选择/Full 范围说明保持原定义 |
| L1A2 三 worker 与隔离 | 现有并发调度/期限/隔离断言和真实 L1 组起止报告；核对峰值、环境和进程所有权 |
| L1A3 结果完整性 | 现有缺失/重复/skip/零匹配/截断日志守卫；不合并失败与补跑成一次成功 |
| L1A4 扫描/样本与新基线 | 现有 boundary/fixture 字节核对和最终 L1；只复用实际采样源码 |

执行安排沿本 Plan 的原测试表：实际变更触及对应验收项时运行其最小入口；未变范围先核对原证据，不默认重跑 Full/真实模型/PR 链。发现必要断言或能力缺口时先补最小场景再验收，零匹配/skip/编译不算通过。新原始报告写 runner 默认临时目录或新的空临时目录，具体命令/源码/摘要/日期、逐项结果与证据定位登记到上述任务专属记录并同步 execution-map，Plan 不作实时状态表。

原文中的历史命令/目录保持取证含义；新执行不再写 docs/validation。自动范围无需额外人工验收门槛；原生场景需人准备环境时预约窗口，不把自动失败/缺权限归类为后置人工。不新增验证矩阵或改原通过条件，本次 Plan 修订只做文档静态检查。
