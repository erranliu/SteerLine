# Runner 兼容接管 Plan

对应 [Spec](../spec/service-generation-recovery.spec.md)；采用既有 [worker-lifecycle](../product-design/worker-lifecycle.md) 和 caller outbox，不新增 lifecycle、持久兼容 manifest 或全局 hold/release。实际记录见[验收](../acceptance/service-generation-recovery.md)。

实施顺序：runnerclient 增加公开 takeover 与裸 startup stops；application 在共享 Runner 启动前采样项目 caller Store、保存原停止信封；按原 Run/input/request digest 重绑当前服务 pin；环境检查沿原 ID/报告和旧 selection 取证；部署分别验证旧服务退出与新候选兼容。所有原任务、候选、结果、PR 和 Delivery 都保留原事实。

## 测试计划

| 验收 / 级别 | 场景、环境及命令 | 时点 / 通过条件 / 证据 |
| --- | --- | --- |
| SLC-A01 / L1 | Windows 隔离进程 CLI，`go test ./internal/runnerclient -count=1 -timeout=3m` | 接线后；错候选 SHA/目录/contract/unsupported/重复 item 拒绝，原 stop 裸信封不变、明确 revision 拒绝不晋升 unknown；专属验收 |
| SLC-A01/A05 / L1 目录兼容 | 同一包的 `TestRunnerCatalogAcceptsWorkflowsWithoutFixedPromptSteps`，沿上行全包命令 | 真实启动发现目录新增合法 conversation/generate 后修复；无固定 prompt steps 不拒绝整个目录，所选配置仍由公开 schema 校验；专属验收 |
| SLC-A05 / L1 发布接线 | Windows 隔离 bare Git/PR 查询 fixture，`go test ./tests/e2e/internal/driver -run '^TestDebugPublicationReusesPR$' -count=1 -timeout=120s` | live 发布暴露认证覆盖后；真实 push 保留作用域内的 credential helper/HTTP header，不把旧 PR 误当发布成功，不重建已关闭/合并 PR；专属验收 |
| SLC-A02 / L1 | 隔离 Git/SQLite/公开 CLI，`go test ./internal/app -run 'TestSharedRunner\|TestTakeover\|TestDeployment' -count=1 -timeout=5m` | 接管后；证明才能更新当前 pin，原 candidate build/input/PR/unknown 保留；错原绑定、活动旧服务、本地未关拒绝；专属验收 |
| SLC-A03 / L1 | 同上，`go test ./internal/app -run '^TestStartupCancellationOutbox' -count=1 -timeout=120s` | 启动前；原 StopRequested 的首次信封先持久保存，第二次收集字节相同，不启动 Runner，不改原 submit/Run/pin；专属验收 |
| SLC-A02/A03 / L1 补充 | `go test ./internal/app ./internal/environmentcheck -run 'TestEnvironmentCheck\|TestStartupCancellationOutbox\|TestSharedRunnerCheck' -count=1 -timeout=4m` | 旧 Check 传输接线后；旧 EXE 不存在仍能在逐项证明后用当前 CLI 读取，原 selection/resource digest 保留，错 input digest 拒绝；专属验收 |
| SLC-A04 / L1 | Windows PowerShell，`powershell -NoProfile -File scripts/test-deployment-stop.ps1`、PowerShell Parser；新版 Go 预检入口用 `go test ./cmd/steerline -run 'Test.*Takeover' -count=1 -timeout=120s` | 部署脚本修改后；保留原 service 退出与 resource=false，跨构建必须另有正确 candidate/takeover 证明，失败不替换；专属验收 |
| SLC-A05 / L3 真实产品组合 | 先用 `scripts/new-debug-release.ps1 -SteerRunnerExe <A> -AgentExecutable <native-codex>` 生成 `C:\SteerlineDebug` 独立 live 包；再运行 `powershell -NoProfile -File scripts/test-runner-update-live.ps1 -DebugPackage <package> -RunnerCandidate <B> -AgentExecutable <native-codex>`。A/B 同源码、不同版本/SHA；真实本机 Codex，真实 `erranliu/SteerlineTest` 本包专属 base/head | 部署规则修复后；实现轮开始后停管理服务，公开原生 thread/read 证实离线完成，使用生产预检/停服核验后替换兼容 Runner；原 Task/Candidate/Run/session/input/build 事实保持，零重复 submit/resume，原结果发布到唯一 PR、精确 head 合并后唯一 Delivery、后继 ready 未启动。命令、原生只读摘要、两代快照及失败/重验记录存本包 `evidence/runner-update-live`，验收登记 SLC-A05 |

Runner 的原 owner/parser/离线审计 checkpoint 与 startup stops 专项仍由其场景验证，此仓不复制原生状态机矩阵。2026-10-20 用户已授权上述独立 live 联验及发现问题直接修复；仅本次兼容构建升级允许改变包内 Runner 传输 pin，live 方案、模型/权限、原任务材料和冻结执行保持。只合并本包 SteerlineTest 专属 base/head 的测试 PR，不操作安装版/保留环境，不跑 full。Orca 创建查询仍独立未验。

SLC-A05 是本次独立调试包的专项联验入口，不属于 `test-feedback` 已登记分级场景，不据此声明完整 L2/L3 或原生恢复矩阵通过。执行材料在首次启动前提交并绑定，不使用 reset 复跑已开始的候选。

驱动在业务领取前失败时，可显式加 `-ContinuePrepared` 延续同一冻结包；入口必须读到 live/ready 方案、服务 stopped、两项 open 任务且零候选，随后不改材料、契约或执行配置。历史失败报告保留，新证据写入独立的 continuation 目录。已有业务候选的包不允许沿此入口重跑。
