# 部署时显式放弃旧运行（2026-11-05）

用户要求为部署增加显式放弃旧运行的选项，避免每次升级 Runner 手工归档状态、恢复任务。本次普通开发基于 `df18050e87a2ef7852c71224063480407527c6c8` 加当前工作区差异，source_dirty=true；验证在未提交工作树上执行，未使用新开关操作实际安装。此前实际安装维护另见[原记录](deployment-discarded-runs-20261105.md)。

## 行为与边界

- `build-deploy-all.bat -DiscardRuns` 或 `build-deploy.bat -UpdateRunner -DiscardRuns` 显式选择新 Go 准备目标 `discard-runs`，作用于所选 home 的全部项目。缺少 UpdateRunner 或原安装时，在构建前拒绝；默认 update/replace 不进入该分支。
- 完成构建及候选静态契约检查后，按原入口停服，核对原宿主/Web 退出、成员、冻结配置、原 pin、项目锁及 checkpoint。通过正式任务校验与创建入口在独立目录先准备完整任务图，再由原固定 Runner 公开冷查询取全部 Run/input，为可停止的原 Run 持久保存 stop 信封，启动 worker 前受理，沿原 request_id 查证，再正式停服并核实资源结算。
- 不放宽默认升级排空判定，不强杀、不重发未知输入、不迁移旧 schema/namespace。未核实停止、未知原控制、Run/input 变化、配置/成员变化、锁未释放、未配置的残余项目状态均阻断；只在显式放弃路径允许丢弃已证明原生资源结算后的业务/报告状态。Orca 等资源证明保留原 scope，不扩大为所有后代资源或凭据已清理。
- 所有任务编号、标题、类型、优先级、目标、完整固定契约和依赖保留；重新创建为 open，manual 且 paused。原执行、领取、交付、环境资格、测试生成、Roadmap 和控制历史只保留于归档，项目/Web 配置、执行方案、默认方案及业务 Git/PR 保留。包含原已完成、已放弃任务重新 open 的明确语义。
- 原 state、application-api、Runner namespace 及存在的 Web 状态原子移入 `<home>/maintenance/discard-runs-<调用ID>/`，准备好的任务库随后就位，继续原安装与摘要核验。`-NoStart` 无需临时启动服务或事后导入。目录只按固定边界移动，不遍历删除其内容。
- `original-shutdown.json`、`discard.json` 保留原诊断、原请求、证明、数量和移动进度；切换中的 `<home>/deployment-discard.json` 阻止当前版本 start/serve、后续准备及升级核验。中断保留现场供核实，不自动回滚或重复取消。写私有文件使用现有 WritePrivateFile，不改安装根目录 ACL。

## 隔离验证

Windows/amd64、Go 1.26.4；全部在隔离 home/repo/DB/Runner/provider 夹具及独立外部输出中执行，不用安装环境作测试。无固定提示词、材料或黄金样本变更。

证据根：`C:\SteerlineDebug\wt-fb57-discard`。下表路径均相对该根；完整受测来源、实际 argv、执行参数及清理信息以各 summary 为准。

| 验证 | 结果 | 证据 |
| --- | --- | --- |
| 新增 TestDeploymentDiscardRuns 定向 | passed，124.684 秒；离线完整归档、任务图/契约/依赖保留、业务状态重新 open、暂停派发、持久 stop 顺序、未知/失败/错请求/错输入/错目录拒绝、中断启动围栏 | `diagnostics/resume-fence-20261105T074351524Z-aaf229440302414aa8960d3d300760d8/summary.json` |
| 固定 L1 | passed/quick；准备 1.459 秒，执行 38.690 秒 | `test-results/L1/20261105T074611947Z-86f41c96721c4bf48d1b70f4703d19af/summary.json` |
| 固定 L2 | passed/e2e，cleanup_complete=true；准备 56.734 秒，执行 235.056 秒 | `test-results/L2/20261105T074657963Z-0cfed7e5e7d74b198599e00256f5dd86/summary.json` |
| L4 go.deployment（后置围栏补丁前） | passed/local；执行 708.504 秒；host、Windows localservice 全量，App 10 根及全部子项，原升级排空根；保持 count1/25m/p1/parallel1 | `test-results/L4/20261105T072739970Z-4954fa5750b747949a45f0e907981168/summary.json` 及 `managed-go.deployment.json` |
| PowerShell 5.1 停止脚本及部署流水线 | exit 0，14 场景通过；包括缺 UpdateRunner 提前拒绝、显式目标、NoStart、构建/停止/排空失败阻止安装、原默认模式 | `diagnostics/scripts-final-20261105T072822035Z-c89e8bd5b4fb47159f6a80d91d9c0853/stop.log`、`pipeline.log`、`pipeline/result.json` |
| 清单及领域源范围 | passed，22.135 秒；机器清单/E2E 一致性及 AppDomain 相关测试 | `diagnostics/catalogue-20261105T073416280Z-7cacdb594b8c41e080891496c0122d44/summary.json` |

完整部署专项通过后，最终核对补充了“已停止但仍有未完成 resume 授权时，不得重开原 Runner”的拒绝条件。该补丁仅影响显式放弃分支；表中定向全项、L1、L2 已按最终版本重跑通过，未再次重复与该条件无关的原有重型 owner 全集。

新增根已加入 go.deployment 的正式源清单，Windows App 根数 9→10，其他平台 8→9；测试指南、可读列表和 JSON 同步。未缩窄原断言、超时或参数。

此前失败均保留：沙箱临时目录/Git/私有 ACL 限制；夹具错误地通过创建口更新已有任务，随后改为正式创建及 CAS 放弃口；维护记录原先使用会更改目录 ACL 的 Write，隔离测试发现后改为 WritePrivateFile；PowerShell 预期错误捕获修正；L1 缺 Vitest 依赖，按锁文件 npm ci 后在新目录复跑。原失败报告不改写、不计通过。

本轮没有执行 go.full、真实模型、远端原生会话停止或真实 PR 交付；已有发布 Runner 的实际停止契约本次仅复用公开接口和隔离调用方验证，不冒充新开关的真实安装验收。

## 独立批处理入口

用户随后要求提供类似 build-deploy-all.bat 的专用入口。新增根目录 [build-deploy-discard-runs.bat](../../../build-deploy-discard-runs.bat)，固定调用 build-deploy.bat 并传入 -UpdateRunner -DiscardRuns，保留其余参数、暂停行为及退出码；文档首选命令同步到新入口。本轮仅新增四行转发入口及文档，没有修改 Go 实现或执行实际部署。

在独立含空格目录中复制新入口，以不具备部署能力的同名下游 stub 实跑，核实两个固定开关、含空格 InstallHome、NoStart/NoPause 转发，以及退出码 0/23 原样返回。证据为上述根下的 diagnostics/bat-entry-20261105T080312463Z-07b6a45444574ee98ab8b7b981f35bf3/result.json。

按项目规则补跑固定 L1、L2，均 passed，L2 cleanup_complete=true。证据为 test-results/L1/bat-entry-20261105T080320091Z-cf870de2bb3b45e3b3e647a2e7b30e01/summary.json 和 test-results/L2/bat-entry-20261105T080415853Z-9c0b29a9fd454f9fb444ee1cf031aa4c/summary.json。未重复业务实现未变的部署重型专项。
