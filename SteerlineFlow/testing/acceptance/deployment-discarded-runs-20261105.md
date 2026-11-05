# 2026-11-05 放弃旧运行后的安装部署恢复

用户要求解决实际安装部署失败，允许放弃旧运行；随后明确批准归档整个安装的项目 state、应用/Web 状态及 Runner 状态，再升级 Runner、恢复任务定义和依赖，任务恢复 open、派发暂停。本次为安装维护，未修改产品代码、固定提示词或材料，未将安装现场作为自动测试环境。

## 原因与处理

原部署在 `deploy.stop-and-verify` 阻断：应用宿主已退出，但 Run `1484b4855c66e9ccd789aac3fedd51c6` 和 `19bc87f7db6dd6bd8faf4f66c2c2ee8a` 没有原生停止结果，本机 Run 的独立 owner 仍存活。

使用原固定 Runner 的公开离线 inspect 取得 revision，经 `service start` 的 `pending_stops` 在启动 worker 前受理两个持久 stop 信封；沿原 request_id 查询，两个停止命令均 completed、`stop_execution` 均 confirmed。正式停止 Runner 后，公开 `service verify-stopped` 核实全部 11 个历史 Run 的 resources_settled=true，候选应用的 `verify-stop` 也通过。Orca 证据范围保持 `orca_agent_session`，不扩大为所有脱离会话的后台资源不存在。

跨构建升级预检仍拒绝：旧本机开发 Run 已 stopped，但保留显式可恢复状态。没有放宽升级判定或改写 Run。首次完整归档被自动审批以超出仅放弃旧 Run 的明确授权拒绝，未执行；用户补充批准后，才在部署锁及启动协调锁内重新核验无归属进程、原生资源已结算，并将精确目录原子归档。项目配置、Web 配置、7 个执行方案、默认方案及业务 Git/PR 保留。

原子归档保存原 application-api、全部项目 state、ui/web 及唯一原 Runner 目录 `C:\slr\rehnosf7lwgci2qaqusuz6z56u`。新状态使用原安装 home 重新创建，不迁移旧协议或数据库；归档不用于继续旧运行。

## 实际部署与核验

证据根为 `C:\SteerlineInstall\maintenance\deploy-stop-20261105-fb57`，原状态位于其中的 `discarded-runtime/`。正式执行原源码入口：

```powershell
& C:\Project\SteerLine\scripts\build-deploy.ps1 -InstallHome C:\SteerlineInstall -RunnerRepo C:\Project\SteerRunner -UpdateRunner -NoStart -NoPause -TimingPath C:\SteerlineInstall\maintenance\deploy-stop-20261105-fb57\deploy-timings.json
```

部署 succeeded，84.051 秒；随后沿既有部署启动代理启动安装版，全部项目 ready 后，通过正式 pause_dispatch、task validate/create 恢复任务。两个整批校验均 valid=true、零 issues；没有过滤问题或直接修改数据库。

| 项目 | 任务数 | 依赖数 | 最终状态 |
| --- | --- | --- | --- |
| merge-h-308-code | 15 | 4 | 全 open，manual/paused |
| SteerLine | 0 | 0 | manual/paused |
| SteerlineTest | 5 | 4 | 全 open，manual/paused |

新任务编号、标题、类型、优先级、目标、完整固定契约及全部依赖与导出逐项一致。三个库 integrity_check=ok、foreign_key_check 无问题，runner_candidates、project_environments、test_generations、roadmaps 均为零。项目/Web 配置摘要、Runner pin 以外执行配置及三个安装 EXE 摘要分别核对通过。

最终部署包为 `C:\SteerlineInstall\deployments\20261105-063939-ddfac1bc`。SteerLine 来源 `df18050e87a2ef7852c71224063480407527c6c8`，source_dirty=false；Runner 来源 `f6a389cfd4ae7827d7c722555a85e94f1e1ba708`，version `0.2.0-dev.4`，SHA256 `cd3d4ace05c00ea81f2888c32833c0b75618a04a73921c4f216f1c31afc2811f`。

2026-11-05 14:46（Asia/Shanghai）最终核验：后端 PID 22556、Web PID 5740、Runner PID 48624，三个项目 ready/online、degraded=false；`http://127.0.0.1:18742` 返回 HTTP 200。Runner 在重新创建的原 home 派生目录中运行。

原失败回执和 stop 请求保留于 `original-shutdown.json`、`pending-stops.json`、逐 Run 的 before/after 文件；资源证明为 `original-verify-stopped.json`、`candidate-verify-stop.json`。部署和最终结果见 `build-deploy.log`、`deploy-timings.json`、逐项目 validation/create-result/pause-receipt、`tasks-final-verified.json`、`installation-final-verified.json`、`ready-final-verified.json`、`status-final.json`、`runner-final.json`。

## 维护期间的失败与边界

两个暂停请求返回 discovery 回执文件 rename Access denied；均沿原 request_id 查询并核实 applied，未另发暂停请求。维护脚本首次将 issues=null 错当非空而停止；原正式校验实际已通过，修正脚本判定后继续，未替换校验结果。以上失败不改记为首次通过。

未运行 L1–L4、真实模型、环境资格或新 PR 交付；本次证明安装部署恢复及所述任务保留结果，不替代产品回归或原生执行专项。
