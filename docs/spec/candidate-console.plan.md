# Plan：候选观察界面

任务 ID：38；任务名称：候选观察界面。唯一执行依据：[Spec](candidate-console.spec.md)。起点 `28b81d82f4afb3e158cf96bd5cbf257fa48b7adc`（#37 已合入）；分支 `codex/candidate-console-38`。本 PR 交付已复审实现；按用户明确授权先合入，CC5 保留待人工补验，不声明通过，不阻止 #39 开发。

## 实际实现

1. `internal/steerline/console/model.go` / `candidates.go` / `view_tasks.go`：每 Task 一条主行，Task 计数去重；详情按稳定 WorkflowRun ID 列出当前和历史候选。所选 ID 暂时缺页不改选兄弟候选。`{` / `}` 显式选择，`b` 选择不同冻结方案新增，`z` 打开放弃确认。
2. `candidates.go` / `preferences.go`：选择按 Task 保存；阅读位置与输入草稿按 Task/WorkflowRun/Execution/session 保存；原请求、payload、版本和实例先保存，再经原产品客户端提交。body/conversation/activity 的 generation 拒绝迟到响应；原请求重读不改投递目标。正文与活动保持规范数据库来源，无日志回填。
3. `profile_choice.go` / `model_keys.go` / `model_update_actions.go` / `view.go`：服务资格为最终权威；五候选只限制开始检查，明确无硬名额、预留、队列和自动补位。放弃回执与停止事实分开，待核实不是成功；已知 merged 禁止放弃；获胜只依据唯一 TaskDelivery。环境 blocker 与原 Decision 分别展示，草稿可编辑，阻断或解除均不自动发送。放弃后不可反馈。
4. `internal/steerlineintegration/candidate_console_test.go`：真实应用构造、客户端、服务、SQLite/Git 和受控 app-server 接线；精确 UI 新增、原请求重读、五个参与/第六次开始拒绝、原会话反馈、不可撤销放弃。复用现有 neutral provider fixture 验证真实合并事实及服务拒绝放弃，无生产服务/任务库、无真实模型请求。
5. 原生 CC5 的工具诊断记录在 `windows/`；用户追加授权后，实际 UI 工具仍拒绝终端启动（`product policy blocks this app`），没有原生通过证据。新增 opt-in 的同一接线 fixture 保持能力 `STEERLINE_CC5_MANIFEST`：写明隔离 home/config/CLI/service 实例和三项受控 Git/history 环境绑定后等待人工操作和 `.stop` 文件；该准备模式本身不证明 CC5。手工模式超时 15 分钟，关闭时清理本次临时服务和 worker；没有新增真实后端矩阵。

## 并行与路径交接

与 #73 并行时只推进本任务候选交互；共同类型/import/app 构造段由 #73 顺序集成，随后更新本任务产品 fixture。#74 如变更资格/结果边界，按精确目标、版本和原回执检查受影响接线。本任务先完成再由 #77 迁完整产品/console；交接实际文件与 CC1–CC5 测试/证据映射，不留下两个 TUI 或宽快照兼容路径。

每次执行固定本任务构建与隔离配置，不在另一验收现场中替换可执行文件。下表使用当前目录；#73 纯类型迁移时只改实际依赖/命令，#77 迁 consumer 后同次改成 `internal/console` 等唯一实际路径。

## 测试计划

所有 runner 产物使用新的空目录；非零命中、无 fail/skip 才计通过。早期失败记录留作诊断，不作为通过证据。最后实际结果见[验收记录](../validation/candidate-console/README.md)。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 Go：CC1–CC4，实际顶层 `TestCC1`–`TestCC4` | Windows；console 产品端口 fake，无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-console/view-final-04 -- ./internal/steerline/console -run '^TestCC[1-4]$' -count=1` | 接线与自查修复后 | 五候选一 Task 主行/计数；配置/会话/退出/资源事实同快照；重排、缺页、迟到事件、重开不串草稿/阅读位置；规范缺口不读 raw；blocker 与 Decision 分开，环境动作保持原目标；原请求重读、5个提示、已合并禁止放弃 | view-final-04/summary.json 与 go-test.jsonl |
| 定向 Go：真实应用接线 `TestCandidateConsoleWiring`（含 merged_PR_service_guard） | 临时 Git/SQLite/服务，受控 app-server 与 loopback provider | `go run ./cmd/test-feedback -out docs/validation/candidate-console/wiring-final-03 -- ./internal/steerlineintegration -run '^TestCandidateConsoleWiring$' -count=1` | 默认客户端接通及自查后 | UI 精确新增/反馈/放弃、原请求重读不新增、五候选开始检查、退出/停止分离；真实 provider 合并后服务拒绝放弃；子用例非零命中 | wiring-final-03/summary.json、请求/回执/规范快照原始 JSONL |
| 受影响 console 包回归 | Windows；无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-console/console-final-04 -- ./internal/steerline/console -count=1` | 自查后、最终 L1 前 | 原有布局、决策编辑、分页、身份守卫和本次候选场景通过 | console-final-04/summary.json 与 go-test.jsonl |
| OS 源码/依赖代表 | Windows/amd64 与 Linux/amd64 交叉构建 | `go build -o <本次临时目录>/steerline-windows-amd64.exe ./cmd/steerline`；设置 `GOOS=linux` 后构建同入口，随后恢复变量 | 自查后 | 两种入口编译成功；编译不记作原生终端验收 | build.json（命令/版本/产物 SHA-256） |
| 手工原生 UI：CC5 | 隔离 Windows 真实终端，使用上述同一受控服务保持模式 | 设 `STEERLINE_CC5_MANIFEST=<新证据目录绝对路径>/fixture.json`，运行 `go test ./internal/steerlineintegration -run '^TestCandidateConsoleWiring$' -count=1 -v -timeout=20m`；另一个真实终端从 manifest 读取 home 和 executable，执行 `console -config <manifest.config>`；操作步骤见 windows/README.md | 定向用例通过后，具备实际 IME/键盘/窗口操作能力时 | 中文组词、选项加备注、候选导航、窄窗口、滚动均保留实际操作与截图证据；准备模式、Go KeyMsg、截图或模拟输入均不替代原生通过 | windows/ 原始操作记录/构建配置身份/截图；当前仅 blocker 诊断，**未通过** |
| 授权后准备能力补充 | 同一 Windows 隔离 fixture，无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-console/windows/bindings-regression -- ./internal/steerlineintegration -run '^TestCandidateConsoleWiring$' -count=1`；保持模式按新 manifest 绑定公开 CLI 读取后写原 stop_file | 追加授权与环境绑定修复后 | 普通接线保持通过，外部 CLI 使用三项原绑定可读同一 Task；不声明 native 输入通过 | windows/bindings-regression 与 authorized-bindings-check/ |
| L1 固定最小入口 | runner 自有隔离 STEERLINE_HOME，无原生开关 | `go run ./cmd/test-feedback -level=L1 -out docs/validation/candidate-console/l1-final` | 实现自查与所有定向修复完成后一次 | 固定 quick、产品客户端/边界/UI/守卫代表非零命中且通过 | l1-final/summary.json 和原始日志 |

不选择 Full/L2/L3：变更为候选 TUI 与现有端口接线；真实后端竞争归 #39，版本/24 小时组合归 #42，本次不重启历史任务，不运行全量、真实模型或新的后端矩阵。CC5 缺口不能以这些级别替代。

## 协调复审测试计划

协调同步最新 main 后，修复候选合并冲突展示，并保留 #38 未交付任务的原 Spec/Plan 活动入口。冲突是已提交候选/PR/唯一 Delivery 的只读展示，不生成新的业务状态或胜者。新增 `TestCandidateMergedConflictPresentation`，验证放弃后合并、多个合并、交付后落选合并仍保留准确资源与唯一交付。

以下均在 Windows 隔离环境执行，不请求模型；证据根为 `C:/Users/lmas1/.codex/task-state/roadmap-runner/steerline-candidates-20261004-01a10247/`，每次新空目录：

- 定向 Go / 受影响 console 包（包含 CC1–CC4 与上述冲突场景）：`go run ./cmd/test-feedback -out <证据根>/coordinator38-console-01 -- ./internal/steerline/console -count=1`；修复后执行，要求实际非零命中、无 fail/skip，summary.json 与 Go JSON 保存证据。
- 定向 Go / 实际应用接线：`go run ./cmd/test-feedback -out <证据根>/coordinator38-wiring-final-03 -- ./internal/steerlineintegration -run '^TestCandidateConsoleWiring$' -count=1`；同步及复审后执行，原新增/反馈/放弃/合并守卫断言保持，两个场景非零命中且无 fail/skip；同目录保存报告和真实服务回执。
- 最终 L1：`go run ./cmd/test-feedback -level=L1 -out <证据根>/coordinator38-l1-01`；复审修复稳定后一次，七个固定组必须通过，summary.json 记录源码和构建身份。

CC5 仍需上述真实终端/IME/导航/窄窗/滚动证据。此次 Go 回归不替代它；用户已明确允许先合入并继续 #39 开发，不选择 Full/L2/L3 或扩权限绕过工具产品策略。

协调诊断保留 `coordinator38-wiring-01` 的真实失败：正式反馈 API 在 command 准入前拒绝了短暂不同步的 ready-service/SQLite projection，报 stale_instance。诊断轮 `coordinator38-wiring-diagnostic-02` 保留即时状态诊断；最终接线夹具沿现有原问题 fixture 的规则，只对该明确准入前拒绝模拟人工刷新和显式再提交，最多三次，每次先核对原服务实例、Workflow/Execution/session、问题 ID 与 pending 状态、原轮次未改变。未知结果、目标变化或已消费问题仍立即失败；不得自动补投或改用直接 API 绕过 UI。反馈实际原轮次增加、候选数与合并守卫断言不变。最终使用新的 wiring-final-03 证据目录；L1 的最终生产源码不再变，后续只补该未纳入固定 quick 选择的接线 fixture 验证。

## 用户授权合入与最新主线增量

本协调聊天用户明确原句：“合并#38，标记待人工补验即可继续开发#39”。这是 CC5 延后人工补验的限定合入例外；保留 CC5 原条件和未通过事实，不扩大到 #39 的远端/混合验收。

同步 main 的 PR #211 后，production 的 PR 文本链接解析与测试子进程清理消费者需要定向复核。证据仍使用上述 run 根的新目录，Windows 隔离 Git/SQLite/受控后端、不请求模型：

- `go run ./cmd/test-feedback -out <证据根>/coordinator38-main211-targeted-01 -- ./internal/steerline/console ./internal/steerlineintegration ./internal/steerline/execution ./internal/testprocess -run '^TestCC[1-4]$|^TestCandidateMergedConflictPresentation$|^TestCandidateConsoleWiring$|^TestPRClues|^TestOwnedProcessWaitsForDescendants$|^TestProcessReportsDescendantCleanupFailure$' -count=1`；最新主线集成后执行，要求每个选择包非零命中且无 fail/skip，保持原候选/反馈/合并守卫及真实子进程清理断言；summary.json 和 Go JSON 保存证据。
- `go run ./cmd/test-feedback -level=L1 -out <证据根>/coordinator38-main211-l1-01`；最终内容稳定后一次，七个固定组必须通过，summary.json 记录最终源码/构建身份。
- 文档变更核验 `python scripts/roadmap_task_id.py check` 与 `git diff --check`；任务身份、剩余前置和本次例外记录一致。

不跑 Full/L2/L3；当前额外验证只覆盖主线集成的实际消费者。CC5 待人工状态见 windows/README.md，合并不产生新的原生通过证据。

## 用户交互反馈增量（2026-10-04）

在 `codex/candidate-interaction-ux` 上落实用户要求；当前增量只修改 console 的视图编号、交互页、键位和本地提交显示，服务、数据库合同、精确 request/Decision/session 绑定保持。已有用户 `windows/manual-*` 现场不改动，不启动其他任务。

实际路径：`interaction.go` 根据同任务完整 WorkflowRun.CreatedAt/WorkflowRunID 排序计算编号，并负责交互状态/固定预览；不持久化窗口私有编号；`model.go` / `model_update_project.go` / `model_keys.go` 管理焦点与显示；`view.go` / `view_conversation.go` 简化默认页面，F8/d 保留完整信息；`model_update_actions.go` 标识原回执所属并行。`interaction_test.go` 补 UX1–UX5。

### 测试计划

| 级别/组或场景 | 环境 | 命令 | 时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 Go UX1–UX5、CC1–CC4 与受影响 console | Windows，纯产品端口 fixture，无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-interaction-ux/console-verified -- ./internal/steerline/console -count=1` | 界面与回执路径自查后 | 按同任务触发顺序计算短编号，跨窗口一致且包含退出历史；数量/选择稳定；无选项直接输入；预览在窄窗口可见；F2 返回会话且原问题不重复输入；新问题可交互；精确目标、草稿/缺口/blocker 守卫保持，无 fail/skip | console-verified/summary.json 和原始 JSONL |
| 真实应用接线 `TestCandidateConsoleWiring` | 同一临时 Git/SQLite/受控后端，无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-interaction-ux/wiring-final -- ./internal/steerlineintegration -run '^TestCandidateConsoleWiring$' -count=1` | console 通过后 | 默认控制台实际接线、原会话反馈、不同方案新增/放弃及合并守卫保持 | wiring-final/summary.json 和原始 JSONL |
| L1 固定入口 | runner 隔离 home，无原生开关 | `go run ./cmd/test-feedback -level=L1 -out docs/validation/candidate-interaction-ux/l1` | 增量自查及定向修复完成后一次 | 当前固定最小组非零命中且通过 | l1/summary.json 和原始日志 |
| CC5 人工补验 | 新构建、新的隔离 Windows 终端现场 | 复用正式 manifest 保持模式与三项 fixture 环境绑定，按新 F6/F7、a/Enter、Tab、F8、F2 操作 | 用户具备实际终端操作条件时 | 实际中文组词、选项加备注、并行导航、窄窗与滚动证据；确认提交后退出本次输入；不使用 Go fixture 或旧现场声称通过 | 新的 windows/manual-* 操作记录/截图 |

不选 Full/L2/L3，不另建后端矩阵；增量为输入/呈现流程与原产品接线，原生 CC5 仍需实际人工证据，不能由上述 Go 验证替代。

编号简化补充：UX5 使用与触发顺序不一致的 ID、退出历史、仅可见部分 run 和独立窗口，核验完整任务记录排序；时间相同按 ID 稳定兜底。仅运行受影响 console 代表回归；未改命令合同/应用构造或服务，无需再次重跑真实接线、L1、Full/L2/L3，前次交互增量验证仍保留。
