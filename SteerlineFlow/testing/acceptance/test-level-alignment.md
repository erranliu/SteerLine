# 测试分级与 L4 入口验收

日期：2026-10-16。普通开发，用户已授权实施，归属 [Plan](../../plans/test-level-alignment.plan.md)，无 Task ID。采用 [通用规范](../../../SteerlineProjectGuideDoc/testing-architecture.md)，本次不修改产品执行/恢复行为。

| ID | 自动验收项 | 状态 | 证据 |
| --- | --- | --- | --- |
| TLA-A1 | L4 选择/报告、跨级与错误输入拒绝、L2 gate、受控并行与失败/取消 | 通过 | owners.jsonl 与 boundaries/L4/summary.json；L2 gate 失败/清理失败阻断、跨包重叠与失败/取消均实际命中 |
| TLA-A2 | L3 三项正常流程实际通过，原事实/独立操作与并行、清理证据 | 通过 | `flows/L3/summary.json`，三项 scenario 全部 passed/cleanup_complete=true；两包起止时间实际重叠 |
| TLA-A3 | L4 四项边界实际通过，独有断言、selected-L4 与清理完整 | 通过 | `boundaries/L4/summary.json`，四项 scenario 全部 passed/cleanup_complete=true，0 skip |
| TLA-A4 | fixture/依赖边界定向测试、E2E 编译、登记/清单一致及文档检查 | 通过（两个无关调试包测试按原前置跳过） | owners.jsonl、catalog-doc.jsonl、e2e-compile.log；下方保留跳过范围 |

## Runner 重合核对

对照 Runner clean commit `46f0946` 的 `tests/catalog.json`、`tests/integration/stability_test.go`、`internal/execution/reliable` 与 `workflow`。Runner 拥有调用方退出、连接重连、服务换代/显式恢复、停止撤销、发布未知及原生执行规则。SteerLine 上一轮已移出相应专项，明细见 [历史精简](runner-test-convergence.md#28-项原注册场景去向)。拆分前的五项 L3 未发现可由 Runner 等价替代而整项删除的测试；本次按职责拆分改级，不把移动或历史删除计为新删测。

保留 caller 原请求、授权、结果消费、唯一交付及停止证明核对；构建/配置/SQLite、SteerLine 宿主和 Task 命名空间不由 Runner 覆盖。完整 L2 仍是必要的跨产品接线与交付冒烟。

## 执行记录

源码基线 `3dcbdc4146a0a9dbca63bb64fd981d712ee0cded` + 验证时工作树改动（dirty=true）。证据根目录：`C:/Users/lmas1/AppData/Local/Temp/steerline-level-alignment-35b903b68f814db5b8321a9fc579306e`。分级 summary 另保存实际源码/dirty、EXE/测试二进制 SHA、选择、包起止时间与资源清理；编译与静态核对不计场景通过。

- TLA-A1/A4：`go test ./cmd/test-feedback ./tests/e2e/internal/... ./internal/testkit/boundary/... -count=1 -json`，7 包通过、169 个测试终态 pass（含子项）、0 fail、2 skip，原始 `owners.jsonl`。跳过的是既有 `TestDebugReleaseWebProjects`、`TestDebugReleaseRuntime`，需要新调试包前置，未运行也不计本次通过；本任务只使用分级隔离夹具，不生成或复用安装现场。
- TLA-A4：新增 `TestScenarioContentListMatchesRegisteredIDsAndLevels` 后单独运行 `go test ./cmd/test-feedback -run '^TestScenarioContentListMatchesRegisteredIDsAndLevels$' -count=1 -json`，通过；`catalog-doc.jsonl`。新增/删除/改级后 README 表格 ID、级别和唯一性与 manifest 不一致会失败，语义仍由代码审查核对。
- TLA-A4：`go test -tags=e2e ./tests/e2e/... -run '^$'` 通过；`e2e-compile.log`。PowerShell 脚本 AST 解析通过；文档导航 447 文件、2,650 本地链接、0 错误；任务编号静态核对和差异格式检查通过。
- TLA-A2：`scripts/test-runtime-isolation.ps1 -Level L3 -Suite 'recovery,runtime' -OutDir <证据根>/flows`，内部为 `go run ./cmd/test-feedback -level=L3 -suite=recovery,runtime -env=controlled -out=<证据根>/flows/L3`，独立同用户 Windows 进程。三项实际通过、0 skip、selected-L3、passed/cleanup_complete=true；执行 385.90 秒。recovery 包 182.42 秒、runtime 包 384.00 秒，两包在 2026-10-16 06:40:03 UTC 附近同时启动，各 `-test.parallel=1`；包间并行预算二，证据为 `package_workers=2` 和逐包 started_at/finished_at。每项业务期限保持原值，无延长或自动重跑。
- TLA-A1/A3：`scripts/test-runtime-isolation.ps1 -Level L4 -OutDir <证据根>/boundaries`，内部为 `go run ./cmd/test-feedback -level=L4 -suite=runtime -env=controlled -out=<证据根>/boundaries/L4`，同一独立进程入口。四项实际通过、0 skip、selected-L4、passed/cleanup_complete=true；执行 413.92 秒，`package_workers=1`、runtime `-test.parallel=2`。构建输出保护 203.10 秒、路径/SQLite 207.56 秒、宿主写入拒绝 180.01 秒、同 ID 碰撞/重放 183.99 秒；每项 context_error 均为 nil，原五分钟业务窗口不变。
- 最终逐项核对 L3 三项、L4 四项的 ID、passed、cleanup_complete 和非空 resources，与 summary 选择一致；派生核对表为 `verified-scenarios.json`。原 summary/scenario/raw log 保留。静态输出为 `doc-links.log`、`task-id-check.log`、`diff-check.log`，新增未跟踪文件另核对无行尾空白。
- L2 live、Full、完整 L1、Web/调试包业务未运行；L2 正常两阶段和公开 final/Git/PR/TaskDelivery 要求保留，选择与 gate 由 A1 验证。本次未调用真实模型或创建真实 PR，未改动安装目录与其他 Runner 工作区；原执行可靠性专项已迁出，不将历史删除重复记为本次删测。

本次自动验收 TLA-A1–A4 已完成，无新增人工待验。单轮场景通过不代表完成了长期运行稳定性验证。
