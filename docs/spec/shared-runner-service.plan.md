# Plan：应用级共享 SteerRunner 服务

日期：2026-10-19。状态：已按用户授权实施；自动验证结果及后置人工项见专属验收。行为以 [Spec](shared-runner-service.spec.md) 为准，实时结果归[专属验收](../acceptance/shared-runner-service.md)。普通开发，不占 Task ID、不修改 execution-map，不创建执行聊天/worktree，不部署安装现场。

## 1. 固定输入与整合决定

调查基线为 SteerLine `abd001f1fc40fb57abec49f4173874c0c942f2f1`，Runner 源码 `59c7597ac8056fc5c29ad622034e10e74e4395de`。已全文读取 `docs/spec/SpecGuide.md`，其最后修改 commit 为 `94e704300485b82d54e44991148074841ed66019`，SHA-256 为 `85b656090674e1f953320b6be62d17b6888248ef7eec5fe044b9d3f408bfe4de`；普通开发沿 AGENTS.md，不采用 roadmap 转正/发布准入。

同时读取 [worker-lifecycle](../product-design/worker-lifecycle.md)、[测试架构规范](../../SteerlineProjectGuideDoc/testing-architecture.md)、[本项目测试入口](../testing.md)与 [E2E 入口/维护约定](../../tests/e2e/README.md)，核对当前宿主、配置、任务 outbox、环境检查、verify-stop、reset、cleanup 和公开 CLI fixture。关联 W01/W08–W14/W18、E04/E05/E07、V01/V03/V06–V08；统一矩阵不复制到本 Plan。

实施前对实际 HEAD 与本基线的差异复核，特别是生命周期、Runner 绑定及调试重置的并行修改；记录实际实现和交付构建。使用固定真实 Runner EXE 作为公开契约依赖，登记来源 commit/version/SHA/resource digest，不能仅凭本地源码 HEAD 宣称 EXE 可用。当前未发现必须修改 Runner 的能力缺口；离线未知请求映射不足按 Spec 阻断，不在旁路读取 Runner 私有库。

| 范围 | 复用、替换与删除 |
| --- | --- |
| `app/application_directory.go`、`application_service.go`、`project_runtime.go`、`project_runner.go` | 应用目录/宿主为共同生命周期 owner；上移现有 Runner 启停与原停止证据，项目仍唯一关闭自己的库/锁/log。删除项目 service stop/final proof 闭包，不新增容器/服务池 |
| `runnerstatepath`、`runner_build_binding.go`、`state_preflight.go`、`runner_service_wiring.go`、`environment_checks.go` | 目录按 home 同源派生；共同构建/实例注入 actor、准入、检查和查询，保留项目 publication、配置快照和 checkpoint 核验 |
| `taskrunner/service.go`、`control.go`、`runnerbridge`、`environmentcheck`、`runnerstate` | 已有信封保存独立随机 Runner ID，原产品 ID 仍负责局部幂等；Action 补产品请求身份，检查补目录绑定，不新增映射表或 scope 配置 |
| `product/applicationcontract`、`application_start.go`、`application_verify_stop.go`、`runnerclient/verify_stopped.go` | 私有停机结果新增单份共享 Runner 证明，stop/status/verify-stop 同步消费当前格式；补公开 Checks 停止证明，不改 Runner 协议 |
| `tests/e2e/internal/driver`、`controlled`、`internal/testkit/protocol`、脚本/Web helper | 统一包级服务/重置/cleanup；多项目 fixture 路由改为仓库/关联绑定，保留公开契约夹具职责 |

## 2. 实施步骤

### A. 固定服务布局、信封与持久绑定

1. 将 `runnerstatepath` 的输入从项目 run-dir 改为应用 home，固定带用途标记的摘要算法及各平台规范化。应用在原 home 租约下、`service start` 前离线验证 EXE/版本，并原子写入或核对私有 `application-api/runner-build.json` 的不可变 BuildPin；pin 缺失但 Runner 目录已存在、pin 错配均零启动。首次先写 pin 后准备 Runner 目录，失败/崩溃/reset 不删除 pin；不把构建摘要加入目录名自动绕开原绑定。保持短路径/私有性/别名拒绝，无操作只读不建目录；删除旧项目派生路径，不回退。
2. submit 和检查首次创建意图时生成独立随机 Runner request_id，沿现有 `id()/newID()` 的 128-bit 做法，保存完整信封后再调。不要新增 home scope 参数、哈希协议、代次或给 `runnercontract` 加生成器。已有随机 Run stop 保持。
3. 覆盖 submit、reply/resume、自动/显式 Run stop、环境检查 start/stop 和原 request-id 查询。`environmentcheck.Operation.RequestID` 保留产品 ID，`Operation.Request.RequestID` 保存 Runner ID；refresh/inspect 按后者核对 `Command.RequestID`，客户端/HTTP 继续返回前者。检查记录增加精确 state-dir，构建/资源摘要继续沿现有字段，不复制配置权威。
4. reply/resume 的 `Action` 补原产品请求 ID/不可变请求摘要；沿已有 project/candidate gate，先查旧 Action 并比较产品请求，再做新的 revision/正文构造。找到原动作即沿原信封，不从最新 projection 重造正文；不存在时才生成/保存随机 Runner ID。调整 `QueueControl` 去重依据，覆盖“Action 已保存、外层回执未保存”、Run revision 已前进和并发重试。候选 submit、检查沿原请求查找，不建映射表；公开结果仍按项目/目标/correlation 核对。
5. 当前格式变化同步 codec/严格来源拒绝和 fixture，旧目录/checkpoint/candidate/check 一律明确不匹配，不迁移、不双写。reset 后新建逻辑操作允许新 operation ID；普通重启不是新操作。

### B. 应用持有 Runner，项目借用

1. 将现有 `projectRunner` 启停和结算实现上移为应用专用 owner，持有固定 client/info/原 stop request，删除项目级 owner。直接在 `ApplicationDirectory.run` 已有 census/冲突检查之后、项目 goroutine 启动之前初始化一次，再把固定绑定作为参数交给各项目。不增加懒加载 Get/Acquire、引用计数或借用等待协调器。
2. census 后没有可启动项目或 Close-before-Start 不初始化 Runner；项目后续 prepare 全失败时允许空闲 Runner 继续归应用持有，无需为了避免一个空闲进程建立额外两阶段预备流程。启动使用应用有界 context；与整体停止竞争时由原 owner 收尾原创建事实，迟到结果不得启动项目。
3. 共同结果完成 EXE/version/SHA/resource/instance 核验后立即绑定原停止义务，再查目录/能力；目录失败仍结算已验证实例。所有项目拿到不可变同源客户端。项目各自验证 publication/profile；局部错误不触发服务关闭。公共初始化失败在本次启动内终止项目准入，保留应用 API/目录诊断，不由项目重试或开替代实例。
4. 应用构建准入 pin 必须在共同服务初始化前检查，不能等原 `bindRunnerBuild` 的项目阶段才发现错构建；后者改为核对并保存项目 checkpoint/candidate/check 的绑定快照，不再拥有服务准入。项目锁仍先于项目写库。共同身份注入每项目 actor/控制/检查/查询；离线消费者显式使用 home 目录和原绑定，不因共享服务创建第二写者。
5. `ProjectRuntime.close` 只关闭项目资源；失败目录保留“已有执行可能继续”的诊断。未完成停止义务保留原记录，actor 已关闭时不承诺继续处理，整体停服或显式重启按原事实结算；不另建后台项目收尾服务。人工退出不升级为 Runner stop。删除旧项目 Runner owner 及只证明旧拓扑的断言，保留身份/资源未知反例并迁到共同 owner。

### C. 整体停止、换代与证明

1. `BeginStop` 先对所有项目封闭 caller 入口/派发，再取消并等待既有进程内调用排空，禁止迟到 ready 重新开放。Runner 已受理 workflow 在真正 service stop 受理前仍由 Runner 管理；不能把 caller 门槛冒充即时撤销其原生授权。共同初始化和项目排空有既定期限，不串行累加为无限等待。
2. 应用 owner 发送一次原实例 service stop，保留现有未知请求补查和最终 last_shutdown 围栏；每项目不再重复停止。项目关闭失败、检查未知、目录无人 ready、初始化失败但可能已创建服务等路径均进入共同收尾，某个项目失败不能跳过服务停服。
3. 私有 `Shutdown` 写新 version，新增一份应用级 Runner 结果，并从 `ProjectShutdown` 删除 `ResourcesSettled`。项目结果只写本地关闭/锁释放；应用 `Clean` 由所有项目本地结果和这一份共享结算计算，不复制全局真假、不在服务关闭后回填各项目。用现有 attempted/原身份事实区分未尝试与结果未知，不新增多阶段持久状态机；预算内补查仍读同一原证明。
4. 同步 `verifyBackendStop`、status、部署脚本的当前结果消费，stop 只在全部既定条件满足时成功。格式旧、原宿主/Runner 错配、缺结果/检查未结算继续失败，不读旧格式回退。保留 start/stop/verify-stop 外层预算及本地失败保锁策略。
5. `verify-stop` 先用只读 catalog/checkpoint/原锁确认成员及本地关闭，再按 home 取得共享 `VerifyStopped` 证明。补公开 Checks 解码与唯一性/input_digest/settled 校验，核对所有项目绑定及历史资源；`ApplicationStopVerification` 保存一份证明，删除 `ProjectStopVerification.Runner` 的重复结构。未绑定未知请求离线无法查证则失败，不启动服务。保留原 failed shutdown 与各库 hash，新证据另存。
6. Runner 换代时所有旧项目保持原 expected-instance 围栏，旧停止也绑定原实例。显式重启 SteerLine 才共同接纳同构建新实例；每项目只读核对原 Run/Check/请求后开放自己的准入；检查沿 `environmentcheck.Service.VerifyBindings` 复用原检查 schema/身份校验，在 ProjectRuntime 创建 actor/端口前执行，不写回缓存或派发操作，unknown 零重发。一个项目原数据冲突不触发全局 stop 或兄弟重绑。

### D. 调试、夹具与验证入口

1. 同步唯一 `scripts/new-debug-release.ps1`、`debug-template/`、debug manifest/driver 对 home 共享目录和进程数量的认识。默认 live、AI 显式 `--simulate`、启动后冻结不变。两个项目使用 SteerlineTest 的独立本地副本/项目身份；不得因共用远端而绕过现有同仓库锁冲突。
2. reset 将共享服务检查上移到包级，先核验全部成员和 Run/Check，再修改任何项目；无项目库也不能绕过。删除 reset 的 `runnerstatepath.RemoveStopped` 及私有目录遍历删除；保留应用构建 pin 和共享 Runner 历史/认证/仓库。检查资源以该共享停止证明及原 Check/input_digest 为准，停服前的 cleanup 缓存不作第二结算门槛，未知操作仍保留/阻断。清理 caller/UI 状态后新操作拥有新 Runner ID，不能因此解除原构建绑定。跨库中途失败如实记录、可重新核验，不宣称原子回滚。
3. `CleanupRunner` 与 debug stop/fixture teardown 在包级按固定 home/service 去重，最后一个项目退出前不得停共同服务；隔离夹具最后删除临时根仍须先证明全体资源停止，只处理其拥有的环境。通用 reset 与测试夹具销毁不是同一动作。
4. controlled 多项目只补真实消费者所需能力：共享 state-dir 的 requests/runs 在同一公开命名空间，按 repository/correlation 选择 provider，后续按原 Run 绑定；复用现有夹具并发保护。环境检查同号/回执反例沿已有 environmentcheck L1 fixture，不为本次 L3 再复制一套检查引擎。不得用两个不同 state-dir 假装共享服务。
5. 新增一个 L3 controlled 场景 `runtime.shared-runner-projects` / `TestSharedRunnerProjects`：同 home 两项目正式创建/领取任务，使用相同产品请求 ID 得到两个不同 Runner Run；两者并存、分别消费事件/PR，任务事实隔离；正常整体停启后零自动新业务输入，最后清理完整。局部失败/未知/错证据组合由 L1 承担，不把它们塞入正常 L3。场景独立拥有 home、两 repo、provider、端口、证据，沿既有 runtime 二并行预算；同步 `suites.json`、README、runner 选择/枚举/证据校验。该 ID 随本次变更注册交付。
6. 在既有 debug driver 测试文件新增 `TestDebugReleaseSharedRunnerService`，使用生成器新建的双项目真实 Runner 包，沿 `STEERLINE_DEBUG_SECOND_TEST_ROOT` 传入路径。正式 debug run/start/status/stop/reset 验证真实空闲拓扑；两项目 snapshot 的 RunnerBuild/state-dir 与公开 `service status` 的精确身份关联，客户端同源/expected-instance 注入由 L1 证明。测试不提交任务/模型，且与现有 simulate 测试分开选取；缺新包/固定 EXE或身份则该验收失败。复用已有 driver 进程/清理/日志能力，不新增 PowerShell wrapper、独立报告 schema 或仅供测试的产品字段。
7. Web 无业务改动，不默认再建包跑 Edge。公共 helper 中必要的目录/stop 适配仍做引用核对和 driver 验证；只有实际改动浏览器行为时才补既有 project-switch 场景，并在验收登记原因。

SQLite 保存沿现有 CAS 入口补齐冻结约束：既有 Action 的产品请求 ID/摘要、命令/完整 Runner 信封，以及既有 Check 的 state-dir 不可覆盖；允许更新原观察状态或追加新动作，不引入映射表或第二套 journal。

### E. 收尾与交付

同步架构进程图、Runner 集成、debug 使用说明、测试清单、停机/部署说明与 worker-lifecycle 当前实现/缺口，明确历史 per-project 结论的后继版本。删除生产及测试脚本的旧目录/逐项目 service stop 假设，记录仍合法的每项目 Task run-dir，不能简单全局替换所有 RunDir。

A→B→C 为主依赖；D 的 fixture/驱动必须在验收前完成。按下表运行最小检查并登记全部验收，失败和补测分别记录。服务所有者与全部消费者在同一 SteerLine 变更交付，无 Runner PR 前置；按用户授权直接实施，不发布或部署安装环境。

## 3. 测试计划

自动项归[专属验收](../acceptance/shared-runner-service.md#自动验收)。新增快速测试用 `TestSharedRunner` 前缀，登记到实际 owner 的现有 L1 选择；缺少对应包选择时只补必要映射，不预设三个新测试组。真实包测试 `TestDebugReleaseSharedRunnerService` 不进入 L1，必须单独传新包并精确运行；缺环境或 skip 不计该验收通过。新场景/测试函数已随 D5/D6 交付；实际选择与结果见专属验收。

| 验收项 | 级别 / 组或场景 | 环境、命令 | 时点 / 通过条件 | 证据登记位置 |
| --- | --- | --- | --- | --- |
| [SRS-A01、SRS-A02](../acceptance/shared-runner-service.md#自动验收) | L1 应用 owner/path | 隔离 home/SQLite/公开 CLI；`go run ./cmd/test-feedback -- ./internal/app ./internal/runnerstatepath -run '^TestSharedRunner' -count=1` | A/B 后；census 后一次初始化、零项目/Close-before-Start、整体停止竞争/未知；错构建/pin 缺失但旧目录存在均 service start 零调用，首次 pin 先于目录、失败及 reset 保留 pin；路径/socket/跨 home/旧绑定拒绝 | A01/A02：summary、pin/原调用/身份/锁/目录断言 |
| [SRS-A03](../acceptance/shared-runner-service.md#自动验收) | L1 原请求/outbox/check | 隔离项目 store 和公开回执；`go run ./cmd/test-feedback -- ./internal/taskrunner ./internal/runnerbridge ./internal/environmentcheck -run '^TestSharedRunner' -count=1` | A 后；跨项目/业务/检查同产品 ID 不串；Action 已保存但外层回执丢失、Run revision 已前进及并发重试仍复用一个原动作/信封；改产品正文拒绝，unknown/重启零重发 | A03：产品 ID/摘要、持久 Action/信封、回执和派发计数 |
| [SRS-A04、SRS-A05](../acceptance/shared-runner-service.md#自动验收) | L1 停机/取证/reset | Windows 隔离 owner/公开证明/debug fixture；`go run ./cmd/test-feedback -- ./internal/app ./internal/runnerclient ./tests/e2e/internal/driver -run '^TestSharedRunner' -count=1` | C/D 后；项目只关本地、停止义务保留；整体排空/原停止/迟到初始化/预算/失败保锁、只有一份全局证明；全体 Runs/Checks 及未知反例；reset 全体预检/保留目录/新操作不撞历史 | A04/A05：原 shutdown、checkpoint/库 hash、共享证明、reset 保留事实与清理 |
| [SRS-A01–SRS-A05](../acceptance/shared-runner-service.md#自动验收) | 固定 L1、分级登记和消费者编译 | `go run ./cmd/test-feedback -level=L1`；`go test ./cmd/test-feedback -count=1`；`go test -tags=e2e ./tests/e2e/... -run '^$'` | E 阶段；受影响旧 owner 反例已迁移、登记不漏/不重复，编译通过；不再另外要求十余包全量重跑 | 各项下登记 summary/实际选择；编译不计场景通过 |
| [SRS-A06](../acceptance/shared-runner-service.md#自动验收) | L3 controlled 新 `runtime.shared-runner-projects`；既有 `recovery.waiting-merge-restart` | `go run ./cmd/test-feedback -level=L3 '-suite=recovery,runtime' -env=controlled '-scenario=runtime.shared-runner-projects,recovery.waiting-merge-restart'`；Windows Job 下用 `./scripts/test-runtime-isolation.ps1 -Level L3 -Suite 'recovery,runtime' -Scenario 'runtime.shared-runner-projects,recovery.waiting-merge-restart'` | D/E 后；同 home 两 Run 并存、同号请求和任务/PR隔离、正常整体停启/零自动输入、原候选恢复和清理完整；新场景已涵盖正常服务生命周期，无需再重复一个相同主流程 | A06：既有 summary.json 与 `scenarios/<id>/scenario.json`，请求/Run/清理证据 |
| [SRS-A02、SRS-A05、SRS-A07](../acceptance/shared-runner-service.md#自动验收) | 定向真实空闲服务集成 | 生成器建全新双项目包后，设置 `STEERLINE_DEBUG_SECOND_TEST_ROOT`；`go test ./tests/e2e/internal/driver -run '^TestDebugReleaseSharedRunnerService$' -count=1 -timeout=10m -v` | D6/E 后；包内固定真实 Runner；两项目实际 ready/snapshot 同 path/build、公开服务身份唯一、重复 start/正常停启/reset、完整清理；无模型/任务/PR | A07：包 manifest、driver 日志、公开 status/项目 snapshot/停机结果、保留目录证据，沿包 evidence 保存 |

部署停止消费者另运行 `./scripts/test-deployment-stop.ps1`，使用内存函数夹具校验一份全局 Runs/Checks 证明，不操作安装目录。真实包在 Windows kill-on-close Job 内须沿 `scripts/independent-test.ps1` 既有的 WMI 独立进程方式运行；不新增 wrapper。

真实包准备使用现有入口，不新增 wrapper：

```powershell
$taskRelease = ./scripts/new-debug-release.ps1 -WebProjects 2 -SteerRunnerExe <固定绝对EXE> | ConvertFrom-Json
$env:STEERLINE_DEBUG_SECOND_TEST_ROOT = $taskRelease.root
# 执行上表精确测试；结束后恢复该环境变量原值
```

生成器已构建双产品 EXE，无需再为同份源码重复运行 build.bat。全部命令核对非零匹配、实际终态和清理；报告使用现有 test-feedback/driver 产物，不新增验收报告协议。文档/引用审计及 `git diff --check` 覆盖所有生产、平台、脚本消费者的旧项目目录/逐项目 service stop 假设。

不默认 Full、完整 L2、L4、Edge 或 Runner 原生专项：本次没有新的 HTTP/Web 业务、仓库锁规则或执行引擎，L1 覆盖局部错误/身份/故障反例，新增 L3 覆盖双项目接线与正常整体停启，既有 restart 场景覆盖调用方恢复，真实包补实际服务拓扑。若实际实现改变 Web 行为或宿主写入准入，再精确选择既有 project-switch 或 runtime.service-write-isolation，并说明新增原因。A07 只证明真实空闲服务；controlled 双 Run 不表述为真实模型双 Run。真实 EXE/新包缺失明确阻断 A07，不降级 simulate。

## 4. 人工窗口及交付记录

[SRS-M01](../acceptance/shared-runner-service.md#人工验收)：自动通过后，在本次新包中体验双项目启动、项目不可用但执行可能继续的诊断、整体停止；用户窗口待定。可后置，仅阻塞日常体验签收，不作为其他任务或本次自动项的先验门槛。

实施和验收时逐项登记实际代码 commit/dirty、固定 Runner 身份、命令、级别/实际选择、原实例/请求、通过/失败、证据路径和清理；失败不覆盖，补测另记。实施只使用隔离夹具和生成器新包，不操作 `C:/SteerlineInstall`、`C:/SteerlineRuntime`。
