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

## 未通过或未执行

- 集成阶段往返未执行：整个 `tests/integration` 包因候选弃用旧接口引用无法编译（`AbandonNativeCandidate`、`CandidateAbandonOptions`、`Abandonment`）。用当前 HEAD 原文件 overlay 复核，同样报错；见 `baseline-integration.log`。
- `tests/e2e/internal/testenv` 无法编译：既有 `driver/candidate.go` 仍使用已移除的 ControlPayload 字段。HEAD overlay 复核相同；见 `baseline-testenv.log`。未修复本次范围外的旧候选夹具。
- 真实本机/远端工作区、worker 和 PR 链路本次未运行。后置人工核验窗口待定，使用新版本生成的独立 `C:\SteerlineDebug` 包，远端事先注册与本包项目目录同名的 Git 仓库，并同步精确 base commit；不阻塞无关开发。
- 未部署、修改或重启 `C:\SteerlineRuntime`；任务 20 原失败现场未变。旧 profile 2 及其已冻结执行无兼容读取。
