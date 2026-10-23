# 架构解耦第三轮验收：会话与活动读取

日期：2026-10-17。归属：用户直接授权的第三轮实现，无 roadmap Task ID。源码基线：`2fb07ba1e74dfe6b16326d7ed462da9c57a5dec0` 加当前工作区前两轮及本轮变更。现状与图见[代码架构](../../../docs/architecture/current-code-architecture.md)，前轮证据见[第二轮验收](architecture-decoupling-round2.md)。

## 实现与范围

- SQLite 会话读取返回原始 Runner final 事件、数据库序号和项目/任务/候选/Run 绑定；筛选端口使用存储值类型。产品查询层校验原身份，再生成公开事件、正文引用及分页游标，沿用 v3 游标、revision 1 正文引用、原始 source/session/turn/item 和角色规则。
- 正文端口使用纯模型引用，仍校验原 Final、引用身份、字节数与 SHA256。产品查询层保留 16MiB 正文上限、256KiB 分页、UTF-8 边界及结果复制语义；能力报告也由查询层映射公开字段。
- 活动读取在同一只读事务中采样候选/Attempt 绑定并打开有界记录流。查询层完成公开 DTO、状态标签与精确 JSON 字节计量；流在成功、失败、预算截断及取消时关闭，关闭/提交失败不返回成功批次。使用流是为了保留原来的投影字节预算及读取范围，避免提前物化全部记录。
- 活动仍使用原候选/Run/Attempt 身份、递增游标、1MiB 公开事件预算；溢出项不推进游标。近期活动保留最近五条状态变化与十条技术记录，再按序号升序返回。会话保留原字节预算、超预算项延后校验和单条超大元数据可前进的规则。
- `storage/sqlite` 与 `storage/ports` 移除全部产品包 import；守卫禁止生产代码直接或传递依赖 `internal/product/*`，并覆盖未启用标签/其他 OS 源文件。仍复用 `execution/model` 等纯值，未声明所有跨模块所有权已解耦。
- 没有修改 schema、写事务、TaskDelivery 或 worker 生命周期。会话测试改用原始 Runner 事实夹具，没有新增旧格式读取、迁移或双读路径。

## 自动验收映射与结果

环境为 Windows/amd64，使用内存夹具、隔离临时数据库及源码依赖图；依赖守卫同时核对 Windows/Linux 生产依赖。均在实现后执行。

| ID | 通过条件 | 场景 / 入口 | 结果与证据 |
| --- | --- | --- | --- |
| DCR3-A01 | 会话原身份、final 验证、筛选绑定、游标、公开字段及精确分页字节语义保持，投影不修改来源 | `TestConversationQueriesUseIndependentEventPortAndBoundCursor`、`TestConversationPageBudgetDefersOverflowValidation`、`TestConversationReaderRejectsMissingOriginalProvenance`；C1 | 通过；E1 |
| DCR3-A02 | 正文引用、摘要、字节数、UTF-8 分页、读取上限及结果复制保持；能力报告只公开指定字段 | `TestConversationBodyQueriesUseIndependentPort`、`TestConversationCapabilitiesNeedNoUnrelatedReader`、`TestQueryEventCursorAndLargeBodyPaging`；C1/C2 | 通过；E1/E2 |
| DCR3-A03 | 活动 DTO/标签、预算、溢出游标及有界读取保持；读取/身份/取消/关闭错误释放资源且不返回部分成功 | `TestActivityProjectionBudgetStopsStreamWithoutAdvancingOverflow`、`TestActivityProjectionClosesOnReadIdentityAndCommitErrors`、既有 `TestStateNamingActivity*`、`TestTM05CanonicalActivity*`；C1 | 通过；E1 |
| DCR3-A04 | SQLite 读取仍绑定原 Final/调用者，活动绑定与记录来自同快照；并发写入留待下一次读取；真实取消后连接可复用 | 扩展的 `TestRunnerPublicReadsKeepOriginalFinalAndCallerBinding`；C2/C3 | 通过；E2/E3；取消子用例在 C3 验证 |
| DCR3-A05 | 既有同快照应用读取、候选视图和原子交付继续成立，HTTP 公开路由保持 | `TestRunnerApplicationSnapshotSharesTransactionAcrossWriterCommit`、`TestRunnerTaskDeliveryWithoutWorkflowReady`、`TestReaderPortsUseCurrentDatabaseAndPreserveReadOnlyBinding`、`TestCandidateReadFactsAcrossViews`、`TestFileQuerySourceReadsCombinedProjectView`；C2；`TestClientHTTPRoutes`，C3 | 通过；E2/E3；交付仅作回归 |
| DCR3-A06 | SQLite/存储端口生产依赖闭包不含产品包，别名、helper 与未启用源文件不能绕过 | `TestTM01StoragePortDependencies`、`TestTM07StorageBoundary`、`TestTM04ExecutionBoundary`；C1；`go list -deps` | 通过；E1，依赖闭包及直接 import 另存文本 |
| DCR3-A07 | 文档本地链接正确，最终差异无空白错误，修改的 Go 文件格式化 | 文档链接脚本、`git diff --check`、`gofmt -l` | 通过；465 份 Markdown、2831 个本地链接、0 错误；差异检查通过，格式检查无输出；E1 |

C1：3 包、62 项（含子用例）、0 跳过，19.473 秒。C2：3 包、18 项、0 跳过，18.501 秒。C3：2 包、7 项、0 跳过，4.915 秒。C3 重跑部分 C2 测试，以上不是独立用例总数。新增测试进入现有 L1 quick-rules/query/sqlite 与依赖守卫范围，不新增测试组。

本轮未执行完整 L1/Full、L2/L3/L4，也没有启动真实模型、调试环境、安装环境或真实 PR。改动集中在读取和投影，最小隔离测试已覆盖本轮自动验收范围。

## 证据

各目录含 `summary.json`、`go-test.jsonl`、`go-test.stderr.txt`：

- E1：`C:/Users/lmas1/AppData/Local/Temp/steerline-decoupling-round3-core-797ba668700947399d4479ac8a83a3b7/`；附 `sqlite-dependencies.txt`、`ports-dependencies.txt`、`storage-imports.txt`、`doc-links.txt`、`diff-check.txt`、`gofmt-check.txt`。
- E2：`C:/Users/lmas1/AppData/Local/Temp/steerline-decoupling-round3-storage-14b12d0332cd4a73a54bf7e111383214/`。
- E3：`C:/Users/lmas1/AppData/Local/Temp/steerline-decoupling-round3-close-http-02cceafdd32c4a11aed6d039a72bdfdb/`。

## 复现入口

每次输出到独立新目录：

```powershell
# C1：纯投影、预算、端口及依赖守卫
$coreOutput = Join-Path $env:TEMP ('steerline-round3-core-' + [guid]::NewGuid().ToString('N'))
go run ./cmd/test-feedback -out $coreOutput -- ./internal/product/query ./internal/storage/ports ./internal/testkit/boundary '-run=^(TestConversation.*|TestActivityProjection.*|TestStateNamingActivity.*|TestTM05CanonicalActivity.*|TestTM01StoragePortDependencies|TestTM07StorageBoundary|TestTM04ExecutionBoundary)$' -count=1

# C2：隔离数据库、同快照、原子交付及查询消费者
$storageOutput = Join-Path $env:TEMP ('steerline-round3-storage-' + [guid]::NewGuid().ToString('N'))
go run ./cmd/test-feedback -out $storageOutput -- ./internal/storage/sqlite ./internal/app ./internal/product/httpapi '-run=^(TestRunnerPublicReadsKeepOriginalFinalAndCallerBinding|TestRunnerApplicationSnapshotSharesTransactionAcrossWriterCommit|TestRunnerTaskDeliveryWithoutWorkflowReady|TestReaderPortsUseCurrentDatabaseAndPreserveReadOnlyBinding|TestQueryEventCursorAndLargeBodyPaging|TestCandidateReadFactsAcrossViews|TestFileQuerySourceReadsCombinedProjectView|Test.*(Conversation|Final|Body|Activity).*)$' -count=1

# C3：补充真实取消及 HTTP 契约
$closeOutput = Join-Path $env:TEMP ('steerline-round3-close-http-' + [guid]::NewGuid().ToString('N'))
go run ./cmd/test-feedback -out $closeOutput -- ./internal/storage/sqlite ./internal/product/httpapi '-run=^(TestRunnerPublicReadsKeepOriginalFinalAndCallerBinding|TestClientHTTPRoutes)$' -count=1
```
