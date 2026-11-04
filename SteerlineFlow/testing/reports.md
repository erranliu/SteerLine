# 测试报告与 Git 历史

固定输出根、每级文件布局、临时 scratch 和仓库外并行目录见[流程路径清单](../paths.md)。本文件维护报告内容与 Git 历史规则，具体运行的绝对路径和源码版本记录在专属验收中。

分级入口保留 summary.json、JSONL 和退出码，并输出每个完整登记测试项的 managed-<id>.json。L1 为整体 ID，E2E 为实际所选场景，L4 本地回归按所选正式 ID 输出（go.full、固定 Go owner、Runner 契约或 web.unit）；未登记定向子集和 build-only 不输出整体通过。缺失、跳过、零匹配、超时、必要证据或清理不完整不能算通过。失败和补测分别保留。E2E 证据位于 `scenarios/<id>/scenario.json`，含实际断言、资源与清理事实。

## 写入 Git 历史

先提交代码，再运行测试，使用新的绝对 out 目录：

```powershell
go run ./cmd/test-feedback -level=L1 -out <新的绝对报告目录>
go run ./cmd/test-feedback record -report-dir <同一报告目录>
```

第二条只读取原报告、校验并写入 `SteerlineFlow/testing/records/<完整受测 SHA>.json`，不执行测试、不连接 SteerLine、不访问数据库、不自动提交或 push。提交该 JSON 随 PR 合入。不同 SHA 不写同一文件；相同 SHA 补测追加 run，相同报告重复导出不重复，相同时间不同结果拒绝。同目录并发写者明确失败，待原写者完成后显式重做导出。

格式只有 runs，每次执行包含 at（UTC 结束时间）及 results；结果项为 test_id、level、environment、status（passed/failed/incomplete）和可选 note。文件名给出 SHA；不保存本机路径、定义快照、命令、PR 或完整日志。例子及边界见 [Spec](../spec/test-records.spec.md)。完整日志留在本次报告目录。

只导出同仓库、同次执行、前后干净且未变的受测 commit 的完整登记项。记录目录自身变化不计源码脏；源码脏/未知/执行期间变动时报告只留本地。记录提交会产生新的 SHA，文件仍指向原受测 SHA；rebase/squash 后不改名假装验证新提交。

## 页面读取与清理

SteerLine 历史页面只读取已核实主分支 commit 中的 records 目录，未提交或尚未合入的文件不展示。目录缺失为空，坏文件报错；每个主分支 commit 解析一次并在内存缓存。通过 PR 删除旧文件即可清理页面历史，Git 仍可追溯。清单继续由 test-list.json 负责。

原始失败和后续通过分别展示，不自动建立复验链或判断失败已解决。历史通过仅证明当次记录的提交与环境，不代表当前主分支通过。定义页显示当前清单定义，简洁历史不冒充保存了旧定义。

原 tests register、record-project/rerun-of、登记回执、数据库历史和 Runner 历史投影已删除，不兼容旧数据。

## 候选运行时进度

候选消费冻结提示中的安排和真实 worktree 文档。运行时进度使用 Runner 绑定的 `STEERLINE_PROGRESS_FILE`，字段为 `test.<test_id>`；开始或重跑清零，完整通过才写 true。入口使用正式写入逻辑，加锁合并并原子替换；失败、跳过、零匹配或清理未核实均未完成。普通 L1–L4 入口的原始日志和报告仍保留。

候选查询通过公开 progress-file 接口核验原 Run、版本、长度和 SHA256，保留最后有效快照；断连或无效 JSON 单独显示诊断。候选不再收集 managed/summary，不调用 R2 或 workspace-git，也不从进度生成详细历史。独立测试历史沿前述 Git JSON 导出和主分支只读机制，详见[运行时进度](../../docs/integration/runtime-progress.md)。

## 正式本地回归

L4 go.http/go.storage/go.environment/go.deployment/go.roadmap/go.runner-contract/go.runner-result 及 web.unit 使用固定执行定义和独立 managed 报告，不能替代 go.full。Go 原始日志为 go-owner/test.jsonl，Web 为 web-unit/vitest.json、stdout.txt、stderr.txt。L1 的 quick-web 复用相同原生格式。Vitest 文件/断言集合须非零且完整，无 skip/todo；不合成 Go 事件。Go Full 在 packages_without_tests 中明确枚举没有普通测试的编译项，原生 package skip 只在该集合内允许；真实测试 skip 或未知包 skip 均使原 summary、退出码、managed 及进度不通过。


## 实际执行参数

每组原始command旁的execution_parameters记录command_kind和status，以及count、timeout、test_parallelism、go_package_parallelism、build_tags、test_filter、test_skip的state/value/source。显式argv为explicit；未显式参数为default且不推断子进程默认数值；不识别、未知自定义flag或不完整参数为unknown；冻结二进制的Go构建并发、Web的Go参数为not_applicable。GOFLAGS环境保留原值，未展开时不拿命令默认替代。-args/--转发尾段不当作可信测试参数；未知flag的值长度不能猜；test2json的-p仅是包名，test.p/test.tags也不冒充Go构建参数。

status=not_executed表示日志/前置等失败导致尚未尝试执行，attempted表示启动已尝试但没有进程终态，executed只在实际ProcessState存在时写入；executed并不表示测试通过，零匹配、skip、超时或原断言失败仍按原规则失败。每组从最终argv取值，诊断也使用追加-json及默认参数后的真实命令。测试二进制的build_tags和build_command来自实际构建记录：Full的core-build-command.json/integration-build-command.json，E2E的test_build_commands（同时保存在build/frozen-binaries.json），没有构建证据不能猜标签；Full缺证据进入原错误链。

顶层environment.concurrency的go_test_parallel/go_build_package_parallel仅聚合同一适用维度：共同显式值直接显示，不同为mixed，未执行/未知为unknown，全部不适用为not_applicable，原始分组仍是权威。controller_runtime_GOMAXPROCS只指报告控制器，GOMAXPROCS_environment是进程环境原值；两者不证明子进程实测GOMAXPROCS。scoped的workers/group_worker_limit是原调度上限，Full为三条core→integration串行流水线加other-packages共4条，不代表Go-p或某一时刻实测重叠；L1保持原上限，local串行为1。E2E的package_workers/package_worker_limit来自实际包调度策略，组内test.parallel另记，不扩大任何并发。

新字段属于原summary内容，并由原managed证据SHA覆盖。不新增历史报告迁移或兼容双读；报告参数修正不构成新性能、Linux或原生模型链通过证明。两个领域的断言与层级梳理、真实结果及未验边界见[专属验收](acceptance/app-domain-test-suites.md)。
