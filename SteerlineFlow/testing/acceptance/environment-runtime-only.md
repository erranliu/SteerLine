# 环境资格与项目测试解耦

日期：2026-11-02。普通开发，按用户要求直接收敛为最简边界，不进入 roadmap。主仓基线 `89130fbc25ed2c2224295d21f3b151baffa91de3`；实际项目 `C:/Projects/SteerlineTest` 基线 `55548a54e274adb957514c6ff56ce3d8f7b4c81d`，两处均为未提交工作区修改。

## 最终行为

环境检测只知道自身说明、入口、辅助程序及工具/SDK、权限、服务要求配置；不读取测试清单、安排、进度和报告，不执行项目测试、构建验收或发布。平台不增加测试路径分类器、旧格式读取、重试或并行资格系统。

SteerlineTest 四个环境的声明依赖由各 50 项减为公共工具检测脚本这一项；环境说明和单次入口自动计入，共三份材料。入口只检查 PowerShell 5+、Git 命令及 Python 3.9+ 的实际可用性，原样保留关联字段并原子写最终结果。测试工具、文档导航、Spec/Plan、roadmap 和源码清洁度均不参与资格。原项目测试入口与 16 个测试 ID 保留。

Git 读取对有限路径执行一次提交验证、一次精确树查询和一次批量 blob 读取，52 个文件也只需三次 Git 进程。保留原字节和 regular blob 限制。未延长后台 35 秒预算；原始超时/启动/读取错误通过错误链保留，不再统一误报提交缺失。只有成功读到 Git tree 后确认路径不存在的专属错误才代表材料删除；Git 程序或 cwd 缺失只暂时阻断，不持久撤销资格。共享读取者的 roadmap 可选文件分支同步采用该区分。

原固定提交的 52 份真实材料已用同一 35 秒上下文只读复测：先读取 manifest 再批量读取原材料集，总计 6 次受管 Git，1.091 秒全部完成、context 无错误。原逐文件版本在 35.12 秒仅完成 35 份后超时。原始新结果在 `diagnostics/20261102T074601037Z-original-52-materials/`，临时诊断测试结束后已删除。

历史活动标题由“检测通过”改为“检测完毕”。删除主仓四份、实际项目三份已退役的阶段式环境脚本及测试，不保留失效旧入口。

## 正文与资产审阅

业务原因：原环境资格绑定完整 L1/L2，间接把 35 份业务导航文档及测试工具全部作为失效依据；52 份材料逐文件启动约 156 次 Git，35.12 秒复现后台核验超时。调整为纯工具检查，避免普通项目文档/测试维护反向撤销环境资格。

实际正文差异：主指导删除“是否跑 L2/是否需要 PR 由项目决定”和可把测试清单声明为环境依赖的许可，新增环境入口与项目测试完全独立的边界；公共环境规则删除全部 tty/prepare/publish、L1/L2 报告和发布要求。实际项目脚本删除 `Verify-Test`、测试调用、summary/managed/清理断言及源码清洁度检查；四份 manifest 删除原 50 项并只声明公共检测脚本。短提示 `environment-invoke` 正文和调用接线未修改，渲染前后均 466 字节；未更新黄金样本。

UTF-8/LF 字节数（规范化仅用于计数，不批量修改原资产换行）：

| 正文 | 修改前 | 修改后 |
| --- | ---: | ---: |
| 主仓 `SteerlineProjectGuideDoc/environment-adaptation.md` | 8982 | 9180 |
| 主仓 `SteerlineFlow/environment-common.md` | 6458 | 664 |
| 实际项目 `scripts/verify-project-environment.ps1` | 7928 | 3897 |
| 实际项目本机 Codex 环境说明 | 3876 | 1107 |
| 实际项目远端 Orca 环境说明 | 3939 | 1170 |
| 实际项目本机 PI 环境说明 | 3849 | 1080 |
| 实际项目本机 DSH 环境说明 | 3850 | 1081 |

完整实际正文差异及逐文件字节数：`C:/SteerlineDebug/wt-fee3-environment-state/diagnostics/20261102T0740-review/` 下的 `SteerLine.patch`、`SteerlineTest.patch` 和 `byte-counts.json`。原现场只读诊断位于同 scope 的 `20261102T0712-environment-timeout/diagnosis.json`，不冒充新正式检测。

独立审核由本聊天独立只读审核者 `environment_review` 执行。其确认的现行说明残留、工具 ENOENT 误当文件删除及共享读取者的三个可选文件分支均已修正；最终明确批准冻结实现、规则正文、资产和对应测试，无剩余 P0–P2 问题。批准不把运行中的完整 owner 算作通过。

实际项目 L4 回归新增了 Windows PowerShell 执行依赖，测试指南、L4 列表及唯一机器清单同步注明；16 项测试 ID/命令/级别不变。三处正文 UTF-8/LF 字节分别为 testing.md 6425→6505、l4.md 2420→2516、test-list.json 10188→10287。独立审核者另行复核这三处增量及最新三份 managed 定义，明确批准；未通过更改定义制造成功。

## 实际验证

主仓使用 `go run ./cmd/test-feedback` 的固定分级/定向入口，报告根统一为 `C:/SteerlineDebug/wt-fee3-environment-state/`。所有测试均隔离，不操作安装版状态或调用真实模型。

| 验证 | 实际结果和报告 |
| --- | --- |
| 52 文件批量读取、原字节、空文件、精确路径、目录/符号链接拒绝；三阶段超时原因；业务文档/测试增删不改资格 | 定向 Go 通过；完整包首次运行也已执行这些用例，正式全套结果按下行单独登记 |
| 正式 `go.environment` | 未通过：`test-results/L4/20261102T073300549Z-environment-only/summary.json`，`passed=false/complete=false`。app 包达到原 `-timeout=25m`，输出 `panic: test timed out after 25m0s`；超时前已完成项无断言失败、零跳过。该运行在审核最后的缺失分类修正前启动，不能记为最终版本的完整通过；最终分类/roadmap 定向结果独立列出，被中断及尚未执行的十个顶层用例另行补验，不拼成正式套件通过 |
| owner 超时后的剩余用例 | 最终源码的十个顶层用例全部补验通过，`diagnostics/20261102T080018877Z-owner-after-timeout/summary.json`，84.45 秒。选择来自最终 `go test -list` 与原报告已完成顶层项的差集，包含被中断的 CallerPromptConvergence 和后续 TM/TR/Web 用例；不跳过子场景、不修改 25 分钟正式预算、不覆盖前次失败 |
| 工具裸名/绝对路径缺失、cwd 缺失不撤销；真实依赖删除仍撤销 | 通过，`diagnostics/20261102T073848192Z-read-error-classification/summary.json`；前次缺少测试导入的编译失败保留在 `20261102T073807956Z-read-error-classification/` |
| 固定 L1 | 初轮通过；最终修改后 `test-results/L1/20261102T074302384Z-environment-only-final/summary.json` 通过，执行 23.4 秒 |
| Web 全量 | 231 项通过，`test-results/L4/20261102T073340209Z-environment-web/summary.json` |
| 固定提示词 | `go test ./internal/prompts -count=1 -v` 通过；所有原黄金样本和固定正文摘要保持 |
| 共享读取者 roadmap 实际发布/准入 | 最终分类修正后通过，`diagnostics/20261102T074259773Z-roadmap-read-errors/summary.json`，执行 92.05 秒 |
| 固定 L2 | 首轮失败且清理完整，`test-results/L2/20261102T073300387Z-environment-only/summary.json`；受控 Git 转发器未传 stdin，修正转发后补真实 batch 二进制回归，不放宽生产断言。最终 `test-results/L2/20261102T073901488Z-environment-only-final/summary.json` 通过，`cleanup_complete=true`，执行 174.60 秒 |
| 实际项目 L1/L2/L4 | 最终按更新后的真实定义全部通过，分别 5/1/21 项、零失败/跳过；`C:/Projects/SteerlineTest/.steerrunner/artifacts/{L1,L2,L4}/20261102T074817036Z-environment-only-definition-final/summary.json`，L4 47.91 秒。L4 含没有 Git 项目、测试目录/清单或业务文档的隔离目录内三种工具正负结果，原身份保留、负结果正常返回及临时结果清理。报告如实记录 `before_clean=false`、`after_clean=false`，不计固定干净源码的正式资格；原两次旧定义通过分别保留在 073257841Z、074700027Z 目录 |

文档链接检查初次因本验收文件尚未创建报一处缺失；最终 646 份 Markdown、4040 个本地链接、零错误，`git diff --check` 两仓通过。原失败与后续通过分别保留，不覆盖正式 owner 超时事实。

## 交付边界

本轮不部署安装版、不提交/推送或合并两处仓库，不改历史活动、Current、数据库或原报告，不直接把现场改成 ready。实际项目新材料正常 Git 采用后须重新检测；旧资格因真实环境要求/入口变化失效属于预期。工具脚本回归不代表四条原生 Agent 链路均已正式检测通过。
