# Runner 停服最终结算

普通开发修复（2026-10-16），不进入 roadmap DAG。对应 W18、E04/E05、V06/V07，统一规则见 [worker lifecycle](../product-design/worker-lifecycle.md)。

问题：部署前 stop 已等待 Runner 服务退出，却使用停机前的候选投影判定原生资源。正常本地关闭后的最终 stopped 无法经已退出服务查询，导致误报资源未核实。

实现：Runner 发布独立的最终结算，service status 在原写者退出后公开返回；新写者使旧证据失效。调用方分别保存服务退出与资源结算结论，校验原实例、停止请求、目录、EXE。未核实继续阻断，不补输入、不重启服务，不修改旧失败回执。

| 项 | 验证入口 | 通过条件 | 结果 |
| --- | --- | --- | --- |
| RSS-A01 | Runner application 的 TestSRShutdownSettlementUsesCommittedFinalNativeEvidence | 最终持久 stopped 才结算，观察脱离仍未核实 | 通过 |
| RSS-A02 | Runner host 的 TestSRShutdownProofRequiresReleasedWriter；integration 的 TestSRServiceOwnership/shutdown_receipt | 写锁占用不公开旧证据；空闲停机最终记录绑定原身份；损坏记录失败 | 通过 |
| RSS-A03 | Runner integration 的 TestSRRestartRequiresResume | 有本地 worker 的服务关闭公开最终结算；重启零自动输入，恢复复用原 session | 通过 |
| RSS-A04 | SteerLine runnerclient 的 TestServiceSettlementRequiresOriginalFinalProof；taskrunner 的 TestRunnerShutdownDoesNotEquateServiceExitWithNativeSettlement | 缺失/错配/未知证据失败；旧 running 投影可被最终证明取代，旧 stopped 不能代替最终证明；关闭不重放 | 通过 |
| RSS-A05 | SteerLine SQLite actor 与 app 统一启动停机定向测试 | 接入后停机、宿主身份及聚合拒绝规则保持 | 通过 |

自动验证均使用各仓库隔离测试夹具，无真实模型/PR，无安装现场操作。命令与结果登记在本文件；Runner checkout 为 C:/Project/SteerRunner，SteerLine 为当前聊天 worktree。

- Runner：go test ./internal/host ./internal/application -run 'TestSRShutdown|TestSRStopProgress' -count=1（通过）。
- Runner：go test ./tests/integration -run 'TestSRServiceOwnership/shutdown_receipt|TestSRRestartRequiresResume' -count=1 -timeout 180s（通过，28.872s）。
- SteerLine：go test ./internal/runnerclient ./internal/taskrunner ./internal/storage/sqlite ./internal/app -run 'TestServiceSettlement|TestRunnerShutdown|TestRunnerActor|Test.*BackendStop|Test.*Application' -count=1（前三包通过；app 未匹配，不计 app 验收）。
- SteerLine：go test ./internal/runnerclient ./internal/taskrunner -count=1（通过）；go test ./internal/app -run '^TestUnifiedStartupLifecycle' -count=1 -timeout 180s（通过，21.960s）。
- SteerLine：go test ./internal/taskrunner ./internal/storage/sqlite -run 'TestRunnerShutdown|TestRunnerSettlement|TestRunnerActor' -count=1（通过，含最终查询超时后仅重读、不重复 stop）。
- Runner：go test ./internal/host ./internal/application ./internal/contracts -count=1 -timeout 120s（通过）。
- Runner：go test ./tests/integration -run '^TestSRServiceOwnership$' -count=1 -timeout 180s（通过，29.104s）；追加替换服务异常退出反例后重跑 shutdown_receipt（通过，10.422s，上一代证明未被复用）。

现有 C:/SteerlineInstall 与 C:/SteerlineRuntime 未使用或修改。已有旧 EXE/失败回执不会因源码修复自动补出缺失证据；当前安装的实际资源状态和恢复仍未验收。
