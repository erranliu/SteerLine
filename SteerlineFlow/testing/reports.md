# 测试报告与 Git 历史

固定输出根、每级文件布局、临时 scratch 和仓库外并行目录见[流程路径清单](../paths.md)。本文件维护报告内容与 Git 历史规则，具体运行的绝对路径和源码版本记录在专属验收中。

分级入口保留 summary.json、JSONL 和退出码，并输出每个完整登记测试项的 managed-<id>.json。L1 为整体 ID，E2E 为实际所选场景，L4 Go 为 go.full；定向子集和 build-only 不输出整体通过。缺失、跳过、零匹配、超时、必要证据或清理不完整不能算通过。失败和补测分别保留。E2E 证据位于 `scenarios/<id>/scenario.json`，含实际断言、资源与清理事实。

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

## 候选证据

候选 PlanTests 继续消费原始 managed 报告及 Runner 公开 artifacts，按原 Run、输入、固定定义、Git 和清理事实核验；简洁历史不替代候选证明。生产者在开始前冻结 ID、时间、定义、选择、命令及仓库；结束后补结果、源码观察和 summary 字节长度/SHA256。定义摘要仍按原规范化 Definition JSON 计算，不散列清单排版。

系统自动安排仍读取固定版本 Plan、[测试指南](../testing.md) 和 test-list.json；无自动测试时 Plan 必须明确原因。运行内证据位于项目忽略的 `.steerrunner/artifacts/<唯一执行ID>/` 新目录；候选旧证据规则及公开收集所有权不变。实际结果仍写任务专属[验收记录](acceptance/README.md)。
