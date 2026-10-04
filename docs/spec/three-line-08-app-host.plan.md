# Plan：应用组装与服务宿主收口

任务 ID：80；任务名称：应用组装与服务宿主收口。唯一执行依据：[Spec](three-line-08-app-host.spec.md)。硬前置：#75、#76、#77、#78、#79。一个独立实现 PR 交付。

## 实施步骤

1. 核对所有前置的局部接线与残留清单，先拆 host 的调度/路由小端口再迁 app 配置构造。

2. 迁服务/心跳/锁/安装及 cmd，复用冻结身份和原控制回执；同步平台资源/launcher/构建入口。

3. 清空并删除主包/contracts 与旧 app 门面，更新守卫、所有消费者及后续产品任务实际路径。

实施中维护 `docs/validation/three-line-08-app-host/ownership.md` 和 `test-mapping.json`：记录实际旧/新文件、类型/端口、构造消费者及迁入测试父/子名称、断言所有者。前置已迁的定义/接线不重复迁移。共享文件按 Spec 的唯一修改所有者顺序集成，每步消费者与测试立即可运行。

## 测试计划

以下新包及 `TestTM*` 是实施时需要接通的精确目标，不表示本次文档 PR 已创建测试或入口。优先将已有代表测试迁到真正所有者并保留断言，只补 Spec 的未覆盖缺口；不能新增仅镜像实现的测试。迁移影响命令时在同次实现中修订到唯一实际包，记录旧/新 parent/subtest，零匹配或 skip 不算通过。TestMC* 当前来源是 `internal/steerline/candidate_workers_test.go`。

环境：Windows 临时 Git/SQLite/受控后端，STEERLINE_HOME 指向本轮隔离目录并在结束后恢复；不设置原生 opt-in。下表输出路径为证据目录基名，每次使用新的空运行子目录，不覆盖前置报告。原始 Go JSON、runner summary、身份/版本/原请求回执和必要清理结果一并保留。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 Go：TM08.1–TM08.3 | Windows；隔离服务/SQLite/可控宿主，无模型 | `go run ./cmd/test-feedback -out docs/validation/three-line-08-app-host/host -- ./internal/app/host -run '^TestTM08(ControlIdentity\|Heartbeat\|Shutdown)$' -count=1` | 服务/路由迁移后 | 原请求/实例拒绝、心跳不改业务、退出不杀 worker/连接关闭代表 | `docs/validation/three-line-08-app-host/host/` |
| 定向 Go：TM08.1–TM08.2 | Windows；真实 app/cmd 构造与可控后端 | `go run ./cmd/test-feedback -out docs/validation/three-line-08-app-host/wiring -- ./internal/steerlineintegration -run '^TestTM08AppHostWiring$' -count=1` | 默认入口和安装路径切换后 | 从默认构造驱动实际服务/命令、恢复原身份；无旧门面 | `docs/validation/three-line-08-app-host/wiring/` |
| L1 固定最小入口 | Windows；隔离 STEERLINE_HOME，无原生 opt-in | `go run ./cmd/test-feedback -level=L1 -out docs/validation/three-line-08-app-host/l1` | 自查/定向修复完成后一次 | 现有规则/构造/守卫代表非零命中且通过，无意外 live | `docs/validation/three-line-08-app-host/l1/` |

边界类变更在本阶段同步既有守卫/负向注入及 L1 选择；扫描全 OS/build tag 源码，核对 Windows/Linux 生产直接及传递依赖。跨系统只做依赖图/编译不冒充运行行为。新增场景或环境能力须先登记包/测试/ID与命令，再执行；当前已注册 E2E 入口和环境约定见 [E2E README](../../tests/e2e/README.md)。

不选 Full/L2/live：宿主承载迁移通过受控进程和实际构造代表验证。#40/#41 的版本/24小时和 #39 原生验收各自负责；若实际恢复行为改变，定向追加已注册 L3 controlled recovery.waiting-merge-restart。

## 自查与交付

逐项核对 Spec 验收，保留提交屏障、同快照、身份/版本和唯一结果，删除迁出路径/无消费者桥接，核对最小命令实际命中。汇总 `docs/validation/three-line-08-app-host/README.md`，注明复用证据、定向重验依据及未具备能力。

#81 得到最终生产路径与测试边界；尚未归组的测试迁到当前实际所有者或外层，不能依赖被删主包。
