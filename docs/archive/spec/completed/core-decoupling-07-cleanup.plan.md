# Plan 主包解耦重构后代码清理

任务 ID：68；任务名称：主包解耦重构后代码清理；目标：`CORE-IMPLEMENTATION-DECOUPLING`。唯一执行依据：[Spec](core-decoupling-07-cleanup.spec.md)及[共同约束](core-maintenance-constraints.md)。硬前置：#67 整体交付；一个实现 PR。

## 实施顺序

1. 读取 #63–#67 最终代码和迁移清单，确认 #67 生产/测试隔离及构造链已交付；未完成的前置范围先修复，不能混为新的可删候选。
2. 用 rg、所有 OS 源码 import/符号和已有守卫盘点候选；补核查 CLI、脚本、嵌入资源和外层集成测试。形成每项删除依据及必要断言保留位置。
3. 按职责删除死代码/无用别名/转发，重复规则切到唯一实现；同步调用和受影响测试，保留恢复/未知结果/拒绝样本。
4. 更新当前模块职责、架构源码索引与测试入口。核查改动没有新增 Store/客户端搬迁或改变协议/格式。

## 验证与交付

CM68-1 对应删除清单、全部源码条件消费者审查及产品构建；CM68-2 对应 #67 既有守卫、受影响代表用例及一次 quick；CM68-3 对应文档/入口/测试映射与 diff。

在实施时按实际改动选择少量现有用例，记录每个所选入口实际命中，不重复 #67 完整扩展性验证或全量测试。quick 已包含守卫时不再另跑重复组；未包含则用现有定向追加方式接入。运行 git diff --check。

PR 提供删除项及依据、唯一所有者、旧到新断言映射和实际验证结果；无真实删除候选的类别如实说明，不制造删除数量。向 #69/#71 交接一致基线与模块修改归属。规划阶段不运行上述代码验证，不创建中心任务。

## 测试计划

实施起点：`d8bacbd75994a183c6d54c027abe103d26ae6896`，平台确认 #67 PR190 已真实合并。删除项固定在 [候选清单](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/core-decoupling-07-cleanup/candidate-inventory.json)，消费者、保留项和断言归属见 [交付记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/core-decoupling-07-cleanup/README.md)。不新增场景或测试环境能力。

| 级别 / 组或场景 ID | 环境、命令 | 时点 | 通过条件 / 证据 |
| --- | --- | --- | --- |
| L1 定向：中心 receipt/intent/rollback、配置复制 | Windows；隔离 Git/SQLite，中性 fake；`go run ./cmd/test-feedback -out bin/cd68/focused -- -p 1 -parallel 1 ./internal/steerline -count=1 -run '^(TestWorkerReceiptFailureRetainsIntentAndCancelsUnregisteredObserver\|TestWorkerIntentIsDurableBeforeDispatchAndFailurePreventsDispatch\|TestWorkerCommitRollsBackOnStaleRecordAndCheckpointFailure\|TestWorkerFlowContextsDoNotExchangeExecutionProfiles)$'` | 清理稳定后、quick 前 | 包及各 parent 有 JSON run/pass，complete/pass，0 fail/skip；`bin/cd68/focused/summary.json`，归档命中至交付目录 |
| L1 既有 quick：中心、纯规则、实现、应用、CD67 守卫 | 同上；协议 mock；`go run ./cmd/test-feedback -quick -out bin/cd68/quick` | 最终代码候选上一次 | 15 包各一次，指定 parent/subtest 实际命中，complete/pass、0 fail/skip；`bin/cd68/quick/summary.json`。quick 已含完整 #67 守卫，不重复独立跑守卫或第三实现矩阵 |
| 静态 CM68-1 / CM68-3、产品构建 | 全 tracked OS/tag Go、脚本、嵌入资源、协议与反射消费者；`go build ./cmd/steerline ./cmd/test-feedback ./cmd/orca-probe`；Windows 与 Linux 产品交叉构建；`git diff --check` | 删除前审查，提交前复核 | 无遗失消费者/协议入口、无前置边界缺口；构建 exit 0、diff 无错误；交付目录 `validation-results.json` 与源码/import 索引 |

每条测试命令显式将 `STEERLINE_HOME` 指向本次临时目录；不启用原生/live 开关，不调用真实项目服务/用户 CLI/任务库或 `C:/SteerlineRuntime`。测试只使用自身临时现场，不恢复或覆盖历史配置。task/readiness 与 projection 代表已由 quick 覆盖；原生身份、未知回执、人工门槛、旧格式拒绝、权限拒绝和唯一 TaskDelivery 复用 CD67 既有组合。仅删除未调用 helper，没有测试断言删除或迁移。

未选 full/L2/L3/live：本次不改变业务行为、启动/权限映射或环境能力；无需真实模型、账号、新平台或完整故障矩阵。
