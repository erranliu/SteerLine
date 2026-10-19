# 三 EXE 安装部署 Plan

2026-10-19 日常部署调整以[同构建重启与显示更新 Plan](application-redeploy.plan.md)为准：已有安装默认复用固定 Runner，显示更新不要求销毁原远端执行；本文件保留首次安装/迁移安排。

本次提供仓库根目录 `build-deploy.bat`，从两个当前源码工作区构建三个 EXE，完成构建后才正常停止显式目标安装版，校准 Runner 版本/摘要并等待所有项目就绪。正常更新保留配置与任务，不引入旧 schema 的长期兼容路径。

首次实际迁移按用户确认单独进行：三项目全部保留，通过正式 task validate/create 导入定义和依赖；任务全部 open、manual+paused，旧状态和交付历史留档。此一次性操作不加入日常部署脚本。

## 实施步骤

1. 构建前核对目标与源码、旧安装、调试目录互不重叠，拒绝链接路径并持有独占部署锁。
2. 构建并保存三个 EXE，读取实际 Runner 契约/版本与 SHA256，正常停止目标应用，确认目标进程全部退出后安装。
3. 保留既有 v3 profiles，仅刷新 Runner 固定信息；写入三个文件的实际摘要与源码版本记录。默认启动并等候宿主、Web 和全部项目 ready；短暂文件占用仅对安装操作有限重试。
4. 调试隔离规则同时保护新安装与保留的旧目录；更新运行说明和测试导航。
5. 首次安装迁移单独留存定义批次、历史、正式回执及前后数据摘要，不构造旧 Runner Run。

## 测试计划

验收项归属：[IC-A01–IC-A06](../acceptance/install-cutover-20261015.md)。使用 Windows/PowerShell 5.1；运行结果统一登记在该验收文档。安装操作仅在用户授权的实际迁移中执行；普通回归使用脚本预检或已有隔离夹具，不把安装目录作为调试现场。

| 验收项 | 最小验证及命令 | 时点与通过条件 | 证据 |
| --- | --- | --- | --- |
| IC-A01、IC-A05 | 授权安装时 `build-deploy.bat -NoPause`；`STEERLINE_HOME` 指向新安装版后 `bin/steerline.exe status --json`、各 state-dir 的 `bin/steerrunner.exe service status --state-dir <dir> --json` | 实施完成、首次交付前；退出 0，三个安装文件摘要与暂存一致，Runner 固定摘要一致；正常停机回执 clean，原进程创建身份已退出，新项目/独立 Runner 全 ready | 安装目录 `deployment.json`、`migration/20261015-task-definitions/oneclick-deploy-final.log`、`deployment-verification.json` |
| IC-A02、IC-A03 | 正式 `task validate -file <batch> -config <project>`；`task create -file <batch> -config <project> -request-id <固定32hex> -expected-authority-version <采样值>`；`task list -config <project> -limit 100`；离线导出与新库只读对比 | 首次迁移、更新前后；三个项目原目标/分支、三个 profiles、全部编号/契约/8 条边一致；20 open、manual+paused、零候选/Run；DB ID 与定义摘要更新前后一致；integrity/foreign_key 通过 | `*.task-batch.json`、`*.validate.json`、`*.create.json`、`*.list.json`、`verification-before.json`、`verification.json` |
| IC-A04 | 停机后的旧库只读导出任务状态、交付/结果与 Workflow/PR 引用，核对 manifest 摘要与原数量 | 导入前后；旧目录完整保留，无凭据复制、无旧执行重启；原状态与交付证据可查 | 同迁移目录 `manifest.json`、`*.history.json` |
| IC-A06 | PS Parser 解析修改脚本；`git diff --check`；部署脚本传入旧安装、调试、源码或相对目标须拒绝；生成器传入新/旧安装的 Runner 路径须拒绝 | 提 PR 前；在构建/生成前按明确原因退出非零，无安装写入 | `.evidence/install-migration/pr-preflight.log`、`pr-preflight-summary.json` |

Runner 的权限修复由其 `docs/spec/steerrunner.plan.md` 与 `docs/acceptance/windows-broker-install.md` 独立覆盖。部署脚本和文档未修改业务执行/交付，不跑完整 L1/L2/L3/full，不重跑历史 live；安装空闲停启不替代模型或恢复验收。无需新增人工验收门槛。
