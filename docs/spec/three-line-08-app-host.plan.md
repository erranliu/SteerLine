# Plan：应用组装与服务宿主收口

任务 ID：80；任务名称：应用组装与服务宿主收口。唯一执行依据：[Spec](three-line-08-app-host.spec.md)。硬前置：#75、#76、#77、#78、#79。一个独立实现 PR 交付。

## 实施步骤

1. 核对所有前置的局部接线与残留清单，先拆 host 的调度/路由小端口再迁 app 配置构造。

2. 迁服务/心跳/锁/安装及 cmd，复用冻结身份和原控制回执；同步平台资源/launcher/构建入口。

3. 清空并删除主包/contracts 与旧 app 门面，更新守卫、所有消费者及后续产品任务实际路径。

实施中维护 `docs/validation/three-line-08-app-host/ownership.md` 和 `test-mapping.json`：记录实际旧/新文件、类型/端口、构造消费者及迁入测试父/子名称、断言所有者。前置已迁的定义/接线不重复迁移。共享文件按 Spec 的唯一修改所有者顺序集成，每步消费者与测试立即可运行。

## 测试计划

验证目标按最终所有者执行：宿主断言位于 `internal/app/host`，真实 app/host 组合断言位于 `tests/integration`。现有 `TestMC1`–`TestMC9` 已由前置迁入 `internal/execution/candidate_workers_test.go`；保持 execution 所有权，记录其真实 parent/subtest，不按历史根包路径重新归组。优先保留迁入测试的断言，只补 Spec 未覆盖缺口；不能新增仅镜像实现的测试。迁移影响命令时在同次修订到唯一实际包，零匹配或 skip 不算通过。

环境：Windows 临时 Git/SQLite/受控后端，STEERLINE_HOME 指向本轮隔离目录并在结束后恢复；不设置原生 opt-in。下表输出路径为证据目录基名，每次使用新的空运行子目录，不覆盖前置报告。原始 Go JSON、runner summary、身份/版本/原请求回执和必要清理结果一并保留。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 Go：TM08.1–TM08.3 与服务错误退出清理 | Windows；隔离服务/SQLite/可控宿主，无模型 | `go run ./cmd/test-feedback -out docs/validation/three-line-08-app-host/host/review-fixes-01 -- ./internal/app/host -run '^(TestTM08(ControlIdentity|Heartbeat|Shutdown)|TestServiceLifetimeCloseOnRunError)$' -count=1` | 服务/路由迁移后 | 原请求/实例拒绝、心跳只更新元数据、真实服务返回关闭存储、Run 错误仍只关闭一次并保留两个错误，不推断 worker 停止 | `docs/validation/three-line-08-app-host/host/review-fixes-01/` |
| 定向 Go：TM08.1–TM08.2 | Windows；真实 app/cmd 构造与可控后端 | `go run ./cmd/test-feedback -out docs/validation/three-line-08-app-host/wiring/commit-bound-02 -- ./tests/integration -run '^TestTM08AppHostWiring$' -count=1` | 默认入口和安装路径切换后 | 默认 app 构造接通 host、原服务身份/请求回执保持、没有旧 facade 导入 | `docs/validation/three-line-08-app-host/wiring/` |
| 定向 Go：TM08 execution scheduler | Windows；execution actor fake、隔离 timer/context，无模型 | `go run ./cmd/test-feedback -out docs/validation/three-line-08-app-host/scheduler/commit-bound-02 -- ./internal/execution/scheduler -run '^TestRun' -count=1` | scheduler loop 归 execution owner 后 | 取消时结束 actor 并等待 sources；未知 dispatch mode 在 actor 启动前拒绝 | `docs/validation/three-line-08-app-host/scheduler/` |
| 定向 Go：E2E SnapshotContext sample | Windows；独立 SQLite fixture 与隔离 STEERLINE_HOME，无模型/原生 opt-in | `go run ./cmd/test-feedback -out docs/validation/three-line-08-app-host/e2e-snapshot/commit-bound-02 -- ./tests/e2e/internal/testenv -run '^TestApplicationProjectSnapshotRejectsDifferentDatabaseRevisions$' -count=1` | E2E/host fixture 类型迁移后 | `SnapshotContext` 只接受同数据库身份/版本的产品页与执行事实 | `docs/validation/three-line-08-app-host/e2e-snapshot/` |
| 定向 Go：F1–F4 app ownership and lifecycle regressions | Windows；真实 app config、隔离 SQLite/STEERLINE_HOME、受控 Codex fixture，无原生 opt-in | `go run ./cmd/test-feedback -out docs/validation/three-line-08-app-host/review-fixes/app-final-01 -- ./internal/app -run '^(TestNativeProjectInitializesOnceAndRejectsMismatchedBinding|TestProjectConfigDecoderPreservesStrictSingleJSONValueAndBOM|TestRunConfiguredServiceRejectsBeadsBeforeStartupWrites|TestRunConfiguredServiceClosesTransferredStoreOnReturn)$' -count=1` | 配置/host boundary 和 app lifetime 修复后 | CAS 重放返回 `stale_version`；app 严格 decoder 保留 BOM 与单值规则；Beads 拒绝不创建 run state；真实服务返回后 SQLite 连接关闭 | `docs/validation/three-line-08-app-host/review-fixes/app-final-01/` |
| 定向 Go：F1 host dependency closure negative | Windows；合成 host→execution→workspace 图加仓库 Windows/Linux 依赖图 | `go run ./cmd/test-feedback -out docs/validation/three-line-08-app-host/review-fixes/boundary-final-01 -- ./internal/coreboundary -run '^TestTM08AppHostBoundary$' -count=1` | host 依赖守卫取消 closure skip 后 | 合成传递图命中禁用 workspace；实际 Windows/Linux host 闭包无违规 | `docs/validation/three-line-08-app-host/review-fixes/boundary-final-01/` |
| L1 固定最小入口 | Windows；隔离 STEERLINE_HOME，无原生 opt-in | `go run ./cmd/test-feedback -level=L1 -out docs/validation/three-line-08-app-host/l1/review-fixes-01` | 定向修复与自查完成后一次 | 固定规则、TM05/TM06/TM07、原组合 TM04、TM08 host dependency guard 与新 app/CAS/decoder/生命周期断言均非零命中且通过，无意外 live | `docs/validation/three-line-08-app-host/l1/review-fixes-01/` |
| 条件追加：L3 controlled `recovery.waiting-merge-restart` | Windows；原 Task/SQLite/Workflow 受控恢复现场 | `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled -scenario=recovery.waiting-merge-restart -out docs/validation/three-line-08-app-host/recovery` | 仅当实现实际改变恢复行为时 | 原身份、数据库、执行与工作区保留且不重派；否则不选择此组 | `docs/validation/three-line-08-app-host/recovery/` |

边界类变更在本阶段同步既有守卫/负向注入及 L1 选择；扫描全 OS/build tag 源码，核对 Windows/Linux 生产直接及传递依赖。跨系统只做依赖图/编译不冒充运行行为。新增场景或环境能力须先登记包/测试/ID与命令，再执行；当前已注册 E2E 入口和环境约定见 [E2E README](../../tests/e2e/README.md)。

不选 Full/L2/live：宿主承载迁移通过受控进程和真实 app 构造代表验证。#40/#41 的版本/24小时和 #39 原生验收各自负责；只有恢复行为实际变化才追加已注册的 L3 controlled `recovery.waiting-merge-restart`。L1 选择须保留 TM05/TM06/TM07、原 TM04 组合回归及新增 TM08 host/app 断言。

## 自查与交付

逐项核对 Spec 验收，保留提交屏障、同快照、身份/版本和唯一结果，删除迁出路径/无消费者桥接，核对最小命令实际命中。汇总 `docs/validation/three-line-08-app-host/README.md`，注明复用证据、定向重验依据及未具备能力。

#81 得到最终生产路径与测试边界；尚未归组的测试迁到当前实际所有者或外层，不能依赖被删主包。
