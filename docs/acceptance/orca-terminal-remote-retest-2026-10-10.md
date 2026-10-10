# Orca 远端 CLI 升级后复测（2026-10-10）

本页只归属 [Orca 单终端验收](orca-terminal-transport.md)，覆盖 OTT-A9；依据既有 [测试计划](../spec/orca-terminal-transport.plan.md)复测，不修改执行方案或增加代理专用逻辑。用户升级远端 Codex CLI 后授权重新验证。[此前记录](orca-terminal-real-flow-2026-10-10.md)保留历史失败，不能覆盖为通过。实现 PR #284 已合并，正式闭环使用主干 `a25eeedd` 生成的独立 debug 包。

## 原生环境与首次信任

本机调用 Orca CLI `C:/Users/lmas1/AppData/Local/Programs/orca/resources/bin/orca.exe`；远端 environment `e68f4edf-8a83-42c2-a9da-6d404d3aa3d8`（台式机64g），runtime `95d30a41-2f86-43ae-ac0d-9c304b5f1f90`。远端 Orca 仍为 1.4.223，实际代理启动屏幕确认为 Codex CLI 0.162.1，默认模型 GPT-6.1-Sol high fast；没有覆盖原生模型、账号或权限设置。

首包 `C:/SteerlineDebug/dev-20261010-183809-76849e2a`：新仓库有原生目录信任菜单。与升级前不同，公开 `terminal wait` 返回 `blockedReason=agent-trust-workspace`；中心拒绝准备，没有业务 send、没有重发或补业务 Enter。原 terminal `term_dc7ba211-b656-4201-8d04-a76c87bb8be3`，run `f2605c22a045eb5545641dc85cc051a9`。本次 start_task 返回 failed，信任确认后该原任务没有自动继续；不能将此项登记为自动恢复通过。

证据：首包 `evidence/start-task.json`、`before-trust.json`、原生屏幕及信任回执、`logs/live-flow.log`。随后停服务、按原身份关闭 terminal，`terminal-close.json` 的 ptyKilled=true。

## 已准备目录的产品闭环

包：`C:/SteerlineDebug/dev-20261010-184217-a25eeedd`，live host=orca，仅固定 environment 路由。测试项目始终为 SteerlineTest，base `codex/debug/dev-20261010-184217-a25eeedd/base`。

产品启动前，通过原生 project setup-clone 准备独立远端仓库。顺序创建一个无业务提示词的准备会话，仅确认该测试仓库的目录信任；确认原生 idle 后立即关闭。其 terminal `term_7e03913f-e314-4171-a399-4fc5f91ac431`，关闭回执 ptyKilled=true。该准备终端没有接收或执行任务，不参与产品输入或结果传输，也没有与产品终端同时活动。

随后启动本包服务、resume_dispatch、start_task 1，后续只做只读 snapshot/native read、专属 PR 核验与合并。原产品 terminal `term_e3aee7b0-d09b-4d71-8060-dac4a84eaf80`，incarnation `4ebc3565-52ec-4719-8640-46e5b79ab959`，worktree `f012805f-ad60-4087-856c-8ee634e572e4::C:/Users/Administrator/orca/workspaces/SteerlineTest/sl-1-c895afda7191b651ea41a1be654846d1`，run `c895afda7191b651ea41a1be654846d1`，attempt `6e17b38636d1ffa2c1be2f532aeecbea`。

本次有限样本通过：默认模型完成 implement.execute → implement.report → publish，三个阶段保持同一个 runtime/terminal/incarnation/session、一个 attempt、三个原业务输入和三个紧凑结果；没有追加 publish.report。原生 request 分别为 `86a6436f-51b1-3e94-e879-5054985144b0`、`17c4e7b7-99d8-c365-74dc-555ccbc5a073`、`9fc3ae84-9384-d067-8cdf-83d1a5ae9825`；三个 Command 分别为 `5fd77a52b58a6eb772039f09c9d83761`、`3b7cfbd09aa6921a33e6875ccc69e2a4`、`450bff3ef040f42cc125d20a1c669fef`。

| 验证项 | 结果与证据 |
| --- | --- |
| 实现/报告 | result.txt 已验证并提交，候选 `55662298cc3062f659a6304ce67936911a26a957`；报告前后 revision 一致；正文经 base64 解码仍为原业务结果。`before-restart.json`、原生 screen、产品日志 |
| 发布 | [SteerlineTest #204](https://github.com/erranliu/SteerlineTest/pull/204)；head `erranliu/sl-1-c895afda7191b651ea41a1be654846d1`，base 为本包专属 base，head SHA 等于原候选，仅一个 result.txt 改动。中心进入 await_merge。`test-pr-204.json`、`pr-before-merge.json` |
| 中心重启 | await_merge 时停服务，原终端继续存在；重启后 session/terminal/incarnation 不变，仍三个输入、三个结果、turn_number=3、pending_command=null，没有重发。`restart-stop-service.json`、`original-terminal-after-service-stop.json`、`after-restart.json`、`restart-check.json` |
| 合并/交付/后继 | 仅合并 #204 到专属测试 base；merge `e144f3d713cc3f5d4c7773115904ab6c1b2354c7`，中心登记相同 PR/merge 的 TaskDelivery，任务 1 closed、workflow completed/delivered、任务 2 dependency_ready=true 且无 startup_blockers。`pr-merged.json`、`delivered.json`、`delivery-check.json` |
| 原生可靠性 | 三轮均经产品正常 send --enter 启动并完成，未复现缺少 Enter；默认模型未再报先前的 400 拒绝。没有交错模型菜单、补业务 Enter、重发或第二个产品终端 |

业务结果来源仍是 `steerline.worker.terminal-result.v1` 的 worker_declared，不冒充 Orca 原生 final。此次运行未修改实现，也没有重复跑已通过的单元/受控测试或 full；新验证对象是升级后的真实远端环境。

原命令与回执保存在包 `evidence/`，产品迁移在 `logs/live-flow.log`。没有 worker-start、辅助传输终端、回调、结果文件或代理私有日志；所有远端调用经本机 Orca CLI 完成。无需远端连接 SteerLine 中心。

## 结论与限制

升级后的远端在已准备好原生目录信任的环境中完成单终端产品闭环，包括中心重启、真实合并交付和后继释放。本次三个输入的有限样本不能推出任意代理/版本/提示词都稳定。新仓库首次目录信任仍需原生确认；它现可被公开 readiness 正确阻塞，首包已记录零业务投递。被拒绝的原任务没有自动继续，此项不登记为恢复通过。

所有自建准备/产品终端及本包服务在验收后按原身份清理，关闭回执保存 `final-cleanup-binding.json`、`final-terminal-close.json`（ptyKilled=true），停服回执为 `final-stop-service.json`。独立包保留，安装环境未修改。
