# Plan：Orca 自动选择项目仓库

2026-10-14。普通开发，不纳入 roadmap。[Spec](orca-terminal-transport.spec.md#项目仓库选择2026-10-14) · [专属验收](../acceptance/orca-project-repository.md)。

## 实施步骤

1. 删除执行方案中的 repository_selector，Orca profile codec 升为 3；严格拒绝旧字段和旧 codec，不增加兼容读取。
2. 工作区创建从项目 RepositoryPath 取得仓库：无 environment 使用项目路径，有 environment 用原生 repo list 唯一匹配实际目录名，再固定仓库 ID 和 runtime。发现失败明确拒绝，创建后身份不一致保存原回执并保持未核实。
3. 恢复仅核验已保存的仓库/runtime/工作区回执，不重新按名称寻找资源。更新示例、生成器及受影响夹具。
4. 执行下列最小验证，结果登记专属验收；回归只使用隔离夹具，不启动真实 worker 或 PR。
5. 用户明确要求同步安装版时，先准备新版程序和无仓库字段方案，结算外部 worker 并关闭服务/控制台。旧冻结方案不兼容时不改写快照；只有用户明确授权丢弃运行历史后才重置该项目执行状态。通过正式任务 API 重建原定义，核对任务、契约、依赖与 Git 工作区保持；安装通用方案，清除代理专用颜色环境后重开，保持 manual、零业务领取。

## 测试计划

环境统一为 Windows、当前源码、现有隔离夹具；实现后执行，原始输出写入本次临时证据目录，摘要按验收项登记 [专属验收](../acceptance/orca-project-repository.md)。

| 级别 / 场景 | 自动项 | 命令 | 通过条件 |
| --- | --- | --- | --- |
| 定向单元 / OPR-SELECT | OPR-A01、OPR-A02 | `go test ./internal/execution/adapters/orcatransport -run 'Test(WorkspaceRepository|RemoteRepository)' -count=1` | 本机路径正确；Windows/Unix 远端路径按 basename 匹配；忽略显示名称与 folder；无匹配、重名、缺 ID、截断/畸形列表在创建前拒绝；创建身份不符不继续启动 |
| 定向恢复 / OPR-RECOVERY | OPR-A03 | 同上及 `go test ./internal/execution/adapters/orcatransport -count=1` | 原目录清单变化不改变已创建工作区；原创建回执可恢复；不一致回执恢复仍拒绝，无重新发现/创建 |
| 定向配置 / OPR-PROFILE | OPR-A04 | `go test ./internal/app -run 'Test(Terminal|CurrentExampleProfiles|OrcaSession|.*Profile)' -count=1`；`go test ./tests/e2e/internal/native -run 'Test(WorkerConfigsPreserveNativeModelAndChainSettings|OrcaSmokeProfileUsesExplicitEmptyArgs)' -count=1` | 新示例与冻结方案有效；旧字段拒绝；测试配置不携带仓库字段 |
| 编译/语法 / OPR-BUILD | OPR-A04 | `go test ./cmd/steerline -run '^$'`；PowerShell Parser.ParseFile 检查 `scripts/new-debug-release.ps1` | 产品入口编译；生成器无语法错误，Orca 生成配置使用 codec 3、无仓库选择参数 |

安装维护的附加核验映射到 OPR-A04：用户授权后运行安装程序 `check -config C:\SteerlineRuntime\etc\merge-h-308-code.json`，通过正式 `control -action resume_dispatch -wait` 恢复 manual/unpaused；一次性维护核验脚本检查 7 个方案/4 个 Orca 方案、旧字段与项目专属标签为零、原定义/契约/依赖不变、15 个任务 open、执行历史为零、服务 started，以及控制台可见颜色属性。证据为维护准备目录的 reset-result.json 和 installation-verification.json，结果只登记验收文档。该安装操作不作为调试现场，也不建立兼容/迁移入口。

额外尝试既有本机/environment阶段往返及 testenv 编译，若被仓库既有编译错误阻断，记录错误并以 HEAD 原文件 overlay 复核，不能计为通过；不改变上述最小自动项的验证入口。不选全套 L1/L2/L3 或 full：本次改变仓库发现与配置，未改变业务阶段/PR 协议。真实远端启动未列为本次必要自动项，人工核验可在更新版本的新独立调试包中后置，不阻塞无关任务。
