# Plan：精简候选控制台与会话阅读

绑定 [Spec](product-simplification-01-console.spec.md)，基线 `75474dcece26d807367e106a89a992f01f53fafd`，分支 `codex/product-simplification-console`。仅实施第一批；其他两批规格保留待实施，不改其产品行为。

## 实现

1. 按用户修订恢复三窗：左任务列表，右上只读候选/方案/状态/环境/PR，右下运行与反馈。既有编辑器嵌入右下，Tab循环列表/选项/输入，Enter换行，F2提交。切换前保存原草稿，问题/实例/版本变化不自动改目标；长背景、最终回复和核验仍在简单只读入口。
2. 保留数据库规范事件采集及必要正文读取，使用FinalsOnly收窄只供最终业务回复的查询，自动有界读取完整 final，提供简单历史final选择。准备轮/过程/未知阶段不成为final；迟到正文响应核验项目、候选、事件、版本和读取代次。
3. UI状态格式升为2，沿用原子文件/窗口锁，只保存项目绑定、候选答案草稿和最小动作记录；删除阅读/候选选择偏好。损坏/旧文件保留并阻止新外发，不以空日志覆盖现场；只读布局仍可使用。重开只查原ID，不重发。
4. 接回既有规范Activity查询，仅保留最近10条短摘要与当前身份游标；身份/Attempt变化清空并拒绝旧响应。问题、提交预览和回执优先显示。删除其余退役读取/测试，不恢复时间线/工具正文/折叠；补三窗焦点、编辑中切换/问题变化、摘要缓冲隔离、长final及未知请求/损坏日志回归。更新README、会话/活动文档和Spec状态。
5. 自查消费者与净行数，保存验证/截图和已移除测试映射；提交、推送并开一个独立PR，附到本聊天。

## 测试计划

证据根目录 `docs/validation/product-simplification-01-console/`；每次runner运行写新的子目录。失败保留原日志，不用旧证据填新结果。

| 级别/场景 | 环境、命令 | 时点 | 通过条件 | 证据 |
| --- | --- | --- | --- | --- |
| 定向 PS1-01–04 | Windows隔离临时home，`go run ./cmd/test-feedback -out=<新目录> -- ./internal/console ./internal/product/query ./internal/product/client -count=1` | 改动完成后 | 非零run/pass，候选/正文/草稿不串线，长final完整，未知请求只查、损坏日志不发、不保存偏好 | `three-pane-targeted/summary.json`及日志 |
| 定向应用接线 PS1-01/03 | 临时SQLite/可控后端，`go run ./cmd/test-feedback -out=<新目录> -- ./tests/integration -run '^TestPS1InlineConsoleWiring$' -count=1` | 产品接口接通后 | 当前真实app客户端连接、精确候选命令/原请求/业务边界保持 | `inline-integration-refreshed/summary.json`及日志 |
| 定向SQL PS1-02 | Windows临时SQLite，`go run ./cmd/test-feedback -out=<新目录> -- ./internal/storage/sqlite -run '^Test(PS1RecentActivity|TM05Activity|ConversationFinalFilter|ConversationReader)' -count=1` | 最终回复投影接通后 | 准备及历史准备轮不进入final；通用诊断查询仍有过程，正文/来源边界保持 | `three-pane-sqlite/summary.json`及日志 |
| L1及边界 | Windows，`go run ./cmd/test-feedback -level=L1 -out=<新目录>` | 定向通过后 | passed/complete为true，无零匹配/缺失结果，生产依赖边界保持 | `l1-three-pane/summary.json`及日志 |
| PS1-05终端 | 隔离Windows终端/临时home，当前构建配可控数据，三窗右下中文多行输入、Tab/Enter/F2、F6/F7、窄屏与F8 | 构建与确定性回归通过后 | 实际VT/Unicode输入、焦点和预览正确，切换候选不串答案，窄屏输入及F8可达；问题版本变化用定向测试验证，退役快捷键无入口 | `terminal-three-pane/capture.json`及原始VT记录 |
| 静态检查/构建 | `git diff --check`、`go build ./cmd/steerline`，必要时e2e tag编译 | PR前 | 无格式/编译错误，无退役入口或未更新消费者 | README/消费者清单 |

不选择Full或L2/L3/live：本批不改后端权限、真实PR交付或生命周期；app受控接线与L1覆盖所触及边界。终端验证不启动真实模型/业务发布。新增Windows终端opt-in fixture `TestPS1WindowsTerminalInput`，只注入公开product DTOs并运行实际Bubble Tea终端程序，不创建测试专用生产能力。使用Windows PTY验证真实UTF-8/VT输入；正常L1跳过此人工驱动fixture，不把skip当PS1-05通过。实机IME/被遮挡桌面截图不在该PTY证据覆盖内。

## 格式与风险

UI操作日志格式升为2；conversation查询游标格式升为2以绑定新增FinalsOnly筛选；活动读取复用既有契约；必要业务记录/采集事件与数据库格式保持。若新增final查询属于产品契约，所有生产/测试消费者同次更新，不保留旧转发。最终正文保持现有字节上限和完整性/身份校验；UI旧格式不迁移，服务/数据库不因此清空。退役阅读测试删除，幂等、故障与输入断言保留并映射。
