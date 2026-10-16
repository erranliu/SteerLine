# Runner 停服最终结算

普通开发修复（2026-10-16），不进入 roadmap DAG。对应 W18、E04/E05、V06/V07，统一规则见 [worker lifecycle](../product-design/worker-lifecycle.md)。

问题：部署前 stop 已等待 Runner 服务退出，却使用停机前的候选投影判定原生资源。正常本地关闭后的最终 stopped 无法经已退出服务查询，导致误报资源未核实。

实现：Runner 发布独立的最终结算，service status 在原写者退出后公开返回；新写者使旧证据失效。调用方分别保存服务退出与资源结算结论，校验原实例、停止请求、目录、EXE。未核实继续阻断，不补输入、不重启服务，不修改旧失败回执。

| 项 | 验证入口 | 通过条件 | 结果 |
| --- | --- | --- | --- |
| RSS-A01 | Runner application 的 TestSRShutdownSettlementUsesCommittedFinalNativeEvidence | 最终持久 stopped 才结算，观察脱离仍未核实 | 通过 |
| RSS-A02 | Runner host 的 TestSRShutdownProofRequiresReleasedWriter；integration 的 TestSRServiceOwnership/shutdown_receipt | 写锁占用不公开旧证据；空闲停机最终记录绑定原身份；损坏记录失败 | 通过 |
| RSS-A03 | Runner integration 的 TestSRRestartRequiresResume | 有本地 worker 的服务关闭公开最终结算；重启零自动输入，恢复复用原 session | 通过 |
| RSS-A04 | SteerLine runnerclient 的 TestServiceSettlementRequiresOriginalFinalProof；app 的 TestRunnerSettlementReadCanRecoverWithoutRepeatingStop / TestRunnerShutdownRetriesOnlyUnconfirmedOriginalStop（生命周期收拢后迁至实际 owner） | 缺失/错配/未知证据失败；旧 running 投影可被最终证明取代，旧 stopped 不能代替最终证明；关闭不重放 | 通过 |
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

## 部署路径补充

用户在两仓 PR 合并后重跑仍失败：build-deploy 在替换前调用安装版 CLI，且原宿主已经保留失败回执。前一修复只覆盖后续宿主正常退出，没有覆盖这一更新入口。

部署现在用暂存 CLI 停止原宿主；原回执失败时，执行显式 verify-stop，经原宿主/Web 退出、项目本地关闭、checkpoint/实例/配置/实际锁核验，再调用暂存 Runner 的公开 service verify-stopped。Runner 在既有 writer.lock 下只读当前 schema，通过已持久化 stopped 或完整 managed Job 原 owner+root 消失证明资源已退出。证明缺失、非 managed、Orca、原资源仍活动继续失败。新证据写入本次部署包 shutdown-verification.json；原失败回执、任务数据和配置不被修复命令改写。

| 项 | 场景/命令 | 通过条件 | 结果 |
| --- | --- | --- | --- |
| RSS-A06 | SteerLine TestDeploymentRechecksFailedReceiptWithoutRestartOrRewrite | 普通 stop 保留原失败；显式重验成功，原回执/DB/配置/fixture 服务记录字节不变；活动锁、配置变化、本地未关闭拒绝 | 通过 |
| RSS-A07 | SteerLine TestDeploymentCandidateVerificationKeepsOriginalBinding | 仅原 Run、input_digest、state-dir 匹配；缺失与替换拒绝 | 通过 |
| RSS-A08 | Runner TestSROfflineStoppedRequiresManagedOwnerAndRootAbsence、TestSROfflineReaderDoesNotCreateOrWrite | owner/root 仍活动、非 managed、Orca、缺身份拒绝；只读库不创建/写入 | 通过 |
| RSS-A09 | Runner TestSRServiceOwnership/offline_verification（原服务归属场景内） | 真实 EXE 活动写者拒绝；有本地 worker 的服务异常退出无最终回执，原 root/owner 消失可验证；零新服务/输入、DB不变 | 通过 |
| RSS-A10 | scripts/test-deployment-stop.ps1 | 暂存 stop 成功直接结束；失败进入独立证明；重验失败或不完整证明阻断部署 | 通过 |

已执行：

- SteerLine：go test ./internal/app ./internal/runnerclient ./cmd/steerline -run 'TestDeployment|TestServiceSettlement' -count=1 -timeout 120s（app 26.654s 通过；runnerclient 通过；cmd 编译、无匹配测试）。
- Runner：go test ./internal/application ./internal/storage ./internal/host ./internal/cli -run 'TestSROffline|TestSRCLIContract' -count=1 -timeout 90s（application/storage/cli 通过；host 编译、无匹配测试）。
- Runner：新增 EXE 隔离验证首次以独立 TestSROfflineVerification 名称通过（29.225s），随后归入既有 TestSRServiceOwnership/offline_verification，不新增测试场景登记。
- Runner：go test ./internal/application ./internal/storage ./internal/host ./internal/cli ./internal/resources -run 'TestSROffline|TestSRCLIContract|TestSRStopProgress|TestSRShutdown' -count=1 -timeout 90s（规则/存储/host/CLI 通过，resources 仅编译）；go test ./tests/integration -run '^TestSRServiceOwnership/offline_verification$' -count=1 -timeout 120s（30.853s 通过）。
- SteerLine：go test ./internal/app ./internal/runnerclient ./cmd/steerline -run 'TestDeployment|TestServiceSettlement|TestUnifiedStartupLifecycle' -count=1 -timeout 180s（app 56.258s、runnerclient 通过；cmd 仅编译）；scripts/test-deployment-stop.ps1（全部通过），部署脚本 PowerShell 语法解析通过。
- 增加核验器摘要后 SteerLine 部署定向测试重跑通过（32.454s）。Runner 共用恢复证明抽取后的 application/host/storage/cli/resources 五个相关包回归全部通过（application 66.540s），没有执行全仓测试或 live。

重验仅支持当前格式，无旧库读取/迁移、重启取证或强行成功。安装现场仍未操作，实际部署结果待用户更新两仓后重跑。
