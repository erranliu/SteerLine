# Spec：分级测试与共享 E2E 环境的最小实现

目标：`TIERED-E2E-TESTING`。状态：已实现并通过最小验证，待实现 PR 评审；未分配 Task ID、创建中心任务或绑定现有任务。日期：2026-10-01。配套 [Plan](tiered-e2e-testing.plan.md)。本期交付一个实现 PR，实现与证据见[验证记录](../validation/tiered-e2e-testing/README.md)。

## 1. 目标与首期范围

建立可以独立迭代的 L2 smoke 和 L3 分组端到端测试；两级共用测试项目、任务及进程环境管理。测试环境、夹具和运行入口属于项目开发内容，随产品改动维护。每份新编写或本次修改的实现 Plan 可以指定测试级别、场景、环境、执行时点与通过条件。

本期固定为：沿用当前 L1 快速入口；一个 `controlled` 环境；一个 L2 交付闭环；一个 L3 `recovery` 组中的等待合并后服务重启场景。首期支持当前 Windows 开发机，串行执行，不要求新增平台认证。

不实施真实 Codex/Orca、远端、UI、并行 E2E、持续调度、通用故障脚本语言、跨运行资源回收服务或完整场景矩阵。后续增加环境或场景组时扩展当前边界，不预建多后端框架。本期不改变产品业务状态机、数据库格式、权限与认证门槛，不添加生产测试模式、直接写库的造状态入口或旧版兼容路径。

| 级别 | 职责 | 首期入口及覆盖 |
| --- | --- | --- |
| L1 | 日常最小回归及改动相关规则/接线 | 当前 `go run ./cmd/test-feedback -quick`；按 Plan 追加定向测试 |
| L2 | 当前构建经正式入口完成关键闭环 | `smoke.delivery`；真实 SteerLine、Git、SQLite，受控 worker/provider |
| L3 | 按业务组独立扩展的 E2E 场景 | `recovery.waiting-merge-restart`；使用同一环境管理能力 |

三级不是强制逐层全跑的流水线；Plan 按改动选择。现有 `-workers=3` 是完整确定性 Go 回归，保留其语义，不等同于 L3 或真实平台验收。已绑定任务的既有验证门槛不由本 Spec 自动替换。

## 2. 包边界与依赖

与产品共用当前仓库和 Go module，采用以下最小目录结构：

```text
tests/e2e/
  README.md
  suites.json
  smoke/delivery_test.go
  suites/recovery/waiting_merge_restart_test.go
  internal/testenv/          # Run/Scenario/Project/Task、资源、生命周期、证据
  internal/driver/           # 正式 CLI/控制客户端与公开只读查询
  internal/controlled/       # worker/provider 的受控状态与协议行为
  testdata/projects/         # 一个最小项目模板
  testdata/tasks/            # A -> B 的 Spec/Plan 与任务批次材料
  testdata/controlled-helper/ # 可独立构建的测试外部组件进程
```

上述目录可以各含少量文件；不把 Project/Task/资源管理提前拆成更多包。测试辅助可执行入口放在 `testdata`，由 E2E 入口显式构建，避免普通包枚举收录辅助命令。

依赖方向为：场景 → testenv/driver/controlled；testenv → driver/controlled；driver → 正式 CLI、已有公开控制客户端、公开只读查询与 DTO；controlled → 当前协议契约。生产包和命令不得导入 E2E 包。driver 不直接执行 SQL，不导入业务包内私有状态。

L2、L3 各有原生 Go 测试包及场景断言，不调用对方的测试函数。共享创建项目、建任务、启动、等待、重启、取证与清理操作；业务预期保留在场景内。环境管理不重新实现任务状态机或业务交付规则。

L2/L3 场景测试加 `//go:build e2e`，仅显式入口启用。辅助包的少量纯逻辑测试可按普通 Go 测试运行，不启动 E2E 环境；两项目隔离等会启动项目的基础设施测试也加 e2e tag，只通过定向命令显式执行。保留既有测试原位置和默认覆盖；不能借本期拆包把已有默认回归移成显式跳过。

## 3. 统一环境与项目/任务管理

### 3.1 所有权

| 对象 | 首期职责 |
| --- | --- |
| Run | 一次执行；当前源码信息、一次构建的产品/helper、选择清单、产物根和总报告 |
| Scenario | 一次场景；独立 scratch 根、资源清单、有界 context、清理及场景证据 |
| Project | 场景内项目；独立 home、仓库、bare remote、项目配置、SQLite、服务与工作区 |
| Task | 通过产品创建的任务；Spec/Plan 引用、任务 ID、依赖及公开执行身份 |

每个场景建立新现场；同一场景允许创建多个项目/任务，首批场景各只创建一个项目和 A → B 两个任务。每个项目的 home 等可写资源置于该场景的独立项目子目录，同场景的两个项目也不共享这些资源。不同场景不共享可写仓库、home、数据库、服务、provider 状态或工作区。构建产物只在同一次 Run 内复用，不加载其他运行的旧二进制。项目模板不可变，复制后再初始化独立 Git 身份。

模拟项目使用真实 Git、SQLite 和当前配置，模拟任务使用当前原生任务流程。Spec/Plan 必须生成在项目内并真实提交；根据当前契约绑定路径、blob/commit 及版本，通过正式 `task validate/create` 创建，读取真实创建回执，不预填 WorkflowRun、Execution、Attempt 或 Delivery。

驱动以产品 CLI 创建、启动、停止和查询；领取使用现有公开控制客户端的 `start_task` 正式路径，提交完整 Profile ID/digest，由正式服务核验。当前 CLI 没有 Profile 选择参数，测试辅助进程复用控制台相同的 SubmitControl/WaitControlReceipt API，不新增生产测试入口。项目首次初始化优先由正式服务/命令完成。额外断言若 CLI 未暴露必要身份，允许 driver 使用现有公开只读 reader/query，仍通过当前产品读取契约，不开放新 SQL 断言入口。不能依赖旧五步工作流或不存在的 `task claim/init` 命令。

### 3.2 生命周期

固定操作为：检查依赖 → 建场景/项目 → 建任务 → 启动服务并等待 readiness → 执行场景 → 收集证据 → 停止资源 → 删除 scratch。先创建必要项目配置，初始化、建任务和业务启动的实际先后按当前正式入口约束执行；运行模式为 manual，B 不自动启动。

每项资源创建后立即登记所有权和反向清理操作，部分准备失败仍清理已有资源。服务通过当前二进制的 `run -dispatch manual` 等现有入口作为受管子进程运行；不因 E2E 添加可见窗口要求。每个子进程显式传入本场景的环境和绝对路径，不用 `t.Setenv`/改变全局工作目录协调不同项目。

`RestartService` 保留原项目仓库、home、数据库、provider 状态和工作区，停止原服务后启动新服务并重新等待 readiness；重新运行整个场景则创建全新现场。这两种语义不可混用。

场景正常或断言失败均先保存最后一次公开状态和已有日志，再停止进程、关闭句柄、删除 scratch；产物目录位于 scratch 之外，包含原始命令、日志与结果。创建一个小型资源清单，记录本次路径及精确进程身份；不建设跨运行资源数据库。

取消、超时和强制结束测试进程时，执行器复用现有精确进程树所有权机制，确保本次启动的后代进程不遗留；Windows 使用 Job 或等价的既有受管机制，不按进程名全局停止。需要清理的目录在删除前校验确实属于本次 scratch。清理失败使本次非成功，并保留诊断与资源清单。断电后的跨运行自动恢复不属本期要求。

准备、构建、单命令、场景及清理均有明确截止时间。首期场景默认上限 5 分钟、单命令 30 秒、清理 30 秒；正式产品内部超时保持。等待查询条件或受控事件，短轮询允许，不使用固定长 sleep 判断成功。缺少 Go/Git、构建失败或当前 OS 不支持时明确拒绝，不能跳过后报告通过。

## 4. controlled 环境

当前构建的 SteerLine、真实 Git 和真实 SQLite 为被测系统；独立 helper 进程扮演外部 worker 与 command PR provider。使用现有配置指定 helper 的可执行路径及参数，遵循当前 app-server/worker 事件和 provider 输出契约，不注入 Service 指针、内部数据库或直接构造执行记录。

参考当前 `worker_local_test.go` 的协议辅助行为和 `publication/provider.go` 的 command provider 契约。仅实现首次准备/业务执行与 PR open/merged 查询所需协议；不实现任意故障 DSL。协议变化同时更新 helper，不能用忽略未知必需请求维持假成功。

helper 在真实分配的工作区中完成最小文件修改和暂存，按当前本机 host_service 契约输出固定 workflow_run_id 的发布标记；中心通过真实 Git 提交/推送并调用受控 GitHub 命令组件创建匹配的 PR。provider 状态文件只属于模拟外部平台，不能写 SteerLine 的业务状态；更新采用原子替换。场景控制 open → merged 时，先在隔离 Git remote 完成对应合并，再返回当前契约所需的真实 commit 身份及合并事实。该证据仍是受控 provider 证据，不是真实托管平台验收。

当前本机 host_service 仅支持 GitHub command provider；受控配置固定相应 GitHub remote 身份，通过现有 Git 命令配置将网络传输映射到本项目 loopback HTTP git http-backend。适配器原样转发真实 Git 的 stdout，remote 身份查询保留配置 URL，提交/推送/合并均真实执行。它只管理测试外部传输，不合成 Git 结果、不访问真实 GitHub。测试项目材料使用本机中心发布职责，不能仍要求 worker 自行提交。

记录 helper 的创建会话/业务派发等必要计数和身份，供重启场景核对是否重复派发。ready 仅说明外部组件可交互，任务交付以产品公开事实判断。将 `STEERLINE_HOME` 和测试临时目录绑定到现场；隔离会改变行为的现有真实验收/helper 开关，报告实际传给子进程的选择，不能让宿主原生开关改变本次覆盖。

## 5. 首批场景

| 场景 ID | Go 顶层测试 | 包 | 必需业务断言 |
| --- | --- | --- | --- |
| `smoke.delivery` | `TestDeliverySmoke` | `tests/e2e/smoke` | A 经正式创建/领取、真实 worker 进程及 Git 操作关联 open PR；合并前 B 阻塞；受控合并后 A 有公开交付事实、B 就绪且未启动 |
| `recovery.waiting-merge-restart` | `TestWaitingMergeRestart` | `tests/e2e/suites/recovery` | A 等待 open PR 合并、worker 业务轮已结束时停止并重启服务；保留原任务、数据库、WorkflowRun、执行及工作区身份，B 仍阻塞；无新增会话或业务派发；合并后完成同一 A、同一公开 Delivery，B 就绪且未启动 |

两场景复用环境操作及 A → B 模板，但各自创建独立现场。L2 不包含重启；L3 不通过调用 L2 测试函数获得前半程。交付后再次查询/对账，公开 Delivery 身份保持，不重复解锁或创建新的工作流。身份检查取已有公开读取契约和 helper 记录，不依赖页面文案或仅检查目录存在。

L3 首例验证服务在等待合并阶段恢复，不声明 worker 活动轮恢复、强制退出恢复、人工继续或故障矩阵已覆盖。真实平台、UI 和其他 L3 组均明确未实施。

## 6. 执行入口与结果

扩展 `cmd/test-feedback`，不另建通用执行器；本期新增以下命令：

```powershell
go run ./cmd/test-feedback -level=L1
go run ./cmd/test-feedback -level=L2 -env=controlled
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled
```

`L1` 映射当前 quick，报告沿用 `scope=quick`；保留原 `-quick` 及定向用法。L2 固定选 smoke，不接受 suite 参数；L3 必须显式指定组，可用逗号选择多组；L3 不自动附带 L1/L2。L2/L3 的 env 缺省为 controlled。L2/L3 与 quick、workers、build-only、summarize-log、原始 Go 参数混用时拒绝；L1 只允许当前 quick 支持的定向参数组合，不接受 suite/env 或重复声明 quick。未指定 level 时仅保留原入口参数，不接受新增 suite/env。组、级别、环境或重复参数无效时在构建/创建现场前拒绝。

`tests/e2e/suites.json` 是唯一场景选择清单，每项仅保存 `id/level/suite/package/test/env`，首期两个条目、env 均为 controlled。超时策略在环境管理中集中定义，不在清单中建立另一套调度配置。新增场景只需增加原生测试、必要模板和清单条目；清单不包含测试步骤。实现用 Go 标准 JSON 解析，不引入配置库。

入口确定仓库根，将输出/scratch/构建路径解析为绝对路径。一次构建产品及 helper，再按选中包用 `-tags=e2e -json -count=1 -p=1 -parallel=1` 和完整转义、首尾匹配的测试名运行；首期不调用 `t.Parallel`。通过进程环境传入构建、输出与现场根，不在每个包的 TestMain 重复构建。直接运行场景包时若缺少必需运行配置必须失败并提示统一入口，不能默认为本机生产配置。

构建阶段比较选中包当前枚举的顶层测试与清单，拒绝无匹配/未登记的场景及重复登记。覆盖核对只针对所选组，不扫描或运行全部 L3。辅助协议组件无需登记为业务场景。带 tag 的场景不能被原 quick/full 包枚举自动启用；普通完整回归报告仍明确未包含 E2E，不因新目录把只有辅助代码的包认定为有场景覆盖。

每次 E2E 报告至少包含 `scope=e2e`、level、env、所选组、计划与实际场景、源码 commit/工作树状态、构建身份、命令、准备/执行耗时、场景结果、清理结果、产物路径，以及项目/任务/工作流与 helper 身份。报告中明确 worker/provider 为受控组件。

复用现有 JSON 日志汇总和缺失终态检查；每个选中场景必须恰好执行一次且有成功终态。失败、skip、零匹配、重复/缺失、非法或截断日志、进程非零退出、超时/取消、缺失场景证据、清理失败和报告写入失败均非成功；不能只看最后一个包退出码。记录失败后不自动重跑。L2/L3 结果不能标成 `full` 或原生认证。

## 7. Plan 约定

在 E2E README 和项目指令中规定：新编写或本次修改的实现 Plan 使用“测试计划”段，列出级别、组/场景 ID、环境、命令、执行时点、通过条件和证据位置；新增行为如需要新场景，显式列出测试代码/环境改动。默认最小必要范围，未选相关组时给出简短理由，不批量重写历史 Plan。

Spec 定义需要证明的行为，Plan 选择具体测试；公共入口的存在不自动要求所有任务跑 L2/L3/full。纯文档修改做链接、格式和语义检查。环境基础设施变更与业务实现一起审阅，不能把环境搭建留成不可复现的手工前置。

## 8. 最小验收

| 编号 | 通过条件 |
| --- | --- |
| TT1 | 独立 L2/L3 包和共享环境成立；无生产到测试依赖、直接 SQL 造状态或新生产测试模式；原 quick/full 语义和默认覆盖保留 |
| TT2 | 两个场景各一次通过，走当前构建和正式入口，具有第 5 节的业务/身份断言与独立现场证据；结果明确为 controlled E2E |
| TT3 | 一个两项目隔离定向检查证明 home/仓库/数据库/provider 状态独立，A 的修改和清理不改变 B；准备中途失败及执行失败/取消的资源清理可定位，已产生日志保留 |
| TT4 | 选择、清单和日志汇总的定向检查证明非法组合、空匹配、重复/未登记、skip/失败/截断或证据缺失不能通过；取消/超时不遗留受管后代进程，清理/写报告失败非成功 |
| TT5 | L1 兼容入口及受影响测试通过；E2E README、Plan 测试段约定和 Spec 索引一致，新增场景无需修改产品状态机或 L2 测试函数 |

验证只做上述必要定向检查、当前 quick 一次和两个场景各一次；失败、新变更或具体未覆盖风险才扩大/重复。默认不跑完整 Go 回归、真机或多平台矩阵。两项目隔离与失败路径是基础设施定向测试，不扩大首批业务场景清单。
