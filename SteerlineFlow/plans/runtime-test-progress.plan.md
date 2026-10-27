# 运行时测试进度 Plan

关联 [Spec](../spec/runtime-test-progress.spec.md)。Runner 来源为独立仓库 `aea8abe` 的公开单文件接口；本次不修改 Runner、安装环境或原 Task 18。

1. 发布调用方输入/快照类型及 capability/schema/字节核验，冻结原安排和初始对象。
2. 引入独立 candidate_progress 存储与进度观察，接入执行页；清除候选证据链，保留主干的独立 Git 历史。
3. 提供通用 PowerShell 入口及 Go 写入器，互通字节锁与原子更新；接入 SteerLine L1–L4 与业务项目既有结果判定。Plan 自动目标支持后续处理。
4. 同步文档、L3 注册、配置 fixture 和数据库 schema；在新现场验证并登记实际结果。

| 验收 | 测试及环境 | 执行时点与通过条件 |
| --- | --- | --- |
| A1/A3 | owner 定向 Go：runnerclient、testmanagement、app、sqlite；isolated | 实现后，原 Run/输入身份、字节/版本、字段严格性及存储 CAS 均通过 |
| A2 | runtimeprogress 混合 Go/PowerShell 多进程；isolated | 实现后，所有并发字段保留，重跑仅清零目标，未知字段拒绝 |
| A3 | `testing.runtime-progress`，L3 controlled | 提交前，真实产品服务/Git/SQLite，公开 Runner fixture；通过/重跑/错误与恢复/重启/原输入和业务 revision/清理完整 |
| A4 | Web build、client tests、历史 owner 回归 | 提交前，显示语义与独立历史功能均通过 |
| A5 | 业务项目 test-runtime-progress.ps1 与 test-managed-reports.ps1；controlled | 入口改造后，分开证明进度协议与原报告协议；不启动 Unity、不声称 L1–L4 业务全量通过 |
| 整体 | `l1.minimum`、`smoke.delivery`；isolated/controlled | 最终实现执行固定 L1/L2；零选择、跳过、超时及清理不明不通过 |

命令与结果登记在[专属验收](../testing/acceptance/runtime-test-progress.md)，每次使用新输出目录，首轮失败保留。Runner 自身真实远端 L3 资格以其原验收为准，不将本仓 controlled 联验当成新的远端业务执行证明。

2026-10-28 提示及写入器分发由[调用方收敛 Plan](caller-prompt-convergence.plan.md)继续实现：完整冻结安排不变，执行提示紧凑投影、固定文件逐项核验、同源 writer 路径引用与缺失/不同源完整内联；同步扩展原 `testing.runtime-progress` 场景。原历史结果不变，本轮证据见[独立验收](../testing/acceptance/caller-prompt-convergence.md)。
## 2026-10-26 后续修复

原 Task18 第二轮 0/6 已取回公开 closed 原始快照：六个 false 字段之外还包含 publication 对象；固定项目版本的原入口未接入写入器。修复由调用方在实施/审计提示中提供同源写入器供原入口接线，并在发布/报告提示中保留原契约、追加测试进度只读约束。严格字段解析及历史原报告不改。

定向测试覆盖缺少工具项目执行内联写入器、失败重跑清零、拒绝 publication 字段、原提示/用户覆盖保留及步骤写权限隔离；随后运行固定 L1/L2。用户要求的两次真实复验须使用新 home/候选，强制 L1/L2 与原四结构及所有额外选项，独立核实原始报告、公开进度和页面一致、真实 PR 与完整资源清理；只有这些都通过才累计。结果见[后续验收](../testing/acceptance/test-progress-0of6-20261026.md)。

新环境预执行门禁再次发现模型只选四结构，遗漏项目规范必跑 L1/L2。修正正式生成提示的优先级，明确规范必跑与 Plan 定向项取并集，只有规范允许且 Plan 明确无需自动测试时才允许空安排。原 Plan/测试清单及 Run 冻结规则不变；先跑受影响 testmanagement owner、固定 L1/L2，再在新普通 clone/home 通过原严格材料门禁后发起唯一业务请求。提示修正及组件通过不等于真实安排合格，仍以公开原安排和完整真实执行复验判断。

2026-10-27 实际结果：`qualified-01` 九项原报告、发布后/closed 进度、API/UI、真实 PR427 和资源结算完整通过；随后 `qualified-02` 保留额外完整 EditMode，原入口 exit2、11 失败和 33 跳过，进度正确保持 6/7，发布操作另被远端连接拒绝。原 Run/宿主已停止结算，连续数归零。不得删去额外项或换一套安排重复抽样；用户随后批准业务断言/夹具/隔离修复并合并 PR428，现已从合并目标恢复两个全新轮次，结果待独立核实。详细原证据、两个源码代际及未验边界见上述后续验收。

2026-10-27，PR399 合入当前 main 并解决唯一 HTTP 边界冲突，保留主线较严格的宿主限定；代码审计、真实执行结果、首轮受控超时/独立复核及暂缓合入建议见[合入审计](../testing/acceptance/pr399-merge-review-20261027.md)。代码/受控验证不替代尚未完成的原生连续两轮，也不冒充新主线代际远端资格。

## 2026-10-27 职责分离与暂停

用户将 Orca → Codex 输入投递、提交和启动确认交由 SteerRunner 对话负责，等待 [PR66](https://github.com/erranliu/SteerRunner/pull/66) 独立验收并合入。本会话不再实现或补充原生粘贴识别、终端输入、启动确认逻辑，不补 Enter、重发输入或放宽确认条件。依赖该链路的业务复验已暂停；定时任务已取消，不新建或恢复。

PR399 的冲突处理、代码审计、固定 L1/L2、受影响 owner 和独立受控进度复核已有原报告；代码未变化时不重复这些工作。当前独立收尾是保存资源结算、职责边界及验收说明。无其他已明确授权的独立实现待办，完成文档后进入等待。

用户告知 PR66 合入后，先核对实际合入提交、Runner 自身验收结果及对应构建，再通过正式生成入口创建新的独立调试包。保留旧 home、构建和接受 pin，不热替换旧运行。随后恢复两轮连续完整复验，保留全部实际安排检查和通过标准，逐轮验证原始报告、进度/API/UI、真实发布与完整清理。Runner 最小探针不计入两轮结果；当前连续数仍为0。

用户现已告知 PR66 合入；实际合入、受测产品/构建对应、PR399 rebase 的完整树一致性、两项补充公开 EXE 契约检查及两个全新正式包均已核实，见[合入审计](../testing/acceptance/pr399-merge-review-20261027.md)。后续原进度复验使用 `merged66-` 独立前缀，保留原 Caller8205、合同、业务 base、SDK/native/model/profile 和全部实际安排。当前 main 的新环境材料/准入仍单列未验，不用原代际通过替代；首轮完整通过且结算后才创建第二轮。禁止旧现场恢复、未改变失败链的重复抽样，以及任何原生输入绕过。
