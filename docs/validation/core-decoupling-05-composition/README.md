# #66 内置实现组装与主包依赖反转：交付与复审记录

记录日期：2026-10-02。执行依据：[Spec](../../archive/spec/completed/core-decoupling-05-composition.spec.md)、[Plan](../../archive/spec/completed/core-decoupling-05-composition.plan.md)、[共同约束](../../archive/spec/completed/core-decoupling-constraints.md)。

**状态：协调会话已独立复审、修复并完成完全访问环境最小验收，等待本 PR 合并。#67 仅在平台确认本 PR 真实合并后启动。以下执行阶段记录保留其原候选和失败结果，不能用来认证后续源码。**

## 协调独立复审与最终验收

执行候选提交 `4ab14ef`；协调修复提交 `755885d52f23c1bb98c4b8c4e7e9cdaf6b97454b`，审核基线 `5bbbfcd6a16d34cdfd77dfa8894c1661a885b799`。亲自阅读实际 diff、Spec/Plan、产品入口、固定 profile/资源恢复及测试迁移；未向实现导出业务写入口。

确认并修复：

- `state_preflight.go` 丢失准备记录时漏检：目录新增纯值 `ReadinessRequired`，按原固定 profile 在恢复写入前拒绝；Claim 和预检使用同一要求，准备预算不变。
- `service_client.go` 与 app `capability.go`：启动/认证先拒绝缺目录、缺端口或非法配置，再访问 Git/目标或写 startup 记录。
- `worker_lifecycle.go` 初建资源仍按旧写范围文本判断，外层使用当前 `workspace`，令后续 Git 授权不同于已冻结资源，原生准备一直未结算。统一当前契约，在首个 agent 资源操作前固定完整 Git 元数据授权，后续不得扩张。
- changed-config fixture 重载后未解析项目身份，原生清理 codec 拒绝空 owner；补原项目解析，保留原配置/原会话/人工门槛和故障断言。可选权限 fixture 补显式目录，未启用 live 测试。
- process host 在已取消 context 下先拒绝，避免创建子进程。

最终一次组合命令（原 CD66 两入口不变，补中心拒绝/rollback 回归包）：

```powershell
go run ./cmd/test-feedback -quick -- -p 1 -parallel 1 ./internal/steerline ./internal/steerlineapp ./internal/steerlineintegration -count=1 -run '^(TestCD66ExplicitCatalogAndEntrypoints|TestCD66FixedProfileRecovery)$'
```

实际 clean 源码 `755885d` 的报告：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261002T130911.299555300Z/summary.json`。准备 1.1 秒、执行 180.5 秒；core 6/6，rules 90/90（9 包，含原 CD63 两 codec 入口），targeted 22/22（中心 7、app 7、integration 8），无失败/跳过，所有组 complete。实际 JSON 核对包含离线持久查询/控制台、写入拒绝、两阶段 app-server、当前配置改变后的原生会话恢复及 Dispatch/send/task/ack 故障断言。

前置诊断报告 `20261002T125258.414211400Z/summary.json` 为失败：core 6、rules 90 通过，原生准备 deadline 失败；仅作定位证据，没有豁免。首次协调定向补丁编译发现 fixture 回调名称错误，修正后 root/app 回归通过。随后原生定向已通过准备和故障恢复，清理因 fixture 空 owner 失败，补原身份后最终组合通过。

产品 `go build ./cmd/steerline ./cmd/test-feedback`、Linux app/core 编译、4 个受影响 console 代表回归及 diff 检查通过。独立扫描 root 100 个直属生产 Go 文件无关键字/文件名命中；Windows/Linux deps 232/221，无具体实现、app、integration、processhost；三实现无反向 core/app、console 无 app 依赖。全构建标签源码 import 图另核对中心传递边，无禁用依赖。最终仅文档追加不改变上述 Go 内容；最终 PR head/base 另由平台复审记录绑定。

新所有者和旧到新映射见下表。生产搬迁已完成；#67 负责仍可编译的具体测试/fixture 和最终守卫。没有运行全量、真实业务服务/任务库/产品 CLI 或新的原生权限认证；remote 既有限制保持。

## 执行阶段候选身份（历史）

- worktree：`C:\Users\lmas1\.codex\worktrees\6aa1\SteerLine`。
- 基线/当前 Git HEAD：`5bbbfcd6a16d34cdfd77dfa8894c1661a885b799`（#65 合并基线，开始时与 origin/main 一致）。当前 detached HEAD，候选为未提交修改。
- 最终 Go 源码集合 SHA256：`c12bb264191a126e00eb08dd371a3d343dd84084b4b7e23ad75af8dbae9a83a7`，457 个现存 Go 文件及 go.mod/go.sum。逐文件摘要与集合算法见 [source-manifest.json](source-manifest.json)。此摘要是源码集合摘要，不是 Git commit。
- 最终安全定向报告、最终构建和编译检查对应此源码集合。其余测试报告对应更早的 dirty tree，不能用来认证最终候选；未保存的中间源码集合摘要不作补推。

## 所有者、入口与删除

| 消费者/职责 | 候选的唯一接线/所有者 |
| --- | --- |
| 三个内置实现的 key、codec、工厂、静态支持规则和工具查询 | `internal/steerlineapp/directory.go`，通过 `NewBuiltinDirectory` 每次创建独立不可变目录 |
| 配置受理、默认目录、安装路径、项目枚举 | `steerlineapp/config.go`；变更使用 LoadConfig，只读使用 LoadQueryConfig，共用配置解析和版本检查 |
| cmd 服务启动、已有配置启动、服务运行 | `steerlineapp.StartService/StartConfiguredService/RunConfiguredService`；根包接收显式目录、生命周期 resolver、会话来源和 process host |
| Claim、任务变更、清理及交付 | app NewCommandService 注入同一 Config/目录；中心在写入和目标访问前执行通用资格检查，保留 intent/receipt/事务 authority |
| CLI show/list/graph/export 与控制台只读 | 使用现存 Reader 和 LoadQueryConfig；缺启动支持时保留持久读取，新的变更拒绝 |
| console | app 提供 ConfigSource、QuerySource、ControlClient、路径和 raw normalizer；console 不导入 app，不隐式构造来源或实现 |
| 能力认证 | app 解释具体原生证据，中心保留报告存储、固定身份及通用 requirement/evidence 检查 |
| 本机进程、身份、文件写入 | `internal/steerline/processhost`；由 app 组合；中心仅依赖 managed process 端口，并只关闭自己拥有的句柄 |
| test-feedback | 保留现有构建/quick/定向 JSON 入口；测试通过外层构造或中性目录，不给工具添加另一套内置目录 |

根包保留通用 `ExecutionDirectory`、版本/重复 key/必需回调校验、固定快照身份、事务及恢复状态机；无全局注册、init 注册、旧默认 NewService 或隐式平台目录。目录返回值复制 metadata slices，两个实例的工厂和配置互不污染。

删除的根包生产文件：`execution_registry.go`、`profile_codecs.go`、`execution_capability.go`、`codex_git_sandbox.go`、`orca.go`、`orca_remote_adapter.go`、`orca_worker_prepare.go`、`projects.go`、`spawner.go`。保留的 `execution_snapshot.go` 仅有通用绑定逻辑。无生产消费者的旧 SpawnerFor/LocalSpawner 实现只保留在明确的 `_test.go` fixture，原有可选原生断言未删除、未启用；其后续测试归位属 #67。

固定配置恢复不重选当前同 ID profile；资源工厂不能改变 owner/key/codec/version/target，返回资源经注册 codec 验证后才可提交。绑定校验失败与事务失败共用候选清理，恢复已提交的私有域并清除 pending operation ID。worktree 根路径必须显式提供，默认路径由 app 受理时填入。

## 格式与 #65 语义

本次没有改变持久格式：内置配置版本仍为 `builtin-execution-v2`，fixed profile v5、具体 profile codec v1；#65 的 conversation v2、SQLite v11、runtime v16 保持。新增目录、process host、应用路径是非序列化接线；不增加旧版读取、迁移、双写或回退。

中心继续拥有准备 nonce/deadline、准备与业务分流、手动恢复门槛、intent/receipt、原身份对账、消息提交后 ack、DeferredDecision/Feedback 和唯一 TaskDelivery。原准备超时行为未增加额外时长。原生 session/Dispatch/send/Task/ack 故障断言保留在外层恢复用例；本机权限和原生在线支持没有因本次结构迁移获得新认证。

## 旧到新测试映射

| 原位置/断言 | 新位置/命中入口 |
| --- | --- |
| 根包 config_backend/global_execution_config/global_execution_example/home/projects 测试 | 相同文件名移入 `internal/steerlineapp`，保留实际外层 loader/home/project 断言 |
| 根包 execution_backend 原生 evidence key/version/target、agent/terminal proof、持久 evidence 断言 | `internal/steerlineintegration/capability_composition_test.go`，由 CD66Explicit 的两个 evidence 子用例调用 |
| publication 原具体支持矩阵 | app CD66Explicit `/publication_support_uses_the_selected_catalog`；publication 包保留通用 provider/secret 断言 |
| 根包同包 Service/Claim/规则 fixture | `core_decoupling_66_test_helpers_test.go` 的显式中性目录、config 与端口；同包测试不反向导入 app |
| 原集成 app-server 两阶段两轮/故障恢复 | CD66FixedProfileRecovery `/original_app_server_two_phase_turns`，复用既有完整 helper |
| 原 native session/重启/人工门槛及故障断言 | CD66FixedProfileRecovery `/changed_config_keeps_native_original_identity_and_manual_gate`，改当前配置后重新加载 app，核对原 fixed JSON、会话和资源 |
| console 选择、request digest、固定执行摘要、raw normalizer/activity | 显式注入公共来源的原测试；保留 mismatch gate 和原业务断言 |
| 原 spawner、本机可选权限、linked worktree helper | 保留原测试断言及 opt-in 开关，实现/工具查询仅在 `_test.go` fixture；本轮未运行真实原生入口 |
| 资源 owner 与提交失败候选恢复的新缺口 | 根包 TestCD66CandidateResourceIdentityAndCommitRollback，含缺显式 workspace root、owner 替换拒绝及 invalid binding rollback |

root fixture 的 LoadConfig/home 等辅助已改为明确的 `loadFixtureConfig/fixtureHome` 命名；它们不是生产 shim。没有为测试导出任意中心私有写入接口。

## 执行阶段实际验证（历史，不认证协调最终 head）

本轮使用 worktree 内 Go cache，未跑全量测试、真实产品 CLI、服务或任务数据库。测试资源限隔离 Git/SQLite/协议 mock；fixture 中的 Git 子进程失败后未重试该组，也未更换权限。

最终源码上以下命令成功：

```powershell
go build ./cmd/steerline ./cmd/test-feedback
go test ./internal/steerline ./internal/steerline/console ./internal/steerlineapp ./internal/steerlineintegration ./cmd/steerline ./internal/steerline/integration ./internal/steerline/publication -run '^$' -count=1
go run ./cmd/test-feedback -out bin/cd66-validation/20261002/final-safe -- -p 1 -parallel 1 ./internal/steerline ./internal/steerlineapp -count=1 -run '^(TestCD66ExplicitCatalogAndEntrypoints|TestCD66CandidateResourceIdentityAndCommitRollback)$'
```

第二条仅编译七个受影响包，不执行测试体。最终定向报告 **11/11 pass，0 fail/skip，6.689 秒**：app 命名入口及六子用例共 7，中心候选入口及三子用例共 4。

规定组合只运行一次，命令为：

```powershell
go run ./cmd/test-feedback -quick -out .tmp-cd66-feedback -- -p 1 -parallel 1 ./internal/steerlineapp ./internal/steerlineintegration -count=1 -run '^(TestCD66ExplicitCatalogAndEntrypoints|TestCD66FixedProfileRecovery)$'
```

结果：**失败且不完整**，准备 1.208 秒、执行 36.919 秒。以下计数含 parent/subtest，按原始 JSON 实际终态事件计算。

| 报告/组 | pass / fail / skip | 结果及限制 |
| --- | --- | --- |
| required-combination / quick-core | 5 / 1 / 0 | 错仓库/分支/base/closed-unmerged 交付拒绝通过；Claim fixture 在目标 Git 操作前因非 canonical profile 失败，随后修复 fixture，但受 Git fixture 限制未重跑该用例 |
| required-combination / quick-rules | 86 / 0 / 0 | 8 个包 pass，projection 构建失败 `undefined: HostForExecution`；Go build JSON 使用 ImportPath 的事件令现有解析器报 incomplete，整体 coverage 不能记为通过。随后修复 projection fixture，定向两个回归通过；未重复规则矩阵 |
| required-combination / quick-targeted | 7 / 8 / 0 | app 7 pass；integration 两 parent + 六子用例 fail，均在隔离 Git push 期间失败，尚未到业务断言 |
| safe-after-fixes | 16 / 0 / 0 | 12.639 秒，含当时 root/app、两个 projection 和四个 console 代表；早于最后入口/fixture/路径调整，只作调试证据 |
| final-app | 7 / 0 / 0 | 15.083 秒，早于最后中心无用辅助外移及路径默认拒绝；最终 app 结果以 final-safe 为准 |
| final-safe | 11 / 0 / 0 | 最终 Go 源码集合，目录隔离/静态拒绝/资源身份/候选 rollback 通过；不替代规定组合 |

quick-rules 实际命中 `execution/TestCD63CodecIdentityAndVersion` 及 `profile_envelope_scope_and_deep_copy`、`resource_envelope_identity_and_bounds`、`formatted_JSON_round_trip`，以及 `orca_remote_worker/TestCD63CodecIdentityAndVersion`，均有 run/pass。全部测试事件见 [test-hits.json](test-hits.json)，按包、parent/subtest 保留真实动作；报告摘要和原始日志哈希见 [validation-results.json](validation-results.json)。

## 依赖与源码检查

最终重新运行 Windows/Linux `go list -deps ./internal/steerline ./internal/steerline/execution ./internal/steerline/contracts`，分别 232/221 个依赖，具体实现、app、integration、processhost 依赖零命中。对这些本项目依赖目录的全部非测试 Go 文件检查导入（包含不同 OS/构建条件的源码），174 个文件零违规边。三个具体实现无反向导入根包/app/integration，console 无 app 反向导入。

根包直属非测试 Go 文件 100 个，大小写不敏感文件名/源码 `codex|orca` 零命中，无忽略名单。这是 #66 生产检查；尚未宣称 #67 的全部测试清零、守卫及第三实现闭环通过。详见 [boundary-checks.json](boundary-checks.json)。最终 `git -c core.autocrlf=false diff --check` 结果记录在 validation-results.json。

## 执行阶段原始报告位置与权限限制（历史）

原始报告保存在本 worktree 已有忽略规则 `/bin/` 下的 `bin/cd66-validation/20261002/`。`required-combination`、`safe-after-fixes`、`final-app`、`final-safe` 各有原始 summary、JSONL、stderr；依赖列表为 `deps-windows.txt/deps-linux.txt`。这些原始文件不是本 PR 的跟踪内容；提交的 JSON 记录保留其摘要及测试事件。

原始报告没有改写。旧路径 `.tmp-cd66-feedback` 已移为 `bin/cd66-validation/20261002/required-combination`，`.tmp-cd66-safe` 已移为 `.../safe-after-fixes`，因此旧 summary 内 stdout/stderr path 仍指向原路径。大 worktree_status 字段含当时 cache 路径，摘要文件不复制该噪声字段。

创建 `codex/66-composition-dependency-inversion` 分支首次失败：

```text
cannot lock ref ... Unable to create C:/Project/SteerLine/.git/refs/heads/codex/66-composition-dependency-inversion.lock: Permission denied
```

integration fixture Git push 的失败证据：

```text
C:\Program Files\Git\usr\bin\sh.exe: *** fatal error - couldn't create signal pipe, Win32 error 5
fatal: Could not read from remote repository.
```

未重试 Git ref/fixture，未请求改变权限，未提交或创建 PR。协调会话须先对这个最终源码摘要独立复审，在可写 Git refs 和可执行隔离 Git fixture 的环境建立分支、提交及 PR，并在最终 commit 上完成规定组合验收、核对实际命中和格式/恢复断言后决定合并。离线持久查询、Claim 和固定 profile 恢复集成仍未验证通过；不能以本轮安全定向结果替代它们。
