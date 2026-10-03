# Plan SQLite写入实现归包

任务 ID：70；任务名称：SQLite写入实现归包；目标：`CORE-IMPLEMENTATION-DECOUPLING`。唯一执行依据：[Spec](core-decoupling-09-sqlite.spec.md)及[共同约束](core-maintenance-constraints.md)。硬前置：#69 整体交付；一个实现 PR。

## 实施顺序

1. 确认 #69 全部消费者已接通，按当前代码建立连接/schema/SQL/事务/错误/测试归属清单。存储请求/结果继续使用原纯契约，不通过中心别名产生依赖环。
2. 迁移连接/schema 和全部读写事务实现到唯一存储包，复用既有 Reader/投影。把当前事务内的权威校验和 CAS 一起迁移，业务规则和平台动作留原用例/实现。
3. 应用组装创建并持有具体连接，再向中心和客户端注入窄端口；切换服务/命令/离线查询/测试构造并删除旧 SQL/Store 入口。禁止长期双路和仅测试的任意写接口。
4. 测试按存储/业务/外层集成归位，核查 Windows 关闭、只读和 WAL；同步所有者、架构及 quick/守卫必要路径。

## 测试计划

环境：本机 Windows、Go 1.24+、临时 SQLite/WAL 文件；不启动真实模型。证据统一保存于 `docs/validation/core-decoupling-09-sqlite/`，原始 JSON 日志与命中统计一并保留。

| 级别 / 组或场景 | 命令 | 执行时点 / 通过条件 | 证据 |
| --- | --- | --- | --- |
| L1 定向：存储 CM70-2 / #69 已有回滚、CAS、幂等、交付、cursor，现有查询/格式拒绝及新增只读/WAL/关闭 | `go test -json ./internal/steerline/sqlite -count=1` | 迁移稳定后；所有迁入断言保留，命中非零、无失败/skip；文件不变、同快照、Windows 可删除 | `storage.jsonl`、测试映射 |
| L1 定向：中心 CM69 声明消费者、原子屏障及恢复代表 | `go test -json ./internal/steerline -run 'TestCM69\|TestWorkerCommit\|TestConversation\|TestWorkerFlowDeliveryCommitsWithoutStepGatesAndUnlocksSuccessor\|TestNativeTaskCASGuardsAndDeliveryProjection\|TestDecisionAnswerFirstWinsIdempotencyAndConflictAreTransactional\|TestExecutionStopIntentAndUnknownReceiptUseOriginalBinding\|TestWorkerRecoveryAttemptCommitsAtomically' -count=1` | 接线完成后；每个选择实际命中，提交失败不发布、不推进、未知结果和人工门槛保留 | `center.jsonl` |
| L1 定向：实际应用构造与 #67 隔离守卫 CM70-1/3 | `go test -json ./internal/steerlineapp ./internal/steerlineintegration -run 'TestCD66\|TestCD67\|TestFileQuerySourceReadsCombinedProjectView' -count=1` | 构造完成及最新 main 集成后；实际连接由应用注入，Windows/Linux 依赖及全源码守卫通过，命中非零 | `wiring.jsonl` |
| L1 固定 quick + 受影响定向组 | `go run ./cmd/test-feedback -quick` | 稳定后一次；既有 quick 命中通过，构建与测试证据完整 | `quick/summary.json` 与原始日志 |
| 静态检查 | `git diff --check`、全构建条件 SQL/依赖/构造扫描 | 自查、提交前；中心无 SQL/具体连接，存储无反向依赖；唯一生产接线 | `README.md` |
| L1 协调复审守卫：`negative_source_fixtures/transitive_storage_edge`、`negative_source_fixtures/reverse_client_subpackage` | `go test -json ./internal/steerlineintegration -run '^TestCD67CoreBoundary$' -count=1`；`go test -json ./cmd/test-feedback -run '^Test(QuickBoundaryCoverageRequiresEachAssignedEntry|ScopedCoverageRejectsMissingDuplicateAndTruncatedEvents)$' -count=1` | 修复确认的全标签传递/子包漏检后；新增反例先失败，修复后全守卫与 mandatory-entry 检查通过，原严格只读断言保持失败/待决 | 协调 run 的 `cm70-boundary-negative-before.jsonl`、`cm70-coordinator-boundary-fixed.jsonl`、`cm70-coordinator-quick-selection.jsonl` |

未选 L2/L3/full：本项只迁移既有存储职责，未改变真实执行启动、权限或平台认证；既有受控事务断言和实际应用构造足以验证受影响边界。若发现接线缺口，仅补必要场景；不扩展测试环境能力或重复故障矩阵。发布前核对并集成最新 origin/main，客户端线唯一构造如已合并须共同验证。

## 验证与交付记录

CM70-1 对应全体 SQL/连接消费者与反向依赖扫描；CM70-2 复用 #69 的实际存储回滚/CAS/幂等/交付/cursor 代表断言，追加现有只读/WAL/连接关闭必要用例；CM70-3 对应产品构建、实际应用构造的一组最小闭环与 #67 守卫。

迁移时记录旧测试到新包/断言的映射，不能只编译而未执行或通过正则零命中。稳定后用现有 test-feedback 定向追加与 quick 合并一次验证，运行 git diff --check，不跑 full 或平台认证。

PR 列最终存储实现与构造所有者、原子写集合、删除清单、实际依赖和命中。客户端并行变更只在构造段协调，公开产品 DTO 不改变；存储格式若无实际需求则保持原版本。规划阶段不运行代码验证或修改数据库。

实际命令/完整命中、29 个迁包测试的旧新映射及所有失败/修复见 [验证记录](../validation/core-decoupling-09-sqlite/README.md)。发布前已顺序集成 #71 PR195 和 #72 PR196 最新 main；新的共享构造只保留一条真实接线。CM70-2 严格只读文件要求未获用户例外批准，Spec/断言保持，不记整体通过；以 draft PR 等待决定与独立复审。
