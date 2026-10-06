# 测试执行耗时定位与方案选择

2026-09-28 对源码 `543f0d4e9dd9559f253de663d3ffb646ceecf079` 完成测试执行定位。用户要求先确定具体改法，再形成独立 Spec/Plan；本次没有分配任务 ID、创建中心任务、修改 DAG 或应用产品代码变更。最终方案见 [Spec 草案](../../archive/spec/completed/test-execution-efficiency.spec.md)与 [Plan 草案](../../archive/spec/completed/test-execution-efficiency.plan.md)。

## 环境与完整对照

主机 liuming-asus，Windows/amd64，Go 1.26.4，20 个逻辑 CPU，Go 构建缓存沿用现状。测试结果不使用缓存，未启用真实 GitHub/Codex/Orca/可见窗口验收。产品服务、数据库配置和超时未调整。

串行基线使用现有工具：

```powershell
go run ./cmd/test-feedback -phase test-execution-diagnosis -out docs/validation/test-execution-diagnosis/baseline-serial -cache-condition 'Existing Go build cache; count=1 forces test execution; build optimization out of scope' -native-acceptance 'No live/native acceptance switches enabled; isolated deterministic fixtures only' -- ./... -count=1 -timeout=45m -p 1 -parallel 1
```

三个进程的对照使用同一未修改源码的核心测试程序，自动枚举 270 个顶层用例，用 `SHA256(name)` 前四字节大端数取模 3 分组；三组分别 90、87、93 项，组内串行，各自独立 home。核心结束后，原包清单中的其他包通过 Go 执行一次。未按耗时或结果挑选用例，没有手工白名单。

| 指标 | 串行基线 | 三进程对照 |
| --- | ---: | ---: |
| 全套执行墙钟 | 658.633 秒 | 349.326 秒 |
| 核心包执行 | 603.209 秒 | 307.333 秒（含分组进程调度） |
| 测试/子测试终态 | 460 | 460 |
| 通过 / 跳过 / 失败 | 443 / 17 / 0 | 443 / 17 / 0 |
| 核心顶层用例 | 270 | 270，恰好一次 |

全套执行对照下降 46.96%，核心包约下降 49%。三进程计时不含预先准备核心测试程序及读取清单的时间；原串行 Go 命令使用已有构建缓存。两种成本已明确区分，这些结果只支持测试执行提速，不用于宣称构建改善。原 17 个平台/原生跳过保持，不能算真实平台验收通过。

每个核心顶层用例、动态子测试及其他包的终态集合和状态均与基线相同。三个核心进程的执行时间分别为约 139.7、307.3、215.0 秒；核心墙钟取实际并发窗口，没有把这些时间相加。分组仍有负载差异，本方案保留已验证的固定算法，不增加基于历史耗时的调度策略。

证据：[串行完整汇总](baseline-serial/summary.json) · [原始输出](baseline-serial/go-test.jsonl) · [分组清单](sharded-core/assignment.json) · [完整对照核验](sharded-core/summary.json) · [实验运行器](probes/run-sharded.py)。每组原始日志与其他包输出位于 `sharded-core/`。

## 三次前后配对

采用临时 Go overlay 替换测试文件，产品 `.go` 文件不变。每组比较按前后、后前、前后的顺序交替，独立进程执行三次；Git TRACE2 只在这些配对实验中启用，用来核对实际 Git 进程数。配对不与完整套件并行执行。

| 实验 | 原版中位数 | 实验版中位数 | Git 进程数 | 决定 |
| --- | ---: | ---: | --- | --- |
| 十个 nativeTaskFixture：一次只读种子、逐例独立复制 | 7.543 秒 | 4.172 秒 | 80 → 27 | 纳入；三对分别减少 41.9%、46.3%、61.4% |
| 指定 DAG 用例：保留跨项目命令接线，五个规则分支用既有校验函数 | 19.402 秒 | 9.161 秒 | 143 → 60 | 纳入；三对分别减少 56.3%、51.4%、56.8% |
| 把 Git 代理放进独立轻量程序 | 34.093 秒 | 27.487 秒 | 153 → 153 | 不纳入；逐对改善仅 1.03%、19.37%、2.41%，额外程序收益不稳定 |

轻量代理的两组总体中位数差异容易掩盖运行间波动，因此同时保留逐对比例；没有据此把新增程序加入任务。只读种子减少真实准备操作，DAG 分层减少与这些断言无关的重复 Git blob 核验，不跳过生产验证。

DAG 对照用例为 `TestTaskBatchRejectsDuplicateAndCrossProjectDependencies`。首个跨项目拒绝仍经过真实 CommandService；契约 ID 冲突、重复边、自环、缺前置及已有节点作为新后继五个反例，使用真实 SQLite 图快照与 `validateTaskBatchAgainstGraph(..., false)`，契约规范摘要及全部原错误断言保留。其他真实 Git/事务/完整闭环测试未删除。

证据：[配对汇总](paired-probes/summary.json) · [实验设计](probe-design.md) · [种子补丁](probes/seed.patch) · [规则分层补丁](probes/layered.patch) · [被排除的代理补丁](probes/helper.patch) · [配对运行器](probes/run-paired.py)。完整 stdout 和 Git TRACE2 日志位于 `paired-probes/`。

## 隔离与方案边界

种子实验另外执行 A/B 独立 worktree/remote 检查，以及原有创建原子性/幂等查询、领取竞争回归，结果见[隔离检查](seed-isolation/summary.json)和[原始测试输出](seed-isolation/go-test.jsonl)。这些定向验证不等于完成所有新夹具实现的全量验收；正式实现仍须按草案跑完整回归。

首轮三进程完整对照未应用夹具优化；当时夹具和 DAG 优化只做上述定向配对及隔离检查，不将几项百分比相加推算完整耗时。后续组合完整实验见下一节。十夹具中位数不高于原版 75%、指定 DAG 用例不高于原版 70% 的门槛保留，留出已观察到的运行波动。

临时 overlay 与测试程序位置记录于 [probe-manifest.json](probe-manifest.json)。源变更补丁已保存；临时目录将来若不存在，可在对应源码的隔离副本中应用补丁、重新准备测试程序后复现实验。`combined.patch` 仅为实验准备产物，包含已排除的代理方案；没有用于完整测试结果，也不是最终实现清单。

## 进一步精简与最终组合

用户要求继续减少不必要的长测试后，对最慢用例逐段核对覆盖。已确认三处可精简：原子创建用三节点取代五节点、图展示去掉重复环批次并将无副作用检查移至专属测试、配置选择用例直接准备合法任务记录。两个决策用例的类似准备替换未取得稳定收益，最终恢复原实现。最长闭环内的恢复、未知启动、清理和精确停止检查有独有覆盖，本轮没有整段删除。

详细原断言到存活检查的映射与定向前后数据见[精简审计](reduction-audit/coverage-map.md)。此外，原哈希分组把两个最长闭环放在同一组，造成约 140/307/215 秒的尾部等待；用本轮已记录的顶层耗时作为固定权重做降序最小负载分配，无需再运行一遍测试来决定分组，新测试仍自动进入。

最终组合仅应用测试 overlay：上述精简、前轮已验证的 Git 种子和五个规则反例分层、固定权重三个进程。全套执行 **264.871 秒**，核心并发窗口 **222.327 秒**；三组分别 88/90/92 个顶层用例，实际约 221.8/222.3/210.3 秒。全部 **460** 个测试/子测试终态仍为 **443 通过、17 原有跳过、0 失败**，原顶层名称、动态子测试及其他包的状态全部核对一致。

相对 349.326 秒的第一轮分组实验，进一步减少约 24.2%；相对串行 658.633 秒，约减少 59.8%。这是组合效果，包含减少无关准备和降低分组长尾，不能把节省都算作删测试。该计时仍不含预先准备核心测试程序，产品 Go 源码未改。[完整结果](reduced-balanced-full/summary.json) · [分组与固定权重](reduced-balanced-full/assignment.json) · [实验补丁](reduction-audit/combined-prototype.patch)。

上述组合完整实验补充了前一节的局部证据。草案因此增加参考机完整执行 **≤300 秒** 的明确目标，同时保留同源码串行 75% 和夹具/规则组门槛；不通过跳过独有场景、改生产超时或降低 SQLite 耐久性达标。

汇总决定见 [decision-summary.json](decision-summary.json)。最终方案和顺序已固定到草案；实现任务中的计时只用于验收。仍未分配任务 ID、创建中心任务或修改 DAG。
