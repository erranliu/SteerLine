# 验收：Orca 自动选择项目仓库

2026-10-14，归属本次普通开发“Orca 自动选择项目仓库”，无 Task ID。[Spec](../spec/orca-terminal-transport.spec.md#项目仓库选择2026-10-14) · [Plan](../spec/orca-project-repository.plan.md)。

## 自动验收

| ID | 范围 | 结果与证据 |
| --- | --- | --- |
| OPR-A01 | 执行方案跨项目复用；本机跟随项目路径；environment 按实际目录名唯一匹配 Git 仓库 | 已通过 `TestWorkspaceRepositoryFollowsProject`，覆盖 Windows/Unix 路径、尾部分隔符、与目录名不同的显示名称、其他项目和 folder 排除 |
| OPR-A02 | 发现失败零创建/输入；创建身份不一致保持未核实 | 已通过 `TestRemoteRepositoryDiscoveryRejectsBeforeCreation`、`TestRemoteRepositoryCreationRetainsReceiptOnMismatch`，覆盖无匹配、重名、缺 ID、截断/畸形清单及仓库身份不一致 |
| OPR-A03 | 恢复只使用原创建回执，无重新发现或重新创建 | 已通过上述路由/身份用例的恢复断言；原清单变更仍复用原工作区，身份不一致恢复仍拒绝；Orca adapter 全包回归通过 |
| OPR-A04 | profile codec 3、旧字段拒绝；示例与夹具不含目录；产品可编译 | 配置/示例/冻结 profile 定向测试、native 配置测试、产品入口编译通过；调试生成器 PowerShell 语法检查通过 |

原始证据目录：`C:\Users\lmas1\AppData\Local\Temp\steerline-repo-selection-check-jtq25k2x`。`targeted-checks.log` 记录路由、恢复、app profile 和 native 配置测试命令及通过结果；controlled 包仅编译、无匹配测试，不作为行为验证。adapter 全包检查见 `adapter-all.log`；产品入口及生成器语法见 `build-and-generator.log`。

## 首轮验证的阻塞与未执行项

- 集成阶段往返未执行：整个 `tests/integration` 包因候选弃用旧接口引用无法编译（`AbandonNativeCandidate`、`CandidateAbandonOptions`、`Abandonment`）。用当前 HEAD 原文件 overlay 复核，同样报错；见 `baseline-integration.log`。
- 首轮 `tests/e2e/internal/testenv` 无法编译：既有 `driver/candidate.go` 仍使用已移除的 ControlPayload 字段。HEAD overlay 复核相同；见 `baseline-testenv.log`。该驱动随后在本机能力验证中修正，剩余候选夹具由主干 PR #308 修正；合并后结果见下节。
- 真实本机/远端工作区、worker 和 PR 链路本次未运行。后置人工核验窗口待定，使用新版本生成的独立 `C:\SteerlineDebug` 包，远端事先注册与本包项目目录同名的 Git 仓库，并同步精确 base commit；不阻塞无关开发。
- 安装更新记录见下节；旧 profile 2 及其已冻结执行无兼容读取。

## 合并主干后的复验（2026-10-14）

PR #307 合并主干 `a8720b5f`。唯一文本冲突位于 `tests/e2e/internal/driver/candidate.go`：保留主干新增的 stop_candidate、ExpectedRunRevision 及 ControlRevision 校验，同时保留移除旧候选弃用动作和字段的结果。该文件最终与主干一致，Orca 自动选择仓库的实现与配置改动保持。

复验均通过：Orca transport/protocol 两包回归；app profile 定向测试；native 配置定向测试；driver/testenv/产品入口编译；调试生成器 PowerShell 语法检查。之前的候选旧接口编译阻塞已由主干 PR #308 修复。定向集成 `go test ./tests/integration -run '^(TestWorkerStageOrca(Terminal|Environment)RoundTrip|TestMC[789])$' -count=1` 实际执行并通过，覆盖候选停止/重建与受控本机、environment 三阶段往返；不代表真实 Orca 业务链验收。

原始日志位于 `C:\Users\lmas1\AppData\Local\Temp\steerline-pr307-conflict-1791958258219`：adapters.log、profiles-build.log、integration.log、generator-syntax.log。没有运行全量测试，没有启动真实 worker/PR，也没有修改安装环境。

## 安装版同步更新（2026-10-14）

安装方案同步为 codec 3，删除 repository_selector；移除两项 merge 专属重复方案，去掉远端方案的项目名称前缀，保留 3 个 Codex 和 4 个 Orca 通用方案。模型、effort、剩余方案 ID 及启动命令保持。仅替换程序和目录方案时，旧数据库的冻结 profile 2 导致服务明确拒绝启动（unsupported_schema），未绕过校验或加入兼容读取。

经用户明确允许丢弃旧执行记录并重置 merge 项目执行状态后，使用新版正式 task validate/create 在空状态中重建原定义，核对原 15 个任务、契约及 4 条依赖完全相同，再清除该项目旧执行/控制/UI 状态并安装。任务全部恢复 open/version 1；未备份旧数据库，未改变 Git 工作区或 PR 事实，其他项目保持。执行准备文件和 reset-result.json 位于 `C:\Users\lmas1\AppData\Local\Temp\steerline-installed-profiles-996bgxn3`；程序/方案更新前副本位于 `C:\SteerlineRuntime\backup\orca-project-repository-20261014-115724`。

安装程序实现来源为 commit `3dfa50e3e6c6b2e76fc2cf6806e80e667a93b9bc`。新控制台启动前清除代理进程继承的 NO_COLOR=1 与 TERM=dumb，避免人工可见控制台丢失样式；不修改用户全局终端变量。最终就绪及控制台颜色证据登记 installation-verification.json；本次未启动业务 worker 或发布真实 PR。

最终核验通过：服务 startup 为 started（PID 49188），控制台 PID 27008、可见屏幕有 127 个带颜色的字符格；目录中 7 个方案、4 个 Orca 方案，项目专属标签和 repository_selector 均为零。正式 resume_dispatch 回执 applied；manual/unpaused，15 个任务全部 open，workflow/execution/attempt/delivery/history 为零，无业务任务领取。完整维护证据见上述准备目录的 installation-verification.json。
