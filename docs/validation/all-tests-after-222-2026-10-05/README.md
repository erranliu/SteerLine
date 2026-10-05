# #222 后全量测试修复验收

2026-10-05，按用户要求 rebase 到 `origin/main` 的 `f8b5959462679faa4f00972f8f9e232044b6ab6c`，运行[测试总入口](../../testing.md)和 [E2E 指南](../../../tests/e2e/README.md)中的默认完整范围。最终代码版本为 `ab97fc706eb5df282d457dabacff5f44fff4826b`；本归档之后只提交验证文档。

## 修复

1. 主机测试 fixture 为 Git 文档读取注入 `ManagedProcessHost`，避免真实 Git 调用因端口缺失而伪装成合同不匹配；checkpoint 通过当前 `Store.Publish` 刷新心跳，避免较慢准备过程留下过期样本。
2. execution 的 capability 测试 fixture 显式注入文件资源端口，包含拒绝旧动作的负例，避免负例因缺少端口而假通过。
3. Full 的三个 worker 改为划分迁包后的 `internal/execution` 业务测试；`internal/steerline` 随其余包完整运行。报告记录实际 `core_package`，新增真实构建枚举与缺失、重复、错误所有者的覆盖检查。保留原有期限和所有用例。
4. 旧 schema 测试使用当前 checkpoint 合同，断言准确拒绝原因；篡改执行协议的测试检查当前固定 profile 绑定拒绝，同时保留无工作区写入和原记录不变的断言。写端口守卫跟随当前文件所有者。
5. E2E 在最后业务断言完成后记录 `body_finished_at`、冻结业务期限结果并取消业务 context，再执行独立服务停止和资源清理。清理跨过业务期限不会改变已完成的业务结果；业务超时或清理失败仍失败。增加未过期/已过期回归，重复清理保留原结果。
6. 候选 usage、冻结权限和原候选正文验证复用同一个已读取的候选 snapshot，减少重复 CLI/数据库读取；保留真实模型、原身份/回合、PR 和唯一交付断言，以及 20 分钟业务上限。

## 验证结果

| 范围 | 结果 | 报告目录名 |
| --- | --- | --- |
| 三 worker Full | 完整通过，1192 条父子测试记录，0 失败、18 默认 skip，执行约 46 分钟 | `full-owner-final` |
| L1 | 完整通过 | `l1-owner-final` |
| 完整 L2 | controlled 门槛和双真实 smoke，3/3 通过 | `l2-fixed` |
| L3 controlled | delivery / recovery，2/2 通过 | `l3-controlled-fixed` |
| L3 live delivery / recovery | 两种后端的交付和恢复，4/4 通过 | `l3-other-final`、`recovery-orca-rerun` |
| L3 live candidates | 1/1 通过，约 18 分钟，usage 与清理通过 | `candidates-efficient` |
| 最终带 tag 基础设施、runner、边界 | 完整通过 | `infra-efficient-final` |
| E2E 全包编译 | 通过；仅编译，不计作业务场景 | `e2e-compile-owner-final`、原始目录中的 `deadline-compile-final` |
| roadmap-runner 离线 Python 测试 | 18/18 通过，不执行调度工作流 | `python-efficient-final.txt` |

注册表的 10 个场景恰好全部覆盖，各场景 `passed=true`、`cleanup_complete=true`，真实场景 `usage_verified=true`。本轮最终 candidates 的真实测试 PR 为 [#114](https://github.com/erranliu/SteerlineTest/pull/114)、[#115](https://github.com/erranliu/SteerlineTest/pull/115)、[#116](https://github.com/erranliu/SteerlineTest/pull/116)、[#117](https://github.com/erranliu/SteerlineTest/pull/117)：114/117 已合并，115/116 已关闭，测试现场已清理。

Full 的 18 个 skip 按默认入口保留，包含平台条件、helper 和需要专门现场的原生 opt-in。未批量启用这些开关，skip 不表示这些独立原生检查通过。

### 版本与增量验证

各原始报告保留实际源码 SHA 和 clean 工作树状态；没有将历史通过结果改写成最终 SHA。Full/L1 基线在 `fbf34fa7`，L2/受控 L3 在 `75b28db1`，其余真实 delivery/recovery 在 `4f12bd3b`。这些版本之间仅修改测试 fixture、拒绝/守卫测试和 Full runner，产品业务实现与 E2E 业务断言不变。

Full 之后的改动仅涉及 E2E testenv 的业务/清理期限和 snapshot 复用，使用最终 `ab97fc70` 的带 tag 基础设施/runner/边界回归及完整 candidates 真实场景验证。没有将 1192 条基线记录声称为在最终 SHA 重跑的 Full。精确版本、命令、skip 清单、差异文件、报告路径与 SHA-256 见 [results.json](results.json)。

主要执行入口（每次使用新的 `-out` 目录）：

```powershell
go run ./cmd/test-feedback -workers=3
go run ./cmd/test-feedback -level=L1
go run ./cmd/test-feedback -level=L2
go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled
go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=live
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.orca-codex'
# manifest 固定本轮源码、本地 Codex/本机 Orca；通过 STEERLINE_CANDIDATE_ACCEPTANCE_CONFIG 输入
go run ./cmd/test-feedback -level=L3 -suite=candidates -env=live '-scenario=candidates.local'
go run ./cmd/test-feedback -- -tags=e2e ./cmd/test-feedback ./tests/e2e/internal/... ./tests/e2e/cmd/... ./internal/testboundary -count=1
go run ./cmd/test-feedback -- -tags=e2e ./tests/e2e/... -run '^$' -count=1
python -B .agents/skills/roadmap-runner/scripts/test_state.py
```

## 失败记录

保留早期 Full 的失败/超时、缺失端口、错误所有者/拒绝预期及首次 Orca recovery 失败。`candidates-owner-final` 的业务断言通过，但旧清理逻辑使最终场景报告超时，整轮仍计失败；`candidates-cleanup-boundary` 则在业务阶段真实超过 20 分钟，修正后的期限逻辑正确保留失败。最终通过来自新现场 `candidates-efficient`，没有延长期限、删除断言或覆盖失败产物。

原始证据位于 `C:\Users\lmas1\AppData\Local\Temp\steerline-all-tests-after-222-20261005-8ae29160`。仓库只保存筛选后的结果及哈希；认证、数据库、原始提示和 dispatch capability 不随 PR 提交。
