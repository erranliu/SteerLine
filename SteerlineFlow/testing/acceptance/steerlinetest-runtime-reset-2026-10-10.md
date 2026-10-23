# 安装版 SteerlineTest 重置验收

归属：用户明确授权修复 `C:/SteerlineRuntime/reset-SteerlineTest.bat` 后直接重置任务；普通管理操作，不进入 roadmap。关联[Orca 结果修复](archive/superseded/orca-supervised-results.md)。独立调试包自带 reset 仍遵循无备份约定；本文件记录的是另外修复的安装版 Python 配套工具。

设计：只读导出任务定义、完整固定契约和依赖，经当前产品 task-batch/v1 验证并在独立临时 home 创建新库。持有仓库、状态与全部控制台视图锁后，备份旧 SQLite 和本项目 control/service/runs，替换为产品创建的新库，删除视图中匹配本项目路径的请求/草稿。短暂启动本工具拥有的 manual 服务，通过公开 control 恢复调度后停止；不启动任务或修改 Git/PR。失败保存新现场并还原旧库/目录/视图，不增加旧 schema 兼容或 SQL 状态改写。

最小验证先在 `C:/SteerlineDebug/dev-fbde4163-dirty-20261010-110711/` 的模拟配置中运行代码副本，仅复制脚本和该包产品二进制；未复制安装版数据/配置/认证。为 helper 进程设置本包 `STEERLINE_E2E_CONTROLLED_STATE`。安装版执行是验证通过后的用户授权动作。

| ID | 方法、条件与命令 | 状态 / 证据 |
| --- | --- | --- |
| SR-A1 | 独立模拟包运行脚本 `--no-pause`；定义/契约/依赖相等，任务 open/version1/无领取，执行历史为空，manual 未暂停，临时服务退出 | 已验收，`...110711/evidence/installed-reset-script-validation.log`；包内 `home/backup/SteerlineTest-reset-20261010-111300-623190/` 保存批次、创建/恢复回执、reset-service.log、公开 tasks-after-reset.json |
| SR-A2 | 安装版 `reset-SteerlineTest.bat --check --no-pause` 只读预览，再运行 `--no-pause`；任务 101–105 保持定义/契约/四条边且重置状态，操作只定位本项目，其他项目视图不变，服务停止 | 已验收，`...110711/evidence/authorized-runtime-reset.log`；`C:/SteerlineRuntime/backup/SteerlineTest-reset-20261010-111621-962706/` 下批次、创建/恢复回执、公开列表和 `reset-verification.json`。5 项均 open/version1，101/105 dependency_ready=true、102–104 false；定义/契约/四条边精确相等，SQLite 完整，历史为空；前后视图仅删目标项目，原安装目录 steerline 进程为空 |
| SR-A3 | 当前产品创建新库，SQLite integrity/foreign_key 与所有定义精确比较；失败路径还原原现场 | 已验收，脚本执行内检查；初次实际操作曾因启动 ID 目录缺失失败并还原原 101/105 in_progress，保留 `C:/SteerlineRuntime/backup/SteerlineTest-reset-20261010-104132-527758/`。后续整理误删 uuid 导入导致阶段创建前失败，补回导入；无状态替换 |

当前工具不保留旧批次/版本/authority/database ID 或运行历史；重新导入定义后为当前新库的原生身份。保留所有业务任务 ID、内容、契约、依赖、Git 工作树及远端 PR。没有恢复任务执行或改变远端合并事实。

2026-10-10 最终状态：安装版二进制来自最新 `fbde4163...` modified=true 的调试包构建，SHA-256 `644B4D3DEA1977110FE1BA3815C4763DFCA65578CFF9DF4A16812C77AEDF7AB6`。manual 调度未暂停，服务仍关闭，用户可重新开服务和控制台。工具本体在安装目录，不属于仓库构建产物；原 `.py/.bat/.txt` 保存于 `C:/SteerlineRuntime/backup/reset-utility-20261010/`。
