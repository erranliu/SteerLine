# 应用与项目生命周期收拢 Plan

2026-10-16 用户授权实施；基线 ae6673e8。[Spec](application-project-lifecycle.spec.md)采用审计后的五项范围，[验收](../acceptance/application-project-lifecycle.md)登记结果。本 Plan 不维护实时结果。

## 实施步骤

1. 核对旧 start/run/window/listener 与 stop_service 消费者，删除无消费者的 app/host 入口及专属采样/回执链；保留业务控制、连接观察和拒绝入口。scheduler 仍按普通 context 取消结束，不新增二阶段回执。
2. 提取只读 catalog；为目录实现显式 Start/Close、never-started/pending/finished 结果。verify-stop 直接使用 catalog，不持有活动目录对象。
3. ProjectRuntime 在取得前建立资源 owner，集中 locks/log/store/actor/product/handler/Runner；factory 只借用资源。Runner 绑定沿用原 request_id/重试/最终证明，部分启动失败保留原事实并统一清理。
4. actor EndActor 仅提交最终 checkpoint，去掉 Close/Runner callbacks；运行时排空进程内端口的实际 Read/Command/Task 使用者后，分别执行 Runner 和本地关闭，按原条件释放锁。
5. 添加有价值的资源/并发回归和结构守卫。保留现有失败停机策略，补 Windows 独立测试入口 Scenario 透传；同步文档与测试清单必要说明。

提 PR 前对齐主干已合入的 Runner 短状态目录与环境检查公开契约：将原 startRunner 中的 `runnerstatepath.Prepare/Directory` 一并迁入项目绑定，保留只读客户端/verify-stop 的 Validate；资源失败断言与双项目夹具清理使用同一短目录。集成范围最小复验 Windows/Linux 生产编译、默认测试编译、固定 L1（含短路径 owner 组），以及 `./scripts/test-runtime-isolation.ps1 -Level L3 -Suite runtime -Scenario runtime.service-lifecycle`。在整合后、提 PR 前执行，以上通过且场景清理完整为条件，证据追加专属 acceptance；既有 recovery/L4 断言不变，不重复完整 L3/L4。

## 测试计划

所有测试用隔离 fixture；不操作安装目录。原始报告用 runner 新临时目录，源码审计/命令与结果登记到上述 acceptance 对应项。

| 验收项 | 级别/组或场景 | 环境与命令 | 执行时点、通过条件、证据 |
| --- | --- | --- | --- |
| ALC-A1 | 源码边界、编译 | 引用检索；go build ./...；临时 GOOS=linux CGO_ENABLED=0 go build ./...；go run ./cmd/test-feedback -build-only | 删除/集成后；旧启动链无生产消费者、跨平台生产/全部默认测试编译成功；记录审计清单和 summary |
| ALC-A1–A5 | L1 / caller owner | go run ./cmd/test-feedback -level=L1 | 代码稳定后；固定集合 complete/passed，零失败/跳过；summary 与组日志 |
| ALC-A2–A4 | 定向 app、taskrunner、scheduler、SQLite、runnerclient、runnerbridge、HTTP/client/host | go run ./cmd/test-feedback -- ./internal/app ./internal/taskrunner ./internal/execution/scheduler ./internal/storage/sqlite ./internal/runnerclient ./internal/runnerbridge ./internal/product/httpapi ./internal/app/host -count=1 | 先补资源取得/关闭、端口排空、目录 Close-before-Start、迟到 ready、兄弟隔离与结构守卫；移除旧停止回执测试，当前 checkpoint/业务控制断言保留；定向报告 |
| ALC-A5 | 现有部署受控脚本 | ./scripts/test-deployment-stop.ps1；既有 TestDeployment* 已在 app 定向中 | 停机/快照集成后；原失败/DB/config 不变，身份/成员/锁反例拒绝；脚本输出和 owner 日志 |
| ALC-A6 | L3 runtime.service-lifecycle、recovery.waiting-merge-restart | ./scripts/test-runtime-isolation.ps1 -Level L3 -Suite 'recovery,runtime' -Scenario 'runtime.service-lifecycle,recovery.waiting-merge-restart' | 实施中补齐 Scenario 参数/独立进程透传后执行；两场景业务与清理通过；新报告目录的 summary/scenario |
| ALC-A6 | L4 runtime.service-write-isolation | ./scripts/test-runtime-isolation.ps1 -Level L4 -Scenario runtime.service-write-isolation | 同上；旧实例、宿主外配置、同仓跨 home 拒绝且正向可用；同级原始证据 |
| 全项文档 | 静态 | python scripts/check_doc_links.py；git diff --check | 最终同步后无错误，记录 acceptance |

资源测试使用真实隔离 SQLite、原公开 Runner CLI helper 和最小本地 closer/调用屏障；新增夹具能力在验收前落实，不只声明后补。测试环境能力变更同步 cmd/test-feedback/脚本定向验证。未选择 Full、L2/live、全 L3/L4：执行规则未改，受控等待合并恢复链验证当前 caller 交付与重启，原生专项归 Runner。无需人工阻塞窗口。

## 结构交付检查

- 旧 StartService/RunConfiguredService/ServiceLifetime/serviceLocalAPI 无产品实现；不留仅测试可达的旧启动模式。
- 项目 factory 和 Actor 不关闭 store；项目结束仅一个资源关闭点；Runner 停机状态只有项目绑定持有。
- 新增项目资源只需在 owner 的取得/关闭处接线，不追加另一组失败 cleanup。
- verify-stop 不引用 NewApplicationDirectory；Close-before-Start 无 goroutine/Runner 启动；结果缺失不默认成功。
