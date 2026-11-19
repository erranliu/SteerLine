# schema 51/52 部署恢复（2026-11-19）

用户授权修复部署报错，并明确旧运行可以放弃。本次为安装维护；产品代码、提示词、配置及旧数据库格式未修改，未增加迁移或兼容路径。

## 原因

安装目录为 `C:\SteerlineInstall`。部署包 `deployments/20261119-030320-565ae638` 已完成构建、停止旧应用、升级预检和程序替换，但在 `deploy.wait-ready` 失败。安装程序来源为 `3a1777a6bbf5d4b245be2ca12aa24c4b2af4e905`，Runner 来源为 `8ebbc717776eb2ad85371060566776c4402773fa`。

启动日志报 `unsupported SQLite schema 51; only schema 52 is accepted`，导致四个项目均 failed。新版宿主在项目打开前失败，原库仍保留上一实例的已停止检查点；正式停止新版宿主后，其回执没有项目实例 ID，并报告 `original Runner shutdown remains unverified`。该失败回执不能替代原实例的停机证明。

## 维护

正式停止安装版后端和 Web，核实精确进程退出。读取上述部署包中原实例的干净停机证明，并以其匹配的原 Runner（`deployments/20261118-082634-8df48b22/steerrunner.exe`）重新执行公开离线核验，28 条 Run 的原生资源均已结清。

临时维护程序持有目标部署独占锁、启动协调锁和所有项目仓库锁，复用现有旧库只读校验、`verifyDiscardProject`、`prepareDiscardedTasks` 及 `commitDiscard`。项目检查点按其实际所属的原实例、原 Runner 构建和干净停机证明核验，保留失败新版宿主的回执与诊断。通过正式任务批量校验和创建入口建立当前 schema 新库，逐项比较任务编号、标题、类型、优先级、目标、完整固定契约及全部依赖；全部任务恢复 open，全部项目关闭自动派发。

提交切换前再次核对配置和原 Runner 资源清单。原项目状态、应用/Web 生命周期记录和 Runner 命名空间完整归档，切换沿用正式 discard 日志和中断标记机制。配置、业务 Git、远端 PR 和安装级重构方向库保留。

完整现场及维护程序源码保存在：

```text
C:\SteerlineInstall\maintenance\schema52-20261119-ad1b8b9a56c445439d19196ef48e6228\
```

入口证据为 `discard.json`、`original-proof.json`、`failed-status.json`、`runner-resources.json`；原数据分别位于 `project-state/`、`application-api/`、`web-state/`、`runner-state/`。临时维护程序已从源码工作区移除。

## 结果

核实安装三个 EXE 与原部署包字节一致后，使用正式 `Start-DeploymentApplication` 启动入口恢复服务。2026-11-19 11:23（Asia/Shanghai）状态为后端、Web 和四个项目全部 ready/online，`degraded=false`；安装版 Runner 服务正常运行，Web `http://127.0.0.1:18742` 返回 HTTP 200。

启动后通过正式 `task graph -config <项目配置>` 接口确认：406-code 和 SteerLine 各零任务；SteerlineTest 为 5 个 open 任务、4 条依赖；merge-h-308-code 为 15 个 open 任务、4 条依赖。合计 20 个任务、8 条依赖，与切换前一致。

最终证据位于上述归档目录的 `status-final.json`、`web-final.json`、`task-graphs-final.json`、各项目 `.graph.json` 和 `startup-0c631204bced4d9185cffddd8aa23b90.log/.json`。原失败部署的 `timings.json` 保留失败结果，不改写为成功。

本次未运行新模型任务或真实 PR 交付；没有产品代码变更，不运行 L1/L2 回归。验证范围为实际停机、资源核验、任务重建及完整比较、归档切换、正式启动、项目状态与 Web 响应。
