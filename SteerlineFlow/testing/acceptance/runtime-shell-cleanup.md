# 旧执行记录与空壳字段清理验收

2026-10-17，用户直接授权的普通开发；[Plan](../../plans/runtime-shell-cleanup.plan.md)。基线 `2fb07ba1`，当前开发分支 `codex/cleanup-runtime-shells`。实时验证结果只登记本文，不回写 Plan。

## 自动验收项

| ID | 内容及通过条件 | 状态 |
| --- | --- | --- |
| <a id="rsc-a01"></a> RSC-A01 | 删除对象的全源码引用闭包已核对；当前 Task/候选、checkpoint、产品/宿主值保留 | 通过：522 份 Go 源码审计 |
| <a id="rsc-a02"></a> RSC-A02 | 默认测试编译通过；固定 L1 非零匹配且 complete/passed、零失败/跳过 | 通过：53 包编译；L1 16 组 / 272 项全通过 |
| <a id="rsc-a03"></a> RSC-A03 | 文档状态校准、链接检查、diff 检查和提 PR 前自查通过 | 通过：本地链接无错误，差异与保留边界已自查 |

无新增人工项、真实模型或 PR 业务验收。本次开发 PR 不属于 SteerlineTest 业务链。

## 删除与保留边界

- 删除 3 个无调用函数：query.workflowSummary、query.executionDetails、sqlite.executionSessionFacts。
- 删除 Runtime 的 5 个旧记录集合，以及 RunView 的 6 个无读写字段（具体清单见 Plan）。它们没有当前生产填充入口或测试构造；删除后不保留旧 JSON 读取/迁移。
- 删除 model/records.go 中的 WorkflowRunRecord、ExecutionRecord、StepAttemptRecord、DecisionRecord、OperationRecord、WorkspaceEvidence，以及 ExecutionRecord 专属 JSON 解码。删除闭包中的 PolicyResources、StageInput、PendingQuestion、ManualStopConfirmation、backend.LifecycleOperationIdentity、backend.WorkspaceIntent。
- 保留实际使用的 StageResult、FixedExecutionProfile、DecisionRequest/State、PRState、TaskDelivery/ProjectTaskAuthority、ServiceCheckpoint 与当前 Runtime/Runs。保留同名 TaskRecord.IdentityMapDigest 的写入拒绝规则、旧类型名称的 DTO 禁入守卫和仍被消费的 backend wire 值/编解码测试。
- 数据库存储仍为 ServiceCheckpoint；应用从同一次读取的 Task 图与公开 Runner facts 投影 Runtime。删除内部空壳不变更 SQLite schema 22 或公开产品 DTO。

## 验证记录

Windows/amd64、Go 1.26.4；源码 `2fb07ba1e74dfe6b16326d7ed462da9c57a5dec0` 加本次 dirty 实现。下列运行后仅补文档证据与历史源码导航，生产源码未再修改。

报告根目录 `G = C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/`。

| 项目 | 命令/方式与结果 | 证据 |
| --- | --- | --- |
| RSC-A01 | 全部已跟踪 Go 源码精确词检索，含平台/build tag/测试；522 文件，仅 pages_test.go 的旧类型禁入字符串保留。Runtime/RunView 删除字段均已核对；检查 ServiceCheckpoint 读写入口确认持久格式独立 | `G/20261016T232827.407385700Z/source-audit.json` |
| RSC-A02 编译 | `go run ./cmd/test-feedback -build-only`：53 包，complete/passed，23.54 秒；零测试执行，11 个无测试包不作为场景通过 | `G/20261016T232827.407385700Z/summary.json`、`go-test.jsonl` |
| RSC-A02 运行 | `go run ./cmd/test-feedback -level=L1`：16 组、272 项（含子测试）全部通过，0 失败/跳过，complete/passed，执行 105.32 秒。包含 CandidateReadFactsAcrossViews 的全部 9 个输入子用例、SQLite/Task 规则、checkpoint/宿主与 Windows/Linux 公开依赖边界 | `G/20261016T232844.164969600Z/summary.json` 及各组 `test.jsonl` |
| RSC-A03 | `python scripts/check_doc_links.py`：463 份 Markdown / 2812 个本地导航链接 / 0 错误；`git diff --check` 通过。逐项自查生产 diff、保留类型/同名字段、文档状态及历史链接，不宣称独立复审 | 本聊天命令输出；静态检查日志保存在编译证据目录 |

首次文档检查发现两份 worker-staged-flow 历史 Spec/Plan 链接已删除的 records.go，随后仅将链接固定到原拆分前提交 `d20a72284705b4daac476d4fa5770151dca03e56`，核实该 Git 对象存在；契约文字与历史验收事实未改写。修正后的链接检查通过，首次两处缺失不计为通过。

环境检查导航依据其现有专属验收校准，明确固定版本本机 full_access 正例、已合入配置/报告刷新修复、其余 Runner 组合与完整人工体验边界；没有在本轮重验真实环境或扩大签收范围。架构重复 schema 21 改为引用数据模型的唯一当前格式表（源码为 schema 22），未修改 schema 本身。

未运行 Full/L2/L3/L4、Web 或模型/调试包；未新增镜像删除清单的测试，未接触安装环境。全部验证复用隔离夹具，无新增外部执行副作用。
