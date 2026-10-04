# Plan：纯类型与契约归属整理

任务 ID：73；任务名称：纯类型与契约归属整理。唯一执行依据：[Spec](three-line-01-types.spec.md)。硬前置：#37（已交付候选基础）。一个独立实现 PR 交付。

## 实施步骤

1. 按 import 和实际类型使用盘点纯值、状态规则、端口与残留聚合；提交 ownership.md，逐条指定后继所有者。

2. 先迁 backend，再迁 task/model 与 execution/model，拆开提示组装；按依赖顺序迁读取/提交契约和产品独立纯值。

3. 同次切换生产/测试/fixture 与 app 构造；扩展既有 CD67/产品边界守卫到新路径，不扩大准许依赖。

实施中维护 `docs/validation/three-line-01-types/ownership.md` 和 `test-mapping.json`：记录实际旧/新文件、类型/端口、构造消费者及迁入测试父/子名称、断言所有者。前置已迁的定义/接线不重复迁移。共享文件按 Spec 的唯一修改所有者顺序集成，每步消费者与测试立即可运行。

## 测试计划

以下新包及 `TestTM*` 是实施时需要接通的精确目标，不表示本次文档 PR 已创建测试或入口。优先将已有代表测试迁到真正所有者并保留断言，只补 Spec 的未覆盖缺口；不能新增仅镜像实现的测试。迁移影响命令时在同次实现中修订到唯一实际包，记录旧/新 parent/subtest，零匹配或 skip 不算通过。TestMC* 当前来源是 `internal/steerline/candidate_workers_test.go`。

环境：Windows 临时 Git/SQLite/受控后端，STEERLINE_HOME 指向本轮隔离目录并在结束后恢复；不设置原生 opt-in。下表输出路径为证据目录基名，每次使用新的空运行子目录，不覆盖前置报告。原始 Go JSON、runner summary、身份/版本/原请求回执和必要清理结果一并保留。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 编译：受影响跨平台/带 tag 调用者 | Windows 主机，Windows/Linux 生产；e2e tag 只编译 | `go build ./...`（GOOS=windows/linux）；`go test -tags=e2e ./tests/e2e/... -run '^$' -count=1` | 消费者切换后 | 构建退出 0；不计入场景非零命中/行为证明 | `run-20261004/build/`、`e2e-compile.txt` |
| 定向 Go：TM01.1–TM01.2 | Windows；纯规则，无模型 | `go run ./cmd/test-feedback -out docs/validation/three-line-01-types/run-20261004/rules -- ./internal/task/model ./internal/execution/model ./internal/execution/backend ./internal/storage/ports -count=1` | 归属和调用者切换后 | 迁入既有规则/契约断言通过，各包非零命中 | `docs/validation/three-line-01-types/run-20261004/rules/` |
| 定向 Go：TM01.3 | Windows；全源码及 Windows/Linux 依赖图 | `go run ./cmd/test-feedback -out docs/validation/three-line-01-types/run-20261004/boundary -- ./internal/steerlineintegration -run '^TestCD67CoreBoundary$' -count=1` | 守卫改为新旧路径后 | 禁止依赖反例与实际应用闭包通过 | `docs/validation/three-line-01-types/run-20261004/boundary/` |
| 定向 Go：生产消费者/候选屏障 | Windows；隔离 Git/SQLite/受控后端 | `go run ./cmd/test-feedback -out docs/validation/three-line-01-types/run-20261004/consumers -- ./internal/steerline -run '^TestMC(1|4|8|9)$' -count=1` | 自查后、一次 L1 前 | 原领取/多候选/唯一交付/迟到结果/原请求与数量断言非零 run/pass，无 skip | `docs/validation/three-line-01-types/run-20261004/consumers/` |
| 定向 Go：TM01.fixture-lock-process-death | Windows；真实受控子进程，无原生 opt-in | `go run ./cmd/test-feedback -out docs/validation/three-line-01-types/run-20261004/fixture-lock -- ./internal/steerline/testprotocol -run '^TestTM01FixtureLockProcessDeath$' -count=1` | L1 原 fixture 停止/清理故障诊断后 | 活进程互斥；持锁子进程被终止后可重新获取并清除锁，不改变恢复断言 | `docs/validation/three-line-01-types/run-20261004/fixture-lock/` |
| 定向 Go：CD67 受影响恢复 | Windows；原受控 terminal 协议 fixture | `go run ./cmd/test-feedback -out docs/validation/three-line-01-types/run-20261004/fixture-recovery -- ./internal/steerlineintegration -run '^TestCD67AdditionalImplementation$/^supported_terminal_recovery_and_original_identity$' -count=1` | fixture 锁修复后 | 原 session/目标/身份/原请求及停止清理断言全部通过，parent/subtest 非零且无 skip | `docs/validation/three-line-01-types/run-20261004/fixture-recovery/` |
| L1 固定最小入口 | Windows；隔离 STEERLINE_HOME，无原生 opt-in | `go run ./cmd/test-feedback -level=L1 -out docs/validation/three-line-01-types/run-20261004/l1-after-fixture-fix` | 自查/定向修复完成后一次 | 现有规则/构造/守卫代表非零命中且通过，无意外 live | `docs/validation/three-line-01-types/run-20261004/l1-after-fixture-fix/` |

边界类变更在本阶段同步既有守卫/负向注入及 L1 选择；扫描全 OS/build tag 源码，核对 Windows/Linux 生产直接及传递依赖。跨系统只做依赖图/编译不冒充运行行为。新增场景或环境能力须先登记包/测试/ID与命令，再执行；当前已注册 E2E 入口和环境约定见 [E2E README](../../tests/e2e/README.md)。

L2/L3/live/Full 不选：只有纯定义与调用路径迁移，业务、协议和发布无变化。复用候选交付证据；L1 保留实际 app 构造。

## 自查与交付

逐项核对 Spec 验收，保留提交屏障、同快照、身份/版本和唯一结果，删除迁出路径/无消费者桥接，核对最小命令实际命中。汇总 `docs/validation/three-line-01-types/README.md`，注明复用证据、定向重验依据及未具备能力。

#74 得到唯一类型与端口及明确残留；#38 可继续做交互，仅共享定义、import 与构造段由本任务顺序集成。

实施接线：规则命令对应四个真实新包；L1 同次迁旧 taskgraph/workerflow/execution，加入 ports 与旧 task/persistence 代表，app 构造组包含 CM71。product/api 的源码纯度及 Windows/Linux 闭包由 CD67 和 CM71 校验，无额外原生环境能力。TaskGraph 投影只搬已有同读快照事实；完整 ProjectTaskAuthority/Projects 保留唯一旧所有者供 #74/#79 拆分，详见 ownership.md。

L1 首次报告保留在 run-20261004/l1；其中原受控 terminal 用例在服务取消杀死持锁 fixture 子进程后遗留文件锁。Windows fixture 使用独占/delete-on-close 句柄，未修改生产或恢复验收。登记最小进程终止回归和原失败子场景；故障修复后的固定 L1 写入新的 l1-after-fixture-fix 空目录，不覆盖首次失败，不靠重试隐藏故障。Linux 保留原 fixture 行为，Windows/Linux 生产另做 go build ./...，带 e2e tag 的调用者只编译（go test -tags=e2e ./tests/e2e/... -run ^$），不声称场景运行。
