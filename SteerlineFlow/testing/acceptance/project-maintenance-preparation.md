# 项目维护材料与测试安排准备

2026-11-05。用户确认将准备收敛到项目维护：显式登记、后台执行，任务页展示状态并提供跳转。普通开发，不调度 roadmap。验证基线 `9b2e5d886b8f56eaf07bc697ca78c578c91eec00`，报告采集时 worktree 含未提交修改；各报告保留其实际源码状态和执行命令。未部署安装版，未修改安装数据库或启动真实业务任务。

## 原因与修复

安装现场五个任务存在，`project_materials` 和 `test_generations` 均为空；远端观察成功。原后台材料准备借用 actor 的五秒辅助观察预算，超时后又用已取消上下文保存材料，记录持续为空。任务只读查询将缺材料记录暴露为持久化错误，项目维护的一键生成还要求已有非 ready 材料，形成无可用入口的循环。

- 项目维护“准备材料与测试安排”在同一 SQLite 事务登记原请求、固定任务范围和执行方案摘要；HTTP 回执表示已登记，独立 worker 随后检查并固定远端提交，读取文档、校验和提交原 generate/1 请求。
- 文档读取按任务使用独立预算，默认一分钟或项目 `command_timeout`；读取超时使用仍有效的 worker 上下文保存原因，继续其他任务。LLM 结果沿原 Generation 查询与校验，任务 actor 不承担材料读取或生成轮询。
- 页面 GET、检查更新、保存生成方案、任务启动不登记生成。首次已知任务无材料显示 `unprepared`，未知任务仍为不存在；准备入口不依赖材料记录或已有成功远端观察。
- 原请求重放和活动批次复用；重启继续已登记范围和提交，执行方案内容变化明确失败。失败不随后台 tick 自动重试，未知生成只核对原请求。
- 任务页提供“前往项目维护”，导航后展开准备区；项目维护显示读取校验与生成进度，并提供生成执行详情、取消生成及显式重试。启动仍保留环境、依赖、任务版本、材料版本和原候选冻结门禁。
- 生成沿用原固定提示词、Git 材料和正文组装，不改提示词、黄金样本或正式 writer。

## 验证与证据

输出根：`C:\SteerlineDebug\wt-c294-start-readiness`。所有受管报告以其 `summary.json` 为入口；初次失败和补验使用不同目录，不覆盖。

| 范围 | 结果 / 报告相对输出根 |
| --- | --- |
| L1 | 最终通过；`test-results/L1/verified-544e6d4fcb014b02a73f755fc79ffcd8/summary.json`。首次通过记录另保留；中间一轮与调试包 npm 重装重叠、缺 Vitest 的失败不计通过，已在依赖稳定后串行补验。 |
| L2 | 最终通过；`test-results/L2/verified-fc6086a3caea49589eb0f64c43cb52a8/summary.json`；此前通过报告另保留。 |
| Web 全集 | 最终通过；`test-results/L4/web-final-4085251bab1046d4a282208303d03694/summary.json`；最终前端生产构建通过。 |
| 定向 owner | 通过；`diagnostics/worker-be03bce6e57145beac1dcc94350f56e6/summary.json`，含慢读取超过五秒、原 HTTP 请求取消后完成、超时保存并继续下一任务、空闲不登记、重放与原子回滚。最终方案摘要补验通过：`diagnostics/final-worker-4d537587d9674b29a1946ad2ca0c18ad/summary.json`；未知原结果恢复/来源变化专项通过：`diagnostics/completion-e508a4588fd64bdb86ca61dd296b8e10/summary.json`。 |
| L3 `testing.material-generation` | 通过；`test-results/L3/materials-5e2947d694ba4d5d89a7faedee4ad85e/summary.json`；公开登记、实际 Git/SQLite、生成绑定、有效结果复用、零业务候选及完整清理。 |
| L4 `go.storage` | 通过；`test-results/L4/storage-eb38fc2c6e75487cb5cd3acde6f505e1/summary.json`。 |
| L4 `runtime.project-maintenance` | 未通过，不能计为完整浏览器验收。基线调试包在页面前缺少当前 v8 recovery/同快照观察声明；临时夹具诊断进一步发现测试入口缺失。临时补齐后两项目材料准备为 ready，首次及后续候选已有实际受控 Run，但完整长场景在候选控制 CLI 60 秒时限失败。最新失败 `test-results/L4/maintenance-d0bdbeda287842fc8f6f00071b61cfc3/summary.json`，原包 `C:\SteerlineDebug\dev-20261105-185951-9b2e5d88-dirty`。临时能力/配置/模板及放宽总时限未保留到本变更；保留原断言和原预算。 |
| L4 `go.http` | 通过；`test-results/L4/http-081282667bfd4be1986ae9db5c0e1ef5/summary.json`；在浏览器场景结束后串行执行，包括原跨进程快照压力。 |

## 未验边界

受控生成不代表真实模型、额度、认证或外部 PR 链通过。上述浏览器长场景仍未通过；部分失败调试包的正式关闭/原候选 Run 对账证明未核实，保留其原报告、scratch 和独立包，不清库或补造清理成功。安装现场未用于自动测试。安装版仍运行原部署版本，需另行部署后才能在实际 SteerlineTest 使用新入口。
