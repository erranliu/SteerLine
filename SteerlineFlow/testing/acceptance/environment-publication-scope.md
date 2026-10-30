# 环境检测发布范围收敛验收

2026-10-30。[Spec](../../spec/environment-publication-scope.spec.md) · [Plan](../../plans/environment-publication-scope.plan.md)。用户确认先收缩脚本到正常交付需要的能力、再重跑本机Codex；不改Runner权限，不部署安装环境。本机Orca等Runner修复，远端连接未恢复不重跑。

## 已实施边界

Agent保留TTY、工具检查、原L1/完整L2、专属commit/push、唯一原PR创建/查询。报告version2只记原URL/base/head/head_revision及脚本自有资源清理/未知，移除Agent检查/审核/合并通过字段。SQLite当前格式36，旧状态不迁移、不兼容，安装库未改。

ready须原Runner的provider、仓库及head仓库、URL/base/head/SHA、可用PR状态与封存报告匹配，QueryError为空；缺事实、关闭未合并、错绑定或空URL查询错误保持unverified，不开下一轮重复发布。TTY/L2/三轮/原请求/停止与result封存规则保留。Web显示“已发布”，不将其写成“已合并”。

外部L4驱动按原源码分支规则核实checks/独立审核和原L2后，对该精确PR以预期head SHA合并。首次写前保存独占merge意图并flush，未知只读对账，已存在意图不能重发。外部结果不回写Agent报告；外部规则/合并/清理不足仍使正式L4失败。没有新生产审核阶段、后台合并器、配置平台或Runner环境检测能力。

## 提示词实际正文及独立审核

实际diff把Agent内分支策略读取和合并职责移给外部驱动，将报告改为version2/head_revision；保留TTY、完整L2、用户准备、三轮、未知只对账和停止封存。增长用于明确新职责边界及禁止虚报未执行动作。短指令和运行时拼装规则没有新增inline兜底。

| UTF-8 LF正文/脚本 | 前字节 | 后字节 |
| --- | --- | --- |
| environment-execution.md（内部及三项目镜像） | 4263 | 4579 |
| environment-generation.md（内部及三项目镜像） | 5500 | 5834 |
| SteerLine environment.md / verify-environment.ps1 | 3834 / 11355 | 4282 / 10169 |
| SteerlineTest environment.md / verify-environment.ps1 | 4492 / 11911 | 4962 / 10821 |
| 308 environment.md / verify-environment.ps1 | 5953 / 15274 | 6453 / 14482 |

独立只读审阅者 `/root/publication_scope_review` 审计Spec/Plan、实际正文/字节、项目脚本与接线；execution SHA256 `7da6fd6727babcd01cac71d32c72cdde45dfb0e9e3a71e527387ebca0b364d2c`、generation SHA256 `fe987ed7dba8957d9ba55d1e3efb05ac596d1ace3372430360a5f7e15c78728b` 获批准后才更新摘要测试。其指出自身脚本策略遗漏、关闭PR和空URL查询错误重试出口，均修复并获复核批准。必需checks与独立审批匹配循环沿原调用提成testenv小函数，补缺失/pending/failure、作者自批/撤销/驳回反例，不新建验证平台。最终全量diff复审及L4结果继续独立补记。

## 固定项目与受控证据

本节原始证据根 `C:/SteerlineDebug/wt-5069-environment-publication-scope/`，每次失败/补测独立目录，不覆盖历史。

- SteerlineTest固定提交 `ced83a41a774246a6275b4fd095cd1e571f9b547`，[PR695](https://github.com/erranliu/SteerlineTest/pull/695)；原完整L1五项/L2一项通过、零跳过。报告在独立clone的 `.steerrunner/artifacts/publication-scope-1b38bc8b13ff4011b6fd258d7b201a61-l1`、`-l2`。
- 308固定提交 `c7ddc68a71ca5097b4c32d4c6fba372f96b56028`，[PR441](http://192.168.0.66:3000/zymobile/merge-h-308-code/pulls/441)；实际PowerShell AST和diff检查通过，未在本机运行Unity/L1/L2。两项目PR均未自动合入。

| 已验范围 | 本节证据根下原始位置 |
| --- | --- |
| 完整环境owner：report2、旧字段拒绝、原PR事实/TTY/L2/空URL/关闭状态、未知零新轮 | `diagnostics/final-report-owner-032c31216ee2410e85c724e473ea53ca/` |
| 报告/提示词/输入及正式L4证据校验 | `diagnostics/contracts-final-9799429750024d3c86b8d985bb0a5a46/` |
| app/SQLite环境资格owner | `diagnostics/app-sqlite-6c6599288f36451a8d58eb9769f0944e/` |
| 受控外部合并/未知回执零重发/原报告不改写、本机取证边界 | `diagnostics/external-merge-d1744ae1c26e4bf9b337fd4abbf94c2e/` |
| 最终外部规则门禁正反例及原合并/取证回归 | `diagnostics/external-gates-final-c3f5bea9340f4b32b197670081d9caf4/` |
| Web环境页8项及生产构建 | `diagnostics/web-63627f1945a548e8b4efe59a498b0544/` |
| 固定L1通过 | `test-results/L1/final-bef65cfe7f964bacb574b88698ab80ef/` |
| 完整L2通过、清理完整 | `test-results/L2/final-d1bd11863367415990712d7a96a3c3d2/` |
| 正式L3准备门禁通过313.57秒、清理完整 | `test-results/L3/readiness-0177b0c1b59e44c4b59a5fb0a58a459f/` |
| 固定干净实现d9d71198的L1、完整L2通过，L2清理完整 | `test-results/L1/fixed-658f13295a3e45b6b36268b1bc7ce302/`、`test-results/L2/fixed-6a1f2d73adc94f7392f67ac5ca419eef/` |

早期编译暴露cmd/test-feedback仍消费旧合并字段，已同步独立验证器；早期owner断言仍期待在原PR身份丢失后开第二轮，现改为保持unverified且禁止重复发布，并新增真实事实冲突反例。未删除TTY/L2或身份断言，不把编译失败计入通过。实际修改前后字节/SHA账本在 `diagnostics/material-edits/bytes.json`；项目固定提交及语法检查在 `diagnostics/project-validation/fixed-projects.json`。

## 真实本机Codex续验

两次均固定干净实现 `d9d71198329da62759379e4b3c1bea0c1c00593e`、原full_access profile及原独立Runner SHA256 `7b57b06472ae7c36befa71222cdc6e61f7bc2fda410adee6d6aaf32e58fdc2b5`，未改账号、代理、PATH或工具。原生检测已证明ready，正式L4的后续准入/停止失败仍分别保留，不把局部通过写成整体验证通过。

| 正式L4原报告 | 环境检测与外部PR结果 | 整体结果与收尾 |
| --- | --- | --- |
| `test-results/L4/codex-publication-14129cb5d0df4c43ba0752b86c8c44e2/` | 原活动6337eeb9042c43d6a1c3e0c9fc3dad5c、Run31b57c0454ca0fa1bad443360a812453，一轮ready，实际TTY/L1/完整L2/原PR发布和停止封存通过；[PR696](https://github.com/erranliu/SteerlineTest/pull/696)由外部精确合并，原403未再出现 | 整体failed，准入探针缺项目版本化runtime-progress.md，零探针Run；cleanup_complete=true |
| `test-results/L4/codex-materials-a0afc567a5274fe6bd5fb2de24f7f3ac/` | 一轮ready；[PR697](https://github.com/erranliu/SteerlineTest/pull/697)由外部精确合并；补齐材料后原start_task创建实际Runc0b27815b7565f5475d9e487966371f9 | 整体failed，测试驱动原stop_candidate信封漏TaskID，CLI在提交前拒绝；原Runner随后经公开cleanup confirmed=true，原summary保留cleanup_complete=false与scratch，外部ref补证另存 |

材料缺口经正常Git修复：SteerlineTest新增同源 `SteerlineFlow/testing/runtime-progress.md` 及 `tools/testing/RuntimeProgress.ps1`，308新增协议，既有writer未改。新增均0→3196 UTF-8 LF字节；协议SHA256 `587708f00a71eb6a992072c5b12c5aa7f92ed327e2c614e11e33fa4e2e604542`、writer `d39d89f2f3353116c7f507da998f518a27564aa418082faba9a20b68755bfcb4`。独立审阅者逐字核对权威源后批准；原未存在的文件没有运行时inline兜底。账本 `diagnostics/project-validation/runtime-materials-lf.json` 明确LF与磁盘字节：308既有writer正文3196、CRLF磁盘3253，未改变。

最新项目固定提交：SteerlineTest `20610e10285d9ab9bb4ea3be0c07410158692fc0`（PR695），原完整L1五项/L2一项通过且零跳过，原报告为独立clone `.steerrunner/artifacts/runtime-materials-257904f6ccf44ffb858ecfcbbe28ed63-l1`、`-l2`；308 `ab3ff27eb1dd8a902258a8fbac6feb1bbabf50a6`（PR441），语法检查通过、未本机执行Unity/L1/L2。两材料PR均未自动合入。

第二次现场补充清理证据为 `diagnostics/second-native-ref-cleanup-369112a9e794410aba9c5afe771a07b8/`：原PR697精确身份/合并核实、准入source base无PR、原Runner清理confirmed后，按原SHA租约原子删除三条自有ref，另核实准入Run的远端分支不存在。原失败报告和scratch保留，未将补证回写为原清理通过。

测试驱动只补停止信封中同一个原candidate.TaskID，原candidate.ID/revision/request和停止断言不变，生产代码未因此更改。此修正已获独立复核批准；固定新实现后另跑L1/L2及真实L4，不覆盖前两次失败。未修改安装环境或Runner权限。
