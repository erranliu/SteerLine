# Orca 单终端真实调试记录（2026-10-10）

本页只归属 [Orca 终端收敛](orca-terminal-transport.md)，覆盖 OTT-A5–A9；普通开发，不更新 roadmap。源代码已 rebase `872dba44`，实现 PR [SteerLine #284](https://github.com/erranliu/SteerLine/pull/284)。真实环境由 `scripts/new-debug-release.ps1 -Orca` 生成，仅使用 SteerlineTest 的专属 base/head，未使用或修改 `C:/SteerlineRuntime`。

## 本机产品闭环：通过有限样本

包：`C:/SteerlineDebug/dev-20261010-180722-4b23fdcc-dirty`，包含随后提交为 `468ea11b` 的 screen/激活修复；最后增加的 draft 拒绝保护另有单元及两目标受控回归，不把它冒充已重新跑过真机。

原 runtime `fb7953d3-29a6-4f81-b6b9-fa257bb98d04`，Orca 1.4.224；终端 `term_0e0c9170-95c1-4e23-b1c6-9f6b359caec7`，incarnation `7999acad-1c0f-4d46-96be-c14633e41691`；run `62a748eb5f7178ad79ef612102aca418`，一个 attempt。

| 项目 | 事实与证据 |
| --- | --- |
| implement.execute | Command `3909e86c9f415e206373a5755b0ec525`；只提交 result.txt，候选 `5ca9418904080652fd72e5d0f0d0f09f8acd2610`；未提前发布 |
| implement.report | Command `c5c2bb982447f4dd789847f48bf4f8d5`；同终端继续，解码业务 JSON，前后 revision 相同；没有第二个 agent 终端 |
| publish | Command `19774475182340b0af86971fbe3d1789`；发布当前实际分支，中心核对原候选、PR head/base/repo；没有追加 publish.report |
| PR 真值与交付 | [SteerlineTest #201](https://github.com/erranliu/SteerlineTest/pull/201)；base `codex/debug/dev-20261010-180722-4b23fdcc-dirty/base`，head `erranliu/sl-1-62a748eb5f7178ad79ef612102aca418`。仅将该测试 PR 转 ready/合并，merge `c57dc61f936a15ebef6114ac505cf143f6160d5e`；中心随后登记 TaskDelivery，任务 1 closed、任务 2 dependency_ready=true |
| 投递/读取 | 三轮各一个原生 request；固定 screen 来源，base64 结果正文保持原字节。没有重发业务输入或补 Enter，没有回调、结果文件或辅助终端 |
| 清理 | `evidence/service-stop.json` 为 stopped / process_exited / locks_released；`cleanup-binding.json` 与 `terminal-close.json` 保持原 runtime/handle，ptyKilled=true |

可复核材料：包 `evidence/current.json`（三轮输入/结果/封套）、`merged.json`（PR 真值、TaskDelivery、后继）、`test-pr-201.json`、原生 screen 与关闭回执；`logs/live-flow.log` 为产品阶段迁移。#284 本身未合并。

## 远端产品闭环：未通过

环境 `e68f4edf-8a83-42c2-a9da-6d404d3aa3d8`（台式机64g），runtime `95d30a41-2f86-43ae-ac0d-9c304b5f1f90`，Orca 1.4.223。所有调用仍来自同一个本机 CLI，只增加 environment；原生 project setup-clone 准备远端仓库，不需要远端访问中心。

1. `dev-20261010-175730-4b23fdcc` 的原生 Claude launcher 报未安装；产品 readiness 超时，零业务投递。
2. `dev-20261010-180653-4b23fdcc-dirty` 的 Codex startup 出现目录信任菜单，Orca idle 没有挡住它，业务输入落入退出后的 PowerShell；产品保留 indeterminate，没有创建测试 PR、交付或重发。
3. 同包独立准备会话 `term_38841eeb-9f02-4aff-b182-ce0ae8748569`：处理本测试仓库的原生信任菜单后，默认 gpt-6.1-sol 请求被当前 ChatGPT 账号以 400 拒绝。通过原生 `/model` 菜单选择 `s session`，仅该测试会话改为可用 gpt-6-luna，随后 READY 短输入及 nonce/base64 计算回复均成功；这是原生通信探针，不能替代产品闭环。
4. `C:/SteerlineDebug/dev-20261010-181633-468ea11b`：尝试在产品原会话准备模型时，启动中发送的 `/model` 留在 draft，后来与业务输入拼接；原终端 `term_c8ba4779-11c4-4ee3-9339-2c68f417663f`、run `ba8ad67858e644609176c469ded5bba5` 保持未核实。准备脚本超时，本场景明确失败。其启动 screen、原生输入回执、start-task indeterminate 保存于 evidence。

第 4 项后补了通用保护：两次 readiness 均用公开 `terminal read --screen` 检查 draft，非空则拒绝且零 send；完整结果也不能在非空 draft 下结算。适配器没有解析菜单、模型名称、代理日志或配置。空 draft 与 native idle 不能证明原生启动菜单已结束，缺口仍在；远端需先具备已准备好的原生 agent/模型环境，再补产品验收，不宣称无人值守稳定性。

以上远端包和独立准备会话均已停止服务/关闭原 terminal，保存原身份与 ptyKilled=true 回执；没有更改远端全局模型设置。旧本机实验 PR #199 提前发布，与阶段契约不符，已关闭，仅保留历史证据。

## 自动回归：通过

| 范围 | 最小命令 | 结果 |
| --- | --- | --- |
| 紧凑帧/公共原语/生命周期 | `go test ./internal/execution/workerreturn ./internal/execution/adapters/protocol/orca ./internal/execution/adapters/orcatransport -count=1` | 最终 draft 保护后通过；`C:/SteerlineDebug/orca-final-unit.log` |
| 目录与配置 | `go test ./internal/app ./internal/console -run 'Terminal\|Orca\|CurrentExample' -count=1` | 通过 |
| 声明结果 | `go test ./internal/execution -run 'Declared' -count=1` | 通过 |
| 两目标连续阶段与重启 | `go test ./tests/integration -run 'WorkerStageOrca(Terminal\|Environment)RoundTrip' -count=1 -timeout 5m -v` | 最终 draft 保护后本机 85.60s、environment 77.74s，总 168.431s，通过 |
| debug 包范围与冻结 | `go test ./tests/e2e/internal/driver -run '^TestDebugReleaseRuntime$' -count=1` | 通过，5.889s；模型替身夹具，不作 Orca 真机证明 |

受控阶段最初失败是模型替身仍匹配旧版 publish 提示词；改为 rebase 后的“使用当前 checked-out branch”文案后通过，不以放宽生产分支核验处理。未跑 full。stage/native 的原始测试进程输出保留当前聊天工具回执；早期失败日志在 `C:/SteerlineDebug/orca-final-integration.log` 与 `orca-stage-investigation.log`，保留失败、不覆盖为通过。
