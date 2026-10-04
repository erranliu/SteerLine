# Plan：存储目录与消费者端口收口

任务 ID：79；任务名称：存储目录与消费者端口收口。唯一执行依据：[Spec](three-line-07-storage-ports.spec.md)。硬前置：#75、#76、#77。一个独立实现 PR 交付。

## 实施步骤

1. 按三线实际需求收窄端口/声明，移动 query 依赖的读取契约到 ports，保持生产闭包无反向依赖。

2. 迁唯一 sqlite 和所有 app 工厂；任务/执行/产品改为各自端口，移除全 Domain 读取。

3. 迁已有 SQLite 原子/只读/快照代表，更新 L1、schema 版本说明和消费者所有权。

实施中维护 `docs/validation/three-line-07-storage-ports/ownership.md` 和 `test-mapping.json`：记录实际旧/新文件、类型/端口、构造消费者及迁入测试父/子名称、断言所有者。前置已迁的定义/接线不重复迁移。共享文件按 Spec 的唯一修改所有者顺序集成，每步消费者与测试立即可运行。

## 测试计划

以下新包及 `TestTM*` 是实施时需要接通的精确目标，不表示本次文档 PR 已创建测试或入口。优先将已有代表测试迁到真正所有者并保留断言，只补 Spec 的未覆盖缺口；不能新增仅镜像实现的测试。迁移影响命令时在同次实现中修订到唯一实际包，记录旧/新 parent/subtest，零匹配或 skip 不算通过。TestMC* 当前来源是 `internal/steerline/candidate_workers_test.go`。

环境：Windows 临时 Git/SQLite/受控后端，STEERLINE_HOME 指向本轮隔离目录并在结束后恢复；不设置原生 opt-in。下表输出路径为证据目录基名，每次使用新的空运行子目录，不覆盖前置报告。原始 Go JSON、runner summary、身份/版本/原请求回执和必要清理结果一并保留。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 Go：TM07.2–TM07.3 | Windows；临时当前格式 SQLite，无模型 | `go run ./cmd/test-feedback -out docs/validation/three-line-07-storage-ports/sqlite -- ./internal/storage/sqlite -run '^(TestSQLiteTransactionRollsBackAllDomainWrites\|TestProjectViewSharesTransactionAcrossWriterCommit\|TestCM70ReadOnlyOpenPreservesFiles\|TestConversationWriteFailureRollsBackContentAndCursor\|TestTM07ResultCAS)$' -count=1` | SQL/端口与调用者切换后 | 迁入原回滚、同快照、只读、会话屏障代表；结果CAS/唯一性缺口补齐 | `docs/validation/three-line-07-storage-ports/sqlite/` |
| 定向 Go：TM07.1、TM07.3 | Windows；实际 app 连接生命周期 | `go run ./cmd/test-feedback -out docs/validation/three-line-07-storage-ports/wiring -- ./internal/steerlineintegration -run '^TestTM07StorageWiring$' -count=1` | 唯一工厂接通后 | 注入窄端口、连接唯一关闭且无写权限泄漏 | `docs/validation/three-line-07-storage-ports/wiring/` |
| L1 固定最小入口 | Windows；隔离 STEERLINE_HOME，无原生 opt-in | `go run ./cmd/test-feedback -level=L1 -out docs/validation/three-line-07-storage-ports/l1` | 自查/定向修复完成后一次 | 现有规则/构造/守卫代表非零命中且通过，无意外 live | `docs/validation/three-line-07-storage-ports/l1/` |

边界类变更在本阶段同步既有守卫/负向注入及 L1 选择；扫描全 OS/build tag 源码，核对 Windows/Linux 生产直接及传递依赖。跨系统只做依赖图/编译不冒充运行行为。新增场景或环境能力须先登记包/测试/ID与命令，再执行；当前已注册 E2E 入口和环境约定见 [E2E README](../../tests/e2e/README.md)。

不选 Full/L2/live：不改变协议或原生启动。L3 controlled 最终组合归 #82；若记录格式/恢复实际改变，追加 recovery.waiting-merge-restart，不扩大旧格式兼容。

## 自查与交付

逐项核对 Spec 验收，保留提交屏障、同快照、身份/版本和唯一结果，删除迁出路径/无消费者桥接，核对最小命令实际命中。汇总 `docs/validation/three-line-07-storage-ports/README.md`，注明复用证据、定向重验依据及未具备能力。

#80 接唯一连接与窄工厂/查询端口；#75/#76/#77 已整体交付才完整实施，接口草案不解除硬前置。
