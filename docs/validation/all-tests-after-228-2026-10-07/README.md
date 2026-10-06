# #228 后全量测试修复验收

按用户要求 rebase 到最新 `origin/main`：`82ce43d9bec8f4950b1b784c948a9304b458ff17`（#228），根据[测试入口](../../testing.md)和 [E2E 指南](../../../tests/e2e/README.md)运行默认完整范围。2026-10-07 最终实现版本为 `120b54f3192d97917eeb3b6b40b7a1aba270e815`；之后只提交验证文档。

## 修复

1. `OpenNativeTaskCommandStore` 在打开写存储前验证 execution directory 和 profile 实现输入，拒绝未知 codec；只读查询和控制台仍能显示持久化数据。校验不提前执行 worker-start 权限准入，已知远程 profile 缺少执行证据仍在 claim 时拒绝。未知实现拒绝且数据库不变、远程 claim 拒绝且无副作用两个回归均通过。
2. 更新迁包后的测试夹具：显式绑定 publication/provider、runtime 和 requirements evidence；写端口守卫扫描当前 storage 所有者；host 测试使用当前错误类型、已解析身份和权限入口。保留别名隔离、无状态写入、身份拒绝和原回执不变断言，并补充 supplied snapshot 身份不符仍可只读显示的回归。
3. CLI 测试的 checkpoint 使用测试进程身份，没有真实服务循环；在较慢的准备完成后仅刷新其 heartbeat，避免正向连接检查读到过期样本。服务身份、control revision 和原请求/回执断言保留。
4. E2E readiness 改为本次项目数据库的原子组合快照，避免读取机器全局 profile 或修改进程全局环境。正式 task validate/create 后绑定 project/database ID，每次快照核对身份，省去仅为获取身份而重复执行的 task-list CLI；不缓存任务和执行状态。双项目隔离回归在故意无效的外部 catalog 下启动/停止两套服务。编译 context 仅用于编译，业务查询使用已有 scenario context。
5. Orca 环境短暂未就绪时只提交一次正式 `recheck_environment`，随后继续等待原异步操作，不重复业务输入；不支持重查的失败仍立即拒绝。修复以前重查后立即失败的问题，真实 Orca delivery/recovery 通过。
6. 候选反馈查询显式使用原 workflow run ID。原生 readiness 和业务 final 可能共享 turn ID，canonical final 按稳定 message item ID 计数并过滤重放；仍验证原 session/turn/body，完整正文和 usage 校验保留。增加同 turn 多 item、重放、fallback/snapshot 和缺失身份回归。
7. 控制台集成测试执行 Update 返回的后续命令及 Batch 子命令，避免 refresh 停留在未完成状态。五个受控候选进程的普通集成现场使用 5 分钟预算（原 3 分钟），单次等待仍为 30 秒；真实 E2E 的 5/10/20 分钟业务期限未修改。
8. Full 将 execution 和 integration 两个耗时包各自枚举、划分三组。每个 worker 顺序执行对应分区，其余包独立串行并发运行，最多四条命令同时执行。保留 50 分钟总期限、45 分钟单组期限和全部用例；增加真实编译枚举、缺失/重复/错误所有者及并发调度回归。解决用例本身通过、旧串行尾段却耗尽 Full 总期限的问题。

## 结果

| 范围 | 结果 | 原始报告目录 |
| --- | --- | --- |
| 最终三 worker Full | 完整通过，1233 条父子测试记录，0 失败，18 默认 skip；执行约 42 分钟 | `full-dual-final` |
| L1 | 459 条记录，0 失败、0 skip | `l1-01` |
| 完整 L2 | controlled 门槛及两条真实 smoke，3/3 | `l2-stable-final` |
| L3 controlled | delivery / recovery，2/2 | `l3-controlled-stable-final` |
| L3 live | 本地/Orca delivery / recovery，4/4 | `l3-live-stable-final` |
| 最终 L3 candidates | 1/1，约 16 分钟，usage 与清理通过 | `candidates-quiet-final` |
| 最终带 tag runner / E2E 基础设施 / 边界 | 161 条记录，0 失败，1 默认 sandbox preflight skip | `infra-owned-final` |
| E2E 全包编译 | 通过；0 业务测试，仅编译 | `e2e-compile-stable-final` |
| roadmap-runner 离线 Python | 18/18；未执行调度工作流 | `python-source-final.txt` |

注册表的 10 个场景恰好全部覆盖；通过报告均 `cleanup_complete=true`，真实场景均 `usage_verified=true`。真实环境使用 Windows、Go 1.26.4、Codex CLI 0.159.2、Orca 1.4.209、`gpt-6-luna/medium` 和显式完全访问。

最终候选现场的真实测试 PR：[129](https://github.com/erranliu/SteerlineTest/pull/129)、[130](https://github.com/erranliu/SteerlineTest/pull/130)、[131](https://github.com/erranliu/SteerlineTest/pull/131)、[132](https://github.com/erranliu/SteerlineTest/pull/132)。130/132 合并，129/131 清理关闭。完整 L2 的 [121](https://github.com/erranliu/SteerlineTest/pull/121) / [122](https://github.com/erranliu/SteerlineTest/pull/122) 已合并。

Full 的 18 个默认 skip 和基础设施的 1 个 skip 按实际报告保留；未批量启用需要独立现场的 native opt-in 或 sandbox 管理员预检，skip 不计为这些检查通过。

### 版本及增量范围

Full、带 tag 基础设施和最终候选场景均运行于 clean `120b54f3`。L2、受控 L3 和 live delivery/recovery 运行于 clean `64d64a1f`；该版本到最终版本的 Go 差异只有 Full runner/其回归，以及 testenv 的项目身份绑定和快照轮询。产品 Go 代码、E2E 业务断言和权限/usage 合同相同。最后的快照改动由双项目隔离及真实双后端多候选现场验证；没有将此前 9 个场景声称为在最终 SHA 重跑。

L1 基线位于 `9483afa1`；最终 Full 包含其默认 Go 回归，并运行所有本次改动的 fixture/runner 回归。E2E 编译记录保留原 SHA，最终 candidate 构建和带 tag 基础设施实际编译并运行了后续改动。精确源码、命令、跳过清单、报告时间/路径及 SHA-256 见 [results.json](results.json)。

主要入口，每轮使用新的 `-out` 目录：

```powershell
go run ./cmd/test-feedback -workers=3
go run ./cmd/test-feedback -level=L1
go run ./cmd/test-feedback -level=L2
go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled
go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=live
# STEERLINE_CANDIDATE_ACCEPTANCE_CONFIG 指向固定当前 SHA 和两套本机 profile 的 manifest
go run ./cmd/test-feedback -level=L3 -suite=candidates -env=live '-scenario=candidates.local'
go run ./cmd/test-feedback -- -tags=e2e ./cmd/test-feedback ./tests/e2e/internal/... ./tests/e2e/cmd/... ./internal/testkit/boundary/... -count=1
go run ./cmd/test-feedback -- -tags=e2e ./tests/e2e/... -run '^$' -count=1
python -B .agents/skills/roadmap-runner/scripts/test_state.py
```

## 失败保留与清理

原始失败报告未覆盖。保留过期夹具 panic/错误预期、写入口拒绝范围过宽、空 workflow 查询、同 turn final 误计数、控制台后续命令缺失、首次快照身份检查位置错误、旧 Full 总超时及高负载下 isolation 超时。修复后在新的输出目录回归。

两轮 candidates 在 20 分钟业务期限内未完成，报告仍为失败；最终使用相同 `120b54f3`，待 Full 完成后新建现场运行通过。没有延长真实候选期限、删除断言或追认超时轮次。首次 Orca smoke 因应用未运行失败；启动已安装的 Orca 后在新现场重跑通过。

初始运行被会话中断，不能计为通过；遗留进程已结束。旧 scratch `C:\Users\lmas1\AppData\Local\Temp\steerline-e2e-2240857957` 的删除被执行策略拒绝，保留现场而未绕过策略。本轮最终通过场景均完成自己的资源清理。

原始证据位于 `C:\Users\lmas1\AppData\Local\Temp\steerline-all-tests-after-226-20261006-a0ecf55b`。仓库只保存筛选后的汇总和报告哈希，不提交认证、私有数据库、原始提示或 dispatch capability。
