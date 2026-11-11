# 默认建议回答与提示去重

2026-11-10，按用户确认方案实现，未部署。需求来自 merge-h-308-code 正式任务 17 的待答页面截图；本次在隔离夹具中验证 UI 和原回答入口。

## 行为与边界

- 候选标题和状态下默认展开回答区，精确读取当前 candidate/Attempt/Question 的持久建议；ready 且 settled 才默认填入可编辑文本框。生成状态、失败原因及折叠上下文放在回答区。
- 手写草稿（包括主动清空）优先，迟到建议、版本刷新和重载不覆盖。草稿按 home/project/instance/candidate/execution/Attempt/Question 隔离，普通观察版本不改变草稿身份。
- 用户点击“回复并继续”才经原 decision-eligibility 和 answer_decision 发送最终框内内容；绑定原候选、原问题、Attempt 和当前版本。核验/提交期间防重，保存原 request_id 并跟踪原回执；已受理/未知保持防重，明确拒绝恢复编辑。提交前失败只有在成功确认原请求未登记时解锁；原行存在或读取失败继续核实原请求。已生效业务反馈可显式继续追加，清空后再编辑。可稍后回答、选项、多题 JSON 和中文输入法保持原规则。
- 原问题/context、Runner diagnostic/controls reason 和候选 error 的相同正文仅展示一次；其他诊断保留。最近消息与原问题相同则显示引用提示，原始记录保持在其 owner。当前原建议可从折叠上下文复制；历史建议继续查看、复制和取消未结算生成。
- `/main-agent?candidate_id=…&occurrence=…` 只读精确查询；没有对应记录返回 `record:null`，不会退到上一轮或其他候选。详情投影补充 occurrence；advisory-v1 生成协议及核心仍只输出 prompt。

## 正文和接线审阅

实际业务原因：原页面建议只允许在下方复制，上方回答区还需展开和粘贴；相同诊断又在多个位置铺开，遮挡当前操作。现在由生成模块之外的回答 UI 默认采用文本，用户显式发送。

固定提示正文、生成组装、模型参数及生成调用点无差异，原文不 trim、不改换行；本次改变的是 UI 外部采用和持久事实读取。与基线 HEAD 比较的 UTF-8 字节：

| 文件 | 修改前 | 修改后 | 正文差异 |
| --- | ---: | ---: | --- |
| `internal/prompts/main-agent.md` | 145 | 145 | 无 |
| `internal/prompts/materials/main-agent.md` | 1025 | 1025 | 无 |
| `internal/prompts/testdata/main-agent.golden.md` | 1180 | 1180 | 无 |

未更新黄金样本、模型提示测试/CI 或生成规则。独立只读审核已通过。首轮提出异步拒绝后入口锁定（并影响固定 ID 的追加反馈），增量提出提交前失败误锁定及停止过渡提示隐藏；均修复并经同一独立审核者增量复核通过。审核者未修改文件或运行测试。

## 验证

受测基线 `50b8fdb5c24fa2a3f6dd32fe10e8799d271061f2` 加本次工作树变更。正式输出根 `C:/SteerlineDebug/wt-994c-answer/`；报告保留各次实际受测工作树，不回写原报告版本。

| 验证 | 结果及证据 |
| --- | --- |
| 定向前端 7 文件 | 73 项通过，最终日志 `C:/Users/lmas1/.codex/visualizations/2026/11/10/01a1c4ed-8d1e-7281-a66b-f09e86d5f67a/answer-unit-final.log`。覆盖原生多题/选项、草稿和清空优先、迟到生成、中文输入法、原问题绑定与单次发送、过期/其他来源/失败/未结算结果拒用、资格失败保留、重复提示和项目切换；新增 submitted/accepted/outcome_unknown→rejected 同问题重试、业务反馈再次追加、未登记提交失败解锁/已登记未知防重。命令为 `npm --prefix web run test -- src/features/runs/answer.test.tsx src/features/runs/answer-suggestion.test.tsx src/features/runs/continuation-suggestion.test.tsx src/features/runs/runs.test.tsx src/features/runs/last-message.test.tsx src/features/runs/presentation.test.ts src/app/project-switch.test.tsx`。 |
| 精确建议查询 owner | 通过，`diagnostics/current-question-20261110-01/summary.json`；SQLite 真实记录验证来源、字节保留、无记录与非法参数、GET 不写入。 |
| 生产构建 | `npm --prefix web run build` 通过。 |
| Edge 浏览器 | 最终 5 场景通过，`diagnostics/browser-20261110-02-final/`（`npm --prefix web run test:e2e -- ui-simplification.spec.ts`）；初轮 B5 为 `diagnostics/browser-20261110-01/`。生产 bundle + 隔离 HTTP 夹具，确认默认填入、正文只出现一次、独立诊断保留、编辑后版本刷新及重载保留、单次发送及完整原问题信封；同时回归进度、历史和导航，1440/800/375 无横向溢出，代表截图已目视核验。 |
| L1 | 通过，`test-results/L1/20261110-01/summary.json`。 |
| L2 | 通过，`test-results/L2/20261110-01/summary.json`。 |
| L4 go.http | 串行补验通过，`test-results/L4/go-http-20261110-02-serial/summary.json`。初轮 `go-http-20261110-01` 误与隔离 E2E 并发，虽通过也不作为串行压力证据；原报告保留。 |
| L4 runtime.main-agent-advisory | 通过，`test-results/L4/advisory-20261110-01/summary.json`；覆盖自动生成、精确导航、上下文与复制、取消/资源结算、重启和源恢复。 |

L1/L2 及原 advisory 专项受测后的变化只涉及回答提交状态恢复、停止过渡展示及相应测试/文档；未改后端业务规则、生成、取消、恢复和生命周期。复用报告中各自受测工作树的共同覆盖，最终差异由 73 项定向测试、最终生产构建及浏览器补验覆盖。

首次沙箱内 npm 安装异常，按锁文件在可用执行环境安装后完成；首次 owner 定向测试的沙箱临时报告不可在后续调用读取，已用上述持久目录重新通过。沙箱 Edge 在测试开始前退出，改在允许启动 Edge 的环境验证，原失败保存在独立目录，未算通过。实际 Task 17 的部署、真实回答和后续执行结果不属于本次隔离验证。
