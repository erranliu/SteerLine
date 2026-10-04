# #73 纯类型与契约归属整理验收

Windows amd64 / Go 1.26.4，2026-10-04。从最新 origin/main `95237e6abc76fbee743ebc347aa1ef599a76efab` 开始；分支 `codex/three-line-types-73`，worktree `C:\Users\lmas1\.codex\worktrees\a562\SteerLine`。各 runner 使用独立 STEERLINE_HOME 子进程环境，无原生 opt-in；宿主环境未持久改写。正式 runner 报告如实记录基线 commit 与脏工作树，最终源码由 [source-index.json](source-index.json) 的逐文件 hash 固定。

固定执行依据：[Spec](../../spec/three-line-01-types.spec.md) / [Plan](../../spec/three-line-01-types.plan.md)；任务身份 73 保留。

## 最终行为与责任

纯任务/DAG、执行记录与状态规则、实现交互契约、独立产品输入/回执/目标、窄存储声明分别由 task/model、execution/model、execution/backend、product/api、storage/ports 唯一定义；prompt 由 execution 用例拥有。SQLite、query/projection、三个实现、app 局部构造、client、cmd 与 console 的生产/测试消费者同次切换。迁出类型 alias、常量和纯函数转发已删除；数据 JSON、schema 13/runtime 18/profile v6、领取/base/候选/Execution/session 身份与原子提交保持。

TaskGraph 从同一次原读快照复制任务级 DeliveryFact；不解码 PR/session，也不授予关闭权限。原 PR 核验、唯一交付与任务关闭/DAG 版本事务仍由现有用例/SQLite 执行。宽产品/连接/Runtime 和任务混合 authority/PR 核验保持唯一旧定义，逐字段后继见 [ownership.md](ownership.md)、[residual-ownership.json](residual-ownership.json)。

TM01.1：生产 import 与构造清单见 [consumers.json](consumers.json)，逐迁移定义见 [migration-inventory.json](migration-inventory.json)。TM01.2：迁入规则、固定契约/身份/回执 JSON 与原候选屏障断言通过；backend 仅标准库，execution/model 只依赖 backend，api 没有内部记录。TM01.3：CD67 检查全 OS/tag 生产和测试源码，以及 Windows/Linux 直接/传递生产依赖；保留原反例并新增纯包跨域/旧聚合/SQL/实现/app/helper/外部模块反例。源码和当前架构索引已同步。

## 真实验证

| 命令 | 结果 | 证据 |
| --- | --- | --- |
| `go run ./cmd/test-feedback -out docs/validation/three-line-01-types/run-20261004/rules -- ./internal/task/model ./internal/execution/model ./internal/execution/backend ./internal/storage/ports -count=1` | 4 包，74 parent/subtest run/pass，0 skip | [rules](run-20261004/rules/summary.json) |
| `go run ./cmd/test-feedback -out docs/validation/three-line-01-types/run-20261004/boundary -- ./internal/steerlineintegration -run '^TestCD67CoreBoundary$' -count=1` | 32 parent/subtest run/pass，0 skip | [boundary](run-20261004/boundary/summary.json) |
| `go run ./cmd/test-feedback -out docs/validation/three-line-01-types/run-20261004/consumers -- ./internal/steerline -run '^TestMC(1|4|8|9)$' -count=1` | 11 parent/subtest run/pass，0 skip；约 290 秒 | [consumers](run-20261004/consumers/summary.json) |
| `go run ./cmd/test-feedback -out docs/validation/three-line-01-types/run-20261004/fixture-lock -- ./internal/steerline/testprotocol -run '^TestTM01FixtureLockProcessDeath$' -count=1` | 活进程互斥、强制终止释放及锁清理通过 | [fixture-lock](run-20261004/fixture-lock/summary.json) |
| `go run ./cmd/test-feedback -out docs/validation/three-line-01-types/run-20261004/fixture-recovery -- ./internal/steerlineintegration -run '^TestCD67AdditionalImplementation$/^supported_terminal_recovery_and_original_identity$' -count=1` | 原恢复 parent/subtest 全部通过、0 skip；约 91 秒 | [fixture-recovery](run-20261004/fixture-recovery/summary.json) |
| `go run ./cmd/test-feedback -level=L1 -out docs/validation/three-line-01-types/run-20261004/l1-after-fixture-fix` | 完整固定选择通过，231 parent/subtest run/pass，0 skip；约 222 秒执行 | [最终 L1](run-20261004/l1-after-fixture-fix/summary.json) |
| `go build ./...`（Windows 与 GOOS=linux） | 两目标退出 0；Linux 只编译/依赖，不声称运行 | [build](run-20261004/build/result.json) |
| `go test -tags=e2e ./tests/e2e/... -run '^$' -count=1` | 带 tag 调用者编译通过，未运行场景 | [e2e compile](run-20261004/e2e-compile.txt) |
| `go test ./cmd/test-feedback -count=1` | runner 实际路径/选择/覆盖校验通过 | 执行工具输出；定向记录同步 test-mapping.json |

原始 Go JSON/stderr 均随各报告保留。逐包/parent/subtest 的 run/pass/skip 数见 [coverage.json](coverage.json)，原/新测试父名称和断言所有者见 [test-mapping.json](test-mapping.json)。迁入规则父/子名称保持；组合 task 测试保留在旧 task 包验证 #74 交付门槛。编译零命中明确不是行为通过证据。

首次固定 L1 的 [失败报告](run-20261004/l1/summary.json) 保留：CD67 原受控 terminal 恢复在服务取消终止持锁 fixture 命令后，O_EXCL 文件锁未删除，随后停止/资源核验报锁超时；其余六组和纯包守卫通过。只修复 Windows testprotocol 锁为独占/delete-on-close 句柄，并增加进程终止回归。原 session、目标、恢复人工门槛、原请求和停止/清理断言未削弱。上述独立恢复复验和故障修复后的固定 L1 均通过；没有覆盖/改写第一次失败来伪称通过。非 Windows fixture 行为保持原实现。

## 自查、复用与限制

完整实际 diff 逐类型/消费者/调用路径核对；[diff-audit.json](diff-audit.json) 辅助对比保留声明与迁入所有者（规范化 import/已删除转发名称/格式，空白接口实现断言由编译验证）。真实业务调用除纯 Snapshot 投影外仅切换定义/规则所有者；21 个旧纯转发/常量/错误句柄删除。提交协调、intent → 副作用 → 原 receipt/binding → 下一步、未知结果只读对账、唯一交付、同快照和身份/版本条件保持。

复用 #37 已有候选基础证据；本轮又定向检查原 MC1/4/8/9 身份/唯一交付/迟到结果/名额断言。结构/import/构造变化不新增原生 launch、权限或协议能力，因此未跑 Full/L2/L3/live，也不把 mock 或 Linux 编译当作真机认证。planning/HTTP/SSE 未实现；#39 远端权限门槛仍按该任务要求验证，#73 不声称消除它。

#73 无未解决实现问题。#74 得到混合 ProjectTaskAuthority/StartupBlockers/DeliveryGate/PR 证据/复合回执的唯一残留和公共 Service/Config/组装段清单；#77 收窄产品，#79 拆存储/连接，#80 消除宿主剩余。后继仍需按固定 Spec/Plan 整体交付，不由本 PR 提前实现。任务身份保留；活动 roadmap/execution-map/Spec 索引移出 #73 并引用本验收基础。
