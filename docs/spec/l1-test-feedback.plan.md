# L1 固定范围精简与并行 Plan

用户要求按现有耗时分析调整 L1，并重新执行 L1 验证。最近通过的基线为 `docs/validation/product-simplification-01-console/l1-release/summary.json`：准备 4.09 秒、执行 1147.33 秒，其中集成组 815.86 秒。

## 实现范围

1. 固定 L1 保留任务/产品/存储规则、生产及 E2E 依赖边界、执行身份和交付证据规则、一次正式领取事务回滚/幂等验证与 SQLite 接线。移出固定范围的恢复、多回合、候选调度、远端复杂协议与重服务查询仍可定向执行，Full 不减少覆盖。
2. 集成选择精确到子测试，避免 app-server 双回合函数和 Orca 恢复变体固定重复执行。定向追加始终在固定范围之外运行，不替换固定覆盖。
3. L1/quick 默认三个独立进程并发，允许显式 `-workers=1`；每个分组继续 `-p=1 -parallel=1`，独立 STEERLINE_HOME/TEMP/TMP/TMPDIR、日志及子进程所有权。报告按固定次序收集，记录 worker 数与组起止时间；覆盖检查仍拒绝缺失、重复终态、skip、零匹配及日志截断。
4. 同一边界测试进程复用真实仓库扫描结果；负例临时仓库单独扫描。更新测试总入口及 E2E 导航，记录定向命令和新基线。

## 测试计划

| 级别 / 组 | 场景 / 范围 | 环境 | 命令 | 时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- | --- |
| 定向 Go / runner | L1 默认/显式 worker 参数、并发上限/顺序、固定选择、缺失/重复/skip/截断/零匹配及 Full 覆盖守卫 | Windows，无模型 | `go test ./cmd/test-feedback -run 'Test(Quick\|Scoped\|LevelSelections\|FullCoverage)' -count=1` | 实现后 | 所选断言通过，Full 选择保持完整 | 本次临时产物目录 `runner.jsonl` / `runner.stderr.txt` |
| L1 固定 | quick-core、quick-rules、quick-execution、quick-execution-boundary、quick-owner-*、quick-boundary | Windows，独立 home 与临时目录，无模型 | `go run ./cmd/test-feedback -level=L1 -out=<本次新目录>` | 定向测试通过后 | 退出码 0，passed=true、complete=true，固定用例全部终态且无 skip，最多三个命令并发 | 本次临时目录 `l1/summary.json` 与各组日志；仓库保存摘要及产物路径 |
| 文档静态 | 链接、差异格式 | 本地仓库 | `python scripts/check_doc_links.py`；`git diff --check` | 文档完成后 | 导航及格式无新增问题 | 本次验证摘要 |

新增能力仅为 L1 有界进程调度及其证据字段，不新增 E2E 场景或真实环境。未选择 Full/L2/L3：此次只调整确定性测试的范围和调度，不改产品行为，不需要真实模型、PR 发布或交付恢复验收。串行选择通过参数及调度单测验证，不重复完整 L1；仅在并发现场失败时使用串行诊断。
