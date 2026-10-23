# 测试架构对齐验收（2026-10-23）

普通开发，按用户确认的 [统一架构](../../SteerlineProjectGuideDoc/testing-architecture.md)整理。源码基于 `c7f90e04e1d1414386c5c421b3536a7a5fce60c5` 加本工作树修改；报告前后均 dirty，不计产品中干净同 commit 的当前通过。

## 范围

项目指南精简为日常入口，拆出 L1–L4 列表和报告约定；JSON 与执行登记共 17 项：L1 1、L2 1、L3 5、L4 10（含 `go.full`）。L1 缩为任务规则、公开契约、原子写入、依赖边界及清单校验；原固定选择中的 148 个具名测试仍全部定义，完整 owner 回归可用 Go Full 运行。

L2 仅 `smoke.delivery` 受控冒烟，`full-L2` 按当前定义解释；三条真实模型链及三个共享构建目录场景的相应条目归 L4（测试管理真实链同时属于后者）。L4 包间串行，独立 runtime 边界可包内二并行，与项目维护同选时串行。更新独立进程脚本、跨级拒绝、清单映射和报告完整性校验。

## 验证结果

| 验证 | 结果与证据 |
| --- | --- |
| 最终 L1 | `go run ./cmd/test-feedback -level=L1`：passed/complete，准备 0.35s、执行 6.98s；`%TEMP%/steerline-test-feedback/20261023T024213.471465700Z/`，受管报告 `l1.minimum` 通过。 |
| 最终 L2 | `scripts/test-l2.ps1` 独立进程调用同一分级入口：仅 `TestDeliverySmoke`，准备 12.93s、执行 74.92s，passed/cleanup_complete、full-L2；`%TEMP%/steerline-test-feedback/20261023T024139.057614900Z/`，受管报告 `smoke.delivery` 通过。 |
| 执行器及夹具/驱动 | `go test ./cmd/test-feedback ./tests/e2e/internal/... -count=1 -timeout=3m` 通过。覆盖改级/环境/空选择拒绝、共享构建串行、全部 JSON 命令与列表映射、L4 Go 报告子集/跳过/零匹配拒绝。随后新增的生成请求与交付输入计数反例经 testenv 包及最终 L2 验证通过。 |
| 编译与文档 | 全部 E2E 包以 `-tags=e2e -run '^$'` 编译通过，不算场景通过；四个修改/新增 PowerShell 脚本语法通过。文档链接及 `git diff --check` 通过。 |

## 失败与修正

- 首次 L2：直接构建的受控 Runner 未声明已有的 generate 能力，自动材料失败；补齐公开目录及生成 schema。原报告 `20261023T023310.375884900Z` 保留，清理完成。
- 第二次 L2：新增目录回执遗漏必需的 null 字段，公开 CLI 拒绝；补齐五字段回执并增加直接 helper 回归。原报告 `20261023T023611.321194800Z` 与未核实清理的 `%TEMP%/steerline-e2e-2055177237/` 保留，不改记通过。
- 第三次 L2：原固定请求计数把材料生成算作重复交付输入；仅排除可核实属于 generate Run 的准备请求，仍检查交付 submit/stop，额外或未分类输入继续计数。反例与最终完整冒烟通过；原失败报告 `20261023T023828.012228400Z` 保留，清理完成。
- 驱动包原有 reset 断言要求环境检查表位于清理列表前两项，但 HEAD 已先清理 test_records；改查请求关联表先于其 owner 的相对顺序，不改变清理实现。修正后相关包通过。

## 未验边界

未运行 Go Full、L3/L4 全场景、真实模型/GitHub PR 或 Web 构建；不继承历史结果。configuration 与其他 L3 的并行组合尚待补齐运行证据。独立 Web/脚本入口仍按列表注明未接入统一受管报告。
