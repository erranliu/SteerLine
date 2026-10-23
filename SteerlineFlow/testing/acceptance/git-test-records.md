# Git 测试历史验收

日期：2026-10-23。范围见[Spec](../../spec/test-records.spec.md)及[Plan](../../plans/test-records.plan.md)。本次替换旧数据库历史，不继承[旧版本验收](test-records.md)结果；不部署安装环境、不迁移旧数据库。

## 自动验收

| 项 | 状态 | 证据 |
| --- | --- | --- |
| GTR-A01 简洁格式与仓库导出 | 通过 | owner 定向与相关模块回归：简洁格式、固定/脏/变动源码、子集、同次报告、跨仓库、重复导出/冲突与失败保留 |
| GTR-A02 Git 只读与删除 | 通过 | `go test ./internal/app -run '^TestTR\|^TestTMPlan' -count=1`：临时真实 Git，无数据库依赖；未提交文件不读、坏 JSON 报错、删文件移除列表/详情，缺清单仍查历史 |
| GTR-A03 查询/Web/候选证据/schema | 通过 | 相关模块回归 279 通过、4 个默认跳过（DebugReleaseRuntime/SharedRunnerService/WebProjects、OwnedBranchCleanup，不计作对应能力通过）；Web 3 文件 20 项通过，生产构建通过；`go test ./... -run '^$'` 全包编译通过 |
| GTR-A04 隔离包真实脚本/Git/API/浏览器 | 通过（含清单展示补验） | 两次新包实际失败 exit=1、修正通过 exit=0；两提交两记录，重复导出不重复、未合入不可见、删除后剩一条；正常停止和资源结算均 true，末次另验证清单展示 |
| 固定 L1、L2 与文档 | 通过 | L1：32 项通过、准备 0.6 秒/执行 14.6 秒；L2：受控冒烟通过、准备 17.0 秒/执行 126.6 秒，清理完整；文档链接 0 错误及 diff 检查通过 |


## 原始证据

本次在源码工作树运行，未部署安装环境。输出均为新目录：

- 相关模块回归：`C:/Users/lmas1/AppData/Local/Temp/steerline-git-history-owner-1fa078d256444d5ca9e37800a7aaa94d/summary.json`，36.3 秒；包含 testmanagement、HTTP、SQLite、driver、test-feedback，原候选证据与实际模板脚本均回归。
- 固定 L1：`C:/Users/lmas1/AppData/Local/Temp/steerline-git-history-l1-e50fc14a22ac47bc88035f5e09561661/summary.json`。
- 固定 L2：`C:/Users/lmas1/AppData/Local/Temp/steerline-git-history-l2-5fc58f91f6ac466f8396609a55622384/summary.json`，coverage=full-L2、passed/cleanup_complete=true。
- 首次 L4：`C:/Users/lmas1/AppData/Local/Temp/steerline-git-history-browser-d6581ded0ad54b1486c1e1a3d9530340/summary.json`，coverage=selected-L4、passed/cleanup_complete=true。新包 `C:/SteerlineDebug/dev-20261023-125631-908c2570-dirty`，程序 `proof.json` 的 passed/stopped/resources_settled 均 true。浏览器真实流程 36.1 秒；截图时清单请求尚未完成，因此追加清单必须展示断言，以另一个新包复验。

- 最终 L4：`C:/Users/lmas1/AppData/Local/Temp/steerline-git-history-browser-final-7df28e2d3361408b90e0e0d2bdbab9f9/summary.json`，passed/cleanup_complete=true；新包 `C:/SteerlineDebug/dev-20261023-130040-908c2570-dirty`，proof 的 passed/stopped/resources_settled 均 true。浏览器 40.2 秒，清单与记录均展示后截图；原始失败和通过均可见，删文件后列表剩一次通过、原详情 404。

一次 PowerShell 未加引号的 `-scenario=testing.records` 被拆为额外参数，在创建现场前退出 2；改用 `-scenario 'testing.records'` 后正常执行，不记作测试失败或通过。

## 人工与边界

GTR-M01 页面密度人工体验后置。首次实现阶段的报告按真实脏源码状态保存；提交实现后另在干净固定 SHA 补跑 L1/L2，并通过正式命令生成随 PR 提交的历史文件，不把旧脏报告改写为固定源码。

## 提交前固定源码复验与本 PR 历史文件

实现提交 `84e57b8e968fd0a2e5627d4f2e428b58c15b312d` 后，在同一干净提交上运行固定 L1、L2，均通过；原 managed 报告前后 SHA 相同且 before_clean/after_clean=true。使用正式 `test-feedback record` 分别导出，两次执行合并到 `SteerlineFlow/testing/records/84e57b8e968fd0a2e5627d4f2e428b58c15b312d.json` 的两个 run，测试项为 l1.minimum 与 smoke.delivery。记录作为后续提交随同一 PR 合入，文件名保留实际受测源码提交。

- L1：`C:/Users/lmas1/AppData/Local/Temp/steerline-git-history-fixed-l1-f7d31681cf474179b9f61683f55b0aed/summary.json`，准备 0.4 秒、执行 12.2 秒，32 项通过。
- L2：`C:/Users/lmas1/AppData/Local/Temp/steerline-git-history-fixed-l2-b463e5b058454324be6da57a12d0fb17/summary.json`，受控交付冒烟实际用例 93.6 秒，passed/cleanup_complete=true。

导出 L1 后产生的历史文件不影响 L2 的源码 clean；这次真实使用也验证了记录目录排除规则和相同 SHA 下多次执行的合并。
