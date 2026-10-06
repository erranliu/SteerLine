# #75 任务应用用例交付候选

基线：main `c4650056d701a158353836616668bfad25011552`（#74 / PR214），开工时 HEAD 与最新 origin/main 相同。分支 `codex/task-usecases-75`；工作树 `C:\Users\lmas1\.codex\worktrees\a439\SteerLine`。固定依据为 [Spec](../../archive/spec/completed/three-line-03-task-usecases.spec.md)、[Plan](../../archive/spec/completed/three-line-03-task-usecases.plan.md) 与共同约束。本候选待协调独立复审/修复/合并，未合并，不据此启动后继。

创建/固定文档校验、批次和追加前置已经归 `internal/task`。真实 product/app/CLI 调用 task.Commands；原主包创建/追加依赖生产方法与文件删除。任务只持最小 Input、文档读取和任务事务小端口；app 投影 persistence，任务没有完整 Config/Runtime、执行模型/backend、SQL 或产品实现依赖。既有 Ready/Fixed/Claim/Declare/Intent/Discard 唯一规则承接 #74，不复制执行资格/PR/模式/容量/清理。

创建仍在同事务保存任务/固定契约/依赖/事件/回执，追加前置的审计仍在同事务；失败返回空成功回执。原请求重放先返回已接受回执，不做新文档读取或写事务。批次目标绑定及图/数据库版本来自同一读取快照，每快照只转换一次 TaskGraph。schema 14、精确候选身份、宽松五个开始检查、唯一结果/交付/关闭原子提交与原回执不增版本均保持。

归属和断言所有者见 [ownership.md](ownership.md)、[test-mapping.json](test-mapping.json)。全 OS/tag 401 个生产源码文件、33 包的 AST imports/hash 见 [source-index.json](source-index.json)，当前架构索引已更新且没有引用已删任务文件。

## 实际验证

Windows/amd64，Go 1.26.4。测试使用隔离临时 Git/SQLite/STEERLINE_HOME 和受控端口，不启用原生 opt-in、不请求模型、不使用生产 SteerLine 服务或中心任务库。所有最终选定运行都非零命中、0 fail、0 skip。

| 运行 | 实际命令选择 | parent/subtest run/pass | 证据 |
| --- | --- | --- | --- |
| 任务代表 | `go run ./cmd/test-feedback -out docs/validation/three-line-03-task-usecases/task-20261004 -- ./internal/task -run '^TestTM03(TaskBatch\|Dependencies\|Result)$' -count=1` | 5 / 5 | [summary](task-20261004/summary.json)、[Go JSON](task-20261004/go-test.jsonl) |
| 实际事务 | `go run ./cmd/test-feedback -out docs/validation/three-line-03-task-usecases/transactions-20261004 -- ./internal/steerline -run '^(TestTaskBatchDeterministicCyclePath\|TestCreateTaskBatchAtomicIdempotentAndQueryable\|TestCreateTaskBatchRollsBackEveryWriteWhenEdgeInsertFails\|TestNativeCommandsRejectConfigurationBindingDriftBeforeSideEffects\|TestTaskBatchRejectsDuplicateAndCrossProjectDependencies\|TestAddNativeTaskPredecessorsIsAtomicIdempotentAndBlocksClaim\|TestTaskContractFilesAndCanonicalDigest\|TestTaskContractRejectsCommittedSymlinkReferences)$' -count=1` | 7 / 7 | [summary](transactions-20261004/summary.json)、[Go JSON](transactions-20261004/go-test.jsonl) |
| 原固定文档消费 | `go run ./cmd/test-feedback -out docs/validation/three-line-03-task-usecases/documents-20261004 -- ./internal/steerline -run '^TestTaskContract(PinsCommittedBlobsAndRejectsInvalidReferences\|RejectsCommittedSymlinkReferences)$' -count=1` | 2 / 2 | [summary](documents-20261004/summary.json) |
| 完整 app/CLI 接线 | `go run ./cmd/test-feedback -out docs/validation/three-line-03-task-usecases/wiring-verified-20261004 -- ./internal/steerlineintegration -run '^TestTM03TaskWiring$' -count=1` | 1 / 1 | [summary](wiring-verified-20261004/summary.json)、[Go JSON](wiring-verified-20261004/go-test.jsonl) |
| 固定 L1 | `go run ./cmd/test-feedback -level=L1 -out docs/validation/three-line-03-task-usecases/l1-20261004` | 241 / 241；passed=true；约 235 秒 | [summary](l1-20261004/summary.json)，7 组原始 JSON/stderr |
| 最终唯一任务构造与内容能力 | `go run ./cmd/test-feedback -out docs/validation/three-line-03-task-usecases/final-construction-20261004 -- ./internal/task ./internal/steerlineintegration -run '^(TestTM03(TaskBatch\|Dependencies\|Result\|TaskWiring)\|TestCD67CoreBoundary)$' -count=1` | 40 / 40 | [summary](final-construction-20261004/summary.json)、[Go JSON](final-construction-20261004/go-test.jsonl) |

事务运行中的 `TestTaskContractFilesAndCanonicalDigest` 是不存在的候选名称，没有被记为命中或通过；原固定文档实际父名随后由 documents 运行明确验证，Plan 已登记正确名称。旧/新每项实际命中见 test-mapping 与原始日志。

L1 覆盖 quick-core 11、quick-rules 155、remote 15、codec 5、identity 3、app 7、boundary 45 个父/子测试；没有改 coreboundary 的断言、负向 fixture 或白名单。最后自查将 NewTaskService 变成任务输入/文档的唯一构造并由生产 NewTaskCommands 复用，批次直接调用公开 VerifyContractFiles；随后 final-construction 定向重新验证任务、真实接线与所有 OS/tag/Windows/Linux 边界，无需重复全量矩阵。

另实际执行 Windows/Linux 各自 `go list -deps -json ./internal/task` 和 `go build ./cmd/steerline ./internal/task`，均 exit 0。两份完整生产依赖图为 [windows](task-deps-windows.json)、[linux](task-deps-linux.json)，task 的唯一项目内依赖为 task/model。Linux 只记依赖/编译通过，不声称运行行为认证。CLI 本机生产二进制由 wiring 测试实际构建并执行。

固定文档新能力保留规范完整 commit、精确路径、regular blob、SHA-256 与验收标题边界；wiring 用真实 Git index 的 120000 link 验证拒绝，Windows 不需要 OS symlink 权限。app 追加依赖在 operations INSERT 上注入失败，验证图未修改且无成功回执；删除测试 trigger 后同一原请求经真实 CLI 重试/重放，版本一致。任务单位测试的窄端口不能代替这些真实数据库/文档断言。

## 诊断与自查

保留两次未通过运行：[wiring-20261004](wiring-20261004/summary.json) 的坏 SHA 样例先触发 AcceptanceRef 与 SpecRef 不一致的结构拒绝；修正样例同步引用，保留原结构守卫和 SHA 断言。[wiring-complete-20261004](wiring-complete-20261004/summary.json) 的测试 helper 从工作文件读 Git index link，改为对实际固定 link blob 内容计算引用摘要；regular-file 拒绝断言保留。中间 [wiring-final-20261004](wiring-final-20261004/summary.json) 和最终两个接线运行通过，未删除断言或放宽验收。

已读完整实际生产 diff 与调用路径，确认 task 无执行/SQL/app/product import；真实客户端/CLI 走新应用，task-only 回调不能获取执行记录或 PutOperation；事件/回执/图变更仍同事务。原主包跨执行测试仅由 `_test.go` 适配窄端口，生产没有旧方法转发。当前架构与唯一测试入口更新，活动规划移出 #75 并保留 ID、#79/#80 必要基础及用户暂停状态。未运行 Full/L2/L3/live：没有改变执行副作用/恢复/产品界面，也没有新增原生能力；#74 的专用结果/CAS/恢复证据保留，不冒充本次原生认证。

## #76 交接与停止条件

精确签名、构造字段、消费方最小修改和旧执行核验删除条件已发布在 [interface-handoff.md](interface-handoff.md)。`task.Service.VerifyContractFiles(context.Context, task/model.TaskContract) error` 是真实生产批次消费者使用的能力；NewTaskService 在 app 唯一构造最小输入与文档端口。当前 main 的领取仍保留实际旧核验消费者，#76 接消费方端口时顺序切换并删旧函数；不是永久 alias/转发，也没有默认 no-op 核验。两个旧共同构造调用形状继续正常构建。

本任务范围内无未解决实现/自动验证问题；跨分支仍需 #76 完成其拥有的领取端口、两个共同执行构造和新公共 Config 的顺序集成。#75 不修改 Service/CommandService、execution wiring、主 cmd/main、runner scoped 或共同存储端口，不把当前编译修复留给后继。提交 PR 后停止写入，等待协调独立复审/修复/合并；不启动其它聊天、不自行发消息、不合并 PR。
